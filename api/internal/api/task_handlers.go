package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/projectmember"
	"taskai/ent/swimlane"
	"taskai/ent/tag"
	"taskai/ent/task"
	"taskai/ent/taskassignee"
	"taskai/ent/tasktag"
)

// parseDate parses a date string in RFC3339 or YYYY-MM-DD format.
// Returns nil if the string is empty or unparseable.
func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

type TaskAssigneeInfo struct {
	UserID   int64  `json:"user_id"`
	UserName string `json:"user_name"`
}

type Task struct {
	ID                  int64              `json:"id"`
	ProjectID           int64              `json:"project_id"`
	TaskNumber          int64              `json:"task_number"`
	Title               string             `json:"title"`
	Description         *string            `json:"description,omitempty"`
	Status              string             `json:"status"`
	SwimLaneID          *int64             `json:"swim_lane_id,omitempty"`
	SwimLaneName        *string            `json:"swim_lane_name,omitempty"`
	StartDate           *string            `json:"start_date,omitempty"`
	DueDate             *string            `json:"due_date,omitempty"`
	SprintID            *int64             `json:"sprint_id,omitempty"`
	SprintName          *string            `json:"sprint_name,omitempty"`
	Priority            string             `json:"priority"`
	AssigneeID          *int64             `json:"assignee_id,omitempty"`
	AssigneeName        *string            `json:"assignee_name,omitempty"`
	Assignees           []TaskAssigneeInfo `json:"assignees,omitempty"`
	EstimatedHours      *float64           `json:"estimated_hours,omitempty"`
	ActualHours         *float64           `json:"actual_hours,omitempty"`
	Tags                []Tag              `json:"tags,omitempty"`
	GithubIssueNumber   *int64             `json:"github_issue_number,omitempty"`
	GithubRepo          string             `json:"github_repo,omitempty"`
	GithubReactions     []GitHubReaction   `json:"github_reactions,omitempty"`
	AgentName           *string            `json:"agent_name,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

type CreateTaskRequest struct {
	Title          string   `json:"title"`
	Description    *string  `json:"description,omitempty"`
	Status         *string  `json:"status,omitempty"`
	SwimLaneID     *int64   `json:"swim_lane_id,omitempty"`
	StartDate      *string  `json:"start_date,omitempty"`
	DueDate        *string  `json:"due_date,omitempty"`
	SprintID       *int64   `json:"sprint_id,omitempty"`
	Priority       *string  `json:"priority,omitempty"`
	AssigneeID     *int64   `json:"assignee_id,omitempty"`
	AssigneeIDs    []int64  `json:"assignee_ids,omitempty"`
	EstimatedHours *float64 `json:"estimated_hours,omitempty"`
	ActualHours    *float64 `json:"actual_hours,omitempty"`
	TagIDs         []int64  `json:"tag_ids,omitempty"`
}

type UpdateTaskRequest struct {
	Title          *string  `json:"title,omitempty"`
	Description    *string  `json:"description,omitempty"`
	Status         *string  `json:"status,omitempty"`
	SwimLaneID     *int64   `json:"swim_lane_id,omitempty"`
	StartDate      *string  `json:"start_date,omitempty"`
	DueDate        *string  `json:"due_date,omitempty"`
	SprintID       *int64   `json:"sprint_id,omitempty"`
	Priority       *string  `json:"priority,omitempty"`
	AssigneeID     *int64   `json:"assignee_id,omitempty"`
	AssigneeIDs    *[]int64 `json:"assignee_ids,omitempty"`
	EstimatedHours *float64 `json:"estimated_hours,omitempty"`
	ActualHours    *float64 `json:"actual_hours,omitempty"`
	TagIDs         *[]int64 `json:"tag_ids,omitempty"`
}

// HandleListTasks returns all tasks for a project
func (s *Server) HandleListTasks(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	// Verify user has access to this project
	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Fetch all tasks with eager loading of related entities
	entTasks, err := s.db.Client.Task.Query().
		Where(task.ProjectID(projectID)).
		WithAssignee().
		WithSprint().
		Order(ent.Desc(task.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch tasks",
			zap.Int64("project_id", projectID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to fetch tasks", "internal_error")
		return
	}

	// Manually load swim lanes to handle orphaned foreign keys
	if len(entTasks) > 0 {
		swimLaneIDs := make([]int64, 0, len(entTasks))
		for _, t := range entTasks {
			if t.SwimLaneID != nil {
				swimLaneIDs = append(swimLaneIDs, *t.SwimLaneID)
			}
		}

		if len(swimLaneIDs) > 0 {
			swimLanes, err := s.db.Client.SwimLane.Query().
				Where(swimlane.IDIn(swimLaneIDs...)).
				All(ctx)
			if err == nil {
				// Map swim lanes by ID
				swimLaneMap := make(map[int64]*ent.SwimLane)
				for _, sl := range swimLanes {
					swimLaneMap[sl.ID] = sl
				}

				// Assign to task edges
				for i := range entTasks {
					if entTasks[i].SwimLaneID != nil {
						if sl, ok := swimLaneMap[*entTasks[i].SwimLaneID]; ok {
							entTasks[i].Edges.SwimLane = sl
						}
					}
				}
			}
		}
	}

	// Load tags separately for all tasks
	if len(entTasks) > 0 {
		taskIDs := make([]int64, len(entTasks))
		for i, t := range entTasks {
			taskIDs[i] = t.ID
		}

		// Query all task_tags for these tasks
		taskTags, err := s.db.Client.TaskTag.Query().
			Where(tasktag.TaskIDIn(taskIDs...)).
			All(ctx)
		if err != nil {
			// Tags are optional, continue without them
			taskTags = nil
		}
		if len(taskTags) > 0 {
			// Get unique tag IDs
			tagIDsMap := make(map[int64]bool)
			for _, tt := range taskTags {
				tagIDsMap[tt.TagID] = true
			}
			tagIDs := make([]int64, 0, len(tagIDsMap))
			for id := range tagIDsMap {
				tagIDs = append(tagIDs, id)
			}

			// Load all tags
			tags, err := s.db.Client.Tag.Query().
				Where(tag.IDIn(tagIDs...)).
				All(ctx)
			if err == nil {
				// Map tags by ID
				tagsMap := make(map[int64]*ent.Tag)
				for _, t := range tags {
					tagsMap[t.ID] = t
				}

				// Assign tags to task_tags edges
				for i := range taskTags {
					if t, ok := tagsMap[taskTags[i].TagID]; ok {
						taskTags[i].Edges.Tag = t
					}
				}

				// Map task_tags to tasks
				taskTagsMap := make(map[int64][]*ent.TaskTag)
				for i := range taskTags {
					taskTagsMap[taskTags[i].TaskID] = append(taskTagsMap[taskTags[i].TaskID], taskTags[i])
				}

				// Assign to task edges
				for i := range entTasks {
					if tts, ok := taskTagsMap[entTasks[i].ID]; ok {
						entTasks[i].Edges.TaskTags = tts
					}
				}
			}
		}
	}

	// Load task_assignees for all tasks
	taskIDs := make([]int64, len(entTasks))
	for i, t := range entTasks {
		taskIDs[i] = t.ID
	}
	assigneesMap := s.loadTaskAssigneesMap(ctx, taskIDs)

	// Convert Ent tasks to API tasks
	tasks := make([]Task, 0, len(entTasks))
	for _, et := range entTasks {
		t := Task{
			ID:             et.ID,
			ProjectID:      et.ProjectID,
			Title:          et.Title,
			Description:    et.Description,
			Status:         et.Status,
			Priority:       et.Priority,
			EstimatedHours: et.EstimatedHours,
			ActualHours:    et.ActualHours,
			AgentName:      et.AgentName,
			CreatedAt:      et.CreatedAt,
			UpdatedAt:      et.UpdatedAt,
			Tags:           []Tag{}, // Initialize empty tags array
		}

		// Convert task_number from *int to int64
		if et.TaskNumber != nil {
			t.TaskNumber = int64(*et.TaskNumber)
		}

		// Convert start_date from time.Time to string if present
		if et.StartDate != nil {
			startDateStr := et.StartDate.Format(time.RFC3339)
			t.StartDate = &startDateStr
		}

		// Convert due_date from time.Time to string if present
		if et.DueDate != nil {
			dueDateStr := et.DueDate.Format(time.RFC3339)
			t.DueDate = &dueDateStr
		}

		// Add assignee info if present
		if et.Edges.Assignee != nil {
			t.AssigneeID = &et.Edges.Assignee.ID
			t.AssigneeName = userDisplayNamePtr(et.Edges.Assignee)
		}

		// Add multi-assignees
		if assignees, ok := assigneesMap[et.ID]; ok {
			t.Assignees = assignees
		}
		// Backfill from legacy single assignee_id if no multi-assignees present
		if len(t.Assignees) == 0 && t.AssigneeID != nil && t.AssigneeName != nil {
			t.Assignees = []TaskAssigneeInfo{{UserID: *t.AssigneeID, UserName: *t.AssigneeName}}
		}

		// Add sprint info if present
		if et.Edges.Sprint != nil {
			t.SprintID = &et.Edges.Sprint.ID
			t.SprintName = &et.Edges.Sprint.Name
		}

		// Add swim lane info if present
		if et.Edges.SwimLane != nil {
			t.SwimLaneID = &et.Edges.SwimLane.ID
			t.SwimLaneName = &et.Edges.SwimLane.Name
		}

		// Add tags if present
		if et.Edges.TaskTags != nil {
			for _, tt := range et.Edges.TaskTags {
				if tt.Edges.Tag != nil {
					t.Tags = append(t.Tags, Tag{
						ID:        int(tt.Edges.Tag.ID),
						UserID:    int(tt.Edges.Tag.UserID),
						Name:      tt.Edges.Tag.Name,
						Color:     tt.Edges.Tag.Color,
						CreatedAt: tt.Edges.Tag.CreatedAt,
					})
				}
			}
		}

		tasks = append(tasks, t)
	}

	// Bulk-fetch github_issue_number and github_repo (not in ent schema)
	if len(tasks) > 0 {
		ghRows, ghErr := s.db.QueryContext(ctx, `
			SELECT id, github_issue_number, github_repo FROM tasks
			WHERE project_id = $1 AND github_issue_number IS NOT NULL
		`, projectID)
		if ghErr == nil {
			type ghInfo struct {
				issueNum int64
				repo     string
			}
			ghMap := map[int64]ghInfo{}
			for ghRows.Next() {
				var tid, inum int64
				var repo string
				if ghRows.Scan(&tid, &inum, &repo) == nil {
					ghMap[tid] = ghInfo{inum, repo}
				}
			}
			ghRows.Close()
			for i := range tasks {
				if info, ok := ghMap[tasks[i].ID]; ok {
					tasks[i].GithubIssueNumber = &info.issueNum
					tasks[i].GithubRepo = info.repo
				}
			}
		}
	}

	// Bulk-fetch GitHub reactions for all tasks in this project, including user_reacted
	if len(tasks) > 0 {
		reactionMap := map[int64][]GitHubReaction{}
		rRows, rErr := s.db.QueryContext(ctx, `
			SELECT gr.task_id, gr.reaction, gr.count, (ur.id IS NOT NULL) AS user_reacted
			FROM github_reactions gr
			LEFT JOIN user_reactions ur ON
			    ur.reaction = gr.reaction AND ur.user_id = $1 AND ur.task_id = gr.task_id
			WHERE gr.task_id IN (SELECT id FROM tasks WHERE project_id = $2)
			  AND gr.count > 0
		`, userID, projectID)
		if rErr == nil {
			for rRows.Next() {
				var tid int64
				var gr GitHubReaction
				if rRows.Scan(&tid, &gr.Reaction, &gr.Count, &gr.UserReacted) == nil {
					reactionMap[tid] = append(reactionMap[tid], gr)
				}
			}
			rRows.Close()
			for i := range tasks {
				if reactions, ok := reactionMap[tasks[i].ID]; ok {
					tasks[i].GithubReactions = reactions
				}
			}
		}
	}

	respondJSON(w, http.StatusOK, tasks)
}

// HandleCreateTask creates a new task
func (s *Server) HandleCreateTask(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	// Verify user has access to this project
	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	// Validation
	if req.Title == "" {
		respondError(w, http.StatusBadRequest, "task title is required", "invalid_input")
		return
	}
	if len(req.Title) > 255 {
		respondError(w, http.StatusBadRequest, "task title is too long (max 255 characters)", "invalid_input")
		return
	}

	// Default status to 'todo' if not provided (for backward compatibility)
	status := "todo"
	if req.Status != nil {
		status = *req.Status
	}

	// Validate status
	if status != "todo" && status != "in_progress" && status != "done" {
		respondError(w, http.StatusBadRequest, "invalid status (must be: todo, in_progress, or done)", "invalid_input")
		return
	}

	// Sync swim_lane_id and status
	var swimLaneID *int64
	if req.SwimLaneID != nil {
		swimLaneID = req.SwimLaneID
		// Derive status from the swim lane's status_category
		lane, err := s.db.Client.SwimLane.Query().
			Where(
				swimlane.ID(*req.SwimLaneID),
				swimlane.ProjectID(projectID),
			).
			Only(ctx)
		if err == nil {
			status = lane.StatusCategory
		}
	} else {
		// Find first swim lane matching the status category
		lane, err := s.db.Client.SwimLane.Query().
			Where(
				swimlane.ProjectID(projectID),
				swimlane.StatusCategory(status),
			).
			Order(ent.Asc(swimlane.FieldPosition)).
			First(ctx)
		if err == nil {
			swimLaneID = &lane.ID
		}
	}

	// Default priority
	priority := "medium"
	if req.Priority != nil {
		priority = *req.Priority
	}

	// Validate priority
	if priority != "low" && priority != "medium" && priority != "high" && priority != "urgent" {
		respondError(w, http.StatusBadRequest, "invalid priority (must be: low, medium, high, or urgent)", "invalid_input")
		return
	}

	// Get next task_number for this project
	// Note: The UNIQUE index on (project_id, task_number) will prevent duplicates
	var maxNumber sql.NullInt64
	err = s.db.QueryRowContext(ctx, `SELECT MAX(task_number) FROM tasks WHERE project_id = $1`, projectID).Scan(&maxNumber)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get next task number", "internal_error")
		return
	}
	nextNumber := 1
	if maxNumber.Valid {
		nextNumber = int(maxNumber.Int64) + 1
	}

	// Parse start_date / due_date — accept RFC3339 or plain YYYY-MM-DD.
	var startDate *time.Time
	if req.StartDate != nil {
		startDate = parseDate(*req.StartDate)
	}
	var dueDate *time.Time
	if req.DueDate != nil {
		dueDate = parseDate(*req.DueDate)
	}

	// Use Ent transaction
	entTx, err := s.db.Client.Tx(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to start transaction", "internal_error")
		return
	}
	defer entTx.Rollback()

	// Create task using Ent
	newTask, err := entTx.Task.Create().
		SetProjectID(projectID).
		SetTaskNumber(nextNumber).
		SetTitle(req.Title).
		SetNillableDescription(req.Description).
		SetStatus(status).
		SetNillableSwimLaneID(swimLaneID).
		SetNillableStartDate(startDate).
		SetNillableDueDate(dueDate).
		SetNillableSprintID(req.SprintID).
		SetPriority(priority).
		SetNillableAssigneeID(req.AssigneeID).
		SetNillableEstimatedHours(req.EstimatedHours).
		SetNillableActualHours(req.ActualHours).
		Save(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create task", "internal_error")
		return
	}

	// Add tags if provided
	if len(req.TagIDs) > 0 {
		for _, tagID := range req.TagIDs {
			_, err := entTx.TaskTag.Create().
				SetTaskID(newTask.ID).
				SetTagID(tagID).
				Save(ctx)
			if err != nil {
				// Continue even if tag insertion fails
				continue
			}
		}
	}

	// Add multi-assignees if provided
	if len(req.AssigneeIDs) > 0 {
		for _, uid := range req.AssigneeIDs {
			if _, err := entTx.TaskAssignee.Create().SetTaskID(newTask.ID).SetUserID(uid).Save(ctx); err != nil {
				continue // best-effort
			}
		}
	}

	// Commit transaction
	if err := entTx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit task creation", "internal_error")
		return
	}

	// Set created_by via raw SQL after commit (not in ent schema)
	if _, err := s.db.ExecContext(ctx, `UPDATE tasks SET created_by = $1 WHERE id = $2`, userID, newTask.ID); err != nil {
		s.logger.Warn("Failed to set created_by on task", zap.Error(err), zap.Int64("task_id", newTask.ID))
	}

	// Set agent_name via raw SQL if present (mirrors created_by pattern)
	if agentName := GetAgentName(r); agentName != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE tasks SET agent_name = $1 WHERE id = $2`, *agentName, newTask.ID); err != nil {
			s.logger.Warn("Failed to set agent_name on task", zap.Error(err), zap.Int64("task_id", newTask.ID))
		}
	}

	// Fetch the created task with all related entities
	createdTask, err := s.db.Client.Task.Query().
		Where(task.ID(newTask.ID)).
		WithAssignee().
		WithSprint().
		WithSwimLane().
		Only(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch created task", "internal_error")
		return
	}

	// Load tags separately
	taskTags, err := s.db.Client.TaskTag.Query().
		Where(tasktag.TaskID(newTask.ID)).
		WithTag().
		All(ctx)
	if err == nil {
		createdTask.Edges.TaskTags = taskTags
	}

	// Convert to API task
	t := Task{
		ID:             createdTask.ID,
		ProjectID:      createdTask.ProjectID,
		Title:          createdTask.Title,
		Description:    createdTask.Description,
		Status:         createdTask.Status,
		Priority:       createdTask.Priority,
		EstimatedHours: createdTask.EstimatedHours,
		ActualHours:    createdTask.ActualHours,
		AgentName:      createdTask.AgentName,
		CreatedAt:      createdTask.CreatedAt,
		UpdatedAt:      createdTask.UpdatedAt,
		Tags:           []Tag{},
	}

	// Convert task_number from *int to int64
	if createdTask.TaskNumber != nil {
		t.TaskNumber = int64(*createdTask.TaskNumber)
	}

	// Convert start_date from time.Time to string if present
	if createdTask.StartDate != nil {
		startDateStr := createdTask.StartDate.Format(time.RFC3339)
		t.StartDate = &startDateStr
	}

	// Convert due_date from time.Time to string if present
	if createdTask.DueDate != nil {
		dueDateStr := createdTask.DueDate.Format(time.RFC3339)
		t.DueDate = &dueDateStr
	}

	// Add assignee info if present
	if createdTask.Edges.Assignee != nil {
		t.AssigneeID = &createdTask.Edges.Assignee.ID
		t.AssigneeName = userDisplayNamePtr(createdTask.Edges.Assignee)
	}

	// Add multi-assignees
	if assignees, ok := s.loadTaskAssigneesMap(ctx, []int64{createdTask.ID})[createdTask.ID]; ok {
		t.Assignees = assignees
	}
	if len(t.Assignees) == 0 && t.AssigneeID != nil && t.AssigneeName != nil {
		t.Assignees = []TaskAssigneeInfo{{UserID: *t.AssigneeID, UserName: *t.AssigneeName}}
	}

	// Add sprint info if present
	if createdTask.Edges.Sprint != nil {
		t.SprintID = &createdTask.Edges.Sprint.ID
		t.SprintName = &createdTask.Edges.Sprint.Name
	}

	// Add swim lane info if present
	if createdTask.Edges.SwimLane != nil {
		t.SwimLaneID = &createdTask.Edges.SwimLane.ID
		t.SwimLaneName = &createdTask.Edges.SwimLane.Name
	}

	// Add tags if present
	if createdTask.Edges.TaskTags != nil {
		for _, tt := range createdTask.Edges.TaskTags {
			if tt.Edges.Tag != nil {
				t.Tags = append(t.Tags, Tag{
					ID:        int(tt.Edges.Tag.ID),
					UserID:    int(tt.Edges.Tag.UserID),
					Name:      tt.Edges.Tag.Name,
					Color:     tt.Edges.Tag.Color,
					CreatedAt: tt.Edges.Tag.CreatedAt,
				})
			}
		}
	}

	respondJSON(w, http.StatusCreated, t)
	go s.broadcastToProjectMembers(t.ProjectID, "task_created", t)
	if createdTask.Description != nil {
		taskNum := t.TaskNumber
		go s.syncGraphLinks(context.Background(), createdTask.ProjectID, "task", createdTask.ID, &taskNum, createdTask.Title, *createdTask.Description)
	}
}

