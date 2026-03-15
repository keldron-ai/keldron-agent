package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
agent:
  id: "test-agent-01"
  log_level: "debug"
  shutdown_timeout: "15s"
adapters:
  dcgm:
    enabled: true
    poll_interval: "10s"
    endpoint: "localhost:5555"
output:
  prometheus: true
rack_mapping:
  node-01: "rack-A1"
sender:
  target: "localhost:50051"
  tls:
    enabled: true
  batch_size: 50
  flush_interval: "3s"
buffer:
  ring_size: 5000
  wal_dir: "/tmp/wal"
  wal_max_size: "100MB"
health:
  enabled: true
  bind: ":9090"
`
	path := writeTemp(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Agent.ID != "test-agent-01" {
		t.Errorf("agent.id = %q, want %q", cfg.Agent.ID, "test-agent-01")
	}
	if cfg.Agent.LogLevel != "debug" {
		t.Errorf("agent.log_level = %q, want %q", cfg.Agent.LogLevel, "debug")
	}
	if cfg.Agent.ShutdownTimeout != 15*time.Second {
		t.Errorf("agent.shutdown_timeout = %v, want %v", cfg.Agent.ShutdownTimeout, 15*time.Second)
	}
	if !cfg.Adapters["dcgm"].Enabled {
		t.Error("adapters.dcgm.enabled = false, want true")
	}
	if cfg.Adapters["dcgm"].PollInterval != 10*time.Second {
		t.Errorf("adapters.dcgm.poll_interval = %v, want %v", cfg.Adapters["dcgm"].PollInterval, 10*time.Second)
	}
	if cfg.RackMapping["node-01"] != "rack-A1" {
		t.Errorf("rack_mapping[node-01] = %q, want %q", cfg.RackMapping["node-01"], "rack-A1")
	}
	if cfg.Sender.BatchSize != 50 {
		t.Errorf("sender.batch_size = %d, want %d", cfg.Sender.BatchSize, 50)
	}
	if cfg.Buffer.RingSize != 5000 {
		t.Errorf("buffer.ring_size = %d, want %d", cfg.Buffer.RingSize, 5000)
	}
	if cfg.Health.Bind != ":9090" {
		t.Errorf("health.bind = %q, want %q", cfg.Health.Bind, ":9090")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	// OSS-004: Missing file uses defaults with auto-detection (no error).
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file (use defaults), got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config from defaults, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTemp(t, `{{{not valid yaml`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	content := `
agent:
  id: "test"
  log_level: "verbose"
  shutdown_timeout: "10s"
adapters:
  dcgm:
    enabled: true
output:
  prometheus: true
sender:
  target: "localhost:50051"
`
	path := writeTemp(t, content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid log_level, got nil")
	}
}

func TestLoad_MissingAgentID(t *testing.T) {
	content := `
agent:
  device_name: ""
  log_level: "info"
  shutdown_timeout: "10s"
adapters:
  dcgm:
    enabled: true
output:
  prometheus: true
sender:
  target: "localhost:50051"
`
	path := writeTemp(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ID is derived from hostname when device_name and id are empty.
	if cfg.Agent.ID == "" {
		t.Error("agent.id should be derived from hostname when empty")
	}
}

func TestLoad_Defaults(t *testing.T) {
	content := `
agent:
  id: "minimal"
  log_level: "info"
  shutdown_timeout: "5s"
adapters:
  dcgm:
    enabled: true
output:
  prometheus: true
sender:
  target: "localhost:50051"
`
	path := writeTemp(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Buffer defaults should apply when not specified in YAML.
	if cfg.Buffer.RingSize != 10000 {
		t.Errorf("buffer.ring_size default = %d, want %d", cfg.Buffer.RingSize, 10000)
	}
	if cfg.Health.Bind != ":8081" {
		t.Errorf("health.bind default = %q, want %q", cfg.Health.Bind, ":8081")
	}
}

func TestHolder_Get(t *testing.T) {
	cfg1 := Defaults()
	cfg1.Agent.ID = "first"
	cfg1.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}

	holder := NewHolder(cfg1)
	if got := holder.Get().Agent.ID; got != "first" {
		t.Errorf("holder.Get().Agent.ID = %q, want %q", got, "first")
	}
}

func TestHolder_Update_ValidConfig_NotifiesSubscribers(t *testing.T) {
	cfg1 := Defaults()
	cfg1.Agent.ID = "first"
	cfg1.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	holder := NewHolder(cfg1)

	notified := false
	var notifiedCfg *Config
	holder.Subscribe(func(cfg *Config) {
		notified = true
		notifiedCfg = cfg
	})

	cfg2 := Defaults()
	cfg2.Agent.ID = "second"
	cfg2.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	if err := holder.Update(cfg2); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !notified {
		t.Error("subscriber was not notified")
	}
	if notifiedCfg.Agent.ID != "second" {
		t.Errorf("notified config has id %q, want %q", notifiedCfg.Agent.ID, "second")
	}
	if holder.Get().Agent.ID != "second" {
		t.Errorf("holder.Get().Agent.ID = %q, want %q", holder.Get().Agent.ID, "second")
	}
}

func TestHolder_Update_InvalidConfig_ReturnsError_KeepsOldConfig(t *testing.T) {
	cfg1 := Defaults()
	cfg1.Agent.ID = "first"
	cfg1.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	holder := NewHolder(cfg1)

	cfg2 := Defaults()
	cfg2.Agent.ID = "" // invalid
	cfg2.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	err := holder.Update(cfg2)
	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
	if holder.Get().Agent.ID != "first" {
		t.Errorf("config was changed despite error: got %q, want %q", holder.Get().Agent.ID, "first")
	}
}

func TestValidate(t *testing.T) {
	validCfg := func() *Config {
		c := Defaults()
		c.Agent.ID = "test"
		c.Agent.LogLevel = "info"
		c.Agent.ShutdownTimeout = 10 * time.Second
		c.Sender.Target = "localhost:443"
		c.Buffer.RingSize = 1000
		c.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
		return c
	}

	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "valid",
			modify:  func(c *Config) {},
			wantErr: "",
		},
		{
			name:    "empty agent id",
			modify:  func(c *Config) { c.Agent.ID = "" },
			wantErr: "agent.id",
		},
		{
			name:    "invalid log level",
			modify:  func(c *Config) { c.Agent.LogLevel = "verbose" },
			wantErr: "log_level",
		},
		{
			name:    "shutdown timeout zero",
			modify:  func(c *Config) { c.Agent.ShutdownTimeout = 0 },
			wantErr: "shutdown_timeout",
		},
		{
			name: "poll interval below 1s when enabled",
			modify: func(c *Config) {
				c.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 500 * time.Millisecond}
			},
			wantErr: "poll_interval",
		},
		{
			name: "empty sender target and cloud key when adapter enabled",
			modify: func(c *Config) {
				c.Sender.Target = ""
				c.Cloud.APIKey = ""
				c.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
			},
			wantErr: "sender.target",
		},
		{
			name:    "zero ring size",
			modify:  func(c *Config) { c.Buffer.RingSize = 0 },
			wantErr: "ring_size",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validCfg()
			tt.modify(cfg)
			err := Validate(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestHolder_ConcurrentGetUpdate(t *testing.T) {
	cfg := Defaults()
	cfg.Agent.ID = "initial"
	cfg.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	holder := NewHolder(cfg)

	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			_ = holder.Get()
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			c := Defaults()
			c.Agent.ID = "concurrent"
			c.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
			_ = holder.Update(c)
		}
		done <- true
	}()
	<-done
	<-done
}

func TestLoad_AdapterRawNodePreserved(t *testing.T) {
	content := `
agent:
  id: "test-raw"
  log_level: "info"
  shutdown_timeout: "5s"
adapters:
  dcgm:
    enabled: true
    poll_interval: "10s"
    endpoint: "localhost:5555"
    use_stub: true
    gpu_ids: [0, 1]
    collect:
      temperature: true
      power: true
output:
  prometheus: true
sender:
  target: "platform.example.com:443"
`
	path := writeTemp(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dcgm := cfg.Adapters["dcgm"]
	if !dcgm.Enabled {
		t.Fatal("dcgm.enabled = false, want true")
	}
	if dcgm.PollInterval != 10*time.Second {
		t.Errorf("dcgm.poll_interval = %v, want 10s", dcgm.PollInterval)
	}

	// The Raw node should contain the adapter-specific fields.
	if dcgm.Raw.Kind == 0 {
		t.Fatal("dcgm.Raw node is empty, want populated YAML node")
	}

	// Verify we can decode adapter-specific config from the Raw node.
	var specific struct {
		UseStub bool  `yaml:"use_stub"`
		GPUIDs  []int `yaml:"gpu_ids"`
		Collect struct {
			Temperature bool `yaml:"temperature"`
			Power       bool `yaml:"power"`
		} `yaml:"collect"`
	}
	if err := dcgm.Raw.Decode(&specific); err != nil {
		t.Fatalf("decoding Raw node: %v", err)
	}
	if !specific.UseStub {
		t.Error("use_stub = false, want true")
	}
	if len(specific.GPUIDs) != 2 || specific.GPUIDs[0] != 0 || specific.GPUIDs[1] != 1 {
		t.Errorf("gpu_ids = %v, want [0 1]", specific.GPUIDs)
	}
	if !specific.Collect.Temperature {
		t.Error("collect.temperature = false, want true")
	}
	if !specific.Collect.Power {
		t.Error("collect.power = false, want true")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	content := `
agent:
  id: "from-yaml"
  log_level: "info"
  poll_interval: "60s"
adapters:
  dcgm:
    enabled: true
output:
  prometheus: true
  prometheus_port: 9090
sender:
  target: "localhost:50051"
`
	path := writeTemp(t, content)

	os.Setenv("KELDRON_AGENT_LOG_LEVEL", "debug")
	os.Setenv("KELDRON_AGENT_POLL_INTERVAL", "10s")
	os.Setenv("KELDRON_OUTPUT_PROMETHEUS_PORT", "9200")
	defer func() {
		os.Unsetenv("KELDRON_AGENT_LOG_LEVEL")
		os.Unsetenv("KELDRON_AGENT_POLL_INTERVAL")
		os.Unsetenv("KELDRON_OUTPUT_PROMETHEUS_PORT")
	}()

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Agent.LogLevel != "debug" {
		t.Errorf("log_level = %q, want debug (env override)", cfg.Agent.LogLevel)
	}
	if cfg.Agent.PollInterval != 10*time.Second {
		t.Errorf("poll_interval = %v, want 10s (env override)", cfg.Agent.PollInterval)
	}
	if cfg.Output.PrometheusPort != 9200 {
		t.Errorf("prometheus_port = %d, want 9200 (env override)", cfg.Output.PrometheusPort)
	}

	// Boolean env override
	os.Setenv("KELDRON_OUTPUT_STDOUT", "true")
	defer os.Unsetenv("KELDRON_OUTPUT_STDOUT")
	cfg2, err := Load(path)
	if err != nil {
		t.Fatalf("Load after KELDRON_OUTPUT_STDOUT override: %v", err)
	}
	if !cfg2.Output.Stdout {
		t.Error("output.stdout should be true via env override")
	}
}

func TestLoad_InvalidPollInterval(t *testing.T) {
	content := `
agent:
  id: "test"
  log_level: "info"
  poll_interval: "1s"
  shutdown_timeout: "10s"
adapters:
  dcgm:
    enabled: true
output:
  prometheus: true
sender:
  target: "localhost:50051"
`
	path := writeTemp(t, content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for poll_interval < 5s, got nil")
	}
	if !strings.Contains(err.Error(), "poll_interval") {
		t.Errorf("error %q does not mention poll_interval", err.Error())
	}
}

func TestLoad_NoAdaptersEnabled(t *testing.T) {
	// Explicitly disable all auto-detectable adapters so none are enabled.
	content := `
agent:
  id: "test"
  log_level: "info"
adapters:
  apple_silicon:
    enabled: false
  nvidia_consumer:
    enabled: false
  dcgm:
    enabled: false
  rocm:
    enabled: false
  linux_thermal:
    enabled: false
  kubernetes:
    enabled: false
output:
  prometheus: true
sender:
  target: "localhost:50051"
`
	path := writeTemp(t, content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error when no adapters enabled, got nil")
	}
	if !strings.Contains(err.Error(), "adapter") {
		t.Errorf("error %q does not mention adapter", err.Error())
	}
}

func TestLoad_CloudKeyWithoutEndpoint(t *testing.T) {
	content := `
agent:
  id: "test"
  log_level: "info"
adapters:
  dcgm:
    enabled: true
output:
  prometheus: true
cloud:
  api_key: "kld_test123"
  endpoint: ""
sender:
  target: "localhost:50051"
`
	path := writeTemp(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Validation sets default endpoint when api_key is set and endpoint empty.
	if cfg.Cloud.Endpoint == "" {
		t.Error("cloud.endpoint should be set to default when api_key set")
	}
}

func TestMaskedCloudAPIKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"ab", "***"},
		{"abcdefgh", "***"},
		{"kld_1234567890", "kld_***7890"},
	}
	for _, tt := range tests {
		got := MaskedCloudAPIKey(tt.in)
		if got != tt.want {
			t.Errorf("MaskedCloudAPIKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestApplyAutoDetection_DarwinArm64(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("skipping: test only applies on darwin/arm64")
	}

	load := defaultConfigLoad()
	load.Adapters.AppleSilicon.Enabled = nil
	load.Adapters.NVIDIAConsumer.Enabled = nil

	ApplyAutoDetection(load)

	// On darwin/arm64, Apple Silicon should be enabled
	if true {
		if load.Adapters.AppleSilicon.Enabled == nil || !*load.Adapters.AppleSilicon.Enabled {
			t.Error("apple_silicon should be enabled on darwin/arm64")
		}
	}
}

func TestToAdapterMap(t *testing.T) {
	a := &AdaptersConfig{}
	v := true
	a.DCGM.Enabled = &v
	a.DCGM.Raw = yaml.Node{Kind: yaml.MappingNode}

	m := ToAdapterMap(a, 30*time.Second)
	if len(m) != 1 {
		t.Fatalf("expected 1 adapter, got %d", len(m))
	}
	acfg, ok := m["dcgm"]
	if !ok {
		t.Fatal("expected dcgm in map")
	}
	if !acfg.Enabled {
		t.Error("dcgm should be enabled")
	}
	if acfg.PollInterval != 30*time.Second {
		t.Errorf("poll_interval = %v, want 30s", acfg.PollInterval)
	}

	// Test multiple adapters
	a2 := &AdaptersConfig{}
	a2.Slurm.Enabled = &v
	a2.Kubernetes.Enabled = &v
	m2 := ToAdapterMap(a2, 20*time.Second)
	if len(m2) != 2 {
		t.Errorf("expected 2 adapters, got %d", len(m2))
	}
}

func TestHolder_Subscribe_Unsubscribe(t *testing.T) {
	cfg := Defaults()
	cfg.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	holder := NewHolder(cfg)

	unsub := holder.Subscribe(func(cfg *Config) {})
	unsub() // Unsubscribe
	// Update should not panic
	cfg2 := Defaults()
	cfg2.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	if err := holder.Update(cfg2); err != nil {
		t.Fatalf("Update after unsubscribe: %v", err)
	}
}

func TestValidate_HubEnabledNoPort(t *testing.T) {
	cfg := Defaults()
	cfg.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
	cfg.Hub.Enabled = true
	cfg.Hub.ListenPort = 0

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for hub.enabled with listen_port 0")
	}
	if !strings.Contains(err.Error(), "listen_port") {
		t.Errorf("error %q does not mention listen_port", err.Error())
	}
}

func TestApplyEnvOverrides_HubAndCloud(t *testing.T) {
	load := defaultConfigLoad()
	os.Setenv("KELDRON_HUB_ENABLED", "true")
	os.Setenv("KELDRON_HUB_STATIC_PEERS", "192.168.1.10:9100, 192.168.1.11:9100")
	os.Setenv("KELDRON_CLOUD_API_KEY", "kld_test")
	os.Setenv("KELDRON_CLOUD_ENDPOINT", "https://custom.api.example.com")
	defer func() {
		os.Unsetenv("KELDRON_HUB_ENABLED")
		os.Unsetenv("KELDRON_HUB_STATIC_PEERS")
		os.Unsetenv("KELDRON_CLOUD_API_KEY")
		os.Unsetenv("KELDRON_CLOUD_ENDPOINT")
	}()

	ApplyEnvOverrides(load)

	if !load.Hub.Enabled {
		t.Error("hub.enabled should be true")
	}
	if len(load.Hub.StaticPeers) != 2 {
		t.Errorf("static_peers = %v, want 2 entries", load.Hub.StaticPeers)
	}
	if load.Cloud.APIKey != "kld_test" {
		t.Errorf("cloud.api_key = %q", load.Cloud.APIKey)
	}
	if load.Cloud.Endpoint != "https://custom.api.example.com" {
		t.Errorf("cloud.endpoint = %q", load.Cloud.Endpoint)
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}
