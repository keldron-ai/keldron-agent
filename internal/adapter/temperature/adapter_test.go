package temperature

import (
	"context"
	"log/slog"
	"testing"
	"time"

	adapterpkg "github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"gopkg.in/yaml.v3"
)

// waitUntil polls condition at short intervals until it returns true or timeout is reached.
func waitUntil(t *testing.T, timeout time.Duration, msg string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestParseSNMPValue_Tenths(t *testing.T) {
	// Value 235 with tenths encoding → 23.5°C
	got, err := parseSNMPValue(int(235), "tenths")
	if err != nil {
		t.Fatalf("parseSNMPValue: %v", err)
	}
	if got != 23.5 {
		t.Errorf("got %v, want 23.5", got)
	}

	// Default encoding (empty) → tenths
	got, err = parseSNMPValue(int(235), "")
	if err != nil {
		t.Fatalf("parseSNMPValue: %v", err)
	}
	if got != 23.5 {
		t.Errorf("got %v, want 23.5", got)
	}
}

func TestParseSNMPValue_Float(t *testing.T) {
	// Float string "23.5" with raw encoding → 23.5°C
	got, err := parseSNMPValue([]byte("23.5"), "raw")
	if err != nil {
		t.Fatalf("parseSNMPValue: %v", err)
	}
	if got != 23.5 {
		t.Errorf("got %v, want 23.5", got)
	}

	// Float string "235" with tenths encoding → 23.5°C
	got, err = parseSNMPValue([]byte("235"), "tenths")
	if err != nil {
		t.Fatalf("parseSNMPValue: %v", err)
	}
	if got != 23.5 {
		t.Errorf("got %v, want 23.5", got)
	}

	// Direct float (tenths encoding applied)
	got, err = parseSNMPValue(float64(23.5), "")
	if err != nil {
		t.Fatalf("parseSNMPValue: %v", err)
	}
	if got != 2.35 {
		t.Errorf("got %v, want 2.35", got)
	}

	// Direct float with raw encoding
	got, err = parseSNMPValue(float64(23.5), "raw")
	if err != nil {
		t.Fatalf("parseSNMPValue: %v", err)
	}
	if got != 23.5 {
		t.Errorf("got %v, want 23.5", got)
	}
}

func TestParseSNMPValue_Raw(t *testing.T) {
	// Raw encoding: 235 → 235.0
	got, err := parseSNMPValue(int(235), "raw")
	if err != nil {
		t.Fatalf("parseSNMPValue: %v", err)
	}
	if got != 235.0 {
		t.Errorf("got %v, want 235.0", got)
	}
}

func TestParseSNMPValue_UnknownEncoding(t *testing.T) {
	_, err := parseSNMPValue(int(235), "unknown")
	if err == nil {
		t.Error("expected error for unknown encoding")
	}
}

func TestModbusScaleFactor(t *testing.T) {
	// Raw register 350 with scale 0.1 → 35.0°C
	// We test the scale logic; actual PollModbus requires a real server
	raw := uint16(350)
	scale := 0.1
	got := float64(raw) * scale
	if got != 35.0 {
		t.Errorf("got %v, want 35.0", got)
	}
}

func TestPollSNMP_EmptyOID(t *testing.T) {
	cfg := SensorConfig{
		Address:   "192.168.1.1:161",
		SensorID:  "test",
		Position:  "inlet",
		Community: "public",
		OID:       "",
	}
	_, err := PollSNMP(cfg)
	if err == nil {
		t.Error("expected error for empty OID")
	}
}

func TestPollSNMP_InvalidAddress(t *testing.T) {
	cfg := SensorConfig{
		Address:   "",
		SensorID:  "test",
		Position:  "inlet",
		Community: "public",
		OID:       "1.3.6.1.4.1.1.1.0",
	}
	_, err := PollSNMP(cfg)
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestPollModbus_InvalidRegisterType(t *testing.T) {
	cfg := SensorConfig{
		Address:         "127.0.0.1:15998",
		SensorID:        "test",
		Position:        "inlet",
		RegisterAddress: 50,
		RegisterType:    "coil",
		ScaleFactor:     0.1,
	}
	_, err := PollModbus(cfg)
	if err == nil {
		t.Error("expected error for invalid register_type")
	}
}

func TestPollModbus_InputRegisterType(t *testing.T) {
	// Exercises the "input" register type path (connect will fail, but we hit the switch)
	cfg := SensorConfig{
		Address:         "127.0.0.1:15998",
		SensorID:        "test",
		Position:        "inlet",
		RegisterAddress: 50,
		RegisterType:    "input",
		ScaleFactor:     0.1,
	}
	_, err := PollModbus(cfg)
	// Expect connect error (no server) - we've exercised the input register branch
	if err == nil {
		t.Error("expected error when connecting to non-existent server")
	}
}

func TestPollModbus_ConnectError(t *testing.T) {
	// PollModbus against non-existent server exercises the connect/error path
	cfg := SensorConfig{
		Address:         "127.0.0.1:15999",
		Protocol:        "modbus",
		SensorID:        "test",
		Position:        "outlet",
		UnitID:          1,
		RegisterAddress: 100,
		RegisterType:    "holding",
		ScaleFactor:     0.1,
	}
	_, err := PollModbus(cfg)
	if err == nil {
		t.Error("expected error when connecting to non-existent modbus server")
	}
}

func TestStaleDetector_FiveIdentical(t *testing.T) {
	d := NewStaleDetector(5)
	sensorID := "test-sensor"

	// 5 identical readings → stale=true
	for i := 0; i < 4; i++ {
		if d.Check(sensorID, 23.5) {
			t.Errorf("reading %d: expected not stale yet", i+1)
		}
	}
	if !d.Check(sensorID, 23.5) {
		t.Error("5th identical reading: expected stale=true")
	}
}

func TestStaleDetector_FourThenDifferent(t *testing.T) {
	d := NewStaleDetector(5)
	sensorID := "test-sensor"

	for i := 0; i < 4; i++ {
		d.Check(sensorID, 23.5)
	}
	// 5th different → not stale
	if d.Check(sensorID, 24.0) {
		t.Error("5th different reading: expected stale=false")
	}
}

func TestStaleDetector_Tolerance(t *testing.T) {
	d := NewStaleDetector(5)
	sensorID := "test-sensor"

	// Values within 0.01°C tolerance → stale=true
	vals := []float64{23.5, 23.501, 23.499, 23.50, 23.509}
	for i := 0; i < 4; i++ {
		if d.Check(sensorID, vals[i]) {
			t.Errorf("reading %d: expected not stale yet", i+1)
		}
	}
	if !d.Check(sensorID, vals[4]) {
		t.Error("5th reading within tolerance: expected stale=true")
	}
}

func TestStaleDetector_ResetOnDifferent(t *testing.T) {
	d := NewStaleDetector(5)
	sensorID := "test-sensor"

	for i := 0; i < 5; i++ {
		d.Check(sensorID, 23.5)
	}
	// Now stale
	if !d.Check(sensorID, 23.5) {
		t.Error("expected stale after 5 identical")
	}

	// Different value: buffer gets 24.0, so not all identical
	d.Check(sensorID, 24.0)
	// 4 more 23.5: buffer has 24.0 + 4x23.5, not all same
	for i := 0; i < 4; i++ {
		if d.Check(sensorID, 23.5) {
			t.Errorf("after different, reading %d: expected not stale (buffer has 24.0)", i+1)
		}
	}
	// 5th 23.5: pushes out 24.0, buffer is now 5x23.5 → stale
	if !d.Check(sensorID, 23.5) {
		t.Error("after 5 identical following different, expected stale")
	}
}

func TestMetricKeyForPosition(t *testing.T) {
	if got := metricKeyForPosition("inlet"); got != "inlet_temp_c" {
		t.Errorf("inlet: got %q", got)
	}
	if got := metricKeyForPosition("outlet"); got != "outlet_temp_c" {
		t.Errorf("outlet: got %q", got)
	}
	if got := metricKeyForPosition(""); got != "inlet_temp_c" {
		t.Errorf("empty: got %q, want inlet_temp_c", got)
	}
}

func TestParseAddress(t *testing.T) {
	host, port, err := parseAddress("192.168.1.200:161", 161)
	if err != nil {
		t.Fatalf("parseAddress: %v", err)
	}
	if host != "192.168.1.200" || port != 161 {
		t.Errorf("got host=%q port=%d", host, port)
	}

	host, port, err = parseAddress("192.168.1.201:502", 161)
	if err != nil {
		t.Fatalf("parseAddress: %v", err)
	}
	if host != "192.168.1.201" || port != 502 {
		t.Errorf("got host=%q port=%d", host, port)
	}

	host, port, err = parseAddress("192.168.1.200", 161)
	if err != nil {
		t.Fatalf("parseAddress: %v", err)
	}
	if host != "192.168.1.200" || port != 161 {
		t.Errorf("no port: got host=%q port=%d", host, port)
	}
}

func TestNew_NoSensors(t *testing.T) {
	y := `
enabled: true
poll_interval: 30s
sensors: []
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          raw,
	}
	_, err := New(cfg, nil, nil)
	if err == nil {
		t.Error("expected error when no sensors configured")
	}
}

func TestNew_EmptySensorID(t *testing.T) {
	y := `
enabled: true
poll_interval: 30s
sensors:
  - address: "192.168.1.200:161"
    protocol: "snmp"
    sensor_id: ""
    position: "inlet"
    community: "public"
    oid: "1.3.6.1.4.1.xxx.1.1.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          raw,
	}
	_, err := New(cfg, nil, nil)
	if err == nil {
		t.Error("expected error for empty sensor_id")
	}
}

func TestNew_DuplicateSensorID(t *testing.T) {
	y := `
enabled: true
poll_interval: 30s
sensors:
  - address: "192.168.1.200:161"
    protocol: "snmp"
    sensor_id: "dup"
    position: "inlet"
    community: "public"
    oid: "1.3.6.1.4.1.xxx.1.1.0"
  - address: "192.168.1.201:161"
    protocol: "snmp"
    sensor_id: "dup"
    position: "outlet"
    community: "public"
    oid: "1.3.6.1.4.1.xxx.1.2.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          raw,
	}
	_, err := New(cfg, nil, nil)
	if err == nil {
		t.Error("expected error for duplicate sensor_id")
	}
}

func TestNew_InvalidPosition(t *testing.T) {
	y := `
enabled: true
poll_interval: 30s
sensors:
  - address: "192.168.1.200:161"
    protocol: "snmp"
    sensor_id: "temp1"
    position: "top"
    community: "public"
    oid: "1.3.6.1.4.1.xxx.1.1.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          raw,
	}
	_, err := New(cfg, nil, nil)
	if err == nil {
		t.Error("expected error for invalid position")
	}
}

