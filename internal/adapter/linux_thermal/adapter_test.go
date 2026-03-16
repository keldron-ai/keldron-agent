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

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"gopkg.in/yaml.v3"
)

// RawReading type alias for use in tests that construct adapters directly.
type RawReading = adapter.RawReading

func adapterMustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func adapterMustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// waitForCondition polls fn every 10ms until it returns true or timeout expires.
func waitForCondition(t *testing.T, fn func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !fn() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !fn() {
		t.Fatal(msg)
	}
}

func TestAdapter_Collect(t *testing.T) {
	dir := t.TempDir()

	hwmon0 := filepath.Join(dir, "hwmon0")
	adapterMustMkdirAll(t, hwmon0)
	adapterMustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	adapterMustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("62000\n"))

	zone0 := filepath.Join(dir, "thermal_zone0")
	adapterMustMkdirAll(t, zone0)
	adapterMustWriteFile(t, filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"))
	adapterMustWriteFile(t, filepath.Join(zone0, "temp"), []byte("65000\n"))

	ltCfg := LinuxThermalAdapterConfig{
		HwmonPath:   dir,
		ThermalPath: dir,
	}
	ltCfg.applyDefaults()

	a := &LinuxThermalAdapter{
		cfg:    ltCfg,
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
			if f, ok := v.(float64); !ok || f != 62.0 {
				t.Errorf("hwmon reading cpu_temp_c = %v, want 62.0", v)
			}
			if bc, ok := r.Metrics["behavior_class"]; !ok || bc != "sbc_constrained" {
				t.Errorf("hwmon reading behavior_class = %v, want sbc_constrained", bc)
			}
		}
		if r.Source == "thermal_zone0" {
			foundThermal = true
			v, ok := r.Metrics["cpu_temp_c"]
			if !ok {
				t.Fatalf("thermal reading %q missing cpu_temp_c metric", r.Source)
			}
			if f, ok := v.(float64); !ok || f != 65.0 {
				t.Errorf("thermal reading cpu_temp_c = %v, want 65.0", v)
			}
			if bc, ok := r.Metrics["behavior_class"]; !ok || bc != "sbc_constrained" {
				t.Errorf("thermal reading behavior_class = %v, want sbc_constrained", bc)
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

func TestAdapter_CollectGPUSensor(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	adapterMustMkdirAll(t, hwmon0)
	adapterMustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("amdgpu\n"))
	adapterMustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("70000\n"))

	ltCfg := LinuxThermalAdapterConfig{HwmonPath: dir, ThermalPath: t.TempDir()}
	ltCfg.applyDefaults()
	a := &LinuxThermalAdapter{cfg: ltCfg, logger: slog.Default()}

	readings, err := a.collect(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 1 {
		t.Fatalf("got %d readings, want 1", len(readings))
	}
	if _, ok := readings[0].Metrics["temperature_c"]; !ok {
		t.Error("GPU sensor should use temperature_c metric key")
	}
	if readings[0].Metrics["sensor_type"] != "gpu" {
		t.Errorf("sensor_type = %v, want gpu", readings[0].Metrics["sensor_type"])
	}
}

func TestAdapter_ExcludeZones(t *testing.T) {
	dir := t.TempDir()

	hwmon0 := filepath.Join(dir, "hwmon0")
	adapterMustMkdirAll(t, hwmon0)
	adapterMustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("nvme\n"))
	adapterMustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("45000\n"))

	hwmon1 := filepath.Join(dir, "hwmon1")
	adapterMustMkdirAll(t, hwmon1)
	adapterMustWriteFile(t, filepath.Join(hwmon1, "name"), []byte("coretemp\n"))
	adapterMustWriteFile(t, filepath.Join(hwmon1, "temp1_input"), []byte("55000\n"))

	a := &LinuxThermalAdapter{
		cfg: LinuxThermalAdapterConfig{
			HwmonPath:    dir,
			ThermalPath:  t.TempDir(),
			ExcludeZones: []string{"nvme"},
		},
		logger: slog.Default(),
	}

	readings, err := a.collect(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range readings {
		if sn, ok := r.Metrics["sensor_name"]; ok {
			if s, ok := sn.(string); ok && s == "nvme" {
				t.Error("nvme should be excluded")
			}
		}
	}
	if len(readings) != 1 {
		t.Errorf("got %d readings, want 1 (coretemp only)", len(readings))
	}
}

func TestAdapter_IncludeZones(t *testing.T) {
	dir := t.TempDir()

	hwmon0 := filepath.Join(dir, "hwmon0")
	adapterMustMkdirAll(t, hwmon0)
	adapterMustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("nvme\n"))
	adapterMustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("45000\n"))

	hwmon1 := filepath.Join(dir, "hwmon1")
	adapterMustMkdirAll(t, hwmon1)
	adapterMustWriteFile(t, filepath.Join(hwmon1, "name"), []byte("coretemp\n"))
	adapterMustWriteFile(t, filepath.Join(hwmon1, "temp1_input"), []byte("55000\n"))

	a := &LinuxThermalAdapter{
		cfg: LinuxThermalAdapterConfig{
			HwmonPath:    dir,
			ThermalPath:  t.TempDir(),
			IncludeZones: []string{"coretemp"},
		},
		logger: slog.Default(),
	}

	readings, err := a.collect(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 1 {
		t.Fatalf("got %d readings, want 1 (coretemp only)", len(readings))
	}
	if readings[0].Metrics["sensor_name"] != "coretemp" {
		t.Errorf("expected coretemp, got %v", readings[0].Metrics["sensor_name"])
	}
}

