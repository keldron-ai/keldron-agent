#!/bin/bash
set -euo pipefail

GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

source "$SCRIPT_DIR/scripts/common.sh"

API_PORT="${KELDRON_API_PORT:-8080}"
FRONTEND_PORT=9200

# MODE: both | agent | frontend-only
# USE_CLOUD=1: agent runs without --local (HTTPS cloud streaming); requires API key via env or keldron-agent.dev.yaml
MODE="both"
USE_CLOUD=0
EXPLICIT_LOCAL=0
AGENT_ARGS=()
while (( $# )); do
  case "$1" in
    --app)
      MODE="both"
      shift
      ;;
    --agent-only)
      MODE="agent"
      shift
      ;;
    --frontend-only)
      MODE="frontend-only"
      shift
      ;;
    --local)
      EXPLICIT_LOCAL=1
      shift
      ;;
    --cloud)
      MODE="agent"
      USE_CLOUD=1
      shift
      ;;
    --full)
      MODE="both"
      USE_CLOUD=0
      shift
      ;;
    --full-cloud)
      MODE="both"
      USE_CLOUD=1
      shift
      ;;
    *)
      AGENT_ARGS+=("$1")
      shift
      ;;
  esac
done

if [ "${EXPLICIT_LOCAL}" -eq 1 ] && [ "${USE_CLOUD}" -eq 1 ]; then
  echo "error: --local cannot be combined with --cloud or --full-cloud" >&2
  exit 1
fi

if [ "${USE_CLOUD}" -eq 1 ] && [ "${MODE}" != "frontend-only" ]; then
  if [ -z "${KELDRON_CLOUD_API_KEY:-}" ]; then
    # Env preferred; YAML fallback for local dev (keldron-agent.dev.yaml is gitignored)
    if [ -f keldron-agent.dev.yaml ] && \
       grep -q "api_key:" keldron-agent.dev.yaml 2>/dev/null && \
       ! grep -q 'api_key: ""' keldron-agent.dev.yaml 2>/dev/null; then
      echo "  Using cloud API key from keldron-agent.dev.yaml"
    else
      echo "ERROR: KELDRON_CLOUD_API_KEY not set. Run:" >&2
      echo "  export KELDRON_CLOUD_API_KEY=kldn_live_xxxxx" >&2
      echo "  Or set cloud.api_key in keldron-agent.dev.yaml (keep that file gitignored)." >&2
      exit 1
    fi
  fi
fi

AGENT_PID=""
FRONTEND_PID=""
CLEANUP_DONE=0

prefix_agent() {
  while IFS= read -r line || [ -n "${line:-}" ]; do
    printf '%b[agent]%b   %s\n' "${GREEN}" "${NC}" "${line}"
  done
}

prefix_frontend() {
  while IFS= read -r line || [ -n "${line:-}" ]; do
    printf '%b[frontend]%b %s\n' "${CYAN}" "${NC}" "${line}"
  done
}

terminate_pid() {
  local pid="$1"
  [ -z "${pid}" ] && return 0
  if ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  kill "$pid" 2>/dev/null || true
  local waited=0
  while kill -0 "$pid" 2>/dev/null && [ "$waited" -lt 50 ]; do
    sleep 0.1
    waited=$((waited + 1))
  done
  if kill -0 "$pid" 2>/dev/null; then
    kill -9 "$pid" 2>/dev/null || true
  fi
}

on_exit() {
  local ec=$?
  if [ "${CLEANUP_DONE:-0}" -eq 1 ]; then
    exit "$ec"
  fi
  CLEANUP_DONE=1
  trap - EXIT
  terminate_pid "${FRONTEND_PID}"
  terminate_pid "${AGENT_PID}"
  if [ "$ec" -eq 130 ] || [ "$ec" -eq 143 ]; then
    ec=0
  fi
  exit "$ec"
}

on_exit_frontend() {
  local ec=$?
  if [ "${CLEANUP_DONE:-0}" -eq 1 ]; then
    exit "$ec"
  fi
  CLEANUP_DONE=1
  trap - EXIT
  terminate_pid "${FRONTEND_PID}"
  if [ "$ec" -eq 130 ] || [ "$ec" -eq 143 ]; then
    ec=0
  fi
  exit "$ec"
}

if [ "${MODE}" = "frontend-only" ]; then
  trap 'on_exit_frontend' EXIT
else
  trap 'on_exit' EXIT
fi

if [ "${MODE}" = "both" ]; then
  if ! command -v curl >/dev/null 2>&1; then
    echo "error: curl is required for combined agent + frontend mode" >&2
    exit 1
  fi
