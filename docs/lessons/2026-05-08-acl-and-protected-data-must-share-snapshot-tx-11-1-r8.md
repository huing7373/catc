---
date: 2026-05-08
source_review: codex review round 8 of story 11-1（/tmp/epic-loop-review-11-1-r8.md）
story: 11-1-接口契约最终化
commit: 814a5a1
lesson_count: 1
---

# Review Lessons — 2026-05-08 — ACL 校验 + 受 ACL 保护的数据返回必须共享同一事务 snapshot（11-1 r8）

## 背景

Epic 11 Story 11.1（接口契约最终化）r7 锁定了 `GET /api/v1/rooms/{roomId}` 的 ACL（caller 必须是当前房间成员，否则 6004），但服务端逻辑被钦定为"只读查询，**不**开 MySQL 事务（步骤 1 + 步骤 3 各自独立 SELECT，允许微小漂移，client 自洽即可）"。

codex r8 抓到这条钦定的并发 race 漏洞：caller 自己在并发跑 `POST /rooms/{roomId}/leave`，两个请求都通过步骤 1 ACL 预检后，leave 在步骤 1 → 步骤 3 之间 commit；本 GET 请求继续跑步骤 3，仍 return 完整 roster —— **ACL 已失效但隐私数据照下发**，绕过 r7 刚锁定的隐私边界。

这与 r4 已修的 §10.1 create / §10.5 leave 写事务并发 race 同模式，但发生在**读路径**上：r4 是写 race（INSERT 撞 UNIQUE / DELETE RowsAffected==0），r8 是读 race（多次 SELECT 跨步骤 snapshot 不一致）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | GET /rooms/{roomId} 步骤 1 + 步骤 3 必须在同一 REPEATABLE READ 事务 snapshot 内 | medium (P2) | security/architecture | fix | `docs/宠物互动App_V1接口设计.md` §10.3 / `docs/宠物互动App_数据库设计.md` §8.8（新增）/ `_bmad-output/implementation-artifacts/11-1-接口契约最终化.md` AC4 §10.3.2 |

## Lesson 1: ACL 校验 + 受 ACL 保护的数据返回必须 atomically 在同一事务 snapshot 内

- **Severity**: medium (P2)
- **Category**: security/architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.3 服务端逻辑（行 1245-1256）+ `docs/宠物互动App_数据库设计.md` §8.8（新增章节）+ story spec AC4 §10.3.2 草稿

### 症状（Symptom）

`GET /rooms/{roomId}` 钦定不开事务、两次 SELECT 各自独立。在以下并发 timeline 下隐私边界被绕过：

```
t0  caller A holds room_members(room=X, user=A) + users.current_room_id = X
t1  A 同时发出两个请求：
      Req-1: GET /rooms/X
      Req-2: POST /rooms/X/leave
t2  Req-1 步骤 1：SELECT users.current_room_id → X（pass，准备进入步骤 3）
t3  Req-2: 完整跑完 leave 写事务（DELETE room_members + UPDATE users.current_room_id = NULL + 必要时 rooms.status = closed）+ commit
t4  Req-1 步骤 3：SELECT room_members WHERE room_id = X JOIN users JOIN pets
        —— 看到的是 t3 commit 后的状态：A 已不在 room_members、其他成员还在
        —— 但仍 return 完整其他成员 roster 给 A
t5  Req-1 return 200 + data.members[...]
```

A 已经离开房间，却还能拿到房间剩余成员的 nickname / avatarUrl / petId —— 这恰恰是 r7 ACL 设计要禁止的"非成员看不到房间隐私字段"。

### 根因（Root cause）

r7 锁 ACL 时只关注了"deny 默认 + 显式 allow"的访问控制层（步骤 1 是否拦），没追问"步骤 1 通过之后到步骤 3 之间，ACL 状态是否仍然成立"。"只读查询不开事务"的传统直觉在**单次 SELECT** 接口上是对的（自然原子），但本接口结构是"先 SELECT A 做 ACL → 再 SELECT B 拿 A 保护的数据"—— 跨多次 SELECT 时，无 snapshot 隔离 = ACL 校验和数据返回看到不同时刻状态 = ACL 形同虚设。

更深层的根因：把"事务"等同于"写"。把"读不开事务"当默认值，忽略了事务的两个独立语义维度：

1. 写原子性（多次写要么全 commit 要么全 rollback）
2. 读快照一致性（多次读看同一 snapshot）

只读接口同样需要 (2) —— 当多次 SELECT 之间存在"前一次决定后一次该不该返"这种 ACL 依赖时。

### 修复（Fix）

V1 设计 §10.3 服务端逻辑：

- 把"**注**：本接口为只读查询，**不**开 MySQL 事务" 改成"**事务边界**：开 MySQL 事务（隔离级别 = REPEATABLE READ，InnoDB 默认）包全部 4 步"
- 步骤 4 改成"提交事务并返回"
- 注解里把 race timeline + rationale 都写出来，并指向数据库设计 §8.8

