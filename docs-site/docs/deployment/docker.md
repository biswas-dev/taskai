---
sidebar_position: 1
---

# Docker Setup

TaskAI runs as a multi-container application using Docker Compose.

## Services

| Service | Image | Port | Description |
|---------|-------|------|-------------|
| `web` | Nginx + React | 8084 → 80 | Frontend + reverse proxy |
| `api` | Go binary | 8083 → 8080 | Backend API |
| `postgres` | PostgreSQL 16 | 5432 | Database |
| `mcp` | Node.js | 8089 → 3000 | MCP server |
| `yjs-processor` | Node.js | 3003 → 3001 | Wiki collaboration |
| `otel-collector` | OTEL Contrib | — | Trace collection |
| `dd-agent` | Datadog Agent | — | Monitoring |

## Quick Start

```bash
# Clone and start
git clone https://github.com/anchoo2kewl/taskai.git
cd taskai
docker compose up --build
```

## Building Individual Services

```bash
# Rebuild just the API
docker compose build api
docker compose up -d api

# Rebuild just the web
docker compose build web
docker compose up -d web
```

## Volumes

| Volume | Purpose |
|--------|---------|
| `taskai-data` | Application data |
| `postgres-data` | PostgreSQL data directory |

## Health Checks

All services have health checks configured:

```bash
# Check service status
docker compose ps

# View health of specific service
docker inspect --format='{{.State.Health.Status}}' taskai-api-1
```

## Environment Variables

Pass environment variables via a `.env` file in the project root:

```bash
# .env
JWT_SECRET=your-secret
POSTGRES_PASSWORD=your-db-password
```

Docker Compose automatically loads `.env` from the project root.

## Resource Limits

For production, consider adding resource limits:

```yaml
services:
  api:
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: '1.0'
```

## Logs

```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f api

# Last 100 lines
docker compose logs --tail=100 api
```
