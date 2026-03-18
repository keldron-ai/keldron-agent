// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

// StateHolder holds the latest telemetry batch and risk scores for the API.
// The output bridge calls Update after each flush; API handlers and WebSocket
// broadcast read from Get.
type StateHolder struct {
	mu     sync.RWMutex
	batch  []normalizer.TelemetryPoint
	scores []scoring.RiskScoreOutput
	hub    *wsHub
}

// NewStateHolder creates a new StateHolder.
func NewStateHolder() *StateHolder {
	return &StateHolder{}
}

// SetBroadcastTarget sets the WebSocket hub to broadcast to on Update.
// Call this after creating the server so Update triggers broadcasts.
func (h *StateHolder) SetBroadcastTarget(hub *wsHub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hub = hub
}

// Update stores the latest batch and scores, and broadcasts to WebSocket clients.
func (h *StateHolder) Update(batch []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) {
	h.mu.Lock()
	h.batch = batch
	h.scores = scores
	hub := h.hub
	h.mu.Unlock()

	if hub != nil && len(batch) > 0 {
		msg := buildTelemetryUpdate(batch, scores)
		if data, err := json.Marshal(msg); err == nil {
			hub.broadcast(data)
		}
	}
}

// Get returns a copy of the latest batch and scores.
func (h *StateHolder) Get() ([]normalizer.TelemetryPoint, []scoring.RiskScoreOutput) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	batch := append([]normalizer.TelemetryPoint(nil), h.batch...)
	scores := append([]scoring.RiskScoreOutput(nil), h.scores...)
	return batch, scores
}

// buildTelemetryUpdate creates a TelemetryUpdate message from batch and scores.
func buildTelemetryUpdate(batch []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) TelemetryUpdate {
	var ts string
	var telemetry TelemetryShort
	var risk RiskShort

	if len(batch) > 0 {
		pt := batch[0]
		ts = pt.Timestamp.UTC().Format(time.RFC3339)
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
		telemetry = TelemetryShort{
			TemperatureC:      getMetricFloat(m, "temperature_c", "temperature_junction_c", "temperature_edge"),
			GPUUtilizationPct: getMetricFloat(m, "gpu_utilization_pct"),
			PowerDrawW:        getMetricFloat(m, "power_usage_w"),
			MemoryUsedPct:     memPct,
			ThermalState:      getTag(pt, "thermal_pressure_state"),
			ThrottleActive:    getMetricFloat(m, "throttled") > 0,
		}
		if telemetry.ThermalState == "" {
			telemetry.ThermalState = "nominal"
		}
	}

	if len(scores) > 0 {
		s := scores[0]
		risk = RiskShort{
			CompositeScore: s.Composite,
			Severity:       s.Severity,
			Trend:          s.Trend,
		}
	}

	return TelemetryUpdate{
		Type:      "telemetry_update",
		Timestamp: ts,
		Telemetry: telemetry,
		Risk:      risk,
	}
}

func getMetricFloat(m map[string]float64, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok && v >= 0 {
			return v
		}
	}
	return 0
}

func getTag(pt normalizer.TelemetryPoint, key string) string {
	if pt.Tags != nil {
		if v, ok := pt.Tags[key]; ok {
			return v
		}
	}
	return ""
}

func hasMetric(m map[string]float64, key string) bool {
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}
