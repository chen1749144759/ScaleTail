// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package controlurl

import (
	"testing"
)

func TestParseControl(t *testing.T) {
	for _, tt := range []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "https domain", raw: "https://control.example.com:60090"},
		{name: "http localhost", raw: "http://localhost:8080"},
		{name: "http localhost dot", raw: "http://localhost.:8080"},
		{name: "http loopback v4", raw: "http://127.0.0.9:8080"},
		{name: "http loopback v6", raw: "http://[::1]:8080"},
		{name: "http remote IP", raw: "http://192.0.2.10:8080"},
		{name: "http remote domain", raw: "http://control.example.com"},
		{name: "credentials", raw: "https://user:pass@control.example.com", wantErr: true},
		{name: "http credentials", raw: "http://user:pass@control.example.com", wantErr: true},
		{name: "path", raw: "https://control.example.com/control", wantErr: true},
		{name: "http path", raw: "http://control.example.com/control", wantErr: true},
		{name: "encoded path", raw: "https://control.example.com/%2f", wantErr: true},
		{name: "query", raw: "https://control.example.com/?redirect=x", wantErr: true},
		{name: "http query", raw: "http://control.example.com/?redirect=x", wantErr: true},
		{name: "fragment", raw: "https://control.example.com/#fragment", wantErr: true},
		{name: "http fragment", raw: "http://control.example.com/#fragment", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://control.example.com", wantErr: true},
		{name: "missing host", raw: "https:///control.example.com", wantErr: true},
		{name: "empty canonical host", raw: "http://./", wantErr: true},
		{name: "empty port", raw: "http://control.example.com:", wantErr: true},
		{name: "zero port", raw: "http://control.example.com:0", wantErr: true},
		{name: "port too large", raw: "http://control.example.com:65536", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseControl(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseControl(%q) error = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
		})
	}
}

func TestOrigin(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want string
	}{
		{raw: "HTTP://CONTROL.EXAMPLE:80/", want: "http://control.example"},
		{raw: "http://control.example.:80", want: "http://control.example"},
		{raw: "http://control.example:60090", want: "http://control.example:60090"},
		{raw: "http://control.example:080", want: "http://control.example"},
		{raw: "https://CONTROL.EXAMPLE:443/register/request", want: "https://control.example"},
		{raw: "http://[2001:DB8::1]:80", want: "http://[2001:db8::1]"},
	} {
		u, err := parseServer(tt.raw)
		if err != nil {
			t.Fatalf("parseServer(%q): %v", tt.raw, err)
		}
		if got := Origin(u); got != tt.want {
			t.Errorf("Origin(%q) = %q, want %q", tt.raw, got, tt.want)
		}
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
