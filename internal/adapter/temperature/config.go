// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package temperature implements a temperature sensor adapter that polls
// inlet/outlet temperatures via SNMP or Modbus TCP (S-047).
package temperature

import (
	"time"
)

// TemperatureConfig holds temperature adapter-specific configuration decoded from the adapter's Raw YAML node.
type TemperatureConfig struct {
	Enabled        bool           `yaml:"enabled"`
	PollInterval   time.Duration  `yaml:"poll_interval"`
	StaleThreshold int            `yaml:"stale_threshold"`
	Sensors        []SensorConfig `yaml:"sensors"`
}

// SensorConfig holds per-sensor configuration.
type SensorConfig struct {
	Address         string  `yaml:"address"`
	Protocol        string  `yaml:"protocol"` // "snmp" or "modbus"
	SensorID        string  `yaml:"sensor_id"`
	RackID          string  `yaml:"rack_id"`
	Position        string  `yaml:"position"` // "inlet" or "outlet"
	Community       string  `yaml:"community"`
	OID             string  `yaml:"oid"`
	Encoding        string  `yaml:"encoding"` // "tenths" (235->23.5) or "raw" or "float" (parse string)
	UnitID          uint8   `yaml:"unit_id"`
	RegisterAddress uint16  `yaml:"register_address"`
	RegisterType    string  `yaml:"register_type"` // "holding" or "input"
	ScaleFactor     float64 `yaml:"scale_factor"`
}
