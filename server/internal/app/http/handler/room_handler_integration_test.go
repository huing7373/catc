//go:build integration
// +build integration

// Story 11.3 端到端集成测试：用 dockertest 起真实 mysql:8.0 容器 + 真实 router（含
// auth middleware + ErrorMappingMiddleware + 真 RoomService + 真 mysql repo）→
// HTTP 请求验证 envelope 全链路。
//
// 覆盖：
//   1. HappyPath：合法 Bearer token + POST /api/v1/rooms → envelope code=0 + data.room 字段格式
//   2. NoToken_Returns1001：无 Authorization header → auth middleware 兜底 1001
//   3. AlreadyInRoom_Returns6003：先调一次成功 + 再调一次同 token → 6003（端到端验证 service
//      6003 路径正确接到 handler envelope）
//
// 复用 4.6 / 4.7 的 startMySQL / runMigrations helper（来自 service 包；本测试通过
// helper bootstrap 同 binary 内多 package 共享）。
//
// **关键反模式（已规避）**：
//   - **不**复用 4.7 auth_handler_integration_test.go 的 `integrationStubAuthService`
//     —— 同 package 同 build tag 集合内同名 type 会冲突；本文件用独立命名（roomIntegrationTest...）
//   - **不**走 stub service —— 本 case 是端到端验证，必须真 service + 真 mysql

package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/infra/migrate"
	"github.com/huing/cat/server/internal/pkg/auth"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// roomIntegrationTestStartMySQL 起一个 mysql:8.0 容器，等 ping 通后返回 (DSN, cleanup)。
//
// **命名差异**：与 service 包同名 helper（startMySQL）独立 —— Go 包独立，跨包不能复用
// helper，复制粘贴是惯例（与 step_service_integration_test 复制 4.6 helper 同模式；
// 详见项目 ADR-0001 §3.5）。
func roomIntegrationTestStartMySQL(t *testing.T) (string, func()) {
	t.Helper()

	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	if err := pool.Client.Ping(); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "mysql",
		Tag:        "8.0",
		Env: []string{
			"MYSQL_ROOT_PASSWORD=catdev",
			"MYSQL_DATABASE=cat_test",
		},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
		hc.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		t.Skipf("could not start mysql container: %v", err)
	}

	hostPort := resource.GetPort("3306/tcp")
	dsn := fmt.Sprintf("root:catdev@tcp(127.0.0.1:%s)/cat_test?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true", hostPort)

	pool.MaxWait = 90 * time.Second
	if err := pool.Retry(func() error {
		sqlDB, err := sql.Open("mysql", dsn)
		if err != nil {
			return err
		}
		defer sqlDB.Close()
		return sqlDB.Ping()
	}); err != nil {
		_ = pool.Purge(resource)
		t.Skipf("mysql container did not become ready: %v", err)
	}

	cleanup := func() {
		_ = pool.Purge(resource)
	}
	return dsn, cleanup
}

// roomIntegrationTestRunMigrations 跑 migrate Up，把所有表（含 0007 / 0008）建好。
func roomIntegrationTestRunMigrations(t *testing.T, dsn string) {
	t.Helper()
	migPath, err := filepath.Abs("../../../../migrations")
	if err != nil {
		t.Fatalf("filepath.Abs migrations: %v", err)
	}
	mig, err := migrate.New(dsn, migPath)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer mig.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := mig.Up(ctx); err != nil {
		t.Fatalf("migrate Up: %v", err)
	}
}

// roomIntegrationTestRouter 构造一个挂上 ErrorMappingMiddleware + Auth + RoomHandler
// 的 router（与 bootstrap.NewRouter 业务子组等价的最小子集）。
//
// **不**直接调 bootstrap.NewRouter —— 那要 SessionMgr / RateLimitCfg 等多余依赖；
// 本 case 只测 POST /api/v1/rooms 单接口端到端，构造最小 router 让 token / auth /
// service / repo 全链路打通。
func roomIntegrationTestRouter(roomSvc service.RoomService, signer *auth.Signer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	roomHandler := handler.NewRoomHandler(roomSvc)

	api := r.Group("/api/v1")
	authedGroup := api.Group("", middleware.Auth(signer))
	authedGroup.POST("/rooms", roomHandler.CreateRoom)
	authedGroup.POST("/rooms/:roomId/join", roomHandler.JoinRoom) // Story 11.4 加
	return r
}

