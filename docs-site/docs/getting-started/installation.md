---
sidebar_position: 1
---

# Installation

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose
- [Git](https://git-scm.com/)

For local development without Docker:
- [Go 1.24+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/)
- [PostgreSQL 16+](https://www.postgresql.org/)

## Docker Setup (Recommended)

```bash
# Clone the repository
git clone https://github.com/anchoo2kewl/taskai.git
cd taskai

# Start all services
docker compose up --build
```

This starts:
- **API** on port 8083 (Go backend + PostgreSQL)
- **Web** on port 8084 (React SPA via Nginx)
- **MCP** on port 8089 (Model Context Protocol server)
- **PostgreSQL** on port 5432

Open [http://localhost:8084](http://localhost:8084) to access the app.

## Environment Configuration

Create a `.env` file in the project root to customize settings:

```bash
# Required
JWT_SECRET=your-secret-key-here

# Database (defaults work for Docker)
POSTGRES_DB=taskai
POSTGRES_USER=taskai
POSTGRES_PASSWORD=taskai-dev-password

# Optional
ENV=development
LOG_LEVEL=debug
CORS_ALLOWED_ORIGINS=http://localhost:5173
```

See [Configuration](/getting-started/configuration) for all available environment variables.

## Local Development (Without Docker)

### Backend

```bash
cd api

# Install Go dependencies
go mod download

# Run the API server
go run cmd/api/main.go
```

The API starts on port 8080 by default.

### Frontend

```bash
cd web

# Install dependencies
npm install

# Start dev server
npm run dev
```

The dev server starts on port 5173 with hot module replacement and API proxying.

## Verify Installation

```bash
# Check API health
curl http://localhost:8080/api/health

# Expected response
{"status":"ok"}
```

## Next Steps

- [Create your first project](/getting-started/first-project)
- [Configure environment variables](/getting-started/configuration)
