// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/adapter/kubernetes"
	"github.com/keldron-ai/keldron-agent/internal/adapter/rocm"
	"github.com/keldron-ai/keldron-agent/internal/adapter/slurm"
	"github.com/keldron-ai/keldron-agent/internal/adapter/snmp_pdu"
	"github.com/keldron-ai/keldron-agent/internal/adapter/temperature"
	"github.com/keldron-ai/keldron-agent/internal/buffer"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/dcgm"
	"github.com/keldron-ai/keldron-agent/internal/fake"
	"github.com/keldron-ai/keldron-agent/internal/health"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/output"
	"github.com/keldron-ai/keldron-agent/internal/sender"
)

// Set at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", "./keldron-agent.yaml", "path to YAML config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	localMode := flag.Bool("local", false, "run in local-only mode (no cloud streaming)")
	showHelp := flag.Bool("help", false, "show usage")
	flag.Parse()

	if *showHelp {
		flag.Usage()
		return 0
	}
	if *showVersion {
		fmt.Printf("keldron-agent v%s\n", version)
		return 0
	}

	// Load configuration.
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	// Initialize structured logger.
	slog.SetDefault(initLogger(cfg.Agent.LogLevel))

	// Config holder for hot-reload support (S-006).
	cfgHolder := config.NewHolder(cfg)

	// Subscribe to log_level changes for hot-reload.
	cfgHolder.Subscribe(func(cfg *config.Config) {
		slog.SetDefault(initLogger(cfg.Agent.LogLevel))
	})

	// Log effective config summary (mask cloud API key when set).
	slog.Info("agent starting",
		"agent_id", cfg.Agent.ID,
		"version", version,
		"config", *configPath,
		"log_level", cfg.Agent.LogLevel,
		"poll_interval", cfg.Agent.PollInterval,
		"output_stdout", cfg.Output.Stdout,
		"output_prometheus", cfg.Output.Prometheus,
		"output_prometheus_port", cfg.Output.PrometheusPort,
	)
	if cfg.Cloud.APIKey != "" {
		slog.Info("cloud configured", "api_key", config.MaskedCloudAPIKey(cfg.Cloud.APIKey), "endpoint", cfg.Cloud.Endpoint)
	}

	// Build adapter registry.
	registry := adapter.NewRegistry()
	registry.Register("dcgm", dcgm.New)
	registry.Register("rocm", rocm.New)
	registry.Register("fake", fake.New)
	registry.Register("kubernetes", kubernetes.New)
	registry.Register("slurm", slurm.New)
	registry.Register("temperature", temperature.New)
	registry.Register("snmp_pdu", snmp_pdu.New)

	// Set up signal handler.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Start config watcher for hot-reload.
	logger := slog.Default()
	watcher := config.NewWatcher(*configPath, cfgHolder, logger.With("component", "config"))
	go func() {
		if err := watcher.Start(ctx); err != nil {
			logger.Error("config watcher stopped", "error", err)
		}
	}()

	// Start enabled adapters.
	running, err := registry.StartAll(ctx, cfgHolder, logger)
	if err != nil {
		slog.Error("failed to start adapters", "error", err)
		return 1
	}
	slog.Info("adapters started", "count", len(running))

	// Create and start normalizer pipeline.
	norm := normalizer.New(cfg.Agent.ID, cfg.RackMapping, cfgHolder, logger.With("component", "normalizer"))
	for _, a := range running {
		norm.AddInput(a.Readings())
	}

	go func() {
		if err := norm.Start(ctx); err != nil {
			slog.Error("normalizer stopped with error", "error", err)
		}
	}()

	// Local mode: --local flag OR no cloud and (prometheus or stdout) enabled
	isLocalMode := *localMode || (cfg.Cloud.APIKey == "" && (cfg.Output.Prometheus || cfg.Output.Stdout))

	var bufMgr *buffer.Manager
	var sndr interface {
		Start(context.Context) error
		SetOnConnChange(func(bool))
		Stats() (uint64, uint64, uint64)
		IsConnected() bool
		LastSendAt() time.Time
		SeqNumber() uint64
		LastError() string
		Target() string
	}
	var outputs []output.Output
	var outputBridgeDone chan struct{}
	var senderDone chan error
	var bufferDone chan error

	if isLocalMode {
		if cfg.Output.Prometheus {
			slog.Info("running in local mode — metrics available at http://localhost:" + fmt.Sprintf("%d", cfg.Output.PrometheusPort) + "/metrics")
		} else {
			slog.Info("running in local mode — Prometheus metrics disabled, stdout output only")
		}

		// Build outputs
		if cfg.Output.Prometheus {
			prom := output.NewPrometheus(cfg.Output.PrometheusPort, version, cfg.Agent.DeviceName, logger.With("component", "prometheus"))
			activeAdapters := make([]string, 0, len(running))
			for _, a := range running {
				activeAdapters = append(activeAdapters, a.Name())
			}
			prom.SetActiveAdapters(activeAdapters)
			outputs = append(outputs, prom)
			go func() {
				if err := prom.Start(ctx); err != nil && err != http.ErrServerClosed {
					logger.Error("Prometheus server stopped", "error", err)
				}
			}()
		}
		if cfg.Output.Stdout {
			activeAdapters := make([]string, 0, len(running))
			for _, a := range running {
				activeAdapters = append(activeAdapters, a.Name())
			}
			std := output.NewStdout(os.Stdout, version, activeAdapters)
			outputs = append(outputs, std)
		}

		// Output bridge: read from normalizer, batch by poll interval, update outputs
		outputBridgeDone = make(chan struct{})
		go runOutputBridge(ctx, norm.Output(), outputs, cfg.Agent.PollInterval, outputBridgeDone, logger)
	} else {
		// Cloud mode: buffer + sender
		var err error
		bufMgr, err = buffer.NewManager(cfg.Buffer, norm.Output(), logger.With("component", "buffer"))
		if err != nil {
			slog.Error("failed to create buffer manager", "error", err)
			return 1
		}
		bufferDone = make(chan error, 1)
		go func() {
			bufferDone <- bufMgr.Start(ctx)
		}()

		sndr = sender.NewGRPC(cfg.Sender, cfg.Agent.ID, bufMgr.Output(), logger.With("component", "sender"))
		sndr.SetOnConnChange(bufMgr.OnConnChange)
		senderDone = make(chan error, 1)
		go func() {
			senderDone <- sndr.Start(ctx)
		}()
	}

	// Create and start health server.
	healthSrv := health.New(cfg.Health.Bind, cfg.Agent.ID, version, logger.With("component", "health"))
	for _, a := range running {
		if ap, ok := a.(health.AdapterProvider); ok {
			healthSrv.RegisterAdapter(ap)
		}
	}
	healthSrv.RegisterNormalizer(norm)
	healthSrv.SetLocalMode(isLocalMode)
	if bufMgr != nil {
		healthSrv.RegisterBuffer(bufMgr)
	}
	if sndr != nil {
		healthSrv.RegisterSender(sndr)
	}
	healthSrv.RegisterConfig(watcher)
	enabledAdapters := make(map[string]bool)
	for name, acfg := range cfg.Adapters {
		enabledAdapters[name] = acfg.Enabled
	}
	healthSrv.SetEnabledAdapters(enabledAdapters)
	if cfg.Health.Enabled {
		go func() {
			if err := healthSrv.Start(ctx); err != nil && err != http.ErrServerClosed {
				logger.Error("health server stopped", "error", err)
			}
		}()
	}

	slog.Info("agent ready, waiting for signal")

	// Block until signal received.
	<-ctx.Done()
	stop()

	slog.Info("shutdown signal received, draining",
		"timeout", cfg.Agent.ShutdownTimeout,
	)

	// Create a deadline context for graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Agent.ShutdownTimeout)
	defer cancel()

	// Stop adapters in reverse order (closes their reading channels).
	for i := len(running) - 1; i >= 0; i-- {
		a := running[i]
		slog.Info("stopping adapter", "adapter", a.Name())
		if err := a.Stop(shutdownCtx); err != nil {
			slog.Error("adapter stop error", "adapter", a.Name(), "error", err)
		}
	}

	// Normalizer will drain remaining readings and close its output channel
	// once all adapter channels are closed. Log final stats.
	processed, rejected := norm.Stats()
	slog.Info("normalizer stats", "processed", processed, "rejected", rejected)

	if isLocalMode {
		// Wait for output bridge to finish
		<-outputBridgeDone
		for _, out := range outputs {
			if err := out.Close(); err != nil {
				logger.Error("output close error", "error", err)
			}
		}
	} else {
		// Wait for sender to flush remaining batches and close its stream.
		if err := <-senderDone; err != nil {
			slog.Error("sender stopped with error", "error", err)
		}
		batchesSent, pointsSent, senderErrors := sndr.Stats()
		slog.Info("sender stats",
			"batches_sent", batchesSent,
			"points_sent", pointsSent,
			"errors", senderErrors,
		)

		// Wait for buffer manager to close WAL.
		if err := <-bufferDone; err != nil {
			slog.Error("buffer manager stopped with error", "error", err)
		}
		ringPushes, walSpills, walDrained, dropped := bufMgr.Stats()
		slog.Info("buffer stats",
			"ring_pushes", ringPushes,
			"wal_spills", walSpills,
			"wal_drained", walDrained,
			"dropped", dropped,
		)
	}

	slog.Info("shutdown complete")
	return 0
}

// runOutputBridge reads from the normalizer output channel, batches by poll interval,
// and calls Update on all outputs. Closes done when finished.
func runOutputBridge(ctx context.Context, ch <-chan normalizer.TelemetryPoint, outputs []output.Output, interval time.Duration, done chan struct{}, logger *slog.Logger) {
	defer close(done)
	if len(outputs) == 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var batch []normalizer.TelemetryPoint
	for {
		select {
		case pt, ok := <-ch:
			if !ok {
				// Channel closed, flush remaining
				if len(batch) > 0 {
					for _, out := range outputs {
						_ = out.Update(batch)
					}
				}
				return
			}
			batch = append(batch, pt)
		case <-ticker.C:
			if len(batch) > 0 {
				for _, out := range outputs {
					if err := out.Update(batch); err != nil {
						logger.Error("output update error", "error", err)
					}
				}
				batch = batch[:0]
			}
		case <-ctx.Done():
			if len(batch) > 0 {
				for _, out := range outputs {
					_ = out.Update(batch)
				}
			}
			return
		}
	}
}

func initLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	return slog.New(handler)
}