func TestNew_NilLogger(t *testing.T) {
	y := `
enabled: true
poll_interval: 30s
sensors:
  - address: "192.168.1.200:161"
    protocol: "snmp"
    sensor_id: "temp1"
    position: "inlet"
    community: "public"
    oid: "1.3.6.1.4.1.xxx.1.1.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          raw,
	}
	a, err := New(cfg, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ta := a.(*TemperatureAdapter)
	if ta.logger == nil {
		t.Error("expected non-nil logger when nil was passed")
	}
}

func TestNew_ValidConfig(t *testing.T) {
	y := `
enabled: true
poll_interval: 30s
stale_threshold: 5
sensors:
  - address: "192.168.1.200:161"
    protocol: "snmp"
    sensor_id: "temp-rack01-inlet"
    rack_id: "rack-01"
    position: "inlet"
    community: "public"
    oid: "1.3.6.1.4.1.xxx.1.1.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          raw,
	}
	a, err := New(cfg, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Name() != "temperature" {
		t.Errorf("Name: got %q", a.Name())
	}
	if a.Readings() == nil {
		t.Error("Readings() should not be nil")
	}
}

func TestParseSNMPValue_UnsupportedType(t *testing.T) {
	_, err := parseSNMPValue(struct{}{}, "")
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestParseSNMPValue_AllIntegerTypes(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want float64
		enc  string
	}{
		{"int32 tenths", int32(235), 23.5, "tenths"},
		{"int64 tenths", int64(235), 23.5, ""},
		{"uint tenths", uint(100), 10.0, ""},
		{"uint32 tenths", uint32(350), 35.0, ""},
		{"uint64 tenths", uint64(200), 20.0, ""},
		{"string tenths", "425", 42.5, ""},
		{"string raw", "42.5", 42.5, "raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSNMPValue(tt.val, tt.enc)
			if err != nil {
				t.Fatalf("parseSNMPValue: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAddress_InvalidPort(t *testing.T) {
	_, _, err := parseAddress("host:notanumber", 161)
	if err == nil {
		t.Error("expected error for invalid port")
	}
}

func TestParseAddress_NoPortUsesDefault(t *testing.T) {
	host, port, err := parseAddress("192.168.1.100", 502)
	if err != nil {
		t.Fatalf("parseAddress: %v", err)
	}
	if host != "192.168.1.100" || port != 502 {
		t.Errorf("got host=%q port=%d", host, port)
	}
}

func TestAdapter_StartStop(t *testing.T) {
	y := `
enabled: true
poll_interval: 1h
stale_threshold: 5
sensors:
  - address: "127.0.0.1:19999"
    protocol: "snmp"
    sensor_id: "test-inlet"
    rack_id: "rack-01"
    position: "inlet"
    community: "public"
    oid: "1.3.6.1.4.1.1.1.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 1 * time.Hour,
		Raw:          raw,
	}
	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ta := a.(*TemperatureAdapter)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Start(ctx)
	}()

	waitUntil(t, time.Second, "expected IsRunning=true while adapter is running", ta.IsRunning)
	pc, ec, lastPoll, lastErr, _ := ta.Stats()
	if pc == 0 && ec == 0 {
		t.Log("Stats: poll/error counts may be 0 if poll hasn't completed yet")
	}
	_ = lastPoll
	_ = lastErr

	cancel()
	err = <-done
	if err != nil {
		t.Errorf("Start: %v", err)
	}

	_ = a.Stop(context.Background())
}

func TestNew_InvalidProtocol(t *testing.T) {
	y := `
enabled: true
poll_interval: 1h
stale_threshold: 5
sensors:
  - address: "127.0.0.1:502"
    protocol: "invalid"
    sensor_id: "test-bad"
    rack_id: "rack-01"
    position: "inlet"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 100 * time.Millisecond,
		Raw:          raw,
	}
	_, err := New(cfg, nil, slog.Default())
	if err == nil {
		t.Error("expected error for invalid protocol")
	}
}

func TestNew_DecodeError(t *testing.T) {
	// Invalid YAML structure
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          yaml.Node{Kind: yaml.ScalarNode, Value: "not a mapping"},
	}
	_, err := New(cfg, nil, slog.Default())
	if err == nil {
		t.Error("expected error when decoding invalid config")
	}
}

func TestNewStaleDetector_DefaultThreshold(t *testing.T) {
	d := NewStaleDetector(0)
	if d == nil {
		t.Fatal("NewStaleDetector(0) should not return nil")
	}
	// With threshold 0, we default to 5; need 5 identical to be stale
	for i := 0; i < 5; i++ {
		d.Check("s", 10.0)
	}
	if !d.Check("s", 10.0) {
		t.Error("expected stale after 5 identical with default threshold")
	}
}

func TestParseAddress_Invalid(t *testing.T) {
	_, _, err := parseAddress("", 161)
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestAdapter_ConfigHotReload(t *testing.T) {
	y := `
enabled: true
poll_interval: 1h
stale_threshold: 5
sensors:
  - address: "127.0.0.1:19999"
    protocol: "snmp"
    sensor_id: "test-inlet"
    rack_id: "rack-01"
    position: "inlet"
    community: "public"
    oid: "1.3.6.1.4.1.1.1.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.Defaults()
	cfg.Sender.Target = "localhost:50051"
	cfg.Adapters["temperature"] = config.AdapterConfig{
		Enabled:      true,
		PollInterval: 1 * time.Hour,
		Raw:          raw,
	}
	holder := config.NewHolder(cfg)
	a, err := New(cfg.Adapters["temperature"], holder, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Start(ctx)
	}()

	ta := a.(*TemperatureAdapter)
	waitUntil(t, time.Second, "expected adapter to be running", ta.IsRunning)

	// Update config with new poll interval to trigger updatePollInterval
	cfg2 := config.Defaults()
	cfg2.Sender.Target = "localhost:50051"
	cfg2.Adapters["temperature"] = config.AdapterConfig{
		Enabled:      true,
		PollInterval: 2 * time.Hour,
		Raw:          raw,
	}
	if err := holder.Update(cfg2); err != nil {
		t.Fatalf("Update: %v", err)
	}
	waitUntil(t, time.Second, "expected poll interval to update", func() bool {
		return ta.getPollInterval() == 2*time.Hour
	})

	cancel()
	<-done
}

func TestAdapter_ConfigHotReload_SameInterval(t *testing.T) {
	// Update with same interval - exercises early return in updatePollInterval
	y := `
enabled: true
poll_interval: 1h
stale_threshold: 5
sensors:
  - address: "127.0.0.1:19999"
    protocol: "snmp"
    sensor_id: "test-inlet"
    rack_id: "rack-01"
    position: "inlet"
    community: "public"
    oid: "1.3.6.1.4.1.1.1.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.Defaults()
	cfg.Sender.Target = "localhost:50051"
	cfg.Adapters["temperature"] = config.AdapterConfig{
		Enabled:      true,
		PollInterval: 1 * time.Hour,
		Raw:          raw,
	}
	holder := config.NewHolder(cfg)
	a, err := New(cfg.Adapters["temperature"], holder, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ta := a.(*TemperatureAdapter)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Start(ctx)
	}()

	waitUntil(t, time.Second, "expected adapter to be running", ta.IsRunning)

	// Update with same poll interval - should early return
	cfg2 := config.Defaults()
	cfg2.Sender.Target = "localhost:50051"
	cfg2.Adapters["temperature"] = config.AdapterConfig{
		Enabled:      true,
		PollInterval: 1 * time.Hour,
		Raw:          raw,
	}
	if err := holder.Update(cfg2); err != nil {
		t.Fatalf("Update: %v", err)
	}

	cancel()
	<-done
}

func TestAdapter_PollSuccessAndStale(t *testing.T) {
	// Mock SNMP to return success so we can test the full poll flow including stale detection
	origSNMP := pollSNMPFunc
	defer func() { pollSNMPFunc = origSNMP }()

	pollSNMPFunc = func(cfg SensorConfig) (adapterpkg.RawReading, error) {
		return adapterpkg.RawReading{
			AdapterName: "temperature",
			Source:      cfg.SensorID,
			Timestamp:   time.Now(),
			Metrics:     map[string]interface{}{"inlet_temp_c": 23.5, "stale": 0.0},
		}, nil
	}

	y := `
enabled: true
poll_interval: 50ms
stale_threshold: 5
sensors:
  - address: "127.0.0.1:161"
    protocol: "snmp"
    sensor_id: "mock-inlet"
    rack_id: "rack-01"
    position: "inlet"
    community: "public"
    oid: "1.3.6.1.4.1.1.1.0"
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 50 * time.Millisecond,
		Raw:          raw,
	}
	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Start(ctx)
	}()

	// Collect readings until we get one with stale=1
	var readings []adapterpkg.RawReading
	timeout := time.After(2 * time.Second)
	for {
		select {
		case r := <-a.Readings():
			readings = append(readings, r)
			if s, ok := r.Metrics["stale"]; ok {
				if v, ok := toFloat64(s); ok && v == 1.0 {
					cancel()
					<-done
					if len(readings) < 5 {
						t.Errorf("expected at least 5 readings (5th is stale), got %d", len(readings))
					}
					return
				}
			}
		case <-timeout:
			cancel()
			<-done
			t.Fatalf("timeout waiting for stale reading; got %d readings", len(readings))
		}
	}
}

func TestAdapter_ModbusPollSuccess(t *testing.T) {
	origModbus := pollModbusFunc
	defer func() { pollModbusFunc = origModbus }()

	pollModbusFunc = func(cfg SensorConfig) (adapterpkg.RawReading, error) {
		return adapterpkg.RawReading{
			AdapterName: "temperature",
			Source:      cfg.SensorID,
			Timestamp:   time.Now(),
			Metrics:     map[string]interface{}{"outlet_temp_c": 35.0, "stale": 0.0},
		}, nil
	}

	y := `
enabled: true
poll_interval: 50ms
stale_threshold: 5
sensors:
  - address: "127.0.0.1:502"
    protocol: "modbus"
    sensor_id: "mock-outlet"
    rack_id: "rack-01"
    position: "outlet"
    unit_id: 1
    register_address: 100
    register_type: "holding"
    scale_factor: 0.1
`
	var raw yaml.Node
	if err := yaml.Unmarshal([]byte(y), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	cfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 50 * time.Millisecond,
		Raw:          raw,
	}
	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- a.Start(ctx)
	}()

	// Wait for at least one reading
	select {
	case r := <-a.Readings():
		if v, ok := r.Metrics["outlet_temp_c"]; !ok || v != 35.0 {
			t.Errorf("expected outlet_temp_c=35, got %v", r.Metrics)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for modbus reading")
	}
	cancel()
	<-done
}

func TestToFloat64(t *testing.T) {
	if v, ok := toFloat64(float64(1.5)); !ok || v != 1.5 {
		t.Errorf("float64: got %v, %v", v, ok)
	}
	if v, ok := toFloat64(float32(2.5)); !ok || v != 2.5 {
		t.Errorf("float32: got %v, %v", v, ok)
	}
	if v, ok := toFloat64(int(3)); !ok || v != 3 {
		t.Errorf("int: got %v, %v", v, ok)
	}
	if v, ok := toFloat64(int64(4)); !ok || v != 4 {
		t.Errorf("int64: got %v, %v", v, ok)
	}
	if v, ok := toFloat64(uint64(5)); !ok || v != 5 {
		t.Errorf("uint64: got %v, %v", v, ok)
	}
	if _, ok := toFloat64("x"); ok {
		t.Error("string: expected ok=false")
	}
}

func TestExtractTempFromReading(t *testing.T) {
	inlet := adapterpkg.RawReading{AdapterName: "temperature", Source: "s1", Metrics: map[string]interface{}{"inlet_temp_c": 22.5}}
	if got, ok := extractTempFromReading(inlet); !ok || got != 22.5 {
		t.Errorf("inlet: got %v, ok=%v", got, ok)
	}

	outlet := adapterpkg.RawReading{AdapterName: "temperature", Source: "s2", Metrics: map[string]interface{}{"outlet_temp_c": 35.0}}
	if got, ok := extractTempFromReading(outlet); !ok || got != 35.0 {
		t.Errorf("outlet: got %v, ok=%v", got, ok)
	}

	empty := adapterpkg.RawReading{Metrics: map[string]interface{}{}}
	if _, ok := extractTempFromReading(empty); ok {
		t.Error("empty: expected ok=false")
	}
}
