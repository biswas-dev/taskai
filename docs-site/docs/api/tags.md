---
sidebar_position: 7
---

# Tags

Tags allow categorizing and labeling tasks across projects.

## List Tags

```bash
curl https://taskai.cc/api/tags?project_id=1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "name": "bug",
    "color": "#ef4444",
    "project_id": 1
  },
  {
    "id": 2,
    "name": "feature",
    "color": "#22c55e",
    "project_id": 1
  }
]
```

## Create Tag

```bash
curl -X POST https://taskai.cc/api/tags \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "documentation",
    "color": "#3b82f6",
    "project_id": 1
  }'
```

## Update Tag

```bash
curl -X PATCH https://taskai.cc/api/tags/1 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"color": "#dc2626"}'
```

## Delete Tag

```bash
curl -X DELETE https://taskai.cc/api/tags/1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```
