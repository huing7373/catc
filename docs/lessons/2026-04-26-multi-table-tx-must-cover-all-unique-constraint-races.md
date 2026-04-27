---
date: 2026-04-26
source_review: epic-loop-review-4-6-r1.md (codex P1)
story: 4-6-游客登录接口-首次初始化事务
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — 多表事务必须穷举所有唯一约束的 race 路径（不同表的唯一约束需要独立 sentinel + 全部走 reuseLogin）

## 背景

Story 4.6 实装游客登录的 5 表初始化事务（users / user_auth_bindings / pets / user_step_accounts / user_chests）。dev-story 在 service 层处理了一条并发 race：`user_auth_bindings.uk_auth_type_identifier` 冲突 → `ErrAuthBindingDuplicate` → 回退到 `reuseLogin`。codex review-r1 指出**还有第二条 race 路径**：`users.uk_guest_uid` 也是唯一约束（migration 0001 + 数据库设计 §5.1），且 users 是事务内的**第一步**（比 binding 更早 INSERT），并发场景下"先 commit 的 Tx → 后到 Tx 在 users 阶段就被 1062 拒掉"完全成立 —— 但当前 service 只识别 binding-dup，users-dup 走 generic error → 1009，违反 V1 §4.1 钦定的 "同一 guestUid 重复调用 → 同一 user_id" 幂等语义。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | users.uk_guest_uid race 没处理 → 1062 落入 generic error 1009 | high | error-handling, architecture | fix | `server/internal/service/auth_service.go:297-308` + `server/internal/repo/mysql/user_repo.go:Create` + `server/internal/repo/mysql/errors.go` |

## Lesson 1: 多表事务下不同表的唯一约束需要独立 sentinel + service 必须穷举所有 race 路径

- **Severity**: high
- **Category**: error-handling, architecture
- **分诊**: fix
- **位置**: `server/internal/service/auth_service.go:297-308` (race 处理分支) + `server/internal/repo/mysql/user_repo.go:Create` (1062 翻译) + `server/internal/repo/mysql/errors.go` (新加 `ErrUsersGuestUIDDuplicate`)

### 症状（Symptom）

5 表初始化事务里，第 1 步 `INSERT users` 和第 3 步 `INSERT user_auth_bindings` 都受唯一约束保护：

- `users.uk_guest_uid (guest_uid)`
- `user_auth_bindings.uk_auth_type_identifier (auth_type, auth_identifier)`

并发场景下 Tx A 先 commit，Tx B 进入 firstTimeLogin 后：

- 路径 1（A 已 commit）：B 的 `INSERT users` 直接 1062（uk_guest_uid 冲突）→ 永远走不到 INSERT binding
- 路径 2（A 未 commit，B INSERT users 不被 block）：B 的 `INSERT binding` 1062（uk_auth_type_identifier 冲突）

dev-story 只覆盖了路径 2（service 检 `ErrAuthBindingDuplicate`），漏了路径 1 → 路径 1 的 B 会拿到 `ErrServiceBusy(1009)` → 客户端误以为服务异常重试，违反"同 guestUid → 同 user_id"幂等。

### 根因（Root cause）

写 race 处理时只回看了"我刚写过 1062 翻译的那一个 repo"（user_auth_bindings_repo），没穷举**事务内所有 INSERT 的所有唯一约束**。错误的隐式假设：

- "binding 表的 uk_auth_type_identifier 是 guestUid 唯一性的实施点 → 唯一 race 点"
- 真相：users 表也有 uk_guest_uid，同样实施 guestUid 唯一性，且**位置更前**

更深层的方法论缺口：5 表事务有 5 个表 × 各自的若干唯一约束 → race 路径数 = sum 而非 1。每加一张表 / 每加一个唯一索引，都必须扩 race 处理。当前事务里只有 users.uk_guest_uid + user_auth_bindings.uk_auth_type_identifier 两条业务可识别 race（pets.uk_user_default_pet / user_step_accounts.PK / user_chests.uk_user_id 都依赖 user_id，而 user_id 是事务内 AUTO_INCREMENT 新生成的，不会撞已存在行 —— 这些约束在本事务流程里**不**会触发并发 race）。

### 修复（Fix）

1. **`server/internal/repo/mysql/errors.go`** 新加 sentinel：
   ```go
   ErrUsersGuestUIDDuplicate = errors.New("mysql: users guest_uid duplicate (uk_guest_uid conflict)")
   ```
   独立而非复用 `ErrAuthBindingDuplicate` —— 不同表的 1062 必须用不同 sentinel，否则 service 无法判断"冲突源是哪个表"，即使最终行为一样，故障定位 / 日志归因都会丢信息。

2. **`server/internal/repo/mysql/user_repo.go:Create`** 检测 1062 翻译为 `ErrUsersGuestUIDDuplicate`：
   ```go
   var mysqlErr *mysql.MySQLError
   if stderrors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrCodeDupEntry {
       return ErrUsersGuestUIDDuplicate
   }
   ```
   不解析 Message 字符串区分 key 名 —— users 表当前**只有** uk_guest_uid 这一个唯一索引（PRIMARY 是 AUTO_INCREMENT 不会冲突），所以 1062 必然是 guest_uid 冲突，无歧义。注释里明示"future 给 users 加新唯一索引时本函数注释要补按 key 名分流"。

