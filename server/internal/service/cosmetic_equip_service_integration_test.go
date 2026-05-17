//go:build integration
// +build integration

// Story 26.3 集成测试（AC6）：用 dockertest 起真实 mysql:8.0 容器跑穿戴
// 事务真值（epics.md §Story 26.3 行 3564-3566 钦定 2 场景）：
//
//  1. EquipFirstHat：创建 user + pet + 1 件 hat 实例（status=1 in_bag）→
//     equip → 断言 DB user_pet_equips 1 行（pet_id / slot / user_cosmetic_item_id
//     正确）+ 该实例 status=2。
//  2. EquipSecondHatSameSlot：接着 equip 另一件 hat（同 slot）→ 断言 DB
//     user_pet_equips **仍 1 行**（同槽换装，旧行删 + 新行 INSERT，净 1 行）
//     + 旧 hat status=1（回 in_bag）+ 新 hat status=2。
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（`go test -tags=integration`）触发。
// 本机 Windows docker 不可用 → t.Skip（startMySQL 内已 skip）。CI Linux 跑。
//
// 复用既有 helper：startMySQL / runMigrations（auth_service_integration_test.go）
// + insertUser / insertPet（home_service_integration_test.go）+
// insertUserCosmeticItem / cosmeticIDByCode（cosmetic_service_integration_test.go）。
// **手工 INSERT 测试数据**（不调 auth_service.GuestLogin），与既有 inventory
// 集成测试同模式（0012/0015 seed 无 user_cosmetic_items / user_pet_equips 行）。
//
// **深度回滚 / 100 并发兜底 / 状态一致性矩阵归 Story 26.5**（26.3/26.4 AC 范围
// 红线 —— 26.3/26.4 仅 2 个 happy + 同槽换装场景，epics.md 行 3592-3616 钦定）。
//
// ============================================================================
// Story 26.5 追加（Layer 2 集成测试 — 穿戴事务全流程；epics.md §Story 26.5
// 行 3592-3616 唯一权威 AC 来源；12 类场景）：
//
// 复用 26.3/26.4 既有 2 case（不重写）：
//   - TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot
//       — equip happy（slot 空直接装）+ 同槽换装（epics.md 行 3603-3604 部分覆盖）
//   - TestCosmeticEquipServiceIntegration_UnequipHappyPath
//       — unequip happy + 重复 unequip 5004（非幂等）
//
// 26.5 追加 10 个新测试函数（全部受文件顶部 build tag 覆盖）：
//   | epics.md 行 | 场景       | 测试函数                                                             |
//   |-------------|------------|----------------------------------------------------------------------|
//   | 行 3603     | 完整流程   | _FullFlow_Equip5SlotsAll                                             |
//   | 行 3604     | 同槽换装   | （复用 26.3 _EquipAndSwapSameSlot；StateConsistencyMatrix 复跑断言）|
//   | 行 3605     | 回滚 1     | _EquipDeleteOldEquipFails_AllRollback                               |
//   | 行 3606     | 回滚 2     | _EquipUpdateStatusFails_AllRollback                                 |
//   | 行 3607     | 回滚 3     | _UnequipUpdateStatusFails_AllRollback                               |
//   | 行 3608     | 并发 1     | _Concurrent100SamePetSlot_FinalStateConsistent                      |
//   | 行 3609     | 并发 2     | _Concurrent100SameInstanceDifferentPets_OnlyOneEquips               |
//   | 行 3610     | 边界 1     | _EquipConsumedInstance_Returns5003                                  |
//   | 行 3611     | 边界 2     | _EquipNotOwnedInstance_Returns5002                                  |
//   | 行 3612     | 边界 3     | _UnequipEmptySlot_Returns5004                                       |
//   | 行 3613     | 状态一致性 | _StateConsistencyMatrix + assertEquipStateConsistency helper        |
//
// 新增基础设施（仅本文件可见，受 build tag 覆盖）：
//   - faultUserPetEquipRepoOnDelete            — 回滚 1（DeleteByPetSlotInTx 注入 err）
//   - faultUserCosmeticItemRepoOnUpdateStatus  — 回滚 2/3（UpdateStatusInTx 注入 err）
//   - buildCosmeticEquipServiceIntegrationWithRepos — 暴露内部原料供 fault case 重装配
//   - assertEquipStateConsistency              — NFR2 双向不变量断言（status=2 ⟺ user_pet_equips 行）
//
// fault injection / 100 goroutine 并发范式直接抄 20.9
// chest_open_service_integration_test.go（faultStepAccountRepoOnSpend 行 1448 /
// buildChestServiceWithRepos 行 414 / _Concurrent100SameKey 行 810）。
//
// 本机 Windows docker 不可用 → startMySQL 内 t.Skip 兜底，CI Linux 跑。
// ============================================================================

package service_test

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// buildCosmeticEquipServiceIntegration: 起容器 → migrate（含 0012 seed
// cosmetic_items 15 行 + 0016 user_pet_equips schema）→ 装配 equip svc（真
// tx.NewManager，与 chest_open_service_integration_test.go 同模式）+ 返
// 清理 closure + raw *sql.DB（断言用）。
func buildCosmeticEquipServiceIntegration(t *testing.T) (svc service.CosmeticEquipService, sqlDB *sql.DB, cleanup func()) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn) // 跑到最新版（含 0012 seed + 0015/0016 schema）

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

	userCosmeticItemRepo := mysql.NewUserCosmeticItemRepo(gormDB)
	cosmeticItemRepo := mysql.NewCosmeticItemRepo(gormDB)
	petRepo := mysql.NewPetRepo(gormDB)
	userPetEquipRepo := mysql.NewUserPetEquipRepo(gormDB)
	txMgr := tx.NewManager(gormDB)

	svc = service.NewCosmeticEquipService(txMgr, userCosmeticItemRepo, cosmeticItemRepo, petRepo, userPetEquipRepo)

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

// TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot 在单一容器内串行
// 跑 AC6 钦定 2 场景（共享 user/pet，第 2 场景依赖第 1 场景的 equip 后态 ——
// 同槽换装必须基于"slot 1 已有装备"前置）。
func TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900101)
	const petID = uint64(700101)
	insertUser(t, rawDB, userID, "guest-equip-26-3", "穿戴测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	// 两件 hat（同 slot=1）：hat_yellow / hat_red（0012 seed，slot=1 rarity=1）
	hatYellowCfgID := cosmeticIDByCode(t, rawDB, "hat_yellow")
	hatRedCfgID := cosmeticIDByCode(t, rawDB, "hat_red")
	inst1 := insertUserCosmeticItem(t, rawDB, userID, hatYellowCfgID, 1) // status=1 in_bag
	inst2 := insertUserCosmeticItem(t, rawDB, userID, hatRedCfgID, 1)    // status=1 in_bag

	// ===== 场景 1：equip 第 1 件 hat（slot 空 → 直接装上）=====
	out1, err := svc.Equip(context.Background(), service.EquipParams{
		UserID: userID, PetID: petID, UserCosmeticItemID: inst1,
	})
	if err != nil {
		t.Fatalf("场景1 Equip(inst1=%d): err = %v, want nil", inst1, err)
	}
	if out1.PetID != petID || out1.Equipped.Slot != 1 ||
		out1.Equipped.UserCosmeticItemID != inst1 || out1.Equipped.CosmeticItemID != hatYellowCfgID {
		t.Errorf("场景1 EquipResult = %+v, want petId=%d slot=1 uci=%d ci=%d", out1, petID, inst1, hatYellowCfgID)
	}

	// DB user_pet_equips 恰 1 行（pet_id / slot / user_cosmetic_item_id 正确）
	// 注：assertCount 内部已前缀 "SELECT COUNT(*) FROM "，故仅传 FROM 之后部分。
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petID}, 1,
		"场景1 后 user_pet_equips 行数")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1 AND user_cosmetic_item_id = ?",
		[]any{petID, inst1}, 1, "场景1 user_pet_equips (pet,slot=1,inst1) 行")
	// inst1 status=2 equipped
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 2", []any{inst1}, 1,
		"场景1 后 inst1 status=2")

	// ===== 场景 2：equip 第 2 件 hat（同 slot=1 → 同槽换装）=====
	out2, err := svc.Equip(context.Background(), service.EquipParams{
		UserID: userID, PetID: petID, UserCosmeticItemID: inst2,
	})
	if err != nil {
		t.Fatalf("场景2 Equip(inst2=%d): err = %v, want nil", inst2, err)
	}
	if out2.Equipped.UserCosmeticItemID != inst2 || out2.Equipped.Slot != 1 ||
		out2.Equipped.CosmeticItemID != hatRedCfgID {
		t.Errorf("场景2 EquipResult = %+v, want uci=%d slot=1 ci=%d", out2, inst2, hatRedCfgID)
	}

	// user_pet_equips **仍 1 行**（同槽换装：旧行删 + 新行 INSERT，净 1 行）
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petID}, 1,
		"场景2 后 user_pet_equips 行数（同槽换装净 1 行）")
	// 现存行指向 inst2（旧行已删）
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1 AND user_cosmetic_item_id = ?",
		[]any{petID, inst2}, 1, "场景2 user_pet_equips 现指向 inst2")
	assertCount(t, rawDB,
		"user_pet_equips WHERE user_cosmetic_item_id = ?", []any{inst1}, 0,
		"场景2 旧 inst1 的 user_pet_equips 行已删除")
	// 旧 hat（inst1）status 回 1 in_bag；新 hat（inst2）status=2 equipped
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 1", []any{inst1}, 1,
		"场景2 后旧 inst1 status 回 1 in_bag")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 2", []any{inst2}, 1,
		"场景2 后新 inst2 status=2 equipped")
}

// TestCosmeticEquipServiceIntegration_UnequipHappyPath（Story 26.4 AC6）：
// 创建 user + pet + 1 件 hat 实例 → 先 Equip 装上（status→2 + user_pet_equips
// 1 行）→ 再 Unequip(petId, slot=1) → 断言 DB user_pet_equips 该 (pet,slot)
// 行不存在（0 行）+ 实例 status=1 (in_bag) + UnequipResult.Unequipped=true。
//
// 建议补（V1 §8.4 行 1651 "已空槽必 5004" 不变量）：unequip 成功后**再次**
// Unequip 同 (petId, slot) → 断言返 5004（apperror.ErrCosmeticSlotMismatch，
// **非**幂等成功）+ DB 状态不变（验证 unequip 非幂等 + 空槽显式报错契约）。
//
// **范围红线**：深度回滚 / 100 并发 unequip 串行化压测 / 状态一致性矩阵归
// Story 26.5（本文件仅 happy + 重复 unequip 5004 两场景，epics.md 行 3592-3616）。
func TestCosmeticEquipServiceIntegration_UnequipHappyPath(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900401)
	const petID = uint64(700401)
	insertUser(t, rawDB, userID, "guest-unequip-26-4", "卸下测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	hatYellowCfgID := cosmeticIDByCode(t, rawDB, "hat_yellow")
	inst1 := insertUserCosmeticItem(t, rawDB, userID, hatYellowCfgID, 1) // status=1 in_bag

	// ===== 前置：Equip 把 hat 装上（slot 1，status→2 + user_pet_equips 1 行）=====
	_, err := svc.Equip(context.Background(), service.EquipParams{
		UserID: userID, PetID: petID, UserCosmeticItemID: inst1,
	})
	if err != nil {
		t.Fatalf("前置 Equip(inst1=%d): err = %v, want nil", inst1, err)
	}
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1", []any{petID}, 1,
		"前置 Equip 后 user_pet_equips slot=1 行存在")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 2", []any{inst1}, 1,
		"前置 Equip 后 inst1 status=2 equipped")

	// ===== 场景 1：Unequip(petId, slot=1) → 卸下成功 =====
	out, err := svc.Unequip(context.Background(), service.UnequipParams{
		UserID: userID, PetID: petID, Slot: 1,
	})
	if err != nil {
		t.Fatalf("场景1 Unequip(slot=1): err = %v, want nil", err)
	}
	if out.PetID != petID || out.Slot != 1 || !out.Unequipped {
		t.Errorf("场景1 UnequipResult = %+v, want petId=%d slot=1 unequipped=true", out, petID)
	}
	// user_pet_equips 该 (pet, slot) 行不存在（0 行）
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1", []any{petID}, 0,
		"场景1 Unequip 后 user_pet_equips slot=1 行已删")
	// 实例 status 回 1 in_bag
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 1", []any{inst1}, 1,
		"场景1 Unequip 后 inst1 status 回 1 in_bag")

	// ===== 场景 2：再次 Unequip 同 (petId, slot) → 5004（非幂等成功）=====
	_, err = svc.Unequip(context.Background(), service.UnequipParams{
		UserID: userID, PetID: petID, Slot: 1,
	})
	if err == nil {
		t.Fatalf("场景2 重复 Unequip 空槽: err = nil, want 5004（非幂等成功）")
	}
	ae, ok := apperror.As(err)
	if !ok || ae.Code != apperror.ErrCosmeticSlotMismatch {
		t.Errorf("场景2 重复 Unequip err = %v, want AppError code=5004 (ErrCosmeticSlotMismatch)", err)
	}
	// DB 状态不变（实例仍 status=1，user_pet_equips 仍 0 行）
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 1", []any{inst1}, 1,
		"场景2 后 inst1 status 仍 1 in_bag（重复 unequip 不改状态）")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1", []any{petID}, 0,
		"场景2 后 user_pet_equips slot=1 仍 0 行")
}

