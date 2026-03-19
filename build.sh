#!/bin/bash
# build.sh — builds the full agent binary with embedded frontend

set -e

echo "Building frontend..."
cd frontend
if command -v pnpm &>/dev/null && [ -f pnpm-lock.yaml ]; then
  pnpm install --frozen-lockfile
  pnpm run build
else
  npm ci 2>/dev/null || npm install
  npm run build
fi
cd ..

echo "Copying frontend to embed location..."
rm -rf internal/api/static
mkdir -p internal/api/static
cp -r frontend/dist/* internal/api/static/

echo "Building Go binary..."
go build -o keldron-agent ./cmd/agent

echo "Done. Binary: ./keldron-agent"
echo "Run: ./keldron-agent"
echo "Dashboard: http://localhost:9200"
