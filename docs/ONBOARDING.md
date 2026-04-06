# Keldron Agent — Contributor Onboarding

This document provides a comprehensive onboarding briefing for new contributors. It covers architecture, interfaces, data flow, and conventions.

---

## 1. Directory Structure (Go Files)

```text
./cmd/agent/bridge_test.go
./cmd/agent/main.go
./cmd/agent/register_darwin_arm64.go
./cmd/agent/register_other.go
./cmd/mock-server/main.go
./internal/adapter/adapter.go
./internal/adapter/apple_silicon/adapter.go, adapter_test.go, chip.go, config.go, iokit.go, memory.go, thermal.go
./internal/adapter/kubernetes/adapter.go, adapter_test.go, config.go, discovery.go, types.go, watcher.go
./internal/adapter/rocm/adapter.go, adapter_test.go, collector.go, config.go, parser.go
./internal/adapter/slurm/adapter.go, adapter_test.go, client.go, config.go, types.go
./internal/adapter/snmp_pdu/adapter.go, adapter_test.go, config.go, oid_map.go, poller.go, poller_test.go
./internal/adapter/temperature/adapter.go, adapter_test.go, config.go, poller_modbus.go, poller_snmp.go, stale.go
./internal/buffer/buffer.go, buffer_test.go, convert.go, ring.go, ring_test.go, segment.go, wal.go, wal_test.go
./internal/config/bytesize.go, bytesize_test.go, config.go, config_test.go, watcher.go, watcher_test.go
./internal/dcgm/client_dcgm.go, client_stub_default.go, dcgm.go, dcgm_test.go, metrics.go, stub.go
./internal/fake/fake.go, fake_test.go
./internal/health/health.go, health_test.go, types.go
./internal/normalizer/normalizer.go, normalizer_test.go, point.go, validator.go
./internal/output/output.go, prometheus.go, prometheus_test.go, stdout.go, stdout_test.go
./internal/proto/telemetry/v1/telemetry.pb.go
./internal/proto/telemetry/v1/telemetry_grpc.pb.go
./internal/sender/batch.go, batch_test.go, local.go, mock_server_test.go, sender.go, sender_test.go
./registry/gpu_specs.go, gpu_specs_test.go, normalize.go
./test/integration/integration_test.go
```

---

## 2. Agent Startup (`cmd/agent/main.go`)

### Flow

1. **Flags:** `--config`, `--version`, `--local`, `--help`
2. **Config:** Load YAML from `./keldron-agent.yaml` (or path given by `--config`)
3. **Logger:** JSON structured logger (`slog`)
4. **Config holder:** `config.NewHolder(cfg)` for hot-reload
5. **Adapter registry:**
   - Platform-specific adapters (e.g. `apple_silicon` on darwin/arm64)
   - Global adapters: `dcgm`, `rocm`, `fake`, `kubernetes`, `slurm`, `temperature`, `snmp_pdu`, `nvidia_consumer` (linux/windows)
6. **Config watcher:** `config.NewWatcher()` watches the config file for reloads
7. **Adapters:** `registry.StartAll()` starts enabled adapters
8. **Normalizer:** Consumes all adapter `Readings()` channels and emits `TelemetryPoint`
9. **Output mode:**
   - **Local:** `--local` or no cloud + (Prometheus or stdout)  
     - Output bridge batches by poll interval and updates Prometheus/stdout
   - **Cloud:** Buffer + gRPC sender when cloud API key is set
10. **Health server:** Optional HTTP health endpoint
11. **Shutdown:** Waits for signal, then drain and stop adapters, normalizer, and outputs

### Main Loop (Output Bridge)

- Reads from the normalizer output channel
- Batches points by poll interval (or on first reading)
- Calls `Update(batch)` on all outputs (Prometheus, stdout)
- On shutdown, flushes and drains the channel

---

## 3. Adapter Interface (`internal/adapter/adapter.go`)

```go
type Adapter interface {
    Name() string                              // e.g. "dcgm", "apple_silicon"
    Start(ctx context.Context) error            // Blocks until ctx cancelled
    Stop(ctx context.Context) error             // Graceful shutdown
    Readings() <-chan RawReading                // Channel for normalizer
}
```

