---
date: 2026-05-15
source_review: codex review round 2 on Story 20.7 (file: /tmp/epic-loop-review-20-7-r2.md)
story: 20-7-dev-端点-post-dev-force-unlock-chest
commit: 1d44ab2
lesson_count: 2
---

# Review Lessons — 2026-05-15 — "current chest" race 的彻底解：把 id 决策权交给 client；MySQL RowsAffected=0 不等同 NOT FOUND

## 背景

Story 20.7（`POST /dev/force-unlock-chest`）r1 lesson 提出"FOR UPDATE SELECT 拿 chest id → UPDATE WHERE id"两步同事务模式来防与 `/chest/open` 并发时跑偏到 next chest。codex r2 review 指出 r1 修复其实没真正堵住 race：FOR UPDATE 阻塞结束后 SELECT 返回的是 commit **之后** 的"当前 chest"（即 OpenChest 刚 INSERT 的 next chest Y），跟 r0 一样跑偏。同时 review 指出 repo 层依赖 `RowsAffected=0 → ErrChestNotFound` 翻译有 MySQL UPDATE 在值未变时也返 RowsAffected=0 的陷阱（同毫秒重复调用会误判 1003）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | "current chest" race 的彻底解：把 id 决策权交给 client（GET /chest/current 拿 id → POST /dev/force-unlock-chest 带 id）| P2 | architecture | fix | `server/internal/service/dev_chest_service.go` `server/internal/app/http/handler/dev_chest_handler.go` `server/internal/repo/mysql/chest_repo.go` |
| 2 | MySQL UPDATE 在值未变时返 RowsAffected=0 ≠ NOT FOUND；存在性校验必须前置到 SELECT，不能拿 UPDATE 的 RowsAffected 当存在性信号 | P2 | error-handling | fix | `server/internal/repo/mysql/chest_repo.go` `server/internal/repo/mysql/chest_repo_test.go` |

## Lesson 1: "current chest" race 的彻底解 —— 把"哪个 id"决策权交给 client，server 不要再猜"current"

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/service/dev_chest_service.go:101` / `server/internal/app/http/handler/dev_chest_handler.go:55-105` / `server/internal/repo/mysql/chest_repo.go`

### 症状（Symptom）

r1 实装："service 事务内 FindByUserIDForUpdate(userID) → 拿 chest.id → UpdateUnlockAtByID(chest.id, now)"。看似可以让 force-unlock 在 OpenChest commit 后串行化执行，命中 commit 后的 chest。

实际 race：

```
T0  client A: POST /chest/open 进入事务 → FOR UPDATE 锁 chest row id=X
T1  client B: POST /dev/force-unlock-chest 进入事务 → FOR UPDATE 阻塞在 X 锁
T2  A: Delete(id=X) + Create(new chest id=Y) → uk_user_id 仍 = user_id
T3  A: commit → 锁释放
T4  B 的 FOR UPDATE 恢复 → SELECT 返回的 chest 是 **commit 后的快照** → 拿到 chest.id = Y（next chest）
T5  B: UpdateUnlockAtByID(Y, now) → 把 next chest 直接推到 unlock_at=now
T6  用户拿到"连开 2 次"的非预期效果
```

r1 跟 r0（WHERE user_id 直更）**等价**：commit 后 SELECT WHERE user_id 也是匹配 Y，UPDATE WHERE id=Y 跟 UPDATE WHERE user_id=user_id（最终命中 Y）结果一致。FOR UPDATE 改造**完全没修这个 race**。

### 根因（Root cause）

试图让 server 在并发场景下"自己猜"哪个 chest 是"当前 chest"——但"当前 chest"是 **client 视角的时刻概念**（"我刚 GET /chest/current 看到的那个 id"），而 server 看到的 commit 后快照可能已经变成 next chest 了。**server 不可能从纯 server-side context 推导出 client 视角的"当前 chest"是哪个 row**。

凡是涉及"current X"的 server 端写操作，只要 X 可能被并发 DELETE+INSERT 刷新（比如 chest 开箱后刷新下一轮），server 用 user_id 匹配现有 row 都会跑到 next X，不管有没有 FOR UPDATE。

### 修复（Fix）

把"哪个 id"的决策权交给 client：

1. **DTO 改造**：`PostForceUnlockChestRequest` 加 `ChestID *string` 字段（BIGINT 字符串化，与 V1 §2.5 + `GET /chest/current` response.data.id 类型对齐）
2. **handler 校验**：`ChestID` 必传 + 长度 1-20 + ParseUint 成功 + != 0（与 `room_handler.JoinRoom` 解析 roomId 同模式）
3. **service 改造**：`ForceUnlockChest(ctx, userID, chestID)` —— 用 `chestRepo.FindByID(chestID)` 校验存在 + 校验 `chest.UserID == claimedUserID`（防越权）；UPDATE WHERE id 直接动 unlock_at
4. **service 不再开事务**：chest 一旦绑定 user_id，user_id 永不变；UPDATE 走 PK；race 不再成立（极端场景 chest 在 SELECT 后被 OpenChest 删除 → UPDATE 命中 0 行视为成功，dev 已无须维护"current"语义）
5. **router 改造**：`NewDevChestService(chestRepo)` —— 移除 txMgr 参数
6. **错误码**：chest 不存在 OR 越权（chest 属于别人）都返 1003 ErrResourceNotFound，与 ChestNotFound 同码避免暴露"chestID 存在但属于别人"信息

before（r1 错误模式）：

```go
return s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
    chest, _ := s.chestRepo.FindByUserIDForUpdate(txCtx, userID) // race-after 拿到 next chest id
    s.chestRepo.UpdateUnlockAtByID(txCtx, chest.ID, now)         // 把 next chest 推到 now
    return nil
})
```

after（r2 正确模式）：

```go
// (1) 取 chest，校验存在 + 归属
chest, err := s.chestRepo.FindByID(ctx, chestID) // client 传入的 id
if err != nil { ... }
if chest.UserID != userID { return apperror.New(ErrResourceNotFound, ...) }

