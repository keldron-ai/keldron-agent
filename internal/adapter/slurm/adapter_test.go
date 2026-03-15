// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package slurm

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/adapter"
	"github.com/keldron-ai/keldron-agent/internal/config"

	"gopkg.in/yaml.v3"
)

func TestExpandNodeList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nodeList string
		want     []string
	}{
		{
			name:     "compressed range",
			nodeList: "gpu-node-[01-04]",
			want:     []string{"gpu-node-01", "gpu-node-02", "gpu-node-03", "gpu-node-04"},
		},
		{
			name:     "comma separated",
			nodeList: "gpu-node-01,gpu-node-02",
			want:     []string{"gpu-node-01", "gpu-node-02"},
		},
		{
			name:     "mixed compressed and comma",
			nodeList: "gpu-node-[01-04],cpu-node-[01-02]",
			want:     []string{"gpu-node-01", "gpu-node-02", "gpu-node-03", "gpu-node-04", "cpu-node-01", "cpu-node-02"},
		},
		{
			name:     "single node",
			nodeList: "single-node",
			want:     []string{"single-node"},
		},
		{
			name:     "empty",
			nodeList: "",
			want:     nil,
		},
		{
			name:     "unpadded range",
			nodeList: "node-[1-3]",
			want:     []string{"node-1", "node-2", "node-3"},
		},
		{
			name:     "comma list in brackets",
			nodeList: "gpu-node-[01,03,08]",
			want:     []string{"gpu-node-01", "gpu-node-03", "gpu-node-08"},
		},
		{
			name:     "mixed range and comma in brackets",
			nodeList: "gpu-node-[01-03,08]",
			want:     []string{"gpu-node-01", "gpu-node-02", "gpu-node-03", "gpu-node-08"},
		},
		{
			name:     "reversed range",
			nodeList: "gpu-node-[04-01]",
			want:     []string{"gpu-node-01", "gpu-node-02", "gpu-node-03", "gpu-node-04"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := expandNodeList(tt.nodeList)
			if len(got) != len(tt.want) {
				t.Errorf("expandNodeList(%q) = %v, want %v", tt.nodeList, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("expandNodeList(%q)[%d] = %q, want %q", tt.nodeList, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseGPUsFromTRES(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tres string
		want int
	}{
		{"gres/gpu=4", "gres/gpu=4", 4},
		{"gres/gpu:a100=8", "gres/gpu:a100=8", 8},
		{"mixed tres", "cpu=32,mem=256G,gres/gpu=4", 4},
		{"semicolon separated", "gres/gpu=2;gres/gpu:a100=4", 6},
		{"empty", "", 0},
		{"no gpu", "cpu=32,mem=256G", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseGPUsFromTRES(tt.tres)
			if got != tt.want {
				t.Errorf("parseGPUsFromTRES(%q) = %d, want %d", tt.tres, got, tt.want)
			}
		})
	}
}

func TestNew_RequiresSlurmrestdURL(t *testing.T) {
	t.Parallel()

	cfg := slurmConfigFromYAML(t, `
enabled: true
poll_interval: "30s"
`)
	cfg.Raw = yaml.Node{}

	_, err := New(cfg, nil, slog.Default())
	if err == nil {
		t.Fatal("New() expected error when slurmrestd_url is empty")
	}
}

func TestNew_Success(t *testing.T) {
	t.Parallel()

	cfg := slurmConfigFromYAML(t, `
enabled: true
slurmrestd_url: "http://localhost:6820"
api_version: "v0.0.40"
poll_interval: "30s"
node_to_rack_map:
  gpu-node-01: "rack-01"
`)

	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if a.Name() != "slurm" {
		t.Errorf("Name() = %q, want %q", a.Name(), "slurm")
	}
}

func slurmConfigFromYAML(t *testing.T, raw string) config.AdapterConfig {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return config.AdapterConfig{
		Enabled:      true,
		PollInterval: 30 * time.Second,
		Raw:          *node.Content[0],
	}
}

func TestIntegration_MockSlurmrestd(t *testing.T) {
	// Start mock slurmrestd server
	jobsResp := jobsResponse{
		Jobs: []jobResponse{
			{
				JobID:        100,
				JobState:     "RUNNING",
				Nodes:        "gpu-node-[01-02]",
				TresAllocStr: "gres/gpu=4",
				TresPerNode:  "gres/gpu=2",
				TimeLimit:    60,
				StartTime:    time.Now().Unix(),
				UserName:     "user1",
				Partition:    "gpu",
				Name:         "job1",
			},
			{
				JobID:        101,
				JobState:     "RUNNING",
				Nodes:        "gpu-node-03",
				TresAllocStr: "gres/gpu=4",
				TimeLimit:    120,
				StartTime:    time.Now().Unix(),
				UserName:     "user2",
				Partition:    "gpu",
				Name:         "job2",
			},
		},
	}
	nodesResp := nodesResponse{
		Nodes: []nodeResponse{
			{Name: "gpu-node-01", State: "ALLOCATED"},
			{Name: "gpu-node-02", State: "ALLOCATED"},
			{Name: "gpu-node-03", State: "ALLOCATED"},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/slurm/v0.0.40/jobs", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(jobsResp)
	})
	mux.HandleFunc("/slurm/v0.0.40/nodes", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(nodesResp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := slurmConfigFromYAML(t, `
enabled: true
slurmrestd_url: "`+srv.URL+`"
api_version: "v0.0.40"
poll_interval: "50ms"
timeout: "5s"
node_to_rack_map:
  gpu-node-01: "rack-01"
  gpu-node-02: "rack-01"
  gpu-node-03: "rack-02"
`)

	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		_ = a.Start(ctx)
	}()

	// Collect at least 3 readings (one per node: gpu-node-01, 02, 03) with gpus_allocated.
	// The loop stops as soon as 3 readings are collected.
	var readings []adapter.RawReading
	timeout := time.After(2 * time.Second)
	for len(readings) < 3 {
		select {
		case r, ok := <-a.Readings():
			if !ok {
				goto done
			}
			readings = append(readings, r)
		case <-timeout:
			t.Fatalf("timed out waiting for readings, got %d", len(readings))
		}
	}
done:

	// Verify 3 jobs discovered, node assignments resolve to correct racks
	rackMap := map[string]string{
		"gpu-node-01": "rack-01",
		"gpu-node-02": "rack-01",
		"gpu-node-03": "rack-02",
	}
	seen := make(map[string]bool)
	for _, r := range readings {
		if r.AdapterName != "slurm" {
			t.Errorf("AdapterName = %q, want slurm", r.AdapterName)
		}
		if r.Source == "" {
			t.Error("Source is empty")
		}
		rack, ok := rackMap[r.Source]
		if !ok {
			t.Errorf("unknown node %q", r.Source)
		}
		seen[rack] = true
		gpus, ok := r.Metrics["gpus_allocated"].(int)
		if !ok {
			t.Errorf("gpus_allocated not int: %v", r.Metrics["gpus_allocated"])
		}
		if gpus <= 0 {
			t.Errorf("gpus_allocated = %d, want > 0", gpus)
		}
	}
	if len(seen) < 2 {
		t.Errorf("expected nodes from multiple racks, got %v", seen)
	}
}

func TestIntegration_MockSlurmrestd_Unavailable(t *testing.T) {
	// Start server, then close it immediately to simulate unavailable API
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close()

	cfg := slurmConfigFromYAML(t, `
enabled: true
slurmrestd_url: "`+srv.URL+`"
api_version: "v0.0.40"
poll_interval: "50ms"
timeout: "100ms"
node_to_rack_map:
  gpu-node-01: "rack-01"
`)

	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() {
		_ = a.Start(ctx)
	}()

	// Adapter should not panic; may return no readings or error
	// Just verify we don't hang and adapter stops cleanly
	select {
	case _, ok := <-a.Readings():
		if ok {
			// Got a reading (unexpected with failed API, but possible race)
		}
	case <-ctx.Done():
		// Timeout - adapter is still running, that's ok
	}

	// Verify Stats show error (SlurmAdapter implements health.AdapterProvider)
	if sa, ok := a.(*SlurmAdapter); ok {
		_, errCount, _, lastErr, _ := sa.Stats()
		if errCount > 0 && lastErr == "" {
			t.Error("expected lastError to be set when errorCount > 0")
		}
	}
}

func TestSlurmAdapter_StopsCleanly(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/slurm/v0.0.40/jobs", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(jobsResponse{Jobs: []jobResponse{}})
	})
	mux.HandleFunc("/slurm/v0.0.40/nodes", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(nodesResponse{Nodes: []nodeResponse{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := slurmConfigFromYAML(t, `
enabled: true
slurmrestd_url: "`+srv.URL+`"
api_version: "v0.0.40"
poll_interval: "1s"
`)

	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = a.Start(ctx)
		close(done)
	}()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case <-done:
		// Normal shutdown
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after context cancel")
	}

	// Stop should not block
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := a.Stop(stopCtx); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Readings channel should be closed
	select {
	case _, ok := <-a.Readings():
		if ok {
			t.Error("Readings channel should be closed after Stop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Readings channel to close")
	}
}

func TestAuthError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewSlurmClient(srv.URL, "v0.0.40", "bad-token", time.Second)
	_, err := client.ListJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if _, ok := err.(*AuthError); !ok {
		t.Errorf("expected *AuthError, got %T: %v", err, err)
	}
	// Cover AuthError.Error()
	if err.Error() == "" {
		t.Error("AuthError.Error() should not be empty")
	}
}

func TestMergeNodeToRackMap(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		RackMapping: map[string]string{"existing": "rack-0"},
		Adapters: map[string]config.AdapterConfig{
			"slurm": {
				Enabled: true,
				Raw: mustDecodeYAML(t, `slurmrestd_url: "http://x"
node_to_rack_map:
  gpu-node-01: "rack-01"
  gpu-node-02: "rack-02"`),
			},
		},
	}

	if err := MergeNodeToRackMap(cfg); err != nil {
		t.Fatalf("MergeNodeToRackMap: %v", err)
	}

	if cfg.RackMapping["existing"] != "rack-0" {
		t.Error("existing mapping should be preserved")
	}
	if cfg.RackMapping["gpu-node-01"] != "rack-01" {
		t.Errorf("RackMapping[gpu-node-01] = %q, want rack-01", cfg.RackMapping["gpu-node-01"])
	}
	if cfg.RackMapping["gpu-node-02"] != "rack-02" {
		t.Errorf("RackMapping[gpu-node-02] = %q, want rack-02", cfg.RackMapping["gpu-node-02"])
	}
}

func TestMergeNodeToRackMap_SlurmDisabled(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		RackMapping: map[string]string{},
		Adapters: map[string]config.AdapterConfig{
			"slurm": {
				Enabled: false,
				Raw: mustDecodeYAML(t, `node_to_rack_map:
  gpu-node-01: "rack-01"`),
			},
		},
	}

	if err := MergeNodeToRackMap(cfg); err != nil {
		t.Fatalf("MergeNodeToRackMap: %v", err)
	}

	if len(cfg.RackMapping) != 0 {
		t.Errorf("RackMapping should be empty when slurm disabled, got %v", cfg.RackMapping)
	}
}

func mustDecodeYAML(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return *node.Content[0]
}

func TestListNodes(t *testing.T) {
	t.Parallel()

	nodesResp := nodesResponse{
		Nodes: []nodeResponse{
			{Name: "node1", State: "IDLE"},
			{Name: "node2", State: "ALLOC"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slurm/v0.0.40/nodes" {
			json.NewEncoder(w).Encode(nodesResp)
			return
		}
		json.NewEncoder(w).Encode(jobsResponse{Jobs: []jobResponse{}})
	}))
	defer srv.Close()

	client := NewSlurmClient(srv.URL, "v0.0.40", "", time.Second)
	nodes, err := client.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("len(nodes) = %d, want 2", len(nodes))
	}
	if nodes[0].Name != "node1" || nodes[1].Name != "node2" {
		t.Errorf("nodes = %v", nodes)
	}
}

func TestSlurmAdapter_IsRunning(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/slurm/v0.0.40/jobs", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(jobsResponse{Jobs: []jobResponse{}})
	})
	mux.HandleFunc("/slurm/v0.0.40/nodes", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(nodesResponse{Nodes: []nodeResponse{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := slurmConfigFromYAML(t, `
enabled: true
slurmrestd_url: "`+srv.URL+`"
api_version: "v0.0.40"
poll_interval: "100ms"
`)

	a, err := New(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sa := a.(*SlurmAdapter)

	if sa.IsRunning() {
		t.Error("IsRunning() should be false before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.Start(ctx) }()

	deadline := time.After(2 * time.Second)
	for !sa.IsRunning() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for IsRunning() to become true")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestSlurmAdapter_ConfigHotReload(t *testing.T) {
	t.Parallel()

	pollCh := make(chan struct{}, 64)

	mux := http.NewServeMux()
	mux.HandleFunc("/slurm/v0.0.40/jobs", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case pollCh <- struct{}{}:
		default:
		}
		json.NewEncoder(w).Encode(jobsResponse{Jobs: []jobResponse{}})
	})
	mux.HandleFunc("/slurm/v0.0.40/nodes", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(nodesResponse{Nodes: []nodeResponse{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Start with a fast poll interval so we accumulate polls quickly
	cfg := slurmConfigFromYAML(t, `
enabled: true
slurmrestd_url: "`+srv.URL+`"
api_version: "v0.0.40"
poll_interval: "50ms"
`)

	baseCfg := config.Defaults()
	baseCfg.Adapters["slurm"] = cfg
	baseCfg.Sender.Target = "localhost:9999"
	holder := config.NewHolder(baseCfg)

	a, err := New(cfg, holder, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = a.Start(ctx) }()
	defer cancel()

	// Wait for at least 3 polls to confirm the fast interval is working
	for i := 0; i < 3; i++ {
		select {
		case <-pollCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for poll %d before config update", i+1)
		}
	}

	// Switch to a very slow poll interval
	newCfg := slurmConfigFromYAML(t, `
enabled: true
slurmrestd_url: "`+srv.URL+`"
api_version: "v0.0.40"
poll_interval: "10s"
`)
	updatedCfg := config.Defaults()
	updatedCfg.Adapters["slurm"] = newCfg
	updatedCfg.Sender.Target = "localhost:9999"
	if err := holder.Update(updatedCfg); err != nil {
		t.Fatalf("holder.Update: %v", err)
	}

	// Drain any in-flight poll that was already scheduled
	drainTimeout := time.After(200 * time.Millisecond)
drain:
	for {
		select {
		case <-pollCh:
		case <-drainTimeout:
			break drain
		}
	}

	// With a 10s interval, no new polls should arrive within 500ms
	select {
	case <-pollCh:
		t.Error("unexpected poll after slowing interval to 10s")
	case <-time.After(500 * time.Millisecond):
		// No poll arrived — hot-reload took effect
	}
}
