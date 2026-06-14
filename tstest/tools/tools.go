// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build tools

// This file exists just so `go mod tidy` won't remove
// tool modules from our go.mod.
package tools

import (
	_ "github.com/tailscale/mkctr"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
