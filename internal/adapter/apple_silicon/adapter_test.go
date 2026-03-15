//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package apple_silicon

import (
	"testing"

	"github.com/keldron-ai/keldron-agent/registry"
)

func TestNormalizeChipName(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"Apple M4 Pro", "M4-Pro"},
		{"Apple M1", "M1"},
		{"Apple M3 Max", "M3-Max"},
		{"Apple M2 Ultra", "M2-Ultra"},
		{"Apple M5 Pro", "M5-Pro"},
		{"Apple M5 Max", "M5-Max"},
		{"Apple M5", "M5"},
	}
	for _, tt := range tests {
		got := NormalizeChipName(tt.raw)
		if got != tt.want {
			t.Errorf("NormalizeChipName(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestThermalStateNames(t *testing.T) {
	tests := []struct {
		state uint64
		want  string
	}{
		{0, "nominal"},
		{1, "fair"},
		{2, "serious"},
		{3, "critical"},
		{4, "critical"},
	}
	for _, tt := range tests {
		if name, ok := thermalStateNames[tt.state]; !ok || name != tt.want {
			t.Errorf("thermalStateNames[%d] = %q (ok=%v), want %q", tt.state, name, ok, tt.want)
		}
	}
}

func TestIsThrottled(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"nominal", false},
		{"", false},
		{"fair", true},
		{"serious", true},
		{"critical", true},
	}
	for _, tt := range tests {
		got := IsThrottled(tt.state)
		if got != tt.want {
			t.Errorf("IsThrottled(%q) = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestParseSwapUsage(t *testing.T) {
	used, total := parseSwapUsage("total = 1024.00M  used = 512.00M  free = 512.00M")
	if used != 512*1024*1024 {
		t.Errorf("parseSwapUsage used = %d, want 536870912", used)
	}
	if total != 1024*1024*1024 {
		t.Errorf("parseSwapUsage total = %d, want 1073741824", total)
	}

	used2, total2 := parseSwapUsage("total = 0.00M  used = 0.00M  free = 0.00M")
	if used2 != 0 || total2 != 0 {
		t.Errorf("parseSwapUsage zeros: got used=%d total=%d", used2, total2)
	}
}

func TestParseVMStatPage(t *testing.T) {
	s := `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                        12345.
Pages active:                      67890.
Pages inactive:                    11111.
Pages speculative:                 2222.
Pages wired down:                  33333.
Pages occupied by compressor:      4444.`
	got, err := parseVMStatPage(s, "Pages active")
	if err != nil {
		t.Fatalf("parseVMStatPage: %v", err)
	}
	if got != 67890 {
		t.Errorf("parseVMStatPage Pages active = %d, want 67890", got)
	}
}

func TestRegistryLookup(t *testing.T) {
	spec := registry.Lookup("M4-Pro")
	if spec.BehaviorClass != "soc_integrated" {
		t.Errorf("M4-Pro BehaviorClass = %q, want soc_integrated", spec.BehaviorClass)
	}
	if spec.Vendor != "apple" {
		t.Errorf("M4-Pro Vendor = %q, want apple", spec.Vendor)
	}

	spec5 := registry.Lookup("M5")
	if spec5.BehaviorClass != "soc_integrated" {
		t.Errorf("M5 BehaviorClass = %q, want soc_integrated", spec5.BehaviorClass)
	}
	if spec5.TDPW != 25 {
		t.Errorf("M5 TDPW = %v, want 25", spec5.TDPW)
	}
}

func TestReadIOKit(t *testing.T) {
	r := ReadIOKit(nil)
	if r == nil {
		t.Fatal("ReadIOKit returned nil")
	}
	// On real Apple Silicon hardware, ReadIOKit returns live metrics.
	// Just verify it returns non-negative values without crashing.
	if r.GPUUtilization < 0 || r.GPUPowerW < 0 || r.SoCTempC < 0 {
		t.Errorf("ReadIOKit: unexpected negative values: util=%v power=%v temp=%v",
			r.GPUUtilization, r.GPUPowerW, r.SoCTempC)
	}
}