// (2) UPDATE WHERE id，不看 RowsAffected
s.chestRepo.UpdateUnlockAtByID(ctx, chestID, time.Now().UTC())
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**设计涉及"current X"的 server 端写操作**时，**必须**把 X.id 作为入参从 client 传入，不要让 server 用 `WHERE user_id` 或类似匹配条件自己"找当前 X"——尤其当 X 可能被并发链路 DELETE+INSERT 刷新时。
>
> **展开**：
> - "current"是 client 视角的时刻概念（"我刚 GET 看到的那个 id"），server 看到的快照可能已经变成下一个 X 了
> - FOR UPDATE / 事务都救不了这个 race —— 阻塞结束后 SELECT 返回的是 commit 后的快照，仍然是下一个 X
> - 让 client 先 GET 拿到 id，再 POST 带 id 来 —— server 用 PK + 归属校验，race 自然消失
> - 配套防御：dev / 无 auth 端点必须在 service 层做 `record.user_id == claimedUserID` 比对，防恶意 client 传别人的 id 越权操作；错误码用 NotFound 同码避免信息泄露
> - **反例 1（最常见）**：dev 端点接 `{userId}` 不接 `{chestId}`，service 用 `chestRepo.FindByUserID(userID)` 拿 chest 后 UPDATE —— 与刷新链路并发时跑偏
> - **反例 2（更隐蔽）**：用 `chestRepo.FindByUserIDForUpdate(userID)` 加 FOR UPDATE 同事务保护 —— **照样**跑偏（FOR UPDATE 阻塞结束后拿到的还是 commit 后的 next chest），但表面上"加了事务+锁"的感觉会误导 reviewer 以为安全
> - **反例 3**：盲目"先 SELECT FOR UPDATE 后 UPDATE"两步同事务模式套用到所有"读后写"场景 —— 这个模式在防"丢失更新"是对的（OpenChest 那种 version+1 路径），但在防"row identity 漂移"（current 概念）时无效

## Lesson 2: MySQL RowsAffected=0 不等同 NOT FOUND —— 存在性校验必须 SELECT 前置

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/chest_repo.go:201-205`

### 症状（Symptom）

r1 repo 实装：

```go
result := db.WithContext(ctx).Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)
if result.RowsAffected == 0 {
    return ErrChestNotFound
}
return nil
```

service 层 `errors.Is(err, ErrChestNotFound)` 后翻译为 1003。

bug：MySQL UPDATE 默认连接（不设 `clientFoundRows=true`）在**值未变时** RowsAffected=0（返回的是"changed rows"而非"matched rows"）。同毫秒内连续两次 force-unlock 同一 chest → 第二次 newUnlockAt 与现值相同（毫秒级精度对齐）→ RowsAffected=0 → 误返 1003 ErrChestNotFound。自动化脚本重试 / 网络重发场景必中。

### 根因（Root cause）

把 UPDATE 的副作用计数（RowsAffected）当成存在性信号用。MySQL 的 RowsAffected 语义在 driver 默认连接下是"实际修改了几行"，不是"WHERE 条件匹配了几行"。如果要区分这两个语义需要在 DSN 加 `clientFoundRows=true`，但这会侵入全局连接行为，不优雅。

正确的语义分离：

- **存在性 + 业务前置校验** → SELECT（GORM First 或自定义 EXISTS query）—— ErrRecordNotFound 是显式信号
- **写副作用** → UPDATE，不看 RowsAffected（或只在乐观锁场景看 RowsAffected 来判断 version 冲突，这是另一码事）

### 修复（Fix）

1. **repo.FindByID 引入**：`SELECT * FROM user_chests WHERE id = ? LIMIT 1`，NotFound → ErrChestNotFound 哨兵
2. **repo.UpdateUnlockAtByID 简化**：不再检查 RowsAffected；只透传 DB error

```go
// before (r1):
result := db...Update("unlock_at", newUnlockAt)
if result.Error != nil { return result.Error }
if result.RowsAffected == 0 { return ErrChestNotFound }
return nil

