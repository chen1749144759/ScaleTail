// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package localapi

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestValidateScaleTailControlURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantErr   bool
		wantHTTPS bool
	}{
		{name: "https_domain", url: "https://control.example.com:60090"},
		{name: "http_localhost", url: "http://localhost:8080"},
		{name: "http_loopback_v4", url: "http://127.0.0.9:8080"},
		{name: "http_loopback_v6", url: "http://[::1]:8080"},
		{name: "http_remote_ip", url: "http://192.0.2.10:8080", wantErr: true, wantHTTPS: true},
		{name: "http_remote_domain", url: "http://control.example.com", wantErr: true, wantHTTPS: true},
		{name: "credentials", url: "https://user:pass@control.example.com", wantErr: true},
		{name: "path", url: "https://control.example.com/control", wantErr: true},
		{name: "query", url: "https://control.example.com/?redirect=x", wantErr: true},
		{name: "fragment", url: "https://control.example.com/#fragment", wantErr: true},
		{name: "unsupported_scheme", url: "ftp://control.example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateScaleTailControlURL(tt.url)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("validateScaleTailControlURL(%q): %v", tt.url, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateScaleTailControlURL(%q) succeeded, want error", tt.url)
			}
			if tt.wantHTTPS && err != errScaleTailHTTPSRequired {
				t.Fatalf("validateScaleTailControlURL(%q) error = %v, want %v", tt.url, err, errScaleTailHTTPSRequired)
			}
		})
	}
}

func TestScaleTailAuthIDFromURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		want    string
		wantErr bool
	}{
		{
			name:   "register_path",
			rawURL: "https://control.example.com/register/hskey-authreq-AbCdEfGhIjKlMnOpQrStUvWx",
			want:   "hskey-authreq-AbCdEfGhIjKlMnOpQrStUvWx",
		},
		{name: "query_parameter", rawURL: "https://control.example.com/login?authId=hskey-authreq-AbCdEfGhIjKlMnOpQrStUvWx", wantErr: true},
		{name: "wrong_path", rawURL: "https://control.example.com/auth/hskey-authreq-AbCdEfGhIjKlMnOpQrStUvWx", wantErr: true},
		{name: "missing", rawURL: "https://control.example.com/login", wantErr: true},
		{name: "invalid_characters", rawURL: "https://control.example.com/register/not%2Fvalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatal(err)
			}
			got, err := scaleTailAuthIDFromURL(u)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("scaleTailAuthIDFromURL(%q) = %q, want error", tt.rawURL, got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("scaleTailAuthIDFromURL(%q) = %q, %v; want %q", tt.rawURL, got, err, tt.want)
			}
		})
	}
}

func TestScaleTailPasswordAuthSessionFromBrowseURL(t *testing.T) {
	const authID = "hskey-authreq-AbCdEfGhIjKlMnOpQrStUvWx"
	controlURL, gotAuthID, err := scaleTailPasswordAuthSession(
		"https://control.example.com:443/",
		"https://CONTROL.EXAMPLE.COM/register/"+authID,
		"HTTPS://control.example.com/",
	)
	if err != nil {
		t.Fatalf("scaleTailPasswordAuthSession() error: %v", err)
	}
	if controlURL.Hostname() != "control.example.com" || gotAuthID != authID {
		t.Fatalf("scaleTailPasswordAuthSession() = %q, %q; want control.example.com, %q", controlURL, gotAuthID, authID)
	}

	if _, _, err := scaleTailPasswordAuthSession(
		"https://control.example.com",
		"https://other.example.com/register/"+authID,
		"https://control.example.com",
	); !errors.Is(err, errScaleTailAuthSessionInvalid) {
		t.Fatalf("mismatched AuthURL error = %v; want %v", err, errScaleTailAuthSessionInvalid)
	}

	if _, _, err := scaleTailPasswordAuthSession(
		"https://control.example.com",
		"https://control.example.com/register/"+authID,
		"https://other.example.com",
	); !errors.Is(err, errScaleTailAuthSessionInvalid) {
		t.Fatalf("mismatched persisted control URL error = %v; want %v", err, errScaleTailAuthSessionInvalid)
	}
}

