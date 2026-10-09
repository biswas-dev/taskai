package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAPI records what the gateway sends to TaskAI.
type fakeAPI struct {
	mu       sync.Mutex
	meCalls  int
	requests []recorded
	polls    int
}

type recorded struct {
	Method, Path, Query, Auth, Agent string
	Body                             map[string]any
}

func (f *fakeAPI) last(path string) recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Path == path {
			return f.requests[i]
		}
	}
	return recorded{}
}

func (f *fakeAPI) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		f.mu.Lock()
		f.requests = append(f.requests, recorded{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("X-Agent-Name"), body})
		f.mu.Unlock()
		reply := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("Authorization") != "ApiKey good-key" {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/me":
			f.mu.Lock()
			f.meCalls++
			f.mu.Unlock()
			reply(map[string]any{"id": 7, "email": "me@example.com", "is_admin": false})
		case r.URL.Path == "/api/projects/3/tasks" && r.Method == "GET":
			// Like the real API: a bare array, whatever the query says.
			tasks := []any{}
			for i := 1; i <= 120; i++ {
				status := "todo"
				if i%3 == 0 {
					status = "done"
				}
				tasks = append(tasks, map[string]any{"id": 10 + i, "task_number": i, "title": fmt.Sprintf("Task %d", i), "status": status, "priority": "high", "swim_lane_name": "To Do", "assignee_name": "Me", "description": "long text"})
			}
			tasks[3].(map[string]any)["title"] = "Ship it"
			reply(tasks)
		case r.URL.Path == "/api/projects":
			reply([]any{map[string]any{"id": 2, "name": "A", "owner_id": 1}, map[string]any{"id": 3, "name": "B", "owner_id": 1}})
		case r.URL.Path == "/api/tasks/11/comments":
			reply([]any{map[string]any{"id": 1, "task_id": 11, "user_id": 7, "user_name": "Me", "comment": "Looks good", "created_at": "now"}})
		case r.URL.Path == "/api/tasks/11" && r.Method == "PATCH":
			reply(map[string]any{"id": 11, "task_number": 4, "title": "Ship it", "status": body["status"], "priority": "high", "description": "long text"})
		case r.URL.Path == "/api/tasks/404" && r.Method == "PATCH":
			http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		case r.URL.Path == "/api/wiki/search":
			reply(map[string]any{"total": 1, "results": []any{map[string]any{"page_id": 5, "page_title": "Arch", "headings_path": "Intro", "snippet": "…", "rank": 0.9}}})
		case r.URL.Path == "/draw/api/new":
			reply(map[string]any{"id": "abc", "edit_url": "/draw/abc/edit", "view_url": "/draw/abc"})
		case r.URL.Path == "/api/projects/3/drawings" && r.Method == "POST":
			reply(map[string]any{"id": 1})
		case r.URL.Path == "/api/wiki/pages/5/annotations":
			reply([]any{
				map[string]any{"id": 1, "page_id": 5, "selected_text": "a", "color": "yellow", "resolved": true, "comments": []any{1, 2}},
				map[string]any{"id": 2, "page_id": 5, "selected_text": "b", "color": "red", "resolved": false},
			})
		case r.URL.Path == "/api/wiki/pages/5/pdf" && r.Method == "POST":
			reply(map[string]any{"job_id": "j1"})
		case r.URL.Path == "/api/wiki/pages/5/pdf/j1":
			f.mu.Lock()
			f.polls++
			done := f.polls >= 2
			f.mu.Unlock()
			if !done {
				reply(map[string]any{"status": "running"})
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="arch.pdf"`)
			_, _ = w.Write([]byte("%PDF-1.7"))
		case r.URL.Path == "/api/wiki/pages/5/markdown":
			w.Header().Set("Content-Disposition", `attachment; filename="arch.md"`)
			_, _ = io.WriteString(w, "# Arch")
		case r.URL.Path == "/api/wiki/pages/9" && r.Method == "PATCH":
			reply(map[string]any{"id": 9, "title": "Moved", "slug": "moved", "parent_id": body["parent_id"], "updated_at": "now", "position": 2})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	})
}

func setup(t *testing.T) (*fakeAPI, http.Handler) {
	t.Helper()
	api := &fakeAPI{}
	upstream := httptest.NewServer(api.handler(t))
	t.Cleanup(upstream.Close)
	g := &Gateway{APIURL: upstream.URL, HTTP: upstream.Client(), CacheTTL: time.Minute, AgentsFile: filepath.Join(t.TempDir(), "agents.json"), PollEvery: time.Millisecond}
	return api, newHandler(g, slog.New(slog.DiscardHandler))
}

func rpc(t *testing.T, h http.Handler, headers map[string]string, method string, params any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

var key = map[string]string{"X-API-Key": "good-key"}

func call(t *testing.T, h http.Handler, headers map[string]string, tool string, args any) map[string]any {
	t.Helper()
	code, res := rpc(t, h, headers, "tools/call", map[string]any{"name": tool, "arguments": args})
	result, ok := res["result"].(map[string]any)
	if code != 200 || !ok {
		t.Fatalf("%s: %d %v", tool, code, res)
	}
	return result
}

func text(r map[string]any) string {
	return r["content"].([]any)[0].(map[string]any)["text"].(string)
}

func TestAuthAndHealth(t *testing.T) {
	api, h := setup(t)
	if code, res := rpc(t, h, nil, "tools/list", nil); code != 401 || res["error"] != "Missing X-API-Key header" {
		t.Fatalf("missing key: %d %v", code, res)
	}
	if code, res := rpc(t, h, map[string]string{"X-API-Key": "bad"}, "tools/list", nil); code != 403 || res["error"] != "Invalid API key" {
		t.Fatalf("bad key: %d %v", code, res)
	}
	for _, hdr := range []map[string]string{key, {"Authorization": "Bearer good-key"}} {
		if code, _ := rpc(t, h, hdr, "tools/list", nil); code != 200 {
			t.Fatalf("valid key %v: %d", hdr, code)
		}
	}
	if api.meCalls != 1 {
		t.Fatalf("the key check should be cached, got %d /api/me calls", api.meCalls)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"service":"taskai-mcp"`) {
		t.Fatalf("health: %d %s", rec.Code, rec.Body.String())
	}
}

