---
sidebar_position: 1
---

# GitHub Issue Sync

Sync TaskAI tasks with GitHub issues for bidirectional issue tracking.

## Overview

When enabled, GitHub sync:
- Creates GitHub issues when TaskAI tasks are created
- Updates GitHub issues when TaskAI tasks change
- Imports GitHub issues as TaskAI tasks
- Syncs comments bidirectionally
- Maps labels to TaskAI tags
- Maps open/closed state to task status

## Setup

### 1. Connect Your GitHub Account

In the web UI, go to **Settings** → **Integrations** → **Connect GitHub**.

This initiates an OAuth flow that grants TaskAI access to your repositories.

### 2. Link a Repository

In your project settings, go to **GitHub** and select a repository:

```bash
curl -X PATCH https://taskai.cc/api/projects/1/github \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "github_repo": "your-org/your-repo",
    "sync_enabled": true
  }'
```

## Sync Behavior

### Task → GitHub Issue

| TaskAI Field | GitHub Issue Field |
|-------------|-------------------|
| `title` | `title` |
| `description` | `body` |
| `status: done` | `state: closed` |
| `status: todo/in_progress` | `state: open` |
| `priority` | Label (e.g., `priority:high`) |
| Tags | Labels |
| Comments | Comments |

### GitHub Issue → Task

New issues in the linked repository are imported as tasks during sync. Existing issues are matched by `github_issue_number`.

## Manual Sync

Trigger a sync manually:

```bash
curl -X POST https://taskai.cc/api/projects/1/github/sync \
  -H "Authorization: Bearer YOUR_TOKEN"
```
