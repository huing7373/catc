---
date: 2026-05-08
source_review: codex review round 9 of story 11-1（/tmp/epic-loop-review-11-1-r9.md）
story: 11-1-接口契约最终化
commit: 9f3a569
lesson_count: 2
---

# Review Lessons — 2026-05-08 — snapshot 隔离 ≠ 锁；ACL guard 需 FOR SHARE 锁不止 snapshot；跨事务状态字段 drift 需 FOR UPDATE 串行化（11-1 r9）

## 背景

Epic 11 Story 11.1（接口契约最终化）r4 锁定写事务并发 race 兜底（DELETE RowsAffected==0 + INSERT UNIQUE 兜底）；r8 锁定 GET /rooms/{roomId} 多次 SELECT 的 snapshot 一致性（ACL+roster 同 REPEATABLE READ 事务）。r9 codex review 抓出 r8 修复的两个深层不足：

1. **r8 的 snapshot 隔离不充分**：snapshot 仅保证事务**内部**两次 SELECT 看到同一时刻状态，**不**保证 caller 在 HTTP 响应发出时仍是成员（外部一致性破坏）。
2. **r4 / r8 都没考虑 join+leave 跨事务 race 导致的 rooms.status 与 room_members drift**：A leave 事务删 row → 看 remaining=0 但未 UPDATE status → B join 事务锁 rooms 看 status=1 → B insert + commit → A UPDATE status=2 commit，结果 rooms.status=closed 但 room_members 非空。

两条 finding 都是真 P1 并发漏洞，必须修。本轮是 review/fix 上限 10 轮里的第 9 轮，要求一次性把所有相关位置（V1接口设计.md / 数据库设计.md / 时序图设计.md / story 11-1 spec）同步到位。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | GET /rooms/{roomId} ACL guard 需 FOR SHARE 锁，snapshot 不够 | P1 (high) | security/concurrency | fix（路径 A：FOR SHARE + REPEATABLE READ） | `docs/宠物互动App_V1接口设计.md` §10.3 / `docs/宠物互动App_数据库设计.md` §8.8 / story spec §10.3.2 |
| 2 | join+leave 跨事务 rooms.status drift 需 FOR UPDATE 串行化 | P1 (high) | concurrency/architecture | fix（路径 A：双侧 SELECT rooms FOR UPDATE） | `docs/宠物互动App_V1接口设计.md` §10.4 + §10.5 / `docs/宠物互动App_数据库设计.md` §8.6 + §8.7 / `docs/宠物互动App_时序图与核心业务流程设计.md` §11.2 + §12.2 / story spec §10.4.3 + §10.5.3 |

## Lesson 1: snapshot 隔离 ≠ 行锁；ACL guard 需 FOR SHARE 锁，仅 snapshot 不阻止并发提交

- **Severity**: high (P1)
- **Category**: security/concurrency
- **分诊**: fix（路径 A：在步骤 1 显式 `SELECT 1 FROM room_members WHERE room_id=? AND user_id=caller FOR SHARE`，配合 REPEATABLE READ 事务）
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.3 服务端逻辑 / `docs/宠物互动App_数据库设计.md` §8.8

### 症状（Symptom）

r8 把 GET /rooms/{roomId} 步骤 1 + 步骤 3 包进 REPEATABLE READ 事务以共享 snapshot，期望 ACL race 被 snapshot 一致性消除。但下面 timeline 仍能让 ACL 失效后 roster 仍下发：

```
t0  本 GET 事务开始；snapshot 锁定在 t0 状态（caller 是成员）
t1  并发 POST /rooms/{roomId}/leave 提交 DELETE room_members + UPDATE current_room_id
t2  本 GET 步骤 3 用 t0 snapshot SELECT room_members
       —— 看到 caller 仍是成员，照返完整 roster
t3  HTTP 200 + data.members[...] 发出
       —— 但 caller 在 t1 已离开，数据泄漏给已离开用户
```

snapshot 只保证两次 SELECT 看同一时刻状态（**内部一致性**），**不**阻止其他事务在期间提交对同行的写入；外部世界（HTTP 响应发出时）可能已与 snapshot 错位。

