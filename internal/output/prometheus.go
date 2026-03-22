// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package output

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
	"github.com/keldron-ai/keldron-agent/registry"
)

const (
	// GPU metrics - labels: device_model, device_vendor, device_id, behavior_class, adapter
	gpuLabels = "device_model,device_vendor,device_id,behavior_class,adapter"
	// device-only labels
	deviceLabels = "device_model,device_id"
	// device_id only
	deviceIDLabels = "device_id"
	// device_id, behavior_class
	deviceBehaviorLabels = "device_id,behavior_class"
	// agent info
	agentInfoLabels = "version,device_name"
)

// Prometheus implements Output by exposing a Prometheus /metrics endpoint
// and updating gauges from TelemetryPoints.
type Prometheus struct {
	port                  int
	version               string
	deviceName            string
	startedAt             time.Time
	logger                *slog.Logger
	activeAdapters        []string
	deviceCount           int
	gatherer              prometheus.Gatherer
	electricityRatePerKWh float64

	httpServer *http.Server
	mu         sync.Mutex

	// Raw telemetry
	gpuTempC             *prometheus.GaugeVec
	gpuHotspotTempC      *prometheus.GaugeVec
	gpuPowerW            *prometheus.GaugeVec
	gpuUtilization       *prometheus.GaugeVec
	gpuMemUsedBytes      *prometheus.GaugeVec
	gpuMemTotalBytes     *prometheus.GaugeVec
	gpuClockSMMHz        *prometheus.GaugeVec
	gpuClockMaxMHz       *prometheus.GaugeVec
	gpuThrottleActive    *prometheus.GaugeVec
	cpuTempC             *prometheus.GaugeVec
	fanSpeedRPM          *prometheus.GaugeVec
	systemSwapUsedBytes  prometheus.Gauge
	systemSwapTotalBytes prometheus.Gauge
	deviceUptimeSeconds  *prometheus.GaugeVec

	// Risk scores (placeholder for OSS-003)
	riskComposite  *prometheus.GaugeVec
	riskThermal    *prometheus.GaugeVec
	riskPower      *prometheus.GaugeVec
	riskVolatility *prometheus.GaugeVec
	riskMemory     *prometheus.GaugeVec
	riskSeverity   *prometheus.GaugeVec
	riskWarmingUp  *prometheus.GaugeVec

	// Bonus metrics
	gpuMemPressureRatio *prometheus.GaugeVec
	gpuClockEfficiency  *prometheus.GaugeVec
	powerCostHourly     *prometheus.GaugeVec
	powerCostDaily      *prometheus.GaugeVec
	powerCostMonthly    *prometheus.GaugeVec
	gpuHotspotDeltaC    *prometheus.GaugeVec

	// Agent meta
	agentInfo *prometheus.GaugeVec
}

// NewPrometheus creates a Prometheus output that serves /metrics on the given port.
func NewPrometheus(port int, version, deviceName string, logger *slog.Logger) *Prometheus {
	return NewPrometheusWithRegistry(port, version, deviceName, prometheus.DefaultRegisterer, logger)
}

// NewPrometheusWithRegistry creates a Prometheus output with a custom registry (for testing).
func NewPrometheusWithRegistry(port int, version, deviceName string, reg prometheus.Registerer, logger *slog.Logger) *Prometheus {
	if logger == nil {
		logger = slog.Default()
	}

	// Determine the matching gatherer for the registerer.
	var gatherer prometheus.Gatherer
	if g, ok := reg.(prometheus.Gatherer); ok {
		gatherer = g
	} else {
		gatherer = prometheus.DefaultGatherer
	}

	p := &Prometheus{
		port:                  port,
		version:               version,
		deviceName:            deviceName,
		startedAt:             time.Now(),
		logger:                logger,
		gatherer:              gatherer,
		electricityRatePerKWh: 0.12,
	}
	p.registerMetricsWith(reg)
	return p
}

