---
date: 2026-05-06
source_review: codex review of Story 10.3 (review file `/tmp/epic-loop-review-10-3-r6.md`, r6)
story: 10-3-ws-网关骨架
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-06 — WS 表存在性 sniff 不能依赖 information_schema 权限（r6）

## 背景

Story 10.3（WS 网关骨架）r6 review 由 codex 提出 1 条 P2 problem：

- r5 修法引入的 wsTablesReady() 用 `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ('rooms','room_members')` 单 query sniff 表存在性，命中数 < 2 panic。
- 这条 query **要求** DB user 有 information_schema 的访问权限。但 prod hardened 部署常见做法是按"最小权限原则"只给 app schema 授权，**不**授权 information_schema。
- 后果：在这种部署里 wsTablesReady 的 query 失败 → panic → 整个 HTTP server 启动失败（**不只是** /ws，所有端点 /ping / /version / 业务 API 都挂）。
- 即"防御性 fail-fast"自己变成了"启动期假阳性 panic"，把 r3 → r5 → r6 的修复路径推回更糟糕的状态。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | wsTablesReady 改用直接 probe 表 + MySQL 1146 分流，不再 require information_schema 权限 | medium | architecture | fix | `server/internal/app/bootstrap/router.go`, `server/internal/app/bootstrap/router_ws_test.go` |

