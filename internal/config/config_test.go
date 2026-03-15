package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
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
  id: ""
  log_level: "info"
  shutdown_timeout: "10s"
`
	path := writeTemp(t, content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty agent.id, got nil")
	}
}

func TestLoad_Defaults(t *testing.T) {
	content := `
agent:
  id: "minimal"
  log_level: "info"
  shutdown_timeout: "5s"
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

	holder := NewHolder(cfg1)
	if got := holder.Get().Agent.ID; got != "first" {
		t.Errorf("holder.Get().Agent.ID = %q, want %q", got, "first")
	}
}

func TestHolder_Update_ValidConfig_NotifiesSubscribers(t *testing.T) {
	cfg1 := Defaults()
	cfg1.Agent.ID = "first"
	holder := NewHolder(cfg1)

	notified := false
	var notifiedCfg *Config
	holder.Subscribe(func(cfg *Config) {
		notified = true
		notifiedCfg = cfg
	})

	cfg2 := Defaults()
	cfg2.Agent.ID = "second"
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
	holder := NewHolder(cfg1)

	cfg2 := Defaults()
	cfg2.Agent.ID = "" // invalid
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
			name:   "empty agent id",
			modify: func(c *Config) { c.Agent.ID = "" },
			wantErr: "agent.id",
		},
		{
			name:   "invalid log level",
			modify: func(c *Config) { c.Agent.LogLevel = "verbose" },
			wantErr: "log_level",
		},
		{
			name:   "shutdown timeout zero",
			modify: func(c *Config) { c.Agent.ShutdownTimeout = 0 },
			wantErr: "shutdown_timeout",
		},
		{
			name:   "poll interval below 1s when enabled",
			modify: func(c *Config) {
				c.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 500 * time.Millisecond}
			},
			wantErr: "poll_interval",
		},
		{
			name:   "empty sender target when adapter enabled",
			modify: func(c *Config) {
				c.Sender.Target = ""
				c.Adapters["dcgm"] = AdapterConfig{Enabled: true, PollInterval: 10 * time.Second}
			},
			wantErr: "sender.target",
		},
		{
			name:   "zero ring size",
			modify: func(c *Config) { c.Buffer.RingSize = 0 },
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
		UseStub bool   `yaml:"use_stub"`
		GPUIDs  []int  `yaml:"gpu_ids"`
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

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}
