// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package normalizer tests cover:
// - Validation rules (required fields, timestamp skew, boundary conditions)
// - Metric type coercion (float64, float32, int, int64, uint64, uint32, bool, string)
// - Rack ID resolution (known mapping, unknown source)
// - End-to-end transform (RawReading → TelemetryPoint with all fields)
// - Fan-in from multiple adapter channels
// - Stats counters (processed, rejected)
// - Clean shutdown (output channel closed after inputs drain)
// - Concurrent safety (race detection via go test -race)
package normalizer

import (
	"context"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
)

// --- Helpers ---

func validReading() adapter.RawReading {
	return adapter.RawReading{
		AdapterName: "dcgm",
		Source:      "gpu-node-01",
		Timestamp:   time.Now(),
		Metrics:     map[string]interface{}{"gpu_temp": 65.0},
	}
}

// --- TestValidate: table-driven ---

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		reading adapter.RawReading
		maxSkew time.Duration
		want    bool
		errMsg  string
	}{
		{
			name:    "valid reading",
			reading: validReading(),
			maxSkew: 30 * time.Second,
			want:    true,
		},
		{
			name: "empty source rejected",
			reading: adapter.RawReading{
				Source: "", AdapterName: "dcgm", Timestamp: time.Now(),
				Metrics: map[string]interface{}{"x": 1.0},
			},
			maxSkew: 30 * time.Second,
			want:    false,
			errMsg:  "source must not be empty",
		},
		{
			name: "empty adapter name rejected",
			reading: adapter.RawReading{
				Source: "node-01", AdapterName: "", Timestamp: time.Now(),
				Metrics: map[string]interface{}{"x": 1.0},
			},
			maxSkew: 30 * time.Second,
			want:    false,
			errMsg:  "adapter name must not be empty",
		},
		{
			name: "nil metrics rejected",
			reading: adapter.RawReading{
				Source: "node-01", AdapterName: "dcgm", Timestamp: time.Now(),
				Metrics: nil,
			},
			maxSkew: 30 * time.Second,
			want:    false,
			errMsg:  "metrics must not be empty",
		},
		{
			name: "empty metrics map rejected",
			reading: adapter.RawReading{
				Source: "node-01", AdapterName: "dcgm", Timestamp: time.Now(),
				Metrics: map[string]interface{}{},
			},
			maxSkew: 30 * time.Second,
			want:    false,
			errMsg:  "metrics must not be empty",
		},
		{
			name: "zero timestamp rejected",
			reading: adapter.RawReading{
				Source: "node-01", AdapterName: "dcgm", Timestamp: time.Time{},
				Metrics: map[string]interface{}{"x": 1.0},
			},
			maxSkew: 30 * time.Second,
			want:    false,
			errMsg:  "timestamp must not be zero",
		},
		{
			name: "future timestamp beyond skew rejected",
			reading: adapter.RawReading{
				Source: "node-01", AdapterName: "dcgm",
				Timestamp: time.Now().Add(5 * time.Minute),
				Metrics:   map[string]interface{}{"x": 1.0},
			},
			maxSkew: 30 * time.Second,
			want:    false,
			errMsg:  "timestamp skew",
		},
		{
			name: "past timestamp beyond skew rejected",
			reading: adapter.RawReading{
				Source: "node-01", AdapterName: "dcgm",
				Timestamp: time.Now().Add(-5 * time.Minute),
				Metrics:   map[string]interface{}{"x": 1.0},
			},
			maxSkew: 30 * time.Second,
			want:    false,
			errMsg:  "timestamp skew",
		},
		{
			name: "exactly at skew boundary accepted",
			reading: adapter.RawReading{
				Source: "node-01", AdapterName: "dcgm",
				// Use 29.9s instead of exactly 30s to avoid flakiness from
				// time passing between Now() and Validate's Since() call.
				Timestamp: time.Now().Add(-29900 * time.Millisecond),
				Metrics:   map[string]interface{}{"x": 1.0},
			},
			maxSkew: 30 * time.Second,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Validate(tt.reading, tt.maxSkew)
			if tt.want && !result.Valid {
				t.Errorf("expected valid but got rejection: %s", result.Reason)
			}
			if !tt.want && result.Valid {
				t.Errorf("expected rejection but got valid")
			}
			if !tt.want && tt.errMsg != "" && !strings.Contains(result.Reason, tt.errMsg) {
				t.Errorf("expected error containing %q, got %q", tt.errMsg, result.Reason)
			}
		})
	}
}

