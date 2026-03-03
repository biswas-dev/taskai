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
	"taskai/ent/tasktag"
)

type Task struct {
	ID             int64     `json:"id"`
	ProjectID      int64     `json:"project_id"`
	TaskNumber     int64     `json:"task_number"`
	Title          string    `json:"title"`
	Description    *string   `json:"description,omitempty"`
	Status         string    `json:"status"`
	SwimLaneID     *int64    `json:"swim_lane_id,omitempty"`
	SwimLaneName   *string   `json:"swim_lane_name,omitempty"`
	DueDate        *string   `json:"due_date,omitempty"`
	SprintID       *int64    `json:"sprint_id,omitempty"`
	SprintName     *string   `json:"sprint_name,omitempty"`
	Priority       string    `json:"priority"`
	AssigneeID     *int64    `json:"assignee_id,omitempty"`
	AssigneeName   *string   `json:"assignee_name,omitempty"`
	EstimatedHours *float64  `json:"estimated_hours,omitempty"`
	ActualHours    *float64  `json:"actual_hours,omitempty"`
	Tags           []Tag     `json:"tags,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateTaskRequest struct {
	Title          string   `json:"title"`
	Description    *string  `json:"description,omitempty"`
	Status         *string  `json:"status,omitempty"`
	SwimLaneID     *int64   `json:"swim_lane_id,omitempty"`
	DueDate        *string  `json:"due_date,omitempty"`
	SprintID       *int64   `json:"sprint_id,omitempty"`
	Priority       *string  `json:"priority,omitempty"`
	AssigneeID     *int64   `json:"assignee_id,omitempty"`
	EstimatedHours *float64 `json:"estimated_hours,omitempty"`
	ActualHours    *float64 `json:"actual_hours,omitempty"`
	TagIDs         []int64  `json:"tag_ids,omitempty"`
}

type UpdateTaskRequest struct {
	Title          *string  `json:"title,omitempty"`
	Description    *string  `json:"description,omitempty"`
	Status         *string  `json:"status,omitempty"`
	SwimLaneID     *int64   `json:"swim_lane_id,omitempty"`
	DueDate        *string  `json:"due_date,omitempty"`
	SprintID       *int64   `json:"sprint_id,omitempty"`
	Priority       *string  `json:"priority,omitempty"`
	AssigneeID     *int64   `json:"assignee_id,omitempty"`
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
			CreatedAt:      et.CreatedAt,
			UpdatedAt:      et.UpdatedAt,
			Tags:           []Tag{}, // Initialize empty tags array
		}

		// Convert task_number from *int to int64
		if et.TaskNumber != nil {
			t.TaskNumber = int64(*et.TaskNumber)
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

	// Parse due_date if provided
	var dueDate *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		parsed, err := time.Parse(time.RFC3339, *req.DueDate)
		if err == nil {
			dueDate = &parsed
		}
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

	// Commit transaction
	if err := entTx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit task creation", "internal_error")
		return
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
		CreatedAt:      createdTask.CreatedAt,
		UpdatedAt:      createdTask.UpdatedAt,
		Tags:           []Tag{},
	}

	// Convert task_number from *int to int64
	if createdTask.TaskNumber != nil {
		t.TaskNumber = int64(*createdTask.TaskNumber)
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

	// Parse due_date if provided
	var dueDate *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		parsed, err := time.Parse(time.RFC3339, *req.DueDate)
		if err == nil {
			dueDate = &parsed
		}
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
		CreatedAt:      updatedTask.CreatedAt,
		UpdatedAt:      updatedTask.UpdatedAt,
		Tags:           []Tag{},
	}

	// Convert task_number
	if updatedTask.TaskNumber != nil {
		t.TaskNumber = int64(*updatedTask.TaskNumber)
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
		CreatedAt:      taskEntity.CreatedAt,
		UpdatedAt:      taskEntity.UpdatedAt,
		Tags:           []Tag{},
	}

	// Convert task_number
	if taskEntity.TaskNumber != nil {
		t.TaskNumber = int64(*taskEntity.TaskNumber)
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

	respondJSON(w, http.StatusOK, t)
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
