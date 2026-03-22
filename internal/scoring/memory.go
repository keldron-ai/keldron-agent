// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import "github.com/keldron-ai/keldron-agent/registry"

// ComputeMemory computes the memory pressure sub-score (0-100).
// Apple Silicon SoCs (soc_integrated) use a gentler curve because macOS uses
// most RAM for cache; discrete-GPU systems use a steeper curve.
func ComputeMemory(memoryUsedPct float64, spec registry.GPUSpec) float64 {
	if spec.BehaviorClass == "soc_integrated" {
		return computeMemoryAppleSilicon(memoryUsedPct)
	}
	return computeMemoryDefault(memoryUsedPct)
}

// Default (discrete GPU / non-unified): piecewise linear 70/85/95 breakpoints.
func computeMemoryDefault(memoryUsedPct float64) float64 {
	switch {
	case memoryUsedPct < 70:
		return 0
	case memoryUsedPct < 85:
		return ((memoryUsedPct - 70) / 15) * 30
	case memoryUsedPct < 95:
		return 30 + ((memoryUsedPct-85)/10)*40
	default:
		pct := memoryUsedPct
		if pct > 100 {
			pct = 100
		}
		return 70 + ((pct-95)/5)*30
	}
}

// Apple Silicon unified memory: 0 below 80%; 80–95% → 0–40; 95–100% → 40–100.
func computeMemoryAppleSilicon(memoryUsedPct float64) float64 {
	switch {
	case memoryUsedPct < 80:
		return 0
	case memoryUsedPct < 95:
		return ((memoryUsedPct - 80) / 15) * 40
	default:
		pct := memoryUsedPct
		if pct > 100 {
			pct = 100
		}
		return 40 + ((pct-95)/5)*60
	}
}
