//go:build integration
// +build integration

// Story 17.4 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 1 条 happy 链路 case：
//  1. migrate up 落地 0010 seed 后 4 行 → svc.ListAvailable 返 4 个 EmojiBrief +
//     字段值正确 + 排序按 sort_order ASC 稳定
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// 复用 home_service_integration_test.go 的 startMySQL / runMigrations helper（同
// service_test package 直接调，与既有集成测试同模式）。
//
// **不**手工 INSERT 测试数据（与 17.3 集成测试不同）—— 直接复用 0010 seed migration
// 落地的 4 行（wave / love / laugh / cry），让本 case 同时验证：
//   - emoji_repo.List SQL 正确（实际跑出 4 行）
//   - service.ListAvailable DTO 转换正确（字段值不丢失）
//   - 与 17.3 seed 跨 story 集成（seed → 接口 endpoint 闭环）

package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// buildEmojiServiceIntegration: 起容器 → migrate → 装配 emoji svc + 返清理 closure。
//
// 与 buildHomeServiceIntegration 同模式：复用 startMySQL / runMigrations helper；
// **不**起 txMgr / signer（emoji_service 不依赖）。
func buildEmojiServiceIntegration(t *testing.T) (svc service.EmojiService, sqlDB *sql.DB, cleanup func()) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn) // 跑到最新版（含 0010 seed）

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		dockerCleanup()
		t.Fatalf("db.Open: %v", err)
	}

	emojiRepo := mysql.NewEmojiRepo(gormDB)
	svc = service.NewEmojiService(emojiRepo)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, cleanup
}

// TestEmojiServiceIntegration_ListAvailable_SeedContent: migrate 后跑 ListAvailable
// 直接验证 17.3 seed 落地的 4 行；与 17.3 migrate_integration_test.go 的 SeedContent
// 是 repo→service 视角而非 raw SQL 视角，闭环 0010 → emoji_repo → emoji_service。
func TestEmojiServiceIntegration_ListAvailable_SeedContent(t *testing.T) {
	svc, _, cleanup := buildEmojiServiceIntegration(t)
	defer cleanup()

	got, err := svc.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4 (seed by 0010 migration)", len(got))
	}

	// 验证字段值（与 0010_seed_emoji_configs.up.sql 钦定 1:1 对齐）+
	// 排序按 sort_order ASC（V1 §11.1 服务端逻辑步骤 2 钦定）
	want := []struct {
		code      string
		name      string
		sortOrder int32
	}{
		{"wave", "挥手", 1},
		{"love", "爱心", 2},
		{"laugh", "大笑", 3},
		{"cry", "哭", 4},
	}
	for i, w := range want {
		if got[i].Code != w.code {
			t.Errorf("got[%d].Code = %q, want %q", i, got[i].Code, w.code)
		}
		if got[i].Name != w.name {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, w.name)
		}
		if got[i].SortOrder != w.sortOrder {
			t.Errorf("got[%d].SortOrder = %d, want %d", i, got[i].SortOrder, w.sortOrder)
		}
		if got[i].AssetURL == "" {
			t.Errorf("got[%d].AssetURL is empty, want non-empty (V1 §11.1 钦定 enabled 表情 assetUrl 必非空)", i)
		}
	}
}
