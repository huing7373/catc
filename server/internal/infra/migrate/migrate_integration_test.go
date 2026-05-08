//go:build integration
// +build integration

// Story 4.3 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 4 条 case：
//   1. happy: migrate Up → 8 张表存在 → migrate Down → 8 张表全消失（仅 schema_migrations）
//   2. edge: 重复 migrate Up → ErrNoChange 被吞 → 返 nil（幂等）
//   3. happy: Up 后通过 INFORMATION_SCHEMA 抽样验关键索引 / 字段类型 / 主键约束
//   4. edge: Up 后 Status 返回 (version=8, dirty=false, nil)
//
// Story 7.2 扩展：把 4 条 case 的断言从 5 张表扩展到 6 张表
//   （新增 user_step_sync_logs：日志表，含 idx_user_date / idx_user_created_at；
//    特殊点：sync_date DATE / accepted_delta_steps INT signed / 无 updated_at / PK = id）
//
// Story 10.3 review r5 [P1] 扩展：把 4 条 case 的断言从 6 张表扩展到 8 张表
//   （新增 rooms / room_members：把 Epic 11.2 的"建表"职责拆出来提前到 Story 10.3，
//    让 WS 网关 self-contained 可部署；JOIN / LEAVE 业务事务仍在 Epic 11.4 / 11.5 落地）
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
	"strings"
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

// TestMigrateIntegration_UpThenDown 起容器 → migrate Up → 8 张表存在 →
// migrate Down → 8 张表全消失（仅 schema_migrations）。
//
// **Story 10.3 review r5 [P1] 扩展**：从 6 改 8（多了 0007_init_rooms +
// 0008_init_room_members；把 Epic 11.2 的"建表"职责拆出来提前到 Story 10.3，
// 让 WS 网关骨架 self-contained 可部署）。
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

	// 验证 8 张表存在
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	expectedTables := []string{"users", "user_auth_bindings", "pets", "user_step_accounts", "user_chests", "user_step_sync_logs", "rooms", "room_members"}
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

