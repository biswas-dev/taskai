---
sidebar_position: 14
---

# Notifications

TaskAI provides in-app notifications and real-time updates via WebSocket.

## List Notifications

```bash
curl https://taskai.cc/api/notifications \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "type": "task_assigned",
    "message": "You were assigned to 'Fix login bug'",
    "read": false,
    "created_at": "2025-10-18T10:00:00Z"
  }
]
```

## Mark as Read

```bash
curl -X PATCH https://taskai.cc/api/notifications/1 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"read": true}'
```

## Notification Types

| Type | Trigger |
|------|---------|
| `task_assigned` | You are assigned to a task |
| `task_commented` | Someone comments on your task |
| `task_status_changed` | A task you're watching changes status |
| `project_invited` | You're invited to a project |
| `mention` | You're mentioned in a comment |

## WebSocket (Real-time)

Connect to the WebSocket endpoint for live updates:

```javascript
const ws = new WebSocket('wss://taskai.cc/api/ws?token=YOUR_TOKEN');

ws.onmessage = (event) => {
  const notification = JSON.parse(event.data);
  console.log('New notification:', notification);
};
```
