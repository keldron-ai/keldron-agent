// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package linux_thermal

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
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
		{"CORETEMP", "cpu"},
		{"AmdGpu", "gpu"},
		{"CPU_freq", "cpu"},
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

	// hwmon0: coretemp with label, max, crit
	hwmon0 := filepath.Join(dir, "hwmon0")
	mustMkdirAll(t, hwmon0)
	mustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("62000\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_label"), []byte("Package id 0\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_max"), []byte("100000\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_crit"), []byte("105000\n"))

	// hwmon1: nvme (no label, no max/crit)
	hwmon1 := filepath.Join(dir, "hwmon1")
	mustMkdirAll(t, hwmon1)
	mustWriteFile(t, filepath.Join(hwmon1, "name"), []byte("nvme\n"))
	mustWriteFile(t, filepath.Join(hwmon1, "temp1_input"), []byte("45000\n"))

	logger := slog.Default()
	sensors := DiscoverHwmon(dir, logger)
	if len(sensors) != 2 {
		t.Fatalf("got %d sensors, want 2", len(sensors))
	}
	var foundCore, foundNvme bool
	for _, s := range sensors {
		if s.Name == "coretemp" && s.SensorType == "cpu" && s.TempC == 62.0 && s.Label == "Package id 0" {
			foundCore = true
			if s.TempMaxC != 100.0 {
				t.Errorf("coretemp TempMaxC = %v, want 100.0", s.TempMaxC)
			}
			if s.TempCritC != 105.0 {
				t.Errorf("coretemp TempCritC = %v, want 105.0", s.TempCritC)
			}
		}
		if s.Name == "nvme" && s.SensorType == "nvme" && s.TempC == 45.0 {
			foundNvme = true
			if s.TempMaxC != -1.0 {
				t.Errorf("nvme TempMaxC = %v, want -1.0", s.TempMaxC)
			}
			if s.TempCritC != -1.0 {
				t.Errorf("nvme TempCritC = %v, want -1.0", s.TempCritC)
			}
		}
	}
	if !foundCore {
		t.Error("expected coretemp sensor with 62.0°C, cpu type")
	}
	if !foundNvme {
		t.Error("expected nvme sensor with 45.0°C")
	}
}

func TestDiscoverHwmon_MultipleTempInputs(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	mustMkdirAll(t, hwmon0)
	mustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("62000\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_label"), []byte("Package id 0\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp2_input"), []byte("58000\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp2_label"), []byte("Core 0\n"))

	sensors := DiscoverHwmon(dir, slog.Default())
	if len(sensors) != 2 {
		t.Fatalf("got %d sensors, want 2", len(sensors))
	}
	for _, s := range sensors {
		if s.Name != "coretemp" || s.SensorType != "cpu" {
			t.Errorf("unexpected sensor: %+v", s)
		}
	}
}

