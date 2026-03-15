package rocm

import "time"

// ROCmConfig holds ROCm-specific configuration decoded from the adapter's Raw YAML node.
type ROCmConfig struct {
	Enabled          bool          `yaml:"enabled"`
	PollInterval     time.Duration `yaml:"poll_interval"`
	ROCmSMIPath      string        `yaml:"rocm_smi_path"`
	CollectionMethod string        `yaml:"collection_method"` // "cli" or "library"
	GPUIndices       []int         `yaml:"gpu_indices"`       // empty = all GPUs
}
