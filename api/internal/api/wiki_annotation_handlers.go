package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"taskai/ent"
)

// ── Types ─────────────────────────────────────────────────────────────────

// WikiAnnotation is a text highlight with threaded comments on a wiki page.
type WikiAnnotation struct {
	ID           int64               `json:"id"`
	WikiPageID   int64               `json:"wiki_page_id"`
	AuthorID     int64               `json:"author_id"`
	AuthorName   *string             `json:"author_name,omitempty"`
	StartOffset  int                 `json:"start_offset"`
	EndOffset    int                 `json:"end_offset"`
	SelectedText string              `json:"selected_text"`
	Color        string              `json:"color"`
	Resolved     bool                `json:"resolved"`
	CreatedAt    time.Time           `json:"created_at"`
	Comments     []AnnotationComment `json:"comments"`
}

// AnnotationComment is a comment (or reply) on a wiki annotation.
type AnnotationComment struct {
	ID              int64     `json:"id"`
	AnnotationID    int64     `json:"annotation_id"`
	AuthorID        int64     `json:"author_id"`
	AuthorName      *string   `json:"author_name,omitempty"`
	ParentCommentID *int64    `json:"parent_comment_id,omitempty"`
	Content         string    `json:"content"`
	Resolved        bool      `json:"resolved"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

var errAnnotationForbidden = errors.New("forbidden")

var validAnnotationColors = map[string]bool{
	"yellow": true,
	"blue":   true,
	"green":  true,
	"red":    true,
}

// ── Access helper ──────────────────────────────────────────────────────────

func (s *Server) checkWikiPageAccess(ctx context.Context, userID, pageID int64) error {
	page, err := s.db.Client.WikiPage.Get(ctx, pageID)
	if err != nil {
		return err
	}
	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		return err
	}
	if !hasAccess {
		return errAnnotationForbidden
	}
	return nil
}

func handleWikiAccessError(w http.ResponseWriter, err error) {
	if ent.IsNotFound(err) {
		respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
	} else if err == errAnnotationForbidden {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
	} else {
		respondError(w, http.StatusInternalServerError, "failed to verify access", "internal_error")
	}
}

// ── Annotation handlers ────────────────────────────────────────────────────

// HandleListWikiAnnotations returns all annotations for a page, each with its comments.
func (s *Server) HandleListWikiAnnotations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	if err := s.checkWikiPageAccess(ctx, userID, pageID); err != nil {
		handleWikiAccessError(w, err)
		return
	}

	// Fetch annotations ordered by position
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.wiki_page_id, a.author_id, u.name,
		       a.start_offset, a.end_offset, a.selected_text, a.color, a.resolved, a.created_at
		FROM wiki_annotations a
		LEFT JOIN users u ON u.id = a.author_id
		WHERE a.wiki_page_id = $1
		ORDER BY a.start_offset ASC
	`, pageID)
	if err != nil {
		s.logger.Error("Failed to fetch wiki annotations", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch annotations", "internal_error")
		return
	}
	defer rows.Close()

	annotations := []WikiAnnotation{}
	annIndex := map[int64]int{} // annotation ID → index in slice

	for rows.Next() {
		var a WikiAnnotation
		if scanErr := rows.Scan(
			&a.ID, &a.WikiPageID, &a.AuthorID, &a.AuthorName,
			&a.StartOffset, &a.EndOffset, &a.SelectedText, &a.Color, &a.Resolved, &a.CreatedAt,
		); scanErr != nil {
			continue
		}
		a.Comments = []AnnotationComment{}
		annIndex[a.ID] = len(annotations)
		annotations = append(annotations, a)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read annotations", "internal_error")
		return
	}

	if len(annotations) > 0 {
		// Fetch all comments for this page's annotations in one query
		commentRows, cErr := s.db.QueryContext(ctx, `
			SELECT c.id, c.annotation_id, c.author_id, u.name, c.parent_comment_id,
			       c.content, c.resolved, c.created_at, c.updated_at
			FROM wiki_annotation_comments c
			LEFT JOIN users u ON u.id = c.author_id
			JOIN wiki_annotations a ON a.id = c.annotation_id
			WHERE a.wiki_page_id = $1
			ORDER BY c.created_at ASC
		`, pageID)
		if cErr == nil {
			defer commentRows.Close()
			for commentRows.Next() {
				var c AnnotationComment
				if scanErr := commentRows.Scan(
					&c.ID, &c.AnnotationID, &c.AuthorID, &c.AuthorName,
					&c.ParentCommentID, &c.Content, &c.Resolved, &c.CreatedAt, &c.UpdatedAt,
				); scanErr != nil {
					continue
				}
				if idx, ok := annIndex[c.AnnotationID]; ok {
					annotations[idx].Comments = append(annotations[idx].Comments, c)
				}
			}
		}
	}

	respondJSON(w, http.StatusOK, annotations)
}

