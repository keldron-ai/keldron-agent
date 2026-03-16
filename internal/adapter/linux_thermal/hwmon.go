//go:build linux

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package linux_thermal

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// HwmonSensor represents a single temperature sensor from hwmon.
type HwmonSensor struct {
	Path       string  // e.g. /sys/class/hwmon/hwmon0
	Name       string  // e.g. "coretemp"
	SensorType string  // "cpu", "gpu", "nvme", "soc", "other"
	TempC      float64
	TempMaxC   float64 // -1 if unavailable
	TempCritC  float64 // -1 if unavailable
	Label      string  // e.g. "Package id 0"
}

// classifySensorType maps hwmon name to sensor type.
func classifySensorType(name string) string {
	n := strings.ToLower(name)
	switch {
	case n == "coretemp", n == "k10temp":
		return "cpu"
	case strings.HasPrefix(n, "cpu"):
		return "cpu"
	case strings.Contains(n, "amdgpu"), strings.Contains(n, "nvidia"), strings.Contains(n, "radeon"):
		return "gpu"
	case strings.Contains(n, "nvme"):
		return "nvme"
	case strings.HasPrefix(n, "soc"), strings.HasPrefix(n, "thermal"):
		return "soc"
	default:
		return "other"
	}
}

// DiscoverHwmon walks basePath (e.g. /sys/class/hwmon), discovers hwmon devices,
// and returns all temperature sensors. Skips sensors on permission error or
// malformed data; never panics.
func DiscoverHwmon(basePath string, logger *slog.Logger) []HwmonSensor {
	if basePath == "" {
		basePath = "/sys/class/hwmon"
	}
	var sensors []HwmonSensor

	entries, err := os.ReadDir(basePath)
	if err != nil {
		if logger != nil {
			logger.Debug("hwmon read dir failed", "path", basePath, "error", err)
		}
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "hwmon") {
			continue
		}
		hwmonPath := filepath.Join(basePath, name)
		ss := readHwmonDevice(hwmonPath, logger)
		sensors = append(sensors, ss...)
	}

	return sensors
}

func readHwmonDevice(hwmonPath string, logger *slog.Logger) []HwmonSensor {
	nameBytes, err := os.ReadFile(filepath.Join(hwmonPath, "name"))
	if err != nil {
		if logger != nil {
			logger.Debug("hwmon read name failed", "path", hwmonPath, "error", err)
		}
		return nil
	}
	deviceName := strings.TrimSpace(string(nameBytes))

	// Find temp*_input files
	entries, err := os.ReadDir(hwmonPath)
	if err != nil {
		if logger != nil {
			logger.Debug("hwmon read dir failed", "path", hwmonPath, "error", err)
		}
		return nil
	}

	var sensors []HwmonSensor
	seen := make(map[int]bool)

	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "temp") || !strings.HasSuffix(n, "_input") {
			continue
		}
		// temp1_input -> 1
		numStr := strings.TrimPrefix(n, "temp")
		numStr = strings.TrimSuffix(numStr, "_input")
		num, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		if seen[num] {
			continue
		}
		seen[num] = true

		s := readTempInput(hwmonPath, deviceName, num, logger)
		if s != nil {
			sensors = append(sensors, *s)
		}
	}

	return sensors
}

func readTempInput(hwmonPath, deviceName string, num int, logger *slog.Logger) *HwmonSensor {
	prefix := "temp" + strconv.Itoa(num)
	inputPath := filepath.Join(hwmonPath, prefix+"_input")

	data, err := os.ReadFile(inputPath)
	if err != nil {
		if logger != nil {
			logger.Debug("hwmon read temp_input failed", "path", inputPath, "error", err)
		}
		return nil
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		if logger != nil {
			logger.Warn("hwmon malformed temp value", "path", inputPath, "value", string(data))
		}
		return nil
	}
	tempC := float64(val) / 1000.0

	label := ""
	if labelData, err := os.ReadFile(filepath.Join(hwmonPath, prefix+"_label")); err == nil {
		label = strings.TrimSpace(string(labelData))
	}

	tempMaxC := -1.0
	if maxData, err := os.ReadFile(filepath.Join(hwmonPath, prefix+"_max")); err == nil {
		if mx, err := strconv.ParseInt(strings.TrimSpace(string(maxData)), 10, 64); err == nil {
			tempMaxC = float64(mx) / 1000.0
		}
	}

	tempCritC := -1.0
	if critData, err := os.ReadFile(filepath.Join(hwmonPath, prefix+"_crit")); err == nil {
		if cx, err := strconv.ParseInt(strings.TrimSpace(string(critData)), 10, 64); err == nil {
			tempCritC = float64(cx) / 1000.0
		}
	}

	return &HwmonSensor{
		Path:       hwmonPath,
		Name:       deviceName,
		SensorType: classifySensorType(deviceName),
		TempC:      tempC,
		TempMaxC:   tempMaxC,
		TempCritC:  tempCritC,
		Label:      label,
	}
}