### 根因（Root cause）

把 "snapshot" 与 "lock" 这两个概念误等价。MySQL InnoDB 的 REPEATABLE READ 提供的是 **MVCC snapshot**：

- 事务开始时建 snapshot view，事务内 SELECT 全部读 snapshot 版本
- **但** snapshot 不抑制其他事务对相同行的写入；其他事务可以正常 DELETE / UPDATE 并 commit，本事务读不到这些新版本（看到旧的 snapshot 版本）

这是**只读隔离**而非**互斥锁定**。要让"caller 在 HTTP 响应那一刻仍是成员"成立，必须主动锁定那一行：

- `FOR SHARE`（共享锁，多个 reader 可同时持有，与 DELETE / UPDATE 等排他锁互斥）
- `FOR UPDATE`（排他锁，与所有其他 lock 互斥，一般用于自己也要写的场景）

ACL guard 是只读场景，**不**自己写 caller 的 row，所以选 `FOR SHARE` —— 不阻塞其他读，但阻塞并发 leave 的 DELETE，直到本事务 commit / rollback 释放锁。

### 修复（Fix）

V1 §10.3 服务端逻辑：

- 步骤 1 拆成 1a + 1b：
  - 1a: `SELECT users.current_room_id` 校验 ACL（不一致 → 6004）
  - 1b: 通过后追加 `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = caller FOR SHARE`（命中 0 行视同 ACL 失败 → 6004 兜底）
- 注解里写出"snapshot 隔离 + FOR SHARE 锁互补，缺一不可"+ r9 race timeline + deadlock 风险分析（caller 锁特定 row，无锁顺序循环）

数据库设计 §8.8（重写）：

- 章节标题改为"房间详情读快照事务（含 ACL 共享锁）"
- 明确写"snapshot 提供事务内部一致性 / FOR SHARE 提供外部一致性"双机制
- 给出 r9 race timeline 作为反例

story spec §10.3.2：同步 1a/1b 拆分

未改动：

- 时序图 §15.6 仅一行索引，不画 SELECT 细节，**不需修改**

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"先 SELECT 做 ACL 校验 → 再 SELECT 取受 ACL 保护的隐私 / 业务数据"形态的只读接口契约**时，**必须**同时做两件事：① 把多次 SELECT 包在 REPEATABLE READ 事务里（snapshot 内部一致性）；② 在 ACL SELECT 上加 `FOR SHARE` 共享锁锁定 caller 的 ACL row（外部一致性）。**禁止**只靠 snapshot —— snapshot 不阻止并发写提交。
>
> **展开**：
> - **snapshot ≠ lock**：MVCC snapshot 提供"读旧版本"语义，**不**阻止其他事务在期间提交对相同行的写入。要让"判定时刻的状态在响应发出时仍成立"，必须主动加行锁。
> - **FOR SHARE vs FOR UPDATE**：只读 ACL guard 用 FOR SHARE（共享锁；不阻塞其他读）；自己也要写的场景用 FOR UPDATE（排他锁）。
> - **deadlock 风险评估**：每加 row lock 都要分析"锁哪些行 / 并发对象锁哪些行 / 锁顺序是否可形成循环"。caller 锁 `(room_id, user_id=caller)` 这一特定行 + 并发 leave 锁同一行 = 单点串行无循环。其他成员 / 其他房间走不同 row 锁路径无关联。
> - 与 r4 写 race（INSERT UNIQUE 兜底归 6003 / DELETE RowsAffected==0 兜底归 6004）形成体系：**写 race 用兜底兜业务码，读 race 用锁阻止破坏发生**，互补不重叠。
> - 反例 1（r8 钦定）：仅 REPEATABLE READ 事务包多步 SELECT —— 内部一致但外部不一致，仍漏数据。
> - 反例 2：在 step 4 return 前再 SELECT 一次 ACL —— 仍存在校验完到 return 的微小窗口，且增加 SQL，不如直接 FOR SHARE 锁。
> - 反例 3：用 SERIALIZABLE 隔离级别 —— 全局 perf 影响大，且会让无关只读事务也排队，过度。
> - 推荐：REPEATABLE READ 事务 + ACL row 加 FOR SHARE 锁，最小改动 + 精确锁定。

