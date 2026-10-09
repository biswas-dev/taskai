package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	gomcp "github.com/anchoo2kewl/go-mcp"
)

// ---- shaping ----

// pick keeps only the named fields of a JSON object.
func pick(v any, keys ...string) map[string]any {
	m, _ := v.(map[string]any)
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if val, ok := m[k]; ok {
			out[k] = val
		} else {
			out[k] = nil
		}
	}
	return out
}

func pickEach(v any, keys ...string) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		out = append(out, pick(item, keys...))
	}
	return out
}

// items reads a list response, whether the API sent a bare array or an
// envelope such as {"projects": [...]}.
func items(v any, key string) []any {
	if list, ok := v.([]any); ok {
		return list
	}
	list, _ := field(v, key).([]any)
	if list == nil {
		list = []any{}
	}
	return list
}

// paginate returns one page of a list. A limit of 0 means everything.
func paginate(list []any, page, limit int) []any {
	if limit <= 0 {
		return list
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * limit
	if start >= len(list) {
		return []any{}
	}
	return list[start:min(start+limit, len(list))]
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return strings.Trim(jsonString(x), `"`)
	}
}

// matchesTask is the list_tasks filter: the API returns every task in the
// project, whatever query parameters it is sent.
func matchesTask(t any, status, query string) bool {
	if status != "" && !strings.EqualFold(asString(field(t, "status")), status) {
		return false
	}
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.TrimPrefix(q, "#") == asString(field(t, "task_number")) {
		return true
	}
	return strings.Contains(strings.ToLower(asString(field(t, "title"))), q) ||
		strings.Contains(strings.ToLower(asString(field(t, "description"))), q)
}

// minimizeComment accepts both the current ("comment", "user_id") and the
// older ("content", "author_id") field names.
func minimizeComment(c any) map[string]any {
	first := func(keys ...string) any {
		for _, k := range keys {
			if v := field(c, k); v != nil {
				return v
			}
		}
		return nil
	}
	return map[string]any{"id": field(c, "id"), "content": first("comment", "content"), "author_id": first("user_id", "author_id"),
		"author_name": first("user_name", "author_name"), "created_at": field(c, "created_at")}
}

func field(v any, key string) any {
	m, _ := v.(map[string]any)
	return m[key]
}

var (
	taskFields       = []string{"id", "task_number", "title", "status", "priority", "swim_lane_name", "assignee_name"}
	projectFields    = []string{"id", "name"}
	laneFields       = []string{"id", "name", "status_category"}
	wikiPageFields   = []string{"id", "title", "slug", "parent_id", "updated_at"}
	wikiBlockFields  = []string{"page_id", "page_title", "headings_path", "snippet"}
	milestoneFields  = []string{"id", "name", "status", "task_count", "target_date"}
	annotationFields = []string{"id", "page_id", "selected_text", "color", "resolved"}
)

func minimizeAnnotation(a any) map[string]any {
	out := pick(a, annotationFields...)
	comments, _ := field(a, "comments").([]any)
	out["comments_count"] = len(comments)
	return out
}

func minimizeAnnotations(list any) []map[string]any {
	items, _ := list.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		out = append(out, minimizeAnnotation(a))
	}
	return out
}

// listOf wraps a list so it can be returned where an object is expected.
func shaped(verbose bool, full any, minimal func(any) any) any {
	if verbose {
		return full
	}
	return minimal(full)
}

func sess(ctx context.Context) (*session, *Client) {
	s := sessionFrom(ctx)
	return s, s.client
}

// defaultProject applies the X-Project-ID scope when no project is given.
func defaultProject(ctx context.Context, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if s := sessionFrom(ctx); s != nil && len(s.projects) > 0 {
		return s.projects[0], nil
	}
	return "", errors.New("project_id is required")
}

// ---- inputs ----

type verboseIn struct {
	Verbose bool `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type pageIn struct {
	Page    int  `json:"page,omitempty"`
	Limit   int  `json:"limit,omitempty"`
	Verbose bool `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type projectIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID"`
	Verbose   bool   `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type optionalProjectIn struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"Project ID (default: the X-Project-ID scope)"`
	Verbose   bool   `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type createLaneIn struct {
	ProjectID      string `json:"project_id" jsonschema:"Project ID"`
	Name           string `json:"name" jsonschema:"Lane name (max 50 chars)"`
	StatusCategory string `json:"status_category" jsonschema:"todo, in_progress or done: controls task status when moved into this lane"`
	Color          string `json:"color,omitempty" jsonschema:"Hex color, e.g. #5e6ad2 (default: #6B7280 gray)"`
	Position       *int   `json:"position,omitempty" jsonschema:"Position (column order). Defaults to 0."`
	Verbose        bool   `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type updateLaneIn struct {
	SwimLaneID     int64   `json:"swim_lane_id" jsonschema:"Swim lane ID"`
	Name           *string `json:"name,omitempty" jsonschema:"New name (max 50 chars)"`
	Color          *string `json:"color,omitempty" jsonschema:"New hex color, e.g. #f59e0b"`
	Position       *int    `json:"position,omitempty" jsonschema:"New position (must be unique within the project)"`
	StatusCategory *string `json:"status_category,omitempty" jsonschema:"New status category: todo, in_progress or done"`
	Verbose        bool    `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type listTasksIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID"`
	Query     string `json:"query,omitempty" jsonschema:"Search query"`
	Status    string `json:"status,omitempty" jsonschema:"Filter by status (e.g. todo, in_progress, done)"`
	Page      int    `json:"page,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Verbose   bool   `json:"verbose,omitempty" jsonschema:"Return full task details (default: false)"`
}

type getTaskIn struct {
	ProjectID  string `json:"project_id" jsonschema:"Project ID"`
	TaskNumber int    `json:"task_number" jsonschema:"Task number within the project (e.g. 1, 2, 3)"`
	Verbose    bool   `json:"verbose,omitempty"`
}

type taskFieldsIn struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty" jsonschema:"Task description (markdown)"`
	Status      *string `json:"status,omitempty" jsonschema:"Task status, e.g. todo, in_progress, done"`
	Priority    *string `json:"priority,omitempty" jsonschema:"low, medium, high or critical"`
	AssignedTo  *string `json:"assigned_to,omitempty" jsonschema:"User ID to assign"`
	SwimLaneID  *int64  `json:"swim_lane_id,omitempty" jsonschema:"Swim lane ID (use list_swim_lanes to get valid IDs)"`
	DueDate     *string `json:"due_date,omitempty" jsonschema:"YYYY-MM-DD"`
	MilestoneID *int64  `json:"milestone_id,omitempty" jsonschema:"Milestone ID (use list_milestones)"`
	SprintID    *int64  `json:"sprint_id,omitempty" jsonschema:"Sprint ID"`
}

// taskBody is what the TaskAI API reads. It takes the assignee as a numeric
// assignee_id; the tools keep accepting assigned_to as before.
type taskBody struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	Priority    *string `json:"priority,omitempty"`
	AssigneeID  *int64  `json:"assignee_id,omitempty"`
	SwimLaneID  *int64  `json:"swim_lane_id,omitempty"`
	DueDate     *string `json:"due_date,omitempty"`
	MilestoneID *int64  `json:"milestone_id,omitempty"`
	SprintID    *int64  `json:"sprint_id,omitempty"`
}

