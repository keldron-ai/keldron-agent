// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import "time"

// TDRResult is Thermal Range / headroom compression over the health window.
type TDRResult struct {
	Available        bool    `json:"available"`
	NoSustainedLoad  bool    `json:"no_sustained_load,omitempty"`
	WarmingUp        bool    `json:"warming_up,omitempty"`
	AvgTempC         float64 `json:"avg_temp_c,omitempty"`
	MaxTempC         float64 `json:"max_temp_c,omitempty"`
	HeadroomUsedPct  float64 `json:"headroom_used_pct,omitempty"`
	PeakProximityPct float64 `json:"peak_proximity_pct,omitempty"`
	ThrottleLimitC   float64 `json:"throttle_limit_c,omitempty"`
	Rating           string  `json:"rating"`
	Note             string  `json:"note,omitempty"`
}

// TREResult is thermal recovery (time from peak to below 50% envelope threshold).
type TREResult struct {
	Available       bool    `json:"available"`
	NoSpikes        bool    `json:"no_spikes,omitempty"`
	SpikeActive     bool    `json:"spike_active,omitempty"`
	WarmingUp       bool    `json:"warming_up,omitempty"`
	ActiveSpikeSec  int     `json:"active_spike_seconds,omitempty"`
	LastRecoverySec int     `json:"last_recovery_seconds,omitempty"`
	LastPeakTempC   float64 `json:"last_peak_temp_c,omitempty"`
	RecoveryTargetC float64 `json:"recovery_target_c,omitempty"`
	Rating          string  `json:"rating"`
	Note            string  `json:"note,omitempty"`
}

// PerfPerWattResult is mean utilization over mean power in the health window.
type PerfPerWattResult struct {
	Available bool    `json:"available"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	UnitID    string  `json:"unit_id,omitempty"`
	Note      string  `json:"note,omitempty"`
}

// ThermalStabilityResult is standard deviation of temperature over the health window.
type ThermalStabilityResult struct {
	Available     bool    `json:"available"`
	WarmingUp     bool    `json:"warming_up,omitempty"`
	StdDevCelsius float64 `json:"std_dev_celsius,omitempty"`
	Rating        string  `json:"rating,omitempty"`
	Note          string  `json:"note,omitempty"`
}

// DeviceHealthSnapshot is the full health snapshot for one device (status API).
type DeviceHealthSnapshot struct {
	WarmingUp           bool                    `json:"warming_up"`
	ThermalDynamicRange *TDRResult              `json:"thermal_dynamic_range"`
	ThermalRecovery     *TREResult              `json:"thermal_recovery"`
	PerfPerWatt         *PerfPerWattResult      `json:"perf_per_watt"`
	ThermalStability    *ThermalStabilityResult `json:"thermal_stability"`
}

// HealthSummary is the lightweight health summary for WebSocket stream.
type HealthSummary struct {
	WarmingUp       *bool    `json:"warming_up,omitempty"`
	HeadroomUsedPct *float64 `json:"headroom_used_pct,omitempty"`
	TDRRating       string   `json:"tdr_rating,omitempty"`
	StdDevCelsius   *float64 `json:"std_dev_celsius,omitempty"`
	PerfPerWatt     *float64 `json:"perf_per_watt,omitempty"`
}

// Clone returns a deep copy of the snapshot.
func (s *DeviceHealthSnapshot) Clone() *DeviceHealthSnapshot {
	if s == nil {
		return nil
	}
	c := &DeviceHealthSnapshot{WarmingUp: s.WarmingUp}
	if s.ThermalDynamicRange != nil {
		v := *s.ThermalDynamicRange
		c.ThermalDynamicRange = &v
	}
	if s.ThermalRecovery != nil {
		v := *s.ThermalRecovery
		c.ThermalRecovery = &v
	}
	if s.PerfPerWatt != nil {
		v := *s.PerfPerWatt
		c.PerfPerWatt = &v
	}
	if s.ThermalStability != nil {
		v := *s.ThermalStability
		c.ThermalStability = &v
	}
	return c
}

// ToHealthSummary converts a DeviceHealthSnapshot to a lightweight HealthSummary for WebSocket.
func (s *DeviceHealthSnapshot) ToHealthSummary() *HealthSummary {
	if s == nil {
		return nil
	}
	summary := &HealthSummary{}
	wu := s.WarmingUp
	summary.WarmingUp = &wu
	if s.ThermalDynamicRange != nil && s.ThermalDynamicRange.Available && !s.ThermalDynamicRange.NoSustainedLoad {
		h := s.ThermalDynamicRange.HeadroomUsedPct
		summary.HeadroomUsedPct = &h
		summary.TDRRating = s.ThermalDynamicRange.Rating
	}
	if s.ThermalStability != nil && s.ThermalStability.Available {
		sd := s.ThermalStability.StdDevCelsius
		summary.StdDevCelsius = &sd
	}
	if s.PerfPerWatt != nil && s.PerfPerWatt.Available {
		summary.PerfPerWatt = &s.PerfPerWatt.Value
	}
	return summary
}

// RecoveryEvent records a completed thermal recovery (internal use / tests).
type RecoveryEvent struct {
	Timestamp       time.Time
	PeakTempC       float64
	RecoverySeconds int
}
