// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package dcgm

import (
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
)

// Metric key constants for DCGM GPU metrics.
const (
	MetricGPUID          = "gpu_id"
	MetricGPUName        = "gpu_name"
	MetricTemperature    = "temperature_c"
	MetricPowerUsage     = "power_usage_w"
	MetricGPUUtilization = "gpu_utilization_pct"
	MetricMemUtilization = "mem_utilization_pct"
	MetricMemUsed        = "mem_used_bytes"
	MetricMemTotal       = "mem_total_bytes"
	MetricSMClock        = "sm_clock_mhz"
	MetricMemClock       = "mem_clock_mhz"
	MetricThrottled      = "throttled"
)

// GPUMetrics holds a snapshot of metrics for a single GPU.
type GPUMetrics struct {
	GPUID          int
	GPUName        string
	Temperature    float64 // Celsius
	PowerUsage     float64 // Watts
	GPUUtilization float64 // 0–100
	MemUtilization float64 // 0–100
	MemUsed        uint64  // Bytes
	MemTotal       uint64  // Bytes
	SMClock        uint32  // MHz
	MemClock       uint32  // MHz
	Throttled      bool
}

// ToRawReading converts GPUMetrics into an adapter.RawReading.
func (m *GPUMetrics) ToRawReading(source string) adapter.RawReading {
	return adapter.RawReading{
		AdapterName: "dcgm",
		Source:      source,
		Timestamp:   time.Now(),
		Metrics: map[string]interface{}{
			MetricGPUID:          m.GPUID,
			MetricGPUName:        m.GPUName,
			MetricTemperature:    m.Temperature,
			MetricPowerUsage:     m.PowerUsage,
			MetricGPUUtilization: m.GPUUtilization,
			MetricMemUtilization: m.MemUtilization,
			MetricMemUsed:        m.MemUsed,
			MetricMemTotal:       m.MemTotal,
			MetricSMClock:        m.SMClock,
			MetricMemClock:       m.MemClock,
			MetricThrottled:      m.Throttled,
		},
	}
}
