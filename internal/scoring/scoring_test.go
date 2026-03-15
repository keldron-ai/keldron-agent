// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scoring

import (
	"math"
	"testing"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/registry"
)

func TestRingBuffer(t *testing.T) {
	rb := NewRingBuffer(5)
	if rb.Len() != 0 {
		t.Errorf("Len() = %d, want 0", rb.Len())
	}
	if rb.IsFull() {
		t.Error("IsFull() = true, want false")
	}

	// Add values
	for i := 1; i <= 5; i++ {
		rb.Add(float64(i))
	}
	if rb.Len() != 5 {
		t.Errorf("Len() = %d, want 5", rb.Len())
	}
	if !rb.IsFull() {
		t.Error("IsFull() = false, want true")
	}

	oldest, ok := rb.Oldest()
	if !ok || oldest != 1 {
		t.Errorf("Oldest() = %v, %v; want 1, true", oldest, ok)
	}
	newest, ok := rb.Newest()
	if !ok || newest != 5 {
		t.Errorf("Newest() = %v, %v; want 5, true", newest, ok)
	}

	mean := rb.Mean()
	wantMean := 3.0
	if math.Abs(mean-wantMean) > 1e-9 {
		t.Errorf("Mean() = %v, want %v", mean, wantMean)
	}
	stdev := rb.Stdev()
	// Sample stdev of 1,2,3,4,5 is sqrt(10/4) ≈ 1.58
	if stdev < 1.5 || stdev > 1.7 {
		t.Errorf("Stdev() = %v, want ~1.58", stdev)
	}

	// Overwrite oldest
	rb.Add(6)
	oldest, ok = rb.Oldest()
	if !ok || oldest != 2 {
		t.Errorf("After overwrite Oldest() = %v, %v; want 2, true", oldest, ok)
	}
}

func TestComputeThermal_Piecewise(t *testing.T) {
	spec := registry.GPUSpec{ThermalLimitC: 100, BehaviorClass: "consumer_active_cooled"}
	rb := NewRingBuffer(10)
	for i := 0; i < 10; i++ {
		rb.Add(50) // 50% of limit
	}

	score, rocPenalty, warmingUp := ComputeThermal(50, rb, spec, "")
	if warmingUp {
		t.Error("warmingUp = true, want false")
	}
	if score != 0 {
		t.Errorf("T=50 (50%% of 100) score = %v, want 0", score)
	}
	if rocPenalty != 0 {
		t.Errorf("rocPenalty = %v, want 0", rocPenalty)
	}

	// 60% of limit -> 0 (use buffer with same temp so RoC=0)
	rb60 := NewRingBuffer(10)
	for i := 0; i < 10; i++ {
		rb60.Add(60)
	}
	score, _, _ = ComputeThermal(60, rb60, spec, "")
	if score != 0 {
		t.Errorf("T=60 (60%%) score = %v, want 0", score)
	}

	// 80% of limit -> 50
	rb2 := NewRingBuffer(10)
	for i := 0; i < 10; i++ {
		rb2.Add(78)
	}
	rb2.Add(80)
	score, _, _ = ComputeThermal(80, rb2, spec, "")
	if math.Abs(score-50) > 1 {
		t.Errorf("T=80 (80%%) score = %v, want ~50", score)
	}
}

func TestComputeThermal_SteppedRoC(t *testing.T) {
	spec := registry.GPUSpec{ThermalLimitC: 83, BehaviorClass: "datacenter_sustained"}
	rb := NewRingBuffer(10)
	// 5 min ago: 50, now: 75 -> RoC = 5 °C/min
	for i := 0; i < 10; i++ {
		rb.Add(50 + float64(i)*2.5)
	}

	score, rocPenalty, warmingUp := ComputeThermal(75, rb, spec, "")
	if warmingUp {
		t.Error("warmingUp = true, want false")
	}
	if rocPenalty != 40 {
		t.Errorf("RoC=5 penalty = %v, want 40", rocPenalty)
	}
	if score < 90 {
		t.Errorf("score = %v, want >= 90", score)
	}
}

func TestComputeThermal_AppleSiliconOverride(t *testing.T) {
	spec := registry.Lookup("M4-Pro")
	if spec.BehaviorClass != "soc_integrated" {
		t.Skip("M4-Pro not in registry or wrong class")
	}
	spec.ThermalPressureStateSupported = true

	rb := NewRingBuffer(10)
	for i := 0; i < 10; i++ {
		rb.Add(72)
	}

	score, rocPenalty, warmingUp := ComputeThermal(72, rb, spec, "fair")
	if warmingUp {
		t.Error("warmingUp = true, want false")
	}
	if score != 40 {
		t.Errorf("fair override score = %v, want 40", score)
	}
	if rocPenalty != 0 {
		t.Errorf("rocPenalty = %v, want 0", rocPenalty)
	}
}

