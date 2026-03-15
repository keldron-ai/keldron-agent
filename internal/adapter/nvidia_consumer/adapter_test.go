//go:build linux || windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package nvidia_consumer

import (
	"context"
	"path/filepath"
	"testing"
)

func TestParseNvidiaSmiCSV_SingleGPU(t *testing.T) {
	csv := `0, NVIDIA GeForce RTX 4090, 62, 83, 95, 45, 18432, 24564, 350, 450, 2520, 2520, 65, 1234567890, 00000000:01:00.0, 0x0000000000000000`
	readings, err := parseNvidiaSmiCSV([]byte(csv))
	if err != nil {
		t.Fatalf("parseNvidiaSmiCSV: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("want 1 reading, got %d", len(readings))
	}
	r := readings[0]
	if r.Index != 0 {
		t.Errorf("Index = %d, want 0", r.Index)
	}
	if r.Name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("Name = %q, want %q", r.Name, "NVIDIA GeForce RTX 4090")
	}
	if r.TemperatureC != 62 {
		t.Errorf("TemperatureC = %v, want 62", r.TemperatureC)
	}
	if r.TempLimitC != 83 {
		t.Errorf("TempLimitC = %v, want 83", r.TempLimitC)
	}
	if r.GPUUtil != 95 {
		t.Errorf("GPUUtil = %v, want 95", r.GPUUtil)
	}
	if r.MemUsedMB != 18432 {
		t.Errorf("MemUsedMB = %v, want 18432", r.MemUsedMB)
	}
	if r.MemTotalMB != 24564 {
		t.Errorf("MemTotalMB = %v, want 24564", r.MemTotalMB)
	}
	if r.PowerDrawW != 350 {
		t.Errorf("PowerDrawW = %v, want 350", r.PowerDrawW)
	}
	if r.ClockSMMHz != 2520 {
		t.Errorf("ClockSMMHz = %v, want 2520", r.ClockSMMHz)
	}
	if r.FanSpeedPct != 65 {
		t.Errorf("FanSpeedPct = %v, want 65", r.FanSpeedPct)
	}
	if r.PCIBusID != "00000000:01:00.0" {
		t.Errorf("PCIBusID = %q, want 00000000:01:00.0", r.PCIBusID)
	}
	if r.ThrottleReason != 0 {
		t.Errorf("ThrottleReason = %d, want 0", r.ThrottleReason)
	}
}

func TestParseNvidiaSmiCSV_MultiGPU(t *testing.T) {
	csv := `0, NVIDIA GeForce RTX 4090, 62, 83, 95, 45, 18432, 24564, 350, 450, 2520, 2520, 65, 1234567890, 00000000:01:00.0, 0x0000000000000000
1, NVIDIA GeForce RTX 3090, 58, 83, 80, 60, 10240, 24576, 280, 350, 1695, 1695, 55, 1234567891, 00000000:02:00.0, 0x0000000000000000
2, NVIDIA GeForce RTX 4090, 64, 83, 0, 0, 0, 24564, 25, 450, 210, 2520, 30, 1234567892, 00000000:03:00.0, 0x0000000000000001`
	readings, err := parseNvidiaSmiCSV([]byte(csv))
	if err != nil {
		t.Fatalf("parseNvidiaSmiCSV: %v", err)
	}
	if len(readings) != 3 {
		t.Fatalf("want 3 readings, got %d", len(readings))
	}
	if readings[0].Index != 0 || readings[1].Index != 1 || readings[2].Index != 2 {
		t.Errorf("Indices wrong: got %d, %d, %d", readings[0].Index, readings[1].Index, readings[2].Index)
	}
	if readings[2].ThrottleReason != 1 {
		t.Errorf("GPU 2 ThrottleReason = %d, want 1 (GPU Idle)", readings[2].ThrottleReason)
	}
}

