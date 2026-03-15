// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package sender transmits normalized telemetry to a remote platform via gRPC.
// It batches TelemetryPoints and streams them using a bidirectional gRPC stream,
// handling reconnection with exponential backoff and mTLS.
package sender

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	telemetryv1 "github.com/keldron-ai/keldron-agent/internal/proto/telemetry/v1"
)

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 60 * time.Second
)

// Sender reads TelemetryPoints from the normalizer output channel, batches them,
// and streams batches to the platform via gRPC.
type Sender struct {
	cfg          config.SenderConfig
	agentID      string
	input        <-chan normalizer.TelemetryPoint
	batcher      *Batcher
	logger       *slog.Logger
	dialOpts     []grpc.DialOption
	onConnChange func(bool)

	seqNum      atomic.Uint64
	batchesSent atomic.Uint64
	pointsSent  atomic.Uint64
	errors      atomic.Uint64
	connected   atomic.Bool
	lastSendAt  atomic.Value // time.Time
	lastError   atomic.Value // string
}

// New is an alias for NewGRPC for backward compatibility with tests.
func New(cfg config.SenderConfig, agentID string, input <-chan normalizer.TelemetryPoint, logger *slog.Logger, dialOpts ...grpc.DialOption) *Sender {
	return NewGRPC(cfg, agentID, input, logger, dialOpts...)
}

// NewGRPC creates a gRPC Sender. Extra dialOpts are appended to the connection options,
// allowing tests to inject a bufconn dialer.
func NewGRPC(cfg config.SenderConfig, agentID string, input <-chan normalizer.TelemetryPoint, logger *slog.Logger, dialOpts ...grpc.DialOption) *Sender {
	return &Sender{
		cfg:      cfg,
		agentID:  agentID,
		input:    input,
		batcher:  NewBatcher(cfg.BatchSize),
		logger:   logger,
		dialOpts: dialOpts,
	}
}

// SetOnConnChange registers a callback that is invoked when connection state
// changes. The buffer manager uses this to pause/resume egress.
func (s *Sender) SetOnConnChange(fn func(bool)) {
	s.onConnChange = fn
}

// Stats returns counters for observability.
func (s *Sender) Stats() (batchesSent, pointsSent, errors uint64) {
	return s.batchesSent.Load(), s.pointsSent.Load(), s.errors.Load()
}

// IsConnected returns true if the sender has an active gRPC connection.
func (s *Sender) IsConnected() bool {
	return s.connected.Load()
}

// LastSendAt returns the time of the last successful batch send.
func (s *Sender) LastSendAt() time.Time {
	if v := s.lastSendAt.Load(); v != nil {
		return v.(time.Time)
	}
	return time.Time{}
}

// SeqNumber returns the current sequence number (next to be used).
func (s *Sender) SeqNumber() uint64 {
	return s.seqNum.Load()
}

