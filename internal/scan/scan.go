// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/config"
)

const (
	clearScreen = "\033[2J\033[H"
)

// Run executes the scan command. Returns exit code.
func Run(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	hub := fs.String("hub", "localhost:9200", "hub address (host:port)")
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

	// Load config for cloud teaser (best effort)
	if cfg, err := config.Load(*configPath); err == nil {
		opts.CloudAPIKey = cfg.Cloud.APIKey
	}

	if *jsonOut {
		return runJSON(*hub, opts)
	}

	// Pre-fetch cloud state once so render loops don't block on network I/O
	opts.Cloud = FetchCloudState(opts.CloudAPIKey)

	if *watch > 0 {
		interval := *watch
		if interval < 2 {
			interval = 2
		}
		return runWatch(*hub, opts, time.Duration(interval)*time.Second)
	}

	return runOnce(*hub, opts)
}

func runOnce(hubAddr string, opts RenderOpts) int {
	fleet, err := FetchFleet(hubAddr)
	if err != nil && !errors.Is(err, ErrNoPeers) {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if fleet == nil {
		fleet = &FleetResponse{}
	}

	RenderTable(os.Stdout, fleet, opts)
	return 0
}

func runJSON(hubAddr string, opts RenderOpts) int {
	fleet, err := FetchFleet(hubAddr)
	if err != nil && !errors.Is(err, ErrNoPeers) {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if fleet == nil {
		fleet = &FleetResponse{}
	}

	if opts.DeviceFilter != "" || opts.Sort != SortRisk {
		devices := FilterAndSortDevices(AllDevices(fleet), opts)
		// Rebuild a single-peer fleet with the filtered/sorted devices
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

	if err := RenderJSON(os.Stdout, fleet); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

const (
	// ANSI: move cursor up one line, clear line
	ansiUpClear = "\033[A\033[2K"
	// ANSI: carriage return, clear line (for in-place countdown update)
	ansiReturnClear = "\r\033[2K"
)

func runWatch(hubAddr string, opts RenderOpts, interval time.Duration) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for {
		fleet, err := FetchFleet(hubAddr)
		if err != nil && !errors.Is(err, ErrNoPeers) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if fleet == nil {
			fleet = &FleetResponse{}
		}

		// Clear screen and redraw
		fmt.Fprint(os.Stdout, clearScreen)
		RenderTable(os.Stdout, fleet, opts)

		now := time.Now().UTC()
		lastUpdated := now.Format("15:04:05")
		secs := int(interval.Seconds())

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

			// First update: cursor is on line below, need to go up. Later: overwrite in place.
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