## Lesson 2: 跨事务状态字段 drift 需 FOR UPDATE 串行化两类事务（不只是同事务并发）

- **Severity**: high (P1)
- **Category**: concurrency/architecture
- **分诊**: fix（路径 A：join 步骤 2 + leave 步骤 2 都 SELECT rooms FOR UPDATE）
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.4 + §10.5 / `docs/宠物互动App_数据库设计.md` §8.6 + §8.7 / `docs/宠物互动App_时序图与核心业务流程设计.md` §11.2 + §12.2

### 症状（Symptom）

r4 钦定 §10.4 join 步骤 2 加 `SELECT FOR UPDATE` 防"5 用户抢 1 空位"同事务并发（同房间多个 join 串行）。但 leave 没加锁，导致 join 与 leave 跨事务可产生 timeline：

```
t0  房间 X 有 1 个成员 A，rooms.status=1
t1  A 调 leave，开事务（不锁 rooms）；DELETE room_members(A) → 1 row affected
t2  A 步骤 4 SELECT remaining_count → 0；准备 UPDATE rooms.status=2，但还没执行
t3  B 调 join 房间 X，开事务，SELECT rooms FOR UPDATE → 锁拿到（A 还没碰 rooms 行）
       看到 rooms.status=1，认为可加入；INSERT room_members(B); UPDATE current_room_id; commit
t4  A 继续：UPDATE rooms.status=2 closed; commit
最终：rooms.status=2 (closed)，room_members 含 B 的行 —— 状态 drift
```

后果：B 接下来 GET /rooms/X 看到 status=2 但自己在 roster 里；后续其他 user join 时步骤 3 看到 status=2 → 6005 房间已关闭，但 B 还在房间里，B 的 WS 握手能成功（行还在），rooms.status drift 长期存在直到 B 也 leave。

### 根因（Root cause）

r4 加 `SELECT FOR UPDATE` 时只想着"同一接口（join）的并发请求要串行" —— 这是同事务模式的并发保护。没意识到 join 与 leave 是**两个不同接口**，但都修改 `rooms.status` + `room_members`，必须**跨事务**串行化。

具体地：

- join 锁 `rooms` row 后再判 status —— 它假定 status 是稳定的，但 leave 不锁 rooms 就 DELETE + UPDATE status，rooms 行的两个属性（status / 成员计数）被 join 与 leave 各看一半
- 任何"事务 A 修改 X / 事务 B 修改 Y / 但 X 和 Y 的不变量是 cross-row 的"场景，都需要在两类事务里**锁定共同的关键行**让它们串行

### 修复（Fix）

V1 §10.4 join 步骤 2：原已 `SELECT rooms FOR UPDATE`，注解里**追加**双重职责说明：

1. 同房间并发 join 串行化（r4 既有职责）
2. 与 §10.5 leave 跨事务串行化（r9 新增职责）

V1 §10.5 leave：在 step 2（之前是直接 DELETE）**前置**插入 `SELECT rooms FOR UPDATE`：

- 原有 step 2 (DELETE) → 现在 step 3
- step 4/5/6/7/8/9 全部 +1
- 错误码表 + 关键约束段中所有"步骤 X"标号同步 +1
- close 4007 表 + WS active set 段引用 §10.5 步骤 7 → §10.5 步骤 8

数据库设计 §8.6（join）+ §8.7（leave）：重写为完整步骤顺序版本，明确"FOR UPDATE 与并发对端事务串行化"语义。

时序图 §11.2 join + §12.2 leave：mermaid block 加入 "SELECT rooms FOR UPDATE" 步骤节点 + 注解写明双重职责。

story spec §10.4.3 / §10.5.3：同步上述步骤序列与注解。

未改动：

