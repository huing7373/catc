---
date: 2026-05-06
source_review: codex review of Story 10.3 (review file `/tmp/epic-loop-review-10-3-r5.md`, r5)
story: 10-3-ws-网关骨架
commit: ea60fbc
lesson_count: 2
---

# Review Lessons — 2026-05-06 — WS 路由"表存在性 gate"循环陷阱 + reconnect 替换的 broadcast 重叠窗口

## 背景

Story 10.3（WS 网关骨架）r5 review 由 codex 提出 2 条 functional 问题：
- **P1**：r3 修的"表存在性 warn-and-skip gate"在当前 migrations 集（0001-0006）下永远 false → /ws/rooms/:roomId 永远不挂 → client 拿 HTTP 404 而非 documented WS close codes，Story 10.3 在正常 server startup 完全不可用。和 r3 形成循环（r3 说"表不存在挂路由会 1011"，r5 说"表不存在不挂路由会 404"）。
- **P2**：Register 替换路径在锁内**先**把 NEW 加进 sessionsByRoom，**然后**才在锁外调 replaced.Close() → notifyClosed → Unregister 移除 OLD。两步之间 ListSessionsByRoomID 同时返 OLD + NEW，BroadcastToRoom 会同 user 双发；client 不能 dedupe（sessionID 不外漏）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | WS 表存在性"warn-and-skip gate"形成两难循环 → 拆 migration 提前到 10.3 + 改 fail-fast | high | architecture | fix | `server/migrations/0007_init_rooms.up.sql`, `server/migrations/0008_init_room_members.up.sql`, `server/internal/app/bootstrap/router.go`, `server/internal/app/bootstrap/router_ws_test.go`, `server/internal/infra/migrate/migrate_integration_test.go` |
| 2 | Register 替换路径锁内只加 NEW 不移 OLD → broadcast 双发窗口 | medium | architecture | fix | `server/internal/app/ws/session_manager.go`, `server/internal/app/ws/ws_test.go` |

