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
	"taskai/ent/team"
	"taskai/ent/teammember"
	"taskai/ent/teaminvitation"
	"taskai/ent/user"
)

type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	OwnerID   int64     `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TeamMember struct {
	ID       int64     `json:"id"`
	TeamID   int64     `json:"team_id"`
	UserID   int64     `json:"user_id"`
	UserName *string   `json:"user_name,omitempty"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	Status   string    `json:"status"`
	JoinedAt time.Time `json:"joined_at"`
}

type TeamInvitation struct {
	ID            int64      `json:"id"`
	TeamID        int64      `json:"team_id"`
	TeamName      string     `json:"team_name"`
	InviterID     int64      `json:"inviter_id"`
	InviterName   *string    `json:"inviter_name,omitempty"`
	InviteeEmail  string     `json:"invitee_email"`
	InviteeID     *int64     `json:"invitee_id,omitempty"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	RespondedAt   *time.Time `json:"responded_at,omitempty"`
}

type CreateTeamRequest struct {
	Name string `json:"name"`
}

type InviteTeamMemberRequest struct {
	Email string `json:"email"`
}

type UpdateTeamMemberRequest struct {
	Role string `json:"role"`
}

type UpdateTeamRequest struct {
	Name string `json:"name"`
}

type AddTeamMemberRequest struct {
	UserID int64 `json:"user_id"`
}

type UserSearchResult struct {
	ID    int64   `json:"id"`
	Email string  `json:"email"`
	Name  *string `json:"name,omitempty"`
}

type SentInvitation struct {
	ID           int64     `json:"id"`
	InviteeEmail string    `json:"invitee_email"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

// HandleGetMyTeam returns the current user's team
func (s *Server) HandleGetMyTeam(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	// Get user's active team membership
	entTeam, err := s.db.Client.Team.Query().
		Where(team.HasMembersWith(
			teammember.UserID(userID),
			teammember.Status("active"),
		)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "no active team found", "not_found")
			return
		}
		s.logger.Error("Failed to get user's team", zap.Error(err), zap.Int64("user_id", userID))
		respondError(w, http.StatusInternalServerError, "failed to fetch team", "internal_error")
		return
	}

	apiTeam := Team{
		ID:        entTeam.ID,
		Name:      entTeam.Name,
		OwnerID:   entTeam.OwnerID,
		CreatedAt: entTeam.CreatedAt,
		UpdatedAt: entTeam.UpdatedAt,
	}

	respondJSON(w, http.StatusOK, apiTeam)
}

// HandleGetTeamMembers returns all members of the user's team
func (s *Server) HandleGetTeamMembers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	// Get user's team ID
	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active team found", "not_found")
		return
	}

	// Get all team members with user info
	entMembers, err := s.db.Client.TeamMember.Query().
		Where(teammember.TeamID(teamID)).
		WithUser().
		Order(ent.Desc(teammember.FieldRole), ent.Asc(teammember.FieldJoinedAt)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to get team members", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to fetch team members", "internal_error")
		return
	}

	members := make([]TeamMember, 0, len(entMembers))
	for _, em := range entMembers {
		m := TeamMember{
			ID:       em.ID,
			TeamID:   em.TeamID,
			UserID:   em.UserID,
			Role:     em.Role,
			Status:   em.Status,
			JoinedAt: em.JoinedAt,
		}

		if em.Edges.User != nil {
			m.UserName = userDisplayNamePtr(em.Edges.User)
			m.Email = em.Edges.User.Email
		}

		members = append(members, m)
	}

	respondJSON(w, http.StatusOK, members)
}

