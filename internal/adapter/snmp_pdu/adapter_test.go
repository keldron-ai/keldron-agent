package snmp_pdu

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"gopkg.in/yaml.v3"
)

func TestDecodeFromRaw_ValidConfig(t *testing.T) {
	t.Parallel()
	raw := `
version: "v2c"
community: "public"
targets:
  - address: "192.168.1.100:161"
    pdu_id: "pdu-1"
    rack_ids: ["rack-01"]
    vendor: "servertech"
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	cfg, err := DecodeFromRaw(&node)
	if err != nil {
		t.Fatalf("DecodeFromRaw: %v", err)
	}
	if cfg.Version != "v2c" {
		t.Errorf("Version = %q, want v2c", cfg.Version)
	}
	if cfg.Community != "public" {
		t.Errorf("Community = %q, want public", cfg.Community)
	}
	if len(cfg.Targets) != 1 {
		t.Fatalf("len(Targets) = %d, want 1", len(cfg.Targets))
	}
	if cfg.Targets[0].Address != "192.168.1.100:161" {
		t.Errorf("Target[0].Address = %q", cfg.Targets[0].Address)
	}
	if cfg.Targets[0].PDUID != "pdu-1" {
		t.Errorf("Target[0].PDUID = %q", cfg.Targets[0].PDUID)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestDecodeFromRaw_V3Config(t *testing.T) {
	t.Parallel()
	raw := `
version: "v3"
username: "snmpuser"
auth_protocol: "SHA"
auth_passphrase: "authpass"
priv_protocol: "AES"
priv_passphrase: "privpass"
targets:
  - address: "192.168.1.100:161"
    pdu_id: "pdu-1"
    rack_ids: ["rack-01"]
    vendor: "raritan"
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	cfg, err := DecodeFromRaw(&node)
	if err != nil {
		t.Fatalf("DecodeFromRaw: %v", err)
	}
	if cfg.Version != "v3" {
		t.Errorf("Version = %q, want v3", cfg.Version)
	}
	if cfg.Username != "snmpuser" {
		t.Errorf("Username = %q", cfg.Username)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidate_InvalidVersion(t *testing.T) {
	t.Parallel()
	cfg := &SNMPPDUConfig{
		Version: "v1",
		Targets: []PDUTarget{{Address: "1.2.3.4:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: "generic"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate: expected error for invalid version")
	}
}

func TestValidate_NoTargets(t *testing.T) {
	t.Parallel()
	cfg := &SNMPPDUConfig{Version: "v2c", Community: "public", Targets: nil}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate: expected error for no targets")
	}
}

func TestValidate_V2cNoCommunity(t *testing.T) {
	t.Parallel()
	cfg := &SNMPPDUConfig{
		Version:   "v2c",
		Community: "",
		Targets:   []PDUTarget{{Address: "1.2.3.4:161", PDUID: "pdu-1", RackIDs: []string{"r1"}}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate: expected error for v2c without community")
	}
}

func TestGetVendorOIDMap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		vendor string
		want   string
	}{
		{"servertech", "1.3.6.1.4.1.1718"},
		{"raritan", "1.3.6.1.4.1.13742"},
		{"apc", "1.3.6.1.4.1.318"},
		{"generic", "1.3.6.1.2.1.33"},
		{"unknown", "1.3.6.1.2.1.33"},
	}
	for _, tt := range tests {
		m := GetVendorOIDMap(tt.vendor)
		if m.TotalPowerKW == "" {
			t.Errorf("vendor %s: TotalPowerKW empty", tt.vendor)
		}
		if !strings.HasPrefix(m.TotalPowerKW, tt.want) {
			t.Errorf("vendor %s: TotalPowerKW %q should start with %q", tt.vendor, m.TotalPowerKW, tt.want)
		}
	}
}

func TestTargetsEqual(t *testing.T) {
	t.Parallel()
	a := []PDUTarget{
		{Address: "1.2.3.4:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: "servertech"},
	}
	b := []PDUTarget{
		{Address: "1.2.3.4:161", PDUID: "pdu-1", RackIDs: []string{"r1"}, Vendor: "servertech"},
	}
	if !targetsEqual(a, b) {
		t.Error("targetsEqual: expected true for identical targets")
	}
	b[0].Vendor = "raritan"
	if targetsEqual(a, b) {
		t.Error("targetsEqual: expected false when vendor differs")
	}
	b[0].Vendor = "servertech"
	b[0].RackIDs = []string{"r1", "r2"}
	if targetsEqual(a, b) {
		t.Error("targetsEqual: expected false when rack_ids differ")
	}
}

func TestParseAddress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		addr      string
		wantHost  string
		wantPort  uint16
		wantError bool
	}{
		{"192.168.1.100:161", "192.168.1.100", 161, false},
		{"192.168.1.100", "192.168.1.100", 161, false},
		{"[::1]:161", "::1", 161, false},
		{"host:bad", "", 0, true},
	}
	for _, tt := range tests {
		host, port, err := parseAddress(tt.addr)
		if tt.wantError {
			if err == nil {
				t.Errorf("parseAddress(%q): expected error", tt.addr)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAddress(%q): %v", tt.addr, err)
			continue
		}
		if host != tt.wantHost || port != tt.wantPort {
			t.Errorf("parseAddress(%q) = %q, %d; want %q, %d", tt.addr, host, port, tt.wantHost, tt.wantPort)
		}
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	t.Parallel()
	acfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
	}
	_, err := New(acfg, nil, slog.Default())
	if err == nil {
		t.Error("New: expected error for empty config")
	}
}

func TestAdapter_NameReadingsStats(t *testing.T) {
	t.Parallel()
	// Construct the adapter directly to avoid network dependency.
	a := &SNMPPDUAdapter{
		snmpCfg:      &SNMPPDUConfig{Version: "v2c", Community: "public"},
		readings:     make(chan adapter.RawReading, channelBuffer),
		logger:       slog.Default(),
		pollInterval: 30 * time.Second,
	}
	if a.Name() != adapterName {
		t.Errorf("Name = %q, want %q", a.Name(), adapterName)
	}
	if a.Readings() == nil {
		t.Error("Readings() returned nil")
	}
	pc, ec, _, _, _ := a.Stats()
	if pc != 0 || ec != 0 {
		t.Errorf("Stats = %d, %d; want 0, 0", pc, ec)
	}
}

func TestNew_NoTargetsError(t *testing.T) {
	t.Parallel()
	holder := config.NewHolder(config.Defaults())
	cfg := holder.Get()
	cfg.Adapters["snmp_pdu"] = config.AdapterConfig{
		Enabled:      false,
		PollInterval: 30 * time.Second,
		Raw:          yaml.Node{},
	}
	_ = holder.Update(cfg)

	// Create adapter with minimal config - will fail at createPollers due to no targets
	acfg := config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
	}
	_, err := New(acfg, holder, nil)
	if err == nil {
		t.Error("expected error for config with no targets")
	}
}

func TestTargetsEqual_DifferentLengths(t *testing.T) {
	t.Parallel()
	a := []PDUTarget{{Address: "a:161", PDUID: "p1", RackIDs: []string{"r1"}, Vendor: "generic"}}
	b := []PDUTarget{
		{Address: "a:161", PDUID: "p1", RackIDs: []string{"r1"}, Vendor: "generic"},
		{Address: "b:161", PDUID: "p2", RackIDs: []string{"r2"}, Vendor: "generic"},
	}
	if targetsEqual(a, b) {
		t.Error("targetsEqual: expected false for different lengths")
	}
}
