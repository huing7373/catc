---
date: 2026-05-15
source_review: /tmp/epic-loop-review-20-7-r5.md (codex round 5)
story: 20-7-dev-端点-post-dev-force-unlock-chest
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-15 — dev 端点正确性 > contract 美感（承认契约变更而非回退根因修复）（20-7 r5）

## 背景

Story 20.7 实装 `POST /dev/force-unlock-chest`，初版 AC（epics.md §20.7 行 2941）钦定 request schema 为 `{userId: int64}`，让 server 内部推断"current chest"并 UPDATE。

经过 r1-r4 多轮 race 修复后，最终设计是 client 必须先 `GET /chest/current` 拿到 chest.id，再 POST `{userId, chestId}`，server 用事务 + FOR UPDATE 锁定该具体行后 UPDATE unlock_at。

r5 codex review 指出：「This changes `/dev/force-unlock-chest` from the documented `{userId}` payload to a mandatory `{userId, chestId}` API. Any existing demo/e2e helper that follows Story 20.7 and posts only `userId` will now fail validation with `1002`, and callers that cache a previously-read chest ID will get `1003` as soon as the user's current chest rotates. Because the endpoint is supposed to 'force-unlock the user's current chest', making a specific chest row ID mandatory breaks the intended contract and adds a new stale-ID failure mode.」

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | dev 端点 contract regression — chestId 从 optional 变 required | P2 | architecture / docs | fix-with-doc-only | `server/internal/app/http/handler/dev_chest_handler.go:55-57` + `_bmad-output/planning-artifacts/epics.md` §20.7 |

## Lesson 1: dev 端点正确性 > contract 美感（承认契约变更，不回退根因修复）

- **Severity**: P2
- **Category**: architecture / docs / process
- **分诊**: fix-with-doc-only
- **位置**: `server/internal/app/http/handler/dev_chest_handler.go:55-57`

### 症状（Symptom）

r5 codex 把 r2-r4 的"client 传 chestId"设计标为 contract regression：

1. 偏离原 AC 钦定的 `{userId}` schema
2. 引入 stale-ID failure mode（client 缓存的 chest.id 被 OpenChest 鬼掉后 → 1003）
3. 端点语义从"force-unlock the user's current chest"变成"force-unlock the specified chest of this user"

codex 暗示的修复方向是回退 chestId 参数。

### 根因（Root cause）

**这不是 bug，是 trade-off 决策**。回退的代价：

- r1 实装"server 端推断 current chest"在并发场景下 race —— `FindByUserIDForUpdate` 的 `FOR UPDATE` 阻塞后看到的可能是 OpenChest 刚 INSERT 的 next chest，而非 client 看到的那个。FOR UPDATE 阻塞窗口可达数百 ms，期间 commit 后的 next row 在阻塞结束后才进入当前事务可见集，跑偏概率不低。
- 选项 A（codex 建议回退）→ r1 race 复活，恢复"server 端猜 current"的根本缺陷
- 选项 B（r2-r4 维持）→ 契约变更 + stale-ID failure mode（mild dev experience 问题）

**关键判断**：dev 端点正确性 > contract 美感。dev 端点专供 demo / 自动化 e2e / 手工调试用，stale-ID 仅在中间被 /chest/open 并发的极端 case 出现，自动化脚本 GET → POST 串行执行 chest id 时延 < 1ms 内 stale 概率极低，stale 时 client 重 GET 一次即可恢复，不是阻塞性 bug。

回退的代价（race 跑偏到错的 row → 影响 demo 流程信任度 / 自动化 e2e 偶发不稳定）远大于契约变更的代价（dev 端点契约变更不影响 prod 用户 / 不引入兼容性债 / 文档化即可）。

### 修复（Fix）

**不修代码 —— r2-r4 的"client 传 chestId + 事务 + FOR UPDATE"设计是正确的**。仅做文档化：

1. `dev_chest_handler.go` `PostForceUnlockChestRequest` doc 顶部加"⚠️ 契约变更通告（Story 20.7 r2 起，r5 文档化）"段，说明变更原因 + stale-ID 失败模式可接受性 + 链回本 lesson
2. `_bmad-output/planning-artifacts/epics.md` §20.7 AC 同步：
   - request schema 改为 `{userId: int64, chestId: string}`
   - happy / edge case 描述同步更新（chestId 不存在 / 越权 → 1003）
   - 集成测试描述同步（先 GET /chest/current 拿 chest.id，再 POST）
   - 加脚注 `[^20-7-contract]` 链回本 lesson 解释变更
