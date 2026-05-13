package mysql

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// Story 17.4 — EmojiRepo.List sqlmock 单测（≥3 case）
//
// 与 room_member_repo_test.go 同模式：复用 newGormWithMock(t) helper
// （user_repo_test.go 中定义；同 package 直接调）。
//
// SQL 字面量与 GORM 生成对齐：
//   - GORM .Select("id, code, name, asset_url, sort_order")
//     .Where("is_enabled = ?", 1)
//     .Order("sort_order ASC, id ASC")
//     .Find(&rows)
//   - GORM 实际生成 SQL（base table name 反引号化）：
//     SELECT id, code, name, asset_url, sort_order FROM `emoji_configs`
//       WHERE is_enabled = ? ORDER BY sort_order ASC, id ASC
//
// V1 §11.1 钦定的字段集 + WHERE / ORDER 1:1 对齐。

// TestEmojiRepo_List_HappyPath_4Rows：4 行 enabled 表情 → repo 透传 4 行 EmojiConfig，
// 排序按 sort_order ASC。
func TestEmojiRepo_List_HappyPath_4Rows(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewEmojiRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id", "code", "name", "asset_url", "sort_order"}).
		AddRow(uint64(1), "wave", "挥手", "https://placehold.co/64x64?text=Wave", int32(1)).
		AddRow(uint64(2), "love", "爱心", "https://placehold.co/64x64?text=Love", int32(2)).
		AddRow(uint64(3), "laugh", "大笑", "https://placehold.co/64x64?text=Laugh", int32(3)).
		AddRow(uint64(4), "cry", "哭", "https://placehold.co/64x64?text=Cry", int32(4))

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, code, name, asset_url, sort_order FROM `emoji_configs` WHERE is_enabled = ? ORDER BY sort_order ASC, id ASC",
	)).
		WithArgs(1).
		WillReturnRows(rows)

	got, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}
	if got[0].Code != "wave" || got[1].Code != "love" || got[2].Code != "laugh" || got[3].Code != "cry" {
		t.Errorf("codes = %v %v %v %v, want wave/love/laugh/cry",
			got[0].Code, got[1].Code, got[2].Code, got[3].Code)
	}
	// 字段值断言（防 GORM Scan 字段映射回归）
	if got[0].AssetURL != "https://placehold.co/64x64?text=Wave" {
		t.Errorf("got[0].AssetURL = %q, want %q", got[0].AssetURL, "https://placehold.co/64x64?text=Wave")
	}
	if got[0].SortOrder != 1 {
		t.Errorf("got[0].SortOrder = %d, want 1", got[0].SortOrder)
	}
}

// TestEmojiRepo_List_EmptyResult_ReturnsEmptySlice：0 行（无 enabled 表情）
// → 返 []EmojiConfig{}（非 nil 切片）。
//
// 防 nil slice 回归：service 层在 Brief 转换时用 make + append 路径，repo 也兜底
// 保证调用方不需要 nil-check。
func TestEmojiRepo_List_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewEmojiRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, code, name, asset_url, sort_order FROM `emoji_configs` WHERE is_enabled = ? ORDER BY sort_order ASC, id ASC",
	)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "asset_url", "sort_order"})) // 0 行

	got, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []EmojiConfig{}")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// TestEmojiRepo_List_DBError_ReturnsRawError：DB 错误（连接断 / 慢查询超时等）
// → 返 raw error 透传给 service 层（service 包成 1009）。
//
// 用 fakeDBError 模拟 driver-level error；与 step_account_repo_test 同模式。
func TestEmojiRepo_List_DBError_ReturnsRawError(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewEmojiRepo(gormDB)

	dbErr := fakeEmojiDBError()
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, code, name, asset_url, sort_order FROM `emoji_configs` WHERE is_enabled = ? ORDER BY sort_order ASC, id ASC",
	)).
		WithArgs(1).
		WillReturnError(dbErr)

	got, err := repo.List(context.Background())
	if err == nil {
		t.Fatal("err == nil, want raw DB error")
	}
	if got != nil {
		t.Errorf("got != nil on error, want nil slice")
	}
}

