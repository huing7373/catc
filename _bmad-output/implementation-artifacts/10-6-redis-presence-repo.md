# Story 10.6: Redis presence repo（房间在线用户 + WS session 映射 + lifecycle 钩子挂载 + TTL 续期 + 单测/集成测覆盖）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 在 Story 10.2 RedisClient 抽象（含 SAdd / SRem / SMembers / Set / Del / Expire 7 个 ctx-aware 命令）+ 10.3 SessionManager（含 `WithRegisterHook(fn func(*Session))` / `WithUnregisterHook(fn func(*Session))` functional option 钩子注入接口）+ 10.4 HeartbeatScanner（含 `ListAllSessions` 全局扫描 + `CloseWithCode(4005)` idle 清理）+ 10.5 BroadcastToRoom primitive（**MVP 单实例阶段已直接用 SessionManager.ListSessionsByRoomID 拿目标 Session，不消费 Redis presence**）这套既有基础上，新建 `internal/repo/redis/` 子目录的第一个文件 `presence_repo.go` 提供 PresenceRepo 接口 + Redis 实装：4 个核心方法 `AddOnline(ctx, roomID, userID, sessionID) error` / `RemoveOnline(ctx, roomID, userID) error` / `IsOnline(ctx, roomID, userID) (bool, error)` / `ListOnline(ctx, roomID) ([]uint64, error)`，按 docs/宠物互动App_数据库设计.md §9.1 钦定的 Redis key schema `room:{roomId}:online_users` (Set, members = userID 字符串化) + `user:{userId}:ws_session` (String, value = sessionID) 落地，所有 key 自带 TTL（默认 5 分钟，可配 `redis.presence_ttl_sec`）防 server crash 留僵尸记录 + 心跳路径心跳一次 TTL 续期一次（`Expire` 调用挂在 Story 10.4 HeartbeatScanner 已 wire 的 lastHeartbeatAt 更新触发点 / 或新增 RenewTTL 方法在心跳路径调用）；同时把 `AddOnline` / `RemoveOnline` 钩子在 main.go bootstrap 期通过 `wsapp.WithRegisterHook` / `wsapp.WithUnregisterHook` 挂到 SessionManager，让 Session lifecycle 自动驱动 presence 写入（**钦定不变量**：Register 钩子 → AddOnline 同步执行 / Unregister 钩子 → RemoveOnline 同步执行 / Reconnect 替换路径 oldSession.onUnregister 必须触发 = RemoveOnline 必触发 —— Story 10.3 r2 P1 钉死的 lesson `2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md` 已在 SessionManager 实装层兜底）；并交付完整测试矩阵（≥ 5 个单测 case 用 miniredis + 真 RedisClient 跑通 SADD/SREM 语义/TTL FastForward 时序；集成测 50 个 user 并发 AddOnline → ListOnline 返 50；钩子集成测验证 Register / Unregister / Reconnect 替换路径都触发 presence 写入；nil RedisClient panic-free 兜底测试）；严格按 V1 §12.1 钦定的"presence 是 ephemeral 在线态，**禁止**作为 membership 校验 single source of truth"分割语义（详见 lesson `2026-05-06-ws-frozen-section-authz-and-snapshot-coherence-r6.md`），**不**反向修改 V1 / 数据库设计 / Go 项目结构三份文档，
so that Story 10.7 房间快照下发框架（在 placeholder 阶段虽然不消费 presence，但 Epic 11.7 真实 SnapshotBuilder 会用 ListOnline 标 isOnline 字段）/ Epic 11.8 成员加入退出广播（用 BroadcastToRoom 间接通过 SessionManager 拿目标，但 presence 自身要随业务事务原子推进）/ Epic 14.4 pet.state.changed 跨实例广播伸缩（节点 9+ 多实例部署时 presence 是跨进程在线态唯一可见入口）/ Epic 36 graceful shutdown 在 SessionManager.Close 路径触发全部 onUnregister → 全部 RemoveOnline 兜底清空 → 不留僵尸 user 在 Redis 等所有"server 端需要知道 user 是否当前有活跃 WS 连接"的下游业务都能依赖单一 PresenceRepo 接口，不再出现"业务模块自己 import go-redis 直连，key schema / TTL / 错误语义各写一套"的拼凑式代码；且本 story 通过把 lifecycle 钩子挂载（Register → AddOnline / Unregister → RemoveOnline / Reconnect 替换 → oldSession.onUnregister → RemoveOnline）三个不变量在 hook 注入层面统一固化（参考 Story 10.3 r2 P1 / 10.5 r3 钉死的同模式：lifecycle 事件 → 钩子精准触发一次），让后续 Epic 加"在线态相关业务"时只需消费 PresenceRepo 接口，不必反向重学 SessionManager 内部 sessionsByID / sessionsByRoom / userToSessionID 三索引并发模型 + onUnregister 钩子触发条件。

## 故事定位（Epic 10 第六条 = 房间在线态 Redis 持久层奠基；对标 Story 4.2 在 Epic 4 的 MySQL 接入角色 + Story 10.5 在 Epic 10 的 broadcast primitive 角色，本 story 是 Epic 10 收官 10.7 房间快照下发框架 + Epic 11 ~ Epic 17 / Epic 36 graceful shutdown 所有 "server 端需要在线态可见性" story 的强前置）

