---
date: 2026-05-15
source_review: codex review round 3 — /tmp/epic-loop-review-20-7-r3.md
story: 20-7-dev-端点-post-dev-force-unlock-chest
commit: dd1237c
lesson_count: 2
---

# Review Lessons — 2026-05-15 — domain-aware RowsAffected 语义 & fix-review 迭代陷阱（over-correction chain）

## 背景

Story 20.7 `POST /dev/force-unlock-chest` 经历了 3 轮 codex review，每轮针对同一个 race 修复方向：
- **r1** [P2] 把 `UPDATE WHERE user_id = ?` 改成 `UPDATE WHERE id = ?` + FOR UPDATE 取 id（修了 r0 的"跑偏 next chest"问题）
- **r2** [P2] 发现 r1 的 FOR UPDATE 也救不了 race，改成 client 传 chest.id；同时**移除** repo 层 `RowsAffected==0 → ErrChestNotFound` 翻译（顾虑"同一毫秒重复 unlock 同一 chest" → value 不变 → rows_affected=0 → 误返 1003）
- **r3** [P2]（本次）—— 发现 r2 的移除是 over-correction：service 步骤 `FindByID` 与 `UpdateUnlockAtByID` 之间，并发 `/chest/open` 可以删 chest → UPDATE 0 行但 service 返 false success。

r3 的修复方向：**在 force-unlock 场景下 `RowsAffected==0` 等同于 NotFound** —— 因为 `newUnlockAt = time.Now().UTC()` 毫秒级唯一，行存在时 UPDATE 必返 1 行；rows_affected=0 唯一来源 = 行已不存在。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | FindByID + UPDATE 之间被并发删除 → false success（缺 RowsAffected==0 检查） | P2 | error-handling | fix | `server/internal/repo/mysql/chest_repo.go` + dev_chest_service.go + 单测 + 集成测试 |