// HandleInviteTeamMember sends an invitation to join the team
func (s *Server) HandleInviteTeamMember(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var req InviteTeamMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	// Validate email
	if req.Email == "" || !isValidEmail(req.Email) {
		respondError(w, http.StatusBadRequest, "valid email is required", "invalid_input")
		return
	}

	// Get user's team ID
	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active team found", "not_found")
		return
	}

	// Check if user is owner or admin
	role, err := s.getUserTeamRole(ctx, userID, teamID)
	if err != nil || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "only team owners and admins can invite members", "forbidden")
		return
	}

	// Check if invitee is already a member
	existingMember, err := s.db.Client.TeamMember.Query().
		Where(
			teammember.TeamID(teamID),
			teammember.HasUserWith(user.Email(req.Email)),
		).
		First(ctx)
	if err == nil && existingMember != nil {
		respondError(w, http.StatusConflict, "user is already a team member", "already_member")
		return
	} else if err != nil && !ent.IsNotFound(err) {
		s.logger.Error("Failed to check existing member", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to check membership", "internal_error")
		return
	}

	// Check if there's already a pending invitation
	existingInv, err := s.db.Client.TeamInvitation.Query().
		Where(
			teaminvitation.TeamID(teamID),
			teaminvitation.InviteeEmail(req.Email),
			teaminvitation.Status("pending"),
		).
		First(ctx)
	if err == nil && existingInv != nil {
		respondError(w, http.StatusConflict, "pending invitation already exists", "invitation_exists")
		return
	} else if err != nil && !ent.IsNotFound(err) {
		s.logger.Error("Failed to check existing invitation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to check invitation", "internal_error")
		return
	}

	// Get invitee user ID if they exist
	var inviteeID *int64
	inviteeUser, err := s.db.Client.User.Query().
		Where(user.Email(req.Email)).
		Only(ctx)
	if err == nil {
		inviteeID = &inviteeUser.ID
	} else if err != nil && !ent.IsNotFound(err) {
		s.logger.Error("Failed to get invitee user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get user", "internal_error")
		return
	}

	// Generate acceptance token for one-click email acceptance
	acceptanceToken, tokenErr := generateInviteCode()
	if tokenErr != nil {
		s.logger.Error("Failed to generate acceptance token", zap.Error(tokenErr))
		respondError(w, http.StatusInternalServerError, "failed to create invitation", "internal_error")
		return
	}

	// Create invitation with acceptance token (expires in 7 days)
	tokenExpires := time.Now().Add(7 * 24 * time.Hour)
	newInv, err := s.db.Client.TeamInvitation.Create().
		SetTeamID(teamID).
		SetInviterID(userID).
		SetInviteeEmail(req.Email).
		SetNillableInviteeID(inviteeID).
		SetStatus("pending").
		SetAcceptanceToken(acceptanceToken).
		SetTokenExpiresAt(tokenExpires).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to create invitation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create invitation", "internal_error")
		return
	}

	invitationID := newInv.ID

	// Fetch created invitation with edges
	invitation, err := s.getInvitation(ctx, invitationID)
	if err != nil {
		s.logger.Error("Failed to fetch created invitation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch invitation", "internal_error")
		return
	}

	s.logger.Info("Team invitation created",
		zap.Int64("invitation_id", invitationID),
		zap.Int64("team_id", teamID),
		zap.String("invitee_email", req.Email),
	)

	// Send email notification if email service is available
	if emailSvc := s.GetEmailService(); emailSvc != nil {
		// Get inviter name
		inviter, err := s.db.Client.User.Get(ctx, userID)
		inviterName := ""
		if err == nil {
			inviterName = userDisplayName(inviter)
		}

		// Get team name
		teamEntity, err := s.db.Client.Team.Get(ctx, teamID)
		teamName := ""
		if err == nil {
			teamName = teamEntity.Name
		}

		appURL := s.getAppURL()

		if inviteeID != nil {
			// Existing user — send project invitation with accept link
			if err := emailSvc.SendProjectInvitation(ctx, req.Email, inviterName, teamName, acceptanceToken, appURL); err != nil {
				s.logger.Warn("Failed to send team invitation email",
					zap.String("to", req.Email),
					zap.Error(err),
				)
			}
		} else {
			// New user — auto-generate invite code and send signup link with accept token
			inviteCode, codeErr := generateTeamInviteCode()
			if codeErr == nil {
				// Create a platform invite for this user
				expireTime := time.Now().Add(7 * 24 * time.Hour)
				_, err := s.db.Client.Invite.Create().
					SetCode(inviteCode).
					SetInviterID(userID).
					SetExpiresAt(expireTime).
					Save(ctx)
				if err == nil {
					// Store invite code on the team invitation for retrieval during acceptance
					_, _ = s.db.Client.TeamInvitation.UpdateOneID(invitationID).
						SetInviteCode(inviteCode).
						Save(ctx)

					if err := emailSvc.SendProjectInvitationNewUser(ctx, req.Email, inviterName, teamName, acceptanceToken, appURL); err != nil {
						s.logger.Warn("Failed to send team invitation email to new user",
							zap.String("to", req.Email),
							zap.Error(err),
						)
					}
				}
			}
		}
	}

	respondJSON(w, http.StatusCreated, invitation)
}

