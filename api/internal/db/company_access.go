package db

import (
	"context"
	"fmt"
)

// A company (team) owns its people. Every active company member gets a
// project_members row on each of the company's shared projects, marked
// via_team, so all the existing per-project access checks keep working.
// Rows added by hand (guests, different roles) are never touched.

// SyncCompanyAccess grants every active member of teamID access to the team's
// shared projects. It is idempotent and never changes an existing row.
func (d *DB) SyncCompanyAccess(ctx context.Context, teamID int64) error {
	_, err := d.ExecContext(ctx, `
		INSERT INTO project_members (project_id, user_id, role, granted_by, via_team)
		SELECT p.id, tm.user_id,
			CASE tm.role WHEN 'owner' THEN 'owner' WHEN 'admin' THEN 'editor' ELSE 'member' END,
			p.owner_id, TRUE
		FROM projects p
		JOIN team_members tm ON tm.team_id = p.team_id AND tm.status = 'active'
		WHERE p.team_id = $1 AND p.shared_with_team
		ON CONFLICT (project_id, user_id) DO NOTHING`, teamID)
	if err != nil {
		return fmt.Errorf("sync company access for team %d: %w", teamID, err)
	}
	return nil
}

// RevokeCompanyAccess removes the access a departing member got from the
// team; rows granted by hand stay.
func (d *DB) RevokeCompanyAccess(ctx context.Context, teamID, userID int64) error {
	_, err := d.ExecContext(ctx, `
		DELETE FROM project_members
		WHERE user_id = $1 AND via_team
		  AND project_id IN (SELECT id FROM projects WHERE team_id = $2)`, userID, teamID)
	if err != nil {
		return fmt.Errorf("revoke company access for user %d in team %d: %w", userID, teamID, err)
	}
	return nil
}
