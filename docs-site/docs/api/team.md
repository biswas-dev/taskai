---
sidebar_position: 10
---

# Team Management

Manage teams and project members for collaboration.

## Project Members

### List Members

```bash
curl https://taskai.cc/api/projects/1/members \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response (200):**
```json
[
  {
    "id": 1,
    "user_id": 1,
    "project_id": 1,
    "role": "owner",
    "email": "owner@example.com"
  },
  {
    "id": 2,
    "user_id": 2,
    "project_id": 1,
    "role": "member",
    "email": "member@example.com"
  }
]
```

### Add Member

```bash
curl -X POST https://taskai.cc/api/projects/1/members \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "newmember@example.com",
    "role": "member"
  }'
```

**Roles:** `owner`, `admin`, `member`

### Update Member Role

```bash
curl -X PATCH https://taskai.cc/api/projects/1/members/2 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role": "admin"}'
```

### Remove Member

```bash
curl -X DELETE https://taskai.cc/api/projects/1/members/2 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Teams

Teams group users across projects.

### List Teams

```bash
curl https://taskai.cc/api/teams \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Create Team

```bash
curl -X POST https://taskai.cc/api/teams \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Engineering",
    "description": "Backend and frontend developers"
  }'
```

## Invitations

Invite users to join your team or project via email.

```bash
curl -X POST https://taskai.cc/api/invitations \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "invited@example.com",
    "team_id": 1,
    "role": "member"
  }'
```
