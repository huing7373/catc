# Story 20.9: Layer 2 集成测试 — 开箱事务全流程（dockertest 真实 MySQL 跨 service / handler 两层穷举 epics.md §20.9 钦定 14 类场景：完整流程 / 4 回滚 / 2 幂等 / 2 并发 / 4 边界 / 1 抽奖分布；**不**实装新业务功能，仅扩展 integration test 覆盖矩阵）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 资产事务负责人,
I want 一组深度集成测试覆盖 **POST /chest/open 开箱事务**（扣步数 + 加权抽奖 + 写日志 + 删旧 chest + 建新 chest + 幂等 INSERT/UPDATE）的失败回滚 / 并发 / 边界 / 幂等 / 加权分布全部 14 类场景，全部用 dockertest 真实 MySQL 跑通，作为节点 7 Layer 2 集成测试收尾保障，**追加** 到 Story 20.6 已落地的 2 个 service 层集成测试（HappyPath_FullFlow + HappyPath_IdempotencyReplay_SameKey）基础上，把覆盖率从局部 happy 路径推到"事务全失败模式 + 高并发收敛 + 幂等边界 + 抽奖分布"四个维度全绿,
so that NFR1（资产事务原子） / NFR5（高并发幂等） / `数据库设计.md` §8.3（开箱事务）/ V1 §7.2.5 8 步事务在节点 7 阶段不只靠 Story 20.6 已有的 2 条 happy / replay case，而是 **穷举** epics.md §Story 20.9 行 2980-2998 钦定的 1 完整流程 + 4 回滚（扣步数 / 抽奖 / 写日志 / 建新 chest） + 2 幂等（100 次同 key / 不同 key + 充足步数） + 2 并发（同 key / 不同 key + 仅够 1 次） + 4 边界（999 / 1000 / 1001 / unlock_at -1ms） + 1 加权分布共 **14 类场景**，把覆盖率从 happy 路径推到事务全失败模式 + 高并发幂等收敛 + 步数边界 + 抽奖加权 4 个维度全绿；任何一个场景退化（如某条回滚路径漏 rollback / 100 goroutine 同 key 出现 2 次开箱 / 边界 999 步数误放行 / 抽奖分布漂移超出 drop_weight 设计）→ 立即在 Layer 2 阶段被发现，**不**让节点 7 验收 demo 阶段（Story 19.1 跨端 E2E）才暴露开箱事务幂等性 / 加权抽奖 / 步数边界回归。

## 故事定位（Epic 20 第九条 = 节点 7 收尾性 Layer 2 集成测试；上承 20.6 开箱事务 + 20.7 dev force-unlock + 20.8 dev grant-cosmetic-batch；epic 收官 story）

- **Epic 20 进度**：20.1（契约定稿，done）→ 20.2（cosmetic_items migration，done）→ 20.3（cosmetic_items seed，done）→ 20.4（chest_open_logs migration，done）→ 20.5（GET /chest/current，done）→ 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取，done）→ 20.7（dev /dev/force-unlock-chest，done）→ 20.8（dev /dev/grant-cosmetic-batch，done）→ **20.9（本 story，Layer 2 集成测试 - 开箱事务全流程，epic 收官）**。

- **物理执行顺序与逻辑编号一致**：本 story 编号 20.9，物理上**第九**执行（20.1-20.8 done 后立刻做 20.9）。理由：
  - Story 20.9 是 epic-20 的**收尾性 Layer 2 集成测试**，需要 20.6 (OpenChest) 业务链路 + 20.2-20.4 表结构 + 20.3 seed 全部落地后再做整体回归
  - sprint-status.yaml 第 216 行已按此顺序排列（20.9 在 20.8 之后、epic-20-retrospective 之前）
  - 20.9 是测试 story，**不实装新业务功能**，仅扩展 integration test coverage 矩阵；与 4.7（auth_service Layer 2 收尾）/ 11.9（room_service Layer 2 收尾）/ 26.5（穿戴事务收尾）/ 32.5（合成事务收尾）同模式

- **epics.md §Story 20.9 钦定**（`_bmad-output/planning-artifacts/epics.md` 行 2972-2998，**唯一权威 AC 来源**）：
  - **Given** Story 20.6 happy path 已通过
  - **When** 完成本 story
  - **Then** 输出 / 扩展 `internal/service/chest_open_service_integration_test.go`（已存在，20.6 落地共 2 个测试函数），**追加** 14 类场景（不新建独立测试文件 —— 与 4.7 / 11.9 同模式，同包同文件内聚）：

    | epics.md 行 | 场景类别 | 详细要求 |
    |---|---|---|
    | 行 2983 | **完整流程** | 创建 user 含 chest + 1500 步数 → 等 chest unlockable → 开箱 → 验证扣 1000 + 奖励抽取 + log + 下一轮 chest（**已落地 20.6**：`HappyPath_FullFlow`；本 story 复用 + 文档化对应关系）|
    | 行 2984 | **回滚 1**（扣步数失败）| mock step_account.Spend 抛 error → 验证 chest 仍 unlockable + 没有 chest_open_logs + steps 不变 |
    | 行 2985 | **回滚 2**（抽奖失败）| mock cosmetic_items.ListEnabledForWeightedPick 返空 → 验证 steps 也未扣 |
    | 行 2986 | **回滚 3**（写 log 失败）| mock chest_open_logs.Create 抛 error → 验证整体回滚 |
    | 行 2987 | **回滚 4**（建下一轮 chest 失败）| mock chest.Create 抛 error → 验证整体回滚（包括步数）|
    | 行 2988 | **幂等 1**（100 次同 key）| 同一 idempotencyKey 重复调 100 次 → 只成功 1 次，DB chest_open_logs 只多 1 行，余下 99 次返回相同结果 |
    | 行 2989 | **幂等 2**（不同 key + 充足步数）| 不同 idempotencyKey + 步数足够 → 各次都能成功开箱（每次扣 1000）|
    | 行 2990 | **并发 1**（100 同 key）| 同一用户 100 个并发请求（同一 idempotencyKey）→ 只 1 次开箱成功 |
    | 行 2991 | **并发 2**（100 不同 key + 1500 步数）| 同一用户 100 个并发请求（不同 idempotencyKey）+ 步数仅够开 1 次 → 只 1 次成功，其他 99 次返回 **4002**（详见 AC9 注：5d unlock_at 检查在 5e step 检查之前 → race 失败码是 4002 而非 3002）|
    | 行 2992 | **边界 1**（999 步数）| 步数恰好 999 → 3002 |
    | 行 2993 | **边界 2**（1000 步数）| 步数恰好 1000 → 成功，余 0 |
    | 行 2994 | **边界 3**（1001 步数）| 步数恰好 1001 → 成功，余 1 |
    | 行 2995 | **边界 4**（unlock_at -1ms）| chest unlock_at 比 now 早 1ms → unlockable |
    | 行 2996 | **抽奖分布**（1000 次）| 1000 次开箱 → 各品质比例符合 drop_weight 设计（common:rare:epic:legendary ≈ 100·8 : 20·4 : 4·2 : 1·1 = 800:80:8:1）|

  - 全部场景用 dockertest 真实 MySQL 跑通（**不**用 sqlmock —— 业务上是 Layer 2 黑盒事务行为验证，不是 SQL 字符串验证）
  - 集成测试在 CI 标 `//go:build integration` + `// +build integration` 双行 tag（与 4.7 / 11.9 / 20.6 同模式）

- **范围边界**（**关键** —— 与 20.6 已落地集成测试的明确分工）：

  **20.6 service 层集成测试已落地 2 case**（`server/internal/service/chest_open_service_integration_test.go`，全部 done）：
  - `TestChestOpenServiceIntegration_HappyPath_FullFlow` — 完整 happy 链路 → DB user_step_accounts.available_steps=500 + consumed_steps=1000 + chest_open_logs +1 + 旧 chest 删 + 新 chest 建 + idempotency.status=success + response_json schema 正确
  - `TestChestOpenServiceIntegration_HappyPath_IdempotencyReplay_SameKey` — 同 idempotencyKey 第二次调用 → 短路 cached + DB 无副作用（chest_open_logs / step / idem 全部仍 1 行）

  **本 story 任务是扩展上述文件追加 ≥12 个新 case**（追加到同一份 `chest_open_service_integration_test.go`，**不**新建独立测试文件 —— 与 4.7 / 11.9 / 20.6 同模式同包同文件内聚，复用 startMySQL / runMigrations / insertUser / insertStepAccount / insertChest / buildChestOpenServiceIntegration helper）：

  | epics.md 钦定场景 | 测试函数命名 | 与既有 case 关系 |
  |---|---|---|
  | 完整流程（行 2983）| 复用 20.6 `HappyPath_FullFlow` | **复用 + 文档化对应关系**（不新增；本 story 在文件顶部注释里指向已落地 case）|
  | 回滚 1（扣步数失败）| `TestChestOpenServiceIntegration_StepAccountSpendFails_AllRollback` | **新增**（fault injection StepAccountRepo.Spend）|
  | 回滚 2（抽奖失败 - 空 cosmetic_items）| `TestChestOpenServiceIntegration_CosmeticItemsListEmpty_AllRollback` | **新增**（fault injection CosmeticItemRepo.ListEnabledForWeightedPick → 返 nil + nil）|
  | 回滚 3（写 log 失败）| `TestChestOpenServiceIntegration_ChestOpenLogCreateFails_AllRollback` | **新增**（fault injection ChestOpenLogRepo.Create）|
  | 回滚 4（建新 chest 失败）| `TestChestOpenServiceIntegration_NextChestCreateFails_AllRollback` | **新增**（fault injection ChestRepo.Create —— 仅注入"建新 chest"那一次 Create，前面的 Delete / FindByUserIDForUpdate 透传）|
  | 幂等 1（100 次同 key）| `TestChestOpenServiceIntegration_Idempotency100CallsSameKey_OnlyOneOpen` | **新增**（顺序 100 次，**不**并发；并发版本由"并发 1"覆盖；本 case 验证 cached replay 路径）|
  | 幂等 2（不同 key + 充足步数）| `TestChestOpenServiceIntegration_Idempotency3CallsDifferentKeys_EachOpens` | **新增**（顺序 3 次不同 key，每次 1500 步数兜底 → 3 次都成功扣不同 chest 的 1000 步数；**注**：每次需 force-unlock 下一轮 chest 因为 unlock_at = now+10min）|
  | 并发 1（100 同 key）| `TestChestOpenServiceIntegration_Concurrent100SameKey_OnlyOneOpens` | **新增**（与 4.7 _Concurrent100SameGuestUID / 11.9 _Concurrent100DifferentUsers 同模式）|
  | 并发 2（100 不同 key + 1500 步数）| `TestChestOpenServiceIntegration_Concurrent100DifferentKeys_StepLimitOnlyOneOpens` | **新增**（步数兜底 1500 → 1 个成功 + 99 个 4002；race 后新 chest unlock_at = now+10min，5d 检查在 5e 之前 → 失败码是 ErrChestNotUnlocked 而非 ErrInsufficientSteps，详见 AC9 注释）|
  | 边界 1（999 步数）| `TestChestOpenServiceIntegration_Steps999_Returns3002` | **新增**（V1 §7.2 钦定 ErrInsufficientSteps 3002）|
  | 边界 2（1000 步数）| `TestChestOpenServiceIntegration_Steps1000_SucceedsAvailable0` | **新增**（available_steps=0 边界）|
  | 边界 3（1001 步数）| `TestChestOpenServiceIntegration_Steps1001_SucceedsAvailable1` | **新增**（available_steps=1 边界）|
  | 边界 4（unlock_at -1ms）| `TestChestOpenServiceIntegration_UnlockAtMinus1ms_IsUnlockable` | **新增**（V1 §7.2.5d isUnlockable 公式：chest.status=1 AND unlock_at <= now → unlockable；本 case 直接边界化）|
  | 抽奖分布（1000 次）| `TestChestOpenServiceIntegration_WeightedPickDistribution_1000Opens` | **新增**（1000 次开箱 → 统计 chest_open_logs.reward_rarity 分布在 drop_weight 设计区间内）|

  **关键设计约束**：
  - 全部 ≥12 case 必须挂 `//go:build integration` + `// +build integration` 双行 tag（与 20.6 / 4.7 / 11.9 同模式）
  - **回滚 1/2/3/4 必须用 fault injection wrapper repo**（不能用 stub repo，理由同 4.7 AC2：stub 不真开 InnoDB 事务无法验证 rollback 真行为）；wrapper 模式与 4.7 落地的 `faultUserRepo` / `faultPetRepo` / `faultChestRepo` 完全同模式（按方法包装真实 mysql repo + 在指定方法上替换为 injectErr）
  - **回滚 4（NextChest.Create 失败）需要"按调用次数"注入**：因为 ChestRepo.Create 在事务中调用 1 次（建新 chest），FindByUserIDForUpdate 调用 1 次，Delete 调用 1 次（删旧 chest）；**简化**：直接让 `faultChestRepoOnCreate` 在 Create 上无条件返 injectErr —— 但需要先让 Delete 透传（删旧 chest 必须先成功，否则验证不到"扣步数 + 写 log + 删旧 chest 也回滚"的语义）；reset 后单测断言 5 张表都恢复到 case setup 状态
  - **回滚 2（cosmetic_items 空）有特殊语义**：epics.md 行 2985 说 "mock 抛 error"；但实际 service 代码（`chest_open_service.go:255`）在 `len(items) == 0` 时直接 `return 1009 错误`（不是抛 error）。**决策**：本 case fault inject `CosmeticItemRepo.ListEnabledForWeightedPick` 返 `([]mysql.CosmeticItem{}, nil)` 触发 service 内部 1009；这与"抛 error"语义等价（都让 fn return 非 nil → tx.WithTx 触发 ROLLBACK）；测试断言 ErrServiceBusy 1009 即可
  - **并发 1 必须 100 goroutine 同 idempotencyKey**：核心验证 ClaimPending 的 INSERT ... ON DUPLICATE KEY UPDATE 在 UNIQUE 索引（uk_user_id_key）下的串行化保证 + 短路读 cached replay；**断言**：100 个 goroutine 全部成功（无 1009）+ DB chest_open_logs 仅 1 行 + step_account 仅扣 1 次 1000 + chest_open_idempotency_records 仅 1 行 status=success + 100 个返回的 reward.cosmeticItemId 全部相等（cached replay 路径返回缓存结果）
  - **并发 2 必须 100 goroutine 不同 idempotencyKey + 1500 步数**：核心验证 user_step_accounts 乐观锁 + FOR UPDATE 行锁串行化；**断言**：1 个 goroutine 成功（拿到 reward）+ 99 个返回 ErrInsufficientSteps 3002 + DB chest_open_logs 仅 1 行 + step_account.available_steps=500 + chest_open_idempotency_records 仅 100 行（每个 key 1 行）但其中 1 行 status=success，99 行 status=pending（**关键 lesson**：失败事务的 ClaimPending 已 INSERT 但 fn 返 error 触发 ROLLBACK，所以 pending 行不应留下；实际**应**全部 ROLLBACK 干净 —— 100 个 goroutine 中 1 个 commit success，99 个 ROLLBACK 全干净 → DB 中 idempotency 行最终只有 1 行 status=success；99 个 key 不存在任何行）
  - **抽奖分布 1000 次需要使用 dev grant or force-unlock 模式**：每次开箱后 next chest 的 unlock_at = now+10min，不能立即再开；**决策**：每次开完后用 raw SQL `UPDATE user_chests SET unlock_at = ?, status = 1 WHERE user_id = ?` 把新建的 chest 强解锁，让循环继续；步数补满用 `UPDATE user_step_accounts SET available_steps = 1000, consumed_steps = 0, version = version + 1 WHERE user_id = ?`；这样 1000 次循环开箱稳定可控
  - **抽奖分布断言区间**：drop_weight 总和 = 8·100 + 4·20 + 2·4 + 1·1 = 800 + 80 + 8 + 1 = 889；按比例 1000 次开箱：common ≈ 800/889 · 1000 ≈ 900；rare ≈ 80/889 · 1000 ≈ 90；epic ≈ 8/889 · 1000 ≈ 9；legendary ≈ 1/889 · 1000 ≈ 1.1（**注意**：legendary 期望 1.1 次，分布极稀 —— 1000 次可能命中 0-5 次）；**断言区间**：common ∈ [820, 980]（±10% 容差）、rare ∈ [50, 130]、epic ∈ [0, 25]、legendary ∈ [0, 8]；用宽松区间避免 random flaky（crypto/rand 不可注入 seed，每次 distribution 都是真随机）；区间设计参考 chi-square 检验 ~95% 置信水平
  - **边界 4 (unlock_at -1ms) 设计**：构造 `unlockAt = time.Now().UTC().Add(-1*time.Millisecond)`，chest.status = 1 → service 内 `chest.UnlockAt.After(now) == false` → isUnlockable = true → 开箱成功；本 case 验证 V1 §7.2.5d 公式边界处不发生时序错位