- **Epic 10 进度**：10.1 (WS 协议骨架文档锚定) done → 10.2 (Redis 接入) done → 10.3 (WS 网关骨架 + lifecycle 钩子接口) done → 10.4 (心跳框架 + HeartbeatScanner) done → 10.5 (BroadcastToRoom primitive) done → **本 story (10.6 Redis presence repo)** → 10.7 (房间快照下发框架)
- **强前置关系**：
  - **Story 10.2 done 提供的强前置**：
    - `redisinfra.RedisClient` 接口（已实装；本 story 直接消费 7 命令 —— 见 `server/internal/infra/redis/client.go:40` 接口定义）
    - `Get / Set / Del / Expire / SAdd / SRem / SMembers / Close` 全部 ctx-aware（ADR-0007 钦定；本 story 内所有 PresenceRepo 方法首参数 `ctx context.Context`）
    - `redisinfra.NewRedisClientFromMiniredis(t, mr)` 测试 helper（已实装；本 story 单测复用此 helper + `testhelper.NewMiniRedis(t)`，**不**写"纯 in-memory map"mock —— 与 redis_test.go 既有 9 case 模式严格一致）
    - `redisinfra.Open(ctx, cfg)` fail-fast 启动路径（已实装；本 story 不动 Open；presence repo 在 main.go 已 wire RedisClient 之后构造）
    - **Set NX 选项**（`Set(ctx, key, value, expiration, nx bool)` 第 5 参数）已实装（本 story 不消费 nx，AddOnline 走"无条件覆盖"语义，让重连场景能更新 sessionID）
  - **Story 10.3 done 提供的强前置**：
    - `wsapp.WithRegisterHook(fn func(*Session))` / `wsapp.WithUnregisterHook(fn func(*Session))` 两个 functional option（已实装 —— 见 `server/internal/app/ws/session_manager.go:88-100`；本 story main.go bootstrap 期 wire `WithRegisterHook(adapter to AddOnline)` + `WithUnregisterHook(adapter to RemoveOnline)`）
    - `Session.UserID() / RoomID() / SessionID()` 公开 getter（已实装；本 story 钩子 adapter 内调）
    - **关键不变量 1（lesson `2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md`）**：reconnect 替换路径 oldSession.onUnregister 钩子精准触发**恰好一次**（已在 r2 P1 修复 —— `removeFromIndicesLocked(oldS)` 改为保留索引到 `oldS.Close()` 跑完后再走 `Unregister(oldID)` 标准路径）；本 story 钩子 adapter 不需要再做"是否 reconnect 替换"判断 —— SessionManager 已经把"每个 lifecycle 事件触发恰好一次钩子"语义锁死，AddOnline / RemoveOnline 直接信任钩子输入即可
    - **关键不变量 2（lesson `2026-05-06-ws-session-send-close-race-and-shutdown-hooks.md`）**：SessionManager.Close 路径必须为**每个**注册的 Session 都触发 onUnregister（已在 r1 修复 —— 保留索引到所有 Close 跑完）；本 story graceful shutdown 路径 main.go defer 调 sessionMgr.Close 时，每个 Session 都会触发 RemoveOnline → Redis presence 不留僵尸记录
  - **Story 10.4 done 提供的强前置**：
    - `wsapp.HeartbeatScanner` 后台 goroutine（已实装；本 story 不动 scanner —— scanner 在 idle Session 超时时调 `CloseWithCode(4005)` → Session 走 closeInternal → notifyClosed → manager.Unregister → onUnregister 钩子 → RemoveOnline；presence 清理路径**完全复用** scanner 已 wire 的 close 路径，本 story **不**直接消费 scanner）
    - `Session.lastHeartbeatAt` 更新点（在 Session 收到 ping 消息时）：本 story **不**直接 hook lastHeartbeatAt 更新；TTL 续期走"主动 RenewTTL"路径（详见 §"实装关键决策" §3）—— 让 RenewTTL 频率与 TTL 长度独立配置
    - **关键不变量 3（lesson `2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md`）**：shutdown 路径 main.go defer 必须 `cancelHeartbeat() → wait scannerDone → sessionMgr.Close()` 串行执行；本 story 钩子 adapter 内的 RemoveOnline 调用必须能容忍 ctx 已 cancel 的场景（详见 §"实装关键决策" §4 ctx 与 lifecycle 钩子的关系）
  - **Story 10.5 done 提供的强前置**：
    - `wsapp.BroadcastToRoom(ctx, mgr, roomID, msg) (sent int, err error)` 包级 primitive（已实装）—— **MVP 单实例阶段不消费 presence**：BroadcastToRoom 直接走 `mgr.ListSessionsByRoomID(ctx, roomID)` 拿 Session 切片，跟 Redis presence 完全正交；presence 在节点 9+ 多实例部署时才作为"跨进程在线态可见性入口"用于 Pub/Sub 路径（与 Story 10.5 §"故事定位" "下游立即依赖" 钦定一致）
    - 本 story **不**改 BroadcastToRoom 实装；只是让 presence 通过钩子与 SessionManager 内部 `sessionsByRoom` 索引保持一致（双写：SessionManager 写内存索引 + presence 写 Redis），让节点 9+ 切多实例时业务层切换 BroadcastToRoom → BroadcastToRoomViaPresence(presenceRepo) 时无需重写 presence 写入路径
  - **下游立即依赖**：
    - **Story 10.7（房间快照下发框架）**：placeholder 阶段**不**消费 presence（用 SELECT FROM room_members 直接查全成员，由 V1 §12.3 钦定 placeholder 行为），但**接口契约预留**让 Epic 11.7 真实 SnapshotBuilder 在内部用 `presenceRepo.ListOnline(roomID)` 标 `isOnline=true/false` 字段；本 story 必须保证 ListOnline 返 `[]uint64` 而非 `[]string`，让 Epic 11.7 直接用业务 ID 类型（与 V1 §12.3 字段表 userId uint64 钦定一致），减少类型转换 boilerplate
    - **Story 11.4（加入房间事务）**：MySQL 事务**成功提交后**调 BroadcastToRoom(roomID, member.joined)；这里**不**调 presence.AddOnline —— presence 的 AddOnline 由 WS Session 注册钩子驱动（用户连 ws 才算 online，光加入房间不算）；语义分层钦定：room_members（durable membership） vs presence（ephemeral online）—— lesson `2026-05-06-ws-frozen-section-authz-and-snapshot-coherence-r6.md` 已锁死
    - **Story 11.5（退出房间事务）**：与 11.4 同模式 —— 调 BroadcastToRoom(roomID, member.left)，**不**调 presence.RemoveOnline；presence 由 ws 断连钩子驱动
    - **Story 11.7（房间快照真实实现）**：epics.md 行 1964 钦定"读 Redis presence `room:{roomId}:online_users`（标识谁在线）"；本 story 的 ListOnline 是该消费点；行 1971 钦定"edge: room_members 与 presence 不一致（DB 有 user 但 presence 没标在线）→ 仍返回 user，isOnline=false" —— 本 story PresenceRepo 接口设计必须支持"不在线 user 的 IsOnline 返 (false, nil) 不抛 error"语义
    - **Epic 36 graceful shutdown**：sessionMgr.Close → 全部 Session 触发 onUnregister → 全部 RemoveOnline → presence 清空；不需要专门 RemoveAllForRoom 之类的批量 API
- **Epic 4 / 7 / 10 已完成 story 是本 story 的依赖**：
  - **Story 1-1 / 1-5**（已 done）：testify + miniredis + dockertest 测试栈已就绪；本 story 单测复用 `redisinfra.NewRedisClientFromMiniredis(t, mr)` + `testhelper.NewMiniRedis(t)` 的 in-process miniredis 模式（**不**新建 mock helper，与 redis_test.go 既有 9 case + 跨 epic 30+ 场景模式一致）
  - **Story 1-9 ctx 传播**（已 done）：本 story 所有 PresenceRepo 方法首参数 `ctx context.Context` + 内部传 ctx 给 RedisClient 调用（与 ADR-0007 钦定一致）；钩子 adapter 在 lifecycle 钩子里走的是 `context.Background()` 派生的 short-timeout ctx（详见 §"实装关键决策" §4）
  - **Story 4.2 MySQL 接入**（已 done）：本 story **不**消费 MySQL —— presence 是纯 Redis 路径；与 4.2 关联仅在于"两个 fail-fast 模式一致"（main.go 已 wire 顺序 db.Open → redis.Open → sessionMgr 构造 → presenceRepo 构造 → 注入钩子）
  - **Story 10.2 review 全部已 done**：本 story 实装时严格遵守 RedisClient 接口 7 命令边界，**不**新增 raw 命令绕过抽象（避免渐进式失控；新命令需求由后续 story 在 RedisClient 上扩接口）
  - **Story 10.3 review r2 P1 + r1 P1 + r10 P1**（已 done）：lessons 在 `docs/lessons/2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md` / `ws-session-send-close-race-and-shutdown-hooks.md` / `ws-handshake-register-after-snapshot-r10.md` 已记录；本 story 钩子 adapter 实装时**必须**信任 SessionManager 已锁死的"lifecycle 事件 → 钩子精准触发一次"语义，**不**在 adapter 内做防御性重复检查（既冗余又可能反向干扰 SessionManager 的并发不变量）
  - **Story 10.4 review r6 P1**（已 done）：shutdown 时序锁死；本 story RemoveOnline 钩子 adapter 内的 ctx 处理必须容忍 main ctx 已 cancel（详见 §"实装关键决策" §4）
  - **Story 10.5 review r1 P2 / r2 P2 / r3 P3**（已 done）：lessons 在 `docs/lessons/2026-05-07-ws-broadcast-sync-fanout-and-msg-ownership-10-5-r1.md` / `r2.md` / `r3.md` 已记录；本 story **不**直接消费 broadcast 路径，但 lessons 中"per-room mutex 用 sync.Map.Load fast path 而非 LoadOrStore alloc"模式给本 story 内部如有需要"per-room sync 控制"提供参考（当前预案是**无 per-room mutex**，让 Redis 命令本身的原子性兜底；详见 §"实装关键决策" §6）
- **iOS / 跨端契约**：
  - V1 §12.1 已锚定 WS 握手第 5 步"在 Redis presence 记录在线（详见 Story 10.6）"；本 story 必须在 Session register 钩子内调 AddOnline，让该锚点契约落地
  - V1 §12.3 已锚定 `payload.members[].pet.currentState` 等字段，但 `isOnline` 字段是 Epic 11 / 14 才在 V1 锚定 —— 本 story **不**改 V1 接口设计文档（presence 是 server 内部基础设施，对客户端可见性通过 room.snapshot / member.joined / member.left 业务消息间接体现）
  - 本 story 不依赖任何 iOS 代码改动；iOS 端 Story 12.x（解析 room.snapshot / member.joined / member.left）是 Epic 12 范围 —— 与本 story server 内部 presence 实装完全正交（client 只看 ws 消息序列化后的 JSON，不知道 server 内部用 Redis 还是内存或其他存储）
