package rocm

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
)

// GPUReading holds normalized metrics for a single GPU.
type GPUReading struct {
	GPUID              int
	GPUTemp            float64 // Celsius
	GPUUtilization     float64 // 0-100
	GPUMemoryUsed      float64 // bytes
	GPUMemoryTotal     float64 // bytes
	GPUPowerW          float64 // watts
	ThrottleReason     string  // "none", "thermal_throttle", "power_throttle"
	ThrottleReasonCode float64 // 0=none, 1=thermal, 2=power
	GPUModel           string  // e.g., "MI300X", "MI355X"
}

// Throttle reason codes for platform compatibility.
const (
	ThrottleNone    = 0
	ThrottleThermal = 1
	ThrottlePower   = 2
)

// rocm6Format is the ROCm 6.x JSON structure.
type rocm6Format struct {
	GPU []rocm6GPU `json:"gpu"`
}

type rocm6GPU struct {
	GPUID           int      `json:"gpu_id"`
	TemperatureEdge *float64 `json:"temperature_edge"`
	GPUUsePercent   *float64 `json:"gpu_use_percent"`
	VRAMUsed        *float64 `json:"vram_used_mb"`
	VRAMTotal       *float64 `json:"vram_total_mb"`
	SocketPower     *float64 `json:"average_socket_power"`
	ThrottleStatus  *string  `json:"throttle_status"`
	GPUName         *string  `json:"gpu_name"`
}

// ParseROCmOutput parses ROCm SMI JSON output. Supports ROCm 5.x (card-based)
// and ROCm 6.x (array-based) formats. Returns partial readings on partial output.
func ParseROCmOutput(data []byte, logger *slog.Logger) ([]GPUReading, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty rocm-smi output")
	}

	// Try ROCm 6.x format first.
	var v6 rocm6Format
	if err := json.Unmarshal(data, &v6); err == nil && len(v6.GPU) > 0 {
		return parseROCm6(v6, logger)
	}

	// Fall back to ROCm 5.x format (card0, card1, ...).
	var v5 map[string]map[string]interface{}
	if err := json.Unmarshal(data, &v5); err != nil {
		return nil, fmt.Errorf("invalid rocm-smi JSON: %w", err)
	}

	return parseROCm5(v5, logger)
}

func parseROCm6(v6 rocm6Format, logger *slog.Logger) ([]GPUReading, error) {
	readings := make([]GPUReading, 0, len(v6.GPU))
	for i, g := range v6.GPU {
		r := GPUReading{GPUID: g.GPUID}
		// ROCm may report gpu_id as 0 for all GPUs when the field is missing.
		// Use the array index as a best-effort ID for non-first entries.
		// Note: a legitimate GPU 0 appearing at index >0 would be mis-assigned.
		if g.GPUID == 0 && i != 0 {
			r.GPUID = i
		}

		if g.TemperatureEdge != nil {
			r.GPUTemp = *g.TemperatureEdge
		}
		if g.GPUUsePercent != nil {
			r.GPUUtilization = *g.GPUUsePercent
		}
		if g.VRAMUsed != nil {
			r.GPUMemoryUsed = *g.VRAMUsed * 1024 * 1024 // MB -> bytes
		}
		if g.VRAMTotal != nil {
			r.GPUMemoryTotal = *g.VRAMTotal * 1024 * 1024 // MB -> bytes
		}
		if g.SocketPower != nil {
			r.GPUPowerW = *g.SocketPower
		}
		if g.ThrottleStatus != nil {
			r.ThrottleReason, r.ThrottleReasonCode = normalizeThrottle(*g.ThrottleStatus, logger)
		} else {
			r.ThrottleReason = "none"
			r.ThrottleReasonCode = ThrottleNone
		}
		if g.GPUName != nil {
			r.GPUModel = detectGPUModel(*g.GPUName)
		}
		if r.GPUModel == "" {
			r.GPUModel = "unknown" // no gpu_name field in JSON
		}

		readings = append(readings, r)
	}
	return readings, nil
}

var cardRe = regexp.MustCompile(`^card(\d+)$`)

func parseROCm5(v5 map[string]map[string]interface{}, logger *slog.Logger) ([]GPUReading, error) {
	var cardIndices []int
	for k := range v5 {
		if m := cardRe.FindStringSubmatch(k); m != nil {
			idx, _ := strconv.Atoi(m[1])
			cardIndices = append(cardIndices, idx)
		}
	}
	if len(cardIndices) == 0 {
		return nil, fmt.Errorf("no card entries in ROCm 5.x JSON")
	}
	sort.Ints(cardIndices)

	readings := make([]GPUReading, 0, len(cardIndices))
	for _, idx := range cardIndices {
		cardKey := fmt.Sprintf("card%d", idx)
		card, ok := v5[cardKey]
		if !ok {
			continue
		}
		r := parseROCm5Card(idx, card, logger)
		readings = append(readings, r)
	}
	return readings, nil
}

