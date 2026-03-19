#!/bin/bash
# full-diagnostic.sh — Comprehensive keldron-agent test suite
# Run from the repository root.
# Tests: build, local monitoring, risk scoring, hub mode, fleet API, all endpoints

set -uo pipefail

PASS=0
FAIL=0
WARN=0
RESULTS=()

GREEN='\033[0;32m'
RED='\033[0;31m'
AMBER='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

check() {
    local name="$1"
    local result="$2"
    if [ "$result" -eq 0 ]; then
        RESULTS+=("${GREEN}✓ PASS${NC}: $name")
        ((PASS++))
    else
        RESULTS+=("${RED}✗ FAIL${NC}: $name")
        ((FAIL++))
    fi
}

warn() {
    local name="$1"
    RESULTS+=("${AMBER}⚠ WARN${NC}: $name")
    ((WARN++))
}

cleanup() {
    pkill -f keldron-agent 2>/dev/null || true
    pkill -f "agent.*--local" 2>/dev/null || true
    rm -f /tmp/keldron-test-local.yaml /tmp/keldron-test-hub.yaml
    rm -f /tmp/keldron-diag-local.log /tmp/keldron-diag-hub.log
}
trap cleanup EXIT INT TERM

echo ""
echo "${CYAN}═══════════════════════════════════════════════════${NC}"
echo "${CYAN}  Keldron Agent — Full Diagnostic Suite${NC}"
echo "${CYAN}═══════════════════════════════════════════════════${NC}"
echo ""

echo "🧹 Cleaning up stale processes..."
cleanup
sleep 2

# ═══════════════════════════════════════════
# SECTION 1: BUILD
# ═══════════════════════════════════════════
echo ""
echo "${CYAN}── Section 1: Build ──${NC}"

echo "  → go build"
go build -o keldron-agent ./cmd/agent 2>&1
check "Binary compiles" $?

echo "  → Binary size"
SIZE=$(wc -c < keldron-agent 2>/dev/null || echo "0")
SIZE_MB=$((SIZE / 1024 / 1024))
echo "    ${SIZE_MB}MB"
[ "$SIZE" -gt 0 ]
check "Binary exists and non-empty" $?

echo "  → --version"
VERSION_OUT=$(./keldron-agent --version 2>&1 || echo "")
echo "    $VERSION_OUT"
echo "$VERSION_OUT" | grep -qi "keldron\|agent\|dev" 2>/dev/null
check "--version prints version" $?

echo "  → --help"
./keldron-agent --help > /dev/null 2>&1
check "--help exits cleanly" $?

echo "  → Cross-compile linux/amd64"
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/agent 2>&1
check "Cross-compile linux/amd64" $?

echo "  → Cross-compile linux/arm64"
GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/agent 2>&1
check "Cross-compile linux/arm64" $?

# ═══════════════════════════════════════════
# SECTION 2: TESTS
# ═══════════════════════════════════════════
echo ""
echo "${CYAN}── Section 2: Tests ──${NC}"

echo "  → go test -race (short mode)"
TEST_OUTPUT=$(go test ./... -short -race -count=1 2>&1)
TEST_EXIT=$?
PASSED_COUNT=$(echo "$TEST_OUTPUT" | grep -c "^ok" || echo "0")
FAILED_COUNT=$(echo "$TEST_OUTPUT" | grep -c "^FAIL" || echo "0")
echo "    Packages passed: $PASSED_COUNT, failed: $FAILED_COUNT"
check "All tests pass with race detector" $TEST_EXIT

echo "  → go vet"
go vet ./... 2>&1
check "go vet clean" $?

echo "  → gofmt check"
UNFORMATTED=$(go list -f '{{.Dir}}' ./... 2>/dev/null | sort -u | xargs gofmt -l 2>/dev/null | grep -v '^$' || true)
[ -z "$UNFORMATTED" ]
check "All files gofmt'd" $?
if [ -n "$UNFORMATTED" ]; then
    echo "    Unformatted: $UNFORMATTED"
fi

# ═══════════════════════════════════════════
# SECTION 3: LOCAL AGENT (no hub)
# ═══════════════════════════════════════════
echo ""
echo "${CYAN}── Section 3: Local Agent ──${NC}"

# Create a test config without hub
cat > /tmp/keldron-test-local.yaml << 'EOF'
agent:
  device_name: "diagnostic-test"
  poll_interval: 10s
  log_level: warn
  electricity_rate: 0.12
adapters:
  apple_silicon:
    enabled: true
output:
  stdout: false
  prometheus: true
  prometheus_port: 9100
hub:
  enabled: false
EOF

