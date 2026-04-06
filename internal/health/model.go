// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"strconv"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

// deviceModelFromPoint extracts a GPU/device model string for registry lookup.
// Mirrors internal/scoring/engine.go deviceModelFromPoint.
func deviceModelFromPoint(pt normalizer.TelemetryPoint) string {
	if pt.Tags != nil {
		for _, k := range []string{"device_model", "gpu_model", "gpu_name", "model"} {
			if v, ok := pt.Tags[k]; ok && v != "" {
				return v
			}
		}
	}
	if pt.Metrics != nil {
		for _, k := range []string{"gpu_name", "model", "device_model"} {
			if v, ok := pt.Metrics[k]; ok {
				return strconv.FormatFloat(v, 'f', -1, 64)
			}
		}
	}
	return "unknown"
}
