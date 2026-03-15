// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package registry

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		model       string
		wantVendor  string
		wantThermal float64
		wantTDP     float64
		wantClass   string
	}{
		{"H100-SXM", "nvidia", 83, 700, "datacenter_sustained"},
		{"h100-sxm", "nvidia", 83, 700, "datacenter_sustained"},
		{"RTX-4090", "nvidia", 83, 450, "consumer_active_cooled"},
		{"rtx-4090", "nvidia", 83, 450, "consumer_active_cooled"},
		{"M4-Pro", "apple", 105, 30, "soc_integrated"},
		{"m4-pro", "apple", 105, 30, "soc_integrated"},
		{"MI300X", "amd", 100, 750, "datacenter_sustained"},
	}
	for _, tt := range tests {
		spec := Lookup(tt.model)
		if spec.Vendor != tt.wantVendor {
			t.Errorf("Lookup(%q).Vendor = %q, want %q", tt.model, spec.Vendor, tt.wantVendor)
		}
		if spec.ThermalLimitC != tt.wantThermal {
			t.Errorf("Lookup(%q).ThermalLimitC = %v, want %v", tt.model, spec.ThermalLimitC, tt.wantThermal)
		}
		if spec.TDPW != tt.wantTDP {
			t.Errorf("Lookup(%q).TDPW = %v, want %v", tt.model, spec.TDPW, tt.wantTDP)
		}
		if spec.BehaviorClass != tt.wantClass {
			t.Errorf("Lookup(%q).BehaviorClass = %q, want %q", tt.model, spec.BehaviorClass, tt.wantClass)
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	spec := Lookup("Unknown-GPU-XYZ-999")
	if spec.BehaviorClass != "consumer_active_cooled" {
		t.Errorf("unknown model BehaviorClass = %q, want consumer_active_cooled", spec.BehaviorClass)
	}
	if spec.CVMax != 0.60 {
		t.Errorf("unknown model CVMax = %v, want 0.60", spec.CVMax)
	}
	if spec.ThermalLimitC != 83 {
		t.Errorf("unknown model ThermalLimitC = %v, want 83 (default)", spec.ThermalLimitC)
	}
	if spec.TDPW != 350 {
		t.Errorf("unknown model TDPW = %v, want 350 (default)", spec.TDPW)
	}

	// LookupWithFallback with driver values
	spec2 := LookupWithFallback("Unknown-GPU", 90, 400)
	if spec2.ThermalLimitC != 90 {
		t.Errorf("fallback with driver thermal: ThermalLimitC = %v, want 90", spec2.ThermalLimitC)
	}
	if spec2.TDPW != 400 {
		t.Errorf("fallback with driver TDP: TDPW = %v, want 400", spec2.TDPW)
	}

	// When driver reports zeros, use defaults
	spec3 := LookupWithFallback("Unknown-GPU", 0, 0)
	if spec3.ThermalLimitC != 83 || spec3.TDPW != 350 {
		t.Errorf("fallback with zero driver values: got thermal=%v tdp=%v, want 83, 350",
			spec3.ThermalLimitC, spec3.TDPW)
	}
	if spec3.BehaviorClass != "consumer_active_cooled" || spec3.CVMax != 0.60 {
		t.Errorf("fallback: class=%q cv_max=%v, want consumer_active_cooled 0.60",
			spec3.BehaviorClass, spec3.CVMax)
	}
}

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"NVIDIA H100 SXM", "H100-SXM"},
		{"NVIDIA A100-SXM4-80GB", "A100-SXM"},
		{"Apple M4 Pro", "M4-Pro"},
		{"AMD MI300X", "MI300X"},
		{"nvidia h100 sxm", "h100-sxm"},
		{"M4-Pro", "M4-Pro"},
		{"MI355X", "MI300X"},
	}
	for _, tt := range tests {
		got := NormalizeModelName(tt.raw)
		if got != tt.want {
			t.Errorf("NormalizeModelName(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestNormalizeThermal(t *testing.T) {
	h100 := Lookup("H100-SXM")
	got := NormalizeThermal(78, h100)
	if got < 0.939 || got > 0.941 {
		t.Errorf("NormalizeThermal(78, H100) = %v, want ~0.940", got)
	}

	m4 := Lookup("M4")
	got2 := NormalizeThermal(72, m4)
	if got2 < 0.685 || got2 > 0.687 {
		t.Errorf("NormalizeThermal(72, M4) = %v, want ~0.686", got2)
	}
}

func TestNormalizePower(t *testing.T) {
	rtx4090 := Lookup("RTX-4090")
	got := NormalizePower(380, rtx4090)
	if got < 0.843 || got > 0.845 {
		t.Errorf("NormalizePower(380, RTX-4090) = %v, want ~0.844", got)
	}
}

func TestEdgeToJunctionCorrection(t *testing.T) {
	mi250x := Lookup("MI250X")
	if mi250x.TempMeasurementType != "edge" {
		t.Fatalf("MI250X should have edge temp type, got %q", mi250x.TempMeasurementType)
	}
	got := ApplyEdgeToJunctionCorrection(70, mi250x)
	if got != 82 {
		t.Errorf("ApplyEdgeToJunctionCorrection(70, MI250X) = %v, want 82", got)
	}

	mi300x := Lookup("MI300X")
	if mi300x.TempMeasurementType != "junction" {
		t.Fatalf("MI300X should have junction temp type, got %q", mi300x.TempMeasurementType)
	}
	got2 := ApplyEdgeToJunctionCorrection(85, mi300x)
	if got2 != 85 {
		t.Errorf("ApplyEdgeToJunctionCorrection(85, MI300X) = %v, want 85 (unchanged)", got2)
	}
}

func TestBehaviorClassAssignment(t *testing.T) {
	datacenter := Lookup("H100-SXM")
	if datacenter.BehaviorClass != "datacenter_sustained" || datacenter.CVMax != 0.30 {
		t.Errorf("datacenter H100: class=%q cv_max=%v, want datacenter_sustained 0.30",
			datacenter.BehaviorClass, datacenter.CVMax)
	}

	consumer := Lookup("RTX-4090")
	if consumer.BehaviorClass != "consumer_active_cooled" || consumer.CVMax != 0.60 {
		t.Errorf("consumer RTX-4090: class=%q cv_max=%v, want consumer_active_cooled 0.60",
			consumer.BehaviorClass, consumer.CVMax)
	}

	soc := Lookup("M4-Pro")
	if soc.BehaviorClass != "soc_integrated" || soc.CVMax != 0.50 {
		t.Errorf("soc M4-Pro: class=%q cv_max=%v, want soc_integrated 0.50",
			soc.BehaviorClass, soc.CVMax)
	}
}

func TestAllEntriesValidation(t *testing.T) {
	validBehaviorClasses := map[string]bool{
		"datacenter_sustained":    true,
		"consumer_active_cooled":  true,
		"soc_integrated":          true,
		"consumer_passive_cooled": true,
	}

	entries := AllEntries()
	if len(entries) < 25 {
		t.Errorf("registry has %d entries, want at least 25", len(entries))
	}

	// Walk top-level JSON tokens to detect duplicate keys (case-insensitive)
	// before any unmarshalling silently overwrites them.
	dec := json.NewDecoder(strings.NewReader(string(gpuSpecsJSON)))
	tok, err := dec.Token() // opening '{'
	if err != nil || fmt.Sprintf("%v", tok) != "{" {
		t.Fatalf("expected opening '{', got %v (err=%v)", tok, err)
	}
	lowerKeys := make(map[string]string) // lowercased → original key
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatalf("reading key token: %v", err)
		}
		key := keyTok.(string)
		lower := strings.ToLower(key)
		if prev, exists := lowerKeys[lower]; exists {
			t.Errorf("duplicate key (case-insensitive): %q collides with %q", key, prev)
		}
		lowerKeys[lower] = key

		// Skip the value object
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatalf("skipping value for key %q: %v", key, err)
		}
	}

	// Parse into raw field maps to verify required keys are present
	requiredFields := []string{
		"vendor", "architecture", "thermal_limit_c", "tdp_w",
		"temp_measurement_type", "behavior_class", "cv_max",
		"thermal_pressure_state_supported",
	}
	var rawFields map[string]map[string]json.RawMessage
	if err := json.Unmarshal(gpuSpecsJSON, &rawFields); err != nil {
		t.Fatalf("failed to parse gpu_specs.json into raw fields: %v", err)
	}
	for key, fields := range rawFields {
		for _, req := range requiredFields {
			if _, ok := fields[req]; !ok {
				t.Errorf("%s: missing required field %q", key, req)
			}
		}
	}

	// Validate decoded values
	for key, spec := range entries {
		if spec.ThermalLimitC <= 0 {
			t.Errorf("%s: thermal_limit_c=%v, want > 0", key, spec.ThermalLimitC)
		}
		if spec.TDPW <= 0 {
			t.Errorf("%s: tdp_w=%v, want > 0", key, spec.TDPW)
		}
		if !validBehaviorClasses[spec.BehaviorClass] {
			t.Errorf("%s: behavior_class=%q, want one of datacenter_sustained, consumer_active_cooled, soc_integrated, consumer_passive_cooled", key, spec.BehaviorClass)
		}
		if spec.CVMax <= 0 || spec.CVMax > 1.0 {
			t.Errorf("%s: cv_max=%v, want > 0 and <= 1.0", key, spec.CVMax)
		}
		if spec.Vendor == "" {
			t.Errorf("%s: vendor is empty", key)
		}
	}
}

func TestAppleSiliconEntries(t *testing.T) {
	variants := []string{
		"M1", "M1-Pro", "M1-Max", "M1-Ultra",
		"M2", "M2-Pro", "M2-Max",
		"M3", "M3-Pro", "M3-Max",
		"M4", "M4-Pro", "M4-Max",
	}
	for _, model := range variants {
		spec := Lookup(model)
		if spec.BehaviorClass != "soc_integrated" {
			t.Errorf("%s: BehaviorClass = %q, want soc_integrated", model, spec.BehaviorClass)
		}
		if !spec.ThermalPressureStateSupported {
			t.Errorf("%s: ThermalPressureStateSupported = false, want true", model)
		}
		if spec.Vendor != "apple" {
			t.Errorf("%s: Vendor = %q, want apple", model, spec.Vendor)
		}
		if spec.ThermalLimitC != 105 {
			t.Errorf("%s: ThermalLimitC = %v, want 105", model, spec.ThermalLimitC)
		}
	}
}
