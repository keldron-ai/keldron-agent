// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"bytes"
	"strings"
	"testing"
)

func TestAllDevices(t *testing.T) {
	fleet := &FleetResponse{
		Peers: []PeerResponse{
			{Devices: []DeviceResponse{{DeviceID: "a"}, {DeviceID: "b"}}},
			{Devices: []DeviceResponse{{DeviceID: "c"}}},
		},
	}
	devices := AllDevices(fleet)
	if len(devices) != 3 {
		t.Errorf("AllDevices: got %d, want 3", len(devices))
	}
}

func TestFilterAndSortDevices(t *testing.T) {
	devices := []DeviceResponse{
		{DeviceID: "m4-mini-02", RiskComposite: 8, TemperatureC: 44},
		{DeviceID: "m4-mini-01", RiskComposite: 65, TemperatureC: 71},
		{DeviceID: "m4-pro-mbp", RiskComposite: 12, TemperatureC: 52},
	}

	// Default sort: risk descending
	sorted := FilterAndSortDevices(devices, RenderOpts{Sort: SortRisk})
	if sorted[0].DeviceID != "m4-mini-01" {
		t.Errorf("SortRisk: first = %q, want m4-mini-01", sorted[0].DeviceID)
	}

	// Sort by name
	sorted = FilterAndSortDevices(devices, RenderOpts{Sort: SortName})
	if sorted[0].DeviceID != "m4-mini-01" {
		t.Errorf("SortName: first = %q, want m4-mini-01", sorted[0].DeviceID)
	}

	// Sort by temp
	sorted = FilterAndSortDevices(devices, RenderOpts{Sort: SortTemp})
	if sorted[0].DeviceID != "m4-mini-01" {
		t.Errorf("SortTemp: first = %q, want m4-mini-01 (71°C)", sorted[0].DeviceID)
	}

	// Filter
	sorted = FilterAndSortDevices(devices, RenderOpts{DeviceFilter: "mini"})
	if len(sorted) != 2 {
		t.Errorf("Filter mini: got %d, want 2", len(sorted))
	}
}

func TestRenderTable_Empty(t *testing.T) {
	fleet := &FleetResponse{
		Timestamp: "2026-03-17T14:32:07Z",
		Peers:     []PeerResponse{},
	}
	var buf bytes.Buffer
	RenderTable(&buf, fleet, RenderOpts{Quiet: false})
	out := buf.String()
	if !strings.Contains(out, "no peers discovered") {
		t.Errorf("expected empty state message, got: %s", out)
	}
}

func TestRenderTable_WithDevices(t *testing.T) {
	fleet := &FleetResponse{
		Timestamp: "2026-03-17T14:32:07Z",
		Peers: []PeerResponse{
			{
				ID: "local",
				Devices: []DeviceResponse{
					{
						DeviceID:         "m4-pro-mbp",
						DeviceModel:      "M4-Pro",
						TemperatureC:     52,
						PowerW:           45,
						RiskComposite:    12,
						RiskSeverity:     "normal",
						MemoryUsedBytes:  18 * 1024 * 1024 * 1024,
						MemoryTotalBytes: 36 * 1024 * 1024 * 1024,
					},
				},
			},
		},
	}
	var buf bytes.Buffer
	RenderTable(&buf, fleet, RenderOpts{Quiet: true})
	out := buf.String()
	if !strings.Contains(out, "m4-pro-mbp") {
		t.Errorf("expected device name, got: %s", out)
	}
	if !strings.Contains(out, "M4-Pro") {
		t.Errorf("expected model, got: %s", out)
	}
	if !strings.Contains(out, "18/36GB") {
		t.Errorf("expected VRAM format 18/36GB, got: %s", out)
	}
}

func TestRenderJSON(t *testing.T) {
	fleet := &FleetResponse{
		Timestamp: "2026-03-17T14:32:07Z",
		Peers:     []PeerResponse{},
	}
	var buf bytes.Buffer
	if err := RenderJSON(&buf, fleet); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"timestamp"`) {
		t.Errorf("expected JSON with timestamp, got: %s", out)
	}
}
