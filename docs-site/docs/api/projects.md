---
sidebar_position: 2
---

# Projects

Projects are the top-level container for tasks, swim lanes, sprints, and wiki pages.

## List Projects

```bash
curl https://taskai.cc/api/projects?page=1&limit=10 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "name": "TaskAI Development",
    "description": "Building the next-gen PM tool",
    "user_id": 1,
    "created_at": "2025-10-18T00:00:00Z",
    "updated_at": "2025-10-18T00:00:00Z"
  }
]
```

Pagination headers: `X-Total-Count`, `X-Page`, `X-Per-Page`, `X-Total-Pages`.

## Get Project

```bash
curl https://taskai.cc/api/projects/1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Create Project

```bash
curl -X POST https://taskai.cc/api/projects \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Mobile App",
    "description": "iOS and Android development"
  }'
```

**Response (201):** Returns the created project with default swim lanes.

## Update Project

```bash
curl -X PATCH https://taskai.cc/api/projects/1 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Mobile App v2"}'
```

## Delete Project

```bash
curl -X DELETE https://taskai.cc/api/projects/1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (204):** No content. Deletes all associated tasks, swim lanes, and wiki pages.
