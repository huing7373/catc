---
date: 2026-05-06
source_review: codex r7 review on Story 10-3-ws-网关骨架（/tmp/epic-loop-review-10-3-r7.md）
story: 10-3-ws-网关骨架
commit: eeb3d11
lesson_count: 2
---

# Review Lessons — 2026-05-06 — 房间存在校验补 status 过滤 & writeLoop priority 配额防 starvation

## 背景

Story 10-3 r7 review 针对 r6 fix 后的 WS 网关骨架代码，覆盖 `RoomMemberRepo.RoomExists` 与 `Session.writeLoop`。前者是 r4/r5 改造遗留的逻辑空缺（status 列已在 0007 migration 引入但未在 query 中过滤），后者是 r4 引入 priority chan 时的设计盲区（严格优先 → 配额缺失 → buggy client 可 starve sendChan）。两条均为 valid 问题，本次全部 fix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RoomExists 未过滤 closed 房间 | P2 | architecture | fix | `server/internal/repo/mysql/room_member_repo.go:106-121` |
| 2 | writeLoop priority chan 严格优先 → 可 starve sendChan | P3 | perf | fix | `server/internal/app/ws/session.go:427-464` |

## Lesson 1: 引入 status 列后必须同步加 query 过滤，不能"先 schema 后业务"

- **Severity**: P2 (medium-high)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/room_member_repo.go:106-121`

### 症状（Symptom）

r5 在 `migrations/0007_init_rooms.up.sql` 引入 `rooms.status TINYINT (1=active / 2=closed)`，但 `RoomExists` 仍是 r4 的 placeholder query：`SELECT 1 FROM rooms WHERE id = ? LIMIT 1`，**没有 status 过滤**。结果：已 close 的房间（status=2）仍被 RoomExists 判为存在 → Gateway.Handle 接受 WS 连接 + 下发 room.snapshot，违反 Story 10.3 AC2 钦定的 close 4004 (room not found) 协议路径。当 prod / test 数据中存在 archived rooms（手动 close 或 Epic 11.6 close 事务标记）+ stale room_members 残留行时，"closed 房间继续接受 WS 连接"会被 client 看成"房间复活"，制造数据/状态混淆。

### 根因（Root cause）

r4/r5 的 fix 在两个层面分开推进：
- r4：把 RoomExists 从 "查 room_members" 改为 "查 rooms"（修了"查错表"问题）
- r5：补上 0007 migration（修了"prod 表不存在"问题）

但**两次 fix 都没有把 r5 引入的 status 列拉到 r4 的 query 里**。理由是 r4 注释里写了"节点 4 阶段没有 closed 路径，等 Epic 11.2 再补"—— 这个推迟是错的，因为：

1. 即使本 story 阶段没有 close 事务，**任何**手动 / 测试 / 未来的 close 都会立刻让 RoomExists 失真；推迟修等于留下未来才会发现的隐藏 bug
2. status 过滤逻辑是**纯 SQL 谓词**，与业务 close 事务**完全解耦** —— 加 `WHERE status = 1` 不需要等任何 service 层代码就绪
3. "schema 已存在" 是 query "可以过滤" 的充分条件；不过滤就是放着语义漏洞

更深层：当 schema 引入新约束列时，**所有读路径必须立刻同步更新**（不只是写路径），否则就是"schema 上了但语义没上"的半成品。

### 修复（Fix）

`RoomExists` query 加 `AND status = 1`：

```go
// before
Raw("SELECT 1 FROM rooms WHERE id = ? LIMIT 1", roomID)

