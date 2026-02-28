package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"

	"go.uber.org/zap"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	sqlitePath := flag.String("sqlite", "./data/taskai.db", "Path to SQLite database")
	postgresDSN := flag.String("postgres", "", "Postgres DSN (required)")
	flag.Parse()

	if *postgresDSN == "" {
		logger.Fatal("Postgres DSN is required. Example: postgres://user:pass@localhost/dbname")
	}

	logger.Info("Starting migration from SQLite to Postgres",
		zap.String("sqlite", *sqlitePath),
		zap.String("postgres", maskPassword(*postgresDSN)),
	)

	// Open SQLite
	sqliteDB, err := sql.Open("sqlite", *sqlitePath)
	if err != nil {
		logger.Fatal("Failed to open SQLite", zap.Error(err))
	}
	defer sqliteDB.Close()

	// Open Postgres
	postgresDB, err := sql.Open("pgx", *postgresDSN)
	if err != nil {
		logger.Fatal("Failed to open Postgres", zap.Error(err))
	}
	defer postgresDB.Close()

	// Test connections
	if err := sqliteDB.Ping(); err != nil {
		logger.Fatal("Failed to ping SQLite", zap.Error(err))
	}
	if err := postgresDB.Ping(); err != nil {
		logger.Fatal("Failed to ping Postgres", zap.Error(err))
	}

	logger.Info("Database connections established")

	// Get list of tables from SQLite
	tables, err := getTables(sqliteDB)
	if err != nil {
		logger.Fatal("Failed to get tables", zap.Error(err))
	}

	logger.Info("Found tables to migrate", zap.Int("count", len(tables)))

	ctx := context.Background()

	// Define table migration order (respecting foreign key dependencies)
	tableOrder := []string{
		"users",           // No dependencies
		"projects",        // Depends on users (owner_id)
		"teams",          // Depends on users (owner_id)
		"team_members",   // Depends on teams, users
		"team_invitations", // Depends on teams
		"project_members", // Depends on projects, users
		"project_invitations", // Depends on projects, users
		"swim_lanes",     // Depends on projects
		"tasks",          // Depends on projects, users
		"sprints",        // Depends on projects
		"tags",           // Depends on projects
		"task_tags",      // Depends on tasks, tags
		"task_comments",  // Depends on tasks, users
		"task_attachments", // Depends on tasks, users
		"user_activity",  // Depends on users
		"api_keys",       // Depends on users
		"cloudinary_credentials", // Depends on users
		"invites",        // Depends on users (inviter_id)
		"email_provider", // No dependencies
		"wiki_pages",     // Depends on projects, users
		"wiki_page_attachments", // Depends on wiki_pages, projects, users
		"yjs_updates",    // Depends on wiki_pages
		"page_versions",  // Depends on wiki_pages
		"wiki_blocks",    // Depends on wiki_pages
	}

	// Migrate tables in order
	for _, table := range tableOrder {
		// Check if table exists
		found := false
		for _, t := range tables {
			if t == table {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		logger.Info("Migrating table", zap.String("table", table))
		count, err := migrateTable(ctx, sqliteDB, postgresDB, table)
		if err != nil {
			logger.Error("Failed to migrate table", zap.String("table", table), zap.Error(err))
			continue
		}
		logger.Info("Migrated table", zap.String("table", table), zap.Int("rows", count))
	}

	// Migrate any remaining tables not in the order list
	for _, table := range tables {
		if table == "schema_migrations" {
			continue
		}

		// Check if already migrated
		alreadyMigrated := false
		for _, ordered := range tableOrder {
			if table == ordered {
				alreadyMigrated = true
				break
			}
		}
		if alreadyMigrated {
			continue
		}

		logger.Info("Migrating table", zap.String("table", table))
		count, err := migrateTable(ctx, sqliteDB, postgresDB, table)
		if err != nil {
			logger.Error("Failed to migrate table", zap.String("table", table), zap.Error(err))
			continue
		}
		logger.Info("Migrated table", zap.String("table", table), zap.Int("rows", count))
	}

	// Migrate schema_migrations last
	logger.Info("Migrating schema_migrations")
	count, err := migrateTable(ctx, sqliteDB, postgresDB, "schema_migrations")
	if err != nil {
		logger.Error("Failed to migrate schema_migrations", zap.Error(err))
	} else {
		logger.Info("Migrated schema_migrations", zap.Int("rows", count))
	}

	// Reset all Postgres sequences to match migrated data
	logger.Info("Resetting Postgres sequences to match migrated data...")
	if err := resetSequences(ctx, postgresDB, logger); err != nil {
		logger.Error("Failed to reset sequences", zap.Error(err))
	} else {
		logger.Info("All sequences reset successfully")
	}

	logger.Info("Migration completed successfully")
}

func getTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func migrateTable(ctx context.Context, from, to *sql.DB, table string) (int, error) {
	// Get all rows from SQLite
	rows, err := from.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", table))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	// Prepare insert statement
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		table,
		joinStrings(columns, ", "),
		joinStrings(placeholders, ", "),
	)

	stmt, err := to.PrepareContext(ctx, insertSQL)
	if err != nil {
		return 0, fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		// Create slice for values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return count, fmt.Errorf("scan row: %w", err)
		}

		// Convert values for Postgres compatibility
		values = convertValues(table, columns, values)

		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return count, fmt.Errorf("insert row: %w", err)
		}
		count++
	}

	return count, rows.Err()
}

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

