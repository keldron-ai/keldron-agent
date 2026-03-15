// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package health exposes an HTTP endpoint for agent observability and k8s probes.
package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// ComponentProvider is the interface each agent component exposes for health reporting.
// Components implement the subset relevant to them via the typed provider interfaces below.

// AdapterProvider is implemented by adapters for health reporting.
type AdapterProvider interface {
	Name() string
	IsRunning() bool
	Stats() (pollCount, errorCount uint64, lastPoll time.Time, lastError string, lastErrorAt time.Time)
}

// NormalizerProvider is implemented by the normalizer for health reporting.
type NormalizerProvider interface {
	Stats() (processed, rejected uint64)
	InputCount() int
	IsRunning() bool
}

// BufferProvider is implemented by the buffer manager for health reporting.
type BufferProvider interface {
	RingStats() (capacity, used int)
	WALStats() (totalSize, maxSize int64, segments int, points uint64, draining bool)
}

// SenderProvider is implemented by the sender for health reporting.
type SenderProvider interface {
	Stats() (batchesSent, pointsSent, errors uint64)
	IsConnected() bool
	LastSendAt() time.Time
	SeqNumber() uint64
	LastError() string
	Target() string
}

// ConfigProvider is implemented by the config watcher for health reporting.
type ConfigProvider interface {
	ReloadStats() (count uint64, lastReloadAt time.Time, lastError string)
	Path() string
}

// Server is the health HTTP server.
type Server struct {
	bind       string
	agentID    string
	version    string
	startedAt  time.Time
	logger     *slog.Logger
	httpServer *http.Server

	adapters   []AdapterProvider
	normalizer NormalizerProvider
	buffer     BufferProvider
	sender     SenderProvider
	config     ConfigProvider

	// enabledAdapters maps adapter name -> enabled for config-aware status
	enabledAdapters map[string]bool
	adaptersMu      sync.RWMutex

	// localMode when true, ready/healthy does not require sender connection
	localMode bool
	localMu   sync.RWMutex
}

// New creates a health Server. Call Register* to wire components, then Start.
func New(bind, agentID, version string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		bind:            bind,
		agentID:         agentID,
		version:         version,
		startedAt:       time.Now(),
		logger:          logger,
		adapters:        nil,
		enabledAdapters: make(map[string]bool),
	}
}

// RegisterAdapter registers an adapter for health reporting.
func (s *Server) RegisterAdapter(p AdapterProvider) {
	s.adaptersMu.Lock()
	defer s.adaptersMu.Unlock()
	s.adapters = append(s.adapters, p)
}

// SetLocalMode sets whether the agent runs in local-only mode (no cloud sender).
// When true, isReady and determineStatus do not require sender connection.
func (s *Server) SetLocalMode(local bool) {
	s.localMu.Lock()
	defer s.localMu.Unlock()
	s.localMode = local
}

// SetEnabledAdapters sets which adapters are enabled (from config). Used for status determination.
func (s *Server) SetEnabledAdapters(names map[string]bool) {
	s.adaptersMu.Lock()
	defer s.adaptersMu.Unlock()
	s.enabledAdapters = make(map[string]bool)
	for k, v := range names {
		s.enabledAdapters[k] = v
	}
}

// RegisterNormalizer registers the normalizer for health reporting.
func (s *Server) RegisterNormalizer(p NormalizerProvider) {
	s.normalizer = p
}

// RegisterBuffer registers the buffer manager for health reporting.
func (s *Server) RegisterBuffer(p BufferProvider) {
	s.buffer = p
}

// RegisterSender registers the sender for health reporting.
func (s *Server) RegisterSender(p SenderProvider) {
	s.sender = p
}

// RegisterConfig registers the config watcher for health reporting.
func (s *Server) RegisterConfig(p ConfigProvider) {
	s.config = p
}

// Handler returns the HTTP handler for testing. Used by Start as well.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)
	return mux
}

// Start begins serving. Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	s.httpServer = &http.Server{Addr: s.bind, Handler: s.Handler()}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}()

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := s.collectHealth()
	w.Header().Set("Content-Type", "application/json")
	if resp.Status == StatusUnhealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ready := s.isReady()
	w.Header().Set("Content-Type", "application/json")
	if ready {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ready": ready})
}

func (s *Server) isReady() bool {
	s.adaptersMu.RLock()
	adapters := append([]AdapterProvider(nil), s.adapters...)
	s.adaptersMu.RUnlock()

	anyAdapterRunning := false
	for _, a := range adapters {
		if a != nil && a.IsRunning() {
			anyAdapterRunning = true
			break
		}
	}
	if !anyAdapterRunning {
		return false
	}
	s.localMu.RLock()
	local := s.localMode
	s.localMu.RUnlock()
	if local {
		return true
	}
	if s.sender != nil && s.sender.IsConnected() {
		return true
	}
	return false
}

