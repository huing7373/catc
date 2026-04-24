// Package testing 提供跨 domain 共享的**测试 helper**。
//
// ⚠️ 包名注意：本包名是 `testing`，与 stdlib "testing" 同名。调用方**必须**用
// 别名 import，否则会覆盖 stdlib testing：
//
//	import testhelper "github.com/huing/cat/server/internal/pkg/testing"
//
// 约定别名为 `testhelper`（全 server 测试统一用该别名，避免风格漂移）。
//
// 路径选择：参考 Go 生态惯例（cobra / kubectl / go-chi 均使用 `pkg/testing/`），
// 本项目沿用同一布局。放 `pkg/` 而非 `infra/` 是因为：infra/ 是"外部设施接入"
// （db / redis / logger / metrics），test helper 属于"内部工具"，语义边界不同。
package testing

import (
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
)

// NewSQLMock 返回 (db, mock, cleanup)：
//
//	db, sqlMock, cleanup := testhelper.NewSQLMock(t)
//	_ = cleanup // 推荐用法：不调用 cleanup，交给 t.Cleanup 自动处理
//	sqlMock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow(1))
//	// ... 把 db 传给被测 repo / 裸 sql.QueryContext ...
//
// cleanup 已经在 t.Cleanup 注册，不需要 `defer cleanup()`。返回 cleanup 句柄
// 只是为了允许调用方**显式提前**关闭（例如在 subtest 里），绝大多数情况下
// 直接忽略即可。
//
// 行为：
//   - db.Close 在 test 结束时自动调一次
//   - mock.ExpectationsWereMet 在 cleanup 里检查；**未满足的期望会通过
//     t.Errorf 报告**（而非静默），避免 dev 忘记手动断言期望导致测试假绿
//
// ⚠️ 默认 sqlmock 使用 QueryMatcherRegexp（正则）匹配查询串。如果想字面量
// 匹配，调用 `sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))`
// 自建，不要用本 helper。
func NewSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	cleanup := func() {
		_ = db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sqlmock expectations not met: %v", err)
		}
	}
	t.Cleanup(cleanup)
	return db, mock, cleanup
}

// NewMiniRedis 启动一个 in-process miniredis server，返回 (server, addr)：
//
//	mr, addr := testhelper.NewMiniRedis(t)
//	// 用 addr 连接真实 redis client：
//	// client := redis.NewClient(&redis.Options{Addr: addr})
//	// 或用 mr 直接操作：mr.Set("k", "v") / mr.FlushAll / mr.FastForward(time.Minute)
//
// 自动在 t.Cleanup 关停 server，无需手动 Close。miniredis 无"期望未满足"语义，
// 只负责"响应命令 + 支持时间 FastForward"两件事。
//
// Epic 20 的宝箱倒计时 / Epic 4 的游客登录 Redis TTL 都可用
// `mr.FastForward(time.Minute)` 人为推进虚拟时间，不需要真 sleep。
//
// miniredis 默认只暴露 db 0（无多 db 概念）—— 业务代码**不应**基于多 db 设计
// key 布局（会在 miniredis 下跑通但在真 Redis / Epic 10+ 下行为不一致）。
func NewMiniRedis(t *testing.T) (*miniredis.Miniredis, string) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	return mr, mr.Addr()
}
