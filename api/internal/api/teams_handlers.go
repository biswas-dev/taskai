package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/project"
	"taskai/ent/team"
	"taskai/ent/teaminvitation"
	"taskai/ent/teammember"
)

// maxOwnedTeams caps how many teams a single user can create.
const maxOwnedTeams = 50

// TeamSummary is one entry in the list of teams the caller belongs to.
type TeamSummary struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	OwnerID      int64  `json:"owner_id"`
	Role         string `json:"role"`
	IsOwner      bool   `json:"is_owner"`
	IsHome       bool   `json:"is_home"`
	MemberCount  int    `json:"member_count"`
	ProjectCount int    `json:"project_count"`
}

type MoveTeamMemberRequest struct {
	TargetTeamID int64 `json:"target_team_id"`
}

func toAPITeam(t *ent.Team) Team {
	return Team{
		ID:        t.ID,
		Name:      t.Name,
		OwnerID:   t.OwnerID,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func isTeamManager(role string) bool {
	return role == "owner" || role == "admin"
}

// resolveTeamID picks the team a request operates on. Routes under
// /teams/{teamId} use that team and require the caller to be an active member
// of it; the legacy /team routes fall back to the caller's home team. On
// failure the error response has already been written.
func (s *Server) resolveTeamID(ctx context.Context, w http.ResponseWriter, r *http.Request, userID int64) (int64, bool) {
	if raw := chi.URLParam(r, "teamId"); raw != "" {
		teamID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || teamID <= 0 {
			respondError(w, http.StatusBadRequest, "invalid team ID", "invalid_input")
			return 0, false
		}
		if _, err := s.getUserTeamRole(ctx, userID, teamID); err != nil {
			// Not distinguishing "doesn't exist" from "not a member" avoids
			// revealing which team IDs exist.
			respondError(w, http.StatusNotFound, "team not found", "not_found")
			return 0, false
		}
		return teamID, true
	}

	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active team found", "not_found")
		return 0, false
	}
	return teamID, true
}

// HandleListMyTeams returns every team the caller is an active member of.
func (s *Server) HandleListMyTeams(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	// Resolved before the list query: the SQLite test DB has one connection.
	homeTeamID, _ := s.getUserTeamID(ctx, userID)

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.owner_id, tm.role,
		       (SELECT COUNT(*) FROM team_members m WHERE m.team_id = t.id AND m.status = 'active'),
		       (SELECT COUNT(*) FROM projects p WHERE p.team_id = t.id)
		FROM team_members tm
		JOIN teams t ON t.id = tm.team_id
		WHERE tm.user_id = $1 AND tm.status = 'active'
		ORDER BY LOWER(t.name), t.id
	`, userID)
	if err != nil {
		s.logger.Error("Failed to list teams", zap.Error(err), zap.Int64("user_id", userID))
		respondError(w, http.StatusInternalServerError, "failed to fetch teams", "internal_error")
		return
	}
	defer rows.Close()

	teams := make([]TeamSummary, 0)
	for rows.Next() {
		var t TeamSummary
		if err := rows.Scan(&t.ID, &t.Name, &t.OwnerID, &t.Role, &t.MemberCount, &t.ProjectCount); err != nil {
			s.logger.Error("Failed to scan team row", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to fetch teams", "internal_error")
			return
		}
		t.IsOwner = t.OwnerID == userID
		t.IsHome = t.ID == homeTeamID
		teams = append(teams, t)
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("Failed to iterate team rows", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch teams", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, teams)
}

// HandleCreateTeam creates a new team owned by the caller.
func (s *Server) HandleCreateTeam(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var req CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		respondError(w, http.StatusBadRequest, "team name is required", "invalid_input")
		return
	}
	if len(name) > 100 {
		respondError(w, http.StatusBadRequest, "team name must be 100 characters or less", "invalid_input")
		return
	}

	owned, err := s.db.Client.Team.Query().Where(team.OwnerID(userID)).Count(ctx)
	if err != nil {
		s.logger.Error("Failed to count owned teams", zap.Error(err), zap.Int64("user_id", userID))
		respondError(w, http.StatusInternalServerError, "failed to create team", "internal_error")
		return
	}
	if owned >= maxOwnedTeams {
		respondError(w, http.StatusConflict, "you have reached the maximum number of teams", "team_limit")
		return
	}

	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create team", "internal_error")
		return
	}
	defer tx.Rollback()

	newTeam, err := tx.Team.Create().
		SetName(name).
		SetOwnerID(userID).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to create team", zap.Error(err), zap.Int64("user_id", userID))
		respondError(w, http.StatusInternalServerError, "failed to create team", "internal_error")
		return
	}

	if _, err := tx.TeamMember.Create().
		SetTeamID(newTeam.ID).
		SetUserID(userID).
		SetRole("owner").
		SetStatus("active").
		Save(ctx); err != nil {
		s.logger.Error("Failed to add team owner", zap.Error(err), zap.Int64("team_id", newTeam.ID))
		respondError(w, http.StatusInternalServerError, "failed to create team", "internal_error")
		return
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit team creation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create team", "internal_error")
		return
	}

	s.logger.Info("Team created",
		zap.Int64("team_id", newTeam.ID),
		zap.Int64("owner_id", userID),
	)

	respondJSON(w, http.StatusCreated, toAPITeam(newTeam))
}

// HandleDeleteTeam deletes a team. Only the owner can delete it, and only once
// it no longer holds projects and is not the owner's home team.
func (s *Server) HandleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	teamID, ok := s.resolveTeamID(ctx, w, r, userID)
	if !ok {
		return
	}

	entTeam, err := s.db.Client.Team.Get(ctx, teamID)
	if err != nil {
		s.logger.Error("Failed to get team", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to delete team", "internal_error")
		return
	}
	if entTeam.OwnerID != userID {
		respondError(w, http.StatusForbidden, "only the team owner can delete the team", "forbidden")
		return
	}

	homeTeamID, err := s.getUserTeamID(ctx, userID)
	if err == nil && homeTeamID == teamID {
		respondError(w, http.StatusConflict, "your primary team cannot be deleted", "home_team")
		return
	}

	projectCount, err := s.db.Client.Project.Query().Where(project.TeamID(teamID)).Count(ctx)
	if err != nil {
		s.logger.Error("Failed to count team projects", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to delete team", "internal_error")
		return
	}
	if projectCount > 0 {
		respondError(w, http.StatusConflict, "move this team's projects to another team before deleting it", "team_has_projects")
		return
	}

	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete team", "internal_error")
		return
	}
	defer tx.Rollback()

	if _, err := tx.TeamInvitation.Delete().Where(teaminvitation.TeamID(teamID)).Exec(ctx); err != nil {
		s.logger.Error("Failed to delete team invitations", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to delete team", "internal_error")
		return
	}
	if _, err := tx.TeamMember.Delete().Where(teammember.TeamID(teamID)).Exec(ctx); err != nil {
		s.logger.Error("Failed to delete team members", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to delete team", "internal_error")
		return
	}
	if err := tx.Team.DeleteOneID(teamID).Exec(ctx); err != nil {
		s.logger.Error("Failed to delete team", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to delete team", "internal_error")
		return
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit team deletion", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete team", "internal_error")
		return
	}

	s.logger.Info("Team deleted", zap.Int64("team_id", teamID), zap.Int64("deleted_by", userID))
	respondJSON(w, http.StatusOK, map[string]string{"message": "team deleted"})
}

// HandleMoveTeamMember moves a member from the team in the URL to another
// team. The caller must manage both teams. Project access is unaffected: it
// is always granted per project.
func (s *Server) HandleMoveTeamMember(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	teamID, ok := s.resolveTeamID(ctx, w, r, userID)
	if !ok {
		return
	}

	memberID, err := strconv.ParseInt(chi.URLParam(r, "memberId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid member ID", "invalid_input")
		return
	}

	var req MoveTeamMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}
	if req.TargetTeamID <= 0 || req.TargetTeamID == teamID {
		respondError(w, http.StatusBadRequest, "a different target team is required", "invalid_input")
		return
	}

	sourceRole, err := s.getUserTeamRole(ctx, userID, teamID)
	if err != nil || !isTeamManager(sourceRole) {
		respondError(w, http.StatusForbidden, "only team owners and admins can move members", "forbidden")
		return
	}
	targetRole, err := s.getUserTeamRole(ctx, userID, req.TargetTeamID)
	if err != nil || !isTeamManager(targetRole) {
		respondError(w, http.StatusForbidden, "you must be an owner or admin of the target team", "forbidden")
		return
	}

	member, err := s.db.Client.TeamMember.Query().
		Where(teammember.ID(memberID), teammember.TeamID(teamID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "member not found", "not_found")
			return
		}
		s.logger.Error("Failed to get member", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to move member", "internal_error")
		return
	}
	if member.Role == "owner" {
		respondError(w, http.StatusForbidden, "cannot move the team owner", "forbidden")
		return
	}

	alreadyInTarget, err := s.db.Client.TeamMember.Query().
		Where(teammember.TeamID(req.TargetTeamID), teammember.UserID(member.UserID)).
		Exist(ctx)
	if err != nil {
		s.logger.Error("Failed to check target membership", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to move member", "internal_error")
		return
	}

	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to move member", "internal_error")
		return
	}
	defer tx.Rollback()

	if err := tx.TeamMember.DeleteOneID(member.ID).Exec(ctx); err != nil {
		s.logger.Error("Failed to remove member from source team", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to move member", "internal_error")
		return
	}
	if !alreadyInTarget {
		if _, err := tx.TeamMember.Create().
			SetTeamID(req.TargetTeamID).
			SetUserID(member.UserID).
			SetRole(member.Role).
			SetStatus("active").
			Save(ctx); err != nil {
			s.logger.Error("Failed to add member to target team", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to move member", "internal_error")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit member move", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to move member", "internal_error")
		return
	}

	s.logger.Info("Team member moved",
		zap.Int64("user_id", member.UserID),
		zap.Int64("from_team_id", teamID),
		zap.Int64("to_team_id", req.TargetTeamID),
		zap.Int64("moved_by", userID),
	)

	respondJSON(w, http.StatusOK, map[string]string{"message": "member moved"})
}

// HandleLeaveTeam removes the caller from a team they don't own.
func (s *Server) HandleLeaveTeam(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	teamID, ok := s.resolveTeamID(ctx, w, r, userID)
	if !ok {
		return
	}

	entTeam, err := s.db.Client.Team.Get(ctx, teamID)
	if err != nil {
		s.logger.Error("Failed to get team", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to leave team", "internal_error")
		return
	}
	if entTeam.OwnerID == userID {
		respondError(w, http.StatusConflict, "the team owner cannot leave; delete the team instead", "owner_cannot_leave")
		return
	}

	activeTeams, err := s.db.Client.TeamMember.Query().
		Where(teammember.UserID(userID), teammember.Status("active")).
		Count(ctx)
	if err != nil {
		s.logger.Error("Failed to count memberships", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to leave team", "internal_error")
		return
	}
	if activeTeams <= 1 {
		respondError(w, http.StatusConflict, "you cannot leave your only team", "last_team")
		return
	}

	if _, err := s.db.Client.TeamMember.Delete().
		Where(teammember.TeamID(teamID), teammember.UserID(userID)).
		Exec(ctx); err != nil {
		s.logger.Error("Failed to leave team", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to leave team", "internal_error")
		return
	}

	s.logger.Info("User left team", zap.Int64("team_id", teamID), zap.Int64("user_id", userID))
	respondJSON(w, http.StatusOK, map[string]string{"message": "left team"})
}