// fakeEmojiDBError 返回一个 driver-level error，模拟连接断 / 慢查询超时等场景。
// 命名带 emoji 前缀避免与同 package 其他 helper 冲突。
func fakeEmojiDBError() error {
	return &emojiMockDBErr{msg: "fake DB connection lost"}
}

type emojiMockDBErr struct{ msg string }

func (e *emojiMockDBErr) Error() string { return e.msg }

// ============================================================================
// Story 17.5 — EmojiRepo.Exists sqlmock 单测（≥3 case）
// ============================================================================
//
// SQL 字面量与 GORM 生成对齐：
//   - GORM .Model(&EmojiConfig{}).Select("1")
//     .Where("code = ? AND is_enabled = ?", code, 1)
//     .Limit(1)
//     .Find(&dummy)
//   - GORM 实际生成 SQL（base table name 反引号化）：
//     SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?
//
// V1 §12.2 服务端逻辑步骤 4 钦定的语义对齐：
//   - is_enabled = 1 过滤
//   - LIMIT 1
//   - 0 行 → (false, nil)；1 行 → (true, nil)；DB err → (false, err)

// TestEmojiRepo_Exists_HappyPath_EnabledRowFound：DB 中有 enabled 行 → 返 (true, nil)
func TestEmojiRepo_Exists_HappyPath_EnabledRowFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewEmojiRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?",
	)).
		WithArgs("wave", 1, 1).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	got, err := repo.Exists(context.Background(), "wave")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !got {
		t.Errorf("got = false, want true (wave is enabled in fixture)")
	}
}

// TestEmojiRepo_Exists_NotFound_ReturnsFalse：code 不存在 → 返 (false, nil)
func TestEmojiRepo_Exists_NotFound_ReturnsFalse(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewEmojiRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?",
	)).
		WithArgs("nonexistent", 1, 1).
		WillReturnRows(sqlmock.NewRows([]string{"1"})) // 0 行

	got, err := repo.Exists(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if got {
		t.Errorf("got = true, want false (code not in DB)")
	}
}

// TestEmojiRepo_Exists_DisabledRow_ReturnsFalse：DB 有 is_enabled=0 行，但
// WHERE is_enabled=1 过滤后 0 行匹配 → (false, nil)。验证"合并语义"：disabled
// 与"完全不存在"对 caller 返回值相同（§12.2 服务端逻辑步骤 4 钦定）。
func TestEmojiRepo_Exists_DisabledRow_ReturnsFalse(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewEmojiRepo(gormDB)

	// DB 即使有 code='secret' 的行但 is_enabled=0 → query 走 WHERE is_enabled=1 不命中
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?",
	)).
		WithArgs("secret", 1, 1).
		WillReturnRows(sqlmock.NewRows([]string{"1"})) // 0 行（is_enabled=1 过滤已生效）

	got, err := repo.Exists(context.Background(), "secret")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if got {
		t.Errorf("got = true, want false (disabled rows filtered by is_enabled=1)")
	}
}

// TestEmojiRepo_Exists_DBError_ReturnsRawError：DB 错误 → 返 (false, raw err)
func TestEmojiRepo_Exists_DBError_ReturnsRawError(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewEmojiRepo(gormDB)

	dbErr := fakeEmojiDBError()
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT 1 FROM `emoji_configs` WHERE code = ? AND is_enabled = ? LIMIT ?",
	)).
		WithArgs("wave", 1, 1).
		WillReturnError(dbErr)

	got, err := repo.Exists(context.Background(), "wave")
	if err == nil {
		t.Fatal("err == nil, want raw DB error")
	}
	if got {
		t.Errorf("got = true on error, want false")
	}
}
