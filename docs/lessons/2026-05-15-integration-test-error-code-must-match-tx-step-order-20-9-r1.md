---
date: 2026-05-15
source_review: codex review round 1 / /tmp/epic-loop-review-20-9-r1.md
story: 20-9-layer-2-集成测试-开箱事务全流程
commit: 09d28df
lesson_count: 1
---

# Review Lessons — 2026-05-15 — 集成测试错误码断言必须忠实于事务步骤顺序（OpenChest 5d-5f 的 unlock_at 检查在 step 检查之前）

## 背景

Story 20.9 落地 12 个 chest_open_service Layer 2 集成测试 case。其中 AC9（`Concurrent100DifferentKeys_StepLimitOnlyOneOpens`）构造 race：

- 同一用户 1500 步（仅够 1 次开箱）
- 100 个 goroutine 用不同 idempotencyKey 并发调 `OpenChest`
- 期望：1 个成功 + 99 个失败

测试原断言 99 个失败码为 `ErrInsufficientSteps` (3002)。codex review r1 指出实际 race 行为不同：失败码应为 `ErrChestNotUnlocked` (4002)。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | concurrent different-key 测试期望错误码 3002 实际应为 4002 | P1 | testing | fix | `server/internal/service/chest_open_service_integration_test.go:799-814` |

## Lesson 1: 并发集成测试的错误码断言必须基于事务**步骤顺序**逐帧推演，而非凭"业务直觉"

- **Severity**: high (P1)
- **Category**: testing / concurrency
- **分诊**: fix
- **位置**: `server/internal/service/chest_open_service_integration_test.go:799-814`

### 症状（Symptom）

测试设计者的直觉推理：

```
1500 步只够开 1 次 → 1 个 goroutine 扣 1000 步 → 剩 500
   → 其他 99 个 FOR UPDATE 排队，commit 后看 available_steps=500 < 1000
   → 返回 ErrInsufficientSteps (3002)
```

实际运行行为（步骤顺序推演）：

```
T0  100 个 goroutine 各 INSERT 自己的 pending idempotency 行（uk_user_id_key 是 user+key
    → 不同 key 互不冲突 → 全部 affectedRows=1 → 全部进步骤 5c 业务流程）
T1  100 个事务在步骤 5c FindByUserIDForUpdate(userID) 等 chest row 的 X-lock
T2  goroutine #X 拿到锁:
       5d: chest.unlock_at <= now → isUnlockable=true → 通过
       5e: account.available_steps=1500 >= 1000 → 通过
       5f: Spend 扣 1000 (available_steps=500)
       5g-h: 抽奖 + 写 log
       5i: DELETE chest id=A + INSERT chest id=B（**unlock_at = now + 10min**，未来时刻）
       5k: MarkSuccess
       commit
T3  其他 99 个 goroutine unblock，各自 FOR UPDATE 拿到的是新 chest id=B
       5d: chest.unlock_at > now → isUnlockable=false → **返回 ErrChestNotUnlocked (4002)**
       (never reach 5e 的 available_steps 检查)
       ROLLBACK（包含 pending idempotency 行）
```

错误码序：99 个失败事务**全部**在 5d 被拦下，never reach 5e → 失败码是 4002 而非 3002。

### 根因（Root cause）

测试设计者的思维漏洞：**用业务语义（"步数不足"）反推错误码，而非按代码实际步骤顺序推演**。

具体的事实链：

1. **OpenChest 步骤顺序**（`chest_open_service.go` runOpenChestTx）：
   - 5c: `FindByUserIDForUpdate` chest
   - 5d: 检查 `chest.unlock_at <= now` → fail 抛 4002
   - 5e: `FindByUserIDForUpdate` step_account → 检查 `available_steps >= 1000` → fail 抛 3002
   - 5d 在 5e **之前** → 任一 chest 未解锁场景永远先抛 4002

2. **race 后 chest 状态**（步骤 5i）：旧 chest DELETE + 新 chest INSERT（unlock_at = now+10min）→ 99 个排队事务拿到的是**新 chest**，未来时刻

3. **uk_user_id 唯一索引**：保证 chest row 在任一时刻同 user_id 仅 1 行 → 99 个事务的 FOR UPDATE 必然全部命中新 chest

测试断言基于"如果 99 个失败事务能跑到 5e"的假设，但实际它们在 5d 就被拦下了 —— 这是事务步骤顺序 + 业务规则（unlock_at 刷新到未来）共同决定的。

**根本误解**：把测试当成"我设计一个场景验证业务直觉"而非"我跟踪代码执行路径验证实际行为"。集成测试的本质是**端到端跟踪生产路径**，断言必须 100% 对应代码会走的分支 —— 不能基于"应该会发生什么"，必须基于"代码顺序读下来会发生什么"。

### 修复（Fix）

`server/internal/service/chest_open_service_integration_test.go` 第 749-829 行：

