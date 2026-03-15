// Package health_test tests the health HTTP server including:
// - Status determination (healthy, degraded, unhealthy)
// - JSON response structure and round-trip
// - HTTP status codes for /health and /ready
// - Server lifecycle
package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/health"
)

// stubAdapter implements AdapterProvider for testing.
type stubAdapter struct {
	name        string
	enabled     bool
	running     bool
	pollCount   uint64
	errorCount  uint64
	lastPoll    time.Time
	lastError   string
	lastErrorAt time.Time
}

func (s *stubAdapter) Name() string { return s.name }
func (s *stubAdapter) IsRunning() bool { return s.running }
func (s *stubAdapter) Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time) {
	return s.pollCount, s.errorCount, s.lastPoll, s.lastError, s.lastErrorAt
}

// stubNormalizer implements NormalizerProvider for testing.
type stubNormalizer struct {
	running     bool
	processed   uint64
	rejected    uint64
	inputCount  int
}

func (s *stubNormalizer) Stats() (processed, rejected uint64) { return s.processed, s.rejected }
func (s *stubNormalizer) InputCount() int { return s.inputCount }
func (s *stubNormalizer) IsRunning() bool { return s.running }

// stubBuffer implements BufferProvider for testing.
type stubBuffer struct {
	ringCap, ringUsed     int
	walSize, walMax       int64
	walSegments           int
	walPoints             uint64
	draining              bool
}

func (s *stubBuffer) RingStats() (capacity, used int) { return s.ringCap, s.ringUsed }
func (s *stubBuffer) WALStats() (totalSize, maxSize int64, segments int, points uint64, draining bool) {
	return s.walSize, s.walMax, s.walSegments, s.walPoints, s.draining
}

// stubSender implements SenderProvider for testing.
type stubSender struct {
	connected    bool
	target      string
	batchesSent uint64
	pointsSent  uint64
	errors      uint64
	lastSendAt  time.Time
	lastError   string
	seqNumber   uint64
}

func (s *stubSender) Stats() (batchesSent, pointsSent, errors uint64) {
	return s.batchesSent, s.pointsSent, s.errors
}
func (s *stubSender) IsConnected() bool { return s.connected }
func (s *stubSender) LastSendAt() time.Time { return s.lastSendAt }
func (s *stubSender) SeqNumber() uint64 { return s.seqNumber }
func (s *stubSender) LastError() string { return s.lastError }
func (s *stubSender) Target() string { return s.target }

// stubConfig implements ConfigProvider for testing.
type stubConfig struct {
	path         string
	reloadCount  uint64
	lastReloadAt time.Time
	lastError    string
}

func (s *stubConfig) ReloadStats() (count uint64, lastReloadAt time.Time, lastError string) {
	return s.reloadCount, s.lastReloadAt, s.lastError
}
func (s *stubConfig) Path() string { return s.path }

func TestHandleHealth_Healthy(t *testing.T) {
	srv := health.New(":0", "agent-1", "0.1.0", nil)
	srv.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true, pollCount: 100})
	srv.RegisterNormalizer(&stubNormalizer{running: true, processed: 500, inputCount: 1})
	srv.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 10, walMax: 1e9})
	srv.RegisterSender(&stubSender{connected: true, target: "platform:443"})
	srv.RegisterConfig(&stubConfig{path: "/etc/config.yaml"})
	// Adapter in enabled map -> healthy when running
	srv.SetEnabledAdapters(map[string]bool{"dcgm": true})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != health.StatusHealthy {
		t.Errorf("status = %q, want %q", resp.Status, health.StatusHealthy)
	}
}

func TestHandleHealth_Degraded(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*health.Server)
	}{
		{
			name: "sender disconnected but WAL has capacity",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true})
				s.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
				s.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 10, walMax: 1e9, walSize: 1e6})
				s.RegisterSender(&stubSender{connected: false, target: "platform:443"})
				s.RegisterConfig(&stubConfig{})
				s.SetEnabledAdapters(map[string]bool{"dcgm": true})
			},
		},
		{
			name: "ring buffer > 80% full",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true})
				s.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
				s.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 850, walMax: 1e9})
				s.RegisterSender(&stubSender{connected: true, target: "platform:443"})
				s.RegisterConfig(&stubConfig{})
				s.SetEnabledAdapters(map[string]bool{"dcgm": true})
			},
		},
		{
			name: "WAL draining",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true})
				s.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
				s.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 0, draining: true, walMax: 1e9})
				s.RegisterSender(&stubSender{connected: true, target: "platform:443"})
				s.RegisterConfig(&stubConfig{})
				s.SetEnabledAdapters(map[string]bool{"dcgm": true})
			},
		},
		{
			name: "adapter errors in last 5 minutes",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{
					name: "dcgm", enabled: true, running: true,
					errorCount: 1, lastError: "collect failed", lastErrorAt: time.Now().Add(-1 * time.Minute),
				})
				s.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
				s.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 10, walMax: 1e9})
				s.RegisterSender(&stubSender{connected: true, target: "platform:443"})
				s.RegisterConfig(&stubConfig{})
				s.SetEnabledAdapters(map[string]bool{"dcgm": true})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := health.New(":0", "agent-1", "0.1.0", nil)
			tt.setup(srv)

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 (degraded returns 200)", rec.Code)
			}
			var resp health.Response
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status != health.StatusDegraded {
				t.Errorf("status = %q, want %q", resp.Status, health.StatusDegraded)
			}
		})
	}
}