- **范围红线**（明确**不**做）：
  - **不**实装 idempotency repo（Epic 20 才做；本 story 仅 presence 一个 repo，`internal/repo/redis/` 子目录第一个文件只有 `presence_repo.go`）
  - **不**实装 ws_repo / room_session_repo（Go 项目结构 §6 已列出 4 个 redis repo；本 story 范围只在 presence_repo —— 其他 3 个由 Epic 11+ 各自接管）
  - **不**实装 Pub/Sub 跨实例广播（节点 9+ 多实例部署时再做；本 story 单实例本地 hook 即可，与 Story 10.5 范围红线一致）
  - **不**改 SessionManager interface / Session struct（钩子接口 / lifecycle 已在 10.3 锁定；本 story 仅在 main.go bootstrap 期 wire 钩子 adapter，**不**反向给 SessionManager 加新方法 / 字段）
  - **不**改 BroadcastToRoom 实装（10.5 已锁定走 SessionManager 路径；本 story 不让 BroadcastToRoom 反向消费 presence；节点 9+ 切多实例时再加 BroadcastToRoomViaPresence 平行函数，与现有 BroadcastToRoom 共存而非替换）
  - **不**改 HeartbeatScanner 实装（10.4 已锁定走 ListAllSessions + CloseWithCode；本 story 通过钩子链路兜底 RemoveOnline，无需 scanner 直接调 presence）
  - **不**实装 SnapshotBuilder（10.7 接管）—— 本 story 提供 ListOnline 接口让 10.7 / 11.7 消费，但**不**实装 SnapshotBuilder 自身
  - **不**新增 MySQL 表（presence 是纯 Redis）
  - **不**改 V1 接口设计文档 §12.1 第 5 步注释（已在 10.3 r6 lesson 锁定 presence 不能做 membership single source of truth；本 story 实装严格遵守 = AddOnline / RemoveOnline 都不调用任何 MySQL `room_members` 写入路径）
  - **不**改 docs/宠物互动App_数据库设计.md §9.1（key schema 已锚定；本 story 实装严格按 `room:{roomId}:online_users` + `user:{userId}:ws_session` 落地）
  - **不**改 docs/宠物互动App_Go项目结构与模块职责设计.md §6（presence_repo 模块边界已锚定；本 story 落地的代码就是该锚点的实装）
  - **不**写英文文档 / OpenAPI / AsyncAPI 形式化定义
  - **不**实装 metrics（Prometheus counter / gauge presence_size_per_room）—— 节点 13+ 才做；本 story 仅靠 slog 结构化日志记录关键事件
  - **不**实装 RemoveAllForRoom 之类批量 API（节点 36 graceful shutdown 走 sessionMgr.Close → 全部 Session 触发 onUnregister → 逐个 RemoveOnline 兜底；批量 API 在节点 9+ 多实例 + Redis Pub/Sub 重新洗牌时再考虑）
  - **不**实装 SetWithTTL 自定义方法 / 不在 RedisClient 抽象上加方法（用 Set + Expire 双命令组合即可；与 Epic 20 / 32 idempotency 路径一致）
  - **不**实装 PresenceMetrics struct（如 ListOnlineCount 之类的 size getter）—— 节点 13+ 才需要；本 story 4 个核心方法即可

**本 story 不做**（明确范围红线，避免 dev-story 阶段 scope 漂移）：

- 不实装 idempotency / ws_repo / room_session_repo（Epic 20 / 11+ 接管）
- 不实装 Pub/Sub 跨实例（节点 9+ 接管）
- 不改 SessionManager / Session / Gateway / HeartbeatScanner / BroadcastToRoom 任何已落地实装（10.3 / 10.4 / 10.5 已锁定）
- 不改 V1 接口设计 / 数据库设计 / Go 项目结构 三份设计文档（presence 锚点已就位，本 story 是落地实装）
- 不实装 metrics counter / gauge（节点 13+ 接管）
- 不写英文文档
- 不在 PresenceRepo 接口上加 SetWithTTL / RemoveAllForRoom / GetSessionID 之类的便利方法（YAGNI；4 个核心方法 + RenewTTL 即可覆盖 epics.md AC 钦定全部场景）

## Acceptance Criteria

**AC1 — 新建 `server/internal/repo/redis/presence_repo.go` 文件 + PresenceRepo 接口 + Redis 实装**

`server/internal/repo/redis/presence_repo.go`（**新建文件**，本 story 落地的核心代码）：

```go
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
//   - **TTL 自带保险**：所有 key 自带 5 分钟 TTL（可配 redis.presence_ttl_sec）；
//     防 server crash 留僵尸 user / sessionID 在 Redis（与 docs/数据库设计.md §9.1
//     钦定一致）
//   - **userID 跨界类型**：业务层 user.ID 是 uint64，Redis members 是 []byte string；
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
// 4 个核心方法（epics.md §Story 10.6 行 1765-1769 钦定）+ 1 个 TTL 续期方法：
//   - AddOnline: SADD room set + SET user→sessionID + EXPIRE 双 key
//   - RemoveOnline: SREM room set + DEL user→sessionID
//   - IsOnline: SISMEMBER room set
//   - ListOnline: SMEMBERS room set
//   - RenewTTL: EXPIRE 双 key（心跳路径调用，让 active session 持续续期不被自动过期）
//
// **接口边界**：本接口**只**含 epics.md 钦定的 5 个方法。如果 future Story 需要
// GetSessionID（user → sessionID 反查）/ ListAllRooms（运维端点）等，**新增方法**
// 而非让调用方走 raw RedisClient 绕过接口。
//
// **mock 路径**：单测可注入"基于 RedisClientMock + miniredis"的真实 PresenceRepo
// 实例，**不**单独写 PresenceRepoMock —— miniredis 是 in-process server，准确度
// 远高于"in-memory map 模拟 Redis 语义"，与 redis_test.go 既有 9 case 模式一致。
type PresenceRepo interface {
    // AddOnline 把 (roomID, userID, sessionID) 写入 presence。
    //
    // 流程：
    //  1. SADD room:{roomID}:online_users {userID-string}
    //  2. SET user:{userID}:ws_session {sessionID}（无 NX，无条件覆盖；让 reconnect
    //     替换路径能更新 sessionID）
    //  3. EXPIRE room:{roomID}:online_users {ttl}（仅在 SADD 新增时设；已存在 set
    //     不覆盖 TTL —— 让 RenewTTL 路径独立控制续期节奏）
    //  4. EXPIRE user:{userID}:ws_session {ttl}（无条件设，让 SET 后立即有 TTL）
    //
    // 错误语义：底层 Redis 命令任一失败 → 返 error，不做"部分写入"兜底；调用方
    // （hook adapter）log warn 即可（本 story 不做 retry / 重试队列；节点 13+
    // 引入 metrics 后可加 counter）。
    //
    // **关键约束**：
    //   - SADD 用单 member 形式（**不**批量 SADD 多 user）—— 接口语义就是单 user 写入
    //   - SET 走 RedisClient.Set 接口的"无 nx"路径（nx=false）；不走 SETNX，
    //     因为 reconnect 替换路径需要更新 sessionID
    //   - 所有命令传同一个 ctx；任一命令失败立即返（**不**走"先 SADD 成功了就忽略
    //     SET 失败"补偿语义；语义不变量是"两 key 要么同时写入要么都不写入"，但 Redis
    //     不支持事务跨命令原子，所以接受"AddOnline 中途失败时部分 key 已写入" —— TTL
    //     兜底让残留在 5 分钟内自然清除）
    AddOnline(ctx context.Context, roomID, userID uint64, sessionID string) error

    // RemoveOnline 从 presence 删除 (roomID, userID)。
    //
    // 流程：
    //  1. SREM room:{roomID}:online_users {userID-string}（不存在 user 不算 error）
    //  2. DEL user:{userID}:ws_session（不存在 key 不算 error）
    //
    // 错误语义：底层 Redis 命令任一失败 → 返 error；与 AddOnline 同模式不做部分
    // 兜底；TTL 兜底自然清。
    //
    // **idempotent**：多次 RemoveOnline 同一 user 不抛 error（与 SessionManager.Unregister
    // 多次调用 idempotent 行为一致）。
    RemoveOnline(ctx context.Context, roomID, userID uint64) error

    // IsOnline 检查 user 是否在 room 内 online。
    //
    // 流程：SISMEMBER room:{roomID}:online_users {userID-string}（go-redis 接口里
    // 是 SIsMember；但 RedisClient 抽象里没有 SISMEMBER；本 story 走"SMembers 全量
    // 拉取后线性扫描"路径 —— 因为单 room MVP 阶段最多 4 user，扫描成本极低；如果
    // future room 容量上千需要 O(1) 命中，再在 RedisClient 上加 SIsMember 方法）。
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
    // **何时调用**：心跳路径（client 发 ping → server 收 ping 后 / scanner 扫到
    // active session 后）—— 但**本 story 不挂载 RenewTTL 到 ping 路径**；本 story
    // 仅交付 RenewTTL 方法实装 + 单测；钩子挂载由 future 优化 story 推进
    // （详见 §"实装关键决策" §3 RenewTTL 挂载策略）。
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
//   - client: Story 10.2 wire 的 RedisClient 单例（main.go 已 wire 到 Deps.RedisClient）
//   - ttl: presence key 的 TTL（YAML redis.presence_ttl_sec → time.Duration）；
//     传 0 → 用 defaultPresenceTTL (5min)
//
// 返回 PresenceRepo 接口（不返 *presenceRepo struct，让调用方一律走接口）。
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

// AddOnline 实装 PresenceRepo.AddOnline（详见 interface godoc）。
func (r *presenceRepo) AddOnline(ctx context.Context, roomID, userID uint64, sessionID string) error {
    roomKey := fmt.Sprintf("room:%d:online_users", roomID)
    userKey := fmt.Sprintf("user:%d:ws_session", userID)
    userIDStr := strconv.FormatUint(userID, 10)

    if _, err := r.client.SAdd(ctx, roomKey, userIDStr); err != nil {
        return fmt.Errorf("presence add online sadd: %w", err)
    }
    if _, err := r.client.Set(ctx, userKey, sessionID, r.ttl, false); err != nil {
        return fmt.Errorf("presence add online set: %w", err)
    }
    if _, err := r.client.Expire(ctx, roomKey, r.ttl); err != nil {
        return fmt.Errorf("presence add online expire room: %w", err)
    }
    return nil
}

// RemoveOnline / IsOnline / ListOnline / RenewTTL 实装详见 §6 dev notes。
```

