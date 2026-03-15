package rocm

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"

	"gopkg.in/yaml.v3"
)

// --- Parser tests ---

func TestParseROCmOutput_ROCm6x(t *testing.T) {
	json6 := `{"gpu":[{"gpu_id":0,"temperature_edge":65.0,"gpu_use_percent":95,"vram_used_mb":1024,"vram_total_mb":8192,"average_socket_power":350.0,"throttle_status":"None","gpu_name":"AMD MI300X"},{"gpu_id":1,"temperature_edge":70.0,"gpu_use_percent":80,"vram_used_mb":2048,"vram_total_mb":8192,"average_socket_power":320.0,"throttle_status":"THERMAL"},{"gpu_id":2,"temperature_edge":60.0,"gpu_use_percent":50,"vram_used_mb":512,"vram_total_mb":8192,"average_socket_power":200.0,"throttle_status":"POWER"},{"gpu_id":3,"temperature_edge":55.0,"gpu_use_percent":10,"vram_used_mb":256,"vram_total_mb":8192,"average_socket_power":100.0,"throttle_status":"None"}]}`
	logger := slog.Default()
	readings, err := ParseROCmOutput([]byte(json6), logger)
	if err != nil {
		t.Fatalf("ParseROCmOutput: %v", err)
	}
	if len(readings) != 4 {
		t.Fatalf("got %d readings, want 4", len(readings))
	}

	// GPU 0
	r0 := readings[0]
	if r0.GPUID != 0 {
		t.Errorf("gpu 0: GPUID = %d, want 0", r0.GPUID)
	}
	if r0.GPUTemp != 65.0 {
		t.Errorf("gpu 0: GPUTemp = %f, want 65.0", r0.GPUTemp)
	}
	if r0.GPUUtilization != 95 {
		t.Errorf("gpu 0: GPUUtilization = %f, want 95", r0.GPUUtilization)
	}
	if r0.GPUMemoryUsed != 1024*1024*1024 {
		t.Errorf("gpu 0: GPUMemoryUsed = %f, want 1073741824", r0.GPUMemoryUsed)
	}
	if r0.GPUMemoryTotal != 8192*1024*1024 {
		t.Errorf("gpu 0: GPUMemoryTotal = %f, want 8589934592", r0.GPUMemoryTotal)
	}
	if r0.GPUPowerW != 350.0 {
		t.Errorf("gpu 0: GPUPowerW = %f, want 350", r0.GPUPowerW)
	}
	if r0.ThrottleReason != "none" || r0.ThrottleReasonCode != ThrottleNone {
		t.Errorf("gpu 0: throttle = %q/%f, want none/0", r0.ThrottleReason, r0.ThrottleReasonCode)
	}
	if r0.GPUModel != "MI300X" {
		t.Errorf("gpu 0: GPUModel = %q, want MI300X", r0.GPUModel)
	}

	// GPU 1 - THERMAL
	r1 := readings[1]
	if r1.ThrottleReason != "thermal_throttle" || r1.ThrottleReasonCode != ThrottleThermal {
		t.Errorf("gpu 1: throttle = %q/%f, want thermal_throttle/1", r1.ThrottleReason, r1.ThrottleReasonCode)
	}

	// GPU 2 - POWER
	r2 := readings[2]
	if r2.ThrottleReason != "power_throttle" || r2.ThrottleReasonCode != ThrottlePower {
		t.Errorf("gpu 2: throttle = %q/%f, want power_throttle/2", r2.ThrottleReason, r2.ThrottleReasonCode)
	}
}

