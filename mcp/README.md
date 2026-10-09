# taskai-mcp

TaskAI's Model Context Protocol server, served at `https://mcp.taskai.cc/mcp`. A stateless Go gateway built on [go-mcp](https://github.com/anchoo2kewl/go-mcp) (the official MCP Go SDK underneath): each request carries the person's TaskAI API key, and every tool calls the TaskAI REST API as them, so permissions and audit trails are the API's own.

```
claude mcp add --transport http taskai https://mcp.taskai.cc/mcp --header "X-API-Key: <TaskAI API key>"
```

- **Auth:** `X-API-Key` (or `Authorization: Bearer`). Keys are checked against `/api/me` and cached for five minutes.
- **Agent attribution:** changes are attributed to the calling agent, named from `X-Agent-Name`, the client name sent with `initialize`, or a known `User-Agent`, and remembered per key in `AGENTS_FILE`.
- **Project scope:** `X-Project-ID: 1,2` (or `?project_id=`) sets the default projects for the wiki tools.
- **Output:** list tools return minimal fields by default; `verbose: true` returns the API's full objects. `list_tasks` filters by status and text and pages its results (50 by default) itself, because the API returns every task.

Configuration: `TASKAI_API_URL` (default `https://taskai.cc`), `PORT` (default `3000`), `AGENTS_FILE` (default `/tmp/taskai-mcp-agents.json`). `GET /health` reports liveness.

```
go test -race ./...
```
