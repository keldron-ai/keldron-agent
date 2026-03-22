// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package hub

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const mockPrometheusMetrics = `
# HELP keldron_agent_info Agent info (always 1)
# TYPE keldron_agent_info gauge
keldron_agent_info{device_name="ransoms-macbook",version="1.0"} 1
# HELP keldron_gpu_temperature_celsius GPU temperature in Celsius
# TYPE keldron_gpu_temperature_celsius gauge
keldron_gpu_temperature_celsius{adapter="apple_silicon",behavior_class="consumer",device_id="ransoms-mbp.lan:0",device_model="M4-Pro",device_vendor="apple"} 52.3
# HELP keldron_gpu_power_watts GPU power draw in watts
# TYPE keldron_gpu_power_watts gauge
keldron_gpu_power_watts{adapter="apple_silicon",behavior_class="consumer",device_id="ransoms-mbp.lan:0",device_model="M4-Pro",device_vendor="apple"} 0.24
# HELP keldron_gpu_utilization_ratio GPU utilization 0-1
# TYPE keldron_gpu_utilization_ratio gauge
keldron_gpu_utilization_ratio{adapter="apple_silicon",behavior_class="consumer",device_id="ransoms-mbp.lan:0",device_model="M4-Pro",device_vendor="apple"} 0.46
# HELP keldron_risk_composite Composite risk score
# TYPE keldron_risk_composite gauge
keldron_risk_composite{behavior_class="consumer",device_id="ransoms-mbp.lan:0"} 1.48
# HELP keldron_risk_severity 0=normal, 1=active, 2=elevated, 3=warning, 4=critical
# TYPE keldron_risk_severity gauge
keldron_risk_severity{device_id="ransoms-mbp.lan:0"} 0
# HELP keldron_gpu_memory_pressure_ratio GPU memory used/total ratio
# TYPE keldron_gpu_memory_pressure_ratio gauge
keldron_gpu_memory_pressure_ratio{adapter="apple_silicon",behavior_class="consumer",device_id="ransoms-mbp.lan:0",device_model="M4-Pro",device_vendor="apple"} 0.12
`

func TestParseMetricsToPeerDevices(t *testing.T) {
	devices, peerID, err := ParseMetricsToPeerDevices(bytes.NewReader([]byte(mockPrometheusMetrics)))
	if err != nil {
		t.Fatalf("ParseMetricsToPeerDevices: %v", err)
	}
	if peerID != "ransoms-macbook" {
		t.Errorf("peerID = %q, want ransoms-macbook", peerID)
	}
	if len(devices) != 1 {
		t.Fatalf("len(devices) = %d, want 1", len(devices))
	}
	d := devices[0]
	if d.DeviceID != "ransoms-mbp.lan:0" {
		t.Errorf("DeviceID = %q, want ransoms-mbp.lan:0", d.DeviceID)
	}
	if d.DeviceModel != "M4-Pro" {
		t.Errorf("DeviceModel = %q, want M4-Pro", d.DeviceModel)
	}
	if d.TemperatureC != 52.3 {
		t.Errorf("TemperatureC = %v, want 52.3", d.TemperatureC)
	}
	if d.PowerW != 0.24 {
		t.Errorf("PowerW = %v, want 0.24", d.PowerW)
	}
	if d.Utilization != 0.46 {
		t.Errorf("Utilization = %v, want 0.46", d.Utilization)
	}
	if d.RiskComposite != 1.48 {
		t.Errorf("RiskComposite = %v, want 1.48", d.RiskComposite)
	}
	if d.RiskSeverity != "normal" {
		t.Errorf("RiskSeverity = %q, want normal", d.RiskSeverity)
	}
	if d.MemoryPressure != 0.12 {
		t.Errorf("MemoryPressure = %v, want 0.12", d.MemoryPressure)
	}
}

