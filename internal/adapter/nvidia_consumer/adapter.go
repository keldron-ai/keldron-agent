//go:build linux || windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package nvidia_consumer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/health"
	"github.com/keldron-ai/keldron-agent/registry"
)

const (
	channelBuffer        = 256
	defaultNvidiaSMIPath = "nvidia-smi"
	defaultPollInterval  = 10 * time.Second
)

// NvidiaConsumerAdapter polls NVIDIA consumer GPU metrics via nvidia-smi CLI.
type NvidiaConsumerAdapter struct {
	cfg          config.AdapterConfig
	nvidiaCfg    NvidiaConsumerConfig
	collector    *NvidiaCollector
	readings     chan adapter.RawReading
	logger       *slog.Logger
	hostname     string
	closeOnce    sync.Once
	holder       *config.Holder
	pollInterval time.Duration
	ticker       *time.Ticker
	mu           sync.Mutex
	disabled     bool
	done         chan struct{}
	stopOnce     sync.Once
	unsubscribe  func()

	running     atomic.Bool
	pollCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastPoll    atomic.Value
	lastError   atomic.Value
	lastErrorAt atomic.Value
}

// New creates a NvidiaConsumerAdapter from the adapter config.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	var nvidiaCfg NvidiaConsumerConfig
	if cfg.Raw.Kind != 0 {
		if err := cfg.Raw.Decode(&nvidiaCfg); err != nil {
			return nil, fmt.Errorf("decoding nvidia_consumer config: %w", err)
		}
	}

	if nvidiaCfg.NvidiaSMIPath == "" {
		nvidiaCfg.NvidiaSMIPath = defaultNvidiaSMIPath
	}

	smiPath, err := resolveNvidiaSMIPath(nvidiaCfg.NvidiaSMIPath)
	if err != nil {
		logger.Warn("nvidia-smi not available, adapter disabled", "error", err)
		return &NvidiaConsumerAdapter{
			cfg:       cfg,
			nvidiaCfg: nvidiaCfg,
			readings:  make(chan adapter.RawReading, channelBuffer),
			logger:    logger,
			hostname:  adapter.Hostname(),
			holder:    holder,
			disabled:  true,
			done:      make(chan struct{}),
		}, nil
	}

	collector := NewNvidiaCollector(smiPath, nvidiaCfg.GPUIndices)

	return &NvidiaConsumerAdapter{
		cfg:       cfg,
		nvidiaCfg: nvidiaCfg,
		collector: collector,
		readings:  make(chan adapter.RawReading, channelBuffer),
		logger:    logger,
		hostname:  adapter.Hostname(),
		holder:    holder,
		done:      make(chan struct{}),
	}, nil
}

// Name returns the adapter identifier.
func (a *NvidiaConsumerAdapter) Name() string { return "nvidia_consumer" }

// Readings returns the channel of raw readings.
func (a *NvidiaConsumerAdapter) Readings() <-chan adapter.RawReading { return a.readings }

// IsRunning returns true if the adapter's Start loop is active.
func (a *NvidiaConsumerAdapter) IsRunning() bool { return a.running.Load() }

// Stats returns poll count, error count, last poll time, last error, and last error time.
func (a *NvidiaConsumerAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
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
	return
}

// Start begins the polling loop. Blocks until ctx is cancelled.
func (a *NvidiaConsumerAdapter) Start(ctx context.Context) error {
	if a.disabled {
		a.logger.Info("NVIDIA consumer adapter disabled, waiting for shutdown")
		select {
		case <-ctx.Done():
		case <-a.done:
		}
		a.closeOnce.Do(func() { close(a.readings) })
		return nil
	}

	a.running.Store(true)
	defer a.running.Store(false)

	interval := a.cfg.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	a.mu.Lock()
	a.pollInterval = interval
	a.ticker = time.NewTicker(interval)
	a.mu.Unlock()

	if a.holder != nil {
		unsub := a.holder.Subscribe(func(cfg *config.Config) {
			adapterCfg, ok := cfg.Adapters["nvidia_consumer"]
			if !ok {
				return
			}
			a.updatePollInterval(adapterCfg.PollInterval)
		})
		a.mu.Lock()
		a.unsubscribe = unsub
		a.mu.Unlock()
		defer func() {
			a.mu.Lock()
			unsubscribe := a.unsubscribe
			a.unsubscribe = nil
			a.mu.Unlock()
			if unsubscribe != nil {
				unsubscribe()
			}
		}()
	}

	a.logger.Info("NVIDIA consumer adapter polling started",
		"interval", interval,
		"nvidia_smi_path", a.nvidiaCfg.NvidiaSMIPath,
	)

	// Initial poll immediately
	a.poll(ctx)

	for {
		tickC := a.getTickChan()

		select {
		case <-ctx.Done():
			a.logger.Info("NVIDIA consumer adapter stopping")
			a.mu.Lock()
			if a.ticker != nil {
				a.ticker.Stop()
				a.ticker = nil
			}
			a.mu.Unlock()
			a.closeOnce.Do(func() { close(a.readings) })
			return nil
		case <-a.done:
			a.logger.Info("NVIDIA consumer adapter stopping")
			a.mu.Lock()
			if a.ticker != nil {
				a.ticker.Stop()
				a.ticker = nil
			}
			a.mu.Unlock()
			a.closeOnce.Do(func() { close(a.readings) })
			return nil
		case <-tickC:
			a.poll(ctx)
		}
	}
}

