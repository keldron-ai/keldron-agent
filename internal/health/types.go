// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package health

import "time"

// Response is the JSON payload returned by GET /health.
type Response struct {
	Status    Status    `json:"status"` // healthy | degraded | unhealthy
	AgentID   string    `json:"agent_id"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"` // Human-readable: "2h34m"
	StartedAt time.Time `json:"started_at"`
	Timestamp time.Time `json:"timestamp"` // When this response was generated

	Adapters   map[string]AdapterStatus `json:"adapters"`
	Normalizer NormalizerStatus         `json:"normalizer"`
	Buffer     BufferStatus             `json:"buffer"`
	Sender     SenderStatus             `json:"sender"`
	Config     ConfigStatus             `json:"config"`
}

// Status is the overall agent health status.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"  // Some components have issues but agent is functional
	StatusUnhealthy Status = "unhealthy" // Critical failure — agent not collecting or sending
)

// AdapterStatus holds per-adapter health information.
type AdapterStatus struct {
	Name        string    `json:"name"`
	Enabled     bool      `json:"enabled"`
	Running     bool      `json:"running"`
	LastPoll    time.Time `json:"last_poll,omitempty"`
	PollCount   uint64    `json:"poll_count"`
	ErrorCount  uint64    `json:"error_count"`
	LastError   string    `json:"last_error,omitempty"`
	LastErrorAt time.Time `json:"last_error_at,omitempty"`
}

// NormalizerStatus holds normalizer health information.
type NormalizerStatus struct {
	Running     bool   `json:"running"`
	Processed   uint64 `json:"processed"`
	Rejected    uint64 `json:"rejected"`
	InputQueues int    `json:"input_queues"`
}

// BufferStatus holds ring buffer and WAL health information.
type BufferStatus struct {
	RingCapacity int     `json:"ring_capacity"`
	RingUsed     int     `json:"ring_used"`
	RingPercent  float64 `json:"ring_percent"` // 0-100
	WALEnabled   bool    `json:"wal_enabled"`
	WALSegments  int     `json:"wal_segments"`
	WALSizeBytes int64   `json:"wal_size_bytes"`
	WALMaxBytes  int64   `json:"wal_max_bytes"` // 0 if unknown
	WALPoints    uint64  `json:"wal_points"`
	Draining     bool    `json:"draining"` // true if WAL is currently draining
}

// SenderStatus holds gRPC sender health information.
type SenderStatus struct {
	Connected   bool      `json:"connected"`
	Target      string    `json:"target"`
	BatchesSent uint64    `json:"batches_sent"`
	PointsSent  uint64    `json:"points_sent"`
	Errors      uint64    `json:"errors"`
	LastSendAt  time.Time `json:"last_send_at,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	SeqNumber   uint64    `json:"sequence_number"`
}

// ConfigStatus holds config watcher health information.
type ConfigStatus struct {
	Path         string    `json:"path"`
	LastReloadAt time.Time `json:"last_reload_at,omitempty"`
	ReloadCount  uint64    `json:"reload_count"`
	LastError    string    `json:"last_error,omitempty"`
}
