// Package registry provides GPU model specifications and telemetry normalization.
package registry

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

//go:embed gpu_specs.json
var gpuSpecsJSON []byte

var (
	registry     map[string]GPUSpec // normalized key -> spec
	registryOnce sync.Once
	appleRe      = regexp.MustCompile(`(?i)^(M\d+)\s*-?\s*(Pro|Max|Ultra)$`)
	aliasMap     = map[string]string{
		"a100-sxm4-80gb": "A100-SXM",
		"a100-sxm4-40gb": "A100-SXM",
		"mi355x":         "MI300X", // MI355X is MI300X variant
	}
)

// GPUSpec holds physical characteristics for a GPU model.
type GPUSpec struct {
	Vendor                        string  `json:"vendor"`
	Architecture                  string  `json:"architecture"`
	ThermalLimitC                 float64 `json:"thermal_limit_c"`
	TDPW                          float64 `json:"tdp_w"`
	TempMeasurementType           string  `json:"temp_measurement_type"`
	BehaviorClass                 string  `json:"behavior_class"`
	CVMax                         float64 `json:"cv_max"`
	ThermalPressureStateSupported bool    `json:"thermal_pressure_state_supported"`
}

// initRegistry loads and parses the embedded JSON, building a case-insensitive lookup map.
func initRegistry() {
	registry = make(map[string]GPUSpec)
	var raw map[string]GPUSpec
	if err := json.Unmarshal(gpuSpecsJSON, &raw); err != nil {
		panic("registry: failed to parse gpu_specs.json: " + err.Error())
	}
	for k, v := range raw {
		registry[strings.ToLower(k)] = v
	}
}

// Lookup returns the spec for a GPU model. If not found, returns a fallback spec
// using default thermal/TDP limits (83°C, 350W).
func Lookup(model string) GPUSpec {
	return LookupWithFallback(model, 83, 350)
}

// LookupWithFallback returns the spec for a GPU model. If not found, builds a fallback
// spec using driver-reported limits (or defaults if zero). Fallback uses
// behavior_class=consumer_active_cooled as a safe default for unknown devices.
func LookupWithFallback(model string, driverThermalLimit, driverTDP float64) GPUSpec {
	registryOnce.Do(initRegistry)
	key := strings.ToLower(NormalizeModelName(model))
	if spec, ok := registry[key]; ok {
		return spec
	}
	// Fallback for unknown models
	if driverThermalLimit <= 0 {
		driverThermalLimit = 83
	}
	if driverTDP <= 0 {
		driverTDP = 350
	}
	return GPUSpec{
		Vendor:                        "unknown",
		Architecture:                  "unknown",
		ThermalLimitC:                 driverThermalLimit,
		TDPW:                          driverTDP,
		TempMeasurementType:           "junction",
		BehaviorClass:                 "consumer_active_cooled",
		CVMax:                         0.60,
		ThermalPressureStateSupported: false,
	}
}

// NormalizeModelName strips vendor prefixes, normalizes spaces to hyphens, and handles known aliases.
func NormalizeModelName(rawName string) string {
	s := strings.TrimSpace(rawName)
	if s == "" {
		return ""
	}
	upper := strings.ToUpper(s)
	// Strip vendor prefixes (case-insensitive)
	for _, prefix := range []string{"NVIDIA ", "AMD ", "APPLE "} {
		if strings.HasPrefix(upper, prefix) {
			s = strings.TrimSpace(s[len(prefix):])
			upper = strings.ToUpper(s)
			break
		}
	}
	// Normalize spaces to hyphens
	s = strings.ReplaceAll(s, " ", "-")
	// Handle known aliases (case-insensitive)
	if a, ok := aliasMap[strings.ToLower(s)]; ok {
		return a
	}
	// Normalize Apple Silicon patterns: "M4 Pro" / "M4Pro" / "M4-Pro" → "M4-Pro"
	if m := appleRe.FindStringSubmatch(s); m != nil {
		suffix := strings.ToLower(m[2])
		r, size := utf8.DecodeRuneInString(suffix)
		suffix = string(unicode.ToUpper(r)) + suffix[size:]
		return strings.ToUpper(m[1]) + "-" + suffix
	}
	return s
}
