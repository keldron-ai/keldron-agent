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
	port           int
	version        string
	deviceName     string
	startedAt      time.Time
	logger         *slog.Logger
	activeAdapters []string
	deviceCount    int

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
	riskComposite    *prometheus.GaugeVec
	riskThermal      *prometheus.GaugeVec
	riskPower        *prometheus.GaugeVec
	riskVolatility   *prometheus.GaugeVec
	riskFleetPenalty *prometheus.GaugeVec
	riskSeverity     *prometheus.GaugeVec
	riskWarmingUp    *prometheus.GaugeVec

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
	p := &Prometheus{
		port:       port,
		version:    version,
		deviceName: deviceName,
		startedAt:  time.Now(),
		logger:     logger,
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
	p.riskFleetPenalty = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_fleet_penalty",
		Help: "Fleet penalty risk score",
	}, stringsToLabels(deviceIDLabels))
	p.riskSeverity = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_severity",
		Help: "0=normal, 1=warning, 2=critical",
	}, stringsToLabels(deviceIDLabels))
	p.riskWarmingUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_risk_warming_up",
		Help: "1 if device warming up, 0 otherwise",
	}, stringsToLabels(deviceIDLabels))

	// Bonus metrics
	p.gpuMemPressureRatio = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_memory_pressure_ratio",
		Help: "GPU memory used/total ratio",
	}, stringsToLabels(deviceIDLabels))
	p.gpuClockEfficiency = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "keldron_gpu_clock_efficiency",
		Help: "GPU clock efficiency ratio",
	}, stringsToLabels(deviceIDLabels))
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
	}, stringsToLabels(deviceIDLabels))

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
		p.riskFleetPenalty, p.riskSeverity, p.riskWarmingUp,
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
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /healthz", p.handleHealthz)
	mux.HandleFunc("GET /api/v1/status", p.handleStatus)
	return mux
}

// Start starts the HTTP server. Blocks until ctx is cancelled.
func (p *Prometheus) Start(ctx context.Context) error {
	addr := ":" + strconv.Itoa(p.port)
	p.httpServer = &http.Server{
		Addr:         addr,
		Handler:      p.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.httpServer.Shutdown(shutdownCtx)
	}()

	p.logger.Info("Prometheus server starting", "addr", addr)
	err := p.httpServer.ListenAndServe()
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

// SetActiveAdapters sets the list of active adapters for /api/v1/status.
func (p *Prometheus) SetActiveAdapters(adapters []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activeAdapters = append([]string(nil), adapters...)
}

// Update applies telemetry points to Prometheus gauges.
func (p *Prometheus) Update(readings []normalizer.TelemetryPoint) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.deviceCount = len(readings)
	p.agentInfo.WithLabelValues(p.version, p.deviceName).Set(1)

	for _, pt := range readings {
		p.updatePoint(pt)
	}
	return nil
}

