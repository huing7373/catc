// Package redis 提供基于 Redis 的 repository 实装。
//
// Story 10.6 引入；本 story 是 server/internal/repo/redis/ 子目录的第一个文件
// （presence_repo.go），后续 Epic 20 加 idempotency_repo.go / Epic 11+ 加
// ws_repo.go / room_session_repo.go（Go 项目结构 §6 已锚定模块边界）。
//
// 包名注意：本包名是 `redis`，与 infra 层 `internal/infra/redis` 包名同名。
// 调用方建议用 alias：
//
//	import (
//	    redisinfra "github.com/huing/cat/server/internal/infra/redis"
//	    redisrepo  "github.com/huing/cat/server/internal/repo/redis"
//	)
//
// 设计原则：
//   - **接口形态先行**：PresenceRepo 是 interface（让 service / hook adapter 注入
//     mock；与 SessionManager / RedisClient 同模式）
//   - **ctx 传播严格**（ADR-0007）：所有方法首参数 ctx context.Context
//   - **error 语义内化**：IsOnline 不存在 user 返 (false, nil) 不抛 error；
//     ListOnline 空 set 返 ([], nil)；RemoveOnline 不存在 user 返 nil 不抛 error
//   - **TTL 自带保险**：所有 key 自带 5 分钟默认 TTL（可配 redis.presence_ttl_sec）；
//     防 server crash 留僵尸 user / sessionID 在 Redis（与 docs/数据库设计.md §9.1
//     钦定一致）
//   - **userID 跨界类型**：业务层 user.ID 是 uint64，Redis members 是 string；
//     PresenceRepo 接口签名用 uint64（业务类型），内部 strconv.FormatUint 序列化，
//     ListOnline 返 []uint64 而非 []string（让 Epic 11.7 SnapshotBuilder 直接用业务
//     类型，减少 boilerplate）
package redis

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	redisinfra "github.com/huing/cat/server/internal/infra/redis"
)

// defaultPresenceTTL 是 presence key 的默认 TTL（5 分钟）。
//
// 选 5 分钟的理由：
//   - 远大于 ws.heartbeat_timeout_sec=60 + scanner 扫描周期 30s，让正常心跳路径下
//     TTL 永远不到（heartbeat 调 RenewTTL 持续续期）
//   - 远小于"server crash 后 stale presence 影响业务"的可容忍窗口（5 分钟内即清，
//     避免开发 / 运维查问题时看到大量"已死 user 还在 online"）
//   - 与 V1 §12.2 ping 间隔 60s 配合：客户端断网 5 分钟仍未重连 → presence 自动过期
//     （即使 server scanner 因故漏扫）
//
// **可配**：YAML `redis.presence_ttl_sec`（dev / test 可短到 5s 走 miniredis FastForward
// 测试 TTL 行为；prod 必须 >= 300s 即 5 分钟，避免心跳间隔触不到续期窗口）。
const defaultPresenceTTL = 5 * time.Minute

