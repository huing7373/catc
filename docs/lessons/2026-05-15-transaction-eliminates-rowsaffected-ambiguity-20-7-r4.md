---
date: 2026-05-15
source_review: /tmp/epic-loop-review-20-7-r4.md（codex review round 4 P2）
story: 20-7-dev-端点-post-dev-force-unlock-chest
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-15 — 用事务把 RowsAffected 语义模糊性从源头消除 & over-correction chain 的终结模式（20-7 r4）

## 背景

Story 20.7 的 dev force-unlock-chest 端点经历了 r1-r2-r3-r4 四次 fix-review 迭代：

- **r0**：UPDATE WHERE user_id —— 跑偏到 next chest（并发 OpenChest DELETE+INSERT 后 user_id 匹配新行）
- **r1**：FOR UPDATE SELECT 拿 id → UPDATE WHERE id（事务）—— 锁释放后 SELECT 返 commit 后快照，依旧跑偏
- **r2**：client 传 chest.id（server 不再猜 current）+ 移除 RowsAffected==0 检查 —— 引入二阶 race false success（FindByID 后 chest 被并发删除 → UPDATE 0 行但 service 返成功）
- **r3**：repo 加回 RowsAffected==0 → ErrChestNotFound 翻译 —— 引入"同毫秒重复 unlock 同 chest 误报 1003"bug（unlock_at 列 DATETIME(3) 毫秒精度，值未变 → rows_affected=0）
- **r4（本轮）**：根因解决 —— 用 **事务 + SELECT FOR UPDATE + UPDATE** 把"RowsAffected==0 含义模糊"从 driver 层迁到 DB 层（事务内行存在性由 FOR UPDATE 锁保证，0 行只能是值未变 = success）

本次 review 由 codex round 4 指出 r3 的 RowsAffected==0 → NotFound 在"同毫秒重复 unlock"场景下误判（DATETIME(3) 毫秒精度，重试落同毫秒 → 列值未变 → 0 行 → 误报 1003）。这是 r2-r3-r4 over-correction chain 的终点 —— 本次跳出"在 RowsAffected 表层微调"的死路。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | [P2] 同毫秒重复 unlock 同 chest → RowsAffected=0 → 误报 1003（NotFound） | high | architecture / error-handling | fix | `server/internal/repo/mysql/chest_repo.go`, `server/internal/service/dev_chest_service.go`, `server/internal/app/bootstrap/router.go`, `server/internal/repo/mysql/chest_repo_test.go`, `server/internal/service/dev_chest_service_test.go`, `server/internal/service/dev_chest_service_integration_test.go`, `server/internal/service/auth_service_test.go`, `server/internal/service/chest_open_service_test.go`, `server/internal/service/home_service_test.go`, `server/internal/service/auth_service_integration_test.go` |

## Lesson 1: 用事务 + SELECT FOR UPDATE + UPDATE 三件套消除 RowsAffected==0 的语义模糊性

