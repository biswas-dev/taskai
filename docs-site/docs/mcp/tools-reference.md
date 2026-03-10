---
sidebar_position: 2
---

# MCP Tools Reference

Complete reference for all 23 MCP tools available in the TaskAI MCP server.

All tools accept an optional `verbose` parameter (default: `false`). When `false`, responses return minimal fields to save tokens.

---

## System

### `get_me`

Get current authenticated user info.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `verbose` | boolean | No | Return full details |

### `get_version`

Get system version information (backend version, DB migration version, build info).

### `health_check`

Check system health status (database connectivity).

---

## Projects

### `list_projects`

List all projects accessible to the authenticated user.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page` | number | No | Page number |
| `limit` | number | No | Items per page |
| `verbose` | boolean | No | Return full details |

### `get_project`

Get project details by ID.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |

---

## Tasks

### `list_tasks`

List tasks in a project with optional filtering.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |
| `query` | string | No | Search query |
| `status` | string | No | Filter: `todo`, `in_progress`, `done` |
| `page` | number | No | Page number |
| `limit` | number | No | Items per page |
| `verbose` | boolean | No | Return full task details |

**Minimal response fields:** `id`, `task_number`, `title`, `status`, `priority`

### `get_task`

Get a single task by its project-scoped task number.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |
| `task_number` | number | Yes | Task number (e.g., 1, 2, 3) |

### `create_task`

Create a new task in a project.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |
| `title` | string | Yes | Task title |
| `description` | string | No | Task description |
| `status` | string | No | Status (default: `todo`) |
| `priority` | string | No | `low`, `medium`, `high`, `critical` |
| `assigned_to` | string | No | User ID to assign |
| `swim_lane_id` | number | No | Swim lane ID |

### `update_task`

Update an existing task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | Yes | Task ID |
| `title` | string | No | New title |
| `description` | string | No | New description |
| `status` | string | No | New status |
| `priority` | string | No | New priority |
| `assigned_to` | string | No | New assignee user ID |
| `swim_lane_id` | number | No | Swim lane ID |

---

## Swim Lanes

### `list_swim_lanes`

List swim lanes (board columns) for a project. Each lane has a `status_category` that maps to task status.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |

---

## Comments

### `list_comments`

List comments on a task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | Yes | Task ID |

### `add_comment`

Add a comment to a task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | Yes | Task ID |
| `content` | string | Yes | Comment text |

---

## Wiki

### `list_wiki_pages`

List all wiki pages in a project.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |

### `get_wiki_page`

Get a specific wiki page by ID including its full markdown content.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page_id` | string | Yes | Wiki page ID |

### `get_wiki_page_content`

Get just the markdown content of a wiki page (lighter than `get_wiki_page`).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page_id` | string | Yes | Wiki page ID |

### `create_wiki_page`

Create a new wiki page in a project.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |
| `title` | string | Yes | Page title |
| `content` | string | No | Initial page content (markdown) |

### `update_wiki_page_content`

Update the content of an existing wiki page.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `page_id` | string | Yes | Wiki page ID |
| `content` | string | Yes | New page content (markdown) |

### `search_wiki`

Full-text search across wiki pages.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Search query |
| `project_id` | string | No | Filter by project |
| `limit` | number | No | Max results (default: 20, max: 100) |
| `recency_days` | number | No | Only pages updated in last N days |

### `autocomplete_wiki_pages`

Fuzzy search for wiki page titles.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Search query for page title |
| `project_id` | string | No | Filter by project |
| `limit` | number | No | Max results (default: 10, max: 50) |

---

## Drawings

### `list_project_drawings`

List all drawings registered to a project.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |

### `create_drawing`

Create a new drawing canvas. Returns the `draw_id` and shortcode for wiki embedding: `[draw:ID:edit:m]`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_id` | string | Yes | Project ID |
| `title` | string | No | Drawing title (default: "Untitled") |
| `scene` | object | No | Initial scene JSON |

**Scene format:**
```json
{
  "version": 1,
  "elements": [
    {"type": "rect", "x": 0, "y": 0, "w": 200, "h": 100, "text": "Box"},
    {"type": "arrow", "x": 100, "y": 100, "x2": 100, "y2": 200}
  ]
}
```

### `save_drawing`

Update an existing drawing's scene data.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `draw_id` | string | Yes | Drawing ID |
| `title` | string | Yes | Drawing title |
| `scene` | object | Yes | Scene JSON |

### `get_drawing`

Get a drawing's current scene data.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `draw_id` | string | Yes | Drawing ID |
