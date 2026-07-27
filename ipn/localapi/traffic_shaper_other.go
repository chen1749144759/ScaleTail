// Copyright (c) ScaleTail Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !windows

package localapi

func clearLegacyUploadThrottle() error { return nil }