// PresenceRepo 抽象 Redis presence 操作。
//
// 5 个方法（epics.md §Story 10.6 行 1765-1769 钦定 4 个 + 1 个 TTL 续期方法）：
//   - AddOnline: SADD room set + SET user→sessionID + EXPIRE room 三命令编排
//   - RemoveOnline: SREM room set + DEL user→sessionID 双命令编排
//   - IsOnline: SMEMBERS room set + 线性扫描（MVP 单 room 最多 4 user，O(n) 成本极低）
//   - ListOnline: SMEMBERS room set + strconv.ParseUint 收集 []uint64
//   - RenewTTL: EXPIRE 双 key（心跳路径调用让 active session 持续续期不被自动过期）
//
// **接口边界**：本接口**只**含 epics.md 钦定的 5 个方法。如果 future Story 需要
// GetSessionID（user → sessionID 反查）/ ListAllRooms（运维端点）等，**新增方法**
// 而非让调用方走 raw RedisClient 绕过接口。
//
// **mock 路径**：单测可注入"基于 RedisClientMock + miniredis"的真实 PresenceRepo
// 实例，**不**单独写 PresenceRepoMock —— miniredis 是 in-process server，准确度
// 远高于"in-memory map 模拟 Redis 语义"，与 redis_test.go 既有 9 case 模式一致。
//
// **语义分层**（V1 §12.1 第 5 步钦定 + lesson 2026-05-06-ws-frozen-section-authz-and-snapshot-coherence-r6）：
// presence 是 ephemeral 在线态，**不能**作为 membership 校验 single source of truth；
// 业务路径的 authz 必须走 mysql room_members 表（durable membership），presence 只回
// "是否当前有活跃 WS 连接"。
type PresenceRepo interface {
	// AddOnline 把 (roomID, userID, sessionID) 写入 presence。
	//
	// 流程（**review 10-6 r2 P2 修后 + r7 P1 修后**）：
	//  1. SET user:{userID}:ws_session "{sessionID}|{roomID}" ttl（SET KEY VAL EX 单命令
	//     原子，包含 TTL；nx=false → reconnect 替换路径能更新；value 编码 sessionID +
	//     roomID 让 RemoveOnline Lua script 能区分 same-room vs cross-room reconnect）
	//  2. SADD room:{roomID}:online_users {userID-string}
	//  3. EXPIRE room:{roomID}:online_users {ttl}（让 set key 也自带 TTL，避免空 room
	//     key 无人续期僵在 Redis）
	//
	// **顺序决策**（r2 P2）：原版顺序 SADD → SET → EXPIRE 在 SADD 成功 + SET 失败
	// 路径下直接 return，让 room set 永远没 EXPIRE → zombie member 永久存活。改为
	// SET 先做（带 TTL 原子）→ 任何后续命令失败时 user:{id}:ws_session 已有 TTL
	// 兜底，不会留永久 zombie。详见实装 godoc + lesson
	// 2026-05-07-presence-add-online-command-order-and-ttl-guarantee-10-6-r2.md。
	//
	// 错误语义：底层 Redis 命令任一失败 → 返 error，不做"部分写入"兜底；调用方
	// （hook adapter）log warn 即可（本 story 不做 retry / 重试队列；节点 13+
	// 引入 metrics 后可加 counter）。
	//
	// **关键约束**：
	//   - SADD 用单 member 形式（**不**批量 SADD 多 user）—— 接口语义就是单 user 写入
	//   - SET 走 RedisClient.Set 接口的 nx=false 路径；不走 SETNX，因为 reconnect 替换
	//     路径需要更新 sessionID
	//   - 任一命令失败立即返（**不**走"先 SADD 成功了就忽略 SET 失败"补偿语义；语义
	//     不变量是"两 key 要么同时写入要么都不写入"，但 Redis 不支持事务跨命令原子，
	//     所以接受"AddOnline 中途失败时部分 key 已写入" —— TTL 兜底让残留在 5 分钟
	//     内自然清除）
	AddOnline(ctx context.Context, roomID, userID uint64, sessionID string) error

	// RemoveOnline 从 presence 删除 (roomID, userID)。**按 reconnect 类型分支处理
	// SREM**：cross-room reconnect 仍 SREM 旧 room；same-room reconnect 跳过 SREM
	// （否则与新 AddOnline 的 SADD 形成 race 把刚加的删掉）。**仅当**当前
	// user→session 映射等于传入 sessionID 时才 DEL user key（sessionID guard）。
	//
	// 流程（**原子** Lua script 跑在 Redis 端）：
	//  1. GET user:{userID}:ws_session（value 编码 "sessionID|roomID"）
	//  2. 若 key 不存在 → 仅 SREM 旧 room（user 已完全离线；user key 已不存在
	//     不需要清；返 1）
	//  3. 若 currentSession == 传入 sessionID（无 reconnect 抢占）→
	//     SREM 旧 room + DEL user key（完整清理；返 2）
	//  4. 若 currentSession != 传入 sessionID **且** currentRoom != 传入 roomID
	//     （cross-room reconnect：user 已重连到新 room）→ **SREM 旧 room**，
	//     不动 user key（新 session 接管 user key；返 3）
	//  5. 若 currentSession != 传入 sessionID **但** currentRoom == 传入 roomID
	//     （same-room reconnect：新 session 仍在 ARGV[3] room；新 AddOnline 已
	//     SADD 把 user 加进去了）→ **跳过 SREM**（否则把刚加的删掉），不动 user key
	//     （返 4）
	//
	// **关键修法历程**：
	//
	//   - **r4 P1 修**：原版（r1）在 case 3/4 都跳 SREM → cross-room reconnect 路径
	//     user 永远留在旧 room set（详见旧版历程注释保留在实装内）。
	//   - **r7 P1 修**：r4 让 case 3/4 都执行 SREM 修了 cross-room；但 r6 fire-and-forget
	//     hook 让 same-room reconnect 路径变成 race：new AddOnline goroutine 先跑
	//     SET user_key=newSession|roomA + SADD roomA + EXPIRE；旧 RemoveOnline
	//     goroutine 后跑 → r4 case 3 → SREM roomA → user 离开 roomA 直到下一次
	//     scanner reconcile（30s）才自愈。修法：value 编码 "sessionID|roomID"，
	//     script 解析 currentRoom 与 ARGV[3] 比较，区分 same-room（跳 SREM）vs
	//     cross-room（SREM）。详见 lesson
	//     2026-05-07-presence-same-room-reconnect-needs-room-aware-guard-10-6-r7.md。
	//
	// **idempotent**：连续两次 RemoveOnline(sameSessionID) 第二次自动 no-op：
	// 第一次执行后 user:{id}:ws_session 已被 DEL，第二次 GET 拿 nil → script 走
	// case 1 仅 SREM；底层 SREM 对不存在 member 是 no-op，所以无副作用。
	//
	// 错误语义：底层 EVAL 命令失败 → 返 error；script 返 1/2/3/4 都不视为 error。
	RemoveOnline(ctx context.Context, roomID, userID uint64, sessionID string) error

	// IsOnline 检查 user 是否在 room 内 online。
	//
	// 流程：SMEMBERS room:{roomID}:online_users + 线性扫描判断 userID-string 是否在 set
	// 内。go-redis 接口里有 SIsMember 命令，但 RedisClient 抽象（Story 10.2）里没有；
	// 走"SMembers 全量拉取后线性扫描"路径 —— 因为单 room MVP 阶段最多 4 user，扫描
	// 成本极低；如果 future room 容量上千需要 O(1) 命中，再在 RedisClient 上加
	// SIsMember 方法（保持单一抽象边界，避免渐进式失控）。
	//
	// **关键约束**：不存在 room set 返 (false, nil) —— 不视为 error（与 redisinfra
	// 内化 nil error 模式一致；redis_test.go 行 79 已有同模式）。
	IsOnline(ctx context.Context, roomID, userID uint64) (bool, error)

	// ListOnline 返回 room 内全部 online userID。
	//
	// 流程：SMEMBERS room:{roomID}:online_users → for 每个 string member 走
	// strconv.ParseUint(s, 10, 64) → 收集到 []uint64。
	//
	// 错误语义：
	//   - 空 room set 返 ([], nil)（与 SMembers 内化语义一致）
	//   - 任一 member 不是合法 uint64 string → 返 error（理论上不会发生，因为只有
	//     AddOnline 写入；如果发生 = Redis 数据被外部污染，必须 fail-fast 让运维
	//     注意，**不**走 silently skip 兜底）
	ListOnline(ctx context.Context, roomID uint64) ([]uint64, error)

	// RenewTTL 续期 (roomID, userID) 双 key 的 TTL。
	//
	// 流程：
	//  1. EXPIRE room:{roomID}:online_users {ttl}（user 不在 room set 时 EXPIRE 仍然
	//     work，因为 set key 由其他 active user 维持存活）
	//  2. EXPIRE user:{userID}:ws_session {ttl}
	//
	// **何时调用**：当前**生产路径不调用 RenewTTL**（Story 10.6 r3 P2 把 scanner
	// reconcile 路径从 RenewTTL 改成 AddOnline，理由见 docs/lessons/2026-05-07-
	// presence-add-online-self-heal-via-scanner-10-6-r3.md）—— RenewTTL 保留作
	// PresenceRepo 接口契约方法 + 单测覆盖，让未来"纯续期不重写"场景（如 Lua
	// reconcile script）能直接复用，不需要重新加方法。
	//
	// 错误语义：底层 EXPIRE 命令失败 → 返 error；key 不存在导致 EXPIRE 返 false ≠
	// error，正常返 nil（让 caller 不必区分）。
	RenewTTL(ctx context.Context, roomID, userID uint64) error
}

