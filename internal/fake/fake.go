// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package fake implements a simulated GPU telemetry adapter for testing
// the full agent pipeline without real NVIDIA hardware.
//
// Drop this into agent/internal/fake/ and register it in main.go.
// Then set adapters.fake.enabled: true in your dev.yaml.
package fake

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
)

const channelBuffer = 256

// FakeConfig holds fake adapter-specific settings decoded from the adapter's Raw YAML node.
type FakeConfig struct {
	NumRacks    int     `yaml:"num_racks"`
	GPUsPerRack int     `yaml:"gpus_per_rack"`
	AmbientC    float64 `yaml:"ambient_temp_c"`
	Scenario    string  `yaml:"scenario"` // "bursty", "steady", "failure", "ramp"
	MemoryGB    int     `yaml:"memory_gb"`
	PowerLimitW float64 `yaml:"power_limit_w"`
}

// --- GPU simulation types ---

type workloadState int

const (
	wsIdle workloadState = iota
	wsRamping
	wsSustained
	wsCheckpointing
	wsCooldown
)

type fakeGPU struct {
	id       string
	nodeID   string
	temp     float64
	util     float64
	workload workloadState
	rng      *rand.Rand
}

// --- Adapter ---

type FakeAdapter struct {
	cfg          config.AdapterConfig
	fakeCfg      FakeConfig
	gpus         []*fakeGPU
	readings     chan adapter.RawReading
	logger       *slog.Logger
	holder       *config.Holder
	pollInterval time.Duration
	ticker       *time.Ticker
	mu           sync.Mutex
	closeOnce    sync.Once

	running    atomic.Bool
	pollCount  atomic.Uint64
	errorCount atomic.Uint64
	lastPoll   atomic.Value
}

// New is the Constructor registered in the adapter registry.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	var fakeCfg FakeConfig
	if cfg.Raw.Kind != 0 {
		if err := cfg.Raw.Decode(&fakeCfg); err != nil {
			return nil, fmt.Errorf("decoding fake config: %w", err)
		}
	}

	// Defaults
	if fakeCfg.NumRacks <= 0 {
		fakeCfg.NumRacks = 4
	}
	if fakeCfg.GPUsPerRack <= 0 {
		fakeCfg.GPUsPerRack = 8
	}
	if fakeCfg.AmbientC <= 0 {
		fakeCfg.AmbientC = 22.0
	}
	if fakeCfg.Scenario == "" {
		fakeCfg.Scenario = "bursty"
	}
	if fakeCfg.MemoryGB <= 0 {
		fakeCfg.MemoryGB = 80
	}
	if fakeCfg.PowerLimitW <= 0 {
		fakeCfg.PowerLimitW = 700.0
	}

	// Build simulated GPUs
	src := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(src)
	total := fakeCfg.NumRacks * fakeCfg.GPUsPerRack
	gpus := make([]*fakeGPU, total)

	for i := 0; i < total; i++ {
		rackIdx := i / fakeCfg.GPUsPerRack
		gpuIdx := i % fakeCfg.GPUsPerRack
		gpus[i] = &fakeGPU{
			id:       fmt.Sprintf("GPU-%04d-%04d", rackIdx, gpuIdx),
			nodeID:   fmt.Sprintf("gpu-node-%02d", rackIdx),
			temp:     fakeCfg.AmbientC + 20 + rng.Float64()*5,
			util:     0,
			workload: wsIdle,
			rng:      rand.New(rand.NewSource(rng.Int63())),
		}
	}

	logger.Info("fake adapter initialized",
		"num_racks", fakeCfg.NumRacks,
		"gpus_per_rack", fakeCfg.GPUsPerRack,
		"total_gpus", total,
		"scenario", fakeCfg.Scenario,
	)

	return &FakeAdapter{
		cfg:      cfg,
		fakeCfg:  fakeCfg,
		gpus:     gpus,
		readings: make(chan adapter.RawReading, channelBuffer),
		logger:   logger,
		holder:   holder,
	}, nil
}

func (f *FakeAdapter) Name() string                        { return "fake" }
func (f *FakeAdapter) Readings() <-chan adapter.RawReading { return f.readings }

