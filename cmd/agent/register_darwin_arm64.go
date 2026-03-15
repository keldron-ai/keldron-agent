//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package main

import (
	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/adapter/apple_silicon"
)

func registerPlatformAdapters(registry *adapter.Registry) {
	registry.Register("apple_silicon", apple_silicon.New)
}