// presenceRepo 是 PresenceRepo 的默认实装。
type presenceRepo struct {
	client redisinfra.RedisClient
	ttl    time.Duration
	logger *slog.Logger
}

// NewPresenceRepo 构造 PresenceRepo 实装。
//
// 参数：
//   - client: Story 10.2 wire 的 RedisClient 单例（main.go 已 wire；不可为 nil；
//     调用方传 nil 会让所有 method 在 client 调用时 NPE，本构造函数**不**做 nil
//     guard —— 与 NewSessionManager / 既有 mysql repo 构造一致，保持简洁）
//   - ttl: presence key 的 TTL（YAML redis.presence_ttl_sec → time.Duration）；
//     传 <= 0 → 用 defaultPresenceTTL (5min)
//
// 返回 PresenceRepo 接口（不返 *presenceRepo struct，让调用方一律走接口；与
// 既有 mysql repo 工厂同模式）。
func NewPresenceRepo(client redisinfra.RedisClient, ttl time.Duration) PresenceRepo {
	if ttl <= 0 {
		ttl = defaultPresenceTTL
	}
	return &presenceRepo{
		client: client,
		ttl:    ttl,
		logger: slog.Default().With(slog.String("component", "presence-repo")),
	}
}

// roomKey 返回 room presence set 的 Redis key。
// 与 docs/宠物互动App_数据库设计.md §9.1 钦定 schema 严格一致：
// `room:{roomId}:online_users` (Set 类型)。
func roomKey(roomID uint64) string {
	return fmt.Sprintf("room:%d:online_users", roomID)
}

