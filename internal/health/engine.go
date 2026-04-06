// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"sort"
	"sync"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/telemetry"
	"github.com/keldron-ai/keldron-agent/registry"
)

// Engine computes device health metrics from telemetry.
type Engine struct {
	mu      sync.RWMutex
	devices map[string]*deviceState
}

// deviceState holds per-device rolling series and throttle limit.
type deviceState struct {
	series        *rollingSeries
	thermalLimitC float64
}

// NewEngine creates a new health engine.
func NewEngine() *Engine {
	return &Engine{
		devices: make(map[string]*deviceState),
	}
}

func lookupThermalLimit(pt normalizer.TelemetryPoint) float64 {
	model := deviceModelFromPoint(pt)
	spec := registry.Lookup(registry.NormalizeModelName(model))
	if pt.Tags != nil {
		if v, ok := pt.Tags["behavior_class"]; ok && v != "" {
			spec.BehaviorClass = v
		}
	}
	return spec.ThermalLimitC
}

// Update processes a batch of telemetry points and returns health snapshots per device.
// Call from the output bridge on each flush. Returns a map of deviceID -> snapshot.
func (e *Engine) Update(batch []normalizer.TelemetryPoint) map[string]*DeviceHealthSnapshot {
	if len(batch) == 0 {
		return nil
	}

	sorted := make([]normalizer.TelemetryPoint, len(batch))
	copy(sorted, batch)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, pt := range sorted {
		deviceID := telemetry.DeviceIDFromPoint(pt)
		state := e.getOrCreateState(deviceID, pt)

		m := pt.Metrics
		if m == nil {
			m = make(map[string]float64)
		}

		tempC, tempOK := getMetricFloatOK(m, "temperature_c", "temperature_junction_c", "temperature_edge")
		utilPct := getMetricFloat(m, "gpu_utilization_pct")
		powerW := getMetricFloat(m, "power_usage_w")

		at := pt.Timestamp

		state.series.append(healthSample{
			at:           at,
			tempC:        tempC,
			tempCPresent: tempOK,
			utilPct:      utilPct,
			powerW:       powerW,
		})

		rt := recoveryTargetC(effectiveThrottleLimit(state.thermalLimitC))
		state.series.updateSpikeSegmentStart(rt)
	}

	snapshots := make(map[string]*DeviceHealthSnapshot)
	for deviceID, state := range e.devices {
		snapshots[deviceID] = e.buildSnapshot(state)
	}
	return snapshots
}

func (e *Engine) getOrCreateState(deviceID string, pt normalizer.TelemetryPoint) *deviceState {
	if s, ok := e.devices[deviceID]; ok {
		// Only overwrite thermalLimitC when the new lookup returns a
		// model-derived (non-default) value, so later points without
		// model tags don't clobber a previously resolved limit.
		newLimit := lookupThermalLimit(pt)
		if newLimit != registry.DefaultThermalLimitC {
			s.thermalLimitC = newLimit
		}
		return s
	}
	s := &deviceState{
		series:        newRollingSeries(),
		thermalLimitC: lookupThermalLimit(pt),
	}
	e.devices[deviceID] = s
	return s
}

func (e *Engine) buildSnapshot(state *deviceState) *DeviceHealthSnapshot {
	now := time.Now()
	samples := state.series.snapshot()
	wu := isWarmingUp(state.series.firstSeen(), now)

	spikeStart := state.series.spikeStart()

	tdr := computeHeadroom(samples, state.thermalLimitC, wu)
	tre := computeThermalRecovery(samples, state.thermalLimitC, spikeStart, now, wu)
	ppw := computePerfPerWatt(samples)
	stab := computeStability(samples, wu)

	return &DeviceHealthSnapshot{
		WarmingUp:           wu,
		ThermalDynamicRange: tdr,
		ThermalRecovery:     tre,
		PerfPerWatt:         ppw,
		ThermalStability:    stab,
	}
}

// Snapshot returns the full health snapshot for a device. Call after Update.
// Returns nil if the device has no data.
func (e *Engine) Snapshot(deviceID string) *DeviceHealthSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state, ok := e.devices[deviceID]
	if !ok {
		return nil
	}
	return e.buildSnapshot(state)
}

// SnapshotForWS returns the lightweight health summary for WebSocket.
func (e *Engine) SnapshotForWS(deviceID string) *HealthSummary {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state, ok := e.devices[deviceID]
	if !ok {
		return nil
	}
	return e.buildSnapshot(state).ToHealthSummary()
}

func getMetricFloat(m map[string]float64, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return 0
}

func getMetricFloatOK(m map[string]float64, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return 0, false
}
