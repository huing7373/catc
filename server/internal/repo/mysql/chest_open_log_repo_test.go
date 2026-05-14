package mysql

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// Story 20.6 — ChestOpenLogRepo.Create sqlmock 单测（≥1 case）

// TestChestOpenLogRepo_Create_HappyPath: 事务内 INSERT 一行 chest_open_logs。
// 验证 GORM 调用 INSERT + LastInsertId 回填到 log.ID。
func TestChestOpenLogRepo_Create_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestOpenLogRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `chest_open_logs`")).
		WillReturnResult(sqlmock.NewResult(7001, 1))

	log := &ChestOpenLog{
		UserID:                   1001,
		ChestID:                  5001,
		CostSteps:                1000,
		RewardUserCosmeticItemID: 0, // 节点 7 占位
		RewardCosmeticItemID:     24,
		RewardRarity:             2,
	}
	if err := repo.Create(context.Background(), log); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if log.ID != 7001 {
		t.Errorf("log.ID = %d, want 7001 (回填)", log.ID)
	}
}