// IsRunning returns true if the adapter's Start loop is active (for health.AdapterProvider).
func (f *FakeAdapter) IsRunning() bool {
	return f.running.Load()
}

func (f *FakeAdapter) Start(ctx context.Context) error {
	f.running.Store(true)
	defer f.running.Store(false)

	interval := f.cfg.PollInterval
	if interval <= 0 {
		interval = time.Second
	}

	f.mu.Lock()
	f.pollInterval = interval
	f.ticker = time.NewTicker(interval)
	f.mu.Unlock()

	// Support hot-reload of poll interval
	if f.holder != nil {
		f.holder.Subscribe(func(cfg *config.Config) {
			acfg, ok := cfg.Adapters["fake"]
			if !ok {
				return
			}
			f.updatePollInterval(acfg.PollInterval)
		})
	}

	f.logger.Info("fake adapter polling started",
		"interval", interval,
		"scenario", f.fakeCfg.Scenario,
	)

	f.poll()

	for {
		select {
		case <-ctx.Done():
			f.logger.Info("fake adapter stopping")
			f.mu.Lock()
			if f.ticker != nil {
				f.ticker.Stop()
			}
			f.mu.Unlock()
			f.closeOnce.Do(func() { close(f.readings) })
			return nil
		case <-f.ticker.C:
			f.poll()
		}
	}
}

