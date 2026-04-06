// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"math"
	"sync"
	"time"
)

const (
	healthWindowDuration = 30 * time.Minute
	warmupWindowDuration = 5 * time.Minute
	baselineTempC        = 20.0
	maxHealthSamples     = 2000
	defaultThrottleC     = 100.0
)

// healthSample is one telemetry point in the rolling health window.
type healthSample struct {
	at           time.Time
	tempC        float64
	tempCPresent bool
	utilPct      float64
	powerW       float64
}

// rollingSeries holds time-ordered samples for a device (last ~30 minutes).
type rollingSeries struct {
	mu sync.Mutex

	samples []healthSample

	firstSeenAt time.Time

	// spikeSegmentStart is set when temp crosses above recoveryTarget from at or below.
	spikeSegmentStart time.Time
}

func newRollingSeries() *rollingSeries {
	return &rollingSeries{
		samples: make([]healthSample, 0, 256),
	}
}

func (r *rollingSeries) append(s healthSample) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.firstSeenAt.IsZero() {
		r.firstSeenAt = s.at
	}
	r.samples = append(r.samples, s)
	r.pruneLocked(s.at)
	r.capLocked()
}

func (r *rollingSeries) pruneLocked(now time.Time) {
	cutoff := now.Add(-healthWindowDuration)
	i := 0
	for i < len(r.samples) && r.samples[i].at.Before(cutoff) {
		i++
	}
	if i > 0 {
		r.samples = append(r.samples[:0], r.samples[i:]...)
	}
}

func (r *rollingSeries) capLocked() {
	if len(r.samples) > maxHealthSamples {
		r.samples = append(r.samples[:0], r.samples[len(r.samples)-maxHealthSamples:]...)
	}
}

func (r *rollingSeries) snapshot() []healthSample {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.pruneLocked(now)
	r.capLocked()
	out := make([]healthSample, len(r.samples))
	copy(out, r.samples)
	return out
}

func (r *rollingSeries) firstSeen() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.firstSeenAt
}

// updateSpikeSegmentStart updates spike segment start from the last appended sample vs previous.
func (r *rollingSeries) updateSpikeSegmentStart(recoveryTarget float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.samples) == 0 {
		r.spikeSegmentStart = time.Time{}
		return
	}
	last := r.samples[len(r.samples)-1]
	if !last.tempCPresent {
		return
	}
	if last.tempC > recoveryTarget {
		if len(r.samples) == 1 {
			r.spikeSegmentStart = last.at
			return
		}
		prev := r.samples[len(r.samples)-2]
		if !prev.tempCPresent || prev.tempC < recoveryTarget {
			r.spikeSegmentStart = last.at
		}
	} else {
		r.spikeSegmentStart = time.Time{}
	}
}

func (r *rollingSeries) lastSample() (healthSample, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.samples) == 0 {
		return healthSample{}, false
	}
	return r.samples[len(r.samples)-1], true
}

func (r *rollingSeries) spikeStart() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spikeSegmentStart
}

func effectiveThrottleLimit(limitC float64) float64 {
	if limitC <= baselineTempC+0.5 {
		return defaultThrottleC
	}
	return limitC
}

func recoveryTargetC(throttleLimit float64) float64 {
	t := effectiveThrottleLimit(throttleLimit)
	return baselineTempC + (t-baselineTempC)*0.5
}

