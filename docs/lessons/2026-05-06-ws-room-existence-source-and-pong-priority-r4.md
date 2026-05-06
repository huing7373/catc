---
date: 2026-05-06
source_review: codex review r4 on Story 10-3 (WS Gateway Skeleton)
story: 10-3-ws-网关骨架
commit: e7cca25
lesson_count: 3
---

# Review Lessons — 2026-05-06 — WS room 存在性来源 / pong 优先级 buffer / sessionID logger 字段（10-3 r4）

## 背景

Story 10.3（WS 网关骨架）r4 codex review 命中 3 条 protocol-level 正确性问题：
①RoomExists placeholder 实装查 `room_members` 而非 `rooms` 表，让 archived rooms
残留 memberships 时仍 accept WS 连接（应 close 4004）；②handlePing 走和业务 msg
共享的 sendChan，buffer 满时 pong 被静默丢弃，client 可能误判 socket dead 而
重连风暴；③newSession 把 `sessionID=""` 注入 logger，Register 用 `sessionID_replay`
非标 key 叠加而不替换，破坏 grep "sessionID=<id>" 关联模式。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RoomExists 应查 rooms 表而非 room_members | P2 | architecture | fix | `server/internal/repo/mysql/room_member_repo.go:91-105` |
| 2 | handlePing pong 在 sendBuffer 满时被丢弃 | P2 | architecture | fix | `server/internal/app/ws/session.go:330-345` |
| 3 | newSession logger 残留 sessionID="" + Register 用 sessionID_replay 叠加 | P3 | observability | fix | `server/internal/app/ws/session_manager.go:164-167` |

## Lesson 1: RoomExists 必须查"权威表"（rooms）而不是"派生表"（room_members）

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/room_member_repo.go:91-105`

### 症状（Symptom）

Gateway.Handle 用 `RoomExists(roomID)` 决定是否 close 4004 (room not found)。
原 placeholder 实装 = `SELECT 1 FROM room_members WHERE room_id=? LIMIT 1`，把
"room_members 有任何成员行" 当作 "room 存在"。Story 11.2 落地 rooms 表 +
room close 事务后，archived 房间（rooms.status=closed 或 rooms 行已删）若
room_members 还有 stale 行（如 cleanup 失败 / race），RoomExists 仍返 true，
WS 握手通过 → server 接受了一个本应被拒的连接，违反 V1 §12.1 校验顺序的语义。

### 根因（Root cause）

placeholder 实装时贪图便利：rooms 表本 story 阶段没建（migration 11.2 才落地），
集成测试 fixture 也是 setup 期临建，于是把 "存在性" 投影到了已经存在的
room_members 表上。这其实是把"实体存在"语义与"实体派生关系存在"语义合二为一 ——
对当前路径恰好等价（创建 room 必同时插入 ≥1 member），但在引入 room close /
member leave 事务后两者会分叉。"接口语义必须基于权威表，不基于派生表"是
DB 设计的硬约束 —— 派生表的存在性 ≠ 主体存在性。

### 修复（Fix）

`room_member_repo.go::RoomExists` 改为查 `rooms` 表：

```go
// before
err := db.Raw("SELECT 1 FROM room_members WHERE room_id = ? LIMIT 1", roomID).Scan(&dummy).Error

