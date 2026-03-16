//go:build linux

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package linux_thermal implements a telemetry adapter for generic Linux temperature
// monitoring via /sys/class/hwmon and /sys/class/thermal. Works on Raspberry Pi,
// Intel NUCs, ARM SBCs, and any Linux server without NVIDIA/AMD GPUs.
package linux_thermal

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/health"
)

const channelBuffer = 256

// LinuxThermalAdapter collects temperatures from hwmon and thermal zones.
type LinuxThermalAdapter struct {
	cfg          LinuxThermalAdapterConfig
	readings     chan adapter.RawReading
	logger       *slog.Logger
	holder       *config.Holder
	pollInterval time.Duration
	intervalMu   sync.RWMutex
	lifecycleMu  sync.Mutex
	closeOnce    sync.Once
	cancel       context.CancelFunc
	unsubscribe  func()

	running     atomic.Bool
	pollCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastPoll    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// New creates a LinuxThermalAdapter. Returns an error if not running on Linux.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("linux_thermal adapter only supports linux (got %s)", runtime.GOOS)
	}

	var ltCfg LinuxThermalAdapterConfig
	if cfg.Raw.Kind != 0 {
		if err := cfg.Raw.Decode(&ltCfg); err != nil {
			return nil, fmt.Errorf("decoding linux_thermal config: %w", err)
		}
	}
	ltCfg.applyDefaults()

	interval := cfg.PollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &LinuxThermalAdapter{
		cfg:          ltCfg,
		readings:     make(chan adapter.RawReading, channelBuffer),
		logger:       logger,
		holder:       holder,
		pollInterval: interval,
	}, nil
}

// Name returns the adapter identifier.
func (a *LinuxThermalAdapter) Name() string { return "linux_thermal" }

// Readings returns the channel of raw readings.
func (a *LinuxThermalAdapter) Readings() <-chan adapter.RawReading { return a.readings }

// IsRunning returns true if the adapter's Start loop is active.
func (a *LinuxThermalAdapter) IsRunning() bool {
	return a.running.Load()
}

