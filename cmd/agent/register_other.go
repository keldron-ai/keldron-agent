//go:build !darwin || !arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package main

import (
	"github.com/keldron-ai/keldron-agent/internal/adapter"
)

func registerPlatformAdapters(registry *adapter.Registry) {
	// apple_silicon only compiles on darwin/arm64
}
