#!/bin/bash
# Fast deployment: build Docker images directly on target servers
#
# Instead of building 8 images on Travis (4 services × 2 arches) and pushing
# to Harbor, this script SSHs into the target server and builds natively.
#
# Speed: ~3-4 min (vs ~12 min for cross-compile + Harbor push/pull)
# Trade-off: Build happens on server (uses server CPU briefly)
#
# Usage: bash deployment/scripts/travis-deploy-fast.sh <staging|uat|production>

set -euo pipefail

ENV="${1:?Usage: travis-deploy-fast.sh <staging|uat|production>}"

case "$ENV" in
  staging)
    SSH_KEY_VAR="STAGING_SSH_KEY_BASE64"
    SERVER_IP="129.213.82.37"
    SERVER_USER="ubuntu"
    DEPLOY_DIR="/home/ubuntu/taskai-staging"
    COMPOSE_PROJECT="taskai-staging"
    DOMAIN="staging.taskai.cc"
    MCP_DOMAIN="mcp.staging.taskai.cc"
    APP_URL="https://staging.taskai.cc"
    DB_DSN_VAR="STAGING_DB_DSN"
    JWT_SECRET_VAR="STAGING_JWT_SECRET"
    LOGIN_GH_ID_VAR="STAGING_LOGIN_GITHUB_CLIENT_ID"
    LOGIN_GH_SECRET_VAR="STAGING_LOGIN_GITHUB_CLIENT_SECRET"
    OAUTH_STATE_VAR="STAGING_OAUTH_STATE_SECRET"
    GOOGLE_ID_VAR="STAGING_GOOGLE_CLIENT_ID"
    GOOGLE_SECRET_VAR="STAGING_GOOGLE_CLIENT_SECRET"
    DD_PROFILING="true"
    EXTRA_PORTS=""
    ;;
  uat)
    SSH_KEY_VAR="UAT_SSH_KEY_BASE64"
    SERVER_IP="92.4.83.28"
    SERVER_USER="ubuntu"
    DEPLOY_DIR="/home/ubuntu/taskai-uat"
    COMPOSE_PROJECT=""
    DOMAIN="uat.taskai.cc"
    MCP_DOMAIN=""
    APP_URL="https://uat.taskai.cc"
    DB_DSN_VAR="UAT_DB_DSN"
    JWT_SECRET_VAR="UAT_JWT_SECRET"
    LOGIN_GH_ID_VAR="UAT_LOGIN_GITHUB_CLIENT_ID"
    LOGIN_GH_SECRET_VAR="UAT_LOGIN_GITHUB_CLIENT_SECRET"
    OAUTH_STATE_VAR="UAT_OAUTH_STATE_SECRET"
    GOOGLE_ID_VAR="UAT_GOOGLE_CLIENT_ID"
    GOOGLE_SECRET_VAR="UAT_GOOGLE_CLIENT_SECRET"
    DD_PROFILING="false"
    EXTRA_PORTS="TASKAI_API_PORT=38888 TASKAI_WEB_PORT=33333"
    ;;
  production)
    SSH_KEY_VAR="PRODUCTION_SSH_KEY_BASE64"
    SERVER_IP="31.97.102.48"
    SERVER_USER="ubuntu"
    DEPLOY_DIR="/home/ubuntu/taskai"
    COMPOSE_PROJECT="taskai"
    DOMAIN="taskai.cc"
    MCP_DOMAIN="mcp.taskai.cc"
    APP_URL="https://taskai.cc"
    DB_DSN_VAR="PRODUCTION_DB_DSN"
    JWT_SECRET_VAR="PRODUCTION_JWT_SECRET"
    LOGIN_GH_ID_VAR="PRODUCTION_LOGIN_GITHUB_CLIENT_ID"
    LOGIN_GH_SECRET_VAR="PRODUCTION_LOGIN_GITHUB_CLIENT_SECRET"
    OAUTH_STATE_VAR="PRODUCTION_OAUTH_STATE_SECRET"
    GOOGLE_ID_VAR="PRODUCTION_GOOGLE_CLIENT_ID"
    GOOGLE_SECRET_VAR="PRODUCTION_GOOGLE_CLIENT_SECRET"
    DD_PROFILING="true"
    EXTRA_PORTS=""
    ;;
  *)
    echo "ERROR: Unknown environment: $ENV"
    exit 1
    ;;
