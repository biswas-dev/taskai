---
sidebar_position: 3
---

# Configuration

TaskAI is configured via environment variables. Set these in a `.env` file or pass them directly to Docker Compose.

## Core Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `ENV` | `production` | Environment name (`development`, `staging`, `production`) |
| `PORT` | `8080` | API server port |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `JWT_SECRET` | (required) | Secret key for signing JWT tokens |
| `CORS_ALLOWED_ORIGINS` | — | Comma-separated list of allowed origins |

## Database

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_DRIVER` | `postgres` | Database driver (`postgres`) |
| `DB_DSN` | — | Full database connection string |
| `POSTGRES_HOST` | `postgres` | PostgreSQL host |
| `POSTGRES_PORT` | `5432` | PostgreSQL port |
| `POSTGRES_DB` | `taskai` | Database name |
| `POSTGRES_USER` | `taskai` | Database user |
| `POSTGRES_PASSWORD` | — | Database password |

## OAuth Login

| Variable | Default | Description |
|----------|---------|-------------|
| `GOOGLE_CLIENT_ID` | — | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | — | Google OAuth client secret |
| `LOGIN_GITHUB_CLIENT_ID` | — | GitHub OAuth client ID (for login) |
| `LOGIN_GITHUB_CLIENT_SECRET` | — | GitHub OAuth client secret (for login) |
| `OAUTH_STATE_SECRET` | — | Secret for CSRF-proof OAuth state JWT |
| `OAUTH_SUCCESS_URL` | — | Redirect URL after successful OAuth login |
| `OAUTH_ERROR_URL` | — | Redirect URL on OAuth error |

## GitHub Integration

| Variable | Default | Description |
|----------|---------|-------------|
| `GITHUB_CLIENT_ID` | — | GitHub OAuth app client ID (for repo sync) |
| `GITHUB_CLIENT_SECRET` | — | GitHub OAuth app client secret |
| `APP_URL` | `http://localhost:5173` | Application URL for OAuth callbacks |

## Observability

| Variable | Default | Description |
|----------|---------|-------------|
| `APM_ENABLED` | `false` | Enable OpenTelemetry tracing |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `otel-collector:4317` | OTLP exporter endpoint |
| `DD_API_KEY` | — | Datadog API key |
| `DD_SITE` | `datadoghq.com` | Datadog site |
| `DD_PROFILING_ENABLED` | `false` | Enable continuous profiling |

## Docker Compose Ports

| Variable | Default | Description |
|----------|---------|-------------|
| `TASKAI_API_PORT` | `8083` | Host port for the API |
| `TASKAI_WEB_PORT` | `8084` | Host port for the web UI |

## Example `.env` File

```bash
# Minimal production config
JWT_SECRET=change-me-to-a-random-64-char-string
POSTGRES_PASSWORD=strong-database-password
CORS_ALLOWED_ORIGINS=https://yourdomain.com
ENV=production

# Optional: GitHub OAuth for social login
LOGIN_GITHUB_CLIENT_ID=your-github-client-id
LOGIN_GITHUB_CLIENT_SECRET=your-github-secret

# Optional: GitHub integration for issue sync
GITHUB_CLIENT_ID=your-github-app-id
GITHUB_CLIENT_SECRET=your-github-app-secret
```
