package ws

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// SessionManager 是进程内全局 Session 注册中心。
//
// 设计目的：
//   - 让 BroadcastToRoom（Story 10.5）通过 ListSessionsByRoomID 拿到目标 Session
//     列表
//   - 让 Story 10.6 Redis presence repo 挂在 OnSessionRegister / OnSessionUnregister
//     钩子上
//   - 让 graceful shutdown（Epic 36+）批量遍历所有 Session 主动 close 1001
//
// 范围边界：
//   - 进程内 in-memory（map[sessionID]*Session + map[roomID]map[sessionID]*Session
//     双索引）
//   - **不**做跨实例 / Pub/Sub（节点 9+ 多实例部署才考虑）
//   - **不**直接消费 Redis client（钩子模式让 Story 10.6 注入，避免直接耦合）
//
// 钩子注入：用 functional option 模式让 bootstrap 期注入：
//
//	mgr := ws.NewSessionManager(
//	    ws.WithRegisterHook(func(s *Session) { presenceRepo.AddOnline(...) }),
//	    ws.WithUnregisterHook(func(s *Session) { presenceRepo.RemoveOnline(...) }),
//	)
//
// 本 story 阶段两个钩子默认 no-op（10.6 落地时才注入具体实现）。
type SessionManager interface {
	// Register 注册 Session 到 manager；触发 OnRegister 钩子（如挂的话）。
	// sessionID 由 manager 内部生成（完整 uuid v4 = 36 字符，128 bit 熵；单实例
	// 进程内可视碰撞为 0），返回供 Session 持有作为 logger 的关联字段。
	//
	// 同一 user 重复 Register（如重连） → 旧 Session 被强制 Close（用 sentinel
	// ErrSessionReplaced 标识；调用方可在 close hook 里区分日志）。
	// 节点 4 阶段单 user 单 session 假设由本约束兜底；多 session 抢占语义留给
	// Epic 36+。
	//
	// **关键**：本方法**先**生成 sessionID + 把 sessionID 注入 Session 的 logger
	// （让 register 钩子日志已经能看到 sessionID），再注入双索引；保证 Register
	// 返回时 ListSessionsByRoomID 能立即看到该 Session。
	Register(ctx context.Context, s *Session) (sessionID string, err error)

	// Unregister 注销 Session；触发 OnUnregister 钩子。
	// sessionID 不存在 → 返 nil（idempotent；与 Session.Close 多次调用一致）。
	Unregister(ctx context.Context, sessionID string) error

	// ListSessionsByRoomID 返回 roomID 对应的所有 active Session（按 sessionID
	// 字典序，让 BroadcastToRoom 调用时遍历顺序确定，便于排障）。
	//
	// roomID 没有任何 Session → 返 ([], nil)。
	// 调用方**不应**修改返回的切片（manager 返回 snapshot，但内部 *Session 共享 ——
	// 修改字段需通过 Session 自身的方法）。
	ListSessionsByRoomID(ctx context.Context, roomID uint64) []*Session

	// Close 关闭 manager：批量 close 所有 Session（不主动推 close 1001 frame，
	// graceful shutdown 业务由 Epic 36 接管）。
	// 必须幂等（与 *sql.DB.Close 一致）。
	Close() error
}

// SessionManagerOption 是 functional option（让 bootstrap 期注入钩子）。
type SessionManagerOption func(*sessionManager)

// WithRegisterHook 注入 Session 注册时的回调钩子（Story 10.6 用来加 Redis
// presence）。**禁止**在钩子内调 manager 接口（自调死锁）；钩子应只做"轻量
// side-effect"如 Redis SADD / metrics counter inc。
func WithRegisterHook(fn func(s *Session)) SessionManagerOption {
	return func(m *sessionManager) {
		m.onRegister = fn
	}
}

// WithUnregisterHook 注入 Session 销毁时的回调钩子。
// 与 WithRegisterHook 同约束（不要在钩子内反向调 manager）。
func WithUnregisterHook(fn func(s *Session)) SessionManagerOption {
	return func(m *sessionManager) {
		m.onUnregister = fn
	}
}

// sessionManager 是 SessionManager 的默认实装。
type sessionManager struct {
	mu              sync.RWMutex
	sessionsByID    map[string]*Session
	sessionsByRoom  map[uint64]map[string]*Session
	userToSessionID map[uint64]string // 单 user 单 session 索引（节点 4 阶段假设）
	onRegister      func(s *Session)
	onUnregister    func(s *Session)
	closed          bool
	closeOnce       sync.Once
}

