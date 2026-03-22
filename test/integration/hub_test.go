// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

//go:build integration

package integration

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/hub"
)

const mockPeerMetrics = `
# HELP keldron_agent_info Agent info (always 1)
# TYPE keldron_agent_info gauge
keldron_agent_info{device_name="gpu-workstation",version="1.0"} 1
# HELP keldron_gpu_temperature_celsius GPU temperature in Celsius
# TYPE keldron_gpu_temperature_celsius gauge
keldron_gpu_temperature_celsius{adapter="dcgm",behavior_class="datacenter",device_id="gpu-0",device_model="RTX-4090",device_vendor="nvidia"} 72.1
# HELP keldron_gpu_power_watts GPU power draw in watts
# TYPE keldron_gpu_power_watts gauge
keldron_gpu_power_watts{adapter="dcgm",behavior_class="datacenter",device_id="gpu-0",device_model="RTX-4090",device_vendor="nvidia"} 350
# HELP keldron_gpu_utilization_ratio GPU utilization 0-1
# TYPE keldron_gpu_utilization_ratio gauge
keldron_gpu_utilization_ratio{adapter="dcgm",behavior_class="datacenter",device_id="gpu-0",device_model="RTX-4090",device_vendor="nvidia"} 0.95
# HELP keldron_risk_composite Composite risk score
# TYPE keldron_risk_composite gauge
keldron_risk_composite{behavior_class="datacenter",device_id="gpu-0"} 34.2
# HELP keldron_risk_severity 0=normal, 1=active, 2=elevated, 3=warning, 4=critical
# TYPE keldron_risk_severity gauge
keldron_risk_severity{device_id="gpu-0"} 0
# HELP keldron_gpu_memory_pressure_ratio GPU memory used/total ratio
# TYPE keldron_gpu_memory_pressure_ratio gauge
keldron_gpu_memory_pressure_ratio{adapter="dcgm",behavior_class="datacenter",device_id="gpu-0",device_model="RTX-4090",device_vendor="nvidia"} 0.8
`

func TestHub_ScrapeMockPeer(t *testing.T) {
	// Start mock peer server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(mockPeerMetrics))
	})
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go srv.Serve(ln)
	addr := ln.Addr().String()

	// Create registry and scraper
	registry := hub.NewPeerRegistry()
	registry.AddPeer(addr)
	scraper := hub.NewScraper(100*time.Millisecond, registry, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	devices, peerID, err := scraper.ScrapePeer(ctx, addr)
	if err != nil {
		t.Fatalf("ScrapePeer: %v", err)
	}
	if peerID != "gpu-workstation" {
		t.Errorf("peerID = %q, want gpu-workstation", peerID)
	}
	if len(devices) != 1 {
		t.Fatalf("len(devices) = %d, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "gpu-0" {
		t.Errorf("DeviceID = %q, want gpu-0", d.DeviceID)
	}
	if d.DeviceModel != "RTX-4090" {
		t.Errorf("DeviceModel = %q, want RTX-4090", d.DeviceModel)
	}
	if d.TemperatureC != 72.1 {
		t.Errorf("TemperatureC = %v, want 72.1", d.TemperatureC)
	}
	if d.PowerW != 350 {
		t.Errorf("PowerW = %v, want 350", d.PowerW)
	}

	_ = srv.Shutdown(context.Background())
}