- **Severity**: high
- **Category**: architecture / error-handling
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/chest_repo.go:267` (UpdateUnlockAtByID), `server/internal/service/dev_chest_service.go` (ForceUnlockChest)

### 症状（Symptom）

`UPDATE user_chests SET unlock_at = ? WHERE id = ?` 在 driver 层返 `RowsAffected==0`，但调用方无法分辨这 0 行是哪种原因：

| 可能原因 | 期望行为 |
|---|---|
| 行不存在（NotFound） | 返 1003 ErrResourceNotFound |
| 行存在但列值已是 newUnlockAt（值未变） | 视为 success 返 nil |
| 行存在但被并发删除（race） | 返 1003 ErrResourceNotFound |

`unlock_at` 列是 `DATETIME(3)`（毫秒精度），同毫秒重复 unlock 同 chest → MySQL 看 column = new value，返 0 行。r3 的"RowsAffected==0 → ErrChestNotFound"翻译在此场景下**误报 1003**（chest 明明存在）。

### 根因（Root cause）

r2/r3 在"driver 层 RowsAffected 含义"上做微调，永远摆脱不了"0 行可能是 NotFound 也可能是值未变"的模糊性 —— 因为 MySQL `UPDATE` 在以下两种场景都返 0：

1. WHERE 子句匹配 0 行（行不存在）
2. WHERE 子句匹配 1 行，但 SET 后所有列值 = 当前值（"值未变"优化）

r3 假设"newUnlockAt = time.Now().UTC() 毫秒级唯一 → 值不会撞"是错的 —— DATETIME(3) 只有毫秒精度，两次调用落同毫秒的概率虽然不高，但在 dev 自动化重试 / demo 快速点击场景下是可发生的边界。

**根因不在 driver 层**，而在"调用方依赖 RowsAffected 表达存在性"这个错误前提。

### 修复（Fix）

跳出 r2-r3-r4 的 RowsAffected 微调 chain，从 DB 层用 **事务 + FOR UPDATE 行锁** 把存在性保证迁到 caller 侧：

```
// service.ForceUnlockChest(r4 改造)
txMgr.WithTx(ctx, func(txCtx) error {
    chest, err := chestRepo.FindByIDForUpdate(txCtx, chestID)  // FOR UPDATE 锁住行
    if err == ErrChestNotFound { return 1003 }
    if chest.UserID != userID { return 1003 }
    return chestRepo.UpdateUnlockAtByID(txCtx, chestID, time.Now().UTC())
    //                                                       ↑
    //  事务内 FOR UPDATE 之后，UPDATE 行存在性已锁定保证；
    //  并发 OpenChest 的 DELETE 必须等本事务 commit；
    //  RowsAffected==0 唯一可能 = 值未变 → repo 返 nil = success
})
```

repo 层 `UpdateUnlockAtByID` r4 实装：

```go
func (r *chestRepo) UpdateUnlockAtByID(ctx, chestID, newUnlockAt) error {
    result := db.Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)
    if result.Error != nil { return result.Error }
    // r4: 不再检查 RowsAffected==0 —— caller 已在事务内 + FindByIDForUpdate 保证行存在；
    // RowsAffected==0 仅可能是同毫秒重复 unlock 同 chest（值未变），视为 success。
    return nil
}
```

具体改动:

- `chest_repo.go`：新增 `FindByIDForUpdate(ctx, chestID)` interface 方法 + 实装（SELECT WHERE id FOR UPDATE）；移除 `UpdateUnlockAtByID` 的 `RowsAffected==0 → ErrChestNotFound` 检查
- `dev_chest_service.go`：重新注入 `txMgr`，`ForceUnlockChest` 用 `txMgr.WithTx` 包 `FindByIDForUpdate + UpdateUnlockAtByID` 两步
- `router.go`：`NewDevChestService(deps.TxMgr, chestRepo)` 同步双参数
- 所有 stub ChestRepo 实装（`stubChestRepo` / `stubOpenChestChestRepo` / `stubHomeChestRepo` / `faultChestRepo`）加 `FindByIDForUpdate` 方法
- 单测 `dev_chest_service_test.go` 改用 `findByIDForUpdateFn` + `defaultStubTxMgr`；case 6 从"二阶 race → 1003"改为"同毫秒重复 unlock → success"
- repo 测试 `chest_repo_test.go` 把 `RowsAffectedZero_ReturnsErrChestNotFound` 改为 `RowsAffectedZero_ReturnsNil`；新增 `FindByIDForUpdate_HappyPath / NotFound` 两 case
- 集成测试 `dev_chest_service_integration_test.go` 改用真 `tx.NewManager`；case 3 改为"连续两次 unlock 同 chest → 两次都 success"
- 顺带：补 `faultChestRepo.FindByID` 方法（auth_service_integration_test.go pre-existing 缺方法，integration vet 失败；本次一起补齐）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **看到 review 反复在 driver 层某个返回值（RowsAffected / driver-specific error code）上微调含义**（比如"0 行是 NotFound 还是值未变？"）时，**必须跳出表层微调，去用事务把"语义不变量"从 driver 层迁到 DB 层（FOR UPDATE 行锁 / UNIQUE 约束 / CHECK 约束等）**。
>
> **展开**：
> - **触发信号 = 同一个 review 维度（"RowsAffected==0 含义"）连续 3+ round 翻来覆去**（r2 移除检查 → r3 加回检查 → r4 又要 ？）。这是 "over-correction chain" 的信号 —— 表层微调永远摆不平含义模糊性，必须升一层抽象。
> - **driver 层信号本身就是 leaky abstraction**：MySQL `UPDATE` 的 `RowsAffected` 默认行为是"匹配且改了行数"（不含"匹配但没改"的行）；这个语义对"存在性"判断天然不可靠。`UPDATE` 是为"写"设计的，不是为"判断存在"设计的；让它兼任两职就是滥用。
> - **事务 + FOR UPDATE 是消除模糊性的正确工具**：在事务内先 `SELECT ... FOR UPDATE` 把存在性 + 锁住行（让并发 DELETE 必须等事务 commit），然后 UPDATE —— UPDATE 的 RowsAffected 就只剩"值变没变"一种语义，调用方完全可以忽略（值未变在大多数业务场景下等同于 success "已是该状态"）。
> - **反例 1**：用 `if result.RowsAffected == 0 { return ErrNotFound }` 来表达存在性。这只在"newValue 必然 ≠ oldValue"的小众场景下成立；任何 caller 传入的值可能撞上 oldValue 的场景（时间常量、固定值、客户端回传 server 已下发的字段）都会误报。
> - **反例 2**：用 `if rowsAffected == 0 { 返 success }` 假装存在性已被保证。这在没有事务 + FOR UPDATE 的前提下是 false success（行真不存在时也会返 success），是 r2 的 bug。
> - **反例 3**：fix-review 时只看本 round review 文本说什么，照着改一行 → 容易掉进 over-correction chain。**真正的修复要看历史 review 链（r1 → r2 → r3 → 本轮）的整体震荡轨迹，找最深的根因层**（本案 = "RowsAffected 不该承担存在性判断"），而不是停在最新一轮 review 的措辞表层。

## Lesson 2: over-correction chain 的终结模式 —— 当微调进入"r1 加 / r2 减 / r3 加" 钟摆时，必须升一层抽象

- **Severity**: high
- **Category**: process / architecture
- **分诊**: fix
- **位置**: 元规则（适用于所有 fix-review 迭代）

### 症状（Symptom）

Story 20.7 的 review 历史：

| Round | 改动 | 引入 bug |
|---|---|---|
| r1 | UPDATE WHERE id（不再 WHERE user_id） + 事务 + FOR UPDATE | FOR UPDATE 锁释放后 SELECT 返 commit 后快照，依旧跑偏 |
| r2 | 移除事务 + client 传 chest.id + 移除 RowsAffected==0 检查 | 二阶 race false success |
| r3 | 加回 RowsAffected==0 → ErrChestNotFound 检查 | 同毫秒重复 unlock 误报 1003 |
| r4 | **跳出表层**：事务 + FOR UPDATE + 不依赖 RowsAffected | （根因解决） |

r2 和 r3 在同一个"RowsAffected==0 含义"维度反复横跳 —— 这是典型 over-correction chain。每一轮 review 都指出当前实装的某个 bug，让 Claude 朝相反方向调一点，结果总有新 bug 出现，直到 Claude 意识到"在这个维度微调摆不平，必须换一个维度"。

### 根因（Root cause）

Claude 在 fix-review 时倾向"局部最优修复" —— 看到 review 指出 bug A，就反向调一点掩盖 bug A，但忽略这反方向会引入 bug B（被下一轮 review 抓到，又反过来调 ...）。这种"在同一维度微调"是 fix-review 的反模式。

终结模式：当观察到自己已经在同一维度反复横跳 **≥ 2 round**（不是 3 round —— 越早察觉越好），必须强制升一层抽象问"为什么这个维度本身有歧义？根因是不是在更下一层？"

本案的"更下一层" = "RowsAffected 不该承担存在性判断"。事务 + FOR UPDATE 把存在性判断迁到 SELECT 上，UPDATE 只承担"写"，单一职责 → 没有歧义可争。

### 修复（Fix）

不是代码修复，是**流程修复**：在 fix-review skill 的认知中加入"over-correction detection"启发式：

- 当本轮 review 的 finding 落在"上一轮 review fix 已经触碰过的代码点"上 → 警惕 over-correction
- 当连续 2 轮 review 在同一函数 / 同一返回值 / 同一约束上反向 ping-pong → **暂停表层修复**，强制问"根因是不是在更上一层抽象？"
- 升级抽象的可选工具：
  - 单点判断 → 用事务保证不变量
  - 业务码歧义 → 拆出新的错误哨兵
  - 时序竞争 → 加显式锁（DB / Redis / mutex）
  - 状态机模糊 → 引入显式 status 字段或 enum

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **执行 fix-review 时发现本轮 finding 与上一轮 fix 反向**（"r1 删了 X，r2 加回 X，r3 又要删 X" 之类）时，**禁止继续在 X 这一维度微调，必须升一层抽象**（找 X 模糊性的根因，用事务 / 锁 / 显式哨兵 / 状态字段消除）。
>
> **展开**：
> - **检测信号**（2+ round 任一满足即触发）：
>   - 本轮改动的代码行 = 上一轮改动的代码行（同一函数 / 同一 if 分支 / 同一返回值）
>   - 本轮 fix 的方向 = 上一轮 fix 的逆方向（"加了又删 / 删了又加 / true 又改 false"）
>   - 本轮 review 的话术含"r1 / r2 的 over-correction"或类似自指（review 本身在指出迭代陷阱）
> - **触发后的标准动作**：
>   1. 列出 r1 → 本轮的所有改动维度（"这条改动在改什么轴？"）
>   2. 找该维度的"语义模糊点"（如本案：UPDATE.RowsAffected==0 的两种可能含义）
>   3. 用更下一层工具消除模糊（如本案：事务 + FOR UPDATE 让其中一种含义不可达）
>   4. lesson 文档明确记录"r1-r2-r3-r4 链 + 本轮跳出方式"，给未来 Claude 留导航锚
> - **反例 1**：看到 review 说"现在 RowsAffected==0 返 ErrNotFound 是错的"→ 直接改成"RowsAffected==0 返 nil"→ 但没意识到这正是 r2 的实装 → 等于把 codebase 推回 r2 状态 + 还多了一轮 lesson 噪音。over-correction 的二次发作。
> - **反例 2**：把 review 当作"标准答案"机械执行。review 提出的是"现状有 bug"，不是"该怎么修"；该怎么修取决于 Claude 是否看出"在当前维度微调摆不平"。
> - **反例 3**：以"快速结束本轮"为目标做最小改动。fix-review 的目标是**让这个 review 维度永久退出问题列表**，不是"让本轮 codex 不再吐槽"。短期看起来一轮 fix 多花 30% 时间，长期省下 r5 / r6 / r7 的反复。

---

## Meta: 本次 review 的宏观教训

Story 20.7 用 4 round review 终于把"force-unlock chest"做对，核心教训是：

**当一个 race 修复在 2 round 内没收敛，根因往往不在你以为的层。** r1-r2-r3 三轮都在"client/server 谁判 current"和"RowsAffected 怎么解读"两个维度反复，r4 才意识到根因 = "用 driver 层副作用判存在性"是错的前提，必须升一层用事务保证。

这与 Story 20.6 的"幂等键 + Redis 写回失败"问题（最终用 DB 事务幂等替代）是同一类抽象升级 —— **遇到 driver 层歧义，先想能不能用事务/约束/锁迁到 DB 层**。
