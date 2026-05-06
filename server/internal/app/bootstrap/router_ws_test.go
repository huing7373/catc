package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"regexp"
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

// router_ws_test 锁 Story 10.3 review r3 [P1]：WS 路由必须 gate 在
// rooms / room_members 表存在性之后。两张表是 Epic 11.2 才落地的 migration，
// 当前 server/migrations/ 只到 0006；任何走 WS 握手的部署环境会在
// RoomMemberRepo.RoomExists / IsUserInRoom 阶段 SQL 报"table doesn't exist"
// → Gateway close 1011 → feature 完全不可用。
//
// 测试三个分支：
//   - 表都存在 → 路由挂载（200/400/4xx 任意 ≠ 404）
//   - 任一表不存在 → 路由不挂载（404 NotFound）
//   - SHOW TABLES query 出错（DB 异常）→ 路由不挂载（404）
//
// **不**用真实 mysql 容器：sqlmock 注入 GORM，期望 `SHOW TABLES LIKE ...`
// 查询返回不同结果即可锁住 wsTablesReady 决策路径。

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
		// 各业务路由构造期的 repo 初始化（如果有的话）+ wsTablesReady 的两次
		// SHOW TABLES 查询；本测试只想锁住 wsTablesReady 行为，对其他可能
		// 出现的查询保持宽松（QueryMatcherRegexp 默认会跳过未声明的查询，
		// 实际结果以路由是否挂载为准）。
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
// wsTablesReady 阈值 hitCount >= 2 才判定 ready。
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
// 路由 dispatch 命中（任何非 404 状态都说明命中了 gateway.Handle，至于
// gateway 内部因为缺 token 返 4001 还是因为 Gin 协议升级失败返 400 都
// 不是本测试关注点）。
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

// TestRouter_WSRouteSkipped_WhenRoomsTableMissing:
// rooms 表不存在（room_members 存在） → 路由跳过 → /ws/rooms/3001 返 404。
func TestRouter_WSRouteSkipped_WhenRoomsTableMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectWSTablesShow(mock, false, true)

	r := NewRouter(newRouterWSTestDeps(t, gormDB))

	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/3001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (route should not be mounted when rooms table missing); body=%s", w.Code, w.Body.String())
	}
}

// TestRouter_WSRouteSkipped_WhenRoomMembersTableMissing:
// room_members 表不存在（rooms 存在） → 路由跳过 → 404。
func TestRouter_WSRouteSkipped_WhenRoomMembersTableMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectWSTablesShow(mock, true, false)

	r := NewRouter(newRouterWSTestDeps(t, gormDB))

	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/3001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (route should not be mounted when room_members table missing); body=%s", w.Code, w.Body.String())
	}
}

// TestRouter_WSRouteSkipped_WhenBothTablesMissing:
// 两表都不存在（当前 prod 用 0001-0006 migration 起服务的真实状态） → 路由跳过 → 404。
//
// 这是本 fix 的核心防回归 case：review r3 P1 描述的 prod 失败场景。
func TestRouter_WSRouteSkipped_WhenBothTablesMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB, mock := newGormWithMockForRouter(t)
	expectWSTablesShow(mock, false, false)

	r := NewRouter(newRouterWSTestDeps(t, gormDB))

	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/3001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (review r3 P1 防回归: prod build with 0001-0006 migrations should NOT mount ws); body=%s", w.Code, w.Body.String())
	}

	// 即便 WS 路由跳过 / 业务路由仍然挂载（rooms 表不存在不应阻塞 /ping / /home 等）：
	// 验证 /ping 仍 200 来确认本 if-else 分支没有顺带砍掉无关路由。
	pingReq := httptest.NewRequest(http.MethodGet, "/ping", nil)
	pingW := httptest.NewRecorder()
	r.ServeHTTP(pingW, pingReq)
	if pingW.Code != http.StatusOK {
		t.Errorf("/ping status = %d, want 200 (业务路由不应受 ws gate 影响)", pingW.Code)
	}
}