func (p *Prometheus) updatePoint(pt normalizer.TelemetryPoint) {
	deviceID := p.deviceID(pt)
	deviceModel := p.deviceModel(pt)
	spec := registry.Lookup(registry.NormalizeModelName(deviceModel))
	deviceVendor := spec.Vendor
	behaviorClass := spec.BehaviorClass
	adapter := pt.AdapterName

	gpuLabels := prometheus.Labels{
		"device_model":   deviceModel,
		"device_vendor":  deviceVendor,
		"device_id":      deviceID,
		"behavior_class": behaviorClass,
		"adapter":        adapter,
	}
	deviceLabels := prometheus.Labels{"device_model": deviceModel, "device_id": deviceID}
	deviceIDLabels := prometheus.Labels{"device_id": deviceID}
	deviceBehaviorLabels := prometheus.Labels{"device_id": deviceID, "behavior_class": behaviorClass}

	m := pt.Metrics
	if m == nil {
		m = make(map[string]float64)
	}

	// GPU metrics
	if v, ok := m["temperature_c"]; ok {
		p.gpuTempC.With(gpuLabels).Set(v)
	}
	if v, ok := m["temperature_junction_c"]; ok {
		p.gpuHotspotTempC.With(gpuLabels).Set(v)
	} else if v, ok := m["temperature_edge"]; ok {
		p.gpuHotspotTempC.With(gpuLabels).Set(v)
	} else if v, ok := m["temperature_c"]; ok {
		p.gpuHotspotTempC.With(gpuLabels).Set(v)
	}
	if v, ok := m["power_usage_w"]; ok {
		p.gpuPowerW.With(gpuLabels).Set(v)
	}
	if v, ok := m["gpu_utilization_pct"]; ok {
		p.gpuUtilization.With(gpuLabels).Set(v / 100)
	}
	if v, ok := m["mem_used_bytes"]; ok {
		p.gpuMemUsedBytes.With(gpuLabels).Set(v)
	}
	if v, ok := m["mem_total_bytes"]; ok {
		p.gpuMemTotalBytes.With(gpuLabels).Set(v)
	}
	if v, ok := m["sm_clock_mhz"]; ok {
		p.gpuClockSMMHz.With(gpuLabels).Set(v)
	}
	if v, ok := m["sm_clock_max_mhz"]; ok {
		p.gpuClockMaxMHz.With(gpuLabels).Set(v)
	}
	if v, ok := m["throttled"]; ok {
		throttle := 0.0
		if v > 0 {
			throttle = 1
		}
		p.gpuThrottleActive.With(gpuLabels).Set(throttle)
	}

	// CPU temp
	if v, ok := m["cpu_temp_c"]; ok {
		p.cpuTempC.With(deviceLabels).Set(v)
	}
	// Fan
	if v, ok := m["fan_speed_rpm"]; ok {
		p.fanSpeedRPM.With(deviceLabels).Set(v)
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
		p.deviceUptimeSeconds.With(deviceIDLabels).Set(v)
	}

	// Risk placeholders (OSS-003 fills in)
	p.riskComposite.With(deviceBehaviorLabels).Set(0)
	p.riskThermal.With(deviceIDLabels).Set(0)
	p.riskPower.With(deviceIDLabels).Set(0)
	p.riskVolatility.With(deviceIDLabels).Set(0)
	p.riskFleetPenalty.With(deviceIDLabels).Set(0)
	p.riskSeverity.With(deviceIDLabels).Set(0)
	p.riskWarmingUp.With(deviceIDLabels).Set(0)

	// Bonus
	if used, ok1 := m["mem_used_bytes"]; ok1 {
		if total, ok2 := m["mem_total_bytes"]; ok2 && total > 0 {
			p.gpuMemPressureRatio.With(deviceIDLabels).Set(used / total)
		}
	}
	if sm, ok1 := m["sm_clock_mhz"]; ok1 {
		if max, ok2 := m["sm_clock_max_mhz"]; ok2 && max > 0 {
			p.gpuClockEfficiency.With(deviceIDLabels).Set(sm / max)
		}
	}
	if power, ok := m["power_usage_w"]; ok {
		// Assume 0.12 $/kWh
		rate := 0.12 / 1000
		hourly := power * rate
		p.powerCostHourly.With(deviceIDLabels).Set(hourly)
		p.powerCostDaily.With(deviceIDLabels).Set(hourly * 24)
		p.powerCostMonthly.With(deviceIDLabels).Set(hourly * 24 * 30)
	}
	// Hotspot delta: junction - edge if both available, -1 otherwise
	if j, ok1 := m["temperature_junction_c"]; ok1 {
		if e, ok2 := m["temperature_edge"]; ok2 {
			p.gpuHotspotDeltaC.With(deviceIDLabels).Set(j - e)
		} else {
			p.gpuHotspotDeltaC.With(deviceIDLabels).Set(-1)
		}
	} else {
		p.gpuHotspotDeltaC.With(deviceIDLabels).Set(-1)
	}
}

func (p *Prometheus) deviceID(pt normalizer.TelemetryPoint) string {
	if gpuID, ok := pt.Metrics["gpu_id"]; ok {
		return pt.Source + ":" + strconv.FormatFloat(gpuID, 'f', 0, 64)
	}
	return pt.Source
}

func (p *Prometheus) deviceModel(pt normalizer.TelemetryPoint) string {
	// gpu_name is typically a string from adapters, so normalizer drops it (Metrics is float64).
	// OSS-003 or normalizer enhancement could add device_model.
	if pt.Metrics == nil {
		return "unknown"
	}
	for _, k := range []string{"gpu_name", "model", "device_model"} {
		if v, ok := pt.Metrics[k]; ok {
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
	}
	return "unknown"
}

// Close shuts down the HTTP server.
func (p *Prometheus) Close() error {
	if p.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.httpServer.Shutdown(ctx)
}
