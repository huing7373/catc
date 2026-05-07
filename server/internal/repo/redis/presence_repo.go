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
	// 流程（**review 10-6 r2 P2 修后**）：
	//  1. SET user:{userID}:ws_session {sessionID} ttl（SET KEY VAL EX 单命令原子，
	//     包含 TTL；nx=false → reconnect 替换路径能更新 sessionID）
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

	// RemoveOnline 从 presence 删除 (roomID, userID)，**仅当**当前 user→sessionID
	// 映射等于传入 sessionID 时才删除（sessionID guard）。
	//
	// 流程（**原子** Lua script 跑在 Redis 端）：
	//  1. GET user:{userID}:ws_session
	//  2. 若返回值 == 传入 sessionID **或** key 不存在（首次 / TTL 已过）→
	//     SREM room:{roomID}:online_users {userID-string} + DEL user:{userID}:ws_session
	//  3. 否则（user→sessionID 已被 reconnect 覆盖为新 sessionID）→ no-op，跳过 SREM/DEL
	//
	// **关键修法**（Story 10.6 r1 P1）：sessionID guard 防 reconnect 路径误删新 session 的
	// presence。SessionManager.Register 替换路径设计是"new Session 完成 Register（新
	// AddOnline 写 user:{id}:ws_session=newSessionID）→ old Session 异步 Close → 旧
	// Unregister 触发钩子调 RemoveOnline"。如果 RemoveOnline 不带 guard 直接 SREM/DEL，
	// 旧 Unregister 会清掉新 session 刚写入的 presence，让 IsOnline / ListOnline 误报
	// user offline 即使 active WS 还在。Lua script 的 GET + SREM + DEL 三命令必须**原子**
	// （非 pipeline）—— 比较 + 删除若拆成两步 check-then-delete，仍存在 race window。
	//
	// **idempotent**：连续两次 RemoveOnline(sameSessionID) 第二次自动 no-op：第一次执行后
	// user:{id}:ws_session 已被 DEL，第二次 GET 拿 nil → script 把 nil 当作 "首次 / TTL
	// 已过" 分支，会再尝试 SREM / DEL，但底层 SREM / DEL 对不存在 key 都是 no-op，所以
	// 无副作用。
	//
	// 错误语义：底层 EVAL 命令失败 → 返 error；script 返 0 (no-op) 不视为 error。
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

// AddOnline 实装 PresenceRepo.AddOnline（详见 interface godoc）。
//
// **review 10-6 r2 P2 修**：命令顺序改为 SET → SADD → EXPIRE（原版是
// SADD → SET → EXPIRE）。原版的 partial-fail 后果是 "SADD 成功 + SET 失败 →
// 直接 return → room set 永远没 EXPIRE → zombie member 永久存活"，与本 repo
// "TTL 兜底清理 zombie" 语义直接矛盾。
//
// 修后顺序的 partial-fail 矩阵：
//   - SET 失败 → return → SADD 没执行 → 不留任何残留（最干净）
//   - SET 成功 + SADD 失败 → return → user:{id}:ws_session 已写入但有 TTL（SET KEY
//     VAL EX 单命令原子）→ 5min 后自然过期 → 不 zombie
//   - SET 成功 + SADD 成功 + EXPIRE 失败 → return → user:{id}:ws_session 有 TTL，
//     room set 没 TTL；但 room set 的存活由 SREM/DEL 在 RemoveOnline 路径走 Lua
//     script 清理（user→sessionID DEL 后 SREM 同步执行），不会真正永久 zombie ——
//     最坏情况是某 user 在 TTL 5min 内未 reconnect / unregister，room set 上残留
//     该 user member 直到下一次 AddOnline 同 room 触发 EXPIRE 续期；与原版
//     "SADD 后 SET 失败 → return → room set 永久无 TTL" 相比，影响范围由"永久"
//     缩到"最多一次 AddOnline cycle 内"，且 RemoveOnline 走 Lua 仍能 SREM 干净。
//
// 三连写仍非原子（Redis 单 client 命令间不保事务）；本简化版接受 "短窗口轻度
// 不一致 + TTL 自愈" 而不是 Lua script 升级 —— Lua 改动范围更大且本 partial-fail
// 场景下后果已可接受。详见 docs/lessons/2026-05-07-presence-add-online-command-order-and-ttl-guarantee-10-6-r2.md。
func (r *presenceRepo) AddOnline(ctx context.Context, roomID, userID uint64, sessionID string) error {
	rk := roomKey(roomID)
	uk := userKey(userID)
	uidStr := strconv.FormatUint(userID, 10)

	// 第一步：SET user:{id}:ws_session 走 SET KEY VAL EX 单命令原子（包含 TTL）。
	// SET nx=false：reconnect 替换路径能更新 sessionID（同 user 二次 AddOnline 覆盖旧值）。
	// 失败立即 return，**之前没动** SADD —— 不留任何残留。
	if _, err := r.client.Set(ctx, uk, sessionID, r.ttl, false); err != nil {
		return fmt.Errorf("presence add online set: %w", err)
	}
	// 第二步：SADD room:{id}:online_users 把 user 加入 set。失败 return → user
	// 已写入 user:{id}:ws_session 但**有 TTL**（5min 后自然过期），不 zombie；下一次
	// AddOnline 同 user 调用会重写 user:{id}:ws_session + 重新 SADD（语义恢复）。
	if _, err := r.client.SAdd(ctx, rk, uidStr); err != nil {
		return fmt.Errorf("presence add online sadd: %w", err)
	}
	// 第三步：EXPIRE room set —— SADD 不带 TTL 选项，需要单独命令兜底防 set key
	// 永久存活。每次 AddOnline 都覆盖 room set 的 TTL（与 SET nx=false 同模式）；
	// 这让 active room 自然续期，与"心跳路径 RenewTTL"分工：AddOnline 续 room TTL
	// 一次，RenewTTL（review 10-6 r2 P1 钩在 HeartbeatScanner 30s tick 上）主动
	// 定期续期。
	if _, err := r.client.Expire(ctx, rk, r.ttl); err != nil {
		return fmt.Errorf("presence add online expire room: %w", err)
	}
	return nil
}