func TestToolListMatchesTheNodeServer(t *testing.T) {
	_, h := setup(t)
	_, res := rpc(t, h, key, "tools/list", nil)
	var names []string
	for _, tool := range res["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	want := strings.Fields(`get_me list_projects get_project list_swim_lanes create_swim_lane update_swim_lane list_tasks get_task
		create_task update_task list_comments add_comment update_comment delete_comment list_project_drawings create_drawing
		save_drawing get_drawing search_wiki reindex_wiki list_wiki_pages get_wiki_page get_wiki_page_content
		list_wiki_annotations create_wiki_annotation update_wiki_annotation delete_wiki_annotation
		create_wiki_annotation_comment update_wiki_annotation_comment delete_wiki_annotation_comment create_wiki_page
		update_wiki_page_content update_wiki_page_title move_wiki_page download_wiki_pdf download_wiki_markdown
		autocomplete_wiki_pages get_version list_milestones create_milestone update_milestone get_milestone_progress
		add_dependency remove_dependency health_check list_project_members add_project_member get_wiki_sharing update_wiki_sharing`)
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	for _, n := range want {
		if !have[n] {
			t.Errorf("missing tool %s", n)
		}
	}
	if len(names) != len(want) {
		t.Errorf("have %d tools, want %d", len(names), len(want))
	}
}

func TestTasksMinimalVerboseAndPartialUpdates(t *testing.T) {
	api, h := setup(t)
	r := call(t, h, key, "list_tasks", map[string]any{"project_id": "3", "query": "ship"})
	if strings.Contains(text(r), "long text") || !strings.Contains(text(r), `"title":"Ship it"`) || !strings.Contains(text(r), `"total":1`) {
		t.Fatalf("search filters client-side: %s", text(r))
	}
	r = call(t, h, key, "list_tasks", map[string]any{"project_id": "3", "status": "done", "page": 2, "limit": 10})
	sc := r["structuredContent"].(map[string]any)
	tasks := sc["tasks"].([]any)
	if sc["total"] != float64(40) || len(tasks) != 10 || tasks[0].(map[string]any)["task_number"] != float64(33) {
		t.Fatalf("status filter and paging: total=%v n=%d first=%v", sc["total"], len(tasks), tasks[0])
	}
	r = call(t, h, key, "list_tasks", map[string]any{"project_id": "3", "verbose": true})
	if sc := r["structuredContent"].(map[string]any); sc["total"] != float64(120) || len(sc["tasks"].([]any)) != 50 || !strings.Contains(text(r), "long text") {
		t.Fatalf("verbose default page: %v", sc["total"])
	}
	r = call(t, h, key, "list_projects", map[string]any{})
	if text(r) != `{"projects":[{"id":2,"name":"A"},{"id":3,"name":"B"}],"total":2}` {
		t.Fatalf("projects from a bare array: %s", text(r))
	}
	r = call(t, h, key, "list_comments", map[string]any{"task_id": "11"})
	if !strings.Contains(text(r), `"content":"Looks good"`) || !strings.Contains(text(r), `"author_id":7`) {
		t.Fatalf("comments: %s", text(r))
	}
	call(t, h, key, "update_task", map[string]any{"task_id": "11", "status": "done"})
	if body := api.last("/api/tasks/11").Body; len(body) != 1 || body["status"] != "done" {
		t.Fatalf("only the given fields are sent: %v", body)
	}
	call(t, h, key, "update_task", map[string]any{"task_id": "11", "assigned_to": "12", "due_date": "2026-10-20"})
	if body := api.last("/api/tasks/11").Body; body["assignee_id"] != float64(12) || body["assigned_to"] != nil || body["due_date"] != "2026-10-20" {
		t.Fatalf("assignee is sent as the API's numeric assignee_id: %v", body)
	}
	if r := call(t, h, key, "update_task", map[string]any{"task_id": "11", "priority": "urgent"}); r["isError"] != true {
		t.Fatal("an unknown priority should be refused")
	}
	r = call(t, h, key, "update_task", map[string]any{"task_id": "404", "status": "done"})
	if r["isError"] != true || !strings.Contains(text(r), "TaskAI API error 404") {
		t.Fatalf("API errors reach the agent: %v", r)
	}
}

func TestAgentAttributionFromInitialize(t *testing.T) {
	api, h := setup(t)
	rpc(t, h, key, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "claude-code", "version": "2"}})
	// A later request without any hint is still attributed to the agent.
	call(t, h, key, "update_task", map[string]any{"task_id": "11", "status": "in_progress"})
	if got := api.last("/api/tasks/11").Agent; got != "Claude Code" {
		t.Fatalf("agent name forwarded: %q", got)
	}
	call(t, h, map[string]string{"X-API-Key": "good-key", "X-Agent-Name": "My Bot"}, "update_task", map[string]any{"task_id": "11", "status": "todo"})
	if got := api.last("/api/tasks/11").Agent; got != "My Bot" {
		t.Fatalf("explicit header wins: %q", got)
	}
}

func TestWikiDefaultsToProjectScope(t *testing.T) {
	api, h := setup(t)
	scoped := map[string]string{"X-API-Key": "good-key", "X-Project-ID": "3, 4"}
	r := call(t, h, scoped, "search_wiki", map[string]any{"query": "auth"})
	body := api.last("/api/wiki/search").Body
	if body["mode"] != "hybrid" || body["limit"] != float64(10) || fmt.Sprint(body["project_ids"]) != "[3 4]" || !strings.Contains(jsonString(body["project_ids"]), "[3,4]") || strings.Contains(text(r), "rank") {
		t.Fatalf("scoped search: %v %s", body, text(r))
	}
	call(t, h, scoped, "search_wiki", map[string]any{"query": "auth", "project_id": "9"})
	if body := api.last("/api/wiki/search").Body; body["project_id"] != float64(9) || body["project_ids"] != nil {
		t.Fatalf("explicit project wins: %v", body)
	}
	r = call(t, h, key, "list_wiki_pages", map[string]any{})
	if r["isError"] != true || !strings.Contains(text(r), "project_id is required") {
		t.Fatalf("no scope, no project: %v", r)
	}
}

func TestDrawingsAnnotationsAndPages(t *testing.T) {
	api, h := setup(t)
	r := call(t, h, key, "create_drawing", map[string]any{"project_id": "3", "title": "Flow"})
	if !strings.Contains(text(r), `"shortcode":"[draw:abc:edit:m]"`) {
		t.Fatalf("drawing: %s", text(r))
	}
	if auth := api.last("/draw/api/new").Auth; auth != "" {
		t.Fatalf("go-draw must not receive the API key: %q", auth)
	}
	if body := api.last("/api/projects/3/drawings").Body; body["draw_id"] != "abc" {
		t.Fatalf("drawing registered: %v", body)
	}
	r = call(t, h, key, "list_wiki_annotations", map[string]any{"page_id": "5", "include_resolved": false})
	if !strings.Contains(text(r), `"selected_text":"b"`) || strings.Contains(text(r), `"selected_text":"a"`) {
		t.Fatalf("resolved filtered: %s", text(r))
	}
	r = call(t, h, key, "list_wiki_annotations", map[string]any{"page_id": "5"})
	if !strings.Contains(text(r), `"comments_count":2`) {
		t.Fatalf("comment counts: %s", text(r))
	}
	r = call(t, h, key, "move_wiki_page", map[string]any{"page_id": "9", "parent_id": nil, "position": 2})
	if body := api.last("/api/wiki/pages/9").Body; body["parent_id"] != nil || body["position"] != float64(2) {
		t.Fatalf("move to top level: %v", body)
	}
	if strings.Contains(text(r), "position") {
		t.Fatalf("minimal page: %s", text(r))
	}
}

func TestDownloads(t *testing.T) {
	_, h := setup(t)
	r := call(t, h, key, "download_wiki_pdf", map[string]any{"page_id": "5"})
	blocks := r["content"].([]any)
	res := blocks[1].(map[string]any)["resource"].(map[string]any)
	if !strings.Contains(text(r), "arch.pdf") || res["mimeType"] != "application/pdf" || res["blob"] != "JVBERi0xLjc=" {
		t.Fatalf("pdf: %v", blocks)
	}
	r = call(t, h, key, "download_wiki_markdown", map[string]any{"page_id": "5"})
	if text(r) != "Markdown file: arch.md\n\n# Arch" {
		t.Fatalf("markdown: %q", text(r))
	}
}
