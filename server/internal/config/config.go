package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the server.
type Config struct {
	// Server
	ServerPort string
	GinMode    string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Redis
	RedisAddr     string
	RedisPassword string

	// JWT
	JWTSecret        string
	JWTRefreshSecret string

	// APNs
	APNsKeyID    string
	APNsTeamID   string
	APNsBundleID string
	APNsKeyPath  string

	// CDN
	CDNBaseURL   string
	CDNUploadKey string
}

// Load reads configuration from environment variables.
// It attempts to load .env.development first (for local dev),
// then falls back to actual environment variables.
func Load() *Config {
	// Try loading .env.development, ignore error if not found
	_ = godotenv.Load(".env.development")

	return &Config{
		ServerPort: getEnv("SERVER_PORT", "8080"),
		GinMode:    getEnv("GIN_MODE", "debug"),

		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "cat"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "cat_dev"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		RedisAddr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		JWTSecret:        getEnv("JWT_SECRET", ""),
		JWTRefreshSecret: getEnv("JWT_REFRESH_SECRET", ""),

		APNsKeyID:    getEnv("APNS_KEY_ID", ""),
		APNsTeamID:   getEnv("APNS_TEAM_ID", ""),
		APNsBundleID: getEnv("APNS_BUNDLE_ID", ""),
		APNsKeyPath:  getEnv("APNS_KEY_PATH", ""),

		CDNBaseURL:   getEnv("CDN_BASE_URL", ""),
		CDNUploadKey: getEnv("CDN_UPLOAD_KEY", ""),
	}
}

// DSN returns the PostgreSQL connection string.
func (c *Config) DSN() string {
	return "host=" + c.DBHost +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" port=" + c.DBPort +
		" sslmode=" + c.DBSSLMode +
		" TimeZone=UTC"
}

// MigrationDSN returns the DSN formatted for golang-migrate.
func (c *Config) MigrationDSN() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword +
		"@" + c.DBHost + ":" + c.DBPort +
		"/" + c.DBName + "?sslmode=" + c.DBSSLMode
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
