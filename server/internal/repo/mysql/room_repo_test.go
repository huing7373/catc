package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestRoomRepo_Create_AssignsAutoIncrementID:
// INSERT rooms → sqlmock 返 LastInsertId=3001 → 验证 r.ID 被 GORM 回填。
func TestRoomRepo_Create_AssignsAutoIncrementID(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `rooms`")).
		WillReturnResult(sqlmock.NewResult(3001, 1))

	r := &Room{
		CreatorUserID: 1001,
		Status:        1,
		MaxMembers:    4,
	}
	if err := repo.Create(context.Background(), r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.ID != 3001 {
		t.Errorf("r.ID = %d, want 3001 (回填的 LastInsertId)", r.ID)
	}
}

// TestRoomRepo_Create_DBError_Propagates:
// INSERT rooms 触发任意非 1062 DB error → repo 透传 raw error。
//
// rooms 表只有 PRIMARY KEY 唯一约束（AUTO_INCREMENT 不会冲突），无业务唯一约束 →
// 1062 路径理论不可达；本测试覆盖一般 DB error 透传路径（service 包成 1009）。
func TestRoomRepo_Create_DBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `rooms`")).
		WillReturnError(wantErr)

	err := repo.Create(context.Background(), &Room{
		CreatorUserID: 1001,
		Status:        1,
		MaxMembers:    4,
	})
	if !stderrors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr (raw DB error 应原样透传给 service)", err)
	}
}