// computeHeadroom builds TDRResult from samples with temp >= baseline (idle/off excluded).
func computeHeadroom(samples []healthSample, throttleLimit float64, warmingUp bool) *TDRResult {
	tLim := effectiveThrottleLimit(throttleLimit)
	denom := tLim - baselineTempC
	if denom <= 0 {
		denom = 80
	}

	if len(samples) == 0 {
		return &TDRResult{
			Available: false,
			Note:      "No temperature data in window",
			WarmingUp: warmingUp,
		}
	}

	var warmed []float64
	anyTempPresent := false
	for _, s := range samples {
		if s.tempCPresent {
			anyTempPresent = true
			if s.tempC >= baselineTempC {
				warmed = append(warmed, s.tempC)
			}
		}
	}
	if !anyTempPresent {
		return &TDRResult{
			Available: false,
			Note:      "No temperature data in window",
			WarmingUp: warmingUp,
		}
	}
	if len(warmed) == 0 {
		return &TDRResult{
			Available:       true,
			NoSustainedLoad: true,
			Rating:          "good",
			Note:            "No sustained load",
			ThrottleLimitC:  tLim,
			WarmingUp:       warmingUp,
		}
	}

	var sum float64
	maxT := warmed[0]
	for _, t := range warmed {
		sum += t
		if t > maxT {
			maxT = t
		}
	}
	avg := sum / float64(len(warmed))
	headroom := ((avg - baselineTempC) / denom) * 100
	peakProx := ((maxT - baselineTempC) / denom) * 100

	rating := headroomRating(headroom)

	return &TDRResult{
		Available:        true,
		AvgTempC:         avg,
		MaxTempC:         maxT,
		HeadroomUsedPct:  headroom,
		PeakProximityPct: peakProx,
		ThrottleLimitC:   tLim,
		Rating:           rating,
		WarmingUp:        warmingUp,
	}
}

func headroomRating(headroomUsedPct float64) string {
	switch {
	case headroomUsedPct < 50:
		return "good"
	case headroomUsedPct <= 75:
		return "fair"
	default:
		return "poor"
	}
}

// scanRecoveries finds the most recent completed recovery duration and whether any spike occurred.
func scanRecoveries(samples []healthSample, recoveryTarget float64) (lastRecoverySec int, lastPeakC float64, lastRecoveryCompleteAt time.Time, hadSpike bool) {
	if len(samples) < 2 {
		return 0, 0, time.Time{}, false
	}

	i := 0
	n := len(samples)
	for i < n {
		// Skip samples at or below recovery target, or without temperature.
		for i < n && (!samples[i].tempCPresent || samples[i].tempC <= recoveryTarget) {
			i++
		}
		if i >= n {
			break
		}
		hadSpike = true
		maxT := -1.0
		peakIdx := -1
		for i < n {
			if !samples[i].tempCPresent {
				i++
				continue
			}
			if samples[i].tempC >= recoveryTarget {
				if samples[i].tempC >= maxT {
					maxT = samples[i].tempC
					peakIdx = i
				}
				i++
			} else {
				break
			}
		}
		if peakIdx < 0 {
			continue
		}
		peakTime := samples[peakIdx].at
		peakC := samples[peakIdx].tempC
		for k := peakIdx + 1; k < n; k++ {
			if !samples[k].tempCPresent {
				continue
			}
			if samples[k].tempC < recoveryTarget {
				sec := int(samples[k].at.Sub(peakTime).Seconds())
				if sec < 0 {
					sec = 0
				}
				if lastRecoveryCompleteAt.IsZero() || samples[k].at.After(lastRecoveryCompleteAt) {
					lastRecoveryCompleteAt = samples[k].at
					lastRecoverySec = sec
					lastPeakC = peakC
				}
				break
			}
		}
	}
	return lastRecoverySec, lastPeakC, lastRecoveryCompleteAt, hadSpike
}

func findSpikeSegmentStart(samples []healthSample, recoveryTarget float64) time.Time {
	if len(samples) == 0 {
		return time.Time{}
	}
	last := samples[len(samples)-1]
	if !last.tempCPresent || last.tempC < recoveryTarget {
		return time.Time{}
	}
	for i := len(samples) - 2; i >= 0; i-- {
		if !samples[i].tempCPresent {
			continue
		}
		if samples[i].tempC < recoveryTarget {
			// Find the first sample after i that is present and above the target.
			for j := i + 1; j < len(samples); j++ {
				if samples[j].tempCPresent && samples[j].tempC >= recoveryTarget {
					return samples[j].at
				}
			}
			return time.Time{}
		}
	}
	// All present samples are above the target; find the first present one.
	for k := 0; k < len(samples); k++ {
		if samples[k].tempCPresent && samples[k].tempC >= recoveryTarget {
			return samples[k].at
		}
	}
	return time.Time{}
}