// userKey 返回 user → sessionID 映射的 Redis key。
// 与 docs/宠物互动App_数据库设计.md §9.1 钦定 schema 严格一致：
// `user:{userId}:ws_session` (String 类型)。
func userKey(userID uint64) string {
	return fmt.Sprintf("user:%d:ws_session", userID)
}

// userValueSeparator 是 user:{id}:ws_session value 内 sessionID 与 roomID 的分隔符。
//
// **review 10-6 r7 P1 修**：value 从纯 sessionID 改成 "sessionID|roomID" 组合字符串，
// 让 RemoveOnline 的 Lua script 能区分 same-room vs cross-room reconnect（详见
// removeOnlineLuaScript 注释）。选 "|" 是因为 sessionID 是 uuid v4（hex + "-"），
// 与分隔符不冲突；roomID 是十进制 uint64 也无 "|"。schema 仍是 String 类型（与
// V1 §9.1 钦定 user:{userId}:ws_session 兼容；只是 value 编码扩展）。
const userValueSeparator = "|"

// formatUserValue 把 sessionID + roomID 打包成 user:{id}:ws_session 的 value。
// 与 parseUserValue / Lua script string.find/sub 解析逻辑严格对偶。
func formatUserValue(sessionID string, roomID uint64) string {
	return sessionID + userValueSeparator + strconv.FormatUint(roomID, 10)
}

