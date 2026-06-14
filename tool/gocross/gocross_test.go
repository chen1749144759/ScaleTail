// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"scaletail.com/tstest/deptest"
)

func TestDeps(t *testing.T) {
	deptest.DepChecker{
		BadDeps: map[string]string{
			"scaletail.com/tailcfg": "circular dependency via go generate",
			"scaletail.com/version": "circular dependency via go generate",
		},
	}.Check(t)
}
