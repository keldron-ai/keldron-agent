package temperature

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/keldron-ai/keldron-agent/internal/adapter"
)

// PollSNMP performs an SNMP GET on the configured OID and returns a RawReading.
// Handles vendor encoding: tenths-of-degree (235=23.5°C), float string, or raw integer.
func PollSNMP(cfg SensorConfig) (adapter.RawReading, error) {
	host, port, err := parseAddress(cfg.Address, 161)
	if err != nil {
		return adapter.RawReading{}, fmt.Errorf("parse address: %w", err)
	}
	if cfg.OID == "" {
		return adapter.RawReading{}, fmt.Errorf("oid is required for SNMP sensor")
	}

	community := cfg.Community
	if community == "" {
		community = "public"
	}
	snmp := &gosnmp.GoSNMP{
		Target:    host,
		Port:      uint16(port),
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   10 * time.Second,
	}

	if err := snmp.Connect(); err != nil {
		return adapter.RawReading{}, fmt.Errorf("connect: %w", err)
	}
	if snmp.Conn != nil {
		defer snmp.Conn.Close()
	}

	result, err := snmp.Get([]string{cfg.OID})
	if err != nil {
		return adapter.RawReading{}, fmt.Errorf("get: %w", err)
	}
	if result.Error != gosnmp.NoError {
		return adapter.RawReading{}, fmt.Errorf("snmp error: %v", result.Error)
	}
	if len(result.Variables) == 0 {
		return adapter.RawReading{}, fmt.Errorf("no variables in response")
	}

	pdu := result.Variables[0]
	tempC, err := parseSNMPValue(pdu.Value, cfg.Encoding)
	if err != nil {
		return adapter.RawReading{}, fmt.Errorf("parse value: %w", err)
	}

	metricKey := metricKeyForPosition(cfg.Position)
	metrics := map[string]interface{}{
		metricKey: tempC,
		"stale":   0.0,
	}

	return adapter.RawReading{
		AdapterName: "temperature",
		Source:      cfg.SensorID,
		Timestamp:   time.Now(),
		Metrics:     metrics,
	}, nil
}

// parseSNMPValue converts an SNMP PDU value to temperature in °C.
// encoding: "tenths" = integer is tenths-of-degree (235->23.5), "raw" = use as-is, "float" = parse string.
func parseSNMPValue(val interface{}, encoding string) (float64, error) {
	switch v := val.(type) {
	case int:
		return parseSNMPInt(int64(v), encoding)
	case int32:
		return parseSNMPInt(int64(v), encoding)
	case int64:
		return parseSNMPInt(v, encoding)
	case uint:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("uint value %d overflows int64", v)
		}
		return parseSNMPInt(int64(v), encoding)
	case uint32:
		return parseSNMPInt(int64(v), encoding)
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("uint64 value %d overflows int64", v)
		}
		return parseSNMPInt(int64(v), encoding)
	case float32:
		return applyEncoding(float64(v), encoding)
	case float64:
		return applyEncoding(v, encoding)
	case []byte:
		f, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return 0, err
		}
		return applyEncoding(f, encoding)
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, err
		}
		return applyEncoding(f, encoding)
	default:
		return 0, fmt.Errorf("unsupported SNMP value type: %T", val)
	}
}

func parseSNMPInt(v int64, encoding string) (float64, error) {
	return applyEncoding(float64(v), encoding)
}

func applyEncoding(f float64, encoding string) (float64, error) {
	switch encoding {
	case "tenths", "":
		return f / 10, nil
	case "raw", "float":
		return f, nil
	default:
		return 0, fmt.Errorf("unknown encoding: %q", encoding)
	}
}

func metricKeyForPosition(position string) string {
	if position == "outlet" {
		return "outlet_temp_c"
	}
	return "inlet_temp_c"
}

func parseAddress(addr string, defaultPort int) (host string, port int, err error) {
	if addr == "" {
		return "", 0, fmt.Errorf("address must not be empty")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "missing port") {
			return addr, defaultPort, nil
		}
		// Bare IPv6 address without brackets (e.g., "::1") causes "too many colons".
		if strings.Contains(errMsg, "too many colons") && net.ParseIP(addr) != nil {
			return addr, defaultPort, nil
		}
		return "", 0, fmt.Errorf("invalid address %q: %w", addr, err)
	}
	p, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return host, int(p), nil
}
