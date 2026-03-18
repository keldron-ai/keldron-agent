// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"math"
	"sync"
	"time"
)

const (
	stabilitySustainedMinutes = 10
	stabilityMinSamples       = 60 // 10 min at 10s polling
)

// StabilityTracker tracks temperature standard deviation during sustained peak load.
type StabilityTracker struct {
	mu                   sync.Mutex
	sustainedLoadSamples []float64
	sustainedSince       time.Time
	consecutivePeakCount int
	lastState            WorkloadState
}

// NewStabilityTracker creates a new thermal stability tracker.
func NewStabilityTracker() *StabilityTracker {
	return &StabilityTracker{
		sustainedLoadSamples: make([]float64, 0, 128),
	}
}

// Update processes a sample. Call with workload state, temperature, and timestamp.
func (s *StabilityTracker) Update(state WorkloadState, tempC float64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state != StatePeak {
		s.sustainedLoadSamples = s.sustainedLoadSamples[:0]
		s.consecutivePeakCount = 0
		s.lastState = state
		return
	}

	if s.lastState != StatePeak {
		s.sustainedSince = at
		s.consecutivePeakCount = 0
	}
	s.consecutivePeakCount++
	s.lastState = state

	// Only add samples after 10+ minutes of sustained peak
	if at.Sub(s.sustainedSince) >= stabilitySustainedMinutes*time.Minute {
		s.sustainedLoadSamples = append(s.sustainedLoadSamples, tempC)
	}
}

// Result returns the thermal stability result.
func (s *StabilityTracker) Result() *ThermalStabilityResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.sustainedLoadSamples) < stabilityMinSamples {
		return &ThermalStabilityResult{
			Available:          false,
			UnderSustainedLoad: len(s.sustainedLoadSamples) > 0,
			Note:               "Requires sustained load (>70% util for 10+ min) to measure",
		}
	}

	sd := stdDev(s.sustainedLoadSamples)
	rating := stabilityRating(sd)

	return &ThermalStabilityResult{
		Available:          true,
		UnderSustainedLoad: true,
		StabilityCelsius:   sd,
		Rating:             rating,
	}
}

func stdDev(x []float64) float64 {
	if len(x) < 2 {
		return 0
	}
	var sum float64
	for _, v := range x {
		sum += v
	}
	mean := sum / float64(len(x))
	var sqDiff float64
	for _, v := range x {
		d := v - mean
		sqDiff += d * d
	}
	return math.Sqrt(sqDiff / float64(len(x)-1))
}

func stabilityRating(stdDev float64) string {
	switch {
	case stdDev < 1.0:
		return "stable"
	case stdDev < 2.5:
		return "normal"
	case stdDev < 5.0:
		return "elevated"
	default:
		return "unstable"
	}
}