// ============================================================================
// Story 26.5 — fault injection wrapper + helper 基础设施（Task 1；AC1/4/5/6/10）
// ============================================================================

// faultUserPetEquipRepoOnDelete 包装真实 mysql.UserPetEquipRepo：
// DeleteByPetSlotInTx 返 injectErr（回滚 1 注入点 = equip 步骤 8 删旧装备），
// 其余 4 方法透传委托真实 repo（按方法包装范式抄 20.9
// faultStepAccountRepoOnSpend 行 1448）。
//
// **范围**：仅本文件可见（命名带 Equip 前缀避免与 20.9/4.7/11.9 同包
// service_test 命名冲突；epics.md §26.5 §关键设计约束钦定本 story 新增 2 个
// wrapper struct）。
type faultUserPetEquipRepoOnDelete struct {
	delegate  mysql.UserPetEquipRepo
	injectErr error
}

func (f *faultUserPetEquipRepoOnDelete) FindByPetSlot(ctx context.Context, petID uint64, slot int8) (*mysql.UserPetEquip, error) {
	return f.delegate.FindByPetSlot(ctx, petID, slot)
}

func (f *faultUserPetEquipRepoOnDelete) DeleteByPetSlotInTx(ctx context.Context, petID uint64, slot int8) error {
	return f.injectErr // 回滚 1 注入点
}

func (f *faultUserPetEquipRepoOnDelete) InsertInTx(ctx context.Context, e *mysql.UserPetEquip) error {
	return f.delegate.InsertInTx(ctx, e)
}

func (f *faultUserPetEquipRepoOnDelete) FindUserCosmeticItemIDByPetSlotForUpdate(ctx context.Context, petID uint64, slot int8) (uint64, error) {
	return f.delegate.FindUserCosmeticItemIDByPetSlotForUpdate(ctx, petID, slot)
}

func (f *faultUserPetEquipRepoOnDelete) DeleteByPetSlotInTxReturningAffected(ctx context.Context, petID uint64, slot int8) (int64, error) {
	return f.delegate.DeleteByPetSlotInTxReturningAffected(ctx, petID, slot)
}

// faultUserCosmeticItemRepoOnUpdateStatus 包装真实 mysql.UserCosmeticItemRepo：
// UpdateStatusInTx 返 injectErr，其余 4 方法透传。服务回滚 2（equip slot 空 →
// INSERT user_pet_equips 成功 → 最后一步 UpdateStatusInTx(当前实例,equipped)
// 失败）+ 回滚 3（unequip DELETE 成功 → UpdateStatusInTx(uciID,in_bag) 失败）
// 两个 case —— 两处均是 userCosmeticRepo.UpdateStatusInTx 失败语义，同一个
// wrapper 服务两个回滚 case。
type faultUserCosmeticItemRepoOnUpdateStatus struct {
	delegate  mysql.UserCosmeticItemRepo
	injectErr error
}

func (f *faultUserCosmeticItemRepoOnUpdateStatus) ListByUserForInventory(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
	return f.delegate.ListByUserForInventory(ctx, userID)
}

func (f *faultUserCosmeticItemRepoOnUpdateStatus) CreateInTx(ctx context.Context, item *mysql.UserCosmeticItem) error {
	return f.delegate.CreateInTx(ctx, item)
}

func (f *faultUserCosmeticItemRepoOnUpdateStatus) FindByIDForEquip(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
	return f.delegate.FindByIDForEquip(ctx, id)
}

func (f *faultUserCosmeticItemRepoOnUpdateStatus) UpdateStatusInTx(ctx context.Context, id uint64, status int8) error {
	return f.injectErr // 回滚 2 / 回滚 3 注入点
}

// buildCosmeticEquipServiceIntegrationWithRepos 暴露内部原料供 fault case 在原料
// 上套 wrapper + service.NewCosmeticEquipService 重装配（模式抄 20.9
// buildChestServiceWithRepos 行 414 "返回完整原料供 fault case 装配"）。
//
// 既有 buildCosmeticEquipServiceIntegration（行 ~47-87）签名/行为**不改**（26.3/26.4
// 既有 2 case 调用点不破坏 —— 本 helper 是**新增**，既有 helper 内部不委托本
// helper 以保持 26.3/26.4 调用 trace 不变；少量 dsn/migrate 模板代码重复可接受，
// 以"不破坏既有 2 case + 最小风险"为准）。
func buildCosmeticEquipServiceIntegrationWithRepos(t *testing.T) (
	gormDBUserCosmeticItemRepo mysql.UserCosmeticItemRepo,
	cosmeticItemRepo mysql.CosmeticItemRepo,
	petRepo mysql.PetRepo,
	userPetEquipRepo mysql.UserPetEquipRepo,
	txMgr tx.Manager,
	rawDB *sql.DB,
	cleanup func(),
) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn) // 跑到最新版（含 0012 seed + 0015/0016 schema）

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

	gormDBUserCosmeticItemRepo = mysql.NewUserCosmeticItemRepo(gormDB)
	cosmeticItemRepo = mysql.NewCosmeticItemRepo(gormDB)
	petRepo = mysql.NewPetRepo(gormDB)
	userPetEquipRepo = mysql.NewUserPetEquipRepo(gormDB)
	txMgr = tx.NewManager(gormDB)

	rawDB, err = gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return gormDBUserCosmeticItemRepo, cosmeticItemRepo, petRepo, userPetEquipRepo, txMgr, rawDB, cleanup
}

// assertEquipStateConsistency 断言 NFR2 双向不变量（epics.md 行 3613 钦定的
// 状态一致性矩阵核心）：
//
//	正向：所有 status=2(equipped) 的实例必然在 user_pet_equips 有对应行
//	      → 无"equipped 但无装备关系"孤儿实例（COUNT == 0）
//	反向：所有 user_pet_equips 行对应的实例必然 status=2
//	      → 无"装备关系存在但实例非 equipped"孤儿行（COUNT == 0）
//
// 任一非 0 → t.Fatalf 报具体破坏的不变量 + userID（便于定位）。
// 复用范围：完整流程末尾 / 3 回滚 case ROLLBACK 后 / 2 并发 case 终态 /
// 独立 StateConsistencyMatrix case 每步后均调用。
func assertEquipStateConsistency(t *testing.T, rawDB *sql.DB, userID uint64) {
	t.Helper()

	// 正向：status=2 ⟹ 有 user_pet_equips 行（孤儿 equipped 实例计数必须 0）
	var orphanEquipped int64
	if err := rawDB.QueryRow(
		`SELECT COUNT(*) FROM user_cosmetic_items uci
		 WHERE uci.user_id = ? AND uci.status = 2
		   AND NOT EXISTS (
		       SELECT 1 FROM user_pet_equips upe
		       WHERE upe.user_cosmetic_item_id = uci.id
		   )`, userID).Scan(&orphanEquipped); err != nil {
		t.Fatalf("一致性正向查询失败 (userID=%d): %v", userID, err)
	}
	if orphanEquipped != 0 {
		t.Fatalf("NFR2 一致性破坏【正向】: userID=%d 有 %d 个 status=2(equipped) 实例无对应 user_pet_equips 行（孤儿 equipped 实例）",
			userID, orphanEquipped)
	}

	// 反向：user_pet_equips 行 ⟹ 实例 status=2（孤儿装备关系行计数必须 0）
	var orphanRow int64
	if err := rawDB.QueryRow(
		`SELECT COUNT(*) FROM user_pet_equips upe
		 JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id
		 WHERE upe.user_id = ? AND uci.status <> 2`, userID).Scan(&orphanRow); err != nil {
		t.Fatalf("一致性反向查询失败 (userID=%d): %v", userID, err)
	}
	if orphanRow != 0 {
		t.Fatalf("NFR2 一致性破坏【反向】: userID=%d 有 %d 个 user_pet_equips 行对应实例 status<>2（孤儿装备关系行）",
			userID, orphanRow)
	}
}

