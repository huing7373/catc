package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// Story 20.6 — IdempotencyRepo sqlmock 单测（≥6 case）
//
// 与 chest_repo_test.go / room_member_repo_test.go 同模式：复用 newGormWithMock(t) helper。
// 覆盖 FindByUserIDAndKey (happy / NotFound / DB error) + ClaimPending (新行 / 已存在)
// + MarkSuccess (happy)。

// TestIdempotencyRepo_FindByUserIDAndKey_HappyPath:
// SELECT 命中 1 行 → repo 透传 IdempotencyRecord 字段填充。
func TestIdempotencyRepo_FindByUserIDAndKey_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewIdempotencyRepo(gormDB)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "idempotency_key", "status", "response_json",
		"created_at", "updated_at",
	}).AddRow(uint64(101), uint64(1001), "test_key_001", "success", []byte(`{"code":0}`), nil, nil)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `chest_open_idempotency_records`")).
		WithArgs(uint64(1001), "test_key_001", 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserIDAndKey(context.Background(), 1001, "test_key_001")
	if err != nil {
		t.Fatalf("FindByUserIDAndKey: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want non-nil IdempotencyRecord")
	}
	if got.ID != 101 {
		t.Errorf("ID = %d, want 101", got.ID)
	}
	if got.UserID != 1001 {
		t.Errorf("UserID = %d, want 1001", got.UserID)
	}
	if got.IdempotencyKey != "test_key_001" {
		t.Errorf("IdempotencyKey = %q, want test_key_001", got.IdempotencyKey)
	}
	if got.Status != IdempotencyStatusSuccess {
		t.Errorf("Status = %q, want %q", got.Status, IdempotencyStatusSuccess)
	}
	if string(got.ResponseJSON) != `{"code":0}` {
		t.Errorf("ResponseJSON = %q, want {\"code\":0}", string(got.ResponseJSON))
	}
}

// TestIdempotencyRepo_FindByUserIDAndKey_NotFound_ReturnsErrIdempotencyRecordNotFound:
// 查不到行 → repo 翻译 gorm.ErrRecordNotFound 为 ErrIdempotencyRecordNotFound 哨兵。
func TestIdempotencyRepo_FindByUserIDAndKey_NotFound_ReturnsErrIdempotencyRecordNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewIdempotencyRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id"}) // 0 行
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `chest_open_idempotency_records`")).
		WithArgs(uint64(9999), "absent_key", 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserIDAndKey(context.Background(), 9999, "absent_key")
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrIdempotencyRecordNotFound) {
		t.Errorf("err = %v, want ErrIdempotencyRecordNotFound", err)
	}
}

// TestIdempotencyRepo_FindByUserIDAndKey_DBError_Propagates:
// driver-level DB error → repo 透传 raw error。
func TestIdempotencyRepo_FindByUserIDAndKey_DBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewIdempotencyRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `chest_open_idempotency_records`")).
		WithArgs(uint64(1001), "k", 1).
		WillReturnError(wantErr)

	got, err := repo.FindByUserIDAndKey(context.Background(), 1001, "k")
	if got != nil {
		t.Errorf("got = %+v, want nil on DB error", got)
	}
	if err == nil || err.Error() != wantErr.Error() {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if stderrors.Is(err, ErrIdempotencyRecordNotFound) {
		t.Errorf("err should not match ErrIdempotencyRecordNotFound sentinel for DB error path")
	}
}

// TestIdempotencyRepo_ClaimPending_NewRow_AffectedRows1:
// INSERT 触发 → affected_rows=1 = 新行 INSERT 生效。
func TestIdempotencyRepo_ClaimPending_NewRow_AffectedRows1(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewIdempotencyRepo(gormDB)

	// sqlmock 用 RawQueryMatcher 匹配 INSERT ... ON DUPLICATE KEY UPDATE
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO chest_open_idempotency_records")).
		WithArgs(uint64(1001), "key_001", IdempotencyStatusPending).
		WillReturnResult(sqlmock.NewResult(0, 1))

	affected, err := repo.ClaimPending(context.Background(), 1001, "key_001")
	if err != nil {
		t.Fatalf("ClaimPending: %v", err)
	}
	if affected != 1 {
		t.Errorf("affectedRows = %d, want 1 (新行 INSERT)", affected)
	}
}

// TestIdempotencyRepo_ClaimPending_ExistingRow_AffectedRows0:
// 行已存在（首事务已 commit）→ ON DUPLICATE 触发 no-op update → affected_rows=0。
func TestIdempotencyRepo_ClaimPending_ExistingRow_AffectedRows0(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewIdempotencyRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO chest_open_idempotency_records")).
		WithArgs(uint64(1001), "key_001", IdempotencyStatusPending).
		WillReturnResult(sqlmock.NewResult(0, 0))

	affected, err := repo.ClaimPending(context.Background(), 1001, "key_001")
	if err != nil {
		t.Fatalf("ClaimPending: %v", err)
	}
	if affected != 0 {
		t.Errorf("affectedRows = %d, want 0 (行已存在，no-op ON DUPLICATE)", affected)
	}
}

// TestIdempotencyRepo_MarkSuccess_HappyPath:
// UPDATE status='success' + response_json → rows_affected=1 → nil error。
// GORM Updates(map) 会自动追加 updated_at 字段（因为 IdempotencyRecord 含 UpdatedAt 字段；
// 哪怕没有 autoUpdateTime tag，GORM 还是把它当 "标准 audit 字段" 自动维护），故 args 是 5 个。
// 用 sqlmock.AnyArg() 兜底 updated_at 时间戳（避免抖动）。
func TestIdempotencyRepo_MarkSuccess_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewIdempotencyRepo(gormDB)

	responseJSON := []byte(`{"code":0,"data":{"reward":{"cosmeticItemId":"24"}}}`)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `chest_open_idempotency_records`")).
		WithArgs(responseJSON, IdempotencyStatusSuccess, sqlmock.AnyArg(), uint64(1001), "key_001").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.MarkSuccess(context.Background(), 1001, "key_001", responseJSON)
	if err != nil {
		t.Fatalf("MarkSuccess: %v", err)
	}
}

// TestIdempotencyRepo_MarkSuccess_NoRowsAffected_ReturnsErrIdempotencyRecordNotFound:
// rows_affected=0（理论不应发生 —— 同事务前面 ClaimPending 必已 INSERT）→ ErrIdempotencyRecordNotFound 哨兵。
func TestIdempotencyRepo_MarkSuccess_NoRowsAffected_ReturnsErrIdempotencyRecordNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewIdempotencyRepo(gormDB)

	responseJSON := []byte(`{"code":0}`)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `chest_open_idempotency_records`")).
		WithArgs(responseJSON, IdempotencyStatusSuccess, sqlmock.AnyArg(), uint64(1001), "key_001").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.MarkSuccess(context.Background(), 1001, "key_001", responseJSON)
	if !stderrors.Is(err, ErrIdempotencyRecordNotFound) {
		t.Errorf("err = %v, want ErrIdempotencyRecordNotFound", err)
	}
}