fi

if [ "${MODE}" != "frontend-only" ]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "error: go is not installed or not on PATH" >&2
    exit 1
  fi
fi

if [ "${MODE}" != "agent" ]; then
  if ! command -v node >/dev/null 2>&1; then
    echo "error: node is not installed or not on PATH" >&2
    exit 1
  fi
  if ! command -v npm >/dev/null 2>&1; then
    echo "error: npm is not installed or not on PATH" >&2
    exit 1
  fi
fi

if [ "${MODE}" != "frontend-only" ]; then
  cleanup_stale_agent
fi

if [ ! -f keldron-agent.dev.yaml ]; then
  echo "📝 Creating dev config..."
  cat > keldron-agent.dev.yaml << 'EOF'
agent:
  device_name: "ransoms-macbook"
  poll_interval: 30s
  log_level: info
  electricity_rate: 0.12

adapters:
  apple_silicon:
    enabled: true
  dcgm:
    enabled: false
  rocm:
    enabled: false
  temperature:
    enabled: false

output:
  stdout: true
  prometheus: true
  prometheus_port: 9100

hub:
  enabled: false

cloud:
  api_key: ""

health:
  enabled: true
  bind: "127.0.0.1:8081"
EOF
fi

HUB_PORT=$(grep -v '^\s*#' keldron-agent.dev.yaml 2>/dev/null | grep 'listen_port' | head -1 | awk '{print $2}' || true)
# Default must not match FRONTEND_PORT (9200); hub and Vite cannot share a port.
HUB_PORT="${HUB_PORT:-9300}"

PROM_PORT=$(grep -v '^\s*#' keldron-agent.dev.yaml 2>/dev/null | grep 'prometheus_port' | head -1 | awk '{print $2}' || true)
PROM_PORT="${PROM_PORT:-9100}"

