---
date: 2026-05-06
source_review: codex review (epic-loop r2 for Story 10.3 ws-网关骨架)
story: 10-3-ws-网关骨架
commit: a5afc6c
lesson_count: 2
---

# Review Lessons — 2026-05-06 — WS reconnect 替换路径漏触发 onUnregister 钩子 + WSConfig 契约字段缺 prod 强制

## 背景

Story 10.3 r2 review。r1 修了 `SessionManager.Close()` 路径漏调 onUnregister 钩子（保留索引到所有 Close 跑完）。r2 codex 又揪到同族 bug 的另一面：`Register()` 替换路径（同 user reconnect）也漏调旧 Session 的 onUnregister 钩子；以及 WSConfig 文档说 `heartbeat_timeout_sec=60 / max_message_size_bytes=16384` 是 prod 不可变契约，但 `NewGateway` 接受 YAML 任意值不强制 prod 校验，跟注释说"应 fail fast"完全相反。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Register 替换路径漏触发旧 Session 的 onUnregister 钩子 | P1/high | architecture | fix | `server/internal/app/ws/session_manager.go` |
| 2 | WSConfig prod-immutable 契约值未强制（注释说"应 fail fast"，实装未做） | P2/medium | config | fix | `server/internal/app/ws/gateway.go`, `server/internal/app/bootstrap/router.go` |

## Lesson 1: Register 替换路径漏触发旧 Session 的 onUnregister 钩子

- **Severity**: P1 / high
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `server/internal/app/ws/session_manager.go:139-146`（修复前）

### 症状（Symptom）

同 user 在不同 room 之间 reconnect（room A → room B），manager 替换索引让 `userToSessionID[user]=newID`，旧 Session 被强制 Close。但是 `WithUnregisterHook` 注入的钩子（10.6 Redis presence cleanup / metrics 计数）**对旧 Session 不触发** —— room A 的 presence 状态、metrics gauge 等外部状态被永久遗留。

### 根因（Root cause）

`Register()` 替换路径与 `Unregister()` 钩子触发逻辑的耦合断裂。修复前流程：

```
1. Register 锁内：m.removeFromIndicesLocked(oldS)  ← 先把旧 session 从索引清掉
2. Register 锁外：oldS.Close()                      ← 再 Close 旧 session
3. oldS.Close() → notifyClosed(oldID) → Unregister(oldID)
4. Unregister 锁内：m.sessionsByID[oldID] 不存在 → 走 no-op 路径 → 钩子漏调
```

`Unregister` 把"是否触发钩子"和"sessionID 在不在 sessionsByID 索引里"绑死了：步骤 1 提前清索引，让步骤 4 看到的是"已不存在的 sessionID"，钩子触发条件失效。

这是 r1 修的 `SessionManager.Close()` 同族 bug 的另一面 —— r1 是关停时漏调，r2 是 reconnect 替换时漏调。两者根因都是"清索引 → 后续路径找不到 session 走 no-op → 钩子触发条件失效"。

### 修复（Fix）

镜像 r1 `Close()` 的同模式：**保留旧索引到 oldS.Close() 跑完**，让 oldS.Close() → notifyClosed → Unregister(oldID) 走标准的索引清理 + 钩子触发路径。

```go
// 修复前（旧）
var replaced *Session
if oldID, ok := m.userToSessionID[s.userID]; ok {
    if oldS, ok2 := m.sessionsByID[oldID]; ok2 {
        replaced = oldS
        m.removeFromIndicesLocked(oldS) // 提前清索引 → 让 Unregister 走 no-op
    }
}

// 修复后（新）
var replaced *Session
if oldID, ok := m.userToSessionID[s.userID]; ok {
    if oldS, ok2 := m.sessionsByID[oldID]; ok2 {
        replaced = oldS
        // 保留 oldS 的索引；让 oldS.Close() → notifyClosed → Unregister(oldID)
        // 走标准 removeFromIndicesLocked + onUnregister 触发路径
    }
}
```

新 session 的索引仍然在 `Register` 锁内注入。`removeFromIndicesLocked` 内 `currentID == s.sessionID` 守卫确保 `userToSessionID[user]` 不被旧 session 的清理路径误删回 oldID（Unregister(oldID) 跑时 userToSessionID[user]=newID，守卫不命中，跳过 delete —— 修复前就有的防御机制，本次直接受益）。

短暂重叠窗口（< 1ms）：oldS.Close() 跑完前 sessionsByID 同时含 oldID + newID。可接受 —— ListSessionsByRoomID 短暂返双倍但 userToSessionID 已指向 newID；广播路径短暂双发对客户端无副作用（旧 session 紧接着 close 1006 让客户端忽略后续）。

