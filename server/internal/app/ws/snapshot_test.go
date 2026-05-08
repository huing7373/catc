package ws_test

// snapshot_test.go：Story 10.7 SnapshotBuilder / Snapshot / SendRoomSnapshot 单测。
//
// 测试策略（black-box `package ws_test`）：
//   - fakeBuilder 实装 wsapp.SnapshotBuilder：让 Case 1 / Case 2 直接控 build 结果
//     与 error
//   - fakeRoomMemberRepo 复用自 ws_test.go 的 stubRoomMemberRepo（同包同文件夹）：
//     让 Case 3 走真实 placeholderSnapshotBuilder.BuildSnapshot 路径
//   - newPipeConn 用 httptest.Server + Upgrader 构造配对的 server / client
//     *websocket.Conn（与 session_close_internal_test.go newPipeWebsocketConn
//     同模式，但因为本测试在 _test 包外不能直接复用 internal helper，本文件
//     局部实装一份）
//   - Case 1 验 happy path（snapshot 字段 + envelope wire 格式）
//   - Case 2 验 BuildSnapshot 失败 → SendRoomSnapshot 返 error + 不写 wire
//   - Case 3 验 placeholderSnapshotBuilder 真实路径 + 字段降级语义
//   - Case 4 验空房间（0 成员）—— Members 序列化为 []，不是 null
//   - Case 5 验 ctx cancel 透传

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// wsappRosterRow 本地别名让 Story 11.7 realSnapshotBuilder 单测的 stub 配置行
// 简洁；与 mysql.RosterRow 等价（type alias）。
type wsappRosterRow = mysql.RosterRow

// ---------- 测试 helper ----------

// fakeBuilder 实装 wsapp.SnapshotBuilder；走 stub 字段。
type fakeBuilder struct {
	snap wsapp.Snapshot
	err  error
}

func (f *fakeBuilder) BuildSnapshot(_ context.Context, _ uint64) (wsapp.Snapshot, error) {
	if f.err != nil {
		return wsapp.Snapshot{}, f.err
	}
	return f.snap, nil
}

// snapEnvelopeForTest 是 serverEnvelope 的测试本地 mirror（unmarshal 用，因为
// serverEnvelope 是 unexported 不能跨包访问；wire JSON 完全 1:1 对齐）。
type snapEnvelopeForTest struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	Payload   json.RawMessage `json:"payload"`
	Ts        int64           `json:"ts"`
}

// newPipeWSConnPair 用 httptest.Server + Upgrader 构造配对的 server-side /
// client-side *websocket.Conn 对。SendRoomSnapshot 直接对 server-side conn
// 写入；client-side 用 ReadMessage 读出 frame 验证 wire 格式。
//
// 与 ws_test.go startGatewayServer 不同：本 helper **不**起完整 gateway，仅
// 暴露一对 conn 让 SendRoomSnapshot 单独可测；测试结束 cleanup 关 ts + 双 conn。
func newPipeWSConnPair(t *testing.T) (server, client *websocket.Conn, cleanup func()) {
	t.Helper()
	upgrader := &websocket.Upgrader{}

	connCh := make(chan *websocket.Conn, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}
		connCh <- c
	})
	ts := httptest.NewServer(mux)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	u, err := url.Parse(wsURL)
	if err != nil {
		ts.Close()
		t.Fatalf("url.Parse: %v", err)
	}

	netConn, err := net.Dial("tcp", u.Host)
	if err != nil {
		ts.Close()
		t.Fatalf("net.Dial: %v", err)
	}
	clientConn, _, err := websocket.NewClient(netConn, u, http.Header{}, 1024, 1024)
	if err != nil {
		ts.Close()
		t.Fatalf("websocket.NewClient: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-connCh:
	case e := <-errCh:
		ts.Close()
		t.Fatalf("server upgrade: %v", e)
	case <-time.After(2 * time.Second):
		ts.Close()
		t.Fatal("server upgrade timeout")
	}

	cleanup = func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		ts.Close()
	}
	return serverConn, clientConn, cleanup
}

// ---------- Case 1: SendRoomSnapshot Happy Path（≥ 2 成员） ----------

