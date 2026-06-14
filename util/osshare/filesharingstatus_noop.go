// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package osshare

import (
	"scaletail.com/types/logger"
)

func SetFileSharingEnabled(enabled bool, logf logger.Logf) {}
