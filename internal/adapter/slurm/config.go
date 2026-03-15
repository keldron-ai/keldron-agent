// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package slurm

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/config"
)

// SlurmConfig holds Slurm-specific configuration decoded from the adapter's Raw YAML node.
type SlurmConfig struct {
	SlurmrestdURL string            `yaml:"slurmrestd_url"`
	APIVersion    string            `yaml:"api_version"`
	AuthToken     string            `yaml:"auth_token"`
	PollInterval  time.Duration     `yaml:"poll_interval"`
	Timeout       time.Duration     `yaml:"timeout"`
	NodeToRackMap map[string]string `yaml:"node_to_rack_map"`
}

func init() {
	config.RegisterPostLoadHook(MergeNodeToRackMap)
}

// MergeNodeToRackMap merges the Slurm adapter's node_to_rack_map into the global
// rack_mapping when the slurm adapter is enabled.
func MergeNodeToRackMap(cfg *config.Config) error {
	acfg, ok := cfg.Adapters[adapterName]
	if !ok || !acfg.Enabled {
		return nil
	}
	var sc SlurmConfig
	if err := acfg.Raw.Decode(&sc); err != nil {
		return fmt.Errorf("decoding slurm adapter config: %w", err)
	}
	if cfg.RackMapping == nil {
		cfg.RackMapping = make(map[string]string)
	}
	for k, v := range sc.NodeToRackMap {
		if old, exists := cfg.RackMapping[k]; exists && old != v {
			slog.Debug("slurm node_to_rack_map overwrites existing rack mapping",
				"node", k, "old_rack", old, "new_rack", v)
		}
		cfg.RackMapping[k] = v
	}
	return nil
}
