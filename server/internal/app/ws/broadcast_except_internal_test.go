package ws

import (
	"context"
	"testing"
)

// TestBroadcastToRoomExcept_FiltersExcludeUserID_StubSessions：
// Story 11.8 r3 [P1] fix 单测 —— 注册 3 stub Session 在同 roomID 下，调
// BroadcastToRoomExcept(roomID, excludeUserID=second user, msg) 应只入队 2 个
// session 的 sendChan，被 exclude 的 session sendChan 应为空。
//
// 对应 V1 §12.3 行 2063 钦定的"广播范围：仅该房间内当前在线的其他 Session（不含
// 加入者自己）"语义；r3 修复 r2 引入的"joiner self-fanout"regression。
//
// **使用 stub Session**（与 broadcast_perf_internal_test.go 同模式）：避免真实
// httptest.Server 起 gateway goroutine 增加测试调度负载（可能放大其他 timing-
// sensitive test 的 flakiness）。
func TestBroadcastToRoomExcept_FiltersExcludeUserID_StubSessions(t *testing.T) {
	// **NOTE**: 不调 mgr.Close() —— stub Session 的 sendPriorityChan 字段没初始化
	// （makeStubSessionForBroadcast 只 init 了 sendChan），mgr.Close() 链路会调
	// Session.Close → closeInternal → close(sendPriorityChan) panic on nil channel。
	// 与既有 broadcast_perf_internal_test.go 同模式（也不调 mgr.Close()）。test 结束
	// 后 mgr 走 GC 回收，stub Session 没有 active goroutine 不会泄漏。
	mgr := NewSessionManager().(*sessionManager)

	const roomID uint64 = 9301
	s1 := makeStubSessionForBroadcast(1001, roomID)
	s2 := makeStubSessionForBroadcast(1002, roomID)
	s3 := makeStubSessionForBroadcast(1003, roomID)
	for _, s := range []*Session{s1, s2, s3} {
		if _, err := mgr.Register(context.Background(), s); err != nil {
			t.Fatalf("Register(uid=%d): %v", s.userID, err)
		}
	}

	msg := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"1002"},"ts":1234567890}`)

	// excludeUserID = 1002 → s2 不应收到 msg
	sent, err := BroadcastToRoomExcept(context.Background(), mgr, roomID, 1002, msg)
	if err != nil {
		t.Fatalf("BroadcastToRoomExcept: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2 (excluded 1 of 3)", sent)
	}

	if got := len(s1.sendChan); got != 1 {
		t.Errorf("s1 (uid=1001) sendChan len = %d, want 1", got)
	}
	if got := len(s2.sendChan); got != 0 {
		t.Errorf("s2 (uid=1002, excluded) sendChan len = %d, want 0 (filtered)", got)
	}
	if got := len(s3.sendChan); got != 1 {
		t.Errorf("s3 (uid=1003) sendChan len = %d, want 1", got)
	}
}

// TestBroadcastToRoomExcept_NoMatchingExclude_AllReceive_StubSessions：
// excludeUserID 不在房间内 → 所有 stub Session 都收到 msg（filter 0 个）；返 (3, nil)。
func TestBroadcastToRoomExcept_NoMatchingExclude_AllReceive_StubSessions(t *testing.T) {
	// **NOTE**: 不调 mgr.Close() —— stub Session 的 sendPriorityChan 字段没初始化
	// （makeStubSessionForBroadcast 只 init 了 sendChan），mgr.Close() 链路会调
	// Session.Close → closeInternal → close(sendPriorityChan) panic on nil channel。
	// 与既有 broadcast_perf_internal_test.go 同模式（也不调 mgr.Close()）。test 结束
	// 后 mgr 走 GC 回收，stub Session 没有 active goroutine 不会泄漏。
	mgr := NewSessionManager().(*sessionManager)

	const roomID uint64 = 9302
	s1 := makeStubSessionForBroadcast(1001, roomID)
	s2 := makeStubSessionForBroadcast(1002, roomID)
	s3 := makeStubSessionForBroadcast(1003, roomID)
	for _, s := range []*Session{s1, s2, s3} {
		if _, err := mgr.Register(context.Background(), s); err != nil {
			t.Fatalf("Register(uid=%d): %v", s.userID, err)
		}
	}

	msg := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"9999"},"ts":1234567890}`)

	// excludeUserID = 9999（不在房间内）→ 全部 3 个 Session 都收到
	sent, err := BroadcastToRoomExcept(context.Background(), mgr, roomID, 9999, msg)
	if err != nil {
		t.Fatalf("BroadcastToRoomExcept: %v", err)
	}
	if sent != 3 {
		t.Errorf("sent = %d, want 3 (no match for excludeUserID)", sent)
	}

	for i, s := range []*Session{s1, s2, s3} {
		if got := len(s.sendChan); got != 1 {
			t.Errorf("session[%d] (uid=%d) sendChan len = %d, want 1", i, s.userID, got)
		}
	}
}

