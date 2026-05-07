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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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
