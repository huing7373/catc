//go:build integration
// +build integration

// Story 10.3 集成测试：用 dockertest 起真实 mysql:8.0 容器（创建临时
// rooms / room_members fixture 表，Epic 11.2 落地 0007 / 0008 migration 后
// 切到 official path），跑真实 WS 拨号 → 握手成功 → 收到 placeholder snapshot
// → ping/pong roundtrip → 主动 close 全流程，验证 Gateway / Session /
// SessionManager / RoomMemberRepo 在真 MySQL 下行为正确。
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration` 触发。
//
// docker 不可用时 t.Skip("docker not available")，不让 CI 阻塞（与 redis /
// mysql_integration_test.go 同模式）。

package ws_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
	mysqlrepo "github.com/huing/cat/server/internal/repo/mysql"
)

// startMySQLWithRoomMemberFixture 起一个 mysql:8.0 容器 + 临时建表 + 插入
// fixture（roomID=3001 内有 userID=1001 / 1002 两个 member）。
//
// **临时建表**：本 story 阶段 room_members migration 还没落地（Epic 11.2 才做）；
// 0007_init_rooms migration 已在 r5 落地（rooms.status 用 TINYINT，1=active /
// 2=closed，对齐数据库设计.md §6.12）。集成测试用以下 SQL 在 setup 时建临时表
// （rooms schema 与 0007 migration **保持一致**，让 RoomExists 的
// `WHERE status = 1` 谓词（review r7 P2 加）在测试与 prod 行为等价）：
//
//	CREATE TABLE rooms (
//	    id BIGINT UNSIGNED NOT NULL,
//	    status TINYINT NOT NULL DEFAULT 1,
//	    max_members INT NOT NULL DEFAULT 4,
//	    member_count INT NOT NULL DEFAULT 0,
//	    PRIMARY KEY (id)
//	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
//
//	CREATE TABLE room_members (
//	    room_id BIGINT UNSIGNED NOT NULL,
//	    user_id BIGINT UNSIGNED NOT NULL,
//	    joined_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
//	    PRIMARY KEY (room_id, user_id),
//	    INDEX idx_user_room (user_id, room_id)
//	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
//
//	INSERT INTO rooms (id, status, max_members, member_count) VALUES (3001, 1, 4, 2);
//	INSERT INTO room_members (room_id, user_id) VALUES (3001, 1001), (3001, 1002);
//
// **Epic 11.2 落地后**：删除本 helper 的 inline DDL，改为跑 official migration。
func startMySQLWithRoomMemberFixture(t *testing.T) (*gorm.DB, func()) {
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

	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		_ = pool.Purge(resource)
		t.Fatalf("gorm.Open: %v", err)
	}
	rawDB, err := gormDB.DB()
	if err != nil {
		_ = pool.Purge(resource)
		t.Fatalf("gormDB.DB: %v", err)
	}

	// 临时建表 + 插 fixture
	stmts := []string{
		`CREATE TABLE rooms (
		    id BIGINT UNSIGNED NOT NULL,
		    status TINYINT NOT NULL DEFAULT 1,
		    max_members INT NOT NULL DEFAULT 4,
		    member_count INT NOT NULL DEFAULT 0,
		    PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE room_members (
		    room_id BIGINT UNSIGNED NOT NULL,
		    user_id BIGINT UNSIGNED NOT NULL,
		    joined_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		    PRIMARY KEY (room_id, user_id),
		    INDEX idx_user_room (user_id, room_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`INSERT INTO rooms (id, status, max_members, member_count) VALUES (3001, 1, 4, 2)`,
		`INSERT INTO room_members (room_id, user_id) VALUES (3001, 1001), (3001, 1002)`,
	}
	for _, stmt := range stmts {
		if _, err := rawDB.Exec(stmt); err != nil {
			_ = pool.Purge(resource)
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	cleanup := func() {
		_ = rawDB.Close()
		_ = pool.Purge(resource)
	}
	return gormDB, cleanup
}

// startGatewayWithRealMySQL 启动一个挂着真实 RoomMemberRepo 的 httptest server。
// 返 (wsURL, signer, mgr, ts)。
func startGatewayWithRealMySQL(t *testing.T, gormDB *gorm.DB) (string, *auth.Signer, wsapp.SessionManager, *httptest.Server) {
	t.Helper()
	signer, err := auth.New("integration-test-secret-32-bytes-min", 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	mgr := wsapp.NewSessionManager()
	repo := mysqlrepo.NewRoomMemberRepo(gormDB)
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 60,
		MaxMessageSizeBytes: 16384,
		WriteTimeoutSec:     5,
	}
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "test")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws/rooms/:roomId", gateway.Handle)
	ts := httptest.NewServer(r)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	return wsURL, signer, mgr, ts
}

// TestWSIntegration_HappyPath_FirstMessageIsSnapshot:
// 启 mysql + 插入 room_members fixture → 启动 httptest gateway → 用 gorilla.Dial
// 拨连 → 握手成功 → 第一条 message 是 type="room.snapshot"，payload.room.id="3001"，
// payload.room.maxMembers=4，payload.room.memberCount=2，payload.members 长度 = 2。
func TestWSIntegration_HappyPath_FirstMessageIsSnapshot(t *testing.T) {
	gormDB, cleanup := startMySQLWithRoomMemberFixture(t)
	defer cleanup()

	wsURL, signer, mgr, ts := startGatewayWithRealMySQL(t, gormDB)
	defer mgr.Close()
	defer ts.Close()

	token, err := signer.Sign(1001, 3600)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(msg, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env["type"] != "room.snapshot" {
		t.Errorf("type = %v, want room.snapshot", env["type"])
	}
	payload := env["payload"].(map[string]any)
	room := payload["room"].(map[string]any)
	if room["id"] != "3001" {
		t.Errorf("room.id = %v, want 3001", room["id"])
	}
	if int(room["maxMembers"].(float64)) != 4 {
		t.Errorf("room.maxMembers = %v, want 4", room["maxMembers"])
	}
	if int(room["memberCount"].(float64)) != 2 {
		t.Errorf("room.memberCount = %v, want 2 (fixture has 2 members)", room["memberCount"])
	}
	members := payload["members"].([]any)
	if len(members) != 2 {
		t.Errorf("len(members) = %d, want 2", len(members))
	}
}

// TestWSIntegration_TokenExpired_Closes4001:
// 用过期 token 拨连 → conn.ReadMessage 返 *websocket.CloseError，Code=4001,
// Text="token expired"。
func TestWSIntegration_TokenExpired_Closes4001(t *testing.T) {
	gormDB, cleanup := startMySQLWithRoomMemberFixture(t)
	defer cleanup()

	wsURL, _, mgr, ts := startGatewayWithRealMySQL(t, gormDB)
	defer mgr.Close()
	defer ts.Close()

	// 用 1s 过期 signer 签 token，等过期后拨号
	signerShort, err := auth.New("integration-test-secret-32-bytes-min", 1)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	expiredToken, err := signerShort.Sign(1001, 1)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)

	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, expiredToken)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("err = %T %v, want *websocket.CloseError", err, err)
	}
	if closeErr.Code != 4001 {
		t.Errorf("close code = %d, want 4001", closeErr.Code)
	}
	if closeErr.Text != "token expired" {
		t.Errorf("close text = %q, want %q", closeErr.Text, "token expired")
	}
}

