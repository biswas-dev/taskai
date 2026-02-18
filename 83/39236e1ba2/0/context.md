# Session Context

## User Prompts

### Prompt 1

make sure we know our project is https://taskai.cc/app/projects/1 commit to memory - please improve the MCP server, it is returning too many tokens that drones and overwhelm the llm. please allow all tokens but default behaviour should be to minimize tokens all the time

### Prompt 2

- Task management per project is out of scope; we only implement "each project has a wiki".
- Wiki pages support rich formatting via a CRDT doc (Yjs). Formatting must be separate from search text.
- Searches must be lightning fast using ONLY Postgres: full-text search on plain text + metadata, block-level.
- Real-time collaboration editing via WebSocket using Yjs, persisted to Postgres.
- Expose content via MCP: tools search_wiki, get_block, get_page_outline, get_page, get_recent_changes.
- Prov...

### Prompt 3

[Request interrupted by user for tool use]

