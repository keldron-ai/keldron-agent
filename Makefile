.PHONY: build build-all build-dcgm frontend-build test test-dcgm lint clean generate docker-build docker-run

VERSION := $(shell cat VERSION 2>/dev/null || echo 0.1.0-dev)

# Build Vite dashboard into internal/api/static/ for go:embed (see internal/api/frontend.go).
# Without npm/pnpm, the committed placeholder index.html is used so go build succeeds.
frontend-build:
	@set -e; \
	if command -v pnpm >/dev/null 2>&1 && [ -f frontend/pnpm-lock.yaml ]; then \
		echo "Building frontend (pnpm)..."; \
		( cd frontend && pnpm install --frozen-lockfile && pnpm run build ); \
	elif command -v npm >/dev/null 2>&1; then \
		echo "Building frontend (npm)..."; \
		( cd frontend && (npm ci 2>/dev/null || npm install) && npm run build ); \
	else \
		echo "npm/pnpm not found — using placeholder dashboard (internal/api/static/index.html)"; \
		mkdir -p internal/api/static; \
		if [ ! -f internal/api/static/index.html ]; then \
			printf '%s\n' '<!DOCTYPE html><html><head><title>Keldron</title></head><body><h1>Dashboard requires frontend build</h1><p>Run: cd frontend && npm install && npm run build</p></body></html>' > internal/api/static/index.html; \
		fi; \
		exit 0; \
	fi; \
	echo "Copying frontend to internal/api/static..."; \
	rm -rf internal/api/static; \
	mkdir -p internal/api/static; \
	cp -R frontend/dist/. internal/api/static/

build: frontend-build
	go build -ldflags "-X main.version=$(VERSION)" -o keldron-agent ./cmd/agent

build-dcgm: frontend-build
	go build -tags dcgm -ldflags "-X main.version=$(VERSION)" -o keldron-agent ./cmd/agent

build-all: frontend-build
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$(VERSION)" -o keldron-agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 go build -ldflags "-X main.version=$(VERSION)" -o keldron-agent-linux-arm64 ./cmd/agent
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X main.version=$(VERSION)" -o keldron-agent-darwin-arm64 ./cmd/agent

test:
	go test ./... -race -v

test-dcgm:
	go test -tags dcgm ./... -race -v

lint:
	golangci-lint run

clean:
	rm -f keldron-agent keldron-agent-*

generate:
	cd internal/proto && buf generate

docker-build:
	docker build -t keldron-agent:$(VERSION) .

docker-run:
	docker run --rm \
	  -p 9100:9100 -p 9200:9200 -p 8081:8081 \
	  -e KELDRON_OUTPUT_PROMETHEUS_HOST=0.0.0.0 \
	  -e KELDRON_API_HOST=0.0.0.0 \
	  -e KELDRON_HEALTH_BIND=0.0.0.0:8081 \
	  -v $(PWD)/configs/keldron-agent.example.yaml:/etc/keldron/keldron-agent.yaml:ro \
	  keldron-agent:$(VERSION)