func TestDiscoverHwmon_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	sensors := DiscoverHwmon(dir, slog.Default())
	if len(sensors) != 0 {
		t.Errorf("empty dir: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_NonexistentDir(t *testing.T) {
	sensors := DiscoverHwmon("/nonexistent/path", slog.Default())
	if len(sensors) != 0 {
		t.Errorf("nonexistent dir: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_NilLogger(t *testing.T) {
	sensors := DiscoverHwmon("/nonexistent/path", nil)
	if len(sensors) != 0 {
		t.Errorf("nil logger: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_MissingNameFile(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	mustMkdirAll(t, hwmon0)
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("62000\n"))
	// no "name" file

	sensors := DiscoverHwmon(dir, slog.Default())
	if len(sensors) != 0 {
		t.Errorf("missing name file: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_MissingTempFile(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	mustMkdirAll(t, hwmon0)
	mustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	// No temp1_input

	sensors := DiscoverHwmon(dir, slog.Default())
	if len(sensors) != 0 {
		t.Errorf("missing temp file: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_MalformedValue(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	mustMkdirAll(t, hwmon0)
	mustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("bad\n"))

	sensors := DiscoverHwmon(dir, slog.Default())
	if len(sensors) != 0 {
		t.Errorf("malformed value: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_MalformedMaxCrit(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	mustMkdirAll(t, hwmon0)
	mustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("62000\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_max"), []byte("bad\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_crit"), []byte("bad\n"))

	sensors := DiscoverHwmon(dir, slog.Default())
	if len(sensors) != 1 {
		t.Fatalf("got %d sensors, want 1", len(sensors))
	}
	if sensors[0].TempMaxC != -1.0 {
		t.Errorf("malformed max: TempMaxC = %v, want -1.0", sensors[0].TempMaxC)
	}
	if sensors[0].TempCritC != -1.0 {
		t.Errorf("malformed crit: TempCritC = %v, want -1.0", sensors[0].TempCritC)
	}
}

func TestDiscoverHwmon_NegativeTemperature(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	mustMkdirAll(t, hwmon0)
	mustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	mustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("-5000\n"))

	sensors := DiscoverHwmon(dir, slog.Default())
	if len(sensors) != 1 {
		t.Fatalf("got %d sensors, want 1", len(sensors))
	}
	if sensors[0].TempC != -5.0 {
		t.Errorf("negative temp: TempC = %v, want -5.0", sensors[0].TempC)
	}
}

func TestDiscoverHwmon_SkipsNonHwmonDirs(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "notahwmon"))
	mustWriteFile(t, filepath.Join(dir, "somefile"), []byte("data"))

	sensors := DiscoverHwmon(dir, slog.Default())
	if len(sensors) != 0 {
		t.Errorf("non-hwmon entries: got %d sensors, want 0", len(sensors))
	}
}

func TestDiscoverHwmon_EmptyBasePath(t *testing.T) {
	sensors := DiscoverHwmon("", slog.Default())
	if _, err := os.ReadDir(defaultHwmonPath); err != nil {
		// Path missing or unreadable — expect nil
		if sensors != nil {
			t.Errorf("expected nil sensors when %s is unreadable, got %d", defaultHwmonPath, len(sensors))
		}
	}
}

func TestDiscoverThermalZones_MockSysfs(t *testing.T) {
	dir := t.TempDir()
	zone0 := filepath.Join(dir, "thermal_zone0")
	mustMkdirAll(t, zone0)
	mustWriteFile(t, filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"))
	mustWriteFile(t, filepath.Join(zone0, "temp"), []byte("65000\n"))

	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 1 {
		t.Fatalf("got %d zones, want 1", len(zones))
	}
	z := zones[0]
	if z.Zone != "thermal_zone0" || z.Type != "x86_pkg_temp" || z.TempC != 65.0 {
		t.Errorf("zone: got %+v", z)
	}
}

func TestDiscoverThermalZones_WithTripPoints(t *testing.T) {
	dir := t.TempDir()
	zone0 := filepath.Join(dir, "thermal_zone0")
	mustMkdirAll(t, zone0)
	mustWriteFile(t, filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"))
	mustWriteFile(t, filepath.Join(zone0, "temp"), []byte("65000\n"))
	mustWriteFile(t, filepath.Join(zone0, "trip_point_0_type"), []byte("passive\n"))
	mustWriteFile(t, filepath.Join(zone0, "trip_point_0_temp"), []byte("90000\n"))
	mustWriteFile(t, filepath.Join(zone0, "trip_point_1_type"), []byte("critical\n"))
	mustWriteFile(t, filepath.Join(zone0, "trip_point_1_temp"), []byte("105000\n"))

	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 1 {
		t.Fatalf("got %d zones, want 1", len(zones))
	}
	z := zones[0]
	if len(z.TripPoints) != 2 {
		t.Fatalf("got %d trip points, want 2", len(z.TripPoints))
	}
	if z.TripPoints[0].Type != "passive" || z.TripPoints[0].TempC != 90.0 {
		t.Errorf("trip point 0: got %+v", z.TripPoints[0])
	}
	if z.TripPoints[1].Type != "critical" || z.TripPoints[1].TempC != 105.0 {
		t.Errorf("trip point 1: got %+v", z.TripPoints[1])
	}
}

func TestDiscoverThermalZones_MalformedTripPointTemp(t *testing.T) {
	dir := t.TempDir()
	zone0 := filepath.Join(dir, "thermal_zone0")
	mustMkdirAll(t, zone0)
	mustWriteFile(t, filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"))
	mustWriteFile(t, filepath.Join(zone0, "temp"), []byte("65000\n"))
	mustWriteFile(t, filepath.Join(zone0, "trip_point_0_type"), []byte("passive\n"))
	mustWriteFile(t, filepath.Join(zone0, "trip_point_0_temp"), []byte("bad\n"))
	mustWriteFile(t, filepath.Join(zone0, "trip_point_1_type"), []byte("critical\n"))
	mustWriteFile(t, filepath.Join(zone0, "trip_point_1_temp"), []byte("105000\n"))

	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 1 {
		t.Fatalf("got %d zones, want 1", len(zones))
	}
	// Malformed trip_point_0 should be skipped, trip_point_1 should be present
	if len(zones[0].TripPoints) != 1 {
		t.Fatalf("got %d trip points, want 1 (malformed one skipped)", len(zones[0].TripPoints))
	}
	if zones[0].TripPoints[0].Type != "critical" {
		t.Errorf("expected critical trip point, got %q", zones[0].TripPoints[0].Type)
	}
}

func TestDiscoverThermalZones_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 0 {
		t.Errorf("empty dir: got %d zones, want 0", len(zones))
	}
}

func TestDiscoverThermalZones_NonexistentDir(t *testing.T) {
	zones := DiscoverThermalZones("/nonexistent/path", slog.Default())
	if len(zones) != 0 {
		t.Errorf("nonexistent dir: got %d zones, want 0", len(zones))
	}
}

func TestDiscoverThermalZones_NilLogger(t *testing.T) {
	zones := DiscoverThermalZones("/nonexistent/path", nil)
	if len(zones) != 0 {
		t.Errorf("nil logger: got %d zones, want 0", len(zones))
	}
}

func TestDiscoverThermalZones_MissingTypeFile(t *testing.T) {
	dir := t.TempDir()
	zone0 := filepath.Join(dir, "thermal_zone0")
	mustMkdirAll(t, zone0)
	mustWriteFile(t, filepath.Join(zone0, "temp"), []byte("65000\n"))
	// No "type" file

	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 0 {
		t.Errorf("missing type file: got %d zones, want 0", len(zones))
	}
}

func TestDiscoverThermalZones_MissingTempFile(t *testing.T) {
	dir := t.TempDir()
	zone0 := filepath.Join(dir, "thermal_zone0")
	mustMkdirAll(t, zone0)
	mustWriteFile(t, filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"))
	// No "temp" file

	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 0 {
		t.Errorf("missing temp file: got %d zones, want 0", len(zones))
	}
}

func TestDiscoverThermalZones_MalformedTemp(t *testing.T) {
	dir := t.TempDir()
	zone0 := filepath.Join(dir, "thermal_zone0")
	mustMkdirAll(t, zone0)
	mustWriteFile(t, filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"))
	mustWriteFile(t, filepath.Join(zone0, "temp"), []byte("notanumber\n"))

	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 0 {
		t.Errorf("malformed temp: got %d zones, want 0", len(zones))
	}
}

func TestDiscoverThermalZones_SkipsNonZoneDirs(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "cooling_device0"))
	mustWriteFile(t, filepath.Join(dir, "somefile"), []byte("data"))

	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 0 {
		t.Errorf("non-zone entries: got %d zones, want 0", len(zones))
	}
}

func TestDiscoverThermalZones_MultipleZones(t *testing.T) {
	dir := t.TempDir()
	for i, zt := range []struct{ typ, temp string }{
		{"x86_pkg_temp", "65000"},
		{"acpitz", "50000"},
	} {
		zone := filepath.Join(dir, fmt.Sprintf("thermal_zone%d", i))
		mustMkdirAll(t, zone)
		mustWriteFile(t, filepath.Join(zone, "type"), []byte(zt.typ+"\n"))
		mustWriteFile(t, filepath.Join(zone, "temp"), []byte(zt.temp+"\n"))
	}

	zones := DiscoverThermalZones(dir, slog.Default())
	if len(zones) != 2 {
		t.Fatalf("got %d zones, want 2", len(zones))
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	c := &LinuxThermalAdapterConfig{}
	c.applyDefaults()
	if c.HwmonPath != defaultHwmonPath {
		t.Errorf("HwmonPath = %q, want %q", c.HwmonPath, defaultHwmonPath)
	}
	if c.ThermalPath != defaultThermalPath {
		t.Errorf("ThermalPath = %q, want %q", c.ThermalPath, defaultThermalPath)
	}
}

func TestConfig_ApplyDefaults_PreservesCustom(t *testing.T) {
	c := &LinuxThermalAdapterConfig{
		HwmonPath:   "/custom/hwmon",
		ThermalPath: "/custom/thermal",
	}
	c.applyDefaults()
	if c.HwmonPath != "/custom/hwmon" {
		t.Errorf("HwmonPath = %q, want /custom/hwmon", c.HwmonPath)
	}
	if c.ThermalPath != "/custom/thermal" {
		t.Errorf("ThermalPath = %q, want /custom/thermal", c.ThermalPath)
	}
}
