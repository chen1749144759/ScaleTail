// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package scaletailota defines the signed manifest used by ScaleTail OTA updates.
package scaletailota

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// PublicKeyBase64 is the raw Ed25519 public key trusted by scaletaild.
// It is intentionally public; the corresponding private key must never be committed.
const PublicKeyBase64 = "vLGmMjFWFdcyPurQt1EZ1cDZgY4FcroH4aRMfDpEP2o="

const (
	signatureV3Prefix     = "v3."
	maxPolicyRevision     = 1<<53 - 1
	maxPolicyArtifactSize = 1 << 30
)

var semanticVersionPattern = regexp.MustCompile(`^[vV]?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

const (
	ActionSuggested = "suggested"
	ActionForced    = "forced"
	ActionClear     = "clear"
)

// Policy is the complete signed OTA release policy. Revision is a
// publisher-controlled, monotonically increasing value used to reject replay.
// Clear policies carry a target Version but no installer metadata.
type Policy struct {
	Revision    uint64 `json:"policy_revision"`
	Action      string `json:"update_type"`
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	SHA256      string `json:"sha256"`
	FileSize    int64  `json:"file_size"`
	DownloadURL string `json:"download_url"`
	Signature   string `json:"signature"`
}

// NormalizeDownloadURL returns the canonical URL accepted by the v2 OTA
// protocol. OTA artifacts must be hosted on an HTTPS DNS name; IP literals,
// localhost names, credentials, and fragments are never accepted.
func NormalizeDownloadURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", fmt.Errorf("invalid OTA download URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" ||
		u.User != nil || u.Fragment != "" || u.ForceQuery {
		return "", fmt.Errorf("invalid OTA download URL")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", fmt.Errorf("OTA download URL must use HTTPS")
	}
	host := strings.ToLower(u.Hostname())
	if !validOTAHost(host) {
		return "", fmt.Errorf("OTA download URL host is not allowed")
	}
	port := u.Port()
	if port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", fmt.Errorf("invalid OTA download URL port")
		}
		if portNumber == 443 {
			port = ""
		} else {
			port = strconv.Itoa(portNumber)
		}
	}
	path, err := normalizeURLComponent(u.EscapedPath())
	if err != nil {
		return "", err
	}
	if path == "" {
		path = "/"
	}
	query, err := normalizeURLComponent(u.RawQuery)
	if err != nil {
		return "", err
	}
	canonical := "https://" + host
	if port != "" {
		canonical += ":" + port
	}
	canonical += path
	if query != "" {
		canonical += "?" + query
	}
	return canonical, nil
}

func validOTAHost(host string) bool {
	if host == "" || strings.HasSuffix(host, ".") || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil || numericHostLiteral(host) {
		return false
	}
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			continue
		}
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	return true
}

func numericHostLiteral(host string) bool {
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		if strings.HasPrefix(label, "0x") {
			if len(label) == 2 {
				return false
			}
			for _, r := range label[2:] {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
					return false
				}
			}
			continue
		}
		for _, r := range label {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func normalizeURLComponent(value string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c < 0x21 || c > 0x7e || c == '\\' {
			return "", fmt.Errorf("invalid OTA download URL component")
		}
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+2 >= len(value) {
			return "", fmt.Errorf("invalid OTA download URL escape")
		}
		hex := value[i+1 : i+3]
		decoded, err := strconv.ParseUint(hex, 16, 8)
		if err != nil {
			return "", fmt.Errorf("invalid OTA download URL escape")
		}
		c = byte(decoded)
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("-._~", rune(c)) {
			b.WriteByte(c)
		} else {
			b.WriteString("%")
			b.WriteString(strings.ToUpper(hex))
		}
		i += 2
	}
	return b.String(), nil
}

// Canonicalize validates a policy and returns its canonical signed form.
func Canonicalize(policy Policy) (Policy, error) {
	policy.Action = strings.ToLower(strings.TrimSpace(policy.Action))
	policy.Version = strings.TrimSpace(policy.Version)
	policy.Platform = strings.ToLower(strings.TrimSpace(policy.Platform))
	policy.SHA256 = strings.ToLower(strings.TrimSpace(policy.SHA256))
	policy.DownloadURL = strings.TrimSpace(policy.DownloadURL)
	policy.Signature = strings.TrimSpace(policy.Signature)

	if policy.Revision == 0 || policy.Revision > maxPolicyRevision {
		return Policy{}, fmt.Errorf("invalid OTA policy revision")
	}
	if policy.Action != ActionSuggested && policy.Action != ActionForced && policy.Action != ActionClear {
		return Policy{}, fmt.Errorf("invalid OTA policy action")
	}
	version, ok := canonicalVersion(policy.Version)
	if !ok {
		return Policy{}, fmt.Errorf("invalid OTA policy version")
	}
	policy.Version = strings.TrimPrefix(version, "v")
	if !validPlatform(policy.Platform) {
		return Policy{}, fmt.Errorf("invalid OTA policy platform")
	}

	if policy.Action == ActionClear {
		if policy.SHA256 != "" || policy.FileSize != 0 || policy.DownloadURL != "" {
			return Policy{}, fmt.Errorf("clear OTA policy must not include installer metadata")
		}
		return policy, nil
	}
	if len(policy.SHA256) != 64 {
		return Policy{}, fmt.Errorf("invalid OTA policy SHA-256")
	}
	if _, err := hex.DecodeString(policy.SHA256); err != nil {
		return Policy{}, fmt.Errorf("invalid OTA policy SHA-256")
	}
	if policy.FileSize <= 0 || policy.FileSize > maxPolicyArtifactSize {
		return Policy{}, fmt.Errorf("invalid OTA policy file size")
	}
	downloadURL, err := NormalizeDownloadURL(policy.DownloadURL)
	if err != nil {
		return Policy{}, err
	}
	policy.DownloadURL = downloadURL
	return policy, nil
}

// Message returns the v3 canonical bytes signed for a release policy.
func Message(policy Policy) ([]byte, error) {
	policy, err := Canonicalize(policy)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf(
		"scaletail-update-v3\n%d\n%s\n%s\n%s\n%s\n%d\n%s\n",
		policy.Revision,
		policy.Action,
		policy.Version,
		policy.Platform,
		policy.SHA256,
		policy.FileSize,
		policy.DownloadURL,
	)), nil
}

// EncodeSignature returns the v3 signature envelope stored in policy metadata.
func EncodeSignature(signature []byte) (string, error) {
	if len(signature) != ed25519.SignatureSize {
		return "", fmt.Errorf("invalid OTA signature length")
	}
	return signatureV3Prefix + base64.StdEncoding.EncodeToString(signature), nil
}

// Verify authenticates and canonicalizes a v3 policy. Older signature formats
// are intentionally rejected because they did not bind the policy action or
// replay-protection revision.
func Verify(policy Policy) (Policy, error) {
	canonical, err := Canonicalize(policy)
	if err != nil {
		return Policy{}, err
	}
	publicKey, err := base64.StdEncoding.DecodeString(PublicKeyBase64)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return Policy{}, fmt.Errorf("invalid embedded OTA public key")
	}
	signature, err := parseSignature(canonical.Signature)
	if err != nil {
		return Policy{}, err
	}
	message, err := Message(canonical)
	if err != nil {
		return Policy{}, err
	}
	if !ed25519.Verify(publicKey, message, signature) {
		return Policy{}, fmt.Errorf("OTA signature verification failed")
	}
	return canonical, nil
}

// IsNewerVersion reports whether candidate is a strictly newer Semantic
// Version than current.
func IsNewerVersion(candidate, current string) bool {
	candidateVersion, candidateOK := canonicalVersion(candidate)
	currentVersion, currentOK := canonicalVersion(current)
	return candidateOK && currentOK && semver.Compare(candidateVersion, currentVersion) > 0
}

// VersionAtLeast reports whether current satisfies required.
func VersionAtLeast(current, required string) bool {
	currentVersion, currentOK := canonicalVersion(current)
	requiredVersion, requiredOK := canonicalVersion(required)
	return currentOK && requiredOK && semver.Compare(currentVersion, requiredVersion) >= 0
}

// CurrentPlatform returns the signed OTA platform identifier for this binary.
func CurrentPlatform() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

func parseSignature(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, signatureV3Prefix) || strings.Count(value, ".") != 1 {
		return nil, fmt.Errorf("OTA signature must use v3 envelope")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, signatureV3Prefix))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid OTA signature encoding")
	}
	return signature, nil
}

func canonicalVersion(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !semanticVersionPattern.MatchString(value) {
		return "", false
	}
	value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
	value = "v" + value
	return value, semver.IsValid(value)
}

func validPlatform(platform string) bool {
	if platform == "" || len(platform) > 64 {
		return false
	}
	for _, r := range platform {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}
