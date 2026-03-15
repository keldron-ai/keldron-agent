package dcgm

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/registry"

	"gopkg.in/yaml.v3"
)

func stubConfig(t *testing.T) config.AdapterConfig {
	t.Helper()
	raw := `
enabled: true
poll_interval: "50ms"
use_stub: true
gpu_ids: [0, 1]
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("unmarshal raw node: %v", err)
	}
	// yaml.Unmarshal wraps in a document node; we want the mapping node inside.
	return config.AdapterConfig{
		Enabled:      true,
		PollInterval: 50 * time.Millisecond,
		Raw:          *node.Content[0],
	}
}

func newTestAdapter(t *testing.T) *DCGMAdapter {
	t.Helper()
	cfg := stubConfig(t)
	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return a.(*DCGMAdapter)
}

func TestNew_StubClient(t *testing.T) {
	a := newTestAdapter(t)
	if a.Name() != "dcgm" {
		t.Errorf("Name() = %q, want %q", a.Name(), "dcgm")
	}
}

func TestEmitsReadings(t *testing.T) {
	a := newTestAdapter(t)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if err := a.Start(ctx); err != nil {
			t.Errorf("Start() error: %v", err)
		}
	}()

	// Collect a few readings (should get 2 per poll for 2 GPUs).
	var readings []adapter.RawReading
	timeout := time.After(2 * time.Second)

	for len(readings) < 4 {
		select {
		case r := <-a.Readings():
			readings = append(readings, r)
		case <-timeout:
			t.Fatalf("timed out waiting for readings, got %d", len(readings))
		}
	}

	cancel()
	_ = a.Stop(context.Background())

	if len(readings) < 4 {
		t.Fatalf("got %d readings, want at least 4", len(readings))
	}
}

func TestAllMetricsPresent(t *testing.T) {
	a := newTestAdapter(t)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if err := a.Start(ctx); err != nil {
			t.Errorf("Start() error: %v", err)
		}
	}()

	var reading adapter.RawReading
	select {
	case reading = <-a.Readings():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reading")
	}

	cancel()
	_ = a.Stop(context.Background())

	expectedKeys := []string{
		MetricGPUID,
		MetricGPUName,
		MetricTemperature,
		MetricPowerUsage,
		MetricGPUUtilization,
		MetricMemUtilization,
		MetricMemUsed,
		MetricMemTotal,
		MetricSMClock,
		MetricMemClock,
		MetricThrottled,
		registry.MetricThermalStress,
		registry.MetricPowerStress,
	}

	for _, key := range expectedKeys {
		if _, ok := reading.Metrics[key]; !ok {
			t.Errorf("missing metric key %q", key)
		}
	}

	if len(reading.Metrics) != len(expectedKeys) {
		t.Errorf("got %d metric keys, want %d", len(reading.Metrics), len(expectedKeys))
	}
}

func TestStopsCleanly(t *testing.T) {
	a := newTestAdapter(t)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = a.Start(ctx)
		close(done)
	}()

	// Let it run briefly.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}

	if err := a.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestChannelClosedAfterStop(t *testing.T) {
	a := newTestAdapter(t)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = a.Start(ctx)
	}()

	// Drain one reading to ensure adapter is running.
	select {
	case <-a.Readings():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first reading")
	}

	cancel()
	_ = a.Stop(context.Background())

	// Channel should be closed; reads should eventually return zero value.
	timer := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-a.Readings():
			if !ok {
				return // channel closed, test passes
			}
			// Still draining buffered readings, continue.
		case <-timer:
			t.Fatal("channel not closed after Stop")
		}
	}
}

func TestSlowConsumerDropsReadings(t *testing.T) {
	cfg := stubConfig(t)
	// Override poll interval to be very fast.
	cfg.PollInterval = 10 * time.Millisecond

	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	da := a.(*DCGMAdapter)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = da.Start(ctx)
	}()

	// Don't read from the channel — let it fill up.
	// With 10ms poll, 2 GPUs, and 256 buffer, it should fill in ~1.3s.
	time.Sleep(2 * time.Second)

	cancel()
	_ = da.Stop(context.Background())

	// Verify we got at most channelBuffer+2 readings (some were dropped).
	// The +2 accounts for a race: `for range` drains items before Start() closes
	// the channel, freeing space for one final poll (2 GPUs) to send before close.
	count := 0
	for range da.readings {
		count++
	}
	if count > channelBuffer+2 {
		t.Errorf("got %d buffered readings, max should be %d", count, channelBuffer+2)
	}
}

func TestPlausibleMetricRanges(t *testing.T) {
	a := newTestAdapter(t)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = a.Start(ctx)
	}()

	var reading adapter.RawReading
	select {
	case reading = <-a.Readings():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reading")
	}

	cancel()
	_ = a.Stop(context.Background())

	// Temperature: 40–85 C
	temp := reading.Metrics[MetricTemperature].(float64)
	if temp < 40 || temp > 85 {
		t.Errorf("temperature = %f, want 40–85", temp)
	}

	// Power: 200–350 W
	power := reading.Metrics[MetricPowerUsage].(float64)
	if power < 200 || power > 350 {
		t.Errorf("power = %f, want 200–350", power)
	}

	// GPU Utilization: 70–99%
	gpuUtil := reading.Metrics[MetricGPUUtilization].(float64)
	if gpuUtil < 70 || gpuUtil > 99 {
		t.Errorf("gpu_utilization = %f, want 70–99", gpuUtil)
	}

	// Memory Utilization: 50–90%
	memUtil := reading.Metrics[MetricMemUtilization].(float64)
	if memUtil < 50 || memUtil > 90 {
		t.Errorf("mem_utilization = %f, want 50–90", memUtil)
	}

	// Memory Total: 80 GB
	memTotal := reading.Metrics[MetricMemTotal].(uint64)
	expected := uint64(80 * 1024 * 1024 * 1024)
	if memTotal != expected {
		t.Errorf("mem_total = %d, want %d", memTotal, expected)
	}

	// SM Clock: 1200–1410 MHz
	smClock := reading.Metrics[MetricSMClock].(uint32)
	if smClock < 1200 || smClock > 1410 {
		t.Errorf("sm_clock = %d, want 1200–1410", smClock)
	}

	// Adapter name
	if reading.AdapterName != "dcgm" {
		t.Errorf("adapter_name = %q, want %q", reading.AdapterName, "dcgm")
	}
}