// HandleGetMyInvitations returns all pending invitations for the current user
func (s *Server) HandleGetMyInvitations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	email := r.Context().Value(UserEmailKey).(string)

	// Get pending invitations for this user
	entInvitations, err := s.db.Client.TeamInvitation.Query().
		Where(
			teaminvitation.Or(
				teaminvitation.InviteeID(userID),
				teaminvitation.InviteeEmail(email),
			),
			teaminvitation.Status("pending"),
		).
		WithTeam().
		WithInviter().
		Order(ent.Desc(teaminvitation.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to get invitations", zap.Error(err), zap.Int64("user_id", userID))
		respondError(w, http.StatusInternalServerError, "failed to fetch invitations", "internal_error")
		return
	}

	invitations := make([]TeamInvitation, 0, len(entInvitations))
	for _, entInv := range entInvitations {
		inv := TeamInvitation{
			ID:           entInv.ID,
			TeamID:       entInv.TeamID,
			InviterID:    entInv.InviterID,
			InviteeEmail: entInv.InviteeEmail,
			InviteeID:    entInv.InviteeID,
			Status:       entInv.Status,
			CreatedAt:    entInv.CreatedAt,
			RespondedAt:  entInv.RespondedAt,
		}

		if entInv.Edges.Team != nil {
			inv.TeamName = entInv.Edges.Team.Name
		}
		if entInv.Edges.Inviter != nil {
			inv.InviterName = userDisplayNamePtr(entInv.Edges.Inviter)
		}

		invitations = append(invitations, inv)
	}

	respondJSON(w, http.StatusOK, invitations)
}

// HandleAcceptInvitation accepts a team invitation
func (s *Server) HandleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	email := r.Context().Value(UserEmailKey).(string)

	invitationID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid invitation ID", "invalid_input")
		return
	}

	// Get invitation and verify it's for this user
	entInv, err := s.db.Client.TeamInvitation.Get(ctx, invitationID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "invitation not found", "not_found")
			return
		}
		s.logger.Error("Failed to get invitation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch invitation", "internal_error")
		return
	}

	// Verify invitation is for this user
	if entInv.InviteeEmail != email && (entInv.InviteeID == nil || *entInv.InviteeID != userID) {
		respondError(w, http.StatusForbidden, "invitation is not for you", "forbidden")
		return
	}

	// Check if invitation is still pending
	if entInv.Status != "pending" {
		respondError(w, http.StatusConflict, "invitation already responded to", "already_responded")
		return
	}

	// Begin Ent transaction
	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to process invitation", "internal_error")
		return
	}
	defer tx.Rollback()

	// Update invitation status
	now := time.Now()
	_, err = tx.TeamInvitation.UpdateOneID(invitationID).
		SetStatus("accepted").
		SetInviteeID(userID).
		SetRespondedAt(now).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to update invitation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update invitation", "internal_error")
		return
	}

	// Add user to team
	_, err = tx.TeamMember.Create().
		SetTeamID(entInv.TeamID).
		SetUserID(userID).
		SetRole("member").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to add team member", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to add team member", "internal_error")
		return
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to process invitation", "internal_error")
		return
	}

	s.logger.Info("Invitation accepted",
		zap.Int64("invitation_id", invitationID),
		zap.Int64("user_id", userID),
		zap.Int64("team_id", entInv.TeamID),
	)

	respondJSON(w, http.StatusOK, map[string]string{"message": "invitation accepted"})
}

// HandleRejectInvitation rejects a team invitation
func (s *Server) HandleRejectInvitation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	email := r.Context().Value(UserEmailKey).(string)

	invitationID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid invitation ID", "invalid_input")
		return
	}

	// Get invitation and verify it's for this user
	entInv, err := s.db.Client.TeamInvitation.Get(ctx, invitationID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "invitation not found", "not_found")
			return
		}
		s.logger.Error("Failed to get invitation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch invitation", "internal_error")
		return
	}

	// Verify invitation is for this user
	if entInv.InviteeEmail != email && (entInv.InviteeID == nil || *entInv.InviteeID != userID) {
		respondError(w, http.StatusForbidden, "invitation is not for you", "forbidden")
		return
	}

	// Check if invitation is still pending
	if entInv.Status != "pending" {
		respondError(w, http.StatusConflict, "invitation already responded to", "already_responded")
		return
	}

	// Update invitation status
	now := time.Now()
	_, err = s.db.Client.TeamInvitation.UpdateOneID(invitationID).
		SetStatus("rejected").
		SetInviteeID(userID).
		SetRespondedAt(now).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to reject invitation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to reject invitation", "internal_error")
		return
	}

	s.logger.Info("Invitation rejected",
		zap.Int64("invitation_id", invitationID),
		zap.Int64("user_id", userID),
	)

	respondJSON(w, http.StatusOK, map[string]string{"message": "invitation rejected"})
}