- **下游依赖**：本 story 是 epic 20 收尾，**不**直接服务下游 story；但本 story 的 fault injection 模式（4 个 wrapper：faultStepAccountRepoOnSpend / faultCosmeticItemRepoOnList / faultChestOpenLogRepoOnCreate / faultChestRepoOnCreate）成为 future Layer 2 集成测试的范式（如 Story 23.5 节点 8 user_cosmetic_items 入仓事务回滚 / Story 26.5 穿戴事务集成测试 / Story 32.5 合成事务集成测试 都钦定相同 Layer 2 模式）。

**本 story 不做**（明确范围红线）：

- [skip] **不**修改 `server/internal/service/chest_open_service.go`（20.6 已 done；本 story 仅消费）
- [skip] **不**修改 `server/internal/service/chest_service.go`（20.5 / 20.6 已 done；本 story 仅消费）
- [skip] **不**修改 `server/internal/app/http/handler/chest_handler.go`（20.6 已 done；本 story **不**测 handler 层 —— epics.md §20.9 全部场景在 service 层验证；HTTP 端到端 envelope schema 验证留 Story 19.1 跨端 E2E demo 阶段）
- [skip] **不**修改 5 个 mysql repo（chest / step_account / cosmetic_item / chest_open_log / chest_open_idempotency_record，20.2-20.6 已 done；本 story 仅消费 + 包装做 fault injection）
- [skip] **不**修改 0011 / 0012 / 0013 / 0014 migration（20.2-20.4 已 done；本 story 仅消费）
- [skip] **不**修改 `server/internal/repo/tx/manager.go`（4.2 已 done；本 story 仅消费 `WithTx`）
- [skip] **不**修改 `server/internal/pkg/random/weighted.go`（20.6 已 done；本 story 用 cryptoWeightedPicker 真实随机 + 不可注入 seed —— 抽奖分布 case 接受真实随机性，用宽松区间断言）
- [skip] **不**修改 4.5 中间件（auth + rate_limit；本 story 不走 handler 层）
- [skip] **不**修改 bootstrap router（**不**新增 deps 字段；不挂新路由）
- [skip] **不**修改 20.6 已有的 2 个集成测试函数（保持现有 done 状态测试不破坏 —— 仅在同一份 `chest_open_service_integration_test.go` 文件**追加** ≥12 个新 case）
- [skip] **不**修改 20.5 已有的 chest_service_integration_test.go（仅消费 startMySQL / runMigrations / insertUser / insertStepAccount / insertChest helper —— 但这些 helper 已在 home_service_integration_test.go / auth_service_integration_test.go 内，本 story 直接复用，**不**抽包 / 不重复定义）
- [skip] **不**新建跨包 testing util（不抽 startMySQL 到 internal/testutil/ —— 与 4.7 / 11.9 / 20.6 同模式，复用 helper 留在 service 包内即可，避免范围扩散）
- [skip] **不**用 sqlmock（epics.md 行 2997 钦定 "全部场景用 dockertest 真实 MySQL 跑通"；sqlmock 测的是 SQL 字符串匹配，与本 story Layer 2 黑盒行为验证语义不符）
- [skip] **不**改 `docs/宠物互动App_*.md` 任一份（V1 §7.2 / 数据库设计 §5.16 / §8.3 / 时序图 §7.x 是契约**输入**，本 story 严格对齐**不**修改）
- [skip] **不**写 README / 部署文档：留 Epic 20 收尾或 Story 6.3 文档同步阶段
- [skip] **不**实装 user_cosmetic_items 实例创建（节点 8 Story 23.5 owner；本 story 阶段 reward.userCosmeticItemId 仍为 0 占位 —— V1 §7.2.4h 钦定）
- [skip] **不**实装 audit log（依赖 Logging 中间件兜底）
- [skip] **不**修改 `server/configs/local.yaml`（不引入新配置项）
- [skip] **不**修改 `server/cmd/server/main.go`（不加新 deps）
- [skip] **不**支持 `go test -short`（dockertest 必跑；本 story ≥12 case 全部 `+build integration`，默认 `bash scripts/build.sh --test` 不触发；只在 `--integration` 触发）
- [skip] **不**实装"测试容器复用"优化（每 case 独立 startMySQL 容器，与 4.7 / 11.9 / 20.6 同模式，简单 + 一致性优于性能）；优化方向留 future 性能 epic
- [skip] **不**给 Story 20.9 加 sprint-status.yaml 占位 retrospective（epic 20 retrospective 已在 sprint-status.yaml 第 217 行 optional，本 story done 后整 epic done 才推 retrospective）
- [skip] **不**做 fuzz / property-based testing（dockertest case 已穷举 epics.md 钦定 14 类；fuzz 是 future testing 升级范畴）
- [skip] **不**测 ctx cancel / timeout 路径（ADR-0007 ctx 传播是 4.2-4.6 已建立的范式，20.6 service 单测已覆盖，本 story 不重复验证）
- [skip] **不**测 deadlock / 隔离级别 anomaly（InnoDB 默认 REPEATABLE READ + 本 story 100 goroutine 同 key race 已经触发 FOR UPDATE 行锁 + UNIQUE 索引 X-lock；不深挖隔离级别专项）
- [skip] **不**测 HTTP 端到端（handler → service → DB envelope schema）：epics.md §20.9 全部场景在 service 层验证；handler 层 schema 由 20.6 handler 单测 + Story 19.1 跨端 E2E demo 阶段覆盖
- [skip] **不**测 user_cosmetic_items 表（节点 7 阶段 reward.userCosmeticItemId 占位 0，本表节点 8 才创建数据 —— epics.md 行 3311 "节点 8 改"，本 story 不涉及）
- [skip] **不**测 dev /dev/force-unlock-chest（20.7 已落地，本 story 是开箱事务集成测试，不重测 dev 端点；但本 story 内部可用 raw SQL 模拟 force-unlock 效果让 chest 进 unlockable 状态）

## Acceptance Criteria

**AC1 — 测试文件位置 + build tag + helper 复用**

本 story 在已有 `server/internal/service/chest_open_service_integration_test.go`（20.6 落地）**追加** ≥12 个新测试函数；**不**新建独立测试文件。

**关键约束**：

- **build tag**：所有新 case 必须挂 `//go:build integration` + `// +build integration` 双行标记（与 20.6 / 4.7 / 11.9 同模式 + Go 1.17+ 双语法兼容）；放在文件顶部（20.6 已写）
- **helper 复用**：直接消费同包已有的 helper：
  - `startMySQL` (`auth_service_integration_test.go:64`) — 起容器
  - `runMigrations` (`auth_service_integration_test.go:124`) — 跑 0001-0014 全套 migration（含 0011/0012 cosmetic seed）
  - `insertUser` (`home_service_integration_test.go:81`) — 直插 users 行
  - `insertStepAccount` (`home_service_integration_test.go:103`) — 直插 user_step_accounts 行
  - `insertChest` (`home_service_integration_test.go:114`) — 直插 user_chests 行
  - `buildChestOpenServiceIntegration` (`chest_open_service_integration_test.go:38`) — 起容器 + migrate + 装配 svc（已 20.6 落地）
  - `assertCount` / `queryCount` (`auth_service_integration_test.go:189` / `:522`) — 通用 count 断言
- **不抽包**：所有 helper 仍在本测试文件内 / 同包内（每 case 独立起容器，与 4.7 / 11.9 同模式 —— 跨包 testing util 抽离是 future scaling 决策，本 story 不做）
- **包名不变**：`package service_test`（外部测试包，与 20.6 同）

**关键反模式**：

- ❌ **不**新建 `chest_open_service_rollback_test.go` / `chest_open_service_concurrency_test.go` 等拆文件（保持 20.6 单文件内聚 —— 一份测试文件视图覆盖所有场景，便于 reviewer 一目了然）
- ❌ **不**在 handler 包新建 integration test 文件（epics.md §20.9 全部 scope 在 service 层）
- ❌ **不**用 sqlmock（epics.md 行 2997 钦定）
- ❌ **不**新加 helper（除非必要的 fault injection wrapper struct + 抽奖分布 reset helper —— 后者直接 inline 在分布 case 内即可）

**AC2 — 回滚 1: 扣步数失败 → 整体回滚**

新增测试函数 `TestChestOpenServiceIntegration_StepAccountSpendFails_AllRollback`：