// buildRoomHandlerIntegration: 起容器 → migrate → 装配真实 RoomService + signer →
// 返 (router, sqlDB, signer, cleanup)。
func buildRoomHandlerIntegration(t *testing.T) (*gin.Engine, *sql.DB, *auth.Signer, func()) {
	t.Helper()

	dsn, dockerCleanup := roomIntegrationTestStartMySQL(t)
	roomIntegrationTestRunMigrations(t, dsn)

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		dockerCleanup()
		t.Fatalf("db.Open: %v", err)
	}

	signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
	if err != nil {
		dockerCleanup()
		t.Fatalf("auth.New: %v", err)
	}

	txMgr := tx.NewManager(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	roomRepo := mysql.NewRoomRepo(gormDB)
	roomMemberRepo := mysql.NewRoomMemberRepo(gormDB)
	roomSvc := service.NewRoomService(txMgr, userRepo, roomRepo, roomMemberRepo)

	router := roomIntegrationTestRouter(roomSvc, signer)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup := func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return router, rawDB, signer, cleanup
}

// roomIntegrationTestInsertUser 直接 INSERT users 行（与 service 包 insertUser 同模式）。
func roomIntegrationTestInsertUser(t *testing.T, sqlDB *sql.DB, id uint64, guestUID, nickname string) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO users (id, guest_uid, nickname, avatar_url, status) VALUES (?, ?, ?, ?, ?)`,
		id, guestUID, nickname, "", 1,
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

// roomIntegrationTestSignToken 用 signer 签发一个测试 token（uid=指定值）。
func roomIntegrationTestSignToken(t *testing.T, signer *auth.Signer, userID uint64) string {
	t.Helper()
	token, err := signer.Sign(userID, 0)
	if err != nil {
		t.Fatalf("signer.Sign: %v", err)
	}
	return token
}

// decodeRoomIntegrationEnvelope 解析 envelope。
func decodeRoomIntegrationEnvelope(t *testing.T, body []byte) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// ============================================================
// AC12.1: HappyPath — 真 token + POST /rooms → envelope code=0 + data.room 完整字段
// ============================================================
func TestRoomHandlerIntegration_CreateRoom_HappyPath(t *testing.T) {
	router, sqlDB, signer, cleanup := buildRoomHandlerIntegration(t)
	defer cleanup()

	const userID = uint64(1001)
	roomIntegrationTestInsertUser(t, sqlDB, userID, "uid-room-1", "用户1001")

	token := roomIntegrationTestSignToken(t, signer, userID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("envelope.code = %d, want 0", env.Code)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", env.Data)
	}
	room, ok := data["room"].(map[string]any)
	if !ok {
		t.Fatalf("data.room not object: %T", data["room"])
	}
	// data.room.id 是 string（V1 §2.5 BIGINT 字符串化）
	roomID, _ := room["id"].(string)
	if roomID == "" {
		t.Errorf("room.id = empty, want non-empty BIGINT 字符串")
	}
	if room["creatorUserId"] != "1001" {
		t.Errorf("room.creatorUserId = %v, want \"1001\"", room["creatorUserId"])
	}
	if maxMembers, _ := room["maxMembers"].(float64); maxMembers != 4 {
		t.Errorf("room.maxMembers = %v, want 4", room["maxMembers"])
	}
	if memberCount, _ := room["memberCount"].(float64); memberCount != 1 {
		t.Errorf("room.memberCount = %v, want 1", room["memberCount"])
	}
	if status, _ := room["status"].(float64); status != 1 {
		t.Errorf("room.status = %v, want 1", room["status"])
	}

	// DB 层校验：rooms / room_members 各有 1 行 + users.current_room_id 已写
	var dbRoomCount int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&dbRoomCount); err != nil {
		t.Fatalf("query rooms count: %v", err)
	}
	if dbRoomCount != 1 {
		t.Errorf("rooms count = %d, want 1", dbRoomCount)
	}
}

// ============================================================
// AC12.2: 无 Authorization header → auth middleware 兜底 1001
// ============================================================
func TestRoomHandlerIntegration_CreateRoom_NoToken_Returns1001(t *testing.T) {
	router, _, _, cleanup := buildRoomHandlerIntegration(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	// 不设 Authorization
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 1001 走 HTTP 200（V1 §2.4 钦定业务码与 HTTP status 正交；ErrorMappingMiddleware
	// 仅把 1009 映射为 500，其他业务码全走 200 + envelope code 区分）
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1001; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrUnauthorized {
		t.Errorf("envelope.code = %d, want %d (ErrUnauthorized 1001)", env.Code, apperror.ErrUnauthorized)
	}
}

// ============================================================
// AC12.3: 同 token 二次调用 → service 走预检 6003 → handler envelope 6003
//
// 端到端验证 service 6003 路径正确接到 handler envelope（双路径都覆盖：本 case 是
// 预检路径；UNIQUE 兜底路径在 service 集成测试 case 3 已覆盖）。
// ============================================================
func TestRoomHandlerIntegration_CreateRoom_AlreadyInRoom_Returns6003(t *testing.T) {
	router, sqlDB, signer, cleanup := buildRoomHandlerIntegration(t)
	defer cleanup()

	const userID = uint64(1001)
	roomIntegrationTestInsertUser(t, sqlDB, userID, "uid-room-1", "用户1001")

	token := roomIntegrationTestSignToken(t, signer, userID)

	// 第一次：成功
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+token)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first call status = %d, want 200; body=%s", w1.Code, w1.Body.String())
	}
	env1 := decodeRoomIntegrationEnvelope(t, w1.Body.Bytes())
	if env1.Code != 0 {
		t.Fatalf("first envelope.code = %d, want 0", env1.Code)
	}

	// 第二次：同 token + 同 user → 6003（service 预检路径）
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader([]byte(`{}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	// 6003 走 HTTP 200（V1 §2.4 钦定业务码与 HTTP status 正交；6xxx 不映射 5xx）
	if w2.Code != http.StatusOK {
		t.Fatalf("second call status = %d, want 200 for 6003; body=%s", w2.Code, w2.Body.String())
	}
	env2 := decodeRoomIntegrationEnvelope(t, w2.Body.Bytes())
	if env2.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("envelope.code = %d, want %d (ErrUserAlreadyInRoom 6003 预检路径)", env2.Code, apperror.ErrUserAlreadyInRoom)
	}
}

