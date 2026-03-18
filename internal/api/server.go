// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
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
	httpServer     *http.Server
	version        string
	pollInterval   time.Duration
	activeAdapters []string
	cloudConnected bool
}

// NewServer creates a new API server.
func NewServer(holder *StateHolder, version string, pollInterval time.Duration, activeAdapters []string, cloudConnected bool) *Server {
	hub := newWSHub()
	holder.SetBroadcastTarget(hub)

	mux := http.NewServeMux()

	s := &Server{
		stateHolder:    holder,
		hub:            hub,
		httpServer:     &http.Server{Handler: corsMiddleware(mux)},
		version:        version,
		pollInterval:   pollInterval,
		activeAdapters: activeAdapters,
		cloudConnected: cloudConnected,
	}

	mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	mux.HandleFunc("GET /api/v1/risk", s.handleRisk)
	mux.HandleFunc("GET /api/v1/processes", s.handleProcesses)
	mux.HandleFunc("GET /ws/telemetry", s.handleWebSocket)
	mux.HandleFunc("GET /", HandleFrontend)

	return s
}

// Start starts the HTTP server. Blocks until the server stops.
func (s *Server) Start(addr string) error {
	slog.Info("API server starting", "addr", addr)
	s.httpServer.Addr = addr
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server and closes WebSocket connections.
func (s *Server) Shutdown(ctx context.Context) error {
	s.hub.closeAll()
	return s.httpServer.Shutdown(ctx)
}

// defaultCORSOrigins are the origins allowed by default when no explicit
// allowlist is configured, covering common local development URLs.
var defaultCORSOrigins = []string{
	"http://localhost",
	"http://127.0.0.1",
	"http://[::1]",
}

// corsAllowedOrigins returns the configured CORS origins from CORS_ALLOWED_ORIGINS env.
// If unset, returns a localhost-only default for safe local development.
func corsAllowedOrigins() []string {
	v := os.Getenv("CORS_ALLOWED_ORIGINS")
	if v == "" {
		return defaultCORSOrigins
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := corsAllowedOrigins()
			for _, a := range allowed {
				if strings.EqualFold(origin, a) || strings.HasPrefix(strings.ToLower(origin), strings.ToLower(a)+":") {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		}
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

	pt := latestPoint(batch)
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
	if sc, ok := matchScore(pt, scores); ok {
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

	pt := latestPoint(batch)
	sc, ok := matchScore(pt, scores)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, RiskResponse{Timestamp: pt.Timestamp.UTC().Format(time.RFC3339)})
		return
	}
	m := pt.Metrics
	if m == nil {
		m = make(map[string]float64)
	}

	model := deviceModelFromPoint(pt)
	spec := registry.Lookup(registry.NormalizeModelName(model))
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
				Weight:               scoring.W_CORRELATED,
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
	note := "Process enumeration not yet implemented"
	resp := ProcessResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Processes: []GPUProcess{},
		Supported: false,
		Note:      &note,
	}
	writeJSON(w, http.StatusNotImplemented, resp)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("WebSocket upgrade failed", "error", err)
		return
	}

	client := s.hub.tryAdd(conn)
	if client == nil {
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "too many WebSocket clients"))
		_ = conn.Close()
		return
	}
	defer s.hub.removeClient(client)

	// Send current state immediately
	batch, scores := s.stateHolder.Get()
	if len(batch) > 0 {
		msg := buildTelemetryUpdate(batch, scores)
		if data, err := json.Marshal(msg); err == nil {
			client.writeMu.Lock()
			_ = conn.WriteMessage(websocket.TextMessage, data)
			client.writeMu.Unlock()
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
	return "unknown"
}

// latestPoint returns the point with the most recent timestamp from batch.
func latestPoint(batch []normalizer.TelemetryPoint) normalizer.TelemetryPoint {
	best := batch[0]
	for _, pt := range batch[1:] {
		if pt.Timestamp.After(best.Timestamp) {
			best = pt
		}
	}
	return best
}

// deviceIDFromPoint mirrors the scoring engine's device ID derivation.
func deviceIDFromPoint(pt normalizer.TelemetryPoint) string {
	if pt.Metrics != nil {
		if gpuID, ok := pt.Metrics["gpu_id"]; ok {
			return pt.Source + ":" + strconv.FormatFloat(gpuID, 'f', 0, 64)
		}
	}
	return pt.Source
}

// matchScore finds the score matching the given point's device ID, or returns
// a zero-value score and false.
func matchScore(pt normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) (scoring.RiskScoreOutput, bool) {
	did := deviceIDFromPoint(pt)
	for _, sc := range scores {
		if sc.DeviceID == did {
			return sc, true
		}
	}
	return scoring.RiskScoreOutput{}, false
}
