package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
	"github.com/huing/cat/server/internal/repo/tx"
)

// router_ws_test 锁 Story 10.3 review r5 [P1] 后的契约：
//
// **r5 之前（r3 形态）**：rooms / room_members migration 不在本 story 范围内，
// 路由用 wsTablesReady() warn-and-skip 兜底；prod 起服务时表缺 → 路由不挂 →
// client 拿 HTTP 404（不是 documented WS close codes） → Story 10.3 在正常
// startup 完全不可用。
//
// **r5 修法**：
//   - 把 rooms / room_members CREATE TABLE 拆出 Epic 11.2 提前到 Story 10.3
//     （migrations 0007 / 0008 已 ship）
//   - WS 路由**总是**挂载（不再 warn-and-skip）
//   - wsTablesReady() 改为**防御性早期检测**：表缺 → log error + panic（fail-fast）
//
// 本测试覆盖三个分支（注意语义都变了）：
//   - 表都存在 → 路由挂载（NewRouter 正常返回 + /ws/rooms/3001 返非 404）
//   - 任一表不存在 → NewRouter 启动期 panic（fail-fast，不再 silent skip）
//   - SHOW / sniff query 出错（DB 异常）→ NewRouter 启动期 panic（同上）
//
// 不用真实 mysql 容器：sqlmock 注入 GORM，期望 information_schema query 返回
// 不同结果即可锁住 wsTablesReady fail-fast 行为。

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

// expectWSTablesShow 配置 sqlmock 让 wsTablesReady 的元数据查询返回 hitCount。
//
// 实装查询单 query 走 information_schema.tables 一次性算两表命中数：
//
//	SELECT COUNT(*) FROM information_schema.tables
//	WHERE table_schema = DATABASE() AND table_name IN ('rooms','room_members')
//
// hitCount 取值含义：
//   - roomsExists && membersExists → 2
//   - 仅一表存在 → 1
//   - 两表都不存在 → 0
//
// wsTablesReady 阈值 hitCount >= 2 才判定 ready；不足则 panic（r5 fail-fast）。
func expectWSTablesShow(mock sqlmock.Sqlmock, roomsExists, membersExists bool) {
	var hitCount int
	if roomsExists {
		hitCount++
	}
	if membersExists {
		hitCount++
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ('rooms','room_members')")).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(hitCount))
}

// TestRouter_WSRouteMounted_WhenTablesExist:
// rooms / room_members 都存在 → /ws/rooms/3001 路由挂载 → 非 404 响应。
//
// **不**做真实 WS 握手（避免拉满 gorilla.Upgrader / hijack 路径）；只验证
// 路由 dispatch 命中（任何非 404 状态都说明命中了 gateway.Handle）。
func TestRouter_WSRouteMounted_WhenTablesExist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectWSTablesShow(mock, true, true)

	r := NewRouter(newRouterWSTestDeps(t, gormDB))

	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/3001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Errorf("status = 404, want non-404 (gateway.Handle should be reached); body=%s", w.Body.String())
	}
}

// TestRouter_WSRouteFailFast_WhenRoomsTableMissing:
// rooms 表不存在（room_members 存在） → NewRouter panic（r5 fail-fast）。
//
// review r5 [P1] 修法：r3 形态下"表缺 → 跳过路由"是 silent skip，让 prod 拿
// 404 而非 documented WS close codes；r5 后改为表缺 → log error + panic 让
// systemd / k8s 立即重启 + 运维告警。
func TestRouter_WSRouteFailFast_WhenRoomsTableMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectWSTablesShow(mock, false, true)

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("NewRouter did not panic when rooms table missing (r5 fail-fast contract violated)")
		}
		msg, _ := rec.(string)
		if !strings.Contains(msg, "ws backing tables missing") {
			t.Errorf("panic msg = %q, want contain 'ws backing tables missing'", msg)
		}
	}()
	NewRouter(newRouterWSTestDeps(t, gormDB))
}

// TestRouter_WSRouteFailFast_WhenRoomMembersTableMissing:
// room_members 表不存在（rooms 存在） → NewRouter panic（同上）。
func TestRouter_WSRouteFailFast_WhenRoomMembersTableMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectWSTablesShow(mock, true, false)

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("NewRouter did not panic when room_members table missing (r5 fail-fast contract violated)")
		}
		msg, _ := rec.(string)
		if !strings.Contains(msg, "ws backing tables missing") {
			t.Errorf("panic msg = %q, want contain 'ws backing tables missing'", msg)
		}
	}()
	NewRouter(newRouterWSTestDeps(t, gormDB))
}

// TestRouter_WSRouteFailFast_WhenBothTablesMissing:
// 两表都不存在（即 r3 时代默认 prod 用 0001-0006 起服务的状态）→ NewRouter
// 启动期 panic（r5 fail-fast，**不**再 silent 跳过 + 起服务）。
//
// 这是本 fix 的核心防回归 case：review r5 P1 描述的"prod 静默 404"场景被
// 替换为"prod 启动期立即 panic"，让运维通过 CrashLoopBackOff 立即看到。
func TestRouter_WSRouteFailFast_WhenBothTablesMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectWSTablesShow(mock, false, false)

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("NewRouter did not panic when both tables missing (r5 fail-fast contract violated)")
		}
		msg, _ := rec.(string)
		if !strings.Contains(msg, "ws backing tables missing") {
			t.Errorf("panic msg = %q, want contain 'ws backing tables missing'", msg)
		}
	}()
	NewRouter(newRouterWSTestDeps(t, gormDB))
}