func (p *Prometheus) registerMetricsWith(reg prometheus.Registerer) {
	// Raw telemetry - GPU
	p.gpuTempC = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_temperature_celsius",
		Help: "GPU temperature in Celsius",
	}, stringsToLabels(gpuLabels))
	p.gpuHotspotTempC = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_hotspot_temperature_celsius",
		Help: "GPU hotspot/junction temperature in Celsius",
	}, stringsToLabels(gpuLabels))
	p.gpuPowerW = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_power_watts",
		Help: "GPU power draw in watts",
	}, stringsToLabels(gpuLabels))
	p.gpuUtilization = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_utilization_ratio",
		Help: "GPU utilization 0-1",
	}, stringsToLabels(gpuLabels))
	p.gpuMemUsedBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_memory_used_bytes",
		Help: "GPU memory used in bytes",
	}, stringsToLabels(gpuLabels))
	p.gpuMemTotalBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_memory_total_bytes",
		Help: "GPU memory total in bytes",
	}, stringsToLabels(gpuLabels))
	p.gpuClockSMMHz = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_clock_sm_mhz",
		Help: "GPU SM clock in MHz",
	}, stringsToLabels(gpuLabels))
	p.gpuClockMaxMHz = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_clock_max_mhz",
		Help: "GPU max clock in MHz",
	}, stringsToLabels(gpuLabels))
	p.gpuThrottleActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_throttle_active",
		Help: "1 if GPU is throttled, 0 otherwise",
	}, stringsToLabels(gpuLabels))

	// CPU, fan
	p.cpuTempC = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_cpu_temperature_celsius",
		Help: "CPU temperature in Celsius",
	}, stringsToLabels(deviceLabels))
	p.fanSpeedRPM = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_fan_speed_rpm",
		Help: "Fan speed in RPM",
	}, stringsToLabels(deviceLabels))

	// System
	p.systemSwapUsedBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "keldron_system_swap_used_bytes",
		Help: "System swap used in bytes",
	})
	p.systemSwapTotalBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "keldron_system_swap_total_bytes",
		Help: "System swap total in bytes",
	})
	p.deviceUptimeSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_device_uptime_seconds",
		Help: "Device uptime in seconds",
	}, stringsToLabels(deviceIDLabels))

	// Risk scores
	p.riskComposite = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_composite",
		Help: "Composite risk score",
	}, stringsToLabels(deviceBehaviorLabels))
	p.riskThermal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_thermal",
		Help: "Thermal risk score",
	}, stringsToLabels(deviceIDLabels))
	p.riskPower = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_power",
		Help: "Power risk score",
	}, stringsToLabels(deviceIDLabels))
	p.riskVolatility = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_volatility",
		Help: "Volatility risk score",
	}, stringsToLabels(deviceIDLabels))
	p.riskMemory = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_memory",
		Help: "Memory pressure risk score",
	}, stringsToLabels(deviceIDLabels))
	p.riskSeverity = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_severity",
		Help: "0=normal, 1=active, 2=elevated, 3=warning, 4=critical",
	}, stringsToLabels(deviceIDLabels))
	p.riskWarmingUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_warming_up",
		Help: "1 if device warming up, 0 otherwise",
	}, stringsToLabels(deviceIDLabels))

	// Bonus metrics — use full GPU labels for joinability with raw GPU metrics
	p.gpuMemPressureRatio = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_memory_pressure_ratio",
		Help: "GPU memory used/total ratio",
	}, stringsToLabels(gpuLabels))
	p.gpuClockEfficiency = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_clock_efficiency",
		Help: "GPU clock efficiency ratio",
	}, stringsToLabels(gpuLabels))
	p.powerCostHourly = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_power_cost_hourly",
		Help: "Estimated power cost per hour",
	}, stringsToLabels(deviceIDLabels))
	p.powerCostDaily = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_power_cost_daily",
		Help: "Estimated power cost per day",
	}, stringsToLabels(deviceIDLabels))
	p.powerCostMonthly = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_power_cost_monthly",
		Help: "Estimated power cost per month",
	}, stringsToLabels(deviceIDLabels))
	p.gpuHotspotDeltaC = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_hotspot_delta_celsius",
		Help: "Hotspot minus edge temp (NVIDIA only); -1 if unavailable",
	}, stringsToLabels(gpuLabels))

	// Agent meta
	p.agentInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_agent_info",
		Help: "Agent info (always 1)",
	}, stringsToLabels(agentInfoLabels))

	// Register all
	reg.MustRegister(
		p.gpuTempC, p.gpuHotspotTempC, p.gpuPowerW, p.gpuUtilization,
		p.gpuMemUsedBytes, p.gpuMemTotalBytes, p.gpuClockSMMHz, p.gpuClockMaxMHz, p.gpuThrottleActive,
		p.cpuTempC, p.fanSpeedRPM,
		p.systemSwapUsedBytes, p.systemSwapTotalBytes, p.deviceUptimeSeconds,
		p.riskComposite, p.riskThermal, p.riskPower, p.riskVolatility,
		p.riskMemory, p.riskSeverity, p.riskWarmingUp,
		p.gpuMemPressureRatio, p.gpuClockEfficiency,
		p.powerCostHourly, p.powerCostDaily, p.powerCostMonthly, p.gpuHotspotDeltaC,
		p.agentInfo,
	)
}