数据库设计 §8.8（新增章节）：

- 在 §8.7 退出房间事务之后新增"§8.8 房间详情读快照事务"
- 明确归类"读快照事务"，与写事务（§8.1 / §8.6 / §8.7）并列
- 给出未来识别规则：多次 SELECT 形态、且任一次 SELECT 是 ACL 校验、任一次 SELECT 是受 ACL 保护的数据 → 必须开读快照事务

story spec AC4 §10.3.2：

- §10.3.2 服务端逻辑草稿同步加事务边界声明（保持 spec 与正式契约文档语义对齐，避免 Story 11.6 实装时漏读）

未改动：

- 时序图 §15.6 仅一行"`GET /api/v1/rooms/{roomId}`"，不画具体 SELECT，**不需修改**
- §10.4 join 步骤 1 / §10.5 leave 步骤 1 的"不开事务"预检 SELECT **不需修改** —— 那是单次 fail-fast 预检（pass 后立即进入写事务），与本接口"两次 SELECT 跨步骤"模式不同

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"先 SELECT 做 ACL 校验 → 再 SELECT 取受 ACL 保护的隐私 / 业务数据"形态的只读接口契约**时，**必须**把这两次 SELECT 放进同一 MySQL 事务（REPEATABLE READ；InnoDB 默认）以共享 snapshot —— **禁止**靠"client 自洽不变量"或"漂移可接受"绕开 ACL race。
>
> **展开**：
> - "事务"有两个独立语义：写原子性 + 读快照一致性。**只读接口同样可能需要后者**，当多次 SELECT 之间存在"前一次决定后一次该不该返"的依赖时
> - 识别触发条件：接口是 GET / 只读 + 服务端逻辑列出 ≥ 2 步 SELECT + 至少一步是"`SELECT users.current_room_id` / `SELECT room_members WHERE user_id = ?` / `SELECT user_role = ?` / `SELECT owner_id = ?`"等 ACL / 权限校验 + 至少一步是"返回受该 ACL 保护的隐私 / 业务数据"
> - **MySQL InnoDB 默认隔离级别就是 REPEATABLE READ**，开事务即天然有 snapshot 隔离，改动成本极低（一行 BEGIN / COMMIT 包多步 SELECT），**没有理由不开**
> - 与 r4 写事务 race（§10.1 create UNIQUE 兜底归 6003 / §10.5 leave RowsAffected==0 兜底归 6004）对照：r4 是写路径上"事务里 race 必须落到正确业务码"，r8 是读路径上"跨步骤 ACL+数据必须 snapshot 一致"—— **同一安全模式（race 不能让 ACL / 业务语义失效）的两个面**
> - 反例 1（绕开 ACL）：契约说"只读不开事务，client 按 `memberCount === members.length` 自洽即可" —— 这把 ACL 边界（server-only 概念）转嫁给 client，等于没有 ACL
> - 反例 2（路径 C 微窗口）：在步骤 4 return 前再做一次 caller 成员校验 SELECT —— 仍然存在校验完到 return 之间的微小窗口，且增加 SQL 来回，不如直接开事务
> - 反例 3（路径 B 复杂 SQL）：把 ACL EXISTS 子查询塞进步骤 3 SQL —— 单 query 但 ACL fail 要后置判 0 行 + 6001/6004 区分逻辑变绕，不如开事务+两步独立 SELECT 直观
> - 推荐路径 A：开 REPEATABLE READ 事务包"步骤 1 SELECT current_room_id（ACL）"+"步骤 3 SELECT roster（受保护数据）"+"步骤 2 SELECT rooms 兜底"，最小改动 + 最直观 + 与已锁定的写事务（§8.6 / §8.7）形成对称
> - 数据库设计 §8 应同步新增"读快照事务"章节，把这类只读多步事务从写事务集合里独立出来便于后续 audit。后续 `/me` 等多步 SELECT 接口套用同一规则

---

## Meta: 本次 review 的宏观教训

11-1 contract freeze 节点在 r4（写 race）和 r8（读 race）两次被 codex 抓到同一类问题：**race condition 让 ACL / 业务语义失效**。教训：

- 任何"先校验后操作"或"先校验后返数据"模式必须主动追问"两步之间能不能被并发改 race"，不能只看单步是否对
- 事务不仅是写工具，也是读 snapshot 工具。"只读不开事务"不是默认正确，要看接口结构
- ACL 校验通过的判定**只在该 snapshot 那一刻有效**，要让"判定"和"基于判定的动作"覆盖在同一 snapshot 内

把这条 meta 教训沉淀进契约审查清单：每出现 ≥ 2 步 SELECT/写组合就主动列 race timeline。
