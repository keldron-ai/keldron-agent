// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/api"
	"github.com/keldron-ai/keldron-agent/internal/config"
)

const (
	clearScreen = "\033[H\033[2J"
)

// Run executes the scan command. Returns exit code.
func Run(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	hub := fs.String("hub", "localhost:9200", "hub address (host:port)")
	port := fs.Int("port", 0, "agent API port (default 9200 or from config)")
	promPort := fs.Int("prometheus-port", 0, "Prometheus metrics port for fallback (default 9100)")
	device := fs.String("device", "", "filter to device matching name/id")
	watch := fs.Int("watch", 0, "refresh interval in seconds (min 2, 0=disabled)")
	jsonOut := fs.Bool("json", false, "output raw JSON")
	quiet := fs.Bool("quiet", false, "table only, no header/footer/cloud teaser")
	sortOrder := fs.String("sort", "risk", "sort by: risk, name, temp, power")
	configPath := fs.String("config", "./keldron-agent.yaml", "path to config file (for cloud teaser)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		fmt.Fprintf(os.Stderr, "scan: %v\n", err)
		return 1
	}

	opts := RenderOpts{
		Quiet:        *quiet,
		Sort:         SortOrder(*sortOrder),
		DeviceFilter: *device,
	}

	// Load config for cloud teaser and port resolution (best effort)
	var cfg *config.Config
	if c, err := config.Load(*configPath); err == nil {
		cfg = c
		opts.CloudAPIKey = c.Cloud.APIKey
	}

	apiPort := *port
	prometheusPort := *promPort
	if cfg != nil {
		if apiPort == 0 {
			apiPort = cfg.API.Port
		}
		if prometheusPort == 0 {
			prometheusPort = cfg.Output.PrometheusPort
		}
	}
	if apiPort == 0 {
		apiPort = 9200
	}
	if prometheusPort == 0 {
		prometheusPort = 9100
	}

	host := parseHostFromAddr(*hub)

	if *jsonOut {
		return runJSON(host, apiPort, prometheusPort, *hub, opts)
	}

	// Pre-fetch cloud state once so render loops don't block on network I/O
	opts.Cloud = FetchCloudState(opts.CloudAPIKey)

	if *watch > 0 {
		interval := *watch
		if interval < 2 {
			interval = 2
		}
		return runWatch(host, apiPort, prometheusPort, *hub, opts, time.Duration(interval)*time.Second)
	}

	return runOnce(host, apiPort, prometheusPort, *hub, opts)
}

// parseHostFromAddr extracts host from "host:port", "http://host:port/path", etc.
func parseHostFromAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "127.0.0.1"
	}

	// If the address looks like a URL (has a scheme), use url.Parse for robust handling.
	if strings.Contains(addr, "://") {
		if u, err := url.Parse(addr); err == nil && u.Host != "" {
			h := u.Hostname()
			if h != "" {
				return h
			}
		}
	}

	// Strip scheme prefix if present without "://" (shouldn't happen, but be safe)
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")

	// Remove any path component
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port, use as host
		if addr != "" {
			return addr
		}
		return "127.0.0.1"
	}
	if host == "" {
		return "127.0.0.1"
	}
	return host
}