// requireEquipAppError 断言 err 是 *apperror.AppError 且 Code == wantCode
// （边界 case 错误码断言；与 chest_open_service_integration_test.go
// requireAppError 行 461 同模式，命名带 Equip 前缀避免同包冲突）。
func requireEquipAppError(t *testing.T, err error, wantCode int, ctx string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: 期望错误码 %d，实际 nil", ctx, wantCode)
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("%s: 期望 *AppError，实际 %T: %v", ctx, err, err)
	}
	if ae.Code != wantCode {
		t.Fatalf("%s: AppError.Code = %d, want %d (完整 err: %v)", ctx, ae.Code, wantCode, err)
	}
}

// ============================================================================
// Story 26.5 Task 2 — 完整流程 + 状态一致性矩阵 case（AC2/AC3/AC10）
// ============================================================================

// TestCosmeticEquipServiceIntegration_FullFlow_Equip5SlotsAll（AC2，epics.md 行
// 3603）：1 user + 1 pet + 5 件不同 slot 的 cosmetic 实例 → 依次 equip 5 次 →
// 断言 user_pet_equips 恰 5 行 + 5 实例全 status=2 + 末尾一致性不变量通过。
//
// 5 slot seed code（migrations/0012_seed_cosmetic_items.up.sql 实际读取确认）：
//
//	hat_yellow=slot1 / gloves_white=slot2 / glasses_round=slot3 /
//	neck_blue=slot4 / back_bag=slot5
func TestCosmeticEquipServiceIntegration_FullFlow_Equip5SlotsAll(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900501)
	const petID = uint64(700501)
	insertUser(t, rawDB, userID, "guest-equip-26-5-full", "完整流程测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	// 5 个分属不同 slot 的 0012 seed code（不臆造 —— 0012 实际行）
	codes := []struct {
		code string
		slot int8
	}{
		{"hat_yellow", 1},
		{"gloves_white", 2},
		{"glasses_round", 3},
		{"neck_blue", 4},
		{"back_bag", 5},
	}
	insts := make([]uint64, len(codes))
	for i, c := range codes {
		cfgID := cosmeticIDByCode(t, rawDB, c.code)
		insts[i] = insertUserCosmeticItem(t, rawDB, userID, cfgID, 1) // status=1 in_bag
	}

	// 依次 equip 5 次（全部 err==nil）
	for i, c := range codes {
		out, err := svc.Equip(context.Background(), service.EquipParams{
			UserID: userID, PetID: petID, UserCosmeticItemID: insts[i],
		})
		if err != nil {
			t.Fatalf("equip 第 %d 件 (%s, slot=%d, inst=%d): err = %v, want nil",
				i+1, c.code, c.slot, insts[i], err)
		}
		if out.Equipped.Slot != c.slot || out.Equipped.UserCosmeticItemID != insts[i] {
			t.Errorf("equip 第 %d 件 EquipResult = %+v, want slot=%d uci=%d",
				i+1, out, c.slot, insts[i])
		}
		// 每个 (slot, user_cosmetic_item_id) 对正确
		assertCount(t, rawDB,
			"user_pet_equips WHERE pet_id = ? AND slot = ? AND user_cosmetic_item_id = ?",
			[]any{petID, c.slot, insts[i]}, 1,
			"完整流程 user_pet_equips (pet,slot,inst) 对")
	}

	// user_pet_equips 恰 5 行
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petID}, 5,
		"完整流程后 user_pet_equips 恰 5 行")
	// 5 个实例全 status=2
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE user_id = ? AND status = 2", []any{userID}, 5,
		"完整流程后 5 实例全 status=2")

	assertEquipStateConsistency(t, rawDB, userID)
}

// TestCosmeticEquipServiceIntegration_StateConsistencyMatrix（AC3/AC10，epics.md
// 行 3613 + 行 3604 复跑同槽换装一致性补断言）：单容器内串行跑一组操作序列
// （equip 3 件不同 slot → unequip 1 件 → 同槽换装 1 件 → equip 第 4 件 →
// unequip 全部），**每个操作后**调 assertEquipStateConsistency 断言任意操作
// 序列后 status↔user_pet_equips 双向一致。
func TestCosmeticEquipServiceIntegration_StateConsistencyMatrix(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900502)
	const petID = uint64(700502)
	insertUser(t, rawDB, userID, "guest-equip-26-5-matrix", "一致性矩阵测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	hat1 := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "hat_yellow"), 1)   // slot1
	hat2 := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "hat_red"), 1)      // slot1（换装用）
	gloves := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "gloves_white"), 1) // slot2
	glasses := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "glasses_round"), 1) // slot3
	neck := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "neck_blue"), 1)    // slot4（第 4 件）

	eq := func(inst uint64, label string) {
		if _, err := svc.Equip(context.Background(), service.EquipParams{
			UserID: userID, PetID: petID, UserCosmeticItemID: inst,
		}); err != nil {
			t.Fatalf("%s: Equip(inst=%d) err = %v, want nil", label, inst, err)
		}
		assertEquipStateConsistency(t, rawDB, userID)
	}
	uneq := func(slot int8, label string) {
		if _, err := svc.Unequip(context.Background(), service.UnequipParams{
			UserID: userID, PetID: petID, Slot: slot,
		}); err != nil {
			t.Fatalf("%s: Unequip(slot=%d) err = %v, want nil", label, slot, err)
		}
		assertEquipStateConsistency(t, rawDB, userID)
	}

	// 序列：equip×3（hat1 slot1 / gloves slot2 / glasses slot3）
	eq(hat1, "步骤1 equip hat1 slot1")
	eq(gloves, "步骤2 equip gloves slot2")
	eq(glasses, "步骤3 equip glasses slot3")
	assertCount(t, rawDB, "user_pet_equips WHERE pet_id = ?", []any{petID}, 3, "equip×3 后 3 行")

	// unequip×1（卸 slot3 glasses）
	uneq(3, "步骤4 unequip slot3")
	assertCount(t, rawDB, "user_pet_equips WHERE pet_id = ?", []any{petID}, 2, "unequip×1 后 2 行")

	// 同槽换装×1（slot1 hat1 → hat2，复跑 AC3 同槽换装序列后断言一致性）
	eq(hat2, "步骤5 同槽换装 slot1 hat1→hat2")
	assertCount(t, rawDB, "user_pet_equips WHERE pet_id = ?", []any{petID}, 2,
		"同槽换装净行数不变（仍 2 行）")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1 AND user_cosmetic_item_id = ?",
		[]any{petID, hat2}, 1, "slot1 现指向 hat2")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 1", []any{hat1}, 1,
		"旧 hat1 status 回 1 in_bag")

	// equip 第 4 件（neck slot4）
	eq(neck, "步骤6 equip neck slot4")
	assertCount(t, rawDB, "user_pet_equips WHERE pet_id = ?", []any{petID}, 3,
		"equip 第 4 件后 3 行（slot1/2/4）")

	// unequip 全部（slot1/2/4）
	uneq(1, "步骤7 unequip slot1")
	uneq(2, "步骤8 unequip slot2")
	uneq(4, "步骤9 unequip slot4")
	assertCount(t, rawDB, "user_pet_equips WHERE pet_id = ?", []any{petID}, 0,
		"全部卸下后 0 行")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE user_id = ? AND status = 2", []any{userID}, 0,
		"全部卸下后无 equipped 实例（双向空集一致）")
	assertEquipStateConsistency(t, rawDB, userID)
}

