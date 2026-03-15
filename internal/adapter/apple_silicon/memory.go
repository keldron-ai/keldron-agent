//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package apple_silicon

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// MemoryInfo holds physical and swap memory statistics.
type MemoryInfo struct {
	PhysicalTotalBytes int64
	PhysicalUsedBytes  int64
	SwapUsedBytes      int64
	SwapTotalBytes     int64
}

const pageSize = 16384 // Apple Silicon page size

// ReadMemoryInfo returns physical and swap memory stats (unprivileged).
func ReadMemoryInfo() (*MemoryInfo, error) {
	info := &MemoryInfo{}

	// Physical total from sysctl
	totalOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return nil, fmt.Errorf("hw.memsize: %w", err)
	}
	total, err := strconv.ParseUint(strings.TrimSpace(string(totalOut)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse hw.memsize: %w", err)
	}
	info.PhysicalTotalBytes = int64(total)

	// Used memory from vm_stat
	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return info, nil // return partial with total only
	}
	active, _ := parseVMStatPage(string(vmOut), "Pages active")
	inactive, _ := parseVMStatPage(string(vmOut), "Pages inactive")
	wired, _ := parseVMStatPage(string(vmOut), "Pages wired down")
	speculative, _ := parseVMStatPage(string(vmOut), "Pages speculative")
	compressed, _ := parseVMStatPage(string(vmOut), "Pages occupied by compressor")
	used := (active + inactive + wired + speculative + compressed) * pageSize
	if used > total {
		used = total
	}
	info.PhysicalUsedBytes = int64(used)

	// Swap from sysctl vm.swapusage
	// Format: "total = 1024.00M  used = 512.00M  free = 512.00M"
	swapOut, err := exec.Command("sysctl", "-n", "vm.swapusage").Output()
	if err != nil {
		return info, nil
	}
	swapUsed, swapTotal := parseSwapUsage(string(swapOut))
	info.SwapUsedBytes = swapUsed
	info.SwapTotalBytes = swapTotal

	return info, nil
}

func parseVMStatPage(s, key string) (uint64, error) {
	idx := strings.Index(s, key)
	if idx < 0 {
		return 0, fmt.Errorf("key %q not found", key)
	}
	rest := s[idx:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return 0, fmt.Errorf("malformed line")
	}
	val := strings.TrimSpace(rest[colon+1:])
	val = strings.TrimSuffix(val, ".")
	return strconv.ParseUint(val, 10, 64)
}

func parseSwapUsage(s string) (used, total int64) {
	// "total = 1024.00M  used = 512.00M  free = 512.00M"
	parseMB := func(key string) int64 {
		idx := strings.Index(s, key+" = ")
		if idx < 0 {
			return 0
		}
		rest := s[idx+len(key)+3:]
		if space := strings.IndexByte(rest, ' '); space >= 0 {
			rest = rest[:space]
		}
		rest = strings.TrimSuffix(rest, "M")
		f, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			return 0
		}
		return int64(f * 1024 * 1024)
	}
	return parseMB("used"), parseMB("total")
}
