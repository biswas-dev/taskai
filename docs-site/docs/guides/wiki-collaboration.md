---
sidebar_position: 2
---

# Wiki Collaboration

TaskAI wiki pages support real-time collaborative editing — multiple users can edit the same page simultaneously with automatic conflict resolution.

## How It Works

When multiple users edit the same wiki page:
1. Each user's changes are tracked as operations
2. Changes are merged in real-time using CRDTs (Conflict-free Replicated Data Types)
3. All connected editors see updates instantly
4. No manual merge conflicts

## Features

- **Real-time cursors** — See where other users are typing
- **Conflict-free** — CRDT-based merging ensures no data loss
- **Offline support** — Changes queue locally and sync when reconnected
- **Markdown** — Full markdown support with live preview

## Embedding Content

Wiki pages support embedding:

### Drawings

```markdown
[draw:DRAWING_ID:edit:m]
```

Embeds an interactive canvas drawing. Users can edit it directly in the wiki page.

### Code Blocks

````markdown
```typescript
const api = new TaskAIClient();
```
````

Standard markdown syntax for tables, images, and links is fully supported.

## API Access

Wiki pages can also be managed via the [REST API](/api/wiki) or [MCP tools](/mcp/tools-reference#wiki).
