#!/bin/bash
# Travis CI deployment script for TaskAI
# Usage: bash deployment/scripts/travis-deploy.sh <environment>

set -euo pipefail

ENV="${1:?Usage: travis-deploy.sh <staging|uat|production>}"

case "$ENV" in
  staging)
    SSH_KEY_VAR="STAGING_SSH_KEY_BASE64"
    SERVER_IP="129.213.82.37"
    SERVER_USER="ubuntu"
    DEPLOY_DIR="/home/ubuntu/taskai-staging/source"
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
    DEPLOY_DIR="/home/ubuntu/taskai/source"
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

echo "=== TaskAI Travis CI Deploy ==="
echo "Environment: $ENV"
echo "Server:      $SERVER_USER@$SERVER_IP"
echo "Version:     $VERSION"
echo "Commit:      $GIT_SHORT"
echo ""

# SSH setup
SSH_KEY_VALUE="${!SSH_KEY_VAR:-}"
if [ -z "$SSH_KEY_VALUE" ]; then
  echo "ERROR: $SSH_KEY_VAR is not set"
  exit 1
fi

mkdir -p ~/.ssh
echo "$SSH_KEY_VALUE" | base64 --decode > ~/.ssh/deploy_key
chmod 600 ~/.ssh/deploy_key
ssh-keyscan "$SERVER_IP" >> ~/.ssh/known_hosts 2>/dev/null

SSH_CMD="ssh -o StrictHostKeyChecking=no -i ~/.ssh/deploy_key $SERVER_USER@$SERVER_IP"

# Determine compose command based on environment
if [ "$ENV" = "uat" ]; then
  # UAT: repo is directly in deploy dir, no compose project name
  COMPOSE_CMD="docker compose"
  GIT_DIR="$DEPLOY_DIR"
  COMPOSE_DIR="$DEPLOY_DIR"
else
  # Staging/Production: source is in a subdirectory, use compose project name
  COMPOSE_CMD="docker compose -f source/docker-compose.yml -p $COMPOSE_PROJECT"
  GIT_DIR="$DEPLOY_DIR"
  COMPOSE_DIR="$(dirname "$DEPLOY_DIR")"
fi

$SSH_CMD "bash -s" <<REMOTE_EOF
  set -e
  ssh-keyscan github.com >> ~/.ssh/known_hosts 2>/dev/null || true

  cd $GIT_DIR

  # Ensure SSH remote (may have been cloned via HTTPS)
  git remote set-url origin git@github.com:anchoo2kewl/taskai.git
  git fetch origin
  git reset --hard $GIT_COMMIT

  cd $COMPOSE_DIR

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

  # Login to Docker Hub
  echo '${DOCKERHUB_TOKEN}' | docker login -u '${DOCKERHUB_USERNAME}' --password-stdin

  # Nginx scripts
  chmod +x $GIT_DIR/deployment/scripts/ensure-draw-route.sh 2>/dev/null || true
  sudo $GIT_DIR/deployment/scripts/ensure-draw-route.sh $DOMAIN || true

  chmod +x $GIT_DIR/deployment/scripts/ensure-zero-downtime.sh 2>/dev/null || true
  sudo $GIT_DIR/deployment/scripts/ensure-zero-downtime.sh $DOMAIN || true

  if [ -n '$MCP_DOMAIN' ]; then
    chmod +x $GIT_DIR/deployment/scripts/ensure-mcp-agent-header.sh 2>/dev/null || true
    sudo $GIT_DIR/deployment/scripts/ensure-mcp-agent-header.sh $MCP_DOMAIN || true
  fi

  # Build and deploy
  if [ '$ENV' = 'uat' ]; then
    # UAT: simple down/up (shared server, no zero-downtime needed)
    $COMPOSE_CMD down || true
    docker builder prune -f || true
    docker image prune -f || true
    $COMPOSE_CMD build --build-arg VERSION="\$VERSION" --build-arg GIT_COMMIT="\$GIT_COMMIT" --build-arg BUILD_TIME="\$BUILD_TIME"
    $COMPOSE_CMD up -d --force-recreate --remove-orphans
  else
    # Staging/Production: zero-downtime (build first, then recreate)
    $COMPOSE_CMD build --build-arg VERSION="\$VERSION" --build-arg GIT_COMMIT="\$GIT_COMMIT" --build-arg BUILD_TIME="\$BUILD_TIME"
    $COMPOSE_CMD up -d --force-recreate --remove-orphans
    docker image prune -f || true
  fi

  sleep 15
  $COMPOSE_CMD ps
REMOTE_EOF

# Verify deployment
echo ""
echo "=== Verifying deployment ==="
sleep 10

echo "Health check: https://$DOMAIN/api/health"
if curl -sf "https://$DOMAIN/api/health" 2>/dev/null; then
  echo ""
  echo "Health check passed"
else
  echo "WARNING: Health check failed (server may still be starting, or behind Cloudflare Access)"
fi

echo ""
echo "=== Deploy to $ENV complete ==="

rm -f ~/.ssh/deploy_key
