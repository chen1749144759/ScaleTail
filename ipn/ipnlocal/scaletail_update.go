// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package ipnlocal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"scaletail.com/clientupdate/scaletailota"
	"scaletail.com/internal/controlurl"
	"scaletail.com/ipn"
	"scaletail.com/ipn/ipnauth"
	"scaletail.com/syncs"
	"scaletail.com/version"
)

const (
	scaleTailUpdatePolicyStateKey      ipn.StateKey = "_scaletail-update-policy"
	scaleTailUpdatePolicySchema                     = 1
	maxScaleTailUpdatePolicyStateBytes              = 64 << 10
)

type scaleTailUpdateState struct {
	Schema        int                 `json:"schema"`
	Policy        scaletailota.Policy `json:"policy"`
	ResumePending bool                `json:"resume_pending"`
}

// ScaleTailUpdateStatus is the device-wide daemon-owned OTA policy state.
type ScaleTailUpdateStatus struct {
	Active         bool                `json:"active"`
	CurrentVersion string              `json:"current_version"`
	ResumePending  bool                `json:"resume_pending"`
	Policy         scaletailota.Policy `json:"policy"`
}

// ScaleTailUpdateRequiredError reports that an authenticated forced update
// policy prevents the daemon from entering the running state.
type ScaleTailUpdateRequiredError struct {
	Version  string
	Revision uint64
}

// RefreshScaleTailUpdatePolicy fetches the public, non-secret policy directly
// from the configured control server. checked is true only after a complete
// server response was received; callers may fail open on transport outages but
// must reject a checked response whose signature or policy is invalid.
func (b *LocalBackend) RefreshScaleTailUpdatePolicy(ctx context.Context, rawControlURL string) (checked bool, err error) {
	rawControlURL = strings.TrimSpace(rawControlURL)
	if rawControlURL == "" || ipn.IsLoginServerSynonym(rawControlURL) {
		return false, nil
	}
	controlURL, err := controlurl.ParseControl(rawControlURL)
	if err != nil {
		return true, err
	}
	status, err := b.ScaleTailUpdateStatus()
	if err != nil {
		return true, err
	}
	target := *controlURL
	target.Path = "/scaletail/v1/client-update"
	target.RawPath = ""
	target.RawQuery = url.Values{
		"current_version":  {version.Short()},
		"platform":         {scaletailota.CurrentPlatform()},
		"current_revision": {strconv.FormatUint(status.Policy.Revision, 10)},
	}.Encode()
	target.Fragment = ""

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = b.dialer.SystemDial
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), http.NoBody)
	if err != nil {
		return true, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	response, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return true, fmt.Errorf("update policy endpoint returned HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return true, fmt.Errorf("invalid update policy response")
	}
	var payload struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return true, fmt.Errorf("invalid update policy response: %w", err)
	}
	if len(payload.Data) == 0 || string(payload.Data) == "null" {
		return true, nil
	}
	var policy scaletailota.Policy
	if err := json.Unmarshal(payload.Data, &policy); err != nil {
		return true, fmt.Errorf("invalid update policy metadata: %w", err)
	}
	if policy.Revision == 0 || policy.Signature == "" {
		return true, nil
	}
	_, err = b.ApplyScaleTailUpdatePolicy(policy)
	return true, err
}

func (b *LocalBackend) startScaleTailUpdateMonitor() {
	b.scaleTailUpdateMonitorOnce.Do(func() {
		b.goTracker.Go(func() {
			refresh := func() {
				controlURL := b.Prefs().ControlURL()
				ctx, cancel := context.WithTimeout(b.ctx, 12*time.Second)
				checked, err := b.RefreshScaleTailUpdatePolicy(ctx, controlURL)
				cancel()
				if err != nil {
					b.logf("OTA: policy refresh failed (checked=%v): %v", checked, err)
				}
			}
			refresh()
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-b.ctx.Done():
					return
				case <-ticker.C:
					refresh()
				}
			}
		})
	})
}

func (e *ScaleTailUpdateRequiredError) Error() string {
	return fmt.Sprintf("ScaleTail %s is required by forced update policy revision %d", e.Version, e.Revision)
}

// ApplyScaleTailUpdatePolicy verifies and atomically persists a newer policy.
// A forced policy is persisted before the network is stopped, so a crash can
// never turn a mandatory update into a bypass.
func (b *LocalBackend) ApplyScaleTailUpdatePolicy(policy scaletailota.Policy) (ScaleTailUpdateStatus, error) {
	canonical, err := scaletailota.Verify(policy)
	if err != nil {
		return ScaleTailUpdateStatus{}, err
	}
	if canonical.Platform != scaletailota.CurrentPlatform() {
		return ScaleTailUpdateStatus{}, fmt.Errorf("OTA policy platform %q does not match %q", canonical.Platform, scaletailota.CurrentPlatform())
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.loadScaleTailUpdatePolicyLocked(); err != nil {
		return ScaleTailUpdateStatus{}, err
	}
	return b.applyVerifiedScaleTailUpdatePolicyLocked(canonical)
}

// ScaleTailUpdateStatus returns the persisted policy after reconciling it with
// the installed version.
func (b *LocalBackend) ScaleTailUpdateStatus() (ScaleTailUpdateStatus, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.loadScaleTailUpdatePolicyLocked(); err != nil {
		return ScaleTailUpdateStatus{}, err
	}
	return b.scaleTailUpdateStatusLocked(), nil
}

// AcknowledgeScaleTailUpdateResume clears the saved pre-update running intent
// after the desktop client has restored connectivity.
func (b *LocalBackend) AcknowledgeScaleTailUpdateResume() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.loadScaleTailUpdatePolicyLocked(); err != nil {
		return err
	}
	if b.scaleTailForcedUpdateRequiredLocked() || !b.scaleTailUpdate.ResumePending {
		return nil
	}
	next := b.scaleTailUpdate
	next.ResumePending = false
	if err := b.writeScaleTailUpdateStateLocked(next); err != nil {
		return err
	}
	b.scaleTailUpdate = next
	return nil
}

