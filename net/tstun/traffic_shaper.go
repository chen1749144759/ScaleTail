// Copyright (c) ScaleTail Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package tstun

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const (
	minTrafficShaperBurstBytes = 4 << 10
	maxTrafficShaperBurstBytes = 256 << 10
)

// TrafficShaperConfig controls aggregate bandwidth for packets crossing the
// ScaleTail TUN. A zero rate means unlimited for that direction.
type TrafficShaperConfig struct {
	UploadBitsPerSecond   uint64 `json:"upload_bits_per_second"`
	DownloadBitsPerSecond uint64 `json:"download_bits_per_second"`
}

// TrafficShaperStatus describes the currently active aggregate limits and the
// amount of traffic paced since the last configuration change.
type TrafficShaperStatus struct {
	Config            TrafficShaperConfig `json:"config"`
	Active            bool                `json:"active"`
	UpdatedAt         time.Time           `json:"updated_at"`
	UploadBytes       uint64              `json:"upload_bytes"`
	DownloadBytes     uint64              `json:"download_bytes"`
	UploadWaitNanos   uint64              `json:"upload_wait_nanos"`
	DownloadWaitNanos uint64              `json:"download_wait_nanos"`
}

type trafficPacerState struct {
	ctx    context.Context
	cancel context.CancelFunc
	limit  *rate.Limiter
	burst  int
}

// trafficPacer applies one aggregate rate across every caller. Waiting here
// paces the TUN packet stream; it does not suspend or classify user processes.
type trafficPacer struct {
	mu     sync.Mutex
	state  atomic.Pointer[trafficPacerState]
	closed atomic.Bool
}

func (p *trafficPacer) configure(bitsPerSecond uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed.Load() {
		return
	}

	var next *trafficPacerState
	if bitsPerSecond > 0 {
		bytesPerSecond := max(uint64(1), bitsPerSecond/8)
		burst := trafficShaperBurst(bytesPerSecond)
		ctx, cancel := context.WithCancel(context.Background())
		limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burst)
		// Start without a free initial burst. Subsequent short bursts are bounded
		// to roughly 50 ms so packet batches and GSO do not defeat the limit.
		limiter.AllowN(time.Now(), burst)
		next = &trafficPacerState{ctx: ctx, cancel: cancel, limit: limiter, burst: burst}
	}

	old := p.state.Swap(next)
	if old != nil {
		old.cancel()
	}
}

func (p *trafficPacer) waitBytes(byteCount int) (paced bool, err error) {
	for remaining := byteCount; remaining > 0; {
		if p.closed.Load() {
			return paced, ErrClosed
		}
		state := p.state.Load()
		if state == nil {
			return paced, nil
		}
		paced = true
		chunk := min(remaining, state.burst)
		err := state.limit.WaitN(state.ctx, chunk)
		if errors.Is(err, context.Canceled) {
			// A hot update or clear canceled the old reservation. Continue with
			// the replacement limiter without leaving stale waits behind.
			continue
		}
		if err != nil {
			return paced, err
		}
		remaining -= chunk
	}
	return paced, nil
}

func (p *trafficPacer) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed.Swap(true) {
		return
	}
	if old := p.state.Swap(nil); old != nil {
		old.cancel()
	}
}

func trafficShaperBurst(bytesPerSecond uint64) int {
	burst := bytesPerSecond / 20 // 50 ms of traffic.
	burst = max(burst, minTrafficShaperBurstBytes)
	burst = min(burst, maxTrafficShaperBurstBytes)
	return int(burst)
}

// TrafficShaper paces all TUN traffic as a pair of aggregate upload/download
// streams. It deliberately has no per-process or per-flow state.
type TrafficShaper struct {
	mu        sync.Mutex
	config    TrafficShaperConfig
	updatedAt time.Time
	closed    bool

	upload   trafficPacer
	download trafficPacer

	uploadBytes       atomic.Uint64
	downloadBytes     atomic.Uint64
	uploadWaitNanos   atomic.Uint64
	downloadWaitNanos atomic.Uint64
}

// Configure replaces both direction limits atomically from the caller's point
// of view and wakes packet goroutines waiting on the previous configuration.
func (s *TrafficShaper) Configure(config TrafficShaperConfig) TrafficShaperStatus {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return s.Status()
	}
	if s.config == config {
		status := s.statusLocked()
		s.mu.Unlock()
		return status
	}
	s.config = config
	s.updatedAt = time.Now()
	s.uploadBytes.Store(0)
	s.downloadBytes.Store(0)
	s.uploadWaitNanos.Store(0)
	s.downloadWaitNanos.Store(0)
	s.upload.configure(config.UploadBitsPerSecond)
	s.download.configure(config.DownloadBitsPerSecond)
	status := s.statusLocked()
	s.mu.Unlock()
	return status
}

func (s *TrafficShaper) waitUpload(byteCount int) error {
	started := time.Now()
	paced, err := s.upload.waitBytes(byteCount)
	if err == nil && paced {
		s.uploadBytes.Add(uint64(max(0, byteCount)))
		s.uploadWaitNanos.Add(uint64(time.Since(started)))
	}
	return err
}

func (s *TrafficShaper) waitDownload(byteCount int) error {
	started := time.Now()
	paced, err := s.download.waitBytes(byteCount)
	if err == nil && paced {
		s.downloadBytes.Add(uint64(max(0, byteCount)))
		s.downloadWaitNanos.Add(uint64(time.Since(started)))
	}
	return err
}

// Status returns a race-free snapshot of the current shaper.
func (s *TrafficShaper) Status() TrafficShaperStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *TrafficShaper) statusLocked() TrafficShaperStatus {
	return TrafficShaperStatus{
		Config:            s.config,
		Active:            s.config.UploadBitsPerSecond > 0 || s.config.DownloadBitsPerSecond > 0,
		UpdatedAt:         s.updatedAt,
		UploadBytes:       s.uploadBytes.Load(),
		DownloadBytes:     s.downloadBytes.Load(),
		UploadWaitNanos:   s.uploadWaitNanos.Load(),
		DownloadWaitNanos: s.downloadWaitNanos.Load(),
	}
}

func (s *TrafficShaper) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.config = TrafficShaperConfig{}
	s.updatedAt = time.Now()
	s.uploadBytes.Store(0)
	s.downloadBytes.Store(0)
	s.uploadWaitNanos.Store(0)
	s.downloadWaitNanos.Store(0)
	s.mu.Unlock()
	s.upload.close()
	s.download.close()
}

// SetTrafficShaper updates the aggregate TUN bandwidth limits immediately.
func (t *Wrapper) SetTrafficShaper(config TrafficShaperConfig) TrafficShaperStatus {
	return t.trafficShaper.Configure(config)
}

// ClearTrafficShaper removes all pacing and resets its counters.
func (t *Wrapper) ClearTrafficShaper() TrafficShaperStatus {
	return t.trafficShaper.Configure(TrafficShaperConfig{})
}

// TrafficShaperStatus returns the active aggregate TUN limits.
func (t *Wrapper) TrafficShaperStatus() TrafficShaperStatus {
	return t.trafficShaper.Status()
}
