package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/wikipage"
)

// WikiShareUser is someone a restricted page is shared with.
type WikiShareUser struct {
	UserID   int64   `json:"user_id"`
	Email    string  `json:"email"`
	UserName *string `json:"user_name,omitempty"`
}

// WikiSharingResponse describes who can see a wiki page.
type WikiSharingResponse struct {
	Visibility string          `json:"visibility"`
	CanManage  bool            `json:"can_manage"`
	CreatedBy  int64           `json:"created_by"`
	SharedWith []WikiShareUser `json:"shared_with"`
	// PublicToken is only returned to people who manage the page.
	PublicToken *string `json:"public_token,omitempty"`
}

// UpdateWikiSharingRequest sets a page's visibility and, for restricted
// pages, the full list of people it is shared with.
type UpdateWikiSharingRequest struct {
	Visibility string  `json:"visibility"`
	UserIDs    []int64 `json:"user_ids"`
}

// PublicWikiPageResponse is what anyone with a public link can read.
type PublicWikiPageResponse struct {
	Title       string    `json:"title"`
	HTML        string    `json:"html"`
	ProjectName string    `json:"project_name"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// loadWikiPageForSharing parses {pageId}, loads the page and checks the
// caller can see it. On failure the error response has been written.
func (s *Server) loadWikiPageForSharing(ctx context.Context, w http.ResponseWriter, r *http.Request, userID int64) (*ent.WikiPage, bool) {
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return nil, false
	}
	page, err := s.db.Client.WikiPage.Get(ctx, pageID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
			return nil, false
		}
		s.logger.Error("Failed to load wiki page", zap.Int64("page_id", pageID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to load wiki page", "internal_error")
		return nil, false
	}
	visible, err := s.canViewWikiPage(ctx, userID, page)
	if err != nil {
		s.logger.Error("Failed to check wiki page access", zap.Int64("page_id", pageID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to verify access", "internal_error")
		return nil, false
	}
	if !visible {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return nil, false
	}
	return page, true
}

// requireWikiSharingManager writes a 403 unless the caller may change sharing.
func (s *Server) requireWikiSharingManager(ctx context.Context, w http.ResponseWriter, userID int64, page *ent.WikiPage) bool {
	canManage, err := s.canManageWikiSharing(ctx, userID, page)
	if err != nil {
		s.logger.Error("Failed to check wiki sharing permission", zap.Int64("page_id", page.ID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to verify access", "internal_error")
		return false
	}
	if !canManage {
		respondError(w, http.StatusForbidden, "only the page creator or project owner can change sharing", "forbidden")
		return false
	}
	return true
}

func (s *Server) wikiSharingResponse(ctx context.Context, userID int64, page *ent.WikiPage) (WikiSharingResponse, error) {
	canManage, err := s.canManageWikiSharing(ctx, userID, page)
	if err != nil {
		return WikiSharingResponse{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email, u.name, u.first_name, u.last_name
		FROM wiki_page_shares ws
		JOIN users u ON u.id = ws.user_id
		WHERE ws.page_id = $1
		ORDER BY LOWER(u.email)`, page.ID)
	if err != nil {
		return WikiSharingResponse{}, err
	}
	defer rows.Close()

	shared := make([]WikiShareUser, 0)
	for rows.Next() {
		var (
			u                         WikiShareUser
			name, firstName, lastName *string
		)
		if err := rows.Scan(&u.UserID, &u.Email, &name, &firstName, &lastName); err != nil {
			return WikiSharingResponse{}, err
		}
		u.UserName = composeDisplayName(name, firstName, lastName)
		shared = append(shared, u)
	}
	if err := rows.Err(); err != nil {
		return WikiSharingResponse{}, err
	}

	resp := WikiSharingResponse{
		Visibility: page.Visibility,
		CanManage:  canManage,
		CreatedBy:  page.CreatedBy,
		SharedWith: shared,
	}
	if canManage {
		resp.PublicToken = page.PublicToken
	}
	return resp, nil
}

