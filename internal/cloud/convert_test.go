// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package cloud

import (
	"math"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

func TestConvertToSamples_deviceIDAndDedupe(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	p1 := normalizer.TelemetryPoint{
		Source: "host", AdapterName: "nvidia_consumer",
		Timestamp: ts,
		Metrics: map[string]float64{
			"gpu_id":              1,
			"temperature_c":       60,
			"power_usage_w":       100,
			"gpu_utilization_pct": 50,
		},
	}
	p2 := normalizer.TelemetryPoint{
		Source: "host", AdapterName: "nvidia_consumer",
		Timestamp: ts.Add(time.Second),
		Metrics: map[string]float64{
			"gpu_id":              1,
			"temperature_c":       70,
			"power_usage_w":       110,
			"gpu_utilization_pct": 55,
		},
	}
	scores := []scoring.RiskScoreOutput{
		{DeviceID: "host:1", Composite: 33, Thermal: 10, Power: 20, Volatility: 5, Severity: scoring.SeverityNormal},
	}

	out := ConvertToSamples([]normalizer.TelemetryPoint{p1, p2}, scores, "1.2.3")
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1 (dedupe)", len(out))
	}
	s := out[0]
	if s.DeviceID != "host:1" {
		t.Errorf("device_id = %q", s.DeviceID)
	}
	if s.AdapterType != "nvml" {
		t.Errorf("adapter_type = %q", s.AdapterType)
	}
	if s.CompositeRiskScore != 33 {
		t.Errorf("composite = %v", s.CompositeRiskScore)
	}
	if s.TemperaturePrimary == nil || *s.TemperaturePrimary != 70 {
		t.Errorf("temperature_primary = %v (want last point 70)", s.TemperaturePrimary)
	}
	if s.AgentVersion == nil || *s.AgentVersion != "1.2.3" {
		t.Errorf("agent_version = %v", s.AgentVersion)
	}
}

func TestConvertToSamples_adapterTypeMapping(t *testing.T) {
	t.Parallel()
	ts := time.Now()
	base := func(name string) normalizer.TelemetryPoint {
		return normalizer.TelemetryPoint{
			Source: "s", AdapterName: name, Timestamp: ts,
			Metrics: map[string]float64{"temperature_c": 40},
		}
	}
	sc := scoring.RiskScoreOutput{DeviceID: "s", Composite: 1, Severity: scoring.SeverityNormal}

	tests := []struct {
		adapter, want string
	}{
		{"apple_silicon", "iokit"},
		{"nvidia_consumer", "nvml"},
		{"rocm", "rocm"},
		{"linux_thermal", "linux_thermal"},
		{"dcgm", "dcgm"},
	}
	for _, tt := range tests {
		t.Run(tt.adapter, func(t *testing.T) {
			t.Parallel()
			out := ConvertToSamples([]normalizer.TelemetryPoint{base(tt.adapter)}, []scoring.RiskScoreOutput{sc}, "")
			if len(out) != 1 || out[0].AdapterType != tt.want {
				t.Fatalf("got %+v", out)
			}
		})
	}
}

func TestConvertToSamples_metricPrecedence(t *testing.T) {
	t.Parallel()
	ts := time.Now()
	pt := normalizer.TelemetryPoint{
		Source: "s", AdapterName: "rocm", Timestamp: ts,
		Metrics: map[string]float64{
			"temperature_c": 50,
			"gpu_temp":      99,
		},
	}
	sc := scoring.RiskScoreOutput{DeviceID: "s", Composite: 1, Severity: scoring.SeverityNormal}
	out := ConvertToSamples([]normalizer.TelemetryPoint{pt}, []scoring.RiskScoreOutput{sc}, "")
	if len(out) != 1 {
		t.Fatal(len(out))
	}
	if out[0].TemperaturePrimary == nil || math.Abs(*out[0].TemperaturePrimary-50) > 1e-9 {
		t.Errorf("want temperature_c=50, got %v", out[0].TemperaturePrimary)
	}
}

func TestConvertToSamples_nilMetricsAndTags(t *testing.T) {
	t.Parallel()
	ts := time.Now()
	pt := normalizer.TelemetryPoint{
		Source: "s", AdapterName: "linux_thermal", Timestamp: ts,
		Tags: map[string]string{"hostname": "myhost"},
		Metrics: map[string]float64{
			"cpu_temp_c": 62,
		},
	}
	sc := scoring.RiskScoreOutput{DeviceID: "s", Composite: 5, Thermal: 1, Power: 2, Volatility: 3, Severity: scoring.SeverityWarning}
	out := ConvertToSamples([]normalizer.TelemetryPoint{pt}, []scoring.RiskScoreOutput{sc}, "")
	if len(out) != 1 {
		t.Fatal(len(out))
	}
	s := out[0]
	if s.Hostname != "myhost" {
		t.Errorf("hostname = %q", s.Hostname)
	}
	if s.TemperaturePrimary == nil || *s.TemperaturePrimary != 62 {
		t.Errorf("primary temp = %v", s.TemperaturePrimary)
	}
	if s.PowerDraw != nil {
		t.Errorf("power should be nil, got %v", s.PowerDraw)
	}
	if s.SeverityBand != scoring.SeverityWarning {
		t.Errorf("severity = %q", s.SeverityBand)
	}
}

func TestConvertToSamples_memoryMB(t *testing.T) {
	t.Parallel()
	ts := time.Now()
	pt := normalizer.TelemetryPoint{
		Source: "s", AdapterName: "nvidia_consumer", Timestamp: ts,
		Metrics: map[string]float64{
			"temperature_c":  40,
			"mem_used_bytes": 1024 * 1024 * 512,
		},
	}
	sc := scoring.RiskScoreOutput{DeviceID: "s", Composite: 1, Severity: scoring.SeverityNormal}
	out := ConvertToSamples([]normalizer.TelemetryPoint{pt}, []scoring.RiskScoreOutput{sc}, "")
	if len(out) != 1 || out[0].MemoryUsed == nil || math.Abs(*out[0].MemoryUsed-512) > 1e-6 {
		t.Fatalf("memory MB = %v", out[0].MemoryUsed)
	}
}