// addRoomMemberLuaScript 把 SADD + EXPIRE 包成一个 Lua 段，让 room set member
// 加入与 TTL 设置在 Redis 端**原子**完成 —— 走 EVAL 路径不可被 server crash /
// 命令间断点 split。
//
// KEYS[1] = roomKey(roomID)            （目标 set key）
// ARGV[1] = userID 字符串              （SADD 的 member）
// ARGV[2] = TTL 秒数（整数字符串）     （EXPIRE 的 seconds）
//
// 返回 1（固定）—— 上层不区分；调用方仅通过 EVAL error 判断整段成功 / 失败。
//
// **review 10-6 r8 P2 修**：原版 AddOnline 用 SADD → EXPIRE 两条独立命令，partial
// fail 矩阵里 "SADD 成功 + EXPIRE 失败" 路径让 room set 写入 member 但**无 TTL**
// → process crash 时 user_key 因 SET KEY VAL EX 自带 TTL 在 5min 后自然过期，但
// room set 上的 member 会**永久残留**（无 TTL 兜底；下次 RemoveOnline 走 Lua
// script Get 拿到的 user_key 已不存在 → 走 case 1 仅 SREM 这一个 member —— 但
// 如果 user 一直没 reconnect 也没 unregister 触发，没人 SREM）。后果：IsOnline /
// ListOnline 永久 over-report 该 (room, user)，违反 "TTL 兜底清理 zombie" 设计意图。
//
// 修法：把 SADD + EXPIRE 包进 Lua —— Redis 单线程串行执行 EVAL 整段，要么两条都
// 成功要么 EVAL 整体失败（network / Redis crash）。EVAL 失败时 SADD 也没执行
// （如果 SADD 已写入 server 但回包失败前 Redis crash，AOF / RDB 持久化路径上是
// 一个非常窄的 corner —— 即便发生，下次 AddOnline 同 user 触发 SADD（idempotent）+
// EXPIRE 立刻自愈；scanner 30s reconcile tick 也走同路径自愈）。
//
// 详见 docs/lessons/2026-05-07-fire-and-forget-hooks-need-per-user-mutex-10-6-r8.md。
const addRoomMemberLuaScript = `redis.call("SADD", KEYS[1], ARGV[1])
redis.call("EXPIRE", KEYS[1], ARGV[2])
return 1`