func (f taskFieldsIn) body() (taskBody, error) {
	b := taskBody{Title: f.Title, Description: f.Description, Status: f.Status, Priority: f.Priority, SwimLaneID: f.SwimLaneID, DueDate: f.DueDate, MilestoneID: f.MilestoneID, SprintID: f.SprintID}
	if f.Priority != nil {
		if err := oneOf("priority", *f.Priority, "low", "medium", "high", "critical"); err != nil {
			return b, err
		}
	}
	if f.AssignedTo != nil && strings.TrimSpace(*f.AssignedTo) != "" {
		id, err := strconv.ParseInt(strings.TrimSpace(*f.AssignedTo), 10, 64)
		if err != nil {
			return b, errors.New("assigned_to must be a user ID")
		}
		b.AssigneeID = &id
	}
	return b, nil
}

// ids converts project IDs to the integers the API expects.
func ids(list ...string) ([]int64, error) {
	out := make([]int64, 0, len(list))
	for _, s := range list {
		id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("project ID %q is not a number", s)
		}
		out = append(out, id)
	}
	return out, nil
}

type createTaskIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID"`
	taskFieldsIn
	Verbose bool `json:"verbose,omitempty"`
}

type updateTaskIn struct {
	TaskID string `json:"task_id" jsonschema:"Task ID"`
	taskFieldsIn
	Verbose bool `json:"verbose,omitempty"`
}

type taskIDIn struct {
	TaskID  string `json:"task_id" jsonschema:"Task ID"`
	Verbose bool   `json:"verbose,omitempty" jsonschema:"Return full details (default: false)"`
}

type addCommentIn struct {
	TaskID  string `json:"task_id" jsonschema:"Task ID"`
	Content string `json:"content" jsonschema:"Comment text (markdown)"`
	Verbose bool   `json:"verbose,omitempty"`
}

type updateCommentIn struct {
	CommentID string `json:"comment_id" jsonschema:"Comment ID"`
	Content   string `json:"content" jsonschema:"New comment text"`
	Verbose   bool   `json:"verbose,omitempty"`
}

type commentIDIn struct {
	CommentID string `json:"comment_id" jsonschema:"Comment ID"`
}

type createDrawingIn struct {
	ProjectID string         `json:"project_id" jsonschema:"Project ID to register the drawing with"`
	Title     string         `json:"title,omitempty" jsonschema:"Drawing title (default: Untitled)"`
	Scene     map[string]any `json:"scene,omitempty" jsonschema:"Scene JSON: {version:1, elements:[...]}. Elements: rect/ellipse (x,y,w,h,text), arrow/line (x,y,x2,y2), text (x,y,w,h,text,fontSize), pencil (pts:[{x,y}]). Fields: strokeColor, fillColor, opacity(0-100), strokeWidth(1-4), angle(radians)."`
	Verbose   bool           `json:"verbose,omitempty"`
}

type saveDrawingIn struct {
	DrawID  string         `json:"draw_id" jsonschema:"Drawing ID (from create_drawing)"`
	Title   string         `json:"title" jsonschema:"Drawing title"`
	Scene   map[string]any `json:"scene" jsonschema:"Scene JSON: {version:1, elements:[...]}"`
	Verbose bool           `json:"verbose,omitempty"`
}

type drawIDIn struct {
	DrawID  string `json:"draw_id" jsonschema:"Drawing ID"`
	Verbose bool   `json:"verbose,omitempty"`
}

type searchWikiIn struct {
	Query       string   `json:"query" jsonschema:"Search query"`
	ProjectID   string   `json:"project_id,omitempty" jsonschema:"Search a single project by ID"`
	ProjectIDs  []string `json:"project_ids,omitempty" jsonschema:"Search several projects by ID (default: the X-Project-ID scope)"`
	Limit       int      `json:"limit,omitempty" jsonschema:"Max results (default: 10, max: 100)"`
	RecencyDays int      `json:"recency_days,omitempty" jsonschema:"Only return pages updated in the last N days"`
	Mode        string   `json:"mode,omitempty" jsonschema:"hybrid (default), fts, semantic or keyword"`
	Verbose     bool     `json:"verbose,omitempty"`
}

