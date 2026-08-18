// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"scaletail.com/client/scaletail/apitype"
)

const maxScaleTailPasswordAuthResponseBytes = 64 << 10

// ScaleTailPasswordAuthError is returned when the daemon or control server
// rejects an account-password proof.
type ScaleTailPasswordAuthError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *ScaleTailPasswordAuthError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Message
}

// ScaleTailAuthenticateAccount submits account proof through LocalAPI. The
// credential is transported inside the existing Noise control connection.
func (lc *Client) ScaleTailAuthenticateAccount(
	ctx context.Context,
	username string,
	password []byte,
) error {
	body, err := json.Marshal(struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{
		Username: username,
		Password: string(password),
	})
	if err != nil {
		return fmt.Errorf("encoding account credential: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://"+apitype.LocalAPIHost+"/localapi/v0/scaletail-auth-password",
		bytes.NewReader(body),
	)
	if err != nil {
		clear(body)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := lc.DoLocalRequest(req)
	clear(body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(res.Body, maxScaleTailPasswordAuthResponseBytes+1))
	if err != nil {
		return err
	}
	if len(responseBody) > maxScaleTailPasswordAuthResponseBytes {
		return newScaleTailPasswordAuthError(res.StatusCode, "", "authentication response is too large")
	}
	if res.StatusCode == http.StatusNoContent {
		return nil
	}

	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(responseBody, &payload)
	message := payload.Message
	if message == "" {
		message = payload.Error
	}
	if message == "" {
		message = string(bytes.TrimSpace(responseBody))
	}
	if message == "" {
		message = res.Status
	}
	return newScaleTailPasswordAuthError(res.StatusCode, payload.Code, message)
}

// ScaleTailChangeAccountPassword rotates an account password through LocalAPI.
// The daemon forwards it only through the encrypted Noise control connection.
func (lc *Client) ScaleTailChangeAccountPassword(
	ctx context.Context,
	username string,
	currentPassword,
	newPassword []byte,
) error {
	body, err := json.Marshal(struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}{
		Username:        username,
		CurrentPassword: string(currentPassword),
		NewPassword:     string(newPassword),
	})
	if err != nil {
		return fmt.Errorf("encoding account password change: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		"http://"+apitype.LocalAPIHost+"/localapi/v0/scaletail-change-password",
		bytes.NewReader(body),
	)
	if err != nil {
		clear(body)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := lc.DoLocalRequest(req)
	clear(body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(res.Body, maxScaleTailPasswordAuthResponseBytes+1))
	if err != nil {
		return err
	}
	if len(responseBody) > maxScaleTailPasswordAuthResponseBytes {
		return newScaleTailPasswordAuthError(res.StatusCode, "", "password change response is too large")
	}
	if res.StatusCode == http.StatusNoContent {
		return nil
	}

	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(responseBody, &payload)
	message := payload.Message
	if message == "" {
		message = payload.Error
	}
	if message == "" {
		message = string(bytes.TrimSpace(responseBody))
	}
	if message == "" {
		message = res.Status
	}
	return newScaleTailPasswordAuthError(res.StatusCode, payload.Code, message)
}

func newScaleTailPasswordAuthError(statusCode int, code, message string) error {
	return &ScaleTailPasswordAuthError{
		StatusCode: statusCode,
		Code:       code,
		Message:    message,
	}
}
