// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package controlclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"scaletail.com/hostinfo"
	"scaletail.com/ipn/ipnstate"
	"scaletail.com/net/netmon"
	"scaletail.com/net/tsdial"
	"scaletail.com/tailcfg"
	"scaletail.com/types/key"
	"scaletail.com/types/persist"
	"scaletail.com/util/eventbus/eventbustest"
)

func TestSetDiscoPublicKey(t *testing.T) {
	initialKey := key.NewDisco().Public()

	c := &Direct{
		discoPubKey: initialKey,
	}

	c.mu.Lock()
	if c.discoPubKey != initialKey {
		t.Fatalf("initial disco key mismatch: got %v, want %v", c.discoPubKey, initialKey)
	}
	c.mu.Unlock()

	newKey := key.NewDisco().Public()
	c.SetDiscoPublicKey(newKey)

	c.mu.Lock()
	if c.discoPubKey != newKey {
		t.Fatalf("disco key not updated: got %v, want %v", c.discoPubKey, newKey)
	}
	if c.discoPubKey == initialKey {
		t.Fatal("disco key should have changed")
	}
	c.mu.Unlock()
}

func TestHTTPNoiseKeyTOFU(t *testing.T) {
	firstKey := key.NewMachine().Public()
	replacementKey := key.NewMachine().Public()
	p := new(persist.Persist)

	if err := checkOrSetHTTPNoiseKeyPin("http://CONTROL.EXAMPLE:80/", firstKey, p); err != nil {
		t.Fatalf("first HTTP pin: %v", err)
	}
	if p.ControlServerNoiseKeyOrigin != "http://control.example" || p.ControlServerNoiseKey != firstKey {
		t.Fatalf("first HTTP pin = %q, %v", p.ControlServerNoiseKeyOrigin, p.ControlServerNoiseKey)
	}
	if err := checkOrSetHTTPNoiseKeyPin("http://control.example", firstKey, p); err != nil {
		t.Fatalf("same HTTP key: %v", err)
	}
	if err := checkOrSetHTTPNoiseKeyPin("http://control.example", replacementKey, p); err == nil || !strings.Contains(err.Error(), "Noise key changed") {
		t.Fatalf("replacement HTTP key error = %v", err)
	}
	if err := checkOrSetHTTPNoiseKeyPin("http://other.example:60090", replacementKey, p); err != nil {
		t.Fatalf("different HTTP origin: %v", err)
	}
	if p.ControlServerNoiseKeyOrigin != "http://other.example:60090" || p.ControlServerNoiseKey != replacementKey {
		t.Fatalf("different origin pin = %q, %v", p.ControlServerNoiseKeyOrigin, p.ControlServerNoiseKey)
	}
}

func TestHTTPSDoesNotDependOnNoiseKeyTOFU(t *testing.T) {
	pinnedKey := key.NewMachine().Public()
	p := &persist.Persist{
		ControlServerNoiseKeyOrigin: "http://control.example",
		ControlServerNoiseKey:       pinnedKey,
	}
	if err := checkOrSetHTTPNoiseKeyPin("https://control.example", key.NewMachine().Public(), p); err != nil {
		t.Fatalf("HTTPS key check: %v", err)
	}
	if p.ControlServerNoiseKeyOrigin != "http://control.example" || p.ControlServerNoiseKey != pinnedKey {
		t.Fatal("HTTPS unexpectedly changed the HTTP TOFU pin")
	}
}

func TestLoadServerPubKeysRejectsCrossOriginRedirect(t *testing.T) {
	keyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tailcfg.OverTLSPublicKeyResponse{PublicKey: key.NewMachine().Public()})
	}))
	defer keyServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, keyServer.URL+"/key", http.StatusFound)
	}))
	defer redirectServer.Close()

	_, err := loadServerPubKeys(context.Background(), http.DefaultClient, redirectServer.URL)
	if err == nil || !strings.Contains(err.Error(), "different origin") {
		t.Fatalf("loadServerPubKeys cross-origin redirect error = %v", err)
	}
}

func TestNewDirect(t *testing.T) {
	hi := hostinfo.New()
	ni := tailcfg.NetInfo{LinkType: "wired"}
	hi.NetInfo = &ni
	bus := eventbustest.NewBus(t)

	k := key.NewMachine()
	dialer := tsdial.NewDialer(netmon.NewStatic())
	dialer.SetBus(bus)
	opts := Options{
		ServerURL: "https://example.com",
		Hostinfo:  hi,
		GetMachinePrivateKey: func() (key.MachinePrivate, error) {
			return k, nil
		},
		Dialer: dialer,
		Bus:    bus,
	}
	c, err := NewDirect(opts)
	if err != nil {
		t.Fatal(err)
	}

	if c.serverURL != opts.ServerURL {
		t.Errorf("c.serverURL got %v want %v", c.serverURL, opts.ServerURL)
	}

	// hi is stored without its NetInfo field.
	hiWithoutNi := *hi
	hiWithoutNi.NetInfo = nil
	if !hiWithoutNi.Equal(c.hostinfo) {
		t.Errorf("c.hostinfo got %v want %v", c.hostinfo, hi)
	}

	changed := c.SetNetInfo(&ni)
	if changed {
		t.Errorf("c.SetNetInfo(ni) want false got %v", changed)
	}
	ni = tailcfg.NetInfo{LinkType: "wifi"}
	changed = c.SetNetInfo(&ni)
	if !changed {
		t.Errorf("c.SetNetInfo(ni) want true got %v", changed)
	}

	changed = c.SetHostinfo(hi)
	if changed {
		t.Errorf("c.SetHostinfo(hi) want false got %v", changed)
	}
	hi = hostinfo.New()
	hi.Hostname = "different host name"
	changed = c.SetHostinfo(hi)
	if !changed {
		t.Errorf("c.SetHostinfo(hi) want true got %v", changed)
	}

	endpoints := fakeEndpoints(1, 2, 3)
	changed = c.newEndpoints(endpoints)
	if !changed {
		t.Errorf("c.newEndpoints want true got %v", changed)
	}
	changed = c.newEndpoints(endpoints)
	if changed {
		t.Errorf("c.newEndpoints want false got %v", changed)
	}
	endpoints = fakeEndpoints(4, 5, 6)
	changed = c.newEndpoints(endpoints)
	if !changed {
		t.Errorf("c.newEndpoints want true got %v", changed)
	}
}