func TestParseROCmOutput_ROCm5x(t *testing.T) {
	json5 := `{"card0":{"Temperature (Sensor edge) (C)":"65.0","GPU use (%)":"95","VRAM Total Used (MB)":"1024","VRAM Total (MB)":"8192","Current Socket Power (W)":"350.0","Throttle Status":"None","Card model":"AMD MI300X"},"card1":{"Temperature (Sensor edge) (C)":"70.0","GPU use (%)":"80","VRAM Total Used (MB)":"2048","VRAM Total (MB)":"8192","Current Socket Power (W)":"320.0","Throttle Status":"THERMAL"}}`
	logger := slog.Default()
	readings, err := ParseROCmOutput([]byte(json5), logger)
	if err != nil {
		t.Fatalf("ParseROCmOutput: %v", err)
	}
	if len(readings) != 2 {
		t.Fatalf("got %d readings, want 2", len(readings))
	}

	r0 := readings[0]
	if r0.GPUID != 0 {
		t.Errorf("gpu 0: GPUID = %d, want 0", r0.GPUID)
	}
	if r0.GPUTemp != 65.0 {
		t.Errorf("gpu 0: GPUTemp = %f, want 65.0", r0.GPUTemp)
	}
	if r0.GPUUtilization != 95 {
		t.Errorf("gpu 0: GPUUtilization = %f, want 95", r0.GPUUtilization)
	}
	if r0.GPUMemoryUsed != 1024*1024*1024 {
		t.Errorf("gpu 0: GPUMemoryUsed = %f", r0.GPUMemoryUsed)
	}
	if r0.GPUMemoryTotal != 8192*1024*1024 {
		t.Errorf("gpu 0: GPUMemoryTotal = %f", r0.GPUMemoryTotal)
	}
	if r0.ThrottleReason != "none" {
		t.Errorf("gpu 0: ThrottleReason = %q, want none", r0.ThrottleReason)
	}

	r1 := readings[1]
	if r1.ThrottleReason != "thermal_throttle" {
		t.Errorf("gpu 1: ThrottleReason = %q, want thermal_throttle", r1.ThrottleReason)
	}
}

func TestParseROCmOutput_Partial(t *testing.T) {
	// Only temp and util, no memory or power
	jsonPartial := `{"gpu":[{"gpu_id":0,"temperature_edge":65.0,"gpu_use_percent":95}]}`
	logger := slog.Default()
	readings, err := ParseROCmOutput([]byte(jsonPartial), logger)
	if err != nil {
		t.Fatalf("ParseROCmOutput: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("got %d readings, want 1", len(readings))
	}
	r := readings[0]
	if r.GPUTemp != 65.0 || r.GPUUtilization != 95 {
		t.Errorf("partial: got temp=%f util=%f, want 65 95", r.GPUTemp, r.GPUUtilization)
	}
	if r.GPUMemoryUsed != 0 || r.GPUMemoryTotal != 0 {
		t.Errorf("partial: expected zero memory, got used=%f total=%f", r.GPUMemoryUsed, r.GPUMemoryTotal)
	}
}

func TestParseROCmOutput_InvalidJSON(t *testing.T) {
	logger := slog.Default()
	_, err := ParseROCmOutput([]byte(`{invalid json`), logger)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseROCmOutput_Empty(t *testing.T) {
	logger := slog.Default()
	_, err := ParseROCmOutput([]byte(""), logger)
	if err == nil {
		t.Fatal("expected error for empty output")
	}
}

// --- Normalization tests ---

func TestNormalizeThrottle(t *testing.T) {
	tests := []struct {
		in       string
		wantStr  string
		wantCode float64
	}{
		{"THERMAL", "thermal_throttle", ThrottleThermal},
		{"thermal", "thermal_throttle", ThrottleThermal},
		{"POWER", "power_throttle", ThrottlePower},
		{"CURRENT", "power_throttle", ThrottlePower},
		{"None", "none", ThrottleNone},
		{"", "none", ThrottleNone},
		{"0", "none", ThrottleNone},
	}
	for _, tt := range tests {
		gotStr, gotCode := normalizeThrottle(tt.in, nil)
		if gotStr != tt.wantStr || gotCode != tt.wantCode {
			t.Errorf("normalizeThrottle(%q) = %q, %f; want %q, %f", tt.in, gotStr, gotCode, tt.wantStr, tt.wantCode)
		}
	}
}

func TestToRawReading_CanonicalKeys(t *testing.T) {
	r := GPUReading{
		GPUID:            0,
		GPUTemp:          65.0,
		GPUUtilization:   95.0,
		GPUMemoryUsed:    1e9,
		GPUMemoryTotal:   8e9,
		GPUPowerW:        350.0,
		ThrottleReason:   "thermal_throttle",
		ThrottleReasonCode: ThrottleThermal,
		GPUModel:         "MI300X",
	}
	raw := r.ToRawReading("host-01")

	if raw.AdapterName != "rocm" {
		t.Errorf("AdapterName = %q, want rocm", raw.AdapterName)
	}
	if raw.Source != "host-01" {
		t.Errorf("Source = %q, want host-01", raw.Source)
	}

	expectedKeys := map[string]bool{
		MetricGPUTemp: true, MetricGPUUtilization: true, MetricGPUMemoryUsed: true,
		MetricGPUMemoryTotal: true, MetricGPUPowerW: true, MetricThrottleReason: true,
		MetricThrottleReasonCode: true, MetricGPUID: true, MetricGPUVendor: true, MetricGPUModel: true,
	}
	for k := range expectedKeys {
		if _, ok := raw.Metrics[k]; !ok {
			t.Errorf("missing metric %q", k)
		}
	}
	if raw.Metrics[MetricGPUTemp].(float64) != 65.0 {
		t.Errorf("gpu_temp = %v, want 65.0", raw.Metrics[MetricGPUTemp])
	}
	if raw.Metrics[MetricGPUUtilization].(float64) != 95.0 {
		t.Errorf("gpu_utilization = %v, want 95.0", raw.Metrics[MetricGPUUtilization])
	}
	if raw.Metrics[MetricThrottleReason].(string) != "thermal_throttle" {
		t.Errorf("throttle_reason = %v, want thermal_throttle", raw.Metrics[MetricThrottleReason])
	}
	if raw.Metrics[MetricGPUVendor].(string) != "amd" {
		t.Errorf("gpu_vendor = %v, want amd", raw.Metrics[MetricGPUVendor])
	}
}

// --- Integration test with mock rocm-smi ---

func createMockROCmSMI(t *testing.T, jsonOutput string) string {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "rocm-smi")
	dataPath := filepath.Join(dir, "output.json")
	if err := os.WriteFile(dataPath, []byte(jsonOutput), 0644); err != nil {
		t.Fatalf("write mock data: %v", err)
	}
	script := "#!/bin/sh\n"
	script += "if [ \"$1\" = \"--help\" ]; then exit 0; fi\n"
	script += "cat " + dataPath + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}
	return scriptPath
}