// TestSendRoomSnapshot_Happy_TwoMembers: stub builder 返 2 成员 snapshot →
// SendRoomSnapshot 把 envelope 写到 conn → client 端读 frame → JSON 字段
// 与 V1 §12.3 placeholder 行严格对齐。
func TestSendRoomSnapshot_Happy_TwoMembers(t *testing.T) {
	stubBuilder := &fakeBuilder{
		snap: wsapp.Snapshot{
			Room: wsapp.SnapshotRoom{ID: "100", MaxMembers: 4, MemberCount: 2},
			Members: []wsapp.SnapshotMember{
				{UserID: "1001", Nickname: "", Pet: &wsapp.SnapshotPet{PetID: "", CurrentState: 1}},
				{UserID: "1002", Nickname: "", Pet: &wsapp.SnapshotPet{PetID: "", CurrentState: 1}},
			},
		},
	}
	serverConn, clientConn, cleanup := newPipeWSConnPair(t)
	defer cleanup()

	if err := wsapp.SendRoomSnapshot(context.Background(), serverConn, 100, stubBuilder, time.Second); err != nil {
		t.Fatalf("SendRoomSnapshot: %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var env snapEnvelopeForTest
	if err := json.Unmarshal(frame, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v (frame=%s)", err, string(frame))
	}
	if env.Type != "room.snapshot" {
		t.Errorf("type = %q, want room.snapshot", env.Type)
	}
	if env.RequestID != "" {
		t.Errorf("requestId = %q, want empty (主动推送类)", env.RequestID)
	}
	if env.Ts <= 0 {
		t.Errorf("ts = %d, want > 0 (unix ms)", env.Ts)
	}

	var snap wsapp.Snapshot
	if err := json.Unmarshal(env.Payload, &snap); err != nil {
		t.Fatalf("unmarshal payload: %v (payload=%s)", err, string(env.Payload))
	}
	if snap.Room.ID != "100" {
		t.Errorf("room.id = %q, want 100", snap.Room.ID)
	}
	if snap.Room.MaxMembers != 4 {
		t.Errorf("room.maxMembers = %d, want 4", snap.Room.MaxMembers)
	}
	if snap.Room.MemberCount != 2 {
		t.Errorf("room.memberCount = %d, want 2", snap.Room.MemberCount)
	}
	if len(snap.Members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(snap.Members))
	}
	if snap.Members[0].UserID != "1001" {
		t.Errorf("members[0].userId = %q, want 1001", snap.Members[0].UserID)
	}
	if snap.Members[0].Nickname != "" {
		t.Errorf("members[0].nickname = %q, want empty (placeholder)", snap.Members[0].Nickname)
	}
	if snap.Members[0].Pet == nil {
		t.Fatal("members[0].pet = nil, want non-nil (placeholder pointer always non-nil)")
	}
	if snap.Members[0].Pet.PetID != "" {
		t.Errorf("members[0].pet.petId = %q, want empty (placeholder)", snap.Members[0].Pet.PetID)
	}
	if snap.Members[0].Pet.CurrentState != 1 {
		t.Errorf("members[0].pet.currentState = %d, want 1 (placeholder)", snap.Members[0].Pet.CurrentState)
	}
}

// ---------- Case 2: BuildSnapshot 返 error → SendRoomSnapshot 返 error，**不**写 wire ----------

// TestSendRoomSnapshot_BuildError_ReturnsErrorNoWrite: builder 返 error →
// SendRoomSnapshot 返 wrapped error，并且**不**调 conn.WriteMessage（client 端
// ReadMessage 应超时 / EOF）。
//
// **关键不变量**（lesson 2026-05-06-ws-frozen-section-authz-and-snapshot-coherence-r6.md）：
// snapshot 失败**不**走推 type=error 消息路径；caller 走 close 1011。
func TestSendRoomSnapshot_BuildError_ReturnsErrorNoWrite(t *testing.T) {
	stubBuilder := &fakeBuilder{err: errors.New("simulated DB error")}
	serverConn, clientConn, cleanup := newPipeWSConnPair(t)
	defer cleanup()

	err := wsapp.SendRoomSnapshot(context.Background(), serverConn, 100, stubBuilder, time.Second)
	if err == nil {
		t.Fatal("SendRoomSnapshot should return error when BuildSnapshot fails")
	}
	if !strings.Contains(err.Error(), "simulated DB error") {
		t.Errorf("error %q should wrap original error", err.Error())
	}

	// **关键不变量**：不能写 type=error 消息到 wire；client 端 ReadMessage 应超时
	_ = clientConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, frame, readErr := clientConn.ReadMessage()
	if readErr == nil {
		t.Fatalf("clientConn unexpectedly read frame %q (snapshot build failed should not write any frame)", string(frame))
	}
}

// ---------- Case 3: placeholderSnapshotBuilder.BuildSnapshot 真实路径 ----------

// TestPlaceholderSnapshotBuilder_BuildSnapshot_FullRoster:
// fakeRoomMemberRepo（复用 ws_test.go 的 stubRoomMemberRepo）→ ListMembers 返
// [1001, 1002] → BuildSnapshot 返 Snapshot 字段对齐 V1 §12.3 placeholder 行。
//
// 不变量（V1 §12.3 末"不变量"小节钦定）：MemberCount == len(Members)。
func TestPlaceholderSnapshotBuilder_BuildSnapshot_FullRoster(t *testing.T) {
	repo := &stubRoomMemberRepo{
		listMembersFn: func(_ context.Context, _ uint64) ([]uint64, error) {
			return []uint64{1001, 1002}, nil
		},
	}
	builder := wsapp.NewPlaceholderSnapshotBuilder(repo)

	snap, err := builder.BuildSnapshot(context.Background(), 100)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}

	if snap.Room.ID != "100" {
		t.Errorf("room.id = %q, want 100", snap.Room.ID)
	}
	if snap.Room.MaxMembers != 4 {
		t.Errorf("room.maxMembers = %d, want 4", snap.Room.MaxMembers)
	}
	if snap.Room.MemberCount != 2 {
		t.Errorf("room.memberCount = %d, want 2", snap.Room.MemberCount)
	}
	if len(snap.Members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(snap.Members))
	}
	if snap.Room.MemberCount != len(snap.Members) {
		t.Errorf("invariant violated: MemberCount=%d != len(Members)=%d", snap.Room.MemberCount, len(snap.Members))
	}
	// userId 字符串化 + placeholder 字段降级
	if snap.Members[0].UserID != "1001" {
		t.Errorf("members[0].userId = %q, want 1001", snap.Members[0].UserID)
	}
	if snap.Members[1].UserID != "1002" {
		t.Errorf("members[1].userId = %q, want 1002", snap.Members[1].UserID)
	}
	for i, m := range snap.Members {
		if m.Nickname != "" {
			t.Errorf("members[%d].nickname = %q, want empty (placeholder)", i, m.Nickname)
		}
		if m.Pet == nil {
			t.Fatalf("members[%d].pet = nil, want non-nil (placeholder pointer always non-nil)", i)
		}
		if m.Pet.PetID != "" {
			t.Errorf("members[%d].pet.petId = %q, want empty (placeholder)", i, m.Pet.PetID)
		}
		if m.Pet.CurrentState != 1 {
			t.Errorf("members[%d].pet.currentState = %d, want 1 (placeholder)", i, m.Pet.CurrentState)
		}
	}
}

