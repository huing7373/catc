//go:build integration
// +build integration

// Story 23.3 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 catalog 链路：
//  1. ListCatalog_SeedContent：migrate up 落地 0012 seed 15 行 →
//     svc.ListCatalog 返 15 个 CosmeticBrief + 字段值正确 + 按
//     rarity ASC, slot ASC, id ASC 三级全序稳定排序
//  2. ListCatalog_DisabledExcluded：UPDATE 一行 is_enabled=0 →
//     ListCatalog 返 14 行且无 hat_yellow（验证 epics.md AC "1 disabled →
//     不返回" 的 SQL 层真值；该 case 用独立容器，测后销毁不污染其他 case）
//
// Story 23.4 扩展：inventory 链路 case（AC8 钦定 —— inventory **必须**手工
// INSERT user_cosmetic_items，与 catalog 复用 0012 seed 闭环**不同**，因
// 0012/0015 seed 不含 user_cosmetic_items 行，inventory 无数据可复用）：
//  3. ListInventory_GroupsAndInstances：手工 INSERT 1 user + 多实例（含
//     同 cosmetic 多实例 + status 1/2/3）→ svc.ListInventory → groups 数量 +
//     count + 两级排序 + status=3(consumed) 不出现（SQL status IN (1,2) 过滤真值）
//  4. ListInventory_EmptyBag：不 INSERT → {groups:[]} 非 nil（空背包真值）
//  5. ListInventory_DisabledConfigStillVisible：INSERT 实例 +
//     UPDATE cosmetic_items is_enabled=0 → 该组仍返回 + row 真实值（态 B
//     真值；验证 ListByIDsForInventory **无** is_enabled=1 过滤）
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
	// Story 23.4 加：注入 userCosmeticItemRepo（GET /cosmetics/inventory 实例
	// 数据源）+ NewCosmeticService 扩 2 参（回归点：既有 catalog 集成 case
	// 复用同 helper，扩签名后此处必须同步改 2 参，否则 catalog 集成测试编译红）。
	userCosmeticItemRepo := mysql.NewUserCosmeticItemRepo(gormDB)
	svc = service.NewCosmeticService(cosmeticItemRepo, userCosmeticItemRepo)

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

// cosmeticIDByCode 查 0012 seed 落地的 cosmetic_items.id（seed 用 AUTO_INCREMENT
// 不写显式 id，故 inventory 测试 INSERT user_cosmetic_items 前先按 code 查真实
// id 关联）。
func cosmeticIDByCode(t *testing.T, rawDB *sql.DB, code string) uint64 {
	t.Helper()
	var id uint64
	err := rawDB.QueryRowContext(context.Background(),
		"SELECT id FROM cosmetic_items WHERE code = ?", code).Scan(&id)
	if err != nil {
		t.Fatalf("查 cosmetic_items id by code=%q: %v", code, err)
	}
	return id
}

