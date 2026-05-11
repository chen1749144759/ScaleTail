// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package systray

import "github.com/toqueteos/webbrowser"

func openDesktopWindow(url string) error {
	return webbrowser.Open(url)
}
