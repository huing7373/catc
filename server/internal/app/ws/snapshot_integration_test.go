//go:build integration
// +build integration

// snapshot_integration_test.go: Story 10.7 集成测试 —— 真实 WS 客户端拨号到挂了
// 真实 RoomMemberRepo + placeholderSnapshotBuilder 的 gateway，验证握手成功后
// 第一条 frame 是 placeholder room.snapshot，全 roster 字段对齐 V1 §12.3 placeholder
// 行 + lesson 2026-05-06-ws-snapshot-placeholder-full-roster-and-disconnect-broadcast-r8.md
// 钦定的"全 roster 不变量"。
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑；
// 只在 `bash scripts/build.sh --integration` 触发。
//
// 复用 ws_integration_test.go 既有 dockertest MySQL fixture（startMySQLWithRoomMemberFixture）
// + WS dial helper（startGatewayWithRealMySQL）—— **不**重复造轮子。

package ws_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	mysqlrepo "github.com/huing/cat/server/internal/repo/mysql"
)

// TestWSIntegration_PlaceholderSnapshot_FullRoster:
// fixture 房间 3001 含 2 成员（user_id=1001 / 1002）→ user 1001 拨连 →
// 第一条 frame 是 type=room.snapshot，payload 全 roster：
//   - room.id="3001"、room.maxMembers=4、room.memberCount=2
//   - members 长度 = 2，包含 {"1001","1002"}（顺序按 user_id ASC，与 ListMembers
//     ORDER BY user_id ASC 一致）
//   - members[].nickname="" / members[].pet.petId="" / members[].pet.currentState=1
//
// **关键不变量**（lesson 2026-05-06-...-full-roster-and-disconnect-broadcast-r8.md
// 钦定）：placeholder snapshot 必须返回**全 roster**，不能只返 1001 自己（房间已
// 有 ≥2 成员时漏返其他成员会让 client 把 snapshot 当 authoritative state 错误清空
// 已加载的 roster）。
//
// **本 case 与既有 TestWSIntegration_HappyPath_FirstMessageIsSnapshot 的区别**：
// 既有 case 是 Story 10.3 inline 实装的回归覆盖；本 case 是 Story 10.7 抽离重构
// 后的回归覆盖（同 wire 行为应保留）—— 两者并存，让 inline 路径与抽离路径都被
// 持续锁住，避免后续 fix-review 改动时无意打破任一路径。
func TestWSIntegration_PlaceholderSnapshot_FullRoster(t *testing.T) {
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
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env["type"] != "room.snapshot" {
		t.Fatalf("type = %v, want room.snapshot", env["type"])
	}
	if env["requestId"] != "" {
		t.Errorf("requestId = %v, want empty (主动推送类)", env["requestId"])
	}
	tsField, ok := env["ts"].(float64)
	if !ok || tsField <= 0 {
		t.Errorf("ts = %v, want > 0 (unix ms)", env["ts"])
	}

	payload, ok := env["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map", env["payload"])
	}
	room, ok := payload["room"].(map[string]any)
	if !ok {
		t.Fatalf("payload.room type = %T, want map", payload["room"])
	}
	if room["id"] != "3001" {
		t.Errorf("room.id = %v, want 3001", room["id"])
	}
	if int(room["maxMembers"].(float64)) != 4 {
		t.Errorf("room.maxMembers = %v, want 4", room["maxMembers"])
	}
	if int(room["memberCount"].(float64)) != 2 {
		t.Errorf("room.memberCount = %v, want 2 (fixture has 2 members; full-roster invariant)", room["memberCount"])
	}

	members, ok := payload["members"].([]any)
	if !ok {
		t.Fatalf("payload.members type = %T, want array", payload["members"])
	}
	if len(members) != 2 {
		t.Fatalf("len(members) = %d, want 2 (full roster: 1001 + 1002)", len(members))
	}

	// 验全 roster 字段对齐 V1 §12.3 placeholder 行；顺序按 user_id ASC（ListMembers 排序）
	expectedUserIDs := []string{"1001", "1002"}
	for i, m := range members {
		mm, ok := m.(map[string]any)
		if !ok {
			t.Fatalf("members[%d] type = %T, want map", i, m)
		}
		if mm["userId"] != expectedUserIDs[i] {
			t.Errorf("members[%d].userId = %v, want %s (ORDER BY user_id ASC)", i, mm["userId"], expectedUserIDs[i])
		}
		if mm["nickname"] != "" {
			t.Errorf("members[%d].nickname = %v, want empty (placeholder)", i, mm["nickname"])
		}
		pet, ok := mm["pet"].(map[string]any)
		if !ok {
			t.Fatalf("members[%d].pet type = %T, want map", i, mm["pet"])
		}
		if pet["petId"] != "" {
			t.Errorf("members[%d].pet.petId = %v, want empty (placeholder)", i, pet["petId"])
		}
		if int(pet["currentState"].(float64)) != 1 {
			t.Errorf("members[%d].pet.currentState = %v, want 1 (placeholder)", i, pet["currentState"])
		}
	}
}

