// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package apple_silicon implements a telemetry adapter for Apple Silicon Macs.
// It collects GPU/system metrics via system_profiler, vm_stat, and optionally
// powermetrics (requires root for temperature/power).
package apple_silicon

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/health"
)

const channelBuffer = 256

var (
	chipRe = regexp.MustCompile(`(?m)^\s*Chip:\s*(.+)$`)
)

// AppleSiliconAdapter collects telemetry from Apple Silicon Macs.
type AppleSiliconAdapter struct {
	cfg          config.AdapterConfig
	readings     chan adapter.RawReading
	logger       *slog.Logger
	holder       *config.Holder
	pollInterval time.Duration
	intervalMu   sync.RWMutex
	closeOnce    sync.Once

	running     atomic.Bool
	pollCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastPoll    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// New creates an AppleSiliconAdapter. Returns an error if not running on darwin.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("apple_silicon adapter only supports darwin (got %s)", runtime.GOOS)
	}

	interval := cfg.PollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &AppleSiliconAdapter{
		cfg:          cfg,
		readings:     make(chan adapter.RawReading, channelBuffer),
		logger:       logger,
		holder:       holder,
		pollInterval: interval,
	}, nil
}

// Name returns the adapter identifier.
func (a *AppleSiliconAdapter) Name() string { return "apple_silicon" }

// Readings returns the channel of raw readings.
func (a *AppleSiliconAdapter) Readings() <-chan adapter.RawReading { return a.readings }

// IsRunning returns true if the adapter's Start loop is active.
func (a *AppleSiliconAdapter) IsRunning() bool {
	return a.running.Load()
}

// Stats returns poll count, error count, last poll time, last error, and last error time.
func (a *AppleSiliconAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	pollCount = a.pollCount.Load()
	errorCount = a.errorCount.Load()
	if v := a.lastPoll.Load(); v != nil {
		lastPoll = v.(time.Time)
	}
	if v := a.lastError.Load(); v != nil {
		lastError = v.(string)
	}
	if v := a.lastErrorAt.Load(); v != nil {
		lastErrorAt = v.(time.Time)
	}
	return pollCount, errorCount, lastPoll, lastError, lastErrorAt
}

// Start begins polling. Blocks until ctx is cancelled.
func (a *AppleSiliconAdapter) Start(ctx context.Context) error {
	if !a.running.CompareAndSwap(false, true) {
		return fmt.Errorf("apple_silicon adapter already started")
	}
	defer a.running.Store(false)

	a.intervalMu.RLock()
	interval := a.pollInterval
	a.intervalMu.RUnlock()

	if a.holder != nil {
		a.holder.Subscribe(func(cfg *config.Config) {
			acfg, ok := cfg.Adapters["apple_silicon"]
			if !ok {
				return
			}
			if acfg.PollInterval > 0 {
				a.intervalMu.Lock()
				a.pollInterval = acfg.PollInterval
				a.intervalMu.Unlock()
			}
		})
	}

	a.logger.Info("apple_silicon adapter started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial poll
	a.poll()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("apple_silicon adapter stopping")
			a.closeOnce.Do(func() { close(a.readings) })
			return nil
		case <-ticker.C:
			a.poll()
		}
	}
}

// Stop gracefully shuts down the adapter.
func (a *AppleSiliconAdapter) Stop(_ context.Context) error {
	a.logger.Info("apple_silicon adapter shutting down")
	return nil
}

func (a *AppleSiliconAdapter) poll() {
	a.pollCount.Add(1)
	a.lastPoll.Store(time.Now())
	now := time.Now()

	reading, err := a.collect(now)
	if err != nil {
		a.errorCount.Add(1)
		a.lastError.Store(err.Error())
		a.lastErrorAt.Store(time.Now())
		a.logger.Warn("apple_silicon poll failed", "error", err)
		return
	}

	select {
	case a.readings <- reading:
	default:
		a.logger.Warn("readings channel full, dropping")
	}
}

