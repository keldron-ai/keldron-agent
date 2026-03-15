//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package apple_silicon

// AppleSiliconConfig holds Apple Silicon adapter settings.
// Most config comes from config.AdapterConfig (poll_interval, etc.).
type AppleSiliconConfig struct {
	// Reserved for future adapter-specific options
}