echo "  → Starting agent (local mode)..."
./keldron-agent --config /tmp/keldron-test-local.yaml --local > /tmp/keldron-diag-local.log 2>&1 &
AGENT_PID=$!
sleep 5

kill -0 $AGENT_PID 2>/dev/null
check "Agent starts and stays running" $?

# Health endpoint
echo "  → Health check"
HEALTH=$(curl -sf localhost:9100/healthz 2>/dev/null || curl -sf localhost:8081/healthz 2>/dev/null || echo "")
[ -n "$HEALTH" ]
check "/healthz returns response" $?
if [ -n "$HEALTH" ]; then
    echo "    $HEALTH" | head -1
fi

# Prometheus endpoint exists
echo "  → Prometheus /metrics"
METRICS_RAW=$(curl -sf localhost:9100/metrics 2>/dev/null || echo "")
[ -n "$METRICS_RAW" ]
check "/metrics returns response" $?

# Count keldron_ metrics
KELDRON_COUNT=$(echo "$METRICS_RAW" | grep -c "^keldron_" || echo "0")
echo "    Found $KELDRON_COUNT keldron_* metric lines"
[ "$KELDRON_COUNT" -gt 5 ]
check "At least 5 keldron_* metric lines present" $?

# ─── Individual metric checks ───
echo ""
echo "  ${CYAN}── Telemetry Metrics ──${NC}"

# Temperature
TEMP=$(echo "$METRICS_RAW" | grep 'keldron_gpu_temperature_celsius{' | awk '{print $2}' | head -1)
[ -n "$TEMP" ]
check "keldron_gpu_temperature_celsius present" $?
if [ -n "$TEMP" ]; then
    TEMP_OK=$(awk -v t="$TEMP" 'BEGIN{print (t+0>0)?1:0}' 2>/dev/null || echo "0")
    [ "$TEMP_OK" = "1" ]
    check "Temperature is non-zero (${TEMP}°C)" $?
fi

# Power
POWER=$(echo "$METRICS_RAW" | grep 'keldron_gpu_power_watts{' | awk '{print $2}' | head -1)
[ -n "$POWER" ]
check "keldron_gpu_power_watts present" $?

# Utilization
UTIL=$(echo "$METRICS_RAW" | grep 'keldron_gpu_utilization_ratio{' | awk '{print $2}' | head -1)
[ -n "$UTIL" ]
check "keldron_gpu_utilization_ratio present" $?

# Memory used
MEM_USED=$(echo "$METRICS_RAW" | grep 'keldron_gpu_memory_used_bytes{' | awk '{print $2}' | head -1)
[ -n "$MEM_USED" ]
check "keldron_gpu_memory_used_bytes present" $?
if [ -n "$MEM_USED" ]; then
    MEM_OK=$(awk -v m="$MEM_USED" 'BEGIN{print (m+0>0)?1:0}' 2>/dev/null || echo "0")
    [ "$MEM_OK" = "1" ]
    check "Memory used is non-zero" $?
fi

# Memory total
MEM_TOTAL=$(echo "$METRICS_RAW" | grep 'keldron_gpu_memory_total_bytes{' | awk '{print $2}' | head -1)
[ -n "$MEM_TOTAL" ]
check "keldron_gpu_memory_total_bytes present" $?

# Memory pressure
MEM_PRESS=$(echo "$METRICS_RAW" | grep 'keldron_gpu_memory_pressure_ratio{' | awk '{print $2}' | head -1)
[ -n "$MEM_PRESS" ]
check "keldron_gpu_memory_pressure_ratio present" $?

# Swap
SWAP_USED=$(echo "$METRICS_RAW" | grep 'keldron_system_swap_used_bytes' | grep -v '^#' | awk '{print $2}' | head -1)
[ -n "$SWAP_USED" ]
check "keldron_system_swap_used_bytes present" $?

SWAP_TOTAL=$(echo "$METRICS_RAW" | grep 'keldron_system_swap_total_bytes' | grep -v '^#' | awk '{print $2}' | head -1)
[ -n "$SWAP_TOTAL" ]
check "keldron_system_swap_total_bytes present" $?

# Throttle
THROTTLE=$(echo "$METRICS_RAW" | grep 'keldron_gpu_throttle_active{' | awk '{print $2}' | head -1)
[ -n "$THROTTLE" ]
check "keldron_gpu_throttle_active present" $?

# Agent info
AGENT_INFO=$(echo "$METRICS_RAW" | grep 'keldron_agent_info{' | head -1)
[ -n "$AGENT_INFO" ]
check "keldron_agent_info present" $?

