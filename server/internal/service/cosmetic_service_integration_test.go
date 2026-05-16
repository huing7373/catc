//go:build integration
// +build integration

// Story 23.3 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 2 条链路 case：
//  1. ListCatalog_SeedContent：migrate up 落地 0012 seed 15 行 →
//     svc.ListCatalog 返 15 个 CosmeticBrief + 字段值正确 + 按
//     rarity ASC, slot ASC, id ASC 三级全序稳定排序
//  2. ListCatalog_DisabledExcluded：UPDATE 一行 is_enabled=0 →
//     ListCatalog 返 14 行且无 hat_yellow（验证 epics.md AC "1 disabled →
//     不返回" 的 SQL 层真值；该 case 用独立容器，测后销毁不污染其他 case）
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）
// 触发。
//
// 复用 home_service_integration_test.go 的 startMySQL / runMigrations helper（同
// service_test package 直接调，与 emoji_service_integration_test.go 同模式）。
//
// **不**手工 INSERT 测试数据（与 emoji 集成测试同）—— 直接复用 0012 seed
// migration 落地的 15 行（hat_yellow / hat_red / gloves_white / ...），让本 case
// 同时验证：
//   - cosmetic_item_repo.ListEnabledForCatalog SQL 正确（实际跑出 15 行 + 三级
//     全序排序）
//   - service.ListCatalog DTO 转换正确（字段值不丢失）
//   - 与 Story 20.3 0012 seed 跨 story 集成（seed → 接口 endpoint 闭环）

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

// buildCosmeticServiceIntegration: 起容器 → migrate → 装配 cosmetic svc + 返
// 清理 closure + raw *sql.DB（DisabledExcluded case 用 raw SQL UPDATE）。
//
// 与 buildEmojiServiceIntegration 同模式：复用 startMySQL / runMigrations
// helper；**不**起 txMgr / signer（cosmetic_service 不依赖）。
func buildCosmeticServiceIntegration(t *testing.T) (svc service.CosmeticService, sqlDB *sql.DB, cleanup func()) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn) // 跑到最新版（含 0012 seed cosmetic_items 15 行）

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

	cosmeticItemRepo := mysql.NewCosmeticItemRepo(gormDB)
	svc = service.NewCosmeticService(cosmeticItemRepo)

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

