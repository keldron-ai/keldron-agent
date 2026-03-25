# Dashboard (Vite) — embed output must exist before go:embed (internal/api/frontend.go)
FROM node:22-alpine AS frontend
WORKDIR /frontend
COPY frontend/ ./
# Match Makefile: prefer npm (lockfile may lag package.json in dev); full install before build
RUN (npm ci 2>/dev/null || npm install) && npm run build

# Go binary
FROM golang:1.26-alpine AS builder
ARG TARGETARCH=amd64
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /frontend/dist/ ./internal/api/static/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.version=$(cat VERSION 2>/dev/null || echo dev)" \
    -o keldron-agent ./cmd/agent

FROM alpine:3.19
RUN apk --no-cache add ca-certificates && \
    addgroup -g 1000 keldron && \
    adduser -u 1000 -G keldron -D keldron
COPY --from=builder /build/keldron-agent /usr/local/bin/keldron-agent
USER keldron
EXPOSE 9100 9200 8081
ENTRYPOINT ["/usr/local/bin/keldron-agent"]
CMD ["--config", "/etc/keldron/keldron-agent.yaml", "--local"]
