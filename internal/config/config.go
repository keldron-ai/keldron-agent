// Package config handles loading and accessing the agent's YAML configuration.
// OSS-004: Single-file YAML with env overrides, auto-detection, and validation.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for the collector agent (runtime representation).
type Config struct {
	Agent       AgentConfig              `yaml:"agent"`
	Adapters    map[string]AdapterConfig `yaml:"-"` // Populated from AdaptersConfig for registry
	Output      OutputConfig             `yaml:"output"`
	Hub         HubConfig                `yaml:"hub"`
	Cloud       CloudConfig              `yaml:"cloud"`
	RackMapping map[string]string        `yaml:"rack_mapping"`
	Sender      SenderConfig             `yaml:"sender"`
	Buffer      BufferConfig             `yaml:"buffer"`
	Health      HealthConfig             `yaml:"health"`
}

// configLoad is the YAML parsing structure (OSS schema).
type configLoad struct {
	Agent       AgentConfig       `yaml:"agent"`
	Adapters    AdaptersConfig    `yaml:"adapters"`
	Output      OutputConfig      `yaml:"output"`
	Hub         HubConfig         `yaml:"hub"`
	Cloud       CloudConfig       `yaml:"cloud"`
	RackMapping map[string]string `yaml:"rack_mapping"`
	Sender      SenderConfig      `yaml:"sender"`
	Buffer      BufferConfig      `yaml:"buffer"`
	Health      HealthConfig      `yaml:"health"`
}

