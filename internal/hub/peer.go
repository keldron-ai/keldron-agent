// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package hub

import (
	"sync"
	"time"
)

// PeerDevice holds device metrics scraped from a peer agent.
type PeerDevice struct {
	DeviceID       string
	DeviceModel    string
	DeviceVendor   string
	BehaviorClass  string
	TemperatureC   float64
	PowerW         float64
	Utilization    float64
	RiskComposite  float64
	RiskSeverity   string // "normal", "warning", "critical"
	MemoryPressure float64
	LastUpdated    time.Time
}

// Peer represents a peer agent on the network.
type Peer struct {
	ID          string
	Address     string
	LastSeen    time.Time
	Healthy     bool
	DeviceCount int
	Devices     []PeerDevice
}

// PeerRegistry manages the set of known peers and their health.
type PeerRegistry struct {
	mu           sync.RWMutex
	peers        map[string]*Peer
	failureCount map[string]int
}

// NewPeerRegistry creates a new peer registry.
func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{
		peers:        make(map[string]*Peer),
		failureCount: make(map[string]int),
	}
}

// AddPeer adds a peer by address. If not present, creates with initial state.
func (r *PeerRegistry) AddPeer(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.peers[address]; !ok {
		r.peers[address] = &Peer{
			ID:       address,
			Address:  address,
			LastSeen: time.Time{},
			Healthy:  false,
		}
	}
}

// RemovePeer removes a peer from the registry.
func (r *PeerRegistry) RemovePeer(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, address)
	delete(r.failureCount, address)
}

// GetPeers returns all peers (copy).
func (r *PeerRegistry) GetPeers() []*Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Peer, 0, len(r.peers))
	for _, p := range r.peers {
		cp := *p
		cp.Devices = append([]PeerDevice(nil), p.Devices...)
		out = append(out, &cp)
	}
	return out
}

// GetHealthyPeers returns only healthy peers.
func (r *PeerRegistry) GetHealthyPeers() []*Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Peer
	for _, p := range r.peers {
		if p.Healthy {
			cp := *p
			cp.Devices = append([]PeerDevice(nil), p.Devices...)
			out = append(out, &cp)
		}
	}
	return out
}

// UpdatePeer updates a peer with fresh devices and marks it healthy.
func (r *PeerRegistry) UpdatePeer(address string, peerID string, devices []PeerDevice) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failureCount[address] = 0
	if p, ok := r.peers[address]; ok {
		p.Healthy = true
		p.LastSeen = time.Now()
		p.Devices = append([]PeerDevice(nil), devices...)
		p.DeviceCount = len(devices)
		if peerID != "" {
			p.ID = peerID
		}
	}
}

// MarkUnhealthy marks a peer as unhealthy and increments failure count.
// Returns the current failure count.
func (r *PeerRegistry) MarkUnhealthy(address string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.peers[address]
	if !ok {
		return 0
	}
	p.Healthy = false
	r.failureCount[address]++
	return r.failureCount[address]
}

// GetFailureCount returns the consecutive failure count for a peer.
func (r *PeerRegistry) GetFailureCount(address string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.failureCount[address]
}