// after (r2):
result := db...Update("unlock_at", newUnlockAt)
return result.Error
```

3. **service 层语义分工**：存在性校验由 `FindByID` 做（在 UPDATE 之前），UPDATE 不再扮演"二次存在性确认"角色
4. **集成测试新增 case**：连续 2-3 次 force-unlock 同 chest，验证不再返 1003（dev_chest_service_integration_test.go case 3）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **MySQL UPDATE / DELETE 之后看 RowsAffected** 时，**禁止**把 `RowsAffected=0` 直接当成"行不存在"，**必须**先用独立的 SELECT 做存在性校验，再做 UPDATE / DELETE 而不看 RowsAffected。
>
> **展开**：
> - MySQL UPDATE 默认 driver 连接（无 `clientFoundRows=true`）的 RowsAffected = "实际改变的行数"，不是"匹配的行数"
> - 值未变时 RowsAffected=0；行不存在时 RowsAffected 也是 0 —— 两个语义混在一个返回值里，永远区分不开
> - 想区分要么改 DSN 加 `clientFoundRows=true`（侵入全局，且团队成员不一定知道），要么用两步：SELECT 校验 + UPDATE 不看 RowsAffected
> - 推荐：**两步法**。语义清晰，不依赖 driver 全局参数，存在性 / 业务前置校验路径在 SELECT 那一步显式表达
> - **乐观锁场景例外**：UPDATE ... WHERE version=? → RowsAffected=0 表达"version 已变（并发改了）"是合法语义，因为查询条件含 version 不只是 PK；但即便这里也建议在错误信息里区分"NotFound vs VersionConflict"靠业务层
> - **反例 1**：`UPDATE ... WHERE id=? SET unlock_at=?`，`if RowsAffected == 0 { return ErrNotFound }` —— 同毫秒重复调误判
> - **反例 2**：`DELETE ... WHERE id=?`，`if RowsAffected == 0 { return ErrNotFound }` —— DELETE 的 RowsAffected 在 MySQL 是 matched rows，这种情况下勉强 OK，但**不要把 UPDATE 的经验混到 DELETE**，DELETE 比 UPDATE 安全只是巧合，不要泛化"用 RowsAffected 当存在性信号"作为通用模式
> - **反例 3（修复方向陷阱）**：用 `clientFoundRows=true` DSN 参数让 RowsAffected 返 matched rows —— 这能让"行不存在"的 RowsAffected=0 语义干净，但**值未变也算 matched**，依然分不开"行不存在"和"行存在但值没变"两个语义；只是把陷阱换了个方向

---

## Meta: 本次 review 的宏观教训

r1 lesson（前一天）和 r2 lesson 形成的对照很有教学价值：

- **r1 lesson 的"FOR UPDATE 防止 WHERE 二次匹配"模式本身没错**，但解决的是**不同的 race 类**（防丢失更新 / 防 phantom read），不是"row identity 漂移"（current 概念）race。把它套用到 dev force-unlock 是误用 —— FOR UPDATE 阻塞结束后 SELECT 返回 commit 后的新 row id，反而**掩盖了**真正的 race。
- 这是一个 **"似是而非的修复"** 反模式：表面上加了"事务 + 锁"的复杂度让 reviewer 觉得"看起来更严谨"，但实际上没解决问题，反而把"哪个 id"的盲打从 `WHERE user_id` 转嫁到 `FOR UPDATE SELECT` 仍然盲打。codex r2 review 能识破这一点是非常关键的 review pass。
- 教训：评估 race 修复时**必须画出完整时序图**（A 的事务边界 + B 的阻塞窗口 + commit 后快照 + B 恢复后的 SELECT 结果），不要凭"加了 FOR UPDATE 应该更安全"的直觉判定。如果时序图上 B 恢复后的 SELECT 仍能看到 A 刷新出来的新 row，那 race 没修。
- 一般规则：涉及"current X"的 server 端操作，**只要 X 可能被 DELETE+INSERT 刷新**，FOR UPDATE 都救不了。正解：让 client 把 X.id 当参数传进来。
