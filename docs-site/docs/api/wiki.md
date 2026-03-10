---
sidebar_position: 8
---

# Wiki

Each project has a wiki for documentation, meeting notes, and knowledge sharing. Wiki pages support markdown with real-time collaborative editing.

## List Wiki Pages

```bash
curl https://taskai.cc/api/projects/1/wiki \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "title": "Architecture Overview",
    "project_id": 1,
    "created_by": 1,
    "created_at": "2025-10-18T00:00:00Z",
    "updated_at": "2025-10-18T12:00:00Z"
  }
]
```

## Get Wiki Page

```bash
curl https://taskai.cc/api/wiki/1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Returns the full page including markdown content.

## Create Wiki Page

```bash
curl -X POST https://taskai.cc/api/projects/1/wiki \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Development Setup",
    "content": "# Dev Setup\n\nInstructions for local development..."
  }'
```

## Update Wiki Page

```bash
curl -X PATCH https://taskai.cc/api/wiki/1 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "# Updated Architecture\n\nNew content here..."
  }'
```

## Search Wiki

Full-text search across all wiki pages:

```bash
curl "https://taskai.cc/api/wiki/search?q=authentication&project_id=1" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Embedding Drawings

You can embed canvas drawings in wiki pages using the shortcode syntax:

```
[draw:DRAWING_ID:edit:m]
```

See [Drawings](/api/drawings) for creating drawings programmatically.

## Real-Time Collaboration

Wiki pages support real-time collaborative editing via Yjs. Multiple users can edit the same page simultaneously with conflict-free merging. See [Wiki Collaboration Guide](/guides/wiki-collaboration).
