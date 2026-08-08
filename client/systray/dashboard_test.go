// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package systray

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"scaletail.com/ipn"
	"scaletail.com/ipn/ipnstate"
	"scaletail.com/util/httpm"
)

type fakeDashboardAccountClient struct {
	events      []string
	statuses    []*ipnstate.Status
	statusIndex int
	startOpts   ipn.Options
	username    string
	password    []byte
	startErr    error
	loginErr    error
	authErr     error
}

func (c *fakeDashboardAccountClient) Start(_ context.Context, opts ipn.Options) error {
	c.events = append(c.events, "start")
	c.startOpts = opts
	return c.startErr
}

func (c *fakeDashboardAccountClient) StartLoginInteractive(context.Context) error {
	c.events = append(c.events, "interactive")
	return c.loginErr
}

func (c *fakeDashboardAccountClient) ScaleTailAuthenticateAccount(_ context.Context, username string, password []byte) error {
	c.events = append(c.events, "authenticate")
	c.username = username
	c.password = append([]byte(nil), password...)
	return c.authErr
}

func (c *fakeDashboardAccountClient) StatusWithoutPeers(context.Context) (*ipnstate.Status, error) {
	c.events = append(c.events, "status")
	if len(c.statuses) == 0 {
		return nil, errors.New("no status configured")
	}
	index := c.statusIndex
	if index >= len(c.statuses) {
		index = len(c.statuses) - 1
	} else {
		c.statusIndex++
	}
	return c.statuses[index], nil
}

func TestConnectDashboardAccountNewNode(t *testing.T) {
	lc := &fakeDashboardAccountClient{statuses: []*ipnstate.Status{
		{BackendState: ipn.NeedsLogin.String(), AuthURL: "https://control.example/machine/auth/password?id=abc"},
		{BackendState: ipn.Running.String()},
	}}
	opts := ipn.Options{UpdatePrefs: &ipn.Prefs{ControlURL: "https://control.example"}}
	status, err := connectDashboardAccount(t.Context(), lc, opts, "alice", []byte("secret"), false)
	if err != nil {
		t.Fatal(err)
	}
	if status.BackendState != ipn.Running.String() {
		t.Fatalf("state = %q, want Running", status.BackendState)
	}
	wantEvents := []string{"start", "interactive", "status", "authenticate", "status"}
	if !reflect.DeepEqual(lc.events, wantEvents) {
		t.Fatalf("events = %v, want %v", lc.events, wantEvents)
	}
	if lc.startOpts.AuthKey != "" {
		t.Fatalf("Start AuthKey = %q, want empty", lc.startOpts.AuthKey)
	}
	if lc.username != "alice" || string(lc.password) != "secret" {
		t.Fatalf("credential = %q/%q", lc.username, lc.password)
	}
}

func TestConnectDashboardAccountExistingNode(t *testing.T) {
	lc := &fakeDashboardAccountClient{statuses: []*ipnstate.Status{{BackendState: ipn.Running.String()}}}
	status, err := connectDashboardAccount(t.Context(), lc, ipn.Options{}, "alice", []byte("secret"), true)
	if err != nil {
		t.Fatal(err)
	}
	if status.BackendState != ipn.Running.String() {
		t.Fatalf("state = %q, want Running", status.BackendState)
	}
	wantEvents := []string{"start", "authenticate", "status"}
	if !reflect.DeepEqual(lc.events, wantEvents) {
		t.Fatalf("events = %v, want %v", lc.events, wantEvents)
	}
}

func TestConnectDashboardAccountDoesNotTreatStartingAsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	lc := &fakeDashboardAccountClient{statuses: []*ipnstate.Status{{BackendState: ipn.Starting.String()}}}
	status, err := connectDashboardAccount(ctx, lc, ipn.Options{}, "alice", []byte("secret"), true)
	if err == nil {
		t.Fatalf("status = %#v; want an error while state is Starting", status)
	}
	if status != nil {
		t.Fatalf("status = %#v, want nil", status)
	}
}

func TestConnectRequestAccountCredential(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantUser string
		wantErr  bool
	}{
		{name: "valid", username: " alice@example.com ", password: "secret", wantUser: "alice@example.com"},
		{name: "missing username", password: "secret", wantErr: true},
		{name: "control in username", username: "alice\nbob", password: "secret", wantErr: true},
		{name: "username too long", username: strings.Repeat("a", 255), password: "secret", wantErr: true},
		{name: "missing password", username: "alice", wantErr: true},
		{name: "password too long in bytes", username: "alice", password: strings.Repeat("密", 25), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := connectRequest{Username: tt.username, Password: tt.password}
			username, password, err := req.accountCredential()
			defer clear(password)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if username != tt.wantUser {
				t.Fatalf("username = %q, want %q", username, tt.wantUser)
			}
			if req.Password != "" {
				t.Fatal("request password was not cleared")
			}
		})
	}
}