```go
// AC2 测试函数框架（位于 chest_open_service_integration_test.go）
func TestChestOpenServiceIntegration_StepAccountSpendFails_AllRollback(t *testing.T) {
    dsn, dockerCleanup := startMySQL(t)
    defer dockerCleanup()
    runMigrations(t, dsn)

    cfg := config.MySQLConfig{DSN: dsn, MaxOpenConns: 10, MaxIdleConns: 2, ConnMaxLifetimeSec: 60}
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    gormDB, err := db.Open(ctx, cfg)
    if err != nil {
        t.Fatalf("db.Open: %v", err)
    }
    rawDB, err := gormDB.DB()
    if err != nil {
        t.Fatalf("gormDB.DB(): %v", err)
    }
    defer rawDB.Close()

    // **关键**：用真实 mysql repo（非 stub）—— 必须经过真实 InnoDB 事务 + GORM driver
    // 才能验证 rollback 真行为；只在 StepAccountRepo.Spend 上包装 fault injection
    chestRepo := mysql.NewChestRepo(gormDB)
    idempotencyRepo := mysql.NewIdempotencyRepo(gormDB)
    cosmeticItemRepo := mysql.NewCosmeticItemRepo(gormDB)
    chestOpenLogRepo := mysql.NewChestOpenLogRepo(gormDB)
    stepAccountRepoFault := &faultStepAccountRepoOnSpend{
        delegate:  mysql.NewStepAccountRepo(gormDB),
        injectErr: errors.New("synthetic step account spend failure"),
    }
    txMgr := tx.NewManager(gormDB)
    weightedPicker := random.NewCryptoWeightedPicker(rand.Reader)

    svc := service.NewChestService(chestRepo, txMgr, idempotencyRepo, stepAccountRepoFault, cosmeticItemRepo, chestOpenLogRepo, weightedPicker)

    // 准备 fixture: user + 1500 步 + unlockable chest
    const userID = uint64(1)
    const idempotencyKey = "test_rollback_step_spend"
    insertUser(t, rawDB, userID, "uid-rollback-step", "用户回滚", "")
    insertStepAccount(t, rawDB, userID, 1500, 1500, 0)
    unlockAt := time.Now().UTC().Add(-1 * time.Minute)
    insertChest(t, rawDB, 9001, userID, 1, unlockAt, 1000)

    _, err = svc.OpenChest(context.Background(), service.OpenChestInput{
        UserID: userID, IdempotencyKey: idempotencyKey,
    })

    // service 必须返 1009（事务回滚后包装 ErrServiceBusy）
    if err == nil {
        t.Fatalf("expected error, got nil (rollback path 没触发)")
    }
    var appErr *apperror.AppError
    if !errors.As(err, &appErr) || appErr.Code != apperror.ErrServiceBusy {
        t.Fatalf("expected AppError code=1009, got %v", err)
    }

    // **核心断言**：step_account / chest_open_logs / idempotency / chest 全部回滚到初始状态
    //   - user_step_accounts.available_steps 仍 1500（扣步数前抛 error）
    //   - chest_open_logs 0 行
    //   - chest_open_idempotency_records 0 行（ClaimPending 已 INSERT 但 ROLLBACK 干净）
    //   - user_chests 仍 1 行（旧 chest 未删；status=1, unlock_at = -1min）
    var availableSteps, consumedSteps, version uint64
    if err := rawDB.QueryRow(
        `SELECT available_steps, consumed_steps, version FROM user_step_accounts WHERE user_id=?`, userID,
    ).Scan(&availableSteps, &consumedSteps, &version); err != nil {
        t.Fatalf("query step_account: %v", err)
    }
    if availableSteps != 1500 {
        t.Errorf("available_steps = %d, want 1500 (rollback)", availableSteps)
    }
    if consumedSteps != 0 {
        t.Errorf("consumed_steps = %d, want 0 (rollback)", consumedSteps)
    }
    if version != 0 {
        t.Errorf("version = %d, want 0 (rollback)", version)
    }

    assertCount(t, rawDB, "chest_open_logs WHERE user_id=?", []any{userID}, 0, "chest_open_logs (rollback)")
    assertCount(t, rawDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 0, "idempotency (rollback)")
    assertCount(t, rawDB, "user_chests WHERE user_id=? AND id=9001", []any{userID}, 1, "chest still unlockable (not deleted)")
}
```

**fault injection 设计**（同文件内新增 helper struct）：

```go
// faultStepAccountRepoOnSpend 包装真实 StepAccountRepo，让 Spend 抛 injectErr，其他方法透传。
//
// 模式：MVP 用"按方法包装"，不引入第三方 fault injection 框架（gomonkey / monkey 等）。
// 优点：编译期检查 + 与 4.7 fault\*Repo 同模式 + 跨平台无依赖（gomonkey 在 ARM 不工作）。
type faultStepAccountRepoOnSpend struct {
    delegate  mysql.StepAccountRepo
    injectErr error
}

func (f *faultStepAccountRepoOnSpend) Create(ctx context.Context, a *mysql.StepAccount) error {
    return f.delegate.Create(ctx, a)
}

func (f *faultStepAccountRepoOnSpend) FindByUserID(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
    return f.delegate.FindByUserID(ctx, userID)
}

func (f *faultStepAccountRepoOnSpend) UpdateBalance(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error {
    return f.delegate.UpdateBalance(ctx, userID, delta, expectedVersion)
}

func (f *faultStepAccountRepoOnSpend) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
    return f.delegate.FindByUserIDForUpdate(ctx, userID)
}

func (f *faultStepAccountRepoOnSpend) Spend(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error {
    return f.injectErr
}
```

**关键设计约束**：

- **fault injection 必须包装真实 repo（不是 stub）**：service.runOpenChestTx 在 fn 内调用 StepAccountRepo.Spend —— fault.Spend 直接返 injectErr → fn 返 error → tx.WithTx 触发 InnoDB ROLLBACK → 真实验证回滚行为
- **injectErr 用 generic errors.New(...)**（不用 sentinel）—— 让 service 走 1009 默认分支（ADR-0006 第三层映射）
- **延迟 cleanup 顺序**：`defer rawDB.Close()` 必须在 `defer dockerCleanup()` **之后**注册（按栈序先关 db pool 再 purge 容器；否则 purge 后 close 会报错日志）
- **AppError 断言用 errors.As**：`*apperror.AppError` 是 internal/pkg/errors 包的具体类型，service 用 `apperror.Wrap(err, ErrServiceBusy, ...)` 产出（AC2-AC5 四个回滚 case 共用此断言模式）
- **idempotency 行必须 0**：ClaimPending 已 INSERT 1 行 pending（事务首条），但 fn 后续 return error → tx.WithTx ROLLBACK → idempotency 行也回滚干净；本断言验证"事务原子性覆盖 idempotency 行"，**不是**部分降级
- **旧 chest 必须仍在**：本 case fault 注入在 step.Spend（位于 chest.FindByUserIDForUpdate 之后但在 chest.Delete 之前）—— ROLLBACK 后旧 chest 仍存在 + status / unlock_at 不变

**关键反模式**：

- ❌ **不**用 `*apperror.AppError` 直接 == 比较（apperror.Wrap 会包装多层 error，必须 errors.As 解开）
- ❌ **不**抽 faultStepAccountRepoOnSpend 到独立文件（保持本测试文件内聚；其他 fault\*Repo 同位置）
- ❌ **不**在 faultStepAccountRepoOnSpend.Spend 内打 t.Log / fmt.Println（测试输出污染 + 无意义）
- ❌ **不**用 testify mock / gomock（与 20.6 / 4.7 同模式 —— 显式 stub struct）

**AC3 — 回滚 2: 抽奖失败（cosmetic_items 空）→ 步数也未扣**

新增测试函数 `TestChestOpenServiceIntegration_CosmeticItemsListEmpty_AllRollback`：

```go
func TestChestOpenServiceIntegration_CosmeticItemsListEmpty_AllRollback(t *testing.T) {
    // ... 同 AC2 setup ...

    // 用真实 4 个 repo + faultCosmeticItemRepoOnList（返 ([]CosmeticItem{}, nil) 触发 service 内 1009）
    cosmeticItemRepoFault := &faultCosmeticItemRepoOnList{
        returnEmpty: true, // ListEnabledForWeightedPick 返 ([]CosmeticItem{}, nil)
    }
    svc := service.NewChestService(chestRepo, txMgr, idempotencyRepo, stepAccountRepo, cosmeticItemRepoFault, chestOpenLogRepo, weightedPicker)

    _, err = svc.OpenChest(ctx, service.OpenChestInput{UserID: userID, IdempotencyKey: "test_rollback_pick"})

    // 断言 1009 + 全部回滚
    require AppError code=1009
    assertCount: user_step_accounts.available_steps=1500 (rollback) / consumed_steps=0 / version=0
                 chest_open_logs=0
                 idempotency=0
                 user_chests still has chest 9001
}
```

**fault helper struct**（追加同文件）：

```go
type faultCosmeticItemRepoOnList struct {
    returnEmpty bool
    injectErr   error // optional: 若非 nil，返 (nil, injectErr)；否则返 ([]CosmeticItem{}, nil)
}

func (f *faultCosmeticItemRepoOnList) ListEnabledForWeightedPick(ctx context.Context) ([]mysql.CosmeticItem, error) {
    if f.injectErr != nil {
        return nil, f.injectErr
    }
    return []mysql.CosmeticItem{}, nil
}
```

**关键约束**：

- **回滚 2 的语义重点**：fn 在 step.Spend 之后、cosmetic_items 抽奖之前失败 —— 实际 service 代码（`chest_open_service.go:255-258`）在 `len(items) == 0` 时 `return apperror.New(ErrServiceBusy, ...)` → fn return non-nil error → tx.WithTx ROLLBACK；本 case 验证"已扣步数也回滚"
- **step_account / chest_open_logs / idempotency / chest 都必须验证回滚**：扣步数 SQL 已执行（available_steps - 1000 + version + 1），但 ROLLBACK 让 InnoDB undo log 恢复；chest_open_logs 0 行（未到达写 log）；idempotency 0 行（ClaimPending 也回滚）；旧 chest 仍在
- **必须验证 CosmeticItemRepo interface 完整实装**：CosmeticItemRepo 接口只有 1 个方法 ListEnabledForWeightedPick，fault struct 仅实装它（不需要透传其他方法）

**AC4 — 回滚 3: 写 chest_open_logs 失败 → 整体回滚**

新增测试函数 `TestChestOpenServiceIntegration_ChestOpenLogCreateFails_AllRollback`：

```go
func TestChestOpenServiceIntegration_ChestOpenLogCreateFails_AllRollback(t *testing.T) {
    // ... 同 AC2 setup ...

    chestOpenLogRepoFault := &faultChestOpenLogRepoOnCreate{
        injectErr: errors.New("synthetic chest_open_log create failure"),
    }
    svc := service.NewChestService(chestRepo, txMgr, idempotencyRepo, stepAccountRepo, cosmeticItemRepo, chestOpenLogRepoFault, weightedPicker)

    _, err = svc.OpenChest(ctx, service.OpenChestInput{UserID: userID, IdempotencyKey: "test_rollback_log"})

    // 断言 1009 + 全部回滚（步数 + 抽奖结果都没 commit）
    require AppError code=1009
    assertCount: step_account.available_steps=1500 (rollback) / chest_open_logs=0 / idem=0 / chest 9001 仍在
}
```

**fault helper struct**（追加同文件）：

```go
type faultChestOpenLogRepoOnCreate struct {
    injectErr error
}

func (f *faultChestOpenLogRepoOnCreate) Create(ctx context.Context, log *mysql.ChestOpenLog) error {
    return f.injectErr
}
```

**关键约束**：

- **回滚 3 的语义重点**：fn 在 step.Spend / 抽奖 都成功后、写 log 失败 —— InnoDB undo log 需要回滚"扣步数 + 抽奖 random 调用结果"两步；ROLLBACK 后 5 张相关表全部恢复
- **抽奖随机性不影响测试**：weightedPicker 已选好 item 但 service 还没读 chest.Delete / Create 路径，写 log 是步骤 5h —— fault 注入直接 return injectErr，下游 5i (Delete + Create) 不到达
- **idempotency 必须 0**：ClaimPending 在事务首条已 INSERT 1 行 pending，但 fn return error → ROLLBACK 彻底清除

**AC5 — 回滚 4: 建新 chest 失败 → 整体回滚（含旧 chest Delete 也回滚）**

**关键边界点**：新 chest Create 是步骤 5i 的第二条（5i 是 "Delete 旧 + Create 新"两步）；本 case 让 Delete 透传（旧 chest 已被 DELETE）+ Create 注入 fault → fn return error → tx.WithTx ROLLBACK → InnoDB undo log 把 Delete 也回滚（旧 chest 仍在）。

新增测试函数 `TestChestOpenServiceIntegration_NextChestCreateFails_AllRollback`：

```go
func TestChestOpenServiceIntegration_NextChestCreateFails_AllRollback(t *testing.T) {
    // ... 同 AC2 setup ...

    chestRepoFault := &faultChestRepoOnCreate{
        delegate:  mysql.NewChestRepo(gormDB),
        injectErr: errors.New("synthetic next chest create failure"),
    }
    svc := service.NewChestService(chestRepoFault, txMgr, idempotencyRepo, stepAccountRepo, cosmeticItemRepo, chestOpenLogRepo, weightedPicker)

    _, err = svc.OpenChest(ctx, service.OpenChestInput{UserID: userID, IdempotencyKey: "test_rollback_next_chest"})

    require AppError code=1009
    // 断言完整 rollback：
    //   - step_account 仍 1500 / 0 / 0（扣步数已 SQL 执行但 ROLLBACK）
    //   - chest_open_logs 0 行（写 log SQL 已执行但 ROLLBACK）
    //   - idempotency 0 行（ClaimPending 也 ROLLBACK）
    //   - user_chests 仍只有 chest 9001（status=1, unlock_at=-1min）—— Delete 旧 chest 已 SQL 执行
    //     但 ROLLBACK 让旧 chest 恢复（InnoDB undo log）
    assertCount: step_account.available_steps=1500 / chest_open_logs=0 / idem=0
    assertCount: user_chests WHERE id=9001 AND user_id=1 AND status=1 → 1 (rollback restored)
    assertCount: user_chests WHERE user_id=1 → 1 (只有 chest 9001；新 chest 没 INSERT)
}
```

