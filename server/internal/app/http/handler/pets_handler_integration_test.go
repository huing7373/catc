//go:build integration
// +build integration

// Story 14.2 端到端集成测试：用 dockertest 起真实 mysql:8.0 容器 + 真实 router（含
// auth middleware + ErrorMappingMiddleware + 真 PetService + 真 mysql repo）→
// HTTP 请求验证 envelope 全链路。
//
// 覆盖：
//   1. HappyPath：合法 Bearer token + POST /api/v1/pets/current/state-sync {"state":3}
//      → envelope code=0 + data.state=3 + DB pets.current_state=3
//   2. NoToken_Returns1001：无 Authorization header → auth middleware 兜底 1001
//   3. StateOutOfRange_Returns1002：POST {"state":4} 带合法 token → 1002
//   4. PetLessAccount_Returns200WithEcho：DB DELETE FROM pets WHERE user_id=? →
//      POST {"state":2} 带 token → HTTP 200 + envelope code=0 + data.state=2 +
//      DB 仍 0 行 pets（r7 锁定：pet-less 走 server-acknowledged noop）
//
// **复用** room_handler_integration_test.go 已 ship 的 startMySQL / runMigrations /
// signer helper —— 同 package 同 build tag 集合（独立命名 petsIntegrationTest... 防冲突）。
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration` 触发。

package handler_test

import (
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
	"github.com/huing/cat/server/internal/service"
)

// petsIntegrationTestStartMySQL 起 mysql:8.0 容器，等 ping 通后返回 (DSN, cleanup)。
//
// **独立命名**（与 roomIntegrationTestStartMySQL 同模式）—— 同 package 同 build tag
// 集合内同名 type / func 会冲突。
func petsIntegrationTestStartMySQL(t *testing.T) (string, func()) {
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

// petsIntegrationTestRunMigrations 跑 migrate Up，把所有表建好。
func petsIntegrationTestRunMigrations(t *testing.T, dsn string) {
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

// petsIntegrationTestRouter 构造最小 router（ErrorMappingMiddleware + Auth + PetsHandler）。
//
// **不**直接调 bootstrap.NewRouter —— 那要 SessionMgr / RateLimitCfg 等多余依赖；
// 本 case 只测 POST /api/v1/pets/current/state-sync 单接口端到端，最小 router 让
// token / auth / service / repo 全链路打通。
func petsIntegrationTestRouter(petSvc service.PetService, signer *auth.Signer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	petsHandler := handler.NewPetsHandler(petSvc)

	api := r.Group("/api/v1")
	authedGroup := api.Group("", middleware.Auth(signer))
	authedGroup.POST("/pets/current/state-sync", petsHandler.PostStateSync)
	return r
}

// buildPetsHandlerIntegration: 起容器 → migrate → 装配真实 PetService + signer →
// 返 (router, sqlDB, signer, cleanup)。
//
// PetService deps 配置：
//   - petRepo / userRepo: 真实 mysql repo
//   - sessionMgr / broadcastFn: nil（本 story 不广播；14.4 才接管）
func buildPetsHandlerIntegration(t *testing.T) (*gin.Engine, *sql.DB, *auth.Signer, func()) {
	t.Helper()

	dsn, dockerCleanup := petsIntegrationTestStartMySQL(t)
	petsIntegrationTestRunMigrations(t, dsn)

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

	petRepo := mysql.NewPetRepo(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	petSvc := service.NewPetService(petRepo, userRepo, nil, nil)

	router := petsIntegrationTestRouter(petSvc, signer)

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

// petsIntegrationTestInsertUserPet 直接 INSERT users + pets（与 service 包同模式）。
func petsIntegrationTestInsertUserPet(t *testing.T, sqlDB *sql.DB, userID uint64, guestUID string) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO users (id, guest_uid, nickname, avatar_url, status) VALUES (?, ?, ?, ?, ?)`,
		userID, guestUID, "", "", 1,
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = sqlDB.Exec(
		`INSERT INTO pets (user_id, pet_type, name, current_state, is_default) VALUES (?, ?, ?, ?, ?)`,
		userID, 1, "默认小猫", 1, 1,
	)
	if err != nil {
		t.Fatalf("insert pet: %v", err)
	}
}

// petsIntegrationTestSignToken 用 signer 签发测试 token。
func petsIntegrationTestSignToken(t *testing.T, signer *auth.Signer, userID uint64) string {
	t.Helper()
	token, err := signer.Sign(userID, 0)
	if err != nil {
		t.Fatalf("signer.Sign: %v", err)
	}
	return token
}