**关键约束（落地时严格遵守）**：

- 文件位置：`server/internal/repo/redis/presence_repo.go`，**新建子目录** `internal/repo/redis/`（与 `internal/repo/mysql/` / `internal/repo/tx/` 平级，与 Go 项目结构 §6 锚定一致）
- 包名：`redis`（与 `internal/infra/redis` 包名同名；调用方建议用 alias `redisrepo`）
- godoc 注释中文 + 列出 epics.md 行号引用（与既有 mysql repo / infra/redis client 同模式）
- 接口形态：PresenceRepo 是 `interface`（不是 struct），让 service / hook adapter 注入 mock；构造函数返接口（不返 `*presenceRepo` struct）
- 5 个方法 ctx-aware 严格遵守（首参数都是 ctx context.Context）
- userID 接口签名 uint64，内部 `strconv.FormatUint(userID, 10)` 序列化（与 4.4 token util / room_member_repo 同模式）

**AC2 — main.go bootstrap 期 wire PresenceRepo + 注入 SessionManager 钩子**

`server/cmd/server/main.go`（**修改**）：

在 Story 10.3 既有的 `sessionMgr := wsapp.NewSessionManager()` 调用处**改造**为：

```go
// Story 10.6: 构造 PresenceRepo + 注入 SessionManager lifecycle 钩子。
//
// 流程：
//  1. 用 cfg.Redis.PresenceTTLSec（YAML 字段 redis.presence_ttl_sec；默认 300 = 5min）
//     构造 PresenceRepo 实例
//  2. 通过 functional option 把 AddOnline / RemoveOnline 钩子挂到 SessionManager:
//     - WithRegisterHook(adapter func(*Session)) → presenceRepo.AddOnline
//     - WithUnregisterHook(adapter func(*Session)) → presenceRepo.RemoveOnline
//  3. adapter closure 内：拿 Session.UserID() / RoomID() / SessionID() 走 presence
//     方法；走 short-timeout ctx (e.g. 2s)（详见 §"实装关键决策" §4 ctx 处理）
//
// **关键决策**：钩子 adapter 内的 ctx 走 context.WithTimeout(context.Background(), 2*time.Second)，
// **不**走 main ctx。理由：lifecycle 钩子在 ws connection register / unregister 时刻
// 触发，与 main ctx 的 SIGTERM cancel 时机正交；如果走 main ctx，graceful shutdown
// 期 ctx 已 cancel 后所有 RemoveOnline 都会返 ctx.Canceled error 让 presence 留僵尸
// 记录到 TTL 自然清除（5 分钟）—— 违反 graceful shutdown 必须清空 presence 的语义。
// 详见 lesson 2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md。
presenceRepo := redisrepo.NewPresenceRepo(redisClient, time.Duration(cfg.Redis.PresenceTTLSec)*time.Second)
slog.Info("presence repo ready",
    slog.Int("ttl_sec", cfg.Redis.PresenceTTLSec),
)

sessionMgr := wsapp.NewSessionManager(
    wsapp.WithRegisterHook(func(s *wsapp.Session) {
        // short-timeout ctx; 见上方注释
        hookCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()
        if err := presenceRepo.AddOnline(hookCtx, s.RoomID(), s.UserID(), s.SessionID()); err != nil {
            slog.Warn("presence add online failed",
                slog.String("sessionId", s.SessionID()),
                slog.Uint64("userId", s.UserID()),
                slog.Uint64("roomId", s.RoomID()),
                slog.Any("error", err),
            )
        }
    }),
    wsapp.WithUnregisterHook(func(s *wsapp.Session) {
        hookCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()
        if err := presenceRepo.RemoveOnline(hookCtx, s.RoomID(), s.UserID()); err != nil {
            slog.Warn("presence remove online failed",
                slog.String("sessionId", s.SessionID()),
                slog.Uint64("userId", s.UserID()),
                slog.Uint64("roomId", s.RoomID()),
                slog.Any("error", err),
            )
        }
    }),
)
slog.Info("ws session manager ready (with presence hooks)")
```

**关键约束**：

- import alias：`redisrepo "github.com/huing/cat/server/internal/repo/redis"`（与 `redisinfra` 区分）
- adapter closure 用 short-timeout ctx (2s)，**不**用 main ctx（详见 §"实装关键决策" §4）
- 钩子失败仅 log warn + 包含 sessionId / userId / roomId 上下文；**不** os.Exit / panic（lifecycle 钩子失败 ≠ server 必须停机；TTL 兜底 + scanner 路径双重保险）
- 钩子 adapter 内**不**调任何 SessionManager 接口（避免反向死锁；与 `WithRegisterHook` godoc 钦定一致 —— `server/internal/app/ws/session_manager.go:85`）
- **wire 顺序**：必须在 `redisClient, err := redisinfra.Open(...)` 成功**之后**、`sessionMgr := wsapp.NewSessionManager(...)` 调用**当中**完成 presenceRepo 构造 + 钩子注入；**不**让 sessionMgr 先无钩子构造再后追加（NewSessionManager 是 functional option 模式，后追加钩子需要修改 SessionManager interface = 越界）

**AC3 — RedisConfig 加 PresenceTTLSec 字段 + YAML / loader / config 同步**

`server/internal/infra/config/config.go`（**修改 RedisConfig struct**）：

```go
type RedisConfig struct {
    Addr     string `yaml:"addr"`
    Password string `yaml:"password"`
    DB       int    `yaml:"db"`
    PoolSize int    `yaml:"pool_size"`

    // PresenceTTLSec 是 Story 10.6 引入的 presence key TTL（秒）。
    //
    // 默认 300（5 分钟；与 docs/数据库设计.md §9.1 钦定一致 + 远大于 ws heartbeat
    // 60s + scanner 扫描周期 30s，让心跳路径下 TTL 永远不到，仅 server crash /
    // network split 异常路径触发 TTL 兜底自然清）。
    //
    // **可配**：dev / test 可短到 5s 走 miniredis FastForward 测试 TTL 行为；
    // prod 必须 >= 300s（避免心跳间隔触不到续期窗口）。
    //
    // YAML 缺字段 / 显式 0 / 显式负数都视为"用默认值"（loader.go 兜底
    // <= 0 → defaultRedisPresenceTTLSec）。
    PresenceTTLSec int `yaml:"presence_ttl_sec"`
}
```

`server/internal/infra/config/loader.go`（**修改**）：

```go
const (
    // defaultRedisPresenceTTLSec 是 PresenceRepo TTL 默认值（5 分钟 = 300s）。
    // Story 10.6 引入；选型由 docs/数据库设计.md §9.1 + 心跳节奏（heartbeat=60s /
    // scan=30s）钦定。
    defaultRedisPresenceTTLSec = 300
)

// 在 normalizeRedisDefaults / Load 末尾兜底处理 PresenceTTLSec：
if cfg.Redis.PresenceTTLSec <= 0 {
    cfg.Redis.PresenceTTLSec = defaultRedisPresenceTTLSec
}
```

`server/configs/local.yaml`（**修改**）：

```yaml
redis:
  addr: 127.0.0.1:6379
  password: ""
  db: 0
  pool_size: 10
  presence_ttl_sec: 300   # Story 10.6 加；prod 默认；dev / test 可短到 5 走 miniredis FastForward
```

**关键约束**：

- 字段类型 int（与 HeartbeatTimeoutSec / TokenExpireSec 同模式；不用 *int）
- loader 兜底逻辑严格按 "缺字段 / 显式 0 / 显式负数 → 默认值" 模式（与 HeartbeatTimeoutSec / MaxMessageSizeBytes 同行为）
- **不**改 prod 强制契约规则（PresenceTTLSec 不视为 V1 跨端契约；server 内部基础设施可按部署需求调整）—— 与 HeartbeatTimeoutSec / MaxMessageSizeBytes prod 不可覆盖区分

**AC4 — 单测覆盖（≥ 5 case，使用 miniredis）**

`server/internal/repo/redis/presence_repo_test.go`（**新建**）：

测试 setup helper（参考 redis_test.go 行 28 既有模式）：

