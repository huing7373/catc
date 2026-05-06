package bootstrap

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	driverMysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
	"github.com/huing/cat/server/internal/repo/tx"
)

// router_ws_test 锁 Story 10.3 review r6 [P2] 后的契约：
//
// **r5 形态（前一轮）**：wsTablesReady 用 `SELECT COUNT(*) FROM information_schema.tables
// WHERE table_schema = DATABASE() AND table_name IN ('rooms','room_members')` 单 query
// 走 information_schema sniff，命中数 < 2 panic；query 出错也 panic。
//
// **r6 [P2] 反指**：prod hardened DB user 通常**没有** information_schema 访问权
// （最小权限原则：只授权 app schema），导致 wsTablesReady 在生产部署 panic →
// 整个 HTTP server 启动失败（不只是 /ws，所有端点 /ping / /version / 业务 API 都挂）。
//
// **r6 修法**（Option A，直 probe + error 分流）：
//   - 改用 `SELECT 1 FROM rooms LIMIT 1` / `SELECT 1 FROM room_members LIMIT 1` 直接
//     探表（走和 RoomExists 同一条 app schema 权限路径）
//   - err == nil（query 成功，含空表）→ 表存在 → continue
//   - MySQL 1146 (ER_NO_SUCH_TABLE) → 真的缺表 → 返 false → 调用方 panic（fail-fast 仍成立）
//   - 其他 err（权限 1142 / 连接断 / 等）→ log warn + continue（视为表存在但当前 probe
//     失败）；让后续 request 阶段 RoomExists 自然 fail 走 documented close 1011，
//     **不**在启动期假阳性 panic
//
// 本测试覆盖六个分支：
//   - 表都存在（两次 probe 均空表 OK）→ 路由挂载（NewRouter 正常返回 + 非 404）
//   - rooms 表不存在（MySQL 1146）→ NewRouter panic
//   - room_members 表不存在（rooms OK，members 1146）→ NewRouter panic
//   - 两表都不存在（rooms 1146）→ NewRouter panic（在 rooms probe 即返 false）
//   - rooms probe 拿 MySQL 1142（access denied，模拟 hardened DB user）→ **不** panic
//   - rooms probe 拿非 mysql error（连接断）→ **不** panic
//
// 后两个 case 是 r6 修复的核心防回归 case：r5 形态下这两种错误都会 panic，让
// hardened deployment 不可用；r6 改为视为"表存在"，让请求阶段处理。
//
// 不用真实 mysql 容器：sqlmock 注入 GORM，期望 SELECT 1 query 返回不同结果 / error
// 即可锁住 wsTablesReady 行为。

// newRouterWSTestDeps 构造测试 Deps，包含挂载 WS 路由所需的所有非 nil 字段
// （GormDB / TxMgr / Signer / RateLimitCfg / SessionMgr / WSCfg / EnvName）。
//
// gormDB 由调用方注入 sqlmock 后传入；本 helper 装填其他字段。
func newRouterWSTestDeps(t *testing.T, gormDB *gorm.DB) Deps {
	t.Helper()
	signer, err := auth.New("router-ws-test-secret-32-bytes-min-len", 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	perKey := int64(60)
	burst := int64(60)
	buckets := int64(100)
	return Deps{
		GormDB: gormDB,
		TxMgr:  tx.NewManager(gormDB),
		Signer: signer,
		RateLimitCfg: config.RateLimitConfig{
			PerKeyPerMin: &perKey,
			BurstSize:    &burst,
			BucketsLimit: &buckets,
		},
		EnvName:    "test",
		SessionMgr: wsapp.NewSessionManager(),
		WSCfg: config.WSConfig{
			HeartbeatTimeoutSec: 60,
			MaxMessageSizeBytes: 16384,
			WriteTimeoutSec:     5,
		},
	}
}

// newGormWithMockForRouter 复用 mysql/user_repo_test.go 范式注入 sqlmock。
//
// SkipInitializeWithVersion / SkipDefaultTransaction：避免 GORM Open 阶段
// 触发未期望的 SELECT VERSION() / 自动事务包裹，与 tx/manager_test.go 一致。
func newGormWithMockForRouter(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		// **不**调 ExpectationsWereMet：NewRouter 内部会触发 4.6 / 4.8 / 7.x
		// 各业务路由构造期的 repo 初始化（如果有的话）+ wsTablesReady 的 sniff
		// 查询；本测试只想锁住 wsTablesReady 行为，对其他可能出现的查询保持
		// 宽松。
	})
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return gormDB, mock
}

