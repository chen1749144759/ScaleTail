// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package controlurl validates ScaleTail control-plane URLs before credentials
// or connection preferences are sent to the daemon.
package controlurl

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

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
		strings.EqualFold(canonicalHostname(a.Hostname()), canonicalHostname(b.Hostname())) &&
		effectivePort(a) == effectivePort(b)
}

// Origin returns the canonical origin of a validated control or authentication
// URL. Default ports are omitted so equivalent origins have one persisted form.
func Origin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	hostname := canonicalHostname(u.Hostname())
	port := u.Port()
	if port != "" {
		n, _ := strconv.Atoi(port)
		port = strconv.Itoa(n)
	}
	if port == effectivePort(&url.URL{Scheme: scheme}) {
		port = ""
	}
	if port != "" {
		hostname = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	return scheme + "://" + hostname
}

func parseServer(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || canonicalHostname(u.Hostname()) == "" || u.User != nil {
		return nil, errors.New("invalid ScaleTail server URL")
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, errors.New("invalid ScaleTail server port")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid ScaleTail server port")
		}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	switch u.Scheme {
	case "http", "https":
		return u, nil
	default:
		return nil, errors.New("invalid control server URL scheme")
	}
}

func canonicalHostname(hostname string) string {
	return strings.TrimSuffix(strings.ToLower(hostname), ".")
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err == nil {
			return strconv.Itoa(n)
		}
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
