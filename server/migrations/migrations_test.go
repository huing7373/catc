package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationFilesExist(t *testing.T) {
	// Find the migrations directory
	dir := "."
	if _, err := os.Stat("000001_create_users.up.sql"); os.IsNotExist(err) {
		dir = "../migrations"
	}

	upFile := filepath.Join(dir, "000001_create_users.up.sql")
	downFile := filepath.Join(dir, "000001_create_users.down.sql")

	_, err := os.Stat(upFile)
	require.NoError(t, err, "up migration file should exist")

	_, err = os.Stat(downFile)
	require.NoError(t, err, "down migration file should exist")
}

func TestMigrationUpSQL_Content(t *testing.T) {
	dir := "."
	if _, err := os.Stat("000001_create_users.up.sql"); os.IsNotExist(err) {
		dir = "../migrations"
	}

	content, err := os.ReadFile(filepath.Join(dir, "000001_create_users.up.sql"))
	require.NoError(t, err)

	sql := string(content)

	// Verify table creation
	assert.Contains(t, sql, "CREATE TABLE users")
	assert.Contains(t, sql, "id UUID PRIMARY KEY")
	assert.Contains(t, sql, "apple_id VARCHAR(255) NOT NULL")
	assert.Contains(t, sql, "display_name VARCHAR(100)")
	assert.Contains(t, sql, "device_id VARCHAR(255)")
	assert.Contains(t, sql, "is_deleted BOOLEAN NOT NULL DEFAULT FALSE")
	assert.Contains(t, sql, "created_at TIMESTAMP WITH TIME ZONE")
	assert.Contains(t, sql, "last_active_at TIMESTAMP WITH TIME ZONE")

	// Verify indexes
	assert.Contains(t, sql, "uq_users_apple_id")
	assert.Contains(t, sql, "idx_users_last_active")
	assert.Contains(t, sql, "idx_users_deletion_scheduled")
}

func TestMigrationDownSQL_Content(t *testing.T) {
	dir := "."
	if _, err := os.Stat("000001_create_users.down.sql"); os.IsNotExist(err) {
		dir = "../migrations"
	}

	content, err := os.ReadFile(filepath.Join(dir, "000001_create_users.down.sql"))
	require.NoError(t, err)

	sql := string(content)

	// Verify it drops indexes and table
	assert.True(t, strings.Contains(sql, "DROP INDEX") || strings.Contains(sql, "DROP TABLE"))
	assert.Contains(t, sql, "DROP TABLE IF EXISTS users")
}
