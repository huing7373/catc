---
date: 2026-05-07
source_review: codex review (epic-loop r6) — /tmp/epic-loop-review-10-6-r6.md
story: 10-6-redis-presence-repo
commit: <pending>
lesson_count: 3
---

# Review Lessons — 2026-05-07 — Presence lifecycle hook 必须 fire-and-forget & TTL 硬下限必须 prod-only env gate（10-6 r6）

## 背景

Story 10.6 r1-r5 把 Redis presence 写入接到 WS SessionManager 的 lifecycle hook（Register → AddOnline / Unregister → RemoveOnline）+ HeartbeatScanner 30s tick 续期。codex 在 r6 codex review 指出两类正交问题：

1. lifecycle hook 内部直接同步调 Redis I/O → 让 connect 握手延迟 / shutdown 关停延迟与 Redis 健康度强耦合（P1×2）。
2. r3 引入的"显式 PresenceTTLSec < 60s 硬下限 fail-fast"与 RedisConfig.PresenceTTLSec 注释 + sample-config "dev / test 可短到 5s 走 miniredis FastForward" 冲突 —— dev 按文档配 5s 启动失败（P2）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Register hook 同步调 AddOnline 阻塞握手 | P1 (high) | perf | fix | `server/cmd/server/main.go:250-253` |
| 2 | Unregister hook 同步调 RemoveOnline 阻塞关停 | P1 (high) | perf | fix | `server/cmd/server/main.go:262-270` |
| 3 | TTL 硬下限与文档注释冲突，dev 启动失败 | P2 (medium) | config | fix | `server/internal/infra/config/loader.go:182-184` |

修了 3 条 / defer 0 条 / wontfix 0 条。

## Lesson 1 + 2: WS lifecycle hook 内部远程 I/O 必须 fire-and-forget goroutine，不能同步阻塞 Register / Unregister 主路径

