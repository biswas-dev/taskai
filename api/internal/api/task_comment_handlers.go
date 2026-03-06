package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"taskai/ent"
	"taskai/ent/taskcomment"
)

type GitHubReaction struct {
	Reaction    string `json:"reaction"`
	Count       int    `json:"count"`
	UserReacted bool   `json:"user_reacted,omitempty"`
}

type TaskComment struct {
	ID              int64            `json:"id"`
	TaskID          int64            `json:"task_id"`
	UserID          int64            `json:"user_id"`
	UserName        *string          `json:"user_name,omitempty"`
	Comment         string           `json:"comment"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	GithubReactions []GitHubReaction `json:"github_reactions,omitempty"`
}

type CreateCommentRequest struct {
	Comment string `json:"comment"`
}

// HandleListTaskComments returns all comments for a task
func (s *Server) HandleListTaskComments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "invalid_input")
		return
	}

	// Get task and verify user has access to the project
	taskEntity, err := s.db.Client.Task.Get(ctx, taskID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get task", "internal_error")
		return
	}

	projectID := taskEntity.ProjectID

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Fetch comments with user info
	entComments, err := s.db.Client.TaskComment.Query().
		Where(taskcomment.TaskID(taskID)).
		WithUser().
		Order(ent.Asc(taskcomment.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch comments", "internal_error")
		return
	}

	comments := make([]TaskComment, 0, len(entComments))
	for _, ec := range entComments {
		c := TaskComment{
			ID:        ec.ID,
			TaskID:    ec.TaskID,
			UserID:    ec.UserID,
			Comment:   ec.Comment,
			CreatedAt: ec.CreatedAt,
			UpdatedAt: ec.UpdatedAt,
		}

		if ec.Edges.User != nil {
			c.UserName = userDisplayNamePtr(ec.Edges.User)
		}

		comments = append(comments, c)
	}

	// Bulk-fetch GitHub reactions for all comments via task_id join, including user_reacted
	if len(comments) > 0 {
		reactionRows, rErr := s.db.QueryContext(ctx, `
			SELECT gr.task_comment_id, gr.reaction, gr.count,
			       (ur.id IS NOT NULL) AS user_reacted
			FROM github_reactions gr
			LEFT JOIN user_reactions ur ON
			    ur.reaction = gr.reaction AND ur.user_id = $2 AND
			    ur.task_comment_id = gr.task_comment_id
			JOIN task_comments tc ON tc.id = gr.task_comment_id
			WHERE tc.task_id = $1 AND gr.count > 0
		`, taskID, userID)
		if rErr == nil {
			reactionMap := map[int64][]GitHubReaction{}
			for reactionRows.Next() {
				var cid int64
				var gr GitHubReaction
				if reactionRows.Scan(&cid, &gr.Reaction, &gr.Count, &gr.UserReacted) == nil {
					reactionMap[cid] = append(reactionMap[cid], gr)
				}
			}
			reactionRows.Close()
			for i := range comments {
				comments[i].GithubReactions = reactionMap[comments[i].ID]
			}
		}
	}

	respondJSON(w, http.StatusOK, comments)
}

// HandleCreateTaskComment creates a new comment on a task
func (s *Server) HandleCreateTaskComment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "invalid_input")
		return
	}

	// Get task and verify user has access to the project
	taskEntity, err := s.db.Client.Task.Get(ctx, taskID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get task", "internal_error")
		return
	}

	projectID := taskEntity.ProjectID

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	var req CreateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	// Validation
	if req.Comment == "" {
		respondError(w, http.StatusBadRequest, "comment is required", "invalid_input")
		return
	}
	if len(req.Comment) > 5000 {
		respondError(w, http.StatusBadRequest, "comment is too long (max 5000 characters)", "invalid_input")
		return
	}

	newComment, err := s.db.Client.TaskComment.Create().
		SetTaskID(taskID).
		SetUserID(userID).
		SetComment(req.Comment).
		Save(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create comment", "internal_error")
		return
	}

	// Fetch with user info
	commentWithUser, err := s.db.Client.TaskComment.Query().
		Where(taskcomment.ID(newComment.ID)).
		WithUser().
		Only(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch created comment", "internal_error")
		return
	}

	c := TaskComment{
		ID:        commentWithUser.ID,
		TaskID:    commentWithUser.TaskID,
		UserID:    commentWithUser.UserID,
		Comment:   commentWithUser.Comment,
		CreatedAt: commentWithUser.CreatedAt,
		UpdatedAt: commentWithUser.UpdatedAt,
	}

	if commentWithUser.Edges.User != nil {
		c.UserName = userDisplayNamePtr(commentWithUser.Edges.User)
	}

	// Best-effort push to GitHub (non-blocking)
	displayName := ""
	if c.UserName != nil {
		displayName = *c.UserName
	}
	go s.tryPushCommentToGitHub(context.Background(), taskID, c.ID, c.Comment, displayName)

	// Notify task assignee and previous commenters (best-effort, non-blocking)
	taskNum := 0
	if taskEntity.TaskNumber != nil {
		taskNum = *taskEntity.TaskNumber
	}
	taskLink := "/app/projects/" + int64ToStr(projectID) + "/tasks/" + strconv.Itoa(taskNum)
	go s.notifyTaskComment(context.Background(), taskID, projectID, userID, c.ID, taskEntity.Title, c.Comment, displayName, taskLink, taskEntity.AssigneeID)

	respondJSON(w, http.StatusCreated, c)
}
