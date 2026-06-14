// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build for_go_mod_tidy_only

package gokrazydeps

import (
	_ "github.com/gokrazy/gokrazy/cmd/dhcp"
	_ "github.com/gokrazy/serial-busybox"
	_ "github.com/tailscale/gokrazy-kernel"
	_ "github.com/tailscale/ts-gokrazy/gokrazyinit"
	_ "scaletail.com/cmd/scaletail"
	_ "scaletail.com/cmd/scaletaild"
	_ "scaletail.com/cmd/tta"
)
