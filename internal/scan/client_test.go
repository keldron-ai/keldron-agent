// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchFleet_Success(t *testing.T) {
	fleet := FleetResponse{
		Timestamp: "2026-03-17T14:32:07Z",
		Peers: []PeerResponse{
			{
				ID:      "local",
				Address: "local",
				Healthy: true,
				Devices: []DeviceResponse{
					{
						DeviceID:      "m4-pro-mbp",
						DeviceModel:   "M4-Pro",
						TemperatureC:  52,
						PowerW:        45,
						RiskComposite: 12,
						RiskSeverity:  "normal",
					},
				},
			},
		},
		Summary: SummaryResponse{
			TotalDevices: 1,
			Healthy:      1,
		},
	}
	body, _ := json.Marshal(fleet)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet" {
			t.Errorf("path = %q, want /api/v1/fleet", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	got, err := FetchFleet(srv.URL[7:]) // strip "http://"
	if err != nil {
		t.Fatalf("FetchFleet: %v", err)
	}
	if got.Timestamp != fleet.Timestamp {
		t.Errorf("Timestamp = %q, want %q", got.Timestamp, fleet.Timestamp)
	}
	if len(got.Peers) != 1 || len(got.Peers[0].Devices) != 1 {
		t.Errorf("expected 1 peer with 1 device, got %d peers", len(got.Peers))
	}
	if got.Peers[0].Devices[0].DeviceID != "m4-pro-mbp" {
		t.Errorf("DeviceID = %q, want m4-pro-mbp", got.Peers[0].Devices[0].DeviceID)
	}
}

func TestFetchFleet_NoPeers(t *testing.T) {
	fleet := FleetResponse{
		Timestamp: "2026-03-17T14:32:07Z",
		Peers:     []PeerResponse{},
		Summary:   SummaryResponse{TotalDevices: 0},
	}
	body, _ := json.Marshal(fleet)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	got, err := FetchFleet(srv.URL[7:])
	if !errors.Is(err, ErrNoPeers) {
		t.Errorf("err = %v, want ErrNoPeers", err)
	}
	if got == nil {
		t.Fatal("fleet should not be nil when ErrNoPeers")
	}
}

func TestFetchFleet_Unreachable(t *testing.T) {
	_, err := FetchFleet("127.0.0.1:19999") // unlikely to be listening
	if err == nil {
		t.Fatal("expected error for unreachable hub")
	}
	if !errors.Is(err, ErrNoPeers) {
		// Should be connection error, not ErrNoPeers
		if errors.Is(err, ErrNoPeers) {
			t.Error("should not be ErrNoPeers for connection failure")
		}
	}
}

func TestBuildFleetURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"localhost:9200", "http://localhost:9200/api/v1/fleet"},
		{"192.168.1.100:9200", "http://192.168.1.100:9200/api/v1/fleet"},
		{"http://localhost:9200", "http://localhost:9200/api/v1/fleet"},
		{"https://hub.example.com:9200", "https://hub.example.com:9200/api/v1/fleet"},
	}
	for _, tt := range tests {
		got := buildFleetURL(tt.in)
		if got != tt.want {
			t.Errorf("buildFleetURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