func (s *Server) collectHealth() *Response {
	resp := &Response{
		Status:    StatusHealthy,
		AgentID:   s.agentID,
		Version:   s.version,
		Uptime:    time.Since(s.startedAt).Round(time.Second).String(),
		StartedAt: s.startedAt,
		Timestamp: time.Now(),
		Adapters:  make(map[string]AdapterStatus),
	}

	s.adaptersMu.RLock()
	adapters := append([]AdapterProvider(nil), s.adapters...)
	enabled := make(map[string]bool)
	for k, v := range s.enabledAdapters {
		enabled[k] = v
	}
	s.adaptersMu.RUnlock()

	for _, a := range adapters {
		if a == nil {
			continue
		}
		name := a.Name()
		pollCount, errorCount, lastPoll, lastError, lastErrorAt := a.Stats()
		isEnabled, ok := enabled[name]
		if !ok {
			isEnabled = true // default to enabled if adapter not in config map
		}
		resp.Adapters[name] = AdapterStatus{
			Name:        name,
			Enabled:     isEnabled,
			Running:     a.IsRunning(),
			LastPoll:    lastPoll,
			PollCount:   pollCount,
			ErrorCount:  errorCount,
			LastError:   lastError,
			LastErrorAt: lastErrorAt,
		}
	}

	if s.normalizer != nil {
		processed, rejected := s.normalizer.Stats()
		resp.Normalizer = NormalizerStatus{
			Running:     s.normalizer.IsRunning(),
			Processed:   processed,
			Rejected:    rejected,
			InputQueues: s.normalizer.InputCount(),
		}
	}

	if s.buffer != nil {
		capacity, used := s.buffer.RingStats()
		totalSize, maxSize, segments, points, draining := s.buffer.WALStats()
		percent := 0.0
		if capacity > 0 {
			percent = float64(used) / float64(capacity) * 100
		}
		walEnabled := maxSize > 0 || segments > 0 || totalSize > 0 || points > 0
		resp.Buffer = BufferStatus{
			RingCapacity: capacity,
			RingUsed:     used,
			RingPercent:  percent,
			WALEnabled:   walEnabled,
			WALSegments:  segments,
			WALSizeBytes: totalSize,
			WALMaxBytes:  maxSize,
			WALPoints:    points,
			Draining:     draining,
		}
	}

	if s.sender != nil {
		batchesSent, pointsSent, errs := s.sender.Stats()
		resp.Sender = SenderStatus{
			Connected:   s.sender.IsConnected(),
			Target:      s.sender.Target(),
			BatchesSent: batchesSent,
			PointsSent:  pointsSent,
			Errors:      errs,
			LastSendAt:  s.sender.LastSendAt(),
			LastError:   s.sender.LastError(),
			SeqNumber:   s.sender.SeqNumber(),
		}
	}

	if s.config != nil {
		count, lastReloadAt, lastError := s.config.ReloadStats()
		resp.Config = ConfigStatus{
			Path:         s.config.Path(),
			LastReloadAt: lastReloadAt,
			ReloadCount:  count,
			LastError:    lastError,
		}
	}

	resp.Status = s.determineStatus(resp)
	return resp
}

func (s *Server) determineStatus(resp *Response) Status {
	// UNHEALTHY if: no adapters running, OR sender disconnected AND WAL > 80% full
	anyAdapterRunning := false
	for _, a := range resp.Adapters {
		if a.Enabled && a.Running {
			anyAdapterRunning = true
			break
		}
	}
	if !anyAdapterRunning {
		return StatusUnhealthy
	}

	s.localMu.RLock()
	local := s.localMode
	s.localMu.RUnlock()
	if local {
		for _, a := range resp.Adapters {
			if a.Enabled && !a.LastErrorAt.IsZero() && a.LastErrorAt.After(time.Now().Add(-5*time.Minute)) {
				return StatusDegraded
			}
		}
		return StatusHealthy
	}

	senderDisconnected := s.sender != nil && !resp.Sender.Connected
	walPercent := 0.0
	if resp.Buffer.WALMaxBytes > 0 {
		walPercent = float64(resp.Buffer.WALSizeBytes) / float64(resp.Buffer.WALMaxBytes) * 100
	}
	if senderDisconnected && (resp.Buffer.RingPercent >= 80 || walPercent >= 80) {
		return StatusUnhealthy
	}
	if senderDisconnected {
		return StatusDegraded
	}

	// DEGRADED: adapter errors in last 5 min, ring > 80% full, WAL draining
	for _, a := range resp.Adapters {
		if a.Enabled && !a.LastErrorAt.IsZero() && a.LastErrorAt.After(time.Now().Add(-5*time.Minute)) {
			return StatusDegraded
		}
	}
	if resp.Buffer.RingPercent >= 80 {
		return StatusDegraded
	}
	if resp.Buffer.Draining {
		return StatusDegraded
	}

	return StatusHealthy
}