// TestBroadcastToRoomExcept_EmptyRoom_ReturnsZero：
// 房间内 0 个 active Session → BroadcastToRoomExcept 返 (0, nil)；不 panic。
func TestBroadcastToRoomExcept_EmptyRoom_ReturnsZero(t *testing.T) {
	// 空房间无 stub Session 注册 → 不需要 mgr.Close() 路径触达 stub Session
	mgr := NewSessionManager()
	defer mgr.Close()

	sent, err := BroadcastToRoomExcept(context.Background(), mgr, 9999, 1001, []byte(`{"type":"x","requestId":"","payload":{},"ts":0}`))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0", sent)
	}
}

// TestBroadcastToRoomFanout_R11_SnapshotVsConcurrentUnregister_LeaverNotSent：
// Story 11.8 r11 [P1] fix —— 模拟 broadcast snapshot 与 concurrent Unregister
// 之间的 race window：
//
// 时间线：
//   - t1: 注册 3 stub Session（s1/s2/s3）到同 roomID
//   - t2: 调 BroadcastToRoom 入口 → ListSessionsByRoomID 拿到 [s1, s2, s3] snapshot
//   - t3: **在 fanout 循环 Send 之前**手动调 mgr.Unregister(s2.SessionID()) 模拟
//     LeaveRoom 同步段抢跑（snapshot+send race window）
//   - t4: fanout 循环到 s2 → r11 fix 在 Send 之前 re-check IsRegistered → false
//     → skip s2 → s2.sendChan 应保持空
//   - t5: s1 / s3 仍 IsRegistered=true → Send 入队成功
//
// **如何在单测里"手动"插入 t3 unregister**：直接顺序调用即可 ——
//  1. 用 listInjector 包裹 mgr 让 ListSessionsByRoomID 返回固定 snapshot 后**再**
//     调 mgr.Unregister(s2)；
//  2. 然后调 broadcastToRoomFanout（用包级 helper 把 ListSessionsByRoomID 替换成
//     固定切片）—— 但这需要改代码注入点。
//
// **更简单的做法**：直接在 broadcastToRoomFanout 调用前先 mgr.Unregister(s2)，
// 但 ListSessionsByRoomID 会立即跳过 s2 → 不能复现 race。
//
// **选用方案**：用一个 SessionManager 包装器 raceMgr —— ListSessionsByRoomID 返回
// 全部 3 session（**绕过** s2 已 Unregister 的状态）；IsRegistered 委托给真实
// mgr（Unregister 后返 false）。这正是 race 的语义模型 —— "snapshot 时 s2 还在，
// IsRegistered check 时 s2 已被 unregister"。
//
// **断言**：
//   - sent == 2（s1/s3 实际 Send，s2 被 IsRegistered guard skip）
//   - s1.sendChan / s3.sendChan 各 1 条
//   - s2.sendChan 长度 0（race window 没让 stale broadcast 漏给 leaver）
func TestBroadcastToRoomFanout_R11_SnapshotVsConcurrentUnregister_LeaverNotSent(t *testing.T) {
	// **NOTE**: 不调 mgr.Close() —— 与既有 stub session 模式同
	mgr := NewSessionManager().(*sessionManager)

	const roomID uint64 = 9311
	s1 := makeStubSessionForBroadcast(1001, roomID)
	s2 := makeStubSessionForBroadcast(1002, roomID)
	s3 := makeStubSessionForBroadcast(1003, roomID)
	for _, s := range []*Session{s1, s2, s3} {
		if _, err := mgr.Register(context.Background(), s); err != nil {
			t.Fatalf("Register(uid=%d): %v", s.userID, err)
		}
	}

	// 模拟 race：snapshot 后 unregister s2 ——
	// 用 raceMgr 让 ListSessionsByRoomID 返回**全部** 3 session（包括已 unregister
	// 的 s2），但 IsRegistered 委托真实 mgr（s2 unregister 后 → false）。
	if err := mgr.Unregister(context.Background(), s2.SessionID()); err != nil {
		t.Fatalf("Unregister s2: %v", err)
	}
	rm := &raceListMgr{
		SessionManager: mgr,
		stale:          []*Session{s1, s2, s3},
		staleRoomID:    roomID,
	}

	msg := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"1002"},"ts":1234567890}`)

	sent, err := BroadcastToRoom(context.Background(), rm, roomID, msg)
	if err != nil {
		t.Fatalf("BroadcastToRoom: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2 (s2 skipped by r11 IsRegistered guard)", sent)
	}

	if got := len(s1.sendChan); got != 1 {
		t.Errorf("s1 (uid=1001) sendChan len = %d, want 1", got)
	}
	if got := len(s2.sendChan); got != 0 {
		t.Errorf("s2 (uid=1002, leaver) sendChan len = %d, want 0 (r11 race fix should skip unregistered)", got)
	}
	if got := len(s3.sendChan); got != 1 {
		t.Errorf("s3 (uid=1003) sendChan len = %d, want 1", got)
	}
}

// TestBroadcastToRoomExceptFanout_R11_SnapshotVsConcurrentUnregister_LeaverNotSent：
// 同 R11 race scenario，但走 BroadcastToRoomExcept 路径 —— 双重保护场景：
// excludeUserID 排除 joiner，同时还有另一个 user (s2) 在 snapshot 后被 Unregister。
// r11 fix + r3 except filter 应共同确保：joiner（excluded）和 leaver（unregistered）
// 都不收到 msg。
func TestBroadcastToRoomExceptFanout_R11_SnapshotVsConcurrentUnregister_LeaverNotSent(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)

	const roomID uint64 = 9312
	s1 := makeStubSessionForBroadcast(2001, roomID) // joiner（被 except 过滤）
	s2 := makeStubSessionForBroadcast(2002, roomID) // leaver（被 r11 IsRegistered 过滤）
	s3 := makeStubSessionForBroadcast(2003, roomID) // 真正的 receiver
	for _, s := range []*Session{s1, s2, s3} {
		if _, err := mgr.Register(context.Background(), s); err != nil {
			t.Fatalf("Register(uid=%d): %v", s.userID, err)
		}
	}

	// 模拟 race：unregister s2 但 ListSessionsByRoomID 仍返完整 snapshot
	if err := mgr.Unregister(context.Background(), s2.SessionID()); err != nil {
		t.Fatalf("Unregister s2: %v", err)
	}
	rm := &raceListMgr{
		SessionManager: mgr,
		stale:          []*Session{s1, s2, s3},
		staleRoomID:    roomID,
	}

	msg := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"2001"},"ts":1234567890}`)

	// excludeUserID=2001（排除 joiner s1）
	sent, err := BroadcastToRoomExcept(context.Background(), rm, roomID, 2001, msg)
	if err != nil {
		t.Fatalf("BroadcastToRoomExcept: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1 (s1 excluded, s2 r11-skipped, only s3 receives)", sent)
	}

	if got := len(s1.sendChan); got != 0 {
		t.Errorf("s1 (excluded) sendChan len = %d, want 0", got)
	}
	if got := len(s2.sendChan); got != 0 {
		t.Errorf("s2 (leaver, r11-skipped) sendChan len = %d, want 0", got)
	}
	if got := len(s3.sendChan); got != 1 {
		t.Errorf("s3 sendChan len = %d, want 1", got)
	}
}

