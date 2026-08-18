// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"scaletail.com/client/local"
)

const managedPasswordRotationAge = 83 * 24 * time.Hour

const managedPasswordAuthenticationTimeout = 45 * time.Second

type accountPasswordAuthFunc func(context.Context, string, []byte) error
type accountPasswordChangeFunc func(context.Context, string, []byte, []byte) error

func authenticateScaleTailAccountWithRotation(
	ctx context.Context,
	username string,
	password []byte,
	passwordFile string,
	autoRotate bool,
) error {
	if !autoRotate {
		return authenticateScaleTailAccount(ctx, username, password)
	}
	return authenticateAccountWithPasswordRotation(
		ctx,
		username,
		password,
		passwordFile,
		func(ctx context.Context, username string, password []byte) error {
			return retryManagedAccountAuthentication(
				ctx,
				username,
				password,
				authenticateScaleTailAccount,
				managedPasswordAuthenticationTimeout,
				750*time.Millisecond,
			)
		},
		func(ctx context.Context, username string, current, next []byte) error {
			return localClient.ScaleTailChangeAccountPassword(ctx, username, current, next)
		},
		time.Now,
	)
}

func retryManagedAccountAuthentication(
	ctx context.Context,
	username string,
	password []byte,
	authenticate accountPasswordAuthFunc,
	timeout time.Duration,
	retryDelay time.Duration,
) error {
	retryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		err := authenticate(retryCtx, username, password)
		if err == nil || !isTransientAccountAuthentication(err) {
			return err
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-retryCtx.Done():
			timer.Stop()
			return err
		case <-timer.C:
		}
	}
}

func authenticateAccountWithPasswordRotation(
	ctx context.Context,
	username string,
	password []byte,
	passwordFile string,
	authenticate accountPasswordAuthFunc,
	changePassword accountPasswordChangeFunc,
	now func() time.Time,
) error {
	stagedPath := passwordFile + ".next"
	if staged, err := readManagedPasswordFile(stagedPath); err == nil {
		defer clear(staged)
		authErr := authenticate(ctx, username, staged)
		if authErr == nil {
			if err := promoteManagedPassword(stagedPath, passwordFile); err != nil {
				return fmt.Errorf("recovering rotated password file: %w", err)
			}
			return nil
		}
		if isExpiredAccountPassword(authErr) {
			if err := promoteManagedPassword(stagedPath, passwordFile); err != nil {
				return fmt.Errorf("recovering expired rotated password file: %w", err)
			}
			password = staged
		} else if !isInvalidAccountCredential(authErr) {
			return fmt.Errorf("checking staged account password: %w", authErr)
		} else if err := os.Remove(stagedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing stale staged password: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading staged account password: %w", err)
	}

	authErr := authenticate(ctx, username, password)
	rotate, err := managedPasswordNeedsRotation(passwordFile, now())
	if err != nil {
		return err
	}
	if authErr == nil && !rotate {
		return nil
	}
	if authErr != nil && !isExpiredAccountPassword(authErr) {
		return authErr
	}

	next, err := generateManagedAccountPassword()
	if err != nil {
		return err
	}
	defer clear(next)
	if err := stageManagedPassword(stagedPath, next); err != nil {
		return err
	}
	if err := changePassword(ctx, username, password, next); err != nil {
		return fmt.Errorf("rotating account password: %w", err)
	}
	if err := authenticate(ctx, username, next); err != nil {
		return fmt.Errorf("authenticating with rotated account password: %w", err)
	}
	if err := promoteManagedPassword(stagedPath, passwordFile); err != nil {
		return fmt.Errorf("committing rotated password file: %w", err)
	}
	return nil
}

func managedPasswordNeedsRotation(path string, now time.Time) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("checking managed password age: %w", err)
	}
	return now.Sub(info.ModTime()) >= managedPasswordRotationAge, nil
}

func generateManagedAccountPassword() ([]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generating account password: %w", err)
	}
	password := []byte(base64.RawURLEncoding.EncodeToString(raw))
	clear(raw)
	return password, nil
}

func stageManagedPassword(path string, password []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating staged account password: %w", err)
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("securing staged account password: %w", err)
	}
	if _, err := file.Write(password); err != nil {
		return fmt.Errorf("writing staged account password: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("syncing staged account password: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing staged account password: %w", err)
	}
	removeOnFailure = false
	return nil
}

func promoteManagedPassword(stagedPath, passwordPath string) error {
	if err := os.Rename(stagedPath, passwordPath); err != nil {
		return err
	}
	if err := os.Chmod(passwordPath, 0o600); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(filepath.Dir(passwordPath))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func readManagedPasswordFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("managed password file must be a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("managed password file must have mode 0600")
	}
	password, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(password) == 0 || len(password) > 72 {
		clear(password)
		return nil, errors.New("managed password file contains an invalid password")
	}
	return password, nil
}

func isExpiredAccountPassword(err error) bool {
	var authErr *local.ScaleTailPasswordAuthError
	return errors.As(err, &authErr) && authErr.Code == "password_expired"
}

func isInvalidAccountCredential(err error) bool {
	var authErr *local.ScaleTailPasswordAuthError
	return errors.As(err, &authErr) && authErr.Code == "invalid_credentials"
}

func isTransientAccountAuthentication(err error) bool {
	var authErr *local.ScaleTailPasswordAuthError
	if !errors.As(err, &authErr) {
		return false
	}
	switch authErr.Code {
	case "network_error", "auth_session_expired", "invalid_auth_session":
		return true
	default:
		return false
	}
}