3. **`server/internal/service/auth_service.go:297-308`** race 分支同时识别两个 sentinel：
   ```go
   if stderrors.Is(err, mysql.ErrAuthBindingDuplicate) ||
      stderrors.Is(err, mysql.ErrUsersGuestUIDDuplicate) {
       binding, ferr := s.authBindingRepo.FindByGuestUID(ctx, guestUID)
       ...
       return s.reuseLogin(ctx, binding.UserID)
   }
   ```
   两个 sentinel 的反应**必须一致**（都回退 reuseLogin），但**判定**必须独立写出 —— 不要写成 `errors.Is(err, "any duplicate")` 类的笼统检查。

4. 单测覆盖（必须）：
   - `users_repo_test.go::TestUserRepo_Create_DuplicateGuestUID_ReturnsErrUsersGuestUIDDuplicate` — sqlmock 抛 1062 with key 名 'uk_guest_uid' → 验证 repo 返 ErrUsersGuestUIDDuplicate
   - `auth_service_test.go::TestAuthService_GuestLogin_DuplicateGuestUID_FallbackToReuseLogin` — stub userRepo.Create 抛 ErrUsersGuestUIDDuplicate → 验证 service 走 reuseLogin（FindByGuestUID 二次调用拿到先入者 binding，FindByID 返 winner user）
   - 集成测试 `auth_service_integration_test.go::TestAuthService_GuestLogin_ConcurrentSameGuestUID_BothSucceedSameUserID` — 真 mysql:8.0 容器 + 两 goroutine 同 guestUid 并发 → 验证两次都成功 + 同 user_id + DB 仅 1 行 user/binding/pet/step/chest

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **多表事务的并发 race 处理分支**里，**必须穷举事务内 INSERT 路径上的所有唯一约束**，并为**每一个**不同表的唯一约束产出独立 repo sentinel + 在 service 层把它们**全部**纳入"先入者赢，回退 reuseLogin"的判定列表 —— 任何一个唯一约束漏判都会让幂等语义在某个 timing 下退化为 1009。
>
> **展开**：
> - 写 race 分支前，列出事务内所有 INSERT 步骤 → 对每个步骤检查目标表的全部 UNIQUE KEY → 评估"在此事务执行期间该唯一索引是否可能被并发 Tx 触发 1062"
> - 一个唯一约束触发条件：**约束列**的取值在并发场景下可能被两个 Tx 写入相同 → race 可达。约束列里只有**事务内 AUTO_INCREMENT 的新 ID** 时（如 pets 的 user_id 来自本事务刚 INSERT 的 users.id），race 不可达
> - 不同表的 1062 必须用**不同的 sentinel error**（`ErrUsersGuestUIDDuplicate` / `ErrAuthBindingDuplicate` / 等等），不要试图共享一个 `ErrAnyDuplicate` —— 故障定位时需要从 sentinel 反推冲突源，单测时也需要按 sentinel 分别构造
> - service 层 race 分支用 `errors.Is(err, A) || errors.Is(err, B) || errors.Is(err, C)` 显式列举所有可能的 race sentinel；**不**用 "errors.Is duplicate-ish" 的 fuzzy 判定
> - 每加一张表 / 每加一个唯一索引到事务，都要回看 race 分支是否需要扩。把"race coverage matrix"放到 service 函数顶部 godoc 里（"# 并发幂等"一节列**所有**唯一约束 + race 路径）
> - **反例**：service 层 race 分支只检 `ErrAuthBindingDuplicate`，理由是"binding 是 guestUid 的实施点"—— 错。多表事务里"业务唯一性"可能被多个表共同实施，每个实施点都是 race 候选。同样错的反例：repo 层把 users 的 1062 和 binding 的 1062 都翻译成 `ErrAnyConflict` —— 共用 sentinel 导致 service 无法区分冲突源 + 单测无法精确断言。
> - **反例（识别策略）**：用 1062 错误信息字符串解析 key 名（"`Duplicate entry ... for key 'uk_guest_uid'`"）—— 不可靠（国际化 / 版本可能改文案）。用 `mysql.MySQLError.Number == 1062` 比较；如果一张表有多个唯一索引，**才**需要进一步按 key 名 fan-out（届时按 Message 解析或在 SQL 层用 `INSERT ... ON DUPLICATE KEY UPDATE` 之类显式区分；不再信赖隐式 1062 + 单一表只有一个索引的假设）

## Meta: 本次 review 的宏观教训

review 暴露的不只是"漏了一条 race"这个具体错误，而是**race 分析的方法论欠缺**：

- dev-story 把 race 处理写成了 "我刚加的 binding 创建可能冲突 → 我加个分支接住" 的局部反应式动作；
- 正确的姿势是 "事务内有 N 个 INSERT，每个 INSERT 撞唯一索引都可能并发 1062 → 完整画一遍 race coverage matrix"。

未来写多表事务的 race 处理：先在 godoc 里列出 race coverage matrix（每一列：步骤号 / 表名 / 唯一索引名 / race 是否可达 / 处理动作），代码再 follow matrix。matrix 跑完后再下笔写 race 分支。