func rocmConfigWithPath(t *testing.T, path string) config.AdapterConfig {
	t.Helper()
	raw := `
enabled: true
poll_interval: "50ms"
rocm_smi_path: "` + path + `"
collection_method: "cli"
gpu_indices: []
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return config.AdapterConfig{
		Enabled:      true,
		PollInterval: 50 * time.Millisecond,
		Raw:          *node.Content[0],
	}
}

func TestCollector_WithMock(t *testing.T) {
	jsonOut := `{"gpu":[{"gpu_id":0,"temperature_edge":72.5,"gpu_use_percent":88,"vram_used_mb":4096,"vram_total_mb":8192,"average_socket_power":400.0,"throttle_status":"None"}]}`
	mockPath := createMockROCmSMI(t, jsonOut)

	collector := NewROCmCollector(mockPath, nil, slog.Default())
	ctx := context.Background()
	readings, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("got %d readings, want 1", len(readings))
	}
	r := readings[0]
	if r.GPUTemp != 72.5 || r.GPUUtilization != 88 {
		t.Errorf("got temp=%f util=%f, want 72.5 88", r.GPUTemp, r.GPUUtilization)
	}
}

func TestCheckROCmSMIAvailable_NotFound(t *testing.T) {
	err := CheckROCmSMIAvailable("/nonexistent/rocm-smi-path")
	if err == nil {
		t.Fatal("expected error when rocm-smi not found")
	}
}

func TestCollector_MockExitsWithError(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "rocm-smi")
	script := "#!/bin/sh\nif [ \"$1\" = \"--help\" ]; then exit 0; fi\nexit 1\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock: %v", err)
	}

	collector := NewROCmCollector(scriptPath, nil, slog.Default())
	_, err := collector.Collect(context.Background())
	if err == nil {
		t.Fatal("expected error when mock exits with code 1")
	}
}

func TestNew_CollectionMethodLibrary(t *testing.T) {
	raw := `
enabled: true
poll_interval: "10s"
rocm_smi_path: "/opt/rocm/bin/rocm-smi"
collection_method: "library"
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 10 * time.Second,
		Raw:          *node.Content[0],
	}
	_, err := New(cfg, nil, slog.Default())
	if err == nil {
		t.Fatal("expected error for library collection method")
	}
	if !strings.Contains(err.Error(), "library") {
		t.Errorf("error should mention library: %v", err)
	}
}

