// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import "math"

// RingBuffer is a fixed-capacity circular buffer for float64 values.
// Used for thermal (5-min) and volatility (30-min) windows.
type RingBuffer struct {
	data     []float64
	capacity int
	head     int
	count    int
}

// NewRingBuffer creates a RingBuffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 1
	}
	return &RingBuffer{
		data:     make([]float64, capacity),
		capacity: capacity,
	}
}

// Add appends a value to the buffer, overwriting the oldest if full.
func (rb *RingBuffer) Add(value float64) {
	if rb.capacity == 0 {
		return
	}
	if rb.count < rb.capacity {
		rb.data[rb.count] = value
		rb.count++
	} else {
		rb.data[rb.head] = value
		rb.head = (rb.head + 1) % rb.capacity
	}
}

// Values returns all values in chronological order (oldest first).
func (rb *RingBuffer) Values() []float64 {
	if rb.count == 0 {
		return nil
	}
	out := make([]float64, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.capacity
		out[i] = rb.data[idx]
	}
	return out
}

// IsFull returns true if the buffer has reached capacity.
func (rb *RingBuffer) IsFull() bool {
	return rb.count >= rb.capacity
}

// Len returns the number of values currently in the buffer.
func (rb *RingBuffer) Len() int {
	return rb.count
}

// Mean returns the arithmetic mean of values in the buffer.
func (rb *RingBuffer) Mean() float64 {
	if rb.count == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.capacity
		sum += rb.data[idx]
	}
	return sum / float64(rb.count)
}

// Stdev returns the sample standard deviation of values in the buffer.
func (rb *RingBuffer) Stdev() float64 {
	if rb.count < 2 {
		return 0
	}
	mean := rb.Mean()
	var sumSq float64
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.capacity
		diff := rb.data[idx] - mean
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(rb.count-1))
}

// Oldest returns the oldest value in the buffer.
func (rb *RingBuffer) Oldest() (float64, bool) {
	if rb.count == 0 {
		return 0, false
	}
	return rb.data[rb.head], true
}

// Newest returns the most recently added value.
func (rb *RingBuffer) Newest() (float64, bool) {
	if rb.count == 0 {
		return 0, false
	}
	lastIdx := (rb.head + rb.count - 1) % rb.capacity
	return rb.data[lastIdx], true
}
