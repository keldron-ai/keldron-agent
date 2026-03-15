//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package apple_silicon

import (
	"os/exec"
	"strings"

	"github.com/keldron-ai/keldron-agent/registry"
)

// DetectChip returns the normalized chip name (e.g. "M4-Pro") from sysctl.
// Uses machdep.cpu.brand_string which reports "Apple M4 Pro" etc.
func DetectChip() (string, error) {
	out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(string(out))
	return NormalizeChipName(raw), nil
}

// NormalizeChipName converts raw brand string to registry key format.
// "Apple M4 Pro" → "M4-Pro", "Apple M1" → "M1", "Apple M3 Max" → "M3-Max".
func NormalizeChipName(raw string) string {
	return registry.NormalizeModelName(raw)
}