func TestConnectRequestControlURLSecurity(t *testing.T) {
	for _, tt := range []struct {
		name    string
		req     connectRequest
		wantErr bool
	}{
		{name: "remote HTTPS", req: connectRequest{ServerIP: "control.example.com", ServerPort: "60090", UseHTTPS: true}},
		{name: "loopback HTTP", req: connectRequest{ServerIP: "127.0.0.1", ServerPort: "60090"}},
		{name: "remote HTTP", req: connectRequest{ServerIP: "control.example.com", ServerPort: "60090"}, wantErr: true},
		{name: "URL credentials", req: connectRequest{ServerIP: "https://user:secret@control.example.com"}, wantErr: true},
		{name: "URL path", req: connectRequest{ServerIP: "https://control.example.com/control"}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.req.controlURL()
			if (err != nil) != tt.wantErr {
				t.Fatalf("controlURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDashboardSecurityHeadersAndSameOrigin(t *testing.T) {
	const panelURL = "http://127.0.0.1:32123"
	ds := &DashboardServer{url: panelURL}
	handler := ds.handler()

	t.Run("normal page fetch", func(t *testing.T) {
		req := dashboardRequest(httpm.GET, panelURL+"/api/ping", "", "", "")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
		}
		for name, want := range map[string]string{
			"Cache-Control":          "no-store",
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "no-referrer",
		} {
			if got := res.Header().Get(name); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
		if !strings.Contains(res.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatalf("missing restrictive CSP: %q", res.Header().Get("Content-Security-Policy"))
		}
	})

	t.Run("reject bad host", func(t *testing.T) {
		req := dashboardRequest(httpm.GET, panelURL+"/api/ping", "", "", "evil.example")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", res.Code)
		}
	})

	for _, origin := range []string{"", "null", "https://evil.example", panelURL + ".evil.example"} {
		t.Run("reject origin "+origin, func(t *testing.T) {
			req := dashboardRequest(httpm.POST, panelURL+"/api/ping", "", origin, "")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body = %s", res.Code, res.Body.String())
			}
		})
	}

	t.Run("allow same origin post", func(t *testing.T) {
		req := dashboardRequest(httpm.POST, panelURL+"/api/ping", "", panelURL, "")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
		}
	})
}

func TestDashboardJSONRequestLimits(t *testing.T) {
	const panelURL = "http://127.0.0.1:32123"
	handler := (&DashboardServer{url: panelURL}).handler()

	t.Run("oversized", func(t *testing.T) {
		body := `{"ID":"` + strings.Repeat("a", maxDashboardJSONBodyBytes) + `"}`
		req := dashboardRequest(httpm.POST, panelURL+"/api/exit-node", body, panelURL, "")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413; body = %s", res.Code, res.Body.String())
		}
	})

	t.Run("wrong content type", func(t *testing.T) {
		req := dashboardRequest(httpm.POST, panelURL+"/api/exit-node", `{}`, panelURL, "")
		req.Header.Set("Content-Type", "text/plain")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415", res.Code)
		}
	})

	t.Run("reject legacy auth key field", func(t *testing.T) {
		req := dashboardRequest(httpm.POST, panelURL+"/api/connect", `{"AuthKey":"legacy"}`, panelURL, "")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", res.Code, res.Body.String())
		}
	})
}

func TestConnectHTMLUsesAccountPasswordWithoutLeakingItIntoPreview(t *testing.T) {
	for _, forbidden := range []string{"AuthKey", "authKey", "--auth-key", "tskey-auth", "浏览器中完成认证"} {
		if strings.Contains(connectHTML, forbidden) {
			t.Errorf("connect HTML still contains %q", forbidden)
		}
	}
	for _, required := range []string{`id="username"`, `id="password"`, `['scaletail','login','--login-server'`, `['scaletail','set'`} {
		if !strings.Contains(connectHTML, required) {
			t.Errorf("connect HTML does not contain %q", required)
		}
	}
	start := strings.Index(connectHTML, "function refreshPreview()")
	end := strings.Index(connectHTML[start:], "function setLocked(")
	if start < 0 || end < 0 {
		t.Fatal("could not locate command preview function")
	}
	previewFunction := connectHTML[start : start+end]
	if strings.Contains(strings.ToLower(previewFunction), "password") {
		t.Fatal("command preview reads the password field")
	}
}

func dashboardRequest(method, target, body, origin, host string) *http.Request {
	req := httptest.NewRequest(method, target, io.NopCloser(strings.NewReader(body)))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if host != "" {
		req.Host = host
	}
	return req
}