echo ""
echo "  ${CYAN}── Risk Score Metrics ──${NC}"

# Risk composite
RISK_COMP=$(echo "$METRICS_RAW" | grep 'keldron_risk_composite{' | awk '{print $2}' | head -1)
[ -n "$RISK_COMP" ]
check "keldron_risk_composite present" $?
echo "    Composite: ${RISK_COMP:-missing}"

# Risk thermal
RISK_THERM=$(echo "$METRICS_RAW" | grep 'keldron_risk_thermal{' | awk '{print $2}' | head -1)
[ -n "$RISK_THERM" ]
check "keldron_risk_thermal present" $?

# Risk power
RISK_POWER=$(echo "$METRICS_RAW" | grep 'keldron_risk_power{' | awk '{print $2}' | head -1)
[ -n "$RISK_POWER" ]
check "keldron_risk_power present" $?

# Risk volatility
RISK_VOL=$(echo "$METRICS_RAW" | grep 'keldron_risk_volatility{' | awk '{print $2}' | head -1)
[ -n "$RISK_VOL" ]
check "keldron_risk_volatility present" $?

# Risk severity
RISK_SEV=$(echo "$METRICS_RAW" | grep 'keldron_risk_severity{' | awk '{print $2}' | head -1)
[ -n "$RISK_SEV" ]
check "keldron_risk_severity present" $?
echo "    Severity: ${RISK_SEV:-missing} (0=normal, 1=warning, 2=critical)"

# Warming up
WARMING=$(echo "$METRICS_RAW" | grep 'keldron_risk_warming_up{' | awk '{print $2}' | head -1)
[ -n "$WARMING" ]
check "keldron_risk_warming_up present" $?

# Fleet penalty
FLEET_PEN=$(echo "$METRICS_RAW" | grep 'keldron_risk_fleet_penalty{' | awk '{print $2}' | head -1)
[ -n "$FLEET_PEN" ]
check "keldron_risk_fleet_penalty present" $?

echo ""
echo "  ${CYAN}── Bonus Metrics ──${NC}"

# Power cost
COST_H=$(echo "$METRICS_RAW" | grep 'keldron_power_cost_hourly{' | awk '{print $2}' | head -1)
[ -n "$COST_H" ]
check "keldron_power_cost_hourly present" $?

COST_D=$(echo "$METRICS_RAW" | grep 'keldron_power_cost_daily{' | awk '{print $2}' | head -1)
[ -n "$COST_D" ]
check "keldron_power_cost_daily present" $?

COST_M=$(echo "$METRICS_RAW" | grep 'keldron_power_cost_monthly{' | awk '{print $2}' | head -1)
[ -n "$COST_M" ]
check "keldron_power_cost_monthly present" $?
echo "    Monthly cost: \$${COST_M:-0}"

# Clock efficiency
CLOCK_EFF=$(echo "$METRICS_RAW" | grep 'keldron_gpu_clock_efficiency{' | awk '{print $2}' | head -1)
[ -n "$CLOCK_EFF" ]
check "keldron_gpu_clock_efficiency present" $?

# Hotspot delta
HOTSPOT=$(echo "$METRICS_RAW" | grep 'keldron_gpu_hotspot_delta_celsius{' | awk '{print $2}' | head -1)
[ -n "$HOTSPOT" ]
check "keldron_gpu_hotspot_delta_celsius present" $?

echo ""
echo "  ${CYAN}── Label Validation ──${NC}"

# Check labels on temperature metric
TEMP_LINE=$(echo "$METRICS_RAW" | grep 'keldron_gpu_temperature_celsius{' | head -1)

echo "$TEMP_LINE" | grep -q 'device_model="M[0-9]'
check "device_model label is M-series (not raw string)" $?

echo "$TEMP_LINE" | grep -q 'behavior_class="soc_integrated"'
check "behavior_class label is soc_integrated" $?

echo "$TEMP_LINE" | grep -q 'device_vendor="apple"'
check "device_vendor label is apple" $?

echo "$TEMP_LINE" | grep -q 'adapter="apple_silicon"'
check "adapter label is apple_silicon" $?

echo "    Labels: $(echo "$TEMP_LINE" | grep -o '{.*}')"

# ─── Wait for second poll and check scores change ───
echo ""
echo "  ${CYAN}── Score Stability (waiting for 2nd poll) ──${NC}"
echo "  → Waiting 12 seconds..."
sleep 12

METRICS_2=$(curl -sf localhost:9100/metrics 2>/dev/null || echo "")
RISK_COMP_2=$(echo "$METRICS_2" | grep 'keldron_risk_composite{' | awk '{print $2}' | head -1)
TEMP_2=$(echo "$METRICS_2" | grep 'keldron_gpu_temperature_celsius{' | awk '{print $2}' | head -1)

