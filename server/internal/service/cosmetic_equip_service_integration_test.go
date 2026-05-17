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
// **深度回滚 / 100 并发兜底 / 状态一致性矩阵归 Story 26.5**（本 story AC 范围
// 红线 —— 本文件仅 2 个 happy + 同槽换装场景，epics.md 行 3592-3616 钦定）。

package service_test

import (
	"context"
	"database/sql"
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
