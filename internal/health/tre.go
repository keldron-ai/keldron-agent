// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"sync"
	"time"
)

const (
	treHoldDuration   = 60 * time.Second
	treTimeout        = 600 * time.Second
	treBaselineMargin = 2.0 // °C
)

// TRETracker tracks Thermal Recovery Efficiency (cooldown time after load ends).
type TRETracker struct {
	mu        sync.Mutex
	tdrState  *TDRState
	prevState WorkloadState
	prevAt    time.Time

	inRecovery     bool
	recoveryStart  time.Time
	peakTemp       float64
	currentPeakMax float64 // running max temp during current peak period
	holdStart      time.Time
	holdActive     bool

	recoveries []RecoveryEvent
}

// NewTRETracker creates a new TRE tracker. Panics if tdrState is nil.
func NewTRETracker(tdrState *TDRState) *TRETracker {
	if tdrState == nil {
		panic("health: NewTRETracker requires non-nil tdrState")
	}
	return &TRETracker{
		tdrState:   tdrState,
		recoveries: make([]RecoveryEvent, 0, 32),
	}
}

// Update processes a sample. Call with workload state, current temp, and timestamp.
func (t *TRETracker) Update(state WorkloadState, tempC float64, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Track running max temperature during peak period
	if state == StatePeak {
		if t.prevState != StatePeak {
			// Entering peak — reset running max
			t.currentPeakMax = tempC
		} else if tempC > t.currentPeakMax {
			t.currentPeakMax = tempC
		}
	}

	// Detect job end: transition from peak to non-peak
	justExitedPeak := t.prevState == StatePeak && state != StatePeak

	if justExitedPeak {
		// Enter recovery mode (need idle baseline from TDR)
		if _, ok := t.tdrState.IdleMedian(); !ok {
			t.prevState = state
			t.prevAt = at
			return
		}
		t.inRecovery = true
		t.recoveryStart = at
		t.peakTemp = t.currentPeakMax
		t.holdActive = false
	}

	t.prevState = state
	t.prevAt = at

	if !t.inRecovery {
		return
	}

	baseline, ok := t.tdrState.IdleMedian()
	if !ok {
		t.inRecovery = false
		return
	}

	threshold := baseline + treBaselineMargin

	// Timeout
	if at.Sub(t.recoveryStart) > treTimeout {
		t.inRecovery = false
		t.holdActive = false
		return
	}

	// Within baseline + 2°C?
	if tempC <= threshold {
		if !t.holdActive {
			t.holdStart = at
			t.holdActive = true
		} else if at.Sub(t.holdStart) >= treHoldDuration {
			// Recovery complete
			recoverySec := int(at.Sub(t.recoveryStart).Seconds())
			t.recoveries = append(t.recoveries, RecoveryEvent{
				Timestamp:       at,
				PeakTempC:       t.peakTemp,
				BaselineTempC:   baseline,
				RecoverySeconds: recoverySec,
			})
			t.inRecovery = false
			t.holdActive = false
		}
	} else {
		// Temp bounced above threshold — reset hold
		t.holdActive = false
	}
}

// Result returns the TRE result for API.
func (t *TRETracker) Result() *TREResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	_, hasBaseline := t.tdrState.IdleMedian()
	if !hasBaseline {
		return &TREResult{
			Available: false,
			Note:      "Needs established idle baseline — let device idle for 5+ minutes first.",
		}
	}

	if len(t.recoveries) == 0 {
		return &TREResult{
			Available: true,
			Note:      "No recovery events yet — stop a workload to measure.",
		}
	}

	last := t.recoveries[len(t.recoveries)-1]
	rating := treRating(last.RecoverySeconds)

	var sum int
	for _, r := range t.recoveries {
		sum += r.RecoverySeconds
	}
	avgSec := sum / len(t.recoveries)

	return &TREResult{
		Available:         true,
		LastRecoverySec:   last.RecoverySeconds,
		LastPeakTempC:     last.PeakTempC,
		LastBaselineTempC: last.BaselineTempC,
		Rating:            rating,
		RecoveryCount:     len(t.recoveries),
		SessionAvgSec:     avgSec,
	}
}

func treRating(sec int) string {
	switch {
	case sec < 60:
		return "excellent"
	case sec < 180:
		return "normal"
	case sec < 300:
		return "slow"
	default:
		return "poor"
	}
}
