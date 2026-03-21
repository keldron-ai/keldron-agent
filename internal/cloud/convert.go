// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package cloud

import (
	"strconv"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

// ConvertToSamples converts a batch of TelemetryPoints and RiskScoreOutputs into cloud API samples.
// One sample per unique device (last point wins per device), matching scoring's grouping.
func ConvertToSamples(
	points []normalizer.TelemetryPoint,
	scores []scoring.RiskScoreOutput,
	agentVersion string,
) []Sample {
	if len(points) == 0 || len(scores) == 0 {
		return nil
	}

	scoreByDevice := make(map[string]scoring.RiskScoreOutput, len(scores))
	for _, s := range scores {
		scoreByDevice[s.DeviceID] = s
	}

	// Last point per device (same order as scoring: map iteration order not used; we iterate points in order).
	byDevice := make(map[string]normalizer.TelemetryPoint)
	for _, pt := range points {
		did := deviceIDFromPoint(pt)
		byDevice[did] = pt
	}

	out := make([]Sample, 0, len(byDevice))
	for did, pt := range byDevice {
		score, ok := scoreByDevice[did]
		if !ok {
			continue
		}
		out = append(out, pointToSample(pt, score, agentVersion))
	}
	return out
}

// deviceIDFromPoint mirrors internal/scoring/engine.go deviceIDFromPoint.
func deviceIDFromPoint(pt normalizer.TelemetryPoint) string {
	if pt.Metrics != nil {
		if gpuID, ok := pt.Metrics["gpu_id"]; ok {
			return pt.Source + ":" + strconv.FormatFloat(gpuID, 'f', 0, 64)
		}
	}
	return pt.Source
}

func pointToSample(pt normalizer.TelemetryPoint, score scoring.RiskScoreOutput, agentVersion string) Sample {
	m := pt.Metrics
	if m == nil {
		m = map[string]float64{}
	}

	hostname := pt.Tags["hostname"]
	if hostname == "" {
		hostname = pt.Source
	}

	s := Sample{
		DeviceID:           deviceIDFromPoint(pt),
		Hostname:           hostname,
		AdapterType:        mapAdapterType(pt.AdapterName),
		HardwareModel:      hardwareModel(pt),
		Timestamp:          pt.Timestamp.Format(time.RFC3339Nano),
		CompositeRiskScore: score.Composite,
		SeverityBand:       score.Severity,
	}

	s.TemperaturePrimary = firstFloatPtr(m,
		"temperature_c", "temperature_junction_c", "temperature_edge",
		"gpu_temp", "cpu_temp_c", "temperature",
	)
	s.TemperatureSecondary = firstFloatPtr(m, "memory_temp", "ambient_temp")
	s.PowerDraw = firstFloatPtr(m, "power_usage_w", "gpu_power_w", "power_w")
	s.Utilization = firstFloatPtr(m, "gpu_utilization_pct", "gpu_utilization", "gpu_util_pct")
	s.FanSpeed = firstFloatPtr(m, "fan_speed_rpm")
	s.ClockSpeed = firstFloatPtr(m, "sm_clock_mhz", "gpu_clock_mhz")

	if memMB := memoryUsedMB(m); memMB != nil {
		s.MemoryUsed = memMB
	}

	s.ThermalSubScore = floatPtr(score.Thermal)
	s.PowerSubScore = floatPtr(score.Power)
	s.VolatilitySubScore = floatPtr(score.Volatility)

	if agentVersion != "" {
		s.AgentVersion = &agentVersion
	}
	return s
}

func mapAdapterType(adapterName string) string {
	switch adapterName {
	case "apple_silicon":
		return "iokit"
	case "nvidia_consumer":
		return "nvml"
	case "rocm":
		return "rocm"
	case "linux_thermal":
		return "linux_thermal"
	default:
		return adapterName
	}
}

func hardwareModel(pt normalizer.TelemetryPoint) string {
	if pt.Tags != nil {
		if v := pt.Tags["gpu_model"]; v != "" {
			return v
		}
		if v := pt.Tags["chip_name"]; v != "" {
			return v
		}
		if v := pt.Tags["device_model"]; v != "" {
			return v
		}
	}
	return ""
}

func firstFloatPtr(m map[string]float64, keys ...string) *float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return floatPtr(v)
		}
	}
	return nil
}

func floatPtr(v float64) *float64 {
	return &v
}

func memoryUsedMB(m map[string]float64) *float64 {
	if v, ok := m["mem_used_bytes"]; ok && v > 0 {
		mb := v / (1024 * 1024)
		return &mb
	}
	if v, ok := m["gpu_memory_used_mb"]; ok {
		return &v
	}
	if v, ok := m["gpu_memory_used"]; ok && v > 0 {
		mb := v / (1024 * 1024)
		return &mb
	}
	return nil
}
