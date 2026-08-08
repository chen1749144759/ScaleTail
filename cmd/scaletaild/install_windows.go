// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build go1.19

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"scaletail.com/cmd/scaletaild/scaletaildhooks"
)

func init() {
	installSystemDaemon = installSystemDaemonWindows
	uninstallSystemDaemon = uninstallSystemDaemonWindows
}

func installSystemDaemonWindows(args []string) (err error) {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to Windows service manager: %v", err)
	}
	defer m.Disconnect()

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	c := mgr.Config{
		ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		DisplayName:  serviceName,
		Description:  "Connects this computer to the ScaleTail network.",
	}

	service, err := m.OpenService(serviceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		service, err = m.CreateService(serviceName, exe, c)
		if err != nil {
			return fmt.Errorf("failed to create %q service: %v", serviceName, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to open %q service: %v", serviceName, err)
	} else {
		existing, err := service.Config()
		if err != nil {
			service.Close()
			return fmt.Errorf("failed to read %q service configuration: %v", serviceName, err)
		}
		existing.ServiceType = c.ServiceType
		existing.StartType = c.StartType
		existing.ErrorControl = c.ErrorControl
		existing.BinaryPathName = exe
		existing.DisplayName = c.DisplayName
		existing.Description = c.Description
		if err := service.UpdateConfig(existing); err != nil {
			service.Close()
			return fmt.Errorf("failed to update %q service configuration: %v", serviceName, err)
		}
	}
	defer service.Close()

	// Exponential backoff is often too aggressive, so use (mostly)
	// squares instead.
	ra := []mgr.RecoveryAction{
		{mgr.ServiceRestart, 1 * time.Second},
		{mgr.ServiceRestart, 2 * time.Second},
		{mgr.ServiceRestart, 4 * time.Second},
		{mgr.ServiceRestart, 9 * time.Second},
		{mgr.ServiceRestart, 16 * time.Second},
		{mgr.ServiceRestart, 25 * time.Second},
		{mgr.ServiceRestart, 36 * time.Second},
		{mgr.ServiceRestart, 49 * time.Second},
		{mgr.ServiceRestart, 64 * time.Second},
	}
	const resetPeriodSecs = 60
	err = service.SetRecoveryActions(ra, resetPeriodSecs)
	if err != nil {
		return fmt.Errorf("failed to set service recovery actions: %v", err)
	}

	return nil
}

func uninstallSystemDaemonWindows(args []string) (ret error) {
	for _, f := range scaletaildhooks.UninstallSystemDaemonWindows {
		f()
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to Windows service manager: %v", err)
	}
	defer m.Disconnect()

	var firstErr error
	for _, name := range []string{serviceName} {
		if err := uninstallWindowsService(m, name); err != nil {
			if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return nil
}

func uninstallWindowsService(m *mgr.Mgr, name string) error {
	service, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("failed to open %q service: %w", name, err)
	}

	st, err := service.Query()
	if err != nil {
		service.Close()
		return fmt.Errorf("failed to query %q service state: %v", name, err)
	}
	if st.State != svc.Stopped {
		service.Control(svc.Stop)
	}
	err = service.Delete()
	service.Close()
	if err != nil {
		return fmt.Errorf("failed to delete %q service: %v", name, err)
	}

	end := time.Now().Add(15 * time.Second)
	for {
		service, err = m.OpenService(name)
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		if err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
			return fmt.Errorf("checking whether %q service was deleted: %w", name, err)
		}
		if err == nil {
			service.Close()
		}
		if !time.Now().Before(end) {
			return fmt.Errorf("timed out waiting for %q service deletion", name)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
