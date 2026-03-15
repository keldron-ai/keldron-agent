package registry

// Metric key constants for normalized stress values.
const (
	MetricThermalStress = "thermal_stress"
	MetricPowerStress  = "power_stress"
)

// NormalizeThermal converts raw temperature (°C) to 0.0-1.0 thermal stress ratio.
// Returns tempC / spec.ThermalLimitC. May exceed 1.0 if above limit.
func NormalizeThermal(tempC float64, spec GPUSpec) float64 {
	if spec.ThermalLimitC <= 0 {
		return 0
	}
	return tempC / spec.ThermalLimitC
}

// NormalizePower converts raw power (W) to 0.0-1.0 power stress ratio.
// Returns powerW / spec.TDPW. May exceed 1.0 if above TDP.
func NormalizePower(powerW float64, spec GPUSpec) float64 {
	if spec.TDPW <= 0 {
		return 0
	}
	return powerW / spec.TDPW
}

// ApplyEdgeToJunctionCorrection applies AMD edge-to-junction correction.
// AMD MI250X reports edge temp which is typically 10-15°C lower than junction.
// Applies +12°C offset for edge sensors; returns value unchanged for junction/soc_package.
func ApplyEdgeToJunctionCorrection(edgeTempC float64, spec GPUSpec) float64 {
	if spec.TempMeasurementType == "edge" {
		return edgeTempC + 12.0
	}
	return edgeTempC
}
