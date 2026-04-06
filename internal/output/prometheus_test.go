// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package output

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func TestPrometheus_UpdateAndScrape(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry("127.0.0.1", 9100, "0.1.0-dev", "test-device", reg, nil)

	// Update with mock readings
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
				"mem_used_bytes":      40 * 1024 * 1024 * 1024,
				"mem_total_bytes":     80 * 1024 * 1024 * 1024,
				"sm_clock_mhz":        1500,
				"throttled":           0,
				"gpu_id":              0,
			},
		},
	}
	if err := p.Update(readings, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Use the handler which now uses the injected gatherer.
	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	metrics := string(b)

	if !strings.Contains(metrics, "keldron_gpu_temperature_celsius") {
		t.Error("expected keldron_gpu_temperature_celsius in /metrics output")
	}
	if !strings.Contains(metrics, "keldron_gpu_power_watts") {
		t.Error("expected keldron_gpu_power_watts in /metrics output")
	}
	if !strings.Contains(metrics, "keldron_agent_info") {
		t.Error("expected keldron_agent_info in /metrics output")
	}
	if !strings.Contains(metrics, "device_model") {
		t.Error("expected device_model label in metrics")
	}
}

func TestPrometheus_Healthz(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry("127.0.0.1", 0, "0.1.0-dev", "test", reg, nil)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	p.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "healthy") {
		t.Errorf("healthz body should contain 'healthy', got %s", rec.Body.String())
	}
}

func TestPrometheus_UpdateWithEmptyReadings(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry("127.0.0.1", 9100, "0.1.0-dev", "test", reg, nil)

	if err := p.Update(nil, nil); err != nil {
		t.Errorf("Update(nil) = %v", err)
	}
	if err := p.Update([]normalizer.TelemetryPoint{}, nil); err != nil {
		t.Errorf("Update([]) = %v", err)
	}
}

func TestPrometheus_DeviceNameNotEmptyWhenUnset(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry("127.0.0.1", 9100, "0.1.0-dev", "", reg, nil)
	if err := p.Update(nil, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}

	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	metrics := string(b)
	if strings.Contains(metrics, `device_name=""`) {
		t.Errorf("keldron_agent_info must not use empty device_name; sample:\n%s", metrics)
	}
	want, err := os.Hostname()
	want = strings.TrimSpace(want)
	if err != nil || want == "" {
		if !strings.Contains(metrics, `device_name="unknown"`) {
			t.Errorf("expected device_name=unknown when hostname unavailable; got:\n%s", metrics)
		}
	} else if !strings.Contains(metrics, `device_name="`+want+`"`) {
		t.Errorf("expected device_name label %q in keldron_agent_info", want)
	}

	// Also verify the /api/v1/status endpoint uses the same fallback.
	resp2, err := http.Get(srv.URL + "/api/v1/status")
	if err != nil {
		t.Fatalf("GET /api/v1/status: %v", err)
	}
	defer resp2.Body.Close()
	var status map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	dn, _ := status["device_name"].(string)
	if dn == "" {
		t.Errorf("status device_name must not be empty")
	}
	if err != nil || want == "" {
		if dn != "unknown" {
			t.Errorf("status device_name = %q, want \"unknown\" when hostname unavailable", dn)
		}
	} else if dn != want {
		t.Errorf("status device_name = %q, want %q", dn, want)
	}
}

func TestPrometheus_Status(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry("127.0.0.1", 9100, "0.1.0-dev", "my-device", reg, nil)
	p.SetActiveAdapters([]string{"dcgm", "fake"})

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	p.handleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var m map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["version"] != "0.1.0-dev" {
		t.Errorf("version = %v", m["version"])
	}
	if m["device_name"] != "my-device" {
		t.Errorf("device_name = %v", m["device_name"])
	}
}

func TestPrometheus_AppleSiliconReading(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry("127.0.0.1", 9100, "0.1.0-dev", "test-device", reg, nil)

	// Simulate exactly what the Apple Silicon adapter produces after normalization.
	readings := []normalizer.TelemetryPoint{
		{
			ID:          "01HAPPLE",
			AgentID:     "agent-1",
			AdapterName: "apple_silicon",
			Source:      "macbook-pro",
			RackID:      "unknown",
			Timestamp:   time.Now(),
			ReceivedAt:  time.Now(),
			Metrics: map[string]float64{
				"temperature_c":       0.0, // IOKit stub returns zeros
				"power_usage_w":       0.0,
				"gpu_utilization_pct": 0.0,
				"mem_total_bytes":     38654705664,
				"mem_used_bytes":      12884901888,
				"swap_total_bytes":    0,
				"swap_used_bytes":     0,
				"throttled":           0,
				"gpu_id":              0,
			},
			Tags: map[string]string{
				"gpu_model":              "M4-Pro",
				"thermal_pressure_state": "nominal",
				"throttle_reason":        "none",
			},
		},
	}
	if err := p.Update(readings, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}

	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	body := string(b)

	// These metrics should appear even when values are zero.
	wantMetrics := []string{
		"keldron_gpu_temperature_celsius",
		"keldron_gpu_power_watts",
		"keldron_gpu_utilization_ratio",
		"keldron_gpu_memory_used_bytes",
		"keldron_gpu_memory_total_bytes",
		"keldron_system_swap_total_bytes",
		"keldron_system_swap_used_bytes",
		"keldron_risk_composite",
		"keldron_gpu_throttle_active",
	}
	for _, m := range wantMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("missing metric %s in output:\n%s", m, body)
		}
	}

	// Check that the device_model label is M4-Pro.
	if !strings.Contains(body, `device_model="M4-Pro"`) {
		t.Errorf("missing device_model=M4-Pro label in output:\n%s", body)
	}
	if !strings.Contains(body, `adapter="apple_silicon"`) {
		t.Errorf("missing adapter=apple_silicon label in output:\n%s", body)
	}
}

func TestStringsToLabels(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"device_model,device_vendor", []string{"device_model", "device_vendor"}},
		{"a", []string{"a"}},
		{"", []string{}},
	}
	for _, tt := range tests {
		got := stringsToLabels(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("stringsToLabels(%q) = %v, want %v", tt.in, got, tt.want)
		} else {
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("stringsToLabels(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		}
	}
}
