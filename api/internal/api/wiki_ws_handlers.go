package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/wikipage"
	"taskai/ent/yjsupdate"
	"taskai/internal/collab"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking based on CORS_ALLOWED_ORIGINS
		return true
	},
}

// YjsUpdatePayload represents a Yjs update message payload
type YjsUpdatePayload struct {
	Update string `json:"update"` // Base64-encoded Yjs update
}

// SyncRequestPayload represents a sync request
type SyncRequestPayload struct {
	PageID int64 `json:"page_id"`
}

// SyncResponsePayload represents a sync response with all updates
type SyncResponsePayload struct {
	Updates []string `json:"updates"` // Array of base64-encoded Yjs updates
}

// ErrorPayload represents an error message
type ErrorPayload struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// HandleWikiWebSocket handles WebSocket connections for wiki collaboration
func (s *Server) HandleWikiWebSocket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from JWT auth (should be set by middleware)
	userID, ok := ctx.Value(UserIDKey).(int64)
	if !ok {
		// Try to get from query parameter for WebSocket (some clients can't set headers)
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate JWT token from query param
		claims, err := s.auth.ValidateToken(token)
		if err != nil {
			s.logger.Warn("Invalid WebSocket token",
				zap.Error(err),
			)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID = claims.UserID
	}

	// Get page ID from query parameter
	pageIDStr := r.URL.Query().Get("page_id")
	if pageIDStr == "" {
		http.Error(w, "page_id parameter required", http.StatusBadRequest)
		return
	}

	pageID, err := strconv.ParseInt(pageIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid page_id", http.StatusBadRequest)
		return
	}

	// Verify page exists and user has access
	page, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ID(pageID)).
		WithProject().
		Only(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch wiki page for WebSocket",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}

	// Check if user has access to the project
	hasAccess, err := s.canViewWikiPage(ctx, userID, page)
	if err != nil {
		s.logger.Error("Failed to check project access",
			zap.Int64("user_id", userID),
			zap.Int64("project_id", page.ProjectID),
			zap.Error(err),
		)
		http.Error(w, "failed to verify access", http.StatusInternalServerError)
		return
	}
	if !hasAccess {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade WebSocket",
			zap.Error(err),
		)
		return
	}

	// Create client
	client := &collab.Client{
		ID:      uuid.New().String(),
		UserID:  userID,
		PageID:  pageID,
		Conn:    conn,
		Send:    make(chan []byte, 256),
	}

	// Store reference to manager and room ID in client
	// We need to modify the collab.Client struct to store these
	roomID := fmt.Sprintf("page:%d", pageID)

	// Register client with the collaboration manager
	s.registerWebSocketClient(client, roomID)

	s.logger.Info("WebSocket connection established",
		zap.String("client_id", client.ID),
		zap.Int64("user_id", userID),
		zap.Int64("page_id", pageID),
	)
}

// registerWebSocketClient registers a client and sets up message handling
func (s *Server) registerWebSocketClient(client *collab.Client, roomID string) {
	// Set up custom message handler
	originalHandleMessage := client.HandleMessage
	client.HandleMessage = func(message []byte) {
		s.handleWikiMessage(client, message, roomID)
		if originalHandleMessage != nil {
			originalHandleMessage(message)
		}
	}

	// Register with the manager (this will start read/write pumps)
	s.collabManager.RegisterClient(client, roomID)
}

// handleWikiMessage processes incoming WebSocket messages for wiki collaboration
func (s *Server) handleWikiMessage(client *collab.Client, message []byte, roomID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Parse message
	var msg collab.Message
	if err := json.Unmarshal(message, &msg); err != nil {
		s.logger.Error("Failed to parse WebSocket message",
			zap.String("client_id", client.ID),
			zap.Error(err),
		)
		s.sendError(client, "Invalid message format", "invalid_message")
		return
	}

	s.logger.Debug("Received WebSocket message",
		zap.String("client_id", client.ID),
		zap.String("type", msg.Type),
	)

	switch msg.Type {
	case "sync_request":
		s.handleSyncRequest(ctx, client, msg.Payload)

	case "update":
		s.handleYjsUpdate(ctx, client, roomID, msg.Payload)

	case "awareness":
		// Just broadcast awareness updates (cursor positions, selections)
		// These are ephemeral and not persisted
		s.collabManager.Broadcast(roomID, message, client)

	default:
		s.logger.Warn("Unknown message type",
			zap.String("client_id", client.ID),
			zap.String("type", msg.Type),
		)
		s.sendError(client, fmt.Sprintf("Unknown message type: %s", msg.Type), "unknown_type")
	}
}

