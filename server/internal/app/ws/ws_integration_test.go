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
	"path/filepath"
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
	"github.com/huing/cat/server/internal/infra/migrate"
	"github.com/huing/cat/server/internal/pkg/auth"
	mysqlrepo "github.com/huing/cat/server/internal/repo/mysql"
)

// migrationsPathForWS 返回 server/migrations 的绝对路径。本测试文件位于
// server/internal/app/ws/，到 server/migrations 是 3 级上跳：
// .. → server/internal/app/ → .. → server/internal/ → .. → server/ → /migrations。
// 与 migrate_integration_test.go::migrationsPath 同深度（migrate 包也在
// server/internal/infra/migrate/，3 级上跳）。
func migrationsPathForWS(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../../migrations")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return abs
}

// startMySQLWithRoomMemberFixture 起一个 mysql:8.0 容器，跑 official migration
// （migrate.Up() → 0001 ~ 0008 共 8 张表落地），并插入 fixture（roomID=3001 内有
// userID=1001 / 1002 两个 member）。
//
// **Story 11.2 落地后**（关键变化）：彻底移除 inline `CREATE TABLE rooms` /
// `CREATE TABLE room_members` 临时建表路径，改跑 official migration。背景：
//   - Story 10.3 阶段为让 ws 集成测试可独立跑，inline 建临时表 rooms / room_members
//     （PRIMARY KEY (room_id, user_id) + member_count 字段）vs prod 0007 / 0008
//     真实 schema（id AUTO_INCREMENT PK + UNIQUE(user_id) + UNIQUE(room_id, user_id)）
//     存在多处漂移；
//   - 11.2 接管后，Epic 11.x 集成测试统一走 official migration 路径，不再各自
//     inline DDL（Layer 2 集成测试 11.9 也走这个模板）；
//   - fixture 行通过 `INSERT INTO rooms (id, creator_user_id, status, max_members)`
//     + `INSERT INTO room_members (room_id, user_id)` 显式插入；rooms.id=3001 +
//     creator_user_id=1001 显式赋值（DB 接受显式 id 值，AUTO_INCREMENT 不阻止），
//     让既有 case 的 roomID=3001 / userID=1001 / 1002 期望保持稳定。
//
// fixture 状态：
//   - rooms: 1 行 (id=3001, creator_user_id=1001, status=1=active, max_members=4)
//   - room_members: 2 行 (3001, 1001) / (3001, 1002)
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

	// Story 11.2：跑 official migration（migrate.Up()）拿到 0001 ~ 0008 全部 8
	// 张表（含 rooms / room_members）；不再 inline CREATE TABLE。
	mig, err := migrate.New(dsn, migrationsPathForWS(t))
	if err != nil {
		_ = rawDB.Close()
		_ = pool.Purge(resource)
		t.Fatalf("migrate.New: %v", err)
	}
	migCtx, migCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer migCancel()
	if err := mig.Up(migCtx); err != nil {
		_ = mig.Close()
		_ = rawDB.Close()
		_ = pool.Purge(resource)
		t.Fatalf("migrate.Up: %v", err)
	}
	_ = mig.Close()

	// 插 fixture：1 个 active 房间 + 2 个 member（user_id=1001 / 1002）。
	// rooms.id=3001 + creator_user_id=1001 显式赋值（让既有 case 的硬编码期望稳定）。
	fixtureStmts := []string{
		`INSERT INTO rooms (id, creator_user_id, status, max_members) VALUES (3001, 1001, 1, 4)`,
		`INSERT INTO room_members (room_id, user_id) VALUES (3001, 1001), (3001, 1002)`,
	}
	for _, stmt := range fixtureStmts {
		if _, err := rawDB.Exec(stmt); err != nil {
			_ = pool.Purge(resource)
			t.Fatalf("exec fixture %q: %v", stmt, err)
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
	builder := wsapp.NewPlaceholderSnapshotBuilder(repo)
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "test", builder)
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

// TestWSIntegration_HeartbeatTimeout_Closes4005（Story 10.4 集成测试）：
//   - 启 mysql + room_members fixture（与既有 case 同 fixture）
//   - 用 newHeartbeatScannerForTest 注入 timeoutMs=2000ms / interval=200ms
//   - 启动 httptest gateway + go scanner.Run
//   - WS Dial 拨连成功 → 收 snapshot → **静默** 3 秒（不发 ping）
//   - conn.ReadMessage 应返 *websocket.CloseError，Code=4005, Text="heartbeat timeout"
//   - SessionManager.ListAllSessions 应为空（scanner 已清理）
//
// V1 §12.1 钦定 4005 = transient network failure；客户端**应**自动重连
// （iOS Story 12.5 落地）。
func TestWSIntegration_HeartbeatTimeout_Closes4005(t *testing.T) {
	gormDB, cleanup := startMySQLWithRoomMemberFixture(t)
	defer cleanup()

	wsURL, signer, mgr, ts := startGatewayWithRealMySQL(t, gormDB)
	defer mgr.Close()
	defer ts.Close()

	// 用同包内的 unexported helper newHeartbeatScannerForTest 注入小 interval +
	// 小 timeoutMs（这里 ws_integration_test.go 与 heartbeat_scanner.go 同
	// package ws_test —— 等等，本文件是 package ws_test 黑盒测试包，需要走 export_test.go
	// 的 exported helper）。改用 wsapp.NewHeartbeatScannerForTest（与 ws_test.go 一致）。
	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 2000 /*ms*/, 200*time.Millisecond, nil)

	scannerCtx, scannerCancel := context.WithCancel(context.Background())
	defer scannerCancel()
	go scanner.Run(scannerCtx)

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

	// **静默** —— 不发 ping，让 server 端 lastHeartbeatAt 停留在握手 + readLoop
	// 收到首条 message（不会有，因为不发）的状态。timeoutMs=2s + interval=200ms
	// → 至多 ~2.2s 后 scanner 触发 close 4005。
	//
	// 5s read deadline 给足窗口。
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected *websocket.CloseError, got %T %v", err, err)
	}
	if closeErr.Code != 4005 {
		t.Errorf("close code = %d, want 4005 (V1 §12.1 heartbeat timeout)", closeErr.Code)
	}
	if closeErr.Text != "heartbeat timeout" {
		t.Errorf("close reason = %q, want %q (V1 §12.1 字面量)", closeErr.Text, "heartbeat timeout")
	}

	// SessionManager.ListAllSessions 应为空（scanner 已触发 CloseWithCode →
	// Session.Close → notifier.notifyClosed → manager.Unregister 清出索引）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListAllSessions(context.Background())) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := len(mgr.ListAllSessions(context.Background())); got != 0 {
		t.Errorf("ListAllSessions len = %d after timeout close, want 0 (Unregister hook 未触发？)", got)
	}
}