## Lesson 1: WS 表存在性 gate 形成两难循环 → 拆 migration 提前到本 story + 改 fail-fast

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/bootstrap/router.go:53-91`（wsTablesReady） + `server/internal/app/bootstrap/router.go:259-290`（路由挂载段）

### 症状（Symptom）

r3 review 反馈：rooms / room_members migration 在 Epic 11.2 才落地，prod 用 0001-0006 起服务时 RoomMemberRepo SQL 报"table doesn't exist" → Gateway close 1011 → feature broken。
r3 修法：启动期 wsTablesReady() sniff 表 → 两表都存在才挂路由；任一缺则 warn-and-skip。

r5 review 反指：以"当前 migrations 集"为基准时 wsTablesReady() **永远** false → /ws/rooms/:roomId **永远**不挂 → client 拿 HTTP 404（不是 documented WS close codes） → Story 10.3 outside fixture 完全无法用。两轮 review 形成"表不存在挂路由 → close 1011" vs "表不存在不挂路由 → 404"的两难循环。

### 根因（Root cause）

把 story 的范围红线（"不实装 rooms / room_members migration，由 Epic 11.2 接管"）当作了**绝对约束**。这条红线本意是"业务 INSERT/UPDATE 逻辑（JOIN room / LEAVE room 事务）由 Epic 11.2 接管"，但被解读为"连 CREATE TABLE 也不能在本 story 做"。结果是：
- Story 10.3 的 WS 网关功能**实质依赖** rooms / room_members 两张表存在
- 不让本 story ship 这两张表 → story 自身不可部署 → 所有 outside-fixture 测试 / iOS 联调 / smoke test 都被堵
- r3 的 warn-and-skip 是在"不能 ship migration"前提下能想到的最善意 fallback，但代价是把功能性失败转成了静默失败（404 比 close 1011 更难诊断）

修复的本质是把"表存在"和"业务逻辑"在 epic 边界上**正交分解**：
- CREATE TABLE = schema 边界 → 应该跟"消费这张表的第一个 story"一起 ship
- 业务 INSERT/UPDATE/事务 = service 层边界 → 跟具体业务 story 一起 ship

### 修复（Fix）

1. **加 0007_init_rooms / 0008_init_room_members migrations**（参照 `docs/宠物互动App_数据库设计.md` §5.13 / §5.14 钦定字段集 + §6.12 状态枚举 + §7 索引）。仅 CREATE TABLE，不含任何业务 INSERT/UPDATE 逻辑（保留给 Epic 11.4 / 11.5）。
2. **router.go 移除 warn-and-skip**：路由**总是**挂载（`if deps.SessionMgr != nil` if-guard 仍保留）。
3. **保留 wsTablesReady() 为防御性早期检测**，但语义反转 → 表缺则 `slog.Error + panic`（fail-fast）。让 systemd / k8s CrashLoopBackOff 立即触发运维告警，而不是 server 起来后客户端在 WS 握手时拿 404 / close 1011 才间接发现 schema 漂移。
4. **router_ws_test.go**：把"表缺 → 路由跳过 → 404"测试改为"表缺 → NewRouter panic"，加 panic msg 内容断言。
5. **migrate_integration_test.go**：把表数量从 6 改 8、版本号从 6 改 8、加 `TestMigrateIntegration_RoomsAndRoomMembers_Schema` 验关键索引 / PK / 默认值。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **写 story dev / fix-review 时遇到"功能依赖某张表，但 story 红线说该表 migration 在后续 epic 落地"** 时，**必须** **把"CREATE TABLE 是否提前到本 story"作为 first-class 决策项**：如果当前 story 的功能（含集成测试 / smoke test / outside-fixture 部署）**实质依赖**该表存在 → 应该把 CREATE TABLE 拆出去提前；只把"业务 INSERT/UPDATE/事务"留给后续 epic。
>
> **展开**：
> - "表存在"和"业务逻辑实装"是**两个独立的 epic 边界**，可以分开 ship。CREATE TABLE 是 schema 边界，跟"第一个消费它的 story"走；业务 CRUD 跟"第一个写入它的 story"走
> - 出现"两难循环"症状（前一轮 review 说 A 不行，下一轮 review 说改成 B 也不行）时，**很可能根因不在 A / B 的取舍上，而在某个被当作绝对约束的红线本身需要松动**。停下来评估"红线最初是为了防止什么 scope creep？现在拆出 X 是不是仍然遵守红线精神？"
> - "warn-and-skip" / "silent fallback" 在功能依赖 backing infrastructure 的场景**几乎总是错的**。对比"silent skip 让用户拿 404"和"fail-fast panic 让 systemd 立即重启 + 运维告警"，后者**总是更安全**（通过 CrashLoopBackOff 把 schema 漂移变成 ops alert 而不是 client error 噪声）
> - **反例 1**：r3 阶段把红线"不做 migration"理解为绝对约束，没考虑"CREATE TABLE 提前 / 业务逻辑保留"的拆解 → 写出 warn-and-skip 形成循环
> - **反例 2**：在另一个场景把 wsTablesReady 改成 fail-fast 但**不**加 migrations → prod 起服务直接 panic、永远起不来。fail-fast 的前提是"在期望路径上表存在"；只有 migrations 提前 + fail-fast 同时做，才是闭环修法
> - **反例 3**：把"防御性早期检测"和"运行时 fallback"概念混淆。防御性检测应该在启动期一锤定音（panic on missing），不应该在每次 request 路径上做（运行时 fallback 该用别的机制）

## Lesson 2: Register 替换路径锁内只加 NEW 不移 OLD → broadcast 双发窗口

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/session_manager.go:142-205`（Register） + `removeFromIndicesLocked`

### 症状（Symptom）

同 user 重连场景：
1. NEW 在 Register 锁内被加进 sessionsByID + sessionsByRoom + userToSessionID
2. 锁释放
3. 锁外 replaced.Close() 启动
4. replaced.Close() → notifyClosed → Unregister(oldID) 才把 OLD 从 sessionsByRoom 移除

在 t2 ~ t4 之间，sessionsByRoom[roomID] 同时含 OLD + NEW（同 user）。BroadcastToRoom（Story 10.5 起作用）在该窗口期把同一条消息**同时**发给 OLD 和 NEW；client 不能 dedupe（sessionID 不外漏到 client）→ 用户看到双倍消息 / 副作用。

### 根因（Root cause）

review r2 P1 修的"reconnect 替换路径必须触发 onUnregister 钩子"修复采用了"保留 OLD 全部索引到 oldS.Close() 跑完"的策略，是**正确**的（让 Unregister(oldID) 走标准索引清理 + 钩子触发路径）。但这个策略把 OLD 在 sessionsByID 和 sessionsByRoom 的生命周期**绑在一起**了，没意识到 broadcast 视角（消费 sessionsByRoom）和 hook-trigger 视角（消费 sessionsByID）有不同的语义需求：
- broadcast 视角应该**立即**只见 NEW（不能在 reconnect 中场看到 OLD）
- hook-trigger 视角需要**保留** OLD 在某个索引（让 notifyClosed → Unregister 找得到）

把两个索引的 OLD 移除时机**绑死**等价于必须在某一边妥协：r2 修法保 hook，代价是 broadcast 双发；如果反过来在锁内移 sessionsByRoom + sessionsByID，hook 就漏调。

