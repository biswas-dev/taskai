---
sidebar_position: 4
---

# API Keys

API keys provide programmatic access to TaskAI without browser-based authentication. Use them for scripts, CI/CD pipelines, and MCP server connections.

## Creating an API Key

### Via Web UI

1. Go to **Settings** → **API Keys**
2. Click **Create API Key**
3. Give it a descriptive name (e.g., "CI Pipeline" or "MCP Server")
4. Copy the key immediately — it won't be shown again

### Via API

```bash
curl -X POST https://taskai.cc/api/api-keys \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "My Script"}'
```

**Response (201):**
```json
{
  "id": 1,
  "name": "My Script",
  "key": "tkai_abc123def456...",
  "key_prefix": "tkai_abc",
  "created_at": "2025-10-18T00:00:00Z"
}
```

:::caution
The full key is only returned once at creation time. Store it securely.
:::

## Using an API Key

Include the key in the `Authorization` header:

```bash
curl https://taskai.cc/api/projects \
  -H "Authorization: ApiKey tkai_abc123def456..."
```

API keys have the same permissions as the user who created them.

## Listing API Keys

```bash
curl https://taskai.cc/api/api-keys \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Returns key metadata (name, prefix, last used) — never the full key.

## Revoking an API Key

```bash
curl -X DELETE https://taskai.cc/api/api-keys/1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Revocation is immediate. Any requests using the deleted key will return 401.

## Using with MCP

API keys are the recommended authentication method for MCP server connections:

```json
{
  "mcpServers": {
    "taskai": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "https://mcp.taskai.cc/sse"],
      "env": {
        "API_KEY": "tkai_abc123def456..."
      }
    }
  }
}
```

## Best Practices

- Create separate keys for each integration (don't reuse keys)
- Use descriptive names so you know what each key is for
- Rotate keys periodically
- Revoke unused keys
- Never commit API keys to source control
