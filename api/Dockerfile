# Build stage — runs on host arch, cross-compiles via GOARCH
# Uses modernc.org/sqlite (pure Go) so CGO is not needed
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Get version information
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETARCH

# Build static binary — no CGO needed (modernc.org/sqlite is pure Go)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s \
      -X 'taskai/internal/version.Version=${VERSION}' \
      -X 'taskai/internal/version.GitCommit=${GIT_COMMIT}' \
      -X 'taskai/internal/version.BuildTime=${BUILD_TIME}'" \
    -o taskai \
    ./cmd/api

# Production stage
FROM alpine:3.19

# Install CA certificates, pg_dump (for backup), and create non-root user
RUN apk --no-cache add ca-certificates tzdata postgresql-client && \
    addgroup -g 1001 -S taskai && \
    adduser -u 1001 -S taskai -G taskai

# Create data directory with correct permissions
RUN mkdir -p /data && chown taskai:taskai /data

WORKDIR /app

# Copy binary from builder
COPY --from=builder --chown=taskai:taskai /build/taskai .

# Copy OpenAPI spec (needed for /api/openapi endpoint)
COPY --chown=taskai:taskai openapi.yaml .

# Copy database migrations
COPY --chown=taskai:taskai internal/db/migrations ./internal/db/migrations

# Switch to non-root user
USER taskai

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Run the binary
CMD ["./taskai"]
