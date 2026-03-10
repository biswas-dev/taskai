---
sidebar_position: 2
---

# Wiki Collaboration

TaskAI wiki pages support real-time collaborative editing using Yjs CRDTs.

## How It Works

When multiple users edit the same wiki page:
1. Each user's changes are tracked as Yjs operations
2. The Yjs processor service merges changes in real-time
3. All connected editors see updates instantly
4. Conflicts are resolved automatically (no manual merge needed)

## Architecture

```
Browser A ──┐
            ├── WebSocket ──→ Yjs Processor ──→ API (save)
Browser B ──┘                    :3001
```

The Yjs processor runs as a separate service (`yjs-processor`) that handles WebSocket connections and operation merging.

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

### Tables, Images, Links

Standard markdown syntax is fully supported.

## API Access

Wiki pages can also be managed via the [REST API](/api/wiki) or [MCP tools](/mcp/tools-reference#wiki).
