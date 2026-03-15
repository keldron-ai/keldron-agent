package buffer

import (
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

func TestPointToProtoRoundTrip(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)
	recvAt := time.Date(2025, 7, 1, 12, 0, 1, 0, time.UTC)

	tests := []struct {
		name  string
		point normalizer.TelemetryPoint
	}{
		{
			name: "all fields",
			point: normalizer.TelemetryPoint{
				ID:          "01HXYZ",
				AgentID:     "agent-1",
				AdapterName: "dcgm",
				Source:      "gpu-node-01",
				RackID:      "rack-A1",
				Timestamp:   ts,
				ReceivedAt:  recvAt,
				Metrics:     map[string]float64{"temperature": 72.5, "power": 300.0},
			},
		},
		{
			name: "empty metrics",
			point: normalizer.TelemetryPoint{
				ID:          "01HABC",
				AgentID:     "agent-2",
				AdapterName: "pdu",
				Source:      "pdu-01",
				RackID:      "rack-B1",
				Timestamp:   ts,
				ReceivedAt:  recvAt,
				Metrics:     map[string]float64{},
			},
		},
		{
			name: "zero timestamps",
			point: normalizer.TelemetryPoint{
				ID:          "01HDEF",
				AgentID:     "agent-3",
				AdapterName: "temperature",
				Source:      "sensor-01",
				RackID:      "rack-C1",
				Timestamp:   time.UnixMilli(0),
				ReceivedAt:  time.UnixMilli(0),
				Metrics:     map[string]float64{"celsius": 22.3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pb := pointToProto(tt.point)
			got := protoToPoint(pb)

			if got.ID != tt.point.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.point.ID)
			}
			if got.AgentID != tt.point.AgentID {
				t.Errorf("AgentID = %q, want %q", got.AgentID, tt.point.AgentID)
			}
			if got.AdapterName != tt.point.AdapterName {
				t.Errorf("AdapterName = %q, want %q", got.AdapterName, tt.point.AdapterName)
			}
			if got.Source != tt.point.Source {
				t.Errorf("Source = %q, want %q", got.Source, tt.point.Source)
			}
			if got.RackID != tt.point.RackID {
				t.Errorf("RackID = %q, want %q", got.RackID, tt.point.RackID)
			}
			if !got.Timestamp.Equal(tt.point.Timestamp) {
				t.Errorf("Timestamp = %v, want %v", got.Timestamp, tt.point.Timestamp)
			}
			if !got.ReceivedAt.Equal(tt.point.ReceivedAt) {
				t.Errorf("ReceivedAt = %v, want %v", got.ReceivedAt, tt.point.ReceivedAt)
			}
			if len(got.Metrics) != len(tt.point.Metrics) {
				t.Fatalf("Metrics count = %d, want %d", len(got.Metrics), len(tt.point.Metrics))
			}
			for k, want := range tt.point.Metrics {
				if v, ok := got.Metrics[k]; !ok {
					t.Errorf("missing metric %q", k)
				} else if v != want {
					t.Errorf("metric %q = %f, want %f", k, v, want)
				}
			}
		})
	}
}

func TestProtoToPointClonesMetrics(t *testing.T) {
	t.Parallel()

	p := normalizer.TelemetryPoint{
		ID:         "01H",
		AgentID:    "a",
		Timestamp:  time.Now(),
		ReceivedAt: time.Now(),
		Metrics:    map[string]float64{"x": 1.0},
	}

	pb := pointToProto(p)
	got := protoToPoint(pb)

	// Mutate the returned metrics map — original proto should not be affected.
	got.Metrics["x"] = 999.0

	if pb.Metrics["x"] == 999.0 {
		t.Error("protoToPoint did not clone metrics map")
	}
}