func TestAdapter_IncludeZones_ThermalZone(t *testing.T) {
	dir := t.TempDir()
	zone0 := filepath.Join(dir, "thermal_zone0")
	adapterMustMkdirAll(t, zone0)
	adapterMustWriteFile(t, filepath.Join(zone0, "type"), []byte("x86_pkg_temp\n"))
	adapterMustWriteFile(t, filepath.Join(zone0, "temp"), []byte("65000\n"))

	zone1 := filepath.Join(dir, "thermal_zone1")
	adapterMustMkdirAll(t, zone1)
	adapterMustWriteFile(t, filepath.Join(zone1, "type"), []byte("acpitz\n"))
	adapterMustWriteFile(t, filepath.Join(zone1, "temp"), []byte("50000\n"))

	a := &LinuxThermalAdapter{
		cfg: LinuxThermalAdapterConfig{
			HwmonPath:    t.TempDir(),
			ThermalPath:  dir,
			IncludeZones: []string{"x86_pkg_temp"},
		},
		logger: slog.Default(),
	}

	readings, err := a.collect(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 1 {
		t.Fatalf("got %d readings, want 1", len(readings))
	}
	if readings[0].Source != "thermal_zone0" {
		t.Errorf("expected thermal_zone0, got %v", readings[0].Source)
	}
}

func TestAdapter_MetricKeyForSensorType(t *testing.T) {
	a := &LinuxThermalAdapter{}
	tests := []struct {
		sensorType string
		want       string
	}{
		{"gpu", "temperature_c"},
		{"cpu", "cpu_temp_c"},
		{"nvme", "cpu_temp_c"},
		{"soc", "cpu_temp_c"},
		{"other", "cpu_temp_c"},
	}
	for _, tt := range tests {
		t.Run(tt.sensorType, func(t *testing.T) {
			if got := a.metricKeyForSensorType(tt.sensorType); got != tt.want {
				t.Errorf("metricKeyForSensorType(%q) = %q, want %q", tt.sensorType, got, tt.want)
			}
		})
	}
}

func TestAdapter_PollAndStats(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	adapterMustMkdirAll(t, hwmon0)
	adapterMustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	adapterMustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("50000\n"))

	a := &LinuxThermalAdapter{
		cfg: LinuxThermalAdapterConfig{
			HwmonPath:   dir,
			ThermalPath: t.TempDir(),
		},
		readings: make(chan RawReading, channelBuffer),
		logger:   slog.Default(),
	}

	if a.IsRunning() {
		t.Error("should not be running before Start")
	}

	a.poll()

	pollCount, errorCount, lastPoll, lastError, _ := a.Stats()
	if pollCount != 1 {
		t.Errorf("pollCount = %d, want 1", pollCount)
	}
	if errorCount != 0 {
		t.Errorf("errorCount = %d, want 0", errorCount)
	}
	if lastPoll.IsZero() {
		t.Error("lastPoll should not be zero")
	}
	if lastError != "" {
		t.Errorf("lastError = %q, want empty", lastError)
	}

	// Drain reading
	select {
	case r := <-a.readings:
		if r.AdapterName != "linux_thermal" {
			t.Errorf("reading adapter = %q", r.AdapterName)
		}
	default:
		t.Error("expected a reading from poll")
	}
}

