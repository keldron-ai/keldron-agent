// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package sender

import (
	"sync"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func testPoint(id string) normalizer.TelemetryPoint {
	return normalizer.TelemetryPoint{
		ID:          id,
		AgentID:     "test-agent",
		AdapterName: "dcgm",
		Source:      "gpu-node-01",
		RackID:      "rack-A1",
		Timestamp:   time.Now(),
		ReceivedAt:  time.Now(),
		Metrics:     map[string]float64{"temperature": 72.5},
	}
}

func TestBatcher_Add(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maxSize  int
		addCount int
		wantFull bool
	}{
		{
			name:     "below max returns false",
			maxSize:  5,
			addCount: 3,
			wantFull: false,
		},
		{
			name:     "at max returns true",
			maxSize:  5,
			addCount: 5,
			wantFull: true,
		},
		{
			name:     "single item batch",
			maxSize:  1,
			addCount: 1,
			wantFull: true,
		},
		{
			name:     "one below max returns false",
			maxSize:  10,
			addCount: 9,
			wantFull: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := NewBatcher(tt.maxSize)
			var full bool
			for i := range tt.addCount {
				full = b.Add(testPoint(string(rune('A' + i))))
			}
			if full != tt.wantFull {
				t.Errorf("Add() returned %v after %d adds (maxSize=%d), want %v",
					full, tt.addCount, tt.maxSize, tt.wantFull)
			}
		})
	}
}

func TestBatcher_Flush(t *testing.T) {
	t.Parallel()

	t.Run("returns accumulated points and resets", func(t *testing.T) {
		t.Parallel()
		b := NewBatcher(10)
		b.Add(testPoint("1"))
		b.Add(testPoint("2"))
		b.Add(testPoint("3"))

		got := b.Flush()
		if len(got) != 3 {
			t.Fatalf("Flush() returned %d points, want 3", len(got))
		}
		if got[0].ID != "1" || got[1].ID != "2" || got[2].ID != "3" {
			t.Errorf("Flush() returned wrong order: %v, %v, %v", got[0].ID, got[1].ID, got[2].ID)
		}

		// Batch should be empty after flush.
		if b.Len() != 0 {
			t.Errorf("Len() after Flush() = %d, want 0", b.Len())
		}
	})

	t.Run("empty batch returns nil", func(t *testing.T) {
		t.Parallel()
		b := NewBatcher(10)
		got := b.Flush()
		if got != nil {
			t.Errorf("Flush() on empty batch = %v, want nil", got)
		}
	})

	t.Run("double flush returns nil on second call", func(t *testing.T) {
		t.Parallel()
		b := NewBatcher(10)
		b.Add(testPoint("1"))
		b.Flush()
		got := b.Flush()
		if got != nil {
			t.Errorf("second Flush() = %v, want nil", got)
		}
	})
}

func TestBatcher_Len(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		addCount int
		wantLen  int
	}{
		{"empty", 0, 0},
		{"one point", 1, 1},
		{"five points", 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := NewBatcher(100)
			for i := range tt.addCount {
				b.Add(testPoint(string(rune('A' + i))))
			}
			if got := b.Len(); got != tt.wantLen {
				t.Errorf("Len() = %d, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestBatcher_ConcurrentAdd(t *testing.T) {
	t.Parallel()

	b := NewBatcher(1000)
	const goroutines = 10
	const pointsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range pointsPerGoroutine {
				b.Add(testPoint(string(rune(id*1000 + j))))
			}
		}(i)
	}
	wg.Wait()

	if got := b.Len(); got != goroutines*pointsPerGoroutine {
		t.Errorf("Len() after concurrent adds = %d, want %d", got, goroutines*pointsPerGoroutine)
	}

	flushed := b.Flush()
	if len(flushed) != goroutines*pointsPerGoroutine {
		t.Errorf("Flush() returned %d points, want %d", len(flushed), goroutines*pointsPerGoroutine)
	}
}
