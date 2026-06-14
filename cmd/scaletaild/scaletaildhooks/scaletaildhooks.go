// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package scaletaildhooks provides hooks for optional features
// to add to during init that scaletaild calls at runtime.
package scaletaildhooks

import "scaletail.com/feature"

// UninstallSystemDaemonWindows is called when the Windows
// system daemon is uninstalled.
var UninstallSystemDaemonWindows feature.Hooks[func()]
