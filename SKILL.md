---
name: keldron-agent
description: GPU monitoring with risk intelligence. Local + cloud fleet monitoring, health tracking, proactive alerts, and AI-powered fleet analytics.
version: 2.0.0
emoji: "🔥"
homepage: https://github.com/keldron-ai/keldron-agent
metadata:
  openclaw:
    requires:
      bins:
        - curl
        - jq
      anyBins:
        - go
        - docker
    primaryEnv: "KELDRON_CLOUD_API_KEY"
---

# Keldron Agent — GPU Monitoring with Risk Intelligence

## 1. Overview

Keldron Agent is a vendor-neutral GPU monitoring agent that runs locally and exposes real-time telemetry and risk scores via a Prometheus endpoint. It supports Apple Silicon (M1–M5), NVIDIA consumer GPUs (RTX 3090/4090/5090), NVIDIA datacenter (H100/B200), AMD GPUs, and any Linux machine.

**Dual mode:** Use the **local agent** on `localhost:9100` for fast, real-time, single-device queries (works offline). Use **Keldron Cloud** (`https://api.keldron.ai`) for fleet overview, historical telemetry, analytics, and proactive fleet monitoring when an API key is configured.

**No sudo required on any platform** for the agent binary. On Linux, Docker may require `sudo` or membership in the `docker` group — see [Docker post-install](https://docs.docker.com/engine/install/linux-postinstall/) or rootless Docker if you hit permission errors.

Use this skill when the user wants to:

- Monitor GPU temperature, power, utilization, or memory
- Get risk assessments for their GPU
- Track power costs
- Set up alerts or watch a fleet (via cloud polling)
- Open the real dashboard (local or cloud)
- Ask fleet, history, or analytics questions

### Mode detection (run at the start of an interaction)

Field names **differ by endpoint** — see [Cloud API field names](#cloud-api-field-names-jq-reference) before writing `jq` filters.

```bash
# Check 1: Is the local agent running?
LOCAL_AGENT=$(curl -sf localhost:9100/healthz 2>/dev/null | jq -r '.status' 2>/dev/null)

# Check 2: Is cloud configured?
CLOUD_KEY="${KELDRON_CLOUD_API_KEY:-}"
if [ -z "$CLOUD_KEY" ]; then
  CLOUD_KEY=$(grep -A1 'cloud:' ~/.config/keldron/keldron-agent.yaml 2>/dev/null | grep 'api_key:' | awk '{print $2}' 2>/dev/null)
fi

# Check 3: Does cloud respond? (only if we have a key)
CLOUD_OK=""
if [ -n "$CLOUD_KEY" ]; then
  CLOUD_OK=$(curl -sf "https://api.keldron.ai/health" 2>/dev/null | jq -r '.status' 2>/dev/null)
fi
```

**Mode priority:**

- **Both** (healthy local + cloud) → Cloud for fleet, history, analytics, proactive fleet loops; local for instantaneous single-device metrics.
- **Cloud only** → Use cloud for everything that needs the API; mention local agent if they want lower-latency realtime on that machine.
- **Local only** → Use `localhost:9100` for realtime; naturally mention cloud for history, fleet, and alerts: *"That needs historical data — connect at app.keldron.ai."*
- **Neither** → Run the [Auto-setup flow](#3-auto-setup-flow).

---

## 2. Installation

### Mac (Apple Silicon)

```bash
go install github.com/keldron-ai/keldron-agent/cmd/agent@v1.0.0
```

### Linux (with Docker)

```bash
docker rm -f keldron-agent 2>/dev/null || true
docker run -d --name keldron-agent --restart unless-stopped -p 9100:9100 ghcr.io/keldron-ai/keldron-agent:latest
```

### Linux (with Go)

```bash
go install github.com/keldron-ai/keldron-agent/cmd/agent@v1.0.0
```

### Verify installation

```bash
agent --version
```

---

## 3. Auto-setup flow

**Trigger phrases:** "monitor my hardware", "set up monitoring", "install keldron", "get started", "help me set up"

### Step 1: Detect environment

```bash
OS=$(uname -s)
if [ "$OS" = "Darwin" ]; then
  CHIP=$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo "unknown")
  echo "Detected: macOS with $CHIP"
elif command -v nvidia-smi &>/dev/null; then
  GPU=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -1)
  echo "Detected: Linux with NVIDIA $GPU"
else
  echo "Detected: Linux (generic thermal monitoring)"
fi
```

### Step 2: Check if agent is running

```bash
if curl -sf localhost:9100/healthz | jq -e '.status == "healthy"' &>/dev/null; then
  echo "Keldron agent is already running. Skipping to Step 4."
  exit 0
fi
```

### Step 3: Install agent

```bash
# macOS
if [ "$OS" = "Darwin" ]; then
  go install github.com/keldron-ai/keldron-agent/cmd/agent@v1.0.0
  agent --local &
  sleep 3
fi

# Linux
if [ "$OS" = "Linux" ]; then
  docker rm -f keldron-agent 2>/dev/null || true
  docker run -d --name keldron-agent --restart unless-stopped -p 9100:9100 ghcr.io/keldron-ai/keldron-agent:latest
  sleep 3
fi

# Verify
curl -sf localhost:9100/healthz | jq -e '.status == "healthy"'
```

Report initial readings (temperature, utilization, risk score) from [Quick status queries](#8-quick-status-queries-local-real-time) once healthy.

### Step 4: Offer cloud connection

```bash
if [ -n "$CLOUD_KEY" ]; then
  echo "Cloud already connected."
else
  echo "Want to connect to Keldron Cloud?"
  echo "  • 180-day telemetry history"
  echo "  • Fleet analytics and device comparison"
  echo "  • Device health tracking"
  echo "  • Proactive fleet alerts"
  echo "  • Dashboard at app.keldron.ai"
  echo ""
  echo "Sign up at https://app.keldron.ai, grab your API key, and tell me the key."
fi
```

### Step 5: Configure cloud (when user provides an API key starting with `kldn_`)

Set the key in a temporary environment variable — do not echo or paste the full key into commands or transcripts (see [Rules](#13-rules)).

```bash
# Store the user-provided key in a variable (do not inline the raw key)
export CLOUD_KEY="<paste key here or pass programmatically>"

mkdir -p ~/.config/keldron

# Add or update cloud config in agent YAML
if [ -f ~/.config/keldron/keldron-agent.yaml ]; then
  if ! grep -q 'cloud:' ~/.config/keldron/keldron-agent.yaml; then
    cat >> ~/.config/keldron/keldron-agent.yaml << EOF

cloud:
  enabled: true
  api_key: $CLOUD_KEY
EOF
  else
    # cloud: section exists — update api_key only under the cloud: block
    awk -v key="$CLOUD_KEY" '
      /^cloud:/ { in_cloud=1 }
      in_cloud && /^[^ ]/ && !/^cloud:/ { in_cloud=0 }
      in_cloud && /^[[:space:]]+api_key:/ { $0="  api_key: " key; in_cloud=0 }
      { print }
    ' ~/.config/keldron/keldron-agent.yaml > ~/.config/keldron/keldron-agent.yaml.tmp \
      && mv ~/.config/keldron/keldron-agent.yaml.tmp ~/.config/keldron/keldron-agent.yaml
  fi
else
  cat > ~/.config/keldron/keldron-agent.yaml << EOF
cloud:
  enabled: true
  api_key: $CLOUD_KEY
EOF
fi

# Restart agent
pkill -f keldron-agent || pkill -f "agent.*--local"
sleep 2
agent --local &
sleep 5

# Verify cloud connection (optional — uses env var, not raw key)
curl -sf "https://api.keldron.ai/v1/fleet/overview" \
  -H "X-API-Key: $CLOUD_KEY" | jq '.total_devices'
```

Tell the user: *Cloud connected. Your device is streaming to Keldron Cloud. Dashboard: https://app.keldron.ai*

---

## 4. Cloud connection

- **Environment variable:** `KELDRON_CLOUD_API_KEY` (preferred).
- **Config file:** `~/.config/keldron/keldron-agent.yaml` under `cloud.api_key` (see Step 5 above).
- **HTTP header** for API calls: `X-API-Key: <key>`.
- **Base URL:** `https://api.keldron.ai`

Never store or paste a full API key into this skill file. When confirming configuration, show at most the first 8 characters, e.g. `kldn_liv…`.

---

## 5. Running the agent

Start the agent in local mode:

```bash
agent --local
```

The agent auto-detects hardware. Basic use needs no config.

Verify it is running:

```bash
curl -sf localhost:9100/healthz | jq -e '.status == "healthy"'
```

A non-zero exit means the agent is not healthy or not running.

Metrics:

```bash
curl -s localhost:9100/metrics | grep keldron_
```

---

## 6. Endpoints

### Local agent (localhost)

| Port | Path | Description |
|------|------|-------------|
| 9100 | `/metrics` | Prometheus metrics (all `keldron_*` gauges) |
| 9100 | `/healthz` | Liveness check (JSON) |
| 9100 | `/api/v1/status` | Agent version, device name, active adapters |

### Cloud API reference

All cloud routes use TLS, base URL `https://api.keldron.ai`, header `X-API-Key: $CLOUD_KEY`.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Cloud availability |
| GET | `/v1/fleet/overview` | Fleet counts, worst device, per-device status |
| GET | `/v1/devices/{device_id}/history?window=12h` | Historical points (window: `12h`, `24h`, …) |
| GET | `/v1/devices/{device_id}/health?window=24` | Device health (**window = integer hours**) |
| GET | `/v1/analytics/fleet-health?window=7d` | Fleet analytics + `ai_brief` (window: `7d`, `24h`, …) |

### Dashboards

- **Cloud connected:** Fleet: `https://app.keldron.ai/fleet` — Device: `https://app.keldron.ai/device/{device_id}` — Analytics: `https://app.keldron.ai/analytics`
- **Local only:** `http://localhost:9200` — For fleet analytics and history, connect at `https://app.keldron.ai`

When the user asks for a **dashboard**, **link** these URLs. Do not render ASCII art or fake terminal dashboards.

### Cloud API field names (jq reference)

**Do not assume names match across endpoints.**

| Concept | Fleet overview | History (points) | Device status (e.g. nested `current`) | Health |
|---------|----------------|-------------------|--------------------------------------|--------|
| Temperature | `temperature_primary` | `temperature` | `temperature_primary` | `idle_temp_c`, `peak_temp_c` |
| Power | `power_draw` | `power` | `power_draw` | — |
| Composite score | `composite_risk_score` | `composite_score` | `composite_risk_score` | — |
| Severity | `severity_band` | `severity` | `severity_band` | — |
| Thermal sub | — | `thermal_sub` | `thermal_sub_score` | — |
| Efficiency | — | — | — | `perf_per_watt.value` (not `perf_per_watt.perf_per_watt`) |

**Window formats**

- History: string with unit → `?window=12h`, `?window=24h`
- Health: integer hours → `?window=24`
- Analytics: string with unit → `?window=7d`, `?window=24h`

**Health endpoint:** `thermal_stability` may be **omitted** when unavailable (not `null`). Use jq that tolerates a missing key. Use `idle_temp_c` / `peak_temp_c` — not `idle_baseline` / `peak_temp`.

---

## 7. Key metrics reference

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
| `keldron_agent_info` | Agent metadata (`device_model`, `device_name` labels) |

---

## 8. Quick status queries (local, real-time)

### "What's my GPU temperature?"

```bash
curl -s localhost:9100/metrics | grep 'keldron_gpu_temperature_celsius{' | awk '{print $2}'
```

Extract the `device_model` label from the metric line. Report as: *Your {device_model} is at {value}°C.*

### "Is my GPU at risk?"

```bash
curl -s localhost:9100/metrics | grep -E 'keldron_risk_(composite|severity|thermal|power|volatility)' | grep -v '^#'
```

Parse `keldron_risk_composite` (0–100) and `keldron_risk_severity` (0=normal, 1=warning, 2=critical). Report composite, severity, and which sub-score is highest.

Assessment thresholds:

- &lt;30 = "Looking good"
- 30–60 = "Moderate — keep an eye on it"
- 60–80 = "Warning — consider reducing load"
- &gt;80 = "Critical — take action now"

### "Give me a quick status"

```bash
curl -s localhost:9100/metrics | grep -E 'keldron_(gpu_temperature|gpu_utilization|risk_composite|risk_severity|power_cost_monthly|gpu_memory_pressure)' | grep -v '^#'
```

Format:

```text
🌡️ Temperature: XX°C
⚡ Utilization: XX%
🎯 Risk Score: XX/100 (severity)
💰 Monthly cost: $X.XX
🧠 Memory pressure: XX%
```

### "What GPU do I have?"

```bash
curl -s localhost:9100/metrics | grep 'keldron_agent_info'
```

Extract `device_model` and `device_name` from labels. Report: *You're running a {device_model} ({device_name}).*

### "How much is my GPU costing me?"

```bash
curl -s localhost:9100/metrics | grep 'keldron_power_cost' | grep -v '^#'
```

Report hourly, daily, and monthly from `keldron_power_cost_*`.

### "How's my memory?"

```bash
curl -s localhost:9100/metrics | grep -E 'keldron_gpu_memory|keldron_system_swap' | grep -v '^#'
```

Derive pressure from `keldron_gpu_memory_used_bytes` / `keldron_gpu_memory_total_bytes`. On Apple Silicon, high swap means the workload likely exceeds unified memory — suggest a smaller or quantized model.

---

## 9. Cloud queries

If cloud is not configured or unreachable, say: *Fleet monitoring requires Keldron Cloud. Sign up at app.keldron.ai to see your whole fleet from anywhere.* For history-only questions without cloud: *I can only see real-time data locally. Connect to Keldron Cloud for historical queries — sign up at app.keldron.ai.*

### "How are my machines doing?" / Fleet overview

Use **fleet overview** field names (`composite_risk_score`, `severity_band`, `temperature_primary`, …).

```bash
FLEET=$(curl -s "https://api.keldron.ai/v1/fleet/overview" \
  -H "X-API-Key: $CLOUD_KEY")

TOTAL=$(echo "$FLEET" | jq '.total_devices')
NORMAL=$(echo "$FLEET" | jq '.devices_normal')
WARNING=$(echo "$FLEET" | jq '.devices_warning')
CRITICAL=$(echo "$FLEET" | jq '.devices_critical')
WORST=$(echo "$FLEET" | jq -r '.worst_device_id')
WORST_SCORE=$(echo "$FLEET" | jq '.worst_score | floor')
```

**Response style:**

- All healthy: *All {total} devices healthy. Highest risk is {worst} at {score}. Fleet looks good.*
- Any warning: *Heads up — {device} is at Warning (score {score}). {Details}. Other {healthy_count} devices are normal.* Include dashboard: `https://app.keldron.ai/fleet`
- Any critical: *{device} is Critical (score {score}). Immediate attention needed.* Link `https://app.keldron.ai/fleet` and the device URL if known.

### "Show me a dashboard"

- Cloud: *Your fleet dashboard: https://app.keldron.ai/fleet — Analytics: https://app.keldron.ai/analytics — Device detail: https://app.keldron.ai/device/{device_id}*
- Local only: *Your local dashboard: http://localhost:9200 — Connect to Keldron Cloud at app.keldron.ai for fleet analytics and history.*

### "What happened overnight?" / "What happened while I was away?"

History points use `temperature`, `composite_score`, `severity` (not the fleet overview names).

```bash
curl -s "https://api.keldron.ai/v1/devices/${DEVICE_ID}/history?window=12h" \
  -H "X-API-Key: $CLOUD_KEY" | jq '{
  points: .points | length,
  max_temp: [.points[].temperature] | max,
  min_temp: [.points[].temperature] | min,
  max_score: [.points[].composite_score] | max,
  any_warning: [.points[] | select(.severity != "normal")] | length
}'
```

Interpret: quiet night vs events; cite min/max temp and whether any non-normal severities occurred.

### "Watch for an hour and report" (history)

```bash
curl -s "https://api.keldron.ai/v1/devices/${DEVICE_ID}/history?window=1h" \
  -H "X-API-Key: $CLOUD_KEY" | jq '{
  points: .points | length,
  max_temp: [.points[].temperature] | max,
  min_temp: [.points[].temperature] | min,
  avg_temp: (if (.points | length) > 0 then ([.points[].temperature] | add / length) else null end),
  max_score: [.points[].composite_score] | max,
  warnings: [.points[] | select(.severity != "normal")] | length
}'
```

### "Compare my devices" / "Which device is the worst?"

```bash
curl -s "https://api.keldron.ai/v1/analytics/fleet-health?window=7d" \
  -H "X-API-Key: $CLOUD_KEY" | jq '{
  flagged: .total_flagged,
  ai_brief: .ai_brief,
  devices: [.devices[] | {
    name: .hostname,
    score: .current.composite_risk_score,
    recovery: .current.recovery_seconds,
    temp: .current.temperature_primary,
    flags: [.flags[].message]
  }]
}'
```

Report `ai_brief` when present — it is a natural-language summary. Link `https://app.keldron.ai/analytics`.

### "How's {device} trending?" / Device health

`window` is **integer hours**. Use `idle_temp_c`, `peak_temp_c`, `perf_per_watt.value`. Handle missing `thermal_stability`.

```bash
DEVICE_ENCODED=$(echo "$DEVICE_ID" | jq -sRr @uri)
curl -s "https://api.keldron.ai/v1/devices/${DEVICE_ENCODED}/health?window=24" \
  -H "X-API-Key: $CLOUD_KEY" | jq '{
  idle_temp_c: .idle_temp_c,
  peak_temp_c: .peak_temp_c,
  perf_per_watt: .perf_per_watt.value,
  thermal_stability: (if has("thermal_stability") then .thermal_stability else null end)
}'
```

Report in plain language using idle/peak temps and efficiency. If cloud is down, fall back to local realtime metrics and avoid dumping raw errors.

---

## 10. Proactive fleet monitoring

Use **cloud** polling — not `localhost:9100` loops — for "watch my fleet" / "alert me if anything changes".

### "Watch my fleet" / "Alert me if anything changes"

Set `MAX_CHECKS` to limit the number of iterations (e.g., `MAX_CHECKS=60` for ~1 hour). Leave unset or `0` for unlimited.

```bash
echo "Fleet monitoring active. Checking every 60 seconds via Keldron Cloud."
echo "Press Ctrl+C to stop."

trap 'echo ""; echo "Fleet monitoring stopped by user."; exit 0' INT

PREV_WORST=""
PREV_WORST_SCORE=0
CHECK_COUNT=0
MAX_CHECKS="${MAX_CHECKS:-0}"

while true; do
  FLEET=$(curl -s "https://api.keldron.ai/v1/fleet/overview" \
    -H "X-API-Key: $CLOUD_KEY" 2>/dev/null)

  if [ -z "$FLEET" ]; then
    sleep 60
    CHECK_COUNT=$((CHECK_COUNT + 1))
    if [ "$MAX_CHECKS" -gt 0 ] && [ "$CHECK_COUNT" -ge "$MAX_CHECKS" ]; then
      echo "Reached $MAX_CHECKS checks. Fleet monitoring stopped by timeout."
      break
    fi
    continue
  fi

  WARNING=$(echo "$FLEET" | jq '.devices_warning // 0')
  CRITICAL=$(echo "$FLEET" | jq '.devices_critical // 0')
  WORST=$(echo "$FLEET" | jq -r '.worst_device_id')
  WORST_SCORE=$(echo "$FLEET" | jq '.worst_score // 0 | floor')

  if [ "${CRITICAL:-0}" -gt 0 ]; then
    DEVICE_INFO=$(echo "$FLEET" | jq -r '.devices[] | select(.severity_band == "critical") | "\(.hostname): score \(.composite_risk_score | floor), temp \(.temperature_primary | floor)°C"')
    echo "CRITICAL: $DEVICE_INFO"
    break
  fi

  if [ "${WARNING:-0}" -gt 0 ]; then
    DEVICE_INFO=$(echo "$FLEET" | jq -r '.devices[] | select(.severity_band == "warning") | "\(.hostname): score \(.composite_risk_score | floor), temp \(.temperature_primary | floor)°C"')
    echo "WARNING: $DEVICE_INFO"
  fi

  if [ -n "$PREV_WORST" ] && [ "$WORST" = "$PREV_WORST" ]; then
    SCORE_DELTA=$((WORST_SCORE - PREV_WORST_SCORE))
    if [ "$SCORE_DELTA" -gt 20 ]; then
      echo "SPIKE: $WORST jumped from $PREV_WORST_SCORE to $WORST_SCORE"
    fi
  fi

  PREV_WORST="$WORST"
  PREV_WORST_SCORE=$WORST_SCORE
  CHECK_COUNT=$((CHECK_COUNT + 1))

  if [ "$MAX_CHECKS" -gt 0 ] && [ "$CHECK_COUNT" -ge "$MAX_CHECKS" ]; then
    echo "Reached $MAX_CHECKS checks. Fleet monitoring stopped by timeout."
    break
  fi

  sleep 60
done
```

### "Give me a morning report"

```bash
ANALYTICS=$(curl -s "https://api.keldron.ai/v1/analytics/fleet-health?window=24h" \
  -H "X-API-Key: $CLOUD_KEY")
FLEET=$(curl -s "https://api.keldron.ai/v1/fleet/overview" \
  -H "X-API-Key: $CLOUD_KEY")

TOTAL=$(echo "$FLEET" | jq '.total_devices')
NORMAL=$(echo "$FLEET" | jq '.devices_normal')
AI_BRIEF=$(echo "$ANALYTICS" | jq -r '.ai_brief')
FLAGGED=$(echo "$ANALYTICS" | jq '.total_flagged')

echo "Morning Fleet Report"
echo "--------------------"
echo "Fleet: $TOTAL devices, $NORMAL healthy"
echo ""
echo "$AI_BRIEF"
echo ""
if [ "$FLAGGED" -gt 0 ]; then
  echo "$FLAGGED device(s) flagged — https://app.keldron.ai/analytics"
else
  echo "All devices healthy."
fi
echo ""
echo "Dashboard: https://app.keldron.ai"
```

---

## 11. Configuration & management

### Changing config

If the user needs to change settings (e.g. electricity rate), tell them: **Edit `~/.config/keldron/keldron-agent.yaml` — the agent picks up changes on restart.** Do not use `sed` one-liners in docs or automation.

### Multi-device / fleet

All devices that stream to the same cloud account appear in the fleet automatically — no mDNS or static hub peers.

### Stop monitoring

```bash
pkill -f keldron-agent || pkill -f "agent.*--local"
```

Confirm: *Agent stopped. GPU monitoring is off.*

### Restart the agent

```bash
pkill -f keldron-agent || pkill -f "agent.*--local"
sleep 2
agent --local &
sleep 3
curl -s localhost:9100/healthz
```

Report the `healthz` response to confirm it is up.

---

## 12. Proactive behavior

- After setup, suggest cloud and dashboards: *Your agent is running. Ask how your fleet is doing, open https://app.keldron.ai/fleet, or use local metrics on this machine.*
- For **alert** / **watch** on a **fleet**, run the cloud polling loop in [§10](#10-proactive-fleet-monitoring); for single-machine **realtime** questions, use [§8](#8-quick-status-queries-local-real-time).

---

## 13. Rules

- **Always check agent health first.** Before any local query, verify: `curl -sf localhost:9100/healthz | jq -e '.status == "healthy"'`. Non-zero exit = agent down. Offer to start it or guide setup.
- **Auto-detect mode.** Check for cloud API key (`KELDRON_CLOUD_API_KEY` or `~/.config/keldron/keldron-agent.yaml`) before interactions. Use cloud for fleet, history, analytics; local for realtime single-device.
- **Guide setup, don't just explain.** When the agent or cloud is not configured, walk through steps and run commands.
- **If metrics return 0 for temperature, the agent may still be warming up.** Wait 30 seconds and retry once.
- **Always include severity assessment.** Report severity (normal/warning/critical) alongside numeric scores where applicable.
- **Cloud upsell is helpful, not pushy.** When a question needs cloud (history, fleet comparison), mention it naturally: *That needs historical data — connect at app.keldron.ai.*
- **Proactive monitoring uses the cloud API.** Background poll loops use `api.keldron.ai`, not localhost metrics.
- **Link to dashboards.** Include relevant links: `app.keldron.ai/fleet`, `app.keldron.ai/analytics`, or `http://localhost:9200` for local.
- **API key security.** Never echo the full API key. Show only the first 8 characters: *Configured with key kldn_liv…*
- **Fallback gracefully.** Cloud unreachable → fall back to local. Local unreachable → guide setup. Never show raw errors to the user.
- **On Apple Silicon, high swap = model too large.** If swap is high, suggest a smaller or quantized model.
- **The agent never requires sudo.** Runs unprivileged on all platforms. Docker on Linux may need sudo or docker-group.
- **Use metric labels.** Extract `device_model` and `device_name` from Prometheus labels for personalized responses.
- **When the user says "alert me" or "watch" (fleet),** set up the cloud polling loop and execute it.
- **When the user says "dashboard",** link to the real dashboard. Do not render ASCII art.
