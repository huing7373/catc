---
date: 2026-05-06
source_review: codex review on Story 10-3-ws-网关骨架 (epic-loop r8)
story: 10-3-ws-网关骨架
commit: 248adc7
lesson_count: 1
---

# Review Lessons — 2026-05-06 — WS 表 probe 错误分流：misconfig 必须 fail-fast，transient 才能 warn-and-continue（r8 收窄 r6）

## 背景

Story 10.3 WS 网关骨架在多轮 review 中反复打磨"启动期 backing table sniff"的错误分流策略。r5 用 `information_schema` sniff，r6 [P2] 反指 hardened DB user 没有 information_schema 权限会假阳性 panic，于是改成"直接 probe app table（`SELECT 1 FROM rooms LIMIT 1`）+ 把所有非 1146 错误一律 warn + continue"。本次 r8 [P1] codex review 反指 r6 这条"非 1146 一律 warn"过于宽松：直 probe app-table 路径下 1142 (ER_TABLEACCESS_DENIED_ERROR) / 1044 (ER_DBACCESS_DENIED_ERROR) 不再是 information_schema 副作用，而是真的 misconfig —— 部署到这种环境 WS feature 完全不可用而 healthcheck 看着健康 = 静默灾难，应该启动期 fail-fast 让 systemd / k8s CrashLoopBackOff 告警。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | wsTablesReady 应把 1142 / 1044 也归到 fail-fast，与 1146 同级 | high (P1) | error-handling | fix | `server/internal/app/bootstrap/router.go:95-115` + `router_ws_test.go` |

## Lesson 1: 当 probe 改路径时，原有 error 分流的"假阳性归类"假设可能不再成立

- **Severity**: high (P1)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/app/bootstrap/router.go:95-115`

### 症状（Symptom）

r6 把 wsTablesReady 的 probe 从 `SELECT COUNT(*) FROM information_schema.tables` 改成了直接打 app table（`SELECT 1 FROM rooms LIMIT 1`），同时把 error 分流定为"err nil → continue / 1146 → fail-fast / 其他 → warn + continue"。这里"其他 → warn + continue"的潜台词是"非 1146 错误大概率是 information_schema 权限副作用，不是真 misconfig"。

但 probe 路径改后，1142 (ER_TABLEACCESS_DENIED_ERROR) 和 1044 (ER_DBACCESS_DENIED_ERROR) 是直接打 `rooms` / `room_members` 这两张 app table 时返回的 —— 这就不是"information_schema 副作用"了，而是"app role 真的没有该表 SELECT 权限"或"app role 真的没有该 schema 访问权限"。这种部署上线后每次 WS 握手都会在 `RoomExists` / `IsUserInRoom` 处以 close 1011 失败，feature 完全不可用，但 server 启动正常 + healthcheck 看着健康 = 静默灾难。

### 根因（Root cause）

**probe 路径变化让"非 1146 错误"的语义集合发生了实质性收缩，但 r6 实装把"非 1146 一律 warn"这条结论从旧路径直接拷贝到了新路径，没重新审视分类**。具体说：

- 旧路径（information_schema）：1142 大概率是 sniff 的副作用（hardened DB user 没 info_schema 权限），把它当 transient 是合理的（app schema 本身可能正常）
- 新路径（直 probe app table）：1142 是直接打目标表时返回的，**只有一个解释** —— app role 没有该表 SELECT 权限。这是 misconfig，不可能"transient 一会儿就好"

r6 的 commit message 也只强调了"修 information_schema 假阳性"这一面，没强调"换路径后非 1146 错误的语义收窄"。审 r6 时如果有人按着新 probe 路径走一遍 1142 / 1044 各自的真实物理含义，会发现这两个错误号在新路径下应该归到 fail-fast 而不是 warn。

### 修复（Fix）

- 在 `mysqlErrCodeNoSuchTable=1146` 旁加常量 `mysqlErrCodeTableAccessDenied=1142` 和 `mysqlErrCodeDBAccessDenied=1044`
- 把 `wsTablesReady` 的 error 分流从单分支扩成 switch：1146 / 1142 / 1044 → 各自 log Error + return false（fail-fast）；其他 mysql 错误（如 1040 too-many-connections）+ 非 mysql.MySQLError → log Warn + continue（保持 transient 语义）
- 测试覆盖收窄：把原 `TestRouter_WSRoute_DoesNotPanic_OnAccessDenied`（1142 → 不 panic）翻转为 `TestRouter_WSRouteFailFast_OnTableAccessDenied`（1142 → panic）；新增 `TestRouter_WSRouteFailFast_OnDBAccessDenied`（1044 → panic）；新增 `TestRouter_WSRoute_DoesNotPanic_OnTransientTooManyConnections`（1040 → 不 panic）作为"transient warn 仍然存在"的对照锚点；保留 `TestRouter_WSRoute_DoesNotPanic_OnConnError`（非 mysql err → 不 panic）

before:
```go
if stderrors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrCodeNoSuchTable {
    slog.Error("ws backing table missing: schema drift detected (MySQL 1146)", ...)
    return false
}
slog.Warn("ws backing table probe non-fatal error ...", ...)
```

after:
```go
if stderrors.As(err, &mysqlErr) {
    switch mysqlErr.Number {
    case mysqlErrCodeNoSuchTable:        // 1146
        slog.Error(...); return false
    case mysqlErrCodeTableAccessDenied:  // 1142
        slog.Error(...); return false
    case mysqlErrCodeDBAccessDenied:     // 1044
        slog.Error(...); return false
    }
}
slog.Warn(...)  // 其他 mysql 错误 + 非 mysql 错误 → transient
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：当一个 helper 的 probe / query 路径在 review 中被换掉时，**必须重新分类原有 error 分流的每一档**，不能假设"原来归到 transient warn 的错误号在新路径下还是 transient"。

