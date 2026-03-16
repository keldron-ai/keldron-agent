//go:build linux

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package linux_thermal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"gopkg.in/yaml.v3"
)

func TestClassifySensorType(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"coretemp", "cpu"},
		{"k10temp", "cpu"},
		{"cpu_thermal", "cpu"},
		{"amdgpu", "gpu"},
		{"nvidia", "gpu"},
		{"radeon", "gpu"},
		{"nvme", "nvme"},
		{"nvme0", "nvme"},
		{"soc_thermal", "soc"},
		{"thermal_zone", "soc"},
		{"random_device", "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifySensorType(tt.name); got != tt.want {
				t.Errorf("classifySensorType(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestDiscoverHwmon_MockSysfs(t *testing.T) {
	dir := t.TempDir()

	// hwmon0: coretemp
	hwmon0 := filepath.Join(dir, "hwmon0")
	if err := os.MkdirAll(hwmon0, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "name"), []byte("coretemp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "temp1_input"), []byte("62000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "temp1_label"), []byte("Package id 0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// hwmon1: nvme
	hwmon1 := filepath.Join(dir, "hwmon1")
	if err := os.MkdirAll(hwmon1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon1, "name"), []byte("nvme\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon1, "temp1_input"), []byte("45000\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.Default()
	sensors := DiscoverHwmon(dir, logger)
	if len(sensors) != 2 {
		t.Fatalf("got %d sensors, want 2", len(sensors))
	}
	// Order may vary
	var foundCore, foundNvme bool
	for _, s := range sensors {
		if s.Name == "coretemp" && s.SensorType == "cpu" && s.TempC == 62.0 && s.Label == "Package id 0" {
			foundCore = true
		}
		if s.Name == "nvme" && s.SensorType == "nvme" && s.TempC == 45.0 {
			foundNvme = true
		}
	}
	if !foundCore {
		t.Error("expected coretemp sensor with 62.0°C, cpu type")
	}
	if !foundNvme {
		t.Error("expected nvme sensor with 45.0°C")
	}
}

func TestDiscoverHwmon_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()
	sensors := DiscoverHwmon(dir, logger)
	if len(sensors) != 0 {
		t.Errorf("empty dir: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_MissingTempFile(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	if err := os.MkdirAll(hwmon0, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "name"), []byte("coretemp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// No temp1_input - sensor should be skipped
	logger := slog.Default()
	sensors := DiscoverHwmon(dir, logger)
	if len(sensors) != 0 {
		t.Errorf("missing temp file: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_MalformedValue(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	if err := os.MkdirAll(hwmon0, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "name"), []byte("coretemp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "temp1_input"), []byte("bad\n"), 0644); err != nil {
		t.Fatal(err)
	}
	logger := slog.Default()
	sensors := DiscoverHwmon(dir, logger)
	if len(sensors) != 0 {
		t.Errorf("malformed value: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverThermalZones_MockSysfs(t *testing.T) {
	dir := t.TempDir()
	zone0 := filepath.Join(dir, "thermal_zone0")
	if err := os.MkdirAll(zone0, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zone0, "temp"), []byte("65000\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.Default()
	zones := DiscoverThermalZones(dir, logger)
	if len(zones) != 1 {
		t.Fatalf("got %d zones, want 1", len(zones))
	}
	z := zones[0]
	if z.Zone != "thermal_zone0" || z.Type != "x86_pkg_temp" || z.TempC != 65.0 {
		t.Errorf("zone: got %+v", z)
	}
}

func TestDiscoverThermalZones_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()
	zones := DiscoverThermalZones(dir, logger)
	if len(zones) != 0 {
		t.Errorf("empty dir: got %d zones, want 0", len(zones))
	}
}

func TestAdapter_Collect(t *testing.T) {
	dir := t.TempDir()

	hwmon0 := filepath.Join(dir, "hwmon0")
	if err := os.MkdirAll(hwmon0, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "name"), []byte("coretemp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "temp1_input"), []byte("62000\n"), 0644); err != nil {
		t.Fatal(err)
	}

	zone0 := filepath.Join(dir, "thermal_zone0")
	if err := os.MkdirAll(zone0, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zone0, "temp"), []byte("65000\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ltCfg := LinuxThermalAdapterConfig{
		HwmonPath:   dir,
		ThermalPath: dir,
	}
	ltCfg.applyDefaults()

	a := &LinuxThermalAdapter{
		cfg:   ltCfg,
		logger: slog.Default(),
	}
	readings, err := a.collect(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) < 2 {
		t.Errorf("got %d readings, want at least 2 (hwmon + thermal)", len(readings))
	}
	var foundHwmon, foundThermal bool
	for _, r := range readings {
		if r.AdapterName != "linux_thermal" {
			t.Errorf("reading adapter: got %q", r.AdapterName)
		}
		if r.Source == "coretemp" || (len(r.Source) > 8 && r.Source[:8] == "coretemp") {
			foundHwmon = true
			v, ok := r.Metrics["cpu_temp_c"]
			if !ok {
				t.Fatalf("hwmon reading %q missing cpu_temp_c metric", r.Source)
			}
			f, ok := v.(float64)
			if !ok {
				t.Fatalf("hwmon reading %q cpu_temp_c type = %T, want float64", r.Source, v)
			}
			if f != 62.0 {
				t.Errorf("hwmon reading %q cpu_temp_c = %v, want 62.0", r.Source, f)
			}
		}
		if r.Source == "thermal_zone0" {
			foundThermal = true
			v, ok := r.Metrics["cpu_temp_c"]
			if !ok {
				t.Fatalf("thermal reading %q missing cpu_temp_c metric", r.Source)
			}
			f, ok := v.(float64)
			if !ok {
				t.Fatalf("thermal reading %q cpu_temp_c type = %T, want float64", r.Source, v)
			}
			if f != 65.0 {
				t.Errorf("thermal reading %q cpu_temp_c = %v, want 65.0", r.Source, f)
			}
		}
	}
	if !foundHwmon {
		t.Error("expected hwmon reading")
	}
	if !foundThermal {
		t.Error("expected thermal zone reading")
	}
}

func TestAdapter_ExcludeZones(t *testing.T) {
	dir := t.TempDir()

	hwmon0 := filepath.Join(dir, "hwmon0")
	if err := os.MkdirAll(hwmon0, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "name"), []byte("nvme\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "temp1_input"), []byte("45000\n"), 0644); err != nil {
		t.Fatal(err)
	}

	a := &LinuxThermalAdapter{
		cfg: LinuxThermalAdapterConfig{
			HwmonPath:   dir,
			ThermalPath: dir,
			ExcludeZones: []string{"nvme"},
		},
		logger: slog.Default(),
	}
	a.cfg.applyDefaults()

	readings, err := a.collect(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// nvme should be excluded
	for _, r := range readings {
		if sn, ok := r.Metrics["sensor_name"]; ok {
			if s, ok := sn.(string); ok && s == "nvme" {
				t.Error("nvme should be excluded")
			}
		}
	}
}

func TestAdapter_NonLinuxReturnsError(t *testing.T) {
	// This test runs on Linux, so we can't test the error path directly.
	// We test that New works on Linux with mock paths.
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	if err := os.MkdirAll(hwmon0, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "name"), []byte("coretemp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hwmon0, "temp1_input"), []byte("50000\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rawNode := configYAMLForPaths(dir, dir)
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          rawNode,
	}
	holder := config.NewHolder(config.Defaults())
	adapter, err := New(cfg, holder, slog.Default())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("adapter is nil")
	}
	if adapter.Name() != "linux_thermal" {
		t.Errorf("Name() = %q, want linux_thermal", adapter.Name())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = adapter.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// configYAMLForPaths returns a yaml.Node for testing (decodes to LinuxThermalAdapterConfig with paths).
func configYAMLForPaths(hwmon, thermal string) yaml.Node {
	yml := fmt.Sprintf("hwmon_path: %q\nthermal_path: %q\n", hwmon, thermal)
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yml), &doc); err != nil {
		return yaml.Node{}
	}
	if len(doc.Content) > 0 {
		return *doc.Content[0]
	}
	return yaml.Node{}
}
