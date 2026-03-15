// Package normalizer transforms raw adapter readings into a canonical telemetry format.
// It fans in from multiple adapter channels, validates readings, resolves rack IDs,
// coerces metrics to float64, assigns ULIDs, and emits TelemetryPoint structs.
package normalizer

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"
)

const (
	outputBuffer = 512
	defaultSkew  = 30 * time.Second
)

// Normalizer consumes RawReadings from multiple adapters,
// validates, transforms, and outputs TelemetryPoints.
type Normalizer struct {
	agentID       string
	rackMapping   atomic.Pointer[map[string]string]
	maxSkew       time.Duration
	inputs        []<-chan adapter.RawReading
	output        chan TelemetryPoint
	logger        *slog.Logger
	entropy       *ulid.MonotonicEntropy
	configHolder  *config.Holder

	processed atomic.Uint64
	rejected  atomic.Uint64
	running   atomic.Bool
}

// New creates a Normalizer configured with the agent ID and rack mapping.
// If holder is non-nil, the normalizer subscribes to config changes for rack_mapping hot-reload.
func New(agentID string, rackMapping map[string]string, holder *config.Holder, logger *slog.Logger) *Normalizer {
	n := &Normalizer{
		agentID:      agentID,
		maxSkew:      defaultSkew,
		output:       make(chan TelemetryPoint, outputBuffer),
		logger:       logger,
		entropy:      ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0),
		configHolder: holder,
	}
	m := make(map[string]string)
	for k, v := range rackMapping {
		m[k] = v
	}
	n.rackMapping.Store(&m)
	return n
}

// UpdateRackMapping atomically updates the rack mapping for hot-reload.
func (n *Normalizer) UpdateRackMapping(m map[string]string) {
	newMap := make(map[string]string)
	for k, v := range m {
		newMap[k] = v
	}
	n.rackMapping.Store(&newMap)
}

// AddInput registers an adapter's readings channel for fan-in.
func (n *Normalizer) AddInput(ch <-chan adapter.RawReading) {
	n.inputs = append(n.inputs, ch)
}

// Output returns the channel of normalized TelemetryPoints.
func (n *Normalizer) Output() <-chan TelemetryPoint {
	return n.output
}

// Stats returns processed and rejected counts for the health endpoint.
func (n *Normalizer) Stats() (processed, rejected uint64) {
	return n.processed.Load(), n.rejected.Load()
}

// InputCount returns the number of adapter input channels being consumed.
func (n *Normalizer) InputCount() int {
	return len(n.inputs)
}

// IsRunning returns true if the normalizer's Start loop is active.
func (n *Normalizer) IsRunning() bool {
	return n.running.Load()
}

// Start begins consuming from all input channels. Blocks until ctx is cancelled
// and all input channels are drained.
func (n *Normalizer) Start(ctx context.Context) error {
	n.running.Store(true)
	defer n.running.Store(false)

	merged := make(chan adapter.RawReading, outputBuffer)

	// Fan-in: one goroutine per input channel, all feeding merged.
	var wg sync.WaitGroup
	for _, ch := range n.inputs {
		wg.Add(1)
		go func(in <-chan adapter.RawReading) {
			defer wg.Done()
			for reading := range in {
				select {
				case merged <- reading:
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}

	// Close merged channel once all fan-in goroutines are done.
	go func() {
		wg.Wait()
		close(merged)
	}()

	if n.configHolder != nil {
		n.configHolder.Subscribe(func(cfg *config.Config) {
			n.UpdateRackMapping(cfg.RackMapping)
		})
	}

	n.logger.Info("normalizer started", "inputs", len(n.inputs))

	// Process merged readings until channel closed.
	for reading := range merged {
		n.process(reading)
	}

	close(n.output)
	p, r := n.Stats()
	n.logger.Info("normalizer stopped", "processed", p, "rejected", r)
	return nil
}

// process validates and transforms a single RawReading into a TelemetryPoint.
func (n *Normalizer) process(reading adapter.RawReading) {
	result := Validate(reading, n.maxSkew)
	if !result.Valid {
		n.rejected.Add(1)
		n.logger.Warn("reading rejected",
			"adapter", reading.AdapterName,
			"source", reading.Source,
			"reason", result.Reason,
		)
		return
	}

	// Resolve rack ID.
	mapping := n.rackMapping.Load()
	if mapping == nil {
		mapping = &map[string]string{}
	}
	rackID, ok := ResolveRackID(reading.Source, *mapping)
	if !ok {
		rackID = "unknown"
		n.logger.Warn("no rack mapping for source, using unknown",
			"source", reading.Source,
		)
	}

	// Coerce metrics to float64.
	metrics := make(map[string]float64, len(reading.Metrics))
	for key, val := range reading.Metrics {
		f, ok := CoerceToFloat64(val)
		if !ok {
			n.logger.Debug("skipping non-numeric metric",
				"key", key,
				"type", typeString(val),
			)
			continue
		}
		metrics[key] = f
	}

	point := TelemetryPoint{
		ID:          ulid.MustNew(ulid.Timestamp(time.Now()), n.entropy).String(),
		AgentID:     n.agentID,
		AdapterName: reading.AdapterName,
		Source:      reading.Source,
		RackID:      rackID,
		Timestamp:   reading.Timestamp,
		ReceivedAt:  time.Now(),
		Metrics:     metrics,
	}

	// Blocking send — applies backpressure to adapters rather than dropping data.
	n.output <- point
	n.processed.Add(1)
}

// typeString returns a string representation of the type for logging.
func typeString(v interface{}) string {
	if v == nil {
		return "nil"
	}
	switch v.(type) {
	case string:
		return "string"
	default:
		return "unknown"
	}
}
