---
sidebar_position: 3
---

# Tech Stack

## Backend

| Technology | Purpose |
|-----------|---------|
| **Go 1.24+** | API server, business logic |
| **Chi** | HTTP router |
| **PostgreSQL 16** | Primary database |
| **bcrypt** | Password hashing |
| **JWT** | Authentication tokens |
| **Zap** | Structured logging (Uber's zap logger) |
| **OpenTelemetry** | Distributed tracing |

## Frontend

| Technology | Purpose |
|-----------|---------|
| **React 18** | UI framework |
| **TypeScript** | Type safety |
| **Vite** | Build tool and dev server |
| **TanStack Router** | File-based routing |
| **Tailwind CSS** | Styling |
| **Vitest** | Unit testing |
| **Playwright** | E2E testing |

## Infrastructure

| Technology | Purpose |
|-----------|---------|
| **Docker** | Containerization |
| **Docker Compose** | Multi-service orchestration |
| **Nginx** | Reverse proxy, static file serving |
| **GitHub Actions** | CI/CD pipeline |
| **Datadog** | Monitoring, APM, logs |
| **Cloudflare** | DNS, CDN, DDoS protection |
| **Cloudinary** | Image/file storage |

## MCP Server

| Technology | Purpose |
|-----------|---------|
| **Node.js** | Runtime |
| **TypeScript** | Type safety |
| **MCP SDK** | Model Context Protocol implementation |
| **SSE** | Server-Sent Events transport |

## Key Design Decisions

- **PostgreSQL over SQLite** — Chosen for production robustness, concurrent access, and full-text search
- **Go for the API** — Fast compilation, excellent concurrency, low memory footprint
- **Separate MCP server** — Isolates AI agent traffic from the main API, allows independent scaling
- **Nginx in the web container** — Single entry point handles static files, API proxy, and security headers
- **Yjs for wiki collaboration** — CRDT-based real-time editing without conflicts