func stringsToLabels(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Handler returns the HTTP handler for testing. Used by Start as well.
func (p *Prometheus) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(p.gatherer, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /healthz", p.handleHealthz)
	mux.HandleFunc("GET /api/v1/status", p.handleStatus)
	return mux
}

// Start starts the HTTP server. Blocks until ctx is cancelled.
func (p *Prometheus) Start(ctx context.Context) error {
	addr := ":" + strconv.Itoa(p.port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      p.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	p.mu.Lock()
	p.httpServer = srv
	p.mu.Unlock()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	p.logger.Info("Prometheus server starting", "addr", addr)
	err := srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (p *Prometheus) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"uptime": time.Since(p.startedAt).Round(time.Second).String(),
	})
}

func (p *Prometheus) handleStatus(w http.ResponseWriter, _ *http.Request) {
	p.mu.Lock()
	adapters := append([]string(nil), p.activeAdapters...)
	count := p.deviceCount
	p.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"version":         p.version,
		"device_name":     p.deviceName,
		"uptime":          time.Since(p.startedAt).Round(time.Second).String(),
		"active_adapters": adapters,
		"device_count":    count,
	})
}

// SetElectricityRate overrides the default electricity rate ($/kWh) used for power cost estimates.
// Negative values are clamped to 0.
func (p *Prometheus) SetElectricityRate(ratePerKWh float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ratePerKWh < 0 {
		p.logger.Warn("negative electricity rate clamped to 0", "input", ratePerKWh)
		ratePerKWh = 0
	}
	p.electricityRatePerKWh = ratePerKWh
}

// SetActiveAdapters sets the list of active adapters for /api/v1/status.
func (p *Prometheus) SetActiveAdapters(adapters []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activeAdapters = append([]string(nil), adapters...)
}

// Update applies telemetry points and risk scores to Prometheus gauges.
// Clears stale metrics for devices no longer present.
// Scores may be nil; risk gauges use placeholders when nil.
func (p *Prometheus) Update(readings []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Reset all per-device GaugeVecs to clear stale label combinations.
	p.gpuTempC.Reset()
	p.gpuHotspotTempC.Reset()
	p.gpuPowerW.Reset()
	p.gpuUtilization.Reset()
	p.gpuMemUsedBytes.Reset()
	p.gpuMemTotalBytes.Reset()
	p.gpuClockSMMHz.Reset()
	p.gpuClockMaxMHz.Reset()
	p.gpuThrottleActive.Reset()
	p.cpuTempC.Reset()
	p.fanSpeedRPM.Reset()
	p.deviceUptimeSeconds.Reset()
	p.riskComposite.Reset()
	p.riskThermal.Reset()
	p.riskPower.Reset()
	p.riskVolatility.Reset()
	p.riskMemory.Reset()
	p.riskSeverity.Reset()
	p.riskWarmingUp.Reset()
	p.gpuMemPressureRatio.Reset()
	p.gpuClockEfficiency.Reset()
	p.powerCostHourly.Reset()
	p.powerCostDaily.Reset()
	p.powerCostMonthly.Reset()
	p.gpuHotspotDeltaC.Reset()

	scoresByDevice := make(map[string]scoring.RiskScoreOutput, len(scores))
	for _, s := range scores {
		scoresByDevice[s.DeviceID] = s
	}

	// Count unique devices (multiple readings may share a device ID).
	uniqueDevices := make(map[string]bool, len(readings))
	for _, pt := range readings {
		uniqueDevices[p.deviceID(pt)] = true
	}
	p.deviceCount = len(uniqueDevices)
	p.agentInfo.WithLabelValues(p.version, p.deviceName).Set(1)
	for _, pt := range readings {
		var score *scoring.RiskScoreOutput
		if sc, ok := scoresByDevice[p.deviceID(pt)]; ok {
			score = &sc
		}
		p.updatePoint(pt, score)
	}
	return nil
}

