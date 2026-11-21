package api

import (
	"context"

	"go.uber.org/zap"
)

// syncCompanyAccess runs after a membership change has committed; a failure is
// logged and the next sync for the team repairs it.
func (s *Server) syncCompanyAccess(ctx context.Context, teamID int64) {
	if err := s.db.SyncCompanyAccess(ctx, teamID); err != nil {
		s.logger.Error("Failed to sync company project access", zap.Error(err), zap.Int64("team_id", teamID))
	}
}

func (s *Server) revokeCompanyAccess(ctx context.Context, teamID, userID int64) {
	if err := s.db.RevokeCompanyAccess(ctx, teamID, userID); err != nil {
		s.logger.Error("Failed to revoke company project access", zap.Error(err),
			zap.Int64("team_id", teamID), zap.Int64("user_id", userID))
	}
}
