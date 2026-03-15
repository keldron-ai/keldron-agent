// Package config handles loading and accessing the agent's YAML configuration.
// The config is designed to be atomically swappable to support hot-reload (S-006).
package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for the collector agent.
type Config struct {
	Agent       AgentConfig              `yaml:"agent"`
	Adapters    map[string]AdapterConfig `yaml:"adapters"`
	RackMapping map[string]string        `yaml:"rack_mapping"`
	Sender      SenderConfig             `yaml:"sender"`
	Buffer      BufferConfig             `yaml:"buffer"`
	Health      HealthConfig             `yaml:"health"`
}

// AgentConfig holds core agent settings.
type AgentConfig struct {
	ID              string        `yaml:"id"`
	LogLevel        string        `yaml:"log_level"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// AdapterConfig holds per-adapter settings.
// Common fields (Enabled, PollInterval, Endpoint) are parsed normally.
// The full YAML node is preserved in Raw so each adapter can decode its own
// specific config struct (e.g., DCGM needs use_stub, gpu_ids, collect.*).
type AdapterConfig struct {
	Enabled      bool          `yaml:"enabled"`
	PollInterval time.Duration `yaml:"poll_interval"`
	Endpoint     string        `yaml:"endpoint,omitempty"`
	Raw          yaml.Node     `yaml:"-"`
}

// UnmarshalYAML implements custom unmarshalling that preserves the full YAML
// node in Raw while also populating the common struct fields.
func (a *AdapterConfig) UnmarshalYAML(value *yaml.Node) error {
	// Preserve the full YAML node for adapter-specific decoding.
	a.Raw = *value

	// Decode common fields using an alias type to avoid infinite recursion.
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

// Defaults returns a Config with sensible default values.
func Defaults() *Config {
	return &Config{
		Agent: AgentConfig{
			ID:              "agent-default",
			LogLevel:        "info",
			ShutdownTimeout: 30 * time.Second,
		},
		Adapters:    make(map[string]AdapterConfig),
		RackMapping: make(map[string]string),
		Sender: SenderConfig{
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
// Adapters can register to merge their config into the main config (e.g. node_to_rack_map).
var (
	postLoadHooks   []func(*Config) error
	postLoadHooksMu sync.Mutex
)

// RegisterPostLoadHook adds a hook to run after config load. Nil functions are ignored.
func RegisterPostLoadHook(fn func(*Config) error) {
	if fn == nil {
		return
	}
	postLoadHooksMu.Lock()
	defer postLoadHooksMu.Unlock()
	postLoadHooks = append(postLoadHooks, fn)
}

// getPostLoadHooks returns a snapshot of the registered hooks.
func getPostLoadHooks() []func(*Config) error {
	postLoadHooksMu.Lock()
	defer postLoadHooksMu.Unlock()
	return append([]func(*Config) error(nil), postLoadHooks...)
}

// Load reads a YAML config file from path and returns the parsed Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := Defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

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

// Validate checks a config for correctness. Returns the first error found.
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

	if cfg.Agent.ShutdownTimeout <= 0 {
		return fmt.Errorf("agent.shutdown_timeout must be positive")
	}

	for name, acfg := range cfg.Adapters {
		if acfg.Enabled && acfg.PollInterval < time.Second {
			return fmt.Errorf("adapters.%s.poll_interval must be >= 1s (got %v)", name, acfg.PollInterval)
		}
	}

	anyAdapterEnabled := false
	for _, acfg := range cfg.Adapters {
		if acfg.Enabled {
			anyAdapterEnabled = true
			break
		}
	}
	if anyAdapterEnabled && cfg.Sender.Target == "" {
		return fmt.Errorf("sender.target must not be empty when any adapter is enabled")
	}

	if cfg.Buffer.RingSize <= 0 {
		return fmt.Errorf("buffer.ring_size must be > 0")
	}

	return nil
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

// Get returns the current config. Safe for concurrent use.
func (h *Holder) Get() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

// Update runs post-load hooks, validates, and applies a new config, then notifies subscribers.
// Returns error if hooks or validation fail (current config unchanged).
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

// Subscribe registers a callback for config changes and returns an unsubscribe function.
func (h *Holder) Subscribe(fn func(*Config)) func() {
	if fn == nil {
		return func() {}
	}
	h.subMu.Lock()
	// Reuse a tombstoned (nil) slot if available to prevent unbounded growth.
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