// ============================================================
// Story 11.4 端到端集成测试 case
// ============================================================

// AC12.4: HappyPath — A 创建房间 + B join → envelope code=0 + DB 校验
func TestRoomHandlerIntegration_JoinRoom_HappyPath(t *testing.T) {
	router, sqlDB, signer, cleanup := buildRoomHandlerIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	roomIntegrationTestInsertUser(t, sqlDB, userA, "uid-a", "A")
	roomIntegrationTestInsertUser(t, sqlDB, userB, "uid-b", "B")

	tokenA := roomIntegrationTestSignToken(t, signer, userA)
	tokenB := roomIntegrationTestSignToken(t, signer, userB)

	// A 创建房间
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+tokenA)
	wCreate := httptest.NewRecorder()
	router.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200; body=%s", wCreate.Code, wCreate.Body.String())
	}
	envCreate := decodeRoomIntegrationEnvelope(t, wCreate.Body.Bytes())
	createData := envCreate.Data.(map[string]any)
	roomData := createData["room"].(map[string]any)
	roomIDStr := roomData["id"].(string)

	// B join
	reqJoin := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomIDStr+"/join", strings.NewReader(`{}`))
	reqJoin.Header.Set("Content-Type", "application/json")
	reqJoin.Header.Set("Authorization", "Bearer "+tokenB)
	wJoin := httptest.NewRecorder()
	router.ServeHTTP(wJoin, reqJoin)

	if wJoin.Code != http.StatusOK {
		t.Fatalf("join status = %d, want 200; body=%s", wJoin.Code, wJoin.Body.String())
	}
	envJoin := decodeRoomIntegrationEnvelope(t, wJoin.Body.Bytes())
	if envJoin.Code != 0 {
		t.Errorf("envelope.code = %d, want 0", envJoin.Code)
	}
	joinData, ok := envJoin.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", envJoin.Data)
	}
	if joinData["roomId"] != roomIDStr {
		t.Errorf("data.roomId = %v, want %q", joinData["roomId"], roomIDStr)
	}
	joined, ok := joinData["joined"].(bool)
	if !ok {
		t.Errorf("data.joined not bool: %T", joinData["joined"])
	}
	if !joined {
		t.Errorf("data.joined = false, want true")
	}

	// DB 校验：room_members 2 行
	var memberCount int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM room_members WHERE room_id = ?", roomIDStr).Scan(&memberCount); err != nil {
		t.Fatalf("query members count: %v", err)
	}
	if memberCount != 2 {
		t.Errorf("room_members count = %d, want 2 (A creator + B joiner)", memberCount)
	}
}

