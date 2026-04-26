package tx

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// newGormWithMock 构造 sqlmock-backed *gorm.DB，用 GORM mysql driver 的
// `New(mysql.Config{Conn: sqlMockDB})` 注入 sqlmock 模拟连接。
//
// 关键点：
//   - DisableAutomaticPing 必须开启，否则 gorm.Open 会主动 ping 一次 → sqlmock 没设
//     ExpectPing 期望就会失败。
//   - SkipDefaultTransaction 与 prod 实装（internal/infra/db/mysql.go）保持一致，
//     避免单条 SQL 自动包事务影响断言。
func newGormWithMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sqlmock expectations not met: %v", err)
		}
	})

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true, // 不让 GORM 在 Open 阶段查 SELECT VERSION()
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return gormDB, mock
}

// TestWithTx_Commit_OnNilReturn 验证 fn 返回 nil 时事务正常 commit：
// sqlmock 期望 BEGIN + COMMIT 序列。
//
// 不断言具体的 INSERT/UPDATE 文本（按 ADR-0001 §3.1 已知坑：sqlmock 对 GORM
// 自动生成的 SQL 用正则匹配较脆，单元测试只断言事务边界）。
func TestWithTx_Commit_OnNilReturn(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	mgr := NewManager(gormDB)

	mock.ExpectBegin()
	mock.ExpectCommit()

	err := mgr.WithTx(context.Background(), func(txCtx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx returned %v, want nil", err)
	}
}

// TestWithTx_Rollback_OnError 验证 fn 返回非 nil error 时事务 rollback，
// 且原 error 透传给调用方（不被 GORM / WithTx 包一层）。
func TestWithTx_Rollback_OnError(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	mgr := NewManager(gormDB)

	mock.ExpectBegin()
	mock.ExpectRollback()

	wantErr := errors.New("force rollback")
	err := mgr.WithTx(context.Background(), func(txCtx context.Context) error {
		return wantErr
	})
	if err == nil {
		t.Fatalf("WithTx returned nil, want %v", wantErr)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("WithTx error = %v, want errors.Is(err, %v) == true", err, wantErr)
	}
}

// TestWithTx_TxCtxPropagates 验证 fn 拿到的 txCtx 与外层 ctx 不同：
//   - FromContext(txCtx, fallback) 返回 GORM 注入的 *gorm.Tx 句柄
//   - 这个句柄不等于 fallback（外层 db） → 表示 fn 内的 repo 调用会走 tx 而非 db pool
//
// 这是 ADR-0007 §2.4 钦定行为的单元测试 contract：fn 内必须用 txCtx 才能进事务。
func TestWithTx_TxCtxPropagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	mgr := NewManager(gormDB)

	mock.ExpectBegin()
	mock.ExpectCommit()

	var txHandle *gorm.DB
	err := mgr.WithTx(context.Background(), func(txCtx context.Context) error {
		txHandle = FromContext(txCtx, gormDB)
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx returned %v, want nil", err)
	}
	if txHandle == nil {
		t.Fatalf("FromContext returned nil inside WithTx")
	}
	if txHandle == gormDB {
		t.Errorf("FromContext inside WithTx returned the outer db handle, want a tx-bound *gorm.DB")
	}
}

// TestFromContext_NoTx_ReturnsFallback 验证 ctx 里没有 tx 时 FromContext 返回 fallback：
// 这是 repo 层"在事务内 / 不在事务内都能用 FromContext"的关键 contract。
//
// 不需要 sqlmock —— 纯 context.Value 行为测试，gormDB 句柄只用作身份比较。
func TestFromContext_NoTx_ReturnsFallback(t *testing.T) {
	gormDB, _ := newGormWithMock(t)

	got := FromContext(context.Background(), gormDB)
	if got != gormDB {
		t.Errorf("FromContext(ctx_without_tx, fallback) returned a different handle, want fallback")
	}
}

// TestFromContext_WithTxKey_NilFallback_ReturnsFallback 边缘 case：ctx 里
// 装了 nil tx → FromContext 必须 fallthrough 到 fallback（避免业务层拿到 nil 的 *gorm.DB
// 然后 NPE）。
//
// 这条 case 防御 future maintenance：如果未来某个 helper 用 context.WithValue(ctx, txKey{}, nil)
// 设了 nil 值，FromContext 不能返回 nil。
func TestFromContext_WithNilTxValue_ReturnsFallback(t *testing.T) {
	gormDB, _ := newGormWithMock(t)

	ctx := context.WithValue(context.Background(), txKey{}, (*gorm.DB)(nil))
	got := FromContext(ctx, gormDB)
	if got != gormDB {
		t.Errorf("FromContext with nil tx value returned %v, want fallback (%v)", got, gormDB)
	}
}