// after
err := db.Raw("SELECT 1 FROM rooms WHERE id = ? LIMIT 1", roomID).Scan(&dummy).Error
```

**节点 4 阶段简化决策**：不带 status 过滤。理由：
- rooms.status 字段已在 `数据库设计.md §6.12` 钦定（1=active, 2=closed），但
  migration 由 Story 11.2 落地；status 过滤逻辑交给 11.2 实装期细化（届时
  改为 `... AND status = 1`）
- 当前节点 4 阶段没有 status=closed 的房间存在路径（创建 / 关闭事务由
  Epic 11.3 / 11.6 才落地），即便不带过滤也不会误判
- 集成测试 fixture 用 VARCHAR(16) DEFAULT 'active' 占位（与 prod TINYINT
  schema 不一致），不带过滤让本 placeholder 实装兼容两种 schema

加 3 条 sqlmock 单测覆盖：rooms 存在 + memberships 有 → true；rooms 不存在 +
memberships 有 → false（核心 fix 验证）；rooms 存在 + 空 → true（边界）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 `<Entity>Exists` / `<Entity>NotFound` 类校验时，
> **必须**查"权威实体表"（rooms / users / pets / chests），**禁止**查"派生关系
> 表"（memberships / bindings / lookups）。
>
> **展开**：
> - "权威表"= 数据库设计.md 里被建模为主体的表（有 PK = id 的）
> - "派生表"= 表达 N:N 关系或外键映射的表（PK 是复合主键 / 外键组合）
> - 即便当前 placeholder 阶段两者**恰好**有相同存在性投影（room 必有 ≥1 成员），
>   也**不**允许查派生表偷懒 —— 一旦上游事务（关闭 / 离开 / 撤回）让两者分叉，
>   下游存在性判定就会假阳性
> - 文档不在 frozen 状态时（如 status 字段值集还没钦定），**不写过滤条件 +
>   显式 TODO 标 11.2 实装期补**比"猜一个值写死"安全
> - **反例**：`SELECT 1 FROM <derived_table> WHERE <fk>=? LIMIT 1`
>   作为 EntityExists 实装

## Lesson 2: WS protocol-level 消息必须有独立 buffer 与业务消息隔离

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/session.go:330-345`（handlePing）+ writeLoop / Send

### 症状（Symptom）

`handlePing` 经 `Send(bytes)` 把 pong 入队和业务消息共享的 sendChan（容量 32）。
当业务消息瞬时积压（snapshot / broadcast / emoji 多发）让 sendChan 满时，
`Send` 走 `default:` 路径返 `ErrSessionSendBufferFull`，pong 被静默丢弃 +
log warn。client 收不到 pong → 心跳超时 → 主动 close 重连，但实际上 socket
完全健康。在 Story 10.5 BroadcastToRoom 落地后会成为热点 race。

### 根因（Root cause）

把"心跳"和"业务广播"看作同质消息流，复用同一 buffer 节省字段。但 protocol-level
msg 的语义优先级**远高于**业务 msg：业务 msg 丢失 = 用户体验降级（少一条
emoji），心跳 msg 丢失 = 整个连接被 client 误判 dead。fire-and-forget 的
"满了就丢" 策略适用于业务 msg（snapshot 可在重连后重发），**不**适用于心跳
（client 没有重发心跳 → 期望连接的能力，超时即 close）。

### 修复（Fix）

加 `sendPriorityChan` 独立 buffer + `SendPriority(msg)` 方法：

```go
// session.go
const sendPriorityChanCapacity = 4

type Session struct {
    sendChan         chan []byte
    sendPriorityChan chan []byte // protocol-level msg 独立 buffer
    // ...
}

func (s *Session) SendPriority(msg []byte) error {
    s.sendMu.RLock()
    defer s.sendMu.RUnlock()
    if s.closed.Load() { return ErrSessionClosed }
    select {
    case s.sendPriorityChan <- msg: return nil
    default: return ErrSessionSendPriorityBufferFull
    }
}
```

writeLoop 改为两段式 priority select（Go 没有内建优先级 select）：

```go
for {
    select {
    case msg, ok := <-s.sendPriorityChan: // 优先非阻塞 drain
        if !ok { return }
        if err := s.writeFrame(msg); err != nil { return }
    default:
        select { // priority 没数据 → 阻塞等任意一边
        case msg, ok := <-s.sendPriorityChan: ...
        case msg, ok := <-s.sendChan: ...
        }
    }
}
```

Close 同时关 sendChan + sendPriorityChan（持 sendMu.Lock）。
handlePing 改用 `s.SendPriority(bytes)`。

容量 4 的理由：节点 4 阶段单 Session 的 protocol msg 频率上限 = 60s 一次
ping/pong；最坏 race 2 次 ping → 4 容量有 2x buffer。writeLoop 优先消费
priority，理论不会满。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 WebSocket / 长连接的写路径上，**必须**给
> protocol-level 消息（heartbeat / control frame / 协议错误回执）独立的 priority
> chan，**禁止**让它们与业务消息共享同一 fire-and-forget buffer。
>
> **展开**：
> - "protocol-level msg" = 客户端协议状态机依赖、丢失即引发协议层副作用（重连 /
>   超时关闭）的消息：心跳回应、auth 回执、close error frame
> - "业务 msg" = 应用层数据，丢失最坏 = UX 降级但连接仍健康：snapshot / broadcast
> - writeLoop 用两段式 priority select 实装：先 nested 非阻塞 select drain
>   priority chan；priority 空才阻塞等任意一边。Go 没有 builtin priority select 语法
> - priority chan 容量小（4-8）即可：单 Session 的协议 msg 频率有界（如
>   每 N 秒一次），且 writeLoop 优先消费让它不会持续积压
> - **反例**：`func handlePing() { s.Send(pong) }` 走和业务共享的 sendChan，
>   依赖 fire-and-forget 默认 fallback；buffer 满时 pong 被丢 + log warn
>   假装不重要

