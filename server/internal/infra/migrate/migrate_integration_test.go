//go:build integration
// +build integration

// Story 4.3 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 4 条 case：
//   1. happy: migrate Up → 13 张表存在 → migrate Down → 13 张表全消失（仅 schema_migrations）
//   2. edge: 重复 migrate Up → ErrNoChange 被吞 → 返 nil（幂等）
//   3. happy: Up 后通过 INFORMATION_SCHEMA 抽样验关键索引 / 字段类型 / 主键约束
//   4. edge: Up 后 Status 返回 (version=15, dirty=false, nil)
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
// Story 20.2 扩展：把 4 条 case 的断言从 9 张表扩展到 10 张表
//   （新增 cosmetic_items：装扮配置表，含 UNIQUE uk_code + KEY idx_slot_rarity +
//    KEY idx_enabled_weight；Epic 20 节点 7 宝箱业务链路 schema 根基，20.3 seed /
//    20.5 GET /chest/current / 20.6 POST /chest/open 加权抽取各路径依赖）；
//   StatusAfterUp 版本号断言从 v=10 改 v=11。
//
// Story 20.3 扩展：在表 schema 不变（仍 10 张）基础上新增 0012_seed_cosmetic_items；
//   主要 case 跑 15 行 seed 的内容正确性 + AR18 各品质数量约束 + common 至少覆盖
//   4 个不同 slot + ON DUPLICATE KEY UPDATE 强制覆盖语义（不影响表数量断言）；
//   StatusAfterUp 版本号断言从 v=11 改 v=12。
//
// Story 20.4 扩展：把 4 条 case 的断言从 10 张表扩展到 11 张表
//   （新增 chest_open_logs：append-only 开箱日志表，含 KEY idx_user_id_created_at +
//    KEY idx_reward_cosmetic_item_id；Epic 20 节点 7 宝箱业务链路 schema 根基，20.6
//    POST /chest/open 事务步骤 5h 写一条 chest_open_logs 行 / 20.9 Layer 2 集成测试
//    断言 chest_open_logs 行数 + 字段值 / 未来运营查询路径依赖）；
//   StatusAfterUp 版本号断言从 v=12 改 v=13。
//
// Story 20.6 扩展（漏更新；Story 23.2 顺手对账补记）：Story 20.6 落地
//   0014_init_chest_open_idempotency_records（开箱接口幂等记录表，含 UNIQUE
//   uk_user_id_key + KEY idx_status_created_at）时**漏更新**本文件断言 —— 真实
//   建表数应为 12 张 / StatusAfterUp 版本号应为 v=14，但 expectedTables 仍停在
//   11 张（停在 0013 chest_open_logs）、StatusAfterUp 仍停在 v=13。该积压由
//   Story 23.2 在加 0015 时一并对账补齐（不改 0014 migration 本身，仅断言对账）。
//
// Story 23.2 扩展：把 4 条 case 的断言一次性对账 + 扩展到 13 张表
//   （补齐 0014 chest_open_idempotency_records 积压 + 新增 0015
//    user_cosmetic_items：玩家装扮实例表，含 KEY idx_user_id_status +
//    KEY idx_user_id_cosmetic_item_id + KEY idx_source，无 UNIQUE；Epic 23 节点 8
//    仓库 / 穿戴 / 合成业务链路 schema 根基，23.4 GET /cosmetics/inventory 聚合 /
//    23.5 开箱补入仓 INSERT / Epic 26 穿戴 status 推进 / Epic 32 合成消耗实例
//    各路径依赖）；StatusAfterUp 版本号断言从 v=13（积压值）一次对账到 v=15
//    （0014 + 0015 两条）；序号纠偏：epics.md §Story 23.2 文字写 0014，因 0014
//    被 Story 20.6 占用，本 story 实际用 0015（属历史时序错位，非契约/范围变更）。
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

// TestMigrateIntegration_UpThenDown 起容器 → migrate Up → 13 张表存在 →
// migrate Down → 13 张表全消失（仅 schema_migrations）。
//
// **Story 10.3 review r5 [P1] 扩展**：从 6 改 8（多了 0007_init_rooms +
// 0008_init_room_members；把 Epic 11.2 的"建表"职责拆出来提前到 Story 10.3，
// 让 WS 网关骨架 self-contained 可部署）。
//
// **Story 17.2 扩展**：从 8 改 9（多了 0009_init_emoji_configs；Epic 17 节点 6
// 表情广播链路 schema 根基）。
//
// **Story 20.2 扩展**：从 9 改 10（多了 0011_init_cosmetic_items；Epic 20 节点 7
// 宝箱业务链路 schema 根基）。
//
// **Story 20.4 扩展**：从 10 改 11（多了 0013_init_chest_open_logs；Epic 20 节点 7
// 宝箱开箱日志表，append-only 无 updated_at / 无 UNIQUE；为 20.6 开箱事务步骤 5h
// INSERT chest_open_logs / 20.9 Layer 2 集成测试 / 未来运营查询路径提供 schema 根基）。
//
// **Story 23.2 一次性对账 + 扩展**：从 11 改 13 ——
//   - 补 chest_open_idempotency_records（Story 20.6 落地 0014 时漏更新
//     expectedTables，本 story 顺手对账补齐；不改 0014 migration 本身）
//   - 加 user_cosmetic_items（本 story 0015；Epic 23 节点 8 玩家装扮实例表
//     schema 根基，23.4 inventory 聚合 / 23.5 开箱补入仓 / Epic 26 穿戴 /
//     Epic 32 合成各路径依赖）。
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

	// 验证 13 张表存在（Story 23.2 一次性对账：补 0014 chest_open_idempotency_records
	// 积压 + 加 0015 user_cosmetic_items）
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	expectedTables := []string{"users", "user_auth_bindings", "pets", "user_step_accounts", "user_chests", "user_step_sync_logs", "rooms", "room_members", "emoji_configs", "cosmetic_items", "chest_open_logs", "chest_open_idempotency_records", "user_cosmetic_items"}
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

	// migrate Down → 13 张表全消失（Story 23.2 对账后 13 张）
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
		('users', 'user_auth_bindings', 'pets', 'user_step_accounts', 'user_chests', 'user_step_sync_logs', 'rooms', 'room_members', 'emoji_configs', 'cosmetic_items', 'chest_open_logs', 'chest_open_idempotency_records', 'user_cosmetic_items')`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 13 {
		t.Errorf("after two Up calls, table count = %d, want 13 (Story 10.3 review r5 [P1] 加 rooms / room_members → Story 17.2 加 emoji_configs → Story 20.2 加 cosmetic_items → Story 20.4 加 chest_open_logs → Story 23.2 顺手对账补 0014 chest_open_idempotency_records 积压 + 加 0015 user_cosmetic_items，总计 13 张表)", tableCount)
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

// TestMigrateIntegration_StatusAfterUp 验证 Up 完成后 Status 返回 (version=15, dirty=false, nil)。
// Story 7.2 扩展：从 5 改 6（多了 0006_init_user_step_sync_logs）
// Story 10.3 review r5 [P1] 扩展：从 6 改 8（多了 0007_init_rooms +
// 0008_init_room_members；把 Epic 11.2 的"建表"职责拆出来提前到 Story 10.3）
// Story 17.2 扩展：从 8 改 9（多了 0009_init_emoji_configs；Epic 17 节点 6
// 表情广播链路 schema 根基）
// Story 17.3 扩展：从 9 改 10（多了 0010_seed_emoji_configs；Epic 17 节点 6 表情 seed）
// Story 20.2 扩展：从 10 改 11（多了 0011_init_cosmetic_items；Epic 20 节点 7
// 宝箱业务链路 schema 根基）
// Story 20.3 扩展：从 11 改 12（多了 0012_seed_cosmetic_items；Epic 20 节点 7 cosmetic seed）
// Story 20.4 扩展：从 12 改 13（多了 0013_init_chest_open_logs；Epic 20 节点 7
// 宝箱开箱日志表，append-only schema 根基）
// Story 20.6 扩展：从 13 改 14（多了 0014_init_chest_open_idempotency_records；
// 开箱接口幂等记录表；**本扩展在 Story 23.2 对账时补记，原 Story 20.6 落地 0014
// 时漏更新本断言导致 v=13 积压**）
// Story 23.2 扩展：从 14 改 15（多了 0015_init_user_cosmetic_items；Epic 23 节点 8
// 玩家装扮实例表 schema 根基；序号纠偏：epics.md §Story 23.2 文字写 0014，因 0014
// 被 Story 20.6 占用，本 story 实际用 0015，属历史时序错位非契约/范围变更）
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
	if v != 15 {
		t.Errorf("Status version = %d, want 15 (0001~0015；Story 23.2 一次性对账：补 Story 20.6 漏更新的 0014 积压 + 加本 story 0015)", v)
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
// migrations/0010_seed_emoji_configs.up.sql 钦定的 ON DUPLICATE KEY UPDATE 语义在
// duplicate-code 路径下的 server 端表现：
// **当 UNIQUE KEY uk_code 命中时，ON DUPLICATE KEY UPDATE 不报错 + 不翻倍 +
// 强制把 name / asset_url / sort_order / is_enabled 4 字段覆盖回 0010 钦定值**。
//
// **背景（Story 17.3 引入；r1 [P2] 重写；r2 [P2] 调整注释语义；r3 [P1] 反转语义）**：
//   - 原 case（r1 前）走 Up → Down → Up 路径，但 Down 把整张表 DROP 掉 → 第二次 Up
//     跑空表 → 没真正测到 duplicate-code 路径 → 把 0010.up 从 INSERT IGNORE 改成普通
//     INSERT 也能通过。r1 重写改成 "预填 admin-flavored 行 → 回滚 schema_migrations
//     版本号 → 重跑 0010.up"，让 duplicate-code 路径真正被触发。
//   - r2 把 0010.down 从 r1 的 no-op 改回 narrow DELETE 4 行（migration invariant
//     优先于 admin 数据保留）；测试**注释语义**调整成"INSERT IGNORE 不覆盖现有值"。
//   - **r3 [P1 #1] 反转 up 语义**：INSERT IGNORE 让 admin 预存的"坏行"（is_enabled=0
//     / asset_url='' / sort_order 乱序）幸存 → Story 17.4/17.5/18.1 依赖的"4 个 enabled
//     emoji 配置 invariant"无法保证。0010.up 改成 `INSERT ... ON DUPLICATE KEY UPDATE`
//     强制覆盖 4 字段。本测试**结论反转**：从 r2 的"不覆盖现有值" → r3 的"强制覆盖现有值"。
//     测试 setup（预填 admin-flavored 行）**不变**，只是断言从"保留 admin 值" → "覆盖回
//     seed 值"。
//
// **覆盖路径（r3 反转后）**：
//  1. migrate Up 全程 (v=10) → emoji_configs 4 行（0010 seed 写入）
//  2. DELETE seed 4 行 + 手动 INSERT 4 行 admin-flavored 数据（asset_url / name
//     都和 seed 不同，模拟 admin 在 0010 owned codes 上做了"违规 customization"）
//  3. UPDATE schema_migrations SET version = 9 → 让 golang-migrate 认为 0010 还没跑过
//  4. migrate Up 重跑 → 触发 0010.up ON DUPLICATE KEY UPDATE 命中 uk_code 4 次
//  5. 断言：行数仍 4（不翻倍到 8）+ admin 字段值被**强制覆盖回 0010 seed 钦定值**
//     —— 这是 r3 决策"0010 owns these 4 codes"的 100% 强保证。
//
// **三种"伪幂等"实现仍被本测试抓到（结论反转后）**：
//   - INSERT INTO（无 IGNORE / ON DUPLICATE KEY UPDATE）→ 步骤 4 撞 uk_code 1062 直接报错
//   - INSERT IGNORE → 行数对但 admin 字段**保留不被覆盖** → 步骤 5b 断言炸
//     （断言期望"覆盖回 seed 值"，IGNORE 路径下还是 admin 值）
//   - REPLACE INTO → 行数对，name/asset_url/sort_order/is_enabled 也覆盖正确，但
//     REPLACE 是 DELETE+INSERT 实现，会触发外键级联 / id 重排 / 触发器副作用 →
//     虽然字段断言可能能过，但在 dockertest 里若 emoji_configs id 被外键引用时
//     语义会失效（17.2 schema 当前无外键引用，故 REPLACE 也能 pass —— 但 ON DUPLICATE
//     KEY UPDATE 更安全；这里不强测 REPLACE 一定挂，靠注释和 SQL review 兜底）
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
		code      string
		name      string
		assetURL  string
		sortOrder int
		isEnabled int
	}
	// **故意**与 0010.up.sql seed 值的全部 4 字段都不同（name / asset_url 改成不同字符串；
	// sort_order 用 90+ 模拟 admin 乱改排序；is_enabled 用 0 模拟 admin "下架"了这 4 个 emoji）。
	// 这是 r3 决策的核心测试场景 —— 模拟 admin 在 0010 owned codes 上做了"违规 customization"
	// （including is_enabled=0 和 asset_url 不变但 sort_order 乱序的"坏行"），验证 0010.up
	// ON DUPLICATE KEY UPDATE 路径会把这 4 字段**强制覆盖回** seed 钦定值。
	adminRows := []adminRow{
		{code: "wave", name: "挥手-admin", assetURL: "https://admin-cdn.example.com/wave.png", sortOrder: 91, isEnabled: 0},
		{code: "love", name: "爱心-admin", assetURL: "https://admin-cdn.example.com/love.png", sortOrder: 92, isEnabled: 0},
		{code: "laugh", name: "大笑-admin", assetURL: "https://admin-cdn.example.com/laugh.png", sortOrder: 93, isEnabled: 0},
		{code: "cry", name: "哭-admin", assetURL: "https://admin-cdn.example.com/cry.png", sortOrder: 94, isEnabled: 0},
	}
	for _, r := range adminRows {
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES (?, ?, ?, ?, ?)`,
			r.code, r.name, r.assetURL, r.sortOrder, r.isEnabled); err != nil {
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

	// 步骤 5a：行数仍恰好 4（不翻倍到 8 —— ON DUPLICATE KEY UPDATE 兜底不重复插入）
	var countAfterSecondUp int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry')`).Scan(&countAfterSecondUp); err != nil {
		t.Fatalf("SELECT COUNT after second Up: %v", err)
	}
	if countAfterSecondUp != 4 {
		t.Errorf("after second Up, emoji_configs seed rows = %d, want 4 (ON DUPLICATE KEY UPDATE 兜底不重复插入；不应翻倍到 8)", countAfterSecondUp)
	}

	// 步骤 5b（r3 反转）：每行 name / asset_url / sort_order / is_enabled **被强制
	// 覆盖回 0010 seed 钦定值**，**不是** admin 写入的值。
	// —— 真正验证 ON DUPLICATE KEY UPDATE 的"强制覆盖"语义；这是 r3 决策
	//   "wave/love/laugh/cry 这 4 个 code 由 0010 完全占用 / 强制覆盖"的 100% 强保证。
	// 0010.up.sql 钦定值（与 SQL 文件 1:1 对齐）：
	type seedRow struct {
		code      string
		name      string
		assetURL  string
		sortOrder int
		isEnabled int
	}
	seedRows := []seedRow{
		{code: "wave", name: "挥手", assetURL: "https://placehold.co/64x64?text=Wave", sortOrder: 1, isEnabled: 1},
		{code: "love", name: "爱心", assetURL: "https://placehold.co/64x64?text=Love", sortOrder: 2, isEnabled: 1},
		{code: "laugh", name: "大笑", assetURL: "https://placehold.co/64x64?text=Laugh", sortOrder: 3, isEnabled: 1},
		{code: "cry", name: "哭", assetURL: "https://placehold.co/64x64?text=Cry", sortOrder: 4, isEnabled: 1},
	}
	for _, want := range seedRows {
		var gotName, gotAssetURL string
		var gotSortOrder, gotIsEnabled int
		err := sqlDB.QueryRowContext(ctx,
			`SELECT name, asset_url, sort_order, is_enabled FROM emoji_configs WHERE code = ?`, want.code).
			Scan(&gotName, &gotAssetURL, &gotSortOrder, &gotIsEnabled)
		if err != nil {
			t.Errorf("SELECT row %q after second Up: %v", want.code, err)
			continue
		}
		if gotName != want.name {
			t.Errorf("emoji %q name = %q, want %q (ON DUPLICATE KEY UPDATE 应强制覆盖回 0010 seed 钦定值)", want.code, gotName, want.name)
		}
		if gotAssetURL != want.assetURL {
			t.Errorf("emoji %q asset_url = %q, want %q (ON DUPLICATE KEY UPDATE 应强制覆盖回 0010 seed 钦定值)", want.code, gotAssetURL, want.assetURL)
		}
		if gotSortOrder != want.sortOrder {
			t.Errorf("emoji %q sort_order = %d, want %d (ON DUPLICATE KEY UPDATE 应强制覆盖回 0010 seed 钦定值)", want.code, gotSortOrder, want.sortOrder)
		}
		if gotIsEnabled != want.isEnabled {
			t.Errorf("emoji %q is_enabled = %d, want %d (ON DUPLICATE KEY UPDATE 应强制覆盖回 0010 seed 钦定值)", want.code, gotIsEnabled, want.isEnabled)
		}
	}
}