## Lesson 1: domain-aware RowsAffected 语义 — 在特定场景下 RowsAffected==0 可以等同于 NotFound

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/chest_repo.go` UpdateUnlockAtByID

### 症状（Symptom）

`service.ForceUnlockChest` 在 r2 实装下，两步：

```
1. chest, _ := chestRepo.FindByID(chestID)
2. 校验 chest.UserID == claimedUserID
3. chestRepo.UpdateUnlockAtByID(chestID, time.Now().UTC())  -- r2 不查 RowsAffected
4. 返回 nil
```

并发 race：步骤 1 与步骤 3 之间，另一个 `/chest/open` 删了 chest → UPDATE 0 行 → service 返 success。用户在 `/dev/force-unlock-chest` 后立刻 `GET /chest/current` 仍看到 `status=counting`（dev 端点声称 unlock 成功但实际没生效）。

### 根因（Root cause）

r2 把 repo 层的 `RowsAffected==0 → ErrChestNotFound` **完全移除**，理由是"MySQL UPDATE 在值未变时也返 rows_affected=0，会把同一毫秒重复 unlock 误判为 NotFound"。

但这个顾虑**只在 `newUnlockAt` 可能与现值相同时**才成立。**force-unlock 场景下，`newUnlockAt = time.Now().UTC()` 走系统时钟单调推进，毫秒级唯一**，行存在时 UPDATE 必返 RowsAffected=1。`RowsAffected==0` **唯一**来源 = 行已不存在。

r2 把"通用安全规则"（"`RowsAffected==0` 不一定 = NotFound，DB 行为依赖 changedRows vs matchedRows"）盲目套用到一个具体场景，丢失了 domain context。

### 修复（Fix）

repo 层 `UpdateUnlockAtByID`：

```go
// r3 [P2] 改造（before → after）
- result := db.WithContext(ctx).Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)
- return result.Error
+ result := db.WithContext(ctx).Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)
+ if result.Error != nil {
+     return result.Error
+ }
+ if result.RowsAffected == 0 {
+     return ErrChestNotFound  // 在 force-unlock 场景下 = 行已被并发删除
+ }
+ return nil
```

service 层 `ForceUnlockChest`：UPDATE 返 `mysql.ErrChestNotFound` 时，翻译为 1003（与步骤 1 FindByID NotFound 同码 —— client 重试时 GET /chest/current 拿新 id）。

repo interface doc + 实装 doc + service doc 全部追加 **"未来调用方注意"** 警告：如未来有其他场景调用 `UpdateUnlockAtByID` 且 `newUnlockAt` 可能与现值"按位相同"（如固定时间常量），需要重新评估 `RowsAffected==0` 语义。当前唯一调用方是 `ForceUnlockChest`。

测试：
- `chest_repo_test.go` 重命名 `_RowsAffectedZero_ReturnsNil` → `_ReturnsErrChestNotFound`；新增 `_ConcurrentDelete_RaceToErrChestNotFound`
- `dev_chest_service_test.go` 新增 case 6 `ConcurrentDeleteRace_Returns1003`（stub `updateUnlockAtByIDFn` 返 `ErrChestNotFound` 模拟 race）
- `dev_chest_service_integration_test.go` 把 case 3 从"DuplicateCallSameMillis_Succeeds"改为 "ChestAlreadyDeleted_Returns1003"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **decide 是否检查 SQL UPDATE 的 RowsAffected** 时，**必须**先判断"在本场景下，`newValue` 是否可能与现值按位相同"——若**不可能**（如 `time.Now()` 单调推进 / 单调递增 ID / UUID 等），则 **RowsAffected==0 等同于 NotFound**，**必须**翻译为业务哨兵。
>
> **展开**：
> - DB 层规则"`UPDATE` 在值未变时也返 `rows_affected=0`"是**通用安全规则**，不是"任何场景都该完全忽略 RowsAffected"的依据
> - 判断 domain-aware 语义的三问：
>   1. 调用方传入的 `newValue` 来源是什么？（系统时钟 / 计数器 / 用户输入 / 固定常量 / 复制自现值？）
>   2. 在本场景下，`newValue == oldValue` 是否可能？
>   3. 如果不可能 → `RowsAffected==0` 唯一来源 = 行不存在 → 应该返 NotFound 哨兵
> - **写 repo doc 时**，必须把"在 X 场景下 RowsAffected=0 等同 NotFound 是因为 newValue 单调"这种 domain context **显式写在方法注释里**；同时**警告未来调用方**：若新增 caller 且 newValue 来源不同，需要重新评估
> - **反例**：r2 把 `RowsAffected==0 → ErrChestNotFound` 完全移除，理由是"MySQL 通用规则"——这是把 generic DB 行为套到 specific domain，丢失了 `time.Now()` 单调这个关键 context

## Lesson 2: fix-review 迭代陷阱 — over-correction chain（一处修复引入二阶问题，过度修复又引入新问题）

- **Severity**: P2（meta，影响 review 流程本身）
- **Category**: process / architecture
- **分诊**: process insight
- **位置**: r0 → r1 → r2 → r3 同一处代码的连续 3 轮修复

### 症状（Symptom）

`POST /dev/force-unlock-chest` 在 3 轮 codex review 中持续被发现 P2 race 问题：

| 轮次 | 修复方向 | 引入的新问题 |
|---|---|---|
| r0 | `UPDATE WHERE user_id = ?` | r1 发现：与 `/chest/open` 并发 → 跑偏 next chest |
| r1 | FOR UPDATE 取 id + `UPDATE WHERE id = ?` | r2 发现：FOR UPDATE 锁释放后 SELECT 返 commit 后快照，race 没真修；同时担心"同毫秒重复 unlock"误判 1003 |
| r2 | client 传 chestID（不再 FOR UPDATE）+ 移除 `RowsAffected==0 → ErrChestNotFound` | r3 发现：移除 RowsAffected 检查 → FindByID 与 UPDATE 之间 chest 被删 → false success |
| r3 | 重新加回 `RowsAffected==0 → ErrChestNotFound`（在 force-unlock 场景下） | 待 r4 验证 |

每轮都引入新的二阶问题。

### 根因（Root cause）

1. **review 修复的"通用安全冲动"**：fix 一个具体 race 时容易引入"看起来更通用的安全 / 更解耦"的改动（如 r2 "移除 RowsAffected 检查"是出于"通用规则"），但这种改动会**剥离 domain context**，引入二阶 race
2. **缺少端到端 race 心智模型**：每轮只盯局部代码段，没把整个"用户体验：dev unlock → GET /chest/current 应立刻 unlockable"这条 invariant 当成 test oracle
3. **修复时只考虑"修了 reviewer 当下提的点"**，没系统性 walk 一遍 race 的所有时序窗口

### 修复（Fix）

本次 r3 修复策略：**回退** r2 的 `RowsAffected` 移除决策；在 doc 里明确写"r2 是 over-correction"的反思，让未来 review 不再走同一条死路。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **fix-review 第 N 轮（N≥2）修复同一处代码** 时，**必须**把前 N-1 轮修复方向作为 "约束"而非"基线"，先 systematic walk 该代码段在并发下的所有时序窗口（FindByID/UPDATE/DELETE/INSERT 的所有交错），列出 invariant table，再决定改动。
>
> **展开**：
> - 多轮 fix-review 同一处代码时，**必须**先在 lesson / commit message 里写出 **"r(N-1) 修了什么 + 哪些 invariant 仍未被覆盖"**，避免循环 over-correction
> - **避免"通用安全规则盲目套用"**：每个"看起来更安全"的改动（如"不再依赖 RowsAffected"、"不再用 FOR UPDATE"、"加事务包裹"）都必须附带 **specific domain context 验证**：在 X 场景下这个改动是否引入二阶问题
> - **race 修复要走 invariant-driven**：列 invariant（如"调用 dev unlock 成功 → GET /chest/current 立刻 unlockable"）→ 列出所有可破坏 invariant 的时序 → 逐个证明被覆盖
> - **反例**：r2 把"RowsAffected 检查"整个移除是 over-correction —— "顾虑同毫秒重复 unlock 误判 1003" 只在 newUnlockAt 可能等于旧值时成立，而 force-unlock 场景下 newUnlockAt 是 `time.Now()` 单调推进，根本不会触发顾虑；为了一个不会发生的 case 牺牲了"chest 不存在"的诊断能力，引入 false success bug
> - **lesson 文档要回溯多轮 chain**：写 lesson 时必须把 r0 → r1 → r2 → r3 的演进路径作为表格写出，让未来 Claude 一眼看到"哪些方向已经试过 + 为什么失败"

---

## Meta: 本次 review 的宏观教训

`POST /dev/force-unlock-chest` 是一个**典型的"看起来简单但实际有 3 重 race window"的端点**：
1. r0 race: UPDATE WHERE user_id ↔ /chest/open 删旧 INSERT 新（修法：WHERE id）
2. r1 race: FOR UPDATE 取 id 在 /chest/open commit 后看到新 id（修法：client 传 id）
3. r2 race: FindByID + UPDATE 之间 chest 被并发删除（修法：RowsAffected==0 → NotFound）

每一轮都需要 walk 完整的时序窗口，不能只盯当下 review 提的那一点。**dev 端点不是"低风险所以可以马虎"的合理化借口** —— dev 端点的产品价值就是"快速验证业务路径"，false success 比直接报错更有破坏性（误导 e2e / demo 流程）。

> **未来 Claude 在写 dev / debug / admin 端点时**：仍然要走完整的 race walk-through —— 这类端点的"低优先级"只是相对业务端点而言，**不是 race-safety 标准的折扣**。