func TestComputeThermal_AppleSiliconFallback(t *testing.T) {
	spec := registry.Lookup("M4-Pro")
	spec.ThermalPressureStateSupported = false
	spec.ThermalLimitC = 105

	rb := NewRingBuffer(10)
	for i := 0; i < 10; i++ {
		rb.Add(72)
	}

	score, _, _ := ComputeThermal(72, rb, spec, "")
	// tRatio = 72/105 = 0.686, tScore = ((0.686-0.60)/0.40)*100 = 21.4
	if math.Abs(score-21.4) > 1 {
		t.Errorf("fallback score = %v, want ~21.4", score)
	}
}

func TestComputeThermal_WarmingUp(t *testing.T) {
	spec := registry.GPUSpec{ThermalLimitC: 83}
	rb := NewRingBuffer(10)
	rb.Add(70)
	rb.Add(71)

	_, _, warmingUp := ComputeThermal(72, rb, spec, "")
	if !warmingUp {
		t.Error("warmingUp = false, want true (buffer not full)")
	}
}

func TestComputePower(t *testing.T) {
	spec := registry.GPUSpec{TDPW: 450}
	got := ComputePower(225, spec)
	if got != 50 {
		t.Errorf("ComputePower(225, 450) = %v, want 50", got)
	}
	got = ComputePower(500, spec)
	if got != 100 {
		t.Errorf("ComputePower(500, 450) = %v, want 100 (capped)", got)
	}
	got = ComputePower(0, spec)
	if got != 0 {
		t.Errorf("ComputePower(0, 450) = %v, want 0", got)
	}
}

func TestComputeVolatility_BurstyConsumer(t *testing.T) {
	// RTX 4090: cv_max=0.60, temp swings 40-70°C
	spec := registry.Lookup("RTX-4090")
	rb := NewRingBuffer(60)
	// Simulate temps: mean ~55, stdev ~10, cv = 10/55 = 0.18
	for i := 0; i < 10; i++ {
		rb.Add(40 + float64(i)*3)
	}

	score, warmingUp := ComputeVolatility(rb, spec)
	if !warmingUp {
		t.Log("warmingUp = false (Len >= 10)")
	}
	// cv = stdev/mean. With 10 values 40,43,46,...,67: mean ~53.5, stdev ~9.5, cv ~0.18
	// S_vol = (cv/0.60)*100 = 30
	if score > 50 {
		t.Errorf("Bursty consumer S_vol = %v, should NOT peg at 100 (want ~36)", score)
	}
}

func TestComputeVolatility_WarmingUp(t *testing.T) {
	spec := registry.GPUSpec{CVMax: 0.60}
	rb := NewRingBuffer(60)
	for i := 0; i < 5; i++ {
		rb.Add(50)
	}

	_, warmingUp := ComputeVolatility(rb, spec)
	if !warmingUp {
		t.Error("warmingUp = false, want true (Len < 10)")
	}
}

func TestComputeFleetPenalty(t *testing.T) {
	// All cold peers
	got := ComputeFleetPenalty([]float64{10, 10, 10})
	if got != 0 {
		t.Errorf("cold peers penalty = %v, want 0", got)
	}

	// >30% stressed
	got = ComputeFleetPenalty([]float64{80, 75, 10, 10, 10})
	if got != 10 {
		t.Errorf(">30%% stressed penalty = %v, want 10", got)
	}

	// Exactly 30% (3/10) - no penalty (strictly > 0.30)
	got = ComputeFleetPenalty([]float64{80, 80, 80, 10, 10, 10, 10, 10, 10, 10})
	if got != 0 {
		t.Errorf("30%% stressed (3/10) penalty = %v, want 0", got)
	}
}

func TestComputeComposite_MeltingMachine(t *testing.T) {
	// S_thermal=100, S_power=95, S_volatility=80, peers all at 10
	thermal := 100.0
	power := 95.0
	volatility := 80.0
	fleetPenalty := 0.0

	// rLocal = 0.50*100 + 0.31*95 + 0.19*80 = 50 + 29.45 + 15.2 = 94.65
	got := ComputeComposite(thermal, power, volatility, fleetPenalty)
	if math.Abs(got-94.65) > 0.1 {
		t.Errorf("composite = %v, want 94.65", got)
	}

	// Classify: datacenter 80->critical
	sev := ClassifySeverity(got, "datacenter_sustained")
	if sev != SeverityCritical {
		t.Errorf("severity = %q, want critical", sev)
	}
}

func TestComputeComposite_All100(t *testing.T) {
	got := ComputeComposite(100, 100, 100, 0)
	if got != 100 {
		t.Errorf("all-100 composite = %v, want 100", got)
	}
	got = ComputeComposite(100, 100, 100, 10)
	if got != 100 {
		t.Errorf("all-100 + fleet composite = %v, want 100 (capped)", got)
	}
}

