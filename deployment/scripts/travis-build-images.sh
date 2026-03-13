#!/bin/bash
# Travis CI image build script for TaskAI
# Builds multi-arch Docker images and pushes to Harbor registry
# Tags with git-<sha> (immutable) and staging-latest
# Captures and persists the image digests for deploy-staging to consume
#
# Required Travis CI env vars:
#   HARBOR_AUTH (base64-encoded "username:password" — avoids $ shell expansion)

set -euo pipefail

GIT_SHA=$(git rev-parse --short HEAD)
VERSION=$(cat VERSION 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
TAG="git-${GIT_SHA}"

# Decode Harbor credentials (base64 avoids Travis $ expansion issues)
HARBOR_USERNAME=$(echo "$HARBOR_AUTH" | base64 -d | cut -d: -f1)
HARBOR_PASSWORD=$(echo "$HARBOR_AUTH" | base64 -d | cut -d: -f2)

echo "=== Building TaskAI Images ==="
echo "Version: $VERSION"
echo "Tag:     $TAG"
echo "Commit:  $GIT_SHA"
echo ""

# Login to Harbor
echo "$HARBOR_PASSWORD" | docker login harbor.biswas.me -u "$HARBOR_USERNAME" --password-stdin

# Set up buildx for multi-arch
docker buildx create --name multiarch --use 2>/dev/null || docker buildx use multiarch

# Helper to build, push, capture digest, and tag staging-latest
build_and_tag() {
  local name="$1"
  local dockerfile="$2"
  local context="$3"
  shift 3
  local extra_args=("$@")

  echo "--- Building $name image ---"
  docker buildx build \
    --platform linux/arm64,linux/amd64 \
    --file "$dockerfile" \
    "${extra_args[@]}" \
    --tag "harbor.biswas.me/biswas/${name}:${TAG}" \
    --metadata-file "/tmp/${name}-metadata.json" \
    --push \
    "$context"

  local digest
  digest=$(jq -r '.["containerimage.digest"]' "/tmp/${name}-metadata.json")
  if [ -z "$digest" ] || [ "$digest" = "null" ]; then
    echo "ERROR: Failed to extract digest for $name"
    cat "/tmp/${name}-metadata.json"
    exit 1
  fi

  echo "$name digest: $digest"

  # Tag as staging-latest via Harbor API
  curl -sf -u "$HARBOR_USERNAME:$HARBOR_PASSWORD" \
    -X POST "https://harbor.biswas.me/api/v2.0/projects/biswas/repositories/${name}/artifacts/${digest}/tags" \
    -H "Content-Type: application/json" \
    -d '{"name": "staging-latest"}'

  echo "Tagged $name as staging-latest"

  # Output digest variable
  local var_name
  var_name=$(echo "$name" | tr '-' '_' | tr '[:lower:]' '[:upper:]')_DIGEST
  echo "${var_name}=${digest}" >> .image-digests.txt
}

# Clear digests file
> .image-digests.txt

# 1. API image
build_and_tag "taskai-api" "api/Dockerfile" "./api" \
  --build-arg "VERSION=$VERSION" \
  --build-arg "GIT_COMMIT=$GIT_SHA" \
  --build-arg "BUILD_TIME=$BUILD_TIME"

# 2. Web image (context is root — Dockerfile copies from web/ and docs/)
build_and_tag "taskai-web" "web/Dockerfile" "." \
  --build-arg "VERSION=$VERSION" \
  --build-arg "GIT_COMMIT=$GIT_SHA" \
  --build-arg "BUILD_TIME=$BUILD_TIME"

# 3. MCP image
build_and_tag "taskai-mcp" "mcp/Dockerfile" "./mcp"

# 4. Yjs processor image
build_and_tag "taskai-yjs" "api/internal/yjs-processor/Dockerfile" "./api/internal/yjs-processor"

echo ""
echo "=== TaskAI images pushed ==="
cat .image-digests.txt