esac

# Resolve indirect variable references
DB_DSN="${!DB_DSN_VAR:-}"
JWT_SECRET="${!JWT_SECRET_VAR:-}"
LOGIN_GH_ID="${!LOGIN_GH_ID_VAR:-}"
LOGIN_GH_SECRET="${!LOGIN_GH_SECRET_VAR:-}"
OAUTH_STATE="${!OAUTH_STATE_VAR:-}"
GOOGLE_ID="${!GOOGLE_ID_VAR:-}"
GOOGLE_SECRET="${!GOOGLE_SECRET_VAR:-}"

VERSION=$(cat VERSION 2>/dev/null || echo "dev")
GIT_COMMIT=$(git rev-parse HEAD)
GIT_SHORT=$(git rev-parse --short HEAD)
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo "=== TaskAI Fast Deploy ==="
echo "Environment: $ENV"
echo "Server:      $SERVER_USER@$SERVER_IP"
echo "Version:     $VERSION ($GIT_SHORT)"
echo "Strategy:    Build on server (native arch, no registry)"
echo ""

# SSH setup
SSH_KEY_VALUE="${!SSH_KEY_VAR:-}"
if [ -z "$SSH_KEY_VALUE" ]; then
  echo "ERROR: $SSH_KEY_VAR is not set"
  exit 1
fi

mkdir -p ~/.ssh
if echo "$SSH_KEY_VALUE" | head -1 | grep -q "BEGIN"; then
  # Raw PEM key (GitHub Actions)
  echo "$SSH_KEY_VALUE" > ~/.ssh/deploy_key
else
  # Base64-encoded key (Travis CI)
  echo "$SSH_KEY_VALUE" | base64 --decode > ~/.ssh/deploy_key
fi
chmod 600 ~/.ssh/deploy_key
ssh-keyscan "$SERVER_IP" >> ~/.ssh/known_hosts 2>/dev/null

SSH_CMD="ssh -o StrictHostKeyChecking=no -i ~/.ssh/deploy_key $SERVER_USER@$SERVER_IP"
SCP_CMD="scp -o StrictHostKeyChecking=no -i ~/.ssh/deploy_key"

# Determine compose command based on environment
if [ "$ENV" = "uat" ]; then
  COMPOSE_CMD="docker compose"
  SOURCE_DIR="$DEPLOY_DIR"
else
  COMPOSE_CMD="docker compose -f source/docker-compose.yml -p $COMPOSE_PROJECT"
  SOURCE_DIR="$DEPLOY_DIR/source"
fi

# --- Step 1: Sync source code to server ---
echo "=== Syncing source to server ==="

# Create target directory
$SSH_CMD "mkdir -p $SOURCE_DIR"

# Use git archive to create a clean tarball (no .git, respects .gitignore)
TARBALL="/tmp/taskai-deploy-$$.tar.gz"
git archive --format=tar HEAD | gzip > "$TARBALL"
$SCP_CMD "$TARBALL" "$SERVER_USER@$SERVER_IP:$TARBALL"

# Extract on server (delete tarball after extraction, then clean local copy)
$SSH_CMD "cd $SOURCE_DIR && tar xzf $TARBALL && rm -f $TARBALL"
rm -f "$TARBALL"
echo "Source synced"

# --- Step 2: Copy deployment scripts and run nginx setup ---
echo "=== Setting up nginx routes ==="
$SSH_CMD "chmod +x $SOURCE_DIR/deployment/scripts/ensure-draw-route.sh 2>/dev/null; sudo $SOURCE_DIR/deployment/scripts/ensure-draw-route.sh $DOMAIN 2>/dev/null || true"
$SSH_CMD "chmod +x $SOURCE_DIR/deployment/scripts/ensure-zero-downtime.sh 2>/dev/null; sudo $SOURCE_DIR/deployment/scripts/ensure-zero-downtime.sh $DOMAIN 2>/dev/null || true"

