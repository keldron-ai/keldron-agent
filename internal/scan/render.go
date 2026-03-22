// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/keldron-ai/keldron-agent/registry"
)

// ANSI color codes
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiCyan   = "\033[36m"
	ansiWhite  = "\033[37m"
)

// SortOrder is the sort mode for the device table.
type SortOrder string

const (
	SortRisk  SortOrder = "risk"
	SortName  SortOrder = "name"
	SortTemp  SortOrder = "temp"
	SortPower SortOrder = "power"
)

// CloudState holds pre-fetched cloud metrics to avoid network I/O during rendering.
type CloudState struct {
	Connected     bool
	HistoryWindow string
	FleetAge      string
}

// FetchCloudState performs the cloud metrics fetch once and returns cached state.
func FetchCloudState(apiKey string) *CloudState {
	if apiKey == "" {
		return nil
	}
	metrics, err := fetchCloudMetrics(apiKey)
	if err != nil {
		return &CloudState{Connected: false}
	}
	return &CloudState{
		Connected:     true,
		HistoryWindow: metrics.HistoryWindow,
		FleetAge:      metrics.FleetAge,
	}
}

// RenderOpts configures table rendering.
type RenderOpts struct {
	Quiet        bool // No header, footer, or cloud teaser
	Sort         SortOrder
	DeviceFilter string      // Substring to filter devices (empty = all)
	CloudAPIKey  string      // For cloud teaser line
	Cloud        *CloudState // Pre-fetched cloud state (nil = no API key)
}

// AllDevices returns a flat list of devices from the fleet response.
func AllDevices(fleet *FleetResponse) []DeviceResponse {
	if fleet == nil {
		return nil
	}
	var out []DeviceResponse
	for _, p := range fleet.Peers {
		out = append(out, p.Devices...)
	}
	return out
}

// FilterAndSortDevices filters and sorts devices per opts.
func FilterAndSortDevices(devices []DeviceResponse, opts RenderOpts) []DeviceResponse {
	filtered := devices
	if opts.DeviceFilter != "" {
		lowerFilter := strings.ToLower(opts.DeviceFilter)
		filtered = make([]DeviceResponse, 0, len(devices))
		for _, d := range devices {
			if strings.Contains(strings.ToLower(d.DeviceID), lowerFilter) ||
				strings.Contains(strings.ToLower(d.DeviceModel), lowerFilter) {
				filtered = append(filtered, d)
			}
		}
	}

	sorted := make([]DeviceResponse, len(filtered))
	copy(sorted, filtered)

	switch opts.Sort {
	case SortName:
		sort.Slice(sorted, func(i, j int) bool {
			return strings.Compare(sorted[i].DeviceID, sorted[j].DeviceID) < 0
		})
	case SortTemp:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].TemperatureC > sorted[j].TemperatureC
		})
	case SortPower:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].PowerW > sorted[j].PowerW
		})
	default: // SortRisk
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].RiskComposite > sorted[j].RiskComposite
		})
	}
	return sorted
}

// RenderTable writes the fleet table and footer to w.
func RenderTable(w io.Writer, fleet *FleetResponse, opts RenderOpts) {
	if fleet == nil {
		fleet = &FleetResponse{}
	}
	devices := FilterAndSortDevices(AllDevices(fleet), opts)

	if !opts.Quiet {
		// Header
		ts := fleet.Timestamp
		if ts == "" {
			ts = "unknown"
		}
		fmt.Fprintf(w, "%sFLEET THERMAL SCAN — %s UTC%s\n\n", ansiBold+ansiCyan, ts, ansiReset)
	}

	// Table header
	colDevice, colModel, colTj, colVRAM, colPower, colRisk, colStatus := 14, 14, 8, 14, 8, 6, 8
	fmt.Fprintf(w, "%s%-*s %-*s %-*s %-*s %-*s %-*s %-*s%s\n",
		ansiBold+ansiWhite,
		colDevice, "DEVICE", colModel, "MODEL", colTj, "Tj", colVRAM, "VRAM", colPower, "POWER", colRisk, "RISK", colStatus, "STATUS",
		ansiReset)

	for _, d := range devices {
		device := d.DeviceID
		if device == "" {
			device = "-"
		}
		model := d.DeviceModel
		if model == "" {
			model = "-"
		}

		tjStr := "-"
		if d.TemperatureC > 0 {
			tjStr = fmt.Sprintf("%.2f°C", d.TemperatureC)
		}

		vramStr := formatVRAM(d)
		powerStr := "-"
		if d.PowerW > 0 {
			powerStr = fmt.Sprintf("%.0fW", d.PowerW)
		}
		riskStr := "-"
		if d.RiskComposite > 0 {
			riskStr = fmt.Sprintf("%.0f", d.RiskComposite)
		}
		statusStr, statusColor := statusDisplay(d.RiskSeverity)

		tjColor := colorTemp(d.TemperatureC, d.DeviceModel)
		riskColor := colorRisk(d.RiskComposite)

		fmt.Fprintf(w, "%-*s %-*s %s%-*s%s %-*s %-*s %s%-*s%s %s%-*s%s\n",
			colDevice, truncate(device, colDevice),
			colModel, truncate(model, colModel),
			tjColor, colTj, tjStr, ansiReset,
			colVRAM, truncate(vramStr, colVRAM),
			colPower, powerStr,
			riskColor, colRisk, riskStr, ansiReset,
			statusColor, colStatus, statusStr, ansiReset,
		)
	}

	if !opts.Quiet {
		fmt.Fprintln(w)
		renderFooter(w, devices, opts)
		renderCloudTeaser(w, opts.Cloud)
	}
}

