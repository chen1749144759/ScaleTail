// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package scaletailota

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

const testDownloadURL = "https://downloads.example.com/releases/ScaleTail.exe?channel=stable~1"

func TestV3MessageBindsCompletePolicy(t *testing.T) {
	canonicalURL, err := NormalizeDownloadURL("HTTPS://Downloads.Example.Com:443/releases/ScaleTail.exe?channel=stable%7e1")
	if err != nil {
		t.Fatalf("NormalizeDownloadURL: %v", err)
	}
	if canonicalURL != testDownloadURL {
		t.Fatalf("canonical URL = %q, want %q", canonicalURL, testDownloadURL)
	}
	policy := Policy{
		Revision:    42,
		Action:      ActionForced,
		Version:     "0.0.8",
		Platform:    "WINDOWS-AMD64",
		SHA256:      strings.Repeat("a", 64),
		FileSize:    1234,
		DownloadURL: canonicalURL,
	}
	message, err := Message(policy)
	if err != nil {
		t.Fatal(err)
	}
	want := "scaletail-update-v3\n42\nforced\n0.0.8\nwindows-amd64\n" + strings.Repeat("a", 64) + "\n1234\n" + testDownloadURL + "\n"
	if string(message) != want {
		t.Fatalf("Message = %q, want %q", message, want)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rawSignature := ed25519.Sign(privateKey, message)
	envelope, err := EncodeSignature(rawSignature)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(publicKey, message, rawSignature) {
		t.Fatal("v3 signature did not verify")
	}
	parsed, err := parseSignature(envelope)
	if err != nil || !ed25519.Verify(publicKey, message, parsed) {
		t.Fatalf("parseSignature failed: %v", err)
	}
	for _, mutate := range []func(*Policy){
		func(p *Policy) { p.Revision++ },
		func(p *Policy) { p.Action = ActionSuggested },
		func(p *Policy) { p.Version = "0.0.9" },
		func(p *Policy) { p.Platform = "windows-arm64" },
		func(p *Policy) { p.SHA256 = strings.Repeat("b", 64) },
		func(p *Policy) { p.FileSize++ },
		func(p *Policy) { p.DownloadURL = "https://downloads.example.com/other.exe" },
	} {
		changed := policy
		mutate(&changed)
		changedMessage, err := Message(changed)
		if err != nil {
			t.Fatal(err)
		}
		if ed25519.Verify(publicKey, changedMessage, rawSignature) {
			t.Fatal("mutated policy still verified")
		}
	}
}

func TestClearPolicyHasNoInstaller(t *testing.T) {
	policy, err := Canonicalize(Policy{
		Revision: 43,
		Action:   ActionClear,
		Version:  "0.0.8",
		Platform: "windows-amd64",
	})
	if err != nil || policy.Action != ActionClear {
		t.Fatalf("Canonicalize(clear) = %#v, %v", policy, err)
	}
	policy.DownloadURL = testDownloadURL
	if _, err := Canonicalize(policy); err == nil {
		t.Fatal("clear policy accepted installer metadata")
	}
}

func TestPolicyRejectsCrossLanguageNumericOverflow(t *testing.T) {
	base := Policy{
		Revision:    1,
		Action:      ActionSuggested,
		Version:     "0.0.8",
		Platform:    "windows-amd64",
		SHA256:      strings.Repeat("a", 64),
		FileSize:    1,
		DownloadURL: testDownloadURL,
	}
	revisionOverflow := base
	revisionOverflow.Revision = maxPolicyRevision + 1
	if _, err := Canonicalize(revisionOverflow); err == nil {
		t.Fatal("policy accepted revision outside JavaScript safe integer range")
	}
	sizeOverflow := base
	sizeOverflow.FileSize = maxPolicyArtifactSize + 1
	if _, err := Canonicalize(sizeOverflow); err == nil {
		t.Fatal("policy accepted an artifact larger than 1 GiB")
	}
}

func TestVersionComparison(t *testing.T) {
	if !IsNewerVersion("v1.2.3", "1.2.2") || IsNewerVersion("1.2.3-beta.1", "1.2.3") {
		t.Fatal("unexpected Semantic Version comparison")
	}
	if !VersionAtLeast("1.2.3", "v1.2.3") || VersionAtLeast("1.2.2", "1.2.3") {
		t.Fatal("unexpected version satisfaction result")
	}
}

func TestNormalizeDownloadURLRejectsUnsafeHosts(t *testing.T) {
	for _, raw := range []string{
		"https://localhost/ScaleTail.exe",
		"https://api.localhost/ScaleTail.exe",
		"https://127.0.0.1/ScaleTail.exe",
		"https://2130706433/ScaleTail.exe",
		"https://0x7f000001/ScaleTail.exe",
		"https://10.0.0.1/ScaleTail.exe",
		"https://[::1]/ScaleTail.exe",
		"https://user:password@downloads.example.com/ScaleTail.exe",
		"https://downloads.example.com/ScaleTail.exe#fragment",
		"https://downloads.example.com/ScaleTail.exe?",
	} {
		if _, err := NormalizeDownloadURL(raw); err == nil {
			t.Errorf("NormalizeDownloadURL(%q) succeeded", raw)
		}
	}
}