[ -n "$RISK_COMP_2" ]
check "Risk composite still present after 2nd poll" $?
echo "    Poll 1: temp=${TEMP:-?}°C risk=${RISK_COMP:-?}"
echo "    Poll 2: temp=${TEMP_2:-?}°C risk=${RISK_COMP_2:-?}"

# Kill local agent
kill $AGENT_PID 2>/dev/null
wait $AGENT_PID 2>/dev/null
sleep 2

# ═══════════════════════════════════════════
# SECTION 4: HUB MODE
# ═══════════════════════════════════════════
echo ""
echo "${CYAN}── Section 4: Hub Mode ──${NC}"

cat > /tmp/keldron-test-hub.yaml << 'EOF'
agent:
  device_name: "diagnostic-hub"
  poll_interval: 10s
  log_level: info
  electricity_rate: 0.12
adapters:
  apple_silicon:
    enabled: true
output:
  stdout: false
  prometheus: true
  prometheus_port: 9100
hub:
  enabled: true
  listen_port: 9200
  scrape_interval: 15s
  mdns_enabled: true
  static_peers: []
EOF

echo "  → Starting agent (hub mode)..."
./keldron-agent --config /tmp/keldron-test-hub.yaml --local > /tmp/keldron-diag-hub.log 2>&1 &
HUB_PID=$!
sleep 5

kill -0 $HUB_PID 2>/dev/null
check "Hub agent starts and stays running" $?

# Check for panics in log
PANICS=$(grep -c "panic" /tmp/keldron-diag-hub.log 2>/dev/null || true)
PANICS=${PANICS:-0}
[ "$PANICS" -eq 0 ]
check "No panics in hub startup" $?
if [ "$PANICS" -gt 0 ]; then
    echo "    PANIC FOUND:"
    grep "panic" /tmp/keldron-diag-hub.log | head -3
fi

# Hub log confirms hub mode
grep -qi "hub" /tmp/keldron-diag-hub.log 2>/dev/null
check "Hub mode confirmed in logs" $?

# Fleet API
echo "  → Fleet API"
FLEET=$(curl -sf localhost:9200/api/v1/fleet 2>/dev/null || echo "")
[ -n "$FLEET" ]
check "/api/v1/fleet returns response" $?
if [ -n "$FLEET" ]; then
    echo "    $(echo "$FLEET" | head -c 200)"
fi

# Fleet devices endpoint
FLEET_DEV=$(curl -sf localhost:9200/api/v1/fleet/devices 2>/dev/null || echo "")
[ -n "$FLEET_DEV" ]
check "/api/v1/fleet/devices returns response" $?

# Fleet peers endpoint
FLEET_PEERS=$(curl -sf localhost:9200/api/v1/fleet/peers 2>/dev/null || echo "")
[ -n "$FLEET_PEERS" ]
check "/api/v1/fleet/peers returns response" $?

# Hub metrics in Prometheus
HUB_METRICS=$(curl -sf localhost:9100/metrics 2>/dev/null || echo "")
echo "$HUB_METRICS" | grep -q 'keldron_hub_peers_total' 2>/dev/null
check "keldron_hub_peers_total metric present" $?

echo "$HUB_METRICS" | grep -q 'keldron_hub_devices_total' 2>/dev/null
check "keldron_hub_devices_total metric present" $?

# mDNS advertising
echo "  → mDNS"
MDNS_LOG=$(grep -i "mdns\|zeroconf\|advertis" /tmp/keldron-diag-hub.log 2>/dev/null || echo "")
if [ -n "$MDNS_LOG" ]; then
    echo "    mDNS activity found in logs"
    check "mDNS initialized" 0
else
    warn "mDNS not visible in logs (may still be working)"
fi

# Kill hub
kill $HUB_PID 2>/dev/null
wait $HUB_PID 2>/dev/null
sleep 2

# ═══════════════════════════════════════════
# SECTION 5: FILES & STRUCTURE
# ═══════════════════════════════════════════
echo ""
echo "${CYAN}── Section 5: Repo Structure ──${NC}"

[ -f "LICENSE" ]
check "LICENSE file exists" $?

[ -f "README.md" ]
check "README.md exists" $?

[ -f "Makefile" ]
check "Makefile exists" $?

[ -f "Dockerfile" ]
check "Dockerfile exists" $?

[ -f ".gitignore" ]
check ".gitignore exists" $?

[ -f "SKILL.md" ]
check "SKILL.md (OpenClaw skill) exists" $?

