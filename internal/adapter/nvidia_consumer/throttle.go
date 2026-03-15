//go:build linux || windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package nvidia_consumer

// NVIDIA throttle reason bitmask constants (clocks_throttle_reasons.active).
const (
	nvThrottleNone         = 0x0000000000000000
	nvThrottleGPUIdle      = 0x0000000000000001
	nvThrottleAppClocks    = 0x0000000000000002
	nvThrottleSwPowerCap   = 0x0000000000000004
	nvThrottleHwSlowdown   = 0x0000000000000008 // thermal or power
	nvThrottleSyncBoost    = 0x0000000000000010
	nvThrottleSwThermal    = 0x0000000000000020
	nvThrottleHwThermal    = 0x0000000000000040
	nvThrottleHwPowerBrake = 0x0000000000000080
	nvThrottleDisplayClk   = 0x0000000000000100
)

// MapThrottleReason maps NVIDIA throttle bitmask to platform categories.
// Returns (active, reason). Thermal check precedes power; GPU idle / none = not active.
func MapThrottleReason(bitmask uint64) (active bool, reason string) {
	if bitmask == nvThrottleNone {
		return false, "none"
	}
	if (bitmask & nvThrottleGPUIdle) != 0 {
		return false, "none"
	}
	if bitmask&(nvThrottleHwThermal|nvThrottleSwThermal|nvThrottleHwSlowdown) != 0 {
		return true, "thermal"
	}
	if bitmask&(nvThrottleSwPowerCap|nvThrottleHwPowerBrake) != 0 {
		return true, "power"
	}
	return true, "other"
}
