// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package snmp_pdu implements the PDU power monitoring adapter via SNMP v2c/v3.
package snmp_pdu

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// SNMPVersion is the SNMP protocol version.
type SNMPVersion string

const (
	SNMPv2c SNMPVersion = "v2c"
	SNMPv3  SNMPVersion = "v3"
)

// AuthProtocol is the SNMP v3 authentication protocol.
type AuthProtocol string

const (
	AuthMD5    AuthProtocol = "MD5"
	AuthSHA    AuthProtocol = "SHA"
	AuthSHA256 AuthProtocol = "SHA256"
)

// PrivProtocol is the SNMP v3 privacy protocol.
type PrivProtocol string

const (
	PrivDES    PrivProtocol = "DES"
	PrivAES    PrivProtocol = "AES"
	PrivAES256 PrivProtocol = "AES256"
)

// PDUTarget represents a single PDU to poll.
type PDUTarget struct {
	Address string   `yaml:"address"`
	PDUID   string   `yaml:"pdu_id"`
	RackIDs []string `yaml:"rack_ids"`
	Vendor  string   `yaml:"vendor"` // servertech, raritan, apc, generic
}

// RetryConfig holds retry and backoff settings.
type RetryConfig struct {
	MaxAttempts    int           `yaml:"max_attempts"`
	BackoffInitial time.Duration `yaml:"backoff_initial"`
	BackoffMax     time.Duration `yaml:"backoff_max"`
}

// SNMPPDUConfig holds PDU-specific configuration decoded from the adapter's Raw YAML node.
type SNMPPDUConfig struct {
	Version        string        `yaml:"version"` // "v2c" or "v3"
	Community      string        `yaml:"community"`
	Username       string        `yaml:"username"`
	AuthProtocol   string        `yaml:"auth_protocol"`
	AuthPassphrase string        `yaml:"auth_passphrase"`
	PrivProtocol   string        `yaml:"priv_protocol"`
	PrivPassphrase string        `yaml:"priv_passphrase"`
	Targets        []PDUTarget   `yaml:"targets"`
	Retry          RetryConfig   `yaml:"retry"`
	Timeout        time.Duration `yaml:"timeout"`
}

// DecodeFromRaw decodes SNMPPDUConfig from a YAML node.
func DecodeFromRaw(raw *yaml.Node) (*SNMPPDUConfig, error) {
	if raw == nil || raw.Kind == 0 {
		return nil, fmt.Errorf("snmp_pdu: config raw node is empty")
	}
	var cfg SNMPPDUConfig
	if err := raw.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decoding snmp_pdu config: %w", err)
	}
	return &cfg, nil
}

// ApplyDefaults fills in zero-value fields with sensible defaults.
// Must be called before Validate.
func (c *SNMPPDUConfig) ApplyDefaults() {
	for i := range c.Targets {
		if c.Targets[i].Vendor == "" {
			c.Targets[i].Vendor = "generic"
		}
	}
	if c.Retry.MaxAttempts <= 0 {
		c.Retry.MaxAttempts = 3
	}
	if c.Retry.BackoffInitial <= 0 {
		c.Retry.BackoffInitial = time.Second
	}
	if c.Retry.BackoffMax <= 0 {
		c.Retry.BackoffMax = 30 * time.Second
	}
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}
}

// Validate checks the config and returns an error if invalid.
// It does not mutate the receiver; call ApplyDefaults first.
func (c *SNMPPDUConfig) Validate() error {
	if c.Version != string(SNMPv2c) && c.Version != string(SNMPv3) {
		return fmt.Errorf("snmp_pdu: version must be %q or %q (got %q)", SNMPv2c, SNMPv3, c.Version)
	}
	if c.Version == string(SNMPv2c) && c.Community == "" {
		return fmt.Errorf("snmp_pdu: community is required for v2c")
	}
	if c.Version == string(SNMPv3) {
		if c.Username == "" {
			return fmt.Errorf("snmp_pdu: username is required for v3")
		}
		// Reciprocal: passphrase requires protocol and vice versa.
		if c.AuthPassphrase != "" && c.AuthProtocol == "" {
			return fmt.Errorf("snmp_pdu: auth_protocol is required when auth_passphrase is set")
		}
		if c.AuthProtocol != "" && c.AuthPassphrase == "" {
			return fmt.Errorf("snmp_pdu: auth_passphrase is required when auth_protocol is set")
		}
		if c.PrivPassphrase != "" && c.PrivProtocol == "" {
			return fmt.Errorf("snmp_pdu: priv_protocol is required when priv_passphrase is set")
		}
		if c.PrivProtocol != "" && c.PrivPassphrase == "" {
			return fmt.Errorf("snmp_pdu: priv_passphrase is required when priv_protocol is set")
		}
		// Privacy requires authentication.
		if c.PrivPassphrase != "" && c.AuthPassphrase == "" {
			return fmt.Errorf("snmp_pdu: auth_passphrase is required when priv_passphrase is set (privacy requires authentication)")
		}
		// Validate protocol values.
		if c.AuthProtocol != "" {
			switch AuthProtocol(c.AuthProtocol) {
			case AuthMD5, AuthSHA, AuthSHA256:
			default:
				return fmt.Errorf("snmp_pdu: auth_protocol must be one of [MD5, SHA, SHA256] (got %q)", c.AuthProtocol)
			}
		}
		if c.PrivProtocol != "" {
			switch PrivProtocol(c.PrivProtocol) {
			case PrivDES, PrivAES, PrivAES256:
			default:
				return fmt.Errorf("snmp_pdu: priv_protocol must be one of [DES, AES, AES256] (got %q)", c.PrivProtocol)
			}
		}
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("snmp_pdu: at least one target is required")
	}
	for i, t := range c.Targets {
		if t.Address == "" {
			return fmt.Errorf("snmp_pdu: targets[%d].address is required", i)
		}
		if t.PDUID == "" {
			return fmt.Errorf("snmp_pdu: targets[%d].pdu_id is required", i)
		}
		if len(t.RackIDs) == 0 {
			return fmt.Errorf("snmp_pdu: targets[%d].rack_ids must not be empty", i)
		}
		switch c.Targets[i].Vendor {
		case VendorServerTech, VendorRaritan, VendorAPC, VendorGeneric:
		default:
			return fmt.Errorf("snmp_pdu: targets[%d].vendor must be one of [%s, %s, %s, %s] (got %q)",
				i, VendorServerTech, VendorRaritan, VendorAPC, VendorGeneric, c.Targets[i].Vendor)
		}
	}
	return nil
}
