// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package systray

import (
	"log"

	"scaletail.com/client/local"
)

// OpenDashboard starts the dashboard HTTP server and opens it in the browser.
func OpenDashboard(lc *local.Client) {
	url, err := StartDashboard(lc)
	if err != nil {
		log.Printf("dashboard: %v", err)
		return
	}
	log.Printf("dashboard: opening window %s", url)
	if err := openDesktopWindow(url); err != nil {
		log.Printf("dashboard: open window: %v", err)
	}
}

// OpenConnectWindow starts the local control panel and opens the server
// configuration window.
func OpenConnectWindow(lc *local.Client) {
	url, err := StartConnectWindow(lc)
	if err != nil {
		log.Printf("connect window: %v", err)
		return
	}
	log.Printf("connect window: opening %s", url)
	if err := openDesktopWindow(url); err != nil {
		log.Printf("connect window: open: %v", err)
	}
}
