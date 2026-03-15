//go:build linux || windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package nvidia_consumer

import (
	"strings"

	"github.com/keldron-ai/keldron-agent/registry"
)

// normalizeModelName converts nvidia-smi GPU name to registry key format.
// e.g. "NVIDIA GeForce RTX 4090" -> "RTX-4090" (via registry which uses GeForce aliases).
func normalizeModelName(rawName string) string {
	return registry.NormalizeModelName(strings.TrimSpace(rawName))
}
