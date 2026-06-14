// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package systray

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/tailscale/go-winio"
	"scaletail.com/client/local"
)

const systrayCommand = "open-dashboard"

// NotifyExistingOrStartCommandServer returns true when another systray instance
// accepted the open-dashboard command and this process should exit.
func NotifyExistingOrStartCommandServer(lc *local.Client) bool {
	pipeName := systrayCommandPipeName()
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	conn, err := winio.DialPipeContext(ctx, pipeName)
	cancel()
	if err == nil {
		_, _ = fmt.Fprintln(conn, systrayCommand)
		_ = conn.Close()
		return true
	}

	ln, err := winio.ListenPipe(pipeName, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;WD)",
		InputBufferSize:    4096,
		OutputBufferSize:   4096,
	})
	if err != nil {
		log.Printf("systray command pipe listen: %v", err)
		return false
	}
	go serveSystrayCommands(ln, lc)
	return false
}

func serveSystrayCommands(ln net.Listener, lc *local.Client) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			line, _ := bufio.NewReader(conn).ReadString('\n')
			if strings.TrimSpace(line) == systrayCommand {
				OpenDashboard(lc)
			}
		}()
	}
}

func systrayCommandPipeName() string {
	user := os.Getenv("USERNAME")
	if user == "" {
		user = "default"
	}
	user = regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(user, "_")
	return `\\.\pipe\TailscaleDevSystray-` + user
}
