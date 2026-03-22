// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"math"

	"github.com/keldron-ai/keldron-agent/registry"
)

// ComputeThermal computes the thermal sub-score (0-100) with a quadratic ramp
// above 70% of thermal limit, stepped RoC penalty, and Apple Silicon thermal_pressure floor.
func ComputeThermal(tCurrent float64, thermalBuffer *RingBuffer, spec registry.GPUSpec, thermalPressureState string) (score float64, rocPenalty float64, warmingUp bool) {
	// Apple Silicon thermal_pressure provides a floor, not an override.
	// We compute both the Apple floor and the piecewise linear score,
	// then take whichever is higher. This way the piecewise formula
	// kicks in as temp approaches the throttle threshold, AND Apple's
	// native "serious"/"critical" states can push the score even higher.
	var appleFloor float64
	if spec.BehaviorClass == "soc_integrated" &&
		spec.ThermalPressureStateSupported &&
		thermalPressureState != "" {
		overrides := map[string]float64{
			"nominal":  0,
			"fair":     40,
			"serious":  70,
			"critical": 95,
		}
		if v, ok := overrides[thermalPressureState]; ok {
			appleFloor = v
		}
	}

	// Piecewise linear based on actual temperature
	if spec.ThermalLimitC <= 0 {
		return math.Max(appleFloor, 0), 0, true
	}

	tRatio := tCurrent / spec.ThermalLimitC

	var tScore float64
	if tRatio < 0.70 {
		tScore = 0
	} else {
		normalized := (tRatio - 0.70) / 0.30
		tScore = normalized * normalized * 100
	}

	// Stepped RoC penalty
	oldest, hasOldest := thermalBuffer.Oldest()
	if !hasOldest || !thermalBuffer.IsFull() {
		return math.Min(100, math.Max(tScore, appleFloor)), 0, true // warming_up
	}

	roc := (tCurrent - oldest) / 5.0 // °C per minute (5-min buffer)

	var rocP float64
	if roc > 3.0 {
		rocP = 25 // emergency
	} else if roc > 1.0 {
		rocP = 10 // rapid heating but expected under sudden load
	} else if roc > 0.5 {
		rocP = (roc - 0.5) * 10 // moderate
	}

	finalScore := math.Min(100, math.Max(tScore+rocP, appleFloor))
	return finalScore, rocP, false
}