// HandleUpdateTask updates an existing task
func (s *Server) HandleUpdateTask(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	taskID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "invalid_input")
		return
	}

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	// Get task and verify access
	taskEntity, err := s.db.Client.Task.Get(ctx, taskID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get task project", "internal_error")
		return
	}

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, taskEntity.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Validations
	if req.Title != nil {
		if *req.Title == "" {
			respondError(w, http.StatusBadRequest, "task title cannot be empty", "invalid_input")
			return
		}
		if len(*req.Title) > 255 {
			respondError(w, http.StatusBadRequest, "task title is too long (max 255 characters)", "invalid_input")
			return
		}
	}

	if req.Priority != nil {
		if *req.Priority != "low" && *req.Priority != "medium" && *req.Priority != "high" && *req.Priority != "urgent" {
			respondError(w, http.StatusBadRequest, "invalid priority (must be: low, medium, high, or urgent)", "invalid_input")
			return
		}
	}

	// Determine status and swim_lane_id with sync logic
	var finalStatus *string
	var finalSwimLaneID *int64

	if req.SwimLaneID != nil && req.Status == nil {
		// Swim lane changed — derive status from lane's status_category
		lane, err := s.db.Client.SwimLane.Query().
			Where(
				swimlane.ID(*req.SwimLaneID),
				swimlane.ProjectID(taskEntity.ProjectID),
			).
			Only(ctx)
		if err == nil {
			finalStatus = &lane.StatusCategory
			finalSwimLaneID = req.SwimLaneID
		}
	} else if req.Status != nil && req.SwimLaneID == nil {
		// Status changed — find first swim lane with matching status_category
		if *req.Status != "todo" && *req.Status != "in_progress" && *req.Status != "done" {
			respondError(w, http.StatusBadRequest, "invalid status (must be: todo, in_progress, or done)", "invalid_input")
			return
		}
		finalStatus = req.Status
		lane, err := s.db.Client.SwimLane.Query().
			Where(
				swimlane.ProjectID(taskEntity.ProjectID),
				swimlane.StatusCategory(*req.Status),
			).
			Order(ent.Asc(swimlane.FieldPosition)).
			First(ctx)
		if err == nil {
			finalSwimLaneID = &lane.ID
		}
	} else if req.Status != nil && req.SwimLaneID != nil {
		// Both provided — trust swim_lane_id, derive status from it
		if *req.Status != "todo" && *req.Status != "in_progress" && *req.Status != "done" {
			respondError(w, http.StatusBadRequest, "invalid status (must be: todo, in_progress, or done)", "invalid_input")
			return
		}
		lane, err := s.db.Client.SwimLane.Query().
			Where(
				swimlane.ID(*req.SwimLaneID),
				swimlane.ProjectID(taskEntity.ProjectID),
			).
			Only(ctx)
		if err == nil {
			finalStatus = &lane.StatusCategory
		} else {
			finalStatus = req.Status
		}
		finalSwimLaneID = req.SwimLaneID
	}

	// Parse start_date / due_date — accept RFC3339 or plain YYYY-MM-DD.
	var startDate *time.Time
	if req.StartDate != nil {
		startDate = parseDate(*req.StartDate)
	}
	var dueDate *time.Time
	if req.DueDate != nil {
		dueDate = parseDate(*req.DueDate)
	}

	// Build update using Ent
	updateBuilder := s.db.Client.Task.UpdateOneID(taskID)

	if req.Title != nil {
		updateBuilder.SetTitle(*req.Title)
	}
	if req.Description != nil {
		updateBuilder.SetNillableDescription(req.Description)
	}
	if finalStatus != nil {
		updateBuilder.SetStatus(*finalStatus)
	}
	if finalSwimLaneID != nil {
		updateBuilder.SetNillableSwimLaneID(finalSwimLaneID)
	}
	if startDate != nil {
		updateBuilder.SetNillableStartDate(startDate)
	}
	if dueDate != nil {
		updateBuilder.SetNillableDueDate(dueDate)
	}
	if req.Priority != nil {
		updateBuilder.SetPriority(*req.Priority)
	}
	if req.SprintID != nil {
		updateBuilder.SetNillableSprintID(req.SprintID)
	}
	if req.AssigneeID != nil {
		updateBuilder.SetNillableAssigneeID(req.AssigneeID)
	}
	if req.EstimatedHours != nil {
		updateBuilder.SetNillableEstimatedHours(req.EstimatedHours)
	}
	if req.ActualHours != nil {
		updateBuilder.SetNillableActualHours(req.ActualHours)
	}

	_, err = updateBuilder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update task", "internal_error")
		return
	}

	// Best-effort push swim lane change to GitHub Projects V2
	if finalSwimLaneID != nil {
		go s.tryPushSwimLaneToGitHub(context.Background(), taskID, finalSwimLaneID)
	}

	// Handle tag updates if provided
	if req.TagIDs != nil {
		// Delete existing task_tags
		_, err = s.db.Client.TaskTag.Delete().
			Where(tasktag.TaskID(taskID)).
			Exec(ctx)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update tags", "internal_error")
			return
		}

		// Add new task_tags
		for _, tagID := range *req.TagIDs {
			_, err = s.db.Client.TaskTag.Create().
				SetTaskID(taskID).
				SetTagID(tagID).
				Save(ctx)
			if err != nil {
				// Continue even if tag insertion fails
				continue
			}
		}
	}

	// Handle assignee_ids updates if provided
	assigneesChanged := req.AssigneeIDs != nil || req.AssigneeID != nil
	if req.AssigneeIDs != nil {
		if err := s.replaceTaskAssignees(ctx, taskID, *req.AssigneeIDs); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update assignees", "internal_error")
			return
		}
	}

	// Best-effort push assignee changes to GitHub
	if assigneesChanged {
		go s.tryPushAssigneesToGitHub(context.Background(), taskID)
	}

	// Fetch the updated task with all related entities
	updatedTask, err := s.db.Client.Task.Query().
		Where(task.ID(taskID)).
		WithAssignee().
		WithSprint().
		WithSwimLane().
		Only(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch updated task", "internal_error")
		return
	}

	// Load tags separately
	taskTags, err := s.db.Client.TaskTag.Query().
		Where(tasktag.TaskID(taskID)).
		All(ctx)
	if err == nil && len(taskTags) > 0 {
		// Get tag IDs
		tagIDs := make([]int64, len(taskTags))
		for i, tt := range taskTags {
			tagIDs[i] = tt.TagID
		}

		// Load tags
		tags, err := s.db.Client.Tag.Query().
			Where(tag.IDIn(tagIDs...)).
			All(ctx)
		if err == nil {
			// Map tags by ID
			tagsMap := make(map[int64]*ent.Tag)
			for i := range tags {
				tagsMap[tags[i].ID] = tags[i]
			}

			// Assign to task_tags edges
			for i := range taskTags {
				if t, ok := tagsMap[taskTags[i].TagID]; ok {
					taskTags[i].Edges.Tag = t
				}
			}
			updatedTask.Edges.TaskTags = taskTags
		}
	}

	// Convert to API task
	t := Task{
		ID:             updatedTask.ID,
		ProjectID:      updatedTask.ProjectID,
		Title:          updatedTask.Title,
		Description:    updatedTask.Description,
		Status:         updatedTask.Status,
		Priority:       updatedTask.Priority,
		EstimatedHours: updatedTask.EstimatedHours,
		ActualHours:    updatedTask.ActualHours,
		AgentName:      updatedTask.AgentName,
		CreatedAt:      updatedTask.CreatedAt,
		UpdatedAt:      updatedTask.UpdatedAt,
		Tags:           []Tag{},
	}

	// Convert task_number
	if updatedTask.TaskNumber != nil {
		t.TaskNumber = int64(*updatedTask.TaskNumber)
	}

	// Convert start_date
	if updatedTask.StartDate != nil {
		startDateStr := updatedTask.StartDate.Format(time.RFC3339)
		t.StartDate = &startDateStr
	}

	// Convert due_date
	if updatedTask.DueDate != nil {
		dueDateStr := updatedTask.DueDate.Format(time.RFC3339)
		t.DueDate = &dueDateStr
	}

	// Add assignee info
	if updatedTask.Edges.Assignee != nil {
		t.AssigneeID = &updatedTask.Edges.Assignee.ID
		t.AssigneeName = userDisplayNamePtr(updatedTask.Edges.Assignee)
	}

	// Add multi-assignees
	if assignees, ok := s.loadTaskAssigneesMap(ctx, []int64{taskID})[taskID]; ok {
		t.Assignees = assignees
	}
	if len(t.Assignees) == 0 && t.AssigneeID != nil && t.AssigneeName != nil {
		t.Assignees = []TaskAssigneeInfo{{UserID: *t.AssigneeID, UserName: *t.AssigneeName}}
	}

	// Add sprint info
	if updatedTask.Edges.Sprint != nil {
		t.SprintID = &updatedTask.Edges.Sprint.ID
		t.SprintName = &updatedTask.Edges.Sprint.Name
	}

	// Add swim lane info
	if updatedTask.Edges.SwimLane != nil {
		t.SwimLaneID = &updatedTask.Edges.SwimLane.ID
		t.SwimLaneName = &updatedTask.Edges.SwimLane.Name
	}

	// Add tags
	if updatedTask.Edges.TaskTags != nil {
		for _, tt := range updatedTask.Edges.TaskTags {
			if tt.Edges.Tag != nil {
				t.Tags = append(t.Tags, Tag{
					ID:        int(tt.Edges.Tag.ID),
					UserID:    int(tt.Edges.Tag.UserID),
					Name:      tt.Edges.Tag.Name,
					Color:     tt.Edges.Tag.Color,
					CreatedAt: tt.Edges.Tag.CreatedAt,
				})
			}
		}
	}

	respondJSON(w, http.StatusOK, t)
	go s.broadcastToProjectMembers(t.ProjectID, "task_updated", t)
	if updatedTask.Description != nil {
		taskNum := t.TaskNumber
		go s.syncGraphLinks(context.Background(), updatedTask.ProjectID, "task", taskID, &taskNum, updatedTask.Title, *updatedTask.Description)
	}
}

