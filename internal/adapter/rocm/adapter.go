// Package rocm implements the ROCm SMI telemetry adapter for AMD GPU metrics.
package rocm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/registry"
)

const (
	channelBuffer        = 256
	defaultROCmSMIPath   = "/opt/rocm/bin/rocm-smi"
	defaultPollInterval  = 10 * time.Second
)

// ROCmAdapter polls AMD GPU metrics via rocm-smi CLI.
type ROCmAdapter struct {
	cfg          config.AdapterConfig
	rocmCfg      ROCmConfig
	collector    *ROCmCollector
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
	lastPoll    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// New creates a ROCmAdapter from the adapter config.
// This is the Constructor registered in the adapter registry.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	var rocmCfg ROCmConfig
	if cfg.Raw.Kind != 0 {
		if err := cfg.Raw.Decode(&rocmCfg); err != nil {
			return nil, fmt.Errorf("decoding ROCm config: %w", err)
		}
	}

	// Defaults
	if rocmCfg.ROCmSMIPath == "" {
		rocmCfg.ROCmSMIPath = defaultROCmSMIPath
	}
	if rocmCfg.CollectionMethod == "" {
		rocmCfg.CollectionMethod = "cli"
	}
	if rocmCfg.CollectionMethod != "cli" {
		return nil, fmt.Errorf("collection_method %q not supported (only \"cli\" is implemented)", rocmCfg.CollectionMethod)
	}

	// Verify rocm-smi is available at startup; if missing, mark adapter as
	// disabled so StartAll can proceed without halting agent startup.
	var disabled bool
	if err := CheckROCmSMIAvailable(rocmCfg.ROCmSMIPath); err != nil {
		logger.Warn("rocm-smi not available, adapter disabled", "error", err)
		disabled = true
	}

	collector := NewROCmCollector(rocmCfg.ROCmSMIPath, rocmCfg.GPUIndices, logger)

	return &ROCmAdapter{
		cfg:       cfg,
		rocmCfg:   rocmCfg,
		collector: collector,
		readings:  make(chan adapter.RawReading, channelBuffer),
		logger:    logger,
		hostname:  adapter.Hostname(),
		holder:    holder,
		disabled:  disabled,
		done:      make(chan struct{}),
	}, nil
}

// Name returns the adapter identifier.
func (r *ROCmAdapter) Name() string { return "rocm" }

// Readings returns the channel of raw readings.
func (r *ROCmAdapter) Readings() <-chan adapter.RawReading { return r.readings }

// IsRunning returns true if the adapter's Start loop is active.
func (r *ROCmAdapter) IsRunning() bool {
	return r.running.Load()
}

// Stats returns poll count, error count, last poll time, last error, and last error time for health reporting.
func (r *ROCmAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	pollCount = r.pollCount.Load()
	errorCount = r.errorCount.Load()
	if v := r.lastPoll.Load(); v != nil {
		lastPoll = v.(time.Time)
	}
	if v := r.lastError.Load(); v != nil {
		lastError = v.(string)
	}
	if v := r.lastErrorAt.Load(); v != nil {
		lastErrorAt = v.(time.Time)
	}
	return pollCount, errorCount, lastPoll, lastError, lastErrorAt
}

// Start begins the polling loop. Blocks until ctx is cancelled.
// If the adapter is disabled (rocm-smi unavailable), it waits for ctx
// cancellation without polling.
func (r *ROCmAdapter) Start(ctx context.Context) error {
	if r.disabled {
		r.logger.Info("ROCm adapter disabled, waiting for shutdown")
		select {
		case <-ctx.Done():
		case <-r.done:
		}
		r.closeOnce.Do(func() { close(r.readings) })
		return nil
	}

	r.running.Store(true)
	defer r.running.Store(false)

	interval := r.cfg.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	r.mu.Lock()
	r.pollInterval = interval
	r.ticker = time.NewTicker(interval)
	r.mu.Unlock()

	if r.holder != nil {
		unsub := r.holder.Subscribe(func(cfg *config.Config) {
			adapterCfg, ok := cfg.Adapters["rocm"]
			if !ok {
				return
			}
			r.updatePollInterval(adapterCfg.PollInterval)
		})
		r.mu.Lock()
		r.unsubscribe = unsub
		r.mu.Unlock()
	}

	r.logger.Info("ROCm adapter polling started",
		"interval", interval,
		"rocm_smi_path", r.rocmCfg.ROCmSMIPath,
		"gpu_indices", r.rocmCfg.GPUIndices,
	)

	// Initial poll immediately
	r.poll(ctx)

	for {
		r.mu.Lock()
		tickC := r.ticker.C
		r.mu.Unlock()

		select {
		case <-ctx.Done():
			r.logger.Info("ROCm adapter stopping")
			r.mu.Lock()
			r.ticker.Stop()
			r.mu.Unlock()
			r.closeOnce.Do(func() { close(r.readings) })
			return nil
		case <-r.done:
			r.logger.Info("ROCm adapter stopping")
			r.mu.Lock()
			r.ticker.Stop()
			r.mu.Unlock()
			r.closeOnce.Do(func() { close(r.readings) })
			return nil
		case <-tickC:
			r.poll(ctx)
		}
	}
}

func (r *ROCmAdapter) updatePollInterval(newInterval time.Duration) {
	if newInterval <= 0 {
		newInterval = defaultPollInterval
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if newInterval == r.pollInterval {
		return
	}
	r.logger.Info("poll interval updated", "old", r.pollInterval, "new", newInterval)
	r.pollInterval = newInterval
	if r.ticker != nil {
		r.ticker.Reset(newInterval)
	} else {
		r.ticker = time.NewTicker(newInterval)
	}
}

// Stop gracefully shuts down the adapter by signalling the Start loop to exit.
func (r *ROCmAdapter) Stop(_ context.Context) error {
	r.logger.Info("ROCm adapter shutting down")
	r.mu.Lock()
	unsub := r.unsubscribe
	r.unsubscribe = nil
	r.mu.Unlock()
	if unsub != nil {
		unsub()
	}
	r.stopOnce.Do(func() { close(r.done) })
	return nil
}

func (r *ROCmAdapter) poll(ctx context.Context) {
	readings, err := r.collector.Collect(ctx)
	if err != nil {
		r.errorCount.Add(1)
		r.lastError.Store(err.Error())
		r.lastErrorAt.Store(time.Now())
		r.logger.Error("ROCm collect failed", "error", err)
		return
	}

	r.pollCount.Add(1)
	r.lastPoll.Store(time.Now())

	for i := range readings {
		raw := readings[i].ToRawReading(r.hostname)

		// Add normalized thermal/power stress; apply edge-to-junction correction for AMD edge sensors (e.g. MI250X).
		spec := registry.Lookup(registry.NormalizeModelName(readings[i].GPUModel))
		tempForNorm := registry.ApplyEdgeToJunctionCorrection(readings[i].GPUTemp, spec)
		raw.Metrics[registry.MetricThermalStress] = registry.NormalizeThermal(tempForNorm, spec)
		raw.Metrics[registry.MetricPowerStress] = registry.NormalizePower(readings[i].GPUPowerW, spec)
		// Canonical keys for platform compatibility with DCGM schema.
		raw.Metrics["temperature_c"] = tempForNorm
		raw.Metrics["power_usage_w"] = readings[i].GPUPowerW

		select {
		case r.readings <- raw:
		default:
			r.logger.Warn("readings channel full, dropping reading",
				"gpu_id", readings[i].GPUID,
			)
		}
	}
}