## Lesson 3: contextual logger 字段不可"叠加替换 with 别名 key" —— 必须用 canonical key 唯一注入

- **Severity**: P3
- **Category**: observability
- **分诊**: fix
- **位置**: `server/internal/app/ws/session.go::newSession` + `session_manager.go::Register`

### 症状（Symptom）

newSession 构造时 sessionID 还没分配（由 Register 内部生成），但 logger 已经
`With(slog.String("sessionID", sessionID))` 注入了**空字符串**。Register 拿到真实
sessionID 后再 `s.logger = s.logger.With("sessionID_replay", sessionID)` 叠加 ——
slog.Logger.With 是**叠加**不是**替换**，结果所有后续 session 日志同时带
`sessionID=""` + `sessionID_replay=<real>` 两个字段。文档钦定的 `grep
"sessionID=<id>"` 关联模式失效（要么命中空字段要么完全 miss），reconnect /
session 排障的标准 SOP 直接坏了。

### 根因（Root cause）

误以为"先用占位值 + 后期 With 叠加替换"是合法 slog 用法。slog.Logger 没有
"删除字段 / 替换字段" API（设计上 logger 是不可变的）；With 永远是追加
attribute。"延迟字段值"必须**延迟到字段名第一次出现的时机**，不能"先占位
后 alias"。这是 slog 与传统 logger（log4j 的 MDC）行为差异 —— MDC 可以覆盖，
slog 不可以。

### 修复（Fix）

newSession 不在 logger 注入 sessionID 字段（只注入 userID / roomID）；Register
拿到真实 sessionID 后用 canonical "sessionID" key 唯一一次 With 叠加：

```go
// session.go::newSession
logger: logger.With(slog.Uint64("userID", userID), slog.Uint64("roomID", roomID)),
// 不 With sessionID（留给 Register 加）

// session_manager.go::Register
s.sessionID = sessionID
s.logger = s.logger.With(slog.String("sessionID", sessionID))
```

删除 "sessionID_replay" 这个非标 key。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 slog contextual logger 注入字段时，**禁止**用
> 占位空值预注入再后期 With 叠加；**必须**等字段值确定后再用 canonical key
> 唯一一次 With 注入。
>
> **展开**：
> - slog.Logger.With 是**追加** attribute，不是 MDC 那种 key→value map 覆盖；
>   两次 With 同名 key 会让日志同时带两个同名 attribute
> - 处理"字段值延迟到 N 步之后才知道"的常见模式：构造 logger 时**不**注入
>   该字段；等值确定的步骤再 `s.logger = s.logger.With(canonical_key, value)`
> - 避免起 alias key（如 "sessionID_replay" / "userID_v2"）来"绕开"覆盖问题 ——
>   所有依赖 grep / 结构化查询的下游工具都钦定 canonical key
> - **反例**：
>   ```go
>   // newXxx
>   logger.With("xxxID", "")  // 占位空字段
>   // later
>   s.logger = s.logger.With("xxxID_v2", realID)  // 叠加 alias key
>   ```

---

## Meta: 本次 review 的宏观教训

三条 finding 都属于"**placeholder 阶段语义偷懒**"：①让 EntityExists 偷懒查
派生表；②让 protocol msg 偷懒共享业务 buffer；③让 logger 字段偷懒占位 alias。
共同模式 = "当前 path 恰好 work，但语义不正确，下个 story 引入并发 / 分叉
路径就 break"。**预防 meta-rule**：placeholder 实装的接口**语义**必须从一开始
就正确（即使内部实装简化），因为接口语义会被多 caller 复用，下游错位 fix
的成本 >> 上游正确实装的成本。