// Stats returns poll count, error count, last poll time, last error, and last error time.
func (a *LinuxThermalAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
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
func (a *LinuxThermalAdapter) Start(ctx context.Context) error {
	if !a.running.CompareAndSwap(false, true) {
		return fmt.Errorf("linux_thermal adapter already started")
	}
	defer a.running.Store(false)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.intervalMu.RLock()
	interval := a.pollInterval
	a.intervalMu.RUnlock()

	resetIntervalCh := make(chan time.Duration, 1)
	var unsubscribe func()
	defer func() {
		if unsubscribe != nil {
			unsubscribe()
		}
		a.lifecycleMu.Lock()
		a.cancel = nil
		a.unsubscribe = nil
		a.lifecycleMu.Unlock()
	}()
	a.lifecycleMu.Lock()
	a.cancel = cancel
	a.unsubscribe = nil
	a.lifecycleMu.Unlock()
	if a.holder != nil {
		unsubscribe = a.holder.Subscribe(func(cfg *config.Config) {
			acfg, ok := cfg.Adapters["linux_thermal"]
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
		a.lifecycleMu.Lock()
		a.unsubscribe = unsubscribe
		a.lifecycleMu.Unlock()
	}

	a.logger.Info("linux_thermal adapter started", "interval", interval, "hwmon_path", a.cfg.HwmonPath, "thermal_path", a.cfg.ThermalPath)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	a.poll()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("linux_thermal adapter stopping")
			a.closeOnce.Do(func() { close(a.readings) })
			return nil
		case newInterval := <-resetIntervalCh:
			ticker.Stop()
			ticker = time.NewTicker(newInterval)
			a.logger.Info("linux_thermal poll interval updated", "interval", newInterval)
		case <-ticker.C:
			a.poll()
		}
	}
}

// Stop gracefully shuts down the adapter.
func (a *LinuxThermalAdapter) Stop(_ context.Context) error {
	a.logger.Info("linux_thermal adapter shutting down")
	a.lifecycleMu.Lock()
	unsubscribe := a.unsubscribe
	cancel := a.cancel
	a.unsubscribe = nil
	a.cancel = nil
	a.lifecycleMu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
	if cancel != nil {
		cancel()
	}
	return nil
}

func (a *LinuxThermalAdapter) poll() {
	a.pollCount.Add(1)
	now := time.Now()
	a.lastPoll.Store(now)

	readings, err := a.collect(now)
	if err != nil {
		a.errorCount.Add(1)
		a.lastError.Store(err.Error())
		a.lastErrorAt.Store(time.Now())
		a.logger.Warn("linux_thermal poll failed", "error", err)
		return
	}

	for _, r := range readings {
		select {
		case a.readings <- r:
		default:
			a.logger.Warn("readings channel full, dropping")
		}
	}
}

func (a *LinuxThermalAdapter) collect(now time.Time) ([]adapter.RawReading, error) {
	var readings []adapter.RawReading

	sensors := DiscoverHwmon(a.cfg.HwmonPath, a.logger)
	for _, s := range sensors {
		if a.isExcluded(s.Name) {
			continue
		}
		if len(a.cfg.IncludeZones) > 0 && !a.isIncluded(s.Name) {
			continue
		}
		deviceID := s.Name
		if s.Label != "" {
			deviceID = s.Name + ":" + s.Label
		}
		metricKey := a.metricKeyForSensorType(s.SensorType)
		m := map[string]interface{}{
			metricKey:        s.TempC,
			"sensor_name":    s.Name,
			"sensor_type":    s.SensorType,
			"adapter":        "linux_thermal",
			"behavior_class": "sbc_constrained",
		}
		if s.TempMaxC >= 0 {
			m["temp_max_c"] = s.TempMaxC
		}
		if s.TempCritC >= 0 {
			m["temp_crit_c"] = s.TempCritC
		}
		readings = append(readings, adapter.RawReading{
			AdapterName: "linux_thermal",
			Source:      deviceID,
			Timestamp:   now,
			Metrics:     m,
		})
	}

	zones := DiscoverThermalZones(a.cfg.ThermalPath, a.logger)
	for _, z := range zones {
		if a.isExcluded(z.Type) || a.isExcluded(z.Zone) {
			continue
		}
		if len(a.cfg.IncludeZones) > 0 && !a.isZoneIncluded(z) {
			continue
		}
		m := map[string]interface{}{
			"cpu_temp_c": z.TempC,
			"zone_type":  z.Type,
			"adapter":    "linux_thermal",
		}
		readings = append(readings, adapter.RawReading{
			AdapterName: "linux_thermal",
			Source:      z.Zone,
			Timestamp:   now,
			Metrics:     m,
		})
	}

	return readings, nil
}

func (a *LinuxThermalAdapter) isExcluded(name string) bool {
	for _, ex := range a.cfg.ExcludeZones {
		if strings.EqualFold(name, ex) || strings.Contains(strings.ToLower(name), strings.ToLower(ex)) {
			return true
		}
	}
	return false
}

func (a *LinuxThermalAdapter) isIncluded(name string) bool {
	for _, inc := range a.cfg.IncludeZones {
		if strings.EqualFold(name, inc) || strings.Contains(strings.ToLower(name), strings.ToLower(inc)) {
			return true
		}
	}
	return false
}

func (a *LinuxThermalAdapter) isZoneIncluded(z ThermalZone) bool {
	if a.isIncluded(z.Type) || a.isIncluded(z.Zone) {
		return true
	}
	return false
}

func (a *LinuxThermalAdapter) metricKeyForSensorType(sensorType string) string {
	switch sensorType {
	case "gpu":
		return "temperature_c"
	default:
		return "cpu_temp_c"
	}
}

// Ensure LinuxThermalAdapter implements health.AdapterProvider.
var _ health.AdapterProvider = (*LinuxThermalAdapter)(nil)
