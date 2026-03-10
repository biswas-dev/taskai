---
sidebar_position: 3
---

# Environments

TaskAI uses a three-tier deployment pipeline: Staging → UAT → Production.

## Environment Overview

| Environment | URL | Purpose |
|-------------|-----|---------|
| **Staging** | `staging.taskai.cc` | Development testing, auto-deploys from `main` |
| **UAT** | `uat.taskai.cc` | User acceptance testing |
| **Production** | `taskai.cc` | Live production |

## Deployment Pipeline

```
main branch → Staging (auto)
           → UAT (manual promote)
           → Production (manual promote)
```

1. **Staging** auto-deploys when code is pushed to `main` via GitHub Actions
2. **UAT** is promoted by pushing `main` to the `uat` branch
3. **Production** is promoted via the `deploy-production.yml` GitHub Action

## MCP Server URLs

Each environment has its own MCP server:

| Environment | MCP URL |
|-------------|---------|
| Staging | `mcp.staging.taskai.cc` |
| Production | `mcp.taskai.cc` |

## Database Migration

Copy databases between environments:

```bash
./script/server db-migrate prod staging
```

## Version Management

Versions are managed with semantic versioning:

```bash
./script/bump-version.sh patch  # 0.1.0 → 0.1.1
./script/bump-version.sh minor  # 0.1.0 → 0.2.0
./script/bump-version.sh major  # 0.1.0 → 1.0.0
```

## Health Checks

Verify deployment health:

```bash
curl https://staging.taskai.cc/api/health
curl https://taskai.cc/api/health
```
