// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"math"

	"github.com/keldron-ai/keldron-agent/registry"
)

// ComputePower computes the power sub-score (0-100) from actual power vs TDP.
// Below 70% of TDP is treated as no concern; 70–100% TDP maps to 0–50; above TDP ramps to 100.
//
// When utilization is high but power score is low, the power sensor is likely
// underreporting (common on Apple Silicon where only partial SoC power is captured).
// A utilization-informed floor prevents the score from reading 0 during obvious heavy load.
func ComputePower(pActual float64, utilization float64, spec registry.GPUSpec) float64 {
	if spec.TDPW <= 0 {
		return 0
	}
	ratio := pActual / spec.TDPW
	var powerScore float64
	if ratio < 0.70 {
		powerScore = 0
	} else if ratio <= 1.0 {
		powerScore = ((ratio - 0.70) / 0.30) * 50
	} else {
		powerScore = math.Min(100, 50+((ratio-1.0)/0.30)*50)
	}

	// Utilization-informed floor: if GPU utilization is high but power score
	// is suspiciously low, use utilization as a secondary signal.
	// At 100% util → floor 30, at 80% util → floor 18, below 50% → no floor.
	if utilization > 50 {
		utilFloor := ((utilization - 50) / 50) * 30
		powerScore = math.Max(powerScore, utilFloor)
	}

	return powerScore
}
