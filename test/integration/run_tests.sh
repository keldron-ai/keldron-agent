#!/bin/bash
set -e

# Run from agent/ directory (where go.mod lives)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$AGENT_ROOT"

COMPOSE_FILE="$SCRIPT_DIR/docker-compose.test.yml"

cleanup() {
    echo "Stopping simulators..."
    docker compose -f "$COMPOSE_FILE" down
}
trap cleanup EXIT

echo "Starting simulators..."
docker compose -f "$COMPOSE_FILE" up -d --wait

echo "Running integration tests..."
go mod download
go test -tags integration ./test/integration/... -v -race -count=1