func convertValues(table string, columns []string, values []interface{}) []interface{} {
	// Define boolean columns per table (SQLite stores as INTEGER 0/1)
	booleanColumns := map[string][]string{
		"users":       {"is_admin", "totp_enabled"},
		"projects":    {"is_public", "github_sync_enabled"},
		"invites":     {"used"},
	}

	result := make([]interface{}, len(values))
	for i, val := range values {
		// Convert []byte to string for text fields
		if b, ok := val.([]byte); ok {
			result[i] = string(b)
			continue
		}

		// Convert INTEGER (0/1) to BOOLEAN for known boolean columns
		if boolCols, ok := booleanColumns[table]; ok {
			for _, boolCol := range boolCols {
				if columns[i] == boolCol {
					if intVal, ok := val.(int64); ok {
						result[i] = intVal != 0
						goto next
					}
				}
			}
		}

		result[i] = val
	next:
	}

	return result
}

// resetSequences fixes all Postgres SERIAL/BIGSERIAL sequences to match the actual
// max ID in each table. This is required after migrating data from SQLite because
// Postgres sequences start at 1 regardless of the inserted IDs.
func resetSequences(ctx context.Context, db *sql.DB, logger *zap.Logger) error {
	rows, err := db.QueryContext(ctx, `
		SELECT s.relname AS seq_name, t.relname AS table_name, a.attname AS column_name
		FROM pg_class s
		JOIN pg_depend d ON d.objid = s.oid
		JOIN pg_class t ON t.oid = d.refobjid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
		WHERE s.relkind = 'S' AND d.deptype = 'a'
	`)
	if err != nil {
		return fmt.Errorf("query sequences: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var seqName, tableName, colName string
		if err := rows.Scan(&seqName, &tableName, &colName); err != nil {
			return fmt.Errorf("scan sequence: %w", err)
		}

		query := fmt.Sprintf(
			`SELECT setval('%s', COALESCE((SELECT MAX(%s) FROM %s), 0) + 1, false)`,
			seqName, colName, tableName,
		)
		var newVal int64
		if err := db.QueryRowContext(ctx, query).Scan(&newVal); err != nil {
			logger.Warn("Failed to reset sequence",
				zap.String("sequence", seqName),
				zap.String("table", tableName),
				zap.Error(err),
			)
			continue
		}
		logger.Info("Reset sequence",
			zap.String("sequence", seqName),
			zap.String("table", tableName),
			zap.Int64("next_value", newVal),
		)
	}
	return rows.Err()
}

func maskPassword(dsn string) string {
	// Simple password masking for logs
	start := 0
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == ':' && i+2 < len(dsn) && dsn[i+1] == '/' && dsn[i+2] == '/' {
			start = i + 3
			break
		}
	}

	for i := start; i < len(dsn); i++ {
		if dsn[i] == ':' {
			for j := i + 1; j < len(dsn); j++ {
				if dsn[j] == '@' {
					return dsn[:i+1] + "***" + dsn[j:]
				}
			}
		}
	}
	return dsn
}