func (f *FakeAdapter) updatePollInterval(newInterval time.Duration) {
	if newInterval <= 0 {
		newInterval = time.Second
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if newInterval == f.pollInterval {
		return
	}
	f.logger.Info("poll interval updated", "old", f.pollInterval, "new", newInterval)
	f.pollInterval = newInterval
	f.ticker.Reset(newInterval)
}

func (f *FakeAdapter) Stop(_ context.Context) error {
	f.logger.Info("fake adapter shutting down")
	return nil
}

// Stats returns adapter health data (same pattern as DCGM adapter, for health.AdapterProvider).
func (f *FakeAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	pollCount = f.pollCount.Load()
	errorCount = f.errorCount.Load()
	if v := f.lastPoll.Load(); v != nil {
		lastPoll = v.(time.Time)
	}
	return pollCount, errorCount, lastPoll, "", time.Time{}
}

// --- Polling & Physics ---

func (f *FakeAdapter) poll() {
	f.pollCount.Add(1)
	f.lastPoll.Store(time.Now())
	now := time.Now()

	for _, gpu := range f.gpus {
		f.stepGPU(gpu)
		reading := f.toReading(gpu, now)

		select {
		case f.readings <- reading:
		default:
			f.logger.Warn("readings channel full, dropping", "gpu_id", gpu.id)
		}
	}
}

// toReading converts simulated GPU state to an adapter.RawReading
// using metric keys that match DCGM (agent/internal/dcgm/metrics.go) for pipeline compatibility.
func (f *FakeAdapter) toReading(gpu *fakeGPU, now time.Time) adapter.RawReading {
	rng := gpu.rng

	// Power correlates with utilization
	powerFrac := 0.15 + 0.85*(gpu.util/100)
	power := f.fakeCfg.PowerLimitW * powerFrac
	power += (rng.Float64() - 0.5) * 10

	// Memory usage correlates with workload state
	memTotalBytes := float64(f.fakeCfg.MemoryGB) * 1024 * 1024 * 1024
	var memFrac float64
	switch gpu.workload {
	case wsIdle:
		memFrac = 0.02 + rng.Float64()*0.03
	case wsRamping:
		memFrac = 0.3 + rng.Float64()*0.3
	case wsSustained:
		memFrac = 0.75 + rng.Float64()*0.2
	case wsCheckpointing:
		memFrac = 0.6 + rng.Float64()*0.15
	case wsCooldown:
		memFrac = 0.1 + rng.Float64()*0.2
	}

	// Inlet/outlet
	inlet := f.fakeCfg.AmbientC + rng.Float64()*2
	outlet := inlet + (gpu.temp-inlet)*0.3 + rng.Float64()*1.5

	// Throttle
	throttleReason := "none"
	throttled := false
	if gpu.temp > 90 {
		throttleReason = "thermal"
		throttled = true
	} else if power > f.fakeCfg.PowerLimitW*0.98 {
		throttleReason = "power"
		throttled = true
	}

	// NVLink
	nvlinkBW := 0.0
	nvlinkErrors := 0
	if gpu.util > 50 {
		nvlinkBW = 400 + rng.Float64()*100
		if rng.Float64() < 0.001 {
			nvlinkErrors = rng.Intn(5) + 1
		}
	}

	// Build metrics map matching DCGM keys (agent/internal/dcgm/metrics.go).
	// Extras: inlet_temp_c, outlet_temp_c, delta_t_c, nvlink_* for richer simulation.
	metrics := map[string]interface{}{
		"temperature_c":         round1(gpu.temp),
		"gpu_utilization_pct":   round1(gpu.util),
		"mem_used_bytes":        memFrac * memTotalBytes,
		"mem_total_bytes":       memTotalBytes,
		"power_usage_w":         round1(power),
		"power_limit_w":         f.fakeCfg.PowerLimitW,
		"inlet_temp_c":          round1(inlet),
		"outlet_temp_c":         round1(outlet),
		"delta_t_c":             round1(outlet - inlet),
		"throttle_reason":       throttleReason,
		"throttled":             throttled,
		"nvlink_bandwidth_gbps": round1(nvlinkBW),
		"nvlink_error_count":    nvlinkErrors,
	}

	return adapter.RawReading{
		AdapterName: "fake",
		Source:      gpu.nodeID,
		Timestamp:   now,
		Metrics:     metrics,
	}
}

// stepGPU advances the thermal and workload simulation by one tick.
func (f *FakeAdapter) stepGPU(gpu *fakeGPU) {
	dt := f.pollInterval.Seconds()
	rng := gpu.rng

	// --- Workload state machine ---
	switch gpu.workload {
	case wsIdle:
		startChance := 0.02
		if f.fakeCfg.Scenario == "bursty" {
			startChance = 0.05
		} else if f.fakeCfg.Scenario == "ramp" {
			startChance = 0.08
		}
		if rng.Float64() < startChance {
			gpu.workload = wsRamping
		}

	case wsRamping:
		gpu.util += (80 + rng.Float64()*15) * dt / 12.0
		if gpu.util >= 90+rng.Float64()*10 {
			gpu.util = 90 + rng.Float64()*10
			gpu.workload = wsSustained
		}

	case wsSustained:
		gpu.util += (rng.Float64() - 0.5) * 3
		gpu.util = clamp(gpu.util, 75, 100)
		if rng.Float64() < 0.01 {
			gpu.workload = wsCheckpointing
		}
		if rng.Float64() < 0.003 {
			gpu.workload = wsCooldown
		}

	case wsCheckpointing:
		gpu.util -= 40 * dt / 5.0
		if gpu.util <= 20+rng.Float64()*15 {
			gpu.util = 20 + rng.Float64()*15
			if rng.Float64() < 0.15 {
				gpu.workload = wsRamping
			}
		}

	case wsCooldown:
		gpu.util -= 50 * dt / 8.0
		if gpu.util <= 2 {
			gpu.util = rng.Float64() * 2
			gpu.workload = wsIdle
		}
	}

	gpu.util = clamp(gpu.util, 0, 100)

	// --- Thermal model (Newton's law of cooling, asymmetric time constants) ---
	targetTemp := f.fakeCfg.AmbientC + 20 + (gpu.util/100)*40

	var tau float64
	if targetTemp > gpu.temp {
		tau = 20.0 // heating: fast
	} else {
		tau = 45.0 // cooling: slow
	}

	gpu.temp += (1.0 / tau) * (targetTemp - gpu.temp) * dt
	gpu.temp += (rng.Float64() - 0.5) * 0.6 // sensor noise

	// Failure injection: one GPU develops a cooling problem
	if f.fakeCfg.Scenario == "failure" && gpu.id == "GPU-0002-0003" {
		gpu.temp += 0.1 * dt
	}

	gpu.temp = clamp(gpu.temp, f.fakeCfg.AmbientC, 105)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