3. `_bmad-output/implementation-artifacts/20-7-dev-端点-post-dev-force-unlock-chest.md` Change Log 加 r5 行（fix-with-doc-only 决策记录）
4. 归档本 lesson 到 `docs/lessons/`

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **review 指出"实装偏离初版 contract"且实装是为修 race / 数据一致性 / 类似根因 bug 而引入** 时，**禁止盲目回退到初版 contract**；必须先评估"回退 vs 维持"的 trade-off：根因 bug 严重度 vs 契约变更代价。

> **展开**：
>
> - **触发条件识别**：
>   - review 指出 "API contract regression" / "breaks documented schema" / "introduces new failure mode"
>   - 实装代码 commit 历史含 race / concurrency / consistency 关键词的修复链路（如本 story r1-r4）
>   - 实装方案是"把推断责任上移到 client"（client 传更精确的 id / version / cursor 等）
>
> - **正确的处理流程**：
>   1. 调出实装的 git log / story Change Log，确认修复链路的根因（如本 story r1 race）
>   2. 模拟"按 review 回退"会复活什么 bug（如本 story 选项 A → race 复活）
>   3. 评估"维持当前实装"的代价（如本 story 选项 B → stale-ID + 契约变更）
>   4. 若根因 bug 严重度 > 契约变更代价 → **fix-with-doc-only**（不回退代码，文档化承认变更）
>   5. 若代价相当或反过来 → 可考虑回退 + 用其他方案重新解决根因（如悲观锁升级、版本号、advisory lock 等）
>   6. 决策记录到 lesson，让未来 Claude 看到同 finding 时不再走回头路
>
> - **dev 端点的 trade-off 偏向**：dev 端点契约稳定性 < prod 端点（dev 端点仅供内部 demo / 自动化用，无外部消费者 / 无 SDK / 无文档化版本号约束）。dev 端点正确性 / 可靠性 > contract 美感。
>
> - **stale-ID failure mode 的可接受性判定**：
>   - ✅ 可接受：dev 端点 + 自动化脚本 GET → POST 串行 + 失败时 client 重 GET 即可恢复
>   - ❌ 不可接受：prod 端点 + 端用户面对错误 + 无法自动恢复 / 需要复杂重试逻辑
>
> - **fix-with-doc-only 的归档要求**：
>   - 必须在被指出的代码位置加"契约变更通告"段，说明 (a) 变更前后对比 (b) 变更原因 (c) 失败模式 (d) 链回 lesson
>   - 必须同步更新初版 contract 文档（如 epics.md AC / V1 接口设计 / OpenAPI spec）+ 加脚注链回 lesson
>   - 必须在 story Change Log 加 review round 一行，明确"fix-with-doc-only"决策性质
>
> - **反例**：
>   - ❌ 看到 review 指出 contract regression 就盲目回退代码，导致 race / 数据一致性 bug 复活
>   - ❌ 在不同 review round 之间反复横跳（r2 改 race → r5 回退 → 下轮 r6 又改 race），陷入 over-correction chain
>   - ❌ 维持实装但不更新初版 contract 文档（epics.md / AC 与代码不一致 → 下一 reviewer 又指出同样问题）
>   - ❌ 把"review 提出 trade-off 类问题"和"review 指出真 bug"混为一谈 —— 前者需评估后决策，后者必须修

---

## Meta: 本次 review 的宏观教训（可选）

**与 r3 / r4 lesson 形成完整的"over-correction chain 终结模式"系列**：

- r3 lesson（`2026-05-15-domain-aware-rowsaffected-and-over-correction-chain-20-7-r3.md`）：识别"为修一个 review 引入新 bug 让下一 review 再指出"的迭代陷阱
- r4 lesson（`2026-05-15-transaction-eliminates-rowsaffected-ambiguity-20-7-r4.md`）：用事务从根因消除 RowsAffected 语义模糊性，跳出 chain
- **r5 lesson（本文）**：识别"review 指出的不是 bug 而是 trade-off 时拒绝盲目跟随"，是 chain 终结的另一面

三者合起来构成完整的"如何不让 fix-review 变成无止境 ping-pong"的方法论：

1. **r3 / r4 教训**：用根因解决方案跳出表层补丁循环
2. **r5 教训**：识别 review 的 trade-off 类问题 vs 真 bug，前者需评估后决策（可能 fix-with-doc-only），不盲从

未来 Claude 在 fix-review 时若发现自己已经在同一文件 / 同一行做了 3 次以上修复，**强烈提示要回到上一层重新审视设计**（root cause / trade-off 评估），而非继续在原地打补丁。
