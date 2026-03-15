// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package hub

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

// Hub aggregates metrics from peer agents and exposes a fleet API.
type Hub struct {
	config           config.HubConfig
	registry         *PeerRegistry
	scraper          *Scraper
	api              *FleetAPI
	deviceName       string
	logger           *slog.Logger
	httpServer       *http.Server
	localDevices     []PeerDevice
	localMu          sync.RWMutex
	peerMetrics      map[string]map[string]*dto.MetricFamily // peerID -> MetricFamilies cache
	peerMetricsMu    sync.RWMutex
	hubSummary       *hubSummaryMetrics
	hubRegistry      prometheus.Gatherer
	lastScrapeErrors int64
	lastScrapeMu     sync.Mutex
	shutdownOnce     sync.Once
}

type hubSummaryMetrics struct {
	peersTotal     prometheus.Gauge
	peersHealthy   prometheus.Gauge
	devicesTotal   prometheus.Gauge
	scrapeDuration prometheus.Gauge
	scrapeErrors   prometheus.Counter
}

// NewHub creates a new Hub.
func NewHub(cfg config.HubConfig, deviceName string, logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	registry := NewPeerRegistry()
	interval := cfg.ScrapeInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	scraper := NewScraper(interval, registry, logger)

	hubReg := prometheus.NewRegistry()
	hubSummary := &hubSummaryMetrics{
		peersTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "keldron_hub_peers_total",
			Help: "Number of known peers",
		}),
		peersHealthy: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "keldron_hub_peers_healthy",
			Help: "Number of healthy peers",
		}),
		devicesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "keldron_hub_devices_total",
			Help: "Total devices across fleet",
		}),
		scrapeDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "keldron_hub_scrape_duration_seconds",
			Help: "Last scrape cycle duration in seconds",
		}),
		scrapeErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "keldron_hub_scrape_errors_total",
			Help: "Cumulative scrape failures",
		}),
	}
	hubReg.MustRegister(
		hubSummary.peersTotal,
		hubSummary.peersHealthy,
		hubSummary.devicesTotal,
		hubSummary.scrapeDuration,
		hubSummary.scrapeErrors,
	)

	h := &Hub{
		config:      cfg,
		registry:    registry,
		scraper:     scraper,
		deviceName:  deviceName,
		logger:      logger,
		peerMetrics: make(map[string]map[string]*dto.MetricFamily),
		hubSummary:  hubSummary,
		hubRegistry: hubReg,
	}

	h.api = NewFleetAPI(func() FleetState {
		h.localMu.RLock()
		local := append([]PeerDevice(nil), h.localDevices...)
		h.localMu.RUnlock()
		return BuildFleetState(local, h.registry)
	})

	scraper.SetPeerMetricsCallback(func(peerID string, families map[string]*dto.MetricFamily) {
		h.peerMetricsMu.Lock()
		h.peerMetrics[peerID] = families
		h.peerMetricsMu.Unlock()
	})

	return h
}

// Start implements output.Output. It blocks on http.Server.ListenAndServe;
// callers should run it in a goroutine if non-blocking behavior is required.
func (h *Hub) Start(ctx context.Context) error {
	// Add static peers
	for _, addr := range h.config.StaticPeers {
		if addr != "" {
			h.registry.AddPeer(addr)
		}
	}
	if h.config.MDNSEnabled {
		h.logger.Info("mDNS discovery not yet implemented (OSS-022)")
	}

	// Start scraper
	go h.scraper.Start(ctx)

	// Build merged /metrics handler
	gatherer := h.buildMetricsGatherer()
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	mux.Handle("/", h.api.Handler())

	addr := ":" + strconv.Itoa(h.config.ListenPort)
	h.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		h.shutdownOnce.Do(func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.httpServer.Shutdown(shutdownCtx)
		})
	}()

	h.logger.Info("Hub mode active — fleet API at http://localhost:"+strconv.Itoa(h.config.ListenPort)+"/api/v1/fleet",
		"port", h.config.ListenPort)
	if err := h.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (h *Hub) buildMetricsGatherer() prometheus.Gatherer {
	return prometheus.GathererFunc(func() ([]*dto.MetricFamily, error) {
		merged := make(map[string]*dto.MetricFamily)

		mergeInto := func(families []*dto.MetricFamily) {
			for _, mf := range families {
				if mf == nil {
					continue
				}
				name := mf.GetName()
				if existing, ok := merged[name]; ok {
					existing.Metric = append(existing.Metric, mf.Metric...)
					if existing.Help == nil && mf.Help != nil {
						existing.Help = mf.Help
					}
					if existing.Type == nil && mf.Type != nil {
						existing.Type = mf.Type
					}
				} else {
					merged[name] = mf
				}
			}
		}

		// Gather local metrics from default registry
		local, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			h.logger.Warn("failed to gather local metrics, continuing with peer/hub metrics", "error", err)
		}
		mergeInto(local)

		// Gather peer metrics from cache
		h.peerMetricsMu.RLock()
		for _, families := range h.peerMetrics {
			for _, mf := range families {
				if mf != nil {
					mergeInto([]*dto.MetricFamily{mf})
				}
			}
		}
		h.peerMetricsMu.RUnlock()

		// Update hub summary
		peers := h.registry.GetPeers()
		healthy := 0
		for _, p := range peers {
			if p.Healthy {
				healthy++
			}
		}
		h.localMu.RLock()
		localCount := len(h.localDevices)
		h.localMu.RUnlock()
		totalDevices := localCount
		for _, p := range peers {
			totalDevices += len(p.Devices)
		}

		h.hubSummary.peersTotal.Set(float64(len(peers)))
		h.hubSummary.peersHealthy.Set(float64(healthy))
		h.hubSummary.devicesTotal.Set(float64(totalDevices))
		h.hubSummary.scrapeDuration.Set(h.scraper.LastDuration().Seconds())
		h.lastScrapeMu.Lock()
		curr := h.scraper.ScrapeErrors()
		delta := curr - h.lastScrapeErrors
		h.lastScrapeErrors = curr
		if delta > 0 {
			h.hubSummary.scrapeErrors.Add(float64(delta))
		}
		h.lastScrapeMu.Unlock()

		// Gather hub summary metrics — log and continue on error (same
		// policy as local gather) so /metrics serves partial results.
		hubFamilies, err := h.hubRegistry.Gather()
		if err != nil {
			h.logger.Warn("failed to gather hub summary metrics, continuing with collected metrics", "error", err)
		}
		mergeInto(hubFamilies)

		out := make([]*dto.MetricFamily, 0, len(merged))
		for _, mf := range merged {
			out = append(out, mf)
		}
		return out, nil
	})
}

