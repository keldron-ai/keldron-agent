package buffer

import (
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	telemetryv1 "github.com/keldron-ai/keldron-agent/internal/proto/telemetry/v1"
)

// pointToProto converts a normalizer.TelemetryPoint to a protobuf TelemetryPoint
// for WAL serialization.
func pointToProto(p normalizer.TelemetryPoint) *telemetryv1.TelemetryPoint {
	return &telemetryv1.TelemetryPoint{
		Id:           p.ID,
		AgentId:      p.AgentID,
		AdapterName:  p.AdapterName,
		Source:       p.Source,
		RackId:       p.RackID,
		TimestampMs:  p.Timestamp.UnixMilli(),
		ReceivedAtMs: p.ReceivedAt.UnixMilli(),
		Metrics:      p.Metrics,
	}
}

// protoToPoint converts a protobuf TelemetryPoint back to a normalizer.TelemetryPoint.
// The Metrics map is cloned for safety.
func protoToPoint(pb *telemetryv1.TelemetryPoint) normalizer.TelemetryPoint {
	metrics := make(map[string]float64, len(pb.Metrics))
	for k, v := range pb.Metrics {
		metrics[k] = v
	}
	return normalizer.TelemetryPoint{
		ID:          pb.Id,
		AgentID:     pb.AgentId,
		AdapterName: pb.AdapterName,
		Source:      pb.Source,
		RackID:      pb.RackId,
		Timestamp:   time.UnixMilli(pb.TimestampMs),
		ReceivedAt:  time.UnixMilli(pb.ReceivedAtMs),
		Metrics:     metrics,
	}
}