// buildAPIBaseURL returns http://host:port for the agent API.
func buildAPIBaseURL(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

func runOnce(host string, apiPort, prometheusPort int, hubAddr string, opts RenderOpts) int {
	apiBase := buildAPIBaseURL(host, apiPort)

	// 1. Try agent API first
	status, err := FetchStatus(apiBase)
	if err == nil {
		risk, errRisk := FetchRisk(apiBase)
		if errRisk == nil {
			RenderDashboard(os.Stdout, status, risk, opts)
			return 0
		}
		// Status ok but risk failed - still render with status (risk may be empty)
		RenderDashboard(os.Stdout, status, nil, opts)
		return 0
	}

	// 2. Try hub fleet API
	fleet, err := FetchFleet(hubAddr)
	if err == nil || errors.Is(err, ErrNoPeers) {
		if fleet == nil {
			fleet = &FleetResponse{}
		}
		RenderTable(os.Stdout, fleet, opts)
		return 0
	}

	// 3. Fall back to Prometheus
	prom, err := FetchFromPrometheus(host, prometheusPort)
	if err == nil {
		status, risk := prom.ToStatusRisk()
		if !opts.Quiet {
			fmt.Fprintln(os.Stderr, "(using legacy Prometheus endpoint — upgrade agent for full dashboard)")
		}
		RenderDashboard(os.Stdout, status, risk, opts)
		return 0
	}

	fmt.Fprintf(os.Stderr, "Error: cannot reach agent API at %s, hub at %s, or Prometheus at %s:%d. Is the agent running?\n", apiBase, hubAddr, host, prometheusPort)
	return 1
}

func runJSON(host string, apiPort, prometheusPort int, hubAddr string, opts RenderOpts) int {
	apiBase := buildAPIBaseURL(host, apiPort)

	// 1. Try agent status API first
	status, err := FetchStatus(apiBase)
	if err == nil {
		return encodeStatusJSON(os.Stdout, status)
	}

	// 2. Fall back to fleet, convert to StatusResponse for stable schema
	fleet, err := FetchFleet(hubAddr)
	if err == nil || errors.Is(err, ErrNoPeers) {
		if fleet == nil {
			fleet = &FleetResponse{}
		}
		if opts.DeviceFilter != "" || opts.Sort != SortRisk {
			devices := FilterAndSortDevices(AllDevices(fleet), opts)
			healthy, warning, critical := 0, 0, 0
			for _, d := range devices {
				switch d.RiskSeverity {
				case "warning":
					warning++
				case "critical":
					critical++
				default:
					healthy++
				}
			}
			fleet = &FleetResponse{
				Timestamp: fleet.Timestamp,
				Peers: []PeerResponse{{
					ID:      "filtered",
					Address: "filtered",
					Healthy: true,
					Devices: devices,
				}},
				Summary: SummaryResponse{
					TotalDevices: len(devices),
					Healthy:      healthy,
					Warning:      warning,
					Critical:     critical,
					TotalPeers:   1,
					HealthyPeers: 1,
				},
			}
		}
		status := fleetToStatusResponse(fleet, opts)
		return encodeStatusJSON(os.Stdout, status)
	}

	// 3. Fall back to Prometheus
	prom, err := FetchFromPrometheus(host, prometheusPort)
	if err == nil {
		status, _ := prom.ToStatusRisk()
		if status != nil {
			return encodeStatusJSON(os.Stdout, status)
		}
	}

	fmt.Fprintf(os.Stderr, "Error: cannot reach agent API, hub, or Prometheus. Is the agent running?\n")
	return 1
}

// encodeStatusJSON writes api.StatusResponse as formatted JSON. Returns exit code.
func encodeStatusJSON(w io.Writer, status *api.StatusResponse) int {
	if status == nil {
		return 1
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(status); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// fleetToStatusResponse converts FleetResponse to api.StatusResponse for stable JSON schema.
// Uses the first device from the fleet; Device.Adapter is set to "fleet" to indicate source.
func fleetToStatusResponse(fleet *FleetResponse, opts RenderOpts) *api.StatusResponse {
	if fleet == nil {
		fleet = &FleetResponse{}
	}
	devices := FilterAndSortDevices(AllDevices(fleet), opts)
	now := time.Now().UTC().Format(time.RFC3339)
	resp := &api.StatusResponse{
		Device: api.DeviceInfo{
			Hostname:      "unknown",
			Adapter:       "fleet",
			Hardware:      "",
			BehaviorClass: "consumer_active_cooled",
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			UptimeSeconds: 0,
		},
		Telemetry: api.TelemetryInfo{
			Timestamp:         now,
			TemperatureC:      0,
			GPUUtilizationPct: 0,
			PowerDrawW:        0,
			MemoryUsedPct:     0,
			MemoryUsedBytes:   0,
			MemoryTotalBytes:  0,
			ThermalState:      "nominal",
		},
		Risk: api.RiskSummary{
			CompositeScore: 0,
			Severity:       "normal",
			Trend:          "stable",
			TrendDelta:     0,
		},
		Agent: api.AgentInfo{
			Version:        "unknown",
			PollIntervalS:  30,
			AdaptersActive: []string{"fleet"},
			CloudConnected: false,
		},
		Health: nil,
	}
	if len(devices) > 0 {
		d := devices[0]
		resp.Device.Hostname = d.DeviceID
		resp.Device.Hardware = d.DeviceModel
		resp.Telemetry.TemperatureC = d.TemperatureC
		resp.Telemetry.GPUUtilizationPct = d.Utilization
		resp.Telemetry.PowerDrawW = d.PowerW
		resp.Telemetry.MemoryUsedBytes = int64(d.MemoryUsedBytes)
		resp.Telemetry.MemoryTotalBytes = int64(d.MemoryTotalBytes)
		if d.MemoryTotalBytes > 0 {
			resp.Telemetry.MemoryUsedPct = d.MemoryUsedBytes / d.MemoryTotalBytes * 100
		}
		resp.Risk.CompositeScore = d.RiskComposite
		resp.Risk.Severity = d.RiskSeverity
	}
	return resp
}

const (
	// ANSI: move cursor up one line, clear line
	ansiUpClear = "\033[A\033[2K"
	// ANSI: carriage return, clear line (for in-place countdown update)
	ansiReturnClear = "\r\033[2K"
)

func runWatch(host string, apiPort, prometheusPort int, hubAddr string, opts RenderOpts, watchInterval time.Duration) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	apiBase := buildAPIBaseURL(host, apiPort)
	useLegacyNote := false
	var lastFrame []byte

	for {
		interval := watchInterval

		// Buffer the entire frame before writing — prevents partial renders.
		var buf bytes.Buffer

		// 1. Try agent API
		status, errStatus := FetchStatus(apiBase)
		if errStatus == nil {
			risk, errRisk := FetchRisk(apiBase)
			if errRisk != nil {
				risk = nil
			}
			// Use poll_interval_s from status for refresh
			if status.Agent.PollIntervalS >= 2 {
				interval = time.Duration(status.Agent.PollIntervalS) * time.Second
			}
			RenderDashboard(&buf, status, risk, opts)
			lastFrame = buf.Bytes()
		} else {
			// 2. Try fleet
			fleet, err := FetchFleet(hubAddr)
			if err == nil || errors.Is(err, ErrNoPeers) {
				if fleet == nil {
					fleet = &FleetResponse{}
				}
				RenderTable(&buf, fleet, opts)
				lastFrame = buf.Bytes()
			} else {
				// 3. Try Prometheus
				prom, err := FetchFromPrometheus(host, prometheusPort)
				if err == nil {
					status, risk := prom.ToStatusRisk()
					if !useLegacyNote {
						useLegacyNote = true
						fmt.Fprintln(os.Stderr, "(using legacy Prometheus endpoint — upgrade agent for full dashboard)")
					}
					RenderDashboard(&buf, status, risk, opts)
					lastFrame = buf.Bytes()
				} else {
					// All sources failed: keep retrying, reuse last frame
					fmt.Fprintf(os.Stderr, "Error: cannot reach agent API, hub, or Prometheus. Is the agent running?\n")
					if lastFrame != nil {
						buf.Write(lastFrame)
					} else {
						buf.WriteString("(waiting for agent...)\n")
					}
				}
			}
		}

		// Clear screen and write the complete frame atomically (single write)
		frame := buf.String()
		fmt.Fprint(os.Stdout, clearScreen+frame)
		os.Stdout.Sync()

		now := time.Now().UTC()
		lastUpdated := now.Format("15:04:05")
		secs := int(interval.Seconds())
		if secs < 2 {
			secs = 2
		}

		// Countdown: update status line every second
		fmt.Fprintf(os.Stdout, "\nLast updated: %s UTC · Next refresh in %ds\n", lastUpdated, secs)
		for s := secs - 1; s >= 0; s-- {
			select {
			case <-ctx.Done():
				return 0
			case <-time.After(time.Second):
			}

			if s == 0 {
				break
			}

			if s == secs-1 {
				fmt.Fprint(os.Stdout, ansiUpClear)
			} else {
				fmt.Fprint(os.Stdout, ansiReturnClear)
			}
			fmt.Fprintf(os.Stdout, "Last updated: %s UTC · Next refresh in %ds", lastUpdated, s)
		}

		select {
		case <-ctx.Done():
			return 0
		default:
			// continue to next fetch
		}
	}
}
