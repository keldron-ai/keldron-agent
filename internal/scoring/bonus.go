// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

// ComputeMemoryPressure returns memUsed/memTotal (0.0-1.0).
func ComputeMemoryPressure(memUsed, memTotal float64) float64 {
	if memTotal <= 0 {
		return 0
	}
	return memUsed / memTotal
}

// ComputeClockEfficiency returns clockActual/clockMax (0.0-1.0).
func ComputeClockEfficiency(clockActual, clockMax float64) float64 {
	if clockMax <= 0 {
		return 1.0
	}
	return clockActual / clockMax
}

// ComputePowerCost returns hourly, daily, monthly cost in $ given power (W) and electricity rate ($/kWh).
func ComputePowerCost(powerW, electricityRate float64) (hourly, daily, monthly float64) {
	hourly = (powerW / 1000) * electricityRate
	daily = hourly * 24
	monthly = daily * 30
	return
}

// ComputeHotspotDelta returns hotspot - core temp. Returns -1 if either sensor unavailable.
func ComputeHotspotDelta(tHotspot, tCore float64) float64 {
	if tHotspot < 0 || tCore < 0 {
		return -1
	}
	return tHotspot - tCore
}

// ComputeSwapPressure returns swapUsed/swapTotal (0.0-1.0).
func ComputeSwapPressure(swapUsed, swapTotal int64) float64 {
	if swapTotal <= 0 {
		return 0
	}
	return float64(swapUsed) / float64(swapTotal)
}