正确的解：**拆开两个索引的 OLD 移除时机**：
- sessionsByRoom 在 Register 锁内**同步**移除（broadcast 立即只见 NEW）
- sessionsByID 保留到 oldS.Close() → Unregister(oldID)（让 hook 触发路径 lookup 成功）

### 修复（Fix）

```go
// session_manager.go Register 替换分支
var replaced *Session
if oldID, ok := m.userToSessionID[s.userID]; ok {
    if oldS, ok2 := m.sessionsByID[oldID]; ok2 {
        replaced = oldS
        // 锁内**同步**移除 OLD 在 sessionsByRoom 的索引（broadcast 立即只见 NEW）
        // **不**触动 sessionsByID（让后续 Unregister(oldID) 触发 onUnregister）
        if room, ok3 := m.sessionsByRoom[oldS.roomID]; ok3 {
            delete(room, oldID)
            if len(room) == 0 {
                delete(m.sessionsByRoom, oldS.roomID)
            }
        }
    }
}
// ... 后续 NEW 注入索引 + 锁外 replaced.Close() 不变 ...
```

`removeFromIndicesLocked` graceful 处理"sessionsByRoom 已被移除但 sessionsByID 还在"的状态：Go map delete 对 missing key 是 no-op，不需要额外 if-guard。

加 2 条防回归测试（`ws_test.go`）：
- `TestSessionManager_Reconnect_NoDoubleBroadcastWindow`：用 WithRegisterHook 在 NEW Register 完成时 sample manager 状态，断言 ListSessionsByRoomID 只返 NEW 不返 OLD（同 room）
- `TestSessionManager_Reconnect_CrossRoom_OldRoomImmediatelyEmpty`：cross-room 变体，断言 OLD 的 roomA 在 NEW 注册完成时已立即为空

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **设计或修复"双索引（多视角索引）数据结构的元素移除时机"** 时，**必须** **针对每个视角（消费方）独立分析"该视角希望何时看到该元素"，不能假设所有索引共享同一个移除时机**。
>
> **展开**：
> - "双索引"或"多索引"数据结构（如 sessionsByID + sessionsByRoom + userToSessionID）的常见陷阱：开发者把"添加"和"移除"都当作单一事务对所有索引执行，但**不同消费视角对元素生命周期的语义需求可能不同**
> - 设计步骤：(1) 列出所有消费视角及其消费的索引；(2) 对每个视角问"在 reconnect / replace / migrate 这种过渡操作中，该视角希望何时看到 OLD 消失？" (3) 找到所有视角的"OLD 消失时机"约束，看是否冲突
> - 如果不同视角对 OLD 消失时机要求冲突，**应该拆开各索引的移除时机**而不是在某个视角妥协。锁内移除"要求立即消失"的索引；保留"等异步路径处理"的索引
> - **反例 1**：r2 修复 onUnregister 钩子漏调时，把 OLD 在所有 3 个索引（byID / byRoom / userToSessionID）的清理统一推迟到 oldS.Close() 跑完。其中 userToSessionID 必须**立即**指向 NEW（"按 user 找当前 session"业务），byRoom 必须**立即**只见 NEW（broadcast），但 byID 必须**保留**到 hook 触发完。结果：r2 把 userToSessionID 在锁内覆盖（正确），byRoom 和 byID 都推迟（前者错后者对）；r5 修复就是把 byRoom 也提前到锁内
> - **反例 2**：用"短暂双倍 acceptable / client 自己 dedupe"自我安慰。任何"client 自己 dedupe"的论证都需要 client **能**dedupe（即有可观察的 dedupe key 暴露）。本场景 sessionID 不外漏，client 没有 dedupe key → 该自我安慰不成立
> - **反例 3**：在锁内调 oldS.Close() 试图同步触发 hook 触发路径。会形成 lock 重入死锁（Close → notifyClosed → Unregister 拿 m.mu 写锁，但 Register 还持着该锁）。锁外 Close + 拆索引时机才是正确解

---

## Meta: 本次 review 的宏观教训

两条 finding 的根因都是**单一约束被解读为绝对** + **未对消费视角做正交分解**：
- P1：把 story 红线"不做 migration"当作绝对约束 → 把不可拆的功能依赖塞进 warn-and-skip
- P2：把"OLD 移除时机"当作单一事务 → 不同视角的语义被绑死

通用反预防：**写 fix 之前先列出"所有受影响的消费视角 + 各自对正确性的要求"**，再判断是否能用单一时间点 / 单一约束满足全部；如果不能，拆。
