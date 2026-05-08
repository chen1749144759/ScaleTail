// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package systray

import (
	"log"

	"github.com/toqueteos/webbrowser"
	"tailscale.com/client/local"
)

// OpenDashboard starts the dashboard HTTP server and opens it in the browser.
func OpenDashboard(lc *local.Client) {
	url, err := StartDashboard(lc)
	if err != nil {
		log.Printf("dashboard: %v", err)
		return
	}
	log.Printf("dashboard: opening %s", url)
	if err := webbrowser.Open(url); err != nil {
		log.Printf("dashboard: open browser: %v", err)
	}
}