// insertUserCosmeticItem 手工 INSERT 一条 user_cosmetic_items 实例并返回其
// AUTO_INCREMENT id（§5.9 表，0015 migration 落地；0012/0015 seed 无数据，
// inventory 集成测试必须手工 INSERT —— 与 catalog 复用 seed 的关键差异）。
func insertUserCosmeticItem(t *testing.T, rawDB *sql.DB, userID, cosmeticItemID uint64, status int8) uint64 {
	t.Helper()
	res, err := rawDB.ExecContext(context.Background(),
		"INSERT INTO user_cosmetic_items (user_id, cosmetic_item_id, status, source, obtained_at) VALUES (?, ?, ?, 1, NOW(3))",
		userID, cosmeticItemID, status)
	if err != nil {
		t.Fatalf("INSERT user_cosmetic_items (user=%d cosmetic=%d status=%d): %v", userID, cosmeticItemID, status, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return uint64(id)
}

// TestCosmeticServiceIntegration_ListInventory_GroupsAndInstances:
// 手工 INSERT 1 user + 多实例（hat_yellow x3 含 1 equipped + hat_red x1 +
// 1 个 status=3 consumed）→ ListInventory 验证 groups 数量 + count +
// 两级排序 + status=3 不出现（SQL status IN (1,2) 过滤真值）。
//
// **注**：inventory 集成测试**必须**手工 INSERT user_cosmetic_items（与
// catalog 集成测试**不同** —— catalog 复用 0012 seed 闭环不手工 INSERT，但
// 0012/0015 seed 不含 user_cosmetic_items 行，inventory 无数据可复用，故必须
// 手工 INSERT；AC8 钦定的关键差异）。
func TestCosmeticServiceIntegration_ListInventory_GroupsAndInstances(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900001)
	hatYellowID := cosmeticIDByCode(t, rawDB, "hat_yellow") // slot=1 rarity=1
	hatRedID := cosmeticIDByCode(t, rawDB, "hat_red")       // slot=1 rarity=1
	hatChefID := cosmeticIDByCode(t, rawDB, "hat_chef")     // slot=1 rarity=2

	// hat_yellow x3（2 in_bag + 1 equipped）
	uy1 := insertUserCosmeticItem(t, rawDB, userID, hatYellowID, 1)
	uy2 := insertUserCosmeticItem(t, rawDB, userID, hatYellowID, 2) // equipped
	uy3 := insertUserCosmeticItem(t, rawDB, userID, hatYellowID, 1)
	// hat_red x1
	ur1 := insertUserCosmeticItem(t, rawDB, userID, hatRedID, 1)
	// hat_chef x1 status=3(consumed) → 应被 SQL status IN (1,2) 过滤，不出现
	_ = insertUserCosmeticItem(t, rawDB, userID, hatChefID, 3)

	got, err := svc.ListInventory(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListInventory: %v", err)
	}
	// 2 组（hat_yellow + hat_red）；hat_chef 唯一实例 status=3 被过滤 → 无该组
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (hat_yellow + hat_red；hat_chef status=3 consumed 被 SQL status IN (1,2) 过滤无该组)", len(got))
	}

	// 两级排序：hat_yellow / hat_red 均 rarity=1 slot=1 → 按 cosmeticItemId ASC
	// 决定性区分（seed 顺序 hat_yellow 先 INSERT id 更小 → got[0]）。
	for i := 0; i+1 < len(got); i++ {
		a, b := got[i], got[i+1]
		if !lexLE(a.Rarity, a.Slot, a.CosmeticItemID, b.Rarity, b.Slot, b.CosmeticItemID) {
			t.Errorf("groups 排序违约 got[%d]=(r=%d,s=%d,id=%d) 应 <= got[%d]=(r=%d,s=%d,id=%d)",
				i, a.Rarity, a.Slot, a.CosmeticItemID, i+1, b.Rarity, b.Slot, b.CosmeticItemID)
		}
	}

	yellowGroup := findGroupByCID(got, hatYellowID)
	if yellowGroup == nil {
		t.Fatalf("缺 hat_yellow 组 (cosmeticItemId=%d)", hatYellowID)
	}
	if yellowGroup.Count != 3 || len(yellowGroup.Instances) != 3 {
		t.Errorf("hat_yellow 组 count=%d instances=%d, want count=3 instances=3 (含 equipped；count=len(instances))", yellowGroup.Count, len(yellowGroup.Instances))
	}
	// instances 按 userCosmeticItemId ASC 全序
	for i := 0; i+1 < len(yellowGroup.Instances); i++ {
		if yellowGroup.Instances[i].UserCosmeticItemID >= yellowGroup.Instances[i+1].UserCosmeticItemID {
			t.Errorf("hat_yellow instances 未按 userCosmeticItemId ASC 排序: %+v", yellowGroup.Instances)
		}
	}
	// 含 equipped(status=2) 实例（uy2）
	gotEquipped := false
	for _, ins := range yellowGroup.Instances {
		if ins.UserCosmeticItemID == uy2 && ins.Status == 2 {
			gotEquipped = true
		}
	}
	if !gotEquipped {
		t.Errorf("hat_yellow 组缺 equipped 实例 uy2=%d status=2 (equipped 不被过滤，status IN (1,2) 含 2)", uy2)
	}
	// 态 A：metadata 用 0012 seed 真实值（hat_yellow name=小黄帽 slot=1 rarity=1 非空 URL）
	if yellowGroup.Name != "小黄帽" || yellowGroup.Slot != 1 || yellowGroup.Rarity != 1 {
		t.Errorf("hat_yellow 组 metadata = {Name:%q Slot:%d Rarity:%d}, want 小黄帽/1/1 (态 A 真实值)", yellowGroup.Name, yellowGroup.Slot, yellowGroup.Rarity)
	}
	if yellowGroup.IconURL == "" || yellowGroup.AssetURL == "" {
		t.Errorf("hat_yellow 组 URL 空, want 非空 (态 A enabled 行 §8.2 行 1372/1373)")
	}

	redGroup := findGroupByCID(got, hatRedID)
	if redGroup == nil || redGroup.Count != 1 {
		t.Fatalf("hat_red 组 = %+v, want count=1", redGroup)
	}
	if redGroup.Instances[0].UserCosmeticItemID != ur1 {
		t.Errorf("hat_red instance id = %d, want %d", redGroup.Instances[0].UserCosmeticItemID, ur1)
	}

	// hat_chef status=3 consumed 实例绝不出现在任何 group（SQL status IN (1,2) 过滤真值）
	if findGroupByCID(got, hatChefID) != nil {
		t.Errorf("got 含 hat_chef 组，want 无 (唯一实例 status=3 consumed 被 SQL status IN (1,2) 过滤)")
	}
	_ = uy1
	_ = uy3
}