## Lesson 1: 启动期 schema sniff 不能 require 比业务路径更高的权限

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/bootstrap/router.go:31-115`（wsTablesReady） + `server/internal/app/bootstrap/router.go:327-336`（call site）

### 症状（Symptom）

r5 实装 wsTablesReady 用 `information_schema.tables` 查询：

```go
db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ('rooms','room_members')").Scan(&hitCount)
if err != nil { panic(...) }
if hitCount < 2 { panic(...) }
```

prod hardened DB user（最小权限原则：只 GRANT 在 app schema 上的 SELECT/INSERT/UPDATE/DELETE）→ 上面的 query 在 server 端返 access denied (1142) → wsTablesReady 进 query-error 分支 panic → NewRouter panic → main.go Run 不返回 → systemd / k8s CrashLoopBackOff，运维看到 "ws backing tables sniff failed" 但其实是权限问题不是表问题，**整个 HTTP server 启动失败（包括完全无关的 /ping / /version / 业务 API）**。

### 根因（Root cause）

把"防御性 sniff"和"业务请求路径"放到了**不同的权限路径**上。业务路径（RoomMemberRepo.RoomExists 等）只需要 app schema 内的表 SELECT 权限；但启动期 sniff 走 information_schema 元数据 query，要求一项**额外**的、业务路径不需要的权限。

最小权限原则下这条额外权限没人 GRANT —— 然后启动期 sniff 反过来阻断 server 起服务。这是经典的"防御代码自己引入更严重的故障"模式。

修复的本质是**让 sniff 走和它要保护的业务路径完全一样的权限路径**：sniff 直接 probe 业务路径会用的表，而不是去查 metadata catalog。这样如果 sniff 能成功，业务路径肯定能成功；如果 sniff 失败，可能是真的缺表（schema drift）也可能是连接 / 权限 transient，需要分流处理。

### 修复（Fix）

按 Option A（review 推荐）：

1. **改 wsTablesReady 实装为直接 probe**：对每张表（rooms / room_members）跑 `SELECT 1 FROM <table> LIMIT 1`。`fmt.Sprintf` 拼表名字符串字面量（**不是**用户输入，无 SQL 注入风险；表名是 hardcoded slice）。
2. **错误分流**（基于 MySQL error number，不解析 Message 字符串）：
   - `err == nil`（query 成功，含空表 / 有 row 都 OK）→ 表存在 → continue
   - `errors.As(err, *mysql.MySQLError) && err.Number == 1146`（ER_NO_SUCH_TABLE）→ 真的缺表 → 返 false → 调用方 panic
   - 其他 err（含 1142 access denied / 连接断 / context 取消）→ `slog.Warn` + continue（视为表存在但当前 probe 失败）。理由：把权限 / transient 错误当缺表会引入假阳性 panic；让后续 request 阶段 RoomExists 自然 fail 走 documented WS close 1011。fail-fast 仍然成立，只是从启动期推迟到 first request。
3. **签名调整**：函数从 `void` panic-direct 改成 `bool`（true=可用 / false=真的缺表）；调用方在 router.go 第 327 行 `if !wsTablesReady(deps.GormDB) { panic(...) }`。
4. **import 调整**：加 `"github.com/go-sql-driver/mysql"`（用 *MySQLError）+ `stderrors "errors"`（用 errors.As）+ `"fmt"`（拼 query 字符串）。原 `"github.com/huing/cat/server/internal/repo/mysql"` 改名 `repomysql` 以避开和 driver mysql 包名冲突；所有 `mysql.NewXxxRepo(...)` 改 `repomysql.NewXxxRepo(...)`。
5. **新增常量** `mysqlErrCodeNoSuchTable = 1146`，与 `repo/mysql/auth_binding_repo.go` 的 `mysqlErrCodeDupEntry = 1062` / `user_repo.go` 同模式（命名常量 + 只比 Number 不解析 Message）。
6. **测试改写** `router_ws_test.go`：
   - 删 `expectWSTablesShow`（不再用 information_schema query）
   - 新增四个 helper：`expectTableProbeOK`（空 rows + nil err = 表存在）、`expectTableProbeNoSuchTable`（MySQL 1146）、`expectTableProbeAccessDenied`（MySQL 1142）、`expectTableProbeConnError`（非 mysql 普通 error）
   - 原 4 个 case（mounted / rooms-missing / members-missing / both-missing）按新 query 重 mock，断言保持 panic 语义
   - **新增 2 个 case**：access-denied 不 panic + conn-error 不 panic（核心防回归 case，锁住 r6 修法不再被回退到 r5 形态）

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **启动期写"schema sniff" / "fixture sniff" / "config 自检"等防御性早期检测**时，**必须** **走和它要保护的业务路径完全一样的权限路径**，**禁止** require 额外的 metadata / catalog / system table 权限（如 `information_schema` / `pg_catalog` / `sys.*`）。
>
> **展开**：
> - 启动期检测的目标是"如果业务路径会失败，让运维**早**点知道"。这是**对运维友好**的设计。但如果检测自己 require 额外权限，就把"业务路径会失败"的弱依赖变成了"运维必须额外配置 GRANT"的强依赖，反而成本上升。
> - 直 probe 业务表的能力 = 业务路径成功的**必要条件**（不充分但必要）。这条是"权限路径同源"的硬要求。
> - **错误分流要细**：检测 query 失败时，区分"真的不存在"（schema drift，需要 fail-fast）和"transient / 权限 / 连接错"（应该让 request 阶段处理）。前者用 error code（如 MySQL 1146）锁定，后者一律宽松放行 + log warn。
> - **永远不要硬编码 error message 字符串匹配**（如 `strings.Contains(err.Error(), "doesn't exist")`）—— 不可靠（locale 不同 / 版本不同文案不同 / 不同 driver 不同）。用 `errors.As(err, &target)` 拿到具体 error type 后比 numeric code。
> - **反例 1**（本 lesson 修的 r5 形态）：`SELECT COUNT(*) FROM information_schema.tables WHERE ...` —— 看起来"高效单 query 直出整数计数"很优雅，但它假设 DB user 有 information_schema 权限。这是错的。
> - **反例 2**：用 `SHOW TABLES LIKE 'rooms'` —— 同样可能在 hardened 环境下受限（某些 DB 把 SHOW 列入需要 PROCESS / SHOW_DATABASES 的特权命令）。
> - **反例 3**：检测代码做"宽松失败 = 视为可用"时**只**对"普通错误"宽松、对"权限错误"严格 → 直接踩同款坑。本 lesson 的修法把所有非"目标 error code"都视为可用 → 是合适的宽松策略。
> - **反例 4**：错误分流时不**真的**用 errors.As / numeric code，而是 `if err != nil { return false }` 一刀切 → schema drift 和 transient 全部混到 false → 启动期 panic 假阳性回归。
> - **正例**：本 lesson 的 wsTablesReady 实装 —— `SELECT 1 FROM <table> LIMIT 1` 走 app schema 权限路径，error 分流用 `*mysql.MySQLError.Number == 1146` 唯一锁定 schema drift case，其他错误 log warn + continue。