> **展开**：
> - probe 路径变化 = error 语义集合变化。同一个 MySQL error number 在不同 query 路径下指向不同物理含义（例如 1142 在 information_schema query 上是"sniff 副作用"，在 app-table query 上是"app role 真没该表权限"）
> - **misconfig vs transient 的判定标准**：问"这个错误如果不修能不能自己好"。能 → transient（warn + continue 让 transient flap 不升级成 CrashLoopBackOff）。不能 → misconfig（fail-fast 让运维立即看见）
> - **静默灾难场景**特别要警惕：startup OK + healthcheck OK + business path 100% fail。这种场景必须用启动期 fail-fast 暴露。typical 信号：错误是"权限 / 配置 / 认证"类，不是"网络 / 资源 / 时序"类
> - 错误号常量集中放在文件顶部 + 加 godoc 解释每个号在当前 probe 路径下的物理含义（不只是 MySQL 字典里的官方含义），让下一轮 review 一目了然
> - **反例**：
>   - 反例 1（本次踩坑）：r6 改路径后保留"非 1146 一律 warn"分支，里面包含 1142 / 1044 这种 misconfig，导致"app role 没 SELECT 权限"被当成 transient
>   - 反例 2：把所有 mysql 错误都升级成 fail-fast。1040 too-many-connections 是真的 transient，连接池抖动等会儿就好；这种 fail-fast 会让 startup 在压力下 CrashLoopBackOff
>   - 反例 3：用错误 message 字符串匹配（"access denied"）做分类。MySQL 错误 message 受 locale 和版本影响不稳定；只能用 Number 比较

---

## Meta: 本次 review 的宏观教训

r5 → r6 → r8 三轮反复在同一段 helper 上打磨，本质是**每轮只改了 probe 实现层，没把"error 分流"作为独立一层显式 review**。错误分流应该和 probe 实现解耦：

- **probe 层**：决定 query 怎么打（information_schema sniff vs 直 probe app table vs SHOW TABLES vs etc）
- **错误分流层**：决定每种错误码怎么处理（fail-fast vs warn vs continue）

每次改 probe 层，**必须把错误分流层重新走一遍每种错误码的物理含义**，写成"如果 probe 路径是 X，错误码 Y 的物理含义是 Z，处置策略是 W"的表格。r6 改 probe 时如果有这张表，1142 / 1044 在新路径下属于 misconfig 这个事实就不会被漏掉。
