// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package cli

import (
	"context"

	"github.com/peterbourgon/ff/v3/ffcli"
)

var loginArgs upArgsT
var loginFlagSet = newUpFlagSet(effectiveGOOS(), &loginArgs, "login")

var loginCmd = &ffcli.Command{
	Name:       "login",
	ShortUsage: "scaletail login [flags]",
	ShortHelp:  "Log in with a ScaleForge account",
	LongHelp: `"scaletail login" logs this machine in to a ScaleTail network with
the same username and password used by ScaleForge. Passwords are prompted for
without echo; unattended systems must use a private --password-file.`,
	FlagSet: loginFlagSet,
	Exec: func(ctx context.Context, args []string) error {
		return runUp(ctx, "login", args, loginArgs, loginFlagSet)
	},
}
