---
sidebar_position: 2
---

# Data Models

Core entities and their relationships in TaskAI.

## Entity Relationships

```
User ──┬── Project ──┬── Task ──── Comment
       │             ├── SwimLane
       │             ├── Sprint
       │             ├── Tag
       │             ├── WikiPage
       │             └── Drawing
       ├── Team
       ├── APIKey
       └── OAuthProvider
```

## Users

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Primary key |
| `email` | string | Unique email address |
| `password_hash` | string | bcrypt-hashed password |
| `display_name` | string | Display name |
| `role` | string | `user` or `admin` |
| `auth_provider` | string | `local`, `google`, or `github` |
| `totp_secret` | string | TOTP secret for 2FA (encrypted) |
| `created_at` | timestamp | Account creation time |

## Projects

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Primary key |
| `name` | string | Project name |
| `description` | string | Project description |
| `user_id` | integer | Owner user ID |
| `created_at` | timestamp | Creation time |
| `updated_at` | timestamp | Last update time |

## Tasks

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Primary key |
| `task_number` | integer | Project-scoped sequential number |
| `title` | string | Task title |
| `description` | string | Markdown description |
| `status` | string | `todo`, `in_progress`, `done` |
| `priority` | string | `low`, `medium`, `high`, `critical` |
| `project_id` | integer | Parent project |
| `assigned_to` | integer | Assigned user ID |
| `swim_lane_id` | integer | Board column |
| `sprint_id` | integer | Sprint association |
| `github_issue_number` | integer | Linked GitHub issue |
| `position` | integer | Order within swim lane |
| `created_at` | timestamp | Creation time |
| `updated_at` | timestamp | Last update time |

## Swim Lanes

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Primary key |
| `name` | string | Lane name (e.g., "To Do") |
| `position` | integer | Display order |
| `status_category` | string | `todo`, `in_progress`, `done` |
| `project_id` | integer | Parent project |

## Sprints

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Primary key |
| `name` | string | Sprint name |
| `start_date` | date | Sprint start |
| `end_date` | date | Sprint end |
| `status` | string | Sprint status |
| `project_id` | integer | Parent project |

## Comments

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Primary key |
| `task_id` | integer | Parent task |
| `user_id` | integer | Author |
| `content` | string | Comment text (markdown) |
| `github_comment_id` | integer | Linked GitHub comment (nullable) |
| `created_at` | timestamp | Creation time |

## Wiki Pages

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Primary key |
| `title` | string | Page title |
| `content` | text | Markdown content |
| `project_id` | integer | Parent project |
| `created_by` | integer | Author user ID |
| `created_at` | timestamp | Creation time |
| `updated_at` | timestamp | Last update time |

## API Keys

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Primary key |
| `name` | string | Key name/label |
| `key_hash` | string | bcrypt-hashed key |
| `key_prefix` | string | First 8 chars for identification |
| `user_id` | integer | Owner user ID |
| `last_used_at` | timestamp | Last usage time |
| `created_at` | timestamp | Creation time |
