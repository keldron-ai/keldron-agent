package kubernetes

import "time"

// KubernetesConfig holds K8s adapter-specific configuration decoded from the adapter's Raw YAML node.
type KubernetesConfig struct {
	Kubeconfig        string            `yaml:"kubeconfig"`
	Namespace         string            `yaml:"namespace"`
	ResyncInterval    time.Duration     `yaml:"resync_interval"`
	GPUResourceNames  []string          `yaml:"gpu_resource_names"`
	NodeToRackMap     map[string]string `yaml:"node_to_rack_map"`
}

// DefaultGPUResourceNames returns the default GPU resource names if not configured.
var DefaultGPUResourceNames = []string{"nvidia.com/gpu", "amd.com/gpu"}

// DefaultResyncInterval is the default full resync period.
const DefaultResyncInterval = 5 * time.Minute

// ApplyDefaults fills in default values for missing config fields.
func (c *KubernetesConfig) ApplyDefaults() {
	if c.ResyncInterval <= 0 {
		c.ResyncInterval = DefaultResyncInterval
	}
	if len(c.GPUResourceNames) == 0 {
		c.GPUResourceNames = append([]string{}, DefaultGPUResourceNames...)
	}
	if c.NodeToRackMap == nil {
		c.NodeToRackMap = make(map[string]string)
	}
}
