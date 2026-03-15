.PHONY: build build-all build-dcgm test test-dcgm lint clean generate docker-build docker-run

VERSION := 0.1.0-dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o keldron-agent ./cmd/agent

build-dcgm:
	go build -tags dcgm -ldflags "-X main.version=$(VERSION)" -o keldron-agent ./cmd/agent

build-all:
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
	docker build -t keldron-agent:dev .

docker-run:
	docker run --rm -p 9100:9100 -p 8081:8081 \
	  -v $(PWD)/configs/keldron-agent.example.yaml:/etc/keldron/keldron-agent.yaml:ro \
	  keldron-agent:dev