// TestMigrateIntegration_CosmeticItems_Schema 验证
// migrations/0011_init_cosmetic_items.up.sql 钦定的 cosmetic_items 表 schema
// 与数据库设计.md §5.8 + V1接口设计.md §7.2 + §8.1 一致：
//
//   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - code VARCHAR(64) NOT NULL + UNIQUE KEY uk_code (code)
//   - name VARCHAR(64) NOT NULL
//   - slot TINYINT NOT NULL
//   - rarity TINYINT NOT NULL
//   - asset_url VARCHAR(255) NOT NULL DEFAULT ''
//   - icon_url VARCHAR(255) NOT NULL DEFAULT ''
//   - drop_weight INT UNSIGNED NOT NULL DEFAULT 0
//   - is_enabled TINYINT NOT NULL DEFAULT 1
//   - created_at / updated_at DATETIME(3)
//   - KEY idx_slot_rarity (slot, rarity)
//   - KEY idx_enabled_weight (is_enabled, drop_weight)
//
// **关键覆盖点**：
//   - INT UNSIGNED（drop_weight）column_type 必须含 "unsigned"（与 INT 区别）；
//     这是本 case 区别于 17.2 EmojiConfigs_Schema 的关键之处 —— emoji_configs.sort_order
//     是 INT (signed)，cosmetic_items.drop_weight 是 INT UNSIGNED；
//     INFORMATION_SCHEMA.COLUMNS.COLUMN_TYPE 字段会精确反映 "int unsigned" vs "int"
//   - 双索引（idx_slot_rarity + idx_enabled_weight）列顺序断言（与 §5.8 钦定一致）
//   - **不含** render_config 字段（节点 10 / Epic 29 才加；本 case 用 11 字段计数兜底
//     防漂移 —— 如有人误加 render_config 字段会让计数变 12 失败）
//
// **背景（Story 20.2 引入）**：本 case 验证 0011 migration 落地的 schema
// 与 §5.8 钦定 1:1 对齐；用于在 epics.md §Story 20.2 钦定的"单元测试覆盖 ≥3 case"
// 中作为 schema-correctness 路径（happy / migrate up 后表存在 + 字段类型 +
// 全部索引和约束都符合 §5.8）。
func TestMigrateIntegration_CosmeticItems_Schema(t *testing.T) {
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

	// 1. INFORMATION_SCHEMA.TABLES：cosmetic_items 表存在
	var tableCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items'`).Scan(&tableCount)
	if err != nil {
		t.Errorf("query cosmetic_items table existence: %v", err)
	} else if tableCount != 1 {
		t.Errorf("cosmetic_items table count = %d, want 1", tableCount)
	}

	// 2. INFORMATION_SCHEMA.COLUMNS：11 列存在 + 类型对齐
	//    **关键**：drop_weight 是 INT UNSIGNED → column_type 必须 = "int unsigned"
	//    （与 emoji_configs.sort_order 的 "int" 区别）。
	cosmeticCols := []struct {
		col          string
		wantDataType string
		wantColType  string
	}{
		{"id", "bigint", "bigint unsigned"},
		{"code", "varchar", "varchar(64)"},
		{"name", "varchar", "varchar(64)"},
		{"slot", "tinyint", "tinyint"},
		{"rarity", "tinyint", "tinyint"},
		{"asset_url", "varchar", "varchar(255)"},
		{"icon_url", "varchar", "varchar(255)"},
		{"drop_weight", "int", "int unsigned"},
		{"is_enabled", "tinyint", "tinyint"},
		{"created_at", "datetime", "datetime(3)"},
		{"updated_at", "datetime", "datetime(3)"},
	}
	for _, c := range cosmeticCols {
		var dt, ct string
		err := sqlDB.QueryRowContext(ctx, `
			SELECT data_type, column_type FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items' AND column_name = ?`,
			c.col).Scan(&dt, &ct)
		if err != nil {
			t.Errorf("query cosmetic_items.%s type: %v", c.col, err)
			continue
		}
		if dt != c.wantDataType {
			t.Errorf("cosmetic_items.%s data_type = %q, want %q", c.col, dt, c.wantDataType)
		}
		if ct != c.wantColType {
			t.Errorf("cosmetic_items.%s column_type = %q, want %q", c.col, ct, c.wantColType)
		}
	}

	// 3. INFORMATION_SCHEMA.COLUMNS：cosmetic_items 表总列数 == 11
	//    兜底防有人误加 render_config 或其他字段；render_config 由 Epic 29 落地。
	var colCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items'`).Scan(&colCount)
	if err != nil {
		t.Errorf("query cosmetic_items total column count: %v", err)
	} else if colCount != 11 {
		t.Errorf("cosmetic_items total column count = %d, want 11 (render_config 不在本 story 范围；如计数 = 12 说明有人误加 render_config 字段)", colCount)
	}

	// 4a. cosmetic_items.id PK = id
	var pkCol string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items' AND constraint_name = 'PRIMARY'`).Scan(&pkCol)
	if err != nil {
		t.Errorf("query cosmetic_items PK: %v", err)
	} else if pkCol != "id" {
		t.Errorf("cosmetic_items PK column = %q, want 'id'", pkCol)
	}

	// 4b. UNIQUE KEY uk_code (code) 存在 + non_unique = 0
	var ukCount int
	var ukNonUnique int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(non_unique) FROM information_schema.statistics
		WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items' AND index_name = 'uk_code'`).Scan(&ukCount, &ukNonUnique)
	if err != nil {
		t.Errorf("query cosmetic_items.uk_code: %v", err)
	} else {
		if ukCount == 0 {
			t.Errorf("cosmetic_items.uk_code: index not found")
		}
		if ukNonUnique != 0 {
			t.Errorf("cosmetic_items.uk_code: non_unique = %d, want 0 (UNIQUE)", ukNonUnique)
		}
	}

	// uk_code 单列 (code)
	var ukCol string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.statistics
		WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items' AND index_name = 'uk_code'
		ORDER BY seq_in_index ASC LIMIT 1`).Scan(&ukCol)
	if err != nil {
		t.Errorf("query cosmetic_items.uk_code column: %v", err)
	} else if ukCol != "code" {
		t.Errorf("cosmetic_items.uk_code column = %q, want 'code'", ukCol)
	}

	// 4c. KEY idx_slot_rarity 存在 + 列顺序 (slot, rarity)
	idxCases := []struct {
		idxName  string
		wantCols []string
	}{
		{"idx_slot_rarity", []string{"slot", "rarity"}},
		{"idx_enabled_weight", []string{"is_enabled", "drop_weight"}},
	}
	for _, ic := range idxCases {
		rows, err := sqlDB.QueryContext(ctx, `
			SELECT column_name FROM information_schema.statistics
			WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items' AND index_name = ?
			ORDER BY seq_in_index ASC`, ic.idxName)
		if err != nil {
			t.Errorf("query cosmetic_items.%s columns: %v", ic.idxName, err)
			continue
		}
		var cols []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Errorf("scan %s column: %v", ic.idxName, err)
				continue
			}
			cols = append(cols, c)
		}
		rows.Close()
		if len(cols) != len(ic.wantCols) {
			t.Errorf("cosmetic_items.%s column count = %d, want %d (cols=%v)", ic.idxName, len(cols), len(ic.wantCols), cols)
			continue
		}
		for i, w := range ic.wantCols {
			if cols[i] != w {
				t.Errorf("cosmetic_items.%s column[%d] = %q, want %q", ic.idxName, i, cols[i], w)
			}
		}
	}

	// 5. INFORMATION_SCHEMA.COLUMNS.COLUMN_DEFAULT 默认值校验
	defaultCases := []struct {
		col         string
		wantDefault string
	}{
		{"asset_url", ""},
		{"icon_url", ""},
		{"drop_weight", "0"},
		{"is_enabled", "1"},
	}
	for _, c := range defaultCases {
		var def sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items' AND column_name = ?`,
			c.col).Scan(&def)
		if err != nil {
			t.Errorf("query cosmetic_items.%s default: %v", c.col, err)
			continue
		}
		if !def.Valid || def.String != c.wantDefault {
			t.Errorf("cosmetic_items.%s default = %v, want %q", c.col, def, c.wantDefault)
		}
	}

	// created_at / updated_at 默认值 = CURRENT_TIMESTAMP(3)
	for _, col := range []string{"created_at", "updated_at"} {
		var def sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'cosmetic_items' AND column_name = ?`,
			col).Scan(&def)
		if err != nil {
			t.Errorf("query cosmetic_items.%s default: %v", col, err)
			continue
		}
		if !def.Valid || !strings.Contains(strings.ToUpper(def.String), "CURRENT_TIMESTAMP") {
			t.Errorf("cosmetic_items.%s default = %v, want substring 'CURRENT_TIMESTAMP'", col, def)
		}
	}
}

// TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected 验证
// migrations/0011_init_cosmetic_items.up.sql 钦定的 UNIQUE KEY uk_code (code)
// 在运行时被 MySQL 真实拒绝重复 code 插入。
//
// **背景（Story 20.2 引入）**：epics.md §Story 20.2 钦定的"集成测试覆盖（dockertest）：
// migrate up → SHOW CREATE TABLE 对比 schema → migrate down"路径在 CosmeticItems_Schema
// case 用 INFORMATION_SCHEMA 字段层精确断言取代了不稳定的 SHOW CREATE TABLE 字符串比对；
// 本 case 是额外的运行时 UNIQUE 拒绝验证 —— 是 Story 20.3 seed 用 INSERT IGNORE
// 兜底 + Story 20.6 加权抽取按 code 索引命中 + admin 后台未来写入路径的 schema 层根基。
//
// **覆盖路径**：
//  1. migrate up → cosmetic_items 表存在
//  2. 插入 cosmetic_items (code='test_unique_cosmetic_a', name='TestA', slot=1, rarity=1,
//     asset_url='https://example.com/test_a.png',
//     icon_url='https://example.com/test_a_icon.png', drop_weight=100, is_enabled=1) → 成功
//  3. 再次插入 cosmetic_items (code='test_unique_cosmetic_a', ...) → DB 拒绝
//     （UNIQUE KEY uk_code (code) 兜底；same code 不能插两次）；
//     err 必须含 "Duplicate entry"（MySQL 错误码 = 1062 ER_DUP_ENTRY）
//
// 用 database/sql 直跑 raw INSERT（**不**走 GORM）让测试结果**不**依赖 ORM
// 行为差异（与 Story 11.2 落地的
// TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected + Story 17.2 落地的
// TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected 同模式）。
//
// **测试专用 code 隔离**：用 `test_unique_cosmetic_a` 与未来 Story 20.3 seed 的
// `hat_yellow / scarf_star / ...` 等业务字面量完全隔离；本 story 阶段 cosmetic_items
// 表无 seed（20.3 owner），但提前用测试专用 code 防 20.3 seed 落地后本 case 第一次
// INSERT 因 seed 已存在而失败（与 Story 17.3 解耦 wave seed 同模式）。
func TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected(t *testing.T) {
	// Story 20.3 注释：本 case 用测试专用 code (test_unique_cosmetic_a) 与 0012 seed
	// 的 15 个 owned codes 字面量隔离（hat_yellow / hat_red / gloves_white /
	// gloves_brown / glasses_round / neck_blue / back_bag / tail_ribbon /
	// hat_chef / glasses_star / neck_scarf_star / body_tshirt / hat_crown /
	// back_wings / body_armor），避免 0012 seed 先写入后该 case 第一次 INSERT 就
	// 触发 UNIQUE 冲突（与 17.3 解耦 0010 emoji seed 同模式 —— 20.2 已经吸取教训
	// 用了测试专用 code，所以 20.3 不需要再改实际 SQL 字面量，仅在此追加注释说明）。
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

	// 步骤 2：首条 cosmetic_items (code='test_unique_cosmetic_a', ...) 必须成功
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO cosmetic_items (code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled) VALUES ('test_unique_cosmetic_a', 'TestA', 1, 1, 'https://example.com/test_a.png', 'https://example.com/test_a_icon.png', 100, 1)`); err != nil {
		t.Fatalf("first insert cosmetic_items (code='test_unique_cosmetic_a') should succeed: %v", err)
	}

	// 步骤 3：UNIQUE(code) 约束 —— 同 code 再插一次必须被拒
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO cosmetic_items (code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled) VALUES ('test_unique_cosmetic_a', 'TestA v2', 2, 2, 'https://example.com/test_a_v2.png', 'https://example.com/test_a_v2_icon.png', 50, 1)`)
	if err == nil {
		t.Fatalf("expected duplicate-entry error on second insert (code='test_unique_cosmetic_a') violating UNIQUE(code), got nil")
	}
	if !strings.Contains(err.Error(), "Duplicate entry") {
		t.Errorf("UNIQUE(code) rejection error message = %q, want substring \"Duplicate entry\"", err.Error())
	}
}

// TestMigrateIntegration_CosmeticItems_SeedContent 验证
// migrations/0012_seed_cosmetic_items.up.sql 钦定的 15 个装扮 seed 在 migrate up
// 后真实写入 cosmetic_items 表，且每行字段值符合 epics.md §Story 20.3 + AR18 +
// V1 §7.2 reward 字段约束：
//
//   - 至少 15 行存在（实际 0012 钦定 15 行；用 >= 15 而非 == 15 兼容未来 0013+
//     新 migration 加 cosmetic）
//   - 各 rarity 数量符合 AR18：
//       rarity=1(common)    ≥ 8
//       rarity=2(rare)      ≥ 4
//       rarity=3(epic)      ≥ 2
//       rarity=4(legendary) ≥ 1
//   - common 至少覆盖 4 个不同 slot（AR18 钦定 + epics.md §Story 20.3 行 2839）
//   - 每行 asset_url 非空（V1 §7.2 reward.assetUrl 钦定 length ≥ 1 + 禁止 ""）
//   - 每行 icon_url 非空（V1 §7.2 reward.iconUrl 钦定 length ≥ 1 + 禁止 ""）
//   - 每行 is_enabled = 1（enabled 才会被 GET /cosmetics/catalog 返回 + 加权抽取命中）
//   - 每行 name 非空（VARCHAR(64) NOT NULL）
//   - 每行 slot ∈ {1,2,3,4,5,6,7,99}（§6.8 枚举值；本 case 用 set 校验）
//   - 每行 rarity ∈ {1,2,3,4}（§6.9 枚举值；本 case 用 set 校验）
//   - 各 rarity 的 drop_weight 按品质递减分布（common > rare > epic > legendary；
//     0012 钦定 common=100 / rare=20 / epic=4 / legendary=1）
//
// **背景（Story 20.3 引入）**：epics.md §Story 20.3 钦定的"集成测试覆盖（dockertest）：
// migrate up → SELECT count(*) GROUP BY rarity → 验证各品质数量 ≥ AR18 约束"
// 路径在本 case 落地；用于 Story 20.4 ~ 20.9 / iOS Epic 21 / Epic 19.1 节点 7
// demo E2E / Epic 23 / Epic 29 Story 29.3 / Epic 32 / 33 实装时验证 seed 数据
// 真实在位 + AR18 数量约束 100% 强保证的根基。
//
// 用 database/sql 直跑 SELECT（**不**走 GORM）让测试结果**不**依赖 ORM 行为差异
// （与 17.3 / 11.2 / 17.2 落地的 dockertest case 同模式）。
func TestMigrateIntegration_CosmeticItems_SeedContent(t *testing.T) {
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

	// 钦定的 15 个 code（与 0012_seed_cosmetic_items.up.sql 1:1 对齐）
	wantCodes := []string{
		"hat_yellow", "hat_red", "gloves_white", "gloves_brown",
		"glasses_round", "neck_blue", "back_bag", "tail_ribbon",
		"hat_chef", "glasses_star", "neck_scarf_star", "body_tshirt",
		"hat_crown", "back_wings", "body_armor",
	}

	// 1. SELECT 全表（按 rarity / slot / code 升序），>= 15 行
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled
		FROM cosmetic_items
		ORDER BY rarity ASC, slot ASC, code ASC`)
	if err != nil {
		t.Fatalf("SELECT cosmetic_items: %v", err)
	}
	defer rows.Close()

	type rowData struct {
		code       string
		name       string
		slot       int
		rarity     int
		assetURL   string
		iconURL    string
		dropWeight int
		isEnabled  int
	}
	var allRows []rowData
	seenCodes := make(map[string]rowData)
	for rows.Next() {
		var r rowData
		if err := rows.Scan(&r.code, &r.name, &r.slot, &r.rarity, &r.assetURL, &r.iconURL, &r.dropWeight, &r.isEnabled); err != nil {
			t.Errorf("scan row: %v", err)
			continue
		}
		allRows = append(allRows, r)
		seenCodes[r.code] = r
	}
	if err := rows.Err(); err != nil {
		t.Errorf("rows.Err: %v", err)
	}

	if len(allRows) < 15 {
		t.Errorf("cosmetic_items row count = %d, want >= 15 (Story 20.3 seed 钦定 15 个装扮)", len(allRows))
	}

	// 2. 各 rarity 数量符合 AR18 最小约束（GROUP BY rarity）
	rarityCount := map[int]int{}
	for _, r := range allRows {
		rarityCount[r.rarity]++
	}
	if rarityCount[1] < 8 {
		t.Errorf("rarity=1 (common) count = %d, want >= 8 (AR18)", rarityCount[1])
	}
	if rarityCount[2] < 4 {
		t.Errorf("rarity=2 (rare) count = %d, want >= 4 (AR18)", rarityCount[2])
	}
	if rarityCount[3] < 2 {
		t.Errorf("rarity=3 (epic) count = %d, want >= 2 (AR18)", rarityCount[3])
	}
	if rarityCount[4] < 1 {
		t.Errorf("rarity=4 (legendary) count = %d, want >= 1 (AR18)", rarityCount[4])
	}

	// 3. common（rarity=1）的 slot 至少覆盖 4 个不同值（AR18 行 184 钦定 +
	//    epics.md §Story 20.3 行 2839 钦定）
	commonSlotSet := map[int]struct{}{}
	for _, r := range allRows {
		if r.rarity == 1 {
			commonSlotSet[r.slot] = struct{}{}
		}
	}
	if len(commonSlotSet) < 4 {
		t.Errorf("common (rarity=1) distinct slot count = %d, want >= 4 (AR18 + epics.md §Story 20.3 行 2839)", len(commonSlotSet))
	}

	// 4. 钦定 15 个 code 必须全部存在 + 每行字段值符合约束
	validSlots := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 99: true}
	validRarities := map[int]bool{1: true, 2: true, 3: true, 4: true}
	for _, code := range wantCodes {
		r, ok := seenCodes[code]
		if !ok {
			t.Errorf("seed missing code = %q (Story 20.3 钦定 15 个 code 都必须存在)", code)
			continue
		}
		// a. name 非空
		if len(r.name) == 0 {
			t.Errorf("seed code=%q name 为空 (VARCHAR(64) NOT NULL + seed 钦定非空)", code)
		}
		// b. asset_url 非空（V1 §7.2 reward.assetUrl + AR18 钦定）
		if len(r.assetURL) == 0 {
			t.Errorf("seed code=%q asset_url 为空 (V1 §7.2 reward.assetUrl + AR18 钦定 enabled cosmetic asset_url 必须非空)", code)
		}
		// c. icon_url 非空（V1 §7.2 reward.iconUrl + AR18 钦定）
		if len(r.iconURL) == 0 {
			t.Errorf("seed code=%q icon_url 为空 (V1 §7.2 reward.iconUrl + AR18 钦定 enabled cosmetic icon_url 必须非空)", code)
		}
		// d. is_enabled == 1
		if r.isEnabled != 1 {
			t.Errorf("seed code=%q is_enabled = %d, want 1 (enabled 才能被 GET /cosmetics/catalog 返回 + 加权抽取命中)", code, r.isEnabled)
		}
		// e. slot ∈ {1,2,3,4,5,6,7,99}（§6.8）
		if !validSlots[r.slot] {
			t.Errorf("seed code=%q slot = %d, want ∈ {1,2,3,4,5,6,7,99} (§6.8 枚举值)", code, r.slot)
		}
		// f. rarity ∈ {1,2,3,4}（§6.9）
		if !validRarities[r.rarity] {
			t.Errorf("seed code=%q rarity = %d, want ∈ {1,2,3,4} (§6.9 枚举值)", code, r.rarity)
		}
	}

	// 5. 各 rarity 的 drop_weight 按品质递减分布（epics.md §Story 20.3 行 2844
	//    钦定 common=100 > rare=20 > epic=4 > legendary=1）：
	//    用 GROUP BY rarity + MIN(drop_weight) / MAX(drop_weight) 断言：
	//      a. common 的 MIN(drop_weight) > rare 的 MAX(drop_weight)（100 > 20）
	//      b. rare 的 MIN(drop_weight) > epic 的 MAX(drop_weight)（20 > 4）
	//      c. epic 的 MIN(drop_weight) > legendary 的 MAX(drop_weight)（4 > 1）
	weightRows, err := sqlDB.QueryContext(ctx, `
		SELECT rarity, MIN(drop_weight) AS min_w, MAX(drop_weight) AS max_w
		FROM cosmetic_items
		WHERE code IN ('hat_yellow', 'hat_red', 'gloves_white', 'gloves_brown',
		               'glasses_round', 'neck_blue', 'back_bag', 'tail_ribbon',
		               'hat_chef', 'glasses_star', 'neck_scarf_star', 'body_tshirt',
		               'hat_crown', 'back_wings', 'body_armor')
		GROUP BY rarity`)
	if err != nil {
		t.Fatalf("SELECT GROUP BY rarity drop_weight: %v", err)
	}
	defer weightRows.Close()

	minByRarity := map[int]int{}
	maxByRarity := map[int]int{}
	for weightRows.Next() {
		var rarity, minW, maxW int
		if err := weightRows.Scan(&rarity, &minW, &maxW); err != nil {
			t.Errorf("scan rarity weight row: %v", err)
			continue
		}
		minByRarity[rarity] = minW
		maxByRarity[rarity] = maxW
	}
	if err := weightRows.Err(); err != nil {
		t.Errorf("weightRows.Err: %v", err)
	}

	if minByRarity[1] <= maxByRarity[2] {
		t.Errorf("common MIN(drop_weight) = %d not > rare MAX(drop_weight) = %d (epics.md §Story 20.3 行 2844 钦定 common=100 > rare=20)", minByRarity[1], maxByRarity[2])
	}
	if minByRarity[2] <= maxByRarity[3] {
		t.Errorf("rare MIN(drop_weight) = %d not > epic MAX(drop_weight) = %d (epics.md §Story 20.3 行 2844 钦定 rare=20 > epic=4)", minByRarity[2], maxByRarity[3])
	}
	if minByRarity[3] <= maxByRarity[4] {
		t.Errorf("epic MIN(drop_weight) = %d not > legendary MAX(drop_weight) = %d (epics.md §Story 20.3 行 2844 钦定 epic=4 > legendary=1)", minByRarity[3], maxByRarity[4])
	}
}

