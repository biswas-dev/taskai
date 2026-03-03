package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// --- Types ---

// GitHubRepo is returned by HandleGitHubListRepos.
type GitHubRepo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Owner         string `json:"owner"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
}

// --- State JWT helpers ---

type oauthStateClaims struct {
	ProjectID int64 `json:"project_id"`
	UserID    int64 `json:"user_id"`
	jwt.RegisteredClaims
}

func signStateJWT(secret string, projectID, userID int64) (string, error) {
	claims := oauthStateClaims{
		ProjectID: projectID,
		UserID:    userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func parseStateJWT(secret, tokenStr string) (projectID, userID int64, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &oauthStateClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return 0, 0, err
	}
	claims, ok := token.Claims.(*oauthStateClaims)
	if !ok || !token.Valid {
		return 0, 0, fmt.Errorf("invalid state token")
	}
	return claims.ProjectID, claims.UserID, nil
}

// --- Handler 1: OAuth Init ---

// HandleGitHubOAuthInit generates a GitHub OAuth authorization URL.
// POST /api/projects/{id}/github/oauth-init
func (s *Server) HandleGitHubOAuthInit(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid project ID", "invalid_id")
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Unauthorized", "unauthorized")
		return
	}

	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), projectID)
	if err != nil || !isOwnerOrAdmin {
		respondError(w, http.StatusForbidden, "Forbidden", "forbidden")
		return
	}

	if s.config.GitHubClientID == "" {
		respondError(w, http.StatusServiceUnavailable, "GitHub OAuth is not configured", "not_configured")
		return
	}

	state, err := signStateJWT(s.config.JWTSecret, int64(projectID), userID)
	if err != nil {
		s.logger.Error("Failed to sign state JWT", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to generate state token", "internal_error")
		return
	}

	callbackURL := s.config.AppURL + "/api/auth/github/callback"
	// login=<username> forces GitHub to always show the full consent screen including
	// org grant buttons — without it, GitHub silently reuses the cached authorization.
	var ghLogin string
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(github_login,'') FROM projects WHERE id = $1`, projectID).Scan(&ghLogin)

	authURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=repo%%2Cread%%3Auser%%2Cread%%3Aorg&state=%s",
		url.QueryEscape(s.config.GitHubClientID),
		url.QueryEscape(callbackURL),
		url.QueryEscape(state),
	)
	if ghLogin != "" {
		authURL += "&login=" + url.QueryEscape(ghLogin)
	}

	respondJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

// --- Handler 2: OAuth Callback ---

// HandleGitHubCallback handles the GitHub OAuth callback.
// GET /api/auth/github/callback  (public)
func (s *Server) HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	stateStr := r.URL.Query().Get("state")

	if code == "" || stateStr == "" {
		http.Error(w, "Missing code or state", http.StatusBadRequest)
		return
	}

	projectID, _, err := parseStateJWT(s.config.JWTSecret, stateStr)
	if err != nil {
		s.logger.Warn("Invalid GitHub OAuth state", zap.Error(err))
		http.Redirect(w, r, s.config.AppURL+"/app?github=error&reason=invalid_state", http.StatusFound)
		return
	}

	// Exchange code for access token
	accessToken, err := s.exchangeGitHubCode(r.Context(), code)
	if err != nil {
		s.logger.Error("Failed to exchange GitHub code", zap.Error(err))
		http.Redirect(w, r, fmt.Sprintf("%s/app/projects/%d/settings?github=error&reason=token_exchange", s.config.AppURL, projectID), http.StatusFound)
		return
	}

	// Fetch GitHub user info
	var ghUserInfo struct {
		Login string `json:"login"`
	}
	if err := fetchGitHubJSON(r.Context(), accessToken, "https://api.github.com/user", &ghUserInfo); err != nil {
		s.logger.Error("Failed to fetch GitHub user info", zap.Error(err))
		http.Redirect(w, r, fmt.Sprintf("%s/app/projects/%d/settings?github=error&reason=user_fetch", s.config.AppURL, projectID), http.StatusFound)
		return
	}

	// Store token and login, clear manual repo fields
	_, err = s.db.ExecContext(r.Context(), `
		UPDATE projects
		SET github_token = $1,
		    github_login = $2,
		    github_owner = NULL,
		    github_repo_name = NULL,
		    github_repo_url = NULL
		WHERE id = $3
	`, accessToken, ghUserInfo.Login, projectID)
	if err != nil {
		s.logger.Error("Failed to save GitHub OAuth token", zap.Error(err), zap.Int64("project_id", projectID))
		http.Redirect(w, r, fmt.Sprintf("%s/app/projects/%d/settings?github=error&reason=db_save", s.config.AppURL, projectID), http.StatusFound)
		return
	}

	s.logger.Info("GitHub OAuth connected",
		zap.Int64("project_id", projectID),
		zap.String("github_login", ghUserInfo.Login),
	)

	http.Redirect(w, r, fmt.Sprintf("%s/app/projects/%d/settings?github=connected", s.config.AppURL, projectID), http.StatusFound)
}

