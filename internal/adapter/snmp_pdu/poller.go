// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package snmp_pdu

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/keldron-ai/keldron-agent/internal/adapter"
)

const (
	adapterName = "snmp_pdu"

	// Metric keys for RawReading.Metrics (normalizer expects float64-coercible values).
	MetricPowerKW       = "power_kw"
	MetricVoltageV      = "voltage_v"
	MetricCurrentA      = "current_a"
	MetricPhaseCurrent1 = "phase_current_a_1"
	MetricPhaseCurrent2 = "phase_current_a_2"
	MetricPhaseCurrent3 = "phase_current_a_3"
	MetricInletTempC    = "inlet_temp_c"
	MetricStatus        = "status"
)

// snmpGetter is the interface for SNMP Get operations (allows mocking in tests).
type snmpGetter interface {
	Get(oids []string) (*gosnmp.SnmpPacket, error)
}

// SNMPPoller polls a single PDU target via SNMP.
type SNMPPoller struct {
	client  *gosnmp.GoSNMP
	getter  snmpGetter // same as client when using real connection
	target  PDUTarget
	oidMap  VendorOIDMap
	cfg     *SNMPPDUConfig
	logger  *slog.Logger
	closeMu sync.Mutex
	closed  bool
}

// NewSNMPPoller creates a poller for the given target. It connects internally
// via gs.Connect(), so callers need not call Connect before Poll. Returns an
// error if the SNMP connection fails.
func NewSNMPPoller(target PDUTarget, cfg *SNMPPDUConfig, logger *slog.Logger) (*SNMPPoller, error) {
	oidMap := GetVendorOIDMap(target.Vendor)
	if oidMap.PowerMultiplier <= 0 {
		oidMap.PowerMultiplier = 1000
	}
	if oidMap.CurrentMultiplier <= 0 {
		oidMap.CurrentMultiplier = 1
	}

	host, port, err := parseAddress(target.Address)
	if err != nil {
		return nil, fmt.Errorf("target %s: %w", target.PDUID, err)
	}

	gs := &gosnmp.GoSNMP{
		Target:    host,
		Port:      port,
		Timeout:   cfg.Timeout,
		Transport: "udp",
		Context:   context.Background(),
	}

	switch cfg.Version {
	case string(SNMPv2c):
		gs.Version = gosnmp.Version2c
		gs.Community = cfg.Community
	case string(SNMPv3):
		gs.Version = gosnmp.Version3
		gs.SecurityModel = gosnmp.UserSecurityModel
		usm := &gosnmp.UsmSecurityParameters{
			UserName:                 cfg.Username,
			AuthenticationProtocol:   authProto(cfg.AuthProtocol),
			AuthenticationPassphrase: cfg.AuthPassphrase,
			PrivacyProtocol:          privProto(cfg.PrivProtocol),
			PrivacyPassphrase:        cfg.PrivPassphrase,
		}
		gs.SecurityParameters = usm
		switch {
		case cfg.AuthPassphrase != "" && cfg.PrivPassphrase != "":
			gs.MsgFlags = gosnmp.AuthPriv
		case cfg.AuthPassphrase != "":
			gs.MsgFlags = gosnmp.AuthNoPriv
		default:
			gs.MsgFlags = gosnmp.NoAuthNoPriv
		}
	default:
		return nil, fmt.Errorf("unsupported SNMP version %q", cfg.Version)
	}

	if err := gs.Connect(); err != nil {
		return nil, fmt.Errorf("target %s connect: %w", target.PDUID, err)
	}

	return &SNMPPoller{
		client: gs,
		getter: gs,
		target: target,
		oidMap: oidMap,
		cfg:    cfg,
		logger: logger.With("pdu_id", target.PDUID, "address", target.Address),
	}, nil
}

// newSNMPPollerForTest creates a poller with a mock getter (for unit tests).
func newSNMPPollerForTest(target PDUTarget, cfg *SNMPPDUConfig, getter snmpGetter, logger *slog.Logger) *SNMPPoller {
	oidMap := GetVendorOIDMap(target.Vendor)
	if oidMap.PowerMultiplier <= 0 {
		oidMap.PowerMultiplier = 1000
	}
	if oidMap.CurrentMultiplier <= 0 {
		oidMap.CurrentMultiplier = 1
	}
	return &SNMPPoller{
		getter: getter,
		target: target,
		oidMap: oidMap,
		cfg:    cfg,
		logger: logger,
	}
}

func parseAddress(addr string) (host string, port uint16, err error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			return addr, 161, nil
		}
		return "", 0, err
	}
	if portStr == "" {
		return host, 161, nil
	}
	p, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %w", err)
	}
	return host, uint16(p), nil
}

func authProto(s string) gosnmp.SnmpV3AuthProtocol {
	switch strings.ToUpper(s) {
	case "MD5":
		return gosnmp.MD5
	case "SHA":
		return gosnmp.SHA
	case "SHA256":
		return gosnmp.SHA256
	default:
		return gosnmp.NoAuth
	}
}

func privProto(s string) gosnmp.SnmpV3PrivProtocol {
	switch strings.ToUpper(s) {
	case "DES":
		return gosnmp.DES
	case "AES":
		return gosnmp.AES
	case "AES256":
		return gosnmp.AES256
	default:
		return gosnmp.NoPriv
	}
}

