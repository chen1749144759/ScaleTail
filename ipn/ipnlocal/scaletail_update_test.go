// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package ipnlocal

import (
	"errors"
	"strings"
	"testing"

	"scaletail.com/clientupdate/scaletailota"
	"scaletail.com/ipn"
)

func TestScaleTailForcedUpdateBlocksEveryRunningEntry(t *testing.T) {
	b := newTestLocalBackend(t)
	policy := scaletailota.Policy{
		Revision:    42,
		Action:      scaletailota.ActionForced,
		Version:     "999.0.0",
		Platform:    scaletailota.CurrentPlatform(),
		SHA256:      strings.Repeat("a", 64),
		FileSize:    1234,
		DownloadURL: "https://downloads.example.com/ScaleTail.exe",
		Signature:   "v3.test-only",
	}

	b.mu.Lock()
	if err := b.loadScaleTailUpdatePolicyLocked(); err != nil {
		b.mu.Unlock()
		t.Fatal(err)
	}
	if _, err := b.applyVerifiedScaleTailUpdatePolicyLocked(policy); err != nil {
		b.mu.Unlock()
		t.Fatal(err)
	}
	b.mu.Unlock()

	_, err := b.EditPrefs(&ipn.MaskedPrefs{
		Prefs:          ipn.Prefs{WantRunning: true},
		WantRunningSet: true,
	})
	var required *ScaleTailUpdateRequiredError
	if !errors.As(err, &required) || required.Version != "999.0.0" || required.Revision != 42 {
		t.Fatalf("EditPrefs error = %v, want forced update error", err)
	}

	err = b.Start(ipn.Options{UpdatePrefs: &ipn.Prefs{WantRunning: true}})
	if !errors.As(err, &required) {
		t.Fatalf("Start error = %v, want forced update error", err)
	}

	b.mu.Lock()
	prefs := &ipn.Prefs{WantRunning: true}
	b.reconcilePrefsLocked(prefs)
	b.mu.Unlock()
	if prefs.WantRunning {
		t.Fatal("reconcilePrefsLocked allowed forced update bypass")
	}
}

func TestScaleTailUpdatePolicyRejectsReplayAndConflict(t *testing.T) {
	b := newTestLocalBackend(t)
	current := scaletailota.Policy{
		Revision:  50,
		Action:    scaletailota.ActionClear,
		Version:   "1.2.3",
		Platform:  scaletailota.CurrentPlatform(),
		Signature: "v3.current",
	}
	b.mu.Lock()
	if err := b.loadScaleTailUpdatePolicyLocked(); err != nil {
		b.mu.Unlock()
		t.Fatal(err)
	}
	if _, err := b.applyVerifiedScaleTailUpdatePolicyLocked(current); err != nil {
		b.mu.Unlock()
		t.Fatal(err)
	}
	if _, err := b.applyVerifiedScaleTailUpdatePolicyLocked(current); err != nil {
		b.mu.Unlock()
		t.Fatalf("idempotent policy failed: %v", err)
	}
	conflict := current
	conflict.Action = scaletailota.ActionSuggested
	if _, err := b.applyVerifiedScaleTailUpdatePolicyLocked(conflict); err == nil {
		b.mu.Unlock()
		t.Fatal("conflicting equal revision was accepted")
	}
	stale := current
	stale.Revision--
	if _, err := b.applyVerifiedScaleTailUpdatePolicyLocked(stale); err == nil {
		b.mu.Unlock()
		t.Fatal("stale policy was accepted")
	}
	b.mu.Unlock()
}
