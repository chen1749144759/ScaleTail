// Copyright (c) ScaleTail Inc & contributors
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

	"scaletail.com/clientupdate/scaletailota"
)

type metadata struct {
	Version   string `json:"version"`
	Platform  string `json:"platform"`
	SHA256    string `json:"sha256"`
	FileSize  int64  `json:"file_size"`
	Signature string `json:"signature"`
}

func main() {
	generate := flag.Bool("generate", false, "generate a new Ed25519 key pair")
	privateKeyPath := flag.String("private-key", "", "base64 encoded private key file")
	publicKeyPath := flag.String("public-key", "", "base64 encoded public key output")
	filePath := flag.String("file", "", "installer to sign")
	version := flag.String("version", "", "release version")
	platform := flag.String("platform", "windows-amd64", "release platform")
	jsonOut := flag.String("json-out", "", "metadata JSON output")
	flag.Parse()

	if *generate {
		generateKeys(*privateKeyPath, *publicKeyPath)
		return
	}
	signFile(*privateKeyPath, *filePath, *version, *platform, *jsonOut)
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

func signFile(privatePath, filePath, version, platform, jsonOut string) {
	if privatePath == "" || filePath == "" || strings.TrimSpace(version) == "" || jsonOut == "" {
		fatalf("-private-key, -file, -version and -json-out are required")
	}
	privateKey := readPrivateKey(privatePath)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	publicBase64 := base64.StdEncoding.EncodeToString(publicKey)
	if scaletailota.PublicKeyBase64 != "" && publicBase64 != scaletailota.PublicKeyBase64 {
		fatalf("private key does not match the public key embedded in ScaleTail")
	}

	f, err := os.Open(filePath)
	if err != nil {
		fatalf("open installer: %v", err)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	closeErr := f.Close()
	if err != nil {
		fatalf("hash installer: %v", err)
	}
	if closeErr != nil {
		fatalf("close installer: %v", closeErr)
	}
	shaHex := hex.EncodeToString(hash.Sum(nil))
	cleanPlatform := strings.ToLower(strings.TrimSpace(platform))
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, scaletailota.Message(version, cleanPlatform, shaHex, size)))
	if err := scaletailota.Verify(version, cleanPlatform, shaHex, size, signature); err != nil {
		fatalf("self-verify signed metadata: %v", err)
	}
	meta := metadata{
		Version:   strings.TrimSpace(version),
		Platform:  cleanPlatform,
		SHA256:    shaHex,
		FileSize:  size,
		Signature: signature,
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		fatalf("encode metadata: %v", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(jsonOut, encoded, 0644); err != nil {
		fatalf("write metadata: %v", err)
	}
	fmt.Printf("signed %s metadata written to %s\n", filepath.Base(filePath), jsonOut)
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
