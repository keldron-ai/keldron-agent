package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/output"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

// TestOutputBridge_FlushesToPrometheus verifies that TelemetryPoints
// sent through the output bridge end up in the Prometheus scrape output.
func TestOutputBridge_FlushesToPrometheus(t *testing.T) {
	reg := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	prom := output.NewPrometheusWithRegistry(0, "test", "test-device", reg, logger)
	outputs := []output.Output{prom}

	// Simulate normalizer output channel.
	ch := make(chan normalizer.TelemetryPoint, 16)

	// Send an Apple-Silicon-like TelemetryPoint.
	ch <- normalizer.TelemetryPoint{
		ID:          "01TEST",
		AgentID:     "agent-test",
		AdapterName: "apple_silicon",
		Source:      "macbook-pro",
		RackID:      "unknown",
		Timestamp:   time.Now(),
		ReceivedAt:  time.Now(),
		Metrics: map[string]float64{
			"temperature_c":       42.0,
			"power_usage_w":       8.5,
			"gpu_utilization_pct": 12.0,
			"mem_total_bytes":     36507222016,
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
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	// Use a very short flush interval for the test.
	scoreEngine := scoring.NewScoreEngine(0.12)
	go runOutputBridge(ctx, ch, outputs, scoreEngine, 50*time.Millisecond, done, logger)

	// Wait for at least one flush.
	time.Sleep(200 * time.Millisecond)

	// Scrape.
	srv := httptest.NewServer(prom.Handler())
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
	body := string(b)

	// Print for debugging.
	t.Logf("Prometheus output (keldron_ lines only):")
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "keldron_") {
			t.Logf("  %s", line)
		}
	}

	wantMetrics := []string{
		"keldron_gpu_temperature_celsius",
		"keldron_gpu_power_watts",
		"keldron_gpu_utilization_ratio",
		"keldron_gpu_memory_used_bytes",
		"keldron_gpu_memory_total_bytes",
		"keldron_system_swap_total_bytes",
		"keldron_system_swap_used_bytes",
		"keldron_risk_composite",
	}
	for _, m := range wantMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("missing metric %s", m)
		}
	}
	if !strings.Contains(body, `device_model="M4-Pro"`) {
		t.Errorf("missing device_model=M4-Pro label")
	}
	if !strings.Contains(body, `adapter="apple_silicon"`) {
		t.Errorf("missing adapter=apple_silicon label")
	}

	// Shut down: close channel first so the drain loop in runOutputBridge can finish.
	close(ch)
	cancel()
	<-done
}

// TestFullPipeline_AdapterToPrometheus tests adapter.RawReading → normalizer → bridge → Prometheus.
func TestFullPipeline_AdapterToPrometheus(t *testing.T) {
	reg := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	prom := output.NewPrometheusWithRegistry(0, "test", "test-device", reg, logger)
	outputs := []output.Output{prom}

	// Simulate adapter channel.
	adapterCh := make(chan adapter.RawReading, 16)

	// Create normalizer with the adapter channel as input.
	norm := normalizer.New("agent-test", map[string]string{"macbook-pro": "rack-1"}, nil, logger)
	norm.AddInput(adapterCh)

	ctx, cancel := context.WithCancel(context.Background())

	// Start normalizer.
	go func() { _ = norm.Start(ctx) }()

	// Start output bridge with short interval.
	scoreEngine := scoring.NewScoreEngine(0.12)
	done := make(chan struct{})
	go runOutputBridge(ctx, norm.Output(), outputs, scoreEngine, 50*time.Millisecond, done, logger)

	// Send a raw reading (as the Apple Silicon adapter would).
	adapterCh <- adapter.RawReading{
		AdapterName: "apple_silicon",
		Source:      "macbook-pro",
		Timestamp:   time.Now(),
		Metrics: map[string]interface{}{
			"gpu_model":              "M4-Pro",
			"gpu_id":                 0.0,
			"temperature_c":          0.0,
			"power_usage_w":          0.0,
			"gpu_utilization_pct":    0.0,
			"mem_total_bytes":        float64(36507222016),
			"mem_used_bytes":         float64(12884901888),
			"swap_total_bytes":       float64(0),
			"swap_used_bytes":        float64(0),
			"throttled":              0.0,
			"thermal_pressure_state": "nominal",
			"throttle_reason":        "none",
		},
	}

	// Wait for normalizer + bridge flush.
	time.Sleep(300 * time.Millisecond)

	// Scrape.
	srv := httptest.NewServer(prom.Handler())
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
	body := string(b)

	t.Logf("Full pipeline keldron_ metrics:")
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "keldron_") {
			t.Logf("  %s", line)
		}
	}

	// GPU metrics must appear even with zero values.
	wantMetrics := []string{
		"keldron_gpu_temperature_celsius",
		"keldron_gpu_power_watts",
		"keldron_gpu_utilization_ratio",
		"keldron_gpu_memory_total_bytes",
		"keldron_risk_composite",
	}
	for _, m := range wantMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("missing metric %s", m)
		}
	}
	if !strings.Contains(body, `device_model="M4-Pro"`) {
		t.Errorf("missing device_model=M4-Pro label")
	}

	close(adapterCh)
	cancel()
	<-done
}
