package snmp_pdu

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/gosnmp/gosnmp"
)

// mockSnmpGetter returns predefined responses for testing.
type mockSnmpGetter struct {
	getFunc func(oids []string) (*gosnmp.SnmpPacket, error)
}

func (m *mockSnmpGetter) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	if m.getFunc != nil {
		return m.getFunc(oids)
	}
	return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{}}, nil
}

func TestPoller_ServerTechResponse(t *testing.T) {
	t.Parallel()
	oidMap := GetVendorOIDMap(VendorServerTech)
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			// ServerTech: power in watts, current in deciamps
			// 50000 watts = 50 kW, 120 deciamps = 12 A
			vars := make([]gosnmp.SnmpPDU, 0, len(oids))
			for _, oid := range oids {
				switch oid {
				case oidMap.TotalPowerKW:
					vars = append(vars, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: 50000})
				case oidMap.VoltageV:
					vars = append(vars, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: 208})
				case oidMap.CurrentA:
					vars = append(vars, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: 120})
				default:
					if len(oidMap.PhaseCurrentA) > 0 && oid == oidMap.PhaseCurrentA[0] {
						vars = append(vars, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: 40})
					}
				}
			}
			return &gosnmp.SnmpPacket{Variables: vars}, nil
		},
	}
	cfg := &SNMPPDUConfig{Version: "v2c", Community: "public", Timeout: 0}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"rack-01"}, Vendor: VendorServerTech}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())

	readings, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("len(readings) = %d, want 1", len(readings))
	}
	r := readings[0]
	if r.AdapterName != adapterName {
		t.Errorf("AdapterName = %q, want %q", r.AdapterName, adapterName)
	}
	if r.Source != "rack-01" {
		t.Errorf("Source = %q, want rack-01", r.Source)
	}
	// 50000 watts / 1000 = 50 kW
	if kw, ok := r.Metrics[MetricPowerKW].(float64); !ok || kw < 49 || kw > 51 {
		t.Errorf("power_kw = %v, want ~50", r.Metrics[MetricPowerKW])
	}
	// 120 deciamps / 10 = 12 A
	if a, ok := r.Metrics[MetricCurrentA].(float64); !ok || a < 11 || a > 13 {
		t.Errorf("current_a = %v, want ~12", r.Metrics[MetricCurrentA])
	}
	if v, ok := r.Metrics[MetricVoltageV].(float64); !ok || v != 208 {
		t.Errorf("voltage_v = %v, want 208", r.Metrics[MetricVoltageV])
	}
}

func TestPoller_MilliwattToKW(t *testing.T) {
	t.Parallel()
	// Simulate vendor that returns milliwatts (e.g. 50000 mW = 0.05 kW)
	oidMap := GetVendorOIDMap(VendorGeneric)
	oidMap.PowerMultiplier = 1000000 // mW -> kW
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			vars := []gosnmp.SnmpPDU{
				{Name: oidMap.TotalPowerKW, Type: gosnmp.Integer, Value: 50000}, // 50000 mW
				{Name: oidMap.VoltageV, Type: gosnmp.Integer, Value: 120},
				{Name: oidMap.CurrentA, Type: gosnmp.Integer, Value: 10},
			}
			return &gosnmp.SnmpPacket{Variables: vars}, nil
		},
	}
	cfg := &SNMPPDUConfig{Version: "v2c", Community: "public"}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: VendorGeneric}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())
	p.oidMap.PowerMultiplier = 1000000

	readings, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	kw := readings[0].Metrics[MetricPowerKW].(float64)
	// 50000 / 1000000 = 0.05 kW
	if kw < 0.049 || kw > 0.051 {
		t.Errorf("power_kw = %v, want ~0.05 (milliwatts normalized)", kw)
	}
}

func TestPoller_APCHundredthsKW(t *testing.T) {
	t.Parallel()
	// APC: power in hundredths of kW (500 = 5.00 kW)
	oidMap := GetVendorOIDMap(VendorAPC)
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			vars := []gosnmp.SnmpPDU{
				{Name: oidMap.TotalPowerKW, Type: gosnmp.Integer, Value: 500},
				{Name: oidMap.VoltageV, Type: gosnmp.Integer, Value: 208},
				{Name: oidMap.CurrentA, Type: gosnmp.Integer, Value: 120}, // tenths of A -> 12 A
			}
			return &gosnmp.SnmpPacket{Variables: vars}, nil
		},
	}
	cfg := &SNMPPDUConfig{Version: "v2c", Community: "public"}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: VendorAPC}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())

	readings, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	kw := readings[0].Metrics[MetricPowerKW].(float64)
	if kw < 4.9 || kw > 5.1 {
		t.Errorf("power_kw = %v, want ~5 (APC hundredths)", kw)
	}
	cur := readings[0].Metrics[MetricCurrentA].(float64)
	if cur < 11 || cur > 13 {
		t.Errorf("current_a = %v, want ~12 (APC tenths)", cur)
	}
}