// Update implements output.Output. Converts telemetry to PeerDevices and stores locally.
func (h *Hub) Update(readings []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) error {
	devices := telemetryToPeerDevices(readings, scores)
	h.localMu.Lock()
	h.localDevices = devices
	h.localMu.Unlock()
	return nil
}

// Close implements output.Output. Shuts down the HTTP server.
func (h *Hub) Close() error {
	if h.httpServer == nil {
		return nil
	}
	var shutdownErr error
	h.shutdownOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr = h.httpServer.Shutdown(ctx)
	})
	return shutdownErr
}

// telemetryToPeerDevices converts TelemetryPoint + RiskScoreOutput to PeerDevice.
func telemetryToPeerDevices(readings []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) []PeerDevice {
	scoresByDevice := make(map[string]scoring.RiskScoreOutput)
	for _, s := range scores {
		scoresByDevice[s.DeviceID] = s
	}

	devicesByID := make(map[string]*PeerDevice)
	for _, pt := range readings {
		deviceID := deviceIDFromPoint(pt)
		model := deviceModelFromPoint(pt)
		d, ok := devicesByID[deviceID]
		if !ok {
			d = &PeerDevice{
				DeviceID:      deviceID,
				DeviceModel:   model,
				DeviceVendor:  "",
				BehaviorClass: "",
				LastUpdated:   time.Now(),
			}
			if pt.Tags != nil {
				if v, ok := pt.Tags["device_vendor"]; ok {
					d.DeviceVendor = v
				}
				if v, ok := pt.Tags["behavior_class"]; ok {
					d.BehaviorClass = v
				}
			}
			devicesByID[deviceID] = d
		}

		m := pt.Metrics
		if m == nil {
			m = make(map[string]float64)
		}
		if v, ok := m["temperature_c"]; ok {
			d.TemperatureC = v
		}
		if v, ok := m["power_usage_w"]; ok {
			d.PowerW = v
		}
		if v, ok := m["gpu_utilization_pct"]; ok {
			d.Utilization = v / 100
		}
		if used, ok1 := m["mem_used_bytes"]; ok1 {
			if total, ok2 := m["mem_total_bytes"]; ok2 && total > 0 {
				d.MemoryPressure = used / total
			}
		}

		if sc, ok := scoresByDevice[deviceID]; ok {
			d.RiskComposite = sc.Composite
			d.RiskSeverity = sc.Severity
		}
	}

	out := make([]PeerDevice, 0, len(devicesByID))
	for _, d := range devicesByID {
		out = append(out, *d)
	}
	return out
}

func deviceIDFromPoint(pt normalizer.TelemetryPoint) string {
	if pt.Metrics != nil {
		if gpuID, ok := pt.Metrics["gpu_id"]; ok {
			return pt.Source + ":" + strconv.FormatFloat(gpuID, 'f', 0, 64)
		}
	}
	return pt.Source
}

func deviceModelFromPoint(pt normalizer.TelemetryPoint) string {
	if pt.Tags != nil {
		for _, k := range []string{"device_model", "gpu_model", "gpu_name", "model"} {
			if v, ok := pt.Tags[k]; ok && v != "" {
				return v
			}
		}
	}
	return "unknown"
}
