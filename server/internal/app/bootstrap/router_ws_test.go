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

// router_ws_test 锁 Story 10.3 review r8 [P1] 后的契约（覆盖 r6 → r8 的 error 分流演进）：
//
// **r5 形态**：wsTablesReady 用 information_schema sniff，缺表 / query 出错都 panic。
//
// **r6 [P2] 反指**：hardened DB user 没有 information_schema 权限 → 启动假阳性 panic。
// r6 修法：改直 probe app table（`SELECT 1 FROM rooms LIMIT 1`）+ 把所有非 1146
// 错误都视为 transient warn-and-continue。
//
// **r8 [P1] 反指**：r6 把 1142 / 1044 也归到"transient warn"是错的。直 probe app table
// 路径下 1142 (ER_TABLEACCESS_DENIED_ERROR) / 1044 (ER_DBACCESS_DENIED_ERROR) 不再
// 是 information_schema 副作用，而是**真的 misconfig** —— DB role 连得上但 SELECT
// rooms/room_members 都被拒，每次 WS 握手都会 close 1011，feature 完全不可用而
// healthcheck 看着健康 = 静默灾难。这种应该启动期立即 fail-fast。
//
// **r8 修法**（error 分流收窄）：
//   - err == nil → continue
//   - 1146 (ER_NO_SUCH_TABLE) → return false → 调用方 panic（schema drift fail-fast）
//   - 1142 (ER_TABLEACCESS_DENIED_ERROR) → return false → panic（permission misconfig）
//   - 1044 (ER_DBACCESS_DENIED_ERROR) → return false → panic（schema-level access denied）
//   - 其他 mysql err（如 1040 too-many-connections，transient）→ log warn + continue
//   - 非 *mysql.MySQLError（如 driver-level "bad connection"）→ log warn + continue
//
// 本测试覆盖以下分支：
//   - 表都存在（两次 probe 均空表 OK）→ 路由挂载（NewRouter 正常返回 + 非 404）
//   - rooms 表不存在（MySQL 1146）→ NewRouter panic
//   - room_members 表不存在（rooms OK，members 1146）→ NewRouter panic
//   - 两表都不存在（rooms 1146）→ NewRouter panic（在 rooms probe 即返 false）
//   - rooms probe 拿 MySQL 1142（access denied）→ NewRouter panic（r8 [P1] 新契约）
//   - rooms probe 拿 MySQL 1044（DB access denied）→ NewRouter panic（r8 [P1] 新契约）
//   - rooms probe 拿 MySQL 1040（too many connections，transient）→ **不** panic
//   - rooms probe 拿非 mysql error（连接断）→ **不** panic
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
// DB role 连得上但没有该表 SELECT 权限。
//
// **r8 [P1] 契约变更**：r6 时这被视为"transient warn + continue"；r8 反指这种场景
// 在直 probe app-table 路径下是真的 misconfig（每次 WS 握手都会 close 1011），
// 必须启动期 fail-fast。wsTablesReady 应返 false → 调用方 panic。
func expectTableProbeAccessDenied(mock sqlmock.Sqlmock, table string) {
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", table))).
		WillReturnError(&driverMysql.MySQLError{
			Number:  1142,
			Message: fmt.Sprintf("SELECT command denied to user 'app'@'%%' for table '%s'", table),
		})
}

// expectTableProbeDBAccessDenied 模拟 MySQL ER_DBACCESS_DENIED_ERROR (1044)：
// DB role 没有 schema 级访问权限。这是 r8 [P1] 新加的 fail-fast 场景：app role
// 连上 server 但访问不了目标 schema → 完全不可用 → 启动期 panic。
func expectTableProbeDBAccessDenied(mock sqlmock.Sqlmock, table string) {
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", table))).
		WillReturnError(&driverMysql.MySQLError{
			Number:  1044,
			Message: "Access denied for user 'app'@'%' to database 'testdb'",
		})
}

