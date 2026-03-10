---
slug: /
sidebar_position: 1
---

# Welcome to TaskAI

TaskAI is an **AI-native project management system** built for teams that work with AI tools. It combines traditional project management with first-class support for AI agents via the [Model Context Protocol (MCP)](/mcp/overview).

## Key Features

- **Task Management** — Create projects, organize tasks with swim lanes, sprints, and priorities
- **Team Collaboration** — Invite members, assign tasks, comment threads
- **Wiki** — Rich markdown wiki with real-time collaborative editing
- **GitHub Integration** — Two-way sync between TaskAI tasks and GitHub issues
- **MCP Server** — Let AI agents (Claude, Cursor, etc.) read and manage your projects
- **Canvas Drawings** — Built-in diagramming tool for architecture docs and wireframes
- **API Keys** — Programmatic access for scripts and integrations
- **2FA Support** — TOTP-based two-factor authentication

## Quick Start

```bash
# Clone and start with Docker
git clone https://github.com/anchoo2kewl/taskai.git
cd taskai
docker compose up --build
```

Then open [http://localhost:8084](http://localhost:8084) to access TaskAI.

## Architecture at a Glance

| Component | Technology |
|-----------|-----------|
| Backend API | Go + Chi router |
| Database | PostgreSQL |
| Frontend | React + TypeScript + Vite |
| MCP Server | Node.js + TypeScript |
| Deployment | Docker Compose + Nginx |

## Next Steps

- [Installation Guide](/getting-started/installation) — Set up TaskAI locally
- [API Reference](/api/authentication) — Integrate with the REST API
- [MCP Integration](/mcp/overview) — Connect AI agents to TaskAI
