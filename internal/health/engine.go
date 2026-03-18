// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"sort"
	"strconv"
	"sync"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

// Engine computes device health metrics from telemetry.
type Engine struct {
	mu      sync.RWMutex
	devices map[string]*deviceState
}

// deviceState holds per-device trackers.
type deviceState struct {
	classifier  *Classifier
	tdr         *TDRState
	tre         *TRETracker
	stability   *StabilityTracker
	lastUtilPct float64
	lastPowerW  float64
}

// NewEngine creates a new health engine.
func NewEngine() *Engine {
	return &Engine{
		devices: make(map[string]*deviceState),
	}
}

// Update processes a batch of telemetry points and returns health snapshots per device.
// Call from the output bridge on each flush. Returns a map of deviceID -> snapshot.
func (e *Engine) Update(batch []normalizer.TelemetryPoint) map[string]*DeviceHealthSnapshot {
	if len(batch) == 0 {
		return nil
	}

	// Sort by timestamp for correct chronological processing
	sorted := make([]normalizer.TelemetryPoint, len(batch))
	copy(sorted, batch)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, pt := range sorted {
		deviceID := deviceIDFromPoint(pt)
		state := e.getOrCreateState(deviceID)

		m := pt.Metrics
		if m == nil {
			m = make(map[string]float64)
		}

		tempC := getMetricFloat(m, "temperature_c", "temperature_junction_c", "temperature_edge")
		utilPct := getMetricFloat(m, "gpu_utilization_pct")
		powerW := getMetricFloat(m, "power_usage_w")

		at := pt.Timestamp

		// Classifier: add then classify
		state.classifier.Add(utilPct, at)
		workloadState := state.classifier.Classify(utilPct, at)

		// TDR: feed idle/peak samples
		if workloadState == StateIdle {
			state.tdr.AddIdle(tempC, at)
		} else if workloadState == StatePeak {
			state.tdr.AddPeak(tempC, at)
		}

		// TRE: needs workload state and temp
		state.tre.Update(workloadState, tempC, at)

		// Stability: needs workload state and temp
		state.stability.Update(workloadState, tempC, at)

		// Store latest for PPW (instantaneous)
		state.lastUtilPct = utilPct
		state.lastPowerW = powerW
	}

	// Build snapshots for all devices we touched
	snapshots := make(map[string]*DeviceHealthSnapshot)
	for deviceID, state := range e.devices {
		snapshots[deviceID] = e.buildSnapshot(state)
	}
	return snapshots
}

func (e *Engine) getOrCreateState(deviceID string) *deviceState {
	if s, ok := e.devices[deviceID]; ok {
		return s
	}
	tdr := NewTDRState()
	s := &deviceState{
		classifier: NewClassifier(),
		tdr:        tdr,
		tre:        NewTRETracker(tdr),
		stability:  NewStabilityTracker(),
	}
	e.devices[deviceID] = s
	return s
}

func (e *Engine) buildSnapshot(state *deviceState) *DeviceHealthSnapshot {
	return &DeviceHealthSnapshot{
		ThermalDynamicRange: state.tdr.Compute(),
		ThermalRecovery:     state.tre.Result(),
		PerfPerWatt:         ComputePerfPerWatt(state.lastUtilPct, state.lastPowerW, state.lastPowerW >= 1.0),
		ThermalStability:    state.stability.Result(),
	}
}

// Snapshot returns the full health snapshot for a device. Call after Update.
// Returns nil if the device has no data.
func (e *Engine) Snapshot(deviceID string) *DeviceHealthSnapshot {
	e.mu.RLock()
	state, ok := e.devices[deviceID]
	e.mu.RUnlock()

	if !ok {
		return nil
	}
	return e.buildSnapshot(state)
}

// SnapshotForWS returns the lightweight health summary for WebSocket.
// TRE is not included (event-driven, fetched via status endpoint).
func (e *Engine) SnapshotForWS(deviceID string) *HealthSummary {
	e.mu.RLock()
	state, ok := e.devices[deviceID]
	e.mu.RUnlock()

	if !ok {
		return nil
	}

	summary := &HealthSummary{}

	tdr := state.tdr.Compute()
	if tdr != nil && tdr.Available {
		summary.TDRCelsius = &tdr.TDRCelsius
		summary.TDRRating = tdr.Rating
	}

	stability := state.stability.Result()
	if stability != nil && stability.Available {
		summary.StabilityCelsius = &stability.StabilityCelsius
	}

	ppw := ComputePerfPerWatt(state.lastUtilPct, state.lastPowerW, state.lastPowerW >= 1.0)
	if ppw != nil && ppw.Available {
		summary.PerfPerWatt = &ppw.Value
	}

	return summary
}

func deviceIDFromPoint(pt normalizer.TelemetryPoint) string {
	if pt.Metrics != nil {
		if gpuID, ok := pt.Metrics["gpu_id"]; ok {
			return pt.Source + ":" + strconv.FormatFloat(gpuID, 'f', 0, 64)
		}
	}
	return pt.Source
}

func getMetricFloat(m map[string]float64, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return 0
}
