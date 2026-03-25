# keldron-agent

**Vendor-neutral GPU monitoring agent with risk intelligence.**

One binary. Every GPU. Real risk scores — not just dashboards.

- **Apple Silicon** (M1–M5) — zero sudo, IOKit native
- **NVIDIA consumer** (RTX 3090/4090/5090) — nvidia-smi
- **NVIDIA datacenter** (H100/B200) — DCGM
- **AMD** (MI300X, RX 7900 XTX) — ROCm SMI
- **Any Linux machine** — hwmon/thermal sysfs

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg?color=00C9B0)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/keldron-ai/keldron-agent?style=flat-square)](https://goreportcard.com/report/github.com/keldron-ai/keldron-agent)
[![Release](https://img.shields.io/github/v/release/keldron-ai/keldron-agent?color=00C9B0&style=flat-square)](https://github.com/keldron-ai/keldron-agent/releases)

## Quick Start (30 seconds)

### Mac (Apple Silicon)

```bash
# Coming soon: brew install keldron-ai/tap/keldron-agent
# For now, build from source:
go install github.com/keldron-ai/keldron-agent/cmd/agent@latest
agent --local
# → Prometheus metrics at http://localhost:9100/metrics
```

### Linux

```bash
# Coming soon: curl -sfL https://get.keldron.ai | sh
# For now, build from source:
go install github.com/keldron-ai/keldron-agent/cmd/agent@latest
agent --local

# or build and run with Docker
make docker-build
make docker-run

# or run a pre-built image (when published)
docker run -p 9100:9100 -p 8081:8081 \
  -e KELDRON_OUTPUT_PROMETHEUS_HOST=0.0.0.0 \
  -e KELDRON_HEALTH_BIND=0.0.0.0:8081 \
  ghcr.io/keldron-ai/keldron-agent:latest

# with a config file
docker run -p 9100:9100 -p 8081:8081 \
  -e KELDRON_OUTPUT_PROMETHEUS_HOST=0.0.0.0 \
  -e KELDRON_HEALTH_BIND=0.0.0.0:8081 \
  -v $(pwd)/configs/keldron-agent.example.yaml:/etc/keldron/keldron-agent.yaml:ro \
  ghcr.io/keldron-ai/keldron-agent:latest
```

### Verify

```bash
curl localhost:9100/metrics | grep keldron_gpu_temperature
# keldron_gpu_temperature_celsius{device_model="M4-Pro",device_vendor="apple",...} 52.3
```

### Connect to Keldron Cloud

Stream telemetry to the cloud for 180-day history, fleet analytics, and device health tracking.

The examples below use `keldron-agent` (the name from `make` / `go build -o keldron-agent ./cmd/agent`). If you installed with `go install ... cmd/agent@latest`, the binary is named `agent` — use `agent login`, `agent whoami`, etc.

**Option 1: Interactive login**

```bash
keldron-agent login
```

**Option 2: Paste your API key from app.keldron.ai**

```bash
keldron-agent login --api-key kldn_live_your_key_here
```

**Option 3: Environment variable**

```bash
export KELDRON_CLOUD_API_KEY=kldn_live_your_key_here
keldron-agent
```

Check your connection:

```bash
keldron-agent whoami
```

Sign up for free at [app.keldron.ai](https://app.keldron.ai).

### CLI reference

| Command | Purpose |
|--------|---------|
| `login` | Authenticate with Keldron Cloud |
| `logout` | Remove stored credentials |
| `whoami` | Show current Cloud connection (masked API key and endpoint) |
| `scan` | One-shot device/fleet status query |

Run `keldron-agent --help` and `keldron-agent <command> -h` for flags.

## What You Get

Example Prometheus output (real data from Apple Silicon):

```text
keldron_gpu_temperature_celsius{adapter="apple_silicon",behavior_class="soc_integrated",device_id="hostname:0",device_model="M4-Pro",device_vendor="apple"} 52.3
keldron_risk_composite{behavior_class="soc_integrated",device_id="hostname:0"} 12.4
keldron_risk_severity{device_id="hostname:0"} 0
keldron_power_cost_monthly{device_id="hostname:0"} 4.32
```

## Why Not Just nvidia-smi / powermetrics / lm-sensors?

| Feature | nvidia-smi | keldron-agent |
|---------|------------|---------------|
| Raw temperature | ✅ | ✅ |
| Risk score (0–100) | ❌ | ✅ |
| "Time to thermal throttle" | ❌ | ✅ |
| Vendor-neutral | ❌ | ✅ |
| Power cost estimation | ❌ | ✅ |
| Prometheus endpoint | ❌ | ✅ |

## Configuration

Create `keldron-agent.yaml`:

```yaml
agent:
  device_name: "my-workstation"
  poll_interval: "2s"           # 2s–5m; use 10s–30s in production to reduce CPU/network load
  log_level: "info"
  electricity_rate: 0.12

adapters:
  apple_silicon:   # Mac: set enabled: true
    enabled: true
  nvidia_consumer: # Linux + NVIDIA: set enabled: true when nvidia-smi in PATH
    enabled: false
  dcgm:            # Datacenter NVIDIA (H100/B200)
    enabled: false
  rocm:            # AMD (MI300X, RX 7900)
    enabled: false

output:
  prometheus: true
  prometheus_port: 9100
```

Full config reference: [configs/keldron-agent.example.yaml](configs/keldron-agent.example.yaml)

## Metrics Reference

| Metric | Type | Description |
|--------|------|-------------|
| `keldron_gpu_temperature_celsius` | gauge | GPU temperature in Celsius |
| `keldron_gpu_hotspot_temperature_celsius` | gauge | GPU hotspot/junction temperature in Celsius |
| `keldron_gpu_power_watts` | gauge | GPU power draw in watts |
| `keldron_gpu_utilization_ratio` | gauge | GPU utilization 0–1 |
| `keldron_gpu_memory_used_bytes` | gauge | GPU memory used in bytes |
| `keldron_gpu_memory_total_bytes` | gauge | GPU memory total in bytes |
| `keldron_gpu_clock_sm_mhz` | gauge | GPU SM clock in MHz |
| `keldron_gpu_clock_max_mhz` | gauge | GPU max clock in MHz |
| `keldron_gpu_throttle_active` | gauge | 1 if GPU is throttled, 0 otherwise |
| `keldron_cpu_temperature_celsius` | gauge | CPU temperature in Celsius |
| `keldron_fan_speed_rpm` | gauge | Fan speed in RPM |
| `keldron_system_swap_used_bytes` | gauge | System swap used in bytes |
| `keldron_system_swap_total_bytes` | gauge | System swap total in bytes |
| `keldron_device_uptime_seconds` | gauge | Device uptime in seconds |
| `keldron_risk_composite` | gauge | Composite risk score |
| `keldron_risk_thermal` | gauge | Thermal risk score |
| `keldron_risk_power` | gauge | Power risk score |
| `keldron_risk_volatility` | gauge | Volatility risk score |
| `keldron_risk_fleet_penalty` | gauge | Fleet penalty risk score |
| `keldron_risk_severity` | gauge | 0=normal, 1=warning, 2=critical |
| `keldron_risk_warming_up` | gauge | 1 if device warming up, 0 otherwise |
| `keldron_gpu_memory_pressure_ratio` | gauge | GPU memory used/total ratio |
| `keldron_gpu_clock_efficiency` | gauge | GPU clock efficiency ratio |
| `keldron_power_cost_hourly` | gauge | Estimated power cost per hour |
| `keldron_power_cost_daily` | gauge | Estimated power cost per day |
| `keldron_power_cost_monthly` | gauge | Estimated power cost per month |
| `keldron_gpu_hotspot_delta_celsius` | gauge | Hotspot minus edge temp (NVIDIA only); -1 if unavailable |
| `keldron_agent_info` | gauge | Agent info (always 1) |

## Architecture

```text
Adapters → Normalizer → Risk Engine → Prometheus /metrics
(IOKit, NVML,                          Stdout JSON
 ROCm, hwmon)                          Keldron Cloud (optional)
```

## Security

The agent is **read-only** — it reads hardware sensors and computes scores. It does not execute arbitrary commands or alter system state beyond writing its own credential file (`~/.keldron/credentials`, created with 0600 permissions). Local HTTP servers (web UI on port 9200, Prometheus metrics on port 9100, health endpoint on port 8081) bind to `127.0.0.1` by default and are not exposed on public interfaces unless explicitly reconfigured.

- All HTTP servers bind to `127.0.0.1` (localhost) by default. Override via config for LAN access.
- Cloud telemetry is transmitted over HTTPS with TLS 1.2+.
- Credentials are stored with restricted file permissions (0600).
- The agent contains no tracking, analytics, or telemetry about your usage — only hardware sensor data.

To report a security issue, email [ransom@keldron.ai](mailto:ransom@keldron.ai).

## Grafana Dashboard

Keldron exposes Prometheus metrics at `/metrics` — import them into any Grafana instance. Example dashboard JSON coming in v0.2.0.

## Upgrade Path

Want fleet dashboards, 180-day history, and device health analytics?

→ Sign up free at [app.keldron.ai](https://app.keldron.ai)
→ Learn more at [keldron.ai](https://keldron.ai)

## Contributing

PRs welcome. See our [contributing guide](CONTRIBUTING.md).

## License

Apache 2.0 — see [LICENSE](LICENSE).
