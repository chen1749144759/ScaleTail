// Copyright (c) ScaleTail Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package localapi

import "testing"

func TestQuotaThrottleRate(t *testing.T) {
	tests := []struct {
		name       string
		configured uint64
		want       uint64
	}{
		{name: "unset", configured: 0, want: quotaThrottleBitsPerSecond},
		{name: "above quota fallback", configured: 2_000_000, want: quotaThrottleBitsPerSecond},
		{name: "equal quota fallback", configured: quotaThrottleBitsPerSecond, want: quotaThrottleBitsPerSecond},
		{name: "stricter configured limit", configured: 250_000, want: 250_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quotaThrottleRate(tt.configured); got != tt.want {
				t.Fatalf("quotaThrottleRate(%d)=%d, want %d", tt.configured, got, tt.want)
			}
		})
	}
}