// HandleRemoveTeamMember removes a member from the team
func (s *Server) HandleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	memberID, err := strconv.ParseInt(chi.URLParam(r, "memberId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid member ID", "invalid_input")
		return
	}

	// Get user's team ID
	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active team found", "not_found")
		return
	}

	// Check if user is owner or admin
	role, err := s.getUserTeamRole(ctx, userID, teamID)
	if err != nil || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "only team owners and admins can remove members", "forbidden")
		return
	}

	// Get member to remove
	member, err := s.db.Client.TeamMember.Query().
		Where(
			teammember.ID(memberID),
			teammember.TeamID(teamID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "member not found", "not_found")
			return
		}
		s.logger.Error("Failed to get member", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get member", "internal_error")
		return
	}

	// Cannot remove team owner
	if member.Role == "owner" {
		respondError(w, http.StatusForbidden, "cannot remove team owner", "forbidden")
		return
	}

	memberUserID := member.UserID

	// Delete team member
	err = s.db.Client.TeamMember.DeleteOneID(memberID).Exec(ctx)
	if err != nil {
		s.logger.Error("Failed to remove team member", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to remove member", "internal_error")
		return
	}

	s.logger.Info("Team member removed",
		zap.Int64("member_id", memberID),
		zap.Int64("user_id", memberUserID),
		zap.Int64("team_id", teamID),
	)

	respondJSON(w, http.StatusOK, map[string]string{"message": "member removed"})
}

// HandleUpdateTeam updates the team name
func (s *Server) HandleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var req UpdateTeamRequest
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

	// Get user's team ID
	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active team found", "not_found")
		return
	}

	// Check if user is owner or admin
	role, err := s.getUserTeamRole(ctx, userID, teamID)
	if err != nil || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "only team owners and admins can update the team", "forbidden")
		return
	}

	// Update team name
	entTeam, err := s.db.Client.Team.UpdateOneID(teamID).
		SetName(name).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to update team", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to update team", "internal_error")
		return
	}

	s.logger.Info("Team updated",
		zap.Int64("team_id", teamID),
		zap.String("new_name", name),
		zap.Int64("updated_by", userID),
	)

	respondJSON(w, http.StatusOK, Team{
		ID:        entTeam.ID,
		Name:      entTeam.Name,
		OwnerID:   entTeam.OwnerID,
		CreatedAt: entTeam.CreatedAt,
		UpdatedAt: entTeam.UpdatedAt,
	})
}

// HandleGetTeamSentInvitations returns all pending invitations sent by the team
func (s *Server) HandleGetTeamSentInvitations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	// Get user's team ID
	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active team found", "not_found")
		return
	}

	// Get all pending invitations for this team
	entInvitations, err := s.db.Client.TeamInvitation.Query().
		Where(
			teaminvitation.TeamID(teamID),
			teaminvitation.Status("pending"),
		).
		Order(ent.Desc(teaminvitation.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to get sent invitations", zap.Error(err), zap.Int64("team_id", teamID))
		respondError(w, http.StatusInternalServerError, "failed to fetch invitations", "internal_error")
		return
	}

	invitations := make([]SentInvitation, 0, len(entInvitations))
	for _, entInv := range entInvitations {
		invitations = append(invitations, SentInvitation{
			ID:           entInv.ID,
			InviteeEmail: entInv.InviteeEmail,
			Status:       entInv.Status,
			CreatedAt:    entInv.CreatedAt,
		})
	}

	respondJSON(w, http.StatusOK, invitations)
}

// HandleSearchUsers searches for users not already in the team
func (s *Server) HandleSearchUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) < 2 {
		respondJSON(w, http.StatusOK, []UserSearchResult{})
		return
	}

	// Get user's team ID
	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active team found", "not_found")
		return
	}

	// Check if user is owner or admin
	role, err := s.getUserTeamRole(ctx, userID, teamID)
	if err != nil || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "only team owners and admins can search users", "forbidden")
		return
	}

	// Search users by email or name, excluding current team members
	users, err := s.db.Client.User.Query().
		Where(
			user.Or(
				user.EmailContainsFold(q),
				user.NameContainsFold(q),
				user.FirstNameContainsFold(q),
				user.LastNameContainsFold(q),
			),
			user.Not(user.HasTeamMembershipsWith(
				teammember.TeamID(teamID),
			)),
		).
		Limit(10).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to search users", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to search users", "internal_error")
		return
	}

	results := make([]UserSearchResult, 0, len(users))
	for _, u := range users {
		results = append(results, UserSearchResult{
			ID:    u.ID,
			Email: u.Email,
			Name:  userDisplayNamePtr(u),
		})
	}

	respondJSON(w, http.StatusOK, results)
}

