// Copyright (c) ScaleTail Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package scaletailota defines the signed manifest used by ScaleTail OTA updates.
package scaletailota

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
)

// PublicKeyBase64 is the raw Ed25519 public key trusted by scaletaild.
// It is intentionally public; the corresponding private key must never be committed.
const PublicKeyBase64 = "vLGmMjFWFdcyPurQt1EZ1cDZgY4FcroH4aRMfDpEP2o="

// Message returns the canonical bytes signed for an installer release.
func Message(version, platform, sha256Hex string, fileSize int64) []byte {
	return []byte(fmt.Sprintf(
		"scaletail-update-v1\n%s\n%s\n%s\n%d\n",
		strings.TrimSpace(version),
		strings.ToLower(strings.TrimSpace(platform)),
		strings.ToLower(strings.TrimSpace(sha256Hex)),
		fileSize,
	))
}

// Verify reports whether signatureBase64 authenticates the manifest.
func Verify(version, platform, sha256Hex string, fileSize int64, signatureBase64 string) error {
	publicKey, err := base64.StdEncoding.DecodeString(PublicKeyBase64)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid embedded OTA public key")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signatureBase64))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid OTA signature encoding")
	}
	if !ed25519.Verify(publicKey, Message(version, platform, sha256Hex, fileSize), signature) {
		return fmt.Errorf("OTA signature verification failed")
	}
	return nil
}