func TestAdapter_Name(t *testing.T) {
	a := &LinuxThermalAdapter{}
	if a.Name() != "linux_thermal" {
		t.Errorf("Name() = %q, want linux_thermal", a.Name())
	}
}

func TestAdapter_StartStop(t *testing.T) {
	dir := t.TempDir()
	hwmon0 := filepath.Join(dir, "hwmon0")
	adapterMustMkdirAll(t, hwmon0)
	adapterMustWriteFile(t, filepath.Join(hwmon0, "name"), []byte("coretemp\n"))
	adapterMustWriteFile(t, filepath.Join(hwmon0, "temp1_input"), []byte("50000\n"))

	rawNode := configYAMLForPaths(dir, t.TempDir())
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          rawNode,
	}
	holder := config.NewHolder(config.Defaults())
	adpt, err := New(cfg, holder, slog.Default())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if adpt.Name() != "linux_thermal" {
		t.Errorf("Name() = %q, want linux_thermal", adpt.Name())
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- adpt.Start(ctx)
	}()

	lta := adpt.(*LinuxThermalAdapter)
	waitForCondition(t, lta.IsRunning, 2*time.Second, "adapter did not enter running state")

	// Stop via Stop method
	if err := adpt.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestAdapter_DoubleStartReturnsError(t *testing.T) {
	a := &LinuxThermalAdapter{
		cfg: LinuxThermalAdapterConfig{
			HwmonPath:   t.TempDir(),
			ThermalPath: t.TempDir(),
		},
		readings: make(chan RawReading, channelBuffer),
		logger:   slog.Default(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.Start(ctx) }()

	waitForCondition(t, a.IsRunning, 2*time.Second, "first Start did not enter running state")

	// Second start should fail
	err := a.Start(ctx)
	if err == nil {
		t.Error("expected error on double start")
	}
	cancel()

	waitForCondition(t, func() bool { return !a.IsRunning() }, 2*time.Second, "adapter did not stop")
}

func TestAdapter_ReadingsChannel(t *testing.T) {
	a := &LinuxThermalAdapter{
		readings: make(chan RawReading, channelBuffer),
	}
	ch := a.Readings()
	if ch == nil {
		t.Fatal("Readings() returned nil")
	}
}

func TestAdapter_IsExcluded(t *testing.T) {
	a := &LinuxThermalAdapter{
		cfg: LinuxThermalAdapterConfig{
			ExcludeZones: []string{"nvme", "ACPI"},
		},
	}
	tests := []struct {
		name string
		want bool
	}{
		{"nvme", true},
		{"nvme0", true},
		{"NVME", true},
		{"coretemp", false},
		{"acpi_thermal", true},
		{"random", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := a.isExcluded(tt.name); got != tt.want {
				t.Errorf("isExcluded(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestAdapter_IsIncluded(t *testing.T) {
	a := &LinuxThermalAdapter{
		cfg: LinuxThermalAdapterConfig{
			IncludeZones: []string{"coretemp", "amdgpu"},
		},
	}
	tests := []struct {
		name string
		want bool
	}{
		{"coretemp", true},
		{"CORETEMP", true},
		{"amdgpu", true},
		{"nvme", false},
		{"random", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := a.isIncluded(tt.name); got != tt.want {
				t.Errorf("isIncluded(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// configYAMLForPaths returns a yaml.Node for testing.
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
