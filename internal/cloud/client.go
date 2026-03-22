// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package cloud streams telemetry to the Keldron Cloud API via HTTPS/JSON.
package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
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
	MemoryTotalBytes     *float64 `json:"memory_total_bytes,omitempty"`
	CompositeRiskScore   float64  `json:"composite_risk_score"`
	ThermalSubScore      *float64 `json:"thermal_sub_score"`
	PowerSubScore        *float64 `json:"power_sub_score"`
	VolatilitySubScore   *float64 `json:"volatility_sub_score"`
	MemorySubScore       *float64 `json:"memory_sub_score,omitempty"`
	TDPW                 *float64 `json:"tdp_w,omitempty"`
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

// isTransientStatus returns true for HTTP status codes that warrant a retry.
func isTransientStatus(code int) bool {
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500
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

	// Compute a deterministic idempotency key from sample content so re-buffered
	// retries send the same key, enabling server-side deduplication.
	batchID := batchIDFromSamples(pending)

	body, err := json.Marshal(IngestRequest{Samples: pending})
	if err != nil {
		c.bufferMu.Lock()
		c.buffer = c.trimBuffer(append(pending, c.buffer...))
		c.bufferMu.Unlock()
		return fmt.Errorf("marshal ingest body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ingestURL, bytes.NewReader(body))
	if err != nil {
		c.bufferMu.Lock()
		c.buffer = c.trimBuffer(append(pending, c.buffer...))
		c.bufferMu.Unlock()
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", batchID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.bufferMu.Lock()
		c.buffer = c.trimBuffer(append(pending, c.buffer...))
		c.bufferMu.Unlock()
		c.logger.Warn("cloud ingest request failed (buffered for retry)",
			"error", err, "batch_id", batchID, "agent_id", c.agentID)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusAccepted {
		if isTransientStatus(resp.StatusCode) {
			c.bufferMu.Lock()
			c.buffer = c.trimBuffer(append(pending, c.buffer...))
			c.bufferMu.Unlock()
			c.logger.Warn("cloud ingest transient error (buffered for retry)",
				"status", resp.StatusCode,
				"body", string(respBody),
				"batch_id", batchID,
				"agent_id", c.agentID,
			)
		} else {
			c.logger.Error("cloud ingest permanent error (samples dropped)",
				"status", resp.StatusCode,
				"body", string(respBody),
				"batch_id", batchID,
				"samples", len(pending),
				"agent_id", c.agentID,
			)
		}
		return fmt.Errorf("cloud ingest: status %d", resp.StatusCode)
	}

	var ing IngestResponse
	if err := json.Unmarshal(respBody, &ing); err != nil {
		c.logger.Warn("cloud ingest: decode response body", "error", err, "batch_id", batchID)
	} else if ing.Rejected > 0 {
		c.logger.Warn("cloud ingest partially rejected",
			"accepted", ing.Accepted,
			"rejected", ing.Rejected,
			"errors", ing.Errors,
			"batch_id", batchID,
			"agent_id", c.agentID,
		)
	} else {
		c.logger.Info("cloud ingest accepted",
			"accepted", ing.Accepted,
			"batch_id", batchID,
			"agent_id", c.agentID,
		)
	}

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

// batchIDFromSamples computes a deterministic idempotency key from sample
// identifying fields (device_id + timestamp), so the same set of samples
// always produces the same key across retries.
func batchIDFromSamples(samples []Sample) string {
	keys := make([]string, len(samples))
	for i, s := range samples {
		keys[i] = s.DeviceID + "|" + s.Timestamp
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// TrackSend increments the in-flight send counter. Call before spawning a Send goroutine.
func (c *Client) TrackSend() {
	if c == nil {
		return
	}
	c.wg.Add(1)
}

// SendDone decrements the in-flight send counter. Call when a Send goroutine completes.
func (c *Client) SendDone() {
	if c == nil {
		return
	}
	c.wg.Done()
}

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
