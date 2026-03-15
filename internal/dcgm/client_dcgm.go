// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Keldron (keldron.ai)

//go:build dcgm

package dcgm

import "fmt"

// realClient wraps the NVIDIA go-dcgm bindings.
// TODO(S-002+): Implement when NVIDIA DCGM SDK is available.
type realClient struct {
	endpoint string
	gpuIDs   []int
}

// NewRealClient creates a real DCGM client using NVIDIA's go-dcgm library.
// Requires the dcgm build tag and NVIDIA DCGM runtime libraries.
func NewRealClient(endpoint string, gpuIDs []int) (dcgmClient, error) {
	// TODO: Initialize go-dcgm bindings, connect to nv-hostengine.
	return nil, fmt.Errorf("real DCGM client not yet implemented: use use_stub: true for development")
}

func (c *realClient) Collect() ([]GPUMetrics, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *realClient) Close() error {
	return nil
}
