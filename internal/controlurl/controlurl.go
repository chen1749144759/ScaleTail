// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package controlurl validates ScaleTail control-plane URLs before credentials
// or connection preferences are sent to the daemon.
package controlurl

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

// ErrHTTPSRequired reports an attempt to use cleartext HTTP with a remote
// control server. HTTP is allowed only for an explicit loopback host.
var ErrHTTPSRequired = errors.New("remote control server requires HTTPS")

// ParseControl validates and parses a control server base URL.
func ParseControl(raw string) (*url.URL, error) {
	u, err := parseServer(raw)
	if err != nil {
		return nil, err
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" ||
		(u.Path != "" && u.Path != "/") {
		return nil, errors.New("invalid control server URL")
	}
	return u, nil
}

// ParseAuth validates and parses a same-server authentication URL. Its path is
// retained because it carries the one-time registration request identifier.
func ParseAuth(raw string) (*url.URL, error) {
	u, err := parseServer(raw)
	if err != nil {
		return nil, err
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("invalid authentication URL")
	}
	return u, nil
}

// SameOrigin reports whether two validated URLs use the same scheme, host and
// effective port.
func SameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

func parseServer(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || u.Hostname() == "" || u.User != nil {
		return nil, errors.New("invalid ScaleTail server URL")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	switch u.Scheme {
	case "https":
		return u, nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return u, nil
		}
		return nil, ErrHTTPSRequired
	default:
		return nil, errors.New("invalid control server URL scheme")
	}
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Unmap().IsLoopback()
}