func TestHandleHealth_Unhealthy(t *testing.T) {
	tests := []struct {
		name string
		setup func(*health.Server)
	}{
		{
			name: "no adapters running",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: false})
				s.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
				s.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 0, walMax: 1e9})
				s.RegisterSender(&stubSender{connected: true, target: "platform:443"})
				s.RegisterConfig(&stubConfig{})
				s.SetEnabledAdapters(map[string]bool{"dcgm": true})
			},
		},
		{
			name: "sender disconnected and WAL > 80% full",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true})
				s.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
				s.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 0, walMax: 100, walSize: 90, walPoints: 1})
				s.RegisterSender(&stubSender{connected: false, target: "platform:443"})
				s.RegisterConfig(&stubConfig{})
				s.SetEnabledAdapters(map[string]bool{"dcgm": true})
			},
		},
		{
			name: "sender disconnected and ring > 80% full",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true})
				s.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
				s.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 850, walMax: 1e9})
				s.RegisterSender(&stubSender{connected: false, target: "platform:443"})
				s.RegisterConfig(&stubConfig{})
				s.SetEnabledAdapters(map[string]bool{"dcgm": true})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := health.New(":0", "agent-1", "0.1.0", nil)
			tt.setup(srv)

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", rec.Code)
			}
			var resp health.Response
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status != health.StatusUnhealthy {
				t.Errorf("status = %q, want %q", resp.Status, health.StatusUnhealthy)
			}
		})
	}
}

func TestHandleHealth_JSONRoundTrip(t *testing.T) {
	srv := health.New(":0", "agent-1", "0.1.0", nil)
	srv.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true, pollCount: 42})
	srv.RegisterNormalizer(&stubNormalizer{running: true, processed: 100, rejected: 2, inputCount: 1})
	srv.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 50, walMax: 1e9})
	srv.RegisterSender(&stubSender{connected: true, target: "platform:443", seqNumber: 10})
	srv.RegisterConfig(&stubConfig{path: "/etc/config.yaml", reloadCount: 1})
	srv.SetEnabledAdapters(map[string]bool{"dcgm": true})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Round-trip through JSON
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var resp2 health.Response
	if err := json.Unmarshal(data, &resp2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp2.AgentID != "agent-1" {
		t.Errorf("agent_id = %q, want agent-1", resp2.AgentID)
	}
	if resp2.Adapters["dcgm"].PollCount != 42 {
		t.Errorf("adapter poll_count = %d, want 42", resp2.Adapters["dcgm"].PollCount)
	}
}

func TestHandleHealth_AdapterNotInEnabledMapDefaultsToEnabled(t *testing.T) {
	// Bug 1: adapter not in enabled map should default to enabled (not false)
	srv := health.New(":0", "agent-1", "0.1.0", nil)
	srv.RegisterAdapter(&stubAdapter{name: "dcgm", running: true})
	srv.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
	srv.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 10, walMax: 1e9})
	srv.RegisterSender(&stubSender{connected: true, target: "platform:443"})
	srv.RegisterConfig(&stubConfig{})
	// Do NOT call SetEnabledAdapters - adapter name absent from map
	// Should default to enabled and report healthy
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Adapters["dcgm"].Enabled {
		t.Error("adapter not in enabled map should default to enabled=true")
	}
	if resp.Status != health.StatusHealthy {
		t.Errorf("status = %q, want healthy when adapter defaults to enabled", resp.Status)
	}
}

func TestHandleHealth_DegradedClearsAfter5Minutes(t *testing.T) {
	// Bug 3: lastErrorAt older than 5 min should not cause degraded
	srv := health.New(":0", "agent-1", "0.1.0", nil)
	srv.RegisterAdapter(&stubAdapter{
		name: "dcgm", enabled: true, running: true,
		errorCount: 5, lastError: "old error", lastErrorAt: time.Now().Add(-10 * time.Minute),
	})
	srv.RegisterNormalizer(&stubNormalizer{running: true, inputCount: 1})
	srv.RegisterBuffer(&stubBuffer{ringCap: 1000, ringUsed: 10, walMax: 1e9})
	srv.RegisterSender(&stubSender{connected: true, target: "platform:443"})
	srv.RegisterConfig(&stubConfig{})
	srv.SetEnabledAdapters(map[string]bool{"dcgm": true})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != health.StatusHealthy {
		t.Errorf("status = %q, want healthy when lastErrorAt is >5 min ago", resp.Status)
	}
}

func TestHandleReady(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*health.Server)
		ready  bool
		status int
	}{
		{
			name: "ready when adapter running and sender connected",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true})
				s.RegisterSender(&stubSender{connected: true})
			},
			ready:  true,
			status: http.StatusOK,
		},
		{
			name: "not ready when no adapters running",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: false})
				s.RegisterSender(&stubSender{connected: true})
			},
			ready:  false,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "not ready when sender disconnected",
			setup: func(s *health.Server) {
				s.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true})
				s.RegisterSender(&stubSender{connected: false})
			},
			ready:  false,
			status: http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := health.New(":0", "agent-1", "0.1.0", nil)
			tt.setup(srv)

			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			var body struct {
				Ready bool `json:"ready"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Ready != tt.ready {
				t.Errorf("ready = %v, want %v", body.Ready, tt.ready)
			}
		})
	}
}

func TestServer_StartStop(t *testing.T) {
	srv := health.New(":0", "agent-1", "0.1.0", nil)
	srv.RegisterAdapter(&stubAdapter{name: "dcgm", enabled: true, running: true})
	srv.RegisterSender(&stubSender{connected: true})
	srv.SetEnabledAdapters(map[string]bool{"dcgm": true})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Health endpoint should respond
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d", resp.StatusCode)
	}

	// Ready endpoint should respond
	resp, err = http.Get(ts.URL + "/ready")
	if err != nil {
		t.Fatalf("GET /ready: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("ready status = %d", resp.StatusCode)
	}
}