- 测试函数头注释完整改写，描述 race 真实流程：
  - 第一个事务 commit 后 chest 已 DELETE + 新 chest（unlock_at = now+10min）
  - 99 个排队事务拿到新 chest → 步骤 5d unlock_at 检查失败 → 4002
  - 步骤 5d 在 5e 之前 → never reach 3002
- 计数器从 `insufficientCount` 扩成三路：`chestNotUnlockedCount` / `insufficientCount` / `otherErr`
- 断言期望从 99 个 3002 改为 99 个 4002 + 0 个 3002 + 0 个 other（用 `apperror.ErrChestNotUnlocked` 常量比较，不硬编码 4002）
- 同步更新文件头 `case 10` 描述（line 20）

**顺带改动**：story 文件 `_bmad-output/implementation-artifacts/20-9-layer-2-集成测试-开箱事务全流程.md` 的 AC9 段（行 37 / 65 / 641-733 / 1073 / 1147）也按真实 race 行为更新 + 添加 r1 修正说明，避免文档与测试断言漂移。

**未影响的 case**：AC8（`Concurrent100SameKey`）用 ClaimPending `INSERT ... ON DUPLICATE KEY UPDATE` → 同 key 后续事务 affectedRows=0 → 走 cached replay 路径（步骤 5b），never racing chest → 不受同类问题影响。

### 预防规则（Rule for future Claude）⚡

> **一句话**：**写并发集成测试断言错误码时，禁止凭"业务直觉"推理，必须打开被测 service 源码、按事务步骤序号（5a / 5b / 5c ...）逐帧推演第二个及后续事务在每一步可能命中的不同状态，第一个抛错的步骤决定错误码**。
>
> **展开**：
> - **认知校准**：业务直觉对"应该"成功/失败有用，但对"为什么具体码"无用 —— "步数不足"和"chest 未解锁"在直觉里都是"开不了箱"，但在错误码层面是两个不同 enum
> - **强制 SOP**：测试 race / 排队场景前，先打开 service 源码画一张"步骤 N: 检查 X → 失败码 Y"的小表，再标注"race 后步骤 N 命中什么状态"，最后断言才有依据
> - **关键观察**：业务事务**通常**会刷新被锁资源（OpenChest 5i 把 chest 推到未来；其他类似流程可能 reset version / unlock_at / status）→ race 第二批事务拿到的资源已不是初始状态，错误码取决于**新状态**而非初始状态
> - **断言模板**：并发 race 失败的断言必须用**三计数器**（expected / unexpected-known / other-unknown），不能只断言期望码总数 —— 期望码错了时这种写法**永远 pass**（计数器累不到 N-1 但 otherErr 也不查就漏掉）。本次 fix 后的模板：
>   ```go
>   succeededCount, expectedErrCount, knownAlternateErrCount, otherErr := 0, 0, 0, 0
>   for _, r := range results {
>       if r.succeeded { succeededCount++ }
>       else if r.errCode == ErrExpected { expectedErrCount++ }
>       else if r.errCode == ErrPlausibleAlternate { knownAlternateErrCount++ }
>       else { otherErr++ }
>   }
>   // 断言全部三路 + otherErr=0
>   ```
> - **反例 1**：本 case 原版只用两计数器 `insufficientCount / otherErr`，当 99 个实际都是 4002 时 `insufficientCount=0` (fail) + `otherErr=99` (fail) —— 测试虽然挂了，但**挂的原因看起来像 race 不稳定**（"99 个其他错误码？是哪些？"）而非"期望码彻底搞错了"。三计数器版能立即告诉你 `chestNotUnlockedCount=99` —— 答案就在断言失败信息里
> - **反例 2**：基于"业务最终结果是步数不够"推理 → 直接断言 `ErrInsufficientSteps`。这是把**业务因果**当成**代码路径**的典型混淆 —— 业务因果是"因为前一个事务花了步数所以后面的人开不了箱"；代码路径是"因为前一个事务**还**重置了 chest unlock_at 所以后面的人在 5d 就被拦下了"。两者结论不同时，代码路径胜
> - **反例 3**：写完测试看 `t.Errorf` 输出 "otherErr=99 (unexpected error codes)" 后**只统计未知码数量**而不进一步打印是什么码 —— 应该把 errCode 直接 t.Logf 出来（或挂死前打印分布），让失败信息含诊断价值。本 case 如果原测试日志 `otherErr=99` 同时也打了 "all errCode=4002"，根因 1 行就能看出

## Meta: 本次 review 的宏观教训

**集成测试 ≠ 行为验证；集成测试 = 代码路径追踪**。Layer 1 单元测试可以 stub 出"业务直觉里的世界"，因为它本来就是验证某个分支；Layer 2 集成测试跑的是**真实 service + 真实 DB + 真实事务隔离级别**，所有断言必须可以追溯到"假如打开 strace / SQL log，每一步会看到什么 SQL + 什么返回值"。

把"我以为会发生什么"写进集成测试断言 = 给未来踩坑埋雷。每一个并发测试 case 在写断言前，**必须**先在脑子里跑一遍"如果我开两个 mysql client 手动序列化执行，看到的错误是什么"—— 然后才写 `t.Errorf`。
