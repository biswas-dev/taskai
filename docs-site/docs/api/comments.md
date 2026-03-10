---
sidebar_position: 4
---

# Comments

Add discussion threads to tasks. Comments support markdown and are synced with GitHub issues when integration is enabled.

## List Comments

```bash
curl https://taskai.cc/api/tasks/1/comments \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "task_id": 1,
    "user_id": 1,
    "content": "Started working on this. Will update by EOD.",
    "created_at": "2025-10-18T10:00:00Z",
    "updated_at": "2025-10-18T10:00:00Z"
  }
]
```

## Add Comment

```bash
curl -X POST https://taskai.cc/api/tasks/1/comments \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Completed the initial implementation. Ready for review."
  }'
```

**Response (201):** Returns the created comment.

## GitHub Sync

When a project has GitHub integration enabled, comments are synced bidirectionally:
- Comments added in TaskAI appear on the linked GitHub issue
- Comments added on GitHub issues appear in TaskAI

See [GitHub Sync Guide](/guides/github-sync) for setup details.
