// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

package kubernetes

import (
	"time"

	v1 "k8s.io/api/core/v1"
)

// Job name label keys in priority order (K8s Job, Kubeflow, app labels).
var jobNameLabels = []string{
	"job-name",
	"training.kubeflow.org/job-name",
	"app.kubernetes.io/name",
	"app",
}

// IsGPUPod returns true if the pod requests any GPU resources from the given list.
func IsGPUPod(pod *v1.Pod, gpuResourceNames []string) bool {
	if pod == nil {
		return false
	}
	for _, c := range pod.Spec.Containers {
		for _, name := range gpuResourceNames {
			if q, ok := c.Resources.Requests[v1.ResourceName(name)]; ok && !q.IsZero() {
				return true
			}
		}
	}
	return false
}

// ExtractGPUCount returns the total GPU count requested across all containers in the pod.
func ExtractGPUCount(pod *v1.Pod, gpuResourceNames []string) int {
	if pod == nil {
		return 0
	}
	var total int64
	for _, c := range pod.Spec.Containers {
		for _, name := range gpuResourceNames {
			if q, ok := c.Resources.Requests[v1.ResourceName(name)]; ok {
				total += q.Value()
			}
		}
	}
	return int(total)
}

// ExtractGPUType returns the first GPU resource name found in the pod, or empty string.
func ExtractGPUType(pod *v1.Pod, gpuResourceNames []string) string {
	if pod == nil {
		return ""
	}
	for _, c := range pod.Spec.Containers {
		for _, name := range gpuResourceNames {
			if q, ok := c.Resources.Requests[v1.ResourceName(name)]; ok && !q.IsZero() {
				return name
			}
		}
	}
	return ""
}

// ExtractJobName extracts the job name from pod labels using the standard hierarchy.
func ExtractJobName(pod *v1.Pod) string {
	if pod == nil {
		return ""
	}
	for _, key := range jobNameLabels {
		if v, ok := pod.Labels[key]; ok && v != "" {
			return v
		}
	}
	return pod.Name
}

// PodToGPUPod converts a v1.Pod to GPUPod if it requests GPU resources.
func PodToGPUPod(pod *v1.Pod, gpuResourceNames []string, nodeToRack map[string]string) (GPUPod, bool) {
	if !IsGPUPod(pod, gpuResourceNames) {
		return GPUPod{}, false
	}

	rackID := ""
	if nodeToRack != nil && pod.Spec.NodeName != "" {
		rackID = nodeToRack[pod.Spec.NodeName]
	}
	if rackID == "" && pod.Spec.NodeName != "" {
		rackID = "unknown"
	}

	labels := make(map[string]string, len(pod.Labels))
	for k, v := range pod.Labels {
		labels[k] = v
	}

	var startTime time.Time
	if pod.Status.StartTime != nil {
		startTime = pod.Status.StartTime.Time
	}

	return GPUPod{
		PodName:   pod.Name,
		Namespace: pod.Namespace,
		NodeName:  pod.Spec.NodeName,
		RackID:    rackID,
		GPUCount:  ExtractGPUCount(pod, gpuResourceNames),
		GPUType:   ExtractGPUType(pod, gpuResourceNames),
		JobName:   ExtractJobName(pod),
		Phase:     string(pod.Status.Phase),
		StartTime: startTime,
		Labels:    labels,
	}, true
}
