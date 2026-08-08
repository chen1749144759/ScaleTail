// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// scaletail-update-sign generates and uses the offline ScaleTail OTA signing key.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"scaletail.com/clientupdate/scaletailota"
)

type metadata struct {
	PolicyRevision uint64 `json:"policy_revision"`
	UpdateType     string `json:"update_type"`
	Version        string `json:"version"`
	Platform       string `json:"platform"`
	DownloadURL    string `json:"download_url"`
	SHA256         string `json:"sha256"`
	FileSize       int64  `json:"file_size"`
	Signature      string `json:"signature"`
}

func main() {
	generate := flag.Bool("generate", false, "generate a new Ed25519 key pair")
	privateKeyPath := flag.String("private-key", "", "base64 encoded private key file")
	publicKeyPath := flag.String("public-key", "", "base64 encoded public key output")
	filePath := flag.String("file", "", "installer to sign")
	version := flag.String("version", "", "release version")
	platform := flag.String("platform", "windows-amd64", "release platform")
	action := flag.String("action", scaletailota.ActionSuggested, "release policy action: suggested, forced, or clear")
	revision := flag.Uint64("revision", uint64(time.Now().UTC().UnixMilli()), "monotonically increasing release policy revision")
	downloadURL := flag.String("download-url", "", "canonical HTTPS installer URL to bind into the release signature")
	jsonOut := flag.String("json-out", "", "metadata JSON output")
	flag.Parse()

	if *generate {
		generateKeys(*privateKeyPath, *publicKeyPath)
		return
	}
	signPolicy(*privateKeyPath, *filePath, *version, *platform, *action, *revision, *downloadURL, *jsonOut)
}

func generateKeys(privatePath, publicPath string) {
	if privatePath == "" || publicPath == "" {
		fatalf("-private-key and -public-key are required with -generate")
	}
	if _, err := os.Stat(privatePath); err == nil {
		fatalf("refusing to overwrite existing private key %s", privatePath)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatalf("generate key: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(privatePath), 0700); err != nil {
		fatalf("create key directory: %v", err)
	}
	if err := os.WriteFile(privatePath, []byte(base64.StdEncoding.EncodeToString(privateKey)+"\n"), 0600); err != nil {
		fatalf("write private key: %v", err)
	}
	if err := os.WriteFile(publicPath, []byte(base64.StdEncoding.EncodeToString(publicKey)+"\n"), 0644); err != nil {
		fatalf("write public key: %v", err)
	}
	fmt.Println("OTA signing key pair generated; keep the private key offline and backed up.")
}

func signPolicy(privatePath, filePath, version, platform, action string, revision uint64, downloadURL, jsonOut string) {
	action = strings.ToLower(strings.TrimSpace(action))
	if privatePath == "" || strings.TrimSpace(version) == "" || jsonOut == "" || revision == 0 {
		fatalf("-private-key, -version, -revision and -json-out are required")
	}
	if action != scaletailota.ActionClear && (filePath == "" || downloadURL == "") {
		fatalf("-file and -download-url are required for suggested and forced policies")
	}
	privateKey := readPrivateKey(privatePath)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	publicBase64 := base64.StdEncoding.EncodeToString(publicKey)
	if scaletailota.PublicKeyBase64 != "" && publicBase64 != scaletailota.PublicKeyBase64 {
		fatalf("private key does not match the public key embedded in ScaleTail")
	}

	var shaHex string
	var size int64
	if action != scaletailota.ActionClear {
		f, err := os.Open(filePath)
		if err != nil {
			fatalf("open installer: %v", err)
		}
		hash := sha256.New()
		size, err = io.Copy(hash, f)
		closeErr := f.Close()
		if err != nil {
			fatalf("hash installer: %v", err)
		}
		if closeErr != nil {
			fatalf("close installer: %v", closeErr)
		}
		shaHex = hex.EncodeToString(hash.Sum(nil))
	}
	policy, err := scaletailota.Canonicalize(scaletailota.Policy{
		Revision:    revision,
		Action:      action,
		Version:     version,
		Platform:    platform,
		SHA256:      shaHex,
		FileSize:    size,
		DownloadURL: downloadURL,
	})
	if err != nil {
		fatalf("invalid OTA policy: %v", err)
	}
	message, err := scaletailota.Message(policy)
	if err != nil {
		fatalf("encode OTA policy: %v", err)
	}
	rawSignature := ed25519.Sign(privateKey, message)
	signature, err := scaletailota.EncodeSignature(rawSignature)
	if err != nil {
		fatalf("encode v3 signature envelope: %v", err)
	}
	policy.Signature = signature
	policy, err = scaletailota.Verify(policy)
	if err != nil {
		fatalf("self-verify signed metadata: %v", err)
	}
	meta := metadata{
		PolicyRevision: policy.Revision,
		UpdateType:     policy.Action,
		Version:        policy.Version,
		Platform:       policy.Platform,
		SHA256:         policy.SHA256,
		FileSize:       policy.FileSize,
		DownloadURL:    policy.DownloadURL,
		Signature:      policy.Signature,
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		fatalf("encode metadata: %v", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(jsonOut, encoded, 0644); err != nil {
		fatalf("write metadata: %v", err)
	}
	label := filepath.Base(filePath)
	if action == scaletailota.ActionClear {
		label = "clear policy"
	}
	fmt.Printf("signed %s metadata written to %s\n", label, jsonOut)
}

func readPrivateKey(path string) ed25519.PrivateKey {
	raw, err := os.ReadFile(path)
	if err != nil {
		fatalf("read private key: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(decoded) != ed25519.PrivateKeySize {
		fatalf("invalid Ed25519 private key")
	}
	return ed25519.PrivateKey(decoded)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