// handleSyncRequest sends all Yjs updates for a page to a client
func (s *Server) handleSyncRequest(ctx context.Context, client *collab.Client, payload json.RawMessage) {
	var req SyncRequestPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		s.sendError(client, "Invalid sync request payload", "invalid_payload")
		return
	}

	// Fetch all Yjs updates for the page from database
	yjsUpdates, err := s.db.Client.YjsUpdate.Query().
		Where(yjsupdate.PageID(client.PageID)).
		Order(ent.Asc(yjsupdate.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch Yjs updates",
			zap.Int64("page_id", client.PageID),
			zap.Error(err),
		)
		s.sendError(client, "Failed to fetch document history", "internal_error")
		return
	}

	// Convert binary updates to base64 strings
	updates := make([]string, len(yjsUpdates))
	for i, update := range yjsUpdates {
		updates[i] = base64.StdEncoding.EncodeToString(update.UpdateData)
	}

	// Send sync response
	response := SyncResponsePayload{
		Updates: updates,
	}
	responseBytes, _ := json.Marshal(collab.Message{
		Type:    "sync_response",
		Payload: json.RawMessage(mustMarshal(response)),
	})

	select {
	case client.Send <- responseBytes:
	default:
		s.logger.Warn("Failed to send sync response, buffer full",
			zap.String("client_id", client.ID),
		)
	}

	s.logger.Info("Sent sync response",
		zap.String("client_id", client.ID),
		zap.Int("update_count", len(updates)),
	)
}

// handleYjsUpdate processes and persists a Yjs update
func (s *Server) handleYjsUpdate(ctx context.Context, client *collab.Client, roomID string, payload json.RawMessage) {
	var updatePayload YjsUpdatePayload
	if err := json.Unmarshal(payload, &updatePayload); err != nil {
		s.sendError(client, "Invalid update payload", "invalid_payload")
		return
	}

	// Decode base64 update data
	updateData, err := base64.StdEncoding.DecodeString(updatePayload.Update)
	if err != nil {
		s.sendError(client, "Invalid update encoding", "invalid_encoding")
		return
	}

	// Persist the update to database
	_, err = s.db.Client.YjsUpdate.Create().
		SetPageID(client.PageID).
		SetUpdateData(updateData).
		SetCreatedBy(client.UserID).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to persist Yjs update",
			zap.Int64("page_id", client.PageID),
			zap.Error(err),
		)
		s.sendError(client, "Failed to save update", "internal_error")
		return
	}

	// Broadcast the update to other clients in the room
	s.collabManager.Broadcast(roomID, mustMarshal(collab.Message{
		Type:    "update",
		Payload: payload,
	}), client)

	s.logger.Debug("Processed Yjs update",
		zap.String("client_id", client.ID),
		zap.Int64("page_id", client.PageID),
		zap.Int("update_size", len(updateData)),
	)
}

// sendError sends an error message to a client
func (s *Server) sendError(client *collab.Client, message, code string) {
	errorMsg := collab.Message{
		Type: "error",
		Payload: json.RawMessage(mustMarshal(ErrorPayload{
			Message: message,
			Code:    code,
		})),
	}

	select {
	case client.Send <- mustMarshal(errorMsg):
	default:
		s.logger.Warn("Failed to send error message, buffer full",
			zap.String("client_id", client.ID),
		)
	}
}

// mustMarshal marshals v to JSON or panics
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return data
}
