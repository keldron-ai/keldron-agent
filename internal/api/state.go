// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/health"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

// StateHolder holds the latest telemetry batch, risk scores, and health for the API.
// The output bridge calls Update after each flush; API handlers and WebSocket
// broadcast read from Get.
type StateHolder struct {
	mu     sync.RWMutex
	batch  []normalizer.TelemetryPoint
	scores []scoring.RiskScoreOutput
	health map[string]*health.DeviceHealthSnapshot
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

// Update stores the latest batch, scores, and health, and broadcasts to WebSocket clients.
func (h *StateHolder) Update(batch []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput, healthSnapshots map[string]*health.DeviceHealthSnapshot) {
	bCopy := make([]normalizer.TelemetryPoint, len(batch))
	copy(bCopy, batch)
	sCopy := make([]scoring.RiskScoreOutput, len(scores))
	copy(sCopy, scores)
	healthCopy := make(map[string]*health.DeviceHealthSnapshot)
	if healthSnapshots != nil {
		for k, v := range healthSnapshots {
			healthCopy[k] = v
		}
	}

	h.mu.Lock()
	h.batch = bCopy
	h.scores = sCopy
	h.health = healthCopy
	hub := h.hub
	h.mu.Unlock()

	if hub != nil && len(batch) > 0 {
		msg := buildTelemetryUpdate(batch, scores, healthSnapshots)
		if data, err := json.Marshal(msg); err == nil {
			hub.broadcast(data)
		}
	}
}

// Get returns a copy of the latest batch, scores, and health.
func (h *StateHolder) Get() ([]normalizer.TelemetryPoint, []scoring.RiskScoreOutput, map[string]*health.DeviceHealthSnapshot) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	batch := append([]normalizer.TelemetryPoint(nil), h.batch...)
	scores := append([]scoring.RiskScoreOutput(nil), h.scores...)
	healthCopy := make(map[string]*health.DeviceHealthSnapshot, len(h.health))
	for k, v := range h.health {
		healthCopy[k] = v
	}
	return batch, scores, healthCopy
}

// buildTelemetryUpdate creates a TelemetryUpdate message from batch, scores, and health.
func buildTelemetryUpdate(batch []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput, healthSnapshots map[string]*health.DeviceHealthSnapshot) TelemetryUpdate {
	var ts string
	var telemetry TelemetryShort
	var risk RiskShort
	var healthSummary *health.HealthSummary

	if len(batch) > 0 {
		pt := latestPoint(batch)
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
		if sc, ok := matchScore(pt, scores); ok {
			risk = RiskShort{
				CompositeScore: sc.Composite,
				Severity:       sc.Severity,
				Trend:          sc.Trend,
			}
		}
		if healthSnapshots != nil {
			did := deviceIDFromPoint(pt)
			if snap := healthSnapshots[did]; snap != nil {
				healthSummary = snap.ToHealthSummary()
			}
		}
	}

	return TelemetryUpdate{
		Type:      "telemetry_update",
		Timestamp: ts,
		Telemetry: telemetry,
		Risk:      risk,
		Health:    healthSummary,
	}
}

func getMetricFloat(m map[string]float64, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
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