func TestPoller_MissingOID_PartialReadings(t *testing.T) {
	t.Parallel()
	oidMap := GetVendorOIDMap(VendorServerTech)
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			// Only return power, omit voltage and current (simulate missing OID)
			vars := []gosnmp.SnmpPDU{
				{Name: oidMap.TotalPowerKW, Type: gosnmp.Integer, Value: 10000},
			}
			return &gosnmp.SnmpPacket{Variables: vars}, nil
		},
	}
	cfg := &SNMPPDUConfig{Version: "v2c", Community: "public"}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: VendorServerTech}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())

	readings, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	// Should still get power_kw
	if _, ok := readings[0].Metrics[MetricPowerKW]; !ok {
		t.Error("expected power_kw in partial readings")
	}
	// voltage and current may be absent
	if len(readings[0].Metrics) == 0 {
		t.Error("expected at least one metric")
	}
}

func TestPoller_TimeoutRetry(t *testing.T) {
	t.Parallel()
	attempts := 0
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			attempts++
			if attempts < 2 {
				return nil, errors.New("i/o timeout")
			}
			oidMap := GetVendorOIDMap(VendorServerTech)
			return &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{Name: oidMap.TotalPowerKW, Type: gosnmp.Integer, Value: 1000},
					{Name: oidMap.VoltageV, Type: gosnmp.Integer, Value: 208},
					{Name: oidMap.CurrentA, Type: gosnmp.Integer, Value: 10},
				},
			}, nil
		},
	}
	cfg := &SNMPPDUConfig{
		Version: "v2c", Community: "public",
		Retry: RetryConfig{MaxAttempts: 3, BackoffInitial: 1, BackoffMax: 10},
	}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: VendorServerTech}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())

	readings, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 (retry once then succeed)", attempts)
	}
	if len(readings) != 1 {
		t.Fatalf("len(readings) = %d, want 1", len(readings))
	}
}

func TestPoller_NonTimeoutErrorNoRetry(t *testing.T) {
	t.Parallel()
	attempts := 0
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			attempts++
			return nil, errors.New("authentication failure")
		},
	}
	cfg := &SNMPPDUConfig{
		Version: "v2c", Community: "public",
		Retry: RetryConfig{MaxAttempts: 3},
	}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: VendorServerTech}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())

	_, err := p.Poll(context.Background())
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on auth error)", attempts)
	}
}

func TestPoller_DeadlineExceededIsTimeout(t *testing.T) {
	t.Parallel()
	attempts := 0
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			attempts++
			return nil, context.DeadlineExceeded
		},
	}
	cfg := &SNMPPDUConfig{
		Version: "v2c", Community: "public",
		Retry: RetryConfig{MaxAttempts: 2},
	}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: VendorServerTech}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())

	_, err := p.Poll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != cfg.Retry.MaxAttempts {
		t.Errorf("attempts = %d, want %d (should retry on deadline exceeded)", attempts, cfg.Retry.MaxAttempts)
	}
}

func TestPoller_CloseIdempotent(t *testing.T) {
	t.Parallel()
	oidMap := GetVendorOIDMap(VendorServerTech)
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			return &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{Name: oidMap.TotalPowerKW, Type: gosnmp.Integer, Value: 1000},
					{Name: oidMap.VoltageV, Type: gosnmp.Integer, Value: 208},
					{Name: oidMap.CurrentA, Type: gosnmp.Integer, Value: 10},
				},
			}, nil
		},
	}
	cfg := &SNMPPDUConfig{Version: "v2c", Community: "public"}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: VendorServerTech}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())

	_ = p.Close()
	_ = p.Close() // second close should be no-op
	_, err := p.Poll(context.Background())
	if err == nil {
		t.Error("expected error when polling closed poller")
	}
}

func TestPoller_SnmpValueTypes(t *testing.T) {
	t.Parallel()
	oidMap := GetVendorOIDMap(VendorGeneric)
	mock := &mockSnmpGetter{
		getFunc: func(oids []string) (*gosnmp.SnmpPacket, error) {
			// Test uint32, float64, int64
			vars := []gosnmp.SnmpPDU{
				{Name: oidMap.TotalPowerKW, Type: gosnmp.Gauge32, Value: uint32(5000)},
				{Name: oidMap.VoltageV, Type: gosnmp.Integer, Value: int64(120)},
				{Name: oidMap.CurrentA, Type: gosnmp.Counter32, Value: uint32(5)},
			}
			return &gosnmp.SnmpPacket{Variables: vars}, nil
		},
	}
	cfg := &SNMPPDUConfig{Version: "v2c", Community: "public"}
	target := PDUTarget{Address: "127.0.0.1:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: VendorGeneric}
	p := newSNMPPollerForTest(target, cfg, mock, slog.Default())

	readings, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	// 5000 watts / 1000 = 5 kW
	if kw := readings[0].Metrics[MetricPowerKW].(float64); kw < 4.9 || kw > 5.1 {
		t.Errorf("power_kw = %v, want ~5", kw)
	}
}