// ============================================================================
// Story 26.5 Task 3 — 3 个回滚 case（AC4/AC5/AC6）
// ============================================================================

// TestCosmeticEquipServiceIntegration_EquipDeleteOldEquipFails_AllRollback（AC4，
// epics.md 行 3605）：正常 svc Equip hatA → slot=1 有旧装备；切 fault svc
// （DeleteByPetSlotInTx 注入 err）Equip hatB 同 slot → equip 步骤 8 删旧装备
// 失败 → fn return error → 真 InnoDB ROLLBACK；断言旧装备仍 equipped + 新装备
// 仍 in_bag + user_pet_equips 不变（指向旧实例）。
func TestCosmeticEquipServiceIntegration_EquipDeleteOldEquipFails_AllRollback(t *testing.T) {
	userCosmeticRepo, cosmeticRepo, petRepo, userPetEquipRepo, txMgr, rawDB, cleanup :=
		buildCosmeticEquipServiceIntegrationWithRepos(t)
	defer cleanup()

	const userID = uint64(900541)
	const petID = uint64(700541)
	insertUser(t, rawDB, userID, "guest-equip-26-5-rb1", "回滚1测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	hatA := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "hat_yellow"), 1)
	hatB := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "hat_red"), 1)

	// 前置：用**正常 svc** Equip hatA 到 slot=1（status=2 + user_pet_equips 1 行指向 hatA）
	normalSvc := service.NewCosmeticEquipService(txMgr, userCosmeticRepo, cosmeticRepo, petRepo, userPetEquipRepo)
	if _, err := normalSvc.Equip(context.Background(), service.EquipParams{
		UserID: userID, PetID: petID, UserCosmeticItemID: hatA,
	}); err != nil {
		t.Fatalf("前置 Equip(hatA=%d): err = %v, want nil", hatA, err)
	}
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1 AND user_cosmetic_item_id = ?",
		[]any{petID, hatA}, 1, "前置后 slot1 指向 hatA")

	// 切 fault svc：DeleteByPetSlotInTx 注入 err（回滚 1 注入点 = equip 步骤 8 删旧）
	faultPetEquipRepo := &faultUserPetEquipRepoOnDelete{
		delegate:  userPetEquipRepo,
		injectErr: stderrors.New("synthetic DeleteByPetSlotInTx failure (回滚1)"),
	}
	faultSvc := service.NewCosmeticEquipService(txMgr, userCosmeticRepo, cosmeticRepo, petRepo, faultPetEquipRepo)

	// fault svc Equip hatB 同 slot=1 → 走步骤 8 删旧装备 → fault → ROLLBACK
	_, err := faultSvc.Equip(context.Background(), service.EquipParams{
		UserID: userID, PetID: petID, UserCosmeticItemID: hatB,
	})
	requireEquipAppError(t, err, apperror.ErrServiceBusy, "回滚1 Equip(hatB) 删旧失败")

	// 断言：ROLLBACK 后 DB 恢复前置态
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 2", []any{hatA}, 1,
		"回滚1 后 hatA 仍 status=2 equipped（ROLLBACK）")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 1", []any{hatB}, 1,
		"回滚1 后 hatB 仍 status=1 in_bag（ROLLBACK）")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petID}, 1,
		"回滚1 后 user_pet_equips 仍 1 行（ROLLBACK）")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1 AND user_cosmetic_item_id = ?",
		[]any{petID, hatA}, 1, "回滚1 后 user_pet_equips 仍指向旧实例 hatA（未变）")

	assertEquipStateConsistency(t, rawDB, userID)
}

// TestCosmeticEquipServiceIntegration_EquipUpdateStatusFails_AllRollback（AC5，
// epics.md 行 3606）：slot 空场景；fault svc（UpdateStatusInTx 注入 err）
// Equip hatA → slot 空跳过删旧 → 步骤 9 InsertInTx user_pet_equips 成功 →
// 最后一步 UpdateStatusInTx(hatA,equipped) 失败 → fn return error → 真 InnoDB
// ROLLBACK 把刚 INSERT 的 user_pet_equips 行也回滚；断言 user_pet_equips 0 行
// + hatA 仍 status=1 + 双向空集一致。
func TestCosmeticEquipServiceIntegration_EquipUpdateStatusFails_AllRollback(t *testing.T) {
	userCosmeticRepo, cosmeticRepo, petRepo, userPetEquipRepo, txMgr, rawDB, cleanup :=
		buildCosmeticEquipServiceIntegrationWithRepos(t)
	defer cleanup()

	const userID = uint64(900551)
	const petID = uint64(700551)
	insertUser(t, rawDB, userID, "guest-equip-26-5-rb2", "回滚2测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	hatA := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "hat_yellow"), 1)

	// fault svc：UpdateStatusInTx 注入 err（回滚 2 注入点 = equip 最后一步）
	faultCosmeticRepo := &faultUserCosmeticItemRepoOnUpdateStatus{
		delegate:  userCosmeticRepo,
		injectErr: stderrors.New("synthetic UpdateStatusInTx failure (回滚2)"),
	}
	faultSvc := service.NewCosmeticEquipService(txMgr, faultCosmeticRepo, cosmeticRepo, petRepo, userPetEquipRepo)

	// slot 空 → 跳过删旧 → 步骤 9 INSERT 成功 → 最后一步 UpdateStatusInTx 失败 → ROLLBACK
	_, err := faultSvc.Equip(context.Background(), service.EquipParams{
		UserID: userID, PetID: petID, UserCosmeticItemID: hatA,
	})
	requireEquipAppError(t, err, apperror.ErrServiceBusy, "回滚2 Equip(hatA) 更新实例 status 失败")

	// 断言：INSERT 的新行被回滚（双向空集一致）
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petID}, 0,
		"回滚2 后 user_pet_equips 0 行（INSERT 的新行被 ROLLBACK）")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 1", []any{hatA}, 1,
		"回滚2 后 hatA 仍 status=1 in_bag（未变 equipped）")

	assertEquipStateConsistency(t, rawDB, userID)
}

