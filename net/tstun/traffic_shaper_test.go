// Copyright (c) ScaleTail Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package tstun

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTrafficShaperConfigureAndClear(t *testing.T) {
	var shaper TrafficShaper
	status := shaper.Configure(TrafficShaperConfig{
		UploadBitsPerSecond:   1_000_000,
		DownloadBitsPerSecond: 2_000_000,
	})
	if !status.Active {
		t.Fatal("configured shaper is not active")
	}
	if status.Config.UploadBitsPerSecond != 1_000_000 || status.Config.DownloadBitsPerSecond != 2_000_000 {
		t.Fatalf("unexpected config: %+v", status.Config)
	}

	status = shaper.Configure(TrafficShaperConfig{})
	if status.Active || status.Config != (TrafficShaperConfig{}) {
		t.Fatalf("clear left active state: %+v", status)
	}
	if status.UploadBytes != 0 || status.DownloadBytes != 0 {
		t.Fatalf("clear left counters: %+v", status)
	}
}

func TestTrafficShaperUnlimitedDoesNotRetainState(t *testing.T) {
	var shaper TrafficShaper
	if err := shaper.waitUpload(4096); err != nil {
		t.Fatalf("unlimited waitUpload: %v", err)
	}
	if err := shaper.waitDownload(8192); err != nil {
		t.Fatalf("unlimited waitDownload: %v", err)
	}
	status := shaper.Status()
	if status.Active || status.UploadBytes != 0 || status.DownloadBytes != 0 {
		t.Fatalf("unlimited shaper retained runtime state: %+v", status)
	}
}

func TestTrafficShaperAggregateRate(t *testing.T) {
	var shaper TrafficShaper
	shaper.Configure(TrafficShaperConfig{UploadBitsPerSecond: 800_000}) // 100 kB/s.

	started := time.Now()
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- shaper.waitUpload(10_000)
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("waitUpload: %v", err)
		}
	}

	elapsed := time.Since(started)
	if elapsed < 140*time.Millisecond {
		t.Fatalf("aggregate limit was bypassed; 20 kB completed in %v", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("aggregate limit took unexpectedly long: %v", elapsed)
	}
	if got := shaper.Status().UploadBytes; got != 20_000 {
		t.Fatalf("UploadBytes=%d, want 20000", got)
	}
}

func TestTrafficShaperRepeatedConfigPreservesState(t *testing.T) {
	var shaper TrafficShaper
	config := TrafficShaperConfig{UploadBitsPerSecond: 8_000_000}
	configured := shaper.Configure(config)
	if err := shaper.waitUpload(4096); err != nil {
		t.Fatalf("waitUpload: %v", err)
	}
	before := shaper.Status()
	after := shaper.Configure(config)

	if !after.UpdatedAt.Equal(configured.UpdatedAt) {
		t.Fatalf("unchanged config reset UpdatedAt: before=%v after=%v", configured.UpdatedAt, after.UpdatedAt)
	}
	if after.UploadBytes != before.UploadBytes || after.UploadBytes != 4096 {
		t.Fatalf("unchanged config reset counters: before=%d after=%d", before.UploadBytes, after.UploadBytes)
	}
}

func TestTrafficShaperHotRateUpdateWakesWaiters(t *testing.T) {
	var shaper TrafficShaper
	shaper.Configure(TrafficShaperConfig{DownloadBitsPerSecond: 8}) // 1 byte/s.

	done := make(chan error, 1)
	go func() { done <- shaper.waitDownload(4096) }()
	time.Sleep(25 * time.Millisecond)
	shaper.Configure(TrafficShaperConfig{DownloadBitsPerSecond: 8_000_000})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("waitDownload after hot update: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("hot rate update did not wake the waiting packet path")
	}
}

func TestTrafficShaperHotClearWakesWaiters(t *testing.T) {
	var shaper TrafficShaper
	shaper.Configure(TrafficShaperConfig{DownloadBitsPerSecond: 8}) // 1 byte/s.

	done := make(chan error, 1)
	go func() { done <- shaper.waitDownload(4096) }()
	time.Sleep(25 * time.Millisecond)
	shaper.Configure(TrafficShaperConfig{})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("waitDownload after hot clear: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("hot clear did not wake the waiting packet path")
	}
}

func TestTrafficShaperCloseWakesWaiters(t *testing.T) {
	var shaper TrafficShaper
	shaper.Configure(TrafficShaperConfig{UploadBitsPerSecond: 8})

	done := make(chan error, 1)
	go func() { done <- shaper.waitUpload(4096) }()
	time.Sleep(25 * time.Millisecond)
	shaper.close()

	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("waitUpload after close=%v, want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not wake the waiting packet path")
	}

	status := shaper.Status()
	if status.Active || status.UploadBytes != 0 || status.DownloadBytes != 0 {
		t.Fatalf("close left runtime state: %+v", status)
	}
}
