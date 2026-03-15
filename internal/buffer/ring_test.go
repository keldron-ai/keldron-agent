package buffer

import (
	"sync"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func testPoint(id string) normalizer.TelemetryPoint {
	return normalizer.TelemetryPoint{
		ID:          id,
		AgentID:     "agent-test",
		AdapterName: "test",
		Source:      "src",
		RackID:      "rack",
		Timestamp:   time.Now(),
		ReceivedAt:  time.Now(),
		Metrics:     map[string]float64{"v": 1.0},
	}
}

func TestNewRing_ZeroPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewRing(0) should panic")
		}
	}()
	NewRing(0)
}

func TestNewRing_NegativePanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewRing(-1) should panic")
		}
	}()
	NewRing(-1)
}

func TestRing_FIFO(t *testing.T) {
	t.Parallel()

	r := NewRing(5)
	for i := range 5 {
		if !r.Push(testPoint(string(rune('A' + i)))) {
			t.Fatalf("Push failed at %d", i)
		}
	}

	for i := range 5 {
		p, ok := r.Pop()
		if !ok {
			t.Fatalf("Pop failed at %d", i)
		}
		want := string(rune('A' + i))
		if p.ID != want {
			t.Errorf("Pop %d: got %q, want %q", i, p.ID, want)
		}
	}
}

func TestRing_FullReturnsFalse(t *testing.T) {
	t.Parallel()

	r := NewRing(3)
	for i := range 3 {
		if !r.Push(testPoint(string(rune('A' + i)))) {
			t.Fatalf("Push should succeed at %d", i)
		}
	}
	if r.Push(testPoint("overflow")) {
		t.Error("Push to full ring should return false")
	}
}

func TestRing_EmptyReturnsFalse(t *testing.T) {
	t.Parallel()

	r := NewRing(3)
	_, ok := r.Pop()
	if ok {
		t.Error("Pop from empty ring should return false")
	}
}

func TestRing_Wraparound(t *testing.T) {
	t.Parallel()

	r := NewRing(4)

	// Fill completely.
	for i := range 4 {
		r.Push(testPoint(string(rune('A' + i))))
	}
	// Drain half.
	for range 2 {
		r.Pop()
	}
	// Fill 2 more (wraps around).
	r.Push(testPoint("E"))
	r.Push(testPoint("F"))

	// Should get C, D, E, F in order.
	expected := []string{"C", "D", "E", "F"}
	for i, want := range expected {
		p, ok := r.Pop()
		if !ok {
			t.Fatalf("Pop failed at %d", i)
		}
		if p.ID != want {
			t.Errorf("Pop %d: got %q, want %q", i, p.ID, want)
		}
	}

	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0", r.Len())
	}
}

func TestRing_LenCap(t *testing.T) {
	t.Parallel()

	r := NewRing(10)
	if r.Cap() != 10 {
		t.Errorf("Cap = %d, want 10", r.Cap())
	}
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0", r.Len())
	}

	r.Push(testPoint("A"))
	r.Push(testPoint("B"))
	if r.Len() != 2 {
		t.Errorf("Len = %d, want 2", r.Len())
	}

	r.Pop()
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}
}

func TestRing_ConcurrentPushPop(t *testing.T) {
	t.Parallel()

	r := NewRing(1000)
	var wg sync.WaitGroup

	// 10 producers pushing 100 each.
	for g := range 10 {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := range 100 {
				r.Push(testPoint(string(rune(g*100 + i))))
			}
		}(g)
	}

	// 5 consumers popping continuously.
	var popped sync.WaitGroup
	var count int64
	var mu sync.Mutex
	for range 5 {
		popped.Add(1)
		go func() {
			defer popped.Done()
			local := 0
			for range 200 {
				if _, ok := r.Pop(); ok {
					local++
				}
			}
			mu.Lock()
			count += int64(local)
			mu.Unlock()
		}()
	}

	wg.Wait()
	popped.Wait()

	// Drain remaining.
	for {
		if _, ok := r.Pop(); !ok {
			break
		}
		count++
	}

	if count != 1000 {
		t.Errorf("total popped = %d, want 1000", count)
	}
}