// AgentConfig holds core agent settings.
type AgentConfig struct {
	ID              string        `yaml:"id"`
	DeviceName      string        `yaml:"device_name"`
	PollInterval    time.Duration `yaml:"poll_interval"`
	LogLevel        string        `yaml:"log_level"`
	ElectricityRate float64       `yaml:"electricity_rate"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// AdaptersConfig holds per-adapter configuration (OSS schema).
type AdaptersConfig struct {
	AppleSilicon   AppleSiliconConfig   `yaml:"apple_silicon"`
	NVIDIAConsumer NVIDIAConsumerConfig `yaml:"nvidia_consumer"`
	DCGM           DCGMConfig           `yaml:"dcgm"`
	ROCm           ROCmConfig           `yaml:"rocm"`
	LinuxThermal   LinuxThermalConfig   `yaml:"linux_thermal"`
	SNMPPDU        SNMPPDUConfig        `yaml:"snmp_pdu"`
	Temperature    TemperatureConfig    `yaml:"temperature"`
	Kubernetes     KubernetesConfig     `yaml:"kubernetes"`
	Slurm          SlurmConfig          `yaml:"slurm"`
}

// AppleSiliconConfig holds Apple Silicon adapter settings.
type AppleSiliconConfig struct {
	Enabled *bool `yaml:"enabled"` // nil = auto-detect
}

// NVIDIAConsumerConfig holds NVIDIA consumer (nvidia-smi) adapter settings.
type NVIDIAConsumerConfig struct {
	Enabled *bool `yaml:"enabled"`
}

// DCGMConfig holds DCGM adapter settings.
type DCGMConfig struct {
	Enabled *bool     `yaml:"enabled"`
	Raw     yaml.Node `yaml:"-"`
}

// UnmarshalYAML preserves the full YAML node for adapter-specific decoding.
func (d *DCGMConfig) UnmarshalYAML(value *yaml.Node) error {
	d.Raw = *value
	type plain DCGMConfig
	return value.Decode((*plain)(d))
}

// ROCmConfig holds ROCm adapter settings.
type ROCmConfig struct {
	Enabled *bool     `yaml:"enabled"`
	Raw     yaml.Node `yaml:"-"`
}

// UnmarshalYAML preserves the full YAML node for adapter-specific decoding.
func (r *ROCmConfig) UnmarshalYAML(value *yaml.Node) error {
	r.Raw = *value
	type plain ROCmConfig
	return value.Decode((*plain)(r))
}

// LinuxThermalConfig holds Linux thermal adapter settings.
type LinuxThermalConfig struct {
	Enabled   *bool  `yaml:"enabled"`
	HwmonPath string `yaml:"hwmon_path"`
}

// SNMPPDUConfig holds SNMP PDU adapter settings.
type SNMPPDUConfig struct {
	Enabled *bool     `yaml:"enabled"`
	Raw     yaml.Node `yaml:"-"`
}

// UnmarshalYAML preserves the full YAML node for adapter-specific decoding.
func (s *SNMPPDUConfig) UnmarshalYAML(value *yaml.Node) error {
	s.Raw = *value
	type plain SNMPPDUConfig
	return value.Decode((*plain)(s))
}

// TemperatureConfig holds temperature sensor adapter settings.
type TemperatureConfig struct {
	Enabled *bool     `yaml:"enabled"`
	Raw     yaml.Node `yaml:"-"`
}

// UnmarshalYAML preserves the full YAML node for adapter-specific decoding.
func (t *TemperatureConfig) UnmarshalYAML(value *yaml.Node) error {
	t.Raw = *value
	type plain TemperatureConfig
	return value.Decode((*plain)(t))
}

// KubernetesConfig holds Kubernetes adapter settings.
type KubernetesConfig struct {
	Enabled *bool     `yaml:"enabled"`
	Raw     yaml.Node `yaml:"-"`
}

// UnmarshalYAML preserves the full YAML node for adapter-specific decoding.
func (k *KubernetesConfig) UnmarshalYAML(value *yaml.Node) error {
	k.Raw = *value
	type plain KubernetesConfig
	return value.Decode((*plain)(k))
}

// SlurmConfig holds Slurm adapter settings.
type SlurmConfig struct {
	Enabled *bool     `yaml:"enabled"`
	Raw     yaml.Node `yaml:"-"`
}

// UnmarshalYAML preserves the full YAML node for adapter-specific decoding.
func (s *SlurmConfig) UnmarshalYAML(value *yaml.Node) error {
	s.Raw = *value
	type plain SlurmConfig
	return value.Decode((*plain)(s))
}

// OutputConfig holds output mode settings.
type OutputConfig struct {
	Stdout         bool `yaml:"stdout"`
	Prometheus     bool `yaml:"prometheus"`
	PrometheusPort int  `yaml:"prometheus_port"`
}

// HubConfig holds hub aggregator settings.
type HubConfig struct {
	Enabled     bool     `yaml:"enabled"`
	MDNSEnabled bool     `yaml:"mdns_enabled"`
	StaticPeers []string `yaml:"static_peers"`
	ListenPort  int      `yaml:"listen_port"`
}

// CloudConfig holds cloud API settings.
type CloudConfig struct {
	APIKey   string `yaml:"api_key"`
	Endpoint string `yaml:"endpoint"`
}

// AdapterConfig holds per-adapter settings for the registry.
type AdapterConfig struct {
	Enabled      bool          `yaml:"enabled"`
	PollInterval time.Duration `yaml:"poll_interval"`
	Endpoint     string        `yaml:"endpoint,omitempty"`
	Raw          yaml.Node     `yaml:"-"`
}

// UnmarshalYAML implements custom unmarshalling that preserves the full YAML node in Raw.
func (a *AdapterConfig) UnmarshalYAML(value *yaml.Node) error {
	a.Raw = *value
	type plain AdapterConfig
	return value.Decode((*plain)(a))
}

// SenderConfig holds gRPC sender settings.
type SenderConfig struct {
	Target        string        `yaml:"target"`
	TLS           TLSConfig     `yaml:"tls"`
	BatchSize     int           `yaml:"batch_size"`
	FlushInterval time.Duration `yaml:"flush_interval"`
}

// TLSConfig holds TLS certificate paths.
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

// BufferConfig holds ring buffer and WAL settings.
type BufferConfig struct {
	RingSize   int    `yaml:"ring_size"`
	WALDir     string `yaml:"wal_dir"`
	WALMaxSize string `yaml:"wal_max_size"`
}

// HealthConfig holds HTTP health endpoint settings.
type HealthConfig struct {
	Enabled bool   `yaml:"enabled"`
	Bind    string `yaml:"bind"`
}

// Default cloud endpoint when api_key is set but endpoint is empty.
const defaultCloudEndpoint = "https://api.keldron.ai"

// Defaults returns a Config with sensible default values.
func Defaults() *Config {
	return &Config{
		Agent: AgentConfig{
			ID:              "agent-default",
			DeviceName:      "",
			PollInterval:    30 * time.Second,
			LogLevel:        "info",
			ElectricityRate: 0.12,
			ShutdownTimeout: 30 * time.Second,
		},
		Adapters:    make(map[string]AdapterConfig),
		RackMapping: make(map[string]string),
		Output: OutputConfig{
			Stdout:         false,
			Prometheus:     true,
			PrometheusPort: 9100,
		},
		Hub: HubConfig{
			Enabled:     false,
			MDNSEnabled: false,
			StaticPeers: nil,
			ListenPort:  9200,
		},
		Cloud: CloudConfig{},
		Sender: SenderConfig{
			Target:        "localhost:50051",
			BatchSize:     100,
			FlushInterval: 5 * time.Second,
		},
		Buffer: BufferConfig{
			RingSize:   10000,
			WALDir:     "/var/lib/keldron-agent/wal",
			WALMaxSize: "500MB",
		},
		Health: HealthConfig{
			Enabled: true,
			Bind:    ":8081",
		},
	}
}

// defaultConfigLoad returns configLoad with defaults for parsing.
func defaultConfigLoad() *configLoad {
	return &configLoad{
		Agent: AgentConfig{
			PollInterval:    30 * time.Second,
			LogLevel:        "info",
			ElectricityRate: 0.12,
			ShutdownTimeout: 30 * time.Second,
		},
		Output: OutputConfig{
			Prometheus:     true,
			PrometheusPort: 9100,
		},
		Hub: HubConfig{
			ListenPort: 9200,
		},
		Adapters: AdaptersConfig{
			LinuxThermal: LinuxThermalConfig{
				HwmonPath: "/sys/class/hwmon",
			},
		},
		Sender: SenderConfig{
			Target:        "localhost:50051",
			BatchSize:     100,
			FlushInterval: 5 * time.Second,
		},
		Buffer: BufferConfig{
			RingSize:   10000,
			WALDir:     "/var/lib/keldron-agent/wal",
			WALMaxSize: "500MB",
		},
		Health: HealthConfig{
			Enabled: true,
			Bind:    ":8081",
		},
	}
}

// postLoadHooks are called after unmarshaling and before Validate.
var (
	postLoadHooks   []func(*Config) error
	postLoadHooksMu sync.Mutex
)

// RegisterPostLoadHook adds a hook to run after config load.
func RegisterPostLoadHook(fn func(*Config) error) {
	if fn == nil {
		return
	}
	postLoadHooksMu.Lock()
	defer postLoadHooksMu.Unlock()
	postLoadHooks = append(postLoadHooks, fn)
}

func getPostLoadHooks() []func(*Config) error {
	postLoadHooksMu.Lock()
	defer postLoadHooksMu.Unlock()
	return append([]func(*Config) error(nil), postLoadHooks...)
}

// Load reads a YAML config file from path and returns the parsed Config.
// If file is not found, uses defaults with auto-detection.
func Load(path string) (*Config, error) {
	load := defaultConfigLoad()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Use defaults, apply auto-detect
			ApplyAutoDetection(load)
			cfg := toConfig(load)
			for _, hook := range getPostLoadHooks() {
				if err := hook(cfg); err != nil {
					return nil, fmt.Errorf("post-load hook: %w", err)
				}
			}
			if err := Validate(cfg); err != nil {
				return nil, fmt.Errorf("validating config: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, load); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	ApplyEnvOverrides(load)
	ApplyAutoDetection(load)

	cfg := toConfig(load)

	for _, hook := range getPostLoadHooks() {
		if err := hook(cfg); err != nil {
			return nil, fmt.Errorf("post-load hook: %w", err)
		}
	}

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// toConfig converts configLoad to Config, populating Adapters map.
func toConfig(load *configLoad) *Config {
	cfg := &Config{
		Agent:       load.Agent,
		Output:      load.Output,
		Hub:         load.Hub,
		Cloud:       load.Cloud,
		RackMapping: load.RackMapping,
		Sender:      load.Sender,
		Buffer:      load.Buffer,
		Health:      load.Health,
		Adapters:    ToAdapterMap(&load.Adapters, load.Agent.PollInterval),
	}
	if cfg.RackMapping == nil {
		cfg.RackMapping = make(map[string]string)
	}
	if cfg.Cloud.APIKey != "" && cfg.Cloud.Endpoint == "" {
		cfg.Cloud.Endpoint = defaultCloudEndpoint
	}
	// Derive Agent.ID from DeviceName or hostname
	if cfg.Agent.ID == "" {
		cfg.Agent.ID = cfg.Agent.DeviceName
		if cfg.Agent.ID == "" {
			if h, err := os.Hostname(); err == nil {
				cfg.Agent.ID = h
			} else {
				cfg.Agent.ID = "agent-default"
			}
		}
	}
	return cfg
}

// ToAdapterMap converts AdaptersConfig to map[string]AdapterConfig for registry.
// Only includes adapters that are enabled. Adapters without constructors (apple_silicon,
// nvidia_consumer, linux_thermal) are included when enabled; registry will skip if not registered.
func ToAdapterMap(a *AdaptersConfig, pollInterval time.Duration) map[string]AdapterConfig {
	m := make(map[string]AdapterConfig)
	if pollInterval <= 0 {
		pollInterval = 30 * time.Second
	}

	add := func(name string, enabled bool, raw yaml.Node) {
		if !enabled {
			return
		}
		pi := pollInterval
		if raw.Kind != 0 {
			var common struct {
				PollInterval time.Duration `yaml:"poll_interval"`
			}
			if err := raw.Decode(&common); err == nil && common.PollInterval > 0 {
				pi = common.PollInterval
			}
		}
		m[name] = AdapterConfig{
			Enabled:      true,
			PollInterval: pi,
			Raw:          raw,
		}
	}

	if a == nil {
		return m
	}

	if v := a.AppleSilicon.Enabled; v != nil && *v {
		add("apple_silicon", true, yaml.Node{})
	}
	if v := a.NVIDIAConsumer.Enabled; v != nil && *v {
		add("nvidia_consumer", true, yaml.Node{})
	}
	if v := a.DCGM.Enabled; v != nil && *v {
		add("dcgm", true, a.DCGM.Raw)
	}
	if v := a.ROCm.Enabled; v != nil && *v {
		add("rocm", true, a.ROCm.Raw)
	}
	if v := a.LinuxThermal.Enabled; v != nil && *v {
		add("linux_thermal", true, yaml.Node{})
	}
	if v := a.SNMPPDU.Enabled; v != nil && *v {
		add("snmp_pdu", true, a.SNMPPDU.Raw)
	}
	if v := a.Temperature.Enabled; v != nil && *v {
		add("temperature", true, a.Temperature.Raw)
	}
	if v := a.Kubernetes.Enabled; v != nil && *v {
		add("kubernetes", true, a.Kubernetes.Raw)
	}
	if v := a.Slurm.Enabled; v != nil && *v {
		add("slurm", true, a.Slurm.Raw)
	}

	return m
}

// ApplyEnvOverrides applies KELDRON_* environment variables to configLoad.
func ApplyEnvOverrides(load *configLoad) {
	if v := os.Getenv("KELDRON_AGENT_DEVICE_NAME"); v != "" {
		load.Agent.DeviceName = v
	}
	if v := os.Getenv("KELDRON_AGENT_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			load.Agent.PollInterval = d
		}
	}
	if v := os.Getenv("KELDRON_AGENT_LOG_LEVEL"); v != "" {
		load.Agent.LogLevel = v
	}
	if v := os.Getenv("KELDRON_AGENT_ELECTRICITY_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			load.Agent.ElectricityRate = f
		}
	}
	if v := os.Getenv("KELDRON_ADAPTERS_APPLE_SILICON_ENABLED"); v != "" {
		b := parseBool(v)
		load.Adapters.AppleSilicon.Enabled = &b
	}
	if v := os.Getenv("KELDRON_ADAPTERS_NVIDIA_CONSUMER_ENABLED"); v != "" {
		b := parseBool(v)
		load.Adapters.NVIDIAConsumer.Enabled = &b
	}
	if v := os.Getenv("KELDRON_ADAPTERS_DCGM_ENABLED"); v != "" {
		b := parseBool(v)
		load.Adapters.DCGM.Enabled = &b
	}
	if v := os.Getenv("KELDRON_ADAPTERS_ROCM_ENABLED"); v == "" {
		if v = os.Getenv("KELDRON_ADAPTERS_ROCm_ENABLED"); v != "" {
			b := parseBool(v)
			load.Adapters.ROCm.Enabled = &b
		}
	} else {
		b := parseBool(v)
		load.Adapters.ROCm.Enabled = &b
	}
	if v := os.Getenv("KELDRON_ADAPTERS_LINUX_THERMAL_ENABLED"); v != "" {
		b := parseBool(v)
		load.Adapters.LinuxThermal.Enabled = &b
	}
	if v := os.Getenv("KELDRON_ADAPTERS_SNMP_PDU_ENABLED"); v != "" {
		b := parseBool(v)
		load.Adapters.SNMPPDU.Enabled = &b
	}
	if v := os.Getenv("KELDRON_ADAPTERS_TEMPERATURE_ENABLED"); v != "" {
		b := parseBool(v)
		load.Adapters.Temperature.Enabled = &b
	}
	if v := os.Getenv("KELDRON_ADAPTERS_KUBERNETES_ENABLED"); v != "" {
		b := parseBool(v)
		load.Adapters.Kubernetes.Enabled = &b
	}
	if v := os.Getenv("KELDRON_ADAPTERS_SLURM_ENABLED"); v != "" {
		b := parseBool(v)
		load.Adapters.Slurm.Enabled = &b
	}
	if v := os.Getenv("KELDRON_OUTPUT_STDOUT"); v != "" {
		load.Output.Stdout = parseBool(v)
	}
	if v := os.Getenv("KELDRON_OUTPUT_PROMETHEUS"); v != "" {
		load.Output.Prometheus = parseBool(v)
	}
	if v := os.Getenv("KELDRON_OUTPUT_PROMETHEUS_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			load.Output.PrometheusPort = p
		}
	}
	if v := os.Getenv("KELDRON_HUB_ENABLED"); v != "" {
		load.Hub.Enabled = parseBool(v)
	}
	if v := os.Getenv("KELDRON_HUB_MDNS_ENABLED"); v != "" {
		load.Hub.MDNSEnabled = parseBool(v)
	}
	if v := os.Getenv("KELDRON_HUB_STATIC_PEERS"); v != "" {
		load.Hub.StaticPeers = strings.Split(v, ",")
		for i, p := range load.Hub.StaticPeers {
			load.Hub.StaticPeers[i] = strings.TrimSpace(p)
		}
	}
	if v := os.Getenv("KELDRON_HUB_LISTEN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			load.Hub.ListenPort = p
		}
	}
	if v := os.Getenv("KELDRON_CLOUD_API_KEY"); v != "" {
		load.Cloud.APIKey = v
	}
	if v := os.Getenv("KELDRON_CLOUD_ENDPOINT"); v != "" {
		load.Cloud.Endpoint = v
	}
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}

// ApplyAutoDetection sets adapter enabled flags when not explicitly set.
func ApplyAutoDetection(load *configLoad) {
	if load.Adapters.AppleSilicon.Enabled == nil {
		v := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
		load.Adapters.AppleSilicon.Enabled = &v
	}
	if load.Adapters.NVIDIAConsumer.Enabled == nil {
		_, err := exec.LookPath("nvidia-smi")
		v := err == nil
		load.Adapters.NVIDIAConsumer.Enabled = &v
	}
	if load.Adapters.DCGM.Enabled == nil {
		// Check if nv-hostengine is running (simplified: check if nvidia-smi works as proxy for NVIDIA stack)
		_, err := exec.LookPath("nv-hostengine")
		v := err == nil
		load.Adapters.DCGM.Enabled = &v
	}
	if load.Adapters.ROCm.Enabled == nil {
		_, err := exec.LookPath("rocm-smi")
		v := err == nil
		load.Adapters.ROCm.Enabled = &v
	}
	if load.Adapters.LinuxThermal.Enabled == nil {
		_, err := os.Stat("/sys/class/hwmon")
		v := err == nil
		load.Adapters.LinuxThermal.Enabled = &v
	}
	if load.Adapters.Kubernetes.Enabled == nil {
		v := os.Getenv("KUBERNETES_SERVICE_HOST") != ""
		load.Adapters.Kubernetes.Enabled = &v
	}
	// Slurm, SNMP PDU, Temperature: disabled by default (nil = false)
	if load.Adapters.Slurm.Enabled == nil {
		v := false
		load.Adapters.Slurm.Enabled = &v
	}
	if load.Adapters.SNMPPDU.Enabled == nil {
		v := false
		load.Adapters.SNMPPDU.Enabled = &v
	}
	if load.Adapters.Temperature.Enabled == nil {
		v := false
		load.Adapters.Temperature.Enabled = &v
	}
}

// Validate checks a config for correctness.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config must not be nil")
	}
	if cfg.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	switch cfg.Agent.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("agent.log_level must be one of: debug, info, warn, error (got %q)", cfg.Agent.LogLevel)
	}
	if cfg.Agent.PollInterval < 5*time.Second || cfg.Agent.PollInterval > 5*time.Minute {
		return fmt.Errorf("agent.poll_interval must be between 5s and 5m (got %v)", cfg.Agent.PollInterval)
	}
	if cfg.Agent.ShutdownTimeout <= 0 {
		return fmt.Errorf("agent.shutdown_timeout must be positive")
	}

	anyAdapterEnabled := false
	for _, acfg := range cfg.Adapters {
		if acfg.Enabled {
			anyAdapterEnabled = true
			break
		}
	}
	if !anyAdapterEnabled {
		return fmt.Errorf("at least one adapter must be enabled")
	}

	anyOutputEnabled := cfg.Output.Stdout || cfg.Output.Prometheus || (cfg.Cloud.APIKey != "")
	if !anyOutputEnabled {
		return fmt.Errorf("at least one output must be enabled (stdout, prometheus, or cloud.api_key)")
	}

	if cfg.Hub.Enabled && cfg.Hub.ListenPort <= 0 {
		return fmt.Errorf("hub.listen_port must be > 0 when hub.enabled is true")
	}

	for name, acfg := range cfg.Adapters {
		if acfg.Enabled && acfg.PollInterval < time.Second {
			return fmt.Errorf("adapters.%s.poll_interval must be >= 1s (got %v)", name, acfg.PollInterval)
		}
	}

	if anyAdapterEnabled && cfg.Sender.Target == "" && cfg.Cloud.APIKey == "" {
		return fmt.Errorf("sender.target or cloud.api_key must be set when adapters are enabled")
	}

	if cfg.Buffer.RingSize <= 0 {
		return fmt.Errorf("buffer.ring_size must be > 0")
	}

	return nil
}

// MaskedCloudAPIKey returns the cloud API key masked for logging.
func MaskedCloudAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "***" + apiKey[len(apiKey)-4:]
}

// Holder provides atomic access to the current config and notifies subscribers on changes.
type Holder struct {
	current *Config
	mu      sync.RWMutex
	subs    []func(*Config)
	subMu   sync.Mutex
}

// NewHolder creates a Holder initialized with the given config.
func NewHolder(initial *Config) *Holder {
	return &Holder{current: initial}
}

// Get returns the current config.
func (h *Holder) Get() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

// Update runs post-load hooks, validates, and applies a new config.
func (h *Holder) Update(newCfg *Config) error {
	if newCfg == nil {
		return fmt.Errorf("config must not be nil")
	}
	for _, hook := range getPostLoadHooks() {
		if err := hook(newCfg); err != nil {
			return fmt.Errorf("post-load hook: %w", err)
		}
	}
	if err := Validate(newCfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	h.mu.Lock()
	h.current = newCfg
	h.mu.Unlock()

	h.subMu.Lock()
	subs := append([]func(*Config){}, h.subs...)
	h.subMu.Unlock()

	for _, fn := range subs {
		if fn != nil {
			fn(newCfg)
		}
	}
	return nil
}

// Subscribe registers a callback for config changes.
func (h *Holder) Subscribe(fn func(*Config)) func() {
	if fn == nil {
		return func() {}
	}
	h.subMu.Lock()
	idx := -1
	for i, s := range h.subs {
		if s == nil {
			idx = i
			break
		}
	}
	if idx >= 0 {
		h.subs[idx] = fn
	} else {
		idx = len(h.subs)
		h.subs = append(h.subs, fn)
	}
	h.subMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			h.subMu.Lock()
			defer h.subMu.Unlock()
			if idx < len(h.subs) {
				h.subs[idx] = nil
			}
		})
	}
}
