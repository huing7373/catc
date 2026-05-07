---
date: 2026-05-07
source_review: codex review for Story 10-6 r3 (/tmp/epic-loop-review-10-6-r3.md)
story: 10-6-redis-presence-repo
commit: 0e75ede
lesson_count: 2
---

# Review Lessons — 2026-05-07 — Presence reconcile 必须走 idempotent 全量重写而非纯 EXPIRE 续期 & TTL 显式配置必须 ≥ 2× scan interval（10-6 r3）

## 背景

Story 10.6 r3 review 由 codex 出。r2 把 HeartbeatScanner.scanOnce 加了对 active session 调 `PresenceRepo.RenewTTL` 续 TTL 的逻辑，避免 long-lived session 过期；同时把 AddOnline 命令顺序改成 SET → SADD → EXPIRE 让 partial-fail 不留永久 zombie。r3 codex 指出两条遗漏：

1. Register hook 调 AddOnline 失败仅 log warn 但仍接受 session；scanner 路径只调 RenewTTL（EXPIRE 双 key），不会重建缺失的 room set 成员 → 整个 session 生命周期内 IsOnline/ListOnline 误报 user offline。
2. `presence_ttl_sec` loader 接受任意正值；HeartbeatScanner tick 写死 30s，TTL ≤ 30s 让连续两次 tick 之间已过期 → user 闪烁 offline 即使 session 活跃。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | scanner reconcile 必须用 AddOnline 而非 RenewTTL，让 Register partial-fail 自愈 | P2 (medium) | architecture | fix | `server/internal/app/ws/heartbeat_scanner.go`, `server/internal/app/ws/ws_test.go` |
| 2 | `presence_ttl_sec` loader 必须 ≥ 60s 下限校验（= 2 × 30s scan interval） | P2 (medium) | config | fix | `server/internal/infra/config/loader.go`, `loader_test.go` |

## Lesson 1: scanner reconcile 必须用 AddOnline 而非 RenewTTL，让 Register partial-fail 自愈

- **Severity**: P2 (medium)
- **Category**: architecture / error-handling
- **分诊**: fix（option b：直接复用 AddOnline 的 idempotency，无新接口）
- **位置**: `server/internal/app/ws/heartbeat_scanner.go:269-298`

### 症状（Symptom）

`SessionManager` 的 Register hook 调 `presenceRepo.AddOnline` 失败时仅 log warn，session 仍被接受。后续 `HeartbeatScanner` 每 30s tick 只调 `RenewTTL`（底层 EXPIRE 双 key），EXPIRE 对不存在的 key 是 no-op —— 不会重建 SADD 缺失的 `room:{id}:online_users` member。结果：partial Register 失败 → 整个 session 生命周期内 IsOnline/ListOnline 误报 user offline，直到 client reconnect 走新一轮 Register。

### 根因（Root cause）

r2 加 RenewTTL 时只想着"long-lived session 过期"这一种风险，没考虑"Register hook partial-fail" 的另一种风险。两种风险症状相同（user 误报 offline）但根因不同：

- 风险 A：keys 已存在 + TTL 即将过期 → EXPIRE 续命就够（RenewTTL 完美适配）
- 风险 B：keys 因 partial-fail 缺失 → 必须重新 SET + SADD + EXPIRE 才能重建（RenewTTL 无效）

r2 选择 RenewTTL 是基于"keys 一定存在"假设，但接受 hook 失败仅 log warn 的语义本身就破坏了这个假设。

### 修复（Fix）

把 `PresenceRenewer` 接口的方法从 `RenewTTL(ctx, roomID, userID)` 改成 `AddOnline(ctx, roomID, userID, sessionID)`：

