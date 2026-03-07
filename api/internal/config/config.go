package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration
type Config struct {
	// Server
	Port string
	Env  string

	// Database
	DBDriver       string // "sqlite" or "postgres"
	DBPath         string // For SQLite
	DBDSN          string // For Postgres
	MigrationsPath string

	// JWT
	JWTSecret      string
	JWTExpiryHours int

	// CORS
	CORSAllowedOrigins []string

	// Rate Limiting
	RateLimitRequests       int
	RateLimitWindowMinutes  int

	// Logging
	LogLevel       string
	EnableSQLLog   bool

	// Profiling
	EnablePprof    bool

	// Database
	DBQueryTimeout time.Duration

	// Yjs Processor
	YJSProcessorURL string

	// GitHub OAuth (repo integration)
	GitHubClientID     string
	GitHubClientSecret string
	AppURL             string

	// OAuth Login (Google + GitHub login, separate from repo integration)
	GoogleClientID          string
	GoogleClientSecret      string
	LoginGitHubClientID     string
	LoginGitHubClientSecret string
	OAuthStateSecret        string
	OAuthSuccessURL         string
	OAuthErrorURL           string

	// Backup (Google Drive) — reuses GOOGLE_CLIENT_ID/SECRET from OAuth login
	BackupEncryptionKey string // 64-char hex-encoded 32-byte AES key
}

// Load reads configuration from environment variables
func Load() *Config {
	dbDriver := getEnv("DB_DRIVER", "postgres")

	cfg := &Config{
		Port:                    getEnv("PORT", "8080"),
		Env:                     getEnv("ENV", "development"),
		DBDriver:                dbDriver,
		DBPath:                  getEnv("DB_PATH", "./data/taskai.db"),
		DBDSN:                   getEnv("DB_DSN", ""),
		MigrationsPath:          getEnv("MIGRATIONS_PATH", "./internal/db/migrations"),
		JWTSecret:               getEnv("JWT_SECRET", "change-this-to-a-secure-random-string-in-production"),
		JWTExpiryHours:          getEnvAsInt("JWT_EXPIRY_HOURS", 24),
		CORSAllowedOrigins:      getEnvAsSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://localhost:3000"}),
		RateLimitRequests:       getEnvAsInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitWindowMinutes:  getEnvAsInt("RATE_LIMIT_WINDOW_MINUTES", 15),
		LogLevel:                getEnv("LOG_LEVEL", "info"),
		EnableSQLLog:            getEnv("ENV", "development") == "development" || getEnv("ENABLE_SQL_LOG", "false") == "true",
		EnablePprof:             getEnv("ENABLE_PPROF", "false") == "true",
		DBQueryTimeout:          time.Duration(getEnvAsInt("DB_QUERY_TIMEOUT_SECONDS", 5)) * time.Second,
		YJSProcessorURL:         getEnv("YJS_PROCESSOR_URL", "http://localhost:3001"),
		GitHubClientID:          getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret:      getEnv("GITHUB_CLIENT_SECRET", ""),
		AppURL:                  getEnv("APP_URL", "http://localhost:5173"),
		GoogleClientID:          getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:      getEnv("GOOGLE_CLIENT_SECRET", ""),
		LoginGitHubClientID:     getEnv("LOGIN_GITHUB_CLIENT_ID", ""),
		LoginGitHubClientSecret: getEnv("LOGIN_GITHUB_CLIENT_SECRET", ""),
		OAuthStateSecret:        getEnv("OAUTH_STATE_SECRET", ""),
		OAuthSuccessURL:         getEnv("OAUTH_SUCCESS_URL", "http://localhost:5173/oauth/callback"),
		OAuthErrorURL:           getEnv("OAUTH_ERROR_URL", "http://localhost:5173/login"),

		BackupEncryptionKey: getEnv("BACKUP_ENCRYPTION_KEY", ""),
	}

	// Validate critical configuration
	if cfg.Env == "production" && cfg.JWTSecret == "change-this-to-a-secure-random-string-in-production" {
		logger := MustInitLogger(cfg.Env, cfg.LogLevel)
		logger.Fatal("JWT_SECRET must be set in production environment")
	}

	return cfg
}

// JWTExpiry returns the JWT expiry duration
func (c *Config) JWTExpiry() time.Duration {
	return time.Duration(c.JWTExpiryHours) * time.Hour
}

// getEnv reads an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt reads an environment variable as int or returns a default value
func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		// Silently use default - logger not available yet during config load
		return defaultValue
	}
	return value
}

// getEnvAsSlice reads an environment variable as comma-separated values
func getEnvAsSlice(key string, defaultValue []string) []string {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	var result []string
	for _, v := range strings.Split(valueStr, ",") {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return defaultValue
	}
	return result
}