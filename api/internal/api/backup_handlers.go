package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// BackupData represents the complete database backup
type BackupData struct {
	Version       int         `json:"version"`        // Schema migration version
	ExportedAt    time.Time   `json:"exported_at"`
	ExportedBy    int64       `json:"exported_by"`
	Tables        TableData   `json:"tables"`
}

// TableData contains all table data
type TableData struct {
	Users                  []map[string]interface{} `json:"users"`
	Teams                  []map[string]interface{} `json:"teams"`
	TeamMembers            []map[string]interface{} `json:"team_members"`
	Projects               []map[string]interface{} `json:"projects"`
	ProjectMembers         []map[string]interface{} `json:"project_members"`
	ProjectInvitations     []map[string]interface{} `json:"project_invitations"`
	Sprints                []map[string]interface{} `json:"sprints"`
	SwimLanes              []map[string]interface{} `json:"swim_lanes"`
	Tasks                  []map[string]interface{} `json:"tasks"`
	TaskComments           []map[string]interface{} `json:"task_comments"`
	TaskAttachments        []map[string]interface{} `json:"task_attachments"`
	Tags                   []map[string]interface{} `json:"tags"`
	TaskTags               []map[string]interface{} `json:"task_tags"`
	Invites                []map[string]interface{} `json:"invites"`
	TeamInvitations        []map[string]interface{} `json:"team_invitations"`
	UserActivity           []map[string]interface{} `json:"user_activity"`
	APIKeys                []map[string]interface{} `json:"api_keys"`
	CloudinaryCredentials  []map[string]interface{} `json:"cloudinary_credentials"`
	EmailProvider          []map[string]interface{} `json:"email_provider"`
}

// HandleExportData exports all database data (admin only)
func (s *Server) HandleExportData(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}

	// Check if user is admin
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	s.logger.Info("Starting data export", zap.Int64("user_id", userID))

	// Get current migration version
	version, err := s.getCurrentMigrationVersion(ctx)
	if err != nil {
		s.logger.Error("Failed to get migration version", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get schema version", "internal_error")
		return
	}

	backup := BackupData{
		Version:    version,
		ExportedAt: time.Now(),
		ExportedBy: userID,
		Tables:     TableData{},
	}

	// Export all tables
	tables := []struct {
		name   string
		target *[]map[string]interface{}
	}{
		{"users", &backup.Tables.Users},
		{"teams", &backup.Tables.Teams},
		{"team_members", &backup.Tables.TeamMembers},
		{"projects", &backup.Tables.Projects},
		{"project_members", &backup.Tables.ProjectMembers},
		{"project_invitations", &backup.Tables.ProjectInvitations},
		{"sprints", &backup.Tables.Sprints},
		{"swim_lanes", &backup.Tables.SwimLanes},
		{"tasks", &backup.Tables.Tasks},
		{"task_comments", &backup.Tables.TaskComments},
		{"task_attachments", &backup.Tables.TaskAttachments},
		{"tags", &backup.Tables.Tags},
		{"task_tags", &backup.Tables.TaskTags},
		{"invites", &backup.Tables.Invites},
		{"team_invitations", &backup.Tables.TeamInvitations},
		{"user_activity", &backup.Tables.UserActivity},
		{"api_keys", &backup.Tables.APIKeys},
		{"cloudinary_credentials", &backup.Tables.CloudinaryCredentials},
		{"email_provider", &backup.Tables.EmailProvider},
	}

	for _, table := range tables {
		data, err := s.exportTable(ctx, table.name)
		if err != nil {
			s.logger.Error("Failed to export table", zap.String("table", table.name), zap.Error(err))
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to export %s", table.name), "internal_error")
			return
		}
		*table.target = data
		s.logger.Info("Exported table", zap.String("table", table.name), zap.Int("rows", len(data)))
	}

	s.logger.Info("Export completed", zap.Int("version", version), zap.Int64("user_id", userID))

	// Set headers for download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=taskai-backup-%s.json", time.Now().Format("20060102-150405")))

	respondJSON(w, http.StatusOK, backup)
}

// HandleImportData imports database data (admin only)
func (s *Server) HandleImportData(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}

	// Check if user is admin
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	var data BackupData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		respondError(w, http.StatusBadRequest, "invalid backup file", "validation_error")
		return
	}

	s.logger.Info("Starting data import",
		zap.Int64("user_id", userID),
		zap.Int("version", data.Version),
		zap.Time("backup_date", data.ExportedAt))

	rows, err := s.importBackupData(ctx, &data)
	if err != nil {
		s.logger.Error("Import failed", zap.Error(err))
		respondError(w, http.StatusBadRequest, err.Error(), "import_error")
		return
	}

	s.logger.Info("Import completed", zap.Int64("user_id", userID))

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Data imported successfully",
		"version": data.Version,
		"rows":    rows,
	})
}

