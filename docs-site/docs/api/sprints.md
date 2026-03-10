---
sidebar_position: 6
---

# Sprints

Sprints help organize work into time-boxed iterations.

## List Sprints

```bash
curl https://taskai.cc/api/sprints?project_id=1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "name": "Sprint 1",
    "start_date": "2025-10-14",
    "end_date": "2025-10-28",
    "status": "active",
    "project_id": 1
  }
]
```

## Create Sprint

```bash
curl -X POST https://taskai.cc/api/sprints \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Sprint 2",
    "start_date": "2025-10-28",
    "end_date": "2025-11-11",
    "project_id": 1
  }'
```

## Update Sprint

```bash
curl -X PATCH https://taskai.cc/api/sprints/1 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "completed"}'
```

## Delete Sprint

```bash
curl -X DELETE https://taskai.cc/api/sprints/1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Tasks assigned to a deleted sprint remain but lose their sprint association.