### Contract

- `Name()`: Stable identifier for the adapter
- `Start()`: Must block; runs until `ctx` is done
- `Stop()`: Graceful shutdown; may be called after `Start` returns
- `Readings()`: Channel of `RawReading`; adapter sends readings, normalizer consumes; close channel when adapter stops

---

## 4. RawReading (`internal/adapter/adapter.go`)

```go
type RawReading struct {
    AdapterName string                 // Which adapter produced this (required)
    Source      string                 // Hostname or device ID (required)
    Timestamp   time.Time              // When the reading was taken (required, must not be zero)
    Metrics     map[string]interface{}  // Key-value metrics; non-empty required
}
```

### Fields

| Field         | Purpose |
|---------------|---------|
| `AdapterName` | Identifies the adapter (e.g. `"apple_silicon"`); required |
| `Source`      | Host or device identifier (e.g. hostname, `rack-1`, `gpu-0`); used for rack mapping and Prometheus labels |
| `Timestamp`   | Measurement time; validated for skew against agent time |
| `Metrics`     | Map of metric name → value; values must be float64-coercible or string (strings go to Tags) |

### Validation (`normalizer/validator.go`)

- `Source != ""`, `AdapterName != ""`, `Metrics` non-empty, `Timestamp` non-zero
- Max timestamp skew: 30 seconds

---

## 5. Apple Silicon Adapter — Implementation Detail

**Location:** `internal/adapter/apple_silicon/adapter.go`

**Registration:** Only on `darwin` and `arm64` via `cmd/agent/register_darwin_arm64.go`.

### Adapter Methods

- `Name()` returns `"apple_silicon"`
- `Start()`: Ticker loop at `PollInterval`, calls `poll()`; hot-reloads interval from config
- `Stop()`: Calls `CleanupIOKit()`
- `Readings()`: Returns the readings channel

### RawReading Construction (in `collect()`)

```go
return adapter.RawReading{
    AdapterName: "apple_silicon",
    Source:      adapter.Hostname(),
    Timestamp:   now,
    Metrics:     metrics,
}
```

### Metric Keys (Prometheus-Compatible)

| Metric key              | Type   | Example value | Notes |
|-------------------------|--------|---------------|-------|
| `gpu_model`             | string | `"M4 Pro"`    | Tags (device model) |
| `device_model`          | string | `"M4 Pro"`    | Tags (Prometheus label) |
| `behavior_class`        | string | `"soc_integrated"` | Tags |
| `device_vendor`         | string | `"apple"`     | Tags |
| `gpu_id`                | float  | `0.0`         | Used for `device_id` labels |
| `temperature_c`         | float  | SoC temp      | GPU temperature |
| `power_usage_w`         | float  | Watts         | Power draw |
| `gpu_utilization_pct`   | float  | 0–100         | Utilization |
| `thermal_pressure_state`| string | `"nominal"`   | Tags (thermal state) |
| `throttled`             | float  | `0` or `1`    | Throttle indicator |
| `throttle_reason`       | string | `"none"`      | Tags |
| `mem_total_bytes`       | float  | bytes         | System memory |
| `mem_used_bytes`        | float  | bytes         | Used memory |
| `swap_total_bytes`      | float  | bytes         | Swap total |
| `swap_used_bytes`       | float  | bytes         | Swap used |

### Metadata / Tags

Strings in `Metrics` become `Tags` in `TelemetryPoint`: `gpu_model`, `device_model`, `behavior_class`, `device_vendor`, `thermal_pressure_state`, `throttle_reason`.

### Error Handling

- `collect()`: On failure, returns `(RawReading{}, err)`
- `poll()`: On error, increments `errorCount`, stores last error, logs, returns without sending
- Dropped readings when channel is full: Log and drop
- Memory read failure: Logs at debug, fills missing memory metrics with `0.0`

---

## 6. Config Structure (`internal/config/config.go`)

### Adapter Configs (from `AdaptersConfig`)