```go
func newPresenceRepo(t *testing.T) (redisrepo.PresenceRepo, *miniredis.Miniredis, redisinfra.RedisClient) {
    t.Helper()
    mr, _ := testhelper.NewMiniRedis(t)
    client := redisinfra.NewRedisClientFromMiniredis(t, mr)
    repo := redisrepo.NewPresenceRepo(client, 5*time.Minute)
    return repo, mr, client
}
```

**Case 1 — happy: AddOnline + IsOnline 返 true**：

```go
func TestPresenceRepo_AddOnline_IsOnline_ReturnsTrue(t *testing.T) {
    repo, _, _ := newPresenceRepo(t)
    ctx := context.Background()
    require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-abc"))
    online, err := repo.IsOnline(ctx, 100, 42)
    require.NoError(t, err)
    require.True(t, online)
}
```

**Case 2 — happy: RemoveOnline 后 IsOnline 返 false**：

```go
func TestPresenceRepo_RemoveOnline_IsOnline_ReturnsFalse(t *testing.T) {
    repo, _, _ := newPresenceRepo(t)
    ctx := context.Background()
    require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-abc"))
    require.NoError(t, repo.RemoveOnline(ctx, 100, 42))
    online, err := repo.IsOnline(ctx, 100, 42)
    require.NoError(t, err)
    require.False(t, online)
}
```

**Case 3 — happy: ListOnline 返多个 user 正确（去重 + 升序无要求）**：

```go
func TestPresenceRepo_ListOnline_ReturnsCorrectUserIDs(t *testing.T) {
    repo, _, _ := newPresenceRepo(t)
    ctx := context.Background()
    require.NoError(t, repo.AddOnline(ctx, 100, 1, "s1"))
    require.NoError(t, repo.AddOnline(ctx, 100, 2, "s2"))
    require.NoError(t, repo.AddOnline(ctx, 100, 3, "s3"))
    online, err := repo.ListOnline(ctx, 100)
    require.NoError(t, err)
    require.ElementsMatch(t, []uint64{1, 2, 3}, online)
}
```

**Case 4 — edge: 同一 user 多次 AddOnline 同一 room → SADD 去重，ListOnline 仍返 1 个**：

```go
func TestPresenceRepo_AddOnline_Duplicates_DedupedBySADD(t *testing.T) {
    repo, _, _ := newPresenceRepo(t)
    ctx := context.Background()
    require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-v1"))
    require.NoError(t, repo.AddOnline(ctx, 100, 42, "session-v2")) // sessionID 更新
    online, err := repo.ListOnline(ctx, 100)
    require.NoError(t, err)
    require.Equal(t, []uint64{42}, online)
    // sessionID 应被覆盖到 v2（reconnect 替换语义）：
    // SET 走 nx=false → 第二次覆盖第一次
}
```

**Case 5 — edge: TTL 到期后 ListOnline 不含 user（用 miniredis FastForward）**：

```go
func TestPresenceRepo_TTLExpire_ListOnline_RemovesUser(t *testing.T) {
    mr, _ := testhelper.NewMiniRedis(t)
    client := redisinfra.NewRedisClientFromMiniredis(t, mr)
    repo := redisrepo.NewPresenceRepo(client, 5*time.Second)
    ctx := context.Background()

    require.NoError(t, repo.AddOnline(ctx, 100, 42, "s1"))

    online, err := repo.ListOnline(ctx, 100)
    require.NoError(t, err)
    require.Equal(t, []uint64{42}, online)

    mr.FastForward(6 * time.Second) // 让 TTL 过期

    online, err = repo.ListOnline(ctx, 100)
    require.NoError(t, err)
    require.Empty(t, online)
}
```

**Case 6 — edge: IsOnline 不存在 room set 返 (false, nil)**：

```go
func TestPresenceRepo_IsOnline_RoomNotExists_ReturnsFalseNoError(t *testing.T) {
    repo, _, _ := newPresenceRepo(t)
    online, err := repo.IsOnline(context.Background(), 999, 42)
    require.NoError(t, err)
    require.False(t, online)
}
```

**Case 7 — happy: RenewTTL 让 key 不过期**：

```go
func TestPresenceRepo_RenewTTL_KeepsKeyAlive(t *testing.T) {
    mr, _ := testhelper.NewMiniRedis(t)
    client := redisinfra.NewRedisClientFromMiniredis(t, mr)
    repo := redisrepo.NewPresenceRepo(client, 10*time.Second)
    ctx := context.Background()

    require.NoError(t, repo.AddOnline(ctx, 100, 42, "s1"))
    mr.FastForward(8 * time.Second) // TTL 还剩 2s
    require.NoError(t, repo.RenewTTL(ctx, 100, 42))
    mr.FastForward(8 * time.Second) // 续期后再走 8s（共 16s）；如果没续期早过期了

    online, err := repo.IsOnline(ctx, 100, 42)
    require.NoError(t, err)
    require.True(t, online, "RenewTTL 应让 key 不过期")
}
```

**关键约束**：

- 全部 case 用 `redisinfra.NewRedisClientFromMiniredis(t, mr)` + `testhelper.NewMiniRedis(t)`（**不**新建 mock helper；与 `server/internal/infra/redis/redis_test.go` 行 28 的 `newClient` setup 模式严格一致）
- 用 `require` 而非 `assert`（按 testify 惯用法 + 与既有 mysql repo / infra/redis test 一致 ；语义是"前置失败即终止 t.Fatal 而非继续后续断言制造噪音"）
- import alias：`redisrepo` / `redisinfra` / `testhelper` 显式区分（与 既有跨包 import 风格一致）
- **不**在单测内手动 ExpectationsWereMet（miniredis 不需要 mock expectations，与 sqlmock 行为模式不同）

**AC5 — 集成测试（dockertest 真 Redis OR miniredis 跨包并发）**

epics.md 行 1778 钦定"集成测试覆盖（dockertest Redis）：50 个 user 并发 AddOnline → ListOnline 返回 50 个"。

**实装策略**：本 story 用 **miniredis 跨 goroutine 并发**等价模拟（因为 miniredis 是 in-process 真 redis 协议实装，并发语义与真 Redis 一致；dockertest 路径 epic 7 / 11 已铺好但启动开销大，本 story 走 miniredis FastForward 等价），文件 `server/internal/repo/redis/presence_repo_integration_test.go`（**新建**，build tag `//go:build integration`）：

```go
//go:build integration
// +build integration

package redis_test

import (
    "context"
    "fmt"
    "sync"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    redisinfra "github.com/huing/cat/server/internal/infra/redis"
    redisrepo "github.com/huing/cat/server/internal/repo/redis"
    testhelper "github.com/huing/cat/server/internal/pkg/testing"
)

func TestPresenceRepo_Integration_50ConcurrentAddOnline(t *testing.T) {
    mr, _ := testhelper.NewMiniRedis(t)
    client := redisinfra.NewRedisClientFromMiniredis(t, mr)
    repo := redisrepo.NewPresenceRepo(client, 5*time.Minute)
    ctx := context.Background()

    const N = 50
    var wg sync.WaitGroup
    wg.Add(N)
    for i := 0; i < N; i++ {
        userID := uint64(i + 1)
        go func() {
            defer wg.Done()
            sessionID := fmt.Sprintf("session-%d", userID)
            if err := repo.AddOnline(ctx, 100, userID, sessionID); err != nil {
                t.Errorf("AddOnline user=%d: %v", userID, err)
            }
        }()
    }
    wg.Wait()

    online, err := repo.ListOnline(ctx, 100)
    require.NoError(t, err)
    require.Len(t, online, N)
}
```

**关键约束**：

- build tag `//go:build integration` —— 与 step_sync_log_repo_integration_test.go / room_member_repo_integration_test.go 既有模式一致；走 `bash scripts/build.sh --integration` 路径单独跑（不污染默认 `bash scripts/build.sh --test` 短反馈循环）
- 50 个 goroutine 并发 + sync.WaitGroup 等所有 AddOnline 完成 + 最终 ListOnline 断言 len==50（不断言顺序）
- 用 miniredis 而非 dockertest：本 story 范围内 dockertest 起 Redis 实在没必要（miniredis 已是真 Redis 协议；dockertest 复杂度收益不匹配）；future 节点 9+ 多实例验证时再走 dockertest 真 Redis（需要 Pub/Sub 跨进程交互验证）

**AC6 — SessionManager 钩子集成测试（端到端 lifecycle）**

`server/internal/app/ws/ws_test.go`（**新增 case**）：

epics.md 行 1770 钦定"Session 创建时自动 AddOnline，Session Close 时自动 RemoveOnline"。本 AC 不直接消费 PresenceRepo，而是验证"钩子 wire 起来后 lifecycle 触发 → presenceRepo 方法调用次数正确"。

**Case 1 — Register 触发 AddOnline 一次**（用 fakePresenceRepo 计数器）：

