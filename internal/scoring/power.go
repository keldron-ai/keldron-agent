// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"math"

	"github.com/keldron-ai/keldron-agent/registry"
)

// ComputePower computes the power sub-score (0-100) from actual power vs TDP.
// Below 70% of TDP is treated as no concern; 70–100% TDP maps to 0–50; above TDP ramps to 100.
func ComputePower(pActual float64, spec registry.GPUSpec) float64 {
	if spec.TDPW <= 0 {
		return 0
	}
	ratio := pActual / spec.TDPW
	if ratio < 0.70 {
		return 0
	}
	if ratio <= 1.0 {
		return ((ratio - 0.70) / 0.30) * 50
	}
	return math.Min(100, 50+((ratio-1.0)/0.30)*50)
}