// expectTableProbeOK 配置 sqlmock 让 `SELECT 1 FROM <table> LIMIT 1` 返回空 rows
// （表存在但空表）→ wsTablesReady 视为表 OK，continue。
//
// sqlmock 的 ExpectQuery 配置只需匹配 query 字符串；返回空 rows 时 GORM .Scan 返
// nil error（仅 row 不存在），与"空表"语义一致。
func expectTableProbeOK(mock sqlmock.Sqlmock, table string) {
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", table))).
		WillReturnRows(sqlmock.NewRows([]string{"1"}))
}

// expectTableProbeNoSuchTable 模拟 MySQL ER_NO_SUCH_TABLE (1146)：表不存在。
// wsTablesReady 应识别为 schema drift → 返 false → 调用方 panic。
func expectTableProbeNoSuchTable(mock sqlmock.Sqlmock, table string) {
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", table))).
		WillReturnError(&driverMysql.MySQLError{
			Number:  1146,
			Message: fmt.Sprintf("Table 'testdb.%s' doesn't exist", table),
		})
}

// expectTableProbeAccessDenied 模拟 MySQL ER_TABLEACCESS_DENIED_ERROR (1142)：
// hardened DB user 没有该表 SELECT 权限。这是 r6 [P2] 修复要避免的"假阳性 panic"
// 场景 —— wsTablesReady 应 log warn + continue（视为表存在），**不** panic。
func expectTableProbeAccessDenied(mock sqlmock.Sqlmock, table string) {
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", table))).
		WillReturnError(&driverMysql.MySQLError{
			Number:  1142,
			Message: fmt.Sprintf("SELECT command denied to user 'app'@'%%' for table '%s'", table),
		})
}

// expectTableProbeConnError 模拟非 mysql.MySQLError 的错误（连接断 / context 取消等）。
// wsTablesReady 应 log warn + continue（视为表存在）→ **不** panic。
func expectTableProbeConnError(mock sqlmock.Sqlmock, table string) {
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", table))).
		WillReturnError(stderrors.New("driver: bad connection"))
}

// TestRouter_WSRouteMounted_WhenTablesExist:
// rooms / room_members 两表均存在（probe 返空 rows OK）→ /ws/rooms/3001 路由挂载 →
// 非 404 响应。
//
// **不**做真实 WS 握手（避免拉满 gorilla.Upgrader / hijack 路径）；只验证
// 路由 dispatch 命中（任何非 404 状态都说明命中了 gateway.Handle）。
func TestRouter_WSRouteMounted_WhenTablesExist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeOK(mock, "rooms")
	expectTableProbeOK(mock, "room_members")

	r := NewRouter(newRouterWSTestDeps(t, gormDB))

	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/3001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Errorf("status = 404, want non-404 (gateway.Handle should be reached); body=%s", w.Body.String())
	}
}

// TestRouter_WSRouteFailFast_WhenRoomsTableMissing:
// rooms 表不存在（MySQL 1146）→ NewRouter panic（fail-fast schema drift）。
//
// 第一个 probe 即返 1146 → wsTablesReady 立即 return false（不再继续探 room_members）→
// 调用方 panic。
func TestRouter_WSRouteFailFast_WhenRoomsTableMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeNoSuchTable(mock, "rooms")

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("NewRouter did not panic when rooms table missing (fail-fast schema drift contract violated)")
		}
		msg, _ := rec.(string)
		if !strings.Contains(msg, "ws backing tables missing") {
			t.Errorf("panic msg = %q, want contain 'ws backing tables missing'", msg)
		}
	}()
	NewRouter(newRouterWSTestDeps(t, gormDB))
}

