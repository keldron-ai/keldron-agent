//go:build integration

package integration

import (
	"context"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter/rocm"
	"github.com/keldron-ai/keldron-agent/internal/adapter/slurm"
	"github.com/keldron-ai/keldron-agent/internal/adapter/snmp_pdu"
	"github.com/keldron-ai/keldron-agent/internal/adapter/temperature"
)

const (
	snmpHost   = "localhost"
	snmpPort   = 1161
	modbusHost = "localhost:1502"
	slurmURL   = "http://localhost:6820"
)

func TestSNMPPDUPoll_Integration(t *testing.T) {
	target := snmp_pdu.PDUTarget{
		Address: "localhost:1161",
		PDUID:   "pdu-sim",
		RackIDs: []string{"rack-01"},
		Vendor:  "servertech",
	}
	cfg := &snmp_pdu.SNMPPDUConfig{
		Version:   "v2c",
		Community: "public",
		Targets:   []snmp_pdu.PDUTarget{target},
		Timeout:   5 * time.Second,
	}
	cfg.ApplyDefaults()

	poller, err := snmp_pdu.NewSNMPPoller(target, cfg, slog.Default())
	if err != nil {
		t.Fatalf("NewSNMPPoller: %v", err)
	}
	defer poller.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	readings, err := poller.Poll(ctx)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(readings) == 0 {
		t.Fatal("expected at least one reading")
	}

	m := readings[0].Metrics
	assertFloat(t, m, "power_kw", 5.4, 0.1)
	assertFloat(t, m, "voltage_v", 208, 1)
	assertFloat(t, m, "current_a", 26, 1)
	assertFloat(t, m, "phase_current_a_1", 8.5, 0.2)
	assertFloat(t, m, "phase_current_a_2", 9.1, 0.2)
	assertFloat(t, m, "phase_current_a_3", 8.4, 0.2)
}

func TestTemperatureSNMP_Integration(t *testing.T) {
	cfg := temperature.SensorConfig{
		Address:   "localhost:1161",
		Protocol:  "snmp",
		SensorID:  "inlet-sim",
		RackID:    "rack-01",
		Position:  "inlet",
		Community: "public",
		OID:       "1.3.6.1.4.1.99999.1.1.0",
		Encoding:  "tenths",
	}

	reading, err := temperature.PollSNMP(cfg)
	if err != nil {
		t.Fatalf("PollSNMP: %v", err)
	}
	assertFloat(t, reading.Metrics, "inlet_temp_c", 22.5, 0.1)
}

func TestTemperatureModbus_Integration(t *testing.T) {
	cfg := temperature.SensorConfig{
		Address:         modbusHost,
		Protocol:        "modbus",
		SensorID:        "inlet-modbus",
		RackID:          "rack-01",
		Position:        "inlet",
		UnitID:          1,
		RegisterAddress: 99, // 0-based PDU address; pymodbus maps to data-block address 100
		RegisterType:    "holding",
		ScaleFactor:    0.1,
	}

	reading, err := temperature.PollModbus(cfg)
	if err != nil {
		t.Fatalf("PollModbus: %v", err)
	}
	assertFloat(t, reading.Metrics, "inlet_temp_c", 22.5, 0.1)
}

func TestSlurmClient_Integration(t *testing.T) {
	client := slurm.NewSlurmClient(slurmURL, "v0.0.40", "test-token", 10*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	jobs, err := client.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) < 3 {
		t.Errorf("expected at least 3 jobs, got %d", len(jobs))
	}
	for _, j := range jobs {
		if j.State != "RUNNING" && j.State != "PENDING" {
			t.Errorf("job %d: unexpected state %q", j.JobID, j.State)
		}
		if len(j.ExpandedNodes) == 0 && j.NodeList != "" {
			t.Errorf("job %d: NodeList %q should expand to nodes", j.JobID, j.NodeList)
		}
	}

	nodes, err := client.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) < 4 {
		t.Errorf("expected at least 4 nodes, got %d", len(nodes))
	}
}