// TestMigrateIntegration_CosmeticItems_SeedForceOverwrite 验证
// migrations/0012_seed_cosmetic_items.up.sql 钦定的 ON DUPLICATE KEY UPDATE 语义在
// duplicate-code 路径下的 server 端表现：
// **当 UNIQUE KEY uk_code 命中时，ON DUPLICATE KEY UPDATE 不报错 + 不翻倍 +
// 强制把 name / slot / rarity / asset_url / icon_url / drop_weight / is_enabled
// 7 字段覆盖回 0012 钦定值**。
//
// **背景（Story 20.3 引入；r0 直接复用 17-3 r3 决断）**：
// epics.md §Story 20.3 钦定的"重复 migrate up → 不重复插入"+ 17-3 r3 lesson
// "0010 owns 4 codes / up 强制覆盖" 路径在本 case 落地，与
// TestMigrateIntegration_EmojiConfigs_SeedIdempotent 同模式（行 1130 ~ 1303）。
//
// **覆盖路径**：
//  1. migrate Up 全程 (v=12) → cosmetic_items 15 行（0012 seed 写入）
//  2. DELETE seed 15 行 + 手动 INSERT 15 行 admin-flavored 数据（name / slot /
//     rarity / asset_url / icon_url / drop_weight / is_enabled 7 字段都和 seed
//     不同，模拟 admin 在 0012 owned codes 上做了"违规 customization"，包括
//     is_enabled=0 把某 cosmetic 临时下架 / asset_url='' 让奖励弹窗破图 /
//     drop_weight=0 让某 cosmetic 永远抽不到）
//  3. UPDATE schema_migrations SET version = 11 → 让 golang-migrate 认为 0012 还没跑过
//  4. migrate Up 重跑 → 触发 0012.up ON DUPLICATE KEY UPDATE 命中 uk_code 15 次
//  5. 断言：
//     a. 行数仍 15（不翻倍到 30）
//     b. 7 字段被**强制覆盖回** 0012 seed 钦定值（hat_yellow 的 name 恢复为
//        "小黄帽" / slot 恢复为 1 / rarity 恢复为 1 / asset_url 恢复为
//        "https://placehold.co/128x128?text=Hat-Yellow" / icon_url 恢复为
//        "https://placehold.co/64x64?text=Hat-Yellow" / drop_weight 恢复为 100 /
//        is_enabled 恢复为 1；其他 14 行同理逐字段抽样验证）
//     这是 r0 决策"0012 owns these 15 codes"的 100% 强保证。
//
// **三种"伪幂等"实现仍被本测试抓到**：
//   - INSERT INTO（无 IGNORE / ON DUPLICATE KEY UPDATE）→ 步骤 4 撞 uk_code 1062 直接报错
//   - INSERT IGNORE → 行数对但 admin 7 字段**保留不被覆盖** → 步骤 5b 断言炸
//     （断言期望"覆盖回 seed 值"，IGNORE 路径下还是 admin 值）
//   - REPLACE INTO → 行数对、字段覆盖正确，但 REPLACE 触发 id 重排会让
//     chest_open_logs.reward_cosmetic_item_id 历史日志断开 reference 语义；
//     虽然字段断言可能能过，但语义不对（这里靠 SQL review 兜底，本 case 不强测）
//
// **为什么不走 force(11)**：本 migrate 包没暴露 Force API（migrate.go 仅 Up / Down
// / Status / Close）。直接 UPDATE schema_migrations 是 dockertest 集成测试可控
// 范围内的最小操作；也忠实模拟了 ops 在生产里手工修复 dirty 后的回退场景；
// 与 17.3 落地的 TestMigrateIntegration_EmojiConfigs_SeedIdempotent 同模式。
//
// 用 database/sql 直跑 SQL（**不**走 GORM）。
func TestMigrateIntegration_CosmeticItems_SeedForceOverwrite(t *testing.T) {
	dsn, cleanup := startMySQL(t)
	defer cleanup()

	mig, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer mig.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 步骤 1：第一次 migrate up → cosmetic_items 15 行（0012 seed 写入）
	if err := mig.Up(ctx); err != nil {
		t.Fatalf("first migrate Up: %v", err)
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()

	const seedCodesINClause = `'hat_yellow', 'hat_red', 'gloves_white', 'gloves_brown',
		'glasses_round', 'neck_blue', 'back_bag', 'tail_ribbon',
		'hat_chef', 'glasses_star', 'neck_scarf_star', 'body_tshirt',
		'hat_crown', 'back_wings', 'body_armor'`

	var countAfterFirstUp int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cosmetic_items WHERE code IN (`+seedCodesINClause+`)`).Scan(&countAfterFirstUp); err != nil {
		t.Fatalf("SELECT COUNT after first Up: %v", err)
	}
	if countAfterFirstUp != 15 {
		t.Fatalf("after first Up, cosmetic_items seed rows = %d, want 15 (Story 20.3 钦定 15 个装扮)", countAfterFirstUp)
	}

	// 步骤 2：DELETE seed 15 行 + 手动 INSERT 15 行 admin-flavored 数据（模拟 admin
	// 在上线后调整过 hat_yellow 等的 name / slot / rarity / asset_url / icon_url /
	// drop_weight / is_enabled 7 字段）
	if _, err := sqlDB.ExecContext(ctx,
		`DELETE FROM cosmetic_items WHERE code IN (`+seedCodesINClause+`)`); err != nil {
		t.Fatalf("DELETE seed rows: %v", err)
	}

	type adminRow struct {
		code       string
		name       string
		slot       int
		rarity     int
		assetURL   string
		iconURL    string
		dropWeight int
		isEnabled  int
	}
	// **故意**与 0012.up.sql seed 值的全部 7 字段都不同（name 加 -admin 后缀 /
	// slot 全用 99=other / rarity 都改成不同值 / asset_url / icon_url 用 admin-cdn /
	// drop_weight 全部为 0（让该 cosmetic 永远抽不到，模拟 admin "禁用"）/
	// is_enabled=0（模拟 admin 下架）。这是 r0 决策的核心测试场景 —— 模拟 admin 在 0012
	// owned codes 上做了"违规 customization"，验证 0012.up ON DUPLICATE KEY UPDATE
	// 路径会把这 7 字段**强制覆盖回** seed 钦定值。
	adminRows := []adminRow{
		{code: "hat_yellow", name: "小黄帽-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/hat_yellow.png", iconURL: "https://admin-cdn.example.com/hat_yellow_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "hat_red", name: "小红帽-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/hat_red.png", iconURL: "https://admin-cdn.example.com/hat_red_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "gloves_white", name: "白手套-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/gloves_white.png", iconURL: "https://admin-cdn.example.com/gloves_white_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "gloves_brown", name: "棕手套-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/gloves_brown.png", iconURL: "https://admin-cdn.example.com/gloves_brown_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "glasses_round", name: "圆框眼镜-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/glasses_round.png", iconURL: "https://admin-cdn.example.com/glasses_round_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "neck_blue", name: "蓝围脖-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/neck_blue.png", iconURL: "https://admin-cdn.example.com/neck_blue_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "back_bag", name: "小书包-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/back_bag.png", iconURL: "https://admin-cdn.example.com/back_bag_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "tail_ribbon", name: "蝴蝶结尾巾-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/tail_ribbon.png", iconURL: "https://admin-cdn.example.com/tail_ribbon_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "hat_chef", name: "厨师帽-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/hat_chef.png", iconURL: "https://admin-cdn.example.com/hat_chef_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "glasses_star", name: "星星眼镜-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/glasses_star.png", iconURL: "https://admin-cdn.example.com/glasses_star_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "neck_scarf_star", name: "星星围巾-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/neck_scarf_star.png", iconURL: "https://admin-cdn.example.com/neck_scarf_star_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "body_tshirt", name: "白T恤-admin", slot: 99, rarity: 4, assetURL: "https://admin-cdn.example.com/body_tshirt.png", iconURL: "https://admin-cdn.example.com/body_tshirt_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "hat_crown", name: "金王冠-admin", slot: 99, rarity: 1, assetURL: "https://admin-cdn.example.com/hat_crown.png", iconURL: "https://admin-cdn.example.com/hat_crown_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "back_wings", name: "天使翅膀-admin", slot: 99, rarity: 1, assetURL: "https://admin-cdn.example.com/back_wings.png", iconURL: "https://admin-cdn.example.com/back_wings_icon.png", dropWeight: 0, isEnabled: 0},
		{code: "body_armor", name: "黄金圣衣-admin", slot: 99, rarity: 1, assetURL: "https://admin-cdn.example.com/body_armor.png", iconURL: "https://admin-cdn.example.com/body_armor_icon.png", dropWeight: 0, isEnabled: 0},
	}
	for _, r := range adminRows {
		if _, err := sqlDB.ExecContext(ctx,
			`INSERT INTO cosmetic_items (code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			r.code, r.name, r.slot, r.rarity, r.assetURL, r.iconURL, r.dropWeight, r.isEnabled); err != nil {
			t.Fatalf("INSERT admin row %q: %v", r.code, err)
		}
	}

	// 步骤 3：回滚 schema_migrations 版本号到 11，让 golang-migrate 认为 0012 还没跑过；
	// 这是触发 "duplicate-code 路径下重跑 0012.up" 的关键。
	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE schema_migrations SET version = 11, dirty = 0 WHERE 1=1`); err != nil {
		t.Fatalf("UPDATE schema_migrations to v=11: %v", err)
	}

	// 重开一个 Migrator 实例（golang-migrate 内部 cache 版本号）
	if err := mig.Close(); err != nil {
		t.Fatalf("close first migrator: %v", err)
	}
	mig2, err := migrate.New(dsn, migrationsPath(t))
	if err != nil {
		t.Fatalf("migrate.New (after rollback to v=11): %v", err)
	}
	defer mig2.Close()

	// 步骤 4：再跑一次 migrate Up → 0012.up 会被重跑 → ON DUPLICATE KEY UPDATE 命中 uk_code
	if err := mig2.Up(ctx); err != nil {
		t.Fatalf("second migrate Up (after schema_migrations rollback): %v", err)
	}

	// 步骤 5a：行数仍恰好 15（不翻倍到 30 —— ON DUPLICATE KEY UPDATE 兜底不重复插入）
	var countAfterSecondUp int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cosmetic_items WHERE code IN (`+seedCodesINClause+`)`).Scan(&countAfterSecondUp); err != nil {
		t.Fatalf("SELECT COUNT after second Up: %v", err)
	}
	if countAfterSecondUp != 15 {
		t.Errorf("after second Up, cosmetic_items seed rows = %d, want 15 (ON DUPLICATE KEY UPDATE 兜底不重复插入；不应翻倍到 30)", countAfterSecondUp)
	}

	// 步骤 5b：每行 name / slot / rarity / asset_url / icon_url / drop_weight /
	// is_enabled 7 字段**被强制覆盖回 0012 seed 钦定值**，**不是** admin 写入的值。
	// —— 真正验证 ON DUPLICATE KEY UPDATE 的"强制覆盖"语义；这是 r0 决策
	//   "0012 owns these 15 codes"的 100% 强保证（与 17-3 r3 同模式）。
	// 0012.up.sql 钦定值（与 SQL 文件 1:1 对齐）：
	type seedRow struct {
		code       string
		name       string
		slot       int
		rarity     int
		assetURL   string
		iconURL    string
		dropWeight int
		isEnabled  int
	}
	seedRows := []seedRow{
		{code: "hat_yellow", name: "小黄帽", slot: 1, rarity: 1, assetURL: "https://placehold.co/128x128?text=Hat-Yellow", iconURL: "https://placehold.co/64x64?text=Hat-Yellow", dropWeight: 100, isEnabled: 1},
		{code: "hat_red", name: "小红帽", slot: 1, rarity: 1, assetURL: "https://placehold.co/128x128?text=Hat-Red", iconURL: "https://placehold.co/64x64?text=Hat-Red", dropWeight: 100, isEnabled: 1},
		{code: "gloves_white", name: "白手套", slot: 2, rarity: 1, assetURL: "https://placehold.co/128x128?text=Gloves-White", iconURL: "https://placehold.co/64x64?text=Gloves-White", dropWeight: 100, isEnabled: 1},
		{code: "gloves_brown", name: "棕手套", slot: 2, rarity: 1, assetURL: "https://placehold.co/128x128?text=Gloves-Brown", iconURL: "https://placehold.co/64x64?text=Gloves-Brown", dropWeight: 100, isEnabled: 1},
		{code: "glasses_round", name: "圆框眼镜", slot: 3, rarity: 1, assetURL: "https://placehold.co/128x128?text=Glasses-Round", iconURL: "https://placehold.co/64x64?text=Glasses-Round", dropWeight: 100, isEnabled: 1},
		{code: "neck_blue", name: "蓝围脖", slot: 4, rarity: 1, assetURL: "https://placehold.co/128x128?text=Neck-Blue", iconURL: "https://placehold.co/64x64?text=Neck-Blue", dropWeight: 100, isEnabled: 1},
		{code: "back_bag", name: "小书包", slot: 5, rarity: 1, assetURL: "https://placehold.co/128x128?text=Back-Bag", iconURL: "https://placehold.co/64x64?text=Back-Bag", dropWeight: 100, isEnabled: 1},
		{code: "tail_ribbon", name: "蝴蝶结尾巾", slot: 7, rarity: 1, assetURL: "https://placehold.co/128x128?text=Tail-Ribbon", iconURL: "https://placehold.co/64x64?text=Tail-Ribbon", dropWeight: 100, isEnabled: 1},
		{code: "hat_chef", name: "厨师帽", slot: 1, rarity: 2, assetURL: "https://placehold.co/128x128?text=Hat-Chef", iconURL: "https://placehold.co/64x64?text=Hat-Chef", dropWeight: 20, isEnabled: 1},
		{code: "glasses_star", name: "星星眼镜", slot: 3, rarity: 2, assetURL: "https://placehold.co/128x128?text=Glasses-Star", iconURL: "https://placehold.co/64x64?text=Glasses-Star", dropWeight: 20, isEnabled: 1},
		{code: "neck_scarf_star", name: "星星围巾", slot: 4, rarity: 2, assetURL: "https://placehold.co/128x128?text=Neck-Scarf-Star", iconURL: "https://placehold.co/64x64?text=Neck-Scarf-Star", dropWeight: 20, isEnabled: 1},
		{code: "body_tshirt", name: "白T恤", slot: 6, rarity: 2, assetURL: "https://placehold.co/128x128?text=Body-Tshirt", iconURL: "https://placehold.co/64x64?text=Body-Tshirt", dropWeight: 20, isEnabled: 1},
		{code: "hat_crown", name: "金王冠", slot: 1, rarity: 3, assetURL: "https://placehold.co/128x128?text=Hat-Crown", iconURL: "https://placehold.co/64x64?text=Hat-Crown", dropWeight: 4, isEnabled: 1},
		{code: "back_wings", name: "天使翅膀", slot: 5, rarity: 3, assetURL: "https://placehold.co/128x128?text=Back-Wings", iconURL: "https://placehold.co/64x64?text=Back-Wings", dropWeight: 4, isEnabled: 1},
		{code: "body_armor", name: "黄金圣衣", slot: 6, rarity: 4, assetURL: "https://placehold.co/128x128?text=Body-Armor", iconURL: "https://placehold.co/64x64?text=Body-Armor", dropWeight: 1, isEnabled: 1},
	}
	for _, want := range seedRows {
		var gotName, gotAssetURL, gotIconURL string
		var gotSlot, gotRarity, gotDropWeight, gotIsEnabled int
		err := sqlDB.QueryRowContext(ctx,
			`SELECT name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled FROM cosmetic_items WHERE code = ?`, want.code).
			Scan(&gotName, &gotSlot, &gotRarity, &gotAssetURL, &gotIconURL, &gotDropWeight, &gotIsEnabled)
		if err != nil {
			t.Errorf("SELECT row %q after second Up: %v", want.code, err)
			continue
		}
		if gotName != want.name {
			t.Errorf("cosmetic %q name = %q, want %q (ON DUPLICATE KEY UPDATE 应强制覆盖回 0012 seed 钦定值)", want.code, gotName, want.name)
		}
		if gotSlot != want.slot {
			t.Errorf("cosmetic %q slot = %d, want %d (ON DUPLICATE KEY UPDATE 应强制覆盖回 0012 seed 钦定值)", want.code, gotSlot, want.slot)
		}
		if gotRarity != want.rarity {
			t.Errorf("cosmetic %q rarity = %d, want %d (ON DUPLICATE KEY UPDATE 应强制覆盖回 0012 seed 钦定值)", want.code, gotRarity, want.rarity)
		}
		if gotAssetURL != want.assetURL {
			t.Errorf("cosmetic %q asset_url = %q, want %q (ON DUPLICATE KEY UPDATE 应强制覆盖回 0012 seed 钦定值)", want.code, gotAssetURL, want.assetURL)
		}
		if gotIconURL != want.iconURL {
			t.Errorf("cosmetic %q icon_url = %q, want %q (ON DUPLICATE KEY UPDATE 应强制覆盖回 0012 seed 钦定值)", want.code, gotIconURL, want.iconURL)
		}
		if gotDropWeight != want.dropWeight {
			t.Errorf("cosmetic %q drop_weight = %d, want %d (ON DUPLICATE KEY UPDATE 应强制覆盖回 0012 seed 钦定值)", want.code, gotDropWeight, want.dropWeight)
		}
		if gotIsEnabled != want.isEnabled {
			t.Errorf("cosmetic %q is_enabled = %d, want %d (ON DUPLICATE KEY UPDATE 应强制覆盖回 0012 seed 钦定值)", want.code, gotIsEnabled, want.isEnabled)
		}
	}
}

