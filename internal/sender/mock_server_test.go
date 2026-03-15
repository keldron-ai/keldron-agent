// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package sender

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	telemetryv1 "github.com/keldron-ai/keldron-agent/internal/proto/telemetry/v1"
)

const bufSize = 1024 * 1024

// mockTelemetryServer implements TelemetryServiceServer for in-process testing.
// It records all received batches and responds with ACK_STATUS_OK.
type mockTelemetryServer struct {
	telemetryv1.UnimplementedTelemetryServiceServer

	mu      sync.Mutex
	batches []*telemetryv1.TelemetryBatch
	ackFunc func(*telemetryv1.TelemetryBatch) *telemetryv1.BatchAck // optional custom ack
}

func (m *mockTelemetryServer) StreamBatch(stream grpc.BidiStreamingServer[telemetryv1.TelemetryBatch, telemetryv1.BatchAck]) error {
	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		m.mu.Lock()
		m.batches = append(m.batches, batch)
		ackFn := m.ackFunc
		m.mu.Unlock()

		var ack *telemetryv1.BatchAck
		if ackFn != nil {
			ack = ackFn(batch)
		} else {
			ack = &telemetryv1.BatchAck{
				SequenceNumber: batch.SequenceNumber,
				Status:         telemetryv1.AckStatus_ACK_STATUS_OK,
				AcceptedCount:  int32(len(batch.Points)),
			}
		}

		if err := stream.Send(ack); err != nil {
			return err
		}
	}
}

func (m *mockTelemetryServer) getBatches() []*telemetryv1.TelemetryBatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*telemetryv1.TelemetryBatch, len(m.batches))
	copy(out, m.batches)
	return out
}

// setupTestServer creates an in-process gRPC server using bufconn and returns
// a dial option for connecting to it, plus the mock server for assertions.
func setupTestServer(t *testing.T) (grpc.DialOption, *mockTelemetryServer) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	mock := &mockTelemetryServer{}
	telemetryv1.RegisterTelemetryServiceServer(srv, mock)

	go func() {
		if err := srv.Serve(lis); err != nil {
			// Server stopped, expected during test cleanup.
		}
	}()

	t.Cleanup(func() {
		srv.GracefulStop()
		lis.Close()
	})

	dialOpt := grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	})

	return dialOpt, mock
}

// dialTestServer creates a client connection to the bufconn test server.
func dialTestServer(t *testing.T, dialOpt grpc.DialOption) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		dialOpt,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dialing bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
