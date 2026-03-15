// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"time"

	"github.com/keldron-ai/keldron-agent/registry"
)

// RiskScoreOutput holds the complete risk score and bonus metrics for a device.
type RiskScoreOutput struct {
	DeviceID          string
	Composite         float64
	Thermal           float64
	ThermalRoCPenalty float64
	Power             float64
	Volatility        float64
	FleetPenalty      float64
	Severity          string // "normal", "warning", "critical"
	Trend             string // "rising", "falling", "stable"
	TimeToHotspot     *float64
	BehaviorClass     string
	WarmingUp         bool

	// Bonus metrics
	MemoryPressure   float64
	ThrottleActive   bool
	ThrottleReason   string
	ClockEfficiency  float64
	PowerCostHourly  float64
	PowerCostDaily   float64
	PowerCostMonthly float64
	HotspotDeltaC    float64 // -1 if unavailable
	SwapPressure     float64 // 0.0-1.0, Apple Silicon only
}

// Severity constants.
const (
	SeverityNormal   = "normal"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// DeviceScoreState holds per-device state for scoring (buffers, last composite).
type DeviceScoreState struct {
	DeviceID      string
	Spec          registry.GPUSpec
	ThermalBuffer *RingBuffer // 5-min (~10 readings at 30s poll)
	VolBuffer     *RingBuffer // 30-min (~60 readings at 30s poll)
	LastComposite float64
	LastUpdate    time.Time
}
