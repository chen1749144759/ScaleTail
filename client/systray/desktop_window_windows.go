// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package systray

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/toqueteos/webbrowser"
)

func openDesktopWindow(url string) error {
	for _, browser := range browserCandidates() {
		if browser == "" {
			continue
		}
		if err := startAppMode(browser, url); err == nil {
			return nil
		}
	}
	return webbrowser.Open(url)
}

func startAppMode(browser, url string) error {
	if !filepath.IsAbs(browser) {
		resolved, err := exec.LookPath(browser)
		if err != nil {
			return err
		}
		browser = resolved
	} else if _, err := os.Stat(browser); err != nil {
		return err
	}

	cmd := exec.Command(browser,
		"--app="+url,
		"--user-data-dir="+panelBrowserUserDataDir(),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-component-extensions-with-background-pages",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func panelBrowserUserDataDir() string {
	base := os.Getenv("LocalAppData")
	if base == "" {
		if d, err := os.UserCacheDir(); err == nil {
			base = d
		} else {
			base = os.TempDir()
		}
	}
	dir := filepath.Join(base, "Tailscale Dev", "PanelBrowser")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

func browserCandidates() []string {
	var candidates []string
	for _, name := range []string{"msedge.exe", "chrome.exe"} {
		candidates = append(candidates, name)
	}
	add := func(base, rel string) {
		if base != "" {
			candidates = append(candidates, filepath.Join(base, rel))
		}
	}
	add(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`)
	add(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`)
	add(os.Getenv("LocalAppData"), `Microsoft\Edge\Application\msedge.exe`)
	add(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`)
	add(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`)
	add(os.Getenv("LocalAppData"), `Google\Chrome\Application\chrome.exe`)
	return candidates
}