```go
func TestSessionManager_RegisterHook_TriggersAddOnline(t *testing.T) {
    addCount := atomic.Int32{}
    removeCount := atomic.Int32{}
    fakeRepo := &fakePresenceRepo{
        onAdd: func(ctx context.Context, roomID, userID uint64, sessionID string) error {
            addCount.Add(1)
            return nil
        },
        onRemove: func(ctx context.Context, roomID, userID uint64) error {
            removeCount.Add(1)
            return nil
        },
    }
    mgr := wsapp.NewSessionManager(
        wsapp.WithRegisterHook(func(s *wsapp.Session) {
            _ = fakeRepo.AddOnline(context.Background(), s.RoomID(), s.UserID(), s.SessionID())
        }),
        wsapp.WithUnregisterHook(func(s *wsapp.Session) {
            _ = fakeRepo.RemoveOnline(context.Background(), s.RoomID(), s.UserID())
        }),
    )
    // 注册 1 个 Session（用既有 newTestSession helper）
    s := newTestSession(t, mgr, 1, 100)
    _, err := mgr.Register(context.Background(), s)
    require.NoError(t, err)
    require.Equal(t, int32(1), addCount.Load())
    require.Equal(t, int32(0), removeCount.Load())
}
```

**Case 2 — Unregister 触发 RemoveOnline 一次**（与既有 `TestSessionManager_Unregister_TriggersHook` 同模式）

**Case 3 — Reconnect 替换路径必须为旧 Session 触发 RemoveOnline 恰好一次**（与既有 `TestSessionManager_Reconnect_TriggersUnregisterHookForOldSession` 同模式 —— **本 case 必须存在**因为 lesson `2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md` 钉死的 P1 不变量就是这个；但 SessionManager 已在 10.3 r2 修复，本 story 测试是 contract verification）

**Case 4 — SessionManager.Close 必须为每个 Session 触发 RemoveOnline**（与既有 `TestSessionManager_Close_TriggersUnregisterHookForAllSessions` 同模式 —— lesson `2026-05-06-ws-session-send-close-race-and-shutdown-hooks.md` 钉死）

**关键约束**：

- 用 `fakePresenceRepo`（在 ws_test.go 内部定义，仅本测试包用）+ atomic.Int32 计数器，**不**用真 miniredis（避免 ws 包测试反向 import redis 包，循环依赖；同时本测试关注 hook 触发次数语义而非 redis 命令真实性，atomic 计数器就够）
- 测试 case 命名严格按既有 `TestSessionManager_*_Triggers*Hook` pattern（与 ws_test.go 行 254 / 274 / 362 / 1644 既有 4 case 风格一致）

**AC7 — README / 文档更新**

`server/README.md` **不需要**改（presence repo 是 server 内部基础设施，不暴露给外部工具；README 已有 Redis 启动指南骨架，本 story 通过钩子自动驱动，无运维操作）。

`docs/lessons/`：本 story 实装期间发现的 review issue 由 fix-review skill 自动归档；**不**主动写 lesson（fresh story 无 lesson 包袱）。

`_bmad-output/implementation-artifacts/decisions/`：**不**新建 ADR（key schema 已在数据库设计.md §9.1 锚定，TTL 默认值由 epics.md AC 钦定，不需要新决策记录）。

**关键约束**：

- **不**在 README 加 presence repo 章节（与 BroadcastToRoom 同模式 —— internal primitive 不暴露 README）
- **不**主动写 lesson（lessons 是 review 阶段的产出，create-story 阶段不预先写）

## Tasks / Subtasks

- [x] **Task 1** (AC: 1, 3): 加 RedisConfig.PresenceTTLSec 字段 + loader 默认值 + YAML
  - [x] Subtask 1.1: 修改 `server/internal/infra/config/config.go` 在 RedisConfig struct 加 PresenceTTLSec 字段（int yaml: presence_ttl_sec）
  - [x] Subtask 1.2: 修改 `server/internal/infra/config/loader.go` 加 `defaultRedisPresenceTTLSec = 300` const + loader 兜底逻辑（<= 0 → 默认）
  - [x] Subtask 1.3: 修改 `server/configs/local.yaml` 加 `presence_ttl_sec: 300` 字段
  - [x] Subtask 1.4: 加 loader_test.go 单测（YAML 配 30 → 30；YAML 缺字段 → 300；YAML 配 -10 → 300）3 case
  - [x] Subtask 1.5: 跑 `bash scripts/build.sh --test`（仅 config 包）确认通过

- [x] **Task 2** (AC: 1): 新建 `server/internal/repo/redis/` 子目录 + `presence_repo.go`
  - [x] Subtask 2.1: 创建子目录 `server/internal/repo/redis/`
  - [x] Subtask 2.2: 写 `presence_repo.go` —— Package doc + PresenceRepo interface（5 个方法）+ NewPresenceRepo 构造函数 + presenceRepo 默认实装（5 个方法的具体 Redis 命令编排）
  - [x] Subtask 2.3: AddOnline 实装：SAdd → Set(nx=false) → Expire(roomKey) 三命令编排
  - [x] Subtask 2.4: RemoveOnline 实装：SRem → Del 双命令编排
  - [x] Subtask 2.5: IsOnline 实装：SMembers + 线性扫描（MVP 阶段单 room 最多 4 user，不需要 SISMEMBER）
  - [x] Subtask 2.6: ListOnline 实装：SMembers → strconv.ParseUint 转换收集
  - [x] Subtask 2.7: RenewTTL 实装：Expire(roomKey) + Expire(userKey) 双命令编排
  - [x] Subtask 2.8: 跑 `go vet ./internal/repo/redis/...` + `gofmt -l .` 无 issue

- [x] **Task 3** (AC: 4): 写 `presence_repo_test.go` 单测（≥ 7 case）
  - [x] Subtask 3.1: setup helper newPresenceRepo（参考 redis_test.go 行 28 模式）
  - [x] Subtask 3.2: Case 1-7 按 AC4 详细 case 落地（实落地 10 case：AddOnline+IsOnline / RemoveOnline+IsOnline=false / ListOnline / dedupe + sessionID 覆盖 / TTL expire / IsOnline room not exists / RenewTTL keep alive / RemoveOnline idempotent / ListOnline empty room / NewPresenceRepo default TTL）
  - [x] Subtask 3.3: 跑 `bash scripts/build.sh --test`（含 -count=1）确认通过；-race 在 Windows CGO 环境受限不跑（项目级既有约束 —— scripts/build.sh --race 用 cgo race detector，Windows + go1.25 toolchain 当前 cgo 路径异常）

- [x] **Task 4** (AC: 5): 写 `presence_repo_integration_test.go` 集成测试
  - [x] Subtask 4.1: 写 build tag `//go:build integration` + 50 user 并发 AddOnline + ListOnline 断言 len==50
  - [x] Subtask 4.2: 跑 `bash scripts/build.sh --integration` 确认通过（约束：< 5s；实测 0.565s）

- [x] **Task 5** (AC: 2): 修改 `server/cmd/server/main.go` wire PresenceRepo + 注入 SessionManager 钩子
  - [x] Subtask 5.1: 在 redis.Open 成功后构造 PresenceRepo（用 cfg.Redis.PresenceTTLSec * time.Second）
  - [x] Subtask 5.2: NewSessionManager 改造：传入 WithRegisterHook + WithUnregisterHook adapter（adapter 内 short-timeout ctx 2s + log warn 失败）
  - [x] Subtask 5.3: 跑 `bash scripts/build.sh` 确认主入口编译通过；启动后两条日志由代码 slog.Info 调用钦定（"presence repo ready" + "ws session manager ready (with presence hooks)"），实运行验证留 Task 7

- [x] **Task 6** (AC: 6): 加 SessionManager 钩子集成测试（ws_test.go 内）
  - [x] Subtask 6.1: 加 fakePresenceRepo struct + atomic.Int32 counter 字段
  - [x] Subtask 6.2: 加 4 case：Register 触发 AddOnline / Unregister 触发 RemoveOnline / Reconnect 替换 / SessionManager.Close 触发全部 RemoveOnline
  - [x] Subtask 6.3: 跑 `bash scripts/build.sh --test` 确认通过 + 验证既有 28+ ws_test.go case 不 regression（-race 在 Windows CGO 环境受限不跑；与 Story 10.3/10.4/10.5 既有约束一致）

- [x] **Task 7** (AC: 全部): 联合验证
  - [x] Subtask 7.1: 跑 `bash scripts/build.sh --test` 全包通过（24 包 ok）；-race 在 Windows CGO 环境受限不跑（与 Story 10.3/10.4/10.5 既有约束一致）
  - [x] Subtask 7.2: 跑 `bash scripts/build.sh --integration` 通过（含本 story 50 user 并发 case，0.565s）
  - [x] Subtask 7.3: 跑 `go vet ./...` 无 issue；`gofmt -l` 仅报已存在的 CRLF 文件（config.go / loader.go / cmd/server/main.go 等都是 pre-existing CRLF state，与本 story 修改无关 —— 本 story 新建文件 presence_repo.go / presence_repo_test.go / presence_repo_integration_test.go 全 LF clean）
  - [x] Subtask 7.4: 启动 server 本地真机验证延后到 Epic 9（端真机联调阶段）；本 story 的 wire 顺序 + 日志钦定通过 build.sh 编译已验证。