type pageIDIn struct {
	PageID  string `json:"page_id" jsonschema:"Wiki page ID"`
	Verbose bool   `json:"verbose,omitempty"`
}

type listAnnotationsIn struct {
	PageID          string `json:"page_id" jsonschema:"Wiki page ID"`
	IncludeResolved *bool  `json:"include_resolved,omitempty" jsonschema:"Include resolved annotations (default: true)"`
	Verbose         bool   `json:"verbose,omitempty"`
}

type createAnnotationIn struct {
	PageID       string `json:"page_id" jsonschema:"Wiki page ID"`
	StartOffset  int    `json:"start_offset" jsonschema:"Start character offset in rendered wiki text"`
	EndOffset    int    `json:"end_offset" jsonschema:"End character offset in rendered wiki text; must be greater than start_offset"`
	SelectedText string `json:"selected_text" jsonschema:"Exact highlighted text"`
	Color        string `json:"color,omitempty" jsonschema:"yellow (default), blue, green or red"`
	Comment      string `json:"comment,omitempty" jsonschema:"Optional initial comment (max 5000 chars)"`
	Verbose      bool   `json:"verbose,omitempty"`
}

type updateAnnotationIn struct {
	AnnotationID string  `json:"annotation_id" jsonschema:"Wiki annotation ID"`
	Color        *string `json:"color,omitempty" jsonschema:"New highlight color: yellow, blue, green or red"`
	Resolved     *bool   `json:"resolved,omitempty" jsonschema:"Set resolved/unresolved state"`
	Verbose      bool    `json:"verbose,omitempty"`
}

type annotationIDIn struct {
	AnnotationID string `json:"annotation_id" jsonschema:"Wiki annotation ID"`
}

type annotationCommentIn struct {
	AnnotationID    string `json:"annotation_id" jsonschema:"Wiki annotation ID"`
	Content         string `json:"content" jsonschema:"Comment body (max 5000 chars)"`
	ParentCommentID *int64 `json:"parent_comment_id,omitempty" jsonschema:"Parent comment ID for threaded replies"`
	Verbose         bool   `json:"verbose,omitempty"`
}

type createPageIn struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"Project ID (default: the X-Project-ID scope)"`
	Title     string `json:"title" jsonschema:"Page title"`
	ParentID  string `json:"parent_id,omitempty" jsonschema:"Parent wiki page ID to nest this page under (omit for a top-level page; max 6 levels deep)"`
	Content   string `json:"content,omitempty" jsonschema:"Initial page content (markdown). References: [^label] (e.g. [^1] or [^usc102]) inline and [^label]: text definitions, numbered in order of first citation"`
	Verbose   bool   `json:"verbose,omitempty"`
}

type pageContentIn struct {
	PageID  string `json:"page_id" jsonschema:"Wiki page ID"`
	Content string `json:"content" jsonschema:"New page content (markdown). References: [^label] inline and [^label]: text definitions, numbered in order of first citation"`
	Verbose bool   `json:"verbose,omitempty"`
}

type pageTitleIn struct {
	PageID  string `json:"page_id" jsonschema:"Wiki page ID"`
	Title   string `json:"title" jsonschema:"New page title (1-500 chars)"`
	Verbose bool   `json:"verbose,omitempty"`
}

type movePageIn struct {
	PageID   string  `json:"page_id" jsonschema:"Wiki page ID to move"`
	ParentID *string `json:"parent_id" jsonschema:"New parent page ID, or null to move to the top level"`
	Position *int    `json:"position,omitempty" jsonschema:"Optional 0-based sort position among siblings. Defaults to last."`
	Verbose  bool    `json:"verbose,omitempty"`
}

type autocompleteIn struct {
	Query     string `json:"query" jsonschema:"Search query for page title"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"Filter by project ID (default: the X-Project-ID scope)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max results (default: 10, max: 50)"`
	Verbose   bool   `json:"verbose,omitempty"`
}

type createMilestoneIn struct {
	ProjectID   string  `json:"project_id" jsonschema:"Project ID"`
	Name        string  `json:"name" jsonschema:"Milestone name"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty" jsonschema:"Hex color (default: #5e6ad2)"`
	TargetDate  *string `json:"target_date,omitempty" jsonschema:"YYYY-MM-DD"`
	Status      *string `json:"status,omitempty" jsonschema:"active (default), completed or cancelled"`
}

type updateMilestoneIn struct {
	MilestoneID string  `json:"milestone_id" jsonschema:"Milestone ID"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty" jsonschema:"New hex color"`
	TargetDate  *string `json:"target_date,omitempty" jsonschema:"YYYY-MM-DD"`
	Status      *string `json:"status,omitempty" jsonschema:"active, completed or cancelled"`
	SortOrder   *int    `json:"sort_order,omitempty" jsonschema:"Display order"`
}

type milestoneIDIn struct {
	MilestoneID string `json:"milestone_id" jsonschema:"Milestone ID"`
	Verbose     bool   `json:"verbose,omitempty"`
}

type addDependencyIn struct {
	TaskID         string `json:"task_id" jsonschema:"Task ID (the task that depends on another)"`
	DependsOnID    int64  `json:"depends_on_id" jsonschema:"ID of the task it depends on (the blocker)"`
	DependencyType string `json:"dependency_type,omitempty" jsonschema:"blocks (default) or related"`
}

type dependencyIDIn struct {
	DependencyID string `json:"dependency_id" jsonschema:"Dependency ID to remove"`
}

func oneOf(field, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of %s", field, strings.Join(allowed, ", "))
}

// ---- registration ----