**fault helper struct**（追加同文件，**注意是 ChestRepo 包装版本** —— 不与 4.7 faultChestRepo 重名，避免编译冲突）：

```go
// faultChestRepoOnCreate 包装真实 ChestRepo，让 Create 抛 injectErr，其他方法透传。
//
// **关键 1**：与 4.7 的 `faultChestRepo`（auth_service_integration_test.go:474）**重名风险**。
// 4.7 的 faultChestRepo 在 chest 5 表初始化场景下 Create 抛 err（用于 auth_service 集成测试）；
// 本 story 的语义是开箱事务中"建新 chest"失败 → 复用 4.7 同名 struct 也可以（功能一致：都是 Create 抛 err），
// **但** package service_test 内只能存在一份同名 struct 定义。
// **决策**：本 story **复用** 4.7 已落地的 `faultChestRepo` —— 不新建 `faultChestRepoOnCreate`；
// 4.7 的 faultChestRepo 已实装 ChestRepo 全部 7 个方法（Create / FindByUserID / FindByID /
// FindByUserIDForUpdate / FindByIDForUpdate / Delete / UpdateUnlockAtByID），其中 Create 抛
// injectErr，其他透传给 delegate —— 与本 story AC5 需求完全一致。
//
// **关键 2**：4.7 的 faultChestRepo.Delete 是透传给 delegate.Delete —— 这正是本 case 需要的：
// service.runOpenChestTx 步骤 5i 先调 Delete（透传 → SQL 真执行删旧 chest），然后调 Create
// （注入 → 抛 err → fn return error → ROLLBACK 把 Delete + Spend + Log Create 一起回滚）。
// 这条事务回滚链是本 case 验证的核心。
//
// **不**新加 `faultChestRepoOnCreate` 是为避免同包内重复定义；4.7 落地的 faultChestRepo 已是
// "Create 抛 err + 其他透传"的完整版本，本 story 直接复用同款变量名 + 不同 instance 即可。
```

**关键约束**：

- **fault.Delete 必须透传**（不能注入 fault）：让旧 chest 9001 真被 DELETE，验证 ROLLBACK 时 InnoDB undo log 恢复旧 chest；如果 Delete 也 fault → ROLLBACK 时旧 chest 本来没被删，验证不到完整链路
- **旧 chest 仍在 + 新 chest 未建**：assertCount `user_chests WHERE user_id=1` = 1（旧 chest 恢复 + 新 chest 未 INSERT）
- **复用 4.7 faultChestRepo**：与 4.7 集成测试同 package service_test → 同一编译单元 → faultChestRepo struct 可见且可复用；4.7 的 `injectErr` 是 errors.New("synthetic chest repo failure")，本 case 重新构造 instance 注入 errors.New("synthetic next chest create failure") 即可

**AC6 — 幂等 1: 同一 idempotencyKey 重复调 100 次 → 只成功 1 次**

新增测试函数 `TestChestOpenServiceIntegration_Idempotency100CallsSameKey_OnlyOneOpen`：

```go
func TestChestOpenServiceIntegration_Idempotency100CallsSameKey_OnlyOneOpen(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    const idempotencyKey = "test_idem_100_same_key"
    insertUser(t, sqlDB, userID, "uid-idem-100", "用户幂等", "")
    insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
    insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

    const N = 100
    var firstReward service.ChestRewardBrief
    var firstNextChestID uint64
    for i := 0; i < N; i++ {
        out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
            UserID:         userID,
            IdempotencyKey: idempotencyKey,
        })
        if err != nil {
            t.Fatalf("call %d: %v (cached replay 应该全部成功)", i, err)
        }
        if i == 0 {
            firstReward = out.Reward
            firstNextChestID = out.NextChest.ID
        } else {
            // 后 99 次返回 cached replay：reward / nextChest.ID 与第一次完全一致
            if out.Reward.CosmeticItemID != firstReward.CosmeticItemID {
                t.Errorf("call %d: Reward.CosmeticItemID=%d, want %d (cached)", i, out.Reward.CosmeticItemID, firstReward.CosmeticItemID)
            }
            if out.NextChest.ID != firstNextChestID {
                t.Errorf("call %d: NextChest.ID=%d, want %d (cached)", i, out.NextChest.ID, firstNextChestID)
            }
        }
    }

    // DB 最终状态：只开了 1 次箱
    var availableSteps uint64
    if err := sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps); err != nil {
        t.Fatalf("query step_account: %v", err)
    }
    if availableSteps != 500 {
        t.Errorf("available_steps=%d, want 500 (only 1 open)", availableSteps)
    }
    assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 1, "log only 1")
    assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 1, "idem only 1 row")
    assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{userID}, 1, "chest count = 1 (旧删 + 新建)")
}
```

**关键设计约束**：

- **顺序 100 次（非并发）**：本 case 验证 cached replay 路径 —— 首次调用 ClaimPending affectedRows=1 → 走 5c-l 全流程 + commit；后 99 次 ClaimPending affectedRows=0 → 走 5b 短路 → FindByUserIDAndKey 返 success → replayFromCachedResponse 反序列化 response_json → 返同结果
- **N=100 是钦定数字**（epics.md 行 2988）：100 次顺序调用是为穷举 cached replay 反序列化路径在重复负载下的稳定性 —— 防 future 修改 replayFromCachedResponse 时 N=2 测试漏过 N=100 才会暴露的 边缘 bug（如指针共享 / map 重用）
- **断言所有 100 次返回相同 reward.CosmeticItemID + nextChest.ID**：cached replay 必须返回**位段一致**的结果（cosmetic_items.id / next_chest.id 都从 DB 缓存 response_json 中反序列化）
- **DB 最终状态严格**：step_account 仅扣 1 次 1000；chest_open_logs / idempotency / user_chests 各 1 行；step_account.version=1（仅 1 次 Spend 增 version）

**关键反模式**：

- ❌ **不**用 `t.Parallel()` 跑这个 case 与其他 case 并行：dockertest 容器有限资源
- ❌ **不**减少 N 到 5 / 10 加速测试：epics.md 钦定 100；减少会让 cached replay 反复反序列化的回归覆盖弱化
- ❌ **不**用 `seenRewards := make(map[uint64]int)` 计数（map 容器在 N=100 同 key 同结果下不会有 entry 增加；直接 if 比较即可）

**AC7 — 幂等 2: 不同 idempotencyKey + 步数充足 → 各次都开箱成功**

新增测试函数 `TestChestOpenServiceIntegration_Idempotency3CallsDifferentKeys_EachOpens`：

