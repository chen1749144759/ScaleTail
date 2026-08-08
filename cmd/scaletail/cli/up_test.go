// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package cli

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"scaletail.com/util/set"
)

// validUpFlags are the only flags that are valid for scaletail up. The up
// command is frozen: no new preferences can be added. Instead, add them to
// scaletail set.
// See tailscale/tailscale#15460.
var validUpFlags = set.Of(
	"accept-dns",
	"accept-risk",
	"accept-routes",
	"advertise-connector",
	"advertise-exit-node",
	"advertise-routes",
	"advertise-tags",
	"exit-node",
	"exit-node-allow-lan-access",
	"force-reauth",
	"host-routes",
	"hostname",
	"json",
	"login-server",
	"netfilter-mode",
	"nickname",
	"operator",
	"report-posture",
	"password-file",
	"reset",
	"shields-up",
	"snat-subnet-routes",
	"ssh",
	"stateful-filtering",
	"timeout",
	"unattended",
	"username",
)

// TestUpFlagSetIsFrozen complains when new flags are added to scaletail up.
func TestUpFlagSetIsFrozen(t *testing.T) {
	upFlagSet.VisitAll(func(f *flag.Flag) {
		name := f.Name
		if !validUpFlags.Contains(name) {
			t.Errorf("--%s flag added to scaletail up, new prefs go in scaletail set: see tailscale/tailscale#15460", name)
		}
	})
}

func TestAccountCredentialFromFile(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(passwordFile, []byte("correct horse battery staple\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	username, password, err := (upArgsT{
		username:     " alice ",
		passwordFile: passwordFile,
	}).accountCredential()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(password)
	if username != "alice" {
		t.Fatalf("username = %q, want alice", username)
	}
	if got, want := string(password), "correct horse battery staple"; got != want {
		t.Fatalf("password = %q, want %q", got, want)
	}
}

func TestAccountCredentialRejectsUnsafeInput(t *testing.T) {
	if _, _, err := (upArgsT{passwordFile: "secret"}).accountCredential(); err == nil {
		t.Fatal("password file without username unexpectedly succeeded")
	}
	if _, _, err := (upArgsT{username: "bad\nname"}).accountCredential(); err == nil {
		t.Fatal("control character in username unexpectedly succeeded")
	}

	passwordFile := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(passwordFile, []byte(strings.Repeat("x", 73)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (upArgsT{username: "alice", passwordFile: passwordFile}).accountCredential(); err == nil {
		t.Fatal("oversized password unexpectedly succeeded")
	}
	if err := os.WriteFile(passwordFile, []byte("invalid\npassword"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (upArgsT{username: "alice", passwordFile: passwordFile}).accountCredential(); err == nil {
		t.Fatal("control character in password unexpectedly succeeded")
	}

	if runtime.GOOS != "windows" {
		if err := os.WriteFile(passwordFile, []byte("secret"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(passwordFile, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := (upArgsT{username: "alice", passwordFile: passwordFile}).accountCredential(); err == nil {
			t.Fatal("group-readable password file unexpectedly succeeded")
		}
	}
}

func TestPrefsFromUpArgsRejectsRemoteHTTPControlServer(t *testing.T) {
	_, err := prefsFromUpArgs(upArgsT{server: "http://control.example.com:60090"}, t.Logf, nil, runtime.GOOS)
	if err == nil || !strings.Contains(err.Error(), "requires HTTPS") {
		t.Fatalf("prefsFromUpArgs() error = %v, want remote HTTP rejection", err)
	}
}
