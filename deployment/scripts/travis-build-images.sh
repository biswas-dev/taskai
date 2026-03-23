#!/bin/bash
# Travis CI image build script for TaskAI
# Builds Docker images and pushes to Harbor registry.
#
# Strategy: Build BOTH arm64 and amd64 as separate tags, in parallel.
#   - staging/UAT use arm64 images  (ARM servers)
#   - production uses amd64 images  (x86 server)
#   - No QEMU emulation needed — each arch builds natively via buildx cross-compilation
#
# Tags: git-<sha>-arm64, git-<sha>-amd64, staging-latest, prod-latest
# Captures and persists the image digests for deploy stages to consume.
#
# Required Travis CI env vars:
#   HARBOR_AUTH (base64-encoded "username:password")

set -euo pipefail

GIT_SHA=$(git rev-parse --short HEAD)
VERSION=$(cat VERSION 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Decode Harbor credentials
HARBOR_USERNAME=$(echo "$HARBOR_AUTH" | base64 -d | cut -d: -f1)
HARBOR_PASSWORD=$(echo "$HARBOR_AUTH" | base64 -d | cut -d: -f2)

echo "=== Building TaskAI Images ==="
echo "Version: $VERSION"
echo "Commit:  $GIT_SHA"
echo ""

# Login to Harbor
echo "$HARBOR_PASSWORD" | docker login harbor.biswas.me -u "$HARBOR_USERNAME" --password-stdin

# Set up buildx
docker buildx create --name taskbuild --use 2>/dev/null || docker buildx use taskbuild

REGISTRY="harbor.biswas.me/biswas"

# Helper: build a single image for a single arch, push, capture digest
build_one() {
  local name="$1"
  local dockerfile="$2"
  local context="$3"
  local arch="$4"
  shift 4
  local extra_args=("$@")

  local tag="${REGISTRY}/${name}:git-${GIT_SHA}-${arch}"

  echo "[${name}] Building for ${arch}..."
  docker buildx build \
    --platform "linux/${arch}" \
    --file "$dockerfile" \
    "${extra_args[@]}" \
    --tag "$tag" \
    --metadata-file "/tmp/${name}-${arch}-metadata.json" \
    --push \
    "$context"

  local digest
  digest=$(jq -r '.["containerimage.digest"]' "/tmp/${name}-${arch}-metadata.json")
  if [ -z "$digest" ] || [ "$digest" = "null" ]; then
    echo "ERROR: Failed to extract digest for $name ($arch)"
    exit 1
  fi
  echo "[${name}] ${arch} digest: $digest"

  # Write digest
  local var_name
  var_name=$(echo "${name}_${arch}" | tr '-' '_' | tr '[:lower:]' '[:upper:]')_DIGEST
  echo "${var_name}=${digest}" >> .image-digests.txt
}

# Helper: tag a digest as an env-latest alias
tag_latest() {
  local name="$1"
  local digest="$2"
  local label="$3"

  # Delete existing tag (Harbor immutable tags)
  curl -sf -u "$HARBOR_USERNAME:$HARBOR_PASSWORD" \
    -X DELETE "https://harbor.biswas.me/api/v2.0/projects/biswas/repositories/${name}/artifacts/${label}/tags/${label}" 2>/dev/null || true

  curl -sf -u "$HARBOR_USERNAME:$HARBOR_PASSWORD" \
    -X POST "https://harbor.biswas.me/api/v2.0/projects/biswas/repositories/${name}/artifacts/${digest}/tags" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"${label}\"}"
  echo "[${name}] Tagged ${label}"
}

# Clear digests file
> .image-digests.txt

BUILD_ARGS_API=(--build-arg "VERSION=$VERSION" --build-arg "GIT_COMMIT=$GIT_SHA" --build-arg "BUILD_TIME=$BUILD_TIME")
BUILD_ARGS_WEB=(--build-arg "VERSION=$VERSION" --build-arg "GIT_COMMIT=$GIT_SHA" --build-arg "BUILD_TIME=$BUILD_TIME")

# Build all images for BOTH architectures in parallel.
# The Dockerfiles use --platform=$BUILDPLATFORM for builder stages (native)
# and cross-compile for target arch — no QEMU emulation for Go/Node builds.
echo ""
echo "=== Building arm64 images (staging/UAT) ==="
build_one "taskai-api" "api/Dockerfile" "./api" "arm64" "${BUILD_ARGS_API[@]}" &
PID_API_ARM=$!
build_one "taskai-web" "web/Dockerfile" "." "arm64" "${BUILD_ARGS_WEB[@]}" &
PID_WEB_ARM=$!
build_one "taskai-mcp" "mcp/Dockerfile" "./mcp" "arm64" &
PID_MCP_ARM=$!
build_one "taskai-yjs" "api/internal/yjs-processor/Dockerfile" "./api/internal/yjs-processor" "arm64" &
PID_YJS_ARM=$!

echo ""
echo "=== Building amd64 images (production) ==="
build_one "taskai-api" "api/Dockerfile" "./api" "amd64" "${BUILD_ARGS_API[@]}" &
PID_API_AMD=$!
build_one "taskai-web" "web/Dockerfile" "." "amd64" "${BUILD_ARGS_WEB[@]}" &
PID_WEB_AMD=$!
build_one "taskai-mcp" "mcp/Dockerfile" "./mcp" "amd64" &
PID_MCP_AMD=$!
build_one "taskai-yjs" "api/internal/yjs-processor/Dockerfile" "./api/internal/yjs-processor" "amd64" &
PID_YJS_AMD=$!

# Wait for all builds
FAILED=0
for pid in $PID_API_ARM $PID_WEB_ARM $PID_MCP_ARM $PID_YJS_ARM $PID_API_AMD $PID_WEB_AMD $PID_MCP_AMD $PID_YJS_AMD; do
  wait "$pid" || FAILED=1
done

if [ "$FAILED" -ne 0 ]; then
  echo "ERROR: One or more builds failed"
  exit 1
fi

echo ""
echo "=== Tagging staging-latest (arm64) and prod-latest (amd64) ==="
for name in taskai-api taskai-web taskai-mcp taskai-yjs; do
  arm_digest=$(jq -r '.["containerimage.digest"]' "/tmp/${name}-arm64-metadata.json")
  amd_digest=$(jq -r '.["containerimage.digest"]' "/tmp/${name}-amd64-metadata.json")
  tag_latest "$name" "$arm_digest" "staging-latest"
  tag_latest "$name" "$amd_digest" "prod-latest"
done

echo ""
echo "=== TaskAI images pushed ==="
cat .image-digests.txt
