---
date: 2026-05-06
source_review: codex review r3 — /tmp/epic-loop-review-10-3-r3.md
story: 10-3-ws-网关骨架
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-06 — WS 路由必须 gate 在 backing tables migration 落地之后（启动期表存在性 sniff）

## 背景

Story 10.3 实装 WS 网关骨架 + 挂载 `/ws/rooms/:roomId` 路由。本 story 范围红线明确**不**做 rooms / room_members migration（钦定 Epic 11.2 才落地）。当前 `server/migrations/` 只到 `0006_init_user_step_sync_logs`，没有 `0007_init_rooms` / `0008_init_room_members`。但 Story 10.3 的 dev/r2 实装把 WS 路由挂载在了 main build 的常规 router 构造分支里 —— 任何走 0001-0006 migration 起服务的真实环境（dev / staging / prod）一旦客户端发起 WS 握手，会在 `RoomMemberRepo.RoomExists` / `IsUserInRoom` 阶段 SQL 报 "Table 'cat.rooms' doesn't exist" → Gateway close 1011 → feature 完全不可用。集成测试 fixture 在 docker 容器内 inline `CREATE TABLE` 跑得通，掩盖了 prod 路径失败。

codex review r3 [P1] 抓出该问题，要求"在 schema 落地前 gate 路由注册或 fail startup"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | WS 路由挂载早于 backing tables migration | high | architecture | fix（Option A：启动期表存在性 sniff） | `server/internal/app/bootstrap/router.go:222-232` |