// HandleCreateWikiAnnotation creates a new annotation with an optional first comment.
func (s *Server) HandleCreateWikiAnnotation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	if err := s.checkWikiPageAccess(ctx, userID, pageID); err != nil {
		handleWikiAccessError(w, err)
		return
	}

	var req struct {
		StartOffset  int    `json:"start_offset"`
		EndOffset    int    `json:"end_offset"`
		SelectedText string `json:"selected_text"`
		Color        string `json:"color"`
		Comment      string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, errInvalidRequestBody, "invalid_input")
		return
	}

	if req.SelectedText == "" {
		respondError(w, http.StatusBadRequest, "selected_text is required", "invalid_input")
		return
	}
	if req.StartOffset < 0 || req.EndOffset <= req.StartOffset {
		respondError(w, http.StatusBadRequest, "invalid offsets", "invalid_input")
		return
	}
	if req.Color == "" {
		req.Color = "yellow"
	}
	if !validAnnotationColors[req.Color] {
		respondError(w, http.StatusBadRequest, "invalid color (yellow/blue/green/red)", "invalid_input")
		return
	}

	var annotationID int64
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO wiki_annotations (wiki_page_id, author_id, start_offset, end_offset, selected_text, color)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, pageID, userID, req.StartOffset, req.EndOffset, req.SelectedText, req.Color).Scan(&annotationID)
	if err != nil {
		s.logger.Error("Failed to create wiki annotation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create annotation", "internal_error")
		return
	}

	// Optionally add first comment
	if req.Comment != "" && len(req.Comment) <= 5000 {
		if _, cErr := s.db.ExecContext(ctx, `
			INSERT INTO wiki_annotation_comments (annotation_id, author_id, content)
			VALUES ($1, $2, $3)
		`, annotationID, userID, req.Comment); cErr != nil {
			s.logger.Warn("Failed to create initial annotation comment", zap.Error(cErr))
		}
	}

	// Return full annotation with comments
	annotation := s.fetchAnnotationByID(ctx, annotationID)
	if annotation == nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch created annotation", "internal_error")
		return
	}

	respondJSON(w, http.StatusCreated, annotation)
}

