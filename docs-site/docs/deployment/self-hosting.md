---
sidebar_position: 2
---

# Self-Hosting Guide

Deploy TaskAI on your own server with Docker Compose.

## Requirements

- Linux server (Ubuntu 22.04+ recommended)
- Docker and Docker Compose installed
- Domain name with DNS configured
- Minimum 2GB RAM, 10GB disk

## Step 1: Clone the Repository

```bash
ssh your-server
git clone https://github.com/anchoo2kewl/taskai.git
cd taskai
```

## Step 2: Configure Environment

Create a `.env` file:

```bash
# Required
JWT_SECRET=$(openssl rand -hex 32)
POSTGRES_PASSWORD=$(openssl rand -hex 16)
ENV=production

# Your domain
CORS_ALLOWED_ORIGINS=https://yourdomain.com
APP_URL=https://yourdomain.com

# Optional: OAuth
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
LOGIN_GITHUB_CLIENT_ID=your-github-client-id
LOGIN_GITHUB_CLIENT_SECRET=your-github-client-secret
OAUTH_STATE_SECRET=$(openssl rand -hex 32)
OAUTH_SUCCESS_URL=https://yourdomain.com/oauth/callback
OAUTH_ERROR_URL=https://yourdomain.com/login?error=oauth
```

## Step 3: Start Services

```bash
docker compose up -d --build
```

## Step 4: Set Up Reverse Proxy

Use Nginx or Caddy on the host to terminate TLS and proxy to the web container:

### Caddy (Simplest)

```
yourdomain.com {
    reverse_proxy localhost:8084
}

mcp.yourdomain.com {
    reverse_proxy localhost:8089
}
```

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://localhost:8084;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Step 5: Verify

```bash
# Check all services are healthy
docker compose ps

# Test the API
curl https://yourdomain.com/api/health
```

## Backups

Back up the PostgreSQL database regularly:

```bash
# Dump database
docker compose exec postgres pg_dump -U taskai taskai > backup.sql

# Restore
docker compose exec -T postgres psql -U taskai taskai < backup.sql
```

## Updates

```bash
cd taskai
git pull
docker compose up -d --build
```

Docker Compose performs rolling updates — services restart one at a time with health checks ensuring availability.

## Monitoring

For production monitoring, configure Datadog by setting `DD_API_KEY` in your `.env` file. This enables:
- Application Performance Monitoring (APM)
- Container metrics
- Log collection
- PostgreSQL query metrics