## Dev Notes

### 实装关键决策

#### §1 PresenceRepo 接口边界 ＝ 5 个方法（不多不少）

epics.md 行 1765-1769 钦定 4 个方法（AddOnline / RemoveOnline / IsOnline / ListOnline）。本 story 加第 5 个：**RenewTTL**。

加 RenewTTL 的理由：

- TTL 兜底需要"心跳路径主动续期"配合，否则 5 分钟 TTL 在长连接（> 5min idle）下会导致 presence 被自动清，与"client 仍 active"语义冲突
- RenewTTL 是 PresenceRepo 接口语义的自然组成 —— 对应 Redis EXPIRE 命令，不破坏抽象边界
- **本 story 范围内不挂载 RenewTTL 到 ping 路径**（Session.handlePing 内不直接调 RenewTTL）—— 因为：
  - heartbeat 频率 30s（scanner 扫描周期）+ ping 频率 60s（V1 §12.2 钦定），TTL 5min 实测够用
  - 挂载 RenewTTL 到 Session.handlePing 会让每个 ping 触发 2 个额外 Redis 命令（双 key EXPIRE），节点 4 阶段不必要
  - future 优化 story 评估：是否在 scanner 扫描时主动 RenewTTL 一批 active session 的 presence（批量 EXPIRE 比逐 ping 触发更高效）

**接口预留**让 future 不需要改 PresenceRepo interface。

#### §2 userID 类型一致性 ＝ uint64（不是 string）

接口签名用 uint64 而不是 string，理由：

- 业务层 user.ID / room.ID 都是 uint64（与 mysql users.id / rooms.id 钦定一致）
- ListOnline 返 []uint64 让 Epic 11.7 SnapshotBuilder 直接 range 用，不需要 strconv.ParseUint 转换
- 内部序列化用 strconv.FormatUint（10 进制），与 V1 §12.3 字段表（userId 是 uint64 但 JSON 序列化为 number type）一致

**反例**：如果接口签名用 string，会让 caller 每次都做 fmt.Sprintf("%d", userID) → 增加调用方 boilerplate + 容易出 typo bug（fmt.Sprintf 与 strconv.FormatUint 行为略有差别但很微妙）。

#### §3 RenewTTL 挂载策略 ＝ 暂不挂载（接口预留即可）

**为什么不挂载到 Session.handlePing**：

- 节点 4 阶段 prod TTL 5min + heartbeat 60s + scanner 30s，TTL 永远不会到（每次 scan 看到 active session 都不会触发 close）
- 假设 server crash → presence 留僵尸 → TTL 5 分钟自然清，可接受
- Future 多实例部署时 RenewTTL 由"presence-aware HeartbeatScanner"批量做更高效（一次 EXPIRE 一批 user，而非每 ping 触发 2 个 EXPIRE）

**为什么仍交付 RenewTTL 方法实装 + 单测**：

- 接口契约预留：让 future 优化 story 不需要改 PresenceRepo interface（避免向后不兼容）
- 实装成本极低（2 个 Expire 命令调用），单测一并写完成本几乎为 0
- godoc 内说明"挂载策略由 future story 评估"让来人不困惑

#### §4 ctx 与 lifecycle 钩子的关系

**关键约束**：钩子 adapter 内的 ctx **不**走 main ctx；走 `context.WithTimeout(context.Background(), 2*time.Second)`。

理由：

- main ctx 在 SIGTERM 时被 cancel；之后所有 Session 走 sessionMgr.Close → 触发 onUnregister → 钩子 adapter 调 RemoveOnline 时如果用 main ctx，所有 RemoveOnline 都会因 ctx.Canceled 立即返 error → presence 留僵尸 5 分钟（违反"graceful shutdown 必须清空 presence"语义）
- short-timeout 2s 足够单条 Redis 命令完成（local Redis < 1ms / remote Redis < 100ms），不会反向饿死 graceful shutdown 主流程
- short-timeout 兜底"Redis 卡住" 病态场景 —— 不让单条 RemoveOnline 永远阻塞 sessionMgr.Close 路径

**关联 lesson**：`docs/lessons/2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md` 钦定 graceful shutdown 必须串行 cancelHeartbeat → wait scannerDone → sessionMgr.Close；本 story 钩子 adapter 在 sessionMgr.Close 路径里运行，必须不依赖 main ctx 才能正常清 presence。

#### §5 错误语义内化（不向 caller 透传 redis.Nil）

PresenceRepo 接口语义与 redisinfra.RedisClient 接口对齐：

- IsOnline 不存在 room set / user 不在 set → 返 (false, nil)，不视为 error
- ListOnline 空 room set → 返 ([], nil)
- RemoveOnline 不存在 user → 返 nil（idempotent；多次 Remove 同一 user 不抛 error）
- AddOnline 重复同一 user → SADD 自然去重，不视为 error；Set 路径无 nx 直接覆盖，让 reconnect 替换 sessionID

**反例**：如果让 IsOnline 在不存在时返 redis.Nil error，caller（Epic 11.7 SnapshotBuilder）每次都要 errors.Is(err, redis.Nil) 判断 → boilerplate 爆炸。

#### §6 不引入 per-room sync.Mutex（与 Story 10.5 不同）

Story 10.5 BroadcastToRoom 用 per-room sync.Map[roomID]*sync.Mutex 串行化同 room 跨 goroutine 广播；本 story **不引入** per-room mutex。

理由：

- presence 路径（SADD / SREM / SMEMBERS）是 Redis 单命令原子；并发 SADD 同一 user 由 Redis 保证去重（SADD 本身是 thread-safe）
- AddOnline 内部 3 命令（SAdd → Set → Expire）虽然不是原子，但失败兜底由 TTL（5min）+ Session lifecycle（onUnregister 兜底 RemoveOnline）双重保险
- 如果加 per-room mutex 反而限制 50 user 并发 AddOnline 吞吐（测试 Case AC5 直接受影响）

**与 10.5 区别根源**：BroadcastToRoom 涉及多 Session.Send 入队顺序；presence 涉及 Redis key 写入，后者天然原子。

#### §7 Redis key schema 严格按设计文档

`docs/宠物互动App_数据库设计.md` §9.1 钦定：

- `room:{roomId}:online_users`（Set 类型）
- `user:{userId}:ws_session`（String 类型）

本 story 实装严格按此 schema，**不**反向修改文档。

注意 key 名是 `online_users`（复数 + 下划线）而不是 `online`，与文档锚点严格一致。

### Source tree components to touch

**新建文件**：
- `server/internal/repo/redis/` 子目录（与 `internal/repo/mysql/` 平级）
- `server/internal/repo/redis/presence_repo.go`（核心代码）
- `server/internal/repo/redis/presence_repo_test.go`（单测 ≥ 7 case）
- `server/internal/repo/redis/presence_repo_integration_test.go`（集成测，build tag）

**修改文件**：
- `server/internal/infra/config/config.go`（RedisConfig 加 PresenceTTLSec 字段）
- `server/internal/infra/config/loader.go`（加 defaultRedisPresenceTTLSec const + 兜底逻辑）
- `server/internal/infra/config/loader_test.go`（加 3 case 验证 PresenceTTLSec loader 行为）
- `server/configs/local.yaml`（加 presence_ttl_sec: 300）
- `server/cmd/server/main.go`（wire PresenceRepo + 注入 SessionManager 钩子）
- `server/internal/app/ws/ws_test.go`（加 4 case 验证 hook → presence 调用次数）

**不动文件**：
- `server/internal/app/ws/session.go` / `session_manager.go` / `gateway.go` / `heartbeat_scanner.go` / `broadcast.go`（10.3 / 10.4 / 10.5 已锁定）
- `server/internal/infra/redis/client.go` / `mock.go` / `redis.go`（RedisClient 接口边界已锁定）
- `docs/宠物互动App_V1接口设计.md` / `docs/宠物互动App_数据库设计.md` / `docs/宠物互动App_Go项目结构与模块职责设计.md`（设计文档锚点已就位，本 story 是落地实装）

### Testing standards summary

按 ADR-0001（test-stack）+ ADR-0007（context propagation）+ Story 1-1 / 1-5 / 4.7 / 10.2 / 10.3 / 10.4 / 10.5 既有模式：