func computeThermalRecovery(samples []healthSample, throttleLimit float64, spikeSegmentStart time.Time, now time.Time, warmingUp bool) *TREResult {
	tLim := effectiveThrottleLimit(throttleLimit)
	rt := recoveryTargetC(tLim)

	lastRecSec, lastPeakC, _, hadSpike := scanRecoveries(samples, rt)

	last, ok := lastSampleFromSlice(samples)
	lastAbove := ok && last.tempCPresent && last.tempC > rt

	var segStart time.Time
	if lastAbove {
		segStart = spikeSegmentStart
		if segStart.IsZero() {
			segStart = findSpikeSegmentStart(samples, rt)
		}
	}

	var activeSec int
	if lastAbove && !segStart.IsZero() {
		activeSec = int(now.Sub(segStart).Seconds())
		if activeSec < 0 {
			activeSec = 0
		}
	}

	// Active spike: prioritize Active display over last recovery.
	if lastAbove {
		rating := recoveryRating(activeSec)
		return &TREResult{
			Available:       true,
			SpikeActive:     true,
			ActiveSpikeSec:  activeSec,
			LastPeakTempC:   last.tempC,
			RecoveryTargetC: rt,
			Rating:          rating,
			WarmingUp:       warmingUp,
		}
	}

	if !hadSpike {
		return &TREResult{
			Available:       true,
			NoSpikes:        true,
			RecoveryTargetC: rt,
			Rating:          "good",
			Note:            "No spikes detected",
			WarmingUp:       warmingUp,
		}
	}

	return &TREResult{
		Available:       true,
		LastRecoverySec: lastRecSec,
		LastPeakTempC:   lastPeakC,
		RecoveryTargetC: rt,
		Rating:          recoveryRating(lastRecSec),
		WarmingUp:       warmingUp,
	}
}

func lastSampleFromSlice(samples []healthSample) (healthSample, bool) {
	if len(samples) == 0 {
		return healthSample{}, false
	}
	return samples[len(samples)-1], true
}

func recoveryRating(sec int) string {
	switch {
	case sec < 60:
		return "good"
	case sec <= 180:
		return "fair"
	default:
		return "poor"
	}
}

func computeStability(samples []healthSample, warmingUp bool) *ThermalStabilityResult {
	if len(samples) == 0 {
		return &ThermalStabilityResult{
			Available: false,
			Note:      "No temperature data in window",
			WarmingUp: warmingUp,
		}
	}
	temps := make([]float64, 0, len(samples))
	for _, s := range samples {
		if s.tempCPresent {
			temps = append(temps, s.tempC)
		}
	}
	if len(temps) == 0 {
		return &ThermalStabilityResult{
			Available: false,
			Note:      "No temperature data in window",
			WarmingUp: warmingUp,
		}
	}
	sd := stdDevSample(temps)
	return &ThermalStabilityResult{
		Available:     true,
		StdDevCelsius: sd,
		Rating:        stabilityRating(sd),
		WarmingUp:     warmingUp,
	}
}

func stdDevSample(x []float64) float64 {
	n := len(x)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return 0
	}
	var sum float64
	for _, v := range x {
		sum += v
	}
	mean := sum / float64(n)
	var sqDiff float64
	for _, v := range x {
		d := v - mean
		sqDiff += d * d
	}
	// Sample std dev; for identical values variance is 0
	return math.Sqrt(sqDiff / float64(n-1))
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

func computePerfPerWatt(samples []healthSample) *PerfPerWattResult {
	var utilSum, powerSum float64
	var n int
	for _, s := range samples {
		if s.powerW >= 1.0 {
			utilSum += s.utilPct
			powerSum += s.powerW
			n++
		}
	}
	if n == 0 {
		return &PerfPerWattResult{
			Available: false,
			Unit:      "%/W",
			UnitID:    "pct_util_per_watt",
			Note:      "(power < 1W)",
		}
	}
	meanU := utilSum / float64(n)
	meanP := powerSum / float64(n)
	if meanP < 1.0 {
		return &PerfPerWattResult{
			Available: false,
			Unit:      "%/W",
			UnitID:    "pct_util_per_watt",
			Note:      "(power < 1W)",
		}
	}
	return &PerfPerWattResult{
		Available: true,
		Value:     meanU / meanP,
		Unit:      "%/W",
		UnitID:    "pct_util_per_watt",
	}
}

func isWarmingUp(firstSeenAt, now time.Time) bool {
	if firstSeenAt.IsZero() {
		return true
	}
	return now.Sub(firstSeenAt) < warmupWindowDuration
}