// ---------- Case 4: 空房间 → Members 序列化为 []（非 null） ----------

// TestPlaceholderSnapshotBuilder_BuildSnapshot_EmptyRoom:
// ListMembers 返 [] → Snapshot{MemberCount: 0, Members: []}。
//
// 关键不变量（V1 §12.3 不变量小节）：JSON 序列化时 members 为 `[]` 而非 `null`，
// 让 client 解析无歧义（"空房间"≠"无字段"）。
func TestPlaceholderSnapshotBuilder_BuildSnapshot_EmptyRoom(t *testing.T) {
	repo := &stubRoomMemberRepo{
		listMembersFn: func(_ context.Context, _ uint64) ([]uint64, error) {
			return []uint64{}, nil
		},
	}
	builder := wsapp.NewPlaceholderSnapshotBuilder(repo)

	snap, err := builder.BuildSnapshot(context.Background(), 100)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if snap.Room.MemberCount != 0 {
		t.Errorf("room.memberCount = %d, want 0", snap.Room.MemberCount)
	}
	if snap.Members == nil {
		t.Fatal("Members must be non-nil empty slice (not nil) so JSON serializes as `[]` not `null`")
	}
	if len(snap.Members) != 0 {
		t.Errorf("len(members) = %d, want 0", len(snap.Members))
	}

	// 序列化后必须是 `[]` 字符串，不是 `null`
	bytes, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(bytes), `"members":[]`) {
		t.Errorf("snapshot JSON must contain `\"members\":[]` (got %s)", string(bytes))
	}
	if strings.Contains(string(bytes), `"members":null`) {
		t.Errorf("snapshot JSON must NOT contain `\"members\":null` (got %s)", string(bytes))
	}
}

