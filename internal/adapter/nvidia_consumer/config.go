//go:build linux || windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package nvidia_consumer

import "time"

// NvidiaConsumerConfig holds NVIDIA consumer (nvidia-smi) adapter settings.
// Decoded from the adapter's Raw YAML node.
type NvidiaConsumerConfig struct {
	Enabled       bool          `yaml:"enabled"`
	PollInterval  time.Duration `yaml:"poll_interval"`
	NvidiaSMIPath string        `yaml:"nvidia_smi_path"` // empty = search PATH
	GPUIndices    []int         `yaml:"gpu_indices"`     // empty = all GPUs
}
