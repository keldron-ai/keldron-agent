// Package snmp_pdu implements the PDU power monitoring adapter via SNMP v2c/v3.
package snmp_pdu

// VendorOIDMap defines the SNMP OID tree for power metrics per vendor.
// Each vendor uses different MIBs; OIDs are documented with MIB references.
type VendorOIDMap struct {
	// TotalPowerKW is the OID for total PDU power draw (in vendor-specific units; see PowerMultiplier).
	TotalPowerKW string
	// PowerMultiplier: divide raw value by this to get kW (e.g. 1000 for watts, 100 for hundredths kW).
	PowerMultiplier float64
	// PerOutletPowerW are OIDs for per-outlet power (variable length by PDU model).
	PerOutletPowerW []string
	// VoltageV is the OID for input voltage (volts).
	VoltageV string
	// CurrentA is the OID for total current draw (amps or deciamps; see CurrentMultiplier).
	CurrentA string
	// CurrentMultiplier: divide raw value by this to get amps (e.g. 10 for deciamps).
	CurrentMultiplier float64
	// PhaseCurrentA are OIDs for per-phase current (for 3-phase PDUs).
	PhaseCurrentA []string
	// InletTempC is the OID for PDU inlet temperature sensor (optional).
	InletTempC string
	// Status is the OID for PDU operational status (optional).
	Status string
}

// Vendor constants for OID map lookup.
const (
	VendorServerTech = "servertech"
	VendorRaritan    = "raritan"
	VendorAPC        = "apc"
	VendorGeneric    = "generic"
)

// GetVendorOIDMap returns a defensive copy of the OID map for the given vendor.
// Returns a copy of the generic map for unknown vendors.
func GetVendorOIDMap(vendor string) VendorOIDMap {
	var m VendorOIDMap
	switch vendor {
	case VendorServerTech:
		m = OIDMapServerTech
	case VendorRaritan:
		m = OIDMapRaritan
	case VendorAPC:
		m = OIDMapAPC
	default:
		m = OIDMapGeneric
	}
	// Deep-copy slice fields so callers cannot mutate shared backing arrays.
	if m.PhaseCurrentA != nil {
		m.PhaseCurrentA = append([]string(nil), m.PhaseCurrentA...)
	}
	if m.PerOutletPowerW != nil {
		m.PerOutletPowerW = append([]string(nil), m.PerOutletPowerW...)
	}
	return m
}

// OIDMapServerTech is for ServerTech Sentry4/PRO2 PDUs.
// MIB: Sentry4.mib (Server Technology)
// Ref: https://www.servertech.com/support/sentry-mib-oid-tree-downloads/
// Power in watts -> kW: /1000. Current in deciamps -> amps: /10.
var OIDMapServerTech = VendorOIDMap{
	TotalPowerKW:      "1.3.6.1.4.1.1718.3.2.2.1.11.0",
	PowerMultiplier:   1000,
	VoltageV:          "1.3.6.1.4.1.1718.3.2.2.1.7.0",
	CurrentA:          "1.3.6.1.4.1.1718.3.2.2.1.8.0",
	CurrentMultiplier: 10,
	PhaseCurrentA: []string{
		"1.3.6.1.4.1.1718.3.2.2.1.9.1.0",
		"1.3.6.1.4.1.1718.3.2.2.1.9.2.0",
		"1.3.6.1.4.1.1718.3.2.2.1.9.3.0",
	},
	InletTempC: "",
	Status:     "1.3.6.1.4.1.1718.3.2.2.1.2.0",
}

// OIDMapRaritan is for Raritan PX3/PDU2 PDUs.
// MIB: RARITAN-PX2-PDU2-MIB, PDU2-MIB. Power in watts, current in amps.
var OIDMapRaritan = VendorOIDMap{
	TotalPowerKW:      "1.3.6.1.4.1.13742.6.5.2.3.1.4.1.1.5.1",
	PowerMultiplier:   1000,
	VoltageV:          "1.3.6.1.4.1.13742.6.5.2.3.1.4.1.1.4.1",
	CurrentA:          "1.3.6.1.4.1.13742.6.5.2.3.1.4.1.1.3.1",
	CurrentMultiplier: 1,
	PhaseCurrentA: []string{
		"1.3.6.1.4.1.13742.6.5.2.3.1.4.1.1.3.1",
		"1.3.6.1.4.1.13742.6.5.2.3.1.4.2.1.3.1",
		"1.3.6.1.4.1.13742.6.5.2.3.1.4.3.1.3.1",
	},
	InletTempC: "",
	Status:     "1.3.6.1.4.1.13742.6.3.2.1.1.0",
}

// OIDMapAPC is for APC PowerNet Rack PDU 2G devices.
// MIB: PowerNet-MIB. Power in hundredths of kW (500=5kW), current in tenths of A.
var OIDMapAPC = VendorOIDMap{
	TotalPowerKW:      "1.3.6.1.4.1.318.1.1.26.4.3.1.5.1",
	PowerMultiplier:   100,
	VoltageV:          "1.3.6.1.4.1.318.1.1.26.4.3.1.2.1",
	CurrentA:          "1.3.6.1.4.1.318.1.1.26.8.3.1.5.1",
	CurrentMultiplier: 10,
	PhaseCurrentA: []string{
		"1.3.6.1.4.1.318.1.1.26.8.3.1.5.1",
		"1.3.6.1.4.1.318.1.1.26.8.3.1.5.2",
		"1.3.6.1.4.1.318.1.1.26.8.3.1.5.3",
	},
	InletTempC: "",
	Status:     "1.3.6.1.4.1.318.1.1.26.4.1.1.1.0",
}

// OIDMapGeneric is the RFC 1628 UPS-MIB fallback for basic power/voltage/current.
// MIB: UPS-MIB (RFC 1628). Power in watts, current in amps.
var OIDMapGeneric = VendorOIDMap{
	TotalPowerKW:      "1.3.6.1.2.1.33.1.4.4.1.2.1",
	PowerMultiplier:   1000,
	VoltageV:          "1.3.6.1.2.1.33.1.3.3.1.3.1",
	CurrentA:          "1.3.6.1.2.1.33.1.3.3.1.4.1",
	CurrentMultiplier: 1,
	PhaseCurrentA: []string{
		"1.3.6.1.2.1.33.1.3.3.1.4.1",
	},
	InletTempC: "",
	Status:     "1.3.6.1.2.1.33.1.2.1.0",
}
