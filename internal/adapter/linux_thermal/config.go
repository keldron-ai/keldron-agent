//go:build linux

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package linux_thermal

const (
	defaultHwmonPath   = "/sys/class/hwmon"
	defaultThermalPath = "/sys/class/thermal"
)

// LinuxThermalAdapterConfig holds adapter-specific configuration decoded from YAML.
type LinuxThermalAdapterConfig struct {
	HwmonPath    string   `yaml:"hwmon_path"`
	ThermalPath  string   `yaml:"thermal_path"`
	IncludeZones []string `yaml:"include_zones"`
	ExcludeZones []string `yaml:"exclude_zones"`
}

// applyDefaults sets default paths when empty.
func (c *LinuxThermalAdapterConfig) applyDefaults() {
	if c.HwmonPath == "" {
		c.HwmonPath = defaultHwmonPath
	}
	if c.ThermalPath == "" {
		c.ThermalPath = defaultThermalPath
	}
}