// TestWSIntegration_UserNotInRoom_Closes4003:
// userID=999（不在 room_members fixture 中）拨连 roomID=3001 → close 4003。
func TestWSIntegration_UserNotInRoom_Closes4003(t *testing.T) {
	gormDB, cleanup := startMySQLWithRoomMemberFixture(t)
	defer cleanup()

	wsURL, signer, mgr, ts := startGatewayWithRealMySQL(t, gormDB)
	defer mgr.Close()
	defer ts.Close()

	token, err := signer.Sign(999, 3600)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("err = %T %v, want *websocket.CloseError", err, err)
	}
	if closeErr.Code != 4003 {
		t.Errorf("close code = %d, want 4003", closeErr.Code)
	}
	if closeErr.Text != "user not in room" {
		t.Errorf("close text = %q, want %q", closeErr.Text, "user not in room")
	}
}

// TestWSIntegration_PingPongRoundtrip:
// 握手成功收到 snapshot 后 → 发 ping{type:"ping", requestId:"ping_001",
// payload:{}} → 收到 pong: type="pong", requestId="ping_001", payload:{}, ts > 0。
func TestWSIntegration_PingPongRoundtrip(t *testing.T) {
	gormDB, cleanup := startMySQLWithRoomMemberFixture(t)
	defer cleanup()

	wsURL, signer, mgr, ts := startGatewayWithRealMySQL(t, gormDB)
	defer mgr.Close()
	defer ts.Close()

	token, err := signer.Sign(1001, 3600)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// 读 snapshot
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	// 发 ping
	pingMsg := `{"type":"ping","requestId":"ping_001","payload":{}}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(pingMsg)); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	// 读 pong
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(msg, &env); err != nil {
		t.Fatalf("unmarshal pong: %v", err)
	}
	if env["type"] != "pong" {
		t.Errorf("type = %v, want pong", env["type"])
	}
	if env["requestId"] != "ping_001" {
		t.Errorf("requestId = %v, want ping_001 (回带)", env["requestId"])
	}
	if ts, ok := env["ts"].(float64); !ok || ts <= 0 {
		t.Errorf("ts = %v, want positive", env["ts"])
	}
}

// 让 ws_test 包能 compile: ListSessions 需要 ctx import；引用一次让 lint 不报。
var _ = context.Background
