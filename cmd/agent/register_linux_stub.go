//go:build !linux

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package main

import "github.com/keldron-ai/keldron-agent/internal/adapter"

func registerLinuxAdapters(registry *adapter.Registry) {
	// linux_thermal adapter only compiles on linux; this stub covers non-linux platforms.
}
