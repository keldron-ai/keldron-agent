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
	Memory            float64
	TDPW              float64 // watts from registry spec (for cloud ingest)
	Severity          string  // "normal", "active", "elevated", "warning", "critical"
	Trend             string  // "rising", "falling", "stable"
	TrendDelta        float64 // composite - previous composite (for API)
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

// Severity constants (composite score bands).
const (
	SeverityNormal   = "normal"   // idle or light use
	SeverityActive   = "active"   // working, expected under load
	SeverityElevated = "elevated" // running hard, worth monitoring
	SeverityWarning  = "warning"  // approaching limits
	SeverityCritical = "critical" // near throttle/shutdown
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