// ---------- Case 5: ctx 透传 ----------

// TestPlaceholderSnapshotBuilder_BuildSnapshot_CtxCancel:
// ctx 已 cancel → fakeRepo.ListMembers 返 ctx.Err → BuildSnapshot 返 wrapped
// error。验证 ctx 严格透传（ADR-0007）。
func TestPlaceholderSnapshotBuilder_BuildSnapshot_CtxCancel(t *testing.T) {
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, _ uint64) ([]uint64, error) {
			// 模拟"GORM .WithContext(ctx) 在 ctx cancel 时返 ctx.Err"行为
			return nil, ctx.Err()
		},
	}
	builder := wsapp.NewPlaceholderSnapshotBuilder(repo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := builder.BuildSnapshot(ctx, 100)
	if err == nil {
		t.Fatal("BuildSnapshot should return error when ctx is cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error chain should contain context.Canceled; got %v", err)
	}
}

// ---------- Story 11.7: realSnapshotBuilder.BuildSnapshot 单测 ----------
//
// 测试策略：复用既有 stubRoomMemberRepo（ws_test.go 加 listRosterByRoomIDFn 字段）
// 配置 ListRosterByRoomID 行为 → 调 wsapp.NewRealSnapshotBuilder + BuildSnapshot
// → assert Snapshot 字段语义对齐 V1 §12.3 going-forward 契约（nickname 真实值 /
// pet pointer pet-less → nil / pet 非 pet-less → 非 nil + petId 真实值 / currentState=1 /
// memberCount === len(Members) 不变量）。
//
// 不依赖网络 / 真实 MySQL，纯 unit；dockertest 集成测试由 snapshot_integration_test.go
// 覆盖（Story 11.7 AC6）。

// uint64Ptr 是 *uint64 字面值便利 helper（避免每个 case 写 var x uint64 = N; &x）。
func uint64Ptr(v uint64) *uint64 { return &v }