// importBackupData verifies the migration version and imports all tables.
func (s *Server) importBackupData(ctx context.Context, data *BackupData) (int, error) {
	currentVersion, err := s.getCurrentMigrationVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get schema version: %w", err)
	}
	if data.Version != currentVersion {
		return 0, fmt.Errorf("migration version mismatch: backup is v%d, database is v%d", data.Version, currentVersion)
	}

	tables := []struct {
		name string
		data []map[string]interface{}
	}{
		{"users", data.Tables.Users},
		{"teams", data.Tables.Teams},
		{"team_members", data.Tables.TeamMembers},
		{"projects", data.Tables.Projects},
		{"project_members", data.Tables.ProjectMembers},
		{"project_invitations", data.Tables.ProjectInvitations},
		{"sprints", data.Tables.Sprints},
		{"swim_lanes", data.Tables.SwimLanes},
		{"tasks", data.Tables.Tasks},
		{"task_comments", data.Tables.TaskComments},
		{"task_attachments", data.Tables.TaskAttachments},
		{"tags", data.Tables.Tags},
		{"task_tags", data.Tables.TaskTags},
		{"invites", data.Tables.Invites},
		{"team_invitations", data.Tables.TeamInvitations},
		{"user_activity", data.Tables.UserActivity},
		{"api_keys", data.Tables.APIKeys},
		{"cloudinary_credentials", data.Tables.CloudinaryCredentials},
		{"email_provider", data.Tables.EmailProvider},
	}

	totalRows := 0
	for _, table := range tables {
		if len(table.data) > 0 {
			if err := s.importTable(ctx, table.name, table.data); err != nil {
				return 0, fmt.Errorf("failed to import %s: %w", table.name, err)
			}
			s.logger.Info("Imported table", zap.String("table", table.name), zap.Int("rows", len(table.data)))
			totalRows += len(table.data)
		}
	}
	return totalRows, nil
}

// HandleCopyFromEnv copies database from another environment (non-production only).
// POST /api/admin/backup/copy-from-env
// Body: {"source_url": "https://...", "source_api_key": "..."}
func (s *Server) HandleCopyFromEnv(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}
	if os.Getenv("ENV") == "production" {
		respondError(w, http.StatusForbidden, "copy from environment is not available on production", "forbidden")
		return
	}

	var req struct {
		SourceURL    string `json:"source_url"`
		SourceAPIKey string `json:"source_api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "validation_error")
		return
	}
	if !strings.HasPrefix(req.SourceURL, "https://") {
		respondError(w, http.StatusBadRequest, "source_url must start with https://", "validation_error")
		return
	}
	if req.SourceAPIKey == "" {
		respondError(w, http.StatusBadRequest, "source_api_key is required", "validation_error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Fetch backup export from the source environment
	exportURL := req.SourceURL + "/api/admin/backup/export"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, exportURL, nil)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid source URL", "validation_error")
		return
	}
	httpReq.Header.Set("Authorization", "ApiKey "+req.SourceAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		s.logger.Error("Failed to fetch backup from source environment", zap.Error(err), zap.String("source_url", req.SourceURL))
		respondError(w, http.StatusBadGateway, "failed to reach source environment", "upstream_error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respondError(w, http.StatusBadGateway,
			fmt.Sprintf("source environment returned status %d", resp.StatusCode), "upstream_error")
		return
	}

	var data BackupData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		respondError(w, http.StatusBadGateway, "failed to parse backup from source environment", "upstream_error")
		return
	}

	s.logger.Info("Starting copy from environment",
		zap.Int64("user_id", userID),
		zap.String("source_url", req.SourceURL),
		zap.Int("version", data.Version))

	rows, err := s.importBackupData(ctx, &data)
	if err != nil {
		s.logger.Error("Copy from environment failed", zap.Error(err))
		respondError(w, http.StatusInternalServerError, err.Error(), "import_error")
		return
	}

	s.logger.Info("Copy from environment completed", zap.Int64("user_id", userID), zap.Int("rows", rows))

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Database copied successfully",
		"version": data.Version,
		"rows":    rows,
	})
}

// exportTable exports all data from a table
func (s *Server) exportTable(ctx context.Context, tableName string) ([]map[string]interface{}, error) {
	query := fmt.Sprintf("SELECT * FROM %s", tableName)
	rows, err := s.db.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for easier JSON handling
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

// importTable imports data into a table
func (s *Server) importTable(ctx context.Context, tableName string, data []map[string]interface{}) error {
	if len(data) == 0 {
		return nil
	}

	// Get column names from first row
	var columns []string
	for col := range data[0] {
		columns = append(columns, col)
	}

	// Build INSERT query
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES (%s)",
		tableName,
		joinStrings(columns, ", "),
		joinStrings(placeholders, ", "))

	stmt, err := s.db.DB.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, row := range data {
		values := make([]interface{}, len(columns))
		for i, col := range columns {
			values[i] = row[col]
		}
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return err
		}
	}

	return nil
}

// getCurrentMigrationVersion gets the current migration version from schema_migrations
func (s *Server) getCurrentMigrationVersion(ctx context.Context) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM schema_migrations"
	err := s.db.DB.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// Helper functions
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

func countTotalRows(tables TableData) int {
	return len(tables.Users) +
		len(tables.Teams) +
		len(tables.TeamMembers) +
		len(tables.Projects) +
		len(tables.ProjectMembers) +
		len(tables.ProjectInvitations) +
		len(tables.Sprints) +
		len(tables.SwimLanes) +
		len(tables.Tasks) +
		len(tables.TaskComments) +
		len(tables.TaskAttachments) +
		len(tables.Tags) +
		len(tables.TaskTags) +
		len(tables.Invites) +
		len(tables.TeamInvitations) +
		len(tables.UserActivity) +
		len(tables.APIKeys) +
		len(tables.CloudinaryCredentials) +
		len(tables.EmailProvider)
}
