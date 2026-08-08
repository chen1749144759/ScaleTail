// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package controlurl

import (
	"errors"
	"testing"
)

func TestParseControl(t *testing.T) {
	for _, tt := range []struct {
		name      string
		raw       string
		wantErr   bool
		wantHTTPS bool
	}{
		{name: "https domain", raw: "https://control.example.com:60090"},
		{name: "http localhost", raw: "http://localhost:8080"},
		{name: "http localhost dot", raw: "http://localhost.:8080"},
		{name: "http loopback v4", raw: "http://127.0.0.9:8080"},
		{name: "http loopback v6", raw: "http://[::1]:8080"},
		{name: "http remote IP", raw: "http://192.0.2.10:8080", wantErr: true, wantHTTPS: true},
		{name: "http remote domain", raw: "http://control.example.com", wantErr: true, wantHTTPS: true},
		{name: "credentials", raw: "https://user:pass@control.example.com", wantErr: true},
		{name: "path", raw: "https://control.example.com/control", wantErr: true},
		{name: "encoded path", raw: "https://control.example.com/%2f", wantErr: true},
		{name: "query", raw: "https://control.example.com/?redirect=x", wantErr: true},
		{name: "fragment", raw: "https://control.example.com/#fragment", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://control.example.com", wantErr: true},
		{name: "missing host", raw: "https:///control.example.com", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseControl(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseControl(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if tt.wantHTTPS && !errors.Is(err, ErrHTTPSRequired) {
				t.Fatalf("ParseControl(%q) error = %v, want %v", tt.raw, err, ErrHTTPSRequired)
			}
		})
	}
}

func TestSameOrigin(t *testing.T) {
	a, err := ParseControl("https://CONTROL.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseAuth("https://control.example.com:443/register/request")
	if err != nil {
		t.Fatal(err)
	}
	if !SameOrigin(a, b) {
		t.Fatal("equivalent default and explicit ports were not treated as the same origin")
	}
}
