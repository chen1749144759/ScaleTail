// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package jsdeps is a just a list of the packages we import in the
// JavaScript/WASM build, to let us test that our transitive closure of
// dependencies doesn't accidentally grow too large, since binary size
// is more of a concern.
package jsdeps

import (
	_ "bytes"
	_ "context"
	_ "encoding/hex"
	_ "encoding/json"
	_ "fmt"
	_ "log"
	_ "math/rand/v2"
	_ "net"
	_ "strings"
	_ "time"

	_ "golang.org/x/crypto/ssh"
	_ "scaletail.com/control/controlclient"
	_ "scaletail.com/ipn"
	_ "scaletail.com/ipn/ipnserver"
	_ "scaletail.com/net/netaddr"
	_ "scaletail.com/net/netns"
	_ "scaletail.com/net/tsdial"
	_ "scaletail.com/safesocket"
	_ "scaletail.com/tailcfg"
	_ "scaletail.com/types/logger"
	_ "scaletail.com/wgengine"
	_ "scaletail.com/wgengine/netstack"
	_ "scaletail.com/words"
)
