// Copyright (c) ScaleTail Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package localapi

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

var legacyUploadThrottleCleanup struct {
	sync.Mutex
	done bool
}

func clearLegacyUploadThrottle() error {
	legacyUploadThrottleCleanup.Lock()
	defer legacyUploadThrottleCleanup.Unlock()
	if legacyUploadThrottleCleanup.done {
		return nil
	}

	const script = `$policy = Get-NetQosPolicy -Name 'ScaleTail-UploadThrottle' -PolicyStore ActiveStore -ErrorAction SilentlyContinue; if ($policy) { Remove-NetQosPolicy -Name 'ScaleTail-UploadThrottle' -PolicyStore ActiveStore -Confirm:$false -ErrorAction Stop }`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	legacyUploadThrottleCleanup.done = true
	return nil
}
