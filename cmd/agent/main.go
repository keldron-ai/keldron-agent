// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/adapter/kubernetes"
	"github.com/keldron-ai/keldron-agent/internal/adapter/rocm"
	"github.com/keldron-ai/keldron-agent/internal/adapter/slurm"
	"github.com/keldron-ai/keldron-agent/internal/adapter/snmp_pdu"
	"github.com/keldron-ai/keldron-agent/internal/adapter/temperature"
	"github.com/keldron-ai/keldron-agent/internal/api"
	"github.com/keldron-ai/keldron-agent/internal/buffer"
	"github.com/keldron-ai/keldron-agent/internal/cloud"
	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/dcgm"
	"github.com/keldron-ai/keldron-agent/internal/discovery"
	"github.com/keldron-ai/keldron-agent/internal/fake"
	"github.com/keldron-ai/keldron-agent/internal/health"
	"github.com/keldron-ai/keldron-agent/internal/hub"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/output"
	"github.com/keldron-ai/keldron-agent/internal/scan"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
	"github.com/keldron-ai/keldron-agent/internal/sender"
)

// Set at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	// Scan subcommand: one-shot fleet query, does not start the agent
	if len(os.Args) > 1 && os.Args[1] == "scan" {
		return scan.Run(os.Args[2:])
	}

	configPath := flag.String("config", "./keldron-agent.yaml", "path to YAML config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	localMode := flag.Bool("local", false, "run in local-only mode (no cloud streaming)")
	quiet := flag.Bool("quiet", false, "disable stdout output (use when running in background with scan --watch)")
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

	// Initialize structured logger. When --quiet, redirect slog to stderr so
	// stdout stays clean for scan --watch dashboard (agent runs in background).
	var logWriter io.Writer = os.Stdout
	if *quiet {
		logWriter = os.Stderr
	}
	slog.SetDefault(initLogger(cfg.Agent.LogLevel, logWriter))

	// Config holder for hot-reload support (S-006).
	cfgHolder := config.NewHolder(cfg)

	// Subscribe to log_level changes for hot-reload.
	cfgHolder.Subscribe(func(cfg *config.Config) {
		slog.SetDefault(initLogger(cfg.Agent.LogLevel, logWriter))
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
		"quiet", *quiet,
	)
	if cfg.Cloud.APIKey != "" {
		slog.Info("cloud configured", "api_key", config.MaskedCloudAPIKey(cfg.Cloud.APIKey), "endpoint", cfg.Cloud.Endpoint)
	}

	// Build adapter registry.
	registry := adapter.NewRegistry()
	registerPlatformAdapters(registry)
	registerNvidia(registry)
	registerLinuxAdapters(registry)
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

	// Local mode: --local flag, hub enabled, OR no cloud and (prometheus or stdout) enabled
	isLocalMode := *localMode || cfg.Hub.Enabled || (cfg.Cloud.APIKey == "" && (cfg.Output.Prometheus || cfg.Output.Stdout))

	var cloudClient *cloud.Client
	if cfg.Cloud.APIKey != "" && !*localMode {
		endpoint := cfg.Cloud.Endpoint
		if endpoint == "" {
			endpoint = "https://api.keldron.ai"
		}
		cloudClient = cloud.NewClient(endpoint, cfg.Cloud.APIKey, cfg.Agent.ID, version)
		slog.Info("cloud streaming enabled", "endpoint", endpoint)
	}

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
	var mdnsAdvertiser *discovery.MDNSAdvertiser
	var apiServer *api.Server
	var stateHolder *api.StateHolder

	// Build adapter name list once (used by outputs and API).
	activeAdapters := make([]string, 0, len(running))
	for _, a := range running {
		activeAdapters = append(activeAdapters, a.Name())
	}

	// Initialize StateHolder, history buffer, and health engine for API (independent of local/cloud mode).
	var healthEngine *health.Engine
	var historyBuffer *api.HistoryBuffer
	if cfg.API.Enabled {
		stateHolder = api.NewStateHolder()
		historyBuffer = api.NewHistoryBuffer(cfg.API.HistoryPoints)
		stateHolder.SetHistoryBuffer(historyBuffer)
		healthEngine = health.NewEngine()
	}

	if isLocalMode {
		if cfg.Output.Prometheus {
			slog.Info("running in local mode — metrics available at http://localhost:" + fmt.Sprintf("%d", cfg.Output.PrometheusPort) + "/metrics")
		} else {
			slog.Info("running in local mode — Prometheus metrics disabled, stdout output only")
		}
		if cfg.Hub.Enabled {
			slog.Info("running as hub — local monitoring + fleet aggregation")
		}

		// Build outputs
		if cfg.Output.Prometheus {
			prom := output.NewPrometheus(cfg.Output.PrometheusPort, version, cfg.Agent.DeviceName, logger.With("component", "prometheus"))
			prom.SetElectricityRate(cfg.Agent.ElectricityRate)
			prom.SetActiveAdapters(activeAdapters)
			outputs = append(outputs, prom)
			go func() {
				if err := prom.Start(ctx); err != nil && err != http.ErrServerClosed {
					logger.Error("Prometheus server stopped", "error", err)
				}
			}()
		}
		if cfg.Output.Stdout && !*quiet {
			std := output.NewStdout(os.Stdout, version, activeAdapters)
			outputs = append(outputs, std)
		}
		if cfg.Hub.Enabled {
			h := hub.NewHub(cfg.Hub, cfg.Agent.DeviceName, cfg.Output.PrometheusPort, logger.With("component", "hub"))
			outputs = append(outputs, h)
			go func() {
				if err := h.Start(ctx); err != nil && err != http.ErrServerClosed {
					logger.Error("Hub server stopped", "error", err)
				}
			}()
		}

		// mDNS advertisement: every agent advertises for zero-config discovery
		if cfg.Output.Prometheus && cfg.Output.MDNSAdvertise {
			mdnsAdvertiser = discovery.NewAdvertiserSafe(
				cfg.Agent.DeviceName,
				cfg.Output.PrometheusPort,
				version,
				1,
				logger.With("component", "mdns"),
			)
		}

		// Output bridge: read from normalizer, batch by poll interval, score, update outputs
		scoreEngine := scoring.NewScoreEngine(cfg.Agent.ElectricityRate)
		outputBridgeDone = make(chan struct{})
		go runOutputBridge(ctx, norm.Output(), outputs, scoreEngine, stateHolder, healthEngine, cfg.Agent.PollInterval, outputBridgeDone, logger, cloudClient, nil, version)
	} else if cfg.Sender.Target != "" {
		// Cloud mode with gRPC sender: buffer + sender + optional output bridge
		normCh := norm.Output()

		// Tee normalizer output to the buffer manager and the output bridge when the API is enabled
		// (dashboard) or HTTPS cloud streaming is enabled (scoring + cloud ingest).
		if cfg.API.Enabled || cloudClient != nil {
			bufCh := make(chan normalizer.TelemetryPoint, 256)
			bridgeCh := make(chan normalizer.TelemetryPoint, 256)
			// Separate lossless channel for cloud-bound telemetry.
			var cloudCh chan normalizer.TelemetryPoint
			if cloudClient != nil {
				cloudCh = make(chan normalizer.TelemetryPoint, 1024)
			}
			var bridgeDropped uint64
			go func() {
				defer close(bufCh)
				defer close(bridgeCh)
				if cloudCh != nil {
					defer close(cloudCh)
				}
				for pt := range normCh {
					select {
					case bufCh <- pt:
					case <-ctx.Done():
						return
					}
					// Dashboard bridge: lossy (non-blocking) — acceptable to drop for UI.
					select {
					case bridgeCh <- pt:
					default:
						bridgeDropped++
						if bridgeDropped%100 == 1 {
							logger.Warn("API bridge channel full, dropping telemetry point",
								"source", pt.Source, "adapter", pt.AdapterName, "total_dropped", bridgeDropped)
						}
					}
					// Cloud channel: lossless (blocking) — every point must reach cloud for retry.
					if cloudCh != nil {
						select {
						case cloudCh <- pt:
						case <-ctx.Done():
							return
						}
					}
				}
			}()
			normCh = bufCh

			scoreEngine := scoring.NewScoreEngine(cfg.Agent.ElectricityRate)
			outputBridgeDone = make(chan struct{})
			// Pass cloudCh (lossless) for cloud sends; bridgeCh (lossy) for dashboard/API only.
			go runOutputBridge(ctx, bridgeCh, nil, scoreEngine, stateHolder, healthEngine, cfg.Agent.PollInterval, outputBridgeDone, logger, cloudClient, cloudCh, version)
		}

		var err error
		bufMgr, err = buffer.NewManager(cfg.Buffer, normCh, logger.With("component", "buffer"))
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
	} else {
		// Cloud-only mode (HTTPS streaming, no gRPC sender): output bridge reads directly
		// from normalizer — no buffer manager or tee needed.
		scoreEngine := scoring.NewScoreEngine(cfg.Agent.ElectricityRate)
		outputBridgeDone = make(chan struct{})
		go runOutputBridge(ctx, norm.Output(), nil, scoreEngine, stateHolder, healthEngine, cfg.Agent.PollInterval, outputBridgeDone, logger, cloudClient, nil, version)
	}

	// API server for dashboard (OSS-028) — works in both local and cloud modes.
	if cfg.API.Enabled {
		apiServer = api.NewServer(stateHolder, version, cfg.Agent.PollInterval, activeAdapters, cfg.Cloud.APIKey != "", historyBuffer)
		host := cfg.API.Host
		if host == "" {
			host = "127.0.0.1"
		}
		addr := net.JoinHostPort(host, strconv.Itoa(cfg.API.Port))
		go func() {
			if err := apiServer.Start(addr); err != nil && err != http.ErrServerClosed {
				logger.Error("API server failed", "error", err)
			}
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

	// Wait for output bridge to finish (runs in both local and cloud+API modes).
	if outputBridgeDone != nil {
		<-outputBridgeDone
	}
	if cloudClient != nil {
		_ = cloudClient.Close()
	}

	// Shut down the API server (works in both modes).
	if apiServer != nil {
		if err := apiServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("API server shutdown error", "error", err)
		}
	}

	if isLocalMode {
		if mdnsAdvertiser != nil {
			mdnsAdvertiser.Stop()
		}
		for _, out := range outputs {
			if err := out.Close(); err != nil {
				logger.Error("output close error", "error", err)
			}
		}
	} else {
		// Wait for sender to flush remaining batches and close its stream.
		if senderDone != nil {
			if err := <-senderDone; err != nil {
				slog.Error("sender stopped with error", "error", err)
			}
			batchesSent, pointsSent, senderErrors := sndr.Stats()
			slog.Info("sender stats",
				"batches_sent", batchesSent,
				"points_sent", pointsSent,
				"errors", senderErrors,
			)
		}

		// Wait for buffer manager to close WAL.
		if bufferDone != nil {
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
	}

	slog.Info("shutdown complete")
	return 0
}

// runOutputBridge reads from the normalizer output channel, batches by poll interval,
// computes risk scores, and calls Update on all outputs. Closes done when finished.
// stateHolder is optional; when set, it receives batch and scores for the API.
// healthEngine is optional; when set with stateHolder, it computes device health metrics.
// cloudCh is an optional separate lossless channel for cloud-bound telemetry; when nil
// the bridge uses its main ch for cloud sends.
func runOutputBridge(ctx context.Context, ch <-chan normalizer.TelemetryPoint, outputs []output.Output, scoreEngine *scoring.ScoreEngine, stateHolder *api.StateHolder, healthEngine *health.Engine, interval time.Duration, done chan struct{}, logger *slog.Logger, cloudClient *cloud.Client, cloudCh <-chan normalizer.TelemetryPoint, version string) {
	defer close(done)

	// WaitGroup tracks in-flight cloud Send goroutines so we can wait on shutdown.
	var sendWg sync.WaitGroup

	flushBatch := func(batch []normalizer.TelemetryPoint) {
		if len(batch) == 0 {
			return
		}
		scores := scoreEngine.Score(batch)
		var healthSnapshots map[string]*health.DeviceHealthSnapshot
		if healthEngine != nil {
			healthSnapshots = healthEngine.Update(batch)
		}
		if stateHolder != nil {
			stateHolder.Update(batch, scores, healthSnapshots)
		}
		if len(outputs) > 0 {
			for _, out := range outputs {
				if err := out.Update(batch, scores); err != nil {
					logger.Error("output update error", "error", err)
				}
			}
		}
	}

	// sendCloud sends samples using a background context so in-flight sends
	// survive parent context cancellation during shutdown.
	sendCloud := func(batch []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) {
		if cloudClient == nil || len(batch) == 0 {
			return
		}
		samples := cloud.ConvertToSamples(batch, scores, version)
		if len(samples) == 0 {
			return
		}
		cloudClient.TrackSend()
		sendWg.Add(1)
		go func() {
			defer sendWg.Done()
			defer cloudClient.SendDone()
			sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := cloudClient.Send(sendCtx, samples); err != nil {
				logger.Warn("cloud send failed (buffered for retry)", "error", err)
			}
		}()
	}

	// If we have a separate lossless cloud channel, run a dedicated goroutine for it.
	if cloudCh != nil && cloudClient != nil {
		go func() {
			var cloudBatch []normalizer.TelemetryPoint
			cloudTicker := time.NewTicker(interval)
			defer cloudTicker.Stop()
			for {
				select {
				case pt, ok := <-cloudCh:
					if !ok {
						scores := scoreEngine.Score(cloudBatch)
						sendCloud(cloudBatch, scores)
						return
					}
					cloudBatch = append(cloudBatch, pt)
				case <-cloudTicker.C:
					if len(cloudBatch) > 0 {
						scores := scoreEngine.Score(cloudBatch)
						sendCloud(cloudBatch, scores)
						cloudBatch = cloudBatch[:0]
					}
				}
			}
		}()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var batch []normalizer.TelemetryPoint
	initialFlush := true // Flush on first reading for immediate metric visibility.
	for {
		select {
		case pt, ok := <-ch:
			if !ok {
				flushBatch(batch)
				// When no separate cloudCh, send remaining via main path.
				if cloudCh == nil {
					scores := scoreEngine.Score(batch)
					sendCloud(batch, scores)
				}
				sendWg.Wait()
				return
			}
			batch = append(batch, pt)
			if initialFlush {
				flushBatch(batch)
				if cloudCh == nil {
					scores := scoreEngine.Score(batch)
					sendCloud(batch, scores)
				}
				batch = batch[:0]
				initialFlush = false
				ticker.Reset(interval)
			}
		case <-ticker.C:
			flushBatch(batch)
			if cloudCh == nil {
				scores := scoreEngine.Score(batch)
				sendCloud(batch, scores)
			}
			batch = batch[:0]
		case <-ctx.Done():
			// Context cancelled — flush current batch then drain ch until closed.
			flushBatch(batch)
			if cloudCh == nil {
				scores := scoreEngine.Score(batch)
				sendCloud(batch, scores)
			}
			batch = batch[:0]
			ticker.Stop()
			for pt := range ch {
				batch = append(batch, pt)
			}
			flushBatch(batch)
			if cloudCh == nil {
				scores := scoreEngine.Score(batch)
				sendCloud(batch, scores)
			}
			sendWg.Wait()
			return
		}
	}
}

func initLogger(level string, w io.Writer) *slog.Logger {
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

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: logLevel,
	})
	return slog.New(handler)
}
