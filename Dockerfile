# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=$(cat VERSION 2>/dev/null || echo dev)" \
    -o keldron-agent ./cmd/agent

# Runtime stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates && \
    addgroup -g 1000 keldron && \
    adduser -u 1000 -G keldron -D keldron
COPY --from=builder /build/keldron-agent /usr/local/bin/keldron-agent
USER keldron
EXPOSE 9100 8081
ENTRYPOINT ["/usr/local/bin/keldron-agent"]
CMD ["--config", "/etc/keldron/keldron-agent.yaml", "--local"]