// TestRealSnapshotBuilder_BuildSnapshot_Happy_3Members_With1PetLess:
// stub 返 3 行 RosterRow（前 2 行 PetID 非 nil + 第 3 行 PetID nil 模拟 pet-less）→
// Snapshot.Members 长度 3 + Members[0/1].Pet ≠ nil + Members[0].Pet.PetID 真实
// 字符串化值 + Members[0].Pet.CurrentState == 1 + Members[2].Pet == nil（pet-less）+
// Members[0/1/2].Nickname 为 RosterRow.Nickname 真实值 + Snapshot.Room.MemberCount
// == 3 == len(Members) 不变量保持 + Snapshot.Room.MaxMembers == 4 + Snapshot.Room.ID
// 字符串化 + Members 顺序与 stub 入参顺序一致（ListRosterByRoomID 已 ORDER BY
// joined_at ASC，stub 直接控顺序）。
func TestRealSnapshotBuilder_BuildSnapshot_Happy_3Members_With1PetLess(t *testing.T) {
	repo := &stubRoomMemberRepo{
		listRosterByRoomIDFn: func(_ context.Context, _ uint64) ([]wsappRosterRow, error) {
			return []wsappRosterRow{
				{UserID: 1001, Nickname: "Alice", AvatarURL: "https://cdn.example.com/a.png", PetID: uint64Ptr(8001)},
				{UserID: 1002, Nickname: "Bob", AvatarURL: "https://cdn.example.com/b.png", PetID: uint64Ptr(8002)},
				{UserID: 1003, Nickname: "Charlie", AvatarURL: "https://cdn.example.com/c.png", PetID: nil}, // pet-less
			}, nil
		},
	}
	builder := wsapp.NewRealSnapshotBuilder(repo)

	snap, err := builder.BuildSnapshot(context.Background(), 3001)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}

	if snap.Room.ID != "3001" {
		t.Errorf("room.id = %q, want 3001 (BIGINT 字符串化)", snap.Room.ID)
	}
	if snap.Room.MaxMembers != 4 {
		t.Errorf("room.maxMembers = %d, want 4", snap.Room.MaxMembers)
	}
	if snap.Room.MemberCount != 3 {
		t.Errorf("room.memberCount = %d, want 3", snap.Room.MemberCount)
	}
	if len(snap.Members) != 3 {
		t.Fatalf("len(members) = %d, want 3", len(snap.Members))
	}
	if snap.Room.MemberCount != len(snap.Members) {
		t.Errorf("invariant violated: MemberCount=%d != len(Members)=%d", snap.Room.MemberCount, len(snap.Members))
	}

	// 顺序断言（ListRosterByRoomID ORDER BY joined_at ASC，stub 直接控顺序）
	wantUserIDs := []string{"1001", "1002", "1003"}
	wantNicknames := []string{"Alice", "Bob", "Charlie"}
	for i, want := range wantUserIDs {
		if snap.Members[i].UserID != want {
			t.Errorf("members[%d].userId = %q, want %s", i, snap.Members[i].UserID, want)
		}
		if snap.Members[i].Nickname != wantNicknames[i] {
			t.Errorf("members[%d].nickname = %q, want %s (真实值，going-forward)", i, snap.Members[i].Nickname, wantNicknames[i])
		}
	}

	// Members[0/1].Pet 非 nil + petId 真实字符串化 + currentState 硬编码 1
	if snap.Members[0].Pet == nil {
		t.Fatal("members[0].pet = nil, want non-nil")
	}
	if snap.Members[0].Pet.PetID != "8001" {
		t.Errorf("members[0].pet.petId = %q, want 8001 (真实值字符串化)", snap.Members[0].Pet.PetID)
	}
	if snap.Members[0].Pet.CurrentState != 1 {
		t.Errorf("members[0].pet.currentState = %d, want 1 (节点 4 阶段固定)", snap.Members[0].Pet.CurrentState)
	}
	if snap.Members[1].Pet == nil {
		t.Fatal("members[1].pet = nil, want non-nil")
	}
	if snap.Members[1].Pet.PetID != "8002" {
		t.Errorf("members[1].pet.petId = %q, want 8002", snap.Members[1].Pet.PetID)
	}

	// Members[2].Pet == nil（pet-less，LEFT JOIN pets 行 NULL → wire `pet: null`）
	if snap.Members[2].Pet != nil {
		t.Errorf("members[2].pet = %+v, want nil (pet-less)", snap.Members[2].Pet)
	}

	// JSON 序列化必须含 `"pet":null`（pet-less）+ `"pet":{...}`（非 pet-less）
	bytes, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	jsonStr := string(bytes)
	if !strings.Contains(jsonStr, `"pet":null`) {
		t.Errorf("snapshot JSON must contain `\"pet\":null` for pet-less member (got %s)", jsonStr)
	}
	if !strings.Contains(jsonStr, `"petId":"8001"`) {
		t.Errorf("snapshot JSON must contain `\"petId\":\"8001\"` (got %s)", jsonStr)
	}

	// 范围红线：JSON 必须不含 avatarUrl / equips / isOnline 字段（V1 §12.3
	// going-forward 契约 + 本 story 范围红线）
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

