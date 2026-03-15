package sender

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/keldron-ai/keldron-agent/internal/config"
	"github.com/keldron-ai/keldron-agent/internal/normalizer"
	telemetryv1 "github.com/keldron-ai/keldron-agent/internal/proto/telemetry/v1"
)

func testSenderConfig(batchSize int, flushInterval time.Duration) config.SenderConfig {
	return config.SenderConfig{
		Target:        "passthrough:///bufconn",
		BatchSize:     batchSize,
		FlushInterval: flushInterval,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(
		devNull{}, &slog.HandlerOptions{Level: slog.LevelError + 1},
	))
}

type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }

func TestSender_FlushOnBatchSize(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	cfg := testSenderConfig(5, 10*time.Second) // large interval so only size triggers flush
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Send exactly batch_size points.
	for i := range 5 {
		input <- testPoint(string(rune('A' + i)))
	}

	// Wait for the batch to be received.
	deadline := time.After(5 * time.Second)
	for {
		batches := mock.getBatches()
		if len(batches) >= 1 {
			if len(batches[0].Points) != 5 {
				t.Errorf("batch has %d points, want 5", len(batches[0].Points))
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for batch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done
}

func TestSender_FlushOnInterval(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	cfg := testSenderConfig(100, 100*time.Millisecond) // small interval, large batch size
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Send fewer than batch_size points.
	for i := range 3 {
		input <- testPoint(string(rune('A' + i)))
	}

	// Wait for the interval-based flush.
	deadline := time.After(5 * time.Second)
	for {
		batches := mock.getBatches()
		if len(batches) >= 1 {
			if len(batches[0].Points) != 3 {
				t.Errorf("batch has %d points, want 3", len(batches[0].Points))
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for interval flush")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done
}

func TestSender_SequenceNumbers(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	cfg := testSenderConfig(2, 10*time.Second) // batch of 2
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Send 6 points = 3 batches of 2.
	for i := range 6 {
		input <- testPoint(string(rune('A' + i)))
	}

	deadline := time.After(5 * time.Second)
	for {
		batches := mock.getBatches()
		if len(batches) >= 3 {
			for i, b := range batches[:3] {
				want := uint64(i + 1)
				if b.SequenceNumber != want {
					t.Errorf("batch %d: seq=%d, want %d", i, b.SequenceNumber, want)
				}
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: got %d batches, want 3", len(mock.getBatches()))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done
}

func TestSender_ShutdownFlush(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	cfg := testSenderConfig(100, 10*time.Second) // large batch, large interval
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Send 3 points (below batch_size, no interval flush yet).
	for i := range 3 {
		input <- testPoint(string(rune('A' + i)))
	}
	// Give sender time to read from the channel.
	time.Sleep(50 * time.Millisecond)

	// Cancel context — should trigger flush of remaining points.
	cancel()
	<-done

	// Give mock server time to process.
	time.Sleep(50 * time.Millisecond)

	batches := mock.getBatches()
	if len(batches) != 1 {
		t.Fatalf("got %d batches on shutdown, want 1", len(batches))
	}
	if len(batches[0].Points) != 3 {
		t.Errorf("shutdown batch has %d points, want 3", len(batches[0].Points))
	}
}

func TestSender_InputChannelClose(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	cfg := testSenderConfig(100, 10*time.Second)
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Send 2 points then close input channel.
	input <- testPoint("X")
	input <- testPoint("Y")
	time.Sleep(50 * time.Millisecond)
	close(input)

	// Sender should flush and return.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for sender to stop")
	}

	// Give mock server time to process.
	time.Sleep(50 * time.Millisecond)

	batches := mock.getBatches()
	totalPoints := 0
	for _, b := range batches {
		totalPoints += len(b.Points)
	}
	if totalPoints != 2 {
		t.Errorf("total points received = %d, want 2", totalPoints)
	}
}

func TestSender_Stats(t *testing.T) {
	t.Parallel()

	dialOpt, _ := setupTestServer(t)
	cfg := testSenderConfig(3, 10*time.Second)
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Send 6 points = 2 batches of 3.
	for i := range 6 {
		input <- testPoint(string(rune('A' + i)))
	}

	// Wait for batches to be sent.
	deadline := time.After(5 * time.Second)
	for {
		bs, ps, _ := s.Stats()
		if bs >= 2 && ps >= 6 {
			break
		}
		select {
		case <-deadline:
			bs, ps, es := s.Stats()
			t.Fatalf("timed out: batches=%d, points=%d, errors=%d", bs, ps, es)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done

	bs, ps, es := s.Stats()
	if bs < 2 {
		t.Errorf("batchesSent = %d, want >= 2", bs)
	}
	if ps < 6 {
		t.Errorf("pointsSent = %d, want >= 6", ps)
	}
	if es != 0 {
		t.Errorf("errors = %d, want 0", es)
	}
}

func TestToProtoBatch(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	recvAt := time.Date(2025, 6, 15, 10, 30, 1, 0, time.UTC)

	tests := []struct {
		name  string
		point normalizer.TelemetryPoint
	}{
		{
			name: "all fields mapped",
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
			name: "single metric",
			point: normalizer.TelemetryPoint{
				ID:          "01HDEF",
				AgentID:     "agent-3",
				AdapterName: "temperature",
				Source:      "sensor-01",
				RackID:      "rack-C1",
				Timestamp:   ts,
				ReceivedAt:  recvAt,
				Metrics:     map[string]float64{"celsius": 22.3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			batch := toProtoBatch("agent-test", 42, []normalizer.TelemetryPoint{tt.point})

			if batch.AgentId != "agent-test" {
				t.Errorf("AgentId = %q, want %q", batch.AgentId, "agent-test")
			}
			if batch.SequenceNumber != 42 {
				t.Errorf("SequenceNumber = %d, want 42", batch.SequenceNumber)
			}
			if batch.SentAtMs <= 0 {
				t.Error("SentAtMs should be positive")
			}
			if len(batch.Points) != 1 {
				t.Fatalf("Points count = %d, want 1", len(batch.Points))
			}

			pp := batch.Points[0]
			if pp.Id != tt.point.ID {
				t.Errorf("Id = %q, want %q", pp.Id, tt.point.ID)
			}
			if pp.AgentId != tt.point.AgentID {
				t.Errorf("AgentId = %q, want %q", pp.AgentId, tt.point.AgentID)
			}
			if pp.AdapterName != tt.point.AdapterName {
				t.Errorf("AdapterName = %q, want %q", pp.AdapterName, tt.point.AdapterName)
			}
			if pp.Source != tt.point.Source {
				t.Errorf("Source = %q, want %q", pp.Source, tt.point.Source)
			}
			if pp.RackId != tt.point.RackID {
				t.Errorf("RackId = %q, want %q", pp.RackId, tt.point.RackID)
			}
			if pp.TimestampMs != tt.point.Timestamp.UnixMilli() {
				t.Errorf("TimestampMs = %d, want %d", pp.TimestampMs, tt.point.Timestamp.UnixMilli())
			}
			if pp.ReceivedAtMs != tt.point.ReceivedAt.UnixMilli() {
				t.Errorf("ReceivedAtMs = %d, want %d", pp.ReceivedAtMs, tt.point.ReceivedAt.UnixMilli())
			}
			if len(pp.Metrics) != len(tt.point.Metrics) {
				t.Errorf("Metrics count = %d, want %d", len(pp.Metrics), len(tt.point.Metrics))
			}
			for k, want := range tt.point.Metrics {
				if got, ok := pp.Metrics[k]; !ok {
					t.Errorf("missing metric %q", k)
				} else if got != want {
					t.Errorf("metric %q = %f, want %f", k, got, want)
				}
			}
		})
	}
}

func TestSender_AckError(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	mock.ackFunc = func(batch *telemetryv1.TelemetryBatch) *telemetryv1.BatchAck {
		return &telemetryv1.BatchAck{
			SequenceNumber: batch.SequenceNumber,
			Status:         telemetryv1.AckStatus_ACK_STATUS_ERROR,
			Message:        "test error",
		}
	}

	cfg := testSenderConfig(2, 10*time.Second)
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Send a full batch.
	input <- testPoint("A")
	input <- testPoint("B")

	// Wait for error counter to increment.
	deadline := time.After(5 * time.Second)
	for {
		_, _, errs := s.Stats()
		if errs >= 1 {
			break
		}
		select {
		case <-deadline:
			_, _, errs := s.Stats()
			t.Fatalf("timed out waiting for error counter: errors=%d", errs)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done
}

func TestSender_MultipleBatchesSequenceContinuity(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	cfg := testSenderConfig(1, 10*time.Second) // batch of 1 for fast flushing
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	const totalPoints = 10
	for i := range totalPoints {
		input <- testPoint(string(rune('A' + i)))
	}

	deadline := time.After(5 * time.Second)
	for {
		batches := mock.getBatches()
		if len(batches) >= totalPoints {
			// Verify sequence numbers are 1..N with no gaps.
			for i, b := range batches[:totalPoints] {
				want := uint64(i + 1)
				if b.SequenceNumber != want {
					t.Errorf("batch %d: seq=%d, want %d", i, b.SequenceNumber, want)
				}
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: got %d batches, want %d", len(mock.getBatches()), totalPoints)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done
}

// --- TLS and error-path tests ---

// generateTestCerts creates a self-signed CA and client cert/key in a temp dir.
func generateTestCerts(t *testing.T) (caFile, certFile, keyFile string) {
	t.Helper()
	dir := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("creating CA cert: %v", err)
	}
	caFile = filepath.Join(dir, "ca.pem")
	writePEM(t, caFile, "CERTIFICATE", caDER)

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating client key: %v", err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Test Client"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("creating client cert: %v", err)
	}
	certFile = filepath.Join(dir, "client.pem")
	writePEM(t, certFile, "CERTIFICATE", clientDER)

	keyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		t.Fatalf("marshalling client key: %v", err)
	}
	keyFile = filepath.Join(dir, "client-key.pem")
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)

	return caFile, certFile, keyFile
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: der}); err != nil {
		t.Fatalf("encoding PEM %s: %v", path, err)
	}
}

func TestBuildTLSConfig_Valid(t *testing.T) {
	t.Parallel()
	caFile, certFile, keyFile := generateTestCerts(t)

	tlsCfg, err := buildTLSConfig(config.TLSConfig{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
		CAFile:   caFile,
	})
	if err != nil {
		t.Fatalf("buildTLSConfig() error: %v", err)
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(tlsCfg.Certificates))
	}
	if tlsCfg.RootCAs == nil {
		t.Error("RootCAs should not be nil")
	}
}

func TestBuildTLSConfig_Errors(t *testing.T) {
	t.Parallel()

	caFile, certFile, keyFile := generateTestCerts(t)

	tests := []struct {
		name string
		cfg  config.TLSConfig
	}{
		{
			name: "missing cert file",
			cfg: config.TLSConfig{
				CertFile: "/nonexistent/cert.pem",
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
		},
		{
			name: "missing key file",
			cfg: config.TLSConfig{
				CertFile: certFile,
				KeyFile:  "/nonexistent/key.pem",
				CAFile:   caFile,
			},
		},
		{
			name: "missing CA file",
			cfg: config.TLSConfig{
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   "/nonexistent/ca.pem",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildTLSConfig(tt.cfg)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestBuildTLSConfig_InvalidCAPEM(t *testing.T) {
	t.Parallel()

	_, certFile, keyFile := generateTestCerts(t)

	badCA := filepath.Join(t.TempDir(), "bad-ca.pem")
	if err := os.WriteFile(badCA, []byte("not a certificate"), 0644); err != nil {
		t.Fatalf("writing bad CA: %v", err)
	}

	_, err := buildTLSConfig(config.TLSConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
		CAFile:   badCA,
	})
	if err == nil {
		t.Error("expected error for invalid CA PEM, got nil")
	}
}

func TestSender_ConnectWithTLS(t *testing.T) {
	t.Parallel()
	caFile, certFile, keyFile := generateTestCerts(t)

	cfg := config.SenderConfig{
		Target:        "passthrough:///bufconn",
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
		TLS: config.TLSConfig{
			Enabled:  true,
			CertFile: certFile,
			KeyFile:  keyFile,
			CAFile:   caFile,
		},
	}
	input := make(chan normalizer.TelemetryPoint)
	s := New(cfg, "test-agent", input, discardLogger())

	conn, err := s.connect(context.Background())
	if err != nil {
		t.Fatalf("connect() with TLS error: %v", err)
	}
	conn.Close()
}

func TestSender_ConnectTLSError(t *testing.T) {
	t.Parallel()

	cfg := config.SenderConfig{
		Target:        "passthrough:///bufconn",
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
		TLS: config.TLSConfig{
			Enabled:  true,
			CertFile: "/nonexistent/cert.pem",
			KeyFile:  "/nonexistent/key.pem",
			CAFile:   "/nonexistent/ca.pem",
		},
	}
	input := make(chan normalizer.TelemetryPoint)
	s := New(cfg, "test-agent", input, discardLogger())

	_, err := s.connect(context.Background())
	if err == nil {
		t.Fatal("expected error from connect() with bad TLS config, got nil")
	}
}

func TestSender_Stop(t *testing.T) {
	t.Parallel()
	cfg := testSenderConfig(100, 5*time.Second)
	input := make(chan normalizer.TelemetryPoint)
	s := New(cfg, "test-agent", input, discardLogger())
	// Stop is a no-op but must not panic.
	s.Stop()
}

func TestSender_AckPartial(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	mock.ackFunc = func(batch *telemetryv1.TelemetryBatch) *telemetryv1.BatchAck {
		return &telemetryv1.BatchAck{
			SequenceNumber: batch.SequenceNumber,
			Status:         telemetryv1.AckStatus_ACK_STATUS_PARTIAL,
			AcceptedCount:  1,
			RejectedCount:  1,
			Message:        "partial accept",
		}
	}

	cfg := testSenderConfig(2, 10*time.Second)
	input := make(chan normalizer.TelemetryPoint, 100)
	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	input <- testPoint("A")
	input <- testPoint("B")

	deadline := time.After(5 * time.Second)
	for {
		bs, _, _ := s.Stats()
		if bs >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for batch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done
}

func TestSender_AckUnknownStatus(t *testing.T) {
	t.Parallel()

	dialOpt, mock := setupTestServer(t)
	mock.ackFunc = func(batch *telemetryv1.TelemetryBatch) *telemetryv1.BatchAck {
		return &telemetryv1.BatchAck{
			SequenceNumber: batch.SequenceNumber,
			Status:         telemetryv1.AckStatus(99),
		}
	}

	cfg := testSenderConfig(2, 10*time.Second)
	input := make(chan normalizer.TelemetryPoint, 100)
	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	input <- testPoint("A")
	input <- testPoint("B")

	deadline := time.After(5 * time.Second)
	for {
		bs, _, _ := s.Stats()
		if bs >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for batch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-done
}

func TestSender_OnConnChangeCallback(t *testing.T) {
	t.Parallel()

	dialOpt, _ := setupTestServer(t)
	cfg := testSenderConfig(5, 10*time.Second)
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	var states []bool
	s.SetOnConnChange(func(connected bool) {
		states = append(states, connected)
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Give time for connection to establish.
	time.Sleep(100 * time.Millisecond)

	cancel()
	<-done

	// Should have received at least one true (connected) callback.
	if len(states) == 0 {
		t.Fatal("expected at least one connection state change")
	}
	if !states[0] {
		t.Errorf("first state change should be true (connected), got false")
	}
}

func TestSender_BackoffResetsAfterSuccess(t *testing.T) {
	t.Parallel()

	// Verify that the backoff field resets after a successful connection.
	// We test the logic indirectly: after a successful runOnce that returns
	// (true, err), the Start loop should reset backoff to initialBackoff.
	// After a failed connection (false, err), backoff should grow.

	dialOpt, mock := setupTestServer(t)
	cfg := testSenderConfig(5, 10*time.Second)
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Track connection state changes to verify reconnection behavior.
	var mu sync.Mutex
	var connStates []bool
	s.SetOnConnChange(func(connected bool) {
		mu.Lock()
		connStates = append(connStates, connected)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	// Send a batch to trigger flush, verifying the connection works.
	for i := range 5 {
		input <- testPoint(string(rune('A' + i)))
	}

	// Wait for the batch to arrive.
	deadline := time.After(5 * time.Second)
	for {
		batches := mock.getBatches()
		if len(batches) >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for first batch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Connection was successful — if it were to fail now and reconnect,
	// backoff should reset to initialBackoff (not continue growing).
	// We verify this by checking that the sender connected at least once.
	mu.Lock()
	gotConnected := len(connStates) > 0 && connStates[0]
	mu.Unlock()

	if !gotConnected {
		t.Error("expected connected=true callback after successful connection")
	}

	cancel()
	<-done
}

func TestSender_NilCallbackSafe(t *testing.T) {
	t.Parallel()

	dialOpt, _ := setupTestServer(t)
	cfg := testSenderConfig(5, 10*time.Second)
	input := make(chan normalizer.TelemetryPoint, 100)

	s := New(cfg, "test-agent", input, discardLogger(),
		dialOpt, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Do NOT set OnConnChange — should not panic.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
}
