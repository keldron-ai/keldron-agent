// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

// FleetResponse matches the hub's GET /api/v1/fleet JSON response.
type FleetResponse struct {
	Timestamp string          `json:"timestamp"`
	Peers     []PeerResponse  `json:"peers"`
	Summary   SummaryResponse `json:"summary"`
}

// PeerResponse represents a peer in the fleet.
type PeerResponse struct {
	ID      string           `json:"id"`
	Address string           `json:"address"`
	Healthy bool             `json:"healthy"`
	Devices []DeviceResponse `json:"devices"`
}

// DeviceResponse represents a device in the fleet.
type DeviceResponse struct {
	DeviceID         string  `json:"device_id"`
	DeviceModel      string  `json:"device_model"`
	TemperatureC     float64 `json:"temperature_c"`
	PowerW           float64 `json:"power_w"`
	Utilization      float64 `json:"utilization"`
	RiskComposite    float64 `json:"risk_composite"`
	RiskSeverity     string  `json:"risk_severity"`
	MemoryUsedBytes  float64 `json:"memory_used_bytes"`
	MemoryTotalBytes float64 `json:"memory_total_bytes"`
}

// SummaryResponse holds fleet summary counts.
type SummaryResponse struct {
	TotalDevices int `json:"total_devices"`
	Healthy      int `json:"healthy"`
	Warning      int `json:"warning"`
	Critical     int `json:"critical"`
	TotalPeers   int `json:"total_peers"`
	HealthyPeers int `json:"healthy_peers"`
}
