#!/bin/bash
set -euo pipefail

# Kill any stale agent process
STALE_PIDS=$(pgrep -f keldron-agent 2>/dev/null || true)
if [ -n "$STALE_PIDS" ]; then
    echo "🧹 Killing stale agent process(es): $STALE_PIDS"
    kill $STALE_PIDS 2>/dev/null
    sleep 1
fi

echo "🔍 Testing keldron-agent..."
echo ""

# Check health
echo "→ Health check"
curl -sf http://localhost:8081/healthz | python3 -m json.tool
echo ""

# Check Prometheus metrics exist
echo "→ Prometheus metrics (keldron_* count)"
METRIC_COUNT=$(curl -sf http://localhost:9100/metrics | grep -c "^keldron_" || echo "0")
echo "   Found $METRIC_COUNT keldron_* metric lines"
echo ""

# Show some actual values
echo "→ Sample metrics:"
curl -sf http://localhost:9100/metrics | grep "keldron_gpu_temperature\|keldron_risk_composite\|keldron_power_cost" | head -10
echo ""

# Check status endpoint
echo "→ Agent status"
curl -sf http://localhost:8081/api/v1/status | python3 -m json.tool 2>/dev/null || echo "   (status endpoint not yet implemented)"
echo ""

echo "✅ Agent is running"
