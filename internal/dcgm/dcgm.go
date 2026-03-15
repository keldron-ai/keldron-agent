// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package dcgm implements the DCGM telemetry adapter for NVIDIA GPU metrics.
package dcgm

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
	defaultRetryInterval = 5 * time.Second
	maxRetryBackoff      = 5 * time.Minute
)

// dcgmClient is the interface for collecting GPU metrics.
// Implemented by StubClient and realClient.
type dcgmClient interface {
	Collect() ([]GPUMetrics, error)
	Close() error
}

// DCGMConfig holds DCGM-specific configuration decoded from the adapter's Raw YAML node.
type DCGMConfig struct {
	UseStub bool  `yaml:"use_stub"`
	GPUIDs  []int `yaml:"gpu_ids"`
	Collect struct {
		Temperature bool `yaml:"temperature"`
		Power       bool `yaml:"power"`
		Utilization bool `yaml:"utilization"`
		Memory      bool `yaml:"memory"`
		Clocks      bool `yaml:"clocks"`
		Throttle    bool `yaml:"throttle"`
	} `yaml:"collect"`
	Connection struct {
		RetryInterval time.Duration `yaml:"retry_interval"`
		MaxRetries    int           `yaml:"max_retries"`
	} `yaml:"connection"`
}

// DCGMAdapter polls GPU metrics via the dcgmClient interface.
type DCGMAdapter struct {
	cfg          config.AdapterConfig
	dcgmCfg      DCGMConfig
	client       dcgmClient
	readings     chan adapter.RawReading
	logger       *slog.Logger
	hostname     string
	closeOnce    sync.Once
	holder       *config.Holder
	pollInterval time.Duration
	ticker       *time.Ticker
	mu           sync.Mutex

	running     atomic.Bool
	pollCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastPoll    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// New creates a DCGMAdapter from the adapter config.
// This is the Constructor registered in the adapter registry.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	var dcgmCfg DCGMConfig
	if cfg.Raw.Kind != 0 {
		if err := cfg.Raw.Decode(&dcgmCfg); err != nil {
			return nil, fmt.Errorf("decoding DCGM config: %w", err)
		}
	}

	// Default GPU IDs if not specified.
	if len(dcgmCfg.GPUIDs) == 0 {
		dcgmCfg.GPUIDs = []int{0}
	}

	// Default retry interval.
	if dcgmCfg.Connection.RetryInterval <= 0 {
		dcgmCfg.Connection.RetryInterval = defaultRetryInterval
	}

	var client dcgmClient
	if dcgmCfg.UseStub {
		client = NewStubClient(dcgmCfg.GPUIDs)
		logger.Info("using stub DCGM client", "gpu_ids", dcgmCfg.GPUIDs)
	} else {
		var err error
		client, err = NewRealClient(cfg.Endpoint, dcgmCfg.GPUIDs)
		if err != nil {
			return nil, fmt.Errorf("creating DCGM client: %w", err)
		}
	}

	return &DCGMAdapter{
		cfg:      cfg,
		dcgmCfg:  dcgmCfg,
		client:   client,
		readings: make(chan adapter.RawReading, channelBuffer),
		logger:   logger,
		hostname: adapter.Hostname(),
		holder:   holder,
	}, nil
}

// Name returns the adapter identifier.
func (d *DCGMAdapter) Name() string { return "dcgm" }

// Readings returns the channel of raw readings.
func (d *DCGMAdapter) Readings() <-chan adapter.RawReading { return d.readings }

// IsRunning returns true if the adapter's Start loop is active.
func (d *DCGMAdapter) IsRunning() bool {
	return d.running.Load()
}

// Stats returns poll count, error count, last poll time, last error, and last error time for health reporting.
func (d *DCGMAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	pollCount = d.pollCount.Load()
	errorCount = d.errorCount.Load()
	if v := d.lastPoll.Load(); v != nil {
		lastPoll = v.(time.Time)
	}
	if v := d.lastError.Load(); v != nil {
		lastError = v.(string)
	}
	if v := d.lastErrorAt.Load(); v != nil {
		lastErrorAt = v.(time.Time)
	}
	return pollCount, errorCount, lastPoll, lastError, lastErrorAt
}

// Start begins the polling loop. Blocks until ctx is cancelled.
func (d *DCGMAdapter) Start(ctx context.Context) error {
	d.running.Store(true)
	defer d.running.Store(false)

	interval := d.cfg.PollInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	d.mu.Lock()
	d.pollInterval = interval
	d.ticker = time.NewTicker(interval)
	d.mu.Unlock()

	if d.holder != nil {
		d.holder.Subscribe(func(cfg *config.Config) {
			adapterCfg, ok := cfg.Adapters["dcgm"]
			if !ok {
				return
			}
			d.updatePollInterval(adapterCfg.PollInterval)
		})
	}

	d.logger.Info("DCGM adapter polling started",
		"interval", interval,
		"gpu_ids", d.dcgmCfg.GPUIDs,
		"use_stub", d.dcgmCfg.UseStub,
	)

	// Initial poll immediately.
	d.poll()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("DCGM adapter stopping")
			d.mu.Lock()
			if d.ticker != nil {
				d.ticker.Stop()
			}
			d.mu.Unlock()
			// Close readings channel here — only the producer (Start) should close it,
			// ensuring no concurrent writes after close.
			d.closeOnce.Do(func() {
				close(d.readings)
			})
			return nil
		case <-d.ticker.C:
			d.poll()
		}
	}
}

// updatePollInterval safely changes the polling ticker.
func (d *DCGMAdapter) updatePollInterval(newInterval time.Duration) {
	if newInterval <= 0 {
		newInterval = 10 * time.Second
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if newInterval == d.pollInterval {
		return
	}
	d.logger.Info("poll interval updated", "old", d.pollInterval, "new", newInterval)
	d.pollInterval = newInterval
	d.ticker.Reset(newInterval)
}

// Stop gracefully shuts down the adapter, closing the client.
// The readings channel is closed by Start when the context is cancelled.
func (d *DCGMAdapter) Stop(_ context.Context) error {
	d.logger.Info("DCGM adapter shutting down")

	var clientErr error
	if d.client != nil {
		clientErr = d.client.Close()
	}

	return clientErr
}

// poll collects metrics from all GPUs and sends them to the readings channel.
func (d *DCGMAdapter) poll() {
	metrics, err := d.client.Collect()
	if err != nil {
		d.errorCount.Add(1)
		d.lastError.Store(err.Error())
		d.lastErrorAt.Store(time.Now())
		d.logger.Error("DCGM collect failed", "error", err)
		return
	}

	d.pollCount.Add(1)
	d.lastPoll.Store(time.Now())

	for i := range metrics {
		reading := metrics[i].ToRawReading(d.hostname)

		// Add normalized thermal/power stress from GPU spec registry.
		spec := registry.Lookup(registry.NormalizeModelName(metrics[i].GPUName))
		reading.Metrics[registry.MetricThermalStress] = registry.NormalizeThermal(metrics[i].Temperature, spec)
		reading.Metrics[registry.MetricPowerStress] = registry.NormalizePower(metrics[i].PowerUsage, spec)

		// Non-blocking send: drop and warn if channel is full (slow consumer).
		select {
		case d.readings <- reading:
		default:
			d.logger.Warn("readings channel full, dropping reading",
				"gpu_id", metrics[i].GPUID,
			)
		}
	}
}