加测试 `TestSessionManager_Reconnect_TriggersUnregisterHookForOldSession`：
- reconnect from room A to room B → 旧 Session 的 onUnregister 钩子触发**恰好一次**（不是 0 次也不是 2 次）
- 钩子收到的是**旧** sessionID（而非新的）
- 旧 sessionID 不再在 manager 索引中

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"manager 替换 / 关停 / 强制 evict 旧资源"路径时，**禁止**在调旧资源 cleanup 方法之前清自己的索引 —— 索引必须保留到 cleanup 方法跑完，让 cleanup 方法回调里的钩子触发条件（"sessionID 是否在索引"）能命中。
>
> **展开**：
> - manager / registry / pool 类型的 hook 触发逻辑常常和"资源是否在索引里"耦合（`Unregister` 看到 sessionID 不存在 → no-op + 不触发钩子）。这是合理设计 —— 让 Unregister 幂等；但同时锁死了"必须先有索引才能走钩子"的前提
> - "替换"路径（reconnect / take-over / evict）和"关停"路径（Close all）都属于"持有引用 + 触发 cleanup"场景。两者都不能提前清索引 —— 否则 cleanup 内部回调的 Unregister 路径走 no-op，钩子漏调
> - **正确模式**：保留索引 → 调 cleanup（cleanup 内部走标准 Unregister 触发钩子并清索引）→ 不需要再额外清索引。如果 cleanup 不走 Unregister 回调路径（例如某些 cleanup 是同步内联清理），那就在 cleanup 后**手动**调钩子 + 清索引；但**禁止**"先清索引再调 cleanup"的反序
> - **反例**：`m.removeFromIndicesLocked(oldS); oldS.Close()` —— oldS.Close() 内部回调 `Unregister(oldID)` 看到 sessionsByID[oldID] 不存在，走 no-op 路径不触发 onUnregister 钩子。所有挂在钩子上的外部状态（presence / metrics / 通知）被泄漏到 stale
> - **同族 bug 已修过两次**：r1 修 `SessionManager.Close()`（关停路径），r2 修 `Register()` 替换路径。下次写新的"持有 → 替换 / 强制清理"代码时直接用"保留索引到 cleanup 跑完"模式，避免第三次踩

## Lesson 2: WSConfig prod-immutable 契约值缺启动期强制（仅靠注释钦定不够）

- **Severity**: P2 / medium
- **Category**: config / fail-fast
- **分诊**: fix
- **位置**: `server/internal/app/ws/gateway.go:44-49`（修复前 `NewGateway` 签名）

### 症状（Symptom）

WSConfig 文档钦定 `heartbeat_timeout_sec=60` 和 `max_message_size_bytes=16384` 是跨节点 / 跨端协议契约一部分（V1 §1 节点 4 冻结 + §12.2 关键约束），prod 部署不可覆盖。但 `NewGateway` 接受任何 YAML 值都不报错。生产配置错（K8s ConfigMap 误注入 `heartbeat_timeout_sec=30`）会让该节点和其他节点 / iOS 客户端协议漂移：
- heartbeat 阈值不一致 → presence 状态抖动
- max_frame_size 不一致 → 一边能收一边超 limit 静默断连

注释说"应 fail fast"，实装却没做。

### 根因（Root cause）

WSConfig 字段加了详细的"prod 不可覆盖"注释（config.go:114 行起），但缺了**启动期 enforcement**。loader 只做 `<= 0 → default` 兜底，无法区分"YAML 缺字段"和"YAML 显式覆盖"。即使能区分，loader 也不该承担业务契约校验（loader 的语义是"格式 + 默认值"）。这种"靠注释钦定 + 期望开发者读文档"的设计模式被 Story 7.3 review r6 [P2] 已经识别过同类问题（StepsConfig.SingleSyncCap / DailyCap 一旦运维误推 dev YAML 到 prod 就静默漂移）—— 那次的解决方案是 `NewStepService` 接受 `envName string` 参数，prod 严格策略下任何正值 cap 直接 panic。

WSConfig 这次又踩了 —— Story 10.3 实装时漏了同模式的 prod gate。

### 修复（Fix）

`NewGateway` 加 `envName string` 参数，复制 `NewStepService` 的 prod 严格策略：

```go
func NewGateway(
    signer *auth.Signer,
    mgr SessionManager,
    roomMember mysql.RoomMemberRepo,
    cfg config.WSConfig,
    envName string, // review r2 P2 加：prod contract override 强制
) *Gateway {
    envLower := strings.ToLower(strings.TrimSpace(envName))
    isOverrideAllowed := envLower == "dev" || envLower == "staging" || envLower == "test"
    if !isOverrideAllowed {
        if cfg.HeartbeatTimeoutSec != wsProdHeartbeatTimeoutSec {
            panic(fmt.Sprintf(
                "ws gateway: prod env (CAT_ENV=%q) must use default heartbeat_timeout_sec=%d; got %d",
                envName, wsProdHeartbeatTimeoutSec, cfg.HeartbeatTimeoutSec,
            ))
        }
        if cfg.MaxMessageSizeBytes != wsProdMaxMessageSizeBytes {
            panic(fmt.Sprintf(
                "ws gateway: prod env (CAT_ENV=%q) must use default max_message_size_bytes=%d; got %d",
                envName, wsProdMaxMessageSizeBytes, cfg.MaxMessageSizeBytes,
            ))
        }
    }
    // ... 原有构造逻辑
}
```