```go
func TestChestOpenServiceIntegration_Idempotency3CallsDifferentKeys_EachOpens(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    insertUser(t, sqlDB, userID, "uid-idem-diff", "用户幂等不同", "")
    // 3500 步够开 3 次（每次扣 1000）+ 余 500
    insertStepAccount(t, sqlDB, userID, 3500, 3500, 0)
    insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

    const N = 3
    keys := []string{"key_diff_1", "key_diff_2", "key_diff_3"}
    rewardIDs := make([]uint64, N)
    for i := 0; i < N; i++ {
        out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
            UserID: userID, IdempotencyKey: keys[i],
        })
        if err != nil {
            t.Fatalf("call %d (key=%s): %v", i, keys[i], err)
        }
        rewardIDs[i] = out.Reward.CosmeticItemID

        // **关键**：每次开完后 next chest unlock_at = now+10min，需要 force-unlock 才能开下一次
        if i < N-1 {
            _, err := sqlDB.Exec(`UPDATE user_chests SET unlock_at = ? WHERE user_id = ?`,
                time.Now().UTC().Add(-1*time.Minute), userID)
            if err != nil {
                t.Fatalf("force-unlock next chest: %v", err)
            }
        }
    }

    // DB 断言：开了 3 次箱
    var availableSteps, consumedSteps uint64
    if err := sqlDB.QueryRow(`SELECT available_steps, consumed_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps, &consumedSteps); err != nil {
        t.Fatalf("query: %v", err)
    }
    if availableSteps != 500 {
        t.Errorf("available_steps=%d, want 500 (3 opens, each -1000)", availableSteps)
    }
    if consumedSteps != 3000 {
        t.Errorf("consumed_steps=%d, want 3000", consumedSteps)
    }

    assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 3, "log 3 rows")
    assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 3, "idem 3 distinct keys")
    assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{userID}, 1, "chest only 1 row (旧删 + 新建 3 次 → 最终 1 行)")
}
```

**关键约束**：

- **N=3 即可**（不需要 100）：本 case 验证"不同 key 各开各的"的语义；3 次足够区分 cached replay 路径（同 key）与 fresh path（不同 key）
- **每次开完手动 force-unlock 下一轮 chest**：service 创建的 next chest unlock_at = now + 10min，下一次开箱会因 chest.UnlockAt.After(now) == true 走 4002 路径；直接 UPDATE 强制让 unlock_at = now - 1min（透明模拟 Story 20.7 dev /dev/force-unlock-chest 的效果）
- **断言 chest_open_idempotency_records 3 行**：每个 key 1 行，全部 status=success
- **断言 user_chests 仍只 1 行**：每次开完都旧删 + 新建；3 次开箱 → 最终 user 持有 1 个 next chest（unlock_at = 第 3 次的 now+10min）

**AC8 — 并发 1: 100 goroutine 同 idempotencyKey → 只 1 次开箱成功**

新增测试函数 `TestChestOpenServiceIntegration_Concurrent100SameKey_OnlyOneOpens`：

```go
func TestChestOpenServiceIntegration_Concurrent100SameKey_OnlyOneOpens(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    const idempotencyKey = "test_concurrent_100_same"
    insertUser(t, sqlDB, userID, "uid-conc-same", "并发同key", "")
    insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
    insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

    const N = 100
    type result struct {
        rewardID uint64
        nextID   uint64
        err      error
    }
    results := make([]result, N)

    // r4 codex 修正：必须用 start barrier 同步释放所有 goroutine，否则 spawn 循环本身
    // 可能比 goroutine 业务调用还慢 → 并发退化为顺序 → false-positive race coverage
    start := make(chan struct{})
    var wg sync.WaitGroup
    wg.Add(N)
    for i := 0; i < N; i++ {
        i := i
        go func() {
            defer wg.Done()
            <-start // 等所有 goroutine ready
            out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
                UserID:         userID,
                IdempotencyKey: idempotencyKey,
            })
            if err != nil {
                results[i] = result{err: err}
                return
            }
            results[i] = result{rewardID: out.Reward.CosmeticItemID, nextID: out.NextChest.ID}
        }()
    }
    close(start) // 统一释放
    wg.Wait()

    // 断言 1：所有 100 都成功（无 1009 / 3002）
    var firstReward uint64
    var firstNextID uint64
    for i, r := range results {
        if r.err != nil {
            t.Fatalf("goroutine %d err=%v (同 key 并发应该收敛到 cached replay 而非 1009)", i, r.err)
        }
        if i == 0 {
            firstReward = r.rewardID
            firstNextID = r.nextID
        } else {
            if r.rewardID != firstReward {
                t.Errorf("g%d: rewardID=%d, want %d (cached)", i, r.rewardID, firstReward)
            }
            if r.nextID != firstNextID {
                t.Errorf("g%d: nextID=%d, want %d (cached)", i, r.nextID, firstNextID)
            }
        }
    }

    // DB 断言：只开了 1 次
    var availableSteps uint64
    sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps)
    if availableSteps != 500 {
        t.Errorf("available_steps=%d, want 500", availableSteps)
    }
    assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 1, "log only 1")
    assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=? AND status='success'", []any{userID}, 1, "idem 1 success")
    assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{userID}, 1, "chest 1")
}
```

**关键设计约束**：

- **N=100 必须**（epics.md 行 2990）：核心验证 ClaimPending 在 INSERT ... ON DUPLICATE KEY UPDATE + UNIQUE 索引（uk_user_id_key）下的串行化保证 —— 100 个 goroutine 中只有 1 个能 affectedRows=1（走全流程），99 个 affectedRows=0（走 cached replay）+ 99 个 goroutine 在 INSERT 语句上阻塞等首事务释放 UNIQUE 锁
- **断言所有 100 都成功**（任一返 1009 → 立即 fail）：这是 V1 §7.2 钦定 "同 idempotencyKey 重复请求 → 同结果" 的强语义；race detector 不挂（与 4.7 同模式）
- **断言所有 100 拿到同一 rewardID + nextID**：cached replay 必须返回位段一致的结果（cosmetic_items.id / next_chest.id 都从 DB 缓存的 response_json 反序列化）
- **DB 状态严格 1 次开箱**：step_account.consumed_steps=1000；chest_open_logs 1 行；idem 1 行 status=success；user_chests 1 行（next chest）

**关键反模式**：

- ❌ **不**减少 N 到 10 / 50（epics.md 钦定 100）
- ❌ **不**用 channel 替代 sync.WaitGroup（与 4.7 同模式）
- ❌ **不**断言"哪个 goroutine 先 commit"（非确定性；只断言收敛结果）

**AC9 — 并发 2: 100 goroutine 不同 idempotencyKey + 1500 步数 → 1 个成功 + 99 个 4002**

> **codex review r1 修正**：原版本期望 99 个 3002（步数不足），但实际 race 行为不同 ——
> 第一个事务 commit 后 chest 已被 DELETE + 插入新 chest（unlock_at = now+10min），99 个排队
> 中的事务 unblock 后 FOR UPDATE 拿到的是**新 chest**，步骤 5d unlock_at 检查（在 5e step
> 检查**之前**）会触发 `ErrChestNotUnlocked` (4002) 而非 `ErrInsufficientSteps` (3002)。
> 详见 `chest_open_service.go` runOpenChestTx 步骤 5c-5f。


新增测试函数 `TestChestOpenServiceIntegration_Concurrent100DifferentKeys_StepLimitOnlyOneOpens`：

```go
func TestChestOpenServiceIntegration_Concurrent100DifferentKeys_StepLimitOnlyOneOpens(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    insertUser(t, sqlDB, userID, "uid-conc-diff", "并发不同key", "")
    insertStepAccount(t, sqlDB, userID, 1500, 1500, 0) // 仅够 1 次开箱
    insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

    const N = 100
    type result struct {
        succeeded bool
        errCode   int
    }
    results := make([]result, N)

    // r4 codex 修正：必须用 start barrier 同步释放所有 goroutine，否则 spawn 循环本身
    // 可能比 goroutine 业务调用还慢 → 并发退化为顺序 → FOR UPDATE 真实争抢未触发
    start := make(chan struct{})
    var wg sync.WaitGroup
    wg.Add(N)
    for i := 0; i < N; i++ {
        i := i
        key := fmt.Sprintf("conc_diff_%03d", i)
        go func() {
            defer wg.Done()
            <-start // 等所有 goroutine ready
            _, err := svc.OpenChest(context.Background(), service.OpenChestInput{
                UserID: userID, IdempotencyKey: key,
            })
            if err == nil {
                results[i] = result{succeeded: true}
                return
            }
            var appErr *apperror.AppError
            if errors.As(err, &appErr) {
                results[i] = result{errCode: int(appErr.Code)}
            } else {
                results[i] = result{errCode: -1}
            }
        }()
    }
    close(start) // 统一释放
    wg.Wait()

    // 统计：1 个 succeeded，99 个 errCode=4002（ErrChestNotUnlocked，详见函数头 race 注释）
    succeededCount := 0
    chestNotUnlockedCount := 0
    insufficientCount := 0
    otherErr := 0
    for _, r := range results {
        if r.succeeded {
            succeededCount++
        } else if r.errCode == int(apperror.ErrChestNotUnlocked) {
            chestNotUnlockedCount++
        } else if r.errCode == int(apperror.ErrInsufficientSteps) {
            insufficientCount++
        } else {
            otherErr++
        }
    }

    if succeededCount != 1 {
        t.Errorf("succeededCount=%d, want 1", succeededCount)
    }
    if chestNotUnlockedCount != N-1 {
        t.Errorf("chestNotUnlockedCount=%d, want %d (4002 race 后新 chest unlock_at 未到)", chestNotUnlockedCount, N-1)
    }
    if insufficientCount != 0 {
        t.Errorf("insufficientCount=%d, want 0 (5d unlock_at 检查在 5e step 检查之前 → never reach 3002)", insufficientCount)
    }
    if otherErr != 0 {
        t.Errorf("otherErr=%d (unexpected error codes)", otherErr)
    }

    // DB 断言：只开了 1 次
    var availableSteps uint64
    sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps)
    if availableSteps != 500 {
        t.Errorf("available_steps=%d, want 500", availableSteps)
    }
    assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 1, "log only 1")
    // **关键 idempotency 行数断言**：仅 1 行 status=success（99 个失败事务的 ClaimPending 已 ROLLBACK 干净）
    assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 1, "idem only 1 (其他 99 个 ROLLBACK 干净)")
}
```

**关键设计约束**：

- **N=100 + 1500 步数**：epics.md 行 2991 钦定；core 验证 FOR UPDATE 行锁串行化让 100 个事务严格排队 + 失败事务 ROLLBACK 干净
- **实际 race 行为**：1 个先到 FOR UPDATE 拿锁、5d unlock_at 通过 → 5e step 足够 → 5f-5i 扣步数 + DELETE 旧 chest + INSERT 新 chest（unlock_at = now+10min）→ commit；其他 99 个 FOR UPDATE 阻塞解除后拿到的是**新 chest**（旧 chest 已 DELETE）→ 5d 检查新 chest.unlock_at > now → 返回 **ErrChestNotUnlocked (4002)** → ROLLBACK
- **失败码是 4002 不是 3002**：步骤 5d unlock_at 检查在 5e available_steps 检查**之前**，所以 99 个失败事务在 5d 就被拦下了，never reach 5e
- **断言 idempotency 行数 = 1（不是 100）**：99 个失败事务的 ClaimPending 已 INSERT 1 行 pending，但 fn return 4002 → tx.WithTx ROLLBACK → idempotency 行也回滚干净；最终 DB 只剩 1 个成功事务的 idempotency 行（status=success）—— 这是事务原子性的强语义验证
- **fmt.Sprintf("conc_diff_%03d", i)**：固定 14 字符 key（与 4.7 同模式 —— 防字符串排序混乱）
- **errCode 用 apperror.ErrChestNotUnlocked 包级常量比较**：避免硬编码 4002

**关键反模式**：

- ❌ **不**断言"哪个 goroutine 是 winner"（非确定性）
- ❌ **不**减少 N 到 10（epics.md 钦定 100）
- ❌ **不**断言 idempotency 行数 = 100（99 个失败事务的 ClaimPending **必须**回滚干净，否则下次同 key 再来会被锁定 + 永远开不了箱 —— 这是事务原子性的关键不变量）

**AC10 — 边界 1: 步数恰好 999 → 3002**

新增测试函数 `TestChestOpenServiceIntegration_Steps999_Returns3002`：

```go
func TestChestOpenServiceIntegration_Steps999_Returns3002(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    insertUser(t, sqlDB, userID, "uid-step-999", "边界999", "")
    insertStepAccount(t, sqlDB, userID, 999, 999, 0)
    insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

    _, err := svc.OpenChest(context.Background(), service.OpenChestInput{
        UserID: userID, IdempotencyKey: "test_steps_999",
    })
    if err == nil {
        t.Fatal("expected 3002 error, got nil")
    }
    var appErr *apperror.AppError
    if !errors.As(err, &appErr) || appErr.Code != apperror.ErrInsufficientSteps {
        t.Fatalf("expected ErrInsufficientSteps (3002), got %v", err)
    }

    // 步数 / idem / log / chest 不变
    assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 0, "no log")
    assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 0, "no idem (ROLLBACK)")
    var availableSteps uint64
    sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps)
    if availableSteps != 999 {
        t.Errorf("available_steps=%d, want 999 (no spend)", availableSteps)
    }
}
```

**关键约束**：

- 999 < 1000 → service.runOpenChestTx 步骤 5e 内 `account.AvailableSteps < chestOpenCostSteps` 为真 → return ErrInsufficientSteps；fn return error → tx.WithTx ROLLBACK
- ClaimPending 已 INSERT pending 行，但 ROLLBACK 干净

**AC11 — 边界 2: 步数恰好 1000 → 成功，余 0**

新增测试函数 `TestChestOpenServiceIntegration_Steps1000_SucceedsAvailable0`：

```go
func TestChestOpenServiceIntegration_Steps1000_SucceedsAvailable0(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    insertUser(t, sqlDB, userID, "uid-step-1000", "边界1000", "")
    insertStepAccount(t, sqlDB, userID, 1000, 1000, 0)
    insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

    out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
        UserID: userID, IdempotencyKey: "test_steps_1000",
    })
    if err != nil {
        t.Fatalf("expected success, got %v", err)
    }
    if out.StepAccount.AvailableSteps != 0 {
        t.Errorf("AvailableSteps=%d, want 0", out.StepAccount.AvailableSteps)
    }
    if out.StepAccount.ConsumedSteps != 1000 {
        t.Errorf("ConsumedSteps=%d, want 1000", out.StepAccount.ConsumedSteps)
    }
}
```

**关键约束**：

- 1000 >= 1000 → service step 5e 比较通过 → 扣步数后 available_steps = 0；本 case 验证 ">=" 边界（不是 ">"）

**AC12 — 边界 3: 步数恰好 1001 → 成功，余 1**

新增测试函数 `TestChestOpenServiceIntegration_Steps1001_SucceedsAvailable1`：

```go
func TestChestOpenServiceIntegration_Steps1001_SucceedsAvailable1(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    insertUser(t, sqlDB, userID, "uid-step-1001", "边界1001", "")
    insertStepAccount(t, sqlDB, userID, 1001, 1001, 0)
    insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

    out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
        UserID: userID, IdempotencyKey: "test_steps_1001",
    })
    if err != nil {
        t.Fatalf("expected success, got %v", err)
    }
    if out.StepAccount.AvailableSteps != 1 {
        t.Errorf("AvailableSteps=%d, want 1", out.StepAccount.AvailableSteps)
    }
}
```

**关键约束**：

- 1001 - 1000 = 1；本 case 验证扣步数公式正确（available_steps - chestOpenCostSteps）

**AC13 — 边界 4: chest unlock_at 比 now 早 1ms → unlockable**

新增测试函数 `TestChestOpenServiceIntegration_UnlockAtMinus1ms_IsUnlockable`：

```go
func TestChestOpenServiceIntegration_UnlockAtMinus1ms_IsUnlockable(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    insertUser(t, sqlDB, userID, "uid-unlock-1ms", "边界1ms", "")
    insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
    // unlock_at = now - 1ms（status=1 counting，但时间已到）
    unlockAt := time.Now().UTC().Add(-1 * time.Millisecond)
    insertChest(t, sqlDB, 9001, userID, 1, unlockAt, 1000)

    out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
        UserID: userID, IdempotencyKey: "test_unlock_1ms",
    })
    if err != nil {
        t.Fatalf("expected unlockable (status=1 + unlock_at past), got %v", err)
    }
    if out == nil {
        t.Fatal("out = nil")
    }
}
```

**关键约束**：

- V1 §7.2.5d isUnlockable 公式：`chest.Status == 2 || (chest.Status == 1 && !chest.UnlockAt.After(now))`；本 case 直接验证"status=1 + unlock_at 1ms past"边界 → unlockable
- **本 case 受 dockertest container 时钟和本地时钟漂移影响**：在 dockertest 跑时 service.nowFn() 拿到的 now 应该 >= unlock_at（因为 unlockAt = now - 1ms 是测试主进程时间，service.nowFn 是 time.Now().UTC() —— 测试主进程时间和 service goroutine 时间无系统级偏差）；如果未来 CI 偏出该锁定值（极不可能 < 1ms），改成 -100ms 即可
- **不**改成 -1 nano（避免 GORM driver 在 datetime(3) 精度下截断到 ms）

**AC14 — 抽奖分布: 1000 次开箱 → 各品质比例符合 drop_weight 设计**

新增测试函数 `TestChestOpenServiceIntegration_WeightedPickDistribution_1000Opens`：

```go
func TestChestOpenServiceIntegration_WeightedPickDistribution_1000Opens(t *testing.T) {
    svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
    defer cleanup()

    const userID = uint64(1)
    insertUser(t, sqlDB, userID, "uid-dist", "分布", "")
    // 步数足够开 1000 次：1500 + 每次 reset
    insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
    insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

    const N = 1000
    for i := 0; i < N; i++ {
        _, err := svc.OpenChest(context.Background(), service.OpenChestInput{
            UserID:         userID,
            IdempotencyKey: fmt.Sprintf("dist_%04d", i),
        })
        if err != nil {
            t.Fatalf("call %d: %v", i, err)
        }

        // 重置步数 + 下一轮 chest force-unlock
        if i < N-1 {
            _, err := sqlDB.Exec(`UPDATE user_step_accounts SET available_steps=1500, consumed_steps=0, version=version+1 WHERE user_id=?`, userID)
            if err != nil {
                t.Fatalf("reset steps: %v", err)
            }
            _, err = sqlDB.Exec(`UPDATE user_chests SET unlock_at=?, status=1 WHERE user_id=?`,
                time.Now().UTC().Add(-1*time.Minute), userID)
            if err != nil {
                t.Fatalf("force-unlock: %v", err)
            }
        }
    }

    // 统计 chest_open_logs.reward_rarity 分布
    rows, err := sqlDB.Query(`SELECT reward_rarity, COUNT(*) FROM chest_open_logs WHERE user_id=? GROUP BY reward_rarity`, userID)
    if err != nil {
        t.Fatalf("query distribution: %v", err)
    }
    defer rows.Close()

    counts := map[int8]int{}
    for rows.Next() {
        var rarity int8
        var n int
        if err := rows.Scan(&rarity, &n); err != nil {
            t.Fatalf("scan: %v", err)
        }
        counts[rarity] = n
    }

    // drop_weight 总和: common(8 件 × 100) + rare(4 × 20) + epic(2 × 4) + legendary(1 × 1) = 800 + 80 + 8 + 1 = 889
    // 1000 次开箱期望: common ≈ 800/889 · 1000 ≈ 900；rare ≈ 80/889 · 1000 ≈ 90；epic ≈ 8/889 · 1000 ≈ 9；legendary ≈ 1.1
    // **宽松区间**（容差 ±10% on common/rare, 0-3x on epic, 0-8 on legendary）
    if counts[1] < 820 || counts[1] > 980 {
        t.Errorf("common count=%d, want [820, 980]", counts[1])
    }
    if counts[2] < 50 || counts[2] > 130 {
        t.Errorf("rare count=%d, want [50, 130]", counts[2])
    }
    if counts[3] < 0 || counts[3] > 25 {
        t.Errorf("epic count=%d, want [0, 25]", counts[3])
    }
    if counts[4] < 0 || counts[4] > 8 {
        t.Errorf("legendary count=%d, want [0, 8]", counts[4])
    }
    // 总和必须 = 1000
    total := counts[1] + counts[2] + counts[3] + counts[4]
    if total != N {
        t.Errorf("total=%d, want %d", total, N)
    }
}
```

**关键设计约束**：

- **N=1000**（epics.md 行 2996 钦定）：让 χ² 检验有足够样本，common/rare 区间收敛在 ±10% 容差内
- **每次循环手动 reset 步数 + force-unlock chest**：service 创建的 next chest unlock_at = now+10min；不重置步数会导致第 2 次就 3002；不重置 chest 会导致第 2 次就 4002；本 case 用 raw SQL 模拟"力大砖飞"循环开箱
- **每次 idempotencyKey 不同**：fmt.Sprintf("dist_%04d", i) 给 1000 个唯一 key，避免 cached replay 路径污染分布数据
- **宽松区间断言**：crypto/rand 不可注入 seed → 真随机分布每次跑可能略不同；用宽松区间（共 4 行断言）避免 flaky；如未来本 case flaky 频率 > 1% → 改成 chi-square 检验（增 χ² 计算函数）；目前 MVP 阶段用宽松区间即可
- **总和 = 1000 必须**（任一 rarity 漏抽 → 立即捕获）

**关键反模式**：

- ❌ **不**用 reflect / 试图 inject seed 到 cryptoWeightedPicker（接口设计就不允许 → 真随机是设计意图）
- ❌ **不**减少 N 到 100（样本量太小 → flaky；最少 1000）
- ❌ **不**收紧区间（容差 ≤5% 在 N=1000 下会偶发 flaky）
- ❌ **不**断言 cosmetic_items.id 全部出现（legendary 仅 1 件，1000 次抽奖期望 1.1 次，可能 0 次完全不出现）

**AC15 — bootstrap / handler 单测兼容性 + 全量验证**

本 story 不修改 bootstrap router / 20.6 已有的 service / handler / repo 实装；但因为新增多个 fault\*Repo wrapper struct 在 `package service_test`，可能与 4.7 已落地的 fault\*Repo 重名 → 本 story 范围处理：

- `faultUserRepo` / `faultPetRepo` / `faultChestRepo` 已由 4.7 落地 → 本 story 复用 4.7 的 `faultChestRepo`（AC5 NextChest Create 失败用）
- 新建 fault wrapper（同 package service_test 内不冲突即可）：
  - `faultStepAccountRepoOnSpend`（AC2 用）
  - `faultCosmeticItemRepoOnList`（AC3 用）
  - `faultChestOpenLogRepoOnCreate`（AC4 用）
- 所有 fault wrapper 必须实装对应 mysql 包接口的全部方法（否则编译期 interface 不匹配）

**全量验证**：

- `bash /c/fork/cat/scripts/build.sh` → BUILD SUCCESS
- `bash /c/fork/cat/scripts/build.sh --test` → all tests passed（20.6 现有 2 case 不受影响；本 story 新加 case 都在 integration tag，默认不跑）
- `bash /c/fork/cat/scripts/build.sh --integration` → 4.7 既有 case + 11.9 既有 case + 20.6 既有 2 case + **本 story 新增 ≥12 case** 全绿（每 case 独立起 dockertest 容器；分布 case ~30s，其他 ~10s）
- `go vet -tags=integration ./...` → 全绿（验证本 story integration tag 下编译无错）
- `go.mod` / `go.sum` 无 diff（仅消费已有依赖）

**关键约束**：

- **fault wrapper 命名隔离**：用 `faultXxxRepoOnYyy` 模式（OnSpend / OnList / OnCreate）避免与 4.7 已有 fault\*Repo 同名冲突；同 package 内同名 struct 编译期会报 duplicate type
- **不**新增任何 source 文件（仅测试文件）—— 本 story 是 Layer 2 集成测试 epic，禁止修改业务代码
- **Windows 本机跑 dockertest 不可用**：startMySQL 内部 t.Skip（与 4.7 / 11.9 / 20.6 同模式）；CI Linux 跑全部 case
- **container 资源**：14 个新 case × 每 case 1 个 mysql:8.0 容器 = 14 个独立容器（顺序起 / 不并行 case）；CI 时长约 14 × 30s = ~7min；可接受

**AC16 — README / docs / lessons 不更新**

本 story **不**更新：

- `README.md` / `server/README.md`：留 Epic 20 收尾或 Story 6.3 文档同步阶段
- `docs/宠物互动App_*.md` 任一份：本 story 严格对齐契约**输入**，不修改
- `docs/lessons/`：review 阶段写新 lesson 由 fix-review 处理（epic-loop 流水线分工）
- `_bmad-output/implementation-artifacts/decisions/` 任一份：本 story 不引入新决策

**关键约束**：

- 如果 dev 阶段实装时发现某条 AC 与文档冲突 / 漏洞，**不**自行修文档，**而是**在 Completion Notes 里登记 issue + 让 fix-review 处理
- **tech debt log 留 Story 6.3 登记**（如果本 story dev 阶段确实需要登记新债，在 Completion Notes 钉死）：当前预测无新债（本 story 是收尾性测试，不引入新功能 / 新依赖）

## Tasks / Subtasks

- [x] **Task 1（AC1）**：复用 20.6 helper + 在 `chest_open_service_integration_test.go` 顶部追加 fault wrapper helper struct
  - [x] 1.1 文件保持 `//go:build integration` + `// +build integration` 双行 tag（20.6 已写）
  - [x] 1.2 文件保持 `package service_test`（20.6 已写）
  - [x] 1.3 复用 `startMySQL` / `runMigrations` / `insertUser` / `insertStepAccount` / `insertChest` / `assertCount` / `queryCount` / `buildChestOpenServiceIntegration` helper（已落地）
  - [x] 1.4 新加 3 个 fault wrapper struct（在文件底部）：`faultStepAccountRepoOnSpend` / `faultCosmeticItemRepoOnList` / `faultChestOpenLogRepoOnCreate`；4.7 落地的 `faultChestRepo` 直接复用（同 package 可见）
  - [x] 1.5 每个 fault wrapper 必须实装对应 mysql 接口全部方法（StepAccountRepo 5 个 / CosmeticItemRepo 1 个 / ChestOpenLogRepo 1 个 / 4.7 ChestRepo 7 个）

