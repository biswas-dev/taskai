---
sidebar_position: 2
---

# Create Your First Project

This guide walks you through creating a project and managing tasks in TaskAI.

## 1. Create an Account

Sign up at the login page or via the API:

```bash
curl -X POST http://localhost:8080/api/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "you@example.com",
    "password": "SecurePass123!"
  }'
```

Save the `token` from the response — you'll use it for authenticated requests.

## 2. Create a Project

```bash
curl -X POST http://localhost:8080/api/projects \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My First Project",
    "description": "Getting started with TaskAI"
  }'
```

Every project automatically gets default swim lanes: **To Do**, **In Progress**, and **Done**.

## 3. Add Tasks

```bash
curl -X POST http://localhost:8080/api/projects/1/tasks \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Set up development environment",
    "description": "Install dependencies and verify builds",
    "priority": "high"
  }'
```

## 4. Move Tasks Across the Board

Update a task's status or swim lane:

```bash
curl -X PATCH http://localhost:8080/api/tasks/1 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "in_progress"}'
```

## 5. Invite Team Members

Add collaborators to your project:

```bash
curl -X POST http://localhost:8080/api/projects/1/members \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "teammate@example.com",
    "role": "member"
  }'
```

## Using the Web UI

The web interface provides a Kanban board view where you can:

1. **Drag and drop** tasks between swim lanes
2. **Click tasks** to view details, add comments, and change priority
3. **Use the sidebar** to navigate between projects
4. **Search tasks** with the search bar (supports full-text search)

## Next Steps

- [Configure your environment](/getting-started/configuration)
- [Set up GitHub integration](/guides/github-sync)
- [Connect an AI agent via MCP](/mcp/overview)
