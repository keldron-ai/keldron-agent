// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package sender

import (
	"sync"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

// Batcher accumulates TelemetryPoints and flushes them as a slice
// when the batch reaches maxSize or the caller triggers a time-based flush.
type Batcher struct {
	maxSize int
	mu      sync.Mutex
	points  []normalizer.TelemetryPoint
}

// NewBatcher creates a Batcher that signals full when maxSize points accumulate.
func NewBatcher(maxSize int) *Batcher {
	return &Batcher{
		maxSize: maxSize,
		points:  make([]normalizer.TelemetryPoint, 0, maxSize),
	}
}

// Add appends a point to the batch. Returns true if the batch is now full
// and the caller should flush.
func (b *Batcher) Add(point normalizer.TelemetryPoint) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.points = append(b.points, point)
	return len(b.points) >= b.maxSize
}

// Flush returns the accumulated points and resets the batch.
// Returns nil if the batch is empty.
func (b *Batcher) Flush() []normalizer.TelemetryPoint {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.points) == 0 {
		return nil
	}
	out := b.points
	b.points = make([]normalizer.TelemetryPoint, 0, b.maxSize)
	return out
}

// Len returns the current number of points in the batch.
func (b *Batcher) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.points)
}
