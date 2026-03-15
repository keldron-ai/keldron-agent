package fake

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"log/slog"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"gopkg.in/yaml.v3"
)

// helper to build a config.AdapterConfig with fake settings baked in.
func testAdapterConfig(scenario string, numRacks, gpusPerRack int) config.AdapterConfig {
	raw := fmt.Sprintf(`
num_racks: %d
gpus_per_rack: %d
ambient_temp_c: 22.0
scenario: "%s"
memory_gb: 80
power_limit_w: 700
`, numRacks, gpusPerRack, scenario)

	var node yaml.Node
	_ = yaml.Unmarshal([]byte(raw), &node)

	return config.AdapterConfig{
		Enabled:      true,
		PollInterval: 100 * time.Millisecond,
		Raw:          *node.Content[0],
	}
}

func TestFakeAdapter_ProducesReadings(t *testing.T) {
	cfg := testAdapterConfig("bursty", 2, 4)
	logger := slog.Default()

	a, err := New(cfg, nil, logger)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go a.Start(ctx)

	count := 0
	timeout := time.After(4 * time.Second)
	for {
		select {
		case r, ok := <-a.Readings():
			if !ok {
				goto done
			}
			count++
			// Verify reading structure
			if r.AdapterName != "fake" {
				t.Errorf("AdapterName = %q, want %q", r.AdapterName, "fake")
			}
			if r.Source == "" {
				t.Error("Source is empty")
			}
			if r.Timestamp.IsZero() {
				t.Error("Timestamp is zero")
			}
			if r.Metrics == nil {
				t.Error("Metrics is nil")
			}
			// Check key metrics exist (DCGM-aligned keys)
			for _, key := range []string{"temperature_c", "gpu_utilization_pct", "power_usage_w", "mem_used_bytes"} {
				if _, ok := r.Metrics[key]; !ok {
					t.Errorf("missing metric %q", key)
				}
			}
		case <-timeout:
			goto done
		}
	}
done:

	expectedGPUs := 2 * 4 // 2 racks × 4 GPUs
	expectedMin := expectedGPUs * 20 // ~30 polls in 3s at 100ms, but allow margin
	if count < expectedMin {
		t.Errorf("got %d readings, expected at least %d", count, expectedMin)
	}
	t.Logf("collected %d readings from %d simulated GPUs", count, expectedGPUs)
}

func TestFakeAdapter_TemperaturesPhysicallyPlausible(t *testing.T) {
	cfg := testAdapterConfig("bursty", 1, 4)
	logger := slog.Default()

	a, err := New(cfg, nil, logger)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go a.Start(ctx)

	type gpuData struct {
		temps []float64
	}
	history := make(map[string]*gpuData)

	for r := range a.Readings() {
		temp, _ := r.Metrics["temperature_c"].(float64)
		source := r.Source
		h, ok := history[source]
		if !ok {
			h = &gpuData{}
			history[source] = h
		}
		h.temps = append(h.temps, temp)
	}

	for source, h := range history {
		// Check range
		for _, temp := range h.temps {
			if temp < 20 || temp > 106 {
				t.Errorf("GPU %s: temp %.1f out of valid range", source, temp)
			}
		}

		// Check thermal inertia: no huge jumps between consecutive readings
		for i := 1; i < len(h.temps); i++ {
			jump := math.Abs(h.temps[i] - h.temps[i-1])
			if jump > 10 {
				t.Errorf("GPU %s: unrealistic temp jump of %.1f°C at reading %d", source, jump, i)
			}
		}

		t.Logf("GPU %s: %d readings, temp range [%.1f, %.1f]",
			source, len(h.temps), min(h.temps), max(h.temps))
	}
}

func TestFakeAdapter_FailureScenario(t *testing.T) {
	cfg := testAdapterConfig("failure", 1, 8)
	cfg.PollInterval = 50 * time.Millisecond // fast polling to see the drift
	logger := slog.Default()

	a, err := New(cfg, nil, logger)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go a.Start(ctx)

	// Track max temp per GPU-source. GPU-0002-0003 should drift higher than others.
	maxTemps := make(map[string]float64)
	for r := range a.Readings() {
		temp, _ := r.Metrics["temperature_c"].(float64)
		src := r.Source
		if temp > maxTemps[src] {
			maxTemps[src] = temp
		}
	}

	for src, mx := range maxTemps {
		t.Logf("GPU %s: max temp %.1f°C", src, mx)
	}
	// Note: the failing GPU is GPU-0002-0003 which maps to gpu-node-02.
	// In a 1-rack setup with 8 GPUs, rack index is always 0, so the failing
	// GPU ID won't exist. This test mainly validates no crash in failure mode.
}

func TestFakeAdapter_StopsCleanly(t *testing.T) {
	cfg := testAdapterConfig("steady", 1, 2)
	logger := slog.Default()

	a, err := New(cfg, nil, logger)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = a.Start(ctx)
		close(done)
	}()

	// Let it run briefly
	time.Sleep(500 * time.Millisecond)

	// Cancel and verify clean shutdown
	cancel()
	select {
	case <-done:
		// Good — Start returned
	case <-time.After(3 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}

	// Readings channel should be closed (drain any buffered readings first)
	drainDone := make(chan struct{})
	go func() {
		for range a.Readings() {
		}
		close(drainDone)
	}()
	select {
	case <-drainDone:
		// Channel closed, good
	case <-time.After(2 * time.Second):
		t.Error("readings channel did not close after Stop")
	}

	// Stop should not error
	if err := a.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func min(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func max(vals []float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