// raceListMgr 是一个 SessionManager 包装器，让 ListSessionsByRoomID 返回固定的
// stale snapshot（绕过底层 mgr 已 Unregister 的状态），其余方法委托底层 mgr。
//
// 用途：模拟 broadcastToRoomFanout 在 ListSessionsByRoomID snapshot 后、Send 之前
// 期间另一个线程调 Unregister 的 race window —— snapshot 仍包含已被 unregister
// 的 session，IsRegistered re-check 应返 false 让该 session 被 skip。
type raceListMgr struct {
	SessionManager
	stale       []*Session
	staleRoomID uint64
}

func (r *raceListMgr) ListSessionsByRoomID(_ context.Context, roomID uint64) []*Session {
	if roomID == r.staleRoomID {
		out := make([]*Session, len(r.stale))
		copy(out, r.stale)
		return out
	}
	return r.SessionManager.ListSessionsByRoomID(context.Background(), roomID)
}

// TestBroadcastToRoomExcept_AllExcluded_ReturnsZero：
// 房间内仅 1 个 Session 且 UserID 命中 excludeUserID → 全部被 filter；返 (0, nil)；
// 该 Session sendChan 应为空。
func TestBroadcastToRoomExcept_AllExcluded_ReturnsZero(t *testing.T) {
	// **NOTE**: 不调 mgr.Close() —— stub Session 的 sendPriorityChan 字段没初始化
	// （makeStubSessionForBroadcast 只 init 了 sendChan），mgr.Close() 链路会调
	// Session.Close → closeInternal → close(sendPriorityChan) panic on nil channel。
	// 与既有 broadcast_perf_internal_test.go 同模式（也不调 mgr.Close()）。test 结束
	// 后 mgr 走 GC 回收，stub Session 没有 active goroutine 不会泄漏。
	mgr := NewSessionManager().(*sessionManager)

	const roomID uint64 = 9303
	s1 := makeStubSessionForBroadcast(1001, roomID)
	if _, err := mgr.Register(context.Background(), s1); err != nil {
		t.Fatalf("Register: %v", err)
	}

	sent, err := BroadcastToRoomExcept(context.Background(), mgr, roomID, 1001, []byte(`{"type":"x","requestId":"","payload":{},"ts":0}`))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0 (all sessions excluded)", sent)
	}
	if got := len(s1.sendChan); got != 0 {
		t.Errorf("s1 sendChan len = %d, want 0 (excluded)", got)
	}
}
