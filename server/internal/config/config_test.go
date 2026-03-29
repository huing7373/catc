package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars to test defaults
	envVars := []string{
		"SERVER_PORT", "GIN_MODE", "DB_HOST", "DB_PORT", "DB_USER",
		"DB_PASSWORD", "DB_NAME", "DB_SSLMODE", "REDIS_ADDR", "REDIS_PASSWORD",
	}
	originals := make(map[string]string)
	for _, key := range envVars {
		originals[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	defer func() {
		for key, val := range originals {
			if val != "" {
				os.Setenv(key, val)
			}
		}
	}()

	cfg := Load()

	assert.Equal(t, "8080", cfg.ServerPort)
	assert.Equal(t, "debug", cfg.GinMode)
	assert.Equal(t, "127.0.0.1", cfg.DBHost)
	assert.Equal(t, "5432", cfg.DBPort)
	assert.Equal(t, "cat", cfg.DBUser)
	assert.Equal(t, "cat_dev", cfg.DBName)
	assert.Equal(t, "disable", cfg.DBSSLMode)
	assert.Equal(t, "127.0.0.1:6379", cfg.RedisAddr)
}

func TestLoad_FromEnv(t *testing.T) {
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("DB_HOST", "postgres.example.com")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("DB_HOST")
	}()

	cfg := Load()

	assert.Equal(t, "9090", cfg.ServerPort)
	assert.Equal(t, "postgres.example.com", cfg.DBHost)
}

func TestDSN(t *testing.T) {
	cfg := &Config{
		DBHost:     "localhost",
		DBUser:     "testuser",
		DBPassword: "testpass",
		DBName:     "testdb",
		DBPort:     "5432",
		DBSSLMode:  "disable",
	}

	dsn := cfg.DSN()
	require.Contains(t, dsn, "host=localhost")
	require.Contains(t, dsn, "user=testuser")
	require.Contains(t, dsn, "password=testpass")
	require.Contains(t, dsn, "dbname=testdb")
	require.Contains(t, dsn, "port=5432")
	require.Contains(t, dsn, "sslmode=disable")
}

func TestMigrationDSN(t *testing.T) {
	cfg := &Config{
		DBUser:     "testuser",
		DBPassword: "testpass",
		DBHost:     "localhost",
		DBPort:     "5432",
		DBName:     "testdb",
		DBSSLMode:  "disable",
	}

	dsn := cfg.MigrationDSN()
	assert.Equal(t, "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable", dsn)
}

func TestGetEnv_EmptyStringFallsBack(t *testing.T) {
	os.Setenv("TEST_EMPTY_VAR", "   ")
	defer os.Unsetenv("TEST_EMPTY_VAR")

	result := getEnv("TEST_EMPTY_VAR", "fallback")
	assert.Equal(t, "fallback", result)
}
