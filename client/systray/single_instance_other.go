// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package systray

import "scaletail.com/client/local"

func NotifyExistingOrStartCommandServer(_ *local.Client) bool {
	return false
}
