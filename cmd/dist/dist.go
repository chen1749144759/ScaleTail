// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// The dist command builds Tailscale release packages for distribution.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

	"scaletail.com/release/dist"
	"scaletail.com/release/dist/cli"
	"scaletail.com/release/dist/unixpkgs"
)

func getTargets() ([]dist.Target, error) {
	var ret []dist.Target

	ret = append(ret, unixpkgs.Targets(unixpkgs.Signers{})...)
	return ret, nil
}

func main() {
	cmd := cli.CLI(getTargets)
	if err := cmd.ParseAndRun(context.Background(), os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		log.Fatal(err)
	}
}