// TestCosmeticServiceIntegration_ListCatalog_SeedContent: migrate 后跑
// ListCatalog 直接验证 Story 20.3 0012 seed 落地的 15 行；闭环
// 0012 → cosmetic_item_repo → cosmetic_service。
func TestCosmeticServiceIntegration_ListCatalog_SeedContent(t *testing.T) {
	svc, _, cleanup := buildCosmeticServiceIntegration(t)
	defer cleanup()

	got, err := svc.ListCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(got) != 15 {
		t.Fatalf("len(got) = %d, want 15 (seed by 0012 migration)", len(got))
	}

	// 排序断言：结果按 rarity ASC, slot ASC, id ASC 三级全序（§8.1 关键约束
	// 行 1306 + 23.1 r1 [P2]）—— 逐对相邻元素断言 lexicographic <=。
	for i := 0; i+1 < len(got); i++ {
		a, b := got[i], got[i+1]
		if !lexLE(a.Rarity, a.Slot, a.CosmeticItemID, b.Rarity, b.Slot, b.CosmeticItemID) {
			t.Errorf("排序违约 got[%d]=(rarity=%d,slot=%d,id=%d) 应 <= got[%d]=(rarity=%d,slot=%d,id=%d) (rarity ASC, slot ASC, id ASC 三级全序)",
				i, a.Rarity, a.Slot, a.CosmeticItemID, i+1, b.Rarity, b.Slot, b.CosmeticItemID)
		}
	}

	// 抽样验证字段值与 0012 seed 1:1（0012 seed hat_yellow 行：
	// code=hat_yellow / name=小黄帽 / slot=1 / rarity=1 / 非空 placeholder URL）。
	// hat_yellow 是 rarity=1, slot=1 内 id 最小行（seed 第 1 行 INSERT），三级
	// 全序下应为 got[0]。
	hatYellow := findByCode(got, "hat_yellow")
	if hatYellow == nil {
		t.Fatalf("seed 应含 hat_yellow，got codes=%v", codesOf(got))
	}
	if hatYellow.Name != "小黄帽" {
		t.Errorf("hat_yellow.Name = %q, want 小黄帽", hatYellow.Name)
	}
	if hatYellow.Slot != 1 {
		t.Errorf("hat_yellow.Slot = %d, want 1", hatYellow.Slot)
	}
	if hatYellow.Rarity != 1 {
		t.Errorf("hat_yellow.Rarity = %d, want 1", hatYellow.Rarity)
	}
	if hatYellow.IconURL == "" {
		t.Errorf("hat_yellow.IconURL is empty, want non-empty (§8.1 行 1267 钦定 enabled cosmetic iconUrl 必非空)")
	}
	if hatYellow.AssetURL == "" {
		t.Errorf("hat_yellow.AssetURL is empty, want non-empty (§8.1 行 1268 钦定 enabled cosmetic assetUrl 必非空)")
	}
	if got[0].Code != "hat_yellow" {
		t.Errorf("got[0].Code = %q, want hat_yellow (rarity=1,slot=1 内 id 最小，三级全序应排首位)", got[0].Code)
	}

	// 抽样验证 body_armor（legendary rarity=4 唯一行，应排末位）。
	bodyArmor := findByCode(got, "body_armor")
	if bodyArmor == nil {
		t.Fatalf("seed 应含 body_armor，got codes=%v", codesOf(got))
	}
	if bodyArmor.Rarity != 4 || bodyArmor.Slot != 6 || bodyArmor.Name != "黄金圣衣" {
		t.Errorf("body_armor = %+v, want rarity=4 slot=6 name=黄金圣衣", *bodyArmor)
	}
	if got[len(got)-1].Code != "body_armor" {
		t.Errorf("got[末位].Code = %q, want body_armor (rarity=4 唯一，三级全序应排末位)", got[len(got)-1].Code)
	}
}

// TestCosmeticServiceIntegration_ListCatalog_DisabledExcluded: migrate 后
// UPDATE cosmetic_items SET is_enabled=0 WHERE code='hat_yellow' → 跑
// ListCatalog → 断言 len==14 且结果中无 hat_yellow（验证 epics.md AC
// "1 disabled → 不返回" 的 repo SQL `WHERE is_enabled=1` 真值；本 case 用
// 独立容器，cleanup 销毁不污染其他 case）。
func TestCosmeticServiceIntegration_ListCatalog_DisabledExcluded(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticServiceIntegration(t)
	defer cleanup()

	res, err := rawDB.ExecContext(context.Background(),
		"UPDATE cosmetic_items SET is_enabled = 0 WHERE code = ?", "hat_yellow")
	if err != nil {
		t.Fatalf("UPDATE hat_yellow is_enabled=0: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("UPDATE RowsAffected = %d, want 1 (seed 应含 hat_yellow)", n)
	}

	got, err := svc.ListCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(got) != 14 {
		t.Fatalf("len(got) = %d, want 14 (15 seed - 1 disabled)", len(got))
	}
	if findByCode(got, "hat_yellow") != nil {
		t.Errorf("got 含 hat_yellow，want 不返回（is_enabled=0 应被 repo SQL WHERE is_enabled=1 过滤）；got codes=%v", codesOf(got))
	}
}

// lexLE 返回 (r1,s1,i1) <= (r2,s2,i2) 的 lexicographic 比较结果。
func lexLE(r1, s1 int8, i1 uint64, r2, s2 int8, i2 uint64) bool {
	if r1 != r2 {
		return r1 < r2
	}
	if s1 != s2 {
		return s1 < s2
	}
	return i1 <= i2
}

func findByCode(briefs []service.CosmeticBrief, code string) *service.CosmeticBrief {
	for i := range briefs {
		if briefs[i].Code == code {
			return &briefs[i]
		}
	}
	return nil
}

func codesOf(briefs []service.CosmeticBrief) []string {
	out := make([]string, 0, len(briefs))
	for _, b := range briefs {
		out = append(out, b.Code)
	}
	return out
}