// HandleGetWikiSharing returns a page's visibility and who it is shared with.
// GET /wiki/pages/{pageId}/sharing
func (s *Server) HandleGetWikiSharing(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	page, ok := s.loadWikiPageForSharing(ctx, w, r, userID)
	if !ok {
		return
	}

	resp, err := s.wikiSharingResponse(ctx, userID, page)
	if err != nil {
		s.logger.Error("Failed to load wiki sharing", zap.Int64("page_id", page.ID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to load sharing", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// HandleUpdateWikiSharing sets a page's visibility and share list. Only
// members of the page's project can be added.
// PUT /wiki/pages/{pageId}/sharing
func (s *Server) HandleUpdateWikiSharing(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	page, ok := s.loadWikiPageForSharing(ctx, w, r, userID)
	if !ok || !s.requireWikiSharingManager(ctx, w, userID, page) {
		return
	}

	var req UpdateWikiSharingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}
	if req.Visibility != wikiVisibilityProject && req.Visibility != wikiVisibilityRestricted {
		respondError(w, http.StatusBadRequest, "visibility must be 'project' or 'restricted'", "invalid_input")
		return
	}
	if len(req.UserIDs) > 200 {
		respondError(w, http.StatusBadRequest, "too many people (max 200)", "invalid_input")
		return
	}

	// De-duplicate and make sure everyone is in the project.
	userIDs := make([]int64, 0, len(req.UserIDs))
	seen := map[int64]bool{}
	for _, uid := range req.UserIDs {
		if uid <= 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		member, err := s.checkProjectAccess(ctx, uid, page.ProjectID)
		if err != nil {
			s.logger.Error("Failed to check project membership", zap.Int64("user_id", uid), zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update sharing", "internal_error")
			return
		}
		if !member {
			respondError(w, http.StatusBadRequest, "pages can only be shared with members of this project", "not_project_member")
			return
		}
		userIDs = append(userIDs, uid)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update sharing", "internal_error")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `UPDATE wiki_pages SET visibility = $1 WHERE id = $2`, req.Visibility, page.ID); err != nil {
		s.logger.Error("Failed to update wiki visibility", zap.Int64("page_id", page.ID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update sharing", "internal_error")
		return
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_page_shares WHERE page_id = $1`, page.ID); err != nil {
		s.logger.Error("Failed to clear wiki shares", zap.Int64("page_id", page.ID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update sharing", "internal_error")
		return
	}
	for _, uid := range userIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO wiki_page_shares (page_id, user_id, granted_by) VALUES ($1, $2, $3)`,
			page.ID, uid, userID,
		); err != nil {
			s.logger.Error("Failed to add wiki share", zap.Int64("page_id", page.ID), zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update sharing", "internal_error")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit wiki sharing", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update sharing", "internal_error")
		return
	}

	s.logger.Info("Wiki page sharing updated",
		zap.Int64("page_id", page.ID),
		zap.String("visibility", req.Visibility),
		zap.Int("shared_with", len(userIDs)),
		zap.Int64("updated_by", userID),
	)

	page.Visibility = req.Visibility
	resp, err := s.wikiSharingResponse(ctx, userID, page)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load sharing", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func newPublicToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HandleCreateWikiPublicLink turns on the read-only public link (or returns
// the existing one).
// POST /wiki/pages/{pageId}/public-link
func (s *Server) HandleCreateWikiPublicLink(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	page, ok := s.loadWikiPageForSharing(ctx, w, r, userID)
	if !ok || !s.requireWikiSharingManager(ctx, w, userID, page) {
		return
	}

	if page.PublicToken == nil {
		token, err := newPublicToken()
		if err != nil {
			s.logger.Error("Failed to generate public token", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to create link", "internal_error")
			return
		}
		page, err = s.db.Client.WikiPage.UpdateOneID(page.ID).SetPublicToken(token).Save(ctx)
		if err != nil {
			s.logger.Error("Failed to save public token", zap.Int64("page_id", page.ID), zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to create link", "internal_error")
			return
		}
		s.logger.Info("Wiki public link enabled", zap.Int64("page_id", page.ID), zap.Int64("user_id", userID))
	}

	respondJSON(w, http.StatusOK, map[string]string{"public_token": *page.PublicToken})
}

// HandleDeleteWikiPublicLink revokes the public link; the old URL stops working.
// DELETE /wiki/pages/{pageId}/public-link
func (s *Server) HandleDeleteWikiPublicLink(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	page, ok := s.loadWikiPageForSharing(ctx, w, r, userID)
	if !ok || !s.requireWikiSharingManager(ctx, w, userID, page) {
		return
	}

	if _, err := s.db.Client.WikiPage.UpdateOneID(page.ID).ClearPublicToken().Save(ctx); err != nil {
		s.logger.Error("Failed to revoke public link", zap.Int64("page_id", page.ID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to revoke link", "internal_error")
		return
	}
	s.logger.Info("Wiki public link revoked", zap.Int64("page_id", page.ID), zap.Int64("user_id", userID))
	respondJSON(w, http.StatusOK, map[string]string{"message": "public link revoked"})
}

// HandleGetPublicWikiPage serves a page to anyone holding its public link.
// No authentication. GET /api/public/wiki/{token}
func (s *Server) HandleGetPublicWikiPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	token := chi.URLParam(r, "token")
	if len(token) < 20 || len(token) > 64 {
		respondError(w, http.StatusNotFound, "page not found", "not_found")
		return
	}

	page, err := s.db.Client.WikiPage.Query().
		Where(wikipage.PublicToken(token)).
		WithProject().
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			s.logger.Error("Failed to load public wiki page", zap.Error(err))
		}
		respondError(w, http.StatusNotFound, "page not found", "not_found")
		return
	}

	resp := PublicWikiPageResponse{
		Title:     page.Title,
		HTML:      renderWikiHTML(page.Content),
		UpdatedAt: page.UpdatedAt,
	}
	if page.Edges.Project != nil {
		resp.ProjectName = page.Edges.Project.Name
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	respondJSON(w, http.StatusOK, resp)
}