// exchangeGitHubCode exchanges an OAuth code for an access token.
func (s *Server) exchangeGitHubCode(ctx context.Context, code string) (string, error) {
	body := url.Values{}
	body.Set("client_id", s.config.GitHubClientID)
	body.Set("client_secret", s.config.GitHubClientSecret)
	body.Set("code", code)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(bytes.NewReader(respBody)).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("github oauth error: %s - %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}
	return result.AccessToken, nil
}

// --- Handler 3: List Repos ---

// HandleGitHubListRepos returns the user's GitHub repositories.
// GET /api/projects/{id}/github/repos
func (s *Server) HandleGitHubListRepos(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid project ID", "invalid_id")
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Unauthorized", "unauthorized")
		return
	}

	hasAccess, err := s.userHasProjectAccess(int(userID), projectID)
	if err != nil || !hasAccess {
		respondError(w, http.StatusForbidden, "Forbidden", "forbidden")
		return
	}

	var tokenNull sql.NullString
	if err := s.db.QueryRowContext(r.Context(), `SELECT github_token FROM projects WHERE id = $1`, projectID).Scan(&tokenNull); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load project", "db_error")
		return
	}
	if !tokenNull.Valid || tokenNull.String == "" {
		respondError(w, http.StatusBadRequest, "GitHub is not connected for this project", "not_connected")
		return
	}
	token := tokenNull.String

	// Fetch up to 3 pages of repos
	type ghRepoRaw struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
		HTMLURL       string `json:"html_url"`
	}

	var repos []GitHubRepo
	for page := 1; page <= 3; page++ {
		var pageRepos []ghRepoRaw
		repoURL := fmt.Sprintf(
			"https://api.github.com/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator,organization_member&page=%d",
			page,
		)
		if err := fetchGitHubJSON(r.Context(), token, repoURL, &pageRepos); err != nil {
			s.logger.Error("Failed to fetch GitHub repos", zap.Int("page", page), zap.Error(err))
			respondError(w, http.StatusBadGateway, "Failed to fetch repositories from GitHub: "+err.Error(), "github_error")
			return
		}
		if len(pageRepos) == 0 {
			break
		}
		for _, pr := range pageRepos {
			repos = append(repos, GitHubRepo{
				ID:            pr.ID,
				FullName:      pr.FullName,
				Name:          pr.Name,
				Owner:         pr.Owner.Login,
				DefaultBranch: pr.DefaultBranch,
				Private:       pr.Private,
				HTMLURL:       pr.HTMLURL,
			})
		}
	}

	respondJSON(w, http.StatusOK, repos)
}

// --- Handler 4: Disconnect ---

// HandleGitHubDisconnect removes GitHub OAuth token and settings from a project.
// DELETE /api/projects/{id}/github/token
func (s *Server) HandleGitHubDisconnect(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid project ID", "invalid_id")
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Unauthorized", "unauthorized")
		return
	}

	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), projectID)
	if err != nil || !isOwnerOrAdmin {
		respondError(w, http.StatusForbidden, "Forbidden", "forbidden")
		return
	}

	_, err = s.db.ExecContext(r.Context(), `
		UPDATE projects
		SET github_token = NULL,
		    github_login = NULL,
		    github_owner = NULL,
		    github_repo_name = NULL,
		    github_repo_url = NULL
		WHERE id = $1
	`, projectID)
	if err != nil {
		s.logger.Error("Failed to disconnect GitHub", zap.Error(err), zap.Int("project_id", projectID))
		respondError(w, http.StatusInternalServerError, "Failed to disconnect GitHub", "db_error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "GitHub disconnected successfully"})
}