// ============================================================
// Story 11.7 集成测试：realSnapshotBuilder.BuildSnapshot 真实 MySQL 路径
// ============================================================

// TestRealSnapshotBuilder_BuildSnapshot_Integration_FullRoster_With1PetLess:
// dockertest 真实 MySQL + seed users / pets（≥ 2 user 含 1 pet-less 边界）+
// 复用 startMySQLWithRoomMemberFixture 已 seed 的 rooms (id=3001, status=1) +
// room_members ((3001,1001),(3001,1002)) → 调 realSnapshotBuilder.BuildSnapshot
// 直接拿 Snapshot；assert：
//   - Snapshot.Members 长度 == 2（== seed 行数）
//   - Members[0].Nickname == seed users.nickname 真实值（非空字符串）
//   - Members[0].Pet.PetID == seed pets.id 字符串化值（非空字符串）
//   - Members[1].Pet == nil（pet-less seed 边界 —— 1002 用户没 seed pets 行）
//   - Members 顺序按 room_members.joined_at ASC（fixture 顺序：1001 → 1002）
//   - Snapshot.Room.MemberCount == 2 == len(Members) 不变量
//   - Snapshot.Room.ID 字符串化为 "3001"
//   - Snapshot.Room.MaxMembers == 4
//
// **本 case 与 ws_integration_test.go::TestWSIntegration_HappyPath_FirstMessageIsSnapshot
// 的区别**：既有 case 走 placeholderSnapshotBuilder（Story 10.7 默认 prod 实装；
// Story 11.7 已切到 real，但 startGatewayWithRealMySQL helper 仍用 placeholder
// 保留 10.7 既有 case 不回归）；本 case **直接构造** realSnapshotBuilder + 调
// BuildSnapshot 验证 Story 11.7 落地的真实 MySQL 路径，跳过 gateway / WS handshake
// 链路（不需要起 httptest server，更聚焦 builder 路径）。
//
// **不**起完整 WS gateway 端到端（11.9 才做完整 lifecycle 测试）；本 case 只测
// realSnapshotBuilder.BuildSnapshot 真实 MySQL 路径 + 直接构造 RoomMemberRepo
// 实例。**不**需要 Redis（V1 §12.3 going-forward 契约不消费 presence）。
func TestRealSnapshotBuilder_BuildSnapshot_Integration_FullRoster_With1PetLess(t *testing.T) {
	gormDB, cleanup := startMySQLWithRoomMemberFixture(t)
	defer cleanup()

	// fixture 已 seed：rooms (id=3001, creator_user_id=1001, status=1, max_members=4) +
	// room_members ((3001,1001),(3001,1002))；本 case 补 seed users (1001/1002) +
	// pets (1001 有 default pet / 1002 是 pet-less 不 seed pets 行)。
	rawDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("gormDB.DB: %v", err)
	}
	seedStmts := []string{
		`INSERT INTO users (id, guest_uid, nickname, avatar_url, status) VALUES (1001, 'uid-1001', 'Alice', 'https://cdn.example.com/a.png', 1)`,
		`INSERT INTO users (id, guest_uid, nickname, avatar_url, status) VALUES (1002, 'uid-1002', 'Bob', '', 1)`,
		`INSERT INTO pets (id, user_id, pet_type, name, current_state, is_default) VALUES (8001, 1001, 1, '小白', 1, 1)`,
		// **不** seed 1002 的 pets 行 → LEFT JOIN pets pet_id 列 NULL → pet-less 边界
	}
	for _, stmt := range seedStmts {
		if _, err := rawDB.Exec(stmt); err != nil {
			t.Fatalf("seed exec %q: %v", stmt, err)
		}
	}

	repo := mysqlrepo.NewRoomMemberRepo(gormDB)
	builder := wsapp.NewRealSnapshotBuilder(repo)

	snap, err := builder.BuildSnapshot(context.Background(), 3001)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}

	// room 字段
	if snap.Room.ID != "3001" {
		t.Errorf("room.id = %q, want 3001 (BIGINT 字符串化)", snap.Room.ID)
	}
	if snap.Room.MaxMembers != 4 {
		t.Errorf("room.maxMembers = %d, want 4", snap.Room.MaxMembers)
	}
	if snap.Room.MemberCount != 2 {
		t.Errorf("room.memberCount = %d, want 2 (fixture seed 2 members)", snap.Room.MemberCount)
	}

	// Members 长度 + 不变量
	if len(snap.Members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(snap.Members))
	}
	if snap.Room.MemberCount != len(snap.Members) {
		t.Errorf("invariant violated: MemberCount=%d != len(Members)=%d", snap.Room.MemberCount, len(snap.Members))
	}

	// 顺序按 room_members.joined_at ASC（fixture INSERT 顺序：1001 先，1002 后；
	// 同 batch INSERT 的 joined_at DEFAULT NOW() 同毫秒，但 InnoDB 物理顺序按 INSERT 排序）
	if snap.Members[0].UserID != "1001" {
		t.Errorf("members[0].userId = %q, want 1001 (joined_at ASC)", snap.Members[0].UserID)
	}
	if snap.Members[1].UserID != "1002" {
		t.Errorf("members[1].userId = %q, want 1002 (joined_at ASC)", snap.Members[1].UserID)
	}

	// Members[0] (1001): 真实 nickname + 非 nil Pet + petId 真实值
	if snap.Members[0].Nickname != "Alice" {
		t.Errorf("members[0].nickname = %q, want Alice (going-forward 真实值)", snap.Members[0].Nickname)
	}
	if snap.Members[0].Pet == nil {
		t.Fatal("members[0].pet = nil, want non-nil (1001 has default pet)")
	}
	if snap.Members[0].Pet.PetID != "8001" {
		t.Errorf("members[0].pet.petId = %q, want 8001 (BIGINT 字符串化)", snap.Members[0].Pet.PetID)
	}
	if snap.Members[0].Pet.CurrentState != 1 {
		t.Errorf("members[0].pet.currentState = %d, want 1 (节点 4 阶段固定)", snap.Members[0].Pet.CurrentState)
	}

	// Members[1] (1002): 真实 nickname + Pet == nil（pet-less 边界）
	if snap.Members[1].Nickname != "Bob" {
		t.Errorf("members[1].nickname = %q, want Bob", snap.Members[1].Nickname)
	}
	if snap.Members[1].Pet != nil {
		t.Errorf("members[1].pet = %+v, want nil (pet-less)", snap.Members[1].Pet)
	}

	// JSON 序列化 wire 验证：必含 `"pet":null`（pet-less）+ `"petId":"8001"`（非 pet-less）
	bytes, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	jsonStr := string(bytes)
	if !strings.Contains(jsonStr, `"pet":null`) {
		t.Errorf("snapshot JSON must contain `\"pet\":null` for pet-less member 1002 (got %s)", jsonStr)
	}
	if !strings.Contains(jsonStr, `"petId":"8001"`) {
		t.Errorf("snapshot JSON must contain `\"petId\":\"8001\"` for member 1001 (got %s)", jsonStr)
	}

	// 范围红线：JSON 必须不含 avatarUrl / equips / isOnline 字段
	if strings.Contains(jsonStr, "avatarUrl") {
		t.Errorf("snapshot JSON must NOT contain avatarUrl (range-red-line; got %s)", jsonStr)
	}
	if strings.Contains(jsonStr, "equips") {
		t.Errorf("snapshot JSON must NOT contain equips (range-red-line; got %s)", jsonStr)
	}
	if strings.Contains(jsonStr, "isOnline") {
		t.Errorf("snapshot JSON must NOT contain isOnline (range-red-line; going-forward 不下发; got %s)", jsonStr)
	}
}