// removeOnlineLuaScript 是 RemoveOnline 用的 Lua script，跑在 Redis 端**原子**完成
// "compare-and-delete"：仅当 user:{id}:ws_session 当前值 == 传入 sessionID（或 key 不
// 存在 / 已 TTL 过期）时才执行 SREM + DEL。
//
// KEYS[1] = roomKey(roomID)         （SREM 的 set key）
// KEYS[2] = userKey(userID)         （GET / DEL 的 string key）
// ARGV[1] = sessionID               （要删的目标 session）
// ARGV[2] = userID 字符串            （SREM 的 member）
//
// 返回值：
//   - 1: compare 命中 / key 不存在，已执行 SREM + DEL（删除路径）
//   - 0: compare 失败（user:{id}:ws_session 已被 reconnect 覆盖为新 sessionID），no-op
//
// **为什么必须 Lua script 而非 pipeline / 三命令分开**：
//   - Pipeline 不保证原子性 —— GET 与 SREM/DEL 之间可能被另一 client 的 SET 插入，
//     仍有 race window
//   - Lua script 在 Redis 端 single-thread 执行，GET + SREM + DEL 三命令原子；
//     reconnect 路径若在 script 执行期发生，会被 Redis 的 single-thread 序列化
//     （要么发生在 script 之前要么之后，永远不会"夹在中间"）
//
// **为什么 GET == false（key 不存在）也走删除分支**：
//   - 首次 RemoveOnline 之后 user:{id}:ws_session 已被 DEL；再次调 RemoveOnline 时
//     GET 返 false → 仍走 SREM/DEL no-op 路径（底层 SREM/DEL 对不存在 key 无副作用）
//   - TTL 自然过期路径 / scanner 清理路径同样合理：key 不存在意味着 "user 已不在线"，
//     SREM/DEL 兜底清残留是正确语义
//   - 若严格 "key 不存在 → 不删" 反而会让 SREM 在 user:{id}:ws_session TTL 提前过期
//     但 room set 还有 member 的边角场景下漏清 → 残留到 room set TTL（5min）才清，
//     违反"unregister 应清干净"的语义
const removeOnlineLuaScript = `local current = redis.call("GET", KEYS[2])
if current == false or current == ARGV[1] then
  redis.call("SREM", KEYS[1], ARGV[2])
  redis.call("DEL", KEYS[2])
  return 1
end
return 0`

// RemoveOnline 实装 PresenceRepo.RemoveOnline（详见 interface godoc）。
//
// Story 10.6 r1 P1 修：原实装是 SREM + DEL 双命令直接删除，会让 reconnect 替换路径
// 旧 Session 的延后 Unregister 钩子误删新 Session 的 presence。改用 Lua script 跑在
// Redis 端原子 compare-and-delete：仅当 user:{id}:ws_session 当前值 == 传入 sessionID
// （或 key 不存在）时才删除。详见 removeOnlineLuaScript 注释 + lesson
// 2026-05-07-redis-presence-remove-needs-session-id-guard-10-6-r1.md。
func (r *presenceRepo) RemoveOnline(ctx context.Context, roomID, userID uint64, sessionID string) error {
	rk := roomKey(roomID)
	uk := userKey(userID)
	uidStr := strconv.FormatUint(userID, 10)

	if _, err := r.client.Eval(ctx, removeOnlineLuaScript, []string{rk, uk}, sessionID, uidStr); err != nil {
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
