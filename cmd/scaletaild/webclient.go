// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !ts_omit_webclient

package main

import (
	"scaletail.com/client/local"
	"scaletail.com/ipn/ipnlocal"
	"scaletail.com/paths"
)

func init() {
	hookConfigureWebClient.Set(func(lb *ipnlocal.LocalBackend) {
		lb.ConfigureWebClient(&local.Client{
			Socket:        args.socketpath,
			UseSocketOnly: args.socketpath != paths.DefaultScaleTaildSocket(),
		})
	})
}