// TestWSIntegration_BroadcastToRoom_3Clients_AllReceive（Story 10.5 集成测试）：
//   - 复用 startMySQLWithRoomMemberFixture（默认含 room=3001 + user 1001/1002）
//   - 在测试体内额外插入 user 1003 到 room=3001，让 fixture 含 3 个 user
//   - 启动 httptest gateway → 用 3 个不同 token 拨连 3 个 WS Dialer
//   - 各自收到 placeholder snapshot 后 → 调 BroadcastToRoom(ctx, sessionMgr, 3001, msg)
//   - 验证：
//     1. 返 (3, nil)
//     2. 3 个 conn 都 ReadMessage 收到同 msg
//     3. 一个 client 主动 Close → 等 manager Unregister → 再调 BroadcastToRoom →
//        剩 2 个 client 收到；返 (2, nil)；无 error
//
// **关键**：本测试用**生产路径** BroadcastToRoom（不是 ForTest 变体）— 通过
// 显式 ReadMessage 等所有 fanout goroutine 把 msg 写到 conn 上 → 验证生产
// fire-and-forget 行为。
func TestWSIntegration_BroadcastToRoom_3Clients_AllReceive(t *testing.T) {
	gormDB, cleanup := startMySQLWithRoomMemberFixture(t)
	defer cleanup()

	// 扩展 fixture：插入 user 1003 到 room=3001，让 room 含 3 个 user
	rawDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("gormDB.DB: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO room_members (room_id, user_id) VALUES (3001, 1003)`); err != nil {
		t.Fatalf("insert user 1003: %v", err)
	}
	// **Story 11.2 后**：rooms 表无 member_count 列（与 prod 0007 schema 对齐 ——
	// memberCount 由 placeholderSnapshotBuilder 从 ListMembers 计算 len()）；
	// 因此此处不再 UPDATE rooms.member_count。

	wsURL, signer, mgr, ts := startGatewayWithRealMySQL(t, gormDB)
	defer mgr.Close()
	defer ts.Close()

	// 拨 3 个 conn
	conns := make([]*websocket.Conn, 0, 3)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	for _, uid := range []uint64{1001, 1002, 1003} {
		token, err := signer.Sign(uid, 3600)
		if err != nil {
			t.Fatalf("Sign uid=%d: %v", uid, err)
		}
		url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("Dial uid=%d: %v", uid, err)
		}
		conns = append(conns, conn)

		// 读 snapshot
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read snapshot uid=%d: %v", uid, err)
		}
	}

	// 等 3 个 Session 全部注册到 manager
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListSessionsByRoomID(context.Background(), 3001)) >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := len(mgr.ListSessionsByRoomID(context.Background(), 3001)); got != 3 {
		t.Fatalf("ListSessionsByRoomID(3001) = %d, want 3", got)
	}

	// 调 BroadcastToRoom 推送 member.joined-shape 消息（生产路径 fire-and-forget）
	msg1 := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"4001","nickname":""},"ts":1234567890}`)
	sent, err := wsapp.BroadcastToRoom(context.Background(), mgr, 3001, msg1)
	if err != nil {
		t.Fatalf("BroadcastToRoom err: %v", err)
	}
	if sent != 3 {
		t.Errorf("sent = %d, want 3", sent)
	}

	// 3 个 conn 都应收到 msg1
	for i, conn := range conns {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("conn[%d] ReadMessage msg1: %v", i, err)
		}
		var env map[string]any
		if err := json.Unmarshal(msg, &env); err != nil {
			t.Errorf("conn[%d] unmarshal msg1: %v", i, err)
			continue
		}
		if env["type"] != "member.joined" {
			t.Errorf("conn[%d] type = %v, want member.joined", i, env["type"])
		}
	}

	// 主动关 conn[1]（user 1002）
	conns[1].Close()

	// 等 manager Unregister user 1002 完成（list 长度从 3 变成 2）
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListSessionsByRoomID(context.Background(), 3001)) <= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := len(mgr.ListSessionsByRoomID(context.Background(), 3001)); got != 2 {
		t.Fatalf("after close conn[1], ListSessionsByRoomID(3001) = %d, want 2", got)
	}

	// 再调 BroadcastToRoom：仅剩 2 个 Session，sent=2
	msg2 := []byte(`{"type":"member.left","requestId":"","payload":{"userId":"1002","nickname":""},"ts":1234567891}`)
	sent2, err := wsapp.BroadcastToRoom(context.Background(), mgr, 3001, msg2)
	if err != nil {
		t.Fatalf("second BroadcastToRoom err: %v", err)
	}
	if sent2 != 2 {
		t.Errorf("second sent = %d, want 2", sent2)
	}

	// conn[0] / conn[2] 都收到 msg2
	for _, idx := range []int{0, 2} {
		_ = conns[idx].SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conns[idx].ReadMessage()
		if err != nil {
			t.Fatalf("conn[%d] ReadMessage msg2: %v", idx, err)
		}
		var env map[string]any
		if err := json.Unmarshal(msg, &env); err != nil {
			t.Errorf("conn[%d] unmarshal msg2: %v", idx, err)
			continue
		}
		if env["type"] != "member.left" {
			t.Errorf("conn[%d] type = %v, want member.left", idx, env["type"])
		}
	}
}
