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

	// ListAllSessions 返回 manager 内所有 active Session（按 sessionID 字典序，
	// 让 HeartbeatScanner（Story 10.4） / 未来 graceful shutdown 遍历顺序确定）。
	//
	// 内部实装：与 ListSessionsByRoomID 同语义 —— 锁内 read-lock copy 切片返回；
	// 调用方拿到的切片在 manager 改 map 后保持不变（避免遍历期 manager 改 map
	// 的并发问题）。
	//
	// 0 个 Session → 返 ([], nil)。
	//
	// ctx 当前不消费（list 操作纯内存）；保留参数让未来 distributed manager 抽象
	// 时可以走 ctx-aware 实装。
	ListAllSessions(ctx context.Context) []*Session

	// IsRegistered 检查 sessionID 当前是否仍在 manager 索引中（review 10-6 r4 P2 加）。
	//
	// 用途：HeartbeatScanner.scanOnce 拿到 ListAllSessions 快照后异步对每个 active
	// session 调 PresenceRenewer.AddOnline 重写 presence。snapshot 与 AddOnline 之
	// 间的窗口期里 session 可能已经 disconnect → manager 已 Unregister → 钩子已
	// RemoveOnline 清干净 presence。如果 scanner 不 check 仍然 AddOnline，会"复活"
	// 已离线 session 的 presence keys，造成 zombie online entry 直到 TTL 自然过期。
	//
	// 走 read lock 直接 lookup `sessionsByID[sessionID]`；零分配 + nanos 量级 ——
	// scanner 每 tick 对每个 session 调一次，性能开销可忽略。
	//
	// 错误语义：sessionID 不存在 → 返 false（不报 error，与 ListAllSessions 等
	// 查询接口一致）。ctx 当前不消费，保留参数让未来 distributed 实装可消费。
	//
	// **不要用本方法做 reconcile 路径的 gate**：reconnect 替换路径**有意**保留
	// OLD session 在 sessionsByID 直到 oldS.Close() 跑完（r2 P1 不变量），导致 OLD
	// 在 IsRegistered 下仍返 true —— scanner 在这个窗口内对 OLD AddOnline 会把
	// user_key 改回 OLD session/room，污染已重连到 NEW 的 user。reconcile gate 必须
	// 用 IsCurrentForUser（review 10-6 r8 P1 加）。本接口保留作为更弱的"会话还活着"
	// check 供其他 caller 使用。
	IsRegistered(ctx context.Context, sessionID string) bool

	// IsCurrentForUser 检查 sessionID 是否仍是该 user 的"当前 active session"
	// （即 sessionsByID 命中 + userToSessionID[user] 仍指向该 sessionID）。
	// review 10-6 r8 P1 加。
	//
	// 用途：HeartbeatScanner.scanOnce 的 reconcile 路径必须用本方法而非 IsRegistered
	// 做 gate。reconnect 替换中场窗口（OLD session 在 sessionsByID **保留**到
	// oldS.Close() 跑完，但 userToSessionID[u] 已指向 NEW），IsRegistered 会让 OLD
	// 通过 → AddOnline(OLD) 改写 user:{id}:ws_session 回 OLD session/room 或重新
	// SADD OLD 到旧 room set；后续 RemoveOnline(oldSessionID) 在 Lua script 看到
	// currentSession==OLD 走 case 2 完整清理 → 真正活的 NEW 的 presence 被清掉。
	//
	// 本方法严格匹配"双索引一致" —— 只有 sessionsByID[id] != nil **且**
	// userToSessionID[user] == id 时才返 true。reconnect 替换路径下 OLD 会返 false
	// （userToSessionID 已指向 NEW），scanner 跳过 OLD 的 reconcile，不污染 NEW。
	//
	// 走 read lock 直接 lookup 双索引；O(1)，与 IsRegistered 同量级。错误语义同
	// IsRegistered（不抛 error，sessionID 不存在 / 已被替换 → false）。
	//
	// 详见 docs/lessons/2026-05-07-fire-and-forget-hooks-need-per-user-mutex-10-6-r8.md。
	IsCurrentForUser(ctx context.Context, sessionID string) bool

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
//  6. 锁外**先**调 onRegister 钩子（10.6 presence AddOnline）—— 让新 session 的
//     AddOnline 在旧 session 的 RemoveOnline **之前**跑，消除 reconnect "false
//     offline window"
//  7. 锁外**后**触发旧 Session 强制 Close（→ notifyClosed → Unregister(oldID) →
//     onUnregister 钩子 → RemoveOnline(OLD)）
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
// **关键不变量 3**（review r10 P2 修）：reconnect 替换路径下 onRegister(NEW) 必须
// **先于** replaced.Close() 触发的 onUnregister(OLD) 跑。修前顺序是 "replaced.Close()
// → onUnregister(OLD) → RemoveOnline(OLD) → onRegister(NEW) → AddOnline(NEW)" ——
// 中间窗口里 user_key 已被 RemoveOnline DEL（sessionID 匹配走 Lua case 2），
// IsOnline / ListOnline 看到 user **暂时离线**直到 AddOnline(NEW) 跑完。这是
// presence 数据准确度的回归（同一 user reconnect 不应有 false offline 窗口）。
// 修后顺序：onRegister(NEW) **先**跑 → AddOnline(NEW) 在 r9 P1 共享 user mutex
// 内同步完成（user_key 改成 newSession|newRoom，newRoom set 含 user，且 r10 P1
// 内置的 cross-room SREM 自愈也跑掉了 oldRoom stale）；之后 replaced.Close() 触发
// RemoveOnline(OLD) → Lua script GET → currentSession==newSession≠oldSession →
// 走 case 3/4（**不**动 user_key），SREM 仅在 cross-room 路径执行（旧 oldRoom
// 已被 r10 P1 SREM 干净，二次 SREM 是 idempotent no-op）。**全程 user 看似 online**。
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

	// 锁外做（**review r10 P2 修后顺序**）：
	//   1. 注册钩子（10.6 Redis presence AddOnline；外部 IO 可能慢，不能持锁）——
	//      **必须**先于 replaced.Close 跑，让 user_key + newRoom set 在旧 session
	//      的 RemoveOnline 之前已就位，消除 reconnect "false offline window"。r10
	//      P1 配套修法：AddOnline 内置 cross-room SREM 自愈把旧 room stale 也清掉
	//   2. 旧 Session 强制 Close（避免 Close → notifyClosed → manager.Unregister
	//      在锁内重入死锁）；Close → notifyClosed → Unregister(oldID) → onUnregister
	//      钩子 → RemoveOnline(oldID) 走标准路径。AddOnline(NEW) 已先跑 →
	//      user_key=newSession|newRoom；RemoveOnline(OLD) 的 Lua script 看到
	//      currentSession=newSession≠oldSession 走 case 3/4 不动 user_key，仅
	//      cross-room 时 SREM oldRoom（已被 AddOnline 自愈过，幂等 no-op）
	if onRegister != nil {
		onRegister(s)
	}
	if replaced != nil {
		// 旧 Session 主动关；调用方可通过 ErrSessionReplaced 标识本次关闭语义
		_ = replaced.Close()
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
// 流程：
//  1. 锁内 read-lock 拿引用切片（避免遍历过程中 manager 改 map 的并发问题）
//  2. **锁外**按 sessionID 字典序排序（review r5 P2 修：避免 O(N log N) sort
//     持 RLock 阻塞 Register/Unregister 的 write lock）
//
// 排序在锁外的安全性：每个 *Session 是独立堆对象，Register/Unregister 改的是
// manager 内部 map，**不**改 *Session 自身字段；快照切片的 Session 引用即便
// 在排序期间被 manager Unregister 也不影响 sessionID 字段（unexported，唯一
// 写入点是 Register，且生命周期内不变 —— Register 在 m.mu.Lock 持锁期赋值后
// 不再修改）。
func (m *sessionManager) ListSessionsByRoomID(ctx context.Context, roomID uint64) []*Session {
	m.mu.RLock()
	room, ok := m.sessionsByRoom[roomID]
	if !ok || len(room) == 0 {
		m.mu.RUnlock()
		return []*Session{}
	}
	// 锁内仅 copy 引用切片（O(N) 但无排序开销）
	out := make([]*Session, 0, len(room))
	for _, s := range room {
		out = append(out, s)
	}
	m.mu.RUnlock()
	// 锁外排序（O(N log N) 不再阻塞 Register/Unregister）
	sort.Slice(out, func(i, j int) bool {
		return out[i].sessionID < out[j].sessionID
	})
	return out
}

// ListAllSessions 实装 SessionManager.ListAllSessions（Story 10.4 加）。
//
// 与 ListSessionsByRoomID 同模式：
//  1. 锁内 read-lock 拿引用切片
//  2. **锁外**按 sessionID 字典序排序（review r5 P2 修）
//
// HeartbeatScanner 每 30s 调本方法；如果 sort 持 RLock，N 大时 Register/Unregister
// 的 write lock（同一 mu）周期性被阻塞，连接 / 断连延迟。把 sort 挪到锁外
// 让 RLock 持锁时间退化到 O(N) copy（约几 us 至几 ms 量级），不再随 sort 复杂度
// 上升。
func (m *sessionManager) ListAllSessions(ctx context.Context) []*Session {
	m.mu.RLock()
	if len(m.sessionsByID) == 0 {
		m.mu.RUnlock()
		return []*Session{}
	}
	// 锁内仅 copy 引用切片
	out := make([]*Session, 0, len(m.sessionsByID))
	for _, s := range m.sessionsByID {
		out = append(out, s)
	}
	m.mu.RUnlock()
	// 锁外排序
	sort.Slice(out, func(i, j int) bool {
		return out[i].sessionID < out[j].sessionID
	})
	return out
}

// IsRegistered 实装 SessionManager.IsRegistered（review 10-6 r4 P2 加）。
//
// 走 RLock + map lookup，无分配。HeartbeatScanner.scanOnce 在 reconcile 路径
// （review 10-6 r2 P1 / r3 P2 引入）拿到 ListAllSessions 快照后调本方法 gate
// PresenceRenewer.AddOnline，避免"快照已过期 → session 已 unregister → 仍 AddOnline
// 复活 presence" 的 zombie 路径。
//
// 注意：本方法只看 sessionsByID 索引；reconnect 替换路径下 OLD session 在
// sessionsByID **保留**到 Close → notifyClosed → Unregister 跑完才被清，所以
// scanner 在 OLD 还没真正销毁前看到 IsRegistered=true 是预期行为（OLD 的 AddOnline
// 写的是 oldRoom + oldID，不会污染 NEW 的 presence；OLD 销毁后下一 tick 不再
// 看到它）。
func (m *sessionManager) IsRegistered(ctx context.Context, sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.sessionsByID[sessionID]
	return ok
}

// IsCurrentForUser 实装 SessionManager.IsCurrentForUser（review 10-6 r8 P1 加）。
//
// 双 map lookup：
//  1. sessionsByID[sessionID] —— 验证 session 仍在 manager 索引中
//  2. userToSessionID[sess.userID] == sessionID —— 验证仍是该 user 的"当前 session"
//     （reconnect 替换路径已把 userToSessionID 指向 NEW，OLD 在这一步返 false）
//
// 严格走"双索引一致" gate，比 IsRegistered 更精确：
//   - reconnect 替换中场（OLD 仍在 sessionsByID + userToSessionID 已指向 NEW）：
//     IsRegistered=true，IsCurrentForUser=false（本方法保留 NEW 的 presence 不被
//     scanner reconcile 误改回 OLD）
//   - 普通 active session（OLD 已 Close + Unregister 跑完）：sessionsByID 已删 OLD，
//     IsRegistered=false，IsCurrentForUser=false（路径回归一致）
//   - 普通 active session 自己 reconcile：sessionsByID 命中 + userToSessionID 指自己，
//     IsRegistered=true，IsCurrentForUser=true（reconcile 正常进行）
func (m *sessionManager) IsCurrentForUser(ctx context.Context, sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessionsByID[sessionID]
	if !ok {
		return false
	}
	currentID, ok := m.userToSessionID[sess.userID]
	return ok && currentID == sessionID
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
