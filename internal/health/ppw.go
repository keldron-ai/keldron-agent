// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

// ComputePerfPerWatt computes the ratio of GPU utilization to power consumption.
// Returns Available: false when power data is unavailable or power < 1W.
func ComputePerfPerWatt(gpuUtilPct, powerW float64, powerAvailable bool) *PerfPerWattResult {
	if !powerAvailable || powerW < 1.0 {
		return &PerfPerWattResult{Available: false}
	}
	return &PerfPerWattResult{
		Available: true,
		Value:     gpuUtilPct / powerW,
		Unit:      "pct_util_per_watt",
	}
}
