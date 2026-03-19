// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/keldron-ai/keldron-agent/internal/api"
	"github.com/keldron-ai/keldron-agent/internal/scoring"
)

const prometheusTimeout = 5 * time.Second

// PrometheusData holds parsed Prometheus metrics for dashboard rendering.
// Used when the HTTP API is unavailable (legacy agent).
type PrometheusData struct {
	Device    api.DeviceInfo
	Telemetry api.TelemetryInfo
	Risk      api.RiskResponse
	Agent     api.AgentInfo
}

// FetchFromPrometheus fetches and parses /metrics from the given host and port.
// Returns data suitable for dashboard rendering, or an error.
func FetchFromPrometheus(host string, port int) (*PrometheusData, error) {
	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	url := "http://" + hostPort + "/metrics"
	client := &http.Client{Timeout: prometheusTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("prometheus request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned %d", resp.StatusCode)
	}

	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse metrics: %w", err)
	}

	return parsePrometheusToDashboard(families)
}

func parsePrometheusToDashboard(families map[string]*dto.MetricFamily) (*PrometheusData, error) {
	getLabel := func(labels []*dto.LabelPair, name string) string {
		for _, lp := range labels {
			if lp.Name != nil && *lp.Name == name && lp.Value != nil {
				return *lp.Value
			}
		}
		return ""
	}

	// Use first device we find; for single-device agents there's typically one
	deviceID := ""
	deviceModel := ""
	behaviorClass := "consumer_active_cooled"

	// Agent info
	version := "unknown"
	deviceName := "unknown"
	if mf, ok := families["keldron_agent_info"]; ok && mf != nil && len(mf.Metric) > 0 {
		for _, lp := range mf.Metric[0].Label {
			if lp.Name != nil && lp.Value != nil {
				switch *lp.Name {
				case "version":
					version = *lp.Value
				case "device_name":
					deviceName = *lp.Value
				case "device_id":
					deviceID = *lp.Value
				}
			}
		}
		if deviceName == "unknown" && deviceID != "" {
			deviceName = deviceID
		}
	}

	// Sort family names for deterministic iteration order
	familyNames := make([]string, 0, len(families))
	for name := range families {
		familyNames = append(familyNames, name)
	}
	sort.Strings(familyNames)

	// Collect metrics - take first metric of each type (single device)
	var tempC, powerW, utilRatio, memUsed, memTotal, riskComposite float64
	var riskThermal, riskPower, riskVolatility, riskFleetPenalty float64
	riskSeverity := "normal"
	uptime := 0.0

	// Single pass: extract device metadata and metric values in sorted order
	for _, name := range familyNames {
		mf := families[name]
		if mf == nil || len(mf.Metric) == 0 {
			continue
		}
		m := mf.Metric[0]
		// Metadata extraction
		if deviceID == "" {
			if id := getLabel(m.Label, "device_id"); id != "" {
				deviceID = id
			}
		}
		if deviceModel == "" {
			if dm := getLabel(m.Label, "device_model"); dm != "" {
				deviceModel = dm
			}
		}
		if bc := getLabel(m.Label, "behavior_class"); bc != "" && behaviorClass == "consumer_active_cooled" {
			behaviorClass = bc
		}
		// Read metric value (Gauge, Counter, or Untyped)
		v := float64(0)
		if m.Gauge != nil {
			v = m.Gauge.GetValue()
		} else if m.Counter != nil {
			v = m.Counter.GetValue()
		} else if m.Untyped != nil {
			v = m.Untyped.GetValue()
		}

		switch name {
		case "keldron_gpu_temperature_celsius":
			tempC = v
		case "keldron_gpu_power_watts":
			powerW = v
		case "keldron_gpu_utilization_ratio":
			utilRatio = v * 100
		case "keldron_gpu_memory_used_bytes":
			memUsed = v
		case "keldron_gpu_memory_total_bytes":
			memTotal = v
		case "keldron_risk_composite":
			riskComposite = v
		case "keldron_risk_thermal":
			riskThermal = v
		case "keldron_risk_power":
			riskPower = v
		case "keldron_risk_volatility":
			riskVolatility = v
		case "keldron_risk_fleet_penalty":
			riskFleetPenalty = v
		case "keldron_risk_severity":
			switch int(v) {
			case 2:
				riskSeverity = "critical"
			case 1:
				riskSeverity = "warning"
			default:
				riskSeverity = "normal"
			}
		case "keldron_device_uptime_seconds":
			uptime = v
		}
	}
	if deviceID == "" && deviceModel != "" {
		deviceID = deviceModel + ":0"
	}
	if deviceID == "" {
		deviceID = "default"
	}
	if deviceName == "unknown" && deviceID != "" {
		deviceName = deviceID
	}

	memPct := 0.0
	if memTotal > 0 {
		memPct = memUsed / memTotal * 100
	}

	// Use authoritative thresholds from scoring engine
	thresholds, ok := scoring.SeverityThresholds[behaviorClass]
	if !ok {
		thresholds = scoring.SeverityThresholds["consumer_active_cooled"]
	}
	warning, critical := thresholds[0], thresholds[1]

	// Use authoritative weights from scoring engine
	thermalWeight, powerWeight, volWeight, corrWeight := scoring.W_THERMAL, scoring.W_POWER, scoring.W_VOLATILITY, scoring.W_CORRELATED

	return &PrometheusData{
		Device: api.DeviceInfo{
			Hostname:      deviceName,
			Adapter:       "prometheus",
			Hardware:      deviceModel,
			BehaviorClass: behaviorClass,
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			UptimeSeconds: uptime,
		},
		Telemetry: api.TelemetryInfo{
			Timestamp:         time.Now().UTC().Format(time.RFC3339),
			TemperatureC:      tempC,
			GPUUtilizationPct: utilRatio,
			PowerDrawW:        powerW,
			MemoryUsedPct:     memPct,
			MemoryUsedBytes:   int64(memUsed),
			MemoryTotalBytes:  int64(memTotal),
			ThermalState:      "nominal",
		},
		Risk: api.RiskResponse{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Composite: api.CompositeInfo{
				Score:      riskComposite,
				Severity:   riskSeverity,
				Trend:      "stable",
				TrendDelta: 0,
			},
			SubScores: api.SubScores{
				Thermal:    api.SubScoreDetail{Score: riskThermal, Weight: thermalWeight},
				Power:      api.SubScoreDetail{Score: riskPower, Weight: powerWeight},
				Volatility: api.SubScoreDetail{Score: riskVolatility, Weight: volWeight},
				Correlated: api.SubScoreDetail{Score: riskFleetPenalty, Weight: corrWeight},
			},
			Thresholds: api.Thresholds{Warning: warning, Critical: critical},
		},
		Agent: api.AgentInfo{
			Version:        version,
			PollIntervalS:  30,
			AdaptersActive: []string{"prometheus"},
			CloudConnected: false,
		},
	}, nil
}

// ToStatusRisk converts PrometheusData to StatusResponse and RiskResponse for dashboard rendering.
func (p *PrometheusData) ToStatusRisk() (*api.StatusResponse, *api.RiskResponse) {
	if p == nil {
		return nil, nil
	}
	status := &api.StatusResponse{
		Device:    p.Device,
		Telemetry: p.Telemetry,
		Risk: api.RiskSummary{
			CompositeScore: p.Risk.Composite.Score,
			Severity:       p.Risk.Composite.Severity,
			Trend:          p.Risk.Composite.Trend,
			TrendDelta:     p.Risk.Composite.TrendDelta,
		},
		Agent:  p.Agent,
		Health: nil,
	}
	return status, &p.Risk
}
