// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package paths returns platform and user-specific default paths to
// Tailscale files and directories.
package paths

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"tailscale.com/syncs"
	"tailscale.com/version/distro"
)

// AppSharedDir is a string set by the iOS or Android app on start
// containing a directory we can read/write in.
var AppSharedDir syncs.AtomicValue[string]

// DefaultScaleTaildSocket returns the path to the scaletaild Unix socket
// or the empty string if there's no reasonable default.
func DefaultScaleTaildSocket() string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\ProtectedPrefix\Administrators\ScaleTail\scaletaild`
	}
	if runtime.GOOS == "darwin" {
		return "/var/run/scaletaild.socket"
	}
	if runtime.GOOS == "plan9" {
		return "/srv/scaletaild.sock"
	}
	switch distro.Get() {
	case distro.Synology:
		if distro.DSMVersion() == 6 {
			return "/var/packages/ScaleTail/etc/scaletaild.sock"
		}
		// DSM 7 (and higher? or failure to detect.)
		return "/var/packages/ScaleTail/var/scaletaild.sock"
	case distro.Gokrazy:
		return "/perm/scaletaild/scaletaild.sock"
	case distro.QNAP:
		return "/tmp/scaletail/scaletaild.sock"
	}
	if fi, err := os.Stat("/var/run"); err == nil && fi.IsDir() {
		return "/var/run/scaletail/scaletaild.sock"
	}
	return "scaletaild.sock"
}

// Overridden in init by OS-specific files.
var (
	stateFileFunc func() string

	// ensureStateDirPerms applies a restrictive ACL/chmod
	// to the provided directory.
	ensureStateDirPerms = func(string) error { return nil }
)

// DefaultScaleTaildStateFile returns the default path to the
// scaletaild state file, or the empty string if there's no reasonable
// default value.
func DefaultScaleTaildStateFile() string {
	if f := stateFileFunc; f != nil {
		return f()
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "ScaleTail", "server-state.conf")
	}
	return ""
}

// DefaultScaleTaildStateDir returns the default state directory
// to use for scaletaild, for use when the user provided neither
// a state directory or state file path to use.
//
// It returns the empty string if there's no reasonable default.
func DefaultScaleTaildStateDir() string {
	if runtime.GOOS == "plan9" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("failed to get home directory: %v", err)
		}
		return filepath.Join(home, "scaletail-state")
	}
	return filepath.Dir(DefaultScaleTaildStateFile())
}

// MakeAutomaticStateDir reports whether the platform
// automatically creates the state directory for scaletaild
// when it's absent.
func MakeAutomaticStateDir() bool {
	switch runtime.GOOS {
	case "plan9":
		return true
	case "linux":
		if distro.Get() == distro.JetKVM {
			return true
		}
	}
	return false
}

// MkStateDir ensures that dirPath, the daemon's configuration directory
// containing machine keys etc, both exists and has the correct permissions.
// We want it to only be accessible to the user the daemon is running under.
func MkStateDir(dirPath string) error {
	if err := os.MkdirAll(dirPath, 0700); err != nil {
		return err
	}
	return ensureStateDirPerms(dirPath)
}
