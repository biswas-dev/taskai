# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Get version information
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

# Build static binary with CGO enabled for SQLite and version info
RUN CGO_ENABLED=1 go build \
    -ldflags="-w -s -extldflags '-static' \
      -X 'taskai/internal/version.Version=${VERSION}' \
      -X 'taskai/internal/version.GitCommit=${GIT_COMMIT}' \
      -X 'taskai/internal/version.BuildTime=${BUILD_TIME}'" \
    -tags sqlite_omit_load_extension \
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