- **Severity**: high (P1) ×2
- **Category**: perf
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:250-253`（Register hook）+ `server/cmd/server/main.go:262-270`（Unregister hook）

### 症状（Symptom）

Register / Unregister hook 直接 `presenceRepo.AddOnline(hookCtx, ...)` / `RemoveOnline(...)` 同步调 Redis：

- **Register 路径**：`gateway.handleWS` 等 `SessionManager.Register` 返回才启 read/write loop。Redis 慢 / brownout 时单次 connect/reconnect 卡到 `presenceHookTimeout=2s` 上限 → 所有用户握手 visible 延迟。
- **Unregister 路径**：`SessionManager.Close` 串行调 Unregister；reconnect 替换路径也串行调旧 Session.Close → notifyClosed → Unregister。Redis 挂时单 session O(2s)，N 个 active session shutdown 退化成 O(N × 2s) → K8s `terminationGracePeriodSeconds`（默认 30s）轻松超。
- 即便 hook timeout 缩短到 100ms，主路径仍同步阻塞 → 用户感知延迟与 Redis 健康度强耦合，违反"presence 写失败仅 log warn 不影响 server 主流程"的最初设计意图。

### 根因（Root cause）

把"hook 是 Session 索引变更时必须做的副作用之一"和"hook 必须在 Register / Unregister return 之前完成"两件事混淆了。实际上：

- 只有"hook 必须看见 lock 内的最终状态"才需要同步（这里满足：hook 在 m.mu.Unlock() 之后调，最终状态已可见）。
- "hook 完成时机"与"主路径 return 时机"无任何业务依赖：presence 是用来给 ListOnline / IsOnline 查询的远程状态，主路径自己不读自己刚写的 presence；同步等 Redis return 没有任何业务价值，只换来"Redis 健康度 → 主路径延迟"耦合。

进一步看：r2 加 scanner 30s reconcile + r1 加 sessionID guard + RedisConfig 5min TTL 三层兜底已经覆盖了"hook 漏写 / partial-fail / late RemoveOnline" 所有失败模式，hook 同步语义不再是必要条件。

### 修复（Fix）

把 hook body 整个包进 `go func() { ... }()`：

```go
sessionMgr := wsapp.NewSessionManager(
    wsapp.WithRegisterHook(func(s *wsapp.Session) {
        go func() {
            hookCtx, cancel := context.WithTimeout(context.Background(), presenceHookTimeout)
            defer cancel()
            if err := presenceRepo.AddOnline(hookCtx, s.RoomID(), s.UserID(), s.SessionID()); err != nil {
                slog.Warn("presence add online failed", ...)
            }
        }()
    }),
    wsapp.WithUnregisterHook(func(s *wsapp.Session) {
        go func() {
            hookCtx, cancel := context.WithTimeout(context.Background(), presenceHookTimeout)
            defer cancel()
            if err := presenceRepo.RemoveOnline(hookCtx, s.RoomID(), s.UserID(), s.SessionID()); err != nil {
                slog.Warn("presence remove online failed", ...)
            }
        }()
    }),
)
```

副作用：

- shutdown 期 fire-and-forget goroutine 可能没机会跑完 → 可接受（presence key 5min TTL 自然过期；shutdown 时优先快速关停而非等所有 RemoveOnline 完成）。
- 单测仍可用 `wsapp.WithRegisterHook` 注入计数器同步收集 —— 单测里直接 hook 自己控制是否 goroutine（main.go 才 fire-and-forget；hook signature 仍是 `func(s *Session)`，goroutine 是 hook body 自己的事）。
- Reconnect 替换路径 hook 顺序仍由 SessionManager 锁外串行触发（旧 Unregister hook 先于新 Register hook dispatch 各自的 goroutine），sessionID guard + scanner reconcile 兜底让乱序也安全。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **lifecycle hook（Register / Unregister / OnConnect / OnClose 等）的 body 里调远程 I/O（Redis / MySQL / HTTP 外调）** 时，**必须包 fire-and-forget goroutine**，不能让远程 I/O 同步阻塞 hook 主路径，**除非**业务有强语义需要主路径见到 hook 完成结果。
>
> **展开**：
> - 判定问句："如果这个 hook 跑慢了 / 失败了，主路径需要 fail / retry 吗？" 答 NO（典型 presence / metrics / audit log）→ goroutine。答 YES（典型 user 创建后立即依赖远程状态做下一步）→ 同步 + 显式 error propagate（不是 log warn）。
> - 兜底栈必须先到位再 fire-and-forget：(a) TTL 自然过期 (b) periodic reconcile / scanner (c) 幂等 + sessionID/idempotency guard。三件齐才能容忍 hook 偶发漏跑。
> - shutdown 路径下 fire-and-forget goroutine 可能 leak / 未跑完 → 用 TTL 兜底而不是 sync.WaitGroup 等所有跑完（除非业务硬要求 graceful 100% 清空，那就开 follow-up story 加 WaitGroup 模式而不是默认）。
> - **反例**：`func(s *Session) { redisRepo.AddOnline(ctx, ...) }` —— 主路径 = SessionManager.Register（被 gateway.handleWS 同步等）= 用户握手延迟。Redis 1 秒卡顿 → 1000 个连接全部多花 1 秒。
> - **反例**：用 `presenceHookTimeout = 100ms` 把同步上限缩短当作"足够快不需要 goroutine" —— 即便 100ms 也是用户感知延迟与 Redis 健康度耦合，且 Redis brownout 期 timeout 实际命中率会高，依旧 visible。

## Lesson 3: 启动期 fail-fast 硬约束必须按 prod-only env gate，dev / test 留逃生口

- **Severity**: medium (P2)
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/infra/config/loader.go:182-184`

### 症状（Symptom）

r3 P2 加 `redis.presence_ttl_sec < 60` 硬下限 fail-fast。但项目配置文档（`RedisConfig.PresenceTTLSec` 注释 + `configs/local.yaml` 注释 + `internal/repo/redis/presence_repo.go` 注释）一致写着 "dev / test 可短到 5s 走 miniredis FastForward 测试 TTL 行为"。dev 按文档配 5s → loader 直接报错启动失败。配置面与校验面互相打架。

### 根因（Root cause）

写 r3 fix 时把"prod 必须 ≥ 60s 防 flap"和"任何环境都不能 < 60s" 混淆了。前者是真实业务约束（HeartbeatScanner 30s tick × 2 = 60s 是数学下限），后者是把 prod 约束错误地往 dev / test 推。dev / test 场景：

- miniredis FastForward 测试 TTL 行为：必须用 5s 这种短 TTL，否则单测要等 5min 才能验证过期路径
- 本地 dev 跑 + 反复重连验证 presence cleanup：5-10s TTL 比 5min 反馈快得多
- prod 真实业务约束（多实例 + heartbeat 续期路径必须保证 keys 不被两个 tick 之间提前过期）在 dev / test 不成立（单实例、没真实长时活跃 session）

校验代码漏了一个常见维度：**fail-fast 不是越严越好**。fail-fast 的目标是"prod 起来后不会因配置错跑出业务异常"，不是"任何环境都拒绝 unusual 配置"。dev / test 留逃生口才符合"防 prod 配置漂移 ≠ 锁死所有覆盖"。

### 修复（Fix）

下限校验加 prod-only env gate，与 Story 7.3 review r6 [P2] StepsConfig prod cap-override 同模式：