func TestClassifySeverity_BehaviorClass(t *testing.T) {
	score := 83.0
	if ClassifySeverity(score, "datacenter_sustained") != SeverityCritical {
		t.Error("83 datacenter = critical")
	}
	if ClassifySeverity(score, "consumer_active_cooled") != SeverityCritical {
		t.Error("83 consumer = critical")
	}
	if ClassifySeverity(score, "soc_integrated") != SeverityWarning {
		t.Error("83 soc = warning")
	}
	if ClassifySeverity(score, "sbc_constrained") != SeverityWarning {
		t.Error("83 sbc = warning")
	}
}

func TestComputeTrend(t *testing.T) {
	if ComputeTrend(85, 82) != "rising" {
		t.Error("delta 3 = rising")
	}
	if ComputeTrend(80, 85) != "falling" {
		t.Error("delta -5 = falling")
	}
	if ComputeTrend(83, 82) != "stable" {
		t.Error("delta 1 = stable")
	}
}

func TestComputeTimeToHotspot(t *testing.T) {
	spec := registry.GPUSpec{ThermalLimitC: 83}

	// Rising temps
	rb := NewRingBuffer(10)
	for i := 0; i < 10; i++ {
		rb.Add(50 + float64(i)*2)
	}
	got := ComputeTimeToHotspot(rb, 68, spec)
	if got == nil {
		t.Error("rising temps: want non-nil time-to-hotspot")
	} else if *got <= 0 {
		t.Errorf("rising temps: got %v, want positive", *got)
	}

	// Stable temps
	rb2 := NewRingBuffer(10)
	for i := 0; i < 10; i++ {
		rb2.Add(70)
	}
	got = ComputeTimeToHotspot(rb2, 70, spec)
	if got != nil {
		t.Errorf("stable temps: got %v, want nil", got)
	}

	// Cooling
	rb3 := NewRingBuffer(10)
	for i := 0; i < 10; i++ {
		rb3.Add(80 - float64(i))
	}
	got = ComputeTimeToHotspot(rb3, 72, spec)
	if got != nil {
		t.Errorf("cooling temps: got %v, want nil", got)
	}
}

func TestComputeTimeToHotspot_NotEnoughData(t *testing.T) {
	spec := registry.GPUSpec{ThermalLimitC: 83}
	rb := NewRingBuffer(10)
	rb.Add(70)
	rb.Add(71)

	got := ComputeTimeToHotspot(rb, 72, spec)
	if got != nil {
		t.Errorf("Len() < 5: got %v, want nil", got)
	}
}

func TestBonusMetrics(t *testing.T) {
	if ComputeMemoryPressure(50, 100) != 0.5 {
		t.Error("memory pressure")
	}
	if ComputeClockEfficiency(1500, 2000) != 0.75 {
		t.Error("clock efficiency")
	}
	h, d, m := ComputePowerCost(100, 0.12)
	if math.Abs(h-0.012) > 1e-6 {
		t.Errorf("power cost hourly = %v", h)
	}
	if math.Abs(d-0.288) > 1e-4 {
		t.Errorf("power cost daily = %v", d)
	}
	if math.Abs(m-8.64) > 1e-2 {
		t.Errorf("power cost monthly = %v", m)
	}
	if ComputeHotspotDelta(90, 80) != 10 {
		t.Error("hotspot delta")
	}
	if ComputeHotspotDelta(-1, 80) != -1 {
		t.Error("hotspot delta unavailable")
	}
	if ComputeSwapPressure(100, 1000) != 0.1 {
		t.Error("swap pressure")
	}
}

func TestScoreEngine_Integration(t *testing.T) {
	engine := NewScoreEngine(0.12)
	// Empty batch
	got := engine.Score(nil)
	if got != nil {
		t.Errorf("Score(nil) = %v, want nil", got)
	}

	// Single device
	batch := []struct {
		pt normalizer.TelemetryPoint
	}{
		{
			pt: normalizer.TelemetryPoint{
				Source:      "host1",
				AdapterName: "dcgm",
				Metrics: map[string]float64{
					"gpu_id":           0,
					"temperature_c":    42,
					"power_usage_w":    100,
					"mem_used_bytes":   8e9,
					"mem_total_bytes":  16e9,
					"sm_clock_mhz":     1500,
					"sm_clock_max_mhz": 2000,
				},
			},
		},
	}
	// Need to convert to TelemetryPoint
	var pts []normalizer.TelemetryPoint
	for _, b := range batch {
		b.pt.Tags = map[string]string{"gpu_model": "RTX-4090", "device_model": "RTX-4090"}
		pts = append(pts, b.pt)
	}
	scores := engine.Score(pts)
	if len(scores) != 1 {
		t.Fatalf("len(scores) = %d, want 1", len(scores))
	}
	s := scores[0]
	if s.DeviceID == "" {
		t.Error("DeviceID empty")
	}
	if s.Composite < 0 || s.Composite > 100 {
		t.Errorf("Composite = %v, want 0-100", s.Composite)
	}
	if s.Severity != SeverityNormal && s.Severity != SeverityWarning && s.Severity != SeverityCritical {
		t.Errorf("Severity = %q", s.Severity)
	}
}