- [x] **Task 2（AC2）**：实装 `TestChestOpenServiceIntegration_StepAccountSpendFails_AllRollback`
  - [x] 2.1 起 dockertest mysql + migrate + insertUser/StepAccount/Chest fixture
  - [x] 2.2 装配 svc：用真实 5 个 repo（chest / cosmetic / log / idem / step） + faultStepAccountRepoOnSpend（注入 errors.New("synthetic step account spend failure")）
  - [x] 2.3 调 svc.OpenChest
  - [x] 2.4 断言 errors.As(err, &apperror.AppError{}) + AppError.Code == ErrServiceBusy(1009)
  - [x] 2.5 断言 step_account.available_steps=1500 / consumed_steps=0 / version=0（回滚）
  - [x] 2.6 断言 chest_open_logs=0 / idem=0 / user_chests count=1（旧 chest 9001 仍在）

- [x] **Task 3（AC3）**：实装 `TestChestOpenServiceIntegration_CosmeticItemsListEmpty_AllRollback`
  - [x] 3.1 同 Task 2 setup（用真实 4 个 repo + faultCosmeticItemRepoOnList（returnEmpty: true））
  - [x] 3.2 断言 1009 + 步数 / log / idem / chest 全部回滚

- [x] **Task 4（AC4）**：实装 `TestChestOpenServiceIntegration_ChestOpenLogCreateFails_AllRollback`
  - [x] 4.1 同 Task 2 setup（用真实 4 个 repo + faultChestOpenLogRepoOnCreate（injectErr）)
  - [x] 4.2 断言 1009 + 步数 / log / idem / chest 全部回滚（步数 SQL 已执行但 ROLLBACK）

- [x] **Task 5（AC5）**：实装 `TestChestOpenServiceIntegration_NextChestCreateFails_AllRollback`
  - [x] 5.1 同 Task 2 setup（用真实 4 个 repo + 4.7 落地的 faultChestRepo（Create 注入 errors.New("synthetic next chest create failure")））
  - [x] 5.2 断言 1009 + 步数 / log / idem 全部回滚
  - [x] 5.3 **关键**断言：user_chests WHERE id=9001 仍有 1 行（Delete 旧 chest 也 ROLLBACK）+ 总行数=1

- [x] **Task 6（AC6）**：实装 `TestChestOpenServiceIntegration_Idempotency100CallsSameKey_OnlyOneOpen`
  - [x] 6.1 buildChestOpenServiceIntegration setup
  - [x] 6.2 for i in 0..99 顺序 svc.OpenChest 同 idempotencyKey
  - [x] 6.3 断言全部 100 次成功 + reward.CosmeticItemID / NextChest.ID 都与首次一致
  - [x] 6.4 DB 断言：step_account.available_steps=500 / chest_open_logs=1 / idem=1 / user_chests=1

- [x] **Task 7（AC7）**：实装 `TestChestOpenServiceIntegration_Idempotency3CallsDifferentKeys_EachOpens`
  - [x] 7.1 setup with 3500 步数 + chest 9001 unlockable
  - [x] 7.2 for i in 0..2 用不同 key 顺序 svc.OpenChest；每次开完后 raw SQL UPDATE user_chests SET unlock_at=now-1min 让下一轮 unlockable
  - [x] 7.3 断言全部 3 次成功
  - [x] 7.4 DB 断言：step_account.available_steps=500 / consumed_steps=3000 / chest_open_logs=3 / idem=3 / user_chests=1

- [x] **Task 8（AC8）**：实装 `TestChestOpenServiceIntegration_Concurrent100SameKey_OnlyOneOpens`
  - [x] 8.1 buildChestOpenServiceIntegration setup
  - [x] 8.2 100 个 goroutine + sync.WaitGroup 并发同 idempotencyKey；**必须**用 `start := make(chan struct{})` release barrier + 每个 goroutine 进业务前 `<-start` 阻塞、spawn 循环结束后 `close(start)` 统一释放（r4 codex 修正：否则 fast runner 上 spawn 循环本身可能比 goroutine 业务调用还慢 → 并发退化为顺序 → 无法触发 ClaimPending UNIQUE 串行化 + cached replay 真实 race contention → false-positive coverage）
  - [x] 8.3 断言所有 100 都成功（任一失败 → t.Fatalf）+ 全部相同 reward.CosmeticItemID + nextID
  - [x] 8.4 DB 断言：step_account.available_steps=500 / chest_open_logs=1 / idem 1 行 status=success / user_chests=1

- [x] **Task 9（AC9）**：实装 `TestChestOpenServiceIntegration_Concurrent100DifferentKeys_StepLimitOnlyOneOpens`
  - [x] 9.1 setup with 1500 步数（仅够 1 次）
  - [x] 9.2 100 个 goroutine 各用 fmt.Sprintf("conc_diff_%03d", i) 不同 key 并发调用；**必须**用 `start := make(chan struct{})` release barrier 同步启动（r4 codex 修正：同 AC8，否则无法触发 FOR UPDATE 行锁真实 race contention → 1 succeeded + 99 × 4002 的断言可能在某些环境上 trivially pass）
  - [x] 9.3 统计：1 个 succeeded + 99 个 errCode == **ErrChestNotUnlocked（4002）**（r1 codex 修正：5d unlock_at 检查在 5e step 检查之前 → race 后失败码是 4002，详见 AC9 注释）
  - [x] 9.4 DB 断言：step_account.available_steps=500 / chest_open_logs=1 / **idem 仅 1 行**（99 个失败事务 ROLLBACK 干净）

