// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

// Package kubernetes implements the Kubernetes scheduler adapter for GPU pod discovery.
package kubernetes

import "time"

// GPUPod represents a pod that requests GPU resources.
type GPUPod struct {
	PodName   string
	Namespace string
	NodeName  string
	RackID    string // Resolved from node_to_rack_map
	GPUCount  int    // Requested GPU count
	GPUType   string // "nvidia.com/gpu" or "amd.com/gpu"
	JobName   string // Extracted from labels (job-name, app, etc.)
	Phase     string // Running, Pending, Succeeded, Failed
	StartTime time.Time
	Labels    map[string]string
}

// WorkloadState is a snapshot of GPU workload across the cluster.
type WorkloadState struct {
	Timestamp  time.Time
	ActivePods []GPUPod
	TotalGPUs  int            // Sum of all active GPU allocations
	NodeGPUMap map[string]int // node -> GPU count on that node
	RackGPUMap map[string]int // rack -> GPU count on that rack
}

// BuildWorkloadState constructs a WorkloadState from a list of GPUPods.
func BuildWorkloadState(pods []GPUPod) WorkloadState {
	nodeGPUMap := make(map[string]int)
	rackGPUMap := make(map[string]int)
	totalGPUs := 0

	for _, p := range pods {
		totalGPUs += p.GPUCount
		if p.NodeName != "" {
			nodeGPUMap[p.NodeName] += p.GPUCount
		}
		if p.RackID != "" {
			rackGPUMap[p.RackID] += p.GPUCount
		}
	}

	return WorkloadState{
		Timestamp:  time.Now(),
		ActivePods: pods,
		TotalGPUs:  totalGPUs,
		NodeGPUMap: nodeGPUMap,
		RackGPUMap: rackGPUMap,
	}
}
