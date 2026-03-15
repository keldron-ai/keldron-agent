//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package apple_silicon

/*
#include <notify.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const thermalPressureNotifyName = "com.apple.system.thermalpressurelevel"

// Thermal pressure states from Darwin notify (OSThermalNotification.h).
// 0=nominal, 1=moderate, 2=heavy, 3=trapping, 4=sleeping.
// We map to spec strings: nominal, fair, serious, critical.
var thermalStateNames = map[uint64]string{
	0: "nominal",
	1: "fair",     // moderate
	2: "serious",  // heavy
	3: "critical", // trapping
	4: "critical", // sleeping
}

// ReadThermalPressure returns the current thermal pressure state.
// Uses notify_get_state on com.apple.system.thermalpressurelevel (no sudo).
// Returns: "nominal", "fair", "serious", "critical", or "" on error.
func ReadThermalPressure() (string, error) {
	cname := C.CString(thermalPressureNotifyName)
	defer C.free(unsafe.Pointer(cname))

	var token C.int
	ret := C.notify_register_check(cname, &token)
	if ret != C.NOTIFY_STATUS_OK {
		return "", fmt.Errorf("notify_register_check: %d", ret)
	}
	defer C.notify_cancel(token)

	var state C.uint64_t
	ret = C.notify_get_state(token, &state)
	if ret != C.NOTIFY_STATUS_OK {
		return "", fmt.Errorf("notify_get_state: %d", ret)
	}

	if name, ok := thermalStateNames[uint64(state)]; ok {
		return name, nil
	}
	return "unknown", nil
}

// IsThrottled returns true if thermal pressure is above nominal.
func IsThrottled(state string) bool {
	return state != "" && state != "nominal"
}
