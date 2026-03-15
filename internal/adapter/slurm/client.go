package slurm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout = 10 * time.Second
	defaultVersion = "v0.0.40"
)

// SlurmClient is the HTTP client for the Slurm REST API.
type SlurmClient struct {
	baseURL    string
	apiVersion string
	httpClient *http.Client
	authToken  string
}

// NewSlurmClient creates a SlurmClient with the given configuration.
func NewSlurmClient(baseURL, apiVersion, authToken string, timeout time.Duration) *SlurmClient {
	if apiVersion == "" {
		apiVersion = defaultVersion
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &SlurmClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiVersion: apiVersion,
		httpClient: &http.Client{Timeout: timeout},
		authToken:  authToken,
	}
}

// ListJobs fetches all jobs and filters to RUNNING and PENDING.
func (c *SlurmClient) ListJobs(ctx context.Context) ([]SlurmJob, error) {
	path := fmt.Sprintf("/slurm/%s/jobs", c.apiVersion)
	var resp jobsResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}

	var result []SlurmJob
	for _, j := range resp.Jobs {
		state := strings.ToUpper(j.JobState)
		if state != "RUNNING" && state != "PENDING" {
			continue
		}
		nodes := expandNodeList(j.Nodes)
		gpusPerNode := parseGPUsFromTRES(j.TresPerNode)
		var totalGPUs int
		if gpusPerNode > 0 {
			totalGPUs = gpusPerNode * len(nodes)
		} else {
			totalGPUs = parseGPUsFromTRES(j.TresAllocStr)
			if totalGPUs > 0 && len(nodes) > 0 {
				gpusPerNode = totalGPUs / len(nodes)
			}
		}
		result = append(result, SlurmJob{
			JobID:         j.JobID,
			JobName:       j.Name,
			UserName:      j.UserName,
			Partition:     j.Partition,
			NodeList:      j.Nodes,
			ExpandedNodes: nodes,
			State:         j.JobState,
			GPUsPerNode:   gpusPerNode,
			TotalGPUs:     totalGPUs,
			StartTime:     j.StartTime,
			TimeLimit:     j.TimeLimit,
		})
	}
	return result, nil
}

// ListNodes fetches all nodes.
func (c *SlurmClient) ListNodes(ctx context.Context) ([]SlurmNode, error) {
	path := fmt.Sprintf("/slurm/%s/nodes", c.apiVersion)
	var resp nodesResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}

	result := make([]SlurmNode, 0, len(resp.Nodes))
	for _, n := range resp.Nodes {
		result = append(result, SlurmNode{
			Name:  n.Name,
			State: n.State,
		})
	}
	return result, nil
}

// SlurmNode represents a Slurm node (minimal fields for now).
type SlurmNode struct {
	Name  string
	State string
}

func (c *SlurmClient) get(ctx context.Context, path string, out interface{}) error {
	u, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return fmt.Errorf("building URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if c.authToken != "" {
		req.Header.Set("X-SLURM-USER-TOKEN", c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &AuthError{StatusCode: resp.StatusCode}
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Slurm API returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// AuthError indicates Slurm API authentication failure.
type AuthError struct {
	StatusCode int
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("Slurm API auth failed (HTTP %d); check auth_token", e.StatusCode)
}

// splitTopLevelComma splits a string on commas that are not inside brackets.
func splitTopLevelComma(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, c := range s {
		switch c {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// expandNodeList expands Slurm compressed node notation to a slice of node names.
// Handles: "gpu-node-[01-04]", "gpu-node-01,gpu-node-02", "single-node".
func expandNodeList(nodeList string) []string {
	if nodeList == "" {
		return nil
	}
	parts := splitTopLevelComma(nodeList)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Match pattern like "prefix[N-M]" e.g. "gpu-node-[01-04]"
		idx := strings.Index(p, "[")
		if idx < 0 {
			result = append(result, p)
			continue
		}
		closeIdx := strings.Index(p[idx:], "]")
		if closeIdx < 0 {
			result = append(result, p)
			continue
		}
		closeIdx += idx
		prefix := p[:idx]
		rangeStr := p[idx+1 : closeIdx]
		// Split bracket contents on commas to handle "01-04,08,10-12"
		for _, item := range strings.Split(rangeStr, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			rangeParts := strings.SplitN(item, "-", 2)
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(rangeParts[0])
				end, err2 := strconv.Atoi(rangeParts[1])
				if err1 != nil || err2 != nil {
					result = append(result, prefix+item)
					continue
				}
				padLen := len(rangeParts[0])
				if len(rangeParts[1]) > padLen {
					padLen = len(rangeParts[1])
				}
				if start > end {
					start, end = end, start
				}
				for i := start; i <= end; i++ {
					result = append(result, prefix+fmt.Sprintf("%0*d", padLen, i))
				}
			} else {
				// Single value like "08"
				if _, err := strconv.Atoi(item); err != nil {
					result = append(result, prefix+item)
					continue
				}
				result = append(result, prefix+item)
			}
		}
	}
	return result
}

// parseGPUsFromTRES extracts total GPU count from TRES string.
// Handles: "gres/gpu=4", "gres/gpu:a100=8", "cpu=32,mem=256G,gres/gpu=4".
func parseGPUsFromTRES(tres string) int {
	if tres == "" {
		return 0
	}
	total := 0
	// TRES can be semicolon or comma separated
	for _, part := range strings.FieldsFunc(tres, func(r rune) bool { return r == ';' || r == ',' }) {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "gres/gpu") {
			continue
		}
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(part[eq+1:]))
		if err != nil {
			continue
		}
		total += n
	}
	return total
}