// LastError returns the last send or ack error message.
func (s *Sender) LastError() string {
	if v := s.lastError.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// Target returns the gRPC target address.
func (s *Sender) Target() string {
	return s.cfg.Target
}

// Start connects to the platform and runs the send loop with automatic
// reconnection on stream failures. It blocks until ctx is cancelled or the
// input channel is closed.
func (s *Sender) Start(ctx context.Context) error {
	backoff := initialBackoff

	for {
		connected, err := s.runOnce(ctx)
		if err == nil {
			// Clean exit (input channel closed or context cancelled).
			return nil
		}

		if ctx.Err() != nil {
			return nil
		}

		s.notifyConnChange(false)
		s.logger.Warn("stream failed, reconnecting",
			"error", err,
			"backoff", backoff,
		)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil
		}

		if connected {
			// Connection was established before failure — reset backoff.
			backoff = initialBackoff
		} else {
			// Failed to connect — exponential backoff.
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

// Stop is a convenience for callers that cancel context externally.
// The Start method handles cleanup on context cancellation.
func (s *Sender) Stop() {
	// No-op: lifecycle managed via context cancellation in Start.
}

// runOnce connects, opens a stream, and runs the send loop. It returns
// (true, nil) or (true, err) if a connection was established, or
// (false, err) if it failed to connect. A nil error means clean shutdown.
func (s *Sender) runOnce(ctx context.Context) (bool, error) {
	conn, err := s.connect(ctx)
	if err != nil {
		return false, fmt.Errorf("connection: %w", err)
	}

	// Use a separate context for the stream so we can flush remaining points
	// after the caller's context is cancelled.
	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()

	client := telemetryv1.NewTelemetryServiceClient(conn)
	stream, err := client.StreamBatch(streamCtx)
	if err != nil {
		conn.Close()
		return false, fmt.Errorf("opening stream: %w", err)
	}

	s.notifyConnChange(true)
	defer s.notifyConnChange(false)

	// Ack receiver goroutine — returns error via channel on stream death.
	ackErr := make(chan error, 1)
	go func() {
		ackErr <- s.receiveAcks(stream)
	}()

	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case point, ok := <-s.input:
			if !ok {
				// Input channel closed (normalizer/buffer shut down).
				s.flush(stream)
				stream.CloseSend()
				conn.Close()
				return true, nil
			}
			if s.batcher.Add(point) {
				if err := s.flushWithError(stream); err != nil {
					conn.Close()
					return true, err
				}
			}
		case <-ticker.C:
			if s.batcher.Len() > 0 {
				if err := s.flushWithError(stream); err != nil {
					conn.Close()
					return true, err
				}
			}
		case err := <-ackErr:
			// Stream died from receive side.
			conn.Close()
			if err != nil {
				return true, fmt.Errorf("ack stream: %w", err)
			}
			return true, fmt.Errorf("ack stream closed unexpectedly")
		case <-ctx.Done():
			s.flush(stream)
			streamCancel()
			stream.CloseSend()
			conn.Close()
			return true, nil
		}
	}
}

func (s *Sender) notifyConnChange(connected bool) {
	s.connected.Store(connected)
	if s.onConnChange != nil {
		s.onConnChange(connected)
	}
}

func (s *Sender) flush(stream grpc.BidiStreamingClient[telemetryv1.TelemetryBatch, telemetryv1.BatchAck]) {
	points := s.batcher.Flush()
	if points == nil {
		return
	}
	s.sendBatch(stream, points)
}

func (s *Sender) flushWithError(stream grpc.BidiStreamingClient[telemetryv1.TelemetryBatch, telemetryv1.BatchAck]) error {
	points := s.batcher.Flush()
	if points == nil {
		return nil
	}
	return s.sendBatchWithError(stream, points)
}

func (s *Sender) sendBatch(stream grpc.BidiStreamingClient[telemetryv1.TelemetryBatch, telemetryv1.BatchAck], points []normalizer.TelemetryPoint) {
	_ = s.sendBatchWithError(stream, points)
}

func (s *Sender) sendBatchWithError(stream grpc.BidiStreamingClient[telemetryv1.TelemetryBatch, telemetryv1.BatchAck], points []normalizer.TelemetryPoint) error {
	seq := s.seqNum.Add(1)
	batch := toProtoBatch(s.agentID, seq, points)

	if err := stream.Send(batch); err != nil {
		s.errors.Add(1)
		s.lastError.Store(err.Error())
		s.logger.Error("failed to send batch",
			"seq", seq,
			"points", len(points),
			"error", err,
		)
		return err
	}

	s.batchesSent.Add(1)
	s.pointsSent.Add(uint64(len(points)))
	s.lastSendAt.Store(time.Now())
	s.logger.Debug("batch sent",
		"seq", seq,
		"points", len(points),
	)
	return nil
}

func (s *Sender) receiveAcks(stream grpc.BidiStreamingClient[telemetryv1.TelemetryBatch, telemetryv1.BatchAck]) error {
	for {
		ack, err := stream.Recv()
		if err != nil {
			s.logger.Debug("ack stream ended", "error", err)
			return err
		}
		switch ack.Status {
		case telemetryv1.AckStatus_ACK_STATUS_OK:
			s.logger.Debug("batch acked",
				"seq", ack.SequenceNumber,
				"accepted", ack.AcceptedCount,
			)
		case telemetryv1.AckStatus_ACK_STATUS_PARTIAL:
			s.logger.Warn("batch partially accepted",
				"seq", ack.SequenceNumber,
				"accepted", ack.AcceptedCount,
				"rejected", ack.RejectedCount,
				"message", ack.Message,
			)
		case telemetryv1.AckStatus_ACK_STATUS_ERROR:
			s.errors.Add(1)
			s.lastError.Store(ack.Message)
			s.logger.Error("batch rejected",
				"seq", ack.SequenceNumber,
				"message", ack.Message,
			)
		default:
			s.logger.Warn("unknown ack status",
				"seq", ack.SequenceNumber,
				"status", ack.Status,
			)
		}
	}
}

func (s *Sender) connect(ctx context.Context) (*grpc.ClientConn, error) {
	opts := make([]grpc.DialOption, 0, len(s.dialOpts)+1)

	if s.cfg.TLS.Enabled {
		tlsCfg, err := buildTLSConfig(s.cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("building TLS config: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	opts = append(opts, s.dialOpts...)

	conn, err := grpc.NewClient(s.cfg.Target, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", s.cfg.Target, err)
	}
	s.logger.Info("connected to platform", "target", s.cfg.Target)
	return conn, nil
}

func buildTLSConfig(cfg config.TLSConfig) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading client cert/key: %w", err)
	}

	caCert, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// toProtoBatch converts a slice of normalizer.TelemetryPoint to a protobuf TelemetryBatch.
func toProtoBatch(agentID string, seqNum uint64, points []normalizer.TelemetryPoint) *telemetryv1.TelemetryBatch {
	protoPoints := make([]*telemetryv1.TelemetryPoint, len(points))
	for i, p := range points {
		protoPoints[i] = &telemetryv1.TelemetryPoint{
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
	return &telemetryv1.TelemetryBatch{
		AgentId:        agentID,
		SequenceNumber: seqNum,
		SentAtMs:       time.Now().UnixMilli(),
		Points:         protoPoints,
	}
}
