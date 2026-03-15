// Package buffer provides an in-memory ring buffer with write-ahead log (WAL)
// for durable telemetry buffering during network outages.
package buffer

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

// Manager coordinates an in-memory ring buffer and a disk WAL to buffer
// telemetry points between the normalizer and sender. When the sender is
// connected, points flow through the ring. On disconnect, the ring is
// flushed to WAL so that WAL becomes the single source of truth, preserving
// FIFO order. While disconnected, new points write directly to WAL. On
// reconnection, WAL is drained (ring is guaranteed empty).
type Manager struct {
	ring       *Ring
	wal        *WAL
	input      <-chan normalizer.TelemetryPoint
	output     chan normalizer.TelemetryPoint
	connected  atomic.Bool
	connMu     sync.Mutex    // serializes disconnect transitions with ingress writes
	connNotify chan struct{} // wake egress on reconnect
	dataNotify chan struct{} // wake egress on new data
	logger     *slog.Logger

	ringPushes atomic.Uint64
	walSpills  atomic.Uint64
	walDrained atomic.Uint64
	dropped    atomic.Uint64
	draining   atomic.Bool
}

// NewManager creates a buffer Manager from configuration.
func NewManager(cfg config.BufferConfig, input <-chan normalizer.TelemetryPoint, logger *slog.Logger) (*Manager, error) {
	walMax, err := config.ParseByteSize(cfg.WALMaxSize)
	if err != nil {
		return nil, err
	}

	wal, err := NewWAL(cfg.WALDir, walMax, logger)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		ring:       NewRing(cfg.RingSize),
		wal:        wal,
		input:      input,
		output:     make(chan normalizer.TelemetryPoint, 256),
		connNotify: make(chan struct{}, 1),
		dataNotify: make(chan struct{}, 1),
		logger:     logger,
	}

	// Start as connected — optimistic default.
	m.connected.Store(true)

	return m, nil
}

// Output returns the channel that the sender reads from.
func (m *Manager) Output() <-chan normalizer.TelemetryPoint {
	return m.output
}

// OnConnChange is called by the sender to notify the buffer of connection
// state changes. On disconnect, the ring is flushed to WAL to preserve FIFO
// order. On reconnect, the egress goroutine is woken to drain the WAL.
func (m *Manager) OnConnChange(connected bool) {
	if !connected {
		m.connMu.Lock()
		m.connected.Store(false)
		m.flushRingToWAL()
		m.connMu.Unlock()
	} else {
		m.connected.Store(true)
	}
	if connected {
		select {
		case m.connNotify <- struct{}{}:
		default:
		}
	}
}

// flushRingToWAL pops all entries from the ring and writes them to WAL in
// order, so that WAL becomes the single source of truth during disconnect.
func (m *Manager) flushRingToWAL() {
	for {
		p, ok := m.ring.Pop()
		if !ok {
			return
		}
		if err := m.wal.Write(p); err != nil {
			m.dropped.Add(1)
			m.logger.Error("WAL write during ring flush failed, point dropped", "error", err)
			continue
		}
		m.walSpills.Add(1)
	}
}

// Start launches the ingress and egress goroutines. It blocks until ctx is
// cancelled, then closes the output channel and WAL.
func (m *Manager) Start(ctx context.Context) error {
	var wg sync.WaitGroup

	// Drain any WAL data from a previous crash before starting normal operation.
	if m.wal.HasData() {
		m.logger.Info("draining WAL data from previous run")
		m.drainWAL(ctx)
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		m.runIngress(ctx)
	}()
	go func() {
		defer wg.Done()
		m.runEgress(ctx)
	}()

	wg.Wait()
	close(m.output)
	return m.wal.Close()
}

// Stats returns buffer manager counters.
func (m *Manager) Stats() (ringPushes, walSpills, walDrained, dropped uint64) {
	return m.ringPushes.Load(), m.walSpills.Load(), m.walDrained.Load(), m.dropped.Load()
}

// RingStats returns ring buffer capacity and current used count.
func (m *Manager) RingStats() (capacity, used int) {
	return m.ring.Cap(), m.ring.Len()
}

// WALStats returns WAL total size, max size, segment count, point count, and draining state.
func (m *Manager) WALStats() (totalSize, maxSize int64, segments int, points uint64, draining bool) {
	totalSize, segments, points = m.wal.Stats()
	maxSize = m.wal.MaxTotal()
	draining = m.draining.Load()
	return totalSize, maxSize, segments, points, draining
}

// runIngress reads from the normalizer output. When connected, it pushes to
// the ring. When the ring is full, it flushes the entire ring to WAL and
// writes the new point to WAL, preserving FIFO order. When disconnected, it
// writes directly to WAL. All paths hold connMu to serialize with disconnect
// transitions.
func (m *Manager) runIngress(ctx context.Context) {
	walWrite := func(p normalizer.TelemetryPoint) {
		if err := m.wal.Write(p); err != nil {
			m.dropped.Add(1)
			m.logger.Error("WAL write failed, point dropped", "error", err)
		} else {
			m.walSpills.Add(1)
		}
	}

	for {
		select {
		case p, ok := <-m.input:
			if !ok {
				return
			}
			m.connMu.Lock()
			if !m.connected.Load() {
				// Disconnected — write directly to WAL (ring was flushed).
				walWrite(p)
			} else if m.ring.Push(p) {
				m.ringPushes.Add(1)
			} else {
				// Ring full — flush entire ring to WAL, then write new point.
				m.flushRingToWAL()
				walWrite(p)
			}
			m.connMu.Unlock()
			// Signal egress that data is available.
			select {
			case m.dataNotify <- struct{}{}:
			default:
			}
		case <-ctx.Done():
			return
		}
	}
}

// runEgress sends buffered data to the output channel. When disconnected, it
// blocks waiting for reconnection. On reconnect, it drains the WAL first,
// then pops from the ring.
func (m *Manager) runEgress(ctx context.Context) {
	for {
		// If disconnected, wait for reconnection.
		if !m.connected.Load() {
			select {
			case <-m.connNotify:
				continue
			case <-ctx.Done():
				return
			}
		}

		// Check WAL and pop from ring atomically wrt ingress flushes.
		// Holding connMu ensures ingress cannot flush ring to WAL between
		// the HasData check and ring.Pop, which would break FIFO.
		m.connMu.Lock()
		hasWAL := m.wal.HasData()
		var p normalizer.TelemetryPoint
		var ok bool
		if !hasWAL {
			p, ok = m.ring.Pop()
		}
		m.connMu.Unlock()

		if hasWAL {
			m.drainWAL(ctx)
			if ctx.Err() != nil {
				return
			}
			continue
		}
		if ok {
			select {
			case m.output <- p:
			case <-ctx.Done():
				return
			}
			continue
		}

		// Both empty — wait for data or connection change.
		select {
		case <-m.dataNotify:
		case <-m.connNotify:
		case <-ctx.Done():
			return
		}
	}
}

// drainWAL reads all WAL data and sends it to the output channel.
func (m *Manager) drainWAL(ctx context.Context) {
	m.draining.Store(true)
	defer m.draining.Store(false)

	ch := m.wal.Drain(ctx)
	for p := range ch {
		m.walDrained.Add(1)
		select {
		case m.output <- p:
		case <-ctx.Done():
			return
		}
	}
}