| Adapter          | Config struct         | Options |
|------------------|----------------------|---------|
| `apple_silicon`  | `AppleSiliconConfig`  | `enabled` (nil = auto on darwin/arm64) |
| `nvidia_consumer`| `NVIDIAConsumerConfig` | `enabled` (nil = auto if nvidia-smi present) |
| `dcgm`           | `DCGMConfig`         | `enabled`, `Raw` (extra YAML) |
| `rocm`           | `ROCmConfig`         | `enabled`, `Raw` |
| `linux_thermal`  | `LinuxThermalConfig` | `enabled`, `hwmon_path` (default `/sys/class/hwmon`) |
| `snmp_pdu`       | `SNMPPDUConfig`      | `enabled`, `Raw` |
| `temperature`    | `TemperatureConfig`  | `enabled`, `Raw` |
| `kubernetes`     | `KubernetesConfig`   | `enabled`, `Raw` |
| `slurm`          | `SlurmConfig`        | `enabled`, `Raw` |

### Adapter Map

`ToAdapterMap()` converts these to `AdapterConfig{Enabled, PollInterval, Endpoint, Raw}` for the registry. Only enabled adapters are started; registry skips adapters not registered (e.g. `linux_thermal`; `nvidia_consumer` is registered on linux/windows).

---

## 7. Prometheus Mapping (`internal/output/prometheus.go`)

**Source:** `TelemetryPoint` (from normalizer), not `RawReading`.

### Metric Keys → Prometheus Gauges

| `TelemetryPoint.Metrics` key | Prometheus metric |
|------------------------------|-------------------|
| `temperature_c`               | `keldron_gpu_temperature_celsius` |
| `temperature_junction_c` / `temperature_edge` / `temperature_c` | `keldron_gpu_hotspot_temperature_celsius` |
| `power_usage_w`              | `keldron_gpu_power_watts` |
| `gpu_utilization_pct`        | `keldron_gpu_utilization_ratio` (÷100) |
| `mem_used_bytes`             | `keldron_gpu_memory_used_bytes` |
| `mem_total_bytes`            | `keldron_gpu_memory_total_bytes` |
| `sm_clock_mhz`               | `keldron_gpu_clock_sm_mhz` |
| `sm_clock_max_mhz`           | `keldron_gpu_clock_max_mhz` |
| `throttled`                  | `keldron_gpu_throttle_active` (0 or 1) |
| `cpu_temp_c`                 | `keldron_cpu_temperature_celsius` |
| `fan_speed_rpm`              | `keldron_fan_speed_rpm` |
| `swap_used_bytes`            | `keldron_system_swap_used_bytes` |
| `swap_total_bytes`           | `keldron_system_swap_total_bytes` |
| `uptime_seconds`             | `keldron_device_uptime_seconds` |

**Labels:** `device_model`, `device_vendor`, `device_id`, `behavior_class`, `adapter` — from `TelemetryPoint.Tags` or `registry.Lookup(deviceModel)`.

---

## 8. Scoring / Risk

Risk scoring is implemented in **`internal/scoring`** (thermal, power, volatility, memory, composite). The output bridge passes computed scores into the Prometheus exporter’s `Update` method, which publishes these gauges with live values:

- `keldron_risk_composite` (labels: `device_id`, `behavior_class`)
- `keldron_risk_thermal`, `keldron_risk_power`, `keldron_risk_volatility`, `keldron_risk_memory` (label: `device_id`)
- `keldron_risk_severity` — `0`–`4` (`0`=normal through `4`=critical; label: `device_id`)
- `keldron_risk_warming_up` — `1` while the device is warming up, else `0`

Stdout JSON in local mode includes the same scores when enabled.

---

## 9. Makefile

| Target       | Command |
|-------------|---------|
| `build`     | `go build -ldflags "-X main.version=$(VERSION)" -o keldron-agent ./cmd/agent` |
| `build-dcgm`| Same with `-tags dcgm` |
| `build-all` | Cross-build: linux/amd64, linux/arm64, darwin/arm64 |
| `test`      | `go test ./... -race -v` |
| `test-dcgm` | Same with `-tags dcgm` |
| `lint`      | `golangci-lint run` |
| `clean`     | Remove built binaries |
| `generate`  | `cd internal/proto && buf generate` |

