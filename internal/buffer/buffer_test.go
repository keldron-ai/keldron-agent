// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package buffer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func testBufferConfig(t *testing.T) config.BufferConfig {
	t.Helper()
	return config.BufferConfig{
		RingSize:   100,
		WALDir:     t.TempDir(),
		WALMaxSize: "10MB",
	}
}

func bufTestPoint(id string) normalizer.TelemetryPoint {
	return normalizer.TelemetryPoint{
		ID:          id,
		AgentID:     "agent-test",
		AdapterName: "test",
		Source:      "src",
		RackID:      "rack",
		Timestamp:   time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC),
		ReceivedAt:  time.Date(2025, 7, 1, 12, 0, 1, 0, time.UTC),
		Metrics:     map[string]float64{"v": 1.0},
	}
}

func collectOutput(ch <-chan normalizer.TelemetryPoint, count int, timeout time.Duration) []normalizer.TelemetryPoint {
	var points []normalizer.TelemetryPoint
	deadline := time.After(timeout)
	for len(points) < count {
		select {
		case p, ok := <-ch:
			if !ok {
				return points
			}
			points = append(points, p)
		case <-deadline:
			return points
		}
	}
	return points
}

func TestManager_NormalFlowThrough(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	input := make(chan normalizer.TelemetryPoint, 100)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Manager starts connected. Send 5 points.
	for i := range 5 {
		input <- bufTestPoint(string(rune('A' + i)))
	}

	got := collectOutput(m.Output(), 5, 5*time.Second)
	if len(got) != 5 {
		t.Fatalf("got %d points, want 5", len(got))
	}

	// Verify FIFO order.
	for i, p := range got {
		want := string(rune('A' + i))
		if p.ID != want {
			t.Errorf("point %d: got %q, want %q", i, p.ID, want)
		}
	}

	cancel()
	<-done
}

func TestManager_DisconnectAndReconnect(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	cfg.RingSize = 5 // Small ring
	input := make(chan normalizer.TelemetryPoint, 100)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Disconnect the sender. Ring is flushed to WAL on disconnect.
	m.OnConnChange(false)
	time.Sleep(50 * time.Millisecond)

	// Send 10 points — all go directly to WAL (ring was flushed on disconnect).
	for i := range 10 {
		input <- bufTestPoint(string(rune('A' + i)))
	}
	time.Sleep(100 * time.Millisecond)

	// Reconnect.
	m.OnConnChange(true)

	// Should get all 10 points in strict FIFO order.
	got := collectOutput(m.Output(), 10, 5*time.Second)
	if len(got) != 10 {
		t.Fatalf("got %d points, want 10", len(got))
	}

	// Verify strict FIFO order, not just presence.
	for i, p := range got {
		want := string(rune('A' + i))
		if p.ID != want {
			t.Errorf("point %d: got %q, want %q", i, p.ID, want)
		}
	}

	cancel()
	<-done
}