func (a *AppleSiliconAdapter) collect(now time.Time) (adapter.RawReading, error) {
	source := adapter.Hostname()
	if source == "" {
		source = "unknown"
	}

	metrics := make(map[string]interface{})

	// Device model from system_profiler (string becomes tag in normalizer)
	chip, err := a.runSystemProfiler()
	if err != nil {
		a.logger.Debug("system_profiler failed", "error", err)
		metrics["gpu_model"] = "Apple Silicon"
	} else {
		metrics["gpu_model"] = strings.TrimSpace(chip)
	}

	// Memory from vm_stat and sysctl
	memTotal, memUsed, err := a.collectMemory()
	if err != nil {
		a.logger.Debug("memory collection failed", "error", err)
	} else {
		metrics["mem_total_bytes"] = float64(memTotal)
		metrics["mem_used_bytes"] = float64(memUsed)
	}

	// Temperature and power from powermetrics (requires root)
	tempC, powerW := a.collectPowermetrics()
	if tempC >= 0 {
		metrics["temperature_c"] = tempC
	}
	if powerW >= 0 {
		metrics["power_usage_w"] = powerW
	}

	// GPU utilization: 0 (not available without root)
	metrics["gpu_utilization_pct"] = 0.0

	// Ensure we have at least one metric for normalizer validation
	if len(metrics) == 0 {
		metrics["gpu_utilization_pct"] = 0.0
		metrics["mem_total_bytes"] = 0.0
		metrics["mem_used_bytes"] = 0.0
	}

	// gpu_id for device_id in Prometheus (hostname:0)
	metrics["gpu_id"] = 0.0

	return adapter.RawReading{
		AdapterName: "apple_silicon",
		Source:      source,
		Timestamp:   now,
		Metrics:     metrics,
	}, nil
}

func (a *AppleSiliconAdapter) runSystemProfiler() (string, error) {
	out, err := exec.Command("system_profiler", "SPHardwareDataType").Output()
	if err != nil {
		return "", err
	}
	m := chipRe.FindSubmatch(out)
	if len(m) < 2 {
		return "", fmt.Errorf("chip not found in system_profiler output")
	}
	return string(m[1]), nil
}

func (a *AppleSiliconAdapter) collectMemory() (total, used uint64, err error) {
	// Total memory from sysctl
	totalOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0, err
	}
	total, err = parseUint64(strings.TrimSpace(string(totalOut)))
	if err != nil {
		return 0, 0, err
	}

	// Used memory from vm_stat (active + wired + compressed)
	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return total, 0, err
	}

	pageSize := uint64(16384) // Apple Silicon page size
	active, _ := parseVMStatPage(string(vmOut), "Pages active")
	inactive, _ := parseVMStatPage(string(vmOut), "Pages inactive")
	wired, _ := parseVMStatPage(string(vmOut), "Pages wired down")
	speculative, _ := parseVMStatPage(string(vmOut), "Pages speculative")
	compressed, _ := parseVMStatPage(string(vmOut), "Pages occupied by compressor")

	used = (active + inactive + wired + speculative + compressed) * pageSize
	if used > total {
		used = total
	}

	return total, used, nil
}

func parseVMStatPage(s, key string) (uint64, error) {
	idx := strings.Index(s, key)
	if idx < 0 {
		return 0, fmt.Errorf("key %q not found", key)
	}
	line := s[idx:]
	colon := strings.Index(line, ":")
	if colon < 0 {
		return 0, fmt.Errorf("malformed line")
	}
	val := strings.TrimSpace(line[colon+1:])
	val = strings.TrimSuffix(val, ".")
	return parseUint64(val)
}

func parseUint64(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}

// collectPowermetrics tries to get temperature and power from powermetrics.
// Requires root. Returns (-1, -1) if unavailable.
func (a *AppleSiliconAdapter) collectPowermetrics() (tempC, powerW float64) {
	tempC = -1
	powerW = -1

	out, err := exec.Command("powermetrics", "-i", "1000", "-n", "1").Output()
	if err != nil {
		return tempC, powerW
	}

	// Parse temperature (e.g. "CPU die temperature: 45 C" or "GPU die temperature: 50 C")
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "temperature") && strings.Contains(line, "C") {
			if v := parseTemperatureLine(line); v >= 0 {
				// Prefer GPU temp if available, else CPU
				if strings.Contains(strings.ToLower(line), "gpu") {
					tempC = v
					break
				}
				if tempC < 0 {
					tempC = v
				}
			}
		}
		if strings.Contains(line, "GPU Power") || strings.Contains(line, "GPU power") {
			if v := parsePowerLine(line); v >= 0 {
				powerW = v
			}
		}
	}

	return tempC, powerW
}

func parseTemperatureLine(line string) float64 {
	// "CPU die temperature: 45 C" or "GPU die temperature: 50 C"
	parts := strings.Fields(line)
	for i, p := range parts {
		if p == "C" && i > 0 {
			if v, err := strconv.ParseFloat(parts[i-1], 64); err == nil {
				return v
			}
		}
	}
	return -1
}

func parsePowerLine(line string) float64 {
	// "GPU Power: 0.12 W" or similar
	parts := strings.Fields(line)
	for i, p := range parts {
		if (p == "W" || p == "mW") && i > 0 {
			if v, err := strconv.ParseFloat(parts[i-1], 64); err == nil {
				if p == "mW" {
					return v / 1000
				}
				return v
			}
		}
	}
	return -1
}

// Ensure AppleSiliconAdapter implements health.AdapterProvider.
var _ health.AdapterProvider = (*AppleSiliconAdapter)(nil)
