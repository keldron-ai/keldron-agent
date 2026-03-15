// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package rocm

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

const (
	collectTimeout = 5 * time.Second
)

// ROCmCollector collects GPU metrics by executing rocm-smi.
type ROCmCollector struct {
	rocmSmiPath string
	gpuIndices  []int
	logger      *slog.Logger
}

// NewROCmCollector creates a collector that uses the CLI strategy.
func NewROCmCollector(rocmSmiPath string, gpuIndices []int, logger *slog.Logger) *ROCmCollector {
	return &ROCmCollector{
		rocmSmiPath: rocmSmiPath,
		gpuIndices:  gpuIndices,
		logger:      logger,
	}
}

// Collect executes rocm-smi and parses the JSON output into GPUReadings.
// On timeout (>5s), returns nil and error; caller should retry next interval.
// On partial output, returns partial readings and logs a warning.
func (c *ROCmCollector) Collect(ctx context.Context) ([]GPUReading, error) {
	ctx, cancel := context.WithTimeout(ctx, collectTimeout)
	defer cancel()

	args := []string{
		"--showtemp",
		"--showuse",
		"--showmeminfo", "vram",
		"--showpower",
		"--showclocks",
		"--showthrottlestatus",
		"--json",
	}

	cmd := exec.CommandContext(ctx, c.rocmSmiPath, args...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("rocm-smi timeout after %v: %w", collectTimeout, err)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("rocm-smi exited with code %d: %w", exitErr.ExitCode(), err)
		}
		return nil, fmt.Errorf("rocm-smi exec failed: %w", err)
	}

	readings, err := ParseROCmOutput(out, c.logger)
	if err != nil {
		return nil, fmt.Errorf("parse rocm-smi output: %w", err)
	}

	// Filter by gpu_indices if specified.
	if len(c.gpuIndices) > 0 {
		idxSet := make(map[int]bool)
		for _, i := range c.gpuIndices {
			idxSet[i] = true
		}
		filtered := make([]GPUReading, 0, len(readings))
		for _, r := range readings {
			if idxSet[r.GPUID] {
				filtered = append(filtered, r)
			}
		}
		readings = filtered
	}

	// Log warning for any GPU with partial output (no temp, mem, or power).
	for _, r := range readings {
		hasKeyMetrics := r.GPUTemp > 0 || r.GPUMemoryTotal > 0 || r.GPUPowerW > 0
		if !hasKeyMetrics {
			c.logger.Warn("partial rocm-smi output: key metrics missing", "gpu_id", r.GPUID)
		}
	}

	return readings, nil
}

// CheckROCmSMIAvailable verifies that rocm-smi exists and is executable.
// Call at adapter startup; if it returns an error, the adapter should not start.
func CheckROCmSMIAvailable(path string) error {
	if path == "" {
		return fmt.Errorf("rocm_smi_path is empty")
	}
	// LookPath finds the executable (handles both PATH and absolute paths)
	if _, err := exec.LookPath(path); err != nil {
		return fmt.Errorf("rocm-smi not found at %q: %w", path, err)
	}
	// Quick sanity check: run with --help to verify it executes
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--help")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rocm-smi not executable at %q: %w", path, err)
	}
	return nil
}