// AC12.5: 无 Authorization header → auth middleware 兜底 1001
func TestRoomHandlerIntegration_JoinRoom_NoToken_Returns1001(t *testing.T) {
	router, _, _, cleanup := buildRoomHandlerIntegration(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1001; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrUnauthorized {
		t.Errorf("envelope.code = %d, want %d (ErrUnauthorized 1001)", env.Code, apperror.ErrUnauthorized)
	}
}

// AC12.6: A 创建房间后再调 POST /rooms/{room_id}/join → envelope.code=6003
// 端到端验证 service 6003 预检路径正确接到 handler envelope。
func TestRoomHandlerIntegration_JoinRoom_AlreadyInRoom_Returns6003(t *testing.T) {
	router, sqlDB, signer, cleanup := buildRoomHandlerIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	roomIntegrationTestInsertUser(t, sqlDB, userA, "uid-a", "A")

	tokenA := roomIntegrationTestSignToken(t, signer, userA)

	// A 创建房间
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer "+tokenA)
	wCreate := httptest.NewRecorder()
	router.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200; body=%s", wCreate.Code, wCreate.Body.String())
	}
	envCreate := decodeRoomIntegrationEnvelope(t, wCreate.Body.Bytes())
	createData := envCreate.Data.(map[string]any)
	roomData := createData["room"].(map[string]any)
	roomIDStr := roomData["id"].(string)

	// A 试图加入自己刚创建的房间 → 6003（已在房间预检）
	reqJoin := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+roomIDStr+"/join", strings.NewReader(`{}`))
	reqJoin.Header.Set("Content-Type", "application/json")
	reqJoin.Header.Set("Authorization", "Bearer "+tokenA)
	wJoin := httptest.NewRecorder()
	router.ServeHTTP(wJoin, reqJoin)

	if wJoin.Code != http.StatusOK {
		t.Fatalf("join status = %d, want 200 for 6003; body=%s", wJoin.Code, wJoin.Body.String())
	}
	envJoin := decodeRoomIntegrationEnvelope(t, wJoin.Body.Bytes())
	if envJoin.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("envelope.code = %d, want %d (ErrUserAlreadyInRoom 6003)", envJoin.Code, apperror.ErrUserAlreadyInRoom)
	}
}

// AC12.7: B 试图 POST /rooms/99999/join（不存在的 roomID）→ envelope.code=6001
func TestRoomHandlerIntegration_JoinRoom_RoomNotFound_Returns6001(t *testing.T) {
	router, sqlDB, signer, cleanup := buildRoomHandlerIntegration(t)
	defer cleanup()

	const userB = uint64(1002)
	roomIntegrationTestInsertUser(t, sqlDB, userB, "uid-b", "B")
	tokenB := roomIntegrationTestSignToken(t, signer, userB)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/99999/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 6001; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrRoomNotFound {
		t.Errorf("envelope.code = %d, want %d (ErrRoomNotFound 6001)", env.Code, apperror.ErrRoomNotFound)
	}
}

// AC12.8: path = "abc" → handler ParseUint 失败 → envelope.code=1002
func TestRoomHandlerIntegration_JoinRoom_InvalidRoomIDPath_Returns1002(t *testing.T) {
	router, sqlDB, signer, cleanup := buildRoomHandlerIntegration(t)
	defer cleanup()

	const userB = uint64(1002)
	roomIntegrationTestInsertUser(t, sqlDB, userB, "uid-b", "B")
	tokenB := roomIntegrationTestSignToken(t, signer, userB)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/abc/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1002; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam 1002)", env.Code, apperror.ErrInvalidParam)
	}
}