func parseROCm5Card(gpuID int, card map[string]interface{}, logger *slog.Logger) GPUReading {
	r := GPUReading{GPUID: gpuID, ThrottleReason: "none", ThrottleReasonCode: ThrottleNone, GPUModel: "unknown"}

	// Temperature - various possible keys
	for _, key := range []string{
		"Temperature (Sensor edge) (C)",
		"Temperature (Sensor junction) (C)",
		"Temperature (Sensor memory) (C)",
		"Temperature (Sensor edge)",
		"temperature_edge",
	} {
		if v, ok := getFloat(card, key); ok {
			r.GPUTemp = v
			break
		}
	}

	// GPU utilization
	for _, key := range []string{"GPU use (%)", "GPU use %", "gpu_use_percent", "GPU utilization (%)"} {
		if v, ok := getFloat(card, key); ok {
			r.GPUUtilization = v
			break
		}
	}

	// VRAM - ROCm 5.x often reports in MB
	for _, key := range []string{"VRAM Total Used (B)", "VRAM Total Used (MB)", "vram_used_mb"} {
		if v, ok := getFloat(card, key); ok {
			if strings.Contains(key, "MB)") || strings.Contains(key, "vram_used_mb") {
				r.GPUMemoryUsed = v * 1024 * 1024
			} else {
				r.GPUMemoryUsed = v
			}
			break
		}
	}
	for _, key := range []string{"VRAM Total (B)", "VRAM Total (MB)", "vram_total_mb"} {
		if v, ok := getFloat(card, key); ok {
			if strings.Contains(key, "MB)") || strings.Contains(key, "vram_total_mb") {
				r.GPUMemoryTotal = v * 1024 * 1024
			} else {
				r.GPUMemoryTotal = v
			}
			break
		}
	}

	// Power
	for _, key := range []string{
		"Current Socket Power (W)",
		"Average Socket Power (W)",
		"Socket Power",
		"average_socket_power",
	} {
		if v, ok := getFloat(card, key); ok {
			r.GPUPowerW = v
			break
		}
	}

	// Throttle
	for _, key := range []string{"Throttle Status", "throttle_status"} {
		if v, ok := card[key]; ok {
			if s, ok := v.(string); ok {
				r.ThrottleReason, r.ThrottleReasonCode = normalizeThrottle(s, logger)
				break
			}
		}
	}

	// GPU name/model
	for _, key := range []string{"Card model", "gpu_name", "GPU Name"} {
		if v, ok := card[key]; ok {
			if s, ok := v.(string); ok {
				r.GPUModel = detectGPUModel(s)
				break
			}
		}
	}

	return r
}

func getFloat(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func normalizeThrottle(s string, logger *slog.Logger) (string, float64) {
	upper := strings.ToUpper(strings.TrimSpace(s))
	switch {
	case upper == "" || upper == "NONE" || upper == "0":
		return "none", ThrottleNone
	case strings.Contains(upper, "THERMAL"):
		return "thermal_throttle", ThrottleThermal
	case strings.Contains(upper, "POWER") || strings.Contains(upper, "CURRENT"):
		return "power_throttle", ThrottlePower
	default:
		if logger != nil {
			logger.Debug("unknown throttle value, defaulting to none", "value", upper)
		}
		return "none", ThrottleNone
	}
}

func detectGPUModel(name string) string {
	upper := strings.ToUpper(name)
	if strings.Contains(upper, "MI355") || strings.Contains(upper, "MI 355") {
		return "MI355X"
	}
	if strings.Contains(upper, "MI300") || strings.Contains(upper, "MI 300") {
		return "MI300X"
	}
	if strings.Contains(upper, "MI250") || strings.Contains(upper, "MI 250") {
		return "MI250X"
	}
	if strings.Contains(upper, "MI210") || strings.Contains(upper, "MI 210") {
		return "MI210"
	}
	return name
}

// Canonical metric keys for vendor-neutral telemetry (same schema as DCGM).
const (
	MetricGPUTemp            = "gpu_temp"
	MetricGPUUtilization     = "gpu_utilization"
	MetricGPUMemoryUsed      = "gpu_memory_used"
	MetricGPUMemoryTotal     = "gpu_memory_total"
	MetricGPUPowerW          = "gpu_power_w"
	MetricThrottleReason     = "throttle_reason"
	MetricThrottleReasonCode = "throttle_reason_code"
	MetricGPUID              = "gpu_id"
	MetricGPUVendor          = "gpu_vendor"
	MetricGPUModel           = "gpu_model"
)

// ToRawReading converts a GPUReading to an adapter.RawReading with canonical metric keys.
func (r *GPUReading) ToRawReading(source string) adapter.RawReading {
	metrics := map[string]interface{}{
		MetricGPUID:              r.GPUID,
		MetricGPUVendor:          "amd",
		MetricGPUModel:           r.GPUModel,
		MetricGPUTemp:            r.GPUTemp,
		MetricGPUUtilization:     r.GPUUtilization,
		MetricGPUMemoryUsed:      r.GPUMemoryUsed,
		MetricGPUMemoryTotal:     r.GPUMemoryTotal,
		MetricGPUPowerW:          r.GPUPowerW,
		MetricThrottleReason:     r.ThrottleReason,
		MetricThrottleReasonCode: r.ThrottleReasonCode,
	}
	return adapter.RawReading{
		AdapterName: "rocm",
		Source:      source,
		Timestamp:   time.Now(),
		Metrics:     metrics,
	}
}
