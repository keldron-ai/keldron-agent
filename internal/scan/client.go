// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const hubTimeout = 5 * time.Second

// FetchFleet fetches the fleet state from the hub API.
// hubAddr may be "host:port" or "http://host:port".
func FetchFleet(hubAddr string) (*FleetResponse, error) {
	url := buildFleetURL(hubAddr)
	client := &http.Client{Timeout: hubTimeout}
	resp, err := client.Get(url)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "dial tcp") {
			return nil, fmt.Errorf("cannot reach hub at %s. Is the agent running in hub mode?", hubAddr)
		}
		if strings.Contains(errStr, "context deadline exceeded") ||
			strings.Contains(errStr, "Client.Timeout") {
			return nil, fmt.Errorf("hub did not respond within 5 seconds")
		}
		return nil, fmt.Errorf("hub request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hub returned status %d", resp.StatusCode)
	}

	var fleet FleetResponse
	if err := json.NewDecoder(resp.Body).Decode(&fleet); err != nil {
		return nil, fmt.Errorf("invalid fleet response: %w", err)
	}

	// Check for empty fleet (hub running but no peers)
	totalDevices := 0
	for _, p := range fleet.Peers {
		totalDevices += len(p.Devices)
	}
	if totalDevices == 0 {
		return &fleet, ErrNoPeers
	}

	return &fleet, nil
}

// ErrNoPeers is returned when the hub is reachable but has no discovered peers/devices.
var ErrNoPeers = errors.New("hub is running but no peers discovered")

// buildFleetURL normalizes hubAddr and returns the fleet API URL.
func buildFleetURL(hubAddr string) string {
	addr := strings.TrimSpace(hubAddr)
	if addr == "" {
		addr = "localhost:9200"
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	return strings.TrimSuffix(addr, "/") + "/api/v1/fleet"
}
