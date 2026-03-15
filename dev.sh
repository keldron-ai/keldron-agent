#!/bin/bash
set -euo pipefail

echo "═══════════════════════════════════════════"
echo "  Keldron Agent — Local Dev Runner"
echo "═══════════════════════════════════════════"

# Build
echo "📦 Building agent..."
go build -o keldron-agent ./cmd/agent

# Create dev config if it doesn't exist
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
  nvidia_consumer:
    enabled: false
  dcgm:
    enabled: false
  rocm:
    enabled: false
  linux_thermal:
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

echo ""
echo "🚀 Starting agent..."
echo "   Prometheus metrics: http://localhost:9100/metrics"
echo "   Health check:       http://localhost:8081/healthz"
echo "   Press Ctrl+C to stop"
echo ""

./keldron-agent --config keldron-agent.dev.yaml --local
