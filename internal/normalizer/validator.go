package normalizer

import (
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
)

// ValidationResult describes why a reading was accepted or rejected.
type ValidationResult struct {
	Valid  bool
	Reason string // Empty if valid, rejection reason if invalid
}

// Validate checks a RawReading against all validation rules.
// Returns the first failure encountered.
func Validate(reading adapter.RawReading, maxSkew time.Duration) ValidationResult {
	if reading.Source == "" {
		return ValidationResult{Valid: false, Reason: "source must not be empty"}
	}

	if reading.AdapterName == "" {
		return ValidationResult{Valid: false, Reason: "adapter name must not be empty"}
	}

	if len(reading.Metrics) == 0 {
		return ValidationResult{Valid: false, Reason: "metrics must not be empty"}
	}

	if reading.Timestamp.IsZero() {
		return ValidationResult{Valid: false, Reason: "timestamp must not be zero"}
	}

	skew := time.Since(reading.Timestamp)
	if skew < 0 {
		skew = -skew
	}
	if skew > maxSkew {
		return ValidationResult{Valid: false, Reason: "timestamp skew exceeds maximum"}
	}

	return ValidationResult{Valid: true}
}

// ResolveRackID maps a source hostname to a rack ID from the config mapping.
// Returns ("", false) if no mapping exists for the source.
func ResolveRackID(source string, rackMapping map[string]string) (string, bool) {
	rackID, ok := rackMapping[source]
	return rackID, ok
}

// CoerceToFloat64 converts adapter metric values to float64.
// Returns (0, false) for unrecognized types (e.g., string), which are skipped.
func CoerceToFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	case uint32:
		return float64(val), true
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}