bootstrap.Deps 已有 EnvName 字段（Story 7.3 review r6 [P2] 加的），router.go 中 NewGateway 调用直接透传 `deps.EnvName`，与 NewStepService 同模式。WriteTimeoutSec 不在契约（仅 server 内部容错），不强制。

加测试覆盖：
- `TestNewGateway_ProdEnv_RejectsNonContractHeartbeat` —— prod + 非契约 heartbeat → panic
- `TestNewGateway_ProdEnv_RejectsNonContractMaxMessageSize` —— prod + 非契约 frame size → panic
- `TestNewGateway_EmptyEnv_BehavesAsProd` —— env="" 走 prod 严格（safe-by-default）
- `TestNewGateway_DevEnv_AcceptsOverride` —— dev 允许覆盖
- `TestNewGateway_ProdEnv_AcceptsContractValues` —— prod + 契约值正常构造

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 config struct 时，凡是在注释里钦定"prod 不可覆盖 / prod 必须用默认值"的字段，**必须**在 service / gateway 构造期加 `envName string` 参数 + prod 严格策略 panic，**禁止**只靠注释 + "期望开发者读文档"。
>
> **展开**：
> - "跨实例 / 跨端协议契约"类字段（heartbeat 阈值、frame size 上限、step 同步 cap、token 过期、JWT secret 长度）一旦在 prod 漂移会引发**静默** bug —— 不会立即 panic / 报错，但行为不一致到一定程度后业务表现"诡异且难复现"。这类字段必须启动期 fail-fast
> - **prod 严格策略 safe-by-default**：env 归一化为小写后，**只**显式 "dev" / "staging" / "test" 才允许覆盖；空 / 未知 / "prod" / "production" / typo 全部按 prod 严格。运维漏配 CAT_ENV / 拼错 / dev YAML 静默推到 prod 都会被启动期 panic 拦截
> - **panic vs return error**：构造期 fail-fast 用 `panic`（与 NewStepService / auth.New / db.Open / redis.Open 同模式 —— main.go 把 panic 转 os.Exit(1) 走 logger 输出）。比起 `error` 返回，panic 更明确"这是配置纪律错误，不是运行时可恢复"
> - **panic 消息必须含 4 个信息**：(1) 业务模块名（"ws gateway:"）；(2) 当前 env 实际值（"CAT_ENV=%q"）；(3) 期望值 vs 实际值（"must use default heartbeat_timeout_sec=60; got 30"）；(4) 修复路径提示（"dev/test 覆盖必须 export CAT_ENV=dev|staging|test"）。让运维一眼能看出怎么修
> - **反例 1**：只在 config struct 注释里写"prod 不可覆盖"，启动期不校验。运维误配后 server 起来正常工作，业务行为漂移但无错误日志，几天后才靠监控曲线发现 → 此时已经污染了线上数据
> - **反例 2**：在 loader 里做契约校验（`if cfg.HeartbeatTimeoutSec != 60 { return error }`）。loader 应该只管"格式 + 默认值"，业务契约属 service / gateway 边界 —— 强行混进 loader 让 dev / 单测 / fixture 路径都被卡死
> - **反例 3**：用 env 白名单的反向逻辑（"prod / production / 空都允许覆盖，只有 dev 严格"）。这违反 safe-by-default —— 运维漏配 CAT_ENV 不应该 = 允许任意覆盖
> - **同模式已存在**：`NewStepService(... envName string)` Story 7.3 review r6 [P2] 引入；`NewGateway(... envName string)` Story 10.3 review r2 [P2] 引入。下次再加 prod-immutable 契约字段时直接复用此模式

---

## Meta: 本次 review 的宏观教训

r2 两条 P 级 finding 都在追问"r1 修了一个症状，但根本病根没修彻底" —— 

- L1：r1 修 Close 路径漏调钩子，r2 又揪到 Register 替换路径漏调钩子。**根本病根**是"清索引 → Unregister 走 no-op 路径不触发钩子"的耦合机制；下次写新的"持有引用 → 强制清理"代码必须直接复用"保留索引到 cleanup 跑完"模式
- L2：StepsConfig 在 Story 7.3 review r6 [P2] 加了 prod gate；WSConfig 实装时漏了同模式。**根本病根**是 config struct 注释里钦定"prod 不可覆盖"但缺机械化的启动期校验。下次再写 prod-immutable contract 字段时**必须**同步加 `NewXxx(... envName string)` + prod 严格策略 panic

下次评审 / 写新代码遇到"manager 替换 / 强制清理"或"config struct 标 prod 不可覆盖"模式，先翻这个 lesson 文档对照检查，避免第三次踩。