---

## 10. Dependencies (`go.mod`)

### Direct

- `github.com/fsnotify/fsnotify` — Config file watching
- `github.com/gosnmp/gosnmp` — SNMP (PDU, temperature)
- `github.com/grid-x/modbus` — Modbus (temperature sensors)
- `github.com/oklog/ulid/v2` — ULIDs for telemetry points
- `google.golang.org/grpc` — Cloud streaming
- `google.golang.org/protobuf` — gRPC messages
- `gopkg.in/yaml.v3` — Config parsing
- `k8s.io/api`, `k8s.io/apimachinery`, `k8s.io/client-go` — Kubernetes adapter

### Indirect

Prometheus client, gRPC tooling, Kubernetes support.

---

# Summary

## Data Flow: Hardware Sensor → Prometheus

```text
Hardware (IOKit, DCGM, rocm-smi, SNMP, etc.)
    → Adapter (poll loop, reads channels)
    → RawReading (Source, AdapterName, Timestamp, Metrics)
    → Normalizer (validate, coerce floats, resolve rack, string→Tags, add ULID)
    → TelemetryPoint
    → Output bridge (batch by poll interval)
    → Prometheus.Update(TelemetryPoints)
```

## Adapters and Platforms

| Adapter         | Platform / Use Case |
|-----------------|---------------------|
| `apple_silicon` | darwin/arm64 (M1/M2/M3/M4) |
| `dcgm`          | Linux + NVIDIA DCGM |
| `rocm`          | Linux + AMD ROCm (rocm-smi) |
| `fake`          | Testing/simulation |
| `kubernetes`    | K8s workload metadata |
| `slurm`         | Slurm clusters |
| `temperature`   | SNMP/Modbus temperature sensors |
| `snmp_pdu`      | PDU power via SNMP |
| `nvidia_consumer` | linux/windows (nvidia-smi-based NVIDIA metrics) |
| `linux_thermal` | (config only; not registered) |

## Metric Keys for Prometheus Compatibility

Use these keys in `RawReading.Metrics` for full Prometheus coverage:

| Purpose       | Key(s) |
|---------------|--------|
| GPU temp      | `temperature_c` |
| Hotspot temp  | `temperature_junction_c` or `temperature_edge` |
| Power         | `power_usage_w` |
| Utilization   | `gpu_utilization_pct` (0–100) |
| GPU memory    | `mem_used_bytes`, `mem_total_bytes` |
| Clocks        | `sm_clock_mhz`, `sm_clock_max_mhz` |
| Throttling    | `throttled` (0 or 1) |
| CPU temp      | `cpu_temp_c` |
| Fan           | `fan_speed_rpm` |
| Swap          | `swap_used_bytes`, `swap_total_bytes` |
| Uptime        | `uptime_seconds` |

Strings in `Metrics` (e.g. `gpu_model`, `device_model`, `behavior_class`, `device_vendor`) become `Tags` and drive Prometheus labels.

## Metadata Keys to Set

Set in `Metrics` as strings (they become `Tags`):

- `device_model` — Prometheus label
- `gpu_model` / `gpu_name` — Fallback for model
- `device_vendor` — Vendor label
- `behavior_class` — Behavior class label
- `gpu_id` — Numeric; used for `device_id` (e.g. `Source:gpu_id`)

## Git Workflow

No `CONTRIBUTING.md` or explicit git workflow docs exist. Use standard practices: feature branches (e.g. `feature/xyz`), clear commits, and PRs.

## What Not to Touch (without team alignment)

- **Prometheus metrics (`keldron_*` names and label sets):** Changing exported metric names or labels can break Grafana dashboards, alerting rules, and external consumers—coordinate before changing contracts
- **Normalizer validation:** Changes affect all adapters
- **Adapter contract:** Keep `Adapter` interface and `RawReading` shape stable for compatibility
