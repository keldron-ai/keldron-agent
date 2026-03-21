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

MODE="both"
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
    *)
      AGENT_ARGS+=("$1")
      shift
      ;;
  esac
done

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

if [ "${MODE}" != "frontend-only" ]; then
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

if [ "${MODE}" != "agent-only" ]; then
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

echo "═══════════════════════════════════════════"
echo "  Keldron Agent — Local Dev Runner"
echo "═══════════════════════════════════════════"

if [ "${MODE}" != "frontend-only" ]; then
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
  bind: ":8081"
EOF
fi

HUB_PORT=$(grep -v '^\s*#' keldron-agent.dev.yaml 2>/dev/null | grep 'listen_port' | head -1 | awk '{print $2}' || true)
# Default must not match FRONTEND_PORT (9200); hub and Vite cannot share a port.
HUB_PORT="${HUB_PORT:-9300}"

wait_for_api() {
  local i
  local code
  for i in $(seq 1 120); do
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
  printf '%b[agent]%b   %s\n' "${GREEN}" "${NC}" "Starting keldron-agent on :${API_PORT}..."
  AGENT_CMD=(./bin/keldron-agent --config keldron-agent.dev.yaml --local)
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
echo "   Prometheus metrics: http://localhost:9100/metrics"
echo "   Health check:       http://localhost:8081/health"
echo "   Verify: curl localhost:9100/metrics | grep keldron_gpu_temperature"
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
