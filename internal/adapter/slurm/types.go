// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package slurm implements the Slurm REST API adapter for HPC/AI training job discovery.
package slurm

import "time"

// SlurmJob represents a Slurm job with parsed fields for workload telemetry.
type SlurmJob struct {
	JobID         int
	JobName       string
	UserName      string
	Partition     string
	NodeList      string   // "gpu-node-[01-04]" or "gpu-node-01,gpu-node-02"
	ExpandedNodes []string // Pre-expanded node names from NodeList
	State         string   // RUNNING, PENDING, etc.
	GPUsPerNode   int      // From TRES (trackable resources)
	TotalGPUs     int
	StartTime     int64 // Unix timestamp
	TimeLimit     int   // Minutes
}

// SlurmWorkloadState holds the computed workload state for a poll cycle.
type SlurmWorkloadState struct {
	Timestamp  time.Time
	ActiveJobs []SlurmJob
	TotalGPUs  int
	NodeGPUMap map[string]int // node -> GPUs allocated
	RackGPUMap map[string]int // rack -> GPUs allocated
}

// jobsResponse is the JSON structure returned by GET /slurm/{version}/jobs
type jobsResponse struct {
	Jobs []jobResponse `json:"jobs"`
}

type jobResponse struct {
	JobID        int    `json:"job_id"`
	JobState     string `json:"job_state"`
	Nodes        string `json:"nodes"`
	TresAllocStr string `json:"tres_alloc_str"`
	TresPerNode  string `json:"tres_per_node"`
	TimeLimit    int    `json:"time_limit"`
	StartTime    int64  `json:"start_time"`
	UserName     string `json:"user_name"`
	Partition    string `json:"partition"`
	Name         string `json:"name"`
}

// nodesResponse is the JSON structure returned by GET /slurm/{version}/nodes
type nodesResponse struct {
	Nodes []nodeResponse `json:"nodes"`
}

type nodeResponse struct {
	Name  string `json:"name"`
	State string `json:"state"`
}
