// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import "time"

// tempSample holds temperature and timestamp for rolling buffers.
type tempSample struct {
	tempC float64
	at    time.Time
}

// TDRResult is the result of Thermal Dynamic Range computation.
type TDRResult struct {
	Available       bool    `json:"available"`
	TDRCelsius      float64 `json:"tdr_celsius"`
	IdleTempC       float64 `json:"idle_temp_c"`
	PeakTempC       float64 `json:"peak_temp_c"`
	Rating          string  `json:"rating"`
	IdleSampleCount int     `json:"idle_sample_count"`
	PeakSampleCount int     `json:"peak_sample_count"`
	WindowHours     int     `json:"window_hours"`
	Note            string  `json:"note,omitempty"`
}

// RecoveryEvent records a completed thermal recovery.
type RecoveryEvent struct {
	Timestamp       time.Time
	PeakTempC       float64
	BaselineTempC   float64
	RecoverySeconds int
}

// TREResult is the result of Thermal Recovery Efficiency.
type TREResult struct {
	Available         bool    `json:"available"`
	LastRecoverySec   int     `json:"last_recovery_seconds"`
	LastPeakTempC     float64 `json:"last_peak_temp_c"`
	LastBaselineTempC float64 `json:"last_baseline_temp_c"`
	Rating            string  `json:"rating"`
	RecoveryCount     int     `json:"recovery_count"`
	SessionAvgSec     int     `json:"session_avg_seconds"`
	Note              string  `json:"note,omitempty"`
}

// PerfPerWattResult is the result of Performance-per-Watt computation.
type PerfPerWattResult struct {
	Available bool    `json:"available"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
}

// ThermalStabilityResult is the result of Thermal Stability Index.
type ThermalStabilityResult struct {
	Available          bool    `json:"available"`
	UnderSustainedLoad bool    `json:"under_sustained_load"`
	StabilityCelsius   float64 `json:"stability_celsius,omitempty"`
	Rating             string  `json:"rating,omitempty"`
	Note               string  `json:"note,omitempty"`
}

// DeviceHealthSnapshot is the full health snapshot for one device (status API).
type DeviceHealthSnapshot struct {
	ThermalDynamicRange *TDRResult              `json:"thermal_dynamic_range"`
	ThermalRecovery     *TREResult              `json:"thermal_recovery"`
	PerfPerWatt         *PerfPerWattResult      `json:"perf_per_watt"`
	ThermalStability    *ThermalStabilityResult `json:"thermal_stability"`
}

// HealthSummary is the lightweight health summary for WebSocket stream.
type HealthSummary struct {
	TDRCelsius       *float64 `json:"tdr_celsius,omitempty"`
	TDRRating        string   `json:"tdr_rating,omitempty"`
	StabilityCelsius *float64 `json:"stability_celsius,omitempty"`
	PerfPerWatt      *float64 `json:"perf_per_watt,omitempty"`
}

// ToHealthSummary converts a DeviceHealthSnapshot to a lightweight HealthSummary for WebSocket.
func (s *DeviceHealthSnapshot) ToHealthSummary() *HealthSummary {
	if s == nil {
		return nil
	}
	summary := &HealthSummary{}
	if s.ThermalDynamicRange != nil && s.ThermalDynamicRange.Available {
		summary.TDRCelsius = &s.ThermalDynamicRange.TDRCelsius
		summary.TDRRating = s.ThermalDynamicRange.Rating
	}
	if s.ThermalStability != nil && s.ThermalStability.Available {
		summary.StabilityCelsius = &s.ThermalStability.StabilityCelsius
	}
	if s.PerfPerWatt != nil && s.PerfPerWatt.Available {
		summary.PerfPerWatt = &s.PerfPerWatt.Value
	}
	return summary
}
