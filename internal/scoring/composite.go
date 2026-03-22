// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import "math"

const (
	W_THERMAL    = 0.40
	W_POWER      = 0.25
	W_VOLATILITY = 0.15
	W_MEMORY     = 0.20
)

// SeverityThresholds maps behavior_class to [warning, critical] thresholds.
var SeverityThresholds = map[string][2]float64{
	"datacenter_sustained":   {60, 80},
	"consumer_active_cooled": {65, 82},
	"soc_integrated":         {70, 85},
	"sbc_constrained":        {72, 87},
}

// ComputeComposite computes the weighted composite score from thermal, power, volatility, and memory sub-scores.
func ComputeComposite(thermal, power, volatility, memory float64) float64 {
	rLocal := W_THERMAL*thermal + W_POWER*power + W_VOLATILITY*volatility + W_MEMORY*memory
	return math.Min(100, rLocal)
}

// ClassifySeverity returns "normal", "warning", or "critical" based on
// behavior-class thresholds.
func ClassifySeverity(composite float64, behaviorClass string) string {
	thresholds, ok := SeverityThresholds[behaviorClass]
	if !ok {
		thresholds = SeverityThresholds["consumer_active_cooled"]
	}
	if composite >= thresholds[1] {
		return SeverityCritical
	}
	if composite >= thresholds[0] {
		return SeverityWarning
	}
	return SeverityNormal
}

// ComputeTrend returns "rising", "falling", or "stable" based on composite delta.
func ComputeTrend(compositeNow, compositePrev float64) string {
	delta := compositeNow - compositePrev
	if delta > 2 {
		return "rising"
	}
	if delta < -2 {
		return "falling"
	}
	return "stable"
}