// AddOnline 实装 PresenceRepo.AddOnline（详见 interface godoc）。
//
// **review 10-6 r2 P2 修**：命令顺序改为 SET → SADD → EXPIRE（原版是
// SADD → SET → EXPIRE）。原版的 partial-fail 后果是 "SADD 成功 + SET 失败 →
// 直接 return → room set 永远没 EXPIRE → zombie member 永久存活"，与本 repo
// "TTL 兜底清理 zombie" 语义直接矛盾。
//
// **review 10-6 r7 P1 修**：user:{id}:ws_session 的 value 从纯 sessionID 改成
// "sessionID|roomID"（分隔符 "|"）。RemoveOnline 的 Lua script 据此区分 same-room
// reconnect（不该 SREM 旧 room，否则刚加进去就被删）vs cross-room reconnect
// （仍要 SREM 旧 room，让 user 干净离开旧 room set）。详见 removeOnlineLuaScript
// 注释 + lesson 2026-05-07-presence-same-room-reconnect-needs-room-aware-guard-10-6-r7.md。
//
// **review 10-6 r8 P2 修**：SADD + EXPIRE 改用 Lua script 原子段（addRoomMemberLuaScript）。
// 原版 r2 修后注释里说"SADD 成功 + EXPIRE 失败 → 影响范围由永久缩到一次 AddOnline
// cycle"，实测仍存在永久残留 corner —— process crash 在 EXPIRE 失败后让 user_key
// TTL 5min 过期，但 room set 上 member 没人触发 SREM 永久残留。修法：SADD + EXPIRE
// Lua 原子化，要么两条都跑要么都没跑。
//
// 修后 partial-fail 矩阵：
//   - SET 失败 → return → Lua 段没执行 → 不留任何残留（最干净）
//   - SET 成功 + Lua 段失败 → return → user:{id}:ws_session 已写入有 TTL，room
//     set 不变（要么 SADD+EXPIRE 都成功要么都没执行）。下次 AddOnline 同 user
//     重试自愈（SET 覆盖 + Lua 重跑 → 完整恢复）。process crash 时 user_key TTL
//     过期，room set 也不变（无残留）。
//   - SET 成功 + Lua 段成功 → 完整一致状态。
//
// 详见 docs/lessons/2026-05-07-fire-and-forget-hooks-need-per-user-mutex-10-6-r8.md。
func (r *presenceRepo) AddOnline(ctx context.Context, roomID, userID uint64, sessionID string) error {
	rk := roomKey(roomID)
	uk := userKey(userID)
	uidStr := strconv.FormatUint(userID, 10)
	userValue := formatUserValue(sessionID, roomID)

	// 第一步：SET user:{id}:ws_session 走 SET KEY VAL EX 单命令原子（包含 TTL）。
	// SET nx=false：reconnect 替换路径能更新 sessionID（同 user 二次 AddOnline 覆盖旧值）。
	// 失败立即 return，**之前没动** Lua 段 —— 不留任何残留。
	// value = "sessionID|roomID"（review 10-6 r7 P1 修）让 RemoveOnline Lua script
	// 能比较 currentRoom vs oldRoom 区分 same-room / cross-room reconnect。
	if _, err := r.client.Set(ctx, uk, userValue, r.ttl, false); err != nil {
		return fmt.Errorf("presence add online set: %w", err)
	}
	// 第二步：Lua 段 SADD + EXPIRE 原子化（review 10-6 r8 P2 修）。
	// 原版分两条命令的 partial-fail 路径让 room set 在 EXPIRE 失败时写入 member
	// 但无 TTL → process crash 后永久残留。Lua 段把两步打包，保证 SADD 写入 ↔
	// TTL 设置原子绑定；EVAL 失败时 SADD 也未生效，残留风险 = 0。
	ttlSeconds := int64(r.ttl / time.Second)
	if _, err := r.client.Eval(ctx, addRoomMemberLuaScript, []string{rk}, uidStr, ttlSeconds); err != nil {
		return fmt.Errorf("presence add online sadd+expire: %w", err)
	}
	return nil
}