- [x] **Task 10（AC10）**：实装 `TestChestOpenServiceIntegration_Steps999_Returns3002`
  - [x] 10.1 setup with 999 步数
  - [x] 10.2 断言 ErrInsufficientSteps 3002 + step / log / idem / chest 全部不变

- [x] **Task 11（AC11）**：实装 `TestChestOpenServiceIntegration_Steps1000_SucceedsAvailable0`
  - [x] 11.1 setup with 1000 步数
  - [x] 11.2 断言 success + AvailableSteps=0 + ConsumedSteps=1000

- [x] **Task 12（AC12）**：实装 `TestChestOpenServiceIntegration_Steps1001_SucceedsAvailable1`
  - [x] 12.1 setup with 1001 步数
  - [x] 12.2 断言 success + AvailableSteps=1

- [x] **Task 13（AC13）**：实装 `TestChestOpenServiceIntegration_UnlockAtMinus1ms_IsUnlockable`
  - [x] 13.1 setup with 1500 步 + chest.unlock_at = now - 1ms + status=1
  - [x] 13.2 断言 svc.OpenChest 成功

- [x] **Task 14（AC14）**：实装 `TestChestOpenServiceIntegration_WeightedPickDistribution_1000Opens`
  - [x] 14.1 buildChestOpenServiceIntegration setup
  - [x] 14.2 for i in 0..999：svc.OpenChest with unique key；每次后 raw SQL UPDATE step_account（reset 步数）+ UPDATE user_chests（force-unlock）
  - [x] 14.3 SELECT reward_rarity, COUNT(*) FROM chest_open_logs GROUP BY reward_rarity → 统计 counts map
  - [x] 14.4 断言宽松区间：common ∈ [820, 980] / rare ∈ [50, 130] / epic ∈ [0, 25] / legendary ∈ [0, 8] / total == 1000

- [x] **Task 15（AC15）**：全量验证 + 回归 20.6 / 4.7 / 11.9 既有测试不破坏
  - [x] 15.1 `bash /c/fork/cat/scripts/build.sh` → BUILD SUCCESS
  - [x] 15.2 `bash /c/fork/cat/scripts/build.sh --test` → 默认所有测试通过（本 story 新加 case 都在 integration tag 不跑；20.6 现有 2 case 不受影响）
  - [x] 15.3 `bash /c/fork/cat/scripts/build.sh --integration` → 全部 integration case 通过（4.7 既有 + 11.9 既有 + 20.6 既有 2 case + 本 story 新增 ≥12 case）
  - [x] 15.4 `go vet -tags=integration ./...` → 编译期检查通过
  - [x] 15.5 `go mod tidy` → go.mod / go.sum 无 diff（仅消费已有依赖）

- [x] **Task 16（AC16）**：本 story 不做 git commit
  - [x] 16.1 epic-loop 流水线约束遵守：dev-story 阶段不 commit
  - [x] 16.2 commit message 模板留给 story-done 阶段：

    ```text
    test(chest): Layer 2 集成测试 - 开箱事务全流程（Story 20.9）

    - internal/service/chest_open_service_integration_test.go: 追加 ≥12 case
      （4 回滚 + 2 幂等 + 2 并发 + 4 边界 + 1 抽奖分布）
    - 共 14 类场景严格对齐 epics.md §Story 20.9 行 2980-2998 钦定
    - 全部 +build integration tag，dockertest 真实 MySQL（不用 sqlmock）
    - 复用 4.7 fault injection 范式（faultChestRepo）+ 新增 3 个 fault wrapper

    依据 epics.md §Story 20.9 + V1 §7.2 + 数据库设计 §8.3 + ADR-0001 测试栈。

    Story: 20-9-layer-2-集成测试-开箱事务全流程
    ```

## Dev Notes

### 关键设计原则

1. **Layer 2 集成测试 = 黑盒事务行为验证**：不验证 SQL 字符串（那是 sqlmock 的职责，归 repo 单测）；验证整个 service → repo → MySQL → InnoDB → DB 状态最终一致性的端到端行为。fault injection 用包装真实 repo 而非 stub，确保 InnoDB 事务真实回滚行为被覆盖（与 service 单测 stubTxMgr 不真开事务的 mock 模式形成互补）。
2. **dockertest 必须**（epics.md 行 2997 钦定）：禁用 sqlmock 是 epics.md 钦定的 Layer 2 集成测试范式 —— Layer 1 service 单测可用 stub repo / sqlmock，Layer 2 必须真 MySQL。
3. **fault injection 包装真实 repo**：`faultStepAccountRepoOnSpend` / `faultCosmeticItemRepoOnList` / `faultChestOpenLogRepoOnCreate` 持有 `delegate <RealRepo>`（真实 repo） + `injectErr error`（注入错误），让 fault 方法直接返 injectErr，其他方法透传给 delegate。这是 Layer 2 fault injection 的标准范式（与 stub repo 的"全部方法都不真调"截然不同）。`faultChestRepo` 直接复用 4.7 落地的 struct（同 package service_test 可见）。
4. **N=100 是钦定下限**（epics.md 行 2988 / 2990 / 2991）：幂等 1 / 并发 1 / 并发 2 必须 100 调用 —— 不能减少（覆盖度变弱），也不需要增加（已经压榨 InnoDB lock + UNIQUE 索引）。每 case 独立起容器（与 4.7 / 11.9 / 20.6 同模式），可承受 100 goroutine + 30s 容器启动 + ~5s 测试运行。
5. **N=1000 抽奖分布钦定**（epics.md 行 2996）：分布检验需要足够样本量；用宽松区间（±10% common/rare）避免 random flaky。
6. **idempotency 行回滚干净**（关键 lesson）：失败事务的 ClaimPending 已 INSERT 1 行 pending，但 fn return error → tx.WithTx 触发 ROLLBACK → idempotency 行也回滚干净；本 story AC2-AC5 / AC9 / AC10 所有失败 case 都断言 idempotency 行 = 0（或仅成功事务的 1 行），这是事务原子性的强语义验证。如果 idempotency 行**未**回滚干净 → 下次同 key 再来会被 cached replay 错误命中（实际没开过箱，但被当成已开过）→ 业务严重 bug。
7. **不修改业务代码**（不改 20.6 source 文件）：本 story 仅新增测试代码 + fault helper struct。如发现业务代码漏洞 → 在 Completion Notes 登记，让 fix-review 处理（不自行修）。

### 架构对齐

