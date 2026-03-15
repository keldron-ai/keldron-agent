// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package temperature

import (
	"math"
	"sync"
)

const tolerance = 0.01

// StaleDetector detects when a sensor's readings have been unchanged for N consecutive polls.
type StaleDetector struct {
	history   map[string][]float64
	threshold int
	mu        sync.RWMutex
}

// NewStaleDetector creates a StaleDetector with the given threshold.
// If threshold is <= 0, it defaults to 5.
func NewStaleDetector(threshold int) *StaleDetector {
	if threshold <= 0 {
		threshold = 5
	}
	return &StaleDetector{
		history:   make(map[string][]float64),
		threshold: threshold,
	}
}

// Check appends the value to the sensor's history and returns true if the reading is stale
// (all values in the buffer are identical within 0.01°C tolerance).
func (s *StaleDetector) Check(sensorID string, value float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	buf := s.history[sensorID]
	buf = append(buf, value)
	if len(buf) > s.threshold {
		buf = buf[1:]
	}
	s.history[sensorID] = buf

	if len(buf) < s.threshold {
		return false
	}

	// All values in buffer must be within tolerance of each other
	ref := buf[0]
	for i := 1; i < len(buf); i++ {
		if math.Abs(buf[i]-ref) > tolerance {
			return false
		}
	}
	return true
}
