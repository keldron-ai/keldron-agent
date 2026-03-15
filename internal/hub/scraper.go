// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package hub

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

const (
	unhealthyLogThreshold = 5
)

// PeerMetricsCallback is called when a peer is successfully scraped, with peerID and parsed MetricFamilies.
// The families have the peer label already added. Optional; used by hub for /metrics merge.
type PeerMetricsCallback func(peerID string, families map[string]*dto.MetricFamily)

// Scraper scrapes Prometheus /metrics from peer agents.
type Scraper struct {
	client            *http.Client
	interval          time.Duration
	registry          *PeerRegistry
	logger            *slog.Logger
	scrapeErrors      int64
	lastDuration      time.Duration
	mu                sync.Mutex
	peerMetricsNotify PeerMetricsCallback
}

// NewScraper creates a new scraper.
func NewScraper(interval time.Duration, registry *PeerRegistry, logger *slog.Logger) *Scraper {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scraper{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		interval: interval,
		registry: registry,
		logger:   logger,
	}
}

// SetPeerMetricsCallback sets the callback for peer metrics (for hub /metrics merge).
func (s *Scraper) SetPeerMetricsCallback(fn PeerMetricsCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerMetricsNotify = fn
}

// Start runs the scrape loop until ctx is cancelled.
func (s *Scraper) Start(ctx context.Context) {
	if s.interval <= 0 {
		s.interval = 30 * time.Second
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Initial scrape immediately
	s.scrapeAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scrapeAll(ctx)
		}
	}
}

func (s *Scraper) scrapeAll(ctx context.Context) {
	start := time.Now()
	peers := s.registry.GetPeers()
	var wg sync.WaitGroup
	for _, p := range peers {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			devices, peerID, families, err := s.ScrapePeerWithMetrics(ctx, addr)
			if err != nil {
				s.mu.Lock()
				s.scrapeErrors++
				s.mu.Unlock()
				failures := s.registry.MarkUnhealthy(addr)
				if failures >= unhealthyLogThreshold {
					s.logger.Error("peer scrape failed repeatedly", "address", addr, "failures", failures, "error", err)
				} else {
					s.logger.Warn("peer scrape failed", "address", addr, "error", err)
				}
				return
			}
			if peerID == "" {
				peerID = addr
			}
			s.registry.UpdatePeer(addr, peerID, devices)
			s.mu.Lock()
			notify := s.peerMetricsNotify
			s.mu.Unlock()
			if notify != nil && families != nil {
				notify(peerID, families)
			}
		}(p.Address)
	}
	wg.Wait()
	s.mu.Lock()
	s.lastDuration = time.Since(start)
	s.mu.Unlock()
}

// ScrapePeer fetches /metrics from a peer and returns parsed PeerDevices and peer ID.
func (s *Scraper) ScrapePeer(ctx context.Context, address string) ([]PeerDevice, string, error) {
	devices, peerID, _, err := s.ScrapePeerWithMetrics(ctx, address)
	return devices, peerID, err
}

// ScrapePeerWithMetrics fetches /metrics and returns PeerDevices, peer ID, and MetricFamilies (with peer label).
func (s *Scraper) ScrapePeerWithMetrics(ctx context.Context, address string) ([]PeerDevice, string, map[string]*dto.MetricFamily, error) {
	url := "http://" + address + "/metrics"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", nil, err
	}
	devices, peerID, err := ParseMetricsToPeerDevices(bytes.NewReader(body))
	if err != nil {
		return nil, "", nil, err
	}
	if peerID == "" {
		peerID = address
	}
	families, err := parseToMetricFamiliesWithPeerLabel(bytes.NewReader(body), peerID)
	if err != nil {
		return devices, peerID, nil, nil // devices still valid
	}
	return devices, peerID, families, nil
}

// ScrapeErrors returns the cumulative scrape error count.
func (s *Scraper) ScrapeErrors() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scrapeErrors
}

// LastDuration returns the last scrape cycle duration.
func (s *Scraper) LastDuration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastDuration
}

