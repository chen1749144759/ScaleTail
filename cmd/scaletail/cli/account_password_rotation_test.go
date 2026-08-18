// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"scaletail.com/client/local"
)

func TestAuthenticateAccountRotatesManagedPasswordBeforeExpiry(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "account-password")
	current := []byte("current managed password")
	if err := os.WriteFile(passwordFile, current, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.Add(-managedPasswordRotationAge - time.Hour)
	if err := os.Chtimes(passwordFile, old, old); err != nil {
		t.Fatal(err)
	}
	serverPassword := append([]byte(nil), current...)
	authenticate := func(_ context.Context, _ string, password []byte) error {
		if string(password) != string(serverPassword) {
			return &local.ScaleTailPasswordAuthError{Code: "invalid_credentials"}
		}
		return nil
	}
	changePassword := func(_ context.Context, _ string, oldPassword, newPassword []byte) error {
		if string(oldPassword) != string(serverPassword) {
			t.Fatalf("old password = %q, want %q", oldPassword, serverPassword)
		}
		serverPassword = append(serverPassword[:0], newPassword...)
		return nil
	}

	if err := authenticateAccountWithPasswordRotation(
		t.Context(), "linux-node", current, passwordFile, authenticate, changePassword, func() time.Time { return now },
	); err != nil {
		t.Fatalf("rotating managed password: %v", err)
	}
	stored, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(serverPassword) || string(stored) == string(current) {
		t.Fatalf("stored password was not rotated")
	}
	if _, err := os.Stat(passwordFile + ".next"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged password remains after success: %v", err)
	}
}

func TestAuthenticateAccountRotatesExpiredInitialPassword(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "account-password")
	current := []byte("temporary managed password")
	if err := os.WriteFile(passwordFile, current, 0o600); err != nil {
		t.Fatal(err)
	}
	serverPassword := append([]byte(nil), current...)
	expired := true
	authenticate := func(_ context.Context, _ string, password []byte) error {
		if string(password) != string(serverPassword) {
			return &local.ScaleTailPasswordAuthError{Code: "invalid_credentials"}
		}
		if expired {
			return &local.ScaleTailPasswordAuthError{Code: "password_expired"}
		}
		return nil
	}
	changePassword := func(_ context.Context, _ string, oldPassword, newPassword []byte) error {
		if string(oldPassword) != string(serverPassword) {
			t.Fatal("rotation used an unexpected current password")
		}
		serverPassword = append(serverPassword[:0], newPassword...)
		expired = false
		return nil
	}

	if err := authenticateAccountWithPasswordRotation(
		t.Context(), "linux-node", current, passwordFile, authenticate, changePassword, time.Now,
	); err != nil {
		t.Fatalf("rotating expired password: %v", err)
	}
}

func TestAuthenticateAccountRecoversCommittedStagedPassword(t *testing.T) {
	dir := t.TempDir()
	passwordFile := filepath.Join(dir, "account-password")
	if err := os.WriteFile(passwordFile, []byte("old managed password"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwordFile+".next", []byte("committed managed password"), 0o600); err != nil {
		t.Fatal(err)
	}
	changeCalled := false
	authenticate := func(_ context.Context, _ string, password []byte) error {
		if string(password) == "committed managed password" {
			return nil
		}
		return &local.ScaleTailPasswordAuthError{Code: "invalid_credentials"}
	}
	changePassword := func(context.Context, string, []byte, []byte) error {
		changeCalled = true
		return nil
	}
	if err := authenticateAccountWithPasswordRotation(
		t.Context(), "linux-node", []byte("old managed password"), passwordFile,
		authenticate, changePassword, time.Now,
	); err != nil {
		t.Fatalf("recovering staged password: %v", err)
	}
	if changeCalled {
		t.Fatal("recovery attempted another password change")
	}
	stored, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != "committed managed password" {
		t.Fatalf("stored password = %q", stored)
	}
}

func TestAuthenticateAccountRerotatesExpiredCommittedStagedPassword(t *testing.T) {
	dir := t.TempDir()
	passwordFile := filepath.Join(dir, "account-password")
	if err := os.WriteFile(passwordFile, []byte("old managed password"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passwordFile+".next", []byte("expired committed password"), 0o600); err != nil {
		t.Fatal(err)
	}
	serverPassword := []byte("expired committed password")
	expired := true
	authenticate := func(_ context.Context, _ string, password []byte) error {
		if string(password) != string(serverPassword) {
			return &local.ScaleTailPasswordAuthError{Code: "invalid_credentials"}
		}
		if expired {
			return &local.ScaleTailPasswordAuthError{Code: "password_expired"}
		}
		return nil
	}
	changePassword := func(_ context.Context, _ string, oldPassword, newPassword []byte) error {
		if string(oldPassword) != string(serverPassword) {
			t.Fatalf("rotation used %q, want recovered staged password", oldPassword)
		}
		serverPassword = append(serverPassword[:0], newPassword...)
		expired = false
		return nil
	}
	if err := authenticateAccountWithPasswordRotation(
		t.Context(), "linux-node", []byte("old managed password"), passwordFile,
		authenticate, changePassword, time.Now,
	); err != nil {
		t.Fatalf("rerotating expired staged password: %v", err)
	}
	stored, err := os.ReadFile(passwordFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(serverPassword) || string(stored) == "expired committed password" {
		t.Fatal("expired staged password was not replaced")
	}
}

func TestAuthenticateAccountRetainsStagedPasswordOnUncertainChange(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "account-password")
	current := []byte("expired managed password")
	if err := os.WriteFile(passwordFile, current, 0o600); err != nil {
		t.Fatal(err)
	}
	authenticate := func(context.Context, string, []byte) error {
		return &local.ScaleTailPasswordAuthError{Code: "password_expired"}
	}
	changePassword := func(context.Context, string, []byte, []byte) error {
		return errors.New("response lost")
	}
	if err := authenticateAccountWithPasswordRotation(
		t.Context(), "linux-node", current, passwordFile, authenticate, changePassword, time.Now,
	); err == nil {
		t.Fatal("uncertain password change unexpectedly succeeded")
	}
	if _, err := os.Stat(passwordFile + ".next"); err != nil {
		t.Fatalf("staged password was not retained: %v", err)
	}
}

func TestRetryManagedAccountAuthenticationRetriesOnlyTransientErrors(t *testing.T) {
	attempts := 0
	authenticate := func(context.Context, string, []byte) error {
		attempts++
		if attempts < 3 {
			return &local.ScaleTailPasswordAuthError{Code: "network_error"}
		}
		return nil
	}
	if err := retryManagedAccountAuthentication(
		t.Context(), "linux-node", []byte("managed password"), authenticate, time.Second, time.Millisecond,
	); err != nil {
		t.Fatalf("retrying transient authentication: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	attempts = 0
	authenticate = func(context.Context, string, []byte) error {
		attempts++
		return &local.ScaleTailPasswordAuthError{Code: "invalid_credentials"}
	}
	err := retryManagedAccountAuthentication(
		t.Context(), "linux-node", []byte("wrong password"), authenticate, time.Second, time.Millisecond,
	)
	if !isInvalidAccountCredential(err) {
		t.Fatalf("error = %v, want invalid credentials", err)
	}
	if attempts != 1 {
		t.Fatalf("invalid credential attempts = %d, want 1", attempts)
	}
}
