package dcgm

import (
	"math/rand"
)

// StubClient generates deterministic synthetic GPU data that mimics
// NVIDIA A100 GPUs with physically plausible values.
type StubClient struct {
	gpuIDs []int
	rng    *rand.Rand
}

// NewStubClient creates a StubClient seeded for reproducible output.
func NewStubClient(gpuIDs []int) *StubClient {
	return &StubClient{
		gpuIDs: gpuIDs,
		rng:    rand.New(rand.NewSource(42)),
	}
}

// Collect returns a GPUMetrics snapshot for each configured GPU.
func (s *StubClient) Collect() ([]GPUMetrics, error) {
	metrics := make([]GPUMetrics, len(s.gpuIDs))
	for i, id := range s.gpuIDs {
		metrics[i] = GPUMetrics{
			GPUID:          id,
			GPUName:        "NVIDIA A100-SXM4-80GB",
			Temperature:    40.0 + s.rng.Float64()*45.0,   // 40–85 C
			PowerUsage:     200.0 + s.rng.Float64()*150.0,  // 200–350 W
			GPUUtilization: 70.0 + s.rng.Float64()*29.0,    // 70–99%
			MemUtilization: 50.0 + s.rng.Float64()*40.0,    // 50–90%
			MemUsed:        40e9 + uint64(s.rng.Int63n(40e9)), // 40–80 GB
			MemTotal:       80 * 1024 * 1024 * 1024,         // 80 GB
			SMClock:        1200 + uint32(s.rng.Intn(210)),  // 1200–1410 MHz
			MemClock:       1200 + uint32(s.rng.Intn(395)),  // 1200–1593 MHz (slightly wider range - HBM2e)
			Throttled:      s.rng.Float64() < 0.05,          // 5% chance
		}
	}
	return metrics, nil
}

// Close is a no-op for the stub client.
func (s *StubClient) Close() error {
	return nil
}