// HandleAddTeamMember directly adds an existing user to the team
func (s *Server) HandleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var req AddTeamMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	if req.UserID <= 0 {
		respondError(w, http.StatusBadRequest, "valid user_id is required", "invalid_input")
		return
	}

	// Get user's team ID
	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "no active team found", "not_found")
		return
	}

	// Check if user is owner or admin
	role, err := s.getUserTeamRole(ctx, userID, teamID)
	if err != nil || (role != "owner" && role != "admin") {
		respondError(w, http.StatusForbidden, "only team owners and admins can add members", "forbidden")
		return
	}

	// Verify the target user exists
	targetUser, err := s.db.Client.User.Get(ctx, req.UserID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "user not found", "not_found")
			return
		}
		s.logger.Error("Failed to get user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get user", "internal_error")
		return
	}

	// Check if user is already a member
	exists, err := s.db.Client.TeamMember.Query().
		Where(
			teammember.TeamID(teamID),
			teammember.UserID(req.UserID),
		).
		Exist(ctx)
	if err != nil {
		s.logger.Error("Failed to check existing member", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to check membership", "internal_error")
		return
	}
	if exists {
		respondError(w, http.StatusConflict, "user is already a team member", "already_member")
		return
	}

	// Begin transaction
	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to add member", "internal_error")
		return
	}
	defer tx.Rollback()

	// Add user as team member
	_, err = tx.TeamMember.Create().
		SetTeamID(teamID).
		SetUserID(req.UserID).
		SetRole("member").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to add team member", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to add team member", "internal_error")
		return
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to add member", "internal_error")
		return
	}

	s.logger.Info("Team member added directly",
		zap.Int64("team_id", teamID),
		zap.Int64("added_user_id", req.UserID),
		zap.String("added_user_email", targetUser.Email),
		zap.Int64("added_by", userID),
	)

	respondJSON(w, http.StatusCreated, map[string]string{"message": "member added"})
}

// Helper functions

func (s *Server) getUserTeamID(ctx context.Context, userID int64) (int64, error) {
	tm, err := s.db.Client.TeamMember.Query().
		Where(
			teammember.UserID(userID),
			teammember.Status("active"),
		).
		First(ctx)
	if err != nil {
		return 0, err
	}
	return tm.TeamID, nil
}

func (s *Server) getUserTeamRole(ctx context.Context, userID, teamID int64) (string, error) {
	tm, err := s.db.Client.TeamMember.Query().
		Where(
			teammember.UserID(userID),
			teammember.TeamID(teamID),
			teammember.Status("active"),
		).
		Only(ctx)
	if err != nil {
		return "", err
	}
	return tm.Role, nil
}

func (s *Server) getInvitation(ctx context.Context, invitationID int64) (*TeamInvitation, error) {
	entInv, err := s.db.Client.TeamInvitation.Query().
		Where(teaminvitation.ID(invitationID)).
		WithTeam().
		WithInviter().
		Only(ctx)
	if err != nil {
		return nil, err
	}

	inv := TeamInvitation{
		ID:           entInv.ID,
		TeamID:       entInv.TeamID,
		InviterID:    entInv.InviterID,
		InviteeEmail: entInv.InviteeEmail,
		InviteeID:    entInv.InviteeID,
		Status:       entInv.Status,
		CreatedAt:    entInv.CreatedAt,
		RespondedAt:  entInv.RespondedAt,
	}

	if entInv.Edges.Team != nil {
		inv.TeamName = entInv.Edges.Team.Name
	}
	if entInv.Edges.Inviter != nil {
		inv.InviterName = userDisplayNamePtr(entInv.Edges.Inviter)
	}

	return &inv, nil
}

// generateTeamInviteCode creates a random invite code (delegates to the shared generator)
func generateTeamInviteCode() (string, error) {
	return generateInviteCode()
}

// TokenInvitationResponse is returned by the token lookup endpoint
type TokenInvitationResponse struct {
	InvitationID int64  `json:"invitation_id"`
	TeamName     string `json:"team_name"`
	InviterName  string `json:"inviter_name"`
	InviteeEmail string `json:"invitee_email"`
	Status       string `json:"status"`
	RequiresSignup bool `json:"requires_signup"`
	InviteCode   string `json:"invite_code,omitempty"`
}

