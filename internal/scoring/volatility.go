// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"math"

	"github.com/keldron-ai/keldron-agent/registry"
)

// ComputeVolatility computes the volatility sub-score (0-100) from temperature-only
// coefficient of variation, using behavior-class cv_max. warmingUp is true if
// insufficient data (Len < 10 for score, Len < 60 for full volatility window).
func ComputeVolatility(volBuffer *RingBuffer, spec registry.GPUSpec) (score float64, warmingUp bool) {
	if volBuffer.Len() < 10 {
		return 0, true
	}
	meanT := volBuffer.Mean()
	if meanT < 1.0 {
		return 0, false
	}
	stdevT := volBuffer.Stdev()
	cvTemp := stdevT / meanT
	if spec.CVMax <= 0 {
		return 0, volBuffer.Len() < 60
	}
	return math.Min(100, (cvTemp/spec.CVMax)*100), volBuffer.Len() < 60
}
