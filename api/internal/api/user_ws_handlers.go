package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"taskai/internal/collab"
)

// HandleUserWebSocket handles per-user WebSocket connections for real-time events.
// Auth is via ?token= query param since browsers cannot set custom headers during WS upgrade.
func (s *Server) HandleUserWebSocket(w http.ResponseWriter, r *http.Request) {
	// Try context first (set by JWTAuth middleware), fall back to ?token= query param
	userID, ok := GetUserID(r)
	if !ok {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := s.auth.ValidateToken(token)
		if err != nil {
			s.logger.Warn("Invalid user WS token", zap.Error(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID = claims.UserID
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade user WebSocket",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return
	}

	client := &collab.Client{
		ID:     uuid.New().String(),
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
	}

	roomID := fmt.Sprintf("user:%d", userID)
	s.collabManager.RegisterClient(client, roomID)

	s.logger.Info("User notification WS connected",
		zap.Int64("user_id", userID),
		zap.String("room_id", roomID),
	)
}

// broadcastToProjectMembers sends a real-time event to all members of a project.
func (s *Server) broadcastToProjectMembers(projectID int64, eventType string, payload interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id FROM project_members WHERE project_id = $1`, projectID,
	)
	if err != nil {
		s.logger.Warn("broadcastToProjectMembers: query failed",
			zap.Int64("project_id", projectID),
			zap.Error(err),
		)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			continue
		}
		s.BroadcastToUser(uid, eventType, payload)
	}
}

// BroadcastToUser sends a real-time event to a user's WebSocket room (if connected).
func (s *Server) BroadcastToUser(userID int64, eventType string, payload interface{}) {
	if s.collabManager == nil {
		return
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		s.logger.Warn("BroadcastToUser: failed to marshal payload", zap.Error(err))
		return
	}
	msg := collab.Message{
		Type:    eventType,
		Payload: json.RawMessage(payloadBytes),
	}
	roomID := fmt.Sprintf("user:%d", userID)
	s.collabManager.Broadcast(roomID, mustMarshal(msg), nil)
}
