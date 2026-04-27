package mysql

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestChestRepo_Create_StoresUTCTime:
// 验证 chest.UnlockAt 使用 UTC 时区时 INSERT 成功 + repo 回填 ID。
// 这是 V1 §2.5 钦定 ISO 8601 UTC 在 service 层的延伸校验：service 必须用
// time.Now().UTC()，repo 只做 CRUD 不再二次校验时区。
func TestChestRepo_Create_StoresUTCTime(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_chests`")).
		WillReturnResult(sqlmock.NewResult(3001, 1))

	utcUnlock := time.Now().UTC().Add(10 * time.Minute)
	c := &UserChest{
		UserID:        1001,
		Status:        1,
		UnlockAt:      utcUnlock,
		OpenCostSteps: 1000,
		Version:       0,
	}
	if err := repo.Create(context.Background(), c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.ID != 3001 {
		t.Errorf("c.ID = %d, want 3001", c.ID)
	}
	// 关键：UnlockAt 仍保持 UTC（repo Create 不应改时区）
	if loc := c.UnlockAt.Location(); loc.String() != "UTC" {
		t.Errorf("c.UnlockAt location = %q, want UTC", loc.String())
	}
}
