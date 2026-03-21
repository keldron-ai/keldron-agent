// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package telemetry

import (
	"testing"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func TestDeviceIDFromPoint(t *testing.T) {
	tests := []struct {
		name string
		pt   normalizer.TelemetryPoint
		want string
	}{
		{
			name: "with gpu_id",
			pt: normalizer.TelemetryPoint{
				Source:  "host1",
				Metrics: map[string]float64{"gpu_id": 2},
			},
			want: "host1:2",
		},
		{
			name: "without gpu_id",
			pt: normalizer.TelemetryPoint{
				Source:  "host1",
				Metrics: map[string]float64{"temperature_c": 42},
			},
			want: "host1",
		},
		{
			name: "nil metrics",
			pt: normalizer.TelemetryPoint{
				Source: "host1",
			},
			want: "host1",
		},
		{
			name: "gpu_id zero",
			pt: normalizer.TelemetryPoint{
				Source:  "host1",
				Metrics: map[string]float64{"gpu_id": 0},
			},
			want: "host1:0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeviceIDFromPoint(tt.pt)
			if got != tt.want {
				t.Errorf("DeviceIDFromPoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
