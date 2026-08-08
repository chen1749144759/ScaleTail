// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package localapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"scaletail.com/ipn"
	"scaletail.com/net/tstun"
	"scaletail.com/util/httpm"
)

const (
	maxTrafficShaperBitsPerSecond = uint64(1_000_000_000_000)
	quotaThrottleBitsPerSecond    = uint64(500_000)
)

type trafficShaperRequest struct {
	UploadBitsPerSecond   uint64 `json:"upload_bits_per_second"`
	DownloadBitsPerSecond uint64 `json:"download_bits_per_second"`
	QuotaExceeded         bool   `json:"quota_exceeded"`
	ExceedAction          string `json:"exceed_action"`
}

type trafficShaperResponse struct {
	Status        tstun.TrafficShaperStatus `json:"status"`
	QuotaExceeded bool                      `json:"quota_exceeded"`
	ExceedAction  string                    `json:"exceed_action"`
	Blocked       bool                      `json:"blocked"`
	Warning       string                    `json:"warning,omitempty"`
}

func init() {
	Register("traffic-shaper", (*Handler).serveTrafficShaper)
}

func (h *Handler) serveTrafficShaper(w http.ResponseWriter, r *http.Request) {
	tun, ok := h.b.Sys().Tun.GetOK()
	if !ok {
		http.Error(w, "ScaleTail TUN is not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case httpm.GET:
		if !h.PermitRead {
			http.Error(w, "traffic shaper access denied", http.StatusForbidden)
			return
		}
		writeTrafficShaperJSON(w, trafficShaperResponse{
			Status:       tun.TrafficShaperStatus(),
			ExceedAction: "alert",
		})
		return
	case httpm.POST:
		if !h.PermitWrite {
			http.Error(w, "traffic shaper access denied", http.StatusForbidden)
			return
		}
	default:
		http.Error(w, "want GET or POST", http.StatusMethodNotAllowed)
		return
	}

	var req trafficShaperRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid traffic shaper request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		http.Error(w, "invalid traffic shaper request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.UploadBitsPerSecond > maxTrafficShaperBitsPerSecond || req.DownloadBitsPerSecond > maxTrafficShaperBitsPerSecond {
		http.Error(w, "traffic shaper rate exceeds the supported maximum", http.StatusBadRequest)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.ExceedAction))
	if action == "" {
		action = "alert"
	}
	if action != "alert" && action != "throttle" && action != "block" {
		http.Error(w, "exceed_action must be alert, throttle, or block", http.StatusBadRequest)
		return
	}

	config := tstun.TrafficShaperConfig{
		UploadBitsPerSecond:   req.UploadBitsPerSecond,
		DownloadBitsPerSecond: req.DownloadBitsPerSecond,
	}
	response := trafficShaperResponse{QuotaExceeded: req.QuotaExceeded, ExceedAction: action}

	if req.QuotaExceeded {
		switch action {
		case "throttle":
			config.UploadBitsPerSecond = quotaThrottleRate(config.UploadBitsPerSecond)
			config.DownloadBitsPerSecond = quotaThrottleRate(config.DownloadBitsPerSecond)
		case "block":
			_, err := h.b.EditPrefsAs(&ipn.MaskedPrefs{
				Prefs:          ipn.Prefs{WantRunning: false},
				WantRunningSet: true,
			}, h.Actor)
			if err != nil {
				http.Error(w, "disable ScaleTail after quota exceeded: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// EditPrefs synchronously stops the backend and clears the shaper.
			// Clear again defensively so a successful block never leaves pacing
			// state behind if the backend was already stopped.
			response.Status = tun.ClearTrafficShaper()
			response.Blocked = true
		}
	}

	if !response.Blocked {
		response.Status = tun.SetTrafficShaper(config)
	}
	if err := clearLegacyUploadThrottle(); err != nil {
		response.Warning = fmt.Sprintf("clear legacy Windows QoS policy: %v", err)
		h.logf("traffic-shaper: %s", response.Warning)
	}
	h.logf("traffic-shaper: upload=%d bps download=%d bps quota_exceeded=%v action=%s blocked=%v",
		response.Status.Config.UploadBitsPerSecond,
		response.Status.Config.DownloadBitsPerSecond,
		response.QuotaExceeded,
		response.ExceedAction,
		response.Blocked)
	writeTrafficShaperJSON(w, response)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func quotaThrottleRate(configured uint64) uint64 {
	if configured == 0 || configured > quotaThrottleBitsPerSecond {
		return quotaThrottleBitsPerSecond
	}
	return configured
}

func writeTrafficShaperJSON(w http.ResponseWriter, response trafficShaperResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