func registerTools(srv *gomcp.Server) {
	registerMembershipTools(srv)
	read := func(name, desc string) gomcp.Tool { return gomcp.Tool{Name: name, Description: desc} }
	write := func(name, desc string) gomcp.Tool { return gomcp.Tool{Name: name, Description: desc, Write: true} }
	update := func(name, desc string) gomcp.Tool {
		return gomcp.Tool{Name: name, Description: desc, Write: true, Destructive: true, Idempotent: true}
	}

	gomcp.Register(srv, read("get_me", "Get current authenticated user info"),
		func(ctx context.Context, _ verboseIn) (map[string]any, error) {
			s, _ := sess(ctx)
			return s.user, nil
		})

	gomcp.Register(srv, read("list_projects", "List all projects (minimal fields by default, use verbose=true for full details)"),
		func(ctx context.Context, in pageIn) (any, error) {
			_, c := sess(ctx)
			res, err := c.get(ctx, "/api/projects")
			if err != nil {
				return nil, err
			}
			all := items(res, "projects")
			page := paginate(all, in.Page, in.Limit)
			if in.Verbose {
				return map[string]any{"projects": page, "total": len(all)}, nil
			}
			return map[string]any{"projects": pickEach(page, projectFields...), "total": len(all)}, nil
		})

	gomcp.Register(srv, read("get_project", "Get project details by ID"),
		func(ctx context.Context, in projectIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, "/api/projects/"+esc(in.ProjectID))
		})

	gomcp.Register(srv, read("list_swim_lanes", "List swim lanes (columns) for a project. Each lane has a status_category (todo, in_progress, done) that determines task status. Returns minimal fields by default."),
		func(ctx context.Context, in projectIn) (any, error) {
			_, c := sess(ctx)
			res, err := c.get(ctx, "/api/projects/"+esc(in.ProjectID)+"/swim-lanes")
			return shaped(in.Verbose, res, func(v any) any { return pickEach(v, laneFields...) }), err
		})

	gomcp.Register(srv, write("create_swim_lane", "Create a new swim lane (column) in a project. Max 6 lanes per project."),
		func(ctx context.Context, in createLaneIn) (any, error) {
			if err := oneOf("status_category", in.StatusCategory, "todo", "in_progress", "done"); err != nil || in.StatusCategory == "" {
				return nil, errors.New("status_category must be one of todo, in_progress, done")
			}
			_, c := sess(ctx)
			body := map[string]any{"name": in.Name, "status_category": in.StatusCategory}
			if in.Color != "" {
				body["color"] = in.Color
			}
			if in.Position != nil {
				body["position"] = *in.Position
			}
			res, err := c.send(ctx, http.MethodPost, "/api/projects/"+esc(in.ProjectID)+"/swim-lanes", body)
			return shaped(in.Verbose, res, func(v any) any { return pick(v, laneFields...) }), err
		})

	gomcp.Register(srv, update("update_swim_lane", "Update an existing swim lane (rename, recolor, reorder, or change status category). Note: (project_id, position) is unique — to swap two lanes, first move one to an unused position."),
		func(ctx context.Context, in updateLaneIn) (any, error) {
			if in.StatusCategory != nil {
				if err := oneOf("status_category", *in.StatusCategory, "todo", "in_progress", "done"); err != nil {
					return nil, err
				}
			}
			_, c := sess(ctx)
			body := struct {
				Name           *string `json:"name,omitempty"`
				Color          *string `json:"color,omitempty"`
				Position       *int    `json:"position,omitempty"`
				StatusCategory *string `json:"status_category,omitempty"`
			}{in.Name, in.Color, in.Position, in.StatusCategory}
			res, err := c.send(ctx, http.MethodPatch, "/api/swim-lanes/"+strconv.FormatInt(in.SwimLaneID, 10), body)
			return shaped(in.Verbose, res, func(v any) any { return pick(v, laneFields...) }), err
		})

	gomcp.Register(srv, read("list_tasks", "List tasks in a project, optionally filtered by status and a search over title, description and task number. Returns a page (default 50) of minimal fields (id, task_number, title, status, priority) with the total; use page to see more and verbose=true for full task details."),
		func(ctx context.Context, in listTasksIn) (any, error) {
			_, c := sess(ctx)
			res, err := c.get(ctx, "/api/projects/"+esc(in.ProjectID)+"/tasks")
			if err != nil {
				return nil, err
			}
			matched := []any{}
			for _, t := range items(res, "tasks") {
				if matchesTask(t, in.Status, in.Query) {
					matched = append(matched, t)
				}
			}
			page, limit := in.Page, in.Limit
			if page <= 0 {
				page = 1
			}
			if limit <= 0 {
				limit = 50
			}
			list := paginate(matched, page, limit)
			out := map[string]any{"total": len(matched), "page": page, "limit": limit}
			if in.Verbose {
				out["tasks"] = list
			} else {
				out["tasks"] = pickEach(list, taskFields...)
			}
			return out, nil
		})

	gomcp.Register(srv, read("get_task", "Get a single task by its project-scoped task number"),
		func(ctx context.Context, in getTaskIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, fmt.Sprintf("/api/projects/%s/tasks/%d", esc(in.ProjectID), in.TaskNumber))
		})

	gomcp.Register(srv, write("create_task", "Create a new task in a project"),
		func(ctx context.Context, in createTaskIn) (any, error) {
			_, c := sess(ctx)
			if in.Title == nil || strings.TrimSpace(*in.Title) == "" {
				return nil, errors.New("title is required")
			}
			body, err := in.taskFieldsIn.body()
			if err != nil {
				return nil, err
			}
			res, err := c.send(ctx, http.MethodPost, "/api/projects/"+esc(in.ProjectID)+"/tasks", body)
			return shaped(in.Verbose, res, func(v any) any { return pick(v, taskFields...) }), err
		})

	gomcp.Register(srv, update("update_task", "Update an existing task"),
		func(ctx context.Context, in updateTaskIn) (any, error) {
			_, c := sess(ctx)
			body, err := in.taskFieldsIn.body()
			if err != nil {
				return nil, err
			}
			res, err := c.send(ctx, http.MethodPatch, "/api/tasks/"+esc(in.TaskID), body)
			return shaped(in.Verbose, res, func(v any) any { return pick(v, taskFields...) }), err
		})

	gomcp.Register(srv, read("list_comments", "List comments on a task. Returns minimal fields by default."),
		func(ctx context.Context, in taskIDIn) (any, error) {
			_, c := sess(ctx)
			res, err := c.get(ctx, "/api/tasks/"+esc(in.TaskID)+"/comments")
			if err != nil {
				return nil, err
			}
			list := items(res, "comments")
			if in.Verbose {
				return map[string]any{"comments": list}, nil
			}
			out := make([]map[string]any, 0, len(list))
			for _, cm := range list {
				out = append(out, minimizeComment(cm))
			}
			return map[string]any{"comments": out}, nil
		})

	gomcp.Register(srv, write("add_comment", "Add a comment to a task"),
		func(ctx context.Context, in addCommentIn) (any, error) {
			_, c := sess(ctx)
			return c.send(ctx, http.MethodPost, "/api/tasks/"+esc(in.TaskID)+"/comments", map[string]string{"comment": in.Content})
		})

	gomcp.Register(srv, update("update_comment", "Update a comment. Only the comment owner, project owner, or super admin can update."),
		func(ctx context.Context, in updateCommentIn) (any, error) {
			_, c := sess(ctx)
			return c.send(ctx, http.MethodPatch, "/api/comments/"+esc(in.CommentID), map[string]string{"comment": in.Content})
		})

	gomcp.Register(srv, update("delete_comment", "Delete a comment. Only the comment owner, project owner, or super admin can delete."),
		func(ctx context.Context, in commentIDIn) (any, error) {
			_, c := sess(ctx)
			return c.send(ctx, http.MethodDelete, "/api/comments/"+esc(in.CommentID), nil)
		})

	gomcp.Register(srv, read("list_project_drawings", "List all drawings registered to a project"),
		func(ctx context.Context, in projectIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, "/api/projects/"+esc(in.ProjectID)+"/drawings")
		})

	gomcp.Register(srv, write("create_drawing", "Create a new drawing canvas and register it with a project. Optionally pre-populate with a scene. Returns the draw_id and the shortcode to embed it in wiki pages: [draw:ID:edit:m]"),
		func(ctx context.Context, in createDrawingIn) (any, error) {
			_, c := sess(ctx)
			var body any
			if in.Title != "" || in.Scene != nil {
				body = map[string]any{"title": in.Title, "scene": in.Scene}
			}
			var draw struct {
				ID      string `json:"id"`
				EditURL string `json:"edit_url"`
				ViewURL string `json:"view_url"`
			}
			// go-draw's /draw/api/new does not take the API key.
			if err := c.callAs(ctx, http.MethodPost, "/draw/api/new", body, &draw, false); err != nil {
				return nil, err
			}
			if err := c.call(ctx, http.MethodPost, "/api/projects/"+esc(in.ProjectID)+"/drawings", map[string]string{"draw_id": draw.ID}, nil); err != nil {
				return nil, err
			}
			return map[string]any{"draw_id": draw.ID, "edit_url": draw.EditURL, "view_url": draw.ViewURL, "shortcode": "[draw:" + draw.ID + ":edit:m]"}, nil
		})

	gomcp.Register(srv, update("save_drawing", "Save/update an existing drawing's scene data. Use this to programmatically set diagram content."),
		func(ctx context.Context, in saveDrawingIn) (any, error) {
			_, c := sess(ctx)
			var out any
			err := c.callAs(ctx, http.MethodPost, "/draw/"+esc(in.DrawID)+"/save", map[string]any{"title": in.Title, "scene": in.Scene}, &out, false)
			return out, err
		})

	gomcp.Register(srv, read("get_drawing", "Get a drawing's current scene data (id, title, scene JSON)"),
		func(ctx context.Context, in drawIDIn) (any, error) {
			_, c := sess(ctx)
			var out any
			err := c.callAs(ctx, http.MethodGet, "/draw/"+esc(in.DrawID)+"/data", nil, &out, false)
			return out, err
		})

	gomcp.Register(srv, read("search_wiki", "Search project wiki for architecture docs, decisions, and implementation details. Modes: 'hybrid' (default, best accuracy — combines full-text + vector similarity via reciprocal rank fusion), 'fts' (full-text ranked), 'semantic' (vector similarity — finds conceptually related content even without keyword overlap), 'keyword' (simple substring match). Scoped to the X-Project-ID projects by default."),
		func(ctx context.Context, in searchWikiIn) (any, error) {
			if err := oneOf("mode", in.Mode, "fts", "semantic", "hybrid", "keyword"); err != nil {
				return nil, err
			}
			s, c := sess(ctx)
			mode, limit := in.Mode, in.Limit
			if mode == "" {
				mode = "hybrid"
			}
			if limit <= 0 {
				limit = 10
			}
			scope := in.ProjectIDs
			if in.ProjectID != "" {
				scope = []string{in.ProjectID}
			} else if scope == nil {
				scope = s.projects
			}
			body := map[string]any{"query": in.Query, "limit": limit, "mode": mode}
			if in.RecencyDays > 0 {
				body["recency_days"] = in.RecencyDays
			}
			numeric, err := ids(scope...)
			if err != nil {
				return nil, err
			}
			if len(numeric) == 1 {
				body["project_id"] = numeric[0]
			} else if len(numeric) > 1 {
				body["project_ids"] = numeric
			}
			res, err := c.send(ctx, http.MethodPost, "/api/wiki/search", body)
			return shaped(in.Verbose, res, func(v any) any {
				return map[string]any{"results": pickEach(field(v, "results"), wikiBlockFields...), "total": field(v, "total")}
			}), err
		})

	gomcp.Register(srv, gomcp.Tool{Name: "reindex_wiki", Description: "Trigger a full re-index of all wiki pages, generating vector embeddings for semantic search. Returns immediately — re-indexing runs in background. Use after first deployment of embeddings or after a model upgrade.", Write: true, Idempotent: true},
		func(ctx context.Context, _ verboseIn) (any, error) {
			_, c := sess(ctx)
			return c.send(ctx, http.MethodPost, "/api/wiki/reindex", nil)
		})

	gomcp.Register(srv, read("list_wiki_pages", "List all wiki pages in a project (default: the X-Project-ID scope). Pages are hierarchical: parent_id is null for top-level pages, otherwise the ID of the parent page (max 6 levels deep)."),
		func(ctx context.Context, in optionalProjectIn) (any, error) {
			pid, err := defaultProject(ctx, in.ProjectID)
			if err != nil {
				return nil, err
			}
			_, c := sess(ctx)
			res, err := c.get(ctx, "/api/projects/"+esc(pid)+"/wiki/pages")
			return shaped(in.Verbose, res, func(v any) any { return pickEach(v, wikiPageFields...) }), err
		})

	gomcp.Register(srv, read("get_wiki_page", "Get a specific wiki page by ID including its full markdown content"),
		func(ctx context.Context, in pageIDIn) (any, error) {
			_, c := sess(ctx)
			var (
				page, content any
				errs          [2]error
				wg            sync.WaitGroup
			)
			wg.Add(2)
			go func() { defer wg.Done(); page, errs[0] = c.get(ctx, "/api/wiki/pages/"+esc(in.PageID)) }()
			go func() { defer wg.Done(); content, errs[1] = c.get(ctx, "/api/wiki/pages/"+esc(in.PageID)+"/content") }()
			wg.Wait()
			if err := errors.Join(errs[:]...); err != nil {
				return nil, err
			}
			out, _ := page.(map[string]any)
			if out == nil {
				out = map[string]any{}
			}
			out["content"] = field(content, "content")
			return out, nil
		})

	gomcp.Register(srv, read("get_wiki_page_content", "Get the markdown content of a wiki page"),
		func(ctx context.Context, in pageIDIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, "/api/wiki/pages/"+esc(in.PageID)+"/content")
		})

	gomcp.Register(srv, read("list_wiki_annotations", "List inline highlights and comments for a wiki page. Returns minimal fields by default; use verbose=true for offsets and full comment threads."),
		func(ctx context.Context, in listAnnotationsIn) (any, error) {
			_, c := sess(ctx)
			res, err := c.get(ctx, "/api/wiki/pages/"+esc(in.PageID)+"/annotations")
			if err != nil {
				return nil, err
			}
			list, _ := res.([]any)
			if in.IncludeResolved != nil && !*in.IncludeResolved {
				open := []any{}
				for _, a := range list {
					if field(a, "resolved") != true {
						open = append(open, a)
					}
				}
				list = open
			}
			if list == nil {
				list = []any{}
			}
			return shaped(in.Verbose, any(list), func(v any) any { return minimizeAnnotations(v) }), nil
		})

	gomcp.Register(srv, write("create_wiki_annotation", "Create an inline wiki highlight, optionally with an initial comment. Offsets are zero-based character offsets in the rendered wiki text."),
		func(ctx context.Context, in createAnnotationIn) (any, error) {
			if in.StartOffset < 0 || in.EndOffset <= in.StartOffset {
				return nil, errors.New("end_offset must be greater than start_offset, and offsets must be zero or more")
			}
			if in.SelectedText == "" {
				return nil, errors.New("selected_text is required")
			}
			if len(in.Comment) > 5000 {
				return nil, errors.New("comment must be at most 5000 characters")
			}
			if err := oneOf("color", in.Color, "yellow", "blue", "green", "red"); err != nil {
				return nil, err
			}
			color := in.Color
			if color == "" {
				color = "yellow"
			}
			_, c := sess(ctx)
			body := map[string]any{"start_offset": in.StartOffset, "end_offset": in.EndOffset, "selected_text": in.SelectedText, "color": color}
			if in.Comment != "" {
				body["comment"] = in.Comment
			}
			res, err := c.send(ctx, http.MethodPost, "/api/wiki/pages/"+esc(in.PageID)+"/annotations", body)
			return shaped(in.Verbose, res, func(v any) any { return minimizeAnnotation(v) }), err
		})

	gomcp.Register(srv, update("update_wiki_annotation", "Update a wiki annotation's color or resolved state."),
		func(ctx context.Context, in updateAnnotationIn) (any, error) {
			if in.Color != nil {
				if err := oneOf("color", *in.Color, "yellow", "blue", "green", "red"); err != nil {
					return nil, err
				}
			}
			_, c := sess(ctx)
			body := struct {
				Color    *string `json:"color,omitempty"`
				Resolved *bool   `json:"resolved,omitempty"`
			}{in.Color, in.Resolved}
			res, err := c.send(ctx, http.MethodPatch, "/api/wiki/annotations/"+esc(in.AnnotationID), body)
			return shaped(in.Verbose, res, func(v any) any { return minimizeAnnotation(v) }), err
		})

	gomcp.Register(srv, update("delete_wiki_annotation", "Delete a wiki annotation and its comment thread."),
		func(ctx context.Context, in annotationIDIn) (gomcp.Result, error) {
			_, c := sess(ctx)
			if err := c.call(ctx, http.MethodDelete, "/api/wiki/annotations/"+esc(in.AnnotationID), nil, nil); err != nil {
				return gomcp.Result{}, err
			}
			return gomcp.Text("Wiki annotation deleted successfully"), nil
		})

	gomcp.Register(srv, write("create_wiki_annotation_comment", "Add a comment to an existing wiki annotation."),
		func(ctx context.Context, in annotationCommentIn) (any, error) {
			if in.Content == "" || len(in.Content) > 5000 {
				return nil, errors.New("content must be 1-5000 characters")
			}
			_, c := sess(ctx)
			body := struct {
				Content         string `json:"content"`
				ParentCommentID *int64 `json:"parent_comment_id,omitempty"`
			}{in.Content, in.ParentCommentID}
			return c.send(ctx, http.MethodPost, "/api/wiki/annotations/"+esc(in.AnnotationID)+"/comments", body)
		})

	gomcp.Register(srv, update("update_wiki_annotation_comment", "Update the body of a wiki annotation comment."),
		func(ctx context.Context, in updateCommentIn) (any, error) {
			if in.Content == "" || len(in.Content) > 5000 {
				return nil, errors.New("content must be 1-5000 characters")
			}
			_, c := sess(ctx)
			return c.send(ctx, http.MethodPatch, "/api/wiki/annotation-comments/"+esc(in.CommentID), map[string]string{"content": in.Content})
		})

	gomcp.Register(srv, update("delete_wiki_annotation_comment", "Delete a wiki annotation comment."),
		func(ctx context.Context, in commentIDIn) (gomcp.Result, error) {
			_, c := sess(ctx)
			if err := c.call(ctx, http.MethodDelete, "/api/wiki/annotation-comments/"+esc(in.CommentID), nil, nil); err != nil {
				return gomcp.Result{}, err
			}
			return gomcp.Text("Wiki annotation comment deleted successfully"), nil
		})

	gomcp.Register(srv, write("create_wiki_page", "Create a new wiki page in a project (default: the X-Project-ID scope). Content supports markdown with extensions: references ([^1] or [^label] inline → numbered superscript citation, [^label]: text → reference list; labels may use letters, digits, - and _), graph links ([[wiki:ID|Label]], [[task:ID|Label]]), drawings ([draw:id]), and Figma embeds ([figma:url])."),
		func(ctx context.Context, in createPageIn) (any, error) {
			pid, err := defaultProject(ctx, in.ProjectID)
			if err != nil {
				return nil, err
			}
			_, c := sess(ctx)
			body := map[string]any{"title": in.Title}
			if in.ParentID != "" {
				parent, err := strconv.ParseInt(in.ParentID, 10, 64)
				if err != nil {
					return nil, errors.New("parent_id must be a page ID")
				}
				body["parent_id"] = parent
			}
			page, err := c.send(ctx, http.MethodPost, "/api/projects/"+esc(pid)+"/wiki/pages", body)
			if err != nil {
				return nil, err
			}
			if in.Content != "" {
				id := strings.Trim(jsonString(field(page, "id")), `"`)
				if err := c.call(ctx, http.MethodPut, "/api/wiki/pages/"+esc(id)+"/content", map[string]any{"content": in.Content, "manual_save": true}, nil); err != nil {
					return nil, fmt.Errorf("page %s created, but saving its content failed: %w", id, err)
				}
			}
			return shaped(in.Verbose, page, func(v any) any { return pick(v, wikiPageFields...) }), nil
		})

	gomcp.Register(srv, update("update_wiki_page_content", "Update the content of an existing wiki page. Content supports markdown with extensions: references ([^1] or [^label] inline → numbered superscript citation, [^label]: text → reference list; labels may use letters, digits, - and _), graph links ([[wiki:ID|Label]], [[task:ID|Label]]), drawings ([draw:id]), and Figma embeds ([figma:url])."),
		func(ctx context.Context, in pageContentIn) (any, error) {
			_, c := sess(ctx)
			res, err := c.send(ctx, http.MethodPut, "/api/wiki/pages/"+esc(in.PageID)+"/content", map[string]any{"content": in.Content, "manual_save": true})
			return shaped(in.Verbose, res, func(v any) any { return pick(v, wikiPageFields...) }), err
		})

	gomcp.Register(srv, update("update_wiki_page_title", "Rename a wiki page. Updates the title and regenerates the URL slug."),
		func(ctx context.Context, in pageTitleIn) (any, error) {
			if t := strings.TrimSpace(in.Title); t == "" || len(t) > 500 {
				return nil, errors.New("title must be 1-500 characters")
			}
			_, c := sess(ctx)
			res, err := c.send(ctx, http.MethodPatch, "/api/wiki/pages/"+esc(in.PageID), map[string]string{"title": in.Title})
			return shaped(in.Verbose, res, func(v any) any { return pick(v, wikiPageFields...) }), err
		})

	gomcp.Register(srv, update("move_wiki_page", "Move a wiki page in the hierarchy: nest it under another page or make it top-level. A page cannot be moved under itself or its descendants, and the tree is limited to 6 levels."),
		func(ctx context.Context, in movePageIn) (any, error) {
			body := map[string]any{"parent_id": nil}
			if in.ParentID != nil && *in.ParentID != "" {
				parent, err := strconv.ParseInt(*in.ParentID, 10, 64)
				if err != nil {
					return nil, errors.New("parent_id must be a page ID or null")
				}
				body["parent_id"] = parent
			}
			if in.Position != nil {
				if *in.Position < 0 {
					return nil, errors.New("position must be 0 or more")
				}
				body["position"] = *in.Position
			}
			_, c := sess(ctx)
			res, err := c.send(ctx, http.MethodPatch, "/api/wiki/pages/"+esc(in.PageID), body)
			return shaped(in.Verbose, res, func(v any) any { return pick(v, wikiPageFields...) }), err
		})

	gomcp.Register(srv, read("download_wiki_pdf", "Generate and download a wiki page as PDF (rendered like the web view). Returns the PDF as an embedded resource."),
		func(ctx context.Context, in pageIDIn) (gomcp.Result, error) {
			_, c := sess(ctx)
			data, name, err := c.pdf(ctx, in.PageID)
			if err != nil {
				return gomcp.Result{}, err
			}
			return gomcp.Text(fmt.Sprintf("PDF generated: %s (%d KB)", name, (len(data)+1023)/1024)).
				With(gomcp.Resource("file:///"+url.PathEscape(name), "application/pdf", data)), nil
		})

	gomcp.Register(srv, read("download_wiki_markdown", "Download a wiki page as raw Markdown file"),
		func(ctx context.Context, in pageIDIn) (gomcp.Result, error) {
			_, c := sess(ctx)
			content, name, err := c.markdown(ctx, in.PageID)
			if err != nil {
				return gomcp.Result{}, err
			}
			return gomcp.Text("Markdown file: " + name + "\n\n" + content), nil
		})

	gomcp.Register(srv, read("autocomplete_wiki_pages", "Autocomplete wiki page titles (fuzzy search), scoped to the X-Project-ID project by default"),
		func(ctx context.Context, in autocompleteIn) (any, error) {
			_, c := sess(ctx)
			limit := in.Limit
			if limit <= 0 {
				limit = 10
			}
			q := url.Values{"query": {in.Query}, "limit": {strconv.Itoa(limit)}}
			if pid, err := defaultProject(ctx, in.ProjectID); err == nil {
				q.Set("project_id", pid)
			}
			return c.get(ctx, "/api/wiki/autocomplete?"+q.Encode())
		})

	gomcp.Register(srv, read("get_version", "Get system version information (backend version, DB migration version, build info)"),
		func(ctx context.Context, _ verboseIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, "/api/version")
		})

	gomcp.Register(srv, read("list_milestones", "List milestones for a project with task counts. Milestones group tasks into deliverables."),
		func(ctx context.Context, in projectIn) (any, error) {
			_, c := sess(ctx)
			res, err := c.get(ctx, "/api/projects/"+esc(in.ProjectID)+"/milestones")
			return shaped(in.Verbose, res, func(v any) any { return pickEach(v, milestoneFields...) }), err
		})

	gomcp.Register(srv, write("create_milestone", "Create a new milestone in a project. Milestones group tasks into deliverables with target dates."),
		func(ctx context.Context, in createMilestoneIn) (any, error) {
			if in.Status != nil {
				if err := oneOf("status", *in.Status, "active", "completed", "cancelled"); err != nil {
					return nil, err
				}
			}
			_, c := sess(ctx)
			body := struct {
				Name        string  `json:"name"`
				Description *string `json:"description,omitempty"`
				Color       *string `json:"color,omitempty"`
				TargetDate  *string `json:"target_date,omitempty"`
				Status      *string `json:"status,omitempty"`
			}{in.Name, in.Description, in.Color, in.TargetDate, in.Status}
			return c.send(ctx, http.MethodPost, "/api/projects/"+esc(in.ProjectID)+"/milestones", body)
		})

	gomcp.Register(srv, update("update_milestone", "Update a milestone's name, description, color, target date, or status."),
		func(ctx context.Context, in updateMilestoneIn) (any, error) {
			if in.Status != nil {
				if err := oneOf("status", *in.Status, "active", "completed", "cancelled"); err != nil {
					return nil, err
				}
			}
			_, c := sess(ctx)
			body := struct {
				Name        *string `json:"name,omitempty"`
				Description *string `json:"description,omitempty"`
				Color       *string `json:"color,omitempty"`
				TargetDate  *string `json:"target_date,omitempty"`
				Status      *string `json:"status,omitempty"`
				SortOrder   *int    `json:"sort_order,omitempty"`
			}{in.Name, in.Description, in.Color, in.TargetDate, in.Status, in.SortOrder}
			return c.send(ctx, http.MethodPatch, "/api/milestones/"+esc(in.MilestoneID), body)
		})

	gomcp.Register(srv, read("get_milestone_progress", "Get computed progress for a milestone: total/completed tasks, percentage, hours, by-assignee breakdown."),
		func(ctx context.Context, in milestoneIDIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, "/api/milestones/"+esc(in.MilestoneID)+"/progress")
		})

	gomcp.Register(srv, write("add_dependency", "Add a dependency between tasks. Task A depends on (is blocked by) task B. Cycle detection prevents circular dependencies."),
		func(ctx context.Context, in addDependencyIn) (any, error) {
			if err := oneOf("dependency_type", in.DependencyType, "blocks", "related"); err != nil {
				return nil, err
			}
			_, c := sess(ctx)
			body := map[string]any{"depends_on_id": in.DependsOnID}
			if in.DependencyType != "" {
				body["dependency_type"] = in.DependencyType
			}
			return c.send(ctx, http.MethodPost, "/api/tasks/"+esc(in.TaskID)+"/dependencies", body)
		})

	gomcp.Register(srv, update("remove_dependency", "Remove a task dependency by its ID."),
		func(ctx context.Context, in dependencyIDIn) (gomcp.Result, error) {
			_, c := sess(ctx)
			if err := c.call(ctx, http.MethodDelete, "/api/task-dependencies/"+esc(in.DependencyID), nil, nil); err != nil {
				return gomcp.Result{}, err
			}
			return gomcp.Text("Dependency removed successfully"), nil
		})

	gomcp.Register(srv, read("health_check", "Check system health status (database connectivity)"),
		func(ctx context.Context, _ verboseIn) (any, error) {
			_, c := sess(ctx)
			return c.get(ctx, "/healthz")
		})
}
