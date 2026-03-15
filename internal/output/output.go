// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package output provides local output backends for telemetry: Prometheus metrics
// and stdout JSON. Used when running in local mode (--local or output.prometheus).
package output

import (
	"context"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

// Output is the interface all output backends implement.
// Prometheus exposes /metrics; Stdout prints JSON lines.
type Output interface {
	// Start begins serving (e.g. HTTP for Prometheus). May block until ctx is cancelled.
	Start(ctx context.Context) error

	// Update applies a batch of telemetry points and risk scores to the output.
	// Scores may be nil (e.g. empty batch); outputs use placeholders when nil.
	Update(readings []normalizer.TelemetryPoint, scores []scoring.RiskScoreOutput) error

	// Close releases resources. Idempotent.
	Close() error
}
