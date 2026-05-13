//go:build integration
// +build integration

// Story 4.3 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 4 条 case：
//   1. happy: migrate Up → 9 张表存在 → migrate Down → 9 张表全消失（仅 schema_migrations）
//   2. edge: 重复 migrate Up → ErrNoChange 被吞 → 返 nil（幂等）
//   3. happy: Up 后通过 INFORMATION_SCHEMA 抽样验关键索引 / 字段类型 / 主键约束
//   4. edge: Up 后 Status 返回 (version=9, dirty=false, nil)
//
// Story 7.2 扩展：把 4 条 case 的断言从 5 张表扩展到 6 张表
//   （新增 user_step_sync_logs：日志表，含 idx_user_date / idx_user_created_at；
//    特殊点：sync_date DATE / accepted_delta_steps INT signed / 无 updated_at / PK = id）
//
// Story 10.3 review r5 [P1] 扩展：把 4 条 case 的断言从 6 张表扩展到 8 张表
//   （新增 rooms / room_members：把 Epic 11.2 的"建表"职责拆出来提前到 Story 10.3，
//    让 WS 网关 self-contained 可部署；JOIN / LEAVE 业务事务仍在 Epic 11.4 / 11.5 落地）
//
// Story 17.2 扩展：把 4 条 case 的断言从 8 张表扩展到 9 张表
//   （新增 emoji_configs：系统表情配置表，含 UNIQUE uk_code + KEY idx_enabled_sort；
//    Epic 17 节点 6 表情广播链路 schema 根基，17.3 seed / 17.4 GET /emojis / 17.5
//    WS emoji.send 校验各路径依赖）
//
// Story 17.3 扩展：在表 schema 不变（仍 9 张）基础上新增 0010_seed_emoji_configs；
//   主要 case 跑 4 行 seed 的内容正确性 + INSERT IGNORE 幂等（不影响表数量断言）；
//   StatusAfterUp 版本号断言从 v=9 改 v=10。
//
// Story 17.3 r1 review [P2] 重写 SeedIdempotent：原版走 Up→Down→Up 路径但 Down
//   把整张表 DROP 掉 → 第二次 Up 跑空表 → 没真正触发 duplicate-code 路径。新版改
//   "预填 admin-flavored 行 → UPDATE schema_migrations 回滚版本号 → 再 Up" 让
//   INSERT IGNORE 真正在 duplicate-code 路径执行 + 断言预存行字段不被 seed 覆盖。
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