// TestMigrateIntegration_ChestOpenLogs_Schema 验证
// migrations/0013_init_chest_open_logs.up.sql 钦定的 chest_open_logs 表 schema
// 与数据库设计.md §5.7 + V1接口设计.md §7.2 一致：
//
//   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - user_id BIGINT UNSIGNED NOT NULL
//   - chest_id BIGINT UNSIGNED NOT NULL
//   - cost_steps INT UNSIGNED NOT NULL（**关键**：column_type 必须含 "unsigned"，
//     与 emoji_configs.sort_order 的 "int" / cosmetic_items.drop_weight 的
//     "int unsigned" 一脉相承）
//   - reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL
//   - reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL
//   - reward_rarity TINYINT NOT NULL
//   - created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - KEY idx_user_id_created_at (user_id, created_at)
//   - KEY idx_reward_cosmetic_item_id (reward_cosmetic_item_id)
//
// **关键覆盖点**：
//   - **无** UpdatedAt 字段 —— 总列数 == 8（不是 9）；append-only 日志表语义
//     兜底防有人误加 updated_at（与 Story 7.2 落地的 user_step_sync_logs 同模式）
//   - **无** UNIQUE 约束 —— 检查 INFORMATION_SCHEMA.STATISTICS 中
//     chest_open_logs 表的 non_unique = 0 的索引必须为空 / 仅有 PRIMARY；
//     日志表允许同 user_id 多次开箱
//   - 双索引（idx_user_id_created_at + idx_reward_cosmetic_item_id）列顺序断言
//     （与 §5.7 钦定一致）
//   - BIGINT UNSIGNED 字段（id / user_id / chest_id / reward_user_cosmetic_item_id /
//     reward_cosmetic_item_id）column_type 必须含 "unsigned"
//   - INT UNSIGNED 字段（cost_steps）column_type 必须含 "unsigned"
//
// **背景（Story 20.4 引入）**：本 case 验证 0013 migration 落地的 schema
// 与 §5.7 钦定 1:1 对齐；用于在 epics.md §Story 20.4 钦定的"单元测试覆盖 ≥3 case"
// 中作为 schema-correctness 路径（happy / migrate up 后表存在 + 字段类型 +
// 全部索引和约束都符合 §5.7）。
func TestMigrateIntegration_ChestOpenLogs_Schema(t *testing.T) {
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

	// 1. INFORMATION_SCHEMA.TABLES：chest_open_logs 表存在
	var tableCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'cat_test' AND table_name = 'chest_open_logs'`).Scan(&tableCount)
	if err != nil {
		t.Errorf("query chest_open_logs table existence: %v", err)
	} else if tableCount != 1 {
		t.Errorf("chest_open_logs table count = %d, want 1", tableCount)
	}

	// 2. INFORMATION_SCHEMA.COLUMNS：8 列存在 + 类型对齐
	//    **关键**：cost_steps 是 INT UNSIGNED → column_type 必须 = "int unsigned"
	//    （与 cosmetic_items.drop_weight 同模式 —— 都是 INT UNSIGNED → "int unsigned"）。
	//    BIGINT UNSIGNED 字段 column_type = "bigint unsigned"（与 0001 users.id 同模式）。
	chestCols := []struct {
		col          string
		wantDataType string
		wantColType  string
	}{
		{"id", "bigint", "bigint unsigned"},
		{"user_id", "bigint", "bigint unsigned"},
		{"chest_id", "bigint", "bigint unsigned"},
		{"cost_steps", "int", "int unsigned"},
		{"reward_user_cosmetic_item_id", "bigint", "bigint unsigned"},
		{"reward_cosmetic_item_id", "bigint", "bigint unsigned"},
		{"reward_rarity", "tinyint", "tinyint"},
		{"created_at", "datetime", "datetime(3)"},
	}
	for _, c := range chestCols {
		var dt, ct string
		err := sqlDB.QueryRowContext(ctx, `
			SELECT data_type, column_type FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'chest_open_logs' AND column_name = ?`,
			c.col).Scan(&dt, &ct)
		if err != nil {
			t.Errorf("query chest_open_logs.%s type: %v", c.col, err)
			continue
		}
		if dt != c.wantDataType {
			t.Errorf("chest_open_logs.%s data_type = %q, want %q", c.col, dt, c.wantDataType)
		}
		if ct != c.wantColType {
			t.Errorf("chest_open_logs.%s column_type = %q, want %q", c.col, ct, c.wantColType)
		}
	}

	// 3. INFORMATION_SCHEMA.COLUMNS：chest_open_logs 表总列数 == 8
	//    **关键**：兜底防有人误加 updated_at 字段（append-only 日志表无 UPDATE 语义；
	//    与 0006 user_step_sync_logs 同模式）；如计数 = 9 说明有人误加 updated_at。
	var colCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'cat_test' AND table_name = 'chest_open_logs'`).Scan(&colCount)
	if err != nil {
		t.Errorf("query chest_open_logs total column count: %v", err)
	} else if colCount != 8 {
		t.Errorf("chest_open_logs total column count = %d, want 8 (append-only 日志表无 updated_at；如计数 = 9 说明有人误加 updated_at 字段)", colCount)
	}

	// 4a. chest_open_logs PK = id
	var pkCol string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = 'cat_test' AND table_name = 'chest_open_logs' AND constraint_name = 'PRIMARY'`).Scan(&pkCol)
	if err != nil {
		t.Errorf("query chest_open_logs PK: %v", err)
	} else if pkCol != "id" {
		t.Errorf("chest_open_logs PK column = %q, want 'id'", pkCol)
	}

	// 4b. **无**其他 UNIQUE 索引（non_unique=0 仅 PRIMARY 一行；
	//     chest_open_logs 是 append-only 日志表，允许同 user_id 多次开箱）。
	//     INFORMATION_SCHEMA.STATISTICS 按 (index_name) 去重计数 non_unique=0 的索引
	//     —— 应只有 PRIMARY 一个；如多于 1 说明有人误加 UNIQUE 约束。
	var uniqueIdxCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics
		WHERE table_schema = 'cat_test' AND table_name = 'chest_open_logs' AND non_unique = 0`).Scan(&uniqueIdxCount)
	if err != nil {
		t.Errorf("query chest_open_logs unique index count: %v", err)
	} else if uniqueIdxCount != 1 {
		t.Errorf("chest_open_logs unique index count = %d, want 1 (仅 PRIMARY；append-only 日志表禁止 UNIQUE 约束)", uniqueIdxCount)
	}

	// 4c. KEY idx_user_id_created_at + KEY idx_reward_cosmetic_item_id 存在 + 列顺序
	idxCases := []struct {
		idxName  string
		wantCols []string
	}{
		{"idx_user_id_created_at", []string{"user_id", "created_at"}},
		{"idx_reward_cosmetic_item_id", []string{"reward_cosmetic_item_id"}},
	}
	for _, ic := range idxCases {
		rows, err := sqlDB.QueryContext(ctx, `
			SELECT column_name FROM information_schema.statistics
			WHERE table_schema = 'cat_test' AND table_name = 'chest_open_logs' AND index_name = ?
			ORDER BY seq_in_index ASC`, ic.idxName)
		if err != nil {
			t.Errorf("query chest_open_logs.%s columns: %v", ic.idxName, err)
			continue
		}
		var cols []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Errorf("scan %s column: %v", ic.idxName, err)
				continue
			}
			cols = append(cols, c)
		}
		rows.Close()
		if len(cols) != len(ic.wantCols) {
			t.Errorf("chest_open_logs.%s column count = %d, want %d (cols=%v)", ic.idxName, len(cols), len(ic.wantCols), cols)
			continue
		}
		for i, w := range ic.wantCols {
			if cols[i] != w {
				t.Errorf("chest_open_logs.%s column[%d] = %q, want %q", ic.idxName, i, cols[i], w)
			}
		}
	}

	// 5. INFORMATION_SCHEMA.COLUMNS.COLUMN_DEFAULT：
	//    - created_at DEFAULT 含 substring "CURRENT_TIMESTAMP"
	//    - **不**检查 user_id / chest_id / cost_steps / reward_user_cosmetic_item_id /
	//      reward_cosmetic_item_id / reward_rarity 的 DEFAULT（NOT NULL 但 DDL 不
	//      预设 DEFAULT；由 service 层显式提供值）
	{
		var def sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'chest_open_logs' AND column_name = 'created_at'`).Scan(&def)
		if err != nil {
			t.Errorf("query chest_open_logs.created_at default: %v", err)
		} else if !def.Valid || !strings.Contains(strings.ToUpper(def.String), "CURRENT_TIMESTAMP") {
			t.Errorf("chest_open_logs.created_at default = %v, want substring 'CURRENT_TIMESTAMP'", def)
		}
	}
}

// TestMigrateIntegration_ChestOpenLogs_AppendOnly 验证
// migrations/0013_init_chest_open_logs.up.sql 钦定的 append-only 日志表语义：
// 同一 user_id + 同一 chest_id 的多行 INSERT 必须**全部成功**（无 UNIQUE 拒绝）。
//
// **背景（Story 20.4 引入）**：epics.md §Story 20.4 钦定的"集成测试覆盖
// （dockertest）：migrate up → SHOW CREATE TABLE 对比 schema → migrate down"路径
// 在 ChestOpenLogs_Schema case 用 INFORMATION_SCHEMA 字段层精确断言取代了
// 不稳定的 SHOW CREATE TABLE 字符串比对；本 case 是额外的运行时 append-only
// 语义验证 —— 与 20.2 落地的 TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected
// 形成对照：后者验证有 UNIQUE 的运行时拒绝，本 case 验证无 UNIQUE 的多行允许。
//
// **覆盖路径**：
//  1. migrate up → chest_open_logs 表存在
//  2. 插入 chest_open_logs (user_id=1, chest_id=1, cost_steps=1000,
//     reward_user_cosmetic_item_id=0, reward_cosmetic_item_id=10, reward_rarity=1) → 成功
//     （reward_user_cosmetic_item_id=0 是节点 7 阶段语义占位，本 case 即模拟 20.6 INSERT 行为）
//  3. 再次插入 chest_open_logs (user_id=1, chest_id=2, cost_steps=1000,
//     reward_user_cosmetic_item_id=0, reward_cosmetic_item_id=11, reward_rarity=2) → 成功
//     （同 user_id，不同 chest_id —— 用户开箱多轮场景）
//  4. 再次插入 chest_open_logs (user_id=1, chest_id=1, cost_steps=1000,
//     reward_user_cosmetic_item_id=0, reward_cosmetic_item_id=12, reward_rarity=1) → 成功
//     （同 user_id + 同 chest_id —— 防御性 case，确保无任何 UNIQUE 阻塞）
//  5. SELECT COUNT(*) FROM chest_open_logs WHERE user_id=1 → count = 3
//     （3 行全部成功插入，证实 append-only 语义）
//
// 用 database/sql 直跑 raw INSERT（**不**走 GORM）让测试结果**不**依赖 ORM
// 行为差异（与 Story 11.2 / 17.2 / 20.2 落地的 UNIQUE 拒绝 case 同模式）。
//
// 测试用 user_id / chest_id 与未来 20.6 / 20.9 业务用 id 完全隔离 —— 用 user_id=1
// + chest_id=1/2 是 dockertest 容器内独立 mysql 实例，与 prod 数据无关；
// 容器测试结束后自动 purge（startMySQL t.Cleanup）。
func TestMigrateIntegration_ChestOpenLogs_AppendOnly(t *testing.T) {
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

	// 步骤 2：首条插入 (user=1, chest=1) → 必须成功
	//        reward_user_cosmetic_item_id=0 模拟节点 7 阶段 Story 20.6 INSERT 行为
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO chest_open_logs (user_id, chest_id, cost_steps, reward_user_cosmetic_item_id, reward_cosmetic_item_id, reward_rarity) VALUES (1, 1, 1000, 0, 10, 1)`); err != nil {
		t.Fatalf("first insert chest_open_logs (user=1, chest=1) should succeed: %v", err)
	}

	// 步骤 3：同 user_id 不同 chest_id (user=1, chest=2) → 必须成功（多轮开箱场景）
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO chest_open_logs (user_id, chest_id, cost_steps, reward_user_cosmetic_item_id, reward_cosmetic_item_id, reward_rarity) VALUES (1, 2, 1000, 0, 11, 2)`); err != nil {
		t.Fatalf("second insert chest_open_logs (user=1, chest=2) should succeed (append-only, no UNIQUE): %v", err)
	}

	// 步骤 4：同 user_id + 同 chest_id (user=1, chest=1) → 必须成功
	//        防御性 case：确保无任何 UNIQUE(user_id, chest_id) 误加
	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO chest_open_logs (user_id, chest_id, cost_steps, reward_user_cosmetic_item_id, reward_cosmetic_item_id, reward_rarity) VALUES (1, 1, 1000, 0, 12, 1)`); err != nil {
		t.Fatalf("third insert chest_open_logs (user=1, chest=1 dup) should succeed (append-only, no UNIQUE): %v", err)
	}

	// 步骤 5：SELECT COUNT(*) WHERE user_id=1 → 3（证实 append-only 语义）
	var count int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chest_open_logs WHERE user_id = 1`).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT chest_open_logs WHERE user_id=1: %v", err)
	}
	if count != 3 {
		t.Errorf("after 3 inserts, chest_open_logs count for user_id=1 = %d, want 3 (append-only 日志表 3 行全部成功，无任何 UNIQUE 阻塞)", count)
	}
}

// TestMigrateIntegration_UserCosmeticItems_Schema 验证
// migrations/0015_init_user_cosmetic_items.up.sql 钦定的 user_cosmetic_items 表
// schema 与数据库设计.md §5.9（行 487-503）+ §6.10 status / §6.11 source 枚举
// + V1接口设计.md §8.2 inventory 字段语义一致：
//
//   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - user_id BIGINT UNSIGNED NOT NULL
//   - cosmetic_item_id BIGINT UNSIGNED NOT NULL
//   - status TINYINT NOT NULL DEFAULT 1
//   - source TINYINT NOT NULL DEFAULT 1
//   - source_ref_id BIGINT UNSIGNED NULL（**IS_NULLABLE = YES 关键断言**）
//   - obtained_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - consumed_at DATETIME(3) NULL（**IS_NULLABLE = YES 关键断言**）
//   - created_at / updated_at DATETIME(3)
//   - KEY idx_user_id_status (user_id, status)
//   - KEY idx_user_id_cosmetic_item_id (user_id, cosmetic_item_id)
//   - KEY idx_source (source, source_ref_id)
//   - **无 UNIQUE 约束**（除 PRIMARY 外 non_unique=0 的索引数 = 0）
//
// **关键覆盖点**：
//   - source_ref_id / consumed_at 的 IS_NULLABLE = YES（可空列断言）—— 与
//     cosmetic_items 全列 NOT NULL 形成对照；下游 23.4 / Epic 32 判"是否已回填 /
//     是否已消耗"依赖此可空性，GORM struct 用 *uint64 / *time.Time 指针映射
//   - 10 列字段计数兜底（防漂移：如有人误加字段会让计数变 11 失败）
//   - 三个普通索引列顺序断言（与 §5.9 钦定一致）
//   - **无 UNIQUE 断言**：与 cosmetic_items 有 uk_code 形成对照（实例表，同种
//     配置可持有多件 —— FR16）
//
// **背景（Story 23.2 引入）**：本 case 验证 0015 migration 落地的 schema
// 与 §5.9 钦定 1:1 对齐；用于在 epics.md §Story 23.2 钦定的"单元测试覆盖 ≥3
// case"中作为 schema-correctness 路径（happy / migrate up 后表存在 + 字段类型 +
// 全部索引和约束都符合 §5.9）。与 Story 20.2 落地的
// TestMigrateIntegration_CosmeticItems_Schema 同模式（INFORMATION_SCHEMA 字段层
// 精确断言取代不稳定的 SHOW CREATE TABLE 字符串比对）。
func TestMigrateIntegration_UserCosmeticItems_Schema(t *testing.T) {
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

	// 1. INFORMATION_SCHEMA.TABLES：user_cosmetic_items 表存在
	var tableCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items'`).Scan(&tableCount)
	if err != nil {
		t.Errorf("query user_cosmetic_items table existence: %v", err)
	} else if tableCount != 1 {
		t.Errorf("user_cosmetic_items table count = %d, want 1", tableCount)
	}

	// 2. INFORMATION_SCHEMA.COLUMNS：10 列存在 + 类型 + 可空性对齐
	//    **关键**：source_ref_id / consumed_at IS_NULLABLE = YES（可空列），
	//    其余列 IS_NULLABLE = NO（与 cosmetic_items 全列 NOT NULL 形成对照）。
	ucCols := []struct {
		col          string
		wantDataType string
		wantColType  string
		wantNullable string // "YES" / "NO"
	}{
		{"id", "bigint", "bigint unsigned", "NO"},
		{"user_id", "bigint", "bigint unsigned", "NO"},
		{"cosmetic_item_id", "bigint", "bigint unsigned", "NO"},
		{"status", "tinyint", "tinyint", "NO"},
		{"source", "tinyint", "tinyint", "NO"},
		{"source_ref_id", "bigint", "bigint unsigned", "YES"},
		{"obtained_at", "datetime", "datetime(3)", "NO"},
		{"consumed_at", "datetime", "datetime(3)", "YES"},
		{"created_at", "datetime", "datetime(3)", "NO"},
		{"updated_at", "datetime", "datetime(3)", "NO"},
	}
	for _, c := range ucCols {
		var dt, ct, nullable string
		err := sqlDB.QueryRowContext(ctx, `
			SELECT data_type, column_type, is_nullable FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items' AND column_name = ?`,
			c.col).Scan(&dt, &ct, &nullable)
		if err != nil {
			t.Errorf("query user_cosmetic_items.%s type: %v", c.col, err)
			continue
		}
		if dt != c.wantDataType {
			t.Errorf("user_cosmetic_items.%s data_type = %q, want %q", c.col, dt, c.wantDataType)
		}
		if ct != c.wantColType {
			t.Errorf("user_cosmetic_items.%s column_type = %q, want %q", c.col, ct, c.wantColType)
		}
		if nullable != c.wantNullable {
			t.Errorf("user_cosmetic_items.%s is_nullable = %q, want %q (§5.9 可空性钦定)", c.col, nullable, c.wantNullable)
		}
	}

	// 3. INFORMATION_SCHEMA.COLUMNS：user_cosmetic_items 表总列数 == 10
	//    兜底防有人误加字段（§5.9 钦定恰 10 列；如计数 = 11 说明漂移）。
	var colCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items'`).Scan(&colCount)
	if err != nil {
		t.Errorf("query user_cosmetic_items total column count: %v", err)
	} else if colCount != 10 {
		t.Errorf("user_cosmetic_items total column count = %d, want 10 (§5.9 钦定恰 10 列；如计数 != 10 说明有人误加/漏字段)", colCount)
	}

	// 4a. user_cosmetic_items.id PK = id
	var pkCol string
	err = sqlDB.QueryRowContext(ctx, `
		SELECT column_name FROM information_schema.key_column_usage
		WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items' AND constraint_name = 'PRIMARY'`).Scan(&pkCol)
	if err != nil {
		t.Errorf("query user_cosmetic_items PK: %v", err)
	} else if pkCol != "id" {
		t.Errorf("user_cosmetic_items PK column = %q, want 'id'", pkCol)
	}

	// 4b. 三个普通索引列顺序断言（与 §5.9 行 500-502 钦定一致）
	idxCases := []struct {
		idxName  string
		wantCols []string
	}{
		{"idx_user_id_status", []string{"user_id", "status"}},
		{"idx_user_id_cosmetic_item_id", []string{"user_id", "cosmetic_item_id"}},
		{"idx_source", []string{"source", "source_ref_id"}},
	}
	for _, ic := range idxCases {
		rows, err := sqlDB.QueryContext(ctx, `
			SELECT column_name FROM information_schema.statistics
			WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items' AND index_name = ?
			ORDER BY seq_in_index ASC`, ic.idxName)
		if err != nil {
			t.Errorf("query user_cosmetic_items.%s columns: %v", ic.idxName, err)
			continue
		}
		var cols []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Errorf("scan %s column: %v", ic.idxName, err)
				continue
			}
			cols = append(cols, c)
		}
		rows.Close()
		if len(cols) != len(ic.wantCols) {
			t.Errorf("user_cosmetic_items.%s column count = %d, want %d (cols=%v)", ic.idxName, len(cols), len(ic.wantCols), cols)
			continue
		}
		for i, w := range ic.wantCols {
			if cols[i] != w {
				t.Errorf("user_cosmetic_items.%s column[%d] = %q, want %q", ic.idxName, i, cols[i], w)
			}
		}
	}

	// 4c. **无 UNIQUE 约束断言**：除 PRIMARY 外无 non_unique=0 的索引
	//     （与 cosmetic_items 有 uk_code 形成对照；实例表同种配置可持有多件 FR16）。
	var uniqueIdxCount int
	err = sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics
		WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items'
		  AND non_unique = 0 AND index_name != 'PRIMARY'`).Scan(&uniqueIdxCount)
	if err != nil {
		t.Errorf("query user_cosmetic_items unique index count: %v", err)
	} else if uniqueIdxCount != 0 {
		t.Errorf("user_cosmetic_items 除 PRIMARY 外 UNIQUE 索引数 = %d, want 0 (实例表无 UNIQUE，同种配置可持有多件 FR16)", uniqueIdxCount)
	}

	// 5. status / source 默认值 = 1（§6.10 status DEFAULT 1=in_bag /
	//    §6.11 source DEFAULT 1=chest）
	for _, c := range []struct {
		col         string
		wantDefault string
	}{
		{"status", "1"},
		{"source", "1"},
	} {
		var def sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items' AND column_name = ?`,
			c.col).Scan(&def)
		if err != nil {
			t.Errorf("query user_cosmetic_items.%s default: %v", c.col, err)
			continue
		}
		if !def.Valid || def.String != c.wantDefault {
			t.Errorf("user_cosmetic_items.%s default = %v, want %q", c.col, def, c.wantDefault)
		}
	}

	// obtained_at / created_at / updated_at 默认值 = CURRENT_TIMESTAMP(3)
	for _, col := range []string{"obtained_at", "created_at", "updated_at"} {
		var def sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items' AND column_name = ?`,
			col).Scan(&def)
		if err != nil {
			t.Errorf("query user_cosmetic_items.%s default: %v", col, err)
			continue
		}
		if !def.Valid || !strings.Contains(strings.ToUpper(def.String), "CURRENT_TIMESTAMP") {
			t.Errorf("user_cosmetic_items.%s default = %v, want substring 'CURRENT_TIMESTAMP'", col, def)
		}
	}

	// source_ref_id / consumed_at 默认值 = NULL（可空列无显式默认）
	for _, col := range []string{"source_ref_id", "consumed_at"} {
		var def sql.NullString
		err := sqlDB.QueryRowContext(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = 'cat_test' AND table_name = 'user_cosmetic_items' AND column_name = ?`,
			col).Scan(&def)
		if err != nil {
			t.Errorf("query user_cosmetic_items.%s default: %v", col, err)
			continue
		}
		if def.Valid {
			t.Errorf("user_cosmetic_items.%s default = %q, want NULL (可空列无显式默认)", col, def.String)
		}
	}
}

// TestMigrateIntegration_UserCosmeticItems_AppendableAndUpdatable 验证
// migrations/0015_init_user_cosmetic_items.up.sql 钦定的可变实例表运行时语义：
//   - 同一 user_id 的多行 INSERT 必须**全部成功**（无 UNIQUE 拒绝，验证同种
//     配置可持有多件 —— FR16）
//   - status / consumed_at 可被 UPDATE 推进（验证可变实例语义，与
//     chest_open_logs append-only 不可 UPDATE 形成对照）
//   - 不带 source_ref_id 插入 → SELECT 回来 source_ref_id IS NULL（验证可空列
//     默认 NULL 非 0）
//
// **背景（Story 23.2 引入）**：与 Story 20.4 落地的
// TestMigrateIntegration_ChestOpenLogs_AppendOnly 同模式（dockertest 运行时
// 语义验证，用 database/sql 直跑 raw INSERT/UPDATE 不走 GORM 避免 ORM 行为
// 差异），但本表额外验证 status 可推进 + consumed_at 可从 NULL 写入 +
// source_ref_id 可空 NULL 语义 —— user_cosmetic_items 是可变实例表，**非**
// append-only 日志表。
//
// 测试用 user_id / cosmetic_item_id 与未来 23.5 / Epic 26 / 32 业务用 id
// 完全隔离 —— dockertest 容器内独立 mysql 实例，与 prod 数据无关，容器测试
// 结束后自动 purge（startMySQL t.Cleanup）。
func TestMigrateIntegration_UserCosmeticItems_AppendableAndUpdatable(t *testing.T) {
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

	// 步骤 1：同 user_id 插入 2 行（同 cosmetic_item_id）→ 都必须成功
	//        （无 UNIQUE 拒绝，验证同种配置可持有多件 FR16）；
	//        source_ref_id 显式给值（开箱来源 = chest_id 非空场景）。
	res1, err := sqlDB.ExecContext(ctx,
		`INSERT INTO user_cosmetic_items (user_id, cosmetic_item_id, status, source, source_ref_id) VALUES (1, 100, 1, 1, 500)`)
	if err != nil {
		t.Fatalf("first insert user_cosmetic_items (user=1, cosmetic=100) should succeed: %v", err)
	}
	row1ID, err := res1.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId of first insert: %v", err)
	}

	if _, err := sqlDB.ExecContext(ctx,
		`INSERT INTO user_cosmetic_items (user_id, cosmetic_item_id, status, source, source_ref_id) VALUES (1, 100, 1, 1, 501)`); err != nil {
		t.Fatalf("second insert user_cosmetic_items (user=1, cosmetic=100 dup) should succeed (无 UNIQUE，同种配置可持有多件 FR16): %v", err)
	}

	var cnt int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_cosmetic_items WHERE user_id = 1 AND cosmetic_item_id = 100`).Scan(&cnt); err != nil {
		t.Fatalf("SELECT COUNT user_cosmetic_items WHERE user_id=1 AND cosmetic_item_id=100: %v", err)
	}
	if cnt != 2 {
		t.Errorf("after 2 inserts, user_cosmetic_items count for (user=1, cosmetic=100) = %d, want 2 (无 UNIQUE 阻塞，同种配置可持有多件 FR16)", cnt)
	}

	// 步骤 2：UPDATE 其中一行 status=3 + consumed_at=NOW(3) → 成功
	//        （验证 status 可推进 + consumed_at 可从 NULL 写入；与
	//        chest_open_logs append-only 不可 UPDATE 形成对照）。
	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE user_cosmetic_items SET status = 3, consumed_at = NOW(3) WHERE id = ?`, row1ID); err != nil {
		t.Fatalf("UPDATE user_cosmetic_items SET status=3, consumed_at=NOW(3) should succeed (可变实例表): %v", err)
	}
	var gotStatus int
	var gotConsumedAt sql.NullTime
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT status, consumed_at FROM user_cosmetic_items WHERE id = ?`, row1ID).Scan(&gotStatus, &gotConsumedAt); err != nil {
		t.Fatalf("SELECT status, consumed_at after UPDATE: %v", err)
	}
	if gotStatus != 3 {
		t.Errorf("after UPDATE, status = %d, want 3 (status 可推进到 consumed)", gotStatus)
	}
	if !gotConsumedAt.Valid {
		t.Errorf("after UPDATE, consumed_at IS NULL, want non-NULL (consumed_at 可从 NULL 写入)")
	}

	// 步骤 3：插入不带 source_ref_id 的一行（依赖 NULL 默认）→ SELECT 回来
	//        source_ref_id IS NULL（验证可空列默认 NULL 非 0；合成产出实例先
	//        NULL 后回填 compose_log_id 场景）。
	res3, err := sqlDB.ExecContext(ctx,
		`INSERT INTO user_cosmetic_items (user_id, cosmetic_item_id, status, source) VALUES (2, 200, 1, 2)`)
	if err != nil {
		t.Fatalf("insert user_cosmetic_items without source_ref_id should succeed (可空列默认 NULL): %v", err)
	}
	row3ID, err := res3.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId of third insert: %v", err)
	}
	var gotSourceRefID sql.NullInt64
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT source_ref_id FROM user_cosmetic_items WHERE id = ?`, row3ID).Scan(&gotSourceRefID); err != nil {
		t.Fatalf("SELECT source_ref_id after insert without it: %v", err)
	}
	if gotSourceRefID.Valid {
		t.Errorf("source_ref_id = %d after insert without it, want NULL (可空列默认 NULL 非 0；下游判'是否已回填'依赖此语义)", gotSourceRefID.Int64)
	}
}
