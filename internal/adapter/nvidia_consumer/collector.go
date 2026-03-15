//go:build linux || windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package nvidia_consumer

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// resolveNvidiaSMIPath returns the absolute path to nvidia-smi.
// If path is empty, searches PATH for "nvidia-smi".
func resolveNvidiaSMIPath(path string) (string, error) {
	if path == "" {
		path = "nvidia-smi"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("nvidia-smi not found: %w", err)
	}
	return resolved, nil
}

const (
	collectTimeout = 5 * time.Second
)

var collectorLogger = slog.Default()

// NvidiaCollector collects GPU metrics by executing nvidia-smi.
type NvidiaCollector struct {
	smiPath    string
	gpuIndices []int
}

// NewNvidiaCollector creates a collector that uses the nvidia-smi CLI.
func NewNvidiaCollector(smiPath string, gpuIndices []int) *NvidiaCollector {
	indicesCopy := append([]int(nil), gpuIndices...)
	return &NvidiaCollector{
		smiPath:    smiPath,
		gpuIndices: indicesCopy,
	}
}

// Collect executes nvidia-smi and parses the CSV output.
func (c *NvidiaCollector) Collect(ctx context.Context) ([]NvidiaReading, error) {
	return CollectNvidiaSmi(ctx, c.smiPath, c.gpuIndices)
}

// NvidiaReading holds parsed metrics for a single GPU from nvidia-smi.
type NvidiaReading struct {
	Index          int
	Name           string
	TemperatureC   float64
	TempLimitC     float64
	GPUUtil        float64
	MemUtil        float64
	MemUsedMB      float64
	MemTotalMB     float64
	PowerDrawW     float64
	PowerLimitW    float64
	ClockSMMHz     float64
	ClockMaxMHz    float64
	FanSpeedPct    float64
	Serial         string
	PCIBusID       string
	ThrottleReason uint64
}

// nvidiaSmiQueryColumns defines the CSV column order from nvidia-smi --query-gpu.
const nvidiaSmiQuery = "index,name,temperature.gpu,temperature.gpu.tlimit," +
	"utilization.gpu,utilization.memory,memory.used,memory.total," +
	"power.draw,power.limit,clocks.current.sm,clocks.max.sm," +
	"fan.speed,gpu_serial,pci.bus_id,clocks_throttle_reasons.active"

// CollectNvidiaSmi executes nvidia-smi with --query-gpu and parses the CSV output.
// Returns one NvidiaReading per GPU. Empty gpuIndices means all GPUs.
// [N/A] values are set to 0 or -1 as appropriate.
func CollectNvidiaSmi(ctx context.Context, smiPath string, gpuIndices []int) ([]NvidiaReading, error) {
	ctx, cancel := context.WithTimeout(ctx, collectTimeout)
	defer cancel()

	args := []string{
		"--query-gpu=" + nvidiaSmiQuery,
		"--format=csv,noheader,nounits",
	}

	cmd := exec.CommandContext(ctx, smiPath, args...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("nvidia-smi timeout after %v: %w", collectTimeout, err)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("nvidia-smi exited with code %d: %w", exitErr.ExitCode(), err)
		}
		return nil, fmt.Errorf("nvidia-smi exec failed: %w", err)
	}

	readings, err := parseNvidiaSmiCSV(out)
	if err != nil {
		return nil, fmt.Errorf("parse nvidia-smi CSV: %w", err)
	}

	if len(gpuIndices) > 0 {
		idxSet := make(map[int]bool)
		for _, i := range gpuIndices {
			idxSet[i] = true
		}
		filtered := make([]NvidiaReading, 0, len(readings))
		for _, r := range readings {
			if idxSet[r.Index] {
				filtered = append(filtered, r)
			}
		}
		readings = filtered
	}

	return readings, nil
}

// parseNvidiaSmiCSV parses the CSV output from nvidia-smi.
// Expects columns: index,name,temperature.gpu,temperature.gpu.tlimit,
// utilization.gpu,utilization.memory,memory.used,memory.total,
// power.draw,power.limit,clocks.current.sm,clocks.max.sm,
// fan.speed,gpu_serial,pci.bus_id,clocks_throttle_reasons.active
func parseNvidiaSmiCSV(data []byte) ([]NvidiaReading, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV parse: %w", err)
	}

	if len(records) == 0 {
		return []NvidiaReading{}, nil
	}

	const expectedCols = 16
	readings := make([]NvidiaReading, 0, len(records))

	for i, rec := range records {
		if len(rec) < expectedCols {
			return nil, fmt.Errorf("line %d: expected at least %d columns, got %d", i+1, expectedCols, len(rec))
		}

		r := NvidiaReading{}

		indexRaw := strings.TrimSpace(rec[0])
		index, err := strconv.Atoi(indexRaw)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid gpu index %q: %w", i+1, rec[0], err)
		}
		r.Index = index
		r.Name = strings.TrimSpace(rec[1])
		r.TemperatureC = parseNAFloat(rec[2], 0, "temperature.gpu")
		r.TempLimitC = parseNAFloat(rec[3], 0, "temperature.gpu.tlimit")
		r.GPUUtil = parseNAFloat(rec[4], 0, "utilization.gpu")
		r.MemUtil = parseNAFloat(rec[5], 0, "utilization.memory")
		r.MemUsedMB = parseNAFloat(rec[6], 0, "memory.used")
		r.MemTotalMB = parseNAFloat(rec[7], 0, "memory.total")
		r.PowerDrawW = parseNAFloat(rec[8], 0, "power.draw")
		r.PowerLimitW = parseNAFloat(rec[9], 0, "power.limit")
		r.ClockSMMHz = parseNAFloat(rec[10], 0, "clocks.current.sm")
		r.ClockMaxMHz = parseNAFloat(rec[11], 0, "clocks.max.sm")
		r.FanSpeedPct = parseNAFloat(rec[12], 0, "fan.speed")
		r.Serial = strings.TrimSpace(rec[13])
		r.PCIBusID = strings.TrimSpace(rec[14])
		r.ThrottleReason = parseNAThrottle(rec[15], "clocks_throttle_reasons.active")

		readings = append(readings, r)
	}

	return readings, nil
}

func parseNAFloat(s string, defaultVal float64, field string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "[N/A]") {
		return defaultVal
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		collectorLogger.Debug("parseNAFloat input parse failed",
			"field", field,
			"input", s,
			"error", err,
		)
		return defaultVal
	}
	return f
}

func parseNAThrottle(s string, field string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "[N/A]") {
		return 0
	}
	// Base 0 handles 0x prefix
	u, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		collectorLogger.Debug("parseNAThrottle input parse failed",
			"field", field,
			"input", s,
			"error", err,
		)
		return 0
	}
	return u
}