// TestRouter_WSRouteFailFast_WhenRoomMembersTableMissing:
// rooms OK + room_members 1146 → NewRouter panic。
//
// 验证 wsTablesReady 在第二次 probe 才发现缺表的场景。
func TestRouter_WSRouteFailFast_WhenRoomMembersTableMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeOK(mock, "rooms")
	expectTableProbeNoSuchTable(mock, "room_members")

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("NewRouter did not panic when room_members table missing")
		}
		msg, _ := rec.(string)
		if !strings.Contains(msg, "ws backing tables missing") {
			t.Errorf("panic msg = %q, want contain 'ws backing tables missing'", msg)
		}
	}()
	NewRouter(newRouterWSTestDeps(t, gormDB))
}

// TestRouter_WSRouteFailFast_WhenBothTablesMissing:
// 两表都不存在（rooms 1146 即返 false，room_members probe 不会执行）→ NewRouter panic。
//
// 这是 r5 P1 描述的"prod 静默 404"场景被替换为"prod 启动期立即 panic"的核心防回归 case：
// schema 漂移让运维通过 CrashLoopBackOff 立即看到，而不是 server 起来后客户端拿 404 /
// close 1011 才间接发现。
func TestRouter_WSRouteFailFast_WhenBothTablesMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeNoSuchTable(mock, "rooms")

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("NewRouter did not panic when both tables missing")
		}
		msg, _ := rec.(string)
		if !strings.Contains(msg, "ws backing tables missing") {
			t.Errorf("panic msg = %q, want contain 'ws backing tables missing'", msg)
		}
	}()
	NewRouter(newRouterWSTestDeps(t, gormDB))
}

// TestRouter_WSRoute_DoesNotPanic_OnAccessDenied:
// rooms probe 返 MySQL 1142（hardened DB user 没有 SELECT 权限）→
// wsTablesReady 视为表存在 → **不** panic → 路由仍挂载（让 request 阶段
// RoomExists 自然处理）。
//
// 这是 r6 [P2] 修复的核心防回归 case：r5 形态下 information_schema query 在 hardened
// DB user 下会失败 → panic → 整个 HTTP server 启动挂。r6 改用直 probe + error 分流：
// 非 1146 错误（含权限错）→ log warn + continue。
//
// rooms 的 probe 1142 后仍会探 room_members，故配两条 expect。
func TestRouter_WSRoute_DoesNotPanic_OnAccessDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeAccessDenied(mock, "rooms")
	expectTableProbeAccessDenied(mock, "room_members")

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("NewRouter panicked on access-denied probe (r6 P2 contract violated): %v", rec)
		}
	}()

	r := NewRouter(newRouterWSTestDeps(t, gormDB))
	// 路由仍挂载：probe 失败不阻断 NewRouter 完成；request 阶段 RoomExists 才会失败。
	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/3001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Errorf("status = 404, want non-404 (route should still be mounted despite probe access-denied); body=%s", w.Body.String())
	}
}

// TestRouter_WSRoute_DoesNotPanic_OnConnError:
// rooms probe 返非 mysql error（如 driver: bad connection）→ wsTablesReady 视为
// 表存在 → **不** panic（同上 access-denied case，验证非 mysql error 路径）。
//
// 与 access-denied case 的差别：access-denied 是 *mysql.MySQLError（Number != 1146），
// conn error 是普通 error（errors.As → false）。两者都应走 log warn + continue。
func TestRouter_WSRoute_DoesNotPanic_OnConnError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeConnError(mock, "rooms")
	expectTableProbeConnError(mock, "room_members")

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("NewRouter panicked on conn-error probe (r6 P2 contract violated): %v", rec)
		}
	}()

	r := NewRouter(newRouterWSTestDeps(t, gormDB))
	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/3001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Errorf("status = 404, want non-404 (route should still be mounted despite probe conn error); body=%s", w.Body.String())
	}
}