func fakeEndpoints(ports ...uint16) (ret []tailcfg.Endpoint) {
	for _, port := range ports {
		ret = append(ret, tailcfg.Endpoint{
			Addr: netip.AddrPortFrom(netip.Addr{}, port),
		})
	}
	return
}

func TestParseRateLimitError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		retryAfter string // Retry-After header value
		wantMsg    string
		wantMin    time.Duration // minimum expected retryAfter
		wantMax    time.Duration // maximum expected retryAfter
	}{
		{
			name:       "retry-after-seconds",
			statusCode: 429,
			body:       "too many requests",
			retryAfter: "30",
			wantMsg:    "too many requests",
			wantMin:    30 * time.Second,
			wantMax:    30 * time.Second,
		},
		{
			name:       "no-retry-after-header",
			statusCode: 429,
			body:       "slow down",
			retryAfter: "",
			wantMsg:    "slow down",
			wantMin:    5 * time.Second,
			wantMax:    10 * time.Second,
		},
		{
			name:       "unparseable-retry-after",
			statusCode: 429,
			body:       "rate limited",
			retryAfter: "not-a-number",
			wantMsg:    "rate limited",
			wantMin:    5 * time.Second,
			wantMax:    10 * time.Second,
		},
		{
			name:       "empty-body",
			statusCode: 429,
			body:       "",
			retryAfter: "5",
			wantMsg:    "",
			wantMin:    5 * time.Second,
			wantMax:    5 * time.Second,
		},
		{
			name:       "body-with-whitespace",
			statusCode: 429,
			body:       "  too many requests  \n",
			retryAfter: "10",
			wantMsg:    "too many requests",
			wantMin:    10 * time.Second,
			wantMax:    10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if tt.retryAfter != "" {
				rec.Header().Set("Retry-After", tt.retryAfter)
			}
			rec.WriteHeader(tt.statusCode)
			rec.Body.WriteString(tt.body)
			res := rec.Result()

			err := parseRateLimitError(res)
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			var rle *rateLimitError
			if !errors.As(err, &rle) {
				t.Fatalf("error is not a *rateLimitError: %T", err)
			}
			if rle.msg != tt.wantMsg {
				t.Errorf("msg = %q, want %q", rle.msg, tt.wantMsg)
			}
			if rle.retryAfter < tt.wantMin || rle.retryAfter > tt.wantMax {
				t.Errorf("retryAfter = %v, want between %v and %v", rle.retryAfter, tt.wantMin, tt.wantMax)
			}

			// Verify the Error() string contains useful information.
			errStr := err.Error()
			if !strings.Contains(errStr, "rate limited") {
				t.Errorf("Error() = %q, want it to contain 'rate limited'", errStr)
			}
		})
	}
}

func TestRateLimitErrorIsError(t *testing.T) {
	err := &rateLimitError{msg: "test", retryAfter: 5 * time.Second}
	var target *rateLimitError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should match *rateLimitError")
	}
	if target.retryAfter != 5*time.Second {
		t.Errorf("retryAfter = %v, want 5s", target.retryAfter)
	}
}

func TestTsmpPing(t *testing.T) {
	hi := hostinfo.New()
	ni := tailcfg.NetInfo{LinkType: "wired"}
	hi.NetInfo = &ni
	bus := eventbustest.NewBus(t)

	k := key.NewMachine()
	dialer := tsdial.NewDialer(netmon.NewStatic())
	dialer.SetBus(bus)
	opts := Options{
		ServerURL: "https://example.com",
		Hostinfo:  hi,
		GetMachinePrivateKey: func() (key.MachinePrivate, error) {
			return k, nil
		},
		Dialer: dialer,
		Bus:    bus,
	}

	c, err := NewDirect(opts)
	if err != nil {
		t.Fatal(err)
	}

	pingRes := &tailcfg.PingResponse{
		Type:     "TSMP",
		IP:       "123.456.7890",
		Err:      "",
		NodeName: "testnode",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body := new(ipnstate.PingResult)
		if err := json.NewDecoder(r.Body).Decode(body); err != nil {
			t.Fatal(err)
		}
		if pingRes.IP != body.IP {
			t.Fatalf("PingResult did not have the correct IP : got %v, expected : %v", body.IP, pingRes.IP)
		}
		w.WriteHeader(200)
	}))
	defer ts.Close()

	now := time.Now()

	pr := &tailcfg.PingRequest{
		URL: ts.URL,
	}

	err = postPingResult(now, t.Logf, c.httpc, pr, pingRes)
	if err != nil {
		t.Fatal(err)
	}
}
