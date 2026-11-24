package main

// Memories (TaskAI memory protocol v1, docs/memory-protocol.md): a user's own store of short lessons that any app or
// LLM writes and searches; the same operations as the REST API.
// Results are trimmed to what an agent needs (id, kind, title, body, scope, rank) unless verbose is set.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	gomcp "github.com/anchoo2kewl/go-mcp"
)

var memoryFields = []string{"id", "kind", "title", "body", "scope", "confidence", "status", "owner_id", "rank", "signals"}

type searchMemoriesIn struct {
	Query           string   `json:"query" jsonschema:"What you want to know; matched by meaning and by words"`
	Repo            string   `json:"repo,omitempty" jsonschema:"owner/name"`
	Kind            string   `json:"kind,omitempty" jsonschema:"fact, decision, gotcha, runbook or handoff"`
	Files           []string `json:"files,omitempty" jsonschema:"Repo-relative paths; matches memories naming any of them"`
	Issue           int      `json:"issue,omitempty"`
	PR              int      `json:"pr,omitempty"`
	OwnerID         int64    `json:"owner_id,omitempty" jsonschema:"Only this person's memories (yours or shared with you)"`
	IncludeInactive bool     `json:"include_inactive,omitempty" jsonschema:"Also superseded and expired memories"`
	Limit           int      `json:"limit,omitempty" jsonschema:"Default 10, max 50"`
	Verbose         bool     `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type listMemoriesIn struct {
	Repo    string `json:"repo,omitempty" jsonschema:"owner/name"`
	Kind    string `json:"kind,omitempty"`
	Status  string `json:"status,omitempty" jsonschema:"all includes superseded and expired"`
	OwnerID int64  `json:"owner_id,omitempty"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Default 20, max 100"`
	Offset  int    `json:"offset,omitempty"`
	Verbose bool   `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type writeMemoryIn struct {
	Kind       string   `json:"kind" jsonschema:"fact, decision, gotcha, runbook or handoff"`
	Title      string   `json:"title,omitempty" jsonschema:"One line; defaults to the body's first line"`
	Body       string   `json:"body" jsonschema:"The lesson, verbatim and short (max 8000 bytes). Never a secret."`
	Repo       string   `json:"repo,omitempty" jsonschema:"owner/name (optional scope)"`
	Folder     string   `json:"folder,omitempty" jsonschema:"Absolute path (optional scope)"`
	Issue      int      `json:"issue,omitempty"`
	PR         int      `json:"pr,omitempty"`
	Files      []string `json:"files,omitempty" jsonschema:"Repo-relative paths it is about"`
	Agent      string   `json:"agent,omitempty" jsonschema:"Optional: who writes it, e.g. claude-code or an app name"`
	Provider   string   `json:"provider,omitempty" jsonschema:"Optional: e.g. claude, codex, gemini, openai, human"`
	Host       string   `json:"host,omitempty"`
	Session    string   `json:"session,omitempty"`
	Confidence string   `json:"confidence,omitempty" jsonschema:"low, medium (default) or high"`
	ExpiresAt  string   `json:"expires_at,omitempty" jsonschema:"RFC 3339; after it the memory is dropped from search"`
	OriginHost string   `json:"origin_host,omitempty" jsonschema:"Optional: your client's name; with origin_id it makes the write an idempotent upsert"`
	OriginID   string   `json:"origin_id,omitempty"`
}

type supersedeMemoryIn struct {
	MemoryID string `json:"memory_id" jsonschema:"The memory the new one replaces"`
	writeMemoryIn
}

type memoryIDIn struct {
	MemoryID string `json:"memory_id" jsonschema:"Memory ID"`
}

type memoryFeedbackIn struct {
	MemoryID string `json:"memory_id" jsonschema:"The memory the signal is about"`
	Signal   string `json:"signal" jsonschema:"useful (it helped this task), confirmed (you checked it is still true) or wrong (it is false or outdated)"`
}

type setMemorySharesIn struct {
	UserIDs []int64 `json:"user_ids" jsonschema:"Everyone who may read and search your memories (people in a team or project with you); [] removes every share"`
}

// memoryID is a positive memory id, so "usage" or "" can never route to another endpoint.
func memoryID(s string) (string, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return "", fmt.Errorf("memory_id must be a memory ID, got %q", s)
	}
	return strconv.FormatInt(id, 10), nil
}

func (in writeMemoryIn) body() map[string]any {
	b := map[string]any{"kind": in.Kind, "title": in.Title, "body": in.Body,
		"scope":  map[string]any{"repo": in.Repo, "folder": in.Folder, "issue": in.Issue, "pr": in.PR, "files": in.Files},
		"author": map[string]any{"agent": in.Agent, "provider": in.Provider, "host": in.Host, "session": in.Session}}
	for k, v := range map[string]string{"confidence": in.Confidence, "expires_at": in.ExpiresAt, "origin_host": in.OriginHost, "origin_id": in.OriginID} {
		if v != "" {
			b[k] = v
		}
	}
	return b
}

func registerMemoryTools(srv *gomcp.Server) {
	read := func(name, desc string) gomcp.Tool { return gomcp.Tool{Name: name, Description: desc} }
	write := func(name, desc string) gomcp.Tool { return gomcp.Tool{Name: name, Description: desc, Write: true} }
	destroy := func(name, desc string) gomcp.Tool {
		return gomcp.Tool{Name: name, Description: desc, Write: true, Destructive: true, Idempotent: true}
	}
	trim := func(verbose bool, res any, key string) any {
		return shaped(verbose, res, func(v any) any { return pickEach(items(v, key), memoryFields...) })
	}

	gomcp.Register(srv, read("search_memories", "Search your memories (and those shared with you): facts, decisions, gotchas, runbooks and handoffs. Filter by repo, kind, files, issue or PR. Check before you work."),
		func(ctx context.Context, in searchMemoriesIn) (any, error) {
			_, c := sess(ctx)
			res, err := c.send(ctx, http.MethodPost, "/api/memories/search", map[string]any{"query": in.Query, "repo": in.Repo,
				"kind": in.Kind, "files": in.Files, "issue": in.Issue, "pr": in.PR, "owner_id": in.OwnerID,
				"include_inactive": in.IncludeInactive, "limit": in.Limit})
			return trim(in.Verbose, res, "results"), err
		})

	gomcp.Register(srv, read("list_memories", "List memories newest first, optionally by repo or kind."),
		func(ctx context.Context, in listMemoriesIn) (any, error) {
			_, c := sess(ctx)
			q := url.Values{}
			for k, v := range map[string]string{"repo": in.Repo, "kind": in.Kind, "status": in.Status} {
				if v != "" {
					q.Set(k, v)
				}
			}
			for k, v := range map[string]int64{"owner_id": in.OwnerID, "limit": int64(in.Limit), "offset": int64(in.Offset)} {
				if v > 0 {
					q.Set(k, strconv.FormatInt(v, 10))
				}
			}
			res, err := c.get(ctx, "/api/memories?"+q.Encode())
			return trim(in.Verbose, res, "memories"), err
		})

	gomcp.Register(srv, write("write_memory", "Save a lesson to your memories (fact, decision, gotcha, runbook, handoff). An exact repeat returns the one already kept. Small and verbatim; never a secret. If the answer lists similar memories that this one replaces, supersede them."),
		func(ctx context.Context, in writeMemoryIn) (any, error) {
			_, c := sess(ctx)
			return c.send(ctx, http.MethodPost, "/api/memories", in.body())
		})

	gomcp.Register(srv, write("supersede_memory", "Save a new memory that replaces an outdated one (the old one is marked superseded and dropped from search)."),
		func(ctx context.Context, in supersedeMemoryIn) (any, error) {
			ids, err := memoryID(in.MemoryID)
			if err != nil {
				return nil, err
			}
			id, _ := strconv.ParseInt(ids, 10, 64)
			_, c := sess(ctx)
			b := in.body()
			b["supersedes"] = id
			return c.send(ctx, http.MethodPost, "/api/memories", b)
		})

	gomcp.Register(srv, read("get_memory", "Get one memory (yours or shared with you) in full."),
		func(ctx context.Context, in memoryIDIn) (any, error) {
			id, err := memoryID(in.MemoryID)
			if err != nil {
				return nil, err
			}
			_, c := sess(ctx)
			return c.get(ctx, "/api/memories/"+id)
		})

	gomcp.Register(srv, read("get_memory_shares", "Who can read your memories, and whose memories you can read."),
		func(ctx context.Context, _ verboseIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, "/api/memories/shares")
		})

	gomcp.Register(srv, destroy("set_memory_shares", "Replace the list of people who can read and search (never change) your memories."),
		func(ctx context.Context, in setMemorySharesIn) (any, error) {
			_, c := sess(ctx)
			return c.send(ctx, http.MethodPut, "/api/memories/shares", map[string]any{"user_ids": in.UserIDs})
		})

	gomcp.Register(srv, destroy("delete_memory", "Delete one of your memories."),
		func(ctx context.Context, in memoryIDIn) (any, error) {
			id, err := memoryID(in.MemoryID)
			if err != nil {
				return nil, err
			}
			_, c := sess(ctx)
			return c.send(ctx, http.MethodDelete, "/api/memories/"+id, nil)
		})

	gomcp.Register(srv, write("memory_feedback", "Tell your memory store how a memory did: useful, confirmed or wrong. Useful and confirmed make it stronger; confirmed and wrong change its belief score. Send it only for a memory you actually used or checked. Needs MEMORY_STRENGTH collect or on at the server."),
		func(ctx context.Context, in memoryFeedbackIn) (any, error) {
			id, err := memoryID(in.MemoryID)
			if err != nil {
				return nil, err
			}
			_, c := sess(ctx)
			return c.send(ctx, http.MethodPost, "/api/memories/"+id+"/feedback", map[string]any{"signal": in.Signal})
		})

	gomcp.Register(srv, read("memory_usage", "How much of your memory quota is used (bytes, count, warning past 80%)."),
		func(ctx context.Context, _ verboseIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, "/api/memories/usage")
		})

	gomcp.Register(srv, write("compact_memories", "Shrink your memory storage without losing knowledge: drops superseded and expired memories, merges near-duplicates and old low-confidence notes. Reports bytes freed."),
		func(ctx context.Context, _ verboseIn) (any, error) {
			_, c := sess(ctx)
			return c.send(ctx, http.MethodPost, "/api/memories/compact", map[string]any{})
		})
}
