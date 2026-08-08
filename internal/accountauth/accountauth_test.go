// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package accountauth

import "testing"

func TestCredentialLifecycle(t *testing.T) {
	Clear()
	t.Cleanup(Clear)

	if _, _, ok := Get("https://control.example"); ok {
		t.Fatal("empty credential store unexpectedly returned a credential")
	}

	request := BeginRequest()
	if !SetIfCurrentRequest(request, "https://control.example/", "alice", "first password") {
		t.Fatal("SetIfCurrentRequest rejected the latest request")
	}
	username, password, ok := Get("https://control.example")
	if !ok || username != "alice" || password != "first password" {
		t.Fatalf("Get() = %q, %q, %v", username, password, ok)
	}
	if _, _, ok := Get("https://other.example"); ok {
		t.Fatal("credential leaked to another control server")
	}
	if username, password, ok := Get("HTTPS://CONTROL.EXAMPLE:443/"); !ok || username != "alice" || password != "first password" {
		t.Fatalf("Get() did not canonicalize the control URL: %q, %q, %v", username, password, ok)
	}

	invalidRequest := BeginRequest()
	if SetIfCurrentRequest(invalidRequest, "not a URL", "mallory", "replacement password") {
		t.Fatal("SetIfCurrentRequest accepted an invalid control URL")
	}
	if username, password, ok := Get("https://control.example"); !ok || username != "alice" || password != "first password" {
		t.Fatalf("invalid replacement destroyed the current credential: %q, %q, %v", username, password, ok)
	}

	replacementRequest := BeginRequest()
	if !SetIfCurrentRequest(replacementRequest, "https://control.example", "bob", "second password") {
		t.Fatal("SetIfCurrentRequest rejected the latest replacement")
	}
	username, password, ok = Get("https://control.example/")
	if !ok || username != "bob" || password != "second password" {
		t.Fatalf("Get() after replacement = %q, %q, %v", username, password, ok)
	}

	Clear()
	if _, _, ok := Get("https://control.example"); ok {
		t.Fatal("Clear did not remove the credential")
	}
	if SetIfCurrentRequest(replacementRequest, "https://control.example", "alice", "stale password") {
		t.Fatal("stale authentication restored a cleared credential")
	}
}

func TestLatestStartedRequestWins(t *testing.T) {
	Clear()
	t.Cleanup(Clear)

	first := BeginRequest()
	second := BeginRequest()

	if SetIfCurrentRequest(first, "https://control.example", "alice", "old response") {
		t.Fatal("an older response overwrote the latest authentication request")
	}
	if _, _, ok := Get("https://control.example"); ok {
		t.Fatal("an older response populated the credential store")
	}
	if !SetIfCurrentRequest(second, "https://control.example", "bob", "latest response") {
		t.Fatal("the latest authentication response was rejected")
	}
	username, password, ok := Get("https://control.example")
	if !ok || username != "bob" || password != "latest response" {
		t.Fatalf("Get() = %q, %q, %v; want latest request", username, password, ok)
	}
	if SetIfCurrentRequest(second, "https://control.example", "mallory", "duplicate response") {
		t.Fatal("a consumed request token was accepted twice")
	}
}