// TestMigrateIntegration_RoomsAndRoomMembers_Schema 验证 Story 10.3 review r5 [P1]
// 新增的两张表 schema 关键元素：
//   - rooms: id PK + idx_creator_user_id + idx_status_created_at + 默认值 + 类型
//   - room_members: id PK + uk_user_id + uk_room_user + idx_room_id + 关键约束
//
// 防回归点：rooms / room_members 表存在 + 索引齐 → wsTablesReady() 启动期 sniff
// 通过 → /ws/rooms/:roomId 路由挂载（不再 r3 时代 silent 404）。
func TestMigrateIntegration_RoomsAndRoomMembers_Schema(t *testing.T) {
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

	// 索引存在性检查
	indexCases := []struct {
		table     string
		indexName string
	}{
		{"rooms", "idx_creator_user_id"},
		{"rooms", "idx_status_created_at"},
		{"room_members", "uk_user_id"},
		{"room_members", "uk_room_user"},
		{"room_members", "idx_room_id"},
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

	// rooms.id PK = id（自增）；与数据库设计 §5.13 钦定一致
	var pkRooms string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = 'cat_test' AND table_name = 'rooms' AND constraint_name = 'PRIMARY'`).Scan(&pkRooms)
	if err != nil {
		t.Errorf("query rooms PK: %v", err)
	} else if pkRooms != "id" {
		t.Errorf("rooms PK column = %q, want 'id'", pkRooms)
	}

	// room_members.id PK = id（与数据库设计 §5.14 钦定一致）
	var pkRM string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = 'cat_test' AND table_name = 'room_members' AND constraint_name = 'PRIMARY'`).Scan(&pkRM)
	if err != nil {
		t.Errorf("query room_members PK: %v", err)
	} else if pkRM != "id" {
		t.Errorf("room_members PK column = %q, want 'id'", pkRM)
	}

	// rooms.max_members 类型 = tinyint unsigned + 默认 4
	var maxMembersDefault sql.NullString
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_default FROM information_schema.columns
		WHERE table_schema = 'cat_test' AND table_name = 'rooms' AND column_name = 'max_members'`).Scan(&maxMembersDefault)
	if err != nil {
		t.Errorf("query rooms.max_members default: %v", err)
	} else if !maxMembersDefault.Valid || maxMembersDefault.String != "4" {
		t.Errorf("rooms.max_members default = %v, want '4'", maxMembersDefault)
	}

	// rooms.status 默认 1（active）
	var statusDefault sql.NullString
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_default FROM information_schema.columns
		WHERE table_schema = 'cat_test' AND table_name = 'rooms' AND column_name = 'status'`).Scan(&statusDefault)
	if err != nil {
		t.Errorf("query rooms.status default: %v", err)
	} else if !statusDefault.Valid || statusDefault.String != "1" {
		t.Errorf("rooms.status default = %v, want '1' (=active per §6.12)", statusDefault)
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
		('users', 'user_auth_bindings', 'pets', 'user_step_accounts', 'user_chests', 'user_step_sync_logs', 'rooms', 'room_members')`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 8 {
		t.Errorf("after two Up calls, table count = %d, want 8 (Story 10.3 review r5 [P1] 加 rooms / room_members)", tableCount)
	}
}

// TestMigrateIntegration_TablesPresent_AfterUp 验证 6 张表的关键 schema 元素：
//   - users.id BIGINT UNSIGNED + uk_guest_uid + idx_current_room_id + idx_created_at
//   - user_auth_bindings.uk_auth_type_identifier + idx_user_id
//   - pets.uk_user_default_pet + idx_user_id
//   - user_step_accounts: PK = user_id（不是自增 id）
//   - user_chests.uk_user_id + idx_status_unlock_at
//   - user_step_sync_logs: PK = id + idx_user_date + idx_user_created_at + 字段类型 +
//     无 updated_at（日志表 append-only）—— Story 7.2 扩展
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
		{"user_step_sync_logs", "idx_user_date"},
		{"user_step_sync_logs", "idx_user_created_at"},
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
		{"user_step_sync_logs", "created_at"},
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

	// Story 7.2: user_step_sync_logs 字段类型逐项验证（严格对齐 §5.5）
	//   - id / user_id / client_total_steps / client_ts: bigint unsigned
	//   - sync_date: data_type=date（不是 datetime）
	//   - accepted_delta_steps: int（signed，不是 unsigned；§5.5 钦定保留 signed 让未来可能的负向修正可用）
	//   - motion_state / source: tinyint
	//   - created_at: datetime(3)（已在 timeCols 验过；这里是字段级一致性兜底）
	syncLogCols := []struct {
		col          string
		wantDataType string
		wantColType  string
	}{
		{"id", "bigint", "bigint unsigned"},
		{"user_id", "bigint", "bigint unsigned"},
		{"sync_date", "date", "date"},
		{"client_total_steps", "bigint", "bigint unsigned"},
		{"accepted_delta_steps", "int", "int"},
		{"motion_state", "tinyint", "tinyint"},
		{"source", "tinyint", "tinyint"},
		{"client_ts", "bigint", "bigint unsigned"},
	}
	for _, c := range syncLogCols {
		var dt, ct string
		err := sqlDB.QueryRowContext(ctx, `
			SELECT data_type, column_type FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'user_step_sync_logs' AND column_name = ?`,
			c.col).Scan(&dt, &ct)
		if err != nil {
			t.Errorf("query user_step_sync_logs.%s type: %v", c.col, err)
			continue
		}
		if dt != c.wantDataType {
			t.Errorf("user_step_sync_logs.%s data_type = %q, want %q", c.col, dt, c.wantDataType)
		}
		if ct != c.wantColType {
			t.Errorf("user_step_sync_logs.%s column_type = %q, want %q", c.col, ct, c.wantColType)
		}
	}

	// Story 7.2: user_step_sync_logs 是日志表（append-only），§5.5 钦定不含 updated_at 列
	var updatedAtCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'cat_test' AND table_name = 'user_step_sync_logs' AND column_name = 'updated_at'`).Scan(&updatedAtCount)
	if err != nil {
		t.Errorf("query user_step_sync_logs.updated_at column existence: %v", err)
	}
	if updatedAtCount != 0 {
		t.Errorf("user_step_sync_logs.updated_at unexpectedly present (column count=%d, want 0; this table is append-only per §5.5)", updatedAtCount)
	}

	// Story 7.2: user_step_sync_logs PK = id（自增），与 user_step_accounts PK = user_id 不同
	var syncLogPK string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = 'cat_test' AND table_name = 'user_step_sync_logs' AND constraint_name = 'PRIMARY'`).Scan(&syncLogPK)
	if err != nil {
		t.Errorf("query user_step_sync_logs PK: %v", err)
	} else if syncLogPK != "id" {
		t.Errorf("user_step_sync_logs PK column = %q, want 'id'", syncLogPK)
	}
}

// TestMigrateIntegration_StatusAfterUp 验证 Up 完成后 Status 返回 (version=8, dirty=false, nil)。
// Story 7.2 扩展：从 5 改 6（多了 0006_init_user_step_sync_logs）
// Story 10.3 review r5 [P1] 扩展：从 6 改 8（多了 0007_init_rooms +
// 0008_init_room_members；把 Epic 11.2 的"建表"职责拆出来提前到 Story 10.3）
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
	if v != 8 {
		t.Errorf("Status version = %d, want 8", v)
	}
	if dirty {
		t.Errorf("Status dirty = true, want false")
	}
}

// TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected 验证
// migrations/0008_init_room_members.up.sql 钦定的两个 UNIQUE 约束在运行时被
// MySQL 真实拒绝重复插入：
//
//   - UNIQUE KEY uk_user_id (user_id)：一个用户同一时间只在一个房间
//     （§5.14 设计说明 + §7.1 钦定）
//   - UNIQUE KEY uk_room_user (room_id, user_id)：房间内同一用户只能出现一次
//
// **背景（Story 11.2 引入）**：Story 10.3 review r5 [P1] 落地的 0008 migration
// 的 schema 元数据校验已在 TestMigrateIntegration_RoomsAndRoomMembers_Schema 覆盖
// （uk_user_id / uk_room_user 索引存在），但**运行时 INSERT 拒绝行为**未覆盖 ——
// epics.md §Story 11.2 钦定的"集成测试覆盖：故意尝试违反 UNIQUE(user_id)（同 user
// 插两条 room_members）→ 数据库拒绝插入"路径在本 case 落地。
//
// **覆盖路径**：
//  1. migrate up → rooms / room_members 表存在
//  2. 插入 rooms (id=3001) + (id=3002) 两个房间
//  3. 插入 room_members (room_id=3001, user_id=2001) → 成功
//  4. 再次插入 room_members (room_id=3002, user_id=2001) → DB 拒绝
//     （UNIQUE(user_id) 单列约束兜底；同 user 不能在两个不同房间）
//  5. 再次插入 room_members (room_id=3001, user_id=2001) → DB 拒绝
//     （UNIQUE(room_id, user_id) 复合约束兜底；同 (room, user) 不能重复）
//
// 错误断言用 substring "Duplicate entry"（MySQL 错误码 1062 = ER_DUP_ENTRY 的
// 消息前缀；不同 MySQL 版本消息文本可能略有差异，但 "Duplicate entry" 是稳定
// contract）。
//
// 用 database/sql 直跑 raw INSERT（**不**走 GORM）让测试结果**不**依赖 ORM
// 行为差异。
func TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected(t *testing.T) {
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

	// 步骤 2：插入 2 个房间（让 UNIQUE 测试可对照"同 user 跨房间"语义）
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO rooms (id, creator_user_id, status, max_members) VALUES (3001, 2001, 1, 4)`); err != nil {
		t.Fatalf("insert rooms id=3001: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO rooms (id, creator_user_id, status, max_members) VALUES (3002, 2002, 1, 4)`); err != nil {
		t.Fatalf("insert rooms id=3002: %v", err)
	}

	// 步骤 3：首条 room_members (room=3001, user=2001) 必须成功
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO room_members (room_id, user_id) VALUES (3001, 2001)`); err != nil {
		t.Fatalf("first insert room_members (3001, 2001) should succeed: %v", err)
	}

	// 步骤 4：UNIQUE(user_id) 单列约束 —— 同 user 插另一房间必须被拒
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO room_members (room_id, user_id) VALUES (3002, 2001)`)
	if err == nil {
		t.Fatalf("expected duplicate-entry error on (room=3002, user=2001) violating UNIQUE(user_id), got nil")
	}
	if !strings.Contains(err.Error(), "Duplicate entry") {
		t.Errorf("UNIQUE(user_id) rejection error message = %q, want substring \"Duplicate entry\"", err.Error())
	}

	// 步骤 5：UNIQUE(room_id, user_id) 复合约束 —— 同 (room, user) 插重复必须被拒
	// 注意：步骤 4 已让 (3002, 2001) 失败 → 仅占用 (3001, 2001)；本步骤再插
	// (3001, 2001) 触发 uk_room_user 兜底（也会触发 uk_user_id；MySQL 报第一个
	// 命中的 UNIQUE，约束名取决于内部检查顺序，但不影响 "Duplicate entry" 关键字判定）
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO room_members (room_id, user_id) VALUES (3001, 2001)`)
	if err == nil {
		t.Fatalf("expected duplicate-entry error on duplicate (room=3001, user=2001), got nil")
	}
	if !strings.Contains(err.Error(), "Duplicate entry") {
		t.Errorf("UNIQUE(room_id, user_id) rejection error message = %q, want substring \"Duplicate entry\"", err.Error())
	}
}
