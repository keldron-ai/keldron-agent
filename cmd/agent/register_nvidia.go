//go:build linux || windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package main

import (
	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/adapter/nvidia_consumer"
)

func registerNvidia(registry *adapter.Registry) {
	registry.Register("nvidia_consumer", nvidia_consumer.New)
}