- AddOnline 是 idempotent（SET nx=false 覆盖、SADD 已存在 no-op、EXPIRE 总是续）→ 重复调安全
- AddOnline 同时覆盖风险 A（已有 keys → 重写更新 + 续 TTL）和风险 B（缺失 keys → 重建）
- 接口名保持 `PresenceRenewer`（语义上现在是"reconcile" 而非"renew"，但避免 rename 牵连面），方法名换成 `AddOnline` 与 PresenceRepo 既有方法对齐 → PresenceRepo 自动满足新接口
- IO 比 RenewTTL 略多（每 session 3 次 Redis command vs 2 次）但 SLO 内可接受

```go
// before（r2）
type PresenceRenewer interface {
    RenewTTL(ctx context.Context, roomID, userID uint64) error
}
// scanner: s.renewer.RenewTTL(ctx, sess.RoomID(), sess.UserID())

// after（r3 P2）
type PresenceRenewer interface {
    AddOnline(ctx context.Context, roomID, userID uint64, sessionID string) error
}
// scanner: s.renewer.AddOnline(ctx, sess.RoomID(), sess.UserID(), sess.SessionID())
```

测试同步更新：`fakeRenewer.RenewTTL` → `fakeRenewer.AddOnline`，加 `lastSession` map 验证 sessionID 正确传播；测试名 `_RenewsTTL` → `_ReconcilesPresence` / `_DoesNotRenewTTL` → `_DoesNotReconcile`。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **scanner / 周期性 reconcile 路径设计接口** 时，**必须** **优先选 idempotent 全量写入 API（如 AddOnline）而非纯续期 API（如 RenewTTL/EXPIRE）**，除非能证明"被续期的 keys 一定存在"这一前提在所有调用路径都成立。
>
> **展开**：
> - 接口名可以保留"renew" 但语义本质必须是 "reconcile = 把当前应有的状态全量写一遍"
> - 触发条件包括：register hook 接受 partial-fail / Redis cluster 节点切换重启 / 运维 FLUSHDB 误操作 / TTL 过期窗口 race / 任何让 keys 可能"应在但不在"的路径
> - "续期" 类 API 适用且仅适用于：keys 在调用前 100% 已存在，调用方只想刷 TTL，不想重写 value（如安全令牌轮转里的 sliding session）—— 这种场景下"全量重写"反而更贵
> - 接口设计时问自己：如果某个上游路径 swallowed 了"创建失败" error，本接口能不能在 30s 内自愈？不能 → 必须改成 idempotent 全量
> - **反例**：r2 选 RenewTTL 设计 `PresenceRenewer interface { RenewTTL(ctx, roomID, userID) }` 看起来"接口窄、IO 少"，但完全没意识到 hook 路径的 swallow-and-warn 模式让 keys 缺失成为常态 → 接口前提不成立 → 30s tick 永远恢复不了 partial-fail session

## Lesson 2: `presence_ttl_sec` loader 必须 ≥ 60s 下限校验（= 2 × scan interval）