func severityFromFloat(v float64) string {
	switch {
	case v >= 2:
		return "critical"
	case v >= 1:
		return "warning"
	default:
		return "normal"
	}
}

// parseToMetricFamiliesWithPeerLabel parses Prometheus text and adds peer label to each metric.
func parseToMetricFamiliesWithPeerLabel(r io.Reader, peerID string) (map[string]*dto.MetricFamily, error) {
	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(r)
	if err != nil {
		return nil, err
	}
	peerName := peerID
	for _, mf := range families {
		if mf == nil {
			continue
		}
		for _, m := range mf.Metric {
			if m.Label == nil {
				m.Label = []*dto.LabelPair{}
			}
			// Add peer label
			m.Label = append(m.Label, &dto.LabelPair{
				Name:  strPtr("peer"),
				Value: strPtr(peerName),
			})
		}
	}
	return families, nil
}

func strPtr(s string) *string { return &s }

// ParseMetricsToPeerDevices parses Prometheus exposition format and extracts keldron_* metrics into PeerDevices.
// Returns devices, peer ID (from keldron_agent_info device_name), and error.
func ParseMetricsToPeerDevices(r io.Reader) ([]PeerDevice, string, error) {
	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(r)
	if err != nil {
		return nil, "", err
	}

	peerID := ""
	if mf, ok := families["keldron_agent_info"]; ok && mf != nil {
		for _, m := range mf.Metric {
			for _, lp := range m.Label {
				if lp.Name != nil && *lp.Name == "device_name" && lp.Value != nil {
					peerID = *lp.Value
					break
				}
			}
			if peerID != "" {
				break
			}
		}
	}

	devices, err := parseMetricsFromFamilies(families)
	if err != nil {
		return nil, "", err
	}
	return devices, peerID, nil
}

func parseMetricsFromFamilies(families map[string]*dto.MetricFamily) ([]PeerDevice, error) {
	getLabel := func(labels []*dto.LabelPair, name string) string {
		for _, lp := range labels {
			if lp.Name != nil && *lp.Name == name && lp.Value != nil {
				return *lp.Value
			}
		}
		return ""
	}

	devices := make(map[string]*PeerDevice)

	for _, name := range []string{
		"keldron_gpu_temperature_celsius",
		"keldron_gpu_power_watts",
		"keldron_gpu_utilization_ratio",
		"keldron_risk_composite",
		"keldron_risk_severity",
		"keldron_gpu_memory_pressure_ratio",
	} {
		mf, ok := families[name]
		if !ok || mf == nil {
			continue
		}
		for _, m := range mf.Metric {
			deviceID := getLabel(m.Label, "device_id")
			if deviceID == "" {
				model := getLabel(m.Label, "device_model")
				if model != "" {
					deviceID = model + ":0"
				} else {
					deviceID = "default"
				}
			}
			d, ok := devices[deviceID]
			if !ok {
				d = &PeerDevice{
					DeviceID:      deviceID,
					DeviceModel:   getLabel(m.Label, "device_model"),
					DeviceVendor:  getLabel(m.Label, "device_vendor"),
					BehaviorClass: getLabel(m.Label, "behavior_class"),
					LastUpdated:   time.Now(),
				}
				devices[deviceID] = d
			}
			var v float64
			if m.Gauge != nil {
				v = m.Gauge.GetValue()
			}
			switch name {
			case "keldron_gpu_temperature_celsius":
				d.TemperatureC = v
			case "keldron_gpu_power_watts":
				d.PowerW = v
			case "keldron_gpu_utilization_ratio":
				d.Utilization = v
			case "keldron_risk_composite":
				d.RiskComposite = v
			case "keldron_risk_severity":
				d.RiskSeverity = severityFromFloat(v)
			case "keldron_gpu_memory_pressure_ratio":
				d.MemoryPressure = v
			}
		}
	}

	out := make([]PeerDevice, 0, len(devices))
	for _, d := range devices {
		out = append(out, *d)
	}
	return out, nil
}
