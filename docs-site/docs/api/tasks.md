---
sidebar_position: 3
---

# Tasks

Tasks belong to a project and can be organized using swim lanes, priorities, sprints, and tags.

## List Tasks

```bash
curl "https://taskai.cc/api/projects/1/tasks?page=1&limit=20" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Filter by Status

```bash
curl "https://taskai.cc/api/projects/1/tasks?status=in_progress" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Search Tasks

```bash
curl "https://taskai.cc/api/projects/1/tasks?query=authentication" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Get Task by Number

Each task has a project-scoped number (e.g., #1, #2):

```bash
curl https://taskai.cc/api/projects/1/tasks/42 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Create Task

```bash
curl -X POST https://taskai.cc/api/projects/1/tasks \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Implement user notifications",
    "description": "Add in-app and email notifications",
    "priority": "high",
    "status": "todo",
    "assigned_to": 2,
    "swim_lane_id": 1
  }'
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | Yes | Task title |
| `description` | string | No | Markdown description |
| `status` | string | No | `todo`, `in_progress`, `done` (default: `todo`) |
| `priority` | string | No | `low`, `medium`, `high`, `critical` (default: `medium`) |
| `assigned_to` | integer | No | User ID of assignee |
| `swim_lane_id` | integer | No | Swim lane ID |
| `sprint_id` | integer | No | Sprint ID |

## Update Task

```bash
curl -X PATCH https://taskai.cc/api/tasks/1 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "status": "in_progress",
    "priority": "critical"
  }'
```

All fields are optional — only include what you want to change.

## Delete Task

```bash
curl -X DELETE https://taskai.cc/api/tasks/1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (204):** No content.
