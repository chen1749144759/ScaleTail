// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !linux || ts_omit_systray

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/peterbourgon/ff/v3/ffcli"
)

// TODO(will): update URL to KB article when available
var systrayHelp = strings.TrimSpace(`
The ScaleTail systray app is not included in this client build.
Install a ScaleTail build that includes Linux desktop support to use it.
`)

var systrayCmd = &ffcli.Command{
	Name:       "systray",
	ShortUsage: "scaletail systray",
	ShortHelp:  "Not available in this client build",
	LongHelp:   hidden + systrayHelp,
	Exec: func(_ context.Context, _ []string) error {
		fmt.Println(systrayHelp)
		return nil
	},
}