- **Severity**: P2 (medium)
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/infra/config/loader.go:162-184`, `server/internal/infra/config/loader_test.go`

### 症状（Symptom）

`presence_ttl_sec` loader 接受任意正值。如配置成 ≤ 30s，HeartbeatScanner 每 30s tick 调 reconcile 期间，TTL 已过期 → user 闪烁 offline 即使 session 活跃。运维侧不会有任何报错信号（只是观察到"presence 抖动"）。

### 根因（Root cause）

loader 既有的"<= 0 兜底成默认值"模式只覆盖了 zero-value / 配错负值 / YAML 缺字段三种 corner case，没考虑"语法上合法但语义上违反 invariant"的中间地带（如 1 ≤ ttl ≤ 30s）。这种"小正值"在 `pool_size` / `bucket_limit` 等字段上是可接受的（性能 degraded 但不出错），但 TTL 字段有"必须 > 周期 tick"的硬不变量 → 配错就直接业务行为异常。

invariant 依赖关系：
- `heartbeatScanIntervalSec = 30`（写死，不开放配置 —— SLO 契约）
- `redis.presence_ttl_sec >= 2 × heartbeatScanIntervalSec = 60` 必须保证
- 选 2× 而非 1× 留 buffer 应对 IO 抖动 / scanner tick 调度延迟

### 修复（Fix）

在 loader 加 `minRedisPresenceTTLSec = 60` 常量 + lower-bound 校验：

```go
// 零值 / 负值仍走默认 300 兜底（YAML 缺字段路径不应被卡住）
if cfg.Redis.PresenceTTLSec <= 0 {
    cfg.Redis.PresenceTTLSec = defaultRedisPresenceTTLSec
}
// 显式配置低于下限 → fail-fast
if cfg.Redis.PresenceTTLSec < minRedisPresenceTTLSec {
    return nil, fmt.Errorf("redis.presence_ttl_sec=%d below minimum %d (must be >= 2x heartbeat scan interval 30s)",
        cfg.Redis.PresenceTTLSec, minRedisPresenceTTLSec)
}
```

测试加：`TestLoad_RedisPresenceTTLSecBelowMin_FailFast`（30s → loader error）、`TestLoad_RedisPresenceTTLSecAtMin_OK`（60s 边界值通过）；既有 fixture `redis_presence_ttl.yaml` 从 30s → 90s（让既有 explicit 测试在新下限下仍合法）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **loader 加可配置数值字段** 时，**必须** **既加"<= 0 兜底默认"也加"低于业务下限 fail-fast"两层校验**，下限值由该字段与系统其他写死常量的不变量推导（如 `TTL >= 2 × scan_interval`）。
>
> **展开**：
> - 三档配置形态：
>   - **zero / 缺字段 / 负值** → 兜底默认值（用户没配，loader 友好补默认）
>   - **正值但低于业务下限** → fail-fast `return error`（用户显式配错，loader 拒绝启动让运维立即注意）
>   - **正值 ≥ 下限** → 原值透传
> - 下限的推导必须显式写在常量上方注释 + error message 里（如 `must be >= 2x heartbeat scan interval 30s`），让运维看到 error 就知道为什么以及怎么改
> - 配套测试三档都要覆盖：default fallback / below-min error / at-min OK / above-min OK
> - 与 lesson 2026-04-26-yaml-default-must-not-mask-explicit-invalid.md 协同：那条说"显式配错不要静默兜底"；本条说"显式配错有两种 — 语法非法（负值）走默认兜底，语义非法（小正值）走 fail-fast"
> - **反例**：r2 写 `if cfg.Redis.PresenceTTLSec <= 0 { ... = default }` 后没继续看"业务下限"的 invariant —— 看起来"零值 / 负值都兜底了"看似完整，但 1 ≤ ttl < scan_interval 的 case 完全漏掉，让运维配 5s TTL 也能起来 → presence 闪烁但没人知道为什么

## Meta: 本次 review 的宏观教训

两条 finding 都是 r2 修复内化的"半成品"：r2 加了周期续期路径但没考虑"被续期 keys 可能不存在" + r2 加了 TTL 配置字段但没考虑"配置值与周期常量的不变量"。共同 meta 教训：

> 加新机制（周期任务 / 配置字段 / 错误处理路径）时，**必须穷举该机制涉及的所有 invariant 并在代码或 schema 层逐一兜住**。问自己三个问题：
> 1. 本机制的输入前提（数据 / 配置 / state）在哪些路径下可能被破坏？破坏后本机制能否自愈？
> 2. 本机制依赖的写死常量（如 30s tick）和可配置字段（如 TTL）之间有没有 invariant？没显式写出来的 invariant 等于没保证。
> 3. 错误处理路径中的"swallow + log" 是否会让上述前提失效？如果 yes，必须在下游路径加自愈或在上游路径改成 fail-fast。

这是 r1 → r2 → r3 三轮迭代里反复出现的同一个 meta 漏洞 —— 每轮修一个症状但没穷举该症状所属的 invariant 全集。
