//go:build linux

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package main

import (
	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/adapter/linux_thermal"
)

func registerLinuxAdapters(registry *adapter.Registry) {
	registry.Register("linux_thermal", linux_thermal.New)
}