// removeOnlineLuaScript 是 RemoveOnline 用的 Lua script，跑在 Redis 端**原子**完成
// "按 reconnect 类型分支处理 SREM；仅 sessionID 匹配 / 已不存在时才 DEL user key"。
//
// KEYS[1] = roomKey(roomID)         （SREM 的 set key —— 调用方传入的旧 room）
// KEYS[2] = userKey(userID)         （GET / DEL 的 string key —— user → sessionID|roomID 映射）
// ARGV[1] = sessionID               （要清的目标旧 sessionID）
// ARGV[2] = userID 字符串            （SREM 的 member）
// ARGV[3] = roomID 字符串            （要清的旧 roomID —— 用于比较 currentRoom 区分 same/cross-room）
//
// 返回值（用于单测断言走对了哪个分支；上层不区分）：
//   - 1: GET 返 false（user key 不存在 / TTL 过期）—— 仅 SREM 旧 room
//   - 2: currentSession == 传入 sessionID（无 reconnect 抢占）—— SREM 旧 room + DEL user key
//   - 3: currentSession != 传入 sessionID **且** currentRoom != 传入 roomID
//        （cross-room reconnect：新 session 在不同 room）—— SREM 旧 room，**不**动 user key
//   - 4: currentSession != 传入 sessionID **但** currentRoom == 传入 roomID
//        （same-room reconnect：新 session 仍在 ARGV[3] room；新 AddOnline 已经
//        SADD 把 user 加进去了）—— **跳过** SREM（否则把刚加的删掉），**不**动 user key
//
// **关键不变量**（review 10-6 r7 P1）：same-room reconnect 路径下旧 RemoveOnline
// 的 SREM 必须**跳过**，否则与新 AddOnline 的 SADD 形成 "add → del" race，让
// IsOnline 在窗口内返 false 直到下一次 scanner reconcile（30s）才自愈。区分
// same-room 与 cross-room 的关键是 currentRoom（user key value 的第二段）。
//
// **r4 P1 修后 r7 P1 修前的 bug**：r4 把 case 3（sessionID 不匹配）的 SREM 总是
// 执行用来修 cross-room reconnect 残留。但 r6 把 hook 改 fire-and-forget 后，
// same-room reconnect 路径变成 "new AddOnline goroutine 先跑：SET user_key=
// newSession|roomA + SADD roomA + EXPIRE；旧 RemoveOnline goroutine 后跑：Lua
// GET → newSession|roomA ≠ oldSession → r4 case 3 → SREM roomA → user 离开
// roomA"。修前依赖 r6 注释里"短窗口自愈"，但实测 IsOnline / ListOnline 在窗口
// 内返错才到 scanner 30s tick 才修复，违反 V1 §12 "presence 是查询时态" 的语义
// 健壮度。
//
// **r7 修法**：value 编码 `"sessionID|roomID"`，Lua script 解析 currentRoom 与
// ARGV[3]（旧 roomID）比较：
//   - currentRoom == oldRoom (ARGV[3]) → same-room reconnect → 跳过 SREM（return 4）
//   - currentRoom != oldRoom → cross-room reconnect → SREM 旧 room（return 3）
// user key 的 DEL 仍受 sessionID guard 保护（case 3/4 都跳过 DEL）。
//
// **为什么必须 Lua script 而非 pipeline / 三命令分开**：
//   - Pipeline 不保证原子性 —— GET 与 SREM/DEL 之间可能被另一 client 的 SET 插入，
//     仍有 race window
//   - Lua script 在 Redis 端 single-thread 执行，GET + SREM + DEL 三命令原子；
//     reconnect 抢占若在 script 执行期发生，会被 Redis 的 single-thread 序列化
//     （要么发生在 script 之前要么之后，永远不会"夹在中间"）
//
// 详见 lesson 2026-05-07-presence-same-room-reconnect-needs-room-aware-guard-10-6-r7.md。
const removeOnlineLuaScript = `local current = redis.call("GET", KEYS[2])
if current == false then
  redis.call("SREM", KEYS[1], ARGV[2])
  return 1
end
local sep = string.find(current, "|", 1, true)
local current_session
local current_room
if sep == nil then
  current_session = current
  current_room = ""
else
  current_session = string.sub(current, 1, sep - 1)
  current_room = string.sub(current, sep + 1)
end
if current_session == ARGV[1] then
  redis.call("SREM", KEYS[1], ARGV[2])
  redis.call("DEL", KEYS[2])
  return 2
end
if current_room == ARGV[3] then
  return 4
end
redis.call("SREM", KEYS[1], ARGV[2])
return 3`

// RemoveOnline 实装 PresenceRepo.RemoveOnline（详见 interface godoc）。
//
// 修法历程：
//   - Story 10.6 r1 P1：原实装是 SREM + DEL 双命令直接删除，会让 reconnect 替换路径
//     旧 Session 的延后 Unregister 钩子误删新 Session 的 user key。改用 Lua script 跑
//     在 Redis 端原子 compare-and-delete：仅当 user:{id}:ws_session 当前值 == 传入
//     sessionID（或 key 不存在）时才执行 SREM + DEL。
//   - Story 10.6 r4 P1：r1 实装存在反向缺陷 —— sessionID 不匹配（被 scanner /
//     reconnect 抢占）时连 SREM 都跳过，cross-room reconnect 路径下 user 永远留在
//     旧 room set。改成"无论 GET 走哪个分支，旧 room 的 SREM 都执行；仅 user key
//     DEL 受 sessionID guard 保护"。详见 removeOnlineLuaScript 注释 + lesson
//     2026-05-07-presence-cross-room-reconnect-srem-old-room-10-6-r4.md。
//   - Story 10.6 r7 P1：r4 改后 r6 fire-and-forget hook 让 same-room reconnect
//     路径出现"new AddOnline goroutine 先跑 SADD，旧 RemoveOnline goroutine 后跑
//     SREM" 的 race —— 旧 SREM 把刚加进去的 member 删掉，IsOnline / ListOnline
//     在 30s scanner 自愈窗口内返错。修法：user value 编码 "sessionID|roomID"，
//     Lua script 比较 currentRoom 与 ARGV[3] 区分 same-room（跳 SREM）vs cross-room
//     （SREM 旧 room）。详见 lesson
//     2026-05-07-presence-same-room-reconnect-needs-room-aware-guard-10-6-r7.md。
func (r *presenceRepo) RemoveOnline(ctx context.Context, roomID, userID uint64, sessionID string) error {
	rk := roomKey(roomID)
	uk := userKey(userID)
	uidStr := strconv.FormatUint(userID, 10)
	roomIDStr := strconv.FormatUint(roomID, 10)

	// ARGV[3] = 旧 roomID 字符串：让 Lua script 比较 currentRoom（user key value 第二段）
	// 与 ARGV[3] 区分 same-room reconnect vs cross-room reconnect。
	if _, err := r.client.Eval(ctx, removeOnlineLuaScript, []string{rk, uk}, sessionID, uidStr, roomIDStr); err != nil {
		return fmt.Errorf("presence remove online eval: %w", err)
	}
	return nil
}

