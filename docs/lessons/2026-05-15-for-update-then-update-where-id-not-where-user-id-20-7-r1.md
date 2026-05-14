---
date: 2026-05-15
source_review: codex review round 1 / /tmp/epic-loop-review-20-7-r1.md
story: 20-7-dev-端点-post-dev-force-unlock-chest
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-15 — FOR UPDATE 后 UPDATE 必须 WHERE id 而非 WHERE user_id（避免与并发 Delete+Create 链路跑偏到 next row）

## 背景

Story 20.7 实装 `POST /dev/force-unlock-chest` dev 端点，service 原实装是单 UPDATE 不开事务：

```go
return s.chestRepo.UpdateUnlockAt(ctx, userID, time.Now().UTC())  // UPDATE user_chests SET unlock_at=? WHERE user_id=?
```

codex review round 1 指出该实装与 Story 20.6 `POST /chest/open` 并发时存在 race：OpenChest 事务持有 chest row 的 FOR UPDATE 锁，期间 Delete 旧 chest + Create 下一轮 chest（同 user_id）；force-unlock 的 UPDATE 阻塞在锁上，commit 后 `WHERE user_id` 二次匹配跑偏到刚插入的 next chest，把它直接推到 unlockable，让用户能连开 2 次。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | dev/force-unlock-chest 与 /chest/open 并发 race | P2 | architecture | fix | `server/internal/repo/mysql/chest_repo.go:184` |

## Lesson 1: FOR UPDATE 后的 UPDATE 必须 WHERE id-of-fetched-row，而非 WHERE 业务索引列二次匹配

- **Severity**: medium (P2)
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/chest_repo.go:184`（原 `UpdateUnlockAt(userID)` → `UpdateUnlockAtByID(chestID)`）

### 症状（Symptom）

并发场景：

```
T0  client A: POST /chest/open → 事务内 SELECT ... FOR UPDATE 锁 chest row id=X
T1  client B: POST /dev/force-unlock-chest → UPDATE ... WHERE user_id=? 阻塞在 X 锁上
T2  A: Delete(id=X) + Create(new chest id=Y)（同 user_id，uk_user_id 仍唯一）
T3  A: commit → 锁释放
T4  B: UPDATE 恢复 → WHERE user_id=? 重新匹配 → 命中 id=Y (next chest)
T5  Y.unlock_at = now → 用户连开 2 次（next chest 已 unlockable）
```

dev 端点本意是"force-unlock 当前 chest"，并发下变成"force-unlock 下一轮 chest"。

### 根因（Root cause）

**`WHERE 业务索引列` 在 MySQL 锁等待 + 业务事务 Delete+Insert 链路下会"二次匹配"**：

- `WHERE user_id=?` 走 uk_user_id 唯一索引匹配的是 **commit 后可见的行**，不是"调用方调 UPDATE 时刻的行"
- MySQL InnoDB 的隔离级别（默认 REPEATABLE READ）只保证**本事务内**快照一致；锁等待结束后的 UPDATE 是新的语句、新的 read view，看到的就是 commit 后的 user_chests 表（含 Y，无 X）
- 调用方的"语义意图"是"unlock 调用时刻的 current chest"，但 `WHERE user_id` 没有携带"那一刻是哪一行"的信息，是盲打

更深层的反模式：把"按业务键查询" 和 "按业务键更新" 误以为是同一动作。实际上前者读 read view，后者持有 write lock —— 当业务键背后的"主键身份"在二者之间可能改变时，必须显式锚定到主键。

### 修复（Fix）

改成两步同事务模式（先 FOR UPDATE SELECT 拿主键 → 再 UPDATE WHERE 主键）：

```go
// before (race-prone)
func (s *devChestServiceImpl) ForceUnlockChest(ctx context.Context, userID uint64) error {
    return s.chestRepo.UpdateUnlockAt(ctx, userID, time.Now().UTC())
}

