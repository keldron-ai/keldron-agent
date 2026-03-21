// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package telemetry provides shared helpers used across internal packages.
package telemetry

import (
	"strconv"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

// DeviceIDFromPoint returns a stable device identifier for a TelemetryPoint.
// For GPU points with a "gpu_id" metric, it returns "source:gpuID"; otherwise just source.
func DeviceIDFromPoint(pt normalizer.TelemetryPoint) string {
	if pt.Metrics != nil {
		if gpuID, ok := pt.Metrics["gpu_id"]; ok {
			return pt.Source + ":" + strconv.FormatFloat(gpuID, 'f', 0, 64)
		}
	}
	return pt.Source
}
