---
sidebar_position: 1
---

# MCP Integration

TaskAI includes a built-in [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that allows AI agents to read and manage your projects, tasks, wiki pages, and drawings.

## What is MCP?

MCP is an open protocol that lets AI tools (like Claude, Cursor, Windsurf, etc.) interact with external services through a standardized interface. Instead of copy-pasting context, the AI agent can directly query your project data.

## Connecting to the MCP Server

### Claude Desktop / Claude Code

Add to your MCP configuration (`claude_desktop_config.json` or `.mcp.json`):

```json
{
  "mcpServers": {
    "taskai": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "https://mcp.taskai.cc/sse"],
      "env": {
        "API_KEY": "your-api-key-here"
      }
    }
  }
}
```

## Getting an API Key

1. Log in to TaskAI at [taskai.cc](https://taskai.cc)
2. Go to **Settings** → **API Keys**
3. Click **Create API Key**
4. Copy the key and add it to your MCP configuration

Or create one via the API — see [API Keys Guide](/guides/api-keys).

## Available Tools

The MCP server exposes 23 tools organized into these categories:

| Category | Tools | Description |
|----------|-------|-------------|
| **System** | `get_me`, `get_version`, `health_check` | User info and system status |
| **Projects** | `list_projects`, `get_project` | Browse projects |
| **Tasks** | `list_tasks`, `get_task`, `create_task`, `update_task` | Full task management |
| **Swim Lanes** | `list_swim_lanes` | View board columns |
| **Comments** | `list_comments`, `add_comment` | Task discussions |
| **Wiki** | `list_wiki_pages`, `get_wiki_page`, `get_wiki_page_content`, `create_wiki_page`, `update_wiki_page_content`, `search_wiki`, `autocomplete_wiki_pages` | Documentation |
| **Drawings** | `list_project_drawings`, `create_drawing`, `save_drawing`, `get_drawing` | Diagrams |

See the [Tools Reference](/mcp/tools-reference) for detailed documentation of each tool.

## Token Efficiency

The MCP server is optimized for minimal token usage. By default, tools return only essential fields (id, title, status). Use `verbose: true` to get full details when needed.

## Example Usage

Once connected, you can ask your AI agent things like:

- "Show me all tasks in project 1 that are in progress"
- "Create a new task to fix the login bug"
- "What does the architecture wiki page say?"
- "Add a comment to task #42 saying the fix is deployed"
- "Create an architecture diagram with boxes for API, DB, and Frontend"
