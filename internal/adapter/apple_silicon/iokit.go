//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package apple_silicon

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -L/usr/lib -framework CoreFoundation -framework IOKit -framework Foundation -lIOReport
#include "smc.h"

typedef struct {
	double gpuPowerW;
	double cpuPowerW;
	double anePowerW;
	double systemPowerW;
	double gpuUtilization;
	float socTempC;
} IOKitMetrics;

int initIOKit(void);
IOKitMetrics sampleIOKitMetrics(int durationMs);
void cleanupIOKit(void);
*/
import "C"

import (
	"log/slog"
	"sync"
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

var (
	initOnce sync.Once
	initOk   bool
	mu       sync.Mutex
	cleaned  bool
)

// ReadIOKit reads GPU utilization, power, and SoC temperature via IOKit/IOReport.
// IOReport is a private framework; channel names vary by macOS version.
// Returns partial data with zeros for unavailable channels. Never panics.
func ReadIOKit(logger *slog.Logger) *IOKitReading {
	r := &IOKitReading{}

	initOnce.Do(func() {
		ret := C.initIOKit()
		initOk = (ret == 0)
		if !initOk {
			l := logger
			if l == nil {
				l = slog.Default()
			}
			l.Debug("IOKit init failed (IOReport is optional, a private framework whose channel names vary by macOS version); reporting zeros", "ret", int(ret))
		}
	})

	if !initOk {
		return r
	}

	pm := C.sampleIOKitMetrics(100)

	r.GPUPowerW = float64(pm.gpuPowerW)
	r.CPUPowerW = float64(pm.cpuPowerW)
	r.ANEPowerW = float64(pm.anePowerW)
	r.SystemPowerW = float64(pm.systemPowerW)
	if r.SystemPowerW == 0 {
		r.SystemPowerW = r.GPUPowerW + r.CPUPowerW + r.ANEPowerW
	}
	r.GPUUtilization = float64(pm.gpuUtilization)
	r.SoCTempC = float64(pm.socTempC)

	return r
}

// CleanupIOKit releases the IOReport subscription and SMC connection.
// Call from adapter Stop() for graceful shutdown. Safe to call multiple
// times; resets initOnce so a subsequent ReadIOKit can re-initialize.
func CleanupIOKit() {
	mu.Lock()
	defer mu.Unlock()
	if !initOk && !cleaned {
		return
	}
	C.cleanupIOKit()
	initOk = false
	initOnce = sync.Once{}
	cleaned = true
}
