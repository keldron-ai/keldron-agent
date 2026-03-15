package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseByteSize parses a human-readable byte size string (e.g. "500MB") into
// the number of bytes as int64. Supported suffixes: B, KB, MB, GB, TB
// (case-insensitive). Whitespace between the number and suffix is tolerated.
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty byte size string")
	}

	// Find where the numeric part ends.
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}

	numStr := strings.TrimSpace(s[:i])
	suffix := strings.TrimSpace(s[i:])

	if numStr == "" {
		return 0, fmt.Errorf("no numeric value in byte size %q", s)
	}

	// Note: negative values are rejected by the scanner which only accepts digits and dots.
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing byte size %q: %w", s, err)
	}

	var multiplier float64
	switch strings.ToUpper(suffix) {
	case "", "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unrecognized byte size suffix %q in %q", suffix, s)
	}

	return int64(val * multiplier), nil
}
