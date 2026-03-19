// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"testing"
	"time"
)

func TestClassifier_FewSamples_ReturnsActive(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// 1 sample — too few
	c.Add(5, base)
	if got := c.Classify(5, base); got != StateActive {
		t.Errorf("1 sample: Classify(5) = %v, want active", got)
	}

	// 2 samples — still too few
	c.Add(5, base.Add(5*time.Second))
	if got := c.Classify(5, base.Add(5*time.Second)); got != StateActive {
		t.Errorf("2 samples: Classify(5) = %v, want active", got)
	}
}

func TestClassifier_Idle_60PercentBelowThreshold(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// 10 samples: 6 below 15%, 4 at or above — 60% passes
	for i := 0; i < 6; i++ {
		c.Add(5, base.Add(time.Duration(i)*10*time.Second))
	}
	for i := 0; i < 4; i++ {
		c.Add(20, base.Add(time.Duration(60+i*10)*time.Second))
	}
	at := base.Add(100 * time.Second)
	c.Add(5, at)
	if got := c.Classify(5, at); got != StateIdle {
		t.Errorf("60%% idle: Classify(5) = %v, want idle", got)
	}
}

func TestClassifier_Idle_SingleSpikeTolerated(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// 10 samples: 9 below 15%, 1 spike at 20% — 90% passes, should still be idle
	for i := 0; i < 9; i++ {
		c.Add(5, base.Add(time.Duration(i)*30*time.Second))
	}
	c.Add(20, base.Add(270*time.Second)) // single spike
	at := base.Add(300 * time.Second)
	c.Add(5, at)
	if got := c.Classify(5, at); got != StateIdle {
		t.Errorf("single spike tolerated: Classify(5) = %v, want idle", got)
	}
}

func TestClassifier_Idle_Below50Percent_ReturnsActive(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// 10 samples: 5 below 15%, 5 above — 50% fails
	for i := 0; i < 5; i++ {
		c.Add(5, base.Add(time.Duration(i)*30*time.Second))
	}
	for i := 0; i < 5; i++ {
		c.Add(50, base.Add(time.Duration(150+i*30)*time.Second))
	}
	at := base.Add(300 * time.Second)
	c.Add(5, at)
	if got := c.Classify(5, at); got != StateActive {
		t.Errorf("50%% idle: Classify(5) = %v, want active", got)
	}
}

func TestClassifier_Idle_CurrentUtilAboveThreshold_ReturnsActive(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// All samples below 15%, but current util is 20%
	for i := 0; i < 10; i++ {
		c.Add(5, base.Add(time.Duration(i)*30*time.Second))
	}
	at := base.Add(300 * time.Second)
	c.Add(20, at)
	if got := c.Classify(20, at); got != StateActive {
		t.Errorf("current util 20%%: Classify(20) = %v, want active (idle requires current < 15%%)", got)
	}
}

func TestClassifier_Peak_60PercentAboveThreshold(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// 2-min window at at=base+135s is base+15s..base+135s: 10 samples, 6 above 70% (5×90% + 85%), 4 below — 6/10 = 60%
	for i := 0; i < 5; i++ {
		c.Add(90, base.Add(30*time.Second+time.Duration(i)*15*time.Second))
	}
	for i := 0; i < 4; i++ {
		c.Add(10, base.Add(120*time.Second+time.Duration(i)*5*time.Second))
	}
	at := base.Add(135 * time.Second)
	c.Add(85, at)
	if got := c.Classify(85, at); got != StatePeak {
		t.Errorf("60%% peak: Classify(85) = %v, want peak", got)
	}
}

func TestClassifier_Peak_FewSamplesIn2MinWindow_ReturnsActive(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// Three buffered samples but only two in the 2-min peak window; ratio would be 100% with total==2
	c.Add(90, base.Add(50*time.Second))
	c.Add(90, base.Add(150*time.Second))
	at := base.Add(200 * time.Second)
	c.Add(90, at)
	if got := c.Classify(90, at); got != StateActive {
		t.Errorf("2 in-window high samples: Classify(90) = %v, want active (peak needs >=3 in window)", got)
	}
}

