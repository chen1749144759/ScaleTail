// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build cgo || !darwin

// systray is a minimal Tailscale systray application.
package main

import (
	"flag"
	"time"

	"scaletail.com/client/local"
	"scaletail.com/client/systray"
	"scaletail.com/paths"
)

var socket = flag.String("socket", paths.DefaultScaleTaildSocket(), "scaletaild socket 路径")
var theme = flag.String("theme", "dark", "Tailscale 图标主题：dark, dark:nobg, light, light:nobg")
var openDashboard = flag.Bool("open-dashboard", false, "启动后打开仪表台")

func main() {
	flag.Parse()
	lc := &local.Client{Socket: *socket}
	if systray.NotifyExistingOrStartCommandServer(lc) {
		return
	}
	systray.SetTheme(*theme)
	if *openDashboard {
		go func() {
			time.Sleep(500 * time.Millisecond)
			systray.OpenDashboard(lc)
		}()
	}
	new(systray.Menu).Run(lc)
}
