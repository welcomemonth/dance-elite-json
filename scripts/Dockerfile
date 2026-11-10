FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS backend
WORKDIR /backend-build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# 使用国内 Go 代理加速依赖下载（七牛云 goproxy.cn 是国内最常用的）
ENV GOPROXY=https://goproxy.cn,direct

# Copy go mod files and download dependencies (cached layer)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code (use .dockerignore to exclude unnecessary files)
COPY . .

ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
      -trimpath \
      -ldflags="-s -w -extldflags '-static'" \
      -tags netgo,osusergo \
      -o dance-elite \
      ./cmd/dance-elite

# Use minimal Alpine with security updates
FROM alpine:3.21 AS monolithic

# Install runtime dependencies and create non-root user in single layer
RUN apk add --no-cache tzdata ca-certificates su-exec && \
    addgroup -g 10001 -S nonroot && \
    adduser -u 10001 -S -G nonroot -h /opt/dance-elite nonroot && \
    mkdir -p /opt/dance-elite /usr/local/dance-elite && \
    chown -R nonroot:nonroot /opt/dance-elite

# Copy binary and entrypoint to /usr/local/dance-elite
COPY --from=backend /backend-build/dance-elite /usr/local/dance-elite/dance-elite
COPY --from=backend --chmod=755 /backend-build/scripts/entrypoint.sh /usr/local/dance-elite/entrypoint.sh

# Run as root to fix permissions, entrypoint will drop to nonroot
USER root

# Set working directory to the writable volume
WORKDIR /opt/dance-elite

# Data directory
VOLUME /opt/dance-elite

ENV TZ="UTC" \
    PORT="8090" \
    DATA_DIR="/opt/dance-elite"

EXPOSE 8090

ENTRYPOINT ["/usr/local/dance-elite/entrypoint.sh", "/usr/local/dance-elite/dance-elite"]