// decodePetsIntegrationEnvelope 解析 envelope。
func decodePetsIntegrationEnvelope(t *testing.T, body []byte) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// ============================================================
// case 1: HappyPath — 真 token + POST /pets/current/state-sync {"state":3}
// → envelope code=0 + data.state=3 + DB pets.current_state=3
// ============================================================
func TestPetsHandlerIntegration_PostStateSync_HappyPath(t *testing.T) {
	router, sqlDB, signer, cleanup := buildPetsHandlerIntegration(t)
	defer cleanup()

	const userID = uint64(1001)
	petsIntegrationTestInsertUserPet(t, sqlDB, userID, "uid-pet-1")

	token := petsIntegrationTestSignToken(t, signer, userID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(`{"state":3}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodePetsIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("envelope.code = %d, want 0", env.Code)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", env.Data)
	}
	if state, _ := data["state"].(float64); state != 3 {
		t.Errorf("data.state = %v, want 3", data["state"])
	}

	// DB 校验：pets.current_state 已写为 3
	var dbState int8
	if err := sqlDB.QueryRow(
		`SELECT current_state FROM pets WHERE user_id = ? AND is_default = 1`, userID,
	).Scan(&dbState); err != nil {
		t.Fatalf("query pets.current_state: %v", err)
	}
	if dbState != 3 {
		t.Errorf("DB pets.current_state = %d, want 3", dbState)
	}
}

// ============================================================
// case 2: NoToken_Returns1001 — 无 Authorization → auth middleware 兜底 1001
// ============================================================
func TestPetsHandlerIntegration_PostStateSync_NoToken_Returns1001(t *testing.T) {
	router, _, _, cleanup := buildPetsHandlerIntegration(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(`{"state":2}`))
	req.Header.Set("Content-Type", "application/json")
	// 不设 Authorization header
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 1001 走 HTTP 200（V1 §2.4 钦定业务码与 HTTP status 正交）
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1001; body=%s", w.Code, w.Body.String())
	}
	env := decodePetsIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrUnauthorized {
		t.Errorf("envelope.code = %d, want %d (ErrUnauthorized 1001)", env.Code, apperror.ErrUnauthorized)
	}
}

// ============================================================
// case 3: StateOutOfRange_Returns1002 — 合法 token + state=4 → 1002
// ============================================================
func TestPetsHandlerIntegration_PostStateSync_StateOutOfRange_Returns1002(t *testing.T) {
	router, sqlDB, signer, cleanup := buildPetsHandlerIntegration(t)
	defer cleanup()

	const userID = uint64(1001)
	petsIntegrationTestInsertUserPet(t, sqlDB, userID, "uid-pet-1")

	token := petsIntegrationTestSignToken(t, signer, userID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(`{"state":4}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 1002 走 HTTP 200（业务码与 status 正交）
	env := decodePetsIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam 1002)", env.Code, apperror.ErrInvalidParam)
	}

	// 关键：DB pets.current_state 仍为 1（handler 层 1002 拦截，未进 service / DB）
	var dbState int8
	if err := sqlDB.QueryRow(
		`SELECT current_state FROM pets WHERE user_id = ? AND is_default = 1`, userID,
	).Scan(&dbState); err != nil {
		t.Fatalf("query pets.current_state: %v", err)
	}
	if dbState != 1 {
		t.Errorf("DB pets.current_state = %d, want 1 (handler 1002 拦截前)", dbState)
	}
}

// ============================================================
// case 4: PetLessAccount_Returns200WithEcho — DELETE pets → POST {"state":2} →
// HTTP 200 + envelope code=0 + data.state=2 + DB 仍 0 行 pets（r7 锁定）
// ============================================================
func TestPetsHandlerIntegration_PostStateSync_PetLessAccount_Returns200WithEcho(t *testing.T) {
	router, sqlDB, signer, cleanup := buildPetsHandlerIntegration(t)
	defer cleanup()

	const userID = uint64(1001)
	petsIntegrationTestInsertUserPet(t, sqlDB, userID, "uid-pet-1")

	// 模拟 pet-less 账号
	if _, err := sqlDB.Exec(`DELETE FROM pets WHERE user_id = ?`, userID); err != nil {
		t.Fatalf("DELETE pets: %v", err)
	}

	token := petsIntegrationTestSignToken(t, signer, userID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(`{"state":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// r7 锁定：pet-less 走 server-acknowledged noop → HTTP 200 + envelope code=0
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for pet-less noop; body=%s", w.Code, w.Body.String())
	}
	env := decodePetsIntegrationEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("envelope.code = %d, want 0 (pet-less noop 路径)", env.Code)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", env.Data)
	}
	if state, _ := data["state"].(float64); state != 2 {
		t.Errorf("data.state = %v, want 2 (回显入参)", data["state"])
	}

	// DB pets 仍 0 行（noop 不重新创建 pet）
	var c int64
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM pets WHERE user_id = ?`, userID).Scan(&c); err != nil {
		t.Fatalf("query pets count: %v", err)
	}
	if c != 0 {
		t.Errorf("pets count = %d, want 0 (pet-less noop 不应重新创建 pet)", c)
	}
}
