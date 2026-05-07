package ws

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// TestSessionManager_IsCurrentForUser_RegisteredSession_ReturnsTrue:
// review 10-6 r8 P1 加 IsCurrentForUser —— happy path：单 session 注册后
// IsCurrentForUser 必须返 true（双索引一致：sessionsByID[id] != nil + userToSessionID[u] == id）。
func TestSessionManager_IsCurrentForUser_RegisteredSession_ReturnsTrue(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Session{userID: 1001, roomID: 3001, logger: logger}
	id, err := mgr.Register(context.Background(), s)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if !mgr.IsCurrentForUser(context.Background(), id) {
		t.Fatalf("IsCurrentForUser(%q) = false, want true (single registered session)", id)
	}
	// IsRegistered 也应返 true（基线对照）
	if !mgr.IsRegistered(context.Background(), id) {
		t.Fatalf("IsRegistered should also be true for live session")
	}
}

// TestSessionManager_IsCurrentForUser_AfterUnregister_ReturnsFalse:
// session Unregister 后 sessionsByID 已删 → IsCurrentForUser 返 false。
// 与 IsRegistered 行为一致（路径回归）。
func TestSessionManager_IsCurrentForUser_AfterUnregister_ReturnsFalse(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Session{userID: 1002, roomID: 3001, logger: logger}
	id, err := mgr.Register(context.Background(), s)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := mgr.Unregister(context.Background(), id); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	if mgr.IsCurrentForUser(context.Background(), id) {
		t.Fatalf("IsCurrentForUser(%q) = true after Unregister, want false", id)
	}
}

// TestSessionManager_IsCurrentForUser_OldSessionAfterReplacement_ReturnsFalse:
// **review 10-6 r8 P1 核心 case** —— reconnect 替换路径下 OLD session 仍在
// sessionsByID（保留到 oldS.Close 跑完触发 Unregister 走标准路径），但
// userToSessionID[u] 已指向 NEW。
//
// IsRegistered(OLD) = true（语义"会话还活着"），IsCurrentForUser(OLD) = false
// （NEW 已抢占 user 的 current session 位置）。这正是 scanner reconcile 必须用
// IsCurrentForUser 而非 IsRegistered 做 gate 的原因 —— 否则 scanner 会对 OLD
// AddOnline 把 user_key 改回 OLD session/room，污染 NEW 的 presence。
//
// 模拟方法：直接构造两个 Session 引用，手动塞 sessionsByID + userToSessionID
// 模拟"OLD 仍在 sessionsByID（Close 还没跑完）+ userToSessionID 已指 NEW"中场态。
// 不走真实 Register（Register 锁内会 close OLD，无法暂停在中场）。
func TestSessionManager_IsCurrentForUser_OldSessionAfterReplacement_ReturnsFalse(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	const userID = uint64(1003)
	oldID := "session-old"
	newID := "session-new"
	oldS := &Session{userID: userID, roomID: 3001, sessionID: oldID, logger: logger}
	newS := &Session{userID: userID, roomID: 3001, sessionID: newID, logger: logger}

	// 手动模拟"reconnect 替换中场"状态：
	//   - sessionsByID 里 OLD 和 NEW 都在（OLD 保留以触发 onUnregister 钩子）
	//   - sessionsByRoom 已被 Register 段从 OLD 移到 NEW（Register 锁内修法）
	//   - userToSessionID[user] 指向 NEW（Register 段已覆盖）
	mgr.mu.Lock()
	mgr.sessionsByID[oldID] = oldS
	mgr.sessionsByID[newID] = newS
	mgr.sessionsByRoom[3001] = map[string]*Session{newID: newS} // OLD 已被移除
	mgr.userToSessionID[userID] = newID
	mgr.mu.Unlock()

	// **关键不变量**（r8 P1 加）：OLD 在替换中场 IsRegistered=true（仍在
	// sessionsByID），但 IsCurrentForUser=false（userToSessionID 已不指 OLD）。
	if !mgr.IsRegistered(context.Background(), oldID) {
		t.Fatalf("IsRegistered(OLD) should still be true during replacement window")
	}
	if mgr.IsCurrentForUser(context.Background(), oldID) {
		t.Fatalf("IsCurrentForUser(OLD) = true during replacement window, want false (would let scanner pollute NEW's presence)")
	}

	// NEW 必须返 true（reconcile 应继续对 NEW 续期）
	if !mgr.IsCurrentForUser(context.Background(), newID) {
		t.Fatalf("IsCurrentForUser(NEW) should be true (NEW is the current active session)")
	}
}

