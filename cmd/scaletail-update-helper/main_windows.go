// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// scaletail-update-helper relaunches the desktop UI after a silent OTA install.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

var markerPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: ScaleTailUpdateHelper signal|wait [flags]")
	}
	switch os.Args[1] {
	case "signal":
		runSignal(os.Args[2:])
	case "wait":
		runWait(os.Args[2:])
	default:
		fatalf("unknown command %q", os.Args[1])
	}
}

func runSignal(args []string) {
	fs := flag.NewFlagSet("signal", flag.ExitOnError)
	markerID := fs.String("marker-id", "", "OTA marker identifier")
	fs.Parse(args)
	marker := markerPath(*markerID)
	if err := os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
		fatalf("create marker directory: %v", err)
	}
	if err := os.WriteFile(marker, []byte("ok\n"), 0644); err != nil {
		fatalf("write marker: %v", err)
	}
}

func runWait(args []string) {
	fs := flag.NewFlagSet("wait", flag.ExitOnError)
	markerID := fs.String("marker-id", "", "OTA marker identifier")
	appPath := fs.String("app", "", "ScaleTail UI executable")
	timeout := fs.Duration("timeout", 10*time.Minute, "maximum wait duration")
	fs.Parse(args)
	defer removeSelfOnReboot()
	marker := markerPath(*markerID)
	app := filepath.Clean(strings.TrimSpace(*appPath))
	if !filepath.IsAbs(app) || !strings.EqualFold(filepath.Ext(app), ".exe") {
		fatalf("invalid application path")
	}

	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			_ = os.Remove(marker)
			launchWhenReady(app, deadline)
			return
		}
		time.Sleep(time.Second)
	}
	// The installer might have failed after closing the old UI. Relaunch any
	// available installation so the user is not left without a desktop entry.
	launchWhenReady(app, time.Now().Add(time.Minute))
}

func removeSelfOnReboot() {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	windows.MoveFileEx(windows.StringToUTF16Ptr(executable), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	windows.MoveFileEx(windows.StringToUTF16Ptr(filepath.Dir(executable)), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
}

func launchWhenReady(appPath string, deadline time.Time) {
	for time.Now().Before(deadline) {
		if info, err := os.Stat(appPath); err == nil && info.Mode().IsRegular() {
			cmd := exec.Command(appPath, "--open-dashboard")
			cmd.Dir = filepath.Dir(appPath)
			cmd.SysProcAttr = &syscall.SysProcAttr{
				HideWindow:    true,
				CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
			}
			if err := cmd.Start(); err == nil {
				_ = cmd.Process.Release()
				return
			}
		}
		time.Sleep(time.Second)
	}
	fatalf("updated ScaleTail UI did not become available")
}

func markerPath(markerID string) string {
	id := strings.ToLower(strings.TrimSpace(markerID))
	if !markerPattern.MatchString(id) {
		fatalf("invalid OTA marker")
	}
	programData := strings.TrimSpace(os.Getenv("ProgramData"))
	if programData == "" {
		fatalf("ProgramData is not configured")
	}
	return filepath.Join(programData, "ScaleTailOTA", id+".done")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