func (a *NvidiaConsumerAdapter) getTickChan() <-chan time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ticker == nil {
		return nil
	}
	return a.ticker.C
}

func (a *NvidiaConsumerAdapter) updatePollInterval(newInterval time.Duration) {
	if newInterval <= 0 {
		newInterval = defaultPollInterval
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if newInterval == a.pollInterval {
		return
	}
	a.logger.Info("poll interval updated", "old", a.pollInterval, "new", newInterval)
	a.pollInterval = newInterval
	if a.ticker != nil {
		a.ticker.Reset(newInterval)
	}
}

// Stop gracefully shuts down the adapter.
func (a *NvidiaConsumerAdapter) Stop(_ context.Context) error {
	a.logger.Info("NVIDIA consumer adapter shutting down")
	a.mu.Lock()
	unsub := a.unsubscribe
	a.unsubscribe = nil
	a.mu.Unlock()
	if unsub != nil {
		unsub()
	}
	a.stopOnce.Do(func() { close(a.done) })
	return nil
}

func (a *NvidiaConsumerAdapter) poll(ctx context.Context) {
	readings, err := a.collector.Collect(ctx)
	if err != nil {
		a.errorCount.Add(1)
		a.lastError.Store(err.Error())
		a.lastErrorAt.Store(time.Now())
		a.logger.Error("NVIDIA consumer collect failed", "error", err)
		return
	}

	a.pollCount.Add(1)
	a.lastPoll.Store(time.Now())

	for _, nr := range readings {
		raw := a.toRawReading(nr)
		select {
		case a.readings <- raw:
		default:
			a.logger.Warn("readings channel full, dropping reading", "gpu_index", nr.Index)
		}
	}
}

func (a *NvidiaConsumerAdapter) toRawReading(nr NvidiaReading) adapter.RawReading {
	deviceID := fmt.Sprintf("gpu-%d", nr.Index)
	modelKey := normalizeModelName(nr.Name)
	spec := registry.LookupWithFallback(modelKey, nr.TempLimitC, nr.PowerLimitW)

	metrics := map[string]interface{}{
		"gpu_id":              nr.Index,
		"device_model":        modelKey,
		"device_vendor":       spec.Vendor,
		"behavior_class":      spec.BehaviorClass,
		"serial":              nr.Serial,
		"pci_bus_id":          nr.PCIBusID,
		"temperature_c":       nr.TemperatureC,
		"power_usage_w":       nr.PowerDrawW,
		"gpu_utilization_pct": nr.GPUUtil,
		"mem_used_bytes":      nr.MemUsedMB * 1024 * 1024,
		"mem_total_bytes":     nr.MemTotalMB * 1024 * 1024,
		"sm_clock_mhz":        nr.ClockSMMHz,
		"sm_clock_max_mhz":    nr.ClockMaxMHz,
	}

	active, reason := MapThrottleReason(nr.ThrottleReason)
	throttleVal := 0.0
	if active {
		throttleVal = 1.0
	}
	metrics["throttled"] = throttleVal
	metrics["throttle_reason"] = reason

	// Thermal/power stress for risk engine
	metrics[registry.MetricThermalStress] = registry.NormalizeThermal(nr.TemperatureC, spec)
	metrics[registry.MetricPowerStress] = registry.NormalizePower(nr.PowerDrawW, spec)

	return adapter.RawReading{
		AdapterName: "nvidia_consumer",
		Source:      deviceID,
		Timestamp:   time.Now(),
		Metrics:     metrics,
	}
}

var _ health.AdapterProvider = (*NvidiaConsumerAdapter)(nil)
