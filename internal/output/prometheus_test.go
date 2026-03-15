// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package output

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func TestPrometheus_UpdateAndScrape(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry(9100, "0.1.0-dev", "test-device", reg, nil)

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
	if err := p.Update(readings); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Use httptest to serve the handler. Must use promhttp.HandlerFor with our registry.
	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", handler)
	mux.HandleFunc("GET /healthz", p.handleHealthz)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	body := make([]byte, 64*1024)
	n, _ := resp.Body.Read(body)
	metrics := string(body[:n])

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
	p := NewPrometheusWithRegistry(0, "0.1.0-dev", "test", reg, nil)
	// Create a test server with the same mux we'd use
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	mux.HandleFunc("GET /healthz", p.handleHealthz)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "healthy") {
		t.Errorf("healthz body should contain 'healthy', got %s", rec.Body.String())
	}
}

func TestPrometheus_UpdateWithEmptyReadings(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry(9100, "0.1.0-dev", "test", reg, nil)

	if err := p.Update(nil); err != nil {
		t.Errorf("Update(nil) = %v", err)
	}
	if err := p.Update([]normalizer.TelemetryPoint{}); err != nil {
		t.Errorf("Update([]) = %v", err)
	}
}

func TestPrometheus_Status(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewPrometheusWithRegistry(9100, "0.1.0-dev", "my-device", reg, nil)
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
