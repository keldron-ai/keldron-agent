// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

// ComputeMemory computes the memory pressure sub-score (0-100).
// On Apple Silicon, GPU and CPU share unified memory — high memory usage
// directly degrades GPU throughput even before the OS reports memory pressure.
//
// Piecewise linear:
//
//	0-70%  usage → score 0       (plenty of headroom)
//	70-85% usage → score 0-30    (moderate, linear ramp)
//	85-95% usage → score 30-70   (high, steeper ramp)
//	95-100% usage → score 70-100 (critical, steep ramp)
//
// This reflects that memory degradation is nonlinear — the last 5%
// (95-100%) is dramatically worse than the 70-85% range because the OS
// is actively compressing and swapping.
func ComputeMemory(memoryUsedPct float64) float64 {
	switch {
	case memoryUsedPct < 70:
		return 0
	case memoryUsedPct < 85:
		// Linear: 70% → 0, 85% → 30
		return ((memoryUsedPct - 70) / 15) * 30
	case memoryUsedPct < 95:
		// Steeper: 85% → 30, 95% → 70
		return 30 + ((memoryUsedPct-85)/10)*40
	default:
		// Critical: 95% → 70, 100% → 100
		pct := memoryUsedPct
		if pct > 100 {
			pct = 100
		}
		return 70 + ((pct-95)/5)*30
	}
}
