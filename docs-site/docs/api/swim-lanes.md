---
sidebar_position: 5
---

# Swim Lanes

Swim lanes are the columns on your Kanban board. Each project can have 2–6 swim lanes, and each lane has a `status_category` that determines the task status (`todo`, `in_progress`, or `done`).

## List Swim Lanes

```bash
curl https://taskai.cc/api/projects/1/swim-lanes \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "name": "To Do",
    "position": 0,
    "status_category": "todo",
    "project_id": 1
  },
  {
    "id": 2,
    "name": "In Progress",
    "position": 1,
    "status_category": "in_progress",
    "project_id": 1
  },
  {
    "id": 3,
    "name": "Done",
    "position": 2,
    "status_category": "done",
    "project_id": 1
  }
]
```

## Create Swim Lane

```bash
curl -X POST https://taskai.cc/api/projects/1/swim-lanes \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "In Review",
    "position": 2,
    "status_category": "in_progress"
  }'
```

Maximum 6 swim lanes per project.

## Update Swim Lane

```bash
curl -X PATCH https://taskai.cc/api/swim-lanes/2 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Working On"}'
```

## Delete Swim Lane

```bash
curl -X DELETE https://taskai.cc/api/swim-lanes/4 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Minimum 2 swim lanes must remain. Tasks in the deleted lane are moved to the first lane.
