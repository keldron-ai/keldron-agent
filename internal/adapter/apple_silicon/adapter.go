//go:build darwin && arm64

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package apple_silicon implements a telemetry adapter for Apple Silicon Macs.
// It collects GPU/system metrics via IOKit/IOReport, thermal pressure via notifyd,
// and memory/swap via sysctl. Runs entirely unprivileged — no sudo required.
package apple_silicon

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/health"
	"github.com/keldron-ai/keldron-agent/registry"
)

const channelBuffer = 256

// AppleSiliconAdapter collects telemetry from Apple Silicon Macs.
type AppleSiliconAdapter struct {
	cfg          config.AdapterConfig
	readings     chan adapter.RawReading
	logger       *slog.Logger
	holder       *config.Holder
	pollInterval time.Duration
	intervalMu   sync.RWMutex
	closeOnce    sync.Once

	// Cached at startup
	chipName string
	spec     registry.GPUSpec

	running     atomic.Bool
	pollCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastPoll    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// New creates an AppleSiliconAdapter. Returns an error if not running on darwin/arm64.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return nil, fmt.Errorf("apple_silicon adapter only supports darwin/arm64 (got %s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	interval := cfg.PollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	if logger == nil {
		logger = slog.Default()
	}

	chipName, err := DetectChip()
	if err != nil {
		chipName = "Apple Silicon"
		logger.Warn("chip detection failed, using fallback", "error", err)
	}
	spec := registry.Lookup(chipName)
	if spec.BehaviorClass != "soc_integrated" {
		// Fallback for unknown Apple Silicon: use soc_integrated defaults
		spec = registry.LookupWithFallback("M1", 105, 25)
	}

	logger.Info("Detected Apple Silicon — using soc_integrated behavior class, no sudo required",
		"chip", chipName,
		"behavior_class", spec.BehaviorClass,
	)

	return &AppleSiliconAdapter{
		cfg:          cfg,
		readings:     make(chan adapter.RawReading, channelBuffer),
		logger:       logger,
		holder:       holder,
		pollInterval: interval,
		chipName:     chipName,
		spec:         spec,
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

	resetIntervalCh := make(chan time.Duration, 1)

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

				select {
				case resetIntervalCh <- acfg.PollInterval:
				default:
				}
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
		case newInterval := <-resetIntervalCh:
			ticker.Stop()
			ticker = time.NewTicker(newInterval)
			a.logger.Info("apple_silicon poll interval updated", "interval", newInterval)
		case <-ticker.C:
			a.poll()
		}
	}
}

// Stop gracefully shuts down the adapter.
func (a *AppleSiliconAdapter) Stop(_ context.Context) error {
	a.logger.Info("apple_silicon adapter shutting down")
	CleanupIOKit()
	return nil
}

func (a *AppleSiliconAdapter) poll() {
	a.pollCount.Add(1)
	now := time.Now()
	a.lastPoll.Store(now)

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

	// Device model (string becomes tag in normalizer)
	metrics["gpu_model"] = a.chipName
	// Device metadata (strings -> Tags) for Prometheus labels
	metrics["device_model"] = a.chipName
	metrics["behavior_class"] = a.spec.BehaviorClass
	metrics["device_vendor"] = a.spec.Vendor
	metrics["gpu_id"] = 0.0

	// IOKit metrics (GPU util, power, SoC temp)
	iokit := ReadIOKit(a.logger)
	metrics["temperature_c"] = iokit.SoCTempC
	metrics["power_usage_w"] = iokit.GPUPowerW
	metrics["gpu_utilization_pct"] = iokit.GPUUtilization * 100

	// Thermal pressure state
	thermalState, _ := ReadThermalPressure()
	if thermalState != "" {
		metrics["thermal_pressure_state"] = thermalState
	}

	// Throttle derived from thermal pressure
	throttleActive := 0.0
	throttleReason := "none"
	if IsThrottled(thermalState) {
		throttleActive = 1.0
		throttleReason = "thermal"
	}
	metrics["throttled"] = throttleActive
	metrics["throttle_reason"] = throttleReason

	// Memory and swap
	mem, err := ReadMemoryInfo()
	if err != nil {
		a.logger.Debug("memory collection failed", "error", err)
	} else {
		metrics["mem_total_bytes"] = float64(mem.PhysicalTotalBytes)
		metrics["mem_used_bytes"] = float64(mem.PhysicalUsedBytes)
		metrics["swap_total_bytes"] = float64(mem.SwapTotalBytes)
		metrics["swap_used_bytes"] = float64(mem.SwapUsedBytes)
	}

	// Ensure schema stability for missing metrics (memory keys may be absent on error)
	if _, ok := metrics["mem_total_bytes"]; !ok {
		metrics["mem_total_bytes"] = 0.0
	}
	if _, ok := metrics["mem_used_bytes"]; !ok {
		metrics["mem_used_bytes"] = 0.0
	}
	if _, ok := metrics["swap_total_bytes"]; !ok {
		metrics["swap_total_bytes"] = 0.0
	}
	if _, ok := metrics["swap_used_bytes"]; !ok {
		metrics["swap_used_bytes"] = 0.0
	}

	return adapter.RawReading{
		AdapterName: "apple_silicon",
		Source:      source,
		Timestamp:   now,
		Metrics:     metrics,
	}, nil
}

// Ensure AppleSiliconAdapter implements health.AdapterProvider.
var _ health.AdapterProvider = (*AppleSiliconAdapter)(nil)
