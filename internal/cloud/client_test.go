// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClient_Send_202(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath, gotKey, gotCT string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-API-Key")
		gotCT = r.Header.Get("Content-Type")
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":1,"rejected":0,"errors":[]}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "kldn_live_test", "agent-1", "1.0.0")
	ctx := context.Background()
	s := Sample{DeviceID: "gpu-0", Timestamp: "2026-03-20T12:00:00Z", CompositeRiskScore: 42, SeverityBand: "normal"}
	if err := c.Send(ctx, []Sample{s}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/telemetry/ingest" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != "kldn_live_test" {
		t.Errorf("X-API-Key = %q", gotKey)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
	var req IngestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(req.Samples) != 1 || req.Samples[0].DeviceID != "gpu-0" {
		t.Errorf("samples = %+v", req.Samples)
	}

	// Buffer cleared after success
	c.bufferMu.Lock()
	blen := len(c.buffer)
	c.bufferMu.Unlock()
	if blen != 0 {
		t.Errorf("buffer len = %d after success, want 0", blen)
	}
}

func TestClient_Send_RetriesAfterFailure(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":2,"rejected":0,"errors":[]}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "key", "a", "v")
	ctx := context.Background()
	s1 := Sample{DeviceID: "a", Timestamp: "2026-03-20T12:00:00Z", CompositeRiskScore: 1, SeverityBand: "normal"}
	if err := c.Send(ctx, []Sample{s1}); err == nil {
		t.Fatal("first Send: want error")
	}

	if err := c.Send(ctx, []Sample{{DeviceID: "b", Timestamp: "2026-03-20T12:00:01Z", CompositeRiskScore: 2, SeverityBand: "normal"}}); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("handler calls = %d, want 2", calls.Load())
	}
}

func TestClient_Send_BufferOverflowDropsOldest(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "key", "a", "v")
	c.MaxBuffer = 2

	ctx := context.Background()
	mk := func(id string) Sample {
		return Sample{DeviceID: id, Timestamp: "2026-03-20T12:00:00Z", CompositeRiskScore: 1, SeverityBand: "normal"}
	}
	_ = c.Send(ctx, []Sample{mk("x1")})
	_ = c.Send(ctx, []Sample{mk("x2")})
	_ = c.Send(ctx, []Sample{mk("x3")})

	c.bufferMu.Lock()
	buf := append([]Sample(nil), c.buffer...)
	c.bufferMu.Unlock()

	if len(buf) != 2 {
		t.Fatalf("buffer len = %d, want 2", len(buf))
	}
	if buf[0].DeviceID != "x2" || buf[1].DeviceID != "x3" {
		t.Fatalf("buffer = %v, want [x2 x3] oldest dropped", buf)
	}
}

func TestClient_Close(t *testing.T) {
	t.Parallel()
	c := NewClient("http://localhost", "k", "a", "v")
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := (*Client)(nil).Close(); err != nil {
		t.Fatal(err)
	}
}
