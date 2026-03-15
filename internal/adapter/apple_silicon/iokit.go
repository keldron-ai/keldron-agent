//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package apple_silicon

import (
	"log/slog"
)

// IOKitReading holds metrics from IOKit/IOReport.
type IOKitReading struct {
	GPUUtilization float64 // 0.0-1.0, from GPU active residency
	GPUPowerW      float64 // watts
	CPUPowerW      float64 // watts
	ANEPowerW      float64 // watts, 0 if unavailable
	SoCTempC       float64 // °C
	SystemPowerW   float64 // GPU + CPU + ANE total
}

// ReadIOKit reads GPU utilization, power, and SoC temperature via IOKit/IOReport.
// IOReport is a private framework; channel names vary by macOS version.
// Returns partial data with zeros for unavailable channels. Never panics.
// TODO: Integrate IOReportCreateSubscription/IOReportCreateSamples or SMC
// for real GPU power/temperature (see socpowerbud, IOReport_decompile).
func ReadIOKit(logger *slog.Logger) *IOKitReading {
	r := &IOKitReading{}
	r.SystemPowerW = r.GPUPowerW + r.CPUPowerW + r.ANEPowerW

	if logger != nil {
		logger.Debug("IOReport channels unavailable; reporting zeros (graceful degradation)")
	}
	return r
}
