package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	gomcp "github.com/anchoo2kewl/go-mcp"
)

// Known MCP client identifiers and the names TaskAI shows for them.
var agentNames = map[string]string{
	"claude-code": "Claude Code",
	"codex-cli":   "Codex",
	"gemini-cli":  "Gemini",
	"cursor":      "Cursor",
	"windsurf":    "Windsurf",
}

func normalizeAgent(raw string) string {
	raw = strings.TrimSpace(raw)
	if name, ok := agentNames[strings.ToLower(raw)]; ok {
		return name
	}
	if len(raw) > 100 {
		raw = raw[:100]
	}
	return raw
}

type session struct {
	client   *Client
	user     map[string]any
	projects []string
}

type sessionKey struct{}

func sessionFrom(ctx context.Context) *session {
	s, _ := ctx.Value(sessionKey{}).(*session)
	return s
}

// Gateway authenticates callers against the TaskAI API and remembers which
// agent each key belongs to, so changes are attributed to "Claude Code"
// rather than to the person.
type Gateway struct {
	APIURL     string
	HTTP       *http.Client
	CacheTTL   time.Duration
	AgentsFile string
	PollEvery  time.Duration

	mu     sync.Mutex
	users  map[string]cachedUser
	agents map[string]string
}

type cachedUser struct {
	user  map[string]any
	until time.Time
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:12])
}

// Authenticate is the go-mcp Authenticator.
func (g *Gateway) Authenticate(r *http.Request) (context.Context, gomcp.Principal, error) {
	key := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if key == "" {
		if bearer, ok := gomcp.BearerToken(r); ok {
			key = bearer
		}
	}
	if key == "" {
		return nil, gomcp.Principal{}, gomcp.Unauthorized("Missing X-API-Key header")
	}
	client := &Client{BaseURL: g.APIURL, APIKey: key, HTTP: g.HTTP, PollEvery: g.PollEvery}
	client.AgentName = g.agentFor(r, key)

	user, err := g.user(r.Context(), client, key)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden) {
			return nil, gomcp.Principal{}, gomcp.Forbidden("Invalid API key")
		}
		return nil, gomcp.Principal{}, &gomcp.AuthError{Status: http.StatusBadGateway, Message: "TaskAI API unavailable"}
	}

	raw := r.Header.Get("X-Project-ID")
	if raw == "" {
		raw = r.URL.Query().Get("project_id")
	}
	var projects []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			projects = append(projects, p)
		}
	}
	subject := ""
	if id, ok := user["id"]; ok {
		subject = strings.Trim(jsonString(id), `"`)
	}
	ctx := context.WithValue(r.Context(), sessionKey{}, &session{client: client, user: user, projects: projects})
	// TaskAI API keys carry the person's full access.
	return ctx, gomcp.Principal{Subject: subject, Scopes: []string{gomcp.ScopeRead, gomcp.ScopeWrite}}, nil
}

func (g *Gateway) user(ctx context.Context, client *Client, key string) (map[string]any, error) {
	g.mu.Lock()
	if g.users == nil {
		g.users = map[string]cachedUser{}
	}
	cached, ok := g.users[hashKey(key)]
	g.mu.Unlock()
	if ok && time.Now().Before(cached.until) {
		return cached.user, nil
	}
	var user map[string]any
	if err := client.call(ctx, http.MethodGet, "/api/me", nil, &user); err != nil {
		return nil, err
	}
	g.mu.Lock()
	g.users[hashKey(key)] = cachedUser{user: user, until: time.Now().Add(g.CacheTTL)}
	g.mu.Unlock()
	return user, nil
}

// agentFor names the calling agent from, in order: the X-Agent-Name header,
// the client name in an initialize request, a known User-Agent, or what this
// key was last seen as (kept across restarts).
func (g *Gateway) agentFor(r *http.Request, key string) string {
	name := ""
	if h := r.Header.Get("X-Agent-Name"); strings.TrimSpace(h) != "" {
		name = normalizeAgent(h)
	}
	if name == "" {
		name = initializeClientName(r)
	}
	if name == "" {
		ua := strings.ToLower(r.Header.Get("User-Agent"))
		for key, display := range agentNames {
			if strings.Contains(ua, key) {
				name = display
				break
			}
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.agents == nil {
		g.agents = g.loadAgents()
	}
	h := hashKey(key)
	if name == "" {
		return g.agents[h]
	}
	if g.agents[h] != name {
		g.agents[h] = name
		g.saveAgents()
	}
	return name
}

// initializeClientName reads clientInfo.name from an initialize request,
// leaving the body readable for the protocol handler.
func initializeClientName(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || !bytes.Contains(body, []byte(`"initialize"`)) {
		return ""
	}
	type msg struct {
		Method string `json:"method"`
		Params struct {
			ClientInfo struct {
				Name string `json:"name"`
			} `json:"clientInfo"`
		} `json:"params"`
	}
	var batch []msg
	if err := json.Unmarshal(body, &batch); err != nil {
		var one msg
		if json.Unmarshal(body, &one) != nil {
			return ""
		}
		batch = []msg{one}
	}
	for _, m := range batch {
		if m.Method == "initialize" && m.Params.ClientInfo.Name != "" {
			return normalizeAgent(m.Params.ClientInfo.Name)
		}
	}
	return ""
}

func (g *Gateway) loadAgents() map[string]string {
	out := map[string]string{}
	if g.AgentsFile == "" {
		return out
	}
	if b, err := os.ReadFile(g.AgentsFile); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func (g *Gateway) saveAgents() {
	if g.AgentsFile == "" {
		return
	}
	if b, err := json.Marshal(g.agents); err == nil {
		_ = os.WriteFile(g.AgentsFile, b, 0o600) // best effort
	}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
