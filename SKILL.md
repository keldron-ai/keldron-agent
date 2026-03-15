---
name: keldron-agent
description: Vendor-neutral GPU monitoring agent with risk intelligence. Install, run, and interact with GPU telemetry and risk scores conversationally.
version: 1.0.0
emoji: "🔥"
homepage: https://github.com/keldron-ai/keldron-agent
metadata:
  openclaw:
    requires:
      bins:
        - curl
      anyBins:
        - go
        - docker
    primaryEnv: ""
---

# Keldron Agent — GPU Monitoring with Risk Intelligence

## Overview

Keldron Agent is a vendor-neutral GPU monitoring agent that runs locally and exposes real-time telemetry and risk scores via a Prometheus endpoint. It supports Apple Silicon (M1–M5), NVIDIA consumer GPUs (RTX 3090/4090/5090), NVIDIA datacenter (H100/B200), AMD GPUs, and any Linux machine.

**No sudo required on any platform.** The agent runs entirely unprivileged.

Use this skill when the user wants to:
- Monitor GPU temperature, power, utilization, or memory
- Get risk assessments for their GPU
- Track power costs
- Set up alerts for thermal issues
- View a dashboard of GPU metrics
- Monitor a fleet of machines

## Installation

### Mac (Apple Silicon)

```bash
go install github.com/keldron-ai/keldron-agent/cmd/agent@latest
```

### Linux (with Docker)

```bash
docker run -d --name keldron-agent -p 9100:9100 -p 8081:8081 ghcr.io/keldron-ai/keldron-agent:latest
```

### Linux (with Go)

```bash
go install github.com/keldron-ai/keldron-agent/cmd/agent@latest
```

### Verify Installation

```bash
agent --version
```

## Running the Agent

Start the agent in local mode (no cloud connection):

```bash
agent --local
```

The agent auto-detects your hardware. No configuration needed for basic use.

Verify it's running:

```bash
curl -s localhost:9100/healthz
```

Metrics are available at:

```bash
curl -s localhost:9100/metrics | grep keldron_
```

## Endpoints

| Port | Path | Description |
|------|------|-------------|
| 9100 | `/metrics` | Prometheus metrics (all `keldron_*` gauges) |
| 9100 | `/healthz` | Quick liveness check (JSON) |
| 9100 | `/api/v1/status` | Agent version, device name, active adapters |
| 8081 | `/health` | Full health (adapters, normalizer, buffer) — when health server enabled |
| 8081 | `/ready` | Readiness probe |

## Monitoring and Conversational Use

### Checking GPU Status

Fetch current temperature, risk, and power cost:

```bash
curl -s localhost:9100/metrics | grep -E "keldron_gpu_temperature|keldron_risk_composite|keldron_risk_severity|keldron_power_cost_monthly"
```

### "Text me if my GPU overheats"

Interpret this as: the user wants alerts when the GPU gets too hot.

1. **Ensure the agent is running** — `curl -s localhost:9100/healthz` should return `{"status":"healthy"}`.
2. **Define thresholds** — Use `keldron_gpu_temperature_celsius` (e.g., > 85°C) or `keldron_risk_severity` (2 = critical).
3. **Explain options** — The agent has no built-in SMS. The user needs:
   - A script that polls metrics and calls a webhook/SMS API (Twilio, etc.)
   - Prometheus Alertmanager + notification config
   - A cron job that runs `curl` + threshold check and triggers a notification

Offer to write a small script that:
- Polls `localhost:9100/metrics`
- Parses `keldron_gpu_temperature_celsius` or `keldron_risk_severity`
- Calls a webhook or SMS API when threshold exceeded

### "Show me a dashboard"

Interpret this as: the user wants a visual or summarized view of GPU metrics.

**Option A — Text summary:** Fetch metrics and render a concise table:

```bash
curl -s localhost:9100/metrics | grep -E "keldron_gpu_temperature|keldron_risk_composite|keldron_power_cost_monthly" | grep -v "^#"
```

**Option B — Grafana:** Add Prometheus datasource `http://localhost:9100` and create panels. Dashboard JSON is coming soon in the repo.

**Option C — Raw:** Link to `http://localhost:9100/metrics` for full Prometheus output.

## Key Metrics Reference

| Metric | Description |
|--------|-------------|
| `keldron_gpu_temperature_celsius` | GPU temperature in Celsius |
| `keldron_risk_severity` | 0=normal, 1=warning, 2=critical |
| `keldron_risk_composite` | Composite risk score (0–100) |
| `keldron_risk_thermal` | Thermal risk score |
| `keldron_power_cost_monthly` | Estimated power cost per month ($) |
| `keldron_gpu_power_watts` | GPU power draw in watts |
| `keldron_gpu_utilization_ratio` | GPU utilization 0–1 |
| `keldron_gpu_throttle_active` | 1 if throttled, 0 otherwise |
