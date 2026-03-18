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
	StateIdle   WorkloadState = "idle"   // util < 10% sustained 5+ min
	StateActive WorkloadState = "active" // util 10-70%
	StatePeak   WorkloadState = "peak"   // util > 70% sustained 2+ min
)

const (
	idleThresholdPct = 10
	peakThresholdPct = 70
	idleWindow       = 5 * time.Minute
	peakWindow       = 2 * time.Minute
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

	// Idle: current util < 10% AND all samples in last 5 minutes are < 10%
	if utilPct < idleThresholdPct {
		allIdle := true
		for _, s := range c.samples {
			if s.utilPct >= idleThresholdPct {
				allIdle = false
				break
			}
		}
		if allIdle {
			return StateIdle
		}
	}

	// Peak: current util > 70% AND all samples in last 2 minutes are > 70%
	if utilPct > peakThresholdPct {
		peakCutoff := at.Add(-peakWindow)
		allPeak := true
		for _, s := range c.samples {
			if s.at.Before(peakCutoff) {
				continue
			}
			if s.utilPct <= peakThresholdPct {
				allPeak = false
				break
			}
		}
		if allPeak {
			return StatePeak
		}
	}

	return StateActive
}