print_mode_banner() {
  local dash_label dash_url
  if [ "${MODE}" = "both" ]; then
    dash_label="Dashboard"
    dash_url="http://localhost:${FRONTEND_PORT}"
  else
    dash_label="Agent API"
    dash_url="http://localhost:${API_PORT}"
  fi

  if [ "${USE_CLOUD}" -eq 1 ]; then
    echo ""
    echo "  ╔══════════════════════════════════════╗"
    echo "  ║  KELDRON AGENT — CLOUD MODE          ║"
    echo "  ║  Streaming → api.keldron.ai          ║"
    printf '  ║  %-10s %s%*s║\n' "${dash_label}:" "${dash_url}" $((22 - ${#dash_url})) ""
    printf '  ║  Metrics:   http://localhost:%s/metrics%*s║\n' "${PROM_PORT}" $((2)) ""
    echo "  ╚══════════════════════════════════════╝"
    echo ""
  else
    echo ""
    echo "  ╔══════════════════════════════════════╗"
    echo "  ║  KELDRON AGENT — LOCAL MODE          ║"
    printf '  ║  %-10s %s%*s║\n' "${dash_label}:" "${dash_url}" $((22 - ${#dash_url})) ""
    printf '  ║  Metrics:   http://localhost:%s/metrics%*s║\n' "${PROM_PORT}" $((2)) ""
    echo "  ╚══════════════════════════════════════╝"
    echo ""
  fi
}

if [ "${MODE}" != "frontend-only" ]; then
  print_mode_banner
  echo "📦 Building agent..."
  mkdir -p bin
  go build -o ./bin/keldron-agent ./cmd/agent

  if [ "$(uname -s)" = "Darwin" ] && [ "$(uname -m)" = "arm64" ]; then
    echo "🍎 Apple Silicon: IOKit adapter active, no sudo required"
  fi
fi

if [ "${MODE}" = "both" ] || [ "${MODE}" = "frontend-only" ]; then
  if [ ! -d "${SCRIPT_DIR}/frontend/node_modules" ]; then
    echo "📦 Installing frontend dependencies..."
    (cd "${SCRIPT_DIR}/frontend" && npm install)
  fi
fi

wait_for_api() {
  local code
  for _ in $(seq 1 120); do
    if [ -n "${AGENT_PID}" ] && ! kill -0 "${AGENT_PID}" 2>/dev/null; then
      wait "${AGENT_PID}" 2>/dev/null
      local agent_ec=$?
      echo "error: agent process (PID ${AGENT_PID}) exited with status ${agent_ec} during startup" >&2
      return "${agent_ec}"
    fi
    code=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${API_PORT}/api/v1/status" 2>/dev/null || true)
    if [ "${code}" = "200" ] || [ "${code}" = "503" ]; then
      return 0
    fi
    sleep 0.25
  done
  echo "error: agent API did not become ready on :${API_PORT} (timed out waiting for /api/v1/status)" >&2
  return 1
}

if [ "${MODE}" = "frontend-only" ]; then
  echo ""
  echo "  Frontend-only mode (no agent)"
  echo ""
  printf '%b[frontend]%b %s\n' "${CYAN}" "${NC}" "Starting Vite dev server on :${FRONTEND_PORT}..."
  printf '%b[frontend]%b %s\n' "${CYAN}" "${NC}" "Proxying /api → localhost:${API_PORT}"
  printf '%b[frontend]%b %s\n' "${CYAN}" "${NC}" "Proxying /ws  → localhost:${API_PORT}"
  (cd "${SCRIPT_DIR}/frontend" && exec npm run dev) > >(prefix_frontend) 2> >(prefix_frontend) &
  FRONTEND_PID=$!
  wait "${FRONTEND_PID}"
  exit 0
fi

if [ "${MODE}" = "agent" ] || [ "${MODE}" = "both" ]; then
  export KELDRON_API_PORT="${API_PORT}"
  if [ "${USE_CLOUD}" -eq 1 ]; then
    export KELDRON_HUB_ENABLED=false
  fi
  printf '%b[agent]%b   %s\n' "${GREEN}" "${NC}" "Starting keldron-agent on :${API_PORT}..."
  AGENT_CMD=(./bin/keldron-agent --config keldron-agent.dev.yaml)
  if [ "${USE_CLOUD}" -eq 0 ]; then
    AGENT_CMD+=(--local)
  fi
  if [ "${#AGENT_ARGS[@]}" -gt 0 ]; then
    AGENT_CMD+=("${AGENT_ARGS[@]}")
  fi
  "${AGENT_CMD[@]}" > >(prefix_agent) 2> >(prefix_agent) &
  AGENT_PID=$!
fi

if [ "${MODE}" = "both" ]; then
  if ! wait_for_api; then
    CLEANUP_DONE=1
    trap - EXIT
    terminate_pid "${AGENT_PID}"
    exit 1
  fi
  printf '%b[frontend]%b %s\n' "${CYAN}" "${NC}" "Starting Vite dev server on :${FRONTEND_PORT}..."
  printf '%b[frontend]%b %s\n' "${CYAN}" "${NC}" "Proxying /api → localhost:${API_PORT}"
  printf '%b[frontend]%b %s\n' "${CYAN}" "${NC}" "Proxying /ws  → localhost:${API_PORT}"
  (cd "${SCRIPT_DIR}/frontend" && exec npm run dev) > >(prefix_frontend) 2> >(prefix_frontend) &
  FRONTEND_PID=$!
fi

echo ""
echo "   Prometheus metrics: http://localhost:${PROM_PORT}/metrics"
echo "   Health check:       http://localhost:8081/health"
echo "   Verify: curl localhost:${PROM_PORT}/metrics | grep keldron_gpu_temperature"
echo "   With hub enabled:   Fleet API at http://localhost:${HUB_PORT}/api/v1/fleet"
if [ "${MODE}" = "both" ]; then
  echo "   Dashboard (Vite):   http://localhost:${FRONTEND_PORT}/"
fi
echo "   Press Ctrl+C to stop"
echo ""

if [ "${MODE}" = "agent" ]; then
  wait "${AGENT_PID}"
  exit $?
elif [ "${MODE}" = "both" ]; then
  # Wait for the first child to exit; terminate the other via the EXIT trap.
  if [ "${BASH_VERSINFO[0]}" -gt 4 ] 2>/dev/null ||
     { [ "${BASH_VERSINFO[0]}" -eq 4 ] && [ "${BASH_VERSINFO[1]}" -ge 3 ]; }; then
    # wait -n available (bash 4.3+)
    wait -n "${AGENT_PID}" "${FRONTEND_PID}"
    exit $?
  else
    # Fallback: poll both PIDs
    while true; do
      if ! kill -0 "${AGENT_PID}" 2>/dev/null; then
        wait "${AGENT_PID}" 2>/dev/null
        exit $?
      fi
      if ! kill -0 "${FRONTEND_PID}" 2>/dev/null; then
        wait "${FRONTEND_PID}" 2>/dev/null
        exit $?
      fi
      sleep 0.2
    done
  fi
fi