// TestRealSnapshotBuilder_BuildSnapshot_PetLess_SingleMember:
// stub 返 1 行 RosterRow（PetID nil 模拟单成员 pet-less 房间）→ Snapshot.Members
// 长度 1 + Members[0].Pet == nil + Members[0].Nickname 真实值 + MemberCount == 1.
func TestRealSnapshotBuilder_BuildSnapshot_PetLess_SingleMember(t *testing.T) {
	repo := &stubRoomMemberRepo{
		listRosterByRoomIDFn: func(_ context.Context, _ uint64) ([]wsappRosterRow, error) {
			return []wsappRosterRow{
				{UserID: 1001, Nickname: "Solo", AvatarURL: "", PetID: nil}, // pet-less 单成员
			}, nil
		},
	}
	builder := wsapp.NewRealSnapshotBuilder(repo)

	snap, err := builder.BuildSnapshot(context.Background(), 3001)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if snap.Room.MemberCount != 1 {
		t.Errorf("room.memberCount = %d, want 1", snap.Room.MemberCount)
	}
	if len(snap.Members) != 1 {
		t.Fatalf("len(members) = %d, want 1", len(snap.Members))
	}
	if snap.Members[0].UserID != "1001" {
		t.Errorf("members[0].userId = %q, want 1001", snap.Members[0].UserID)
	}
	if snap.Members[0].Nickname != "Solo" {
		t.Errorf("members[0].nickname = %q, want Solo", snap.Members[0].Nickname)
	}
	if snap.Members[0].Pet != nil {
		t.Errorf("members[0].pet = %+v, want nil (pet-less)", snap.Members[0].Pet)
	}

	bytes, _ := json.Marshal(snap)
	if !strings.Contains(string(bytes), `"pet":null`) {
		t.Errorf("snapshot JSON must contain `\"pet\":null` for pet-less single member (got %s)", string(bytes))
	}
}

// TestRealSnapshotBuilder_BuildSnapshot_DBError_Propagates:
// stub 返 raw DB error → BuildSnapshot 返 (Snapshot{}, err 包装)；assert err 内含
// "ws snapshot: list roster" 子串 + raw error 通过 errors.Is / errors.Unwrap 可访问
// （与 placeholder 错误处理路径一致）。
func TestRealSnapshotBuilder_BuildSnapshot_DBError_Propagates(t *testing.T) {
	rawErr := errors.New("simulated DB connection lost")
	repo := &stubRoomMemberRepo{
		listRosterByRoomIDFn: func(_ context.Context, _ uint64) ([]wsappRosterRow, error) {
			return nil, rawErr
		},
	}
	builder := wsapp.NewRealSnapshotBuilder(repo)

	snap, err := builder.BuildSnapshot(context.Background(), 3001)
	if err == nil {
		t.Fatal("BuildSnapshot should return error when ListRosterByRoomID fails")
	}
	if !strings.Contains(err.Error(), "ws snapshot: list roster") {
		t.Errorf("error %q should contain `ws snapshot: list roster` prefix", err.Error())
	}
	if !errors.Is(err, rawErr) {
		t.Errorf("error chain should wrap raw error; got %v", err)
	}
	// 关键不变量：error 时返 zero-value Snapshot（不是 partial fill）
	if snap.Room.MemberCount != 0 || len(snap.Members) != 0 {
		t.Errorf("BuildSnapshot on error should return zero-value Snapshot; got %+v", snap)
	}
}

// TestRealSnapshotBuilder_BuildSnapshot_EmptyRoom_Returns0Members:
// stub 返 []RosterRow{} 空数组 → Snapshot.Members 长度 0 + Snapshot.Members 不为 nil
// （应为空 slice，序列化为 `"members": []` 不是 `null`，与 V1 §12.3 不变量一致）+
// MemberCount == 0.
func TestRealSnapshotBuilder_BuildSnapshot_EmptyRoom_Returns0Members(t *testing.T) {
	repo := &stubRoomMemberRepo{
		listRosterByRoomIDFn: func(_ context.Context, _ uint64) ([]wsappRosterRow, error) {
			return []wsappRosterRow{}, nil
		},
	}
	builder := wsapp.NewRealSnapshotBuilder(repo)

	snap, err := builder.BuildSnapshot(context.Background(), 3001)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if snap.Room.MemberCount != 0 {
		t.Errorf("room.memberCount = %d, want 0", snap.Room.MemberCount)
	}
	if snap.Members == nil {
		t.Fatal("Members must be non-nil empty slice (not nil) so JSON serializes as `[]` not `null`")
	}
	if len(snap.Members) != 0 {
		t.Errorf("len(members) = %d, want 0", len(snap.Members))
	}

	bytes, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(bytes), `"members":[]`) {
		t.Errorf("snapshot JSON must contain `\"members\":[]` for empty room (got %s)", string(bytes))
	}
}
