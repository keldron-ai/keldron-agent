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

// TripPoint represents a thermal trip point.
type TripPoint struct {
	Type  string
	TempC float64
}

// ThermalZone represents a thermal zone from /sys/class/thermal.
type ThermalZone struct {
	Zone       string
	Type       string
	TempC      float64
	TripPoints []TripPoint
}

// DiscoverThermalZones walks basePath (e.g. /sys/class/thermal), discovers
// thermal_zone* directories, and returns all zones. Skips zones on error; never panics.
func DiscoverThermalZones(basePath string, logger *slog.Logger) []ThermalZone {
	if basePath == "" {
		basePath = "/sys/class/thermal"
	}
	var zones []ThermalZone

	entries, err := os.ReadDir(basePath)
	if err != nil {
		if logger != nil {
			logger.Debug("thermal read dir failed", "path", basePath, "error", err)
		}
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "thermal_zone") {
			continue
		}
		zonePath := filepath.Join(basePath, name)
		z := readThermalZone(zonePath, name, logger)
		if z != nil {
			zones = append(zones, *z)
		}
	}

	return zones
}

func readThermalZone(zonePath, zoneName string, logger *slog.Logger) *ThermalZone {
	typeBytes, err := os.ReadFile(filepath.Join(zonePath, "type"))
	if err != nil {
		if logger != nil {
			logger.Debug("thermal read type failed", "path", zonePath, "error", err)
		}
		return nil
	}
	zoneType := strings.TrimSpace(string(typeBytes))

	tempBytes, err := os.ReadFile(filepath.Join(zonePath, "temp"))
	if err != nil {
		if logger != nil {
			logger.Debug("thermal read temp failed", "path", zonePath, "error", err)
		}
		return nil
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(tempBytes)), 10, 64)
	if err != nil {
		if logger != nil {
			logger.Warn("thermal malformed temp value", "path", zonePath, "value", string(tempBytes))
		}
		return nil
	}
	tempC := float64(val) / 1000.0

	var tripPoints []TripPoint
	for i := 0; i < 32; i++ {
		typePath := filepath.Join(zonePath, "trip_point_"+strconv.Itoa(i)+"_type")
		tempPath := filepath.Join(zonePath, "trip_point_"+strconv.Itoa(i)+"_temp")
		tpType, err := os.ReadFile(typePath)
		if err != nil {
			break
		}
		tpTemp, err := os.ReadFile(tempPath)
		if err != nil {
			break
		}
		tVal, err := strconv.ParseInt(strings.TrimSpace(string(tpTemp)), 10, 64)
		if err != nil {
			continue
		}
		tripPoints = append(tripPoints, TripPoint{
			Type:  strings.TrimSpace(string(tpType)),
			TempC: float64(tVal) / 1000.0,
		})
	}

	return &ThermalZone{
		Zone:       zoneName,
		Type:       zoneType,
		TempC:      tempC,
		TripPoints: tripPoints,
	}
}
