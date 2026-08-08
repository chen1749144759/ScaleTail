// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package accountauth keeps the current account credential in daemon memory.
// The password is used only inside the encrypted Noise control channel and is
// never written to the ScaleTail state database.
package accountauth

import (
	"net"
	"net/url"
	"strings"
	"sync"
)

const (
	UsernameHeader = "X-ScaleTail-Account"
	PasswordHeader = "X-ScaleTail-Password"
)

var current struct {
	sync.RWMutex
	requestID  uint64
	controlURL string
	username   string
	password   []byte
}

// RequestToken identifies an account authentication request.
type RequestToken uint64

// BeginRequest starts a new authentication request and invalidates every
// previously issued token. This makes the latest started request authoritative,
// regardless of the order in which remote responses arrive.
func BeginRequest() RequestToken {
	current.Lock()
	defer current.Unlock()
	current.requestID++
	return RequestToken(current.requestID)
}

// SetIfCurrentRequest stores a credential only when token still identifies the
// latest authentication request. A successful store consumes the token.
func SetIfCurrentRequest(token RequestToken, controlURL, username, password string) bool {
	controlURL = canonicalControlURL(controlURL)
	username = strings.TrimSpace(username)
	if controlURL == "" || username == "" || password == "" {
		return false
	}

	current.Lock()
	defer current.Unlock()
	if token == 0 || uint64(token) != current.requestID {
		return false
	}
	clear(current.password)
	current.controlURL = controlURL
	current.username = username
	current.password = []byte(password)
	current.requestID++
	return true
}

// Get returns the credential only for the control server that authenticated
// it. This prevents credentials from crossing server or profile boundaries.
func Get(controlURL string) (username, password string, ok bool) {
	current.RLock()
	defer current.RUnlock()
	if current.controlURL == "" || current.controlURL != canonicalControlURL(controlURL) ||
		current.username == "" || len(current.password) == 0 {
		return "", "", false
	}
	return current.username, string(current.password), true
}

func Clear() {
	current.Lock()
	defer current.Unlock()
	clear(current.password)
	current.requestID++
	current.controlURL = ""
	current.username = ""
	current.password = nil
}

func canonicalControlURL(controlURL string) string {
	u, err := url.Parse(strings.TrimSpace(controlURL))
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
		return ""
	}
	u.Scheme = strings.ToLower(u.Scheme)
	hostname := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		u.Host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		u.Host = "[" + hostname + "]"
	} else {
		u.Host = hostname
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u.String()
}
