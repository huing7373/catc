//go:build integration
// +build integration

// Story 4.3 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 4 条 case：
//   1. happy: migrate Up → 5 张表存在 → migrate Down → 5 张表全消失（仅 schema_migrations）
//   2. edge: 重复 migrate Up → ErrNoChange 被吞 → 返 nil（幂等）
//   3. happy: Up 后通过 INFORMATION_SCHEMA 抽样验关键索引 / 字段类型 / 主键约束
//   4. edge: Up 后 Status 返回 (version=5, dirty=false, nil)
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// docker 不可用时 t.Skip("docker not available")，不让 CI 阻塞（不要写 panic / fail）。
//
// 每条测试独立起一个容器（`t.Cleanup` 释放）—— 避免测试间状态污染。容器端口动态分配
// 不固定 3306，避免与本机 MySQL 冲突。

package migrate_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/huing/cat/server/internal/infra/migrate"
)

// startMySQL 起一个 mysql:8.0 容器，等 ping 通后返回 (DSN, cleanup)。
//
// 设计与 4.2 db/mysql_integration_test.go 的 startMySQL 一致：
//   - MYSQL_ROOT_PASSWORD=catdev / MYSQL_DATABASE=cat_test
//   - 端口由 dockertest 动态分配（不固定 3306）
//   - retry 60 次，每次 2 秒（mysql:8.0 冷启 ~10-30s）
//   - DSN 含 multiStatements=true 让 golang-migrate 接受多语句
//
// **故意复制一份**而非抽到跨包 helper —— 避免新建跨包 testing util 的范围扩散。
// 4.2 的副本 db/mysql_integration_test.go 不动。
func startMySQL(t *testing.T) (string, func()) {
	t.Helper()

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	if err := pool.Client.Ping(); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "mysql",
		Tag:        "8.0",
		Env: []string{
			"MYSQL_ROOT_PASSWORD=catdev",
			"MYSQL_DATABASE=cat_test",
		},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		t.Skipf("could not start mysql container: %v", err)
	}

	hostPort := resource.GetPort("3306/tcp")
	dsn := fmt.Sprintf("root:catdev@tcp(127.0.0.1:%s)/cat_test?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true", hostPort)

	pool.MaxWait = 90 * time.Second
	if err := pool.Retry(func() error {
		sqlDB, err := sql.Open("mysql", dsn)
		if err != nil {
			return err
		}
		defer sqlDB.Close()
		return sqlDB.Ping()
	}); err != nil {
		_ = pool.Purge(resource)
		t.Skipf("mysql container did not become ready: %v", err)
	}

	cleanup := func() {
		_ = pool.Purge(resource)
	}
	return dsn, cleanup
}

// migrationsPath 返回 `server/migrations` 的绝对路径。集成测试可能从 server/ 或
// server/internal/infra/migrate/ 启动（go test 默认从测试文件所在目录运行），
// 用相对路径很容易因 cwd 不同而 fail —— 用 filepath.Abs 锚定到本测试文件相对路径。
func migrationsPath(t *testing.T) string {
	t.Helper()
	// 测试文件在 server/internal/infra/migrate/ 下，migrations 在 server/migrations/
	// → 相对路径 ../../../migrations
	abs, err := filepath.Abs("../../../migrations")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return abs
}

// TestMigrateIntegration_UpThenDown 起容器 → migrate Up → 5 张表存在 →
// migrate Down → 5 张表全消失（仅 schema_migrations）。
func TestMigrateIntegration_UpThenDown(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	mig, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer mig.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := mig.Up(ctx); err != nil {
		t.Fatalf("migrate Up: %v", err)
	}

	// 验证 5 张表存在
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	expectedTables := []string{"users", "user_auth_bindings", "pets", "user_step_accounts", "user_chests"}
	for _, table := range expectedTables {
		var count int
		err := sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = 'cat_test' AND table_name = ?`, table).Scan(&count)
		if err != nil {
			t.Errorf("query table %s: %v", table, err)
			continue
		}
		if count != 1 {
			t.Errorf("after Up, table %s: count=%d, want 1", table, count)
		}
	}

	// migrate Down → 5 张表全消失
	if err := mig.Down(ctx); err != nil {
		t.Fatalf("migrate Down: %v", err)
	}

	for _, table := range expectedTables {
		var count int
		err := sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = 'cat_test' AND table_name = ?`, table).Scan(&count)
		if err != nil {
			t.Errorf("query table %s after Down: %v", table, err)
			continue
		}
		if count != 0 {
			t.Errorf("after Down, table %s: count=%d, want 0 (should be dropped)", table, count)
		}
	}
}

