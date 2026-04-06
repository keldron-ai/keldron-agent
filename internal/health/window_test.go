// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import (
	"math"
	"testing"
	"time"
)

func TestComputeHeadroom_NoSustainedLoad(t *testing.T) {
	t0 := time.Now()
	samples := []healthSample{
		{at: t0, tempC: 18, tempCPresent: true, utilPct: 0, powerW: 2},
		{at: t0.Add(time.Second), tempC: 19, tempCPresent: true, utilPct: 0, powerW: 2},
	}
	r := computeHeadroom(samples, 100, false)
	if r == nil || !r.Available || !r.NoSustainedLoad {
		t.Fatalf("want no sustained load, got %+v", r)
	}
}

func TestComputeHeadroom_HeadroomRating(t *testing.T) {
	t0 := time.Now()
	samples := []healthSample{
		{at: t0, tempC: 40, tempCPresent: true, utilPct: 10, powerW: 5},
	}
	r := computeHeadroom(samples, 100, false)
	if r == nil || !r.Available || r.NoSustainedLoad {
		t.Fatalf("expected headroom result, got %+v", r)
	}
	if r.HeadroomUsedPct <= 0 {
		t.Errorf("headroom %% = %v, want > 0", r.HeadroomUsedPct)
	}
	if got := headroomRating(r.HeadroomUsedPct); got != r.Rating {
		t.Errorf("rating mismatch %q vs %q", got, r.Rating)
	}
}

func TestScanRecoveries_SingleSpike(t *testing.T) {
	t0 := time.Now()
	rt := 60.0
	samples := []healthSample{
		{at: t0, tempC: 50, tempCPresent: true, utilPct: 0, powerW: 2},
		{at: t0.Add(10 * time.Second), tempC: 80, tempCPresent: true, utilPct: 90, powerW: 40},
		{at: t0.Add(40 * time.Second), tempC: 82, tempCPresent: true, utilPct: 90, powerW: 40},
		{at: t0.Add(100 * time.Second), tempC: 55, tempCPresent: true, utilPct: 10, powerW: 5},
	}
	sec, _, _, had := scanRecoveries(samples, rt)
	if !had {
		t.Fatal("expected hadSpike")
	}
	if sec <= 0 {
		t.Errorf("recovery sec = %d, want > 0", sec)
	}
}

func TestStdDevSample_ZeroVariance(t *testing.T) {
	x := []float64{50, 50, 50}
	sd := stdDevSample(x)
	if sd != 0 {
		t.Errorf("stddev = %v, want 0", sd)
	}
}

func TestStdDevSample_OnePoint(t *testing.T) {
	sd := stdDevSample([]float64{42})
	if sd != 0 {
		t.Errorf("stddev = %v, want 0", sd)
	}
}

func TestRecoveryRating_Bands(t *testing.T) {
	if recoveryRating(30) != "good" {
		t.Errorf("30s want good")
	}
	if recoveryRating(90) != "fair" {
		t.Errorf("90s want fair")
	}
	if recoveryRating(200) != "poor" {
		t.Errorf("200s want poor")
	}
}

func TestComputePerfPerWatt_MeanRatio(t *testing.T) {
	t0 := time.Now()
	samples := []healthSample{
		{at: t0, tempC: 50, tempCPresent: true, utilPct: 50, powerW: 10},
		{at: t0.Add(time.Second), tempC: 51, tempCPresent: true, utilPct: 30, powerW: 10},
	}
	r := computePerfPerWatt(samples)
	if r == nil || !r.Available {
		t.Fatalf("want available ppw, got %+v", r)
	}
	want := (50.0 + 30.0) / 2 / 10.0
	if math.Abs(r.Value-want) > 1e-9 {
		t.Errorf("ppw = %v, want %v", r.Value, want)
	}
}
