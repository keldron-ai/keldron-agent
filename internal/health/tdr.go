// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"sort"
	"sync"
	"time"
)

const (
	tdrWindowHours    = 24
	tdrMaxSamples     = 8640 // 24h at 10s polling
	tdrMinIdleSamples = 10
	tdrMinPeakSamples = 10
)

// TDRState tracks idle and peak temperature samples for Thermal Dynamic Range.
type TDRState struct {
	mu          sync.Mutex
	idleSamples []tempSample
	peakSamples []tempSample
}

// NewTDRState creates a new TDR tracker.
func NewTDRState() *TDRState {
	return &TDRState{
		idleSamples: make([]tempSample, 0, 256),
		peakSamples: make([]tempSample, 0, 256),
	}
}

// AddIdle records an idle-tagged temperature sample. Prunes samples older than 24h.
func (t *TDRState) AddIdle(tempC float64, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.idleSamples = append(t.idleSamples, tempSample{tempC: tempC, at: at})
	t.pruneLocked(at)
}

// AddPeak records a peak-tagged temperature sample. Prunes samples older than 24h.
func (t *TDRState) AddPeak(tempC float64, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.peakSamples = append(t.peakSamples, tempSample{tempC: tempC, at: at})
	t.pruneLocked(at)
}

func (t *TDRState) pruneLocked(now time.Time) {
	cutoff := now.Add(-tdrWindowHours * time.Hour)

	prune := func(samples []tempSample) []tempSample {
		i := 0
		for i < len(samples) && samples[i].at.Before(cutoff) {
			i++
		}
		if i > 0 {
			samples = append(samples[:0], samples[i:]...)
		}
		// Cap at max to bound memory
		if len(samples) > tdrMaxSamples {
			samples = samples[len(samples)-tdrMaxSamples:]
		}
		return samples
	}

	t.idleSamples = prune(t.idleSamples)
	t.peakSamples = prune(t.peakSamples)
}

// IdleMedian returns the median idle temperature for TRE baseline. Returns 0, false if insufficient data.
func (t *TDRState) IdleMedian() (float64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.idleSamples) < tdrMinIdleSamples {
		return 0, false
	}
	temps := make([]float64, len(t.idleSamples))
	for i, s := range t.idleSamples {
		temps[i] = s.tempC
	}
	return median(temps), true
}

// Compute returns the TDR result. Requires at least 10 idle and 10 peak samples.
func (t *TDRState) Compute() *TDRResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.idleSamples) < tdrMinIdleSamples || len(t.peakSamples) < tdrMinPeakSamples {
		return &TDRResult{
			Available: false,
			Note:      "Insufficient data — needs idle and peak observations",
		}
	}

	idleTemps := make([]float64, len(t.idleSamples))
	for i, s := range t.idleSamples {
		idleTemps[i] = s.tempC
	}
	peakTemps := make([]float64, len(t.peakSamples))
	for i, s := range t.peakSamples {
		peakTemps[i] = s.tempC
	}

	tIdle := median(idleTemps)
	tPeak := percentile(peakTemps, 90)
	tdr := tPeak - tIdle

	var rating string
	switch {
	case tdr > 25:
		rating = "healthy"
	case tdr > 15:
		rating = "normal"
	case tdr > 8:
		rating = "compressed"
	default:
		rating = "critical"
	}

	return &TDRResult{
		Available:       true,
		TDRCelsius:      tdr,
		IdleTempC:       tIdle,
		PeakTempC:       tPeak,
		Rating:          rating,
		IdleSampleCount: len(t.idleSamples),
		PeakSampleCount: len(t.peakSamples),
		WindowHours:     tdrWindowHours,
	}
}

func median(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	sorted := make([]float64, len(x))
	copy(sorted, x)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func percentile(x []float64, p float64) float64 {
	if len(x) == 0 {
		return 0
	}
	sorted := make([]float64, len(x))
	copy(sorted, x)
	sort.Float64s(sorted)
	idx := (p / 100) * float64(len(sorted)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