func formatVRAM(d DeviceResponse) string {
	if d.MemoryTotalBytes > 0 && d.MemoryUsedBytes >= 0 {
		usedGB := d.MemoryUsedBytes / (1024 * 1024 * 1024)
		totalGB := d.MemoryTotalBytes / (1024 * 1024 * 1024)
		return fmt.Sprintf("%.0f/%.0fGB", usedGB, totalGB)
	}
	return "-"
}

func statusDisplay(severity string) (string, string) {
	switch strings.ToLower(severity) {
	case "critical":
		return "CRIT", ansiRed
	case "warning":
		return "WARN", ansiYellow
	case "elevated":
		return "HIGH", ansiYellow
	case "active":
		return "BUSY", ansiCyan
	case "normal", "":
		return "OK", ansiGreen
	default:
		return "UNKNOWN", ansiYellow
	}
}

func colorTemp(tempC float64, model string) string {
	if tempC <= 0 {
		return ansiReset
	}
	spec := registry.Lookup(model)
	limit := spec.ThermalLimitC
	if limit <= 0 {
		limit = 83
	}
	pct := tempC / limit
	if pct >= 0.8 {
		return ansiRed
	}
	if pct >= 0.6 {
		return ansiYellow
	}
	return ansiGreen
}

func colorRisk(risk float64) string {
	if risk >= 66 {
		return ansiRed
	}
	if risk >= 41 {
		return ansiYellow
	}
	return ansiGreen
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

func renderFooter(w io.Writer, devices []DeviceResponse, opts RenderOpts) {
	if len(devices) == 0 {
		if opts.DeviceFilter != "" {
			fmt.Fprintf(w, "No devices matched filter %q.\n", opts.DeviceFilter)
		} else {
			fmt.Fprintln(w, "Fleet scan: hub is running but no peers discovered. Check mDNS or static peer config.")
		}
		return
	}

	hasWarnOrCrit := false
	var highest DeviceResponse
	highest.RiskComposite = -1
	var sumRisk float64
	for _, d := range devices {
		sev := strings.ToLower(d.RiskSeverity)
		if sev == "elevated" || sev == "warning" || sev == "critical" {
			hasWarnOrCrit = true
		}
		if d.RiskComposite > highest.RiskComposite {
			highest = d
		}
		sumRisk += d.RiskComposite
	}

	if hasWarnOrCrit && highest.RiskComposite >= 0 {
		statusStr, _ := statusDisplay(highest.RiskSeverity)
		fmt.Fprintf(w, "%sRISK ENGINE%s  Highest risk: %s (score: %.0f, %s)\n",
			ansiBold, ansiReset, highest.DeviceID, highest.RiskComposite, statusStr)

		spec := registry.Lookup(highest.DeviceModel)
		limit := spec.ThermalLimitC
		if limit <= 0 {
			limit = 83
		}
		if highest.TemperatureC > 0 && limit > 0 {
			pct := (highest.TemperatureC / limit) * 100
			fmt.Fprintf(w, "  Primary driver: thermal_stress (Tj %.2f°C, %.0f%% of limit)\n",
				highest.TemperatureC, pct)
		}
	} else {
		avg := sumRisk / float64(len(devices))
		minR, maxR := devices[0].RiskComposite, devices[0].RiskComposite
		for _, d := range devices[1:] {
			if d.RiskComposite < minR {
				minR = d.RiskComposite
			}
			if d.RiskComposite > maxR {
				maxR = d.RiskComposite
			}
		}
		fmt.Fprintf(w, "%sFLEET STATUS%s  All %d devices healthy · Fleet risk: %.0f (avg) · %.0f (min) · %.0f (max)\n",
			ansiBold, ansiReset, len(devices), avg, minR, maxR)
	}
}

func renderCloudTeaser(w io.Writer, cloud *CloudState) {
	fmt.Fprint(w, ansiDim)
	if cloud == nil {
		fmt.Fprintln(w, "ℹ  History, GPU Age, and job tracking available with Keldron Cloud → keldron.ai/cloud")
	} else if cloud.Connected {
		fmt.Fprintf(w, "☁  Connected to Keldron Cloud · %s history · Fleet age: %s\n", cloud.HistoryWindow, cloud.FleetAge)
	} else {
		fmt.Fprintln(w, "⚠  Keldron Cloud configured but unreachable")
	}
	fmt.Fprint(w, ansiReset)
}

type cloudMetrics struct {
	FleetAge      string `json:"fleet_age"`
	HistoryWindow string `json:"history_window"`
}

func fetchCloudMetrics(apiKey string) (*cloudMetrics, error) {
	req, err := http.NewRequest("GET", "https://api.keldron.ai/v1/fleet/metrics", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloud API returned status %d", resp.StatusCode)
	}
	var m cloudMetrics
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// RenderJSON writes the fleet response as formatted JSON to w.
func RenderJSON(w io.Writer, fleet *FleetResponse) error {
	if fleet == nil {
		fleet = &FleetResponse{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(fleet)
}