func TestScaleTailPasswordAuthCode(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "code", status: http.StatusForbidden, body: `{"code":"account_disabled"}`, want: passwordAuthAccountDisabled},
		{name: "error_code", status: http.StatusForbidden, body: `{"error_code":"password_expired"}`, want: passwordAuthPasswordExpired},
		{name: "message", status: http.StatusForbidden, body: `{"message":"network-not-assigned"}`, want: passwordAuthNetworkNotAssigned},
		{name: "unauthorized_fallback", status: http.StatusUnauthorized, want: passwordAuthInvalidCredentials},
		{name: "locked_fallback", status: http.StatusLocked, want: passwordAuthAccountLocked},
		{name: "unknown_forbidden", status: http.StatusForbidden, body: `{"message":"denied"}`, want: passwordAuthFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scaleTailPasswordAuthCode(tt.status, []byte(tt.body)); got != tt.want {
				t.Fatalf("scaleTailPasswordAuthCode(%d, %q) = %q, want %q", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestValidScaleTailAccountUsername(t *testing.T) {
	for _, tt := range []struct {
		name     string
		username string
		want     bool
	}{
		{name: "normal", username: "alice@example.com", want: true},
		{name: "empty"},
		{name: "control character", username: "alice\nbob"},
		{name: "too long", username: strings.Repeat("a", maxScaleTailUsernameBytes+1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := validScaleTailAccountUsername(tt.username); got != tt.want {
				t.Fatalf("validScaleTailAccountUsername(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestValidScaleTailAccountPassword(t *testing.T) {
	for _, tt := range []struct {
		name     string
		password string
		want     bool
	}{
		{name: "normal", password: "correct horse battery staple", want: true},
		{name: "unicode", password: "密码-安全", want: true},
		{name: "empty"},
		{name: "too long", password: strings.Repeat("a", maxScaleTailPasswordBytes+1)},
		{name: "newline", password: "line1\nline2"},
		{name: "tab", password: "left\tright"},
		{name: "delete", password: "left\x7fright"},
		{name: "c1 control", password: "left\u0085right"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := validScaleTailAccountPassword(tt.password); got != tt.want {
				t.Fatalf("validScaleTailAccountPassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}

func TestServeStartRejectsLegacyAuthKey(t *testing.T) {
	h := &Handler{PermitWrite: true}
	req := httptest.NewRequest(http.MethodPost, "/localapi/v0/start", strings.NewReader(`{"AuthKey":"hskey-auth-legacy"}`))
	res := httptest.NewRecorder()
	h.serveStart(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", res.Code, http.StatusBadRequest, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "not supported") {
		t.Fatalf("unexpected response: %s", res.Body.String())
	}
}

func TestDecodeLimitedJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"alice","password":"secret"}`))
		var got scaleTailPasswordAuthRequest
		if err := decodeLimitedJSON(httptest.NewRecorder(), req, &got, 128); err != nil {
			t.Fatal(err)
		}
		if got.Username != "alice" || got.Password != "secret" {
			t.Fatalf("decoded request = %#v", got)
		}
	})

	t.Run("too_large", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bytes.Repeat([]byte("x"), 129)))
		var got scaleTailPasswordAuthRequest
		if err := decodeLimitedJSON(httptest.NewRecorder(), req, &got, 128); err == nil {
			t.Fatal("decodeLimitedJSON succeeded, want size error")
		}
	})

	t.Run("unknown_field", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"alice","password":"secret","authKey":"forbidden"}`))
		var got scaleTailPasswordAuthRequest
		if err := decodeLimitedJSON(httptest.NewRecorder(), req, &got, 256); err == nil {
			t.Fatal("decodeLimitedJSON accepted an unknown field")
		}
	})
}