func TestParseNvidiaSmiCSV_NAHandling(t *testing.T) {
	// Some fields [N/A] should be handled gracefully
	csv := `0, NVIDIA GeForce RTX 4090, [N/A], 83, 95, 45, 18432, 24564, [N/A], 450, 2520, 2520, [N/A], 1234567890, 00000000:01:00.0, 0x0000000000000000`
	readings, err := parseNvidiaSmiCSV([]byte(csv))
	if err != nil {
		t.Fatalf("parseNvidiaSmiCSV: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("want 1 reading, got %d", len(readings))
	}
	r := readings[0]
	if r.TemperatureC != 0 {
		t.Errorf("TemperatureC [N/A] = %v, want 0", r.TemperatureC)
	}
	if r.PowerDrawW != 0 {
		t.Errorf("PowerDrawW [N/A] = %v, want 0", r.PowerDrawW)
	}
	if r.FanSpeedPct != 0 {
		t.Errorf("FanSpeedPct [N/A] = %v, want 0", r.FanSpeedPct)
	}
}

func TestParseNvidiaSmiCSV_EmptyOutput(t *testing.T) {
	readings, err := parseNvidiaSmiCSV([]byte{})
	if err != nil {
		t.Fatalf("parseNvidiaSmiCSV empty: %v", err)
	}
	if len(readings) != 0 {
		t.Errorf("want 0 readings, got %d", len(readings))
	}
}

func TestParseNvidiaSmiCSV_InvalidCSV(t *testing.T) {
	// Too few columns
	csv := `0, NVIDIA GeForce RTX 4090, 62`
	_, err := parseNvidiaSmiCSV([]byte(csv))
	if err == nil {
		t.Fatal("want error for invalid CSV, got nil")
	}
}

func TestMapThrottleReason(t *testing.T) {
	tests := []struct {
		bitmask uint64
		active  bool
		reason  string
	}{
		{0x0000000000000000, false, "none"},
		{0x0000000000000001, false, "none"}, // GPU Idle
		{0x0000000000000040, true, "thermal"},
		{0x0000000000000004, true, "power"},
		{0x0000000000000044, true, "thermal"}, // thermal and power, thermal wins
		{0x0000000000000020, true, "thermal"}, // Sw thermal
		{0x0000000000000080, true, "power"},
		{0x0000000000000002, true, "other"},
	}
	for _, tt := range tests {
		active, reason := MapThrottleReason(tt.bitmask)
		if active != tt.active || reason != tt.reason {
			t.Errorf("MapThrottleReason(0x%016x) = (%v, %q), want (%v, %q)",
				tt.bitmask, active, reason, tt.active, tt.reason)
		}
	}
}

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"NVIDIA GeForce RTX 4090", "RTX-4090"},
		{"NVIDIA GeForce RTX 3090 Ti", "RTX-3090-Ti"},
		{"NVIDIA A100-SXM4-80GB", "A100-SXM"},
		{"NVIDIA GeForce RTX 5090", "RTX-5090"},
	}
	for _, tt := range tests {
		got := normalizeModelName(tt.raw)
		if got != tt.want {
			t.Errorf("normalizeModelName(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestResolveNvidiaSMIPath(t *testing.T) {
	// On linux/windows with nvidia-smi in PATH, should succeed
	path, err := resolveNvidiaSMIPath("nvidia-smi")
	if err != nil {
		t.Skipf("nvidia-smi not in PATH, skipping: %v", err)
	}
	if path == "" {
		t.Error("resolveNvidiaSMIPath returned empty path")
	}
}

func TestCollectNvidiaSmi_NoExec(t *testing.T) {
	// Use a non-existent path - should fail
	ctx := context.Background()
	tempDir := t.TempDir()
	missingPath := filepath.Join(tempDir, "nvidia-smi-missing")
	_, err := CollectNvidiaSmi(ctx, missingPath, nil)
	if err == nil {
		t.Fatal("CollectNvidiaSmi with bad path should fail")
	}
}