- §10.5 步骤 3 的 `RowsAffected == 0` 兜底（r4 锁定）保留 —— 两者**职责正交**：FOR UPDATE 解决 cross-tx leave-vs-join drift，RowsAffected==0 解决同 user 并发两次 leave 输家走完后续步骤。锁对象不同（rooms 行 vs room_members 行），不冲突。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 / 审查"两个或多个不同接口都修改同一组关键状态字段（含跨表的不变量）"的契约**时，**必须**在所有相关接口的事务里都对**共同的关键行**加 `FOR UPDATE` 排他锁，让两类事务在该 row lock 上串行。**禁止**只在一类事务里加锁、另一类事务不加 —— 那等于没加。
>
> **展开**：
> - 识别触发条件：≥ 2 个不同 HTTP 接口的事务路径 + 修改同一组状态字段（如 `rooms.status` / `room_members count` / `users.current_room_id`） + 字段间存在跨字段 / 跨表不变量（"status=closed ⇔ room_members 空"）。
> - **加锁原则对称**：所有修改同一不变量的事务都必须锁同一行（一般是承载状态字段那张表的主行，本案是 `rooms` 行）。**任何一方未加锁 = 整体未串行**。
> - **锁对象选择**：选粒度最细但能覆盖不变量的行。本案选 `rooms` 行（不变量根行），不选 `room_members` 行（不变量是 cross-row 计数）。
> - **mismatch 信号**：如果你发现"接口 A 加了 FOR UPDATE 但接口 B 没加"，**几乎必然有 cross-tx drift bug**，无论它有多隐蔽。需要主动构造 timeline 验证。
> - 与 Lesson 1 互补：Lesson 1 是单接口的 ACL race（snapshot + FOR SHARE），Lesson 2 是 cross-tx 的状态 drift（双侧 FOR UPDATE）。两者结合形成"读路径锁 ACL row、写路径锁状态 root row"的并发安全谱系。
> - 反例 1（r4 / r8 钦定后状态）：只在 join 加 FOR UPDATE，不在 leave 加 —— cross-tx race 仍存在，r9 抓到。
> - 反例 2：把 status update + count check 合并成一个 SQL（`UPDATE rooms SET status=2 WHERE id=? AND (SELECT COUNT(*) FROM room_members WHERE room_id=?) = 0`） —— MySQL 不支持 UPDATE 的 WHERE 子查询同表（除非 derived table），SQL 复杂易写错，不如双侧 FOR UPDATE。
> - 反例 3：用 optimistic concurrency / row-version —— 复杂、要全表加 version 列 + retry loop，不在协议层。
> - 推荐路径 A：所有相关事务都对状态根行加 FOR UPDATE，最直观最对称。

---

## Meta: 本次 review 的宏观教训

11-1 r4 / r8 / r9 三轮连续被 codex 抓到并发 race，构成完整的并发安全教训谱系：

- **r4**：写事务自身 race（INSERT UNIQUE 兜底 / DELETE RowsAffected==0 兜底）
- **r8**：单接口多次 SELECT 的 snapshot 一致性（REPEATABLE READ 事务）
- **r9 P1#1**：snapshot ≠ lock，ACL guard 需 FOR SHARE 主动锁定（不只是被动看 snapshot）
- **r9 P1#2**：跨事务的状态字段 drift 需双侧 FOR UPDATE（不只是同事务并发）

四条构成并发安全设计的四个层次：

1. **同事务自洽**（r4：DELETE 0 row 兜底）
2. **同事务多步一致性**（r8：snapshot）
3. **跨事务读保护**（r9-1：FOR SHARE on ACL row）
4. **跨事务写串行化**（r9-2：FOR UPDATE on state root row）

每层各自必要、不可互替，叠加形成完整防护。

把这条 meta 沉淀到契约审查 checklist：每出现"接口逻辑里有多步 SELECT/写"或"多个接口修改同一状态字段"时，主动列 timeline + 标注"哪一层防护被覆盖、哪一层缺失"。Lesson 1 + 2 构成第 3 / 4 层防护的完整范例。
