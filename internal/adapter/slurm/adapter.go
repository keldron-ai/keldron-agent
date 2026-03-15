// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package slurm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
)

const (
	channelBuffer = 256
	adapterName   = "slurm"
)

// SlurmAdapter polls the Slurm REST API for job discovery and emits workload telemetry.
type SlurmAdapter struct {
	cfg          config.AdapterConfig
	slurmCfg     SlurmConfig
	client       *SlurmClient
	readings     chan adapter.RawReading
	logger       *slog.Logger
	holder       *config.Holder
	pollInterval time.Duration
	ticker       *time.Ticker
	mu           sync.Mutex

	cancelFunc  context.CancelFunc
	closeOnce   sync.Once
	running     atomic.Bool
	pollCount   atomic.Uint64
	errorCount  atomic.Uint64
	lastPoll    atomic.Value // time.Time
	lastError   atomic.Value // string
	lastErrorAt atomic.Value // time.Time
}

// New creates a SlurmAdapter from the adapter config.
func New(cfg config.AdapterConfig, holder *config.Holder, logger *slog.Logger) (adapter.Adapter, error) {
	var slurmCfg SlurmConfig
	if cfg.Raw.Kind != 0 {
		if err := cfg.Raw.Decode(&slurmCfg); err != nil {
			return nil, fmt.Errorf("decoding Slurm config: %w", err)
		}
	}

	if slurmCfg.SlurmrestdURL == "" {
		return nil, fmt.Errorf("slurmrestd_url is required")
	}
	if slurmCfg.APIVersion == "" {
		slurmCfg.APIVersion = defaultVersion
	}
	if slurmCfg.Timeout <= 0 {
		slurmCfg.Timeout = defaultTimeout
	}
	if slurmCfg.NodeToRackMap == nil {
		slurmCfg.NodeToRackMap = make(map[string]string)
	}

	interval := slurmCfg.PollInterval
	if interval <= 0 {
		interval = cfg.PollInterval
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}

	client := NewSlurmClient(
		slurmCfg.SlurmrestdURL,
		slurmCfg.APIVersion,
		slurmCfg.AuthToken,
		slurmCfg.Timeout,
	)

	return &SlurmAdapter{
		cfg:          cfg,
		slurmCfg:     slurmCfg,
		client:       client,
		readings:     make(chan adapter.RawReading, channelBuffer),
		logger:       logger,
		holder:       holder,
		pollInterval: interval,
	}, nil
}

// Name returns the adapter identifier.
func (s *SlurmAdapter) Name() string { return adapterName }

// Readings returns the channel of raw readings.
func (s *SlurmAdapter) Readings() <-chan adapter.RawReading { return s.readings }

// IsRunning returns true if the adapter's Start loop is active.
func (s *SlurmAdapter) IsRunning() bool {
	return s.running.Load()
}

// Stats returns poll count, error count, last poll time, last error, and last error time for health reporting.
func (s *SlurmAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	pollCount = s.pollCount.Load()
	errorCount = s.errorCount.Load()
	if v := s.lastPoll.Load(); v != nil {
		lastPoll = v.(time.Time)
	}
	if v := s.lastError.Load(); v != nil {
		lastError = v.(string)
	}
	if v := s.lastErrorAt.Load(); v != nil {
		lastErrorAt = v.(time.Time)
	}
	return pollCount, errorCount, lastPoll, lastError, lastErrorAt
}

