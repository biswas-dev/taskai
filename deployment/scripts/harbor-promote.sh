#!/bin/bash
# Promotes image digests from one environment to the next via Harbor API.
# Usage: harbor-promote.sh <project> <source-env> <target-env> <repo1> [repo2...]
# Example: harbor-promote.sh biswas staging uat taskai-api taskai-web taskai-mcp taskai-yjs
#
# For each repo, finds the digest tagged as <source-env>-latest, adds <target-env>-latest tag.
# Outputs digests as REPO=DIGEST lines for eval by caller.
#
# Required env vars: HARBOR_USERNAME, HARBOR_PASSWORD (or HARBOR_AUTH base64)

set -euo pipefail

# Decode Harbor credentials if using base64 format
if [ -n "${HARBOR_AUTH:-}" ]; then
  HARBOR_USERNAME=$(echo "$HARBOR_AUTH" | base64 -d | cut -d: -f1)
  HARBOR_PASSWORD=$(echo "$HARBOR_AUTH" | base64 -d | cut -d: -f2)
fi

PROJECT="${1:?Usage: harbor-promote.sh <project> <source-env> <target-env> <repo1> [repo2...]}"
SOURCE_ENV="$2"
TARGET_ENV="$3"
shift 3

HARBOR_URL="https://harbor.biswas.me"

for REPO in "$@"; do
  echo "=== Promoting $REPO: $SOURCE_ENV -> $TARGET_ENV ===" >&2

  DIGEST=$(curl -sf -u "$HARBOR_USERNAME:$HARBOR_PASSWORD" \
    "$HARBOR_URL/api/v2.0/projects/$PROJECT/repositories/$REPO/artifacts?q=tags%3D${SOURCE_ENV}-latest" \
    | jq -r '.[0].digest')

  if [ -z "$DIGEST" ] || [ "$DIGEST" = "null" ]; then
    echo "ERROR: No artifact found with tag ${SOURCE_ENV}-latest in $PROJECT/$REPO" >&2
    exit 1
  fi

  echo "Found digest: $DIGEST" >&2

  curl -sf -u "$HARBOR_USERNAME:$HARBOR_PASSWORD" \
    -X POST "$HARBOR_URL/api/v2.0/projects/$PROJECT/repositories/$REPO/artifacts/$DIGEST/tags" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"${TARGET_ENV}-latest\"}"

  echo "Tagged as ${TARGET_ENV}-latest" >&2

  # Output as KEY=VALUE (repo name with hyphens converted to underscores, uppercased)
  VAR_NAME=$(echo "$REPO" | tr '-' '_' | tr '[:lower:]' '[:upper:]')_DIGEST
  echo "${VAR_NAME}=${DIGEST}"
done