// TestCosmeticEquipServiceIntegration_UnequipUpdateStatusFails_AllRollback（AC6，
// epics.md 行 3607）：正常 svc Equip hatA → slot=1（status=2 + user_pet_equips
// 1 行）；切 fault svc（UpdateStatusInTx 注入 err）Unequip slot=1 → unequip
// 步骤 6 DeleteByPetSlotInTxReturningAffected 成功 → UpdateStatusInTx(hatA,
// in_bag) 失败 → fn return error → 真 InnoDB ROLLBACK 把 DELETE 也回滚；
// 断言 user_pet_equips 仍 1 行 + hatA 仍 status=2 + 1↔1 一致。
func TestCosmeticEquipServiceIntegration_UnequipUpdateStatusFails_AllRollback(t *testing.T) {
	userCosmeticRepo, cosmeticRepo, petRepo, userPetEquipRepo, txMgr, rawDB, cleanup :=
		buildCosmeticEquipServiceIntegrationWithRepos(t)
	defer cleanup()

	const userID = uint64(900561)
	const petID = uint64(700561)
	insertUser(t, rawDB, userID, "guest-equip-26-5-rb3", "回滚3测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	hatA := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "hat_yellow"), 1)

	// 前置：用**正常 svc** Equip hatA 到 slot=1
	normalSvc := service.NewCosmeticEquipService(txMgr, userCosmeticRepo, cosmeticRepo, petRepo, userPetEquipRepo)
	if _, err := normalSvc.Equip(context.Background(), service.EquipParams{
		UserID: userID, PetID: petID, UserCosmeticItemID: hatA,
	}); err != nil {
		t.Fatalf("前置 Equip(hatA=%d): err = %v, want nil", hatA, err)
	}
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1", []any{petID}, 1,
		"前置 Equip 后 slot1 1 行")

	// 切 fault svc：UpdateStatusInTx 注入 err（回滚 3 注入点 = unequip 步骤 6 后）
	faultCosmeticRepo := &faultUserCosmeticItemRepoOnUpdateStatus{
		delegate:  userCosmeticRepo,
		injectErr: stderrors.New("synthetic UpdateStatusInTx failure (回滚3)"),
	}
	faultSvc := service.NewCosmeticEquipService(txMgr, faultCosmeticRepo, cosmeticRepo, petRepo, userPetEquipRepo)

	// fault svc Unequip slot=1 → DELETE 成功 → UpdateStatusInTx 失败 → ROLLBACK 把 DELETE 也回滚
	_, err := faultSvc.Unequip(context.Background(), service.UnequipParams{
		UserID: userID, PetID: petID, Slot: 1,
	})
	requireEquipAppError(t, err, apperror.ErrServiceBusy, "回滚3 Unequip(slot=1) 更新实例 status 失败")

	// 断言：DELETE 被回滚（行未删）+ 实例仍 equipped
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1", []any{petID}, 1,
		"回滚3 后 user_pet_equips slot=1 仍 1 行（DELETE 被 ROLLBACK）")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1 AND user_cosmetic_item_id = ?",
		[]any{petID, hatA}, 1, "回滚3 后 user_pet_equips 仍指向 hatA")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 2", []any{hatA}, 1,
		"回滚3 后 hatA 仍 status=2 equipped（未变 in_bag）")

	assertEquipStateConsistency(t, rawDB, userID)
}

// ============================================================================
// Story 26.5 Task 4 — 2 个并发 case（100 goroutine）（AC7/AC8）
// ============================================================================

