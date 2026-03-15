// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package hub

import (
	"encoding/json"
	"log"
	"net/http"
)

// FleetStateProvider returns the current fleet state.
type FleetStateProvider func() FleetState

// FleetAPI serves the fleet REST endpoints.
type FleetAPI struct {
	getState FleetStateProvider
}

// NewFleetAPI creates a FleetAPI that uses the given provider for state.
func NewFleetAPI(getState FleetStateProvider) *FleetAPI {
	return &FleetAPI{getState: getState}
}

// Handler returns an http.Handler that serves fleet endpoints.
func (a *FleetAPI) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/fleet", a.handleFleet)
	mux.HandleFunc("GET /api/v1/fleet/devices", a.handleFleetDevices)
	mux.HandleFunc("GET /api/v1/fleet/peers", a.handleFleetPeers)
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	return mux
}

// fleetResponse is the JSON structure for GET /api/v1/fleet.
type fleetResponse struct {
	Timestamp string          `json:"timestamp"`
	Peers     []peerResponse  `json:"peers"`
	Summary   summaryResponse `json:"summary"`
}

type peerResponse struct {
	ID      string           `json:"id"`
	Address string           `json:"address"`
	Healthy bool             `json:"healthy"`
	Devices []deviceResponse `json:"devices"`
}

type deviceResponse struct {
	DeviceID      string  `json:"device_id"`
	DeviceModel   string  `json:"device_model"`
	TemperatureC  float64 `json:"temperature_c"`
	PowerW        float64 `json:"power_w"`
	Utilization   float64 `json:"utilization"`
	RiskComposite float64 `json:"risk_composite"`
	RiskSeverity  string  `json:"risk_severity"`
}

type summaryResponse struct {
	TotalDevices int `json:"total_devices"`
	Healthy      int `json:"healthy"`
	Warning      int `json:"warning"`
	Critical     int `json:"critical"`
	TotalPeers   int `json:"total_peers"`
	HealthyPeers int `json:"healthy_peers"`
}

// buildPeerList constructs the peer list from a FleetState (local + registry peers).
func buildPeerList(state FleetState) []peerResponse {
	peerList := make([]peerResponse, 0, 1+len(state.Peers))
	peerList = append(peerList, peerResponse{
		ID:      "local",
		Address: "local",
		Healthy: true,
		Devices: devicesToResponse(state.LocalDevices),
	})
	for _, p := range state.Peers {
		peerList = append(peerList, peerResponse{
			ID:      p.ID,
			Address: p.Address,
			Healthy: p.Healthy,
			Devices: devicesToResponse(p.Devices),
		})
	}
	return peerList
}

func (a *FleetAPI) handleFleet(w http.ResponseWriter, r *http.Request) {
	state := a.getState()
	peerList := buildPeerList(state)

	healthyPeerCount := 0
	for _, p := range peerList {
		if p.Healthy {
			healthyPeerCount++
		}
	}

	resp := fleetResponse{
		Timestamp: state.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		Peers:     peerList,
		Summary: summaryResponse{
			TotalDevices: state.TotalGPUs,
			Healthy:      state.HealthyGPUs,
			Warning:      state.WarningGPUs,
			Critical:     state.CriticalGPUs,
			TotalPeers:   len(peerList),
			HealthyPeers: healthyPeerCount,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("fleet: failed to encode response for %s: %v", r.URL.Path, err)
	}
}

func devicesToResponse(devices []PeerDevice) []deviceResponse {
	out := make([]deviceResponse, len(devices))
	for i, d := range devices {
		out[i] = deviceResponse{
			DeviceID:      d.DeviceID,
			DeviceModel:   d.DeviceModel,
			TemperatureC:  d.TemperatureC,
			PowerW:        d.PowerW,
			Utilization:   d.Utilization,
			RiskComposite: d.RiskComposite,
			RiskSeverity:  d.RiskSeverity,
		}
	}
	return out
}

func (a *FleetAPI) handleFleetDevices(w http.ResponseWriter, r *http.Request) {
	state := a.getState()
	devices := devicesToResponse(state.AllDevices)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(devices); err != nil {
		log.Printf("fleet: failed to encode response for %s: %v", r.URL.Path, err)
	}
}

func (a *FleetAPI) handleFleetPeers(w http.ResponseWriter, r *http.Request) {
	state := a.getState()
	peerList := buildPeerList(state)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(peerList); err != nil {
		log.Printf("fleet: failed to encode response for %s: %v", r.URL.Path, err)
	}
}

func (a *FleetAPI) handleHealthz(w http.ResponseWriter, r *http.Request) {
	state := a.getState()
	peerCount := 1 + len(state.Peers) // local + registry peers

	status := "healthy"
	var issues []string
	if state.TotalGPUs == 0 {
		status = "degraded"
		issues = append(issues, "no devices detected")
	}
	if state.CriticalGPUs > 0 {
		status = "degraded"
		issues = append(issues, "critical devices present")
	}

	resp := map[string]interface{}{
		"status":  status,
		"mode":    "hub",
		"peers":   peerCount,
		"devices": state.TotalGPUs,
	}
	if len(issues) > 0 {
		resp["issues"] = issues
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("fleet: failed to encode response for %s: %v", r.URL.Path, err)
	}
}