// TestMigrateIntegration_UpThenDown 起容器 → migrate Up → 9 张表存在 →
// migrate Down → 9 张表全消失（仅 schema_migrations）。
//
// **Story 10.3 review r5 [P1] 扩展**：从 6 改 8（多了 0007_init_rooms +
// 0008_init_room_members；把 Epic 11.2 的"建表"职责拆出来提前到 Story 10.3，
// 让 WS 网关骨架 self-contained 可部署）。
//
// **Story 17.2 扩展**：从 8 改 9（多了 0009_init_emoji_configs；Epic 17 节点 6
// 表情广播链路 schema 根基）。
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

	// 验证 9 张表存在
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	expectedTables := []string{"users", "user_auth_bindings", "pets", "user_step_accounts", "user_chests", "user_step_sync_logs", "rooms", "room_members", "emoji_configs"}
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

	// migrate Down → 9 张表全消失
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
		('users', 'user_auth_bindings', 'pets', 'user_step_accounts', 'user_chests', 'user_step_sync_logs', 'rooms', 'room_members', 'emoji_configs')`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 9 {
		t.Errorf("after two Up calls, table count = %d, want 9 (Story 10.3 review r5 [P1] 加 rooms / room_members → Story 17.2 加 emoji_configs，总计 9 张表)", tableCount)
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

// TestMigrateIntegration_StatusAfterUp 验证 Up 完成后 Status 返回 (version=10, dirty=false, nil)。
// Story 7.2 扩展：从 5 改 6（多了 0006_init_user_step_sync_logs）
// Story 10.3 review r5 [P1] 扩展：从 6 改 8（多了 0007_init_rooms +
// 0008_init_room_members；把 Epic 11.2 的"建表"职责拆出来提前到 Story 10.3）
// Story 17.2 扩展：从 8 改 9（多了 0009_init_emoji_configs；Epic 17 节点 6
// 表情广播链路 schema 根基）
// Story 17.3 扩展：从 9 改 10（多了 0010_seed_emoji_configs；Epic 17 节点 6 表情 seed）
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
	if v != 10 {
		t.Errorf("Status version = %d, want 10", v)
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

// TestMigrateIntegration_EmojiConfigs_Schema 验证
// migrations/0009_init_emoji_configs.up.sql 钦定的 emoji_configs 表 schema
// 与数据库设计.md §5.15 + V1接口设计.md §11.1 一致：
//
//   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - code VARCHAR(64) NOT NULL + UNIQUE KEY uk_code (code)
//   - name VARCHAR(64) NOT NULL
//   - asset_url VARCHAR(255) NOT NULL DEFAULT ''
//   - sort_order INT NOT NULL DEFAULT 0
//   - is_enabled TINYINT NOT NULL DEFAULT 1
//   - created_at / updated_at DATETIME(3)
//   - KEY idx_enabled_sort (is_enabled, sort_order)
//
// **背景（Story 17.2 引入）**：本 case 验证 0009 migration 落地的 schema
// 与 §5.15 钦定 1:1 对齐；用于在 epics.md §Story 17.2 钦定的"单元测试覆盖 ≥3 case"
// 中作为 schema-correctness 路径（happy / migrate up 后表存在 + 字段类型 +
// 全部索引和约束都符合 §5.15）。
func TestMigrateIntegration_EmojiConfigs_Schema(t *testing.T) {
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

	// 1. INFORMATION_SCHEMA.TABLES：emoji_configs 表存在
	var tableCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'cat_test' AND table_name = 'emoji_configs'`).Scan(&tableCount)
	if err != nil {
		t.Errorf("query emoji_configs table existence: %v", err)
	} else if tableCount != 1 {
		t.Errorf("emoji_configs table count = %d, want 1", tableCount)
	}

	// 2. INFORMATION_SCHEMA.COLUMNS：8 列存在 + 类型对齐
	emojiCols := []struct {
		col          string
		wantDataType string
		wantColType  string
	}{
		{"id", "bigint", "bigint unsigned"},
		{"code", "varchar", "varchar(64)"},
		{"name", "varchar", "varchar(64)"},
		{"asset_url", "varchar", "varchar(255)"},
		{"sort_order", "int", "int"},
		{"is_enabled", "tinyint", "tinyint"},
		{"created_at", "datetime", "datetime(3)"},
		{"updated_at", "datetime", "datetime(3)"},
	}
	for _, c := range emojiCols {
		var dt, ct string
		err := sqlDB.QueryRowContext(ctx, `
			SELECT data_type, column_type FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'emoji_configs' AND column_name = ?`,
			c.col).Scan(&dt, &ct)
		if err != nil {
			t.Errorf("query emoji_configs.%s type: %v", c.col, err)
			continue
		}
		if dt != c.wantDataType {
			t.Errorf("emoji_configs.%s data_type = %q, want %q", c.col, dt, c.wantDataType)
		}
		if ct != c.wantColType {
			t.Errorf("emoji_configs.%s column_type = %q, want %q", c.col, ct, c.wantColType)
		}
	}

	// 3a. emoji_configs.id PK = id（自增；§5.15 + §3.1 钦定）
	var pkCol string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = 'cat_test' AND table_name = 'emoji_configs' AND constraint_name = 'PRIMARY'`).Scan(&pkCol)
	if err != nil {
		t.Errorf("query emoji_configs PK: %v", err)
	} else if pkCol != "id" {
		t.Errorf("emoji_configs PK column = %q, want 'id'", pkCol)
	}

	// 3b. UNIQUE KEY uk_code (code) 存在 + non_unique = 0
	var ukCount int
	var ukNonUnique int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(non_unique) FROM information_schema.statistics
		WHERE table_schema = 'cat_test' AND table_name = 'emoji_configs' AND index_name = 'uk_code'`).Scan(&ukCount, &ukNonUnique)
	if err != nil {
		t.Errorf("query emoji_configs.uk_code: %v", err)
	} else {
		if ukCount == 0 {
			t.Errorf("emoji_configs.uk_code: index not found")
		}
		if ukNonUnique != 0 {
			t.Errorf("emoji_configs.uk_code: non_unique = %d, want 0 (UNIQUE)", ukNonUnique)
		}
	}

	// uk_code 单列 (code)
	var ukCol string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.statistics
		WHERE table_schema = 'cat_test' AND table_name = 'emoji_configs' AND index_name = 'uk_code'
		ORDER BY seq_in_index ASC LIMIT 1`).Scan(&ukCol)
	if err != nil {
		t.Errorf("query emoji_configs.uk_code column: %v", err)
	} else if ukCol != "code" {
		t.Errorf("emoji_configs.uk_code column = %q, want 'code'", ukCol)
	}

	// 3c. KEY idx_enabled_sort 存在 + 列顺序 (is_enabled, sort_order)
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT column_name FROM information_schema.statistics
		WHERE table_schema = 'cat_test' AND table_name = 'emoji_configs' AND index_name = 'idx_enabled_sort'
		ORDER BY seq_in_index ASC`)
	if err != nil {
		t.Errorf("query emoji_configs.idx_enabled_sort columns: %v", err)
	} else {
		defer rows.Close()
		var cols []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Errorf("scan idx_enabled_sort column: %v", err)
				continue
			}
			cols = append(cols, c)
		}
		want := []string{"is_enabled", "sort_order"}
		if len(cols) != 2 {
			t.Errorf("emoji_configs.idx_enabled_sort column count = %d, want 2 (cols=%v)", len(cols), cols)
		} else {
			for i, w := range want {
				if cols[i] != w {
					t.Errorf("emoji_configs.idx_enabled_sort column[%d] = %q, want %q", i, cols[i], w)
				}
			}
		}
	}

	// 4. INFORMATION_SCHEMA.COLUMNS.COLUMN_DEFAULT 默认值校验
	defaultCases := []struct {
		col         string
		wantDefault string
	}{
		{"asset_url", ""},
		{"sort_order", "0"},
		{"is_enabled", "1"},
	}
	for _, c := range defaultCases {
		var def sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'emoji_configs' AND column_name = ?`,
			c.col).Scan(&def)
		if err != nil {
			t.Errorf("query emoji_configs.%s default: %v", c.col, err)
			continue
		}
		if !def.Valid || def.String != c.wantDefault {
			t.Errorf("emoji_configs.%s default = %v, want %q", c.col, def, c.wantDefault)
		}
	}

	// created_at / updated_at 默认值 = CURRENT_TIMESTAMP(3)
	// MySQL 8.0 INFORMATION_SCHEMA 返回的字符串可能是 "CURRENT_TIMESTAMP(3)" 或带函数语义；
	// 用 substring contains 兜底匹配。
	for _, col := range []string{"created_at", "updated_at"} {
		var def sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'emoji_configs' AND column_name = ?`,
			col).Scan(&def)
		if err != nil {
			t.Errorf("query emoji_configs.%s default: %v", col, err)
			continue
		}
		if !def.Valid || !strings.Contains(strings.ToUpper(def.String), "CURRENT_TIMESTAMP") {
			t.Errorf("emoji_configs.%s default = %v, want substring 'CURRENT_TIMESTAMP'", col, def)
		}
	}
}

// TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected 验证
// migrations/0009_init_emoji_configs.up.sql 钦定的 UNIQUE KEY uk_code (code)
// 在运行时被 MySQL 真实拒绝重复 code 插入。
//
// **背景（Story 17.2 引入）**：epics.md §Story 17.2 钦定的"集成测试覆盖（dockertest）：
// migrate up → 尝试 INSERT 重复 code → 数据库拒绝"路径在本 case 落地；
// 是 Story 17.3 seed 用 INSERT IGNORE 兜底 + Story 17.5 校验 emojiCode 合法性 +
// admin 后台未来写入路径的 schema 层根基。
//
// **覆盖路径**：
//  1. migrate up → emoji_configs 表存在
//  2. 插入 emoji_configs (code='test_unique_code_a', name='TestA', sort_order=1001) → 成功
//  3. 再次插入 emoji_configs (code='test_unique_code_a', name='TestA v2', sort_order=1002) → DB 拒绝
//     （UNIQUE KEY uk_code (code) 兜底；same code 不能插两次）；
//     err 必须含 "Duplicate entry"（MySQL 错误码 = 1062 ER_DUP_ENTRY）
//
// 用 database/sql 直跑 raw INSERT（**不**走 GORM）让测试结果**不**依赖 ORM
// 行为差异（与 Story 11.2 落地的
// TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected 同模式）。
//
// **Story 17.3 解耦**：本 case 早期版本用 `'wave'` 作为测试 code，但 17.3 落地
// `0010_seed_emoji_configs.up.sql` 把 `'wave'` 写入 emoji_configs 后，本 case 第一次
// INSERT 会直接因为 seed 已存在而失败 → 违反"先插成功再插冲突"的两步语义。
// 现改用测试专用 code（`test_unique_code_a`）与 seed 的 wave/love/laugh/cry 字面量
// 完全隔离；sort_order 用 1001 / 1002 大于 1000 的值，也与 seed 段的 1-4 隔离。
func TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected(t *testing.T) {
	// **用测试专用 code（test_unique_code_a）与 0010 seed 的 wave/love/laugh/cry 字面量
	// 隔离**，避免 seed 先写入 wave 后导致本 case 第一次 INSERT 就触发 UNIQUE 而非预期的
	// 第二次（Story 17.3 解耦）。
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

	// 步骤 2：首条 emoji_configs (code='test_unique_code_a', name='TestA', sort_order=1001) 必须成功
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES ('test_unique_code_a', 'TestA', 'https://example.com/test_a.png', 1001, 1)`); err != nil {
		t.Fatalf("first insert emoji_configs (code='test_unique_code_a') should succeed: %v", err)
	}

	// 步骤 3：UNIQUE(code) 约束 —— 同 code 再插一次必须被拒
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES ('test_unique_code_a', 'TestA v2', 'https://example.com/test_a_v2.png', 1002, 1)`)
	if err == nil {
		t.Fatalf("expected duplicate-entry error on second insert (code='test_unique_code_a') violating UNIQUE(code), got nil")
	}
	if !strings.Contains(err.Error(), "Duplicate entry") {
		t.Errorf("UNIQUE(code) rejection error message = %q, want substring \"Duplicate entry\"", err.Error())
	}
}