func TestROCmCollector_Integration(t *testing.T) {
	mockPath := findMockRocmSmi(t)
	collector := rocm.NewROCmCollector(mockPath, nil, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	readings, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(readings) != 4 {
		t.Fatalf("expected 4 GPUs, got %d", len(readings))
	}

	expectedTemps := []float64{65, 72, 68, 71}
	expectedUtil := []float64{95, 87, 92, 88}
	expectedPower := []float64{550, 520, 540, 510}
	for i, r := range readings {
		if math.Abs(r.GPUTemp-expectedTemps[i]) > 1 {
			t.Errorf("GPU %d: temp got %.1f, want %.1f", i, r.GPUTemp, expectedTemps[i])
		}
		if math.Abs(r.GPUUtilization-expectedUtil[i]) > 1 {
			t.Errorf("GPU %d: util got %.1f, want %.1f", i, r.GPUUtilization, expectedUtil[i])
		}
		if math.Abs(r.GPUPowerW-expectedPower[i]) > 1 {
			t.Errorf("GPU %d: power got %.1f, want %.1f", i, r.GPUPowerW, expectedPower[i])
		}
	}
}

func TestSNMP_SimulatorDown(t *testing.T) {
	// UDP is connectionless, so NewSNMPPoller won't fail at creation.
	// Instead, verify that Poll returns an error (timeout) against a
	// non-existent endpoint.
	target := snmp_pdu.PDUTarget{
		Address: "127.0.0.1:19999",
		PDUID:   "pdu-down",
		RackIDs: []string{"rack-01"},
		Vendor:  "servertech",
	}
	cfg := &snmp_pdu.SNMPPDUConfig{
		Version:   "v2c",
		Community: "public",
		Targets:   []snmp_pdu.PDUTarget{target},
		Timeout:   1 * time.Second,
	}
	cfg.ApplyDefaults()

	poller, err := snmp_pdu.NewSNMPPoller(target, cfg, slog.Default())
	if err != nil {
		t.Fatalf("NewSNMPPoller: %v", err)
	}
	defer poller.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = poller.Poll(ctx)
	if err == nil {
		t.Fatal("expected error when polling a down simulator")
	}
}

func TestModbus_SimulatorDown(t *testing.T) {
	cfg := temperature.SensorConfig{
		Address:         "127.0.0.1:19998",
		Protocol:        "modbus",
		SensorID:        "down",
		RegisterAddress: 100,
		RegisterType:    "holding",
		ScaleFactor:    0.1,
	}
	_, err := temperature.PollModbus(cfg)
	if err == nil {
		t.Fatal("expected error when connecting to down Modbus simulator")
	}
}

func TestSlurm_SimulatorDown(t *testing.T) {
	client := slurm.NewSlurmClient("http://127.0.0.1:19997", "v0.0.40", "token", 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.ListJobs(ctx)
	if err == nil {
		t.Fatal("expected error when connecting to down Slurm simulator")
	}
}

func TestROCm_InvalidPath(t *testing.T) {
	collector := rocm.NewROCmCollector("/nonexistent/rocm-smi", nil, slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := collector.Collect(ctx)
	if err == nil {
		t.Fatal("expected error when rocm-smi path is invalid")
	}
}

func assertFloat(t *testing.T, m map[string]interface{}, key string, want, tol float64) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Errorf("missing metric %q", key)
		return
	}
	var got float64
	switch x := v.(type) {
	case float64:
		got = x
	case float32:
		got = float64(x)
	case int:
		got = float64(x)
	case int64:
		got = float64(x)
	default:
		t.Errorf("%q: unsupported type %T", key, v)
		return
	}
	if math.Abs(got-want) > tol {
		t.Errorf("%q: got %.2f, want %.2f (tol %.2f)", key, got, want, tol)
	}
}

func findMockRocmSmi(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	candidates := []string{
		filepath.Join(wd, "test", "integration", "rocm_simulator", "mock_rocm_smi.sh"),
		filepath.Join(wd, "agent", "test", "integration", "rocm_simulator", "mock_rocm_smi.sh"),
		filepath.Join(wd, "rocm_simulator", "mock_rocm_smi.sh"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatal("mock_rocm_smi.sh not found; run tests from agent/ or repo root")
	return ""
}