// TestSessionManager_IsCurrentForUser_UnknownSession_ReturnsFalse:
// 不存在的 sessionID → 返 false（不抛 error，与 IsRegistered 同语义）。
func TestSessionManager_IsCurrentForUser_UnknownSession_ReturnsFalse(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)
	if mgr.IsCurrentForUser(context.Background(), "no-such-session") {
		t.Fatalf("IsCurrentForUser of unknown sessionID = true, want false")
	}
}

// TestNewSessionID_FullUUIDFormat: review r9 P3 修后 newSessionID 必须返完整 uuid v4
// （36 字符 + 4 个 hyphen），而不是早期实装的 8 字符前缀截断。
//
// 这是结构性回归测试：防止未来有人把 newSessionID 改回 `[:N]` 截断模式（哪怕"日志
// 短一点"的微弱理由），让 sessionsByID/sessionsByRoom map key 重新进入 birthday
// paradox 风险区。
func TestNewSessionID_FullUUIDFormat(t *testing.T) {
	id := newSessionID()
	if got, want := len(id), 36; got != want {
		t.Fatalf("len(newSessionID()) = %d, want %d (full uuid v4 = 36 chars)", got, want)
	}
	if got, want := strings.Count(id, "-"), 4; got != want {
		t.Errorf("hyphen count = %d, want %d (uuid v4 8-4-4-4-12 layout)", got, want)
	}
}

// TestNewSessionID_NoDuplicatesIn10k: 直接调 newSessionID 1 万次，检查无碰撞。
//
// 这是 statistical sanity check —— uuid v4 = 128 bit 熵，1 万样本碰撞概率
// ≈ 10^4 * 10^4 / 2^129 ≈ 10^-31，完全不可能命中。如果命中说明 newSessionID
// 实装有 bug（如有人把它改成低熵 PRNG）。
func TestNewSessionID_NoDuplicatesIn10k(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := newSessionID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate sessionID generated at iter %d: %q", i, id)
		}
		seen[id] = struct{}{}
	}
	if got := len(seen); got != n {
		t.Fatalf("unique IDs = %d, want %d", got, n)
	}
}

// TestSessionManager_Register_NoCollisionUnder1kSessions: 直接 Register 1000 个
// 不同 userID 的 Session，断言每个 Register 返回的 sessionID 都唯一 + 全部仍在
// sessionsByID 索引里（没有"碰撞导致 silent overwrite"丢失）。
//
// 这是 review r9 P3 的核心回归测试：8 字符截断方案下，4 万 session ~50% 碰撞，
// 1000 session 也有约 1.2% 概率（n²/2^33 ≈ 10^6/8.6e9 ≈ 0.012%；其实 1000 远
// 不到非平凡区，但放在这里防御性兜底未来如果有人把 entropy 再改小）。改全 UUID
// 后这个测试在任意 N 下都应该 pass。
func TestSessionManager_Register_NoCollisionUnder1kSessions(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)
	// 注意：不 defer mgr.Close() —— 本测试构造的 Session 是裸字段（没初始化
	// sendChan / sendPriorityChan / conn / cancelCtx），Close 路径会 panic。
	// 测试只关心 Register 的 sessionID 唯一性 + 索引完整性，不需要走 Close 路径。
	// 测试结束后 mgr 进程随测试退出 GC，无 leak 担忧。

	const n = 1000
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	idsSeen := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		s := &Session{
			userID: uint64(1000 + i), // 每个 user 不同 → 不走 replace 路径
			roomID: 3001,
			logger: logger,
		}
		id, err := mgr.Register(context.Background(), s)
		if err != nil {
			t.Fatalf("Register iter %d: unexpected err = %v", i, err)
		}
		if _, dup := idsSeen[id]; dup {
			t.Fatalf("duplicate sessionID at iter %d: %q (collision in Register — entropy too low)", i, id)
		}
		idsSeen[id] = struct{}{}
	}

	// 全部 sessionID 必须仍在 sessionsByID 索引里（没有 silent overwrite）。
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	if got, want := len(mgr.sessionsByID), n; got != want {
		t.Fatalf("len(sessionsByID) = %d, want %d (silent overwrite indicates collision)", got, want)
	}
	for id := range idsSeen {
		if _, ok := mgr.sessionsByID[id]; !ok {
			t.Fatalf("sessionID %q registered but not in sessionsByID (overwritten by collision)", id)
		}
	}
}