func TestPeerRegistry_AddRemoveUpdateHealth(t *testing.T) {
	r := NewPeerRegistry()
	r.AddPeer("192.168.1.10:9100")
	r.AddPeer("192.168.1.11:9100")

	peers := r.GetPeers()
	if len(peers) != 2 {
		t.Fatalf("GetPeers: got %d, want 2", len(peers))
	}

	r.UpdatePeer("192.168.1.10:9100", "host-a", []PeerDevice{
		{DeviceID: "gpu-0", DeviceModel: "RTX-4090", RiskSeverity: "normal"},
	})
	r.UpdatePeer("192.168.1.11:9100", "host-b", []PeerDevice{
		{DeviceID: "gpu-0", DeviceModel: "RTX-4080", RiskSeverity: "normal"},
	})
	peers = r.GetPeers()
	p := findPeer(peers, "192.168.1.10:9100")
	if p == nil {
		t.Fatal("peer not found")
	}
	if !p.Healthy {
		t.Error("peer should be healthy after UpdatePeer")
	}
	if p.ID != "host-a" {
		t.Errorf("ID = %q, want host-a", p.ID)
	}
	if len(p.Devices) != 1 {
		t.Errorf("Devices = %d, want 1", len(p.Devices))
	}

	r.MarkUnhealthy("192.168.1.10:9100")
	peers = r.GetPeers()
	p = findPeer(peers, "192.168.1.10:9100")
	if p.Healthy {
		t.Error("peer should be unhealthy after MarkUnhealthy")
	}

	healthy := r.GetHealthyPeers()
	if len(healthy) != 1 {
		t.Errorf("GetHealthyPeers: got %d, want 1 (host-b should remain healthy)", len(healthy))
	}

	r.RemovePeer("192.168.1.10:9100")
	peers = r.GetPeers()
	if len(peers) != 1 {
		t.Errorf("after RemovePeer: got %d peers, want 1", len(peers))
	}
}

func TestPeerRegistry_Deduplication(t *testing.T) {
	r := NewPeerRegistry()
	r.AddPeer("192.168.1.50:9100")
	r.AddPeer("192.168.1.50:9100") // duplicate
	peers := r.GetPeers()
	if len(peers) != 1 {
		t.Errorf("AddPeer twice with same address: got %d peers, want 1 (deduplication)", len(peers))
	}
}

func findPeer(peers []*Peer, addr string) *Peer {
	for _, p := range peers {
		if p.Address == addr {
			return p
		}
	}
	return nil
}

func TestBuildFleetState(t *testing.T) {
	local := []PeerDevice{
		{DeviceID: "local-0", RiskSeverity: "normal"},
	}
	r := NewPeerRegistry()
	r.AddPeer("p1:9100")
	r.UpdatePeer("p1:9100", "peer1", []PeerDevice{
		{DeviceID: "p1-gpu0", RiskSeverity: "warning"},
		{DeviceID: "p1-gpu1", RiskSeverity: "critical"},
	})
	r.AddPeer("p2:9100")
	r.UpdatePeer("p2:9100", "peer2", []PeerDevice{
		{DeviceID: "p2-gpu0", RiskSeverity: "normal"},
	})

	state := BuildFleetState(local, r)
	if state.TotalGPUs != 4 {
		t.Errorf("TotalGPUs = %d, want 4", state.TotalGPUs)
	}
	if state.HealthyGPUs != 2 {
		t.Errorf("HealthyGPUs = %d, want 2", state.HealthyGPUs)
	}
	if state.WarningGPUs != 1 {
		t.Errorf("WarningGPUs = %d, want 1", state.WarningGPUs)
	}
	if state.CriticalGPUs != 1 {
		t.Errorf("CriticalGPUs = %d, want 1", state.CriticalGPUs)
	}
	if state.PeerCount != 2 {
		t.Errorf("PeerCount = %d, want 2", state.PeerCount)
	}
	if state.HealthyPeers != 2 {
		t.Errorf("HealthyPeers = %d, want 2", state.HealthyPeers)
	}
}

func TestSeverityFromFloat(t *testing.T) {
	tests := []struct {
		v    float64
		want string
	}{
		{-1, "critical"},
		{-0.5, "critical"},
		{0, "normal"},
		{0.5, "normal"},
		{1, "active"},
		{1.5, "active"},
		{2, "elevated"},
		{3, "warning"},
		{4, "critical"},
		{5, "critical"},
	}
	for _, tt := range tests {
		got := severityFromFloat(tt.v)
		if got != tt.want {
			t.Errorf("severityFromFloat(%v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestFleetAPI_Endpoints(t *testing.T) {
	getState := func() FleetState {
		return BuildFleetState(
			[]PeerDevice{{DeviceID: "local-0", DeviceModel: "M4-Pro", RiskSeverity: "normal"}},
			func() *PeerRegistry {
				r := NewPeerRegistry()
				r.AddPeer("p1:9100")
				r.UpdatePeer("p1:9100", "peer1", []PeerDevice{
					{DeviceID: "p1-gpu0", DeviceModel: "RTX-4090", RiskSeverity: "normal"},
				})
				return r
			}(),
		)
	}
	api := NewFleetAPI(getState)
	handler := api.Handler()

	tests := []struct {
		path     string
		wantCode int
		wantSub  string
	}{
		{"/api/v1/fleet", 200, `"peers"`},
		{"/api/v1/fleet/devices", 200, `"device_id"`},
		{"/api/v1/fleet/peers", 200, `"id"`},
		{"/healthz", 200, `"mode":"hub"`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if !strings.Contains(rec.Body.String(), tt.wantSub) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantSub)
			}
		})
	}
}