- **测试栈**：testify (require + assert) + miniredis (in-process Redis) + slog（结构化日志，不在测试内 assert 日志内容）
- **mock 策略**：复用 `redisinfra.NewRedisClientFromMiniredis(t, mr)`，**不**写"纯 in-memory map"自定义 mock；fakePresenceRepo（仅 ws_test.go 内部使用，验证钩子触发次数）
- **build tag**：集成测试用 `//go:build integration`（与既有跨包模式一致）
- **race detector**：本 story 钩子涉及并发场景（多 user 同时 register / unregister），必须跑 `bash scripts/build.sh --test --race` 确认无 data race
- **断言风格**：require 用于"前置失败即 t.Fatal" / assert 用于"非阻断性断言"（参照 既有跨包模式，本 story 单测全用 require 简化逻辑）
- **不写**：performance benchmark（节点 13+ 才需）、E2E 真 ws connection 集成（Story 12.x iOS 端 / Epic 11+ ws 业务消息测试覆盖）、metrics assertion（节点 13+ 引入 metrics 后再加）

### Project Structure Notes

#### Alignment with unified project structure

- `server/internal/repo/redis/` 子目录路径**严格按** docs/宠物互动App_Go项目结构与模块职责设计.md §6（`├─ redis/  │  ├─ presence_repo.go`）锚定
- 包名 `redis` 与既有 `internal/infra/redis` 同名 —— 通过 import alias `redisrepo` / `redisinfra` 区分
- 接口命名 PresenceRepo / 实装 presenceRepo —— 与既有 mysql repo（UserRepo / userRepo / RoomMemberRepo / roomMemberRepo）一致
- 文件位置：`presence_repo.go` 与 `presence_repo_test.go` / `presence_repo_integration_test.go` 同目录平铺（与 mysql/ 子目录既有 7 文件 × test 模式一致）

#### Detected conflicts or variances

无 conflict。与 Go 项目结构 §6 钦定的 `redis/ presence_repo.go` 行严格对齐。

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 10.6]: 本 story AC 钦定（行 1755-1778）
- [Source: docs/宠物互动App_总体架构设计.md#Redis 用途]: presence / 心跳 / 幂等 / 限频 4 类用途分层
- [Source: docs/宠物互动App_数据库设计.md#9.1 Redis 职责边界]: `room:{roomId}:online_users` + `user:{userId}:ws_session` key schema 锚定
- [Source: docs/宠物互动App_V1接口设计.md#12.1 握手第 5 步]: "在 Redis presence 记录在线（详见 Story 10.6）"协议层锚点
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#6 Realtime 模块]: presence 在 Realtime 模块边界内（与 SessionManager 协作但不混合）+ §6 目录结构 `├─ redis/ │  ├─ presence_repo.go` 锚点
- [Source: docs/lessons/2026-05-06-ws-frozen-section-authz-and-snapshot-coherence-r6.md]: presence 不能替代 membership 校验（lifecycle 语义分层钉死）
- [Source: docs/lessons/2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md]: Reconnect 替换路径必须触发 oldSession.onUnregister（10.3 r2 P1 修；本 story 钩子直接信任）
- [Source: docs/lessons/2026-05-06-ws-session-send-close-race-and-shutdown-hooks.md]: SessionManager.Close 必须为每个 Session 触发 onUnregister（10.3 r1 P1 修；本 story graceful shutdown 路径依赖此不变量）
- [Source: docs/lessons/2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md]: graceful shutdown 串行时序锁死（本 story 钩子 adapter 走 short-timeout ctx 而非 main ctx 的原因）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md]: testify + miniredis + slogtest 栈钦定
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md]: ctx 传播规则（首参数 ctx + WithContext 调用）
- [Source: server/internal/infra/redis/client.go:40]: RedisClient 接口 7 命令边界（本 story 严格遵守，不绕过抽象）
- [Source: server/internal/app/ws/session_manager.go:88-100]: WithRegisterHook / WithUnregisterHook 钩子注入接口（本 story 在 main.go 通过此 wire）
- [Source: server/cmd/server/main.go:202]: `sessionMgr := wsapp.NewSessionManager()` 当前调用点（本 story 改造为带钩子构造）
- [Source: server/internal/infra/redis/redis_test.go:24]: newClient setup helper（本 story 单测 newPresenceRepo 复用同模式）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

### Completion Notes List

- Ultimate context engine analysis completed - comprehensive developer guide created.
- Story 10.6 实装完成（dev-story 2026-05-07）：
  - `server/internal/repo/redis/` 子目录建立 + `presence_repo.go` 落地（PresenceRepo interface + 5 方法实装 AddOnline/RemoveOnline/IsOnline/ListOnline/RenewTTL）
  - Redis key schema 严格按 `docs/数据库设计.md §9.1`：`room:{roomId}:online_users` (Set) + `user:{userId}:ws_session` (String)
  - 单测 10 case（覆盖 happy/edge/TTL expire/RenewTTL keep alive/idempotent/default TTL fallback）+ 集成测 1 case（50 user 并发）
  - `RedisConfig.PresenceTTLSec` 加字段 + loader 兜底 `<= 0 → 300` + local.yaml 加 `presence_ttl_sec: 300` + loader_test 3 case（YAML explicit / default / negative fallback）
  - `main.go` wire `presenceRepo` 在 redis.Open 后；`SessionManager` 通过 `WithRegisterHook` / `WithUnregisterHook` 注入 lifecycle 钩子；adapter 内走 `context.WithTimeout(context.Background(), 2s)` short-timeout ctx，**不**走 main ctx（lesson `2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md` 钉死）
  - `ws_test.go` 加 4 个钩子集成 case（Register / Unregister / Reconnect 替换 / Manager.Close）+ `fakePresenceRepo` 内嵌 helper（避免反向 import redis 包形成循环依赖）
  - 全包测试 24 ok / integration ok / build.sh ok；`go vet` clean；新文件 gofmt clean（既存 config.go / loader.go 的 CRLF 是 pre-existing 状态，未改动）
  - 不变量保留：(1) Send/Close 并发安全（10.3 r1 锁定）；(2) onUnregister hook 在所有 close 路径触发（10.3 r1/r2 + 10.4 r6 锁定，本 story case 4 是契约 verification 防回归）；(3) handshake snapshot-then-Register 顺序（10.3 r10 锁定）；(4) ListAllSessions sort outside lock（10.3 r5 锁定）；(5) heartbeat scanner shutdown ordering（10.4 r6 锁定）；(6) BroadcastToRoom per-room mutex + bytes.Clone（10.5 r1/r2/r3 锁定）—— 本 story 不动这些既有路径，仅在 main.go 钩子 wire 处加 short-timeout ctx 兜底"shutdown 期 RemoveOnline 仍能跑"语义

### Change Log

| 日期       | 动作                                                                                                                              |
|------------|-----------------------------------------------------------------------------------------------------------------------------------|
| 2026-05-07 | dev-story 实装：PresenceRepo + Redis schema 落地 + main.go 钩子 wire + 配置项 + 单测 10 case + 集成测 1 case + ws hook 集成测 4 case |
| 2026-05-07 | review r1 P1 修：RemoveOnline 加 sessionID guard（Lua script atomic compare-and-delete）防 reconnect 路径误删新 session presence；RedisClient 接口扩 Eval 方法；新增 3 个单测 case 覆盖 ReconnectRace / MatchingSessionID / TTLExpiredKey 场景；归档 lesson `2026-05-07-redis-presence-remove-needs-session-id-guard-10-6-r1.md` |

### File List

**新建文件**:
- `server/internal/repo/redis/presence_repo.go`（PresenceRepo interface + 5 方法实装）
- `server/internal/repo/redis/presence_repo_test.go`（单测 10 case）
- `server/internal/repo/redis/presence_repo_integration_test.go`（集成测 50 user 并发，build tag integration）
- `server/internal/infra/config/testdata/redis_presence_ttl.yaml`（loader test fixture：explicit positive YAML）
- `server/internal/infra/config/testdata/redis_presence_ttl_negative.yaml`（loader test fixture：negative YAML fallback）

**修改文件**:
- `server/internal/infra/config/config.go`（RedisConfig 加 PresenceTTLSec int 字段）
- `server/internal/infra/config/loader.go`（加 defaultRedisPresenceTTLSec=300 + 兜底逻辑 `<= 0 → 默认`）
- `server/internal/infra/config/loader_test.go`（加 3 case：default / explicit YAML / negative fallback）
- `server/configs/local.yaml`（加 `presence_ttl_sec: 300` 字段）
- `server/cmd/server/main.go`（加 redisrepo import alias + presenceHookTimeout const + wire PresenceRepo + WithRegisterHook / WithUnregisterHook adapter 注入）
- `server/internal/app/ws/ws_test.go`（加 fakePresenceRepo helper struct + 4 case 钩子集成测试）