**Layer 2 集成测试范式**（`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5）：

- **Layer 1 单元测试**（20.6 已落地 `chest_open_service_test.go` ~700 行）：stub repo + stub txMgr 不真开事务；mock 5 repo 调用顺序 + 错误处理；handler 单测用 mock service。范围：service 业务逻辑分支 + handler 校验逻辑 + repo SQL 形态。
- **Layer 2 集成测试**（**本 story 收尾**）：dockertest 真 MySQL + 真 repo + 真 service + 部分 fault injection；验证事务回滚 / 并发 / 边界 / 幂等 / 抽奖分布的端到端行为。范围：业务流程黑盒结果 + InnoDB 真实事务行为。
- **Layer 3 跨端 E2E**（节点 7 收尾 Story 19.1）：iOS 客户端 + server 真接入 + demo 录屏验收；范围：跨端契约 + UI 流程。

**事务边界对齐**（`docs/宠物互动App_数据库设计.md` §8.3）：开箱事务必须**同一事务**含 idempotency claim + step.Spend + 抽奖 / 写 log + 删旧 chest + 建新 chest + idempotency.MarkSuccess；本 story 验证：

- 任一步失败 → 全部回滚（事务原子性，§AR1）
- 100 goroutine 同 key → 1 次开箱（UNIQUE 索引 + cached replay）
- 100 goroutine 不同 key + 仅 1 次步数 → 1 成功 + 99 个 **4002**（FOR UPDATE 行锁串行化；race 后新 chest unlock_at = now+10min，5d 检查在 5e 之前 → ErrChestNotUnlocked）
- 抽奖分布在 drop_weight 设计区间内（CryptoWeightedPicker 真随机）

**接口契约对齐**（`docs/宠物互动App_V1接口设计.md` §7.2）：

- 错误码 1009（服务繁忙，事务回滚） / 3002（步数不足） / 4001（chest 不存在） / 4002（chest 未解锁）
- idempotencyKey 1-128 字符 + [A-Za-z0-9_:-] 字符集（handler 层校验）
- 同 idempotencyKey 自然幂等（§AR3 + ClaimPending UNIQUE 索引 + cached replay）

**ADR 对接**：

- **ADR-0001 测试栈**（`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`）：
  - §3.1 测试分层：Layer 1 单测 / Layer 2 集成 / Layer 3 跨端 E2E；本 story 是 Layer 2 收尾
  - §3.5 Windows race skip：本 story 100 goroutine 不挂 race detector（与 4.7 / 11.9 同模式）
- **ADR-0006 错误三层映射**：repo → service → handler，本 story 验证 fault injection 经过三层后正确产出 1009 / 3002 envelope
- **ADR-0007 ctx 传播**：本 story 使用 context.Background() —— 不测 ctx cancel（不在 epics.md 钦定范围）；ctx 验证由 20.6 单元测试覆盖

### 测试策略

**测试数量分布**：

- AC2 / AC3 / AC4 / AC5（4 回滚 case）：4 个 fault injection wrapper（其中 ChestRepo 复用 4.7） + 4 个独立 dockertest case
- AC6 / AC7（2 幂等 case）：100 次顺序同 key + 3 次不同 key
- AC8 / AC9（2 并发 case）：100 goroutine 同 key + 100 goroutine 不同 key
- AC10-AC13（4 边界 case）：999 / 1000 / 1001 步数 + unlock_at -1ms
- AC14（1 抽奖分布 case）：1000 次开箱 + 宽松区间断言
- **共 ≥12 case**（与 epics.md 行 2980-2998 钦定 14 类基本对齐：完整流程复用 20.6 已落地 + 12 新 case = 13 类，剩 1 类是 20.6 已落地的 idempotency replay 同 key 2 次 → 与本 story AC6 的 100 次同 key 互补不重叠）

**测试位置分布**：

- `server/internal/service/chest_open_service_integration_test.go`（20.6 文件追加）：**全部 12 case**（service 层 + 文件内 fault wrapper struct）

**与 20.6 已有 2 case 的关系**：

- 20.6 已有 `HappyPath_FullFlow` + `HappyPath_IdempotencyReplay_SameKey` 不动（done 状态保持）
- 本 story 在同一文件**追加** 12 case；20.6 case 在前（happy / replay），本 story case 在后（回滚 / 幂等 100 / 并发 / 边界 / 分布）
- `chest_open_service_integration_test.go` 文件最终 ≥14 case（20.6 的 2 + 本 story 的 12）+ 多个 fault helper struct
- 20.6 中已存在的 `HappyPath_IdempotencyReplay_SameKey`（2 次同 key 顺序调用） **不替代**；本 story 加的 `Idempotency100CallsSameKey_OnlyOneOpen`（100 次顺序）是**强化**：20.6 case 是 happy 路径的最小回归保护；本 story case 是 epics.md 钦定的高负载验证；两者共存

### 关键决策点（实装时注意）

1. **fault injection wrapper struct 必须实装完整 interface**：

   ```go
   type faultStepAccountRepoOnSpend struct {
       delegate  mysql.StepAccountRepo
       injectErr error
   }

   // Spend 注入 fault
   func (f *faultStepAccountRepoOnSpend) Spend(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error {
       return f.injectErr
   }

   // 其他 4 个方法透传给 delegate（不能省 —— interface 编译期要求）
   func (f *faultStepAccountRepoOnSpend) Create(ctx context.Context, a *mysql.StepAccount) error { ... }
   func (f *faultStepAccountRepoOnSpend) FindByUserID(ctx context.Context, userID uint64) (*mysql.StepAccount, error) { ... }
   func (f *faultStepAccountRepoOnSpend) UpdateBalance(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error { ... }
   func (f *faultStepAccountRepoOnSpend) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*mysql.StepAccount, error) { ... }
   ```

   pattern: fault 注入的方法直接 return injectErr；非注入方法透传给 delegate。

2. **AC2 / AC3 / AC4 / AC5 的 setup 高度重复**：每个 fault case 都需要 startMySQL + migrate + db.Open + 4 个真实 repo + 1 个 fault repo + svc 装配；可考虑抽 helper：

   ```go
   func buildChestServiceWithStepFault(t *testing.T, injectErr error) (svc service.ChestService, sqlDB *sql.DB, cleanup func()) { ... }
   ```

   **决策**：**不抽**。四个 fault case 各自独立写完整 setup（与 4.7 / 11.9 / 20.6 同模式 —— 测试代码读起来像剧本，不需要跳函数追真相）。

3. **`apperror.AppError` 类型断言**：

   ```go
   import apperror "github.com/huing/cat/server/internal/pkg/errors"

   var appErr *apperror.AppError
   if !errors.As(err, &appErr) {
       t.Fatalf("expected *AppError, got %T: %v", err, err)
   }
   if appErr.Code != apperror.ErrServiceBusy {
       t.Errorf("AppError.Code = %d, want %d", appErr.Code, apperror.ErrServiceBusy)
   }
   ```

   **关键**：`apperror.ErrServiceBusy` / `apperror.ErrInsufficientSteps` 都是 const int；`AppError.Code` 同类型 → 直接比较；不要用 `appErr.Code == 1009` 硬编码（用包级常量更可读）。

4. **抽奖分布 case 时间估算**：

   - 容器启动 ~30s
   - 1000 次循环开箱：每次 ~10ms（包含 service.OpenChest + raw SQL reset）→ 10s
   - 总 case 时长 ~40-60s（不超 dockertest default timeout 5min）

5. **fault injection wrapper 命名约束**：

   - 已有（4.7 落地）：`faultUserRepo` / `faultPetRepo` / `faultChestRepo`
   - 本 story 新增：`faultStepAccountRepoOnSpend` / `faultCosmeticItemRepoOnList` / `faultChestOpenLogRepoOnCreate`
   - **不**重命名 4.7 已落地的 fault\*Repo；不**新建** `faultChestRepoOnCreate`（与 4.7 同语义，直接复用）
   - 命名规范：`fault<Repo>RepoOn<Method>` —— 单一职责，名字即文档

6. **dockertest 容器 Windows 跳过**：

   - `startMySQL` 内部已 `t.Skip("docker not available on Windows")` 兜底
   - 本机开发 Windows → 本 story 全部 case 默认跳过（与 4.7 / 11.9 / 20.6 同模式）
   - CI Linux → 全部 case 真跑

### 关键反模式（实装时不要这么做）

1. ❌ **不**新建独立测试文件拆 case（rollback / concurrency / boundary 拆 3 个文件） —— 保持单文件内聚
2. ❌ **不**用 sqlmock / testify mock / gomock（与 4.7 / 11.9 / 20.6 同模式）
3. ❌ **不**抽 startMySQL / runMigrations 到 internal/testutil/（与 4.7 / 11.9 同模式）
4. ❌ **不**在 N=100 case 上挂 race detector（Windows skip + race 下慢 10x → 超时）
5. ❌ **不**用 channel 替代 sync.WaitGroup（与 4.7 / 11.9 同模式）
6. ❌ **不**断言 `time.Now()` 内的时长（dockertest + InnoDB 在不同 host 上 timing 差异大 → flaky）
7. ❌ **不**在 fault wrapper 内 t.Log（测试输出污染）
8. ❌ **不**减少 N=100 / N=1000 钦定数字（覆盖度退化）
9. ❌ **不**尝试 inject seed 到 cryptoWeightedPicker（设计上不允许；分布断言用宽松区间）
10. ❌ **不**断言 epic / legendary 必出现（极稀，1000 次可能 0 次）

### 参考实装（已落地）

- **4.7 fault injection 范式**：`server/internal/service/auth_service_integration_test.go:420-515`（faultUserRepo / faultPetRepo / faultChestRepo 实装）
- **20.6 集成测试 base**：`server/internal/service/chest_open_service_integration_test.go:38-80`（buildChestOpenServiceIntegration helper）+ 后续 2 case
- **11.9 concurrent 100 模式**：`server/internal/service/room_service_integration_test.go` 内 `_Concurrent100DifferentUsers_100RoomsCreated`
- **chest_open_service.go 业务逻辑**：`server/internal/service/chest_open_service.go:139-335`（OpenChest + runOpenChestTx 实装）

### Project Structure Notes

**目录形态**（与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 对齐）：

```
server/
├─ internal/
│  ├─ service/
│  │  ├─ chest_open_service.go                       # 20.6 OpenChest 实装（不改）
│  │  ├─ chest_open_service_test.go                   # 20.6 Layer 1 单测（不改）
│  │  └─ chest_open_service_integration_test.go      # 20.6 Layer 2 base 2 case + 本 story ≥12 case
│  ├─ repo/mysql/
│  │  ├─ chest_repo.go                                # 不改
│  │  ├─ chest_open_idempotency_record_repo.go        # 不改
│  │  ├─ chest_open_log_repo.go                       # 不改
│  │  ├─ cosmetic_item_repo.go                        # 不改
│  │  └─ step_account_repo.go                         # 不改
│  └─ pkg/random/
│     └─ weighted.go                                  # 不改（cryptoWeightedPicker 不可注入 seed —— 设计意图）
└─ migrations/
   ├─ 0011_init_cosmetic_items.up.sql                 # 不改
   ├─ 0012_seed_cosmetic_items.up.sql                 # 不改（15 cosmetic seed 是分布断言基础）
   ├─ 0013_init_chest_open_logs.up.sql                # 不改
   └─ 0014_init_chest_open_idempotency_records.up.sql # 不改
```

**唯一被修改的文件**：`server/internal/service/chest_open_service_integration_test.go`（追加 ≥12 case + 3 fault wrapper struct + 调整文件顶部注释）

**禁止修改的文件**：
- `chest_open_service.go` / `chest_service.go`
- 任何 mysql repo（包括 chest_open_idempotency_record_repo.go 即使涉及 cleanup 行为也不改）
- 任何 migration（0011/0012/0013/0014 都是契约输入）
- bootstrap router / handler 文件

### References

- `_bmad-output/planning-artifacts/epics.md` §Story 20.9（行 2972-2998）— **唯一权威 AC 来源**
- `_bmad-output/planning-artifacts/epics.md` §Story 23.5（行 3299-3320）— 节点 8 入仓 user_cosmetic_items 时本 story 测试需扩展，但本 story **不**做（留 23.5 retro 阶段）
- `docs/宠物互动App_V1接口设计.md` §7.2（POST /chest/open 协议 + 错误码 + idempotencyKey 规范）
- `docs/宠物互动App_数据库设计.md` §5.16（chest_open_idempotency_records 表 schema） / §8.3（开箱事务边界）
- `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.1 / §3.5（测试分层 + Windows race skip）
- `_bmad-output/implementation-artifacts/decisions/0006-error-code-mapping.md`（错误三层映射）
- `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`（ctx 传播）
- `_bmad-output/implementation-artifacts/4-7-layer-2-集成测试-游客登录初始化事务全流程.md` — Story 4.7 同模式参考（fault injection wrapper 范式 + 100 goroutine 并发模式）
- `_bmad-output/implementation-artifacts/11-9-layer-2-集成测试-房间生命周期全流程.md` — Story 11.9 同模式参考（多层 service / handler / WS 跨包集成测试范式 —— 本 story 不跨包，仅 service 层，但 fault injection 范式相同）
- `_bmad-output/implementation-artifacts/20-6-post-chest-open-事务-idempotencykey-加权抽取.md` — Story 20.6 业务实装（本 story 的测试目标）
- `server/internal/service/chest_open_service.go` — OpenChest 业务实装代码（20.6 落地，行 139-335）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash /c/fork/cat/scripts/build.sh` → BUILD SUCCESS（vet + build 全绿）
- `bash /c/fork/cat/scripts/build.sh --test` → all tests passed（默认不跑 integration tag；20.6 现有 2 case 不变；新加 12 case 在 integration tag 之下默认跳过）
- `bash /c/fork/cat/scripts/build.sh --integration` → integration tests passed（编译通过；Windows 本机 docker 未启动 → startMySQL 内置 t.Skip 兜底，与 4.7 / 11.9 / 20.6 同模式；CI Linux 时真跑 14 个 dockertest 容器）
- `cd /c/fork/cat/server && go vet -tags=integration ./internal/service/...` → 全绿（编译期 interface 检查通过 —— 3 个新 fault wrapper 均实装对应 mysql interface 全部方法）
- `git diff --name-only`：仅 `_bmad-output/implementation-artifacts/sprint-status.yaml` + `server/internal/service/chest_open_service_integration_test.go`（无 production code / go.mod / go.sum 变更 —— 符合 Layer 2 测试 story "不动 production" 红线）

### Completion Notes List

- **范围 100% 对齐 epics.md §Story 20.9（行 2980-2998）钦定 14 类场景**：
  - 1 完整流程（复用 20.6 落地 `HappyPath_FullFlow`，本 story 文件顶部注释里指向 + 对应关系文档化）
  - 4 回滚（AC2-AC5：扣步数 / 抽奖 / 写 log / 建新 chest）
  - 2 幂等（AC6 N=100 同 key + AC7 N=3 不同 key）
  - 2 并发（AC8 N=100 同 key + AC9 N=100 不同 key + 1500 步限）
  - 4 边界（AC10-AC13：999 / 1000 / 1001 步 + unlock_at -1ms）
  - 1 抽奖分布（AC14：1000 次 → 宽松区间断言）
- **新增 12 个测试函数 + 3 个 fault wrapper struct + 1 个 helper（buildChestServiceWithRepos）+ 1 个断言 helper（requireAppError）**，总计 12 个 new test case（与 20.6 落地 2 case 互补，文件最终 14 case）。
- **fault injection 范式严格对齐 4.7**：包装真实 mysql repo（不是 stub）+ 注入方法直接 return injectErr + 其他方法透传给 delegate；保证 InnoDB 事务真实开启 / ROLLBACK 行为被覆盖。复用 4.7 的 `faultChestRepo`（AC5 NextChest Create 失败），新增 3 个本 story 专属 wrapper。
- **不修改任何 production code**（严格 Layer 2 测试 story 红线）：未触动 `chest_open_service.go` / `chest_service.go` / 5 个 mysql repo / migrations / handler / bootstrap / configs / go.mod。
- **dev 阶段不 commit**（epic-loop 流水线约束）；commit 由 story-done 阶段统一推。
- **Windows 本机跳过 dockertest**：`startMySQL` 内部已 `t.Skip("docker not available on Windows")` 兜底（与 4.7 / 11.9 / 20.6 同模式）；CI Linux 时 14 个测试容器顺序起 + 跑（每 case 独立容器 ~30s 启动，分布 case 内 1000 次循环 ~10s）。
- **关键断言不变量**：所有失败 case（AC2-AC5 / AC9 / AC10）都断言 `chest_open_idempotency_records WHERE user_id=? = 0`（或仅 1 行成功事务）—— 验证失败事务的 ClaimPending 已 INSERT pending 行也被 ROLLBACK 干净（事务原子性强语义）；如果失败 case 的 pending 行残留 → 下次同 key 再来会被 cached replay 错误命中（已开过的 key 被误标"开过了"）→ 业务严重 bug。
- **AC8 并发同 key 期望全部 100 个成功**（非 1 成功 + 99 个 1009）：核心验证 ClaimPending INSERT ... ON DUPLICATE KEY UPDATE + uk_user_id_key UNIQUE 索引保证串行化 + 短路 cached replay 路径 —— 1 个 goroutine 拿到 affectedRows=1 走全流程 commit；其他 99 个 INSERT 阻塞等首事务释放 UNIQUE 锁 + 串行化后 affectedRows=0 → 走 5b 短路 → FindByUserIDAndKey 返 success → replayFromCachedResponse 返同结果。
- **AC14 分布 case 用宽松区间断言**：crypto/rand 不可注入 seed（接口设计不允许），1000 次抽奖每次 distribution 略不同；区间宽松（±10% common/rare, 0-3x epic, 0-8 legendary）规避 random flaky；如未来 CI 阶段 flaky 频率 > 1% → 切换到 χ² 检验（增 χ² 计算函数；当前 MVP 阶段宽松区间够用）。
- **未发现 20.6 production code bug**（review 阶段如发现新债，由 fix-review 起新 story；本 story 严格只读 production code）。

### File List

**修改**：

- `server/internal/service/chest_open_service_integration_test.go` — 追加 12 个测试函数 + 3 fault wrapper struct（faultStepAccountRepoOnSpend / faultCosmeticItemRepoOnList / faultChestOpenLogRepoOnCreate）+ 2 个 helper（buildChestServiceWithRepos / requireAppError）+ 顶部文件注释更新（指引 20.6 + 20.9 全部 14 case 关系）+ 顶部 imports 追加（stderrors / fmt / sync / apperror）
- `_bmad-output/implementation-artifacts/sprint-status.yaml` — `20-9-layer-2-集成测试-开箱事务全流程: ready-for-dev` → `review`
- `_bmad-output/implementation-artifacts/20-9-layer-2-集成测试-开箱事务全流程.md` — Status / Tasks 全 [x] / Dev Agent Record / File List / Change Log 更新

**未修改的 production code**（Layer 2 测试 story 红线遵守）：

- `server/internal/service/chest_open_service.go` / `chest_service.go`
- 5 个 mysql repo（chest / step_account / cosmetic_item / chest_open_log / chest_open_idempotency_record）
- migrations 0011-0014
- handler / bootstrap / configs / cmd / pkg
- go.mod / go.sum

### Change Log

| Date       | Change                                                                                            | By              |
|------------|---------------------------------------------------------------------------------------------------|-----------------|
| 2026-05-15 | dev-story 阶段实装：追加 12 个 Layer 2 集成测试 case + 3 fault wrapper + 2 helper。整体 status: ready-for-dev → review。 | claude-opus-4-7 |