// --- TestCoerceToFloat64: table-driven ---

func TestCoerceToFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input interface{}
		want  float64
		ok    bool
	}{
		{"float64", float64(42.5), 42.5, true},
		{"float32", float32(42.5), 42.5, true},
		{"int", int(42), 42.0, true},
		{"int64", int64(42), 42.0, true},
		{"uint64", uint64(42), 42.0, true},
		{"uint32", uint32(42), 42.0, true},
		{"bool true", true, 1.0, true},
		{"bool false", false, 0.0, true},
		{"string unrecognized", "not-a-number", 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := CoerceToFloat64(tt.input)
			if ok != tt.ok {
				t.Errorf("CoerceToFloat64(%v) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("CoerceToFloat64(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- TestResolveRackID ---

func TestResolveRackID(t *testing.T) {
	t.Parallel()

	mapping := map[string]string{
		"gpu-node-01": "rack-A1",
		"gpu-node-02": "rack-A2",
	}

	t.Run("known source", func(t *testing.T) {
		t.Parallel()
		rackID, ok := ResolveRackID("gpu-node-01", mapping)
		if !ok {
			t.Fatal("expected ok=true for known source")
		}
		if rackID != "rack-A1" {
			t.Errorf("rackID = %q, want %q", rackID, "rack-A1")
		}
	})

	t.Run("unknown source", func(t *testing.T) {
		t.Parallel()
		_, ok := ResolveRackID("unknown-host", mapping)
		if ok {
			t.Fatal("expected ok=false for unknown source")
		}
	})

	t.Run("empty mapping", func(t *testing.T) {
		t.Parallel()
		_, ok := ResolveRackID("gpu-node-01", map[string]string{})
		if ok {
			t.Fatal("expected ok=false for empty mapping")
		}
	})
}

// --- TestNormalizerTransform ---

func TestNormalizerTransform(t *testing.T) {
	t.Parallel()

	rackMapping := map[string]string{"gpu-node-01": "rack-A1"}
	norm := New("agent-01", rackMapping, nil, slog.Default())

	ch := make(chan adapter.RawReading, 1)
	norm.AddInput(ch)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = norm.Start(ctx)
	}()

	reading := adapter.RawReading{
		AdapterName: "dcgm",
		Source:      "gpu-node-01",
		Timestamp:   time.Now(),
		Metrics: map[string]interface{}{
			"temperature_c":   65.0,
			"power_usage_w":   250.0,
			"gpu_utilization": float64(85),
			"mem_used_bytes":  uint64(40e9),
			"throttled":       false,
			"gpu_name":        "NVIDIA A100", // string — should be skipped
		},
	}

	ch <- reading
	close(ch) // signal no more readings

	var point TelemetryPoint
	select {
	case p, ok := <-norm.Output():
		if !ok {
			t.Fatal("output channel closed before receiving point")
		}
		point = p
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for TelemetryPoint")
	}

	cancel()

	// Verify all fields.
	if point.ID == "" {
		t.Error("ID should be non-empty ULID")
	}
	if len(point.ID) != 26 {
		t.Errorf("ID length = %d, want 26 (ULID)", len(point.ID))
	}
	if point.AgentID != "agent-01" {
		t.Errorf("AgentID = %q, want %q", point.AgentID, "agent-01")
	}
	if point.AdapterName != "dcgm" {
		t.Errorf("AdapterName = %q, want %q", point.AdapterName, "dcgm")
	}
	if point.Source != "gpu-node-01" {
		t.Errorf("Source = %q, want %q", point.Source, "gpu-node-01")
	}
	if point.RackID != "rack-A1" {
		t.Errorf("RackID = %q, want %q", point.RackID, "rack-A1")
	}
	if point.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	if point.ReceivedAt.IsZero() {
		t.Error("ReceivedAt should not be zero")
	}

	// Numeric metrics should be present; string "gpu_name" should be skipped.
	if _, ok := point.Metrics["temperature_c"]; !ok {
		t.Error("missing metric temperature_c")
	}
	if v := point.Metrics["temperature_c"]; v != 65.0 {
		t.Errorf("temperature_c = %v, want 65.0", v)
	}
	if v := point.Metrics["mem_used_bytes"]; v != 40e9 {
		t.Errorf("mem_used_bytes = %v, want 4e10", v)
	}
	if v := point.Metrics["throttled"]; v != 0.0 {
		t.Errorf("throttled = %v, want 0.0 (false)", v)
	}
	if _, ok := point.Metrics["gpu_name"]; ok {
		t.Error("string metric gpu_name should have been skipped")
	}

	// Expect 5 numeric metrics (temperature_c, power_usage_w, gpu_utilization, mem_used_bytes, throttled).
	if len(point.Metrics) != 5 {
		t.Errorf("got %d metrics, want 5 (string skipped)", len(point.Metrics))
	}
}

// --- TestUnknownRackID ---

func TestUnknownRackID(t *testing.T) {
	t.Parallel()

	norm := New("agent-01", map[string]string{}, nil, slog.Default())

	ch := make(chan adapter.RawReading, 1)
	norm.AddInput(ch)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = norm.Start(ctx)
	}()

	ch <- validReading() // source "gpu-node-01" not in empty mapping
	close(ch)

	select {
	case point := <-norm.Output():
		if point.RackID != "unknown" {
			t.Errorf("RackID = %q, want %q", point.RackID, "unknown")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for point")
	}

	cancel()
}

// --- TestFanInMultipleInputs ---

func TestFanInMultipleInputs(t *testing.T) {
	t.Parallel()

	norm := New("agent-01", map[string]string{
		"gpu-node-01": "rack-A1",
		"pdu-01":      "rack-B1",
	}, nil, slog.Default())

	ch1 := make(chan adapter.RawReading, 1)
	ch2 := make(chan adapter.RawReading, 1)
	norm.AddInput(ch1)
	norm.AddInput(ch2)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = norm.Start(ctx)
	}()

	// Send one reading per channel.
	ch1 <- adapter.RawReading{
		AdapterName: "dcgm", Source: "gpu-node-01",
		Timestamp: time.Now(), Metrics: map[string]interface{}{"temp": 65.0},
	}
	ch2 <- adapter.RawReading{
		AdapterName: "pdu", Source: "pdu-01",
		Timestamp: time.Now(), Metrics: map[string]interface{}{"power": 1200.0},
	}
	close(ch1)
	close(ch2)

	seen := map[string]bool{}
	timeout := time.After(2 * time.Second)

	for len(seen) < 2 {
		select {
		case p, ok := <-norm.Output():
			if !ok {
				if len(seen) < 2 {
					t.Fatalf("output closed early, got %d points", len(seen))
				}
				break
			}
			seen[p.AdapterName] = true
		case <-timeout:
			t.Fatalf("timed out, got %d of 2 points", len(seen))
		}
	}

	cancel()

	if !seen["dcgm"] {
		t.Error("missing point from dcgm adapter")
	}
	if !seen["pdu"] {
		t.Error("missing point from pdu adapter")
	}
}

// --- TestStatsCounters ---

func TestStatsCounters(t *testing.T) {
	t.Parallel()

	norm := New("agent-01", map[string]string{}, nil, slog.Default())

	ch := make(chan adapter.RawReading, 4)
	norm.AddInput(ch)

	// 2 valid, 2 invalid readings.
	ch <- validReading()
	ch <- validReading()
	ch <- adapter.RawReading{Source: "", AdapterName: "dcgm", Timestamp: time.Now(), Metrics: map[string]interface{}{"x": 1.0}}
	ch <- adapter.RawReading{Source: "n", AdapterName: "", Timestamp: time.Now(), Metrics: map[string]interface{}{"x": 1.0}}
	close(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = norm.Start(ctx)
	}()

	// Drain valid points.
	for i := 0; i < 2; i++ {
		select {
		case <-norm.Output():
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for point")
		}
	}

	// Wait for output to close (all inputs drained).
	select {
	case _, ok := <-norm.Output():
		if ok {
			t.Error("expected output channel to close, got extra point")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for output channel close")
	}

	processed, rejected := norm.Stats()
	if processed != 2 {
		t.Errorf("processed = %d, want 2", processed)
	}
	if rejected != 2 {
		t.Errorf("rejected = %d, want 2", rejected)
	}
}

// --- TestCleanShutdown ---

func TestCleanShutdown(t *testing.T) {
	t.Parallel()

	norm := New("agent-01", map[string]string{}, nil, slog.Default())

	ch := make(chan adapter.RawReading, 1)
	norm.AddInput(ch)

	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = norm.Start(ctx)
		close(done)
	}()

	// Send one reading and close input.
	ch <- validReading()
	close(ch)

	// Drain the output.
	select {
	case <-norm.Output():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for point")
	}

	// Output channel should be closed after input drains.
	select {
	case _, ok := <-norm.Output():
		if ok {
			t.Error("expected channel closed, got point")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("output channel not closed after inputs drained")
	}

	// Start goroutine should have returned.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after all inputs closed")
	}

	cancel()
}

// --- TestConcurrentProducers (race detection) ---

func TestConcurrentProducers(t *testing.T) {
	t.Parallel()

	norm := New("agent-01", map[string]string{"node": "rack-1"}, nil, slog.Default())

	const numChannels = 4
	const readingsPerChannel = 50

	channels := make([]chan adapter.RawReading, numChannels)
	for i := range channels {
		channels[i] = make(chan adapter.RawReading, readingsPerChannel)
		norm.AddInput(channels[i])
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = norm.Start(ctx)
	}()

	// Concurrently produce readings on all channels.
	for i := range channels {
		go func(ch chan adapter.RawReading) {
			for j := 0; j < readingsPerChannel; j++ {
				ch <- adapter.RawReading{
					AdapterName: "test",
					Source:      "node",
					Timestamp:   time.Now(),
					Metrics:     map[string]interface{}{"val": float64(j)},
				}
			}
			close(ch)
		}(channels[i])
	}

	// Drain all output.
	count := 0
	for range norm.Output() {
		count++
	}

	expected := numChannels * readingsPerChannel
	if count != expected {
		t.Errorf("got %d points, want %d", count, expected)
	}

	processed, rejected := norm.Stats()
	if processed != uint64(expected) {
		t.Errorf("processed = %d, want %d", processed, expected)
	}
	if rejected != 0 {
		t.Errorf("rejected = %d, want 0", rejected)
	}
}

// --- TestRejectedReadingNotOnOutput ---

func TestRejectedReadingNotOnOutput(t *testing.T) {
	t.Parallel()

	norm := New("agent-01", map[string]string{}, nil, slog.Default())

	ch := make(chan adapter.RawReading, 2)
	norm.AddInput(ch)

	// Send invalid then valid.
	ch <- adapter.RawReading{Source: "", Metrics: nil}
	ch <- validReading()
	close(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = norm.Start(ctx)
	}()

	// Should only get the valid reading.
	var points []TelemetryPoint
	for p := range norm.Output() {
		points = append(points, p)
	}

	if len(points) != 1 {
		t.Errorf("got %d points, want 1 (only valid reading)", len(points))
	}
}