func TestManager_WALDrainsBeforeRing(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	cfg.RingSize = 100
	input := make(chan normalizer.TelemetryPoint, 100)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Manually write some data to the WAL before starting.
	m.wal.Write(bufTestPoint("WAL-1"))
	m.wal.Write(bufTestPoint("WAL-2"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Send some ring data.
	time.Sleep(50 * time.Millisecond)
	input <- bufTestPoint("RING-1")

	got := collectOutput(m.Output(), 3, 5*time.Second)
	if len(got) < 3 {
		t.Fatalf("got %d points, want >= 3", len(got))
	}

	// WAL points should come first.
	if got[0].ID != "WAL-1" {
		t.Errorf("first point: got %q, want WAL-1", got[0].ID)
	}
	if got[1].ID != "WAL-2" {
		t.Errorf("second point: got %q, want WAL-2", got[1].ID)
	}

	cancel()
	<-done
}

func TestManager_ShutdownClosesOutput(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	input := make(chan normalizer.TelemetryPoint, 100)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	cancel()
	<-done

	// Output channel should be closed after Start returns.
	select {
	case _, ok := <-m.Output():
		if ok {
			t.Error("expected output channel to be closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("output channel not closed after shutdown")
	}
}

func TestManager_StatsCounters(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	cfg.RingSize = 3 // Small ring
	input := make(chan normalizer.TelemetryPoint, 100)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Disconnect — ring is flushed to WAL (currently empty, so no flush).
	m.OnConnChange(false)
	time.Sleep(50 * time.Millisecond)

	// Send 6 points — all go directly to WAL while disconnected.
	for i := range 6 {
		input <- bufTestPoint(string(rune('A' + i)))
	}
	time.Sleep(200 * time.Millisecond)

	// Reconnect and drain.
	m.OnConnChange(true)
	collectOutput(m.Output(), 6, 5*time.Second)

	ringPushes, walSpills, walDrained, dropped := m.Stats()
	// All 6 points go directly to WAL when disconnected (ring was empty).
	if ringPushes != 0 {
		t.Errorf("ringPushes = %d, want 0", ringPushes)
	}
	if walSpills != 6 {
		t.Errorf("walSpills = %d, want 6", walSpills)
	}
	if walDrained != 6 {
		t.Errorf("walDrained = %d, want 6", walDrained)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}

	cancel()
	<-done
}

func TestManager_FIFOOrderAcrossDisconnect(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	cfg.RingSize = 10
	input := make(chan normalizer.TelemetryPoint, 100)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Send first 5 points while connected — they go to ring.
	for i := range 5 {
		input <- bufTestPoint(string(rune('A' + i)))
	}
	time.Sleep(100 * time.Millisecond)

	// Disconnect — ring points (A-E) are flushed to WAL.
	m.OnConnChange(false)
	time.Sleep(50 * time.Millisecond)

	// Send 5 more while disconnected — they go directly to WAL.
	for i := 5; i < 10; i++ {
		input <- bufTestPoint(string(rune('A' + i)))
	}
	time.Sleep(100 * time.Millisecond)

	// Reconnect.
	m.OnConnChange(true)

	// Should get all 10 points in strict FIFO order: A,B,C,D,E,F,G,H,I,J.
	got := collectOutput(m.Output(), 10, 5*time.Second)
	if len(got) != 10 {
		t.Fatalf("got %d points, want 10", len(got))
	}
	for i, p := range got {
		want := string(rune('A' + i))
		if p.ID != want {
			t.Errorf("point %d: got %q, want %q", i, p.ID, want)
		}
	}

	cancel()
	<-done
}

func TestManager_ShortDisconnectFIFO(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	cfg.RingSize = 20 // Large enough that no WAL spill before disconnect
	input := make(chan normalizer.TelemetryPoint, 100)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Send 5 points while connected — all go to ring (no WAL).
	for i := range 5 {
		input <- bufTestPoint(string(rune('A' + i)))
	}
	time.Sleep(100 * time.Millisecond)

	// Disconnect — ring is flushed to WAL.
	m.OnConnChange(false)
	time.Sleep(50 * time.Millisecond)

	// Reconnect immediately — no new points during disconnect.
	m.OnConnChange(true)

	got := collectOutput(m.Output(), 5, 5*time.Second)
	if len(got) != 5 {
		t.Fatalf("got %d points, want 5", len(got))
	}
	for i, p := range got {
		want := string(rune('A' + i))
		if p.ID != want {
			t.Errorf("point %d: got %q, want %q", i, p.ID, want)
		}
	}

	cancel()
	<-done
}

func TestManager_CrashRecovery(t *testing.T) {
	t.Parallel()

	walDir := t.TempDir()
	cfg := config.BufferConfig{
		RingSize:   100,
		WALDir:     walDir,
		WALMaxSize: "10MB",
	}

	// Simulate a previous run that wrote WAL data.
	wal, err := NewWAL(walDir, 10<<20, discardLogger())
	if err != nil {
		t.Fatalf("NewWAL: %v", err)
	}
	wal.Write(bufTestPoint("PREV-1"))
	wal.Write(bufTestPoint("PREV-2"))
	wal.Close()

	// Now create a new manager (simulating restart).
	input := make(chan normalizer.TelemetryPoint, 100)
	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Should drain the WAL data from the previous run.
	got := collectOutput(m.Output(), 2, 5*time.Second)
	if len(got) != 2 {
		t.Fatalf("got %d points, want 2", len(got))
	}
	if got[0].ID != "PREV-1" {
		t.Errorf("first point: got %q, want PREV-1", got[0].ID)
	}
	if got[1].ID != "PREV-2" {
		t.Errorf("second point: got %q, want PREV-2", got[1].ID)
	}

	cancel()
	<-done
}

func TestManager_ConcurrentDisconnectFIFO(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	cfg.RingSize = 5 // Small ring to stress flush path
	input := make(chan normalizer.TelemetryPoint, 500)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	const total = 200

	// Start a goroutine writing points continuously.
	go func() {
		for i := range total {
			input <- bufTestPoint(fmt.Sprintf("%04d", i))
		}
	}()

	// Disconnect mid-stream.
	time.Sleep(10 * time.Millisecond)
	m.OnConnChange(false)

	// Brief disconnect window.
	time.Sleep(50 * time.Millisecond)
	m.OnConnChange(true)

	// Collect all points.
	got := collectOutput(m.Output(), total, 10*time.Second)
	if len(got) != total {
		t.Fatalf("got %d points, want %d", len(got), total)
	}

	// Verify strict FIFO order — no losses, no reordering.
	for i, p := range got {
		want := fmt.Sprintf("%04d", i)
		if p.ID != want {
			t.Errorf("point %d: got %q, want %q", i, p.ID, want)
			break
		}
	}

	cancel()
	<-done
}

func TestManager_RingOverflowFIFO(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	cfg.RingSize = 3 // Very small ring to force overflow
	input := make(chan normalizer.TelemetryPoint, 300)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Send many points while connected — with ring size 3, overflow will occur.
	const total = 50
	for i := range total {
		input <- bufTestPoint(fmt.Sprintf("%04d", i))
	}

	got := collectOutput(m.Output(), total, 10*time.Second)
	if len(got) != total {
		t.Fatalf("got %d points, want %d", len(got), total)
	}

	// Verify strict FIFO order.
	for i, p := range got {
		want := fmt.Sprintf("%04d", i)
		if p.ID != want {
			t.Errorf("point %d: got %q, want %q", i, p.ID, want)
			break
		}
	}

	cancel()
	<-done
}

func TestManager_RepeatedRingOverflowFIFO(t *testing.T) {
	t.Parallel()

	cfg := testBufferConfig(t)
	cfg.RingSize = 2 // Tiny ring — overflow on every 3rd point
	input := make(chan normalizer.TelemetryPoint, 500)

	m, err := NewManager(cfg, input, discardLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Start(ctx) }()

	// Send enough points to trigger many overflow+flush cycles.
	const total = 300
	for i := range total {
		input <- bufTestPoint(fmt.Sprintf("%04d", i))
	}

	got := collectOutput(m.Output(), total, 15*time.Second)
	if len(got) != total {
		t.Fatalf("got %d points, want %d", len(got), total)
	}

	// Verify strict FIFO order across multiple overflow cycles.
	for i, p := range got {
		want := fmt.Sprintf("%04d", i)
		if p.ID != want {
			t.Errorf("point %d: got %q, want %q", i, p.ID, want)
			break
		}
	}

	cancel()
	<-done
}
