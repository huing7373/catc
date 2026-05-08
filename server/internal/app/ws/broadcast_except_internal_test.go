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
