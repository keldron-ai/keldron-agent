//go:build !dcgm

package dcgm

import "fmt"

// NewRealClient returns an error when built without the dcgm build tag.
// The DCGM adapter requires NVIDIA's go-dcgm bindings and the DCGM
// runtime libraries. Use the stub client for development/testing.
func NewRealClient(endpoint string, gpuIDs []int) (dcgmClient, error) {
	return nil, fmt.Errorf("DCGM support not compiled in: build with -tags dcgm and ensure NVIDIA DCGM libraries are installed")
}
