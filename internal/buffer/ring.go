package buffer

import (
	"sync"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

// Ring is a fixed-capacity circular buffer for TelemetryPoints.
// It is safe for concurrent use. Push and Pop are non-blocking.
type Ring struct {
	mu    sync.Mutex
	items []normalizer.TelemetryPoint
	head  int // index of next Pop
	tail  int // index of next Push
	count int
	cap   int
}

// NewRing creates a Ring with the given capacity. Capacity must be > 0.
func NewRing(capacity int) *Ring {
	if capacity <= 0 {
		panic("ring: capacity must be > 0")
	}
	return &Ring{
		items: make([]normalizer.TelemetryPoint, capacity),
		cap:   capacity,
	}
}

// Push adds a point to the ring. Returns false if the ring is full (caller
// should spill to WAL).
func (r *Ring) Push(p normalizer.TelemetryPoint) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == r.cap {
		return false
	}

	r.items[r.tail] = p
	r.tail = (r.tail + 1) % r.cap
	r.count++
	return true
}

// Pop removes and returns the oldest point. Returns false if the ring is empty.
func (r *Ring) Pop() (normalizer.TelemetryPoint, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count == 0 {
		var zero normalizer.TelemetryPoint
		return zero, false
	}

	p := r.items[r.head]
	r.items[r.head] = normalizer.TelemetryPoint{} // clear reference
	r.head = (r.head + 1) % r.cap
	r.count--
	return p, true
}

// Len returns the number of items currently in the ring.
func (r *Ring) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// Cap returns the ring's capacity.
func (r *Ring) Cap() int {
	return r.cap
}
