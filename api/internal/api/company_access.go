package api

import (
	"context"

	"go.uber.org/zap"
)

// A company (team) owns its people. Every active company member gets a
// project_members row on each of the company's shared projects, so all the
// existing per-project access checks keep working unchanged. Rows added by
// hand stay the per-project exceptions: guests from outside, or roles that
// differ from the company role.

// syncCompanyAccess grants every active member of teamID access to the team's
// shared projects. It is idempotent and never changes an existing row.
func (s *Server) syncCompanyAccess(ctx context.Context, teamID int64) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO project_members (project_id, user_id, role, granted_by)
		SELECT p.id, tm.user_id,
			CASE tm.role WHEN 'owner' THEN 'owner' WHEN 'admin' THEN 'editor' ELSE 'member' END,
			p.owner_id
		FROM projects p
		JOIN team_members tm ON tm.team_id = p.team_id AND tm.status = 'active'
		WHERE p.team_id = $1 AND p.shared_with_team
		ON CONFLICT (project_id, user_id) DO NOTHING`, teamID)
	if err != nil {
		// The membership change itself already succeeded; the next sync for this team retries.
		s.logger.Error("Failed to sync company project access", zap.Error(err), zap.Int64("team_id", teamID))
	}
}

// revokeCompanyAccess removes a departing member from the team's shared
// projects, except the ones they own.
func (s *Server) revokeCompanyAccess(ctx context.Context, teamID, userID int64) {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM project_members
		WHERE user_id = $1 AND project_id IN (
			SELECT id FROM projects WHERE team_id = $2 AND shared_with_team AND owner_id <> $1
		)`, userID, teamID)
	if err != nil {
		s.logger.Error("Failed to revoke company project access", zap.Error(err),
			zap.Int64("team_id", teamID), zap.Int64("user_id", userID))
	}
}
