---
sidebar_position: 2
---

# Create Your First Project

This guide walks you through creating a project and managing tasks in TaskAI.

## 1. Create a Project

From the dashboard, click **New Project**. Enter a name and optional description.

Every project automatically gets default swim lanes: **To Do**, **In Progress**, and **Done**. You can customize these later in project settings.

### Via API

```bash
curl -X POST https://taskai.cc/api/projects \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My First Project",
    "description": "Getting started with TaskAI"
  }'
```

## 2. Add Tasks

Click **Add Task** on any swim lane, or use the API:

```bash
curl -X POST https://taskai.cc/api/projects/1/tasks \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Set up development environment",
    "description": "Install dependencies and verify builds",
    "priority": "high"
  }'
```

## 3. Move Tasks Across the Board

Drag and drop tasks between swim lanes in the web UI, or update via API:

```bash
curl -X PATCH https://taskai.cc/api/tasks/1 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "in_progress"}'
```

## 4. Invite Team Members

Go to **Project Settings** → **Members** and invite collaborators by email:

```bash
curl -X POST https://taskai.cc/api/projects/1/members \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "teammate@example.com",
    "role": "member"
  }'
```

## 5. Using the Web UI

The web interface provides a Kanban board view where you can:

- **Drag and drop** tasks between swim lanes
- **Click tasks** to view details, add comments, and change priority
- **Use the sidebar** to navigate between projects
- **Search tasks** with the search bar (supports full-text search)

## Next Steps

- [Set up GitHub integration](/guides/github-sync)
- [Connect an AI agent via MCP](/mcp/overview)
- [Create API keys](/guides/api-keys) for automation
