// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"math"

	"github.com/keldron-ai/keldron-agent/registry"
)

// ComputePower computes the power sub-score (0-100) from actual power vs TDP.
func ComputePower(pActual float64, spec registry.GPUSpec) float64 {
	if spec.TDPW <= 0 {
		return 0
	}
	return math.Min(100, (pActual/spec.TDPW)*100)
}
