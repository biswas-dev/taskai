# TaskAI memory protocol, version 1

An open protocol for **memories**: short, durable lessons (a fact, a decision, a gotcha, a runbook, a handoff) that any application or LLM writes and searches. TaskAI stores them per user. Claude, Codex, Gemini, a CLI hub or your own app all speak the same operations, over REST or the TaskAI MCP server.

The machine-readable contract is [`api/openapi.yaml`](../api/openapi.yaml) (tag `Memories`). This page explains the semantics. Every memory response carries the header `TaskAI-Memory-Protocol: 1`.

## Concepts

| Term | Meaning |
|---|---|
| **Memory** | One lesson: `kind`, a one-line `title` (≤ 200 bytes; defaults to the body's first line) and a `body` (≤ 8000 bytes). |
| **Owner** | The user whose API key (or session) wrote it. Memories belong to a user, not a project. |
| **Scope** | Optional filters: `repo` (`owner/name`), `issue`, `pr`, `folder` (absolute path), `files` (≤ 50). With no scope, the memory is about the user as a whole. |
| **Author** | Optional metadata: `agent`, `provider`, `host`, `session`. Nothing depends on it. |
| **Origin** | Optional `(origin_host, origin_id)`: the writing client's name and its own id for the memory. This is the idempotency key. |
| **Status** | `active`, `superseded` (replaced by a newer memory), or `compacted` (a tombstone: removed, but its origin is remembered). |

Kinds: `fact`, `decision`, `gotcha`, `runbook`, `handoff`. Confidence: `low`, `medium` (default), `high`. `expires_at` takes a memory out of search after that time.

## Authentication

Send one of:

- `Authorization: ApiKey <key>` — a TaskAI API key, made under Settings → API Keys. It acts as its owner.
- `Authorization: Bearer <jwt>` — a signed-in session.

Over MCP, send `X-API-Key: <key>` to `https://mcp.taskai.cc/mcp`.

## Operations

| Operation | REST | MCP tool |
|---|---|---|
| Write / upsert | `POST /api/memories` | `write_memory` |
| Supersede | `POST /api/memories` with `supersedes` or `supersedes_origin_id` | `supersede_memory` |
| Search | `POST /api/memories/search` | `search_memories` |
| List | `GET /api/memories` | `list_memories` |
| Get | `GET /api/memories/{id}` | `get_memory` |
| Delete | `DELETE /api/memories/{id}` | `delete_memory` |
| Share | `GET` / `PUT /api/memories/shares` | `get_memory_shares` / `set_memory_shares` |
| Usage | `GET /api/memories/usage` | `memory_usage` |
| Compact | `POST /api/memories/compact` | `compact_memories` |
| Feedback (optional) | `POST /api/memories/{id}/feedback` | `memory_feedback` |

### Write and idempotency

`POST /api/memories` answers `{memory, outcome, usage}`:

| outcome | When | Status |
|---|---|---|
| `created` | A new memory. | 201 |
| `updated` | The same origin with changed content or metadata (expiry, confidence, supersede link, author). An upsert replaces the row. | 200 |
| `duplicate` | Nothing new: an exact repeat. | 200 |
| `ignored` | The origin was compacted or deleted. Do not send it again. | 200 |

The rules:

- **With an origin**, a write always lands on its own `(origin_host, origin_id)` row, so retrying is safe. If you copy memories between systems, keep the **original writer's** origin, so one memory has one key everywhere.
- **Without an origin**, a write that repeats an active memory's content and scope returns that memory as `duplicate`. Content is compared by a hash of kind, title, body and scope that ignores case and spacing. The repeat may extend the memory's expiry or raise its confidence.
- Replaying a memory that has since been superseded keeps it superseded.

### Supersede

To replace a lesson, write the new one with either:

- `supersedes: <TaskAI id>`, or
- `supersedes_origin_id: <your id>`, resolved under the same `origin_host`. It may arrive **before** the memory it replaces; that memory is marked superseded when it lands.

The old memory becomes `superseded` and leaves search, but stays listable with `status=all`. A memory cannot supersede itself, a superseded memory cannot supersede another, and two memories cannot supersede each other.

### Search

`POST /api/memories/search {query, repo?, kind?, issue?, pr?, files?, owner_id?, include_inactive?, limit?}`

- **Ranking.** Full text and vector similarity, merged with Reciprocal Rank Fusion. If the query can't be embedded within 1.5 s, it falls back to full text; with no full-text hits, to keyword matching.
- **What is searched.** The caller's memories and those shared with them, active and unexpired unless `include_inactive`.
- **Filters.** Applied before ranking: `repo` is case-insensitive, and `files` matches a memory naming any of them.
- **Limits and speed.** At most 50 results; built to answer well within an agent's 4 s context budget.

### Share

`PUT /api/memories/shares {user_ids: [...]}` replaces the list of people who can read and search the caller's memories. Sharees can read; only the owner and the owner's keys can write, delete or compact.

- **Who.** Only people in a team or project with the owner can be added, at most 100.
- **Removing everyone.** `[]` removes every share; a body without `user_ids` is refused.
- **Default.** Memories are private to their owner.

### Quota

Each user has 1 GiB by default; an admin can change it per user (`PUT /api/admin/users/{id}/memory-quota`).

- **What counts.** Stored bytes: text, metadata and 1,536 bytes for each memory's vector.
- **Warning.** At 80%, `usage.warning` is set.
- **Refusal.** A write that would exceed the quota runs a compaction first. If it still doesn't fit, it is refused with `413` and code `memory_quota_exceeded`. Treat that as terminal: retry only after compacting, deleting or a quota change.

### Compact

`POST /api/memories/compact` shrinks storage without losing knowledge and reports what it freed (`freed_bytes` and the counts). It also runs automatically past 80%. It is deterministic and idempotent: a second run frees nothing.

1. Superseded and expired memories are removed.
2. Copies (the same content hash) fold into the copy that dominates the rest: it lasts longest and is the most confident, with the newest winning ties. If no copy dominates, the group is left alone.
3. Low-confidence memories older than 90 days, of one kind and scope and without an expiry, are merged into one memory, bodies kept verbatim, within the body and file limits.

Similar text that isn't an exact copy is never merged: "Use X" and "Do not use X" overlap almost entirely. To replace a lesson, supersede it.

Removed rows that have an origin leave a tombstone, so a client re-sending them gets `ignored`.

## Strength and belief (optional)

A server with `MEMORY_STRENGTH=on` adds two numbers to each memory and uses them to order search results. They are additive: the protocol version stays `1`, and clients that do not know them ignore them. The evidence and the eval behind them are in [research/memory-strength.md](research/memory-strength.md).

| Field (`signals`) | Meaning |
|---|---|
| `strength` | How much a memory is in use: ACT-R base-level activation, from its uses and how recent they are. It decays with time. The write counts as the first use. |
| `belief` | An evidence score in (0, 1): a Beta-Bernoulli posterior of `confirmed` and `wrong` votes over a prior from the stated `confidence` level. It is not a calibrated probability until feedback labels exist to fit it. |
| `uses`, `confirmed`, `wrong`, `last_used_at` | The evidence behind the two numbers. |

Tell the store how a memory did with `POST /api/memories/{id}/feedback {signal}` (MCP `memory_feedback`). Only the owner's credentials can give feedback.

| `signal` | Send it when | Effect |
|---|---|---|
| `useful` | The memory helped the task you did. | One more use. Counted at most once an hour per memory (`spaced`). |
| `confirmed` | You checked that it is still true. | One more use and one confirmation. One vote per credential (`duplicate`). |
| `wrong` | It is false or outdated. | One wrong vote. One vote per credential. Supersede it if you know the new value. |

The rules:

- **Search is not use.** A memory that search returns gets no signal, or the strong would only get stronger.
- **Re-rank, never hide.** Strength and belief reorder the memories that relevance found; they never add one. The most relevant memory always stays in the results.
- **Evidence survives replays.** A replay from a hub keeps the counters. New text resets the votes, because they judged the old text; the uses stay. Compaction adds a folded copy's counters to the copy it keeps, and does not merge a memory that someone confirmed or used in the last 90 days.

## Errors

`{"error": "human readable", "code": "machine_code"}`:

| Status | Code | Meaning |
|---|---|---|
| 400 | `validation_error` / `invalid_request` | The request is wrong; the message says what to fix. |
| 401 | `unauthorized` | Missing or bad credentials. |
| 403 | `forbidden` | Not yours (for example, a sharee deleting), or not shared with you. |
| 404 | `not_found` | No such memory you can see. |
| 413 | `memory_quota_exceeded` | Over quota, as above. |

## Quick start

```sh
export TASKAI_KEY=...   # Settings → API Keys

# Write (idempotent with an origin)
curl -s https://taskai.cc/api/memories -H "Authorization: ApiKey $TASKAI_KEY" -H 'Content-Type: application/json' \
  -d '{"kind":"gotcha","title":"Partial index needs WHERE","body":"ON CONFLICT on a partial unique index must repeat its WHERE clause.","scope":{"repo":"acme/api"},"origin_host":"my-app","origin_id":"42"}'

# Search
curl -s https://taskai.cc/api/memories/search -H "Authorization: ApiKey $TASKAI_KEY" -H 'Content-Type: application/json' \
  -d '{"query":"on conflict partial index","repo":"acme/api"}'
```

MCP (Claude Code shown; any MCP client takes the same URL and header):

```sh
claude mcp add --transport http taskai https://mcp.taskai.cc/mcp --header "X-API-Key: $TASKAI_KEY"
```

Then ask your agent, for example: "search my memories for the deploy gotchas". The MCP tools are `write_memory`, `search_memories`, `get_memory`, `list_memories`, `supersede_memory`, `get_memory_shares`, `memory_usage` and `compact_memories`.

This protocol works with Claude, Codex, Gemini, ChatGPT and any other client that can send HTTP or MCP requests.

## Versioning

This is version `1`. Additive changes (new optional fields, new endpoints, new outcomes clients may ignore) keep the version. An incompatible change bumps `TaskAI-Memory-Protocol`, and the previous version stays served until clients have moved. Clients should ignore fields they don't know.

## Migrating from wiki-page memories

Earlier llm-brain hubs stored one wiki page per memory in an "Agent memory" project. `POST /api/memories/import-wiki {project_id, origin_host}` copies them; the page footer's `llm-brain memory N` becomes `origin_id`. Run it as the user who owns the hub's API key, and use the hub's name as `origin_host`, so the hub's later writes land on the same rows. The pages are left in place.