func (p *Prometheus) updatePoint(pt normalizer.TelemetryPoint, score *scoring.RiskScoreOutput) {
	deviceID := p.deviceID(pt)
	deviceModel := p.deviceModel(pt)
	spec := registry.Lookup(registry.NormalizeModelName(deviceModel))
	deviceVendor := spec.Vendor
	behaviorClass := spec.BehaviorClass
	// Prefer adapter-provided Tags when present
	if pt.Tags != nil {
		if v, ok := pt.Tags["device_vendor"]; ok && v != "" {
			deviceVendor = v
		}
		if v, ok := pt.Tags["behavior_class"]; ok && v != "" {
			behaviorClass = v
		}
	}
	adapter := pt.AdapterName

	gpuLbls := prometheus.Labels{
		"device_model":   deviceModel,
		"device_vendor":  deviceVendor,
		"device_id":      deviceID,
		"behavior_class": behaviorClass,
		"adapter":        adapter,
	}
	deviceLbls := prometheus.Labels{"device_model": deviceModel, "device_id": deviceID}
	deviceIDLbls := prometheus.Labels{"device_id": deviceID}
	deviceBehaviorLbls := prometheus.Labels{"device_id": deviceID, "behavior_class": behaviorClass}

	m := pt.Metrics
	if m == nil {
		m = make(map[string]float64)
	}

	// GPU metrics
	if v, ok := m["temperature_c"]; ok {
		p.gpuTempC.With(gpuLbls).Set(v)
	}
	if v, ok := m["temperature_junction_c"]; ok {
		p.gpuHotspotTempC.With(gpuLbls).Set(v)
	} else if v, ok := m["temperature_edge"]; ok {
		p.gpuHotspotTempC.With(gpuLbls).Set(v)
	} else if v, ok := m["temperature_c"]; ok {
		p.gpuHotspotTempC.With(gpuLbls).Set(v)
	}
	if v, ok := m["power_usage_w"]; ok {
		p.gpuPowerW.With(gpuLbls).Set(v)
	}
	if v, ok := m["gpu_utilization_pct"]; ok {
		p.gpuUtilization.With(gpuLbls).Set(v / 100)
	}
	if v, ok := m["mem_used_bytes"]; ok {
		p.gpuMemUsedBytes.With(gpuLbls).Set(v)
	}
	if v, ok := m["mem_total_bytes"]; ok {
		p.gpuMemTotalBytes.With(gpuLbls).Set(v)
	}
	if v, ok := m["sm_clock_mhz"]; ok {
		p.gpuClockSMMHz.With(gpuLbls).Set(v)
	}
	if v, ok := m["sm_clock_max_mhz"]; ok {
		p.gpuClockMaxMHz.With(gpuLbls).Set(v)
	}
	if v, ok := m["throttled"]; ok {
		throttle := 0.0
		if v > 0 {
			throttle = 1
		}
		p.gpuThrottleActive.With(gpuLbls).Set(throttle)
	}

	// CPU temp
	if v, ok := m["cpu_temp_c"]; ok {
		p.cpuTempC.With(deviceLbls).Set(v)
	}
	// Fan
	if v, ok := m["fan_speed_rpm"]; ok {
		p.fanSpeedRPM.With(deviceLbls).Set(v)
	}

	// System swap
	if v, ok := m["swap_used_bytes"]; ok {
		p.systemSwapUsedBytes.Set(v)
	}
	if v, ok := m["swap_total_bytes"]; ok {
		p.systemSwapTotalBytes.Set(v)
	}
	// Uptime
	if v, ok := m["uptime_seconds"]; ok {
		p.deviceUptimeSeconds.With(deviceIDLbls).Set(v)
	}

	// Risk scores — from scoring engine when available
	if score != nil {
		p.riskComposite.With(deviceBehaviorLbls).Set(score.Composite)
		p.riskThermal.With(deviceIDLbls).Set(score.Thermal)
		p.riskPower.With(deviceIDLbls).Set(score.Power)
		p.riskVolatility.With(deviceIDLbls).Set(score.Volatility)
		p.riskMemory.With(deviceIDLbls).Set(score.Memory)
		sev := 0.0
		switch score.Severity {
		case scoring.SeverityActive:
			sev = 1
		case scoring.SeverityElevated:
			sev = 2
		case scoring.SeverityWarning:
			sev = 3
		case scoring.SeverityCritical:
			sev = 4
		}
		p.riskSeverity.With(deviceIDLbls).Set(sev)
		warm := 0.0
		if score.WarmingUp {
			warm = 1
		}
		p.riskWarmingUp.With(deviceIDLbls).Set(warm)
		p.gpuMemPressureRatio.With(gpuLbls).Set(score.MemoryPressure)
		p.gpuClockEfficiency.With(gpuLbls).Set(score.ClockEfficiency)
		p.powerCostHourly.With(deviceIDLbls).Set(score.PowerCostHourly)
		p.powerCostDaily.With(deviceIDLbls).Set(score.PowerCostDaily)
		p.powerCostMonthly.With(deviceIDLbls).Set(score.PowerCostMonthly)
		p.gpuHotspotDeltaC.With(gpuLbls).Set(score.HotspotDeltaC)
		throttle := 0.0
		if score.ThrottleActive {
			throttle = 1
		}
		p.gpuThrottleActive.With(gpuLbls).Set(throttle)
	} else {
		p.riskComposite.With(deviceBehaviorLbls).Set(0)
		p.riskThermal.With(deviceIDLbls).Set(0)
		p.riskPower.With(deviceIDLbls).Set(0)
		p.riskVolatility.With(deviceIDLbls).Set(0)
		p.riskMemory.With(deviceIDLbls).Set(0)
		p.riskSeverity.With(deviceIDLbls).Set(0)
		p.riskWarmingUp.With(deviceIDLbls).Set(0)
		// Bonus fallbacks from metrics
		if used, ok1 := m["mem_used_bytes"]; ok1 {
			if total, ok2 := m["mem_total_bytes"]; ok2 && total > 0 {
				p.gpuMemPressureRatio.With(gpuLbls).Set(used / total)
			}
		}
		if sm, ok1 := m["sm_clock_mhz"]; ok1 {
			if max, ok2 := m["sm_clock_max_mhz"]; ok2 && max > 0 {
				p.gpuClockEfficiency.With(gpuLbls).Set(sm / max)
			}
		}
		if power, ok := m["power_usage_w"]; ok {
			rate := p.electricityRatePerKWh / 1000
			hourly := power * rate
			p.powerCostHourly.With(deviceIDLbls).Set(hourly)
			p.powerCostDaily.With(deviceIDLbls).Set(hourly * 24)
			p.powerCostMonthly.With(deviceIDLbls).Set(hourly * 24 * 30)
		}
		if j, ok1 := m["temperature_junction_c"]; ok1 {
			if e, ok2 := m["temperature_edge"]; ok2 {
				p.gpuHotspotDeltaC.With(gpuLbls).Set(j - e)
			} else {
				p.gpuHotspotDeltaC.With(gpuLbls).Set(-1)
			}
		} else {
			p.gpuHotspotDeltaC.With(gpuLbls).Set(-1)
		}
	}
}

func (p *Prometheus) deviceID(pt normalizer.TelemetryPoint) string {
	if gpuID, ok := pt.Metrics["gpu_id"]; ok {
		return pt.Source + ":" + strconv.FormatFloat(gpuID, 'f', 0, 64)
	}
	return pt.Source
}

func (p *Prometheus) deviceModel(pt normalizer.TelemetryPoint) string {
	// Check Tags for string metadata preserved from adapters.
	// Prefer device_model and gpu_model (adapter-provided) over gpu_name (raw system string).
	if pt.Tags != nil {
		for _, k := range []string{"device_model", "gpu_model", "gpu_name", "model"} {
			if v, ok := pt.Tags[k]; ok && v != "" {
				return v
			}
		}
	}
	// Fallback to numeric Metrics (unlikely but defensive).
	if pt.Metrics != nil {
		for _, k := range []string{"gpu_name", "model", "device_model"} {
			if v, ok := pt.Metrics[k]; ok {
				return strconv.FormatFloat(v, 'f', -1, 64)
			}
		}
	}
	return "unknown"
}

// Close shuts down the HTTP server.
func (p *Prometheus) Close() error {
	p.mu.Lock()
	srv := p.httpServer
	p.mu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
