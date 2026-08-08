// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package localapi

import "testing"

func TestIsNewerOTAVersion(t *testing.T) {
	tests := []struct {
		candidate string
		current   string
		want      bool
	}{
		{"0.0.2", "0.0.1", true},
		{"v1.2.3", "1.2.2", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3-beta.2", "1.2.3-beta.1", true},
		{"1.2.3-beta.1", "1.2.3", false},
		{"1.2", "1.1.9", false},
		{"1.2.03", "1.2.2", false},
	}
	for _, tt := range tests {
		t.Run(tt.candidate+"_from_"+tt.current, func(t *testing.T) {
			if got := isNewerOTAVersion(tt.candidate, tt.current); got != tt.want {
				t.Fatalf("isNewerOTAVersion(%q, %q) = %v, want %v", tt.candidate, tt.current, got, tt.want)
			}
		})
	}
}