// HandleDeleteTask deletes a task
func (s *Server) HandleDeleteTask(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	taskID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "invalid_input")
		return
	}

	// Get task and verify it exists
	taskEntity, err := s.db.Client.Task.Get(ctx, taskID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get task project", "internal_error")
		return
	}

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, taskEntity.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Delete task using Ent
	err = s.db.Client.Task.DeleteOneID(taskID).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete task", "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
	go s.broadcastToProjectMembers(taskEntity.ProjectID, "task_deleted", map[string]int64{
		"id":         taskID,
		"project_id": taskEntity.ProjectID,
	})
}

// HandleGetTaskByNumber returns a single task by project-scoped task number
func (s *Server) HandleGetTaskByNumber(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}
	taskNumber, err := strconv.ParseInt(chi.URLParam(r, "taskNumber"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task number", "invalid_input")
		return
	}

	// Verify user has access to this project
	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Fetch task with related entities
	taskEntity, err := s.db.Client.Task.Query().
		Where(
			task.ProjectID(projectID),
			task.TaskNumber(int(taskNumber)),
		).
		WithAssignee().
		WithSprint().
		WithSwimLane().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch task", "internal_error")
		return
	}

	// Load tags separately
	taskTags, err := s.db.Client.TaskTag.Query().
		Where(tasktag.TaskID(taskEntity.ID)).
		All(ctx)
	if err == nil && len(taskTags) > 0 {
		// Get tag IDs
		tagIDs := make([]int64, len(taskTags))
		for i, tt := range taskTags {
			tagIDs[i] = tt.TagID
		}

		// Load tags
		tags, err := s.db.Client.Tag.Query().
			Where(tag.IDIn(tagIDs...)).
			All(ctx)
		if err == nil {
			// Map tags by ID
			tagsMap := make(map[int64]*ent.Tag)
			for i := range tags {
				tagsMap[tags[i].ID] = tags[i]
			}

			// Assign to task_tags edges
			for i := range taskTags {
				if t, ok := tagsMap[taskTags[i].TagID]; ok {
					taskTags[i].Edges.Tag = t
				}
			}
			taskEntity.Edges.TaskTags = taskTags
		}
	}

	// Convert to API task
	t := Task{
		ID:             taskEntity.ID,
		ProjectID:      taskEntity.ProjectID,
		Title:          taskEntity.Title,
		Description:    taskEntity.Description,
		Status:         taskEntity.Status,
		Priority:       taskEntity.Priority,
		EstimatedHours: taskEntity.EstimatedHours,
		ActualHours:    taskEntity.ActualHours,
		AgentName:      taskEntity.AgentName,
		CreatedAt:      taskEntity.CreatedAt,
		UpdatedAt:      taskEntity.UpdatedAt,
		Tags:           []Tag{},
	}

	// Convert task_number
	if taskEntity.TaskNumber != nil {
		t.TaskNumber = int64(*taskEntity.TaskNumber)
	}

	// Convert start_date
	if taskEntity.StartDate != nil {
		startDateStr := taskEntity.StartDate.Format(time.RFC3339)
		t.StartDate = &startDateStr
	}

	// Convert due_date
	if taskEntity.DueDate != nil {
		dueDateStr := taskEntity.DueDate.Format(time.RFC3339)
		t.DueDate = &dueDateStr
	}

	// Add assignee info
	if taskEntity.Edges.Assignee != nil {
		t.AssigneeID = &taskEntity.Edges.Assignee.ID
		t.AssigneeName = userDisplayNamePtr(taskEntity.Edges.Assignee)
	}

	// Add multi-assignees
	if assignees, ok := s.loadTaskAssigneesMap(ctx, []int64{taskEntity.ID})[taskEntity.ID]; ok {
		t.Assignees = assignees
	}
	// Backfill from legacy single assignee_id if no multi-assignees present
	if len(t.Assignees) == 0 && t.AssigneeID != nil && t.AssigneeName != nil {
		t.Assignees = []TaskAssigneeInfo{{UserID: *t.AssigneeID, UserName: *t.AssigneeName}}
	}

	// Add sprint info
	if taskEntity.Edges.Sprint != nil {
		t.SprintID = &taskEntity.Edges.Sprint.ID
		t.SprintName = &taskEntity.Edges.Sprint.Name
	}

	// Add swim lane info
	if taskEntity.Edges.SwimLane != nil {
		t.SwimLaneID = &taskEntity.Edges.SwimLane.ID
		t.SwimLaneName = &taskEntity.Edges.SwimLane.Name
	}

	// Add tags
	if taskEntity.Edges.TaskTags != nil {
		for _, tt := range taskEntity.Edges.TaskTags {
			if tt.Edges.Tag != nil {
				t.Tags = append(t.Tags, Tag{
					ID:        int(tt.Edges.Tag.ID),
					UserID:    int(tt.Edges.Tag.UserID),
					Name:      tt.Edges.Tag.Name,
					Color:     tt.Edges.Tag.Color,
					CreatedAt: tt.Edges.Tag.CreatedAt,
				})
			}
		}
	}

	// Load github_issue_number and github_repo (not in ent schema, raw SQL)
	var ghIssueNum sql.NullInt64
	var ghRepo sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT github_issue_number, github_repo FROM tasks WHERE id = $1`, taskEntity.ID).Scan(&ghIssueNum, &ghRepo); err == nil {
		if ghIssueNum.Valid {
			t.GithubIssueNumber = &ghIssueNum.Int64
		}
		if ghRepo.Valid && ghRepo.String != "" {
			t.GithubRepo = ghRepo.String
		}
	}

	// Load GitHub reactions for this task, including user_reacted
	reactionRows, err := s.db.QueryContext(ctx, `
		SELECT gr.reaction, gr.count, (ur.id IS NOT NULL) AS user_reacted
		FROM github_reactions gr
		LEFT JOIN user_reactions ur ON
		    ur.reaction = gr.reaction AND ur.user_id = $2 AND ur.task_id = gr.task_id
		WHERE gr.task_id = $1 AND gr.count > 0
	`, taskEntity.ID, userID)
	if err == nil {
		for reactionRows.Next() {
			var gr GitHubReaction
			if reactionRows.Scan(&gr.Reaction, &gr.Count, &gr.UserReacted) == nil {
				t.GithubReactions = append(t.GithubReactions, gr)
			}
		}
		reactionRows.Close()
	}

	respondJSON(w, http.StatusOK, t)
}

// loadTaskAssigneesMap loads task_assignees for a set of task IDs and returns a map[taskID][]TaskAssigneeInfo.
func (s *Server) loadTaskAssigneesMap(ctx context.Context, taskIDs []int64) map[int64][]TaskAssigneeInfo {
	result := make(map[int64][]TaskAssigneeInfo)
	if len(taskIDs) == 0 {
		return result
	}
	rows, err := s.db.Client.TaskAssignee.Query().
		Where(taskassignee.TaskIDIn(taskIDs...)).
		WithUser().
		All(ctx)
	if err != nil {
		return result
	}
	for _, row := range rows {
		if row.Edges.User != nil {
			result[row.TaskID] = append(result[row.TaskID], TaskAssigneeInfo{
				UserID:   row.Edges.User.ID,
				UserName: userDisplayName(row.Edges.User),
			})
		}
	}
	return result
}

// replaceTaskAssignees atomically replaces all assignees for a task.
func (s *Server) replaceTaskAssignees(ctx context.Context, taskID int64, userIDs []int64) error {
	if _, err := s.db.Client.TaskAssignee.Delete().Where(taskassignee.TaskID(taskID)).Exec(ctx); err != nil {
		return err
	}
	for _, uid := range userIDs {
		if _, err := s.db.Client.TaskAssignee.Create().SetTaskID(taskID).SetUserID(uid).Save(ctx); err != nil {
			continue // best-effort
		}
	}
	return nil
}

// checkProjectAccess verifies that a user has access to a project via project_members table
func (s *Server) checkProjectAccess(ctx context.Context, userID, projectID int64) (bool, error) {
	exists, err := s.db.Client.ProjectMember.Query().
		Where(
			projectmember.ProjectID(projectID),
			projectmember.UserID(userID),
		).
		Exist(ctx)
	return exists, err
}
