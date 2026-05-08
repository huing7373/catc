---
date: 2026-05-08
source_review: codex review round 1 (file: /tmp/epic-loop-review-11-3-r1.md)
story: 11-3-创建房间事务
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-08 — Schema fail-fast probe 必须随依赖该 schema 的 handler 挂载条件同步前移（11-3 r1）

## 背景

Story 11-3（创建房间事务）r1（review/fix 子循环第 1 轮）。codex review 指出 1 条 P2 fail-fast 范围错配：11-3 把 `POST /api/v1/rooms` HTTP handler 挂载条件从 r3 钦定的 `deps.SessionMgr != nil`（WS-only gate）放宽到 `GormDB / TxMgr / Signer` 都有就挂（HTTP-only wiring 也能跑），但 `rooms` / `room_members` 两表的启动期 schema sniff probe（r5/r6/r8 多轮迭代后的 `wsTablesReady`）仍然只在 `deps.SessionMgr != nil` 块里跑。结果：HTTP-only 部署形态（或 fixture）下 SessionMgr=nil 时缺 0007/0008 migration 在启动期不被捕获，第一个 POST /api/v1/rooms 请求才会拿 generic 1009 而不是 startup 时的清晰报错。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | rooms / room_members schema fail-fast probe 仍只在 SessionMgr 分支内执行，与新加的 HTTP-only room handler 挂载条件不一致 | P2 | architecture / fail-fast | fix | `server/internal/app/bootstrap/router.go:394-396` |

## Lesson 1: 启动期防御性 probe 的 if-guard 必须随"消费该资源的 handler 挂载条件"同步前移

- **Severity**: P2
- **Category**: architecture / fail-fast
- **分诊**: fix
- **位置**: `server/internal/app/bootstrap/router.go:394-396`（修复后 probe 已上移到 `if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil` 块顶部，room handler 挂载之前）

### 症状（Symptom）

- 部署形态 A（HTTP-only：SessionMgr=nil，但 GormDB / TxMgr / Signer 完整 → POST /api/v1/rooms 路由正常挂载）下，缺 0007 / 0008 migration 时：
  - 启动期 **没有** fail-fast panic（probe 跳过）
  - server 起来后第一个 `POST /api/v1/rooms` 请求进入事务内 INSERT rooms 时 MySQL 1146 → service 层 wrap 成 generic ErrInternal → handler 返 1009 给客户端 + healthcheck 看着健康
  - = silent disaster：feature 完全不可用但运维 / k8s healthcheck 都不告警；只有"用户首次创建房间报错"才间接发现 schema 漂移
- 部署形态 B（HTTP+WS：SessionMgr 非 nil）下：probe 仍执行，schema 漂移启动期就 panic → CrashLoopBackOff → 立即告警。两种形态对 schema 漂移的捕获时机出现不一致，违反 fail-fast 一致性。

### 根因（Root cause）

防御性 schema sniff helper（`wsTablesReady`）原本是 Story 10.3 r5 [P1] 引入，用来在 WS 路由挂载之前防御 rooms / room_members 表缺失（当时这两表只被 WS gateway 消费）。Probe 自然就放在 `if deps.SessionMgr != nil` 块（WS gate）内 —— 当时这个 if-guard 既是 "WS feature 是否启用" 又是 "rooms/room_members 表是否被消费" 的同义条件。

11-3 把 `POST /api/v1/rooms` HTTP handler 加到了"GormDB / TxMgr / Signer 都有就挂"的更早期位置（不依赖 SessionMgr —— HTTP-only 部署也要能挂这条路由，同时 router_test.go 的 Deps{} 零值场景仍跳过）。这一刻，"rooms / room_members 表的消费者集合" 就从 `{WS gateway}` 扩成了 `{WS gateway, POST /rooms HTTP handler}`，与 SessionMgr 这个 if-guard 的语义脱钩 —— SessionMgr=nil 不再代表"这两表无人消费"。

但 r1 reviewer 抓到的实际改动里，dev-story 只移动了 `POST /rooms` 的挂载位置 + 把 roomMemberRepo 实例上移共享，**没有** 同步把 probe 移出 SessionMgr 块。这是经典的"加新消费者时漏改 fail-fast 范围"——每个原本绑定到 if-guard A 的防御性检查，在 if-guard A 不再唯一覆盖该资源消费时都要重新评估归属。

更深层：**fail-fast probe 的 if-guard 应该等于"该资源消费者集合并集"**，不是某个具体 feature 的开关。当某资源的消费者集合发生变化（新加一个 handler / repo path），probe 的 gate 必须随之调整到新的最弱前置条件（最早的 GormDB != nil 之类）。

### 修复（Fix）

把 `wsTablesReady` 调用从 `deps.SessionMgr != nil` 块（WS 路由挂载段）内移到外层 `if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil` 块的顶部，紧贴 `roomSvc := service.NewRoomService(...)` 之前（与 `POST /rooms` handler 挂载条件对齐）。