// Start begins the polling loop. Blocks until ctx is cancelled or Stop is called.
// Returns an error if the adapter is already running.
func (s *SlurmAdapter) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	if s.running.Load() {
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("slurm adapter already running")
	}
	s.cancelFunc = cancel
	s.ticker = time.NewTicker(s.pollInterval)
	s.running.Store(true)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running.Store(false)
		s.mu.Unlock()
		cancel()
	}()

	if s.holder != nil {
		s.holder.Subscribe(func(cfg *config.Config) {
			acfg, ok := cfg.Adapters[adapterName]
			if !ok {
				return
			}
			interval := acfg.PollInterval
			if interval <= 0 {
				var sc SlurmConfig
				if err := acfg.Raw.Decode(&sc); err == nil && sc.PollInterval > 0 {
					interval = sc.PollInterval
				}
			}
			if interval > 0 {
				s.updatePollInterval(interval)
			}
		})
	}

	s.logger.Info("Slurm adapter polling started",
		"interval", s.pollInterval,
		"url", s.slurmCfg.SlurmrestdURL,
	)

	s.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Slurm adapter stopping")
			s.mu.Lock()
			if s.ticker != nil {
				s.ticker.Stop()
			}
			s.mu.Unlock()
			s.closeOnce.Do(func() {
				close(s.readings)
			})
			return nil
		case <-s.ticker.C:
			s.poll(ctx)
		}
	}
}

func (s *SlurmAdapter) updatePollInterval(newInterval time.Duration) {
	if newInterval <= 0 {
		newInterval = 30 * time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if newInterval == s.pollInterval {
		return
	}
	s.logger.Info("poll interval updated", "old", s.pollInterval, "new", newInterval)
	s.pollInterval = newInterval
	s.ticker.Reset(newInterval)
}

// Stop gracefully shuts down the adapter by cancelling the polling loop.
func (s *SlurmAdapter) Stop(_ context.Context) error {
	s.logger.Info("Slurm adapter shutting down")
	s.mu.Lock()
	cancel := s.cancelFunc
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *SlurmAdapter) poll(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, s.slurmCfg.Timeout)
	defer cancel()

	jobs, err := s.client.ListJobs(ctx)
	if err != nil {
		s.errorCount.Add(1)
		s.lastError.Store(err.Error())
		s.lastErrorAt.Store(time.Now())
		if _, isAuth := err.(*AuthError); isAuth {
			s.logger.Error("Slurm API auth failed", "error", err)
		} else {
			s.logger.Warn("Slurm API request failed", "error", err)
		}
		return
	}

	s.pollCount.Add(1)
	s.lastPoll.Store(time.Now())

	state := s.buildWorkloadState(jobs)
	s.emitReadings(state)
}

func (s *SlurmAdapter) buildWorkloadState(jobs []SlurmJob) SlurmWorkloadState {
	nodeGPUMap := make(map[string]int)
	rackGPUMap := make(map[string]int)
	totalGPUs := 0

	for _, job := range jobs {
		nodes := job.ExpandedNodes
		if len(nodes) == 0 {
			nodes = expandNodeList(job.NodeList)
		}
		gpusPerNode := job.GPUsPerNode
		if gpusPerNode == 0 && len(nodes) > 0 && job.TotalGPUs > 0 {
			gpusPerNode = job.TotalGPUs / len(nodes)
		}
		for _, node := range nodes {
			nodeGPUMap[node] += gpusPerNode
			totalGPUs += gpusPerNode
			rackID := s.slurmCfg.NodeToRackMap[node]
			if rackID != "" {
				rackGPUMap[rackID] += gpusPerNode
			}
		}
	}

	return SlurmWorkloadState{
		Timestamp:  time.Now(),
		ActiveJobs: jobs,
		TotalGPUs:  totalGPUs,
		NodeGPUMap: nodeGPUMap,
		RackGPUMap: rackGPUMap,
	}
}

func (s *SlurmAdapter) emitReadings(state SlurmWorkloadState) {
	// Emit one reading per node with GPU allocation
	for node, gpus := range state.NodeGPUMap {
		reading := adapter.RawReading{
			AdapterName: adapterName,
			Source:      node,
			Timestamp:   state.Timestamp,
			Metrics: map[string]interface{}{
				"gpus_allocated": gpus,
			},
		}
		select {
		case s.readings <- reading:
		default:
			s.logger.Warn("readings channel full, dropping reading", "node", node)
		}
	}

	// If no nodes have allocations, emit nothing (empty state)
	// This allows the platform to distinguish "no jobs" from "API error"
}
