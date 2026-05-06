package ws

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

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