func (b *LocalBackend) applyVerifiedScaleTailUpdatePolicyLocked(policy scaletailota.Policy) (ScaleTailUpdateStatus, error) {
	syncs.RequiresMutex(&b.mu)
	current := b.scaleTailUpdate
	if current.Policy.Revision > policy.Revision {
		return b.scaleTailUpdateStatusLocked(), fmt.Errorf("stale OTA policy revision %d; current revision is %d", policy.Revision, current.Policy.Revision)
	}
	if current.Policy.Revision == policy.Revision {
		if current.Policy != policy {
			return b.scaleTailUpdateStatusLocked(), fmt.Errorf("OTA policy revision %d conflicts with persisted policy", policy.Revision)
		}
		return b.scaleTailUpdateStatusLocked(), nil
	}

	next := scaleTailUpdateState{
		Schema:        scaleTailUpdatePolicySchema,
		Policy:        policy,
		ResumePending: current.ResumePending,
	}
	active := policy.Action == scaletailota.ActionForced && !scaletailota.VersionAtLeast(version.Short(), policy.Version)
	if active && b.pm.CurrentPrefs().WantRunning() {
		next.ResumePending = true
	}
	if err := b.writeScaleTailUpdateStateLocked(next); err != nil {
		return b.scaleTailUpdateStatusLocked(), fmt.Errorf("persist OTA policy: %w", err)
	}
	b.scaleTailUpdate = next

	if active && b.pm.CurrentPrefs().WantRunning() {
		_, err := b.editPrefsLocked(ipnauth.Self, &ipn.MaskedPrefs{
			Prefs:          ipn.Prefs{WantRunning: false},
			WantRunningSet: true,
		})
		if err != nil {
			return b.scaleTailUpdateStatusLocked(), fmt.Errorf("stop network for forced update: %w", err)
		}
	}
	return b.scaleTailUpdateStatusLocked(), nil
}

func (b *LocalBackend) loadScaleTailUpdatePolicyLocked() error {
	syncs.RequiresMutex(&b.mu)
	if b.scaleTailUpdateLoaded {
		return nil
	}
	if b.store == nil {
		b.scaleTailUpdateLoaded = true
		return nil
	}
	raw, err := b.store.ReadState(scaleTailUpdatePolicyStateKey)
	if errors.Is(err, ipn.ErrStateNotExist) {
		b.scaleTailUpdateLoaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("read OTA policy state: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxScaleTailUpdatePolicyStateBytes {
		return fmt.Errorf("invalid OTA policy state size")
	}
	var state scaleTailUpdateState
	if err := json.Unmarshal(raw, &state); err != nil || state.Schema != scaleTailUpdatePolicySchema {
		return fmt.Errorf("invalid OTA policy state")
	}
	canonical, err := scaletailota.Verify(state.Policy)
	if err != nil {
		return fmt.Errorf("verify persisted OTA policy: %w", err)
	}
	if canonical.Platform != scaletailota.CurrentPlatform() {
		return fmt.Errorf("persisted OTA policy platform %q does not match %q", canonical.Platform, scaletailota.CurrentPlatform())
	}
	state.Policy = canonical
	b.scaleTailUpdate = state
	b.scaleTailUpdateLoaded = true
	return nil
}

func (b *LocalBackend) writeScaleTailUpdateStateLocked(state scaleTailUpdateState) error {
	syncs.RequiresMutex(&b.mu)
	if b.store == nil {
		return errors.New("OTA policy state store is unavailable")
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(encoded) > maxScaleTailUpdatePolicyStateBytes {
		return fmt.Errorf("OTA policy state is too large")
	}
	return ipn.WriteState(b.store, scaleTailUpdatePolicyStateKey, encoded)
}

func (b *LocalBackend) scaleTailForcedUpdateRequiredLocked() bool {
	syncs.RequiresMutex(&b.mu)
	policy := b.scaleTailUpdate.Policy
	return b.scaleTailUpdateLoaded && policy.Action == scaletailota.ActionForced &&
		!scaletailota.VersionAtLeast(version.Short(), policy.Version)
}

func (b *LocalBackend) scaleTailUpdateRequiredErrorLocked() error {
	syncs.RequiresMutex(&b.mu)
	return &ScaleTailUpdateRequiredError{
		Version:  b.scaleTailUpdate.Policy.Version,
		Revision: b.scaleTailUpdate.Policy.Revision,
	}
}

func (b *LocalBackend) scaleTailUpdateStatusLocked() ScaleTailUpdateStatus {
	syncs.RequiresMutex(&b.mu)
	return ScaleTailUpdateStatus{
		Active:         b.scaleTailForcedUpdateRequiredLocked(),
		CurrentVersion: version.Short(),
		ResumePending:  b.scaleTailUpdate.ResumePending,
		Policy:         b.scaleTailUpdate.Policy,
	}
}
