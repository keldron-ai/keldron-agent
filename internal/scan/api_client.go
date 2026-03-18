// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package scan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/keldron-ai/keldron-agent/internal/api"
)

const apiTimeout = 5 * time.Second

// FetchStatus fetches the agent status from GET /api/v1/status.
// baseURL may be "host:port" or "http://host:port".
func FetchStatus(baseURL string) (*api.StatusResponse, error) {
	url := buildAPIURL(baseURL, "/api/v1/status")
	client := &http.Client{Timeout: apiTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("status request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status returned %d", resp.StatusCode)
	}

	var status api.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("invalid status response: %w", err)
	}
	return &status, nil
}

// FetchRisk fetches the risk breakdown from GET /api/v1/risk.
// baseURL may be "host:port" or "http://host:port".
func FetchRisk(baseURL string) (*api.RiskResponse, error) {
	url := buildAPIURL(baseURL, "/api/v1/risk")
	client := &http.Client{Timeout: apiTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("risk request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("risk returned %d", resp.StatusCode)
	}

	var risk api.RiskResponse
	if err := json.NewDecoder(resp.Body).Decode(&risk); err != nil {
		return nil, fmt.Errorf("invalid risk response: %w", err)
	}
	return &risk, nil
}

// buildAPIURL normalizes baseURL and appends path.
func buildAPIURL(baseURL, path string) string {
	addr := strings.TrimSpace(baseURL)
	if addr == "" {
		addr = "http://127.0.0.1:9200"
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	return strings.TrimSuffix(addr, "/") + path
}
