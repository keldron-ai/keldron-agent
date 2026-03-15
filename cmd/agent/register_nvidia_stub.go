//go:build !linux && !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package main

import "github.com/keldron-ai/keldron-agent/internal/adapter"

func registerNvidia(registry *adapter.Registry) {
	// nvidia_consumer adapter only compiles on linux/windows; this stub covers non-linux/non-windows platforms (e.g., macOS, *BSD).
}
