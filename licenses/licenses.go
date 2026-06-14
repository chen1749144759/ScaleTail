// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package licenses provides utilities for working with open source licenses.
package licenses

import "runtime"

// LicensesURL returns the absolute URL containing open source license information for the current platform.
func LicensesURL() string {
	switch runtime.GOOS {
	case "android":
		return "https://scaletail.com/licenses/android"
	case "darwin", "ios":
		return "https://scaletail.com/licenses/apple"
	case "windows":
		return "https://scaletail.com/licenses/windows"
	default:
		return "https://scaletail.com/licenses/scaletail"
	}
}
