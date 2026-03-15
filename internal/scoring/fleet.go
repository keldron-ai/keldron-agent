// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

// ComputeFleetPenalty returns an additive penalty of 10 if more than 30%
// of peers have composite score > 70 (stressed). Otherwise returns 0.
func ComputeFleetPenalty(peerComposites []float64) float64 {
	if len(peerComposites) == 0 {
		return 0
	}
	stressed := 0
	for _, c := range peerComposites {
		if c > 70 {
			stressed++
		}
	}
	if float64(stressed)/float64(len(peerComposites)) > 0.30 {
		return 10
	}
	return 0
}
