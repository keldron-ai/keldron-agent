#!/bin/bash
# build.sh — builds the full agent binary with embedded frontend (delegates to Make)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

make build

echo "Done. Binary: ./keldron-agent"
echo "Run: ./keldron-agent"
echo "Dashboard: http://localhost:9200"
