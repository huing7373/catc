package testing_test

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testhelper "github.com/huing/cat/server/internal/pkg/testing"
)

// TestNewSQLMock_SmokePath 验证 NewSQLMock：
//   - 返回的 *sql.DB 可以真正执行带 mock 期望的 Query
//   - cleanup 在 t.Cleanup 自动调用时，ExpectationsWereMet 成功通过
//
// 本测试同时示范 dev 在真实业务测试里应当怎么用本 helper：
//
//	db, mock, _ := testhelper.NewSQLMock(t)
//	mock.ExpectQuery(...).WillReturnRows(...)
//	// 跑被测代码
func TestNewSQLMock_SmokePath(t *testing.T) {
	db, mock, _ := testhelper.NewSQLMock(t)

	mock.ExpectQuery("SELECT v FROM t").
		WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow(42))

	rows, err := db.QueryContext(context.Background(), "SELECT v FROM t")
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next(), "expected one row")
	var v int
	require.NoError(t, rows.Scan(&v))
	assert.Equal(t, 42, v)
	assert.False(t, rows.Next(), "should have exactly one row")
}

// TestNewMiniRedis_SmokePath 验证 NewMiniRedis：
//   - 返回的 Miniredis 可以通过返回的 addr 被外部 TCP 连接（基本用例：使用 miniredis API）
//   - server 在 t.Cleanup 自动关停
//   - 支持 FastForward（Epic 20 宝箱倒计时 / Epic 4 TTL 测试的关键能力）
func TestNewMiniRedis_SmokePath(t *testing.T) {
	mr, addr := testhelper.NewMiniRedis(t)
	require.NotEmpty(t, addr, "addr should be non-empty")

	// 直接通过 mr 写入，避免本 smoke test 引入 redis client 依赖
	require.NoError(t, mr.Set("k", "v"))

	got, err := mr.Get("k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)

	// FastForward 推进虚拟时间（TTL 场景关键能力）
	mr.SetTTL("k", time.Second)
	mr.FastForward(2 * time.Second)
	// FastForward 后 key 应已过期
	_, err = mr.Get("k")
	require.Error(t, err, "expected miniredis.ErrKeyNotFound after TTL forward")
}
