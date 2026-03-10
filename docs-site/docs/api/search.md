---
sidebar_position: 9
---

# Search

TaskAI provides a global search endpoint that searches across tasks, wiki pages, and other entities.

## Global Search

```bash
curl "https://taskai.cc/api/search?q=authentication&project_id=1" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
{
  "tasks": [
    {
      "id": 1,
      "title": "Implement JWT authentication",
      "status": "done",
      "project_id": 1
    }
  ],
  "wiki_pages": [
    {
      "id": 3,
      "title": "Authentication Architecture",
      "project_id": 1
    }
  ]
}
```

## Task Search

Search within a specific project's tasks:

```bash
curl "https://taskai.cc/api/projects/1/tasks?query=bug+fix" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

The search query matches against task titles and descriptions using full-text search.

## Wiki Search

Search wiki content across all projects or within a specific project:

```bash
curl "https://taskai.cc/api/wiki/search?q=deployment&project_id=1" \
  -H "Authorization: Bearer YOUR_TOKEN"
```