// expectTableProbeTooManyConnections 模拟 MySQL ER_CON_COUNT_ERROR (1040)：
// transient infrastructure flap（连接池耗尽，等会儿就好）。这种**不**应该让启动
// 期 fail-fast；wsTablesReady 应 log warn + continue。这是 r8 [P1] 收窄后仍保持
// "transient warn"的代表性 case：和 1142 / 1044 misconfig 形成对比。
func expectTableProbeTooManyConnections(mock sqlmock.Sqlmock, table string) {
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", table))).
		WillReturnError(&driverMysql.MySQLError{
			Number:  1040,
			Message: "Too many connections",
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

// TestRouter_WSRouteFailFast_OnTableAccessDenied:
// rooms probe 返 MySQL 1142 (ER_TABLEACCESS_DENIED_ERROR) → wsTablesReady 返 false →
// NewRouter panic。
//
// **r8 [P1] 契约**（推翻 r6 的 warn-and-continue）：直 probe app-table 路径下 1142 不
// 是 information_schema 副作用而是真的"app role 没该表 SELECT 权限"，每次握手都会
// close 1011 → feature 不可用 + healthcheck 健康 = 静默灾难。启动期 fail-fast。
//
// 第一个 probe 即返 1142 → wsTablesReady 立即 return false（不再继续探 room_members）→
// 调用方 panic。
func TestRouter_WSRouteFailFast_OnTableAccessDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeAccessDenied(mock, "rooms")

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("NewRouter did not panic when rooms SELECT denied (r8 P1 fail-fast contract violated)")
		}
		msg, _ := rec.(string)
		if !strings.Contains(msg, "ws backing tables missing") {
			t.Errorf("panic msg = %q, want contain 'ws backing tables missing'", msg)
		}
	}()
	NewRouter(newRouterWSTestDeps(t, gormDB))
}

// TestRouter_WSRouteFailFast_OnDBAccessDenied:
// rooms probe 返 MySQL 1044 (ER_DBACCESS_DENIED_ERROR) → wsTablesReady 返 false →
// NewRouter panic。
//
// **r8 [P1] 契约**：app role 没有 schema 级访问权限，和 1142 同属 misconfig，应启动
// 期 fail-fast 而非 warn。
func TestRouter_WSRouteFailFast_OnDBAccessDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeDBAccessDenied(mock, "rooms")

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("NewRouter did not panic when rooms DB-level access denied (r8 P1 fail-fast contract violated)")
		}
		msg, _ := rec.(string)
		if !strings.Contains(msg, "ws backing tables missing") {
			t.Errorf("panic msg = %q, want contain 'ws backing tables missing'", msg)
		}
	}()
	NewRouter(newRouterWSTestDeps(t, gormDB))
}

// TestRouter_WSRoute_DoesNotPanic_OnTransientTooManyConnections:
// rooms probe 返 MySQL 1040 (ER_CON_COUNT_ERROR, too many connections) →
// wsTablesReady 视为 transient → log warn + continue → **不** panic → 路由仍挂载。
//
// 这是 r8 [P1] 收窄后仍保留的"transient warn"代表 case：与 1142/1044 (misconfig
// fail-fast) 形成对比，证明 r8 不是把所有 mysql 错误都升级成 fail-fast，而是只针对
// "明确指向 misconfig 的错误号"（1146/1142/1044）；其他 mysql 错误（包括 1040
// 这种连接池抖动）保持 warn-and-continue，避免把 transient flap 升级成 CrashLoopBackOff。
func TestRouter_WSRoute_DoesNotPanic_OnTransientTooManyConnections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeTooManyConnections(mock, "rooms")
	expectTableProbeTooManyConnections(mock, "room_members")

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("NewRouter panicked on transient too-many-connections probe (r8 P1 contract violated — should warn + continue): %v", rec)
		}
	}()

	r := NewRouter(newRouterWSTestDeps(t, gormDB))
	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/3001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Errorf("status = 404, want non-404 (route should still be mounted despite transient probe error); body=%s", w.Body.String())
	}
}

// TestRouter_WSRoute_DoesNotPanic_OnConnError:
// rooms probe 返非 mysql error（如 driver: bad connection）→ wsTablesReady 视为
// 表存在 → **不** panic（验证非 mysql.MySQLError 路径，errors.As → false）。
//
// 与 misconfig fail-fast case 对比：misconfig (1146/1142/1044) 是 *mysql.MySQLError
// + 特定 Number；conn error 是普通 error（errors.As → false），走 transient warn 分支。
func TestRouter_WSRoute_DoesNotPanic_OnConnError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectTableProbeConnError(mock, "rooms")
	expectTableProbeConnError(mock, "room_members")

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("NewRouter panicked on conn-error probe (transient contract violated): %v", rec)
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
