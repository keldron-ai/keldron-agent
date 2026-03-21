// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package cloud streams telemetry to the Keldron Cloud API via HTTPS/JSON.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultMaxBuffer = 1000

// Client streams telemetry to the Keldron Cloud API via HTTPS/JSON.
type Client struct {
	endpoint   string
	ingestURL  string
	apiKey     string
	httpClient *http.Client
	agentID    string
	version    string
	buffer     []Sample
	bufferMu   sync.Mutex
	MaxBuffer  int
	logger     *slog.Logger
	wg         sync.WaitGroup // tracks in-flight Send goroutines
}

// Sample is the JSON payload format matching the cloud ingest API.
type Sample struct {
	DeviceID             string   `json:"device_id"`
	Hostname             string   `json:"hostname,omitempty"`
	AdapterType          string   `json:"adapter_type"`
	HardwareModel        string   `json:"hardware_model,omitempty"`
	Timestamp            string   `json:"timestamp"`
	TemperaturePrimary   *float64 `json:"temperature_primary"`
	TemperatureSecondary *float64 `json:"temperature_secondary"`
	PowerDraw            *float64 `json:"power_draw"`
	Utilization          *float64 `json:"utilization"`
	FanSpeed             *float64 `json:"fan_speed"`
	ClockSpeed           *float64 `json:"clock_speed"`
	MemoryUsed           *float64 `json:"memory_used"`
	CompositeRiskScore   float64  `json:"composite_risk_score"`
	ThermalSubScore      *float64 `json:"thermal_sub_score"`
	PowerSubScore        *float64 `json:"power_sub_score"`
	VolatilitySubScore   *float64 `json:"volatility_sub_score"`
	SeverityBand         string   `json:"severity_band"`
	StressState          *string  `json:"stress_state"`
	StressInstanceID     *string  `json:"stress_instance_id"`
	TransitionEvent      *string  `json:"transition_event"`
	AgentVersion         *string  `json:"agent_version,omitempty"`
}

// IngestRequest is the POST body for /v1/telemetry/ingest.
type IngestRequest struct {
	Samples []Sample `json:"samples"`
}

// IngestResponse is the JSON body on 202 Accepted.
type IngestResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors"`
}

// NewClient returns a cloud ingest client. endpoint should be the API base (e.g. https://api.keldron.ai).
func NewClient(endpoint, apiKey, agentID, version string) *Client {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	ingestURL := endpoint + "/v1/telemetry/ingest"
	return &Client{
		endpoint:  endpoint,
		ingestURL: ingestURL,
		apiKey:    apiKey,
		agentID:   agentID,
		version:   version,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		MaxBuffer: defaultMaxBuffer,
		logger:    slog.Default(),
	}
}

// Send POSTs samples to the ingest endpoint. On failure, payloads are retained in an in-memory FIFO buffer
// (up to MaxBuffer); oldest samples are dropped when full. Thread-safe.
func (c *Client) Send(ctx context.Context, samples []Sample) error {
	if c == nil {
		return nil
	}

	// Snapshot pending samples under lock, then release before network I/O.
	c.bufferMu.Lock()
	pending := make([]Sample, 0, len(c.buffer)+len(samples))
	pending = append(pending, c.buffer...)
	pending = append(pending, samples...)
	c.buffer = nil
	c.bufferMu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	body, err := json.Marshal(IngestRequest{Samples: pending})
	if err != nil {
		c.bufferMu.Lock()
		c.buffer = c.trimBuffer(append(c.buffer, pending...))
		c.bufferMu.Unlock()
		return fmt.Errorf("marshal ingest body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ingestURL, bytes.NewReader(body))
	if err != nil {
		c.bufferMu.Lock()
		c.buffer = c.trimBuffer(append(c.buffer, pending...))
		c.bufferMu.Unlock()
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.bufferMu.Lock()
		c.buffer = c.trimBuffer(append(c.buffer, pending...))
		c.bufferMu.Unlock()
		c.logger.Warn("cloud ingest request failed (buffered for retry)", "error", err, "agent_id", c.agentID)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusAccepted {
		c.bufferMu.Lock()
		c.buffer = c.trimBuffer(append(c.buffer, pending...))
		c.bufferMu.Unlock()
		c.logger.Warn("cloud ingest non-202 (buffered for retry)",
			"status", resp.StatusCode,
			"body", string(respBody),
			"agent_id", c.agentID,
		)
		return fmt.Errorf("cloud ingest: status %d", resp.StatusCode)
	}

	var ing IngestResponse
	if err := json.Unmarshal(respBody, &ing); err != nil {
		c.logger.Warn("cloud ingest: decode response body", "error", err)
	} else {
		c.logger.Info("cloud ingest accepted",
			"accepted", ing.Accepted,
			"rejected", ing.Rejected,
			"agent_id", c.agentID,
		)
	}

	// Success — buffer already cleared above (set to nil).
	return nil
}

func (c *Client) trimBuffer(merged []Sample) []Sample {
	max := c.MaxBuffer
	if max <= 0 {
		max = defaultMaxBuffer
	}
	if len(merged) <= max {
		out := make([]Sample, len(merged))
		copy(out, merged)
		return out
	}
	dropped := len(merged) - max
	c.logger.Warn("cloud ingest buffer overflow, dropping oldest samples",
		"dropped", dropped,
		"max_buffer", max,
		"agent_id", c.agentID,
	)
	out := make([]Sample, max)
	copy(out, merged[dropped:])
	return out
}

// TrackSend increments the in-flight send counter. Call before spawning a Send goroutine.
func (c *Client) TrackSend() { c.wg.Add(1) }

// SendDone decrements the in-flight send counter. Call when a Send goroutine completes.
func (c *Client) SendDone() { c.wg.Done() }

// Close waits for in-flight Send goroutines, then flushes any remaining buffered samples.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}

	// Wait for all in-flight sends to finish.
	c.wg.Wait()

	// Flush any remaining buffered samples with a generous timeout.
	c.bufferMu.Lock()
	remaining := c.buffer
	c.buffer = nil
	c.bufferMu.Unlock()

	if len(remaining) == 0 {
		return nil
	}

	c.logger.Info("cloud client flushing remaining samples on close", "count", len(remaining))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return c.Send(ctx, remaining)
}