// TestCosmeticEquipServiceIntegration_Concurrent100SamePetSlot_FinalStateConsistent
// （AC7，epics.md 行 3608「只 1 成功其余 99 error，DB UNIQUE(pet_id,slot) 兜底」
// + 行 3613 NFR2 一致性约束）：1 user + 1 pet + slot **初始为空** + 100 件不同
// hat 实例（全 slot=1）→ `<-start` 屏障强制 100 goroutine 同时释放 → 各 Equip
// 不同实例 → 断言**恰 1 成功 / 99 撞 1009 回滚** + **终态一致性矩阵**（恰 1 行 /
// 恰 1 个 status=2 / 无中间态 / 无部分提交 / 双向一致）。
//
// ===== 守门注释：根因不变量（改此 case 前必读，违反即"改坏测试"）=====
//
// **此 case 的恰-1 不变量来自测试结构，不是断言强弱选择**：
//
//  1. setup 钉死「slot 初始为空」：insertPet 后 **无任何 prior equip**；
//  2. `<-start` 屏障钉死「真并发」：100 goroutine 同时释放，**无错峰**；
//  3. 被测路径因 (1)+(2) 结构上**确定性收敛到唯一一条**：
//     `runEquipTx` 步骤 8 `FindByPetSlot` 是**普通 SELECT（无 FOR UPDATE）**
//     —— 见 user_pet_equip_repo.go:214 `First()`；FOR UPDATE 变体
//     `FindUserCosmeticItemIDByPetSlotForUpdate` **仅** runUnequipTx 步骤 5 用，
//     equip **不**用。slot 空 + 屏障同步 ⟹ 绝大多数 tx 的 MVCC 快照看到
//     「无旧行」⟹ 全部跳过 swap 分支、直奔步骤 9 `InsertInTx` ⟹ `uk_pet_slot`
//     UNIQUE **确定性恰放 1 个 INSERT 过、其余 99 撞 1062**
//     → ErrUserPetEquipPetSlotDuplicate → 步骤 9 Wrap 成 ErrServiceBusy(1009)
//     → 整事务回滚 → Equip 返 1009。
//
// **r1 codex 担心的"串行化 swap 让多个成功"在此 case 结构上不可达**：swap
// 路径要"成功换装"需该 tx 看得到一条**已提交的旧行**——只在 ① slot 非空，
// 或 ② goroutine 错峰（后启动者看到先提交者的行）时成立。本 case 用屏障专门
// 排除错峰、slot 又是空的，**swap 路径在此结构上不可达**。故 `successCount
// == 1` 在服务正确时**不会误失败**（化解 r1 合法担忧），又**能抓 uk_pet_slot
// 回滚回归**（满足 epics.md 行 3608 AC + r2 codex）。r1 把它放松成 `>= 1`
// 是过度修正（断言强弱 ping-pong 的表层翻转，非根因解决）。
//
// **⚠️ 谁若在此 case 前置一条 equip 让 slot 非空、或移除 `<-start` 屏障让
// goroutine 错峰 —— 恰-1 不变量就会被 swap 路径破坏。那是改坏测试结构，不是
// "放松断言"。想再把 `==1` 放松回 `>=1` 前，先反驳上面 (1)(2)(3) 的结构论证。**
//
// goroutine 收集 + start barrier 模式抄 20.9 _Concurrent100SameKey 行 810。
func TestCosmeticEquipServiceIntegration_Concurrent100SamePetSlot_FinalStateConsistent(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900571)
	const petID = uint64(700571)
	insertUser(t, rawDB, userID, "guest-equip-26-5-c1", "并发1测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	const N = 100
	hatYellowCfgID := cosmeticIDByCode(t, rawDB, "hat_yellow") // slot=1
	insts := make([]uint64, N)
	for i := 0; i < N; i++ {
		insts[i] = insertUserCosmeticItem(t, rawDB, userID, hatYellowCfgID, 1) // status=1 in_bag
	}

	errs := make([]error, N)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start // 等所有 goroutine ready 后统一释放（真并发，避免退化为顺序）
			_, err := svc.Equip(context.Background(), service.EquipParams{
				UserID: userID, PetID: petID, UserCosmeticItemID: insts[i],
			})
			errs[i] = err
		}()
	}
	close(start)
	wg.Wait()

	// 恰 1 个成功（epics.md 行 3608 AC + 守门注释根因不变量：slot 空 + 屏障
	// 同步 ⟹ swap 路径不可达 ⟹ 全部走 INSERT 竞争 ⟹ uk_pet_slot 确定性恰 1
	// 个 INSERT commit 过；**不**放松成 >= 1，见函数头守门注释）。
	successCount := 0
	for i, err := range errs {
		if err == nil {
			successCount++
			continue
		}
		// 那 99 个失败必须是**确切的 uk_pet_slot 重复键回滚错**（1009
		// ErrServiceBusy —— InsertInTx 撞 uk_pet_slot 1062 →
		// ErrUserPetEquipPetSlotDuplicate → 步骤 9 Wrap 成 1009）。用
		// AppError code 精确断言，**不**只数 err != nil —— 证明确实走了
		// uk_pet_slot 回滚路径，而非偶发别的错（否则"99 个 error"会被任意
		// 错误满足，回归抓不住根因）。
		requireEquipAppError(t, err, apperror.ErrServiceBusy,
			fmt.Sprintf("并发1 goroutine[%d] 失败应为 uk_pet_slot 重复键回滚错(1009)", i))
	}
	if successCount != 1 {
		t.Fatalf("并发1 同 pet 同 slot 100 equip: 成功 %d 个, want 恰 1（slot 初始空 + <-start 屏障 ⟹ swap 路径不可达 ⟹ uk_pet_slot 确定性恰 1 成功 / 99 撞 1009 回滚；见函数头守门注释）", successCount)
	}

	// ===== 终态一致性矩阵（r1 增量价值，保留 —— 在恰-1 计数断言之上再校 DB
	// 终态的双向一致性 / 无中间态 / 无部分提交，是并发正确性的补充真不变量）=====
	//
	// 不变量 1：uk_pet_slot 兜底 —— (pet_id, slot) 恰 1 行（至多 1 行由 UNIQUE
	// 保证；恰 1 行因唯一赢家的 INSERT commit 并占住 slot，99 个失败 tx 全回滚
	// 不留行 → 无脏写 / 无多行）。
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petID}, 1,
		"并发1 终态：user_pet_equips 恰 1 行（uk_pet_slot 兜底，无脏写/无多行）")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ? AND slot = 1", []any{petID}, 1,
		"并发1 终态：(pet,slot=1) 恰 1 行")

	// 不变量 2：状态分布一致 —— 恰 1 个实例 status=2（被现存 user_pet_equips
	// 行指向的赢家）；其余实例全 status=1（被换下/未装上的都回 in_bag，**无**
	// 实例卡在中间态：consumed/invalid/越界值 → status IN (1,2) 计数 == N）。
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE user_id = ? AND status = 2", []any{userID}, 1,
		"并发1 终态：恰 1 个实例 status=2（现存装备行指向的赢家）")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE user_id = ? AND status = 1", []any{userID}, int64(N-1),
		"并发1 终态：其余 N-1 个实例全 status=1（被换下/未装上都回 in_bag）")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE user_id = ? AND status IN (1,2)", []any{userID}, int64(N),
		"并发1 终态：全部 N 个实例都在 {in_bag,equipped}，无实例卡中间态（无部分提交）")

	// 不变量 3：现存装备行指向的实例正是那个唯一 status=2 实例（行↔状态对齐，
	// 无"装备行指向 A 但 A 不是 equipped / 另有 B 是 equipped"错位）。
	assertCount(t, rawDB,
		`user_pet_equips upe JOIN user_cosmetic_items uci
		   ON uci.id = upe.user_cosmetic_item_id
		 WHERE upe.pet_id = ? AND upe.slot = 1 AND uci.status = 2`,
		[]any{petID}, 1,
		"并发1 终态：现存装备行指向的实例正是唯一 status=2 实例（行↔状态对齐）")

	// 不变量 4：双向一致性（NFR2）—— 无孤儿 equipped 实例 / 无孤儿装备行。
	assertEquipStateConsistency(t, rawDB, userID)
}

