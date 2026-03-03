package api

import (
	"encoding/json"
	"fmt"
	"net/http"

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