// HandleGetInvitationByToken returns invitation info for a given acceptance token (public, no auth required)
func (s *Server) HandleGetInvitationByToken(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	token := r.URL.Query().Get("token")
	if token == "" {
		respondError(w, http.StatusBadRequest, "token is required", "invalid_input")
		return
	}

	// Get invitation by token with team and inviter edges
	entInv, err := s.db.Client.TeamInvitation.Query().
		Where(teaminvitation.AcceptanceToken(token)).
		WithTeam().
		WithInviter().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "invitation not found or token is invalid", "not_found")
			return
		}
		s.logger.Error("Failed to get invitation by token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch invitation", "internal_error")
		return
	}

	// Check token expiry
	if entInv.TokenExpiresAt != nil && time.Now().After(*entInv.TokenExpiresAt) {
		respondError(w, http.StatusGone, "invitation link has expired", "token_expired")
		return
	}

	// Check if invitation is still pending
	if entInv.Status != "pending" {
		respondError(w, http.StatusConflict, "invitation has already been "+entInv.Status, "already_responded")
		return
	}

	// Build response
	resp := TokenInvitationResponse{
		InvitationID: entInv.ID,
		InviteeEmail: entInv.InviteeEmail,
		Status:       entInv.Status,
		RequiresSignup: entInv.InviteeID == nil,
	}

	if entInv.Edges.Team != nil {
		resp.TeamName = entInv.Edges.Team.Name
	}

	if entInv.Edges.Inviter != nil {
		resp.InviterName = userDisplayName(entInv.Edges.Inviter)
	}

	if resp.RequiresSignup && entInv.InviteCode != nil {
		resp.InviteCode = *entInv.InviteCode
	}

	respondJSON(w, http.StatusOK, resp)
}

// HandleAcceptInvitationByToken accepts a team invitation using the acceptance token (requires auth)
func (s *Server) HandleAcceptInvitationByToken(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	email := r.Context().Value(UserEmailKey).(string)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		respondError(w, http.StatusBadRequest, "token is required", "invalid_input")
		return
	}

	// Find invitation by token
	entInv, err := s.db.Client.TeamInvitation.Query().
		Where(teaminvitation.AcceptanceToken(req.Token)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "invitation not found or token is invalid", "not_found")
			return
		}
		s.logger.Error("Failed to get invitation by token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch invitation", "internal_error")
		return
	}

	// Check token expiry
	if entInv.TokenExpiresAt != nil && time.Now().After(*entInv.TokenExpiresAt) {
		respondError(w, http.StatusGone, "invitation link has expired", "token_expired")
		return
	}

	// Verify invitation is for this user
	if entInv.InviteeEmail != email {
		respondError(w, http.StatusForbidden, "this invitation is for a different email address", "forbidden")
		return
	}

	// Check if invitation is still pending
	if entInv.Status != "pending" {
		respondError(w, http.StatusConflict, "invitation has already been "+entInv.Status, "already_responded")
		return
	}

	// Begin Ent transaction
	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to process invitation", "internal_error")
		return
	}
	defer tx.Rollback()

	// Update invitation status
	now := time.Now()
	_, err = tx.TeamInvitation.UpdateOneID(entInv.ID).
		SetStatus("accepted").
		SetInviteeID(userID).
		SetRespondedAt(now).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to update invitation", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update invitation", "internal_error")
		return
	}

	// Add user to team
	_, err = tx.TeamMember.Create().
		SetTeamID(entInv.TeamID).
		SetUserID(userID).
		SetRole("member").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to add team member", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to add team member", "internal_error")
		return
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to process invitation", "internal_error")
		return
	}

	s.logger.Info("Invitation accepted via token",
		zap.Int64("invitation_id", entInv.ID),
		zap.Int64("user_id", userID),
		zap.Int64("team_id", entInv.TeamID),
	)

	respondJSON(w, http.StatusOK, map[string]string{"message": "invitation accepted"})
}

func isValidEmail(email string) bool {
	// Basic email validation
	if len(email) < 3 || len(email) > 254 {
		return false
	}

	atIndex := -1
	for i, c := range email {
		if c == '@' {
			if atIndex >= 0 {
				return false // Multiple @ symbols
			}
			atIndex = i
		}
	}

	if atIndex <= 0 || atIndex >= len(email)-1 {
		return false
	}

	return true
}