// NewSessionManager 构造 SessionManager；本 story 阶段默认无钩子（10.6 落地时
// 通过 WithRegisterHook / WithUnregisterHook 注入）。
func NewSessionManager(opts ...SessionManagerOption) SessionManager {
	m := &sessionManager{
		sessionsByID:    map[string]*Session{},
		sessionsByRoom:  map[uint64]map[string]*Session{},
		userToSessionID: map[uint64]string{},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Register 实装 SessionManager.Register。
//
// 流程：
//  1. 生成 sessionID（完整 uuid v4 = 36 字符；review r9 P3 修：早期实装截 8 字符
//     前缀，几千 session 起 birthday paradox 碰撞概率非平凡；改全 UUID 后单实例
//     进程内 128 bit 熵可视碰撞为 0）
//  2. 检查同 user 是否已有 active session：
//     - 是 → 记录 replaced 引用 + **锁内**把 OLD 从 sessionsByRoom 移除
//     （让 broadcast 不再看到 OLD），但**保留** OLD 在 sessionsByID（让后续
//     oldS.Close() → notifyClosed → Unregister(oldID) 仍能找到它，触发
//     onUnregister 钩子走标准路径）
//  3. 把新 Session 的 sessionID 字段赋值（newSession 时还没有；这里**回填**）
//     + 重建 contextual logger（带 sessionID）
//  4. 注入新 Session 的双索引 + userToSessionID（覆盖）
//  5. 把 manager 自身设为 session.notifier，让 session 自闭时反向通知 manager
//  6. 锁外调 onRegister 钩子（如有）+ 旧 Session 强制 Close（触发 onUnregister
//     钩子）
//
// **关键不变量 1**（review r2 P1 修）：reconnect 替换路径**必须**触发旧 Session
// 的 onUnregister 钩子。修复前实装是"锁内 removeFromIndicesLocked(oldS) → 锁外
// oldS.Close()"；oldS.Close() → notifyClosed → Unregister 看到索引已空 → 走 no-op
// 路径 → onUnregister 漏调，10.6 Redis presence cleanup / metrics 计数被永久
// 遗留。
//
// **关键不变量 2**（review r5 P2 修）：reconnect 替换的中场窗口期 ListSessionsByRoomID
// **不能**同时返回 OLD 和 NEW（会让 BroadcastToRoom 同 user 双发，client 不能 dedupe
// 因为 sessionID 不外漏）。修复策略：把 OLD 与 NEW 在 byRoom 索引的生命周期拆分 ——
// Register lock 内**先**从 sessionsByRoom 移除 OLD（broadcast 立即不再见到 OLD）；
// 保留 OLD 在 sessionsByID 让后续 Close → Unregister 仍能找到 + 触发钩子；OLD 在
// sessionsByID 的清理由 Unregister(oldID) 路径接管，**不**再二次动 sessionsByRoom
// （已被 Register 段移除）。
//
// 配合修改：removeFromIndicesLocked 必须 graceful 处理"sessionsByRoom 已不在
// 但 sessionsByID 在"的状态（只 delete 存在的索引）—— 已就绪（map delete 对
// missing key 是 no-op，本身就 graceful）。
func (m *sessionManager) Register(ctx context.Context, s *Session) (string, error) {
	if s == nil {
		return "", errors.New("ws: register nil session")
	}

	sessionID := newSessionID()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", errors.New("ws: session manager closed")
	}

	// 同 user 重连：记录 replaced 引用 + 锁内**仅**从 sessionsByRoom 移除 OLD
	// （broadcast 路径立即看不到 OLD，避免重叠窗口双发 —— review r5 P2 修）；
	// 保留 OLD 在 sessionsByID，让后续 oldS.Close() → notifyClosed → Unregister(oldID)
	// 仍能找到 + 触发 onUnregister 钩子（review r2 P1 不变量）。
	var replaced *Session
	if oldID, ok := m.userToSessionID[s.userID]; ok {
		if oldS, ok2 := m.sessionsByID[oldID]; ok2 {
			replaced = oldS
			// 锁内移除 OLD 在 sessionsByRoom 的索引（**不**触发钩子，钩子在
			// Unregister(oldID) 路径触发；这里只是让 broadcast 立即不可见）。
			// 注意：旧 session 的 roomID 可能与新 session 不同（cross-room reconnect），
			// 用 oldS.roomID 而不是 s.roomID。
			if room, ok3 := m.sessionsByRoom[oldS.roomID]; ok3 {
				delete(room, oldID)
				if len(room) == 0 {
					delete(m.sessionsByRoom, oldS.roomID)
				}
			}
		}
	}

	// 把 sessionID 回填到 Session + 给 logger 叠加 canonical "sessionID" 字段
	// （review r4 P3 修：之前用 "sessionID_replay" 非标 key，且 newSession 已注入
	// "sessionID="" 空字段，破坏 grep "sessionID=<id>" 关联模式。现在
	// newSession **不**注入 sessionID 字段，Register 在此用 With 唯一一次叠加
	// canonical 字段）。
	s.sessionID = sessionID
	s.logger = s.logger.With(slog.String("sessionID", sessionID))
	s.notifier = m

	// 注入新 Session 的双索引（覆盖 userToSessionID[user]=newID）。
	// OLD 在 sessionsByID 的清理交给 Unregister(oldID)，OLD 在 sessionsByRoom
	// 已在上面 replaced 分支移除（review r5 P2 修：避免 broadcast 重叠窗口双发）。
	m.sessionsByID[sessionID] = s
	if _, ok := m.sessionsByRoom[s.roomID]; !ok {
		m.sessionsByRoom[s.roomID] = map[string]*Session{}
	}
	m.sessionsByRoom[s.roomID][sessionID] = s
	m.userToSessionID[s.userID] = sessionID
	onRegister := m.onRegister
	m.mu.Unlock()

	// 锁外做：
	//   1. 旧 Session 强制 Close（避免 Close → notifyClosed → manager.Unregister
	//      在锁内重入死锁）；Close → notifyClosed → Unregister(oldID) 走标准
	//      removeFromIndicesLocked + onUnregister 钩子触发路径
	//   2. 注册钩子（10.6 Redis presence；外部 IO 可能慢，不能持锁）
	if replaced != nil {
		// 旧 Session 主动关；调用方可通过 ErrSessionReplaced 标识本次关闭语义
		_ = replaced.Close()
	}
	if onRegister != nil {
		onRegister(s)
	}

	if replaced != nil {
		// 返非空 sessionID 但用 ErrSessionReplaced 标识"成功 + 替换" —— 上层 logger
		// 可借此区分是否走了重连路径
		return sessionID, ErrSessionReplaced
	}
	return sessionID, nil
}

// Unregister 实装 SessionManager.Unregister。
//
// 流程：
//  1. 锁内查 sessionsByID；不存在 → 返 nil（幂等）
//  2. 锁内移除双索引 + userToSessionID
//  3. 锁外调 onUnregister 钩子（如有）
//
// **关键**：本方法**不**调 session.Close —— Close 由 readLoop / writeLoop /
// 外部调用方触发；Unregister 只清理 manager 内的索引。否则 Close → notifyClosed
// → Unregister → Close 形成循环。
func (m *sessionManager) Unregister(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	s, ok := m.sessionsByID[sessionID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	m.removeFromIndicesLocked(s)
	onUnregister := m.onUnregister
	m.mu.Unlock()

	if onUnregister != nil {
		onUnregister(s)
	}
	return nil
}

// removeFromIndicesLocked 从双索引 + userToSessionID 移除 Session（caller 持
// m.mu 写锁）。
//
// **graceful 不变量**（review r5 P2 修配套）：本函数对"索引已被前置路径移除"
// 的状态保持 graceful —— Go map delete 对 missing key 是 no-op，所以即便
// Register 替换路径已经把 OLD 从 sessionsByRoom 移除，本函数二次 delete 仍然
// 安全（不 panic / 不报错）。
//
// 用法场景：
//  1. 标准 Unregister 路径（session 主动 Close → notifyClosed → Unregister）：
//     sessionsByID + sessionsByRoom + userToSessionID 都在 → 三处都成功 delete
//  2. Register 替换路径下旧 Session 的 Close → Unregister：
//     sessionsByID 在（被 Register 段保留以触发钩子）；sessionsByRoom 已被
//     Register 段移除 → 本函数 delete missing key no-op；
//     userToSessionID 已指向 NEW sessionID → currentID != s.sessionID 守卫不删
//  3. SessionManager.Close() 路径下批量遍历 → 每个 session 走标准 Unregister
func (m *sessionManager) removeFromIndicesLocked(s *Session) {
	delete(m.sessionsByID, s.sessionID)
	if room, ok := m.sessionsByRoom[s.roomID]; ok {
		// graceful：room map 里不一定有 s.sessionID（如 Register 替换路径已
		// 提前移除）；map delete 对 missing key 是 no-op，安全。
		delete(room, s.sessionID)
		if len(room) == 0 {
			delete(m.sessionsByRoom, s.roomID)
		}
	}
	// 注意：userToSessionID 可能已被 Register 替换路径覆盖；只在指向当前 sessionID
	// 时才删除（防止 race 1: A 重连 → manager 替换 userToSessionID[u]=B；race 2:
	// 旧 Session A 在 Close 路径触发 Unregister(A) → 这里 sessionID=A，但
	// userToSessionID[u]=B 不应该被误删）
	if currentID, ok := m.userToSessionID[s.userID]; ok && currentID == s.sessionID {
		delete(m.userToSessionID, s.userID)
	}
}

// ListSessionsByRoomID 实装 SessionManager.ListSessionsByRoomID。
//
// 锁内 read-lock copy 切片返回（避免遍历过程中 manager 改 map 的并发问题）；
// 按 sessionID 字典序排序（让 BroadcastToRoom 调用时遍历顺序确定，便于排障）。
func (m *sessionManager) ListSessionsByRoomID(ctx context.Context, roomID uint64) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	room, ok := m.sessionsByRoom[roomID]
	if !ok || len(room) == 0 {
		return []*Session{}
	}
	out := make([]*Session, 0, len(room))
	ids := make([]string, 0, len(room))
	for id := range room {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out = append(out, room[id])
	}
	return out
}

// Close 实装 SessionManager.Close。批量 close 所有 Session + 清空索引（idempotent）。
//
// **关键**：必须**保留**索引到 s.Close() 跑完之后再清空 —— 否则 s.Close() →
// notifyClosed → Unregister 进 sessionsByID 找不到 sessionID 走 no-op 路径，
// onUnregister 钩子不会触发，所有挂在 WithUnregisterHook 上的 caller（10.6 Redis
// presence cleanup / metrics 计数等）会漏调，外部状态被泄漏到 stale。
//
// 顺序：
//  1. 锁内拿快照 + 标 closed=true（拒绝新 Register）；**不**清索引
//  2. 锁外逐个 s.Close() —— 每个 Close 触发 notifyClosed → Unregister(sessionID)
//     → 锁内 removeFromIndicesLocked + 锁外调 onUnregister 钩子
//  3. 全部 Close 跑完后再锁内清空残留索引（防御兜底；正常路径上 Unregister 已
//     把它清光了）
func (m *sessionManager) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		all := make([]*Session, 0, len(m.sessionsByID))
		for _, s := range m.sessionsByID {
			all = append(all, s)
		}
		m.mu.Unlock()

		// 锁外逐个 Close。每个 Close → notifyClosed → Unregister(sessionID) →
		// onUnregister 钩子。Unregister 自身拿写锁，与上面 m.mu.Unlock 不冲突。
		for _, s := range all {
			_ = s.Close()
		}

		// 防御兜底：理论上 Unregister 已把所有 session 从索引清出；如果钩子里
		// 抛 panic / 或 Session 没注入 notifier，残余索引在这一步整体重置。
		m.mu.Lock()
		m.sessionsByID = map[string]*Session{}
		m.sessionsByRoom = map[uint64]map[string]*Session{}
		m.userToSessionID = map[uint64]string{}
		m.mu.Unlock()
	})
	return nil
}

// notifyClosed 实装 closeNotifier 接口；Session.Close 调本方法触发 manager 自动
// Unregister。锁内只移除索引，钩子在锁外调。
func (m *sessionManager) notifyClosed(sessionID string) {
	// 复用 Unregister 路径，但避免 ctx 依赖（这是 Session 内部触发的 close path）
	_ = m.Unregister(context.Background(), sessionID)
}

// newSessionID 返回完整的 uuid v4（36 字符，128 bit 熵；单实例进程内永不碰撞）。
//
// review r9 P3 修：之前实装是 `uuid.NewString()[:8]`，截 8 字符前缀 ≈ 32 bit 熵，
// 几千 session 起 birthday paradox 碰撞概率非平凡（4 万活跃 ~50%）。碰撞会让
// sessionsByID/sessionsByRoom 静默覆盖旧 entry，后续 Unregister/replacement 误删
// 错 session → 原始连接 leak。改全 UUID 是结构性修法（128 bit 熵单实例可视碰撞为 0）；
// 日志多 28 字符（一行 36 vs 8）的开销完全可接受，grep 仍能用前缀搜索。
func newSessionID() string {
	return uuid.NewString()
}