```go
// before（旧形态：probe 仅在 WS 路由挂载段执行）：
if deps.SessionMgr != nil {
    if !wsTablesReady(deps.GormDB) {
        panic("ws backing tables missing: ...")
    }
    snapshotBuilder := wsapp.NewPlaceholderSnapshotBuilder(roomMemberRepo)
    gateway := wsapp.NewGateway(...)
    r.GET("/ws/rooms/:roomId", gateway.Handle)
}

// after（修复形态：probe 上移到 GormDB 块顶部，HTTP-only / HTTP+WS 共用一份）：
// ... roomRepo / roomMemberRepo 已在 if deps.GormDB 块顶部构造 ...
if !wsTablesReady(deps.GormDB) {
    panic("ws backing tables missing: rooms / room_members must exist (run migrations 0007 / 0008)")
}
roomSvc := service.NewRoomService(deps.TxMgr, userRepo, roomRepo, roomMemberRepo)
roomHandler := handler.NewRoomHandler(roomSvc)
// ...
authedGroup.POST("/rooms", roomHandler.CreateRoom)
// WS 路由挂载段现在不再重复 probe（redundant 且会让 sqlmock 测试期望对不上）：
if deps.SessionMgr != nil {
    snapshotBuilder := wsapp.NewPlaceholderSnapshotBuilder(roomMemberRepo)
    gateway := wsapp.NewGateway(...)
    r.GET("/ws/rooms/:roomId", gateway.Handle)
}
```

测试影响（已验证）：
- `router_test.go`（4 处 `NewRouter(Deps{})` 零值场景）：`GormDB == nil` → 整个 if-guard 块跳过 → probe 不触发 → 不需要 sqlmock。无回归。
- `router_dev_test.go` / `router_version_test.go` / `error_mapping_integration_test.go`（Deps{} 或仅注入 Signer / RateLimitCfg）：同上，`GormDB == nil` → probe 不触发。无回归。
- `router_ws_test.go`（注入 GormDB + sqlmock + SessionMgr 完整 deps）：probe 在新位置仍然执行一次（rooms 然后 room_members 顺序），sqlmock 期望串完全匹配。所有 8 个 ws probe 矩阵 case 都 pass：表存在 / 1146 缺表 / 1142 表权限 / 1044 schema 权限 → panic；1040 too-many-connections / driver bad connection → warn-and-continue。
- `room_handler_integration_test.go`：自建最小 router 不走 `bootstrap.NewRouter`，无影响。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改 `bootstrap/router.go` 把某个 handler 挂载条件从严 if-guard A 放宽到更弱 if-guard B 时**，**必须** **同步审视所有原本贴在 if-guard A 内的"该 handler 依赖资源（DB 表 / Redis key / 配置项）"的 fail-fast probe，把 probe 一并前移到 if-guard B 之前；否则放宽后那部分新覆盖的部署形态会失去 startup-time fail-fast 保护**。
>
> **展开**：
> - 修改前先列：本 handler 消费哪些资源（表 / Redis / 文件 / 网络端点）；哪些资源已经有 startup probe；那些 probe 当前在哪个 if-guard 下。
> - 把 probe 的 if-guard 当成"该资源消费者集合并集对应的最弱前置条件"，不是某个 feature flag。新加消费者 → 重新计算并集 → 调整 gate。
> - 修改后必须在所有"会进入 if-guard B 但不进入 if-guard A"的部署形态（HTTP-only 单测 fixture / 主机部署 / k8s pod template）下手动 mental-trace probe 的执行路径，确认 fail-fast 仍生效。
> - **反例**：bootstrap/router.go r3-r8 间 `wsTablesReady` 一直贴在 `if deps.SessionMgr != nil` 块内（WS gate），因为当时只有 WS gateway 消费 rooms / room_members 表 —— 这是合理的。11-3 dev-story 加了 `POST /api/v1/rooms` HTTP handler 后，SessionMgr 不再是消费者并集的唯一 gate，但 probe 没动 → SessionMgr=nil 部署形态的 fail-fast 失效。Claude 当时的思维盲区是把 "WS gate" 当成了"rooms/room_members 资源使用 gate"的同义条件 —— 一旦扩展消费者集合，这种同义关系就不成立。
> - **正例**：另一种合法修法是把 probe 显式拆成两次（HTTP 路径前一次 + WS 路径前一次）；本 story 选择了"上移到 GormDB 块顶部一次"路线，因为 probe 本身是幂等只读操作 + 共用同一份 sqlmock 期望串更简单。两种路线都能解决问题，关键判定是 probe 的 if-guard 必须 ⊆ 资源消费者集合的最弱前置条件。
> - **额外延伸**：bootstrap 期的"防御性 sniff" pattern（Story 10.3 r5 / r6 / r8 演化出来的）本质是把 schema 漂移从 request-time 1009 推到 startup-time CrashLoopBackOff，让运维告警通道立即触发；想清楚每个 sniff 的覆盖范围 = 本 lesson 的核心。