func TestClassifier_Peak_SawtoothPattern(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// Sawtooth: 12 high (90), 5 low (5) — 12/17 = 70.6% above 70% in 2-min window
	for i := 0; i < 12; i++ {
		c.Add(90, base.Add(time.Duration(i*10)*time.Second))
	}
	for i := 0; i < 5; i++ {
		c.Add(5, base.Add(time.Duration(15+i*20)*time.Second))
	}
	at := base.Add(130 * time.Second)
	c.Add(95, at)
	if got := c.Classify(95, at); got != StatePeak {
		t.Errorf("sawtooth pattern: Classify(95) = %v, want peak", got)
	}
}

func TestClassifier_Peak_CurrentUtilBelowThreshold_ReturnsActive(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// All samples in 2-min window above 70%, but current util is 50%
	for i := 0; i < 10; i++ {
		c.Add(90, base.Add(time.Duration(i)*10*time.Second))
	}
	at := base.Add(100 * time.Second)
	c.Add(50, at)
	if got := c.Classify(50, at); got != StateActive {
		t.Errorf("current util 50%%: Classify(50) = %v, want active (peak requires current > 70%%)", got)
	}
}

func TestClassifier_Peak_ExcludesSamplesOutside2MinWindow(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// Samples 3 min ago: all high (would skew ratio if counted for idle)
	// Samples in last 2 min: 2 high, 8 low — 20% peak, should be active
	for i := 0; i < 10; i++ {
		c.Add(95, base.Add(-3*time.Minute+time.Duration(i)*10*time.Second))
	}
	for i := 0; i < 8; i++ {
		c.Add(10, base.Add(time.Duration(i)*10*time.Second))
	}
	c.Add(90, base.Add(80*time.Second))
	c.Add(85, base.Add(90*time.Second))
	at := base.Add(90 * time.Second)
	if got := c.Classify(85, at); got != StateActive {
		t.Errorf("peak window excludes old samples: Classify(85) = %v, want active", got)
	}
}

func TestClassifier_Exactly60Percent_Idle(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// 10 samples: exactly 6 below 15%
	for i := 0; i < 6; i++ {
		c.Add(5, base.Add(time.Duration(i)*30*time.Second))
	}
	for i := 0; i < 4; i++ {
		c.Add(50, base.Add(time.Duration(180+i*30)*time.Second))
	}
	at := base.Add(300 * time.Second)
	c.Add(5, at)
	if got := c.Classify(5, at); got != StateIdle {
		t.Errorf("exactly 60%% idle: Classify(5) = %v, want idle", got)
	}
}

func TestClassifier_Boundary_IdleAt15Percent(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// 14% is below 15% threshold
	for i := 0; i < 10; i++ {
		c.Add(14, base.Add(time.Duration(i)*30*time.Second))
	}
	at := base.Add(300 * time.Second)
	c.Add(14, at)
	if got := c.Classify(14, at); got != StateIdle {
		t.Errorf("14%% util: Classify(14) = %v, want idle", got)
	}

	// 15% is at threshold — strictly < 15% for "below", so 15% does NOT count as below
	// idleThresholdPct is 15, so s.utilPct < 15 means 14.99 counts, 15 does not
	c2 := NewClassifier()
	for i := 0; i < 10; i++ {
		c2.Add(15, base.Add(time.Duration(i)*30*time.Second))
	}
	c2.Add(15, at)
	if got := c2.Classify(15, at); got != StateActive {
		t.Errorf("15%% util (at threshold): Classify(15) = %v, want active (15 is not < 15)", got)
	}
}

func TestClassifier_Boundary_PeakAt70Percent(t *testing.T) {
	c := NewClassifier()
	base := time.Now()

	// 71% is above 70% threshold
	for i := 0; i < 10; i++ {
		c.Add(71, base.Add(time.Duration(i)*10*time.Second))
	}
	at := base.Add(100 * time.Second)
	c.Add(71, at)
	if got := c.Classify(71, at); got != StatePeak {
		t.Errorf("71%% util: Classify(71) = %v, want peak", got)
	}

	// 70% is at threshold — strictly > 70% for "above", so 70% does NOT count
	c2 := NewClassifier()
	for i := 0; i < 10; i++ {
		c2.Add(70, base.Add(time.Duration(i)*10*time.Second))
	}
	c2.Add(70, at)
	if got := c2.Classify(70, at); got != StateActive {
		t.Errorf("70%% util (at threshold): Classify(70) = %v, want active (70 is not > 70)", got)
	}
}
