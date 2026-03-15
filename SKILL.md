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
        - bc
        - jq
      anyBins:
        - go
        - docker
    primaryEnv: ""
---

# Keldron Agent — GPU Monitoring with Risk Intelligence

## Overview

Keldron Agent is a vendor-neutral GPU monitoring agent that runs locally and exposes real-time telemetry and risk scores via a Prometheus endpoint. It supports Apple Silicon (M1–M5), NVIDIA consumer GPUs (RTX 3090/4090/5090), NVIDIA datacenter (H100/B200), AMD GPUs, and any Linux machine.

**No sudo required on any platform.** The agent binary runs entirely unprivileged. On Linux, Docker itself may require `sudo` or membership in the `docker` group — see [Docker post-install](https://docs.docker.com/engine/install/linux-postinstall/) or use rootless Docker if you hit permission errors.

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
go install github.com/keldron-ai/keldron-agent/cmd/agent@v1.0.0
```

### Linux (with Docker)

```bash
docker run -d --name keldron-agent -p 9100:9100 -p 8081:8081 ghcr.io/keldron-ai/keldron-agent:latest
```

### Linux (with Go)

```bash
go install github.com/keldron-ai/keldron-agent/cmd/agent@v1.0.0
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
curl -sf localhost:9100/healthz | jq -e '.status == "healthy"'
```

A non-zero exit code means the agent is not healthy or not running.

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

## Key Metrics Reference

| Metric | Description |
|--------|-------------|
| `keldron_gpu_temperature_celsius` | GPU temperature in Celsius |
| `keldron_risk_severity` | 0=normal, 1=warning, 2=critical |
| `keldron_risk_composite` | Composite risk score (0–100) |
| `keldron_risk_thermal` | Thermal risk score |
| `keldron_risk_power` | Power risk score |
| `keldron_risk_volatility` | Volatility risk score |
| `keldron_power_cost_monthly` | Estimated power cost per month ($) |
| `keldron_power_cost_daily` | Estimated power cost per day ($) |
| `keldron_power_cost_hourly` | Estimated power cost per hour ($) |
| `keldron_gpu_power_watts` | GPU power draw in watts |
| `keldron_gpu_utilization_ratio` | GPU utilization 0–1 |
| `keldron_gpu_memory_used_bytes` | GPU memory in use |
| `keldron_gpu_memory_total_bytes` | GPU memory total |
| `keldron_gpu_memory_pressure_ratio` | Memory pressure 0–1 |
| `keldron_gpu_throttle_active` | 1 if throttled, 0 otherwise |
| `keldron_system_swap_used_bytes` | System swap in use |
| `keldron_agent_info` | Agent metadata (device_model, device_name labels) |

## Conversational Interaction Patterns

### 1. Quick Status Queries

#### "What's my GPU temperature?"

Run:
```bash
curl -s localhost:9100/metrics | grep 'keldron_gpu_temperature_celsius{' | awk '{print $2}'
```
Extract the `device_model` label from the metric line.
Report as: "Your {device_model} is at {value}°C."

#### "Is my GPU at risk?"

Run:
```bash
curl -s localhost:9100/metrics | grep -E 'keldron_risk_(composite|severity|thermal|power|volatility)' | grep -v '^#'
```
Parse `keldron_risk_composite` (0–100) and `keldron_risk_severity` (0=normal, 1=warning, 2=critical).
Report the composite score, severity, and which sub-score (thermal/power/volatility) is highest.

Assessment thresholds:
- <30 = "Looking good"
- 30–60 = "Moderate — keep an eye on it"
- 60–80 = "Warning — consider reducing load"
- >80 = "Critical — take action now"

#### "Give me a quick status"

Run:
```bash
curl -s localhost:9100/metrics | grep -E 'keldron_(gpu_temperature|gpu_utilization|risk_composite|risk_severity|power_cost_monthly|gpu_memory_pressure)' | grep -v '^#'
```
Format as:
```text
🌡️ Temperature: XX°C
⚡ Utilization: XX%
🎯 Risk Score: XX/100 (severity)
💰 Monthly cost: $X.XX
🧠 Memory pressure: XX%
```

#### "What GPU do I have?"

Run:
```bash
curl -s localhost:9100/metrics | grep 'keldron_agent_info'
```
Extract `device_model` and `device_name` from the labels.
Report: "You're running a {device_model} ({device_name})."

#### "How much is my GPU costing me?"

Run:
```bash
curl -s localhost:9100/metrics | grep 'keldron_power_cost' | grep -v '^#'
```
Report hourly, daily, and monthly cost from the three `keldron_power_cost_*` metrics.

#### "How's my memory?"

Run:
```bash
curl -s localhost:9100/metrics | grep -E 'keldron_gpu_memory|keldron_system_swap' | grep -v '^#'
```
Calculate memory pressure from `keldron_gpu_memory_used_bytes` / `keldron_gpu_memory_total_bytes`.
On Apple Silicon, high swap usage means the ML model exceeds unified memory — suggest a smaller model or quantized version.

### 2. Alert & Watch Mode

#### "Text me if my GPU overheats"

(Or any variation: "alert me if it gets hot", "watch my GPU while I'm out", "let me know if anything goes wrong")

First, verify the agent is running:
```bash
curl -sf localhost:9100/healthz | jq -e '.status == "healthy"'
```

Then set up a background monitoring loop:
```bash
while true; do
  METRICS=$(curl -s localhost:9100/metrics)
  SEVERITY=$(echo "$METRICS" | grep 'keldron_risk_severity{' | awk '{print $2}')
  TEMP=$(echo "$METRICS" | grep 'keldron_gpu_temperature_celsius{' | awk '{print $2}')
  COMPOSITE=$(echo "$METRICS" | grep 'keldron_risk_composite{' | awk '{print $2}')
  if [ "$(echo "$SEVERITY >= 1" | bc -l)" -eq 1 ] 2>/dev/null; then
    echo "⚠️ GPU ALERT: severity=$SEVERITY, temp=${TEMP}°C, risk=$COMPOSITE"
    break
  fi
  sleep 60
done
```

Tell the user: "Got it — I'll watch your GPU. Checking every 60 seconds. I'll alert you if risk severity goes above normal."

When triggered, report what happened and suggest: "Consider reducing GPU load or checking cooling."

#### "Alert me overnight while my training runs"

(Or any variation: "watch it overnight", "keep an eye on things while I sleep")

Set up a more comprehensive monitoring loop with multiple alert conditions, checking every 2 minutes:
```bash
while true; do
  METRICS=$(curl -s localhost:9100/metrics)
  SEVERITY=$(echo "$METRICS" | grep 'keldron_risk_severity{' | awk '{print $2}')
  TEMP=$(echo "$METRICS" | grep 'keldron_gpu_temperature_celsius{' | awk '{print $2}')
  MEM=$(echo "$METRICS" | grep 'keldron_gpu_memory_pressure_ratio{' | awk '{print $2}')
  THROTTLE=$(echo "$METRICS" | grep 'keldron_gpu_throttle_active{' | awk '{print $2}')

  ALERT=""
  [ "$(echo "${SEVERITY:-0} >= 1" | bc -l 2>/dev/null)" = "1" ] && ALERT="Risk severity elevated"
  [ "$(echo "${TEMP:-0} > 90" | bc -l 2>/dev/null)" = "1" ] && ALERT="Temperature above 90°C"
  [ "$(echo "${MEM:-0} > 0.95" | bc -l 2>/dev/null)" = "1" ] && ALERT="Memory pressure critical"
  [ "$(echo "${THROTTLE:-0} > 0" | bc -l 2>/dev/null)" = "1" ] && ALERT="GPU throttling detected"

  if [ -n "$ALERT" ]; then
    echo "🚨 ALERT: $ALERT | temp=${TEMP}°C severity=$SEVERITY"
    break
  fi
  sleep 120
done
```

Tell the user: "I'll keep watch overnight. Checking every 2 minutes for thermal risk, memory pressure, and throttling. Sleep well — I'll only wake you if something needs attention."

#### "Watch my GPU for an hour and give me a report"

Collect metrics every 60 seconds for 1 hour, then summarize:
```bash
LOGFILE="/tmp/keldron-watch-$(date +%s).csv"
echo "timestamp,temp_c,utilization,power_w,risk_composite,severity" > $LOGFILE
for i in $(seq 1 60); do
  METRICS=$(curl -s localhost:9100/metrics)
  TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  TEMP=$(echo "$METRICS" | grep 'keldron_gpu_temperature_celsius{' | awk '{print $2}')
  UTIL=$(echo "$METRICS" | grep 'keldron_gpu_utilization_ratio{' | awk '{print $2}')
  POWER=$(echo "$METRICS" | grep 'keldron_gpu_power_watts{' | awk '{print $2}')
  COMP=$(echo "$METRICS" | grep 'keldron_risk_composite{' | awk '{print $2}')
  SEV=$(echo "$METRICS" | grep 'keldron_risk_severity{' | awk '{print $2}')
  echo "$TS,$TEMP,$UTIL,$POWER,$COMP,$SEV" >> $LOGFILE
  sleep 60
done
```

After collecting, read the CSV and summarize:
- Temperature range (min/max/avg)
- Peak utilization
- Power cost for the hour
- Any risk events (severity >= 1)
- Overall trend (stable, rising, cooling)

### 3. Dashboard

#### "Show me a dashboard"

(Or "give me a dashboard view", "dashboard", etc.)

Fetch all metrics and present a formatted view:
```bash
curl -s localhost:9100/metrics | grep -E '^keldron_' | grep -v '^#'
```

Parse and format as:
```text
╔══════════════════════════════════════════╗
║  🖥️  {device_model} Dashboard            ║
╠══════════════════════════════════════════╣
║  🌡️ Temperature    {temp}°C              ║
║  ⚡ Utilization    {util}%               ║
║  🔌 Power          {power}W              ║
║  🧠 Memory         {mem_used}GB / {mem_total}GB  ║
╠══════════════════════════════════════════╣
║  🎯 Risk Score     {composite}/100       ║
║     Thermal        {thermal}             ║
║     Power          {power_score}         ║
║     Volatility     {volatility}          ║
║     Severity       {severity_badge}      ║
╠══════════════════════════════════════════╣
║  💰 Monthly cost   ${monthly}            ║
╚══════════════════════════════════════════╝
```

Use color context: temperature green <60°C, yellow 60–80°C, red >80°C.
Severity badges: 0 → "✅ Normal", 1 → "⚠️ Warning", 2 → "🔴 Critical".

#### "Show me a live dashboard" / "Keep refreshing"

Run the dashboard fetch in a loop every 10 seconds, clearing between refreshes:
```bash
while true; do
  clear
  METRICS=$(curl -s localhost:9100/metrics)
  # Parse and render the dashboard format above
  echo "Last updated: $(date)"
  echo "Press Ctrl+C to stop"
  sleep 10
done
```

Tell the user: "Live dashboard running — refreshing every 10 seconds. Press Ctrl+C to stop."

### 4. Fleet Monitoring

#### "How are all my machines doing?"

First check hub availability:
```bash
curl -s localhost:9200/api/v1/fleet
```

If the hub is not running, tell the user: "Fleet hub isn't available on port 9200. Start the agent with `--hub.enabled=true` to monitor multiple machines."

If available, parse the JSON response. For each peer: report device model, temperature, risk score, severity. Sort by risk score descending. Highlight any in warning or critical.

#### "Which machine is running hottest?"

Run:
```bash
curl -s localhost:9200/api/v1/fleet
```

Find the device with the highest `keldron_gpu_temperature_celsius` across all peers.
Report: "{device_model} on {hostname} is the hottest at {temp}°C."

#### "Are any of my machines at risk?"

Run:
```bash
curl -s localhost:9200/api/v1/fleet
```

Filter to severity >= 1. If none: "All clear — everything's running normal across your fleet."
If any: list them with device model, temperature, risk score, and severity.

#### "Show me the fleet dashboard"

Run:
```bash
curl -s localhost:9200/api/v1/fleet
```

Format as:
```text
╔══════════════════════════════════════════════════╗
║  🌐 Fleet Dashboard — {count} machines           ║
╠══════════════════════════════════════════════════╣
║  {hostname1}  {model}  {temp}°C  {severity_badge} ║
║  {hostname2}  {model}  {temp}°C  {severity_badge} ║
║  ...                                              ║
╠══════════════════════════════════════════════════╣
║  Total: {n} GPUs | {ok} ✅ OK | {warn} ⚠️ | {crit} 🔴 ║
╚══════════════════════════════════════════════════╝
```

### 5. Configuration & Management

#### "Change my electricity rate to $0.15"

Find and update the config (pick the command for your OS):

**macOS (BSD sed):**
```bash
sed -i '' 's/electricity_rate:.*/electricity_rate: 0.15/' ~/.config/keldron/keldron-agent.yaml
```

**Linux (GNU sed):**
```bash
sed -i 's/electricity_rate:.*/electricity_rate: 0.15/' ~/.config/keldron/keldron-agent.yaml
```

**OS-agnostic (yq):**
```bash
yq -i '.electricity_rate = 0.15' ~/.config/keldron/keldron-agent.yaml
```

If config not found, check `./keldron-agent.dev.yaml` or create one.
Tell the user: "Updated. Cost estimates will refresh in ~30 seconds."

#### "Add a machine to my fleet"

Check if mDNS is enabled by looking at the config. If yes: "Just start keldron-agent on the new machine — mDNS will auto-discover it in about 30 seconds."
If using static peers: "Add the IP:port to `static_peers` in your hub config and restart the agent."

#### "Stop monitoring"

Run:
```bash
pkill -f keldron-agent || pkill -f "agent.*--local"
```
Confirm: "Agent stopped. GPU monitoring is off."

#### "Restart the agent"

Run:
```bash
pkill -f keldron-agent || pkill -f "agent.*--local"
sleep 2
agent --local &
sleep 3
curl -s localhost:9100/healthz
```
Report the healthz response to confirm it's back up.

## Rules

- **Always check agent health first.** Before any query, verify the agent is running: `curl -sf localhost:9100/healthz | jq -e '.status == "healthy"'`. A non-zero exit code means the agent is down — offer to start it with `agent --local`.
- **If metrics return 0 for temperature, the agent may still be warming up.** Wait 30 seconds and retry once before reporting zero values.
- **Always include severity assessment.** When reporting risk, always include the severity level (normal/warning/critical) alongside the numeric score.
- **Alert loops run in the background.** Tell the user what you're watching, how often, and what thresholds will trigger an alert.
- **When the user says "text me" or "alert me", set up a polling loop.** Do not just explain how alerts could work — actually write and execute the monitoring script.
- **When the user says "dashboard", render one.** Do not just link to Grafana or explain options — fetch the metrics and format the output.
- **For fleet queries, check hub availability first.** If the hub is not running on port 9200, explain how to enable it with `--hub.enabled=true`.
- **On Apple Silicon, high swap = model too large.** If `keldron_system_swap_used_bytes` is high, suggest a smaller or quantized model.
- **Never require sudo.** The agent runs unprivileged on all platforms.
- **Use the metric labels.** Device model and name are in the metric labels — extract and use them in responses for a personalized experience.
