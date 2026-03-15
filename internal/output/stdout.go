// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package output

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/normalizer"
)

// StdoutLine is the JSON schema for one line of stdout output.
type StdoutLine struct {
	Timestamp string         `json:"timestamp"`
	Devices   []StdoutDevice `json:"devices"`
	Agent     StdoutAgent    `json:"agent"`
}

// StdoutDevice represents one device in the output.
type StdoutDevice struct {
	DeviceID      string  `json:"device_id"`
	DeviceModel   string  `json:"device_model"`
	TemperatureC  float64 `json:"temperature_c,omitempty"`
	PowerW        float64 `json:"power_w,omitempty"`
	Utilization   float64 `json:"utilization,omitempty"`
	RiskComposite float64 `json:"risk_composite,omitempty"`
	RiskSeverity  string  `json:"risk_severity,omitempty"`
}

// StdoutAgent holds agent metadata.
type StdoutAgent struct {
	Version        string   `json:"version"`
	UptimeSeconds  float64  `json:"uptime_seconds"`
	ActiveAdapters []string `json:"active_adapters"`
}

// Stdout implements Output by printing one JSON line per Update call.
type Stdout struct {
	writer         io.Writer
	version        string
	startedAt      time.Time
	activeAdapters []string
	mu             sync.Mutex
}

// NewStdout creates a Stdout output that writes to w (default os.Stdout).
func NewStdout(w io.Writer, version string, activeAdapters []string) *Stdout {
	if w == nil {
		w = io.Discard
	}
	return &Stdout{
		writer:         w,
		version:        version,
		startedAt:      time.Now(),
		activeAdapters: append([]string(nil), activeAdapters...),
	}
}

// Start is a no-op for Stdout.
func (s *Stdout) Start(_ context.Context) error {
	return nil
}

// Update prints one JSON line with all devices and agent info.
func (s *Stdout) Update(readings []normalizer.TelemetryPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	devices := make([]StdoutDevice, 0, len(readings))
	seenAdapters := make(map[string]bool)

	for _, pt := range readings {
		seenAdapters[pt.AdapterName] = true
		dev := s.pointToDevice(pt)
		devices = append(devices, dev)
	}

	adapters := s.activeAdapters
	if len(adapters) == 0 {
		for a := range seenAdapters {
			adapters = append(adapters, a)
		}
		sort.Strings(adapters)
	}

	line := StdoutLine{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Devices:   devices,
		Agent: StdoutAgent{
			Version:        s.version,
			UptimeSeconds:  time.Since(s.startedAt).Seconds(),
			ActiveAdapters: adapters,
		},
	}

	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(s.writer, string(data))
	return err
}

func (s *Stdout) pointToDevice(pt normalizer.TelemetryPoint) StdoutDevice {
	deviceID := deviceIDFromPoint(pt)
	deviceModel := deviceModelFromPoint(pt)

	dev := StdoutDevice{
		DeviceID:    deviceID,
		DeviceModel: deviceModel,
	}

	if m := pt.Metrics; m != nil {
		if v, ok := m["temperature_c"]; ok {
			dev.TemperatureC = v
		}
		if v, ok := m["power_usage_w"]; ok {
			dev.PowerW = v
		}
		if v, ok := m["gpu_utilization_pct"]; ok {
			dev.Utilization = v / 100
		}
		// Risk placeholders (OSS-003 fills in)
		if v, ok := m["risk_composite"]; ok {
			dev.RiskComposite = v
		}
		if v, ok := m["risk_severity"]; ok {
			dev.RiskSeverity = severityString(v)
		}
	}

	if dev.RiskSeverity == "" {
		dev.RiskSeverity = "normal"
	}

	return dev
}

func deviceIDFromPoint(pt normalizer.TelemetryPoint) string {
	if m := pt.Metrics; m != nil {
		if gpuID, ok := m["gpu_id"]; ok {
			return pt.Source + ":" + fmt.Sprintf("%.0f", gpuID)
		}
	}
	return pt.Source
}

func deviceModelFromPoint(pt normalizer.TelemetryPoint) string {
	// gpu_name is typically a string from adapters, so normalizer drops it (Metrics is float64).
	// OSS-003 or normalizer enhancement could add device_model.
	return "unknown"
}

func severityString(v float64) string {
	switch {
	case v >= 2:
		return "critical"
	case v >= 1:
		return "warning"
	default:
		return "normal"
	}
}

// SetActiveAdapters updates the list of active adapters for the agent section.
func (s *Stdout) SetActiveAdapters(adapters []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeAdapters = append([]string(nil), adapters...)
}

// Close is a no-op for Stdout.
func (s *Stdout) Close() error {
	return nil
}