// TestCosmeticServiceIntegration_ListInventory_EmptyBag:
// migrate 后不 INSERT 任何 user_cosmetic_items → ListInventory 返
// len==0 && != nil（空背包 {groups:[]} 真值，§8.2 行 1341/1440）。
func TestCosmeticServiceIntegration_ListInventory_EmptyBag(t *testing.T) {
	svc, _, cleanup := buildCosmeticServiceIntegration(t)
	defer cleanup()

	got, err := svc.ListInventory(context.Background(), 777777)
	if err != nil {
		t.Fatalf("ListInventory err = %v, want nil (空背包非 error)", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []InventoryGroup{} (§8.2 行 1440 groups:[] not null)")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0 (无 user_cosmetic_items → 空背包)", len(got))
	}
}

// TestCosmeticServiceIntegration_ListInventory_DisabledConfigStillVisible:
// INSERT 实例关联 hat_yellow → UPDATE cosmetic_items SET is_enabled=0
// WHERE code='hat_yellow' → ListInventory 仍返回该组且用 row 真实值（态 B
// 真值；验证 ListByIDsForInventory **无** is_enabled=1 过滤 —— §8.2 行 1437
// 关键约束：admin 下架配置后已拥有道具**不得**静默消失）。
func TestCosmeticServiceIntegration_ListInventory_DisabledConfigStillVisible(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900002)
	hatYellowID := cosmeticIDByCode(t, rawDB, "hat_yellow")
	insertUserCosmeticItem(t, rawDB, userID, hatYellowID, 1)

	// admin 下架 hat_yellow（is_enabled=0）→ 态 B disabled-but-exists
	res, err := rawDB.ExecContext(context.Background(),
		"UPDATE cosmetic_items SET is_enabled = 0 WHERE code = ?", "hat_yellow")
	if err != nil {
		t.Fatalf("UPDATE hat_yellow is_enabled=0: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("UPDATE RowsAffected = %d, want 1", n)
	}

	got, err := svc.ListInventory(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListInventory: %v", err)
	}
	// **关键**：态 B 已拥有项**仍可见**（ListByIDsForInventory 无 is_enabled=1
	// 过滤；若误加过滤该组会从 config map 消失被误判态 C 降级 → 此断言会失败）。
	g := findGroupByCID(got, hatYellowID)
	if g == nil {
		t.Fatalf("hat_yellow 组缺失，want 仍可见 (态 B 已拥有不得静默丢失；§8.2 行 1437 禁止 is_enabled=1 过滤)")
	}
	// 态 B 用 row 真实值（**非** 态 C 降级占位 未知装扮/99/1/空串）
	if g.Name != "小黄帽" || g.Slot != 1 || g.Rarity != 1 {
		t.Errorf("态 B 组 metadata = {Name:%q Slot:%d Rarity:%d}, want row 真实值 小黄帽/1/1（**非**态 C 占位 未知装扮/99/1）", g.Name, g.Slot, g.Rarity)
	}
	if g.IconURL == "" || g.AssetURL == "" {
		t.Errorf("态 B 组 URL 空, want row 真实非空值（§8.2 行 1372/1373：态 B 用真实值非降级空串）")
	}
	if g.Count != 1 {
		t.Errorf("态 B 组 count=%d, want 1", g.Count)
	}
}

// findGroupByCID 按 cosmeticItemId 找 InventoryGroup（nil if not found）。
func findGroupByCID(groups []service.InventoryGroup, cid uint64) *service.InventoryGroup {
	for i := range groups {
		if groups[i].CosmeticItemID == cid {
			return &groups[i]
		}
	}
	return nil
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
