// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import "github.com/keldron-ai/keldron-agent/internal/health"

// StatusResponse is the JSON shape for GET /api/v1/status.
type StatusResponse struct {
	Device    DeviceInfo                   `json:"device"`
	Telemetry TelemetryInfo                `json:"telemetry"`
	Risk      RiskSummary                  `json:"risk"`
	Agent     AgentInfo                    `json:"agent"`
	Health    *health.DeviceHealthSnapshot `json:"health,omitempty"`
}

type DeviceInfo struct {
	Hostname      string  `json:"hostname"`
	Adapter       string  `json:"adapter"`
	Hardware      string  `json:"hardware"`
	BehaviorClass string  `json:"behavior_class"`
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

type TelemetryInfo struct {
	Timestamp           string   `json:"timestamp"`
	TemperatureC        float64  `json:"temperature_c"`
	GPUUtilizationPct   float64  `json:"gpu_utilization_pct"`
	PowerDrawW          float64  `json:"power_draw_w"`
	MemoryUsedPct       float64  `json:"memory_used_pct"`
	MemoryUsedBytes     int64    `json:"memory_used_bytes"`
	MemoryTotalBytes    int64    `json:"memory_total_bytes"`
	ThermalState        string   `json:"thermal_state"`
	ThrottleActive      bool     `json:"throttle_active"`
	FanRPM              *float64 `json:"fan_rpm,omitempty"`
	NeuralEngineUtilPct *float64 `json:"neural_engine_util_pct,omitempty"`
}

type RiskSummary struct {
	CompositeScore float64 `json:"composite_score"`
	Severity       string  `json:"severity"`
	Trend          string  `json:"trend"`
	TrendDelta     float64 `json:"trend_delta"`
}

type AgentInfo struct {
	Version        string   `json:"version"`
	PollIntervalS  int      `json:"poll_interval_s"`
	AdaptersActive []string `json:"adapters_active"`
	CloudConnected bool     `json:"cloud_connected"`
}

// RiskResponse is the JSON shape for GET /api/v1/risk.
type RiskResponse struct {
	Timestamp  string        `json:"timestamp"`
	Composite  CompositeInfo `json:"composite"`
	SubScores  SubScores     `json:"sub_scores"`
	Thresholds Thresholds    `json:"thresholds"`
}

type CompositeInfo struct {
	Score      float64 `json:"score"`
	Severity   string  `json:"severity"`
	Trend      string  `json:"trend"`
	TrendDelta float64 `json:"trend_delta"`
}

type SubScores struct {
	Thermal    SubScoreDetail `json:"thermal"`
	Power      SubScoreDetail `json:"power"`
	Volatility SubScoreDetail `json:"volatility"`
	Correlated SubScoreDetail `json:"correlated"`
}

type SubScoreDetail struct {
	Score                float64                `json:"score"`
	Weight               float64                `json:"weight"`
	WeightedContribution float64                `json:"weighted_contribution"`
	Details              map[string]interface{} `json:"details"`
}

type Thresholds struct {
	Warning  float64 `json:"warning"`
	Critical float64 `json:"critical"`
}

// ProcessResponse is the JSON shape for GET /api/v1/processes.
type ProcessResponse struct {
	Timestamp string       `json:"timestamp"`
	Processes []GPUProcess `json:"processes"`
	Supported bool         `json:"supported"`
	Note      *string      `json:"note,omitempty"`
}

type GPUProcess struct {
	PID               int     `json:"pid"`
	Name              string  `json:"name"`
	GPUMemoryBytes    int64   `json:"gpu_memory_bytes"`
	GPUUtilizationPct float64 `json:"gpu_utilization_pct"`
	RuntimeSeconds    int64   `json:"runtime_seconds"`
	User              string  `json:"user"`
}

// TelemetryUpdate is the JSON shape for WebSocket /ws/telemetry messages.
type TelemetryUpdate struct {
	Type      string                `json:"type"`
	Timestamp string                `json:"timestamp"`
	Telemetry TelemetryShort        `json:"telemetry"`
	Risk      RiskShort             `json:"risk"`
	Health    *health.HealthSummary `json:"health,omitempty"`
}

type TelemetryShort struct {
	TemperatureC      float64 `json:"temperature_c"`
	GPUUtilizationPct float64 `json:"gpu_utilization_pct"`
	PowerDrawW        float64 `json:"power_draw_w"`
	MemoryUsedPct     float64 `json:"memory_used_pct"`
	ThermalState      string  `json:"thermal_state"`
	ThrottleActive    bool    `json:"throttle_active"`
}

type RiskShort struct {
	CompositeScore float64 `json:"composite_score"`
	Severity       string  `json:"severity"`
	Trend          string  `json:"trend"`
}
