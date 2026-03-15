// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// mock-server is a standalone gRPC server for testing the agent's telemetry pipeline.
// It implements TelemetryService.StreamBatch, logs received batches, and sends BatchAck.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	telemetryv1 "github.com/keldron-ai/keldron-agent/internal/proto/telemetry/v1"
)

type mockServer struct {
	telemetryv1.UnimplementedTelemetryServiceServer

	totalBatches atomic.Uint64
	totalPoints  atomic.Uint64
	startTime    time.Time
}

func (m *mockServer) StreamBatch(stream grpc.BidiStreamingServer[telemetryv1.TelemetryBatch, telemetryv1.BatchAck]) error {
	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		seq := batch.SequenceNumber
		pointCount := len(batch.Points)
		agentID := batch.AgentId

		// Sample first point's metrics
		sample := ""
		if len(batch.Points) > 0 {
			pt := batch.Points[0]
			temp := pt.Metrics["temperature_c"]
			util := pt.Metrics["gpu_utilization_pct"]
			throttled := pt.Metrics["throttled"]
			sample = fmt.Sprintf(" temp=%.1f°C util=%.1f%% throttled=%.0f",
				temp, util, throttled)
		}

		log.Printf("batch seq=%d points=%d agent=%s%s",
			seq, pointCount, agentID, sample)

		// Update totals
		m.totalBatches.Add(1)
		m.totalPoints.Add(uint64(pointCount))

		// Print running totals every 10 batches
		if m.totalBatches.Load()%10 == 0 {
			elapsed := time.Since(m.startTime).Seconds()
			ptsPerSec := float64(0)
			if elapsed > 0 {
				ptsPerSec = float64(m.totalPoints.Load()) / elapsed
			}
			log.Printf("--- totals: batches=%d points=%d pts/sec=%.0f",
				m.totalBatches.Load(), m.totalPoints.Load(), ptsPerSec)
		}

		ack := &telemetryv1.BatchAck{
			SequenceNumber: batch.SequenceNumber,
			Status:         telemetryv1.AckStatus_ACK_STATUS_OK,
			AcceptedCount:  int32(pointCount),
		}
		if err := stream.Send(ack); err != nil {
			return err
		}
	}
}

func main() {
	mock := &mockServer{
		startTime: time.Now(),
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	telemetryv1.RegisterTelemetryServiceServer(srv, mock)

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("shutdown signal received, stopping server...")
		srv.GracefulStop()
	}()

	fmt.Println("mock-server listening on :50051 (Ctrl+C to stop)")
	if err := srv.Serve(lis); err != nil && err != grpc.ErrServerStopped {
		log.Fatalf("serve: %v", err)
	}
	log.Println("server stopped")
	os.Exit(0)
}