// TestMigrateIntegration_EmojiConfigs_SeedContent 验证
// migrations/0010_seed_emoji_configs.up.sql 钦定的 4 个表情 seed 在 migrate up
// 后真实写入 emoji_configs 表，且每行字段值符合 V1 §11.1 + AR19 + 17-1 r2 lesson
// 约束：
//
//   - 4 个 code 都存在：wave / love / laugh / cry
//   - 每行 asset_url 非空（V1 §11.1 钦定 length ≥ 1；17-1 r2 lesson 收紧禁止 ""）
//   - 每行 is_enabled = 1（enabled 表情才会被 GET /emojis 返回）
//   - 每行 name 非空（VARCHAR(64) NOT NULL）
//   - sort_order 唯一且单调（避免 client 端排序退化到 id 次要键）
//
// **背景（Story 17.3 引入）**：epics.md §Story 17.3 钦定的"集成测试覆盖（dockertest）：
// migrate up → SELECT * FROM emoji_configs → 验证 4 个表情存在 + URL 字段格式合法"
// 路径在本 case 落地；用于 Story 17.4 / 17.5 / Epic 18.1 / Epic 19.1 实装时
// 验证 seed 数据真实在位的根基。
//
// 用 database/sql 直跑 SELECT（**不**走 GORM）让测试结果**不**依赖 ORM 行为差异
// （与 11.2 / 17.2 落地的 dockertest case 同模式）。
func TestMigrateIntegration_EmojiConfigs_SeedContent(t *testing.T) {
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

	// 钦定的 4 个 code + 对应 sort_order（与 0010_seed_emoji_configs.up.sql 1:1 对齐）
	wantSortOrders := map[string]int{
		"wave":  1,
		"love":  2,
		"laugh": 3,
		"cry":   4,
	}

	// 1. SELECT 全表（按 sort_order 升序），>=4 行
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT code, name, asset_url, sort_order, is_enabled
		FROM emoji_configs
		ORDER BY sort_order ASC`)
	if err != nil {
		t.Fatalf("SELECT emoji_configs: %v", err)
	}
	defer rows.Close()

	type rowData struct {
		code      string
		name      string
		assetURL  string
		sortOrder int
		isEnabled int
	}
	var allRows []rowData
	seenCodes := make(map[string]rowData)
	for rows.Next() {
		var r rowData
		if err := rows.Scan(&r.code, &r.name, &r.assetURL, &r.sortOrder, &r.isEnabled); err != nil {
			t.Errorf("scan row: %v", err)
			continue
		}
		allRows = append(allRows, r)
		seenCodes[r.code] = r
	}
	if err := rows.Err(); err != nil {
		t.Errorf("rows.Err: %v", err)
	}

	if len(allRows) < 4 {
		t.Errorf("emoji_configs row count = %d, want >= 4 (Story 17.3 seed 钦定 4 个表情)", len(allRows))
	}

	// 2. 4 个钦定 code 必须全部存在 + 每行字段值符合约束
	for code, wantSO := range wantSortOrders {
		r, ok := seenCodes[code]
		if !ok {
			t.Errorf("seed missing code = %q (钦定 4 个 wave/love/laugh/cry 都必须存在)", code)
			continue
		}
		// a. name 非空
		if len(r.name) == 0 {
			t.Errorf("seed code=%q name 为空 (VARCHAR(64) NOT NULL + seed 钦定非空)", code)
		}
		// b. asset_url 非空（V1 §11.1 + 17-1 r2 lesson）
		if len(r.assetURL) == 0 {
			t.Errorf("seed code=%q asset_url 为空 (V1 §11.1 + 17-1 r2 lesson 钦定 enabled 表情 asset_url 必须非空)", code)
		}
		// c. is_enabled == 1
		if r.isEnabled != 1 {
			t.Errorf("seed code=%q is_enabled = %d, want 1 (enabled 才能被 GET /emojis 返回)", code, r.isEnabled)
		}
		// d. sort_order 与钦定值一致
		if r.sortOrder != wantSO {
			t.Errorf("seed code=%q sort_order = %d, want %d (与 0010 SQL 钦定 1/2/3/4 一致)", code, r.sortOrder, wantSO)
		}
	}

	// 3. sort_order 在 4 个钦定 code 中必须唯一（避免 client 端排序退化到次要键 id）
	sortOrderSeen := make(map[int]string)
	for code := range wantSortOrders {
		r, ok := seenCodes[code]
		if !ok {
			continue
		}
		if prev, dup := sortOrderSeen[r.sortOrder]; dup {
			t.Errorf("sort_order=%d 在 seed 中被 %q 和 %q 重复使用 (4 个 sort_order 必须唯一)", r.sortOrder, prev, code)
		}
		sortOrderSeen[r.sortOrder] = code
	}
}

// TestMigrateIntegration_EmojiConfigs_SeedIdempotent 验证
// migrations/0010_seed_emoji_configs.up.sql 钦定的 INSERT IGNORE 语义在
// duplicate-code 路径下的 server 端表现：
// **当 UNIQUE KEY uk_code 命中时，INSERT IGNORE 不报错 + 不翻倍 + 不覆盖现有字段值**。
//
// **背景（Story 17.3 引入；17.3 r1 [P2] 重写；17.3 r2 [P2] 调整注释语义）**：
//   - 原 case（r1 前）走 Up → Down → Up 路径，但 Down 把整张表 DROP 掉 → 第二次 Up
//     跑空表 → 没真正测到 duplicate-code 路径 → 把 0010.up 从 INSERT IGNORE 改成普通
//     INSERT 也能通过。r1 重写改成 "预填 admin-flavored 行 → 回滚 schema_migrations
//     版本号 → 重跑 0010.up"，让 INSERT IGNORE 真正在 duplicate-code 路径上被执行。
//   - r2 review 把 0010.down 从 r1 的 no-op 改回 narrow DELETE 4 行（migration invariant
//     优先于 admin 数据保留；admin 数据保留改用"code 钦定占用 + 新 migration"约定）。
//     这意味着"down 后预存行保留"语义在 r2 之后**不再成立** —— 本测试**不**测这条；
//     测的核心是上面那句："INSERT IGNORE 在 duplicate code 时 server 端表现：不报错 +
//     不翻倍 + 不覆盖现有值"。测试 setup 仍手工预填 admin-flavored 行（不变），
//     只是结论从"down 后保留 admin 行"收紧成"up 重跑撞 uk_code 不破坏现有数据"。
//
// **覆盖路径**：
//  1. migrate Up 全程 (v=10) → emoji_configs 4 行（0010 seed 写入）
//  2. DELETE seed 4 行 + 手动 INSERT 4 行 admin-flavored 数据（asset_url / name
//     都和 seed 不同，模拟 admin 在上线后改过这些行；本步骤**不**走 down，因此
//     和 r2 后 down=narrow DELETE 的语义不冲突）
//  3. UPDATE schema_migrations SET version = 9 → 让 golang-migrate 认为 0010 还没跑过
//  4. migrate Up 重跑 → 触发 0010.up INSERT IGNORE 命中 uk_code 4 次
//  5. 断言：行数仍 4（不翻倍到 8）+ admin 写入的 asset_url / name 保留不被 seed 覆盖
//
// **三种"伪幂等"实现仍被本测试抓到**：
//   - INSERT INTO（无 IGNORE）→ 步骤 4 撞 uk_code 1062 直接报错
//   - INSERT ... ON DUPLICATE KEY UPDATE name=VALUES(name) → 行数对但 admin 字段被覆盖 → 步骤 5b 断言炸
//   - REPLACE INTO → 同上（覆盖式语义）
//
// **为什么不走 force(9)**：本 migrate 包没暴露 Force API（migrate.go 仅 Up / Down
// / Status / Close）。直接 UPDATE schema_migrations 是 dockertest 集成测试可控
// 范围内的最小操作；也忠实模拟了 ops 在生产里手工修复 dirty 后的回退场景。
//
// 用 database/sql 直跑 SQL（**不**走 GORM）。
func TestMigrateIntegration_EmojiConfigs_SeedIdempotent(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	mig, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer mig.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 步骤 1：第一次 migrate up → emoji_configs 4 行（0010 seed 写入）
	if err := mig.Up(ctx); err != nil {
		t.Fatalf("first migrate Up: %v", err)
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	var countAfterFirstUp int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry')`).Scan(&countAfterFirstUp); err != nil {
		t.Fatalf("SELECT COUNT after first Up: %v", err)
	}
	if countAfterFirstUp != 4 {
		t.Fatalf("after first Up, emoji_configs seed rows = %d, want 4 (Story 17.3 钦定 4 个表情)", countAfterFirstUp)
	}

	// 步骤 2：DELETE seed 4 行 + 手动 INSERT 4 行 admin-flavored 数据（模拟 admin
	// 在上线后调整过 wave/love/laugh/cry 的 asset_url / name 等字段）
	if _, err := sqlDB.ExecContext(ctx,
		`DELETE FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry')`); err != nil {
		t.Fatalf("DELETE seed rows: %v", err)
	}

	type adminRow struct {
		code     string
		name     string
		assetURL string
	}
	// **故意**与 0010.up.sql seed 值不同（asset_url 用 admin-cdn 域名 / name 改成
	// 不同字符），用于步骤 5 断言 INSERT IGNORE 没把 admin 值覆盖回 seed 值。
	adminRows := []adminRow{
		{code: "wave", name: "挥手-admin", assetURL: "https://admin-cdn.example.com/wave.png"},
		{code: "love", name: "爱心-admin", assetURL: "https://admin-cdn.example.com/love.png"},
		{code: "laugh", name: "大笑-admin", assetURL: "https://admin-cdn.example.com/laugh.png"},
		{code: "cry", name: "哭-admin", assetURL: "https://admin-cdn.example.com/cry.png"},
	}
	for i, r := range adminRows {
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES (?, ?, ?, ?, 1)`,
			r.code, r.name, r.assetURL, i+1); err != nil {
			t.Fatalf("INSERT admin row %q: %v", r.code, err)
		}
	}

	// 步骤 3：回滚 schema_migrations 版本号到 9，让 golang-migrate 认为 0010 还没跑过；
	// 这是触发 "duplicate-code 路径下重跑 0010.up" 的关键。
	// schema_migrations 表是 golang-migrate 自动维护的版本元数据表
	// （PRIMARY KEY version + dirty TINYINT）；直接 UPDATE 是集成测试 fixture 可控操作。
	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE schema_migrations SET version = 9, dirty = 0 WHERE 1=1`); err != nil {
		t.Fatalf("UPDATE schema_migrations to v=9: %v", err)
	}

	// migrate New 已经在步骤 0 打开了 source / database driver；要重新读 schema_migrations
	// 需要新开一个 Migrator 实例（golang-migrate 内部 cache 版本号；不重开会以为还在 v=10）。
	if err := mig.Close(); err != nil {
		t.Fatalf("close first migrator: %v", err)
	}
	mig2, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New (after rollback to v=9): %v", err)
	}
	defer mig2.Close()

	// 步骤 4：再跑一次 migrate Up → 0010.up 会被重跑 → INSERT IGNORE 命中 uk_code
	if err := mig2.Up(ctx); err != nil {
		t.Fatalf("second migrate Up (after schema_migrations rollback): %v", err)
	}

	// 步骤 5a：行数仍恰好 4（不翻倍到 8 —— INSERT IGNORE 没插入重复行）
	var countAfterSecondUp int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry')`).Scan(&countAfterSecondUp); err != nil {
		t.Fatalf("SELECT COUNT after second Up: %v", err)
	}
	if countAfterSecondUp != 4 {
		t.Errorf("after second Up, emoji_configs seed rows = %d, want 4 (INSERT IGNORE 兜底不重复插入；不应翻倍到 8)", countAfterSecondUp)
	}

	// 步骤 5b：每行 asset_url / name 仍是 admin 写入的值，不是 0010 seed 值
	// —— 真正验证 INSERT IGNORE 没覆盖预存行；这是把 0010.up 改成 INSERT INTO
	// （非 IGNORE）时**必然**炸的断言（普通 INSERT 会撞 uk_code 1062 直接报错）。
	for _, want := range adminRows {
		var gotName, gotAssetURL string
		err := sqlDB.QueryRowContext(ctx,
			`SELECT name, asset_url FROM emoji_configs WHERE code = ?`, want.code).
			Scan(&gotName, &gotAssetURL)
		if err != nil {
			t.Errorf("SELECT admin row %q after second Up: %v", want.code, err)
			continue
		}
		if gotName != want.name {
			t.Errorf("emoji %q name = %q, want %q (INSERT IGNORE 应保留 admin 值不覆盖)", want.code, gotName, want.name)
		}
		if gotAssetURL != want.assetURL {
			t.Errorf("emoji %q asset_url = %q, want %q (INSERT IGNORE 应保留 admin 值不覆盖)", want.code, gotAssetURL, want.assetURL)
		}
	}
}
