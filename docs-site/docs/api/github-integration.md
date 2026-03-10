---
sidebar_position: 12
---

# GitHub Integration

Sync TaskAI tasks with GitHub issues for two-way issue tracking.

## Get GitHub Settings

```bash
curl https://taskai.cc/api/projects/1/github \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
{
  "github_repo": "anchoo2kewl/taskai",
  "sync_enabled": true,
  "last_synced_at": "2025-10-20T12:00:00Z"
}
```

## Update GitHub Settings

```bash
curl -X PATCH https://taskai.cc/api/projects/1/github \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "github_repo": "org/repo",
    "sync_enabled": true
  }'
```

## How Sync Works

When enabled:
1. **TaskAI → GitHub:** Creating/updating tasks creates/updates GitHub issues
2. **GitHub → TaskAI:** GitHub webhook events create/update TaskAI tasks
3. **Comments** are synced bidirectionally
4. **Labels** map to TaskAI tags
5. **Status** changes (open/closed) map to task status

## OAuth Setup

GitHub sync requires OAuth authorization. The user must connect their GitHub account:

```
GET /api/auth/github?redirect=/app/projects/1/settings
```

This initiates the OAuth flow and stores the access token for API calls.

See [GitHub Sync Guide](/guides/github-sync) for detailed setup instructions.
