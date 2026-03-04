package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"taskai/ent"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// DB wraps the database connection and Ent client
type DB struct {
	*sql.DB        // Raw SQL connection for migrations
	Client *ent.Client  // Ent ORM client for queries
	Driver string      // "sqlite" or "postgres"
	logger *zap.Logger
}

// Rebind converts SQLite-style ? placeholders to Postgres-style $1, $2, ...
// when the driver is postgres. A no-op for SQLite.
func (db *DB) Rebind(query string) string {
	if db.Driver != "postgres" {
		return query
	}
	var out []byte
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out = append(out, fmt.Sprintf("$%d", n)...)
			n++
		} else {
			out = append(out, query[i])
		}
	}
	return string(out)
}

// Config holds database configuration
type Config struct {
	Driver         string // "sqlite" or "pgx" (postgres)
	DBPath         string // For SQLite
	DSN            string // For Postgres
	MigrationsPath string
	EnableSQLLog   bool   // Enable Ent ORM query logging (expensive in production)
}

// New creates a new database connection and runs migrations
func New(cfg Config, logger *zap.Logger) (*DB, error) {
	var sqlDB *sql.DB
	var err error

	// Determine driver and connection string
	driver := cfg.Driver
	if driver == "" {
		driver = "sqlite" // Default to SQLite for backward compatibility
	}

	switch driver {
	case "sqlite":
		// Ensure data directory exists for SQLite
		dataDir := filepath.Dir(cfg.DBPath)
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}

		// Open SQLite connection
		sqlDB, err = sql.Open("sqlite", cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open SQLite database: %w", err)
		}

		// Configure connection pool for SQLite
		sqlDB.SetMaxOpenConns(1) // SQLite supports only one writer
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(time.Hour)

		// Enable foreign keys and WAL mode for better concurrency
		pragmas := []string{
			"PRAGMA foreign_keys = ON",
			"PRAGMA journal_mode = WAL",
			"PRAGMA synchronous = NORMAL",
			"PRAGMA busy_timeout = 5000",
			"PRAGMA cache_size = -64000",       // 64MB page cache (default ~2MB)
			"PRAGMA mmap_size = 268435456",     // 256MB memory-mapped I/O
			"PRAGMA temp_store = MEMORY",       // Keep temp tables in RAM
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for _, pragma := range pragmas {
			if _, err := sqlDB.ExecContext(ctx, pragma); err != nil {
				sqlDB.Close()
				return nil, fmt.Errorf("failed to execute %s: %w", pragma, err)
			}
		}

	case "postgres", "pgx":
		// Open Postgres connection
		sqlDB, err = sql.Open("pgx", cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("failed to open Postgres database: %w", err)
		}

		// Configure connection pool for Postgres
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
		sqlDB.SetConnMaxIdleTime(time.Minute)

	default:
		return nil, fmt.Errorf("unsupported database driver: %s (expected 'sqlite' or 'postgres')", driver)
	}

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create Ent client
	var entDialect string
	if driver == "postgres" || driver == "pgx" {
		entDialect = dialect.Postgres
	} else {
		entDialect = dialect.SQLite
	}

	entDriver := entsql.OpenDB(entDialect, sqlDB)
	entClient := ent.NewClient(ent.Driver(entDriver))

	// Enable Ent query logging only when explicitly requested (expensive in production)
	if cfg.EnableSQLLog {
		entClient = entClient.Debug()
	}

	db := &DB{
		DB:     sqlDB,
		Client: entClient,
		Driver: driver,
		logger: logger,
	}

	// Run migrations (still use SQL migrations, not Ent auto-migration)
	if cfg.MigrationsPath != "" {
		if err := db.runMigrations(ctx, cfg.MigrationsPath, driver); err != nil {
			sqlDB.Close()
			entClient.Close()
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	logger.Info("Database initialized",
		zap.String("driver", driver),
		zap.String("dialect", entDialect),
		zap.String("path", cfg.DBPath),
		zap.String("dsn_host", maskDSN(cfg.DSN)))
	return db, nil
}

// maskDSN returns a masked version of the DSN for logging (hides password)
func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	// Simple masking: show only the host part
	if strings.Contains(dsn, "@") {
		parts := strings.Split(dsn, "@")
		if len(parts) > 1 {
			return "***@" + parts[1]
		}
	}
	return "***"
}

// runMigrations executes all pending SQL migration files
func (db *DB) runMigrations(ctx context.Context, migrationsPath string, driver string) error {
	// Use postgres subdirectory for Postgres migrations
	if driver == "postgres" || driver == "pgx" {
		migrationsPath = filepath.Join(migrationsPath, "postgres")
	}

	// Create migrations table if it doesn't exist
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	if _, err := db.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	appliedMigrations := make(map[string]bool)
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("failed to scan migration version: %w", err)
		}
		appliedMigrations[version] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating migrations: %w", err)
	}

	// Check if migrations directory exists
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		db.logger.Warn("Migrations directory does not exist, skipping migrations",
			zap.String("path", migrationsPath))
		return nil
	}

	// Read migration files
	files, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Filter and sort .sql files
	var migrationFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".sql") {
			migrationFiles = append(migrationFiles, file.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Apply pending migrations
	for _, filename := range migrationFiles {
		version := strings.TrimSuffix(filename, ".sql")

		if appliedMigrations[version] {
			continue
		}

		db.logger.Info("Applying migration", zap.String("file", filename))

		// Read migration file
		content, err := os.ReadFile(filepath.Join(migrationsPath, filename))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", filename, err)
		}

		// Execute migration in a transaction
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for %s: %w", filename, err)
		}

		// Execute migration SQL
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", filename, err)
		}

		// Record migration (use correct placeholder syntax based on driver)
		placeholder := "?"
		if driver == "postgres" || driver == "pgx" {
			placeholder = "$1"
		}
		recordSQL := fmt.Sprintf("INSERT INTO schema_migrations (version) VALUES (%s)", placeholder)
		_, err = tx.ExecContext(ctx, recordSQL, version)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", filename, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", filename, err)
		}

		db.logger.Info("Migration applied successfully", zap.String("file", filename))
	}

	return nil
}

// Close closes the database connection and Ent client
func (db *DB) Close() error {
	if db.Client != nil {
		db.Client.Close()
	}
	return db.DB.Close()
}

// HealthCheck verifies database connectivity
func (db *DB) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return db.PingContext(ctx)
}

// IsUserDeleted returns true when the user does not exist or has been soft-deleted.
// Used by JWTAuth middleware to invalidate tokens for deleted accounts.
func (db *DB) IsUserDeleted(ctx context.Context, userID int64) (bool, error) {
	var deletedAt *time.Time
	err := db.QueryRowContext(ctx,
		db.Rebind(`SELECT deleted_at FROM users WHERE id = ? LIMIT 1`),
		userID,
	).Scan(&deletedAt)
	if err == sql.ErrNoRows {
		return false, nil // user doesn't exist — let the handler return 404
	}
	if err != nil {
		return false, err
	}
	return deletedAt != nil, nil
}

// GetMigrationVersion returns the count of applied migrations
func (db *DB) GetMigrationVersion(ctx context.Context) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get migration version: %w", err)
	}
	return count, nil
}
