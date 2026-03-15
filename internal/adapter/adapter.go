// Package adapter defines the interface all telemetry adapters must implement.
// Implementations: DCGM (S-002), PDU/SNMP (S-046), Temperature (S-047), K8s (S-048), Slurm (S-080).
package adapter

import (
	"context"
	"os"
	"time"
)

// Adapter is the interface all telemetry adapters must implement.
type Adapter interface {
	// Name returns the adapter identifier (e.g., "dcgm", "pdu").
	Name() string

	// Start begins polling/watching. Blocks until ctx is cancelled.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the adapter.
	Stop(ctx context.Context) error

	// Readings returns a channel of raw readings for the normalizer to consume.
	Readings() <-chan RawReading
}

// Hostname returns the OS hostname or "unknown" on error.
// Shared by adapters that use the hostname as the reading source.
func Hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// WorkloadAdapter is implemented by adapters that produce workload metadata (e.g., K8s).
// The platform (S-049+) consumes this for job-to-rack mapping in the Training Impact Model.
type WorkloadAdapter interface {
	GetWorkloadState() interface{} // WorkloadState from kubernetes package
}

// RawReading is the raw telemetry from an adapter before normalization.
type RawReading struct {
	AdapterName string
	Source      string // e.g., hostname or device ID
	Timestamp   time.Time
	Metrics     map[string]interface{} // Flexible key-value metrics
}