// after (race-safe)
func (s *devChestServiceImpl) ForceUnlockChest(ctx context.Context, userID uint64) error {
    return s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
        chest, err := s.chestRepo.FindByUserIDForUpdate(txCtx, userID)  // FOR UPDATE 拿 id
        if err != nil { /* 1003 / 1009 */ }
        return s.chestRepo.UpdateUnlockAtByID(txCtx, chest.ID, time.Now().UTC())  // WHERE id
    })
}
```

关键点：

- `FindByUserIDForUpdate` 让本事务排在 OpenChest commit 之后才能拿到锁
- 拿到 chest.ID 后，UPDATE 走 PRIMARY KEY 索引（WHERE id=?）—— 即便 commit 后 user_id 对应的物理行变了，本事务拿到的 id 就是 commit 后看到的那一行，UPDATE 必然命中**同一行**
- repo 接口签名同步从 `UpdateUnlockAt(userID, time)` → `UpdateUnlockAtByID(chestID, time)`；service 注入 txMgr；router wire 传 deps.TxMgr；4 个 stub 文件（stubChestRepo / stubHomeChestRepo / stubOpenChestChestRepo / faultChestRepo）签名同步；单测加 race-safe 断言（断 service 传给 repo 的是 chest.ID 而非 user_id）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"按业务索引列做 UPDATE 但该行可能被并发事务 Delete+Insert 替换" 的场景时，**必须**改成"事务内 SELECT ... FOR UPDATE 拿主键 id → UPDATE WHERE id = 该主键"两步模式；**禁止**用 `WHERE 业务索引列 = ?` 直 UPDATE，即使该列有唯一约束。
>
> **展开**：
> - 触发条件识别（任一满足）：
>   - 同 user_id / 同业务键的行会被并发事务**先 DELETE 再 INSERT**（如 chest 刷新下一轮、订单升级版本号、room 重建）
>   - 该业务键有唯一约束（uk_user_id / uk_business_key），让"删旧 + 插新"在唯一约束层面无冲突
>   - 你写的 UPDATE 与上述事务**可能并发**且**会阻塞在锁上**（即使你不在事务里）
> - 修复模式（"FOR UPDATE then UPDATE-by-id"）：
>   1. 把单 UPDATE 改成 `txMgr.WithTx(ctx, fn)` 包装
>   2. fn 内先 `repo.FindByXxxForUpdate(txCtx, businessKey)` 拿到该行的主键 id
>   3. 再 `repo.UpdateXxxByID(txCtx, id, ...)` 用主键 WHERE
>   4. repo interface 的 Update 方法签名必须接 `id` 而非业务键 —— 调用方拿到 id 是 race 安全的前提；接口签名层面就锁死
> - 单测断言要点：
>   - stub 的 UpdateXxxByID fn 内**显式断言**收到的 chestID 字段是从 stub Find 返回的 id（即 service 没把 user_id 误传成 chestID）；这是 race 修复在单测层面唯一能验证的语义信号
>   - 加 ChestNotFound 短路 case：FindByXxxForUpdate 返 ErrXxxNotFound 时，UpdateXxxByID 必须**不被调用**（事务 fn 内短路）
> - 集成测试要点：
>   - 用 dockertest 起真实 MySQL，并发跑"模拟 OpenChest（手工 SQL: BEGIN + SELECT FOR UPDATE + sleep + DELETE + INSERT + COMMIT）"+ `svc.ForceUnlockChest`
>   - 关键断言：force-unlock 不报错（特别是不报 rows_affected=0 ErrChestNotFound 这种 race 中间态）、表里仍只 1 行（uk 不破坏）
> - **反例**：
>   - 反例 A：`repo.UpdateUnlockAt(ctx, userID, time)` 直 UPDATE 单语句 —— 与 OpenChest 并发跑偏到 next chest
>   - 反例 B：用 `Save(&chest)` 模式 —— Save 会写全部字段；如果 chest 是 stale read 出来的，写回会覆盖 commit 后的 status/version；同样是 "WHERE id 但用 stale 字段" 的破坏
>   - 反例 C：在 service 层 SELECT 拿到 id 后**事务外**调 UPDATE —— FOR UPDATE 锁随 SELECT 返回即释放（autocommit 模式），race 窗口仍在
>   - 反例 D：把 `FindByXxxForUpdate` 放在 `WithTx` 外面、`UpdateXxxByID` 放在 `WithTx` 里面 —— FOR UPDATE 与 UPDATE 不同事务 = 锁不串行化，等于反例 C

## Meta: 本次 review 的宏观教训

dev 工具端点的"原子性需求"容易被低估 —— "dev 是给开发者用的，并发也不要紧" 是错的，因为 dev 端点常在自动化 e2e / 性能压测 / demo 演示里和业务路径**同时**触发。任何与业务事务**共享底层 row** 的写操作（不论是 dev 还是 prod）都必须按"会与业务并发"的标准做事务设计，**不**因为标的是 dev 就降级。

具体到本 lesson：判断"是否需要事务"的不是"语句数量"（单 UPDATE 看起来原子），而是"语句和并发链路的语义交互"（WHERE 子句会不会在锁等待后跑偏）。
