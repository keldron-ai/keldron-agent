// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package hub

import "time"

// FleetState holds the aggregated fleet view (local + peer devices).
type FleetState struct {
	Timestamp    time.Time
	LocalDevices []PeerDevice
	PeerDevices  []PeerDevice
	AllDevices   []PeerDevice
	Peers        []*Peer // Peers from registry (excludes local)
	TotalGPUs    int
	HealthyGPUs  int
	WarningGPUs  int
	CriticalGPUs int
	PeerCount    int
	HealthyPeers int
}

// BuildFleetState merges local devices and peer registry into a FleetState.
func BuildFleetState(local []PeerDevice, registry *PeerRegistry) FleetState {
	var peers []*Peer
	if registry != nil {
		peers = registry.GetPeers()
	}
	var peerDevices []PeerDevice
	healthyPeers := 0
	for _, p := range peers {
		peerDevices = append(peerDevices, p.Devices...)
		if p.Healthy {
			healthyPeers++
		}
	}

	allDevices := make([]PeerDevice, 0, len(local)+len(peerDevices))
	allDevices = append(allDevices, local...)
	allDevices = append(allDevices, peerDevices...)

	var healthy, warning, critical int
	for _, d := range allDevices {
		switch d.RiskSeverity {
		case "critical":
			critical++
		case "warning":
			warning++
		default:
			healthy++
		}
	}

	return FleetState{
		Timestamp:    time.Now(),
		LocalDevices: local,
		PeerDevices:  peerDevices,
		AllDevices:   allDevices,
		Peers:        peers,
		TotalGPUs:    len(allDevices),
		HealthyGPUs:  healthy,
		WarningGPUs:  warning,
		CriticalGPUs: critical,
		PeerCount:    len(peers),
		HealthyPeers: healthyPeers,
	}
}