// IsOnline 实装 PresenceRepo.IsOnline（详见 interface godoc）。
//
// 走 SMembers + 线性扫描，原因见 interface godoc。
func (r *presenceRepo) IsOnline(ctx context.Context, roomID, userID uint64) (bool, error) {
	rk := roomKey(roomID)
	uidStr := strconv.FormatUint(userID, 10)

	members, err := r.client.SMembers(ctx, rk)
	if err != nil {
		return false, fmt.Errorf("presence is online smembers: %w", err)
	}
	// SMembers 对不存在 set 返 ([], nil)（redisinfra 已内化）；线性扫描判断 uidStr 是否在内。
	for _, m := range members {
		if m == uidStr {
			return true, nil
		}
	}
	return false, nil
}

// ListOnline 实装 PresenceRepo.ListOnline（详见 interface godoc）。
func (r *presenceRepo) ListOnline(ctx context.Context, roomID uint64) ([]uint64, error) {
	rk := roomKey(roomID)

	members, err := r.client.SMembers(ctx, rk)
	if err != nil {
		return nil, fmt.Errorf("presence list online smembers: %w", err)
	}
	if len(members) == 0 {
		// 显式返 nil 切片让调用方拿到 zero-value（与 redisinfra SMembers 空 set 行为一致）；
		// require.Empty 对 nil / [] 都视为通过，单测兼容。
		return nil, nil
	}
	out := make([]uint64, 0, len(members))
	for _, m := range members {
		uid, perr := strconv.ParseUint(m, 10, 64)
		if perr != nil {
			// 理论上不会发生：AddOnline 是唯一写入路径，写的就是 strconv.FormatUint。
			// 如果发生 = 外部污染或代码 bug，必须 fail-fast 而非 silently skip
			// （否则 caller 拿到的 []uint64 漏掉某 member，behavior 反向 hidden）。
			return nil, fmt.Errorf("presence list online parse member %q: %w", m, perr)
		}
		out = append(out, uid)
	}
	return out, nil
}

// RenewTTL 实装 PresenceRepo.RenewTTL（详见 interface godoc）。
//
// 双 key 顺序续期：room set + user→session string；任一失败立即返 error。
// EXPIRE 对不存在 key 返 (false, nil)，**不**视为 error —— caller 不必区分。
func (r *presenceRepo) RenewTTL(ctx context.Context, roomID, userID uint64) error {
	rk := roomKey(roomID)
	uk := userKey(userID)

	if _, err := r.client.Expire(ctx, rk, r.ttl); err != nil {
		return fmt.Errorf("presence renew ttl expire room: %w", err)
	}
	if _, err := r.client.Expire(ctx, uk, r.ttl); err != nil {
		return fmt.Errorf("presence renew ttl expire user: %w", err)
	}
	return nil
}