```go
if isProdEnv() && cfg.Redis.PresenceTTLSec < minRedisPresenceTTLSec {
    return nil, fmt.Errorf("redis.presence_ttl_sec=%d below minimum %d in prod (...; dev/test 覆盖必须 export CAT_ENV=dev|staging|test)", ...)
}

// isProdEnv: CAT_ENV ∈ {"dev","staging","test"} → false；其他 / 空 → true
// safe-by-default：未注入 / 拼写错都视为 prod，避免 dev YAML 静默漂到 prod
func isProdEnv() bool {
    switch os.Getenv("CAT_ENV") {
    case "dev", "staging", "test":
        return false
    default:
        return true
    }
}
```

测试覆盖（5 个单测）：
- prod env（CAT_ENV="" 默认）+ 30s → fail-fast
- dev env (CAT_ENV="dev") + 30s → 通过
- staging env (CAT_ENV="staging") + 30s → 通过
- test env (CAT_ENV="test") + 30s → 通过
- 拼错 (CAT_ENV="production") + 30s → safe-by-default → fail-fast

未覆盖维度（仍由现有测试守护）：边界值 60s（prod 路径仍通过）、负值兜底成 300（fallback 路径不进 floor 校验）。

为什么不挂 `cfg.Env` 字段：项目既有 prod gate（`service.NewStepService` envName 参数 / `wsapp.NewGateway`）都就近读 `os.Getenv("CAT_ENV")`，不挂 cfg 字段。loader 自身用同模式保持一致；新加 cfg.Env 字段会引入"loader 加载 cfg → loader 校验 cfg.Env"的循环依赖（cfg 自己定义自己的校验语义），且需要给所有 yaml 加 `env:` 字段并在 main.go 同步赋值，scope creep。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 加 **启动期 fail-fast 硬约束**（loader 校验 / 工厂 panic）时，**必须先问"这个约束在 dev / test 也成立吗？"**，dev / test 路径有合理的"违反约束"业务用法（fixture / FastForward / 反复重启验证）→ 必须包 prod-only env gate（`os.Getenv("CAT_ENV")` 白名单 + safe-by-default 视为 prod）。
>
> **展开**：
> - 判定问句："这条约束的工程依据是什么？" 如果依据是"prod 多实例 + 长时运行 + 真实流量"才成立（如 heartbeat × N tick 数学下限、JWT 强密钥、step cap V1 契约值）→ env gate 必填。
> - safe-by-default 锚点：env gate 必须用"白名单 dev/staging/test 才放过，其余视为 prod"模式，**不**用"黑名单 prod 才严"模式 —— 后者让 CAT_ENV 拼错（"production" / "produciton"）静默漂到 prod。
> - 配置面与校验面**双向同步**：加 fail-fast 必须同步检查所有写着"dev / test 可设 X" 的注释 / sample-config / repo doc，没同步会立刻被新人 / lint / dev fixture 撞穿。
> - **反例**：`if cfg.Redis.PresenceTTLSec < 60 { return error }` —— 任何环境都拒绝 < 60，与 RedisConfig 注释 "dev/test 可短到 5s" 直接冲突；dev fixture 跑不起来。
> - **反例**：用 cfg.Env 字段挂在 Config 上 —— 多了一层"cfg 校验 cfg.Env" 的循环、且打破现有"就近读 CAT_ENV"模式（service.NewStepService / wsapp.NewGateway 的 envName 参数）。本项目惯用 `os.Getenv("CAT_ENV")` 就近读，loader 自身也走同模式保持一致。
> - **反例**：把 5s 注释改成 "必须 ≥ 60s" 来强行对齐 fail-fast —— 等于剥夺 dev / test 的 fast-feedback 工具（FastForward + miniredis 5s TTL 单测路径直接挂）。

---

## Meta: r6 整体教训

r6 codex review 三条 finding 共享一个底层模式：**为某条业务约束（presence 不能写僵尸 / TTL 不能短于 scan tick）写"防御代码"时，把"约束"和"实现细节"混淆了**。

- finding 1+2：约束 = "presence 状态最终被写入 Redis"。实现细节误加 = "必须在 hook return 之前同步完成"。修法：剥离实现细节，让 fire-and-forget + scanner reconcile + TTL 三层兜底承担约束。
- finding 3：约束 = "prod 多实例不能让 presence flap"。实现细节误加 = "任何环境 < 60s 都拒"。修法：env gate 让约束只在 prod 生效。

未来加防御代码 / 校验 / fail-fast 路径时，先把"业务约束的真实工程依据"与"防御代码的实现细节"分开列出来，再问"实现细节在 dev / test / shutdown / partial-failure 路径下也必要吗？"答否就得加 escape hatch（goroutine / env gate / TTL 兜底 / scanner reconcile）。