[ -f "go.mod" ]
check "go.mod exists" $?

# Check module path
grep -q "keldron-ai/keldron-agent" go.mod 2>/dev/null
check "go.mod has correct module path" $?

# No uponline references
UPONLINE_COUNT=$(grep -rc "uponline\|UpOnline" --include="*.go" . 2>/dev/null | awk -F: '{s+=$NF} END{print s+0}')
[ "$UPONLINE_COUNT" -eq 0 ]
check "Zero uponline references in .go files" $?

# Registry
REGISTRY=$(find . -name "gpu_specs.json" -o -name "gpu_specs.go" 2>/dev/null | head -1)
[ -n "$REGISTRY" ]
check "GPU spec registry exists" $?
if [ -n "$REGISTRY" ] && [[ "$REGISTRY" == *.json ]]; then
    ENTRY_COUNT=$(grep -c '"thermal_limit' "$REGISTRY" 2>/dev/null || echo "0")
    echo "    Registry entries: $ENTRY_COUNT"
    [ "$ENTRY_COUNT" -ge 25 ]
    check "Registry has 25+ entries" $?
fi

# Pre-commit hooks
[ -f ".githooks/pre-commit" ]
check "Pre-commit hook exists" $?

# ═══════════════════════════════════════════
# SECTION 6: DOCKER
# ═══════════════════════════════════════════
echo ""
echo "${CYAN}── Section 6: Docker ──${NC}"

if command -v docker &> /dev/null; then
    echo "  → Docker build"
    docker build -t keldron-agent:diag-test . > /dev/null 2>&1
    BUILD_EXIT=$?
    check "Docker build succeeds" $BUILD_EXIT

    if [ "$BUILD_EXIT" -eq 0 ]; then
        IMG_SIZE=$(docker image inspect keldron-agent:diag-test --format='{{.Size}}' 2>/dev/null || echo "0")
        IMG_SIZE_MB=$((IMG_SIZE / 1024 / 1024))
        echo "    Image size: ${IMG_SIZE_MB}MB"
        [ "$IMG_SIZE_MB" -lt 100 ]
        check "Docker image under 100MB" $?

        # Quick run test
        docker run --rm keldron-agent:diag-test --version > /dev/null 2>&1
        check "Docker container runs --version" $?

        # Cleanup
        docker rmi keldron-agent:diag-test > /dev/null 2>&1
    fi
else
    warn "Docker not available — skipping container tests"
fi

# ═══════════════════════════════════════════
# SECTION 7: CLEANUP
# ═══════════════════════════════════════════
echo ""
echo "${CYAN}── Cleanup ──${NC}"
cleanup
echo "  Cleaned up temp files and processes"

# ═══════════════════════════════════════════
# RESULTS
# ═══════════════════════════════════════════
echo ""
echo "${CYAN}═══════════════════════════════════════════════════${NC}"
echo "${CYAN}  Results${NC}"
echo "${CYAN}═══════════════════════════════════════════════════${NC}"
echo ""

for result in "${RESULTS[@]}"; do
    echo -e "  $result"
done

echo ""
echo "${CYAN}───────────────────────────────────────────────────${NC}"
echo -e "  ${GREEN}Passed: $PASS${NC}  ${RED}Failed: $FAIL${NC}  ${AMBER}Warnings: $WARN${NC}"
echo "${CYAN}───────────────────────────────────────────────────${NC}"

if [ "$FAIL" -eq 0 ]; then
    echo ""
    echo -e "  ${GREEN}🎉 All checks passed. Agent is ready for handoff.${NC}"
    echo ""
else
    echo ""
    echo -e "  ${RED}⚠️  $FAIL check(s) failed. Review above for details.${NC}"
    echo ""
fi

# Quick summary for pasting
echo "${CYAN}── Quick Summary (copy-paste friendly) ──${NC}"
echo ""
echo "Keldron Agent Diagnostic — $(date)"
echo "Build: $(./keldron-agent --version 2>&1 | head -1)"
echo "Platform: $(uname -ms)"
echo "Checks: $PASS passed, $FAIL failed, $WARN warnings"
echo "Temperature: ${TEMP:-?}°C"
echo "Risk Composite: ${RISK_COMP:-?}"
echo "Risk Severity: ${RISK_SEV:-?}"
echo "Monthly Cost: \$${COST_M:-?}"
echo "Memory Pressure: ${MEM_PRESS:-?}"
echo "Metrics Count: ${KELDRON_COUNT:-?} lines"
echo "Hub Mode: $([ -n "$FLEET" ] && echo "Working" || echo "Failed")"
echo ""

exit $FAIL
