// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"strconv"
	"sync"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/telemetry"
	"github.com/keldron-ai/keldron-agent/registry"
)

const (
	thermalBufferCap    = 10 // 5 min at 30s poll
	volatilityBufferCap = 60 // 30 min at 30s poll
)

// ScoreEngine maintains per-device state and computes risk scores.
type ScoreEngine struct {
	mu              sync.Mutex
	states          map[string]*DeviceScoreState
	electricityRate float64
}

// NewScoreEngine creates a ScoreEngine with the given electricity rate ($/kWh).
func NewScoreEngine(electricityRate float64) *ScoreEngine {
	return &ScoreEngine{
		states:          make(map[string]*DeviceScoreState),
		electricityRate: electricityRate,
	}
}

// Score computes risk scores for all devices in the batch.
func (e *ScoreEngine) Score(batch []normalizer.TelemetryPoint) []RiskScoreOutput {
	if len(batch) == 0 {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Group by device_id, keeping last point per device
	byDevice := make(map[string]normalizer.TelemetryPoint)
	for _, pt := range batch {
		did := telemetry.DeviceIDFromPoint(pt)
		byDevice[did] = pt
	}

	// First pass: compute local scores for each device
	type localResult struct {
		deviceID   string
		thermal    float64
		power      float64
		volatility float64
		memory     float64
		rLocal     float64
		warmingUp  bool
		output     RiskScoreOutput
	}
	locals := make([]localResult, 0, len(byDevice))

	for did, pt := range byDevice {
		state := e.getOrCreateState(did, pt)
		m := pt.Metrics
		if m == nil {
			m = make(map[string]float64)
		}

		// Extract metrics
		tCurrent := getFloat(m, "temperature_c", "temperature_junction_c", "temperature_edge")
		powerW := getFloat(m, "power_usage_w")
		thermalPressureState := ""
		if pt.Tags != nil {
			thermalPressureState = pt.Tags["thermal_pressure_state"]
		}

		// Update buffers with temperature
		if tCurrent >= 0 {
			state.ThermalBuffer.Add(tCurrent)
			state.VolBuffer.Add(tCurrent)
		}

		// Compute sub-scores
		thermal, rocPenalty, thermalWarming := ComputeThermal(tCurrent, state.ThermalBuffer, state.Spec, thermalPressureState)
		power := ComputePower(powerW, state.Spec)
		volatility, volWarming := ComputeVolatility(state.VolBuffer, state.Spec)
		warmingUp := thermalWarming || volWarming

		memUsed := getFloat(m, "mem_used_bytes")
		memTotal := getFloat(m, "mem_total_bytes")
		memoryUsedPct := 0.0
		if memTotal > 0 {
			memoryUsedPct = memUsed / memTotal * 100
		}
		memory := ComputeMemory(memoryUsedPct)
		rLocal := ComputeComposite(thermal, power, volatility, memory)

		// Bonus metrics
		clockActual := getFloat(m, "sm_clock_mhz")
		clockMax := getFloat(m, "sm_clock_max_mhz")
		tJunction := getFloat(m, "temperature_junction_c")
		tEdge := getFloat(m, "temperature_edge")
		swapUsed := int64(getFloat(m, "swap_used_bytes"))
		swapTotal := int64(getFloat(m, "swap_total_bytes"))

		hourly, daily, monthly := ComputePowerCost(powerW, e.electricityRate)
		hotspotDelta := ComputeHotspotDelta(tJunction, tEdge)

		throttleActive := getFloat(m, "throttled") > 0
		throttleReason := "none"
		if pt.Tags != nil {
			if r, ok := pt.Tags["throttle_reason"]; ok {
				throttleReason = r
			}
		}

		timeToHotspot := ComputeTimeToHotspot(state.ThermalBuffer, tCurrent, state.Spec)

		out := RiskScoreOutput{
			DeviceID:          did,
			Thermal:           thermal,
			ThermalRoCPenalty: rocPenalty,
			Power:             power,
			Volatility:        volatility,
			Memory:            memory,
			BehaviorClass:     state.Spec.BehaviorClass,
			WarmingUp:         warmingUp,
			MemoryPressure:    ComputeMemoryPressure(memUsed, memTotal),
			ThrottleActive:    throttleActive,
			ThrottleReason:    throttleReason,
			ClockEfficiency:   ComputeClockEfficiency(clockActual, clockMax),
			PowerCostHourly:   hourly,
			PowerCostDaily:    daily,
			PowerCostMonthly:  monthly,
			HotspotDeltaC:     hotspotDelta,
			SwapPressure:      ComputeSwapPressure(swapUsed, swapTotal),
			TimeToHotspot:     timeToHotspot,
		}

		locals = append(locals, localResult{
			deviceID:   did,
			thermal:    thermal,
			power:      power,
			volatility: volatility,
			memory:     memory,
			rLocal:     rLocal,
			warmingUp:  warmingUp,
			output:     out,
		})
	}

	// Second pass: severity, trend
	results := make([]RiskScoreOutput, 0, len(locals))
	for _, l := range locals {
		composite := l.rLocal
		state := e.states[l.deviceID]
		out := l.output
		out.Composite = composite
		out.Severity = ClassifySeverity(composite, out.BehaviorClass)
		if state != nil {
			if state.LastUpdate.IsZero() {
				out.Trend = "stable"
				out.TrendDelta = 0
				state.LastComposite = composite
				state.LastUpdate = time.Now()
			} else {
				out.TrendDelta = composite - state.LastComposite
				out.Trend = ComputeTrend(composite, state.LastComposite)
				state.LastComposite = composite
				state.LastUpdate = time.Now()
			}
		} else {
			out.Trend = "stable"
		}

		results = append(results, out)
	}

	return results
}

func (e *ScoreEngine) getOrCreateState(deviceID string, pt normalizer.TelemetryPoint) *DeviceScoreState {
	if s, ok := e.states[deviceID]; ok {
		return s
	}
	model := deviceModelFromPoint(pt)
	spec := registry.Lookup(registry.NormalizeModelName(model))
	if pt.Tags != nil {
		if v, ok := pt.Tags["behavior_class"]; ok && v != "" {
			spec.BehaviorClass = v
		}
	}
	s := &DeviceScoreState{
		DeviceID:      deviceID,
		Spec:          spec,
		ThermalBuffer: NewRingBuffer(thermalBufferCap),
		VolBuffer:     NewRingBuffer(volatilityBufferCap),
	}
	e.states[deviceID] = s
	return s
}

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

func getFloat(m map[string]float64, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return -1
}