// TestMigrateIntegration_UpTwice_Idempotent 验证 Up 幂等：连续两次都返 nil
// （第二次底层 ErrNoChange 被本包吞掉）。
func TestMigrateIntegration_UpTwice_Idempotent(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	mig, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer mig.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := mig.Up(ctx); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if err := mig.Up(ctx); err != nil {
		t.Errorf("second Up (should be no-op): got %v, want nil", err)
	}

	// 表数量应不变
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	var tableCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'cat_test' AND table_name IN
		('users', 'user_auth_bindings', 'pets', 'user_step_accounts', 'user_chests')`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 5 {
		t.Errorf("after two Up calls, table count = %d, want 5", tableCount)
	}
}

// TestMigrateIntegration_TablesPresent_AfterUp 验证 5 张表的关键 schema 元素：
//   - users.id BIGINT UNSIGNED + uk_guest_uid + idx_current_room_id + idx_created_at
//   - user_auth_bindings.uk_auth_type_identifier + idx_user_id
//   - pets.uk_user_default_pet + idx_user_id
//   - user_step_accounts: PK = user_id（不是自增 id）
//   - user_chests.uk_user_id + idx_status_unlock_at
//   - 所有 created_at / updated_at 类型 = datetime(3)
func TestMigrateIntegration_TablesPresent_AfterUp(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	mig, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer mig.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := mig.Up(ctx); err != nil {
		t.Fatalf("migrate Up: %v", err)
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	// 索引验证：从 INFORMATION_SCHEMA.STATISTICS 读
	indexCases := []struct {
		table     string
		indexName string
	}{
		{"users", "uk_guest_uid"},
		{"users", "idx_current_room_id"},
		{"users", "idx_created_at"},
		{"user_auth_bindings", "uk_auth_type_identifier"},
		{"user_auth_bindings", "idx_user_id"},
		{"pets", "uk_user_default_pet"},
		{"pets", "idx_user_id"},
		{"user_chests", "uk_user_id"},
		{"user_chests", "idx_status_unlock_at"},
	}
	for _, c := range indexCases {
		var count int
		err := sqlDB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM information_schema.statistics
			WHERE table_schema = 'cat_test' AND table_name = ? AND index_name = ?`,
			c.table, c.indexName).Scan(&count)
		if err != nil {
			t.Errorf("query index %s.%s: %v", c.table, c.indexName, err)
			continue
		}
		if count == 0 {
			t.Errorf("index %s.%s: not found", c.table, c.indexName)
		}
	}

	// user_step_accounts 主键 = user_id
	var pkColumn string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = 'cat_test' AND table_name = 'user_step_accounts'
		  AND constraint_name = 'PRIMARY'`).Scan(&pkColumn)
	if err != nil {
		t.Errorf("query user_step_accounts PK: %v", err)
	} else if pkColumn != "user_id" {
		t.Errorf("user_step_accounts PK column = %q, want 'user_id'", pkColumn)
	}

	// users.id 类型 = bigint unsigned
	var dataType, columnType string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT data_type, column_type FROM information_schema.columns
		WHERE table_schema = 'cat_test' AND table_name = 'users' AND column_name = 'id'`).
		Scan(&dataType, &columnType)
	if err != nil {
		t.Errorf("query users.id type: %v", err)
	} else {
		if dataType != "bigint" {
			t.Errorf("users.id data_type = %q, want 'bigint'", dataType)
		}
		// MySQL 8.0 column_type for BIGINT UNSIGNED is "bigint unsigned"
		if columnType != "bigint unsigned" {
			t.Errorf("users.id column_type = %q, want 'bigint unsigned'", columnType)
		}
	}

	// users.guest_uid VARCHAR(128)
	var charLen int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT character_maximum_length FROM information_schema.columns
		WHERE table_schema = 'cat_test' AND table_name = 'users' AND column_name = 'guest_uid'`).
		Scan(&charLen)
	if err != nil {
		t.Errorf("query users.guest_uid length: %v", err)
	} else if charLen != 128 {
		t.Errorf("users.guest_uid length = %d, want 128", charLen)
	}

	// 所有 created_at 字段都是 datetime(3)
	timeCols := []struct{ table, col string }{
		{"users", "created_at"},
		{"users", "updated_at"},
		{"user_auth_bindings", "created_at"},
		{"pets", "updated_at"},
		{"user_step_accounts", "created_at"},
		{"user_chests", "updated_at"},
	}
	for _, c := range timeCols {
		var ct string
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_type FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = ? AND column_name = ?`,
			c.table, c.col).Scan(&ct)
		if err != nil {
			t.Errorf("query %s.%s type: %v", c.table, c.col, err)
			continue
		}
		if ct != "datetime(3)" {
			t.Errorf("%s.%s column_type = %q, want 'datetime(3)'", c.table, c.col, ct)
		}
	}

	// version 字段（user_step_accounts / user_chests）类型 = bigint unsigned
	for _, table := range []string{"user_step_accounts", "user_chests"} {
		var ct string
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_type FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = ? AND column_name = 'version'`, table).Scan(&ct)
		if err != nil {
			t.Errorf("query %s.version type: %v", table, err)
			continue
		}
		if ct != "bigint unsigned" {
			t.Errorf("%s.version column_type = %q, want 'bigint unsigned'", table, ct)
		}
	}
}

// TestMigrateIntegration_StatusAfterUp 验证 Up 完成后 Status 返回 (version=5, dirty=false, nil)。
func TestMigrateIntegration_StatusAfterUp(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	mig, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer mig.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Up 之前 Status 应返回 (0, false, nil)（ErrNilVersion 被吞掉）
	v0, dirty0, err := mig.Status(ctx)
	if err != nil {
		t.Fatalf("Status before Up: %v", err)
	}
	if v0 != 0 || dirty0 {
		t.Errorf("Status before Up: got (%d, %t), want (0, false)", v0, dirty0)
	}

	if err := mig.Up(ctx); err != nil {
		t.Fatalf("migrate Up: %v", err)
	}

	v, dirty, err := mig.Status(ctx)
	if err != nil {
		t.Fatalf("Status after Up: %v", err)
	}
	if v != 5 {
		t.Errorf("Status version = %d, want 5", v)
	}
	if dirty {
		t.Errorf("Status dirty = true, want false")
	}
}