// HandleDeleteWikiAnnotation deletes an annotation (author only).
func (s *Server) HandleDeleteWikiAnnotation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	annotationID, err := strconv.ParseInt(chi.URLParam(r, "annotationId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid annotation ID", "invalid_input")
		return
	}

	// Verify ownership and access
	var authorID int64
	var pageID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT author_id, wiki_page_id FROM wiki_annotations WHERE id = $1`, annotationID,
	).Scan(&authorID, &pageID); err != nil {
		respondError(w, http.StatusNotFound, "annotation not found", "not_found")
		return
	}

	if err := s.checkWikiPageAccess(ctx, userID, pageID); err != nil {
		handleWikiAccessError(w, err)
		return
	}
	if authorID != userID {
		respondError(w, http.StatusForbidden, "only the author can delete an annotation", "forbidden")
		return
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM wiki_annotations WHERE id = $1`, annotationID); err != nil {
		s.logger.Error("Failed to delete wiki annotation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete annotation", "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleUpdateWikiAnnotation updates color or resolved state of an annotation.
func (s *Server) HandleUpdateWikiAnnotation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	annotationID, err := strconv.ParseInt(chi.URLParam(r, "annotationId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid annotation ID", "invalid_input")
		return
	}

	var req struct {
		Color    *string `json:"color,omitempty"`
		Resolved *bool   `json:"resolved,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, errInvalidRequestBody, "invalid_input")
		return
	}

	var pageID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT wiki_page_id FROM wiki_annotations WHERE id = $1`, annotationID,
	).Scan(&pageID); err != nil {
		respondError(w, http.StatusNotFound, "annotation not found", "not_found")
		return
	}

	if err := s.checkWikiPageAccess(ctx, userID, pageID); err != nil {
		handleWikiAccessError(w, err)
		return
	}

	if req.Color != nil {
		if !validAnnotationColors[*req.Color] {
			respondError(w, http.StatusBadRequest, "invalid color (yellow/blue/green/red)", "invalid_input")
			return
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE wiki_annotations SET color = $1 WHERE id = $2`, *req.Color, annotationID,
		); err != nil {
			s.logger.Error("Failed to update annotation color", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update annotation", "internal_error")
			return
		}
	}

	if req.Resolved != nil {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE wiki_annotations SET resolved = $1 WHERE id = $2`, *req.Resolved, annotationID,
		); err != nil {
			s.logger.Error("Failed to update annotation resolved", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update annotation", "internal_error")
			return
		}
	}

	annotation := s.fetchAnnotationByID(ctx, annotationID)
	if annotation == nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch annotation", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, annotation)
}

// ── Comment handlers ───────────────────────────────────────────────────────

// HandleCreateAnnotationComment adds a comment or reply to an annotation.
func (s *Server) HandleCreateAnnotationComment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	annotationID, err := strconv.ParseInt(chi.URLParam(r, "annotationId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid annotation ID", "invalid_input")
		return
	}

	var req struct {
		Content         string `json:"content"`
		ParentCommentID *int64 `json:"parent_comment_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, errInvalidRequestBody, "invalid_input")
		return
	}
	if req.Content == "" {
		respondError(w, http.StatusBadRequest, "content is required", "invalid_input")
		return
	}
	if len(req.Content) > 5000 {
		respondError(w, http.StatusBadRequest, "content too long (max 5000 characters)", "invalid_input")
		return
	}

	// Verify annotation exists and user has access
	var pageID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT wiki_page_id FROM wiki_annotations WHERE id = $1`, annotationID,
	).Scan(&pageID); err != nil {
		respondError(w, http.StatusNotFound, "annotation not found", "not_found")
		return
	}

	if err := s.checkWikiPageAccess(ctx, userID, pageID); err != nil {
		handleWikiAccessError(w, err)
		return
	}

	// Fetch project_id for notifications
	var projectID int64
	_ = s.db.QueryRowContext(ctx, `SELECT project_id FROM wiki_pages WHERE id = $1`, pageID).Scan(&projectID)

	var commentID int64
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO wiki_annotation_comments (annotation_id, author_id, parent_comment_id, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, annotationID, userID, req.ParentCommentID, req.Content).Scan(&commentID)
	if err != nil {
		s.logger.Error("Failed to create annotation comment", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create comment", "internal_error")
		return
	}

	comment := s.fetchAnnotationCommentByID(ctx, commentID)
	if comment == nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch created comment", "internal_error")
		return
	}

	// Notifications (best-effort, non-blocking)
	if projectID > 0 {
		commenterName := ""
		if comment.AuthorName != nil {
			commenterName = *comment.AuthorName
		}
		annLink := "/app/projects/" + int64ToStr(projectID) + "?tab=wiki"
		go s.notifyAnnotationComment(context.Background(), annotationID, projectID, userID, commentID, req.Content, commenterName, annLink, req.ParentCommentID)
	}

	respondJSON(w, http.StatusCreated, comment)
}

// HandleUpdateAnnotationComment edits or resolves a comment (author only).
func (s *Server) HandleUpdateAnnotationComment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	commentID, err := strconv.ParseInt(chi.URLParam(r, "commentId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid comment ID", "invalid_input")
		return
	}

	var req struct {
		Content  *string `json:"content,omitempty"`
		Resolved *bool   `json:"resolved,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, errInvalidRequestBody, "invalid_input")
		return
	}

	var authorID int64
	var annotationID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT author_id, annotation_id FROM wiki_annotation_comments WHERE id = $1`, commentID,
	).Scan(&authorID, &annotationID); err != nil {
		respondError(w, http.StatusNotFound, "comment not found", "not_found")
		return
	}

	if authorID != userID {
		respondError(w, http.StatusForbidden, "only the author can edit this comment", "forbidden")
		return
	}

	if req.Content != nil {
		if len(*req.Content) > 5000 {
			respondError(w, http.StatusBadRequest, "content too long (max 5000 characters)", "invalid_input")
			return
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE wiki_annotation_comments SET content = $1, updated_at = NOW() WHERE id = $2`,
			*req.Content, commentID,
		); err != nil {
			s.logger.Error("Failed to update annotation comment content", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update comment", "internal_error")
			return
		}
	}

	if req.Resolved != nil {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE wiki_annotation_comments SET resolved = $1, updated_at = NOW() WHERE id = $2`,
			*req.Resolved, commentID,
		); err != nil {
			s.logger.Error("Failed to update annotation comment resolved", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update comment", "internal_error")
			return
		}
	}

	comment := s.fetchAnnotationCommentByID(ctx, commentID)
	if comment == nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch comment", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, comment)
}

// HandleDeleteAnnotationComment deletes a comment (author only).
func (s *Server) HandleDeleteAnnotationComment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	commentID, err := strconv.ParseInt(chi.URLParam(r, "commentId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid comment ID", "invalid_input")
		return
	}

	var authorID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT author_id FROM wiki_annotation_comments WHERE id = $1`, commentID,
	).Scan(&authorID); err != nil {
		respondError(w, http.StatusNotFound, "comment not found", "not_found")
		return
	}

	if authorID != userID {
		respondError(w, http.StatusForbidden, "only the author can delete this comment", "forbidden")
		return
	}

	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM wiki_annotation_comments WHERE id = $1`, commentID,
	); err != nil {
		s.logger.Error("Failed to delete annotation comment", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete comment", "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Internal fetch helpers ─────────────────────────────────────────────────

func (s *Server) fetchAnnotationByID(ctx context.Context, annotationID int64) *WikiAnnotation {
	var a WikiAnnotation
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id, a.wiki_page_id, a.author_id, u.name,
		       a.start_offset, a.end_offset, a.selected_text, a.color, a.resolved, a.created_at
		FROM wiki_annotations a
		LEFT JOIN users u ON u.id = a.author_id
		WHERE a.id = $1
	`, annotationID).Scan(
		&a.ID, &a.WikiPageID, &a.AuthorID, &a.AuthorName,
		&a.StartOffset, &a.EndOffset, &a.SelectedText, &a.Color, &a.Resolved, &a.CreatedAt,
	)
	if err != nil {
		return nil
	}
	a.Comments = []AnnotationComment{}

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.annotation_id, c.author_id, u.name, c.parent_comment_id,
		       c.content, c.resolved, c.created_at, c.updated_at
		FROM wiki_annotation_comments c
		LEFT JOIN users u ON u.id = c.author_id
		WHERE c.annotation_id = $1
		ORDER BY c.created_at ASC
	`, annotationID)
	if err != nil {
		return &a
	}
	defer rows.Close()
	for rows.Next() {
		var c AnnotationComment
		if scanErr := rows.Scan(
			&c.ID, &c.AnnotationID, &c.AuthorID, &c.AuthorName,
			&c.ParentCommentID, &c.Content, &c.Resolved, &c.CreatedAt, &c.UpdatedAt,
		); scanErr == nil {
			a.Comments = append(a.Comments, c)
		}
	}
	return &a
}

func (s *Server) fetchAnnotationCommentByID(ctx context.Context, commentID int64) *AnnotationComment {
	var c AnnotationComment
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.annotation_id, c.author_id, u.name, c.parent_comment_id,
		       c.content, c.resolved, c.created_at, c.updated_at
		FROM wiki_annotation_comments c
		LEFT JOIN users u ON u.id = c.author_id
		WHERE c.id = $1
	`, commentID).Scan(
		&c.ID, &c.AnnotationID, &c.AuthorID, &c.AuthorName,
		&c.ParentCommentID, &c.Content, &c.Resolved, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil
	}
	return &c
}
