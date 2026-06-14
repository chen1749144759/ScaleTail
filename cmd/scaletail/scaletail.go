// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// The scaletail command is the Tailscale command-line client. It interacts
// with the scaletaild node agent.
package main // import "scaletail.com/cmd/scaletail"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"scaletail.com/cmd/scaletail/cli"
)

func main() {
	args := os.Args[1:]
	if name, _ := os.Executable(); strings.HasSuffix(filepath.Base(name), ".cgi") {
		args = []string{"web", "-cgi"}
	}
	if err := cli.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
