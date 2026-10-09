# Build on the host architecture and cross-compile; the binary is static.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY *.go ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /taskai-mcp .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    addgroup -g 1001 -S taskai && adduser -u 1001 -S taskai -G taskai
COPY --from=builder /taskai-mcp /usr/local/bin/taskai-mcp
USER taskai
EXPOSE 3000
# busybox wget, used by the compose healthcheck as well.
HEALTHCHECK --interval=30s --timeout=3s --retries=3 --start-period=5s \
  CMD wget -q --spider http://localhost:3000/health || exit 1
CMD ["taskai-mcp"]
