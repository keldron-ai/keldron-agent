// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"math"

	"github.com/keldron-ai/keldron-agent/registry"
)

// ComputeThermal computes the thermal sub-score (0-100) with piecewise linear
// temperature ratio, stepped RoC penalty, and Apple Silicon thermal_pressure override.
func ComputeThermal(tCurrent float64, thermalBuffer *RingBuffer, spec registry.GPUSpec, thermalPressureState string) (score float64, rocPenalty float64, warmingUp bool) {
	// Apple Silicon override
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
			return v, 0, false
		}
	}

	// Piecewise linear
	if spec.ThermalLimitC <= 0 {
		return 0, 0, true
	}
	tRatio := tCurrent / spec.ThermalLimitC
	var tScore float64
	if tRatio < 0.60 {
		tScore = 0
	} else {
		tScore = ((tRatio - 0.60) / 0.40) * 100
	}

	// Stepped RoC penalty
	oldest, hasOldest := thermalBuffer.Oldest()
	if !hasOldest || !thermalBuffer.IsFull() {
		return math.Min(100, tScore), 0, true // warming_up
	}

	roc := (tCurrent - oldest) / 5.0 // °C per minute (5-min buffer)
	var rocP float64
	if roc > 3.0 {
		rocP = 40 // emergency
	} else if roc > 1.0 {
		rocP = 20 // serious
	} else if roc > 0.5 {
		rocP = (roc - 0.5) * 20 // moderate: linear 0-10
	}

	return math.Min(100, tScore+rocP), rocP, false
}