// TestCosmeticEquipServiceIntegration_Concurrent100SameInstanceDifferentPets_OnlyOneEquips
// （AC8，epics.md 行 3609 "理论不发生因 1 user 1 pet，但测一致性约束"）：
// 1 user + 100 个 pet（同 user_id，pet 表 1 user N pet 物理可建，与节点 9
// 业务约束正交）+ 1 件 hat 实例 → 100 goroutine 各 Equip 同实例到不同 pet →
// 只 1 个成功（DB uk_user_cosmetic_item_id UNIQUE X-lock + 1062 兜底）。
//
// **为何 100 pet 必须 is_default 各不相同**（fix-review 26-5 r1 [P1]）：
// 0003_init_pets.up.sql 有 `UNIQUE KEY uk_user_default_pet (user_id,
// is_default)`。同 user 100 个 pet 若全 is_default=0，第 2 条 insertPet 即撞
// UNIQUE 1062 → t.Fatalf，case 根本到不了被测代码。pets.is_default 是
// TINYINT NOT NULL **无 CHECK 约束**（schema 注释"MVP 阶段取值 0/1"是业务
// 约定非 DB 约束），物理可存 0..99；UNIQUE 在 (user_id, is_default) 复合列上
// → 同 user 不同 is_default 值合法不冲突。本 case 须保持**同一 user**（equip
// 步骤 4/6 校验实例归属 + pet 归属均须 == in.UserID，单实例单 owner → 100 pet
// 必属同一 user，**不能**改 100 个 user 否则全 5002）；故用 is_default=i
// 构造 100 个合法 pet —— 与节点 9 "1 user 1 pet" 业务约束正交，本 case 纯测
// DB uk_user_cosmetic_item_id 兜底。
func TestCosmeticEquipServiceIntegration_Concurrent100SameInstanceDifferentPets_OnlyOneEquips(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900581)
	insertUser(t, rawDB, userID, "guest-equip-26-5-c2", "并发2测试用户", "")

	const N = 100
	petIDs := make([]uint64, N)
	for i := 0; i < N; i++ {
		petIDs[i] = uint64(700600 + i)
		// is_default = i（0..99）保证 uk_user_default_pet (user_id, is_default)
		// 不冲突；同 user（equip 归属校验须 == userID）。
		insertPet(t, rawDB, petIDs[i], userID, 1, "并发猫", 1, i)
	}

	// 1 件 hat 实例（status=1 in_bag）
	sameInst := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "hat_yellow"), 1)

	errs := make([]error, N)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.Equip(context.Background(), service.EquipParams{
				UserID: userID, PetID: petIDs[i], UserCosmeticItemID: sameInst,
			})
			errs[i] = err
		}()
	}
	close(start)
	wg.Wait()

	successCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
		}
	}
	if successCount != 1 {
		t.Fatalf("并发2 同实例 100 equip 到不同 pet: 成功 %d 个, want 恰 1（DB uk_user_cosmetic_item_id UNIQUE 兜底）", successCount)
	}

	// DB 终态：user_pet_equips 恰 1 行（指向 sameInst）+ 该实例 status=2
	assertCount(t, rawDB,
		"user_pet_equips WHERE user_cosmetic_item_id = ?", []any{sameInst}, 1,
		"并发2 后 user_pet_equips 恰 1 行（uk_user_cosmetic_item_id 兜底）")
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 2", []any{sameInst}, 1,
		"并发2 后该实例 status=2")

	assertEquipStateConsistency(t, rawDB, userID)
}

// ============================================================================
// Story 26.5 Task 5 — 3 个边界 case（AC9）
// ============================================================================

// TestCosmeticEquipServiceIntegration_EquipConsumedInstance_Returns5003（AC9，
// epics.md 行 3610）：实例 status=3 consumed → Equip → 5003
// （apperror.ErrCosmeticInvalidState）+ DB 状态不变。
func TestCosmeticEquipServiceIntegration_EquipConsumedInstance_Returns5003(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900591)
	const petID = uint64(700591)
	insertUser(t, rawDB, userID, "guest-equip-26-5-b1", "边界1测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	// status=3 consumed
	hatA := insertUserCosmeticItem(t, rawDB, userID, cosmeticIDByCode(t, rawDB, "hat_yellow"), 3)

	_, err := svc.Equip(context.Background(), service.EquipParams{
		UserID: userID, PetID: petID, UserCosmeticItemID: hatA,
	})
	requireEquipAppError(t, err, apperror.ErrCosmeticInvalidState, "边界1 Equip consumed 实例")

	// DB 状态不变（hatA 仍 status=3 + user_pet_equips 0 行）
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 3", []any{hatA}, 1,
		"边界1 后 hatA 仍 status=3 consumed")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petID}, 0,
		"边界1 后 user_pet_equips 0 行")
}

// TestCosmeticEquipServiceIntegration_EquipNotOwnedInstance_Returns5002（AC9，
// epics.md 行 3611）：实例属于 user B，user A Equip → 5002
// （apperror.ErrCosmeticNotOwned）+ DB 状态不变。
func TestCosmeticEquipServiceIntegration_EquipNotOwnedInstance_Returns5002(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userA = uint64(900601)
	const userB = uint64(900602)
	const petA = uint64(700601)
	insertUser(t, rawDB, userA, "guest-equip-26-5-b2-a", "边界2用户A", "")
	insertUser(t, rawDB, userB, "guest-equip-26-5-b2-b", "边界2用户B", "")
	insertPet(t, rawDB, petA, userA, 1, "用户A小猫", 1, 1)

	// 实例属于 user B
	hatB := insertUserCosmeticItem(t, rawDB, userB, cosmeticIDByCode(t, rawDB, "hat_yellow"), 1)

	// user A 用 user A 的 pet equip user B 的实例 → 5002
	_, err := svc.Equip(context.Background(), service.EquipParams{
		UserID: userA, PetID: petA, UserCosmeticItemID: hatB,
	})
	requireEquipAppError(t, err, apperror.ErrCosmeticNotOwned, "边界2 Equip 非本人实例")

	// DB 状态不变（hatB 仍 status=1 + user_pet_equips 0 行）
	assertCount(t, rawDB,
		"user_cosmetic_items WHERE id = ? AND status = 1", []any{hatB}, 1,
		"边界2 后 hatB 仍 status=1 in_bag")
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petA}, 0,
		"边界2 后 user_pet_equips 0 行")
}

// TestCosmeticEquipServiceIntegration_UnequipEmptySlot_Returns5004（AC9，
// epics.md 行 3612）：pet 无任何装备 → Unequip slot=1 → 5004
// （apperror.ErrCosmeticSlotMismatch）+ DB 状态不变。
func TestCosmeticEquipServiceIntegration_UnequipEmptySlot_Returns5004(t *testing.T) {
	svc, rawDB, cleanup := buildCosmeticEquipServiceIntegration(t)
	defer cleanup()

	const userID = uint64(900611)
	const petID = uint64(700611)
	insertUser(t, rawDB, userID, "guest-equip-26-5-b3", "边界3测试用户", "")
	insertPet(t, rawDB, petID, userID, 1, "默认小猫", 1, 1)

	// pet 无任何装备 → Unequip slot=1 → 5004（非幂等空槽显式报错）
	_, err := svc.Unequip(context.Background(), service.UnequipParams{
		UserID: userID, PetID: petID, Slot: 1,
	})
	requireEquipAppError(t, err, apperror.ErrCosmeticSlotMismatch, "边界3 Unequip 空 slot")

	// DB 状态不变（user_pet_equips 0 行）
	assertCount(t, rawDB,
		"user_pet_equips WHERE pet_id = ?", []any{petID}, 0,
		"边界3 后 user_pet_equips 0 行")
}
