// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import (
	"sync"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

// TelemetryPoint is the flattened history point for the /api/v1/history endpoint.
type TelemetryPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	TemperatureC   float64   `json:"temperature_c"`
	GPUUtilPct     float64   `json:"gpu_utilization_pct"`
	PowerDrawW     float64   `json:"power_draw_w"`
	MemoryUsedPct  float64   `json:"memory_used_pct"`
	CompositeScore float64   `json:"composite_score"`
	Severity       string    `json:"severity"`
}

// HistoryBuffer holds a ring buffer of telemetry points for backfill.
type HistoryBuffer struct {
	mu     sync.RWMutex
	points []TelemetryPoint
	max    int
}

// NewHistoryBuffer creates a new buffer with capacity for maxPoints.
func NewHistoryBuffer(maxPoints int) *HistoryBuffer {
	return &HistoryBuffer{
		points: make([]TelemetryPoint, 0, maxPoints),
		max:    maxPoints,
	}
}

// Add appends a point, dropping the oldest when full.
func (h *HistoryBuffer) Add(p TelemetryPoint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.points) >= h.max {
		copy(h.points, h.points[1:])
		h.points = h.points[:len(h.points)-1]
	}
	h.points = append(h.points, p)
}

// Points returns all points with timestamp >= since.
func (h *HistoryBuffer) Points(since time.Time) []TelemetryPoint {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]TelemetryPoint, 0)
	for _, p := range h.points {
		if !p.Timestamp.Before(since) {
			result = append(result, p)
		}
	}
	return result
}

// buildHistoryPoint creates a history TelemetryPoint from batch and scores.
func buildHistoryPoint(batch []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) TelemetryPoint {
	if len(batch) == 0 {
		return TelemetryPoint{}
	}
	pt := latestPoint(batch)
	m := pt.Metrics
	if m == nil {
		m = make(map[string]float64)
	}
	memUsed := getMetricFloat(m, "mem_used_bytes")
	memTotal := getMetricFloat(m, "mem_total_bytes")
	memPct := 0.0
	if memTotal > 0 {
		memPct = memUsed / memTotal * 100
	}
	severity := "normal"
	composite := 0.0
	if sc, ok := matchScore(pt, scores); ok {
		composite = sc.Composite
		if sc.Severity != "" {
			severity = sc.Severity
		}
	}
	return TelemetryPoint{
		Timestamp:      pt.Timestamp.UTC(),
		TemperatureC:   getMetricFloat(m, "temperature_c", "temperature_junction_c", "temperature_edge"),
		GPUUtilPct:     getMetricFloat(m, "gpu_utilization_pct"),
		PowerDrawW:     getMetricFloat(m, "power_usage_w"),
		MemoryUsedPct:  memPct,
		CompositeScore: composite,
		Severity:       severity,
	}
}
