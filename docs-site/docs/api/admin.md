---
sidebar_position: 11
---

# Admin

Administrative endpoints require the `admin` role. These endpoints manage users, system settings, and platform-wide configuration.

## List Users

```bash
curl https://taskai.cc/api/admin/users?page=1&limit=20 \
  -H "Authorization: Bearer ADMIN_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "email": "admin@example.com",
    "role": "admin",
    "created_at": "2025-10-18T00:00:00Z",
    "last_login_at": "2025-10-20T09:00:00Z"
  }
]
```

## Update User Role

```bash
curl -X PATCH https://taskai.cc/api/admin/users/2 \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role": "admin"}'
```

## System Statistics

```bash
curl https://taskai.cc/api/admin/stats \
  -H "Authorization: Bearer ADMIN_TOKEN"
```

Returns counts of users, projects, tasks, and other system metrics.

## Access Control

Only users with `role: "admin"` can access `/api/admin/*` endpoints. Attempting to access admin endpoints with a non-admin token returns `403 Forbidden`.
