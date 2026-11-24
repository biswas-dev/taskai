package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// client talks to a LOCAL TaskAI API (never production): it writes the eval's memories and runs the real hybrid
// search, so the flat baseline is the shipped ranker, not a copy of it.
type client struct {
	base  string
	token string
	http  *http.Client
}

func login(base, email, password string) (*client, error) {
	c := &client{base: base, http: &http.Client{Timeout: 30 * time.Second}}
	var out struct {
		Token string `json:"token"`
	}
	if err := c.do(http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": password}, &out); err != nil {
		return nil, err
	}
	c.token = out.Token
	return c, nil
}

func (c *client) do(method, path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(method, c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, body)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

type memScope struct {
	Repo   string   `json:"repo,omitempty"`
	Folder string   `json:"folder,omitempty"`
	Files  []string `json:"files,omitempty"`
}

type memWrite struct {
	Kind       string     `json:"kind"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Scope      memScope   `json:"scope"`
	Confidence string     `json:"confidence,omitempty"`
	OriginHost string     `json:"origin_host"`
	OriginID   string     `json:"origin_id"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
}

type hit struct {
	ID   int64   `json:"id"`
	Rank float64 `json:"rank"`
}

func (c *client) write(m memWrite) (int64, error) {
	var out struct {
		Memory struct {
			ID int64 `json:"id"`
		} `json:"memory"`
	}
	err := c.do(http.MethodPost, "/api/memories", m, &out)
	return out.Memory.ID, err
}

// search runs the shipped search; it returns the hits, the mode used and the wall time.
func (c *client) search(query, repo string, limit int) ([]hit, string, time.Duration, error) {
	var out struct {
		Results []hit  `json:"results"`
		Mode    string `json:"mode"`
	}
	start := time.Now()
	err := c.do(http.MethodPost, "/api/memories/search",
		map[string]any{"query": query, "repo": repo, "limit": limit, "include_inactive": true}, &out)
	return out.Results, out.Mode, time.Since(start), err
}

// waitEmbedded blocks until the API's worker has embedded every memory of the owner, so the vector leg is live.
func waitEmbedded(ctx context.Context, dsn string, owner int64) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	for {
		var pending int
		if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM agent_memories WHERE owner_id = $1 AND embedding IS NULL`, owner).Scan(&pending); err != nil {
			return err
		}
		if pending == 0 {
			return nil
		}
		logf("waiting for %d embeddings", pending)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

// embeddings reads the stored vectors (for the conflict-candidate experiment).
func embeddings(ctx context.Context, dsn string, owner int64) (map[int64][]float32, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close(ctx) }()
	rows, err := conn.Query(ctx, `SELECT id, embedding::text FROM agent_memories WHERE owner_id = $1 AND embedding IS NOT NULL`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]float32{}
	for rows.Next() {
		var id int64
		var s string
		if err := rows.Scan(&id, &s); err != nil {
			return nil, err
		}
		var v []float32
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return nil, err
		}
		out[id] = v
	}
	return out, rows.Err()
}

func ownerID(ctx context.Context, dsn, email string) (int64, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close(ctx) }()
	var id int64
	err = conn.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	return id, err
}
