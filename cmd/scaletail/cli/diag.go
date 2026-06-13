// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build (linux || windows || darwin) && !ts_omit_cliconndiag

package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	ps "github.com/mitchellh/go-ps"
	"tailscale.com/version/distro"
)

func init() {
	hookFixScaleTaildConnectError.Set(fixScaleTaildConnectErrorImpl)
}

// fixScaleTaildConnectErrorImpl is called when the local scaletaild has
// been determined unreachable due to the provided origErr value. It
// returns either the same error or a better one to help the user
// understand why scaletaild isn't running for their platform.
func fixScaleTaildConnectErrorImpl(origErr error) error {
	procs, err := ps.Processes()
	if err != nil {
		return fmt.Errorf("failed to connect to local ScaleTaild process and failed to enumerate processes while looking for it")
	}
	var foundProc ps.Process
	for _, proc := range procs {
		base := filepath.Base(proc.Executable())
		if base == "scaletaild" {
			foundProc = proc
			break
		}
		if runtime.GOOS == "darwin" && base == "IPNExtension" {
			foundProc = proc
			break
		}
		if runtime.GOOS == "windows" && strings.EqualFold(base, "scaletaild.exe") {
			foundProc = proc
			break
		}
	}
	if foundProc == nil {
		switch runtime.GOOS {
		case "windows":
			return fmt.Errorf("failed to connect to local scaletaild process; is the ScaleTail service running?")
		case "darwin":
			return fmt.Errorf("failed to connect to local ScaleTail service; is ScaleTail running?")
		case "linux":
			var hint string
			if isSystemdSystem() {
				hint = " (sudo systemctl start scaletaild ?)"
			}
			return fmt.Errorf("failed to connect to local scaletaild; it doesn't appear to be running%s", hint)
		}
		return fmt.Errorf("failed to connect to local scaletaild process; it doesn't appear to be running")
	}
	return fmt.Errorf("failed to connect to local scaletaild (which appears to be running as %v, pid %v). Got error: %w", foundProc.Executable(), foundProc.Pid(), origErr)
}

// isSystemdSystem reports whether the current machine uses systemd
// and in particular whether the systemctl command is available.
func isSystemdSystem() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	switch distro.Get() {
	case distro.QNAP, distro.Gokrazy, distro.Synology, distro.Unraid:
		return false
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}
