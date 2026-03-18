// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
	"github.com/keldron-ai/keldron-agent/registry"
)

// Server is the HTTP API server for the dashboard.
type Server struct {
	stateHolder    *StateHolder
	hub            *wsHub
	mux            *http.ServeMux
	version        string
	pollInterval   time.Duration
	activeAdapters []string
	cloudConnected bool
}

// NewServer creates a new API server.
func NewServer(holder *StateHolder, version string, pollInterval time.Duration, activeAdapters []string, cloudConnected bool) *Server {
	hub := newWSHub()
	holder.SetBroadcastTarget(hub)

	s := &Server{
		stateHolder:    holder,
		hub:            hub,
		mux:            http.NewServeMux(),
		version:        version,
		pollInterval:   pollInterval,
		activeAdapters: activeAdapters,
		cloudConnected: cloudConnected,
	}

	s.mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/v1/risk", s.handleRisk)
	s.mux.HandleFunc("GET /api/v1/processes", s.handleProcesses)
	s.mux.HandleFunc("GET /ws/telemetry", s.handleWebSocket)
	s.mux.HandleFunc("GET /", HandleFrontend)

	return s
}

// Start starts the HTTP server. Blocks until the server stops.
func (s *Server) Start(addr string) error {
	slog.Info("API server starting", "addr", addr)
	return http.ListenAndServe(addr, corsMiddleware(s.mux))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	batch, scores := s.stateHolder.Get()
	if len(batch) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, StatusResponse{
			Device: DeviceInfo{Hostname: adapter.Hostname(), OS: runtime.GOOS, Arch: runtime.GOARCH},
			Agent:  AgentInfo{Version: s.version, PollIntervalS: int(s.pollInterval.Seconds()), AdaptersActive: s.activeAdapters, CloudConnected: s.cloudConnected},
		})
		return
	}

	pt := batch[0]
	m := pt.Metrics
	if m == nil {
		m = make(map[string]float64)
	}

	uptime := getMetricFloat(m, "uptime_seconds")
	if uptime <= 0 {
		uptime = SystemUptimeSeconds()
	}

	hardware := getTag(pt, "device_model")
	if hardware == "" {
		hardware = getTag(pt, "gpu_model")
	}
	if hardware == "" {
		hardware = getTag(pt, "gpu_name")
	}
	if hardware == "" {
		hardware = "unknown"
	}

	behaviorClass := getTag(pt, "behavior_class")
	if behaviorClass == "" {
		behaviorClass = "consumer_active_cooled"
	}

	memUsed := getMetricFloat(m, "mem_used_bytes")
	memTotal := getMetricFloat(m, "mem_total_bytes")
	memPct := 0.0
	if memTotal > 0 {
		memPct = memUsed / memTotal * 100
	}

	telemetry := TelemetryInfo{
		Timestamp:         pt.Timestamp.UTC().Format(time.RFC3339),
		TemperatureC:      getMetricFloat(m, "temperature_c", "temperature_junction_c", "temperature_edge"),
		GPUUtilizationPct: getMetricFloat(m, "gpu_utilization_pct"),
		PowerDrawW:        getMetricFloat(m, "power_usage_w"),
		MemoryUsedPct:     memPct,
		MemoryUsedBytes:   int64(memUsed),
		MemoryTotalBytes:  int64(memTotal),
		ThermalState:      getTag(pt, "thermal_pressure_state"),
		ThrottleActive:    getMetricFloat(m, "throttled") > 0,
	}
	if telemetry.ThermalState == "" {
		telemetry.ThermalState = "nominal"
	}

	// Optional nullable fields — only set when adapter provides them
	if hasMetric(m, "fan_speed_rpm") {
		v := getMetricFloat(m, "fan_speed_rpm")
		telemetry.FanRPM = &v
	}
	if hasMetric(m, "neural_engine_util_pct") {
		v := getMetricFloat(m, "neural_engine_util_pct")
		telemetry.NeuralEngineUtilPct = &v
	}

	var risk RiskSummary
	if len(scores) > 0 {
		sc := scores[0]
		risk = RiskSummary{
			CompositeScore: sc.Composite,
			Severity:       sc.Severity,
			Trend:          sc.Trend,
			TrendDelta:     sc.TrendDelta,
		}
	}

	resp := StatusResponse{
		Device: DeviceInfo{
			Hostname:      adapter.Hostname(),
			Adapter:       pt.AdapterName,
			Hardware:      hardware,
			BehaviorClass: behaviorClass,
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			UptimeSeconds: uptime,
		},
		Telemetry: telemetry,
		Risk:      risk,
		Agent: AgentInfo{
			Version:        s.version,
			PollIntervalS:  int(s.pollInterval.Seconds()),
			AdaptersActive: s.activeAdapters,
			CloudConnected: s.cloudConnected,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRisk(w http.ResponseWriter, r *http.Request) {
	batch, scores := s.stateHolder.Get()
	if len(batch) == 0 || len(scores) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, RiskResponse{Timestamp: time.Now().UTC().Format(time.RFC3339)})
		return
	}

	pt := batch[0]
	sc := scores[0]
	m := pt.Metrics
	if m == nil {
		m = make(map[string]float64)
	}

	model := deviceModelFromPoint(pt)
	spec := registry.Lookup(model)
	if sc.BehaviorClass != "" {
		spec.BehaviorClass = sc.BehaviorClass
	}

	thresholds, ok := scoring.SeverityThresholds[spec.BehaviorClass]
	if !ok {
		thresholds = scoring.SeverityThresholds["consumer_active_cooled"]
	}

	tCurrent := getMetricFloat(m, "temperature_c", "temperature_junction_c", "temperature_edge")
	powerW := getMetricFloat(m, "power_usage_w")
	utilPct := 0.0
	if spec.TDPW > 0 {
		utilPct = powerW / spec.TDPW * 100
	}
	headroomPct := 0.0
	if spec.ThermalLimitC > 0 && tCurrent >= 0 {
		headroomPct = (spec.ThermalLimitC - tCurrent) / spec.ThermalLimitC * 100
	}

	resp := RiskResponse{
		Timestamp: pt.Timestamp.UTC().Format(time.RFC3339),
		Composite: CompositeInfo{
			Score:      sc.Composite,
			Severity:   sc.Severity,
			Trend:      sc.Trend,
			TrendDelta: sc.TrendDelta,
		},
		SubScores: SubScores{
			Thermal: SubScoreDetail{
				Score:                sc.Thermal,
				Weight:               scoring.W_THERMAL,
				WeightedContribution: sc.Thermal * scoring.W_THERMAL,
				Details: map[string]interface{}{
					"current_temp_c":       tCurrent,
					"throttle_threshold_c": spec.ThermalLimitC,
					"roc_penalty":          sc.ThermalRoCPenalty,
					"headroom_pct":         headroomPct,
				},
			},
			Power: SubScoreDetail{
				Score:                sc.Power,
				Weight:               scoring.W_POWER,
				WeightedContribution: sc.Power * scoring.W_POWER,
				Details: map[string]interface{}{
					"current_power_w": powerW,
					"tdp_w":           spec.TDPW,
					"utilization_pct": utilPct,
				},
			},
			Volatility: SubScoreDetail{
				Score:                sc.Volatility,
				Weight:               scoring.W_VOLATILITY,
				WeightedContribution: sc.Volatility * scoring.W_VOLATILITY,
				Details: map[string]interface{}{
					"cv":             nil,
					"window_minutes": 60,
				},
			},
			Correlated: SubScoreDetail{
				Score:                sc.FleetPenalty,
				Weight:               0.20,
				WeightedContribution: sc.FleetPenalty,
				Details: map[string]interface{}{
					"note": "Single device mode — no zone correlation available",
				},
			},
		},
		Thresholds: Thresholds{
			Warning:  thresholds[0],
			Critical: thresholds[1],
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	note := "Process enumeration not available for this adapter"
	resp := ProcessResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Processes: []GPUProcess{},
		Supported: false,
		Note:      &note,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.hub.clientCount() >= maxWebSocketClients {
		http.Error(w, "too many WebSocket clients", http.StatusServiceUnavailable)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("WebSocket upgrade failed", "error", err)
		return
	}

	s.hub.add(conn)
	defer s.hub.remove(conn)

	// Send current state immediately
	batch, scores := s.stateHolder.Get()
	if len(batch) > 0 {
		msg := buildTelemetryUpdate(batch, scores)
		if data, err := json.Marshal(msg); err == nil {
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}

	// Read loop to detect disconnect
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func deviceModelFromPoint(pt normalizer.TelemetryPoint) string {
	if pt.Tags != nil {
		for _, k := range []string{"device_model", "gpu_model", "gpu_name", "model"} {
			if v, ok := pt.Tags[k]; ok && v != "" {
				return v
			}
		}
	}
	if pt.Metrics != nil {
		for _, k := range []string{"gpu_name", "model", "device_model"} {
			if v, ok := pt.Metrics[k]; ok {
				return strconv.FormatFloat(v, 'f', -1, 64)
			}
		}
	}
	return "unknown"
}
