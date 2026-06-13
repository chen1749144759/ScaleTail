// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build ((linux && !android) || windows || (darwin && !ios) || freebsd) && !ts_omit_cliconndiag

package safesocket

import (
	"strings"

	ps "github.com/mitchellh/go-ps"
)

func init() {
	scaletaildProcExists.Set(func() bool {
		procs, err := ps.Processes()
		if err != nil {
			return false
		}
		for _, proc := range procs {
			name := proc.Executable()
			const scaletaild = "scaletaild"
			if len(name) < len(scaletaild) {
				continue
			}
			// Do case insensitive comparison for Windows,
			// notably, and ignore any ".exe" suffix.
			if strings.EqualFold(name[:len(scaletaild)], scaletaild) {
				return true
			}
		}
		return false
	})
}
