---
sidebar_position: 1
---

# Architecture Overview

TaskAI is a multi-service application deployed as Docker containers behind Nginx.

## System Diagram

```
                    ┌─────────────┐
                    │   Browser   │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │    Nginx    │ :80/443
                    │ (web container)
                    └──┬───┬───┬──┘
                       │   │   │
          ┌────────────┘   │   └────────────┐
          │                │                │
   ┌──────▼──────┐ ┌──────▼──────┐ ┌───────▼──────┐
   │  React SPA  │ │   Go API    │ │  MCP Server  │
   │  (static)   │ │   :8080     │ │   :3000      │
   └─────────────┘ └──────┬──────┘ └───────┬──────┘
                          │                │
                   ┌──────▼──────┐         │
                   │ PostgreSQL  │◄────────┘
                   │   :5432     │  (via API)
                   └─────────────┘

   ┌─────────────┐ ┌─────────────┐
   │ Yjs Processor│ │ OTEL/Datadog│
   │   :3001      │ │  Collector  │
   └─────────────┘ └─────────────┘
```

## Service Roles

| Service | Technology | Purpose |
|---------|-----------|---------|
| **web** | Nginx + React | Serves SPA, proxies `/api/` and `/draw/` to backend |
| **api** | Go + Chi | REST API, business logic, auth, GitHub sync |
| **postgres** | PostgreSQL 16 | Primary data store |
| **mcp** | Node.js + TypeScript | MCP protocol server for AI agents |
| **yjs-processor** | Node.js | Real-time wiki collaboration via Yjs |
| **otel-collector** | OpenTelemetry | Trace collection and export |
| **dd-agent** | Datadog Agent | Logs, metrics, APM |

## Request Flow

1. **Browser** → Nginx (port 80/443)
2. Nginx routes:
   - `/api/*` → Go API (port 8080)
   - `/draw/*` → Go API (canvas editor)
   - `/*` → React SPA (static files with SPA fallback)
3. **MCP clients** → MCP server (port 8089) → Go API (internal)

## Authentication Flow

```
Client → POST /api/auth/login → JWT token
Client → GET /api/projects (Authorization: Bearer <token>) → Response
```

Two auth methods:
- **JWT Bearer Token** — from signup/login, stored in browser localStorage
- **API Key** — for programmatic access (scripts, MCP server)

## Data Flow

All data flows through the Go API. The MCP server and frontend both call the same REST endpoints. PostgreSQL is the single source of truth.

## Networking

All services communicate over a Docker bridge network (`taskai`). Only the web container (Nginx) is exposed to the host. The API, PostgreSQL, and MCP server are internal.
