// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package sender provides local and gRPC senders for telemetry.
// LocalSender drains the input channel and discards points (no cloud streaming).
package sender

import (
	"context"
	"log/slog"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

// LocalSender drains TelemetryPoints from the input channel and discards them.
// It implements the same interface as the gRPC Sender for health reporting.
type LocalSender struct {
	agentID      string
	input        <-chan normalizer.TelemetryPoint
	logger       *slog.Logger
	onConnChange func(bool)

	pointsDrained uint64
}

// NewLocal creates a LocalSender that discards all telemetry (no cloud streaming).
func NewLocal(agentID string, input <-chan normalizer.TelemetryPoint, logger *slog.Logger) *LocalSender {
	return &LocalSender{
		agentID: agentID,
		input:   input,
		logger:  logger,
	}
}

// SetOnConnChange registers a callback for connection state changes.
// LocalSender never connects, so this is a no-op for compatibility.
func (s *LocalSender) SetOnConnChange(fn func(bool)) {
	s.onConnChange = fn
}

// Stats returns counters. LocalSender always reports 0 for batches/errors.
func (s *LocalSender) Stats() (batchesSent, pointsSent, errors uint64) {
	return 0, s.pointsDrained, 0
}

// IsConnected returns false (local mode has no connection).
func (s *LocalSender) IsConnected() bool {
	return false
}

// LastSendAt returns zero time.
func (s *LocalSender) LastSendAt() time.Time {
	return time.Time{}
}

// SeqNumber returns 0.
func (s *LocalSender) SeqNumber() uint64 {
	return 0
}

// LastError returns empty string.
func (s *LocalSender) LastError() string {
	return ""
}

// Target returns "local" to indicate local-only mode.
func (s *LocalSender) Target() string {
	return "local"
}

// Start drains the input channel until it is closed or ctx is cancelled.
func (s *LocalSender) Start(ctx context.Context) error {
	s.logger.Info("running in local mode, discarding telemetry")
	for {
		select {
		case _, ok := <-s.input:
			if !ok {
				return nil
			}
			s.pointsDrained++
		case <-ctx.Done():
			return nil
		}
	}
}
