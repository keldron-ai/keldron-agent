// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"sync"
	"time"
)

// WorkloadState classifies each telemetry sample by GPU utilization level.
type WorkloadState string

const (
	StateIdle   WorkloadState = "idle"   // >= 60% of samples < 15% over 5-min window
	StateActive WorkloadState = "active" // neither idle nor peak
	StatePeak   WorkloadState = "peak"   // >= 60% of samples > 70% over 2-min window
)

const (
	idleThresholdPct = 15 // was 10 — macOS compositor can spike to 10–15% briefly
	peakThresholdPct = 70
	idleWindow       = 5 * time.Minute
	peakWindow       = 2 * time.Minute
	classifyRatio    = 0.60 // 60% of samples in window must meet threshold
)

// utilSample holds utilization and timestamp for the rolling buffer.
type utilSample struct {
	utilPct float64
	at      time.Time
}

// Classifier maintains a rolling buffer of utilization samples and classifies
// each new sample by workload state. Uses timestamp-based pruning for
// correct behavior across different poll intervals.
type Classifier struct {
	mu      sync.Mutex
	samples []utilSample
}

// NewClassifier creates a new workload state classifier.
func NewClassifier() *Classifier {
	return &Classifier{
		samples: make([]utilSample, 0, 60), // pre-allocate for typical 5-min window
	}
}

// Add records a utilization sample and prunes samples older than 5 minutes.
func (c *Classifier) Add(utilPct float64, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.samples = append(c.samples, utilSample{utilPct: utilPct, at: at})

	// Prune samples older than 5 minutes (needed for idle check)
	cutoff := at.Add(-idleWindow)
	i := 0
	for i < len(c.samples) && c.samples[i].at.Before(cutoff) {
		i++
	}
	if i > 0 {
		c.samples = append(c.samples[:0], c.samples[i:]...)
	}
}

// Classify returns the workload state for the current sample.
// Call Add before Classify to include the current sample in the buffer.
func (c *Classifier) Classify(utilPct float64, at time.Time) WorkloadState {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Need at least a few samples to classify
	if len(c.samples) < 3 {
		return StateActive
	}

	// Idle: current util < 15% AND >= 60% of samples in last 5 minutes are < 15%
	if utilPct < idleThresholdPct {
		total := 0
		belowThreshold := 0
		for _, s := range c.samples {
			total++
			if s.utilPct < idleThresholdPct {
				belowThreshold++
			}
		}
		if total > 0 && float64(belowThreshold)/float64(total) >= classifyRatio {
			return StateIdle
		}
	}

	// Peak: current util > 70% AND >= 60% of samples in last 2 minutes are > 70%
	if utilPct > peakThresholdPct {
		peakCutoff := at.Add(-peakWindow)
		total := 0
		aboveThreshold := 0
		for _, s := range c.samples {
			if s.at.Before(peakCutoff) {
				continue
			}
			total++
			if s.utilPct > peakThresholdPct {
				aboveThreshold++
			}
		}
		if total >= 3 && float64(aboveThreshold)/float64(total) >= classifyRatio {
			return StatePeak
		}
	}

	return StateActive
}
