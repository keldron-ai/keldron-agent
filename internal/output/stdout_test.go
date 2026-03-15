// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package output

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func TestStdout_Update_JSONSchema(t *testing.T) {
	var buf bytes.Buffer
	std := NewStdout(&buf, "0.1.0-dev", []string{"dcgm", "fake"})

	readings := []normalizer.TelemetryPoint{
		{
			ID:          "01HXXX",
			AgentID:     "agent-1",
			AdapterName: "dcgm",
			Source:      "host1",
			RackID:      "rack-1",
			Timestamp:   time.Now(),
			ReceivedAt:  time.Now(),
			Metrics: map[string]float64{
				"temperature_c":       72.5,
				"power_usage_w":       350.0,
				"gpu_utilization_pct": 85.0,
			},
		},
	}

	if err := std.Update(readings, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	var out StdoutLine
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, line)
	}

	if out.Timestamp == "" {
		t.Error("expected timestamp")
	}
	if _, err := time.Parse(time.RFC3339, out.Timestamp); err != nil {
		t.Errorf("timestamp not valid ISO 8601: %v", err)
	}
	if len(out.Devices) != 1 {
		t.Errorf("devices len = %d, want 1", len(out.Devices))
	}
	if out.Devices[0].DeviceID == "" {
		t.Error("expected device_id")
	}
	if out.Devices[0].TemperatureC == nil || *out.Devices[0].TemperatureC != 72.5 {
		t.Errorf("temperature_c = %v, want 72.5", out.Devices[0].TemperatureC)
	}
	if out.Agent.Version != "0.1.0-dev" {
		t.Errorf("agent.version = %q, want 0.1.0-dev", out.Agent.Version)
	}
	if len(out.Agent.ActiveAdapters) == 0 {
		t.Error("expected active_adapters")
	}
}

func TestStdout_Update_EmptyReadings(t *testing.T) {
	var buf bytes.Buffer
	std := NewStdout(&buf, "0.1.0-dev", nil)

	if err := std.Update(nil, nil); err != nil {
		t.Errorf("Update(nil) = %v", err)
	}
	if err := std.Update([]normalizer.TelemetryPoint{}, nil); err != nil {
		t.Errorf("Update([]) = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
	for _, line := range lines {
		var out StdoutLine
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			t.Errorf("invalid JSON %q: %v", line, err)
		}
		if out.Timestamp == "" {
			t.Error("expected timestamp")
		}
	}
}

func TestStdout_SeverityString(t *testing.T) {
	// Test severityString via pointToDevice with risk_severity in metrics
	var buf bytes.Buffer
	std := NewStdout(&buf, "0.1.0-dev", nil)
	readings := []normalizer.TelemetryPoint{
		{Source: "gpu-0", Metrics: map[string]float64{"risk_severity": 2}},
		{Source: "gpu-1", Metrics: map[string]float64{"risk_severity": 1}},
		{Source: "gpu-2", Metrics: map[string]float64{"risk_severity": 0}},
	}
	if err := std.Update(readings, nil); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	var out StdoutLine
	if err := json.Unmarshal([]byte(lines[0]), &out); err != nil {
		t.Fatal(err)
	}
	// Last device has severity 0 -> normal
	if out.Devices[2].RiskSeverity != "normal" {
		t.Errorf("severity 0 = %q, want normal", out.Devices[2].RiskSeverity)
	}
	if out.Devices[1].RiskSeverity != "warning" {
		t.Errorf("severity 1 = %q, want warning", out.Devices[1].RiskSeverity)
	}
	if out.Devices[0].RiskSeverity != "critical" {
		t.Errorf("severity 2 = %q, want critical", out.Devices[0].RiskSeverity)
	}
}

func TestStdout_SetActiveAdapters(t *testing.T) {
	var buf bytes.Buffer
	std := NewStdout(&buf, "0.1.0-dev", nil)
	std.SetActiveAdapters([]string{"dcgm"})
	if err := std.Update([]normalizer.TelemetryPoint{}, nil); err != nil {
		t.Fatal(err)
	}
	var out StdoutLine
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Agent.ActiveAdapters) != 1 || out.Agent.ActiveAdapters[0] != "dcgm" {
		t.Errorf("active_adapters = %v", out.Agent.ActiveAdapters)
	}
}

func TestStdout_ZeroValuesPreserved(t *testing.T) {
	var buf bytes.Buffer
	std := NewStdout(&buf, "0.1.0-dev", []string{"fake"})

	readings := []normalizer.TelemetryPoint{
		{
			Source:      "host1",
			AdapterName: "fake",
			Metrics: map[string]float64{
				"temperature_c":       0,
				"power_usage_w":       0,
				"gpu_utilization_pct": 0,
				"risk_composite":      0,
			},
		},
	}
	if err := std.Update(readings, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var out StdoutLine
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	dev := out.Devices[0]
	if dev.TemperatureC == nil {
		t.Error("TemperatureC should not be nil for zero value")
	} else if *dev.TemperatureC != 0 {
		t.Errorf("TemperatureC = %v, want 0", *dev.TemperatureC)
	}
	if dev.PowerW == nil {
		t.Error("PowerW should not be nil for zero value")
	} else if *dev.PowerW != 0 {
		t.Errorf("PowerW = %v, want 0", *dev.PowerW)
	}
	if dev.Utilization == nil {
		t.Error("Utilization should not be nil for zero value")
	} else if *dev.Utilization != 0 {
		t.Errorf("Utilization = %v, want 0", *dev.Utilization)
	}
	if dev.RiskComposite == nil {
		t.Error("RiskComposite should not be nil for zero value")
	} else if *dev.RiskComposite != 0 {
		t.Errorf("RiskComposite = %v, want 0", *dev.RiskComposite)
	}
}

func TestStdout_DeviceModelFromTags(t *testing.T) {
	var buf bytes.Buffer
	std := NewStdout(&buf, "0.1.0-dev", []string{"dcgm"})

	readings := []normalizer.TelemetryPoint{
		{
			Source:      "host1",
			AdapterName: "dcgm",
			Metrics:     map[string]float64{"temperature_c": 65},
			Tags:        map[string]string{"gpu_name": "NVIDIA A100-SXM4-80GB"},
		},
		{
			Source:      "host2",
			AdapterName: "dcgm",
			Metrics:     map[string]float64{"temperature_c": 70},
			Tags:        map[string]string{"device_model": "MI300X"},
		},
		{
			Source:      "host3",
			AdapterName: "dcgm",
			Metrics:     map[string]float64{"temperature_c": 55},
		},
	}
	if err := std.Update(readings, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var out StdoutLine
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(out.Devices) != 3 {
		t.Fatalf("devices len = %d, want 3", len(out.Devices))
	}
	if out.Devices[0].DeviceModel != "NVIDIA A100-SXM4-80GB" {
		t.Errorf("device[0].DeviceModel = %q, want NVIDIA A100-SXM4-80GB", out.Devices[0].DeviceModel)
	}
	if out.Devices[1].DeviceModel != "MI300X" {
		t.Errorf("device[1].DeviceModel = %q, want MI300X", out.Devices[1].DeviceModel)
	}
	if out.Devices[2].DeviceModel != "unknown" {
		t.Errorf("device[2].DeviceModel = %q, want unknown", out.Devices[2].DeviceModel)
	}
}

func TestStdout_StartAndClose(t *testing.T) {
	var buf bytes.Buffer
	std := NewStdout(&buf, "0.1.0-dev", nil)
	if err := std.Start(context.Background()); err != nil {
		t.Errorf("Start = %v", err)
	}
	if err := std.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}
