// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"math"

	"github.com/keldron-ai/keldron-agent/registry"
)

// ComputeTimeToHotspot estimates minutes until temperature reaches thermal limit
// using 5-minute (10-sample at 30s poll) linear regression. Returns nil if the
// buffer is not yet full or if temperature is stable/cooling (slope <= 0.1 °C/min).
func ComputeTimeToHotspot(thermalBuffer *RingBuffer, tCurrent float64, spec registry.GPUSpec) *float64 {
	if thermalBuffer.Len() < 10 {
		return nil
	}

	values := thermalBuffer.Values()
	n := len(values)

	// Least-squares linear regression
	// x = index (proxy for time in 30s intervals)
	// y = temperature
	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	nf := float64(n)
	denom := nf*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-10 {
		return nil
	}

	slope := (nf*sumXY - sumX*sumY) / denom // °C per 30s interval
	slopePerMin := slope * 2                // convert to °C per minute

	if slopePerMin <= 0.1 {
		return nil // stable or cooling
	}

	minutesToHotspot := (spec.ThermalLimitC - tCurrent) / slopePerMin
	if minutesToHotspot <= 0 {
		zero := 0.0
		return &zero
	}
	return &minutesToHotspot
}