// after
Raw("SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1", roomID)
```

同步把集成测试 fixture（`ws_integration_test.go startMySQLWithRoomMemberFixture`）从 `status VARCHAR(16) DEFAULT 'active'` 改为 `status TINYINT NOT NULL DEFAULT 1`，让 fixture 与 0007 migration schema 一致，`status = 1` 谓词在两端等价。

加单测 `TestRoomMemberRepo_RoomExists_FalseOnClosedRoom` 直接验证 status 过滤生效。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **migration 引入新过滤列（如 status / deleted_at / tenant_id）** 时，**必须**在同一个 PR/commit 中**同步更新所有读路径 query**（即便业务 close/delete/multi-tenancy 事务还没落地）。

> **展开**：
> - schema 加约束列 ≠ 列已在语义层生效；**query 加谓词**才是。两步缺一不可
> - 不要用"未来 Epic 才会真的写入此列"作为推迟过滤的理由 —— 任何 manual / test / migration default 写入都会立刻暴露语义漏洞
> - 同时检查测试 fixture / seed 数据：fixture schema 必须与 prod migration **完全等价**，否则 query 谓词在测试与 prod 行为不一致
> - **反例**：为新列写 migration 但 query 注释"等业务事务上线再加过滤" → 未来 closed 行被误判 active，引发隐蔽 bug；测试 fixture 用 VARCHAR 占位但 prod 用 TINYINT，`WHERE status = 1` 在测试通过但 prod 行为漂移

## Lesson 2: priority chan 必须配 quota，否则 writeLoop 严格优先就是 starvation 设计漏洞

- **Severity**: P3 (low-medium)
- **Category**: perf
- **分诊**: fix
- **位置**: `server/internal/app/ws/session.go:427-464`

### 症状（Symptom）

r4 给 Session writeLoop 加了 priority chan（`sendPriorityChan`），让 protocol-level 消息（pong）比业务消息（snapshot / broadcast / emoji）优先送达，避免 client 在业务 buffer 压力下因收不到 pong 而误判 connection dead → reconnect 风暴。但 r4 的实装是**严格优先**（fast path 永远先 drain priority chan，priority 为空才走双分支）。

后果：buggy / malicious client 持续以高于 server drain 速率发 ping → handlePing 持续填 sendPriorityChan → priority chan 始终非空 → writeLoop 永远走 fast path 消费 priority → sendChan 中堆积的 room.snapshot / broadcast / emoji 等业务消息**永远不被消费**。connection 心跳层看起来健康（client 持续收 pong），但 client **永远收不到真实业务更新**。这是典型的 priority queue starvation bug。

### 根因（Root cause）

priority queue 模式需要**同时满足两个不变量**：

1. 高优消息在压力下不被低优消息阻塞（priority 设计目的）
2. 低优消息在高优消息流持续时仍能被处理（不被 starve）

r4 实装只满足 #1，没满足 #2。Go 没有内建 priority select 语法，标准的"两段 nested select"模式天然偏向 #1 → 必须**人工加 quota**（连续 N 次 priority 后强制让低优一次机会）才能恢复 #2。这是 priority queue 设计的**通用约束**，不是 ws 模块的特殊问题。

更深层：把"优先级"实装为"严格优先"是新人/AI 常见误解 —— 真正生产级 priority queue 都有 anti-starvation 机制（如 quota / aging / weighted fair queueing）。

### 修复（Fix）

`writeLoop` 加 `maxConsecutivePriority = 4` 配额：连续 drain priority 4 次后，第 5 次必须走双分支阻塞 select（priority + sendChan 平等竞争，go select 自带随机选择 → ~50% 概率让 sendChan 被命中），消费完后清零 quota 计数。

```go
const maxConsecutivePriority = 4

func (s *Session) writeLoop() {
    consecutivePriority := 0
    for {
        if consecutivePriority < maxConsecutivePriority {
            select {
            case msg, ok := <-s.sendPriorityChan:
                // ... write
                consecutivePriority++
                continue
            default:
                // priority 空，让出
            }
        }
        // 双分支阻塞 select（priority + normal 平等）
        select {
        case msg, ok := <-s.sendPriorityChan:
            // ...
            consecutivePriority = 0
        case msg, ok := <-s.sendChan:
            // ...
            consecutivePriority = 0
        }
    }
}
```

quota = 4 与 `sendPriorityChanCapacity = 4` 对齐："一次 priority buffer 容量 worth 的 pong 发完，就给业务消息一次让路机会"。

加测试 `TestSession_WriteLoop_DoesNotStarveSendChan`：先 fill priority chan 容量 4 + enqueue 2 normal 到 sendChan + 持续灌 28 条 priority（共 32），客户端读全部消息后断言"至少有 1 条 normal 出现在 priority 流中间（前后都有 priority）"，证明 quota 让 sendChan 在持续 priority 灌入下仍能 drain。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **设计 priority queue / priority chan 消费循环** 时，**必须**同时实装**反 starvation 机制**（quota / aging / fair queueing），**禁止**使用纯严格优先（"高优非空就永远不读低优"）。

> **展开**：
> - 严格优先 = 设计漏洞，**不是** "MVP 简化"。在压力测试 / 恶意 client 场景下立刻暴露
> - 最简实装：连续 N 次高优后强制走"平等双分支 select" 一次，N 与 priority chan 容量同量级（如本案 4）
> - Go select 多分支随机性是天然的 fair queueing 工具：双分支 select 在两边都有数据时 ~50/50 命中，正好让 starve 的一方有机会
> - 不要用"调高低优 chan 容量"作为 starvation 的解 —— 那只是延迟问题，不解决问题（高优持续灌入只会让低优 chan 越积越满）
> - 不要用"高优入口限流"作为 starvation 的解 —— 限流违反协议语义（如 V1 §12.3 钦定 pong 必发）
> - **反例**：实装 priority queue 时只写 `select { high; default: select { high / low } }` 嵌套，没有 quota → high 持续灌入 → low 永不消费；或者把 priority 字段改成"高优消息+1 容量"想用容量限制冷却 priority 流，等于把锅推给上游

---

## Meta: 本次 review 的宏观教训

两条 finding 都是"**在原设计基础上叠加新功能时，新功能改变了原设计的某个边界条件，但没回头修原设计的对应处**"：

1. r5 引入 status 列改变了 rooms 表的"存在 = 有效"等价关系，但 r4 的 RoomExists query 没回头加过滤
2. r4 引入 priority chan 改变了 writeLoop 的"FIFO consume"等价关系，但 priority 优先模式没考虑高频 priority 灌入下低优消息的反 starvation

**抽象规则**：每次给已有结构（DB schema / chan / queue / cache）**叠加新约束 / 新分类**时，**必须**列出"哪些既有 query / consumer / lookup 路径会被新约束影响"，**逐一**同步更新；不能依赖"后续 epic / 业务事务上线时再补"——因为"叠加约束"与"叠加业务"是两件事，前者是纯结构调整，必须立即一致。