## Lesson 1: 跨 epic 依赖的路由必须用启动期表存在性 sniff gate，而非"等 migration 落地再合并"

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/bootstrap/router.go` (`wsTablesReady` helper + WS route mount block)

### 症状（Symptom）

Story 10.3 完成后 prod 环境（用 repo 现存 0001-0006 migration 拉起）任意 WS 握手会触发：

```
ERROR Table 'cat.rooms' doesn't exist
WS close 1011 (server error)
```

集成测试因为 fixture 在容器内 `CREATE TABLE rooms / room_members` → 测试全绿，掩盖 prod 失败。

### 根因（Root cause）

跨 epic 依赖的"骨架先行 / migration 后置"模式有一个**隐藏失败模式**：骨架 story 把消费 future schema 的代码挂在了 main build / 默认路由树上，但 future schema 在当前 branch 不存在。集成测试用 ad-hoc fixture 绕开了该缺口，让 review / CI 都看不到失败 → 直到部署到真实 schema 才爆。

具体到本 case，触发条件 = **三件事同时成立**：

1. Story 范围红线钦定**不**做下一 epic 的 migration（rooms / room_members 是 Epic 11.2 才落地）
2. 但 router 注册分支无条件挂了消费这两张表的路由（`r.GET("/ws/rooms/:roomId", gateway.Handle)`）
3. 集成测试用 fixture inline `CREATE TABLE` 让测试绿 → 掩盖了 prod 失败路径

正确做法 = 把"路由注册"与"路由可用 SLO"分离 —— 路由注册条件必须包含**所有运行时依赖（包括 schema）**的就绪检测，而不只是依赖对象（如 SessionMgr）的 nil 检查。

### 修复（Fix）

在 `bootstrap/router.go` 加 `wsTablesReady(*gorm.DB) bool` helper：

```go
// 单 query 走 information_schema 一次性算两表命中数；
// hitCount >= 2 才判定 ready；任一 query 出错 → log warn + 返 false。
func wsTablesReady(db *gorm.DB) bool {
    var hitCount int64
    if err := db.Raw(
        "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ('rooms','room_members')",
    ).Scan(&hitCount).Error; err != nil {
        slog.Warn("ws table sniff failed", slog.Any("error", err))
        return false
    }
    return hitCount >= 2
}
```

WS 路由挂载分支加 gate：

```go
if deps.SessionMgr != nil {
    if wsTablesReady(deps.GormDB) {
        // ... 构造 RoomMemberRepo / Gateway ...
        r.GET("/ws/rooms/:roomId", gateway.Handle)
    } else {
        slog.Warn("WS route /ws/rooms/:roomId disabled: rooms/room_members tables not yet migrated (待 Epic 11.2 落地 migration)")
    }
}
```

测试文件 `router_ws_test.go` 加 4 个 case：两表都在 → 路由挂；任一缺 → 路由不挂；两表都缺 → 路由不挂 + 业务路由（/ping）仍 OK。用 sqlmock 注入 `information_schema.tables` 查询响应。

**SQL 选型踩坑**：codex review 建议用 `SHOW TABLES LIKE 'rooms'` + `db.Count(&int64)`，但 GORM `.Count()` 翻译为 `SELECT COUNT(*) FROM (...)`，对 SHOW TABLES 元数据查询不兼容（`SHOW TABLES` 不是合法子查询）。改用 `SELECT COUNT(*) FROM information_schema.tables WHERE ...` 单 query 直出整数，与 `Raw().Scan(&int64)` 完全兼容。information_schema 在 MySQL / MariaDB 都是标准元数据视图，`table_schema = DATABASE()` 自动绑定当前 connection 的 schema → 不依赖 hardcode schema 名。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"骨架先行 / future epic 才落 migration"的路由 / handler 代码**时，**必须**在 main build 路由注册段加**启动期表存在性 sniff**（用 `information_schema.tables` 单 query 探测），任一缺表则**跳过路由挂载 + log warn**（**不**fail-fast 启动）；**禁止**仅在集成测试 fixture 内 `CREATE TABLE` 让测试绿。
>
> **展开**：
> - 集成测试 fixture 能 `CREATE TABLE` 让测试绿不等于 prod 路径就绿 —— prod 用的是 repo 内 `migrations/` 目录，不是测试 fixture。任何"测试 fixture 与 production migration 不一致"的状态必须有运行时 gate 兜底
> - 路由注册条件 = `dep_objects != nil` ∧ `runtime_resources_ready`（含 schema、配置开关、外部服务连通性）；不能只 check 第一项
> - sniff 出错（DB 异常 / 元数据 query 失败） → 当作"表不存在"处理，避免 transient DB 故障让路由进入半坏态。warn 级日志够用，不需要 fail-fast（启动期 DB 已经 ping 通过，sniff 这一步出错是配置 / schema 异常，不是 connectivity 问题）
> - SQL 选型：用 `information_schema.tables` 单 query 算 hitCount，不要用 `SHOW TABLES LIKE` + GORM `.Count()`（GORM 把 Count 翻译成 `SELECT COUNT(*) FROM (subquery)`，SHOW TABLES 不能当子查询用）
> - **反例 1**：仅检查 `if deps.SessionMgr != nil { r.GET("/ws/...", handler) }` —— SessionMgr 是纯内存对象，永远 != nil，但底层 schema 没落地一样 1011
> - **反例 2**：在 NewGateway 内部做 sniff —— Gateway 已经构造了 SessionMgr / RoomMemberRepo 等，太晚（构造 cost 已支出 + warn 时机模糊）。sniff 必须在路由注册的**外层 if 条件**内，挂之前决策
> - **反例 3**：用 env-based gate（如 `cfg.WS.Enabled bool`）—— 配置漂移风险（部署侧忘记开 / 跑错 env）；启动期 sniff 是无配置自动化方案，安全得多
> - **反例 4**：fail-fast（启动期 sniff 失败直接 `os.Exit(1)`）—— 当前阶段两表不存在是**预期状态**，fail-fast 会让 dev/staging/prod 全起不来。warn-and-skip 是 safe-by-default
> - **反例 5**：用 `SHOW TABLES LIKE 'rooms'` + GORM `Raw().Count(&int64)` —— 实运行报 "Scan error converting string to int64"（SHOW TABLES 返回字符串列，Count 期望整数；codex review 给的伪代码不能直接用）