if [ -n "$MCP_DOMAIN" ]; then
  $SSH_CMD "chmod +x $SOURCE_DIR/deployment/scripts/ensure-mcp-agent-header.sh 2>/dev/null; sudo $SOURCE_DIR/deployment/scripts/ensure-mcp-agent-header.sh $MCP_DOMAIN 2>/dev/null || true"
fi

# --- Step 3: Build and deploy on server ---
echo "=== Building images on server (native arch) ==="

$SSH_CMD "bash -s" <<REMOTE_EOF
  set -e

  cd $DEPLOY_DIR

  export VERSION='$VERSION'
  export GIT_COMMIT='$GIT_SHORT'
  export BUILD_TIME='$BUILD_TIME'
  export DB_DRIVER='postgres'
  export DB_DSN='$DB_DSN'
  export JWT_SECRET='$JWT_SECRET'
  export APP_URL='$APP_URL'
  export OAUTH_SUCCESS_URL='${APP_URL}/oauth/callback'
  export OAUTH_ERROR_URL='${APP_URL}/login'
  export ENV='$ENV'

  # OAuth/Login secrets
  if [ -n '$LOGIN_GH_ID' ] && [ '$LOGIN_GH_ID' != '-' ]; then
    export LOGIN_GITHUB_CLIENT_ID='$LOGIN_GH_ID'
    export LOGIN_GITHUB_CLIENT_SECRET='$LOGIN_GH_SECRET'
  fi
  if [ -n '$OAUTH_STATE' ]; then
    export OAUTH_STATE_SECRET='$OAUTH_STATE'
  fi
  if [ -n '$GOOGLE_ID' ]; then
    export GOOGLE_CLIENT_ID='$GOOGLE_ID'
    export GOOGLE_CLIENT_SECRET='$GOOGLE_SECRET'
  fi

  # Datadog
  if [ -n '${DD_API_KEY:-}' ]; then
    export DD_API_KEY='${DD_API_KEY:-}'
    export DD_SITE='${DD_SITE:-datadoghq.com}'
    export APM_ENABLED='true'
    export DD_PROFILING_ENABLED='$DD_PROFILING'
  fi

  # UAT custom ports
  $( [ -n "$EXTRA_PORTS" ] && echo "export $EXTRA_PORTS" || echo "true" )

  # Disk space management
  DISK_USAGE=\$(df / | tail -1 | awk '{print \$5}' | tr -d '%')
  if [ "\$DISK_USAGE" -gt 80 ]; then
    echo "Disk usage \${DISK_USAGE}% — pruning..."
    docker builder prune -f --filter "until=48h" || true
    docker image prune -f || true
  fi

  # Build images natively on this server (no cross-compilation, no registry)
  echo "Building Docker images..."
  $COMPOSE_CMD build --parallel --build-arg VERSION='$VERSION' --build-arg GIT_COMMIT='$GIT_SHORT' --build-arg BUILD_TIME='$BUILD_TIME'

  # Deploy with force-recreate
  echo "Deploying..."
  $COMPOSE_CMD up -d --force-recreate --remove-orphans

  # Prune old images
  docker image prune -f || true

  sleep 10
  $COMPOSE_CMD ps
REMOTE_EOF

# --- Step 4: Verify deployment ---
echo ""
echo "=== Verifying deployment ==="
sleep 5

echo "Health check: https://$DOMAIN/api/health"
if curl -sf "https://$DOMAIN/api/health" 2>/dev/null; then
  echo ""
  echo "Health check passed"
else
  echo "WARNING: Health check failed (may be behind Cloudflare Access or still starting)"
fi

echo ""
echo "=== Fast deploy to $ENV complete ==="

rm -f ~/.ssh/deploy_key