func TestNew_ValidConfig(t *testing.T) {
	jsonOut := `{"gpu":[{"gpu_id":0}]}`
	mockPath := createMockROCmSMI(t, jsonOut)
	raw := `
enabled: true
poll_interval: "10s"
rocm_smi_path: "` + mockPath + `"
collection_method: "cli"
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg := config.AdapterConfig{Raw: *node.Content[0]}
	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Name() != "rocm" {
		t.Errorf("Name() = %q, want rocm", a.Name())
	}
}

func TestAdapter_WithMock(t *testing.T) {
	jsonOut := `{"gpu":[{"gpu_id":0,"temperature_edge":65.0,"gpu_use_percent":95,"vram_used_mb":1024,"vram_total_mb":8192,"average_socket_power":350.0,"throttle_status":"None","gpu_name":"AMD MI300X"}]}`
	mockPath := createMockROCmSMI(t, jsonOut)
	cfg := rocmConfigWithPath(t, mockPath)

	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = a.Start(ctx)
	}()

	var raw adapter.RawReading
	select {
	case raw = <-a.Readings():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reading")
	}

	cancel()
	_ = a.Stop(context.Background())

	if raw.AdapterName != "rocm" {
		t.Errorf("AdapterName = %q, want rocm", raw.AdapterName)
	}
	if _, ok := raw.Metrics[MetricGPUTemp]; !ok {
		t.Errorf("missing gpu_temp in metrics")
	}
	if _, ok := raw.Metrics[MetricGPUUtilization]; !ok {
		t.Errorf("missing gpu_utilization in metrics")
	}
	if _, ok := raw.Metrics[MetricGPUMemoryUsed]; !ok {
		t.Errorf("missing gpu_memory_used in metrics")
	}
	if _, ok := raw.Metrics[MetricGPUPowerW]; !ok {
		t.Errorf("missing gpu_power_w in metrics")
	}
	if _, ok := raw.Metrics[MetricThrottleReasonCode]; !ok {
		t.Errorf("missing throttle_reason_code in metrics")
	}
	if raw.Metrics[MetricGPUVendor] != "amd" {
		t.Errorf("gpu_vendor = %v, want amd", raw.Metrics[MetricGPUVendor])
	}
}

func TestAdapter_IsRunningAndStats(t *testing.T) {
	jsonOut := `{"gpu":[{"gpu_id":0,"temperature_edge":65.0}]}`
	mockPath := createMockROCmSMI(t, jsonOut)
	cfg := rocmConfigWithPath(t, mockPath)

	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rocma := a.(*ROCmAdapter)

	if rocma.IsRunning() {
		t.Error("IsRunning() should be false before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = rocma.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	if !rocma.IsRunning() {
		t.Error("IsRunning() should be true after Start")
	}
	pollCount, errCount, lastPoll, lastErr, _ := rocma.Stats()
	if pollCount < 1 {
		t.Errorf("pollCount = %d, want >= 1", pollCount)
	}
	if !lastPoll.IsZero() && lastPoll.Before(time.Now().Add(-time.Minute)) {
		t.Error("lastPoll should be recent")
	}
	_ = errCount
	_ = lastErr

	cancel()
	_ = a.Stop(context.Background())
}

func TestCollector_FilterGPUIndices(t *testing.T) {
	jsonOut := `{"gpu":[{"gpu_id":0,"temperature_edge":65.0},{"gpu_id":1,"temperature_edge":70.0},{"gpu_id":2,"temperature_edge":60.0}]}`
	mockPath := createMockROCmSMI(t, jsonOut)
	collector := NewROCmCollector(mockPath, []int{0, 2}, slog.Default())
	readings, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(readings) != 2 {
		t.Fatalf("got %d readings, want 2 (filtered to gpu 0 and 2)", len(readings))
	}
	ids := map[int]bool{}
	for _, r := range readings {
		ids[r.GPUID] = true
	}
	if !ids[0] || !ids[2] || ids[1] {
		t.Errorf("expected gpu 0 and 2, got %v", ids)
	}
}

func TestDetectGPUModel(t *testing.T) {
	if got := detectGPUModel("AMD MI300X"); got != "MI300X" {
		t.Errorf("detectGPUModel(MI300X) = %q", got)
	}
	if got := detectGPUModel("AMD MI355X"); got != "MI355X" {
		t.Errorf("detectGPUModel(MI355X) = %q", got)
	}
	if got := detectGPUModel("Unknown GPU"); got != "Unknown GPU" {
		t.Errorf("detectGPUModel(Unknown GPU) = %q, want %q", got, "Unknown GPU")
	}
}