// Close closes the SNMP connection.
func (p *SNMPPoller) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.client != nil && p.client.Conn != nil {
		err := p.client.Conn.Close()
		p.client = nil
		return err
	}
	return nil
}

// Poll fetches metrics from the PDU and returns one RawReading per rack.
func (p *SNMPPoller) Poll(ctx context.Context) ([]adapter.RawReading, error) {
	p.closeMu.Lock()
	if p.closed {
		p.closeMu.Unlock()
		return nil, fmt.Errorf("poller closed")
	}
	p.closeMu.Unlock()

	oids := p.collectOIDs()
	if len(oids) == 0 {
		return nil, fmt.Errorf("no OIDs to poll")
	}

	var result *gosnmp.SnmpPacket
	var err error
	backoff := p.cfg.Retry.BackoffInitial
	if backoff <= 0 {
		backoff = time.Second
	}
	maxBackoff := p.cfg.Retry.BackoffMax
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}
	maxAttempts := p.cfg.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}

		result, err = p.getter.Get(oids)
		if err == nil {
			break
		}

		if isTimeout(err) {
			p.logger.Warn("SNMP timeout, retrying",
				"attempt", attempt+1,
				"max_attempts", maxAttempts,
				"error", err,
			)
			continue
		}

		// Auth failure or other error - don't retry
		p.logger.Error("SNMP error", "error", err)
		return nil, err
	}

	if err != nil {
		return nil, fmt.Errorf("SNMP get after %d attempts: %w", maxAttempts, err)
	}

	values := parseVariables(result.Variables, p.logger)
	metrics := p.buildMetrics(values)

	// One RawReading per rack
	now := time.Now()
	readings := make([]adapter.RawReading, 0, len(p.target.RackIDs))
	for _, rackID := range p.target.RackIDs {
		readings = append(readings, adapter.RawReading{
			AdapterName: adapterName,
			Source:      rackID,
			Timestamp:   now,
			Metrics:     copyMetrics(metrics),
		})
	}

	return readings, nil
}

func (p *SNMPPoller) collectOIDs() []string {
	var oids []string
	seen := make(map[string]bool)
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			oids = append(oids, s)
		}
	}
	add(p.oidMap.TotalPowerKW)
	add(p.oidMap.VoltageV)
	add(p.oidMap.CurrentA)
	for _, o := range p.oidMap.PhaseCurrentA {
		add(o)
	}
	add(p.oidMap.InletTempC)
	add(p.oidMap.Status)
	return oids
}

func parseVariables(vars []gosnmp.SnmpPDU, logger *slog.Logger) map[string]float64 {
	values := make(map[string]float64)
	for _, v := range vars {
		if v.Name == "" {
			continue
		}
		f, ok := snmpValueToFloat64(v)
		if !ok {
			logger.Debug("OID not found or non-numeric, skipping",
				"oid", v.Name,
				"type", v.Type,
			)
			continue
		}
		// gosnmp returns OIDs with a leading dot; strip it so keys match the OID map.
		name := strings.TrimPrefix(v.Name, ".")
		values[name] = f
	}
	return values
}

func snmpValueToFloat64(v gosnmp.SnmpPDU) (float64, bool) {
	switch v.Type {
	case gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.EndOfMibView:
		return 0, false
	}
	if v.Value == nil {
		return 0, false
	}
	switch val := v.Value.(type) {
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case float32:
		return float64(val), true
	case float64:
		return val, true
	case []byte:
		// OctetString - try to parse as number
		s := string(val)
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func (p *SNMPPoller) buildMetrics(values map[string]float64) map[string]interface{} {
	metrics := make(map[string]interface{})

	if v, ok := values[p.oidMap.TotalPowerKW]; ok && p.oidMap.PowerMultiplier > 0 {
		metrics[MetricPowerKW] = v / p.oidMap.PowerMultiplier
	}
	if v, ok := values[p.oidMap.VoltageV]; ok {
		metrics[MetricVoltageV] = v
	}
	if v, ok := values[p.oidMap.CurrentA]; ok && p.oidMap.CurrentMultiplier > 0 {
		metrics[MetricCurrentA] = v / p.oidMap.CurrentMultiplier
	}
	for i, oid := range p.oidMap.PhaseCurrentA {
		if v, ok := values[oid]; ok && p.oidMap.CurrentMultiplier > 0 {
			key := MetricPhaseCurrent1
			switch i {
			case 1:
				key = MetricPhaseCurrent2
			case 2:
				key = MetricPhaseCurrent3
			}
			metrics[key] = v / p.oidMap.CurrentMultiplier
		}
	}
	if p.oidMap.InletTempC != "" {
		if v, ok := values[p.oidMap.InletTempC]; ok {
			metrics[MetricInletTempC] = v
		}
	}
	if p.oidMap.Status != "" {
		if v, ok := values[p.oidMap.Status]; ok {
			metrics[MetricStatus] = v
		}
	}

	return metrics
}

func copyMetrics(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// isTimeout detects SNMP timeout errors via string matching because gosnmp and
// the net package do not expose a stable typed timeout error across Go/gosnmp
// versions. This is intentionally fragile — if gosnmp introduces a typed
// timeout error in the future, prefer errors.Is/As over string matching.
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "i/o timeout") ||
		strings.Contains(err.Error(), "deadline exceeded")
}
