# Story 20.6: POST /chest/open 事务 + idempotencyKey + 加权抽取

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 我可以开启已解锁的宝箱（消耗 1000 步数）获得一个加权抽取的装扮道具（节点 7 阶段仅展示不入仓），并且网络抖动时同 idempotencyKey 重试始终安全（不重复扣步数、不重复出箱）,
so that 我能体验"开宝箱"核心玩法（Epic 20 节点 7 业务最重的一条 story —— 落地"扣步数 + 加权抽奖 + 写日志 + 刷新下一轮 + 持久化幂等"在同一个 MySQL 事务里原子完成）。

## 故事定位（Epic 20 第六条 = 节点 7 业务最重 / 上承 20.1 ~ 20.5 / 下启 20.7 ~ 20.9 + Epic 21 / 22）

- **Epic 20 进度**：20.1（接口契约 r1~r15 锚定，**done**）→ 20.2（cosmetic_items migration，**done**）→ 20.3（cosmetic_items seed ≥15 行，**done**）→ 20.4（chest_open_logs migration + ChestOpenLog struct，**done**）→ 20.5（GET /chest/current，**done**）→ **20.6（本 story，POST /chest/open 事务 + idempotencyKey + 加权抽取 + §5.16 chest_open_idempotency_records migration 落地）** → 20.7（dev /dev/force-unlock-chest）→ 20.8（dev /dev/grant-cosmetic-batch）→ 20.9（Layer 2 集成测试 - 开箱事务全流程）。

- **本 story 是 Epic 20 最重的一条 server 实装**（按 epics.md §Epic 20 钦定 / 与 20.1 r15 review 锁定的契约对应）：
  - 单一 MySQL 事务覆盖 8 个子步骤（V1 §7.2.5a~5l）：幂等预声明 INSERT → FOR UPDATE chest → 动态判定 unlockable → FOR UPDATE step_account → 扣 1000 步 + 乐观锁 version+1 → 加权抽取 cosmetic_items（按 drop_weight）→ 写 chest_open_logs（reward_user_cosmetic_item_id=0 占位）→ DELETE 旧 chest + INSERT 新 chest（unlock_at=now+10min）→ 序列化 response_json → UPDATE idempotency.status='success' + response_json
  - **handler 内层 rate_limit**（V1 §7.2.5 r10/r11 决策）：本接口在路由层显式 opt-out 全局 RateLimit middleware；rate_limit 检查在 handler 内层，**置于幂等命中预检之后**，仅命中 committed success replay 时免配额
  - **committed success replay 短路**（V1 §7.2.3 r11）：handler 入口先做 autocommit SELECT idempotency 行，命中 `status='success'` → 反序列化 response_json + **同源同时刻**重算 `nextChest.status` 和 `nextChest.remainingSeconds` + 填本次 requestId → 直接返回（跳过步骤 4 rate_limit + 跳过步骤 5 业务事务）
  - **加权抽取**（V1 §7.2.5g + 关键约束「加权抽奖语义」）：按 `cosmetic_items.drop_weight` 加权抽取 1 条（仅 `is_enabled=1` 参与）；random 源用 `io.Reader` interface 注入，便于单测断言权重分布

- **§5.16 chest_open_idempotency_records migration scope 决策（20.1 r5 follow-up 钦定）**：
  - **选项 A（本 story 采纳）**：在 20.6 内一并落地 migration（`server/migrations/0014_init_chest_open_idempotency_records.up.sql` + `.down.sql`）+ `IdempotencyRecord` GORM struct + `IdempotencyRepo` interface + impl + 单测；migration 跟 service / handler 实装在**同一 PR / 同一 commit**
  - **选项 B（不采纳）**：新起 Story 20.10 单独 owner 该 migration
  - **决策理由（紧耦合 = A 优于 B）**：
    1. **集成测试不可分**：20.6 集成测试（dockertest）必须有 idempotency 表才能跑（步骤 5a INSERT 是事务首条语句，缺表整个事务失败）；如果 migration 独立 story，20.6 集成测试要等 20.10 done 才能跑，story 间不可独立 deliver
    2. **AC 边界天然合体**：epics.md §20.6 AC（行 2899-2930）已经隐含"集成测试覆盖事务全流程"，含 idempotency 表写入；20.1 r5/r6/r7/r9/r10/r11 review 锁定的 schema 是契约层（V1 §7.2 + DB §5.16）已完成，本 story migration 是**纯落地**（无 schema 决策风险），与 service / handler 实装同 story 不引入额外契约风险
    3. **YAGNI vs 必要拆分**：拆 story 通常因"独立交付节奏 / 不同 owner"；本场景 migration 没有独立 deliver 价值（节点 7 阶段表只服务 POST /chest/open，无其他消费者），拆分纯增加流水线开销
    4. **历史先例**：Story 20.2（cosmetic_items migration）+ 20.3（seed）+ 20.4（chest_open_logs migration）已经把"DDL migration + GORM struct"作为独立 story；本 idempotency 表虽然是新增 schema，但属于 20.6 业务事务必备的 schema 根基 —— 与 20.4 同样定位为"业务 owner 自带 migration"模式更一致（20.4 也是 chest_open_logs 服务 20.6 开箱事务）
  - **本 story migration 边界**（在 AC 段会进一步钦定）：仅 CREATE TABLE 与 `down.sql` DROP；**不**含 INSERT 数据；**不**含其他表改动；DDL 1:1 匹配数据库设计 §5.16 落地的 `status ENUM('pending', 'success')` 二态机 schema + `UNIQUE (user_id, idempotency_key)` + `idx_status_created_at` 辅助索引

- **下游 Story 依赖关系**：
  - **20.7 dev /dev/force-unlock-chest**：UPDATE user_chests SET unlock_at=now WHERE user_id=?；demo 时用于跳过 10min 倒计时；20.7 自身**不**调本 service，但其集成测试会先 force-unlock 再调本 service 验证开箱流程（依赖本 story 落地的 OpenChest 端到端可用）
  - **20.8 dev /dev/grant-cosmetic-batch**：节点 8 才真正写库；节点 7 阶段仅路由 + handler 框架 + 单测 mock
  - **20.9 Layer 2 集成测试 - 开箱事务全流程**：本 story 已含 happy + 单测 ≥7 + 集成 ≥1（5a~5l 完整事务）；20.9 进一步扩展回滚 / 并发 / 边界 / 幂等深度场景（详见 epics.md §20.9 行 2970-2994）；本 story 与 20.9 协议是"20.6 落地核心事务能力 + 单测覆盖代码分支，20.9 扩展边界 / 并发 / 抽奖分布等深度集成测试"
  - **iOS Epic 21**（21.3 开箱按钮 + idempotencyKey 生成 / 21.4 奖励弹窗 / 21.5 开箱前主动同步步数）：**强依赖**本 story 服务端可用；Epic 22 跨端 E2E 验证全链路

- **20.1 r1~r15 review 锁定的契约要点（本 story 必须严格遵循，禁止偏离）**：
  - **r5**：DB 持久化幂等替代 Redis（避 Redis 非事务写回失败导致重复出箱风险）
  - **r6**：幂等预声明 INSERT 在业务事务**首条语句**（同事务原子写消除 pending 卡死悖论）
  - **r7**：schema 简化为 `('pending', 'success')` 二态机（移除 r6 保留的 best-effort failed upsert race）
  - **r9**：`response_json` 缓存**不**包含 `nextChest.status` / `nextChest.remainingSeconds`（time-derived 字段；cached replay 路径必须同源同时刻重算）
  - **r10**：rate_limit 从全局 middleware 挪到 handler 内层；置于幂等命中预检之后；committed success replay 免配额
  - **r11**：`response_json` 缓存补充不含顶层 `requestId`（每次请求独立 trace ID，重试请求填本次）；MVCC 下 pending 不可见 → 步骤 3 仅检查 committed success；1008 错误码在本接口节点 7 阶段无可达触发路径（保留全局错误码定义但不在本接口错误码表中列出）；cached success 短路返回路径必须**同源同时刻**补算 `nextChest.status` 和 `nextChest.remainingSeconds` 两字段
  - **r15**：1008 行已从 §7.2 错误码表移除（dead path）

- **新增 4xxx 业务错误码**：本 story 不新增 codes.go 错误码（4001 / 4002 / 1002 / 1005 / 1009 / 3002 均已在 codes.go 注册）；**关键**：DB §5.16 + V1 §7.2 r11 钦定的 "1008 在本接口节点 7 不可达"路径下，**禁止** service 层翻译任何分支为 1008（即便步骤 5b 兜底分支读到 pending，按 V1 §7.2 错误码表 1009 行钦定走 1009 而非 1008，详见 V1 §7.2「1008 错误码 r11 退役决策」段）

## 范围红线（明确不做）

**本 story 只做**：

1. **§5.16 chest_open_idempotency_records migration**：
   - 新建 `server/migrations/0014_init_chest_open_idempotency_records.up.sql`（CREATE TABLE 1:1 匹配 DB §5.16 钦定 schema：`id` / `user_id` / `idempotency_key` / `status ENUM('pending','success')` / `response_json` / `created_at` / `updated_at` + UNIQUE `uk_user_id_key` + KEY `idx_status_created_at`）
   - 新建 `server/migrations/0014_init_chest_open_idempotency_records.down.sql`（DROP TABLE）
   - 新建 `server/internal/repo/mysql/chest_open_idempotency_record_repo.go`：GORM `IdempotencyRecord` struct + `IdempotencyRepo` interface + `idempotencyRepo` impl + `NewIdempotencyRepo` 构造函数 + 3 个方法：
     - `FindByUserIDAndKey(ctx, userID, idempotencyKey) (*IdempotencyRecord, error)`（autocommit / handler 入口预检；NotFound → 哨兵 `ErrIdempotencyRecordNotFound`；其他 DB 错透传）
     - `ClaimPending(ctx, userID, idempotencyKey) (affectedRows int64, err error)`（事务内首条语句；`INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`；返 affected_rows：1 = 新行 / 0 = 行已存在）
     - `MarkSuccess(ctx, userID, idempotencyKey, responseJSON []byte) error`（事务内最终化 UPDATE；rows_affected=0 → 哨兵 `ErrIdempotencyRecordNotFound`；其他错透传）
   - `errors.go` 追加 `ErrIdempotencyRecordNotFound` 哨兵
   - 单测 ≥6 case（参考 chest_repo_test.go / room_repo_test.go 模式，sqlmock 驱动）

2. **`server/internal/repo/mysql/chest_repo.go` 扩展**：
   - 新增 `FindByUserIDForUpdate(ctx, userID) (*UserChest, error)` 方法（事务内 `SELECT ... FOR UPDATE`；NotFound → `ErrChestNotFound`；走 `gorm.io/gorm/clause.Locking{Strength: "UPDATE"}` —— 与 room_repo.go FindByIDForUpdate 同模式）
   - 新增 `Delete(ctx, id) error` 方法（事务内 `DELETE FROM user_chests WHERE id = ?`；用于步骤 5i 刷新下一轮 chest）
   - 注意：`Create` 已存在（4.6 落地），本 story 复用作为步骤 5i 第二步 INSERT 新 chest 的入口
   - 单测追加 ≥2 case（`FindByUserIDForUpdate` happy + NotFound / `Delete` happy）

3. **`server/internal/repo/mysql/step_account_repo.go` 扩展**：
   - 新增 `FindByUserIDForUpdate(ctx, userID) (*StepAccount, error)` 方法（事务内 `SELECT ... FOR UPDATE`；NotFound → `ErrStepAccountNotFound`）
   - 新增 `Spend(ctx, userID, amount uint64, expectedVersion uint64) error` 方法（事务内 `UPDATE user_step_accounts SET available_steps = available_steps - ?, consumed_steps = consumed_steps + ?, version = version + 1 WHERE user_id = ? AND version = ?`；rows_affected=0 → `ErrStepAccountVersionMismatch`；`gorm.Expr` 走 SQL 层减法 race-free）
   - 注意：现有 `UpdateBalance` 方法是 7.3 落地（仅加 total_steps + available_steps，**不**改 consumed_steps）；本 story 必须新增 `Spend` 方法而非复用 `UpdateBalance`（语义完全不同：UpdateBalance 是"sync 入账"加 total + available；Spend 是"开箱消费"减 available + 加 consumed，total 不变 —— 详见 step_account_repo.go 既有注释 §UpdateBalance「关键 3」）
   - 单测追加 ≥3 case（`FindByUserIDForUpdate` happy + NotFound / `Spend` happy + 乐观锁冲突）

4. **`server/internal/repo/mysql/cosmetic_item_repo.go` 扩展（CosmeticItemRepo 首次落地）**：
   - 新建 `CosmeticItemRepo` interface + `cosmeticItemRepo` impl + `NewCosmeticItemRepo` 构造（cosmetic_item_repo.go 现有只是 GORM struct + TableName，无 interface / impl —— 见 20.2 落地的注释 §"YAGNI；20.6 落地加权抽取方法"）
   - 唯一方法：`ListEnabledForWeightedPick(ctx) ([]CosmeticItem, error)`（事务内或事务外都可调；返 `is_enabled=1` 的所有行，含 `id` / `rarity` / `drop_weight` / `name` / `slot` / `asset_url` / `icon_url` —— V1 §7.2.5g 钦定的 SELECT 字段集；DB 错透传）
   - 单测 ≥2 case（happy 返多行 / 空集合 —— 后者由 service 翻译为 1009 "seed 未执行" 数据完整性异常，V1 §7.2.5g 钦定）
   - **不**做 `Create` / `Update` / `Delete`（YAGNI；那是 20.2 / 20.3 已落地的 seed 路径，本 story 仅消费查询）
   - **不**做 `FindByID` 单查（YAGNI；本 story 加权抽取一次性拉全表足够 —— enabled 集合在节点 7 阶段约 15-20 行，单查 N+1 反而劣化）

5. **`server/internal/repo/mysql/chest_open_log_repo.go` 扩展（ChestOpenLogRepo 首次落地）**：
   - 新建 `ChestOpenLogRepo` interface + `chestOpenLogRepo` impl + `NewChestOpenLogRepo` 构造（chest_open_log_repo.go 现有只是 GORM struct + TableName，无 interface / impl —— 见 20.4 落地的注释 §"YAGNI；20.6 落地 Create 方法"）
   - 唯一方法：`Create(ctx, log *ChestOpenLog) error`（事务内 INSERT 一行；DB 错透传）
   - 单测 ≥1 case（happy）
   - **不**做 `FindByUserID` / 历史查询（YAGNI；那是未来运营 epic 才需要）

6. **`server/internal/pkg/random/` 包（新建）**：
   - 新建 `server/internal/pkg/random/weighted.go`：导出 `WeightedPicker` interface + `cryptoWeightedPicker` impl + `NewCryptoWeightedPicker(reader io.Reader) WeightedPicker` 构造（生产用 `crypto/rand.Reader` 注入；单测可注入 `mathrand.New(...)` 走确定性 seed 验证分布）
   - 唯一方法：`Pick(items []WeightedItem) (selectedIndex int, err error)`：累加 `drop_weight` 计算 `total`，从 reader 取 8 字节 → `binary.BigEndian.Uint64` → `mod total` → 二分查找区间 → 返 index；`len(items)==0` → `(0, ErrEmptyItems)` 哨兵；`total==0`（全部 drop_weight=0，不应发生但兜底）→ `(0, ErrZeroTotalWeight)`
   - 抽象目的：service 层注入 picker 实例，单测确定性验证分布 + crypto/rand 替代 math/rand 全局源（math/rand 默认 seed=1 在并发安全后仍是可预测序列，开箱抽奖不应可预测 —— 与 V1 §7.2.5g + epics.md §20.9 行 4403 "合成抽奖加权随机用 math/rand 全局源（与 Story 20.6 同问题）" 警示对齐）
   - 单测 ≥3 case（happy 单元素 / happy 多元素分布断言用确定性 seed / 空集合错误）

7. **新建 `server/internal/service/chest_open_service.go`**（**不**塞到 chest_service.go！见 §"为什么独立文件"段）：
   - 复用既有 `ChestService` interface（chest_service.go 已建），**追加** `OpenChest(ctx, in OpenChestInput) (*OpenChestOutput, error)` 方法签名
   - 复用既有 `chestServiceImpl` struct，**扩展**字段（用 `NewChestServiceForOpen` 或修改既有 `NewChestService` 接受额外依赖；推荐路径：扩展既有 `NewChestService` 签名以容纳所有依赖，避免双构造函数）
   - 实装 `OpenChest`：8 步事务（详见 AC2 完整代码）
   - **不**新建 `ChestOpenService` 独立 interface（YAGNI；handler 已注入 `ChestService` interface，本 story 只是给该 interface 加一个方法）
   - 单测 ≥7 case（见 AC4）

8. **`server/internal/app/http/handler/chest_handler.go` 扩展**：
   - 复用既有 `ChestHandler` struct，**追加** `Open(c *gin.Context)` 方法 + 内部 helper（请求体解析 + idempotencyKey 校验 + handler 内层 rate_limit 检查 + 调 service）
   - 复用既有 `NewChestHandler` 构造（添加 ChestService 已含 Open 方法）
   - 添加 `openChestRequestDTO` / `openChestResponseDTO` helper
   - 单测 ≥6 case（见 AC5）

9. **`server/internal/app/http/middleware/rate_limit.go` 公开复用接口**（轻量改动）：
   - 现状：`RateLimit(cfg, extractor) gin.HandlerFunc` 是 middleware 工厂；本 story 需要在 handler 内调用同款限频检查（V1 §7.2.5 r10 决策：rate_limit 挪到 handler 内层）
   - 实装路径：在 `middleware/rate_limit.go` 末尾 export 一个 `CheckRateLimitByUserID(ctx context.Context, cfg config.RateLimitConfig, userID uint64) error` 函数（返 `nil` = 通过 / 返 `*AppError(1005, ...)` = 超限），便于 handler 在事务前显式调用；middleware 自身的 `RateLimit` 工厂保持不变（其他接口继续走 middleware 链）
   - **关键**：该 export 函数是节点 7 阶段 chest_open 接口**专用**的 ad-hoc 调用入口；不破坏 §4.5 middleware 链架构（仅 chest_open 这一条 r10 钦定的"opt-out"路径用，详见 V1 §7.2.5 r10 决策段 + 关键约束「rate_limit 位置 r10 调整 + r11 修订」段）
   - **不**在 router.go 给 chest_open 路由挂 RateLimit middleware（与其他 authedGroup 路由的关键区别 —— 见 AC3 钦定）
   - 单测 ≥3 case（CheckRateLimitByUserID happy / 超限 / userID=0 兜底）

10. **`server/internal/app/bootstrap/router.go` 修改**：
    - **关键架构变更**：chest_open 路由**不**挂在 `authedGroup`（authedGroup 全局挂了 `RateLimit by userID` middleware；本接口需要在 handler 内层做 rate_limit，必须从该路由组移出）
    - 新建独立子组 `chestOpenGroup := api.Group("", middleware.Auth(deps.Signer))`（仅挂 Auth，**不**挂 RateLimit；rate_limit 在 handler 内层 r10 路径走）
    - 注册 `chestOpenGroup.POST("/chest/open", chestHandler.Open)`
    - 注意：GET /chest/current 仍在 authedGroup（与其他 GET 共享中间件）；只有 POST /chest/open 从 authedGroup 移出
    - wire 新增的 5 个 repo / picker / 扩 deps（idempotencyRepo / cosmeticItemRepo / chestOpenLogRepo / weightedPicker；复用 chestRepo / stepAccountRepo / txMgr / RateLimitCfg / Signer），扩 `NewChestService` 调用签名

11. **集成测试**：
    - 新建 `server/internal/service/chest_open_service_integration_test.go`：≥1 dockertest case（HappyPath_FullFlow：创建 user + 1500 步 + force-unlock chest → 调 OpenChest → 验证 DB user_step_accounts.available_steps=500 + consumed_steps=1000 + chest_open_logs 多 1 行 + 旧 chest 删除 + 新 chest 创建 + idempotency 行 status=success + response_json 完整）
    - 同文件追加 ≥1 case：`HappyPath_IdempotencyReplay`（第一次 open success → 第二次同 idempotencyKey → 短路返回 cached + DB 无副作用）

12. **本 story 文件本身 + sprint-status.yaml 状态流转**

**本 story 不做**：

- **不**创建 `user_cosmetic_items` 实例（V1 §7.2.4h + 数据库设计 §8 节点 7 vs 节点 8 注解：节点 7 阶段 `reward.userCosmeticItemId` 固定字符串 `"0"`；节点 8 Story 23.5 修改本事务才补该步骤）
- **不**新增 `user_cosmetic_items` 表的 migration（节点 8 Epic 23 Story 23.2 owner）
- **不**修改 `docs/宠物互动App_*.md` 任一字（契约**输入**侧 / 20.1 已 finalize）
- **不**修改 V1 §7.2 接口契约任一字（r1~r15 + 20.1 已冻结）
- **不**修改 DB §5.16 schema（20.1 r11 + r15 已锁定 + 本 story migration 1:1 落地）
- **不**修改 codes.go 错误码（4001 / 4002 / 1002 / 1005 / 1009 / 3002 已注册；**禁止**新增 4xxx 或修改既有错误码）
- **不**给 chest_open 路由挂全局 RateLimit middleware（V1 §7.2.5 r10 钦定该路由 opt-out，rate_limit 由 handler 内层显式做）
- **不**触发 1008 错误码（V1 §7.2 r11/r15 锁定 1008 在本接口不可达；步骤 5b 兜底分支按 1009 走）
- **不**写 Redis 任何 key（V1 §7.2 r5 锁定移除 Redis 在本接口的角色；rate_limit middleware 仍可用 Redis，但本路由 opt-out）
- **不**用 `math/rand` 全局源做加权抽奖（V1 §7.2 + epics.md §20.9 行 4403 警示；本 story 引入 `internal/pkg/random` 包，`crypto/rand.Reader` 注入）
- **不**写 e2e 跨端测试（Epic 22 Story 22.1 才做）
- **不**预实装 20.7 dev /dev/force-unlock-chest（即便顺手把 ChestService.ForceUnlock 加到 interface 也禁止 —— YAGNI，20.7 owner）
- **不**预实装 20.8 dev /dev/grant-cosmetic-batch（YAGNI，20.8 owner）
- **不**实装 20.9 Layer 2 集成测试的全部场景（本 story 集成测试覆盖 happy + idempotency replay 两个 path，20.9 owner 回滚 / 并发 / 抽奖分布等深度场景）
- **不**修改 `_bmad-output/implementation-artifacts/decisions/*.md`（无新 ADR 决策；本 story 严格按 20.1 已 finalize 的契约落地）
- **不**修改 `cmd/server/main.go`（无新 Deps 字段；本 story 仅复用 GormDB / TxMgr / Signer / RateLimitCfg / RedisClient）
- **不**改 `local.yaml` / 任一 `*.yaml`（无新配置项）
- **不**写性能压测（节点 7 阶段不做 NFR16 性能 baseline；详见 V1 §7.2 元信息 60/min/userID 兜底足够）

**任何超出上述清单的改动 → HALT 并问设计**。

## Acceptance Criteria

### AC1 — §5.16 migration + IdempotencyRepo 落地

**Migration**（`server/migrations/0014_init_chest_open_idempotency_records.up.sql`）：

```sql
-- 对齐 docs/宠物互动App_数据库设计.md §5.16
-- chest_open_idempotency_records 表：开箱接口幂等记录（20.1 r5/r6/r7/r11 锁定 DB 持久化方案，
--   预声明 + 业务写入 + 最终化全部同事务原子提交）
-- 详见 V1接口设计 §7.2 服务端逻辑步骤 3 / 5a / 5b / 5k / 7 + 关键约束「事务边界」+
--   「r7 移除 best-effort failed upsert 决策」+「MVCC 下 pending 不可见 r11 决策」+
--   「1008 错误码 r11 退役决策」段。
--
-- **本 migration 由 Story 20.6 落地**（20.1 r5 follow-up 钦定的 owner 决策）：
--   - 选项 A 采纳：与 service / handler 实装在同 story 一并落地，紧耦合 = AC 边界天然合体
--   - 集成测试不可分：本 story dockertest case 必须有此表才能跑（事务首条语句 INSERT）
--
-- 字段（与 §5.16 钦定 1:1 对齐）：
--   - id: BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - user_id: BIGINT UNSIGNED NOT NULL（归属用户 id，语义上 ref users.id，**不**建 FK）
--   - idempotency_key: VARCHAR(128) NOT NULL（client 传入幂等键；V1 §7.2 钦定 [A-Za-z0-9_:-] + length 1-128）
--   - status: ENUM('pending', 'success') NOT NULL DEFAULT 'pending'
--       **二态机**（r7 锁定，从 r6 三态机简化）；**无** 'failed' 状态
--       pending: 业务事务持锁执行中（InnoDB MVCC 决定对其他事务的 autocommit SELECT 不可见）
--       success: 业务事务已 commit，response_json 已落盘
--   - response_json: JSON NULL
--       status='success' 时存 V1 §7.2 钦定缓存内容（{code, message, data: {reward, stepAccount, nextChest.{id, unlockAt, openCostSteps}}}）
--       **不**含 nextChest.status / nextChest.remainingSeconds（time-derived 字段；cached replay 时同源同时刻重算）
--       **不**含顶层 requestId（每次请求独立 trace ID；重试请求填本次）
--       status='pending' 时为 NULL
--   - created_at / updated_at: 标准毫秒时间戳；updated_at 在 status 推进时自动更新
--
-- 索引（§5.16 钦定）：
--   - UNIQUE KEY uk_user_id_key (user_id, idempotency_key): 兼任原子声明依据 + 并发阻塞排队依据
--       V1 §7.2.5a 用 INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id) 借此做 single-statement 原子 claim
--       InnoDB unique-key X-lock 让同 key 并发请求 serialize 排队（首事务结束前其他事务 INSERT 阻塞）
--   - KEY idx_status_created_at (status, created_at): 辅助索引，运维清理任务按 status + created_at 范围扫描
--       （MVP 阶段无需主动清理；future 容量增长时按此索引清理 N 天前的 success 记录）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据（运行时表，无 seed 阶段）；
--   不含其他表改动；不含 FK 约束（与本设计其他表一致）。
CREATE TABLE chest_open_idempotency_records (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    status ENUM('pending', 'success') NOT NULL DEFAULT 'pending',
    response_json JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_id_key (user_id, idempotency_key),
    KEY idx_status_created_at (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

`down.sql`：

```sql
DROP TABLE IF EXISTS chest_open_idempotency_records;
```

**Repo**（`server/internal/repo/mysql/chest_open_idempotency_record_repo.go`）：

```go
package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// IdempotencyRecord 是 chest_open_idempotency_records 表的 GORM domain struct。
// 字段 1:1 匹配 0014 migration + 数据库设计 §5.16。
type IdempotencyRecord struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         uint64    `gorm:"column:user_id;not null"`
	IdempotencyKey string    `gorm:"column:idempotency_key;not null;size:128"`
	Status         string    `gorm:"column:status;not null;size:16"` // ENUM('pending', 'success') —— GORM 用 string 映射
	ResponseJSON   []byte    `gorm:"column:response_json"`            // JSON NULL；用 []byte 让 GORM 走 raw JSON
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "chest_open_idempotency_records"。
func (IdempotencyRecord) TableName() string { return "chest_open_idempotency_records" }

// 二态机常量（V1 §7.2 r7 + DB §5.16 r11 钦定；service 层用做字段比较 / claim 写值）。
const (
	IdempotencyStatusPending = "pending"
	IdempotencyStatusSuccess = "success"
)

// IdempotencyRepo 是 chest_open_idempotency_records 表的访问接口（Story 20.6 引入）。
//
// 设计原则（V1 §7.2 r5/r6/r7/r11 + DB §5.16）：
//   - **FindByUserIDAndKey**: handler 入口 autocommit 预检；仅命中 status='success' 时短路（MVCC 决定 pending 不可见）
//   - **ClaimPending**: 事务内**首条**语句；借 UNIQUE(user_id, idempotency_key) 做 single-statement 原子 claim；
//                       返 affected_rows：1 = 新行 / 0 = 行已存在（首事务已 commit success；本事务走步骤 5b 短路）
//   - **MarkSuccess**: 事务内最终化（步骤 5k）；UPDATE status='success' + response_json
//
// **不**实装 MarkFailed（r7 决策：彻底移除 best-effort failed upsert，schema 已无 'failed' 状态）。
// **不**实装 Delete（运维清理任务由 future epic owner，本 story 仅落地业务路径所需方法）。
type IdempotencyRepo interface {
	// FindByUserIDAndKey 查 (user_id, idempotency_key) 行（走 uk_user_id_key 唯一索引）。
	//
	// **autocommit 调用**（V1 §7.2.3 钦定）：传入的 ctx **不**应带 tx 句柄 —— 这是 handler 入口
	// 在业务事务之前的预检。
	//
	// NotFound → ErrIdempotencyRecordNotFound 哨兵；其他 DB error 透传给 service。
	//
	// **MVCC 不可见 r11**：如果首事务正在 INSERT pending 但未 commit，本 autocommit SELECT
	// 在 InnoDB REPEATABLE READ 隔离级别下**看不到** pending 行 → 返 NotFound；这是
	// 协议层硬约束，不要试图通过降低隔离级别"修复"（详见 V1 §7.2 r11 决策段）。
	FindByUserIDAndKey(ctx context.Context, userID uint64, idempotencyKey string) (*IdempotencyRecord, error)

	// ClaimPending 事务内首条语句：原子声明 idempotency 行。
	//
	// SQL: INSERT INTO chest_open_idempotency_records (user_id, idempotency_key, status, ...)
	//      VALUES (?, ?, 'pending', NULL, NOW(3), NOW(3))
	//      ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)
	//
	// **必须**通过事务 ctx 调用（service 层走 txMgr.WithTx）—— 否则 ON DUPLICATE 的语义错乱。
	//
	// 返回值：
	//   - affected_rows = 1: 新行 INSERT，本请求是同 key 首次到达 **或** 首事务已 rollback 后到达
	//   - affected_rows = 0: 行已存在，且首事务已 commit；service 走步骤 5b 短路
	//
	// **InnoDB unique-key X-lock**：同 key 并发请求只有一个能拿锁；其他事务在 INSERT 语句上
	// 阻塞等首事务释放锁后再继续。这是协议层并发兜底（详见 V1 §7.2 关键约束「事务边界」段）。
	ClaimPending(ctx context.Context, userID uint64, idempotencyKey string) (int64, error)

	// MarkSuccess 事务内最终化（V1 §7.2.5k 钦定）：UPDATE status='success' + response_json。
	//
	// 必须在业务事务内调用（与业务表 INSERT / UPDATE / DELETE 同事务原子 commit）。
	//
	// 返回值：
	//   - nil: UPDATE 成功（rows_affected ≥ 1）
	//   - ErrIdempotencyRecordNotFound: rows_affected = 0（理论不该发生 —— 同事务前面 ClaimPending 必已 INSERT；
	//                                  实际触发说明上游调用顺序错乱，应作为 1009 透传）
	//   - 其他 DB error 透传
	MarkSuccess(ctx context.Context, userID uint64, idempotencyKey string, responseJSON []byte) error
}

// idempotencyRepo 是 IdempotencyRepo 的默认实装。
type idempotencyRepo struct {
	db *gorm.DB
}

// NewIdempotencyRepo 构造 IdempotencyRepo。
func NewIdempotencyRepo(db *gorm.DB) IdempotencyRepo {
	return &idempotencyRepo{db: db}
}

// FindByUserIDAndKey 实装：autocommit 单查；走 uk_user_id_key UNIQUE 索引。
func (r *idempotencyRepo) FindByUserIDAndKey(ctx context.Context, userID uint64, idempotencyKey string) (*IdempotencyRecord, error) {
	db := tx.FromContext(ctx, r.db)
	var rec IdempotencyRecord
	err := db.WithContext(ctx).
		Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).
		First(&rec).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIdempotencyRecordNotFound
		}
		return nil, err
	}
	return &rec, nil
}

// ClaimPending 实装：INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)。
//
// **关键 1**：本 SQL 走 raw `db.Exec` 而非 GORM Create / Upsert —— GORM 高级 API 默认行为
// 在 ON DUPLICATE 上有版本差异（v1.21+ 的 OnConflict 在某些版本生成不同 SQL）；用 raw SQL
// 保证 cross-version 行为一致。
//
// **关键 2**：affected_rows 用 `result.RowsAffected` 取（GORM 的 Raw / Exec 都暴露此字段）。
// affected_rows = 1 = INSERT 生效（新行）；affected_rows = 0 = ON DUPLICATE 触发但 update 列
// 是 no-op（id = LAST_INSERT_ID(id) 不改任何字段，故 update path rows_affected = 0）。
// **注**：MySQL ON DUPLICATE 在 update 路径下若有真实字段变更，affected_rows = 2；本 SQL 用
// `id = LAST_INSERT_ID(id)` 是惯用 no-op upsert pattern，affected_rows 只能是 0 或 1。
func (r *idempotencyRepo) ClaimPending(ctx context.Context, userID uint64, idempotencyKey string) (int64, error) {
	db := tx.FromContext(ctx, r.db)
	sql := "INSERT INTO chest_open_idempotency_records (user_id, idempotency_key, status, response_json, created_at, updated_at) " +
		"VALUES (?, ?, ?, NULL, NOW(3), NOW(3)) " +
		"ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)"
	result := db.WithContext(ctx).Exec(sql, userID, idempotencyKey, IdempotencyStatusPending)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// MarkSuccess 实装：UPDATE status='success' + response_json + updated_at。
func (r *idempotencyRepo) MarkSuccess(ctx context.Context, userID uint64, idempotencyKey string, responseJSON []byte) error {
	db := tx.FromContext(ctx, r.db)
	res := db.WithContext(ctx).
		Model(&IdempotencyRecord{}).
		Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).
		Updates(map[string]interface{}{
			"status":        IdempotencyStatusSuccess,
			"response_json": responseJSON,
			// updated_at 由 DDL `ON UPDATE CURRENT_TIMESTAMP(3)` 自动维护，**不**手动 set
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrIdempotencyRecordNotFound
	}
	return nil
}
```

**errors.go 追加**：

```go
// ErrIdempotencyRecordNotFound: IdempotencyRepo.FindByUserIDAndKey 查不到行（合法 case —— 首次到达）
// / MarkSuccess rows_affected=0（理论不应发生 —— 见 MarkSuccess 注释）。service 层用 errors.Is 区分语义。
ErrIdempotencyRecordNotFound = errors.New("mysql: idempotency record not found")
```

**关键约束**：

- migration `up.sql` schema 必须 1:1 匹配 DB §5.16 钦定（status ENUM 二态机 / response_json JSON NULL / UNIQUE uk_user_id_key / KEY idx_status_created_at）
- `down.sql` 必须是 `DROP TABLE IF EXISTS`（与既有 0011 / 0013 同模式）
- `IdempotencyRecord` struct **不**用 `gorm:"primaryKey;autoIncrement"` 在 user_id 上（PK 是 id 自增；user_id 是 UNIQUE 索引一部分，**不是** PK）
- `Status` 字段用 `string` 映射 ENUM（与 GORM 兼容；service 层用 `IdempotencyStatusSuccess` 常量比较）
- `ResponseJSON` 字段用 `[]byte` 映射 JSON NULL（与 GORM 兼容 + service 层 `json.Marshal` / `json.Unmarshal` 直接处理）
- repo 三个方法都用 `tx.FromContext(ctx, r.db)` 取 db handle（与既有 repo 同模式 —— ADR-0007）
- `ClaimPending` 用 raw SQL `db.Exec` 而非 GORM Create/Upsert（cross-version 行为一致）
- `FindByUserIDAndKey` autocommit 路径 + tx 路径**共用**该方法实装（`tx.FromContext` 兜底兼容）；service 层在事务外 / 事务内都可调，只是不同语义场景使用（事务外是 handler 入口预检；事务内是步骤 5b 短路读）
- repo 单测 ≥6 case（参考 chest_repo_test.go sqlmock 模式，详见 AC4 单测段）：
  - `TestIdempotencyRepo_FindByUserIDAndKey_HappyPath`
  - `TestIdempotencyRepo_FindByUserIDAndKey_NotFound_ReturnsErrIdempotencyRecordNotFound`
  - `TestIdempotencyRepo_FindByUserIDAndKey_DBError_Propagates`
  - `TestIdempotencyRepo_ClaimPending_NewRow_AffectedRows1`
  - `TestIdempotencyRepo_ClaimPending_ExistingRow_AffectedRows0`
  - `TestIdempotencyRepo_MarkSuccess_HappyPath`

### AC2 — `chest_open_service.go`（OpenChest 8 步事务实装）

新建 `server/internal/service/chest_open_service.go`，**追加** `OpenChest` 方法到既有 `ChestService` interface + `chestServiceImpl`。

**关键路径骨架**（完整实装，禁止简化）：

```go
package service

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strconv"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/random"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
)

// 业务常量（V1 §7.2.5 + DB §5.6 钦定）。
const (
	// chestOpenCostSteps: 开箱固定消耗步数（V1 §7.2.5 + epics.md §FR13 钦定；节点 7 阶段固定 1000）
	chestOpenCostSteps = 1000

	// chestRefreshNextDelay: 下一轮 chest unlock_at 偏移（V1 §7.2.5i + epics.md §FR11 钦定；
	// 节点 7 阶段固定 10 分钟）
	chestRefreshNextDelay = 10 * time.Minute

	// idempotencyKeyMinLength / idempotencyKeyMaxLength: V1 §7.2 + DB §5.16 钦定
	// 1 ≤ length ≤ 128 + 字符集 [A-Za-z0-9_:-]（字符集校验在 handler 层做 regex；本 service
	// 仅做 length 兜底防御 —— 与 handler 双重校验，避免 handler 误漏校直接进 service）
	idempotencyKeyMinLength = 1
	idempotencyKeyMaxLength = 128

	// chestStatusOpeningInTransaction: 数据库设计 §6.7 钦定（事务中间态，不下发 client）
	// 节点 7 阶段本 service 用本常量做"是否处于开箱事务中"的内部判定（如果未来需要）；
	// 本 story 阶段实际不写入 DB（V1 §7.2.5 没有显式 UPDATE chest.status 到 3 的步骤），
	// 常量保留作 future 兼容。
	chestStatusOpeningInTransaction = 3
)

// OpenChestInput 是 ChestService.OpenChest 输入 DTO（handler → service 转换）。
type OpenChestInput struct {
	UserID         uint64 // auth 中间件注入；handler 兜底校验非 0
	IdempotencyKey string // V1 §7.2 钦定 1-128 字符集 [A-Za-z0-9_:-]；handler 已 regex 校验
	RequestID      string // 顶层 trace ID（handler 从 c.Get("request_id") 取）；用于响应填充，**不**写入 response_json 缓存（V1 §7.2 r7 锁定）
}

// OpenChestOutput 是 ChestService.OpenChest 输出 DTO（handler 转译为 V1 §7.2 wire DTO）。
//
// 字段构造规则（V1 §7.2.5j 钦定）：
//   - Reward 三段嵌套 + StepAccount + NextChest 由 service 层填充
//   - **NextChest.Status / RemainingSeconds 由 handler 层补算**（time-derived，service 透传 UnlockAt 即可）
//   - **NextChest.Status / RemainingSeconds 由 handler 层补算** 这条规则适用于"首次成功路径"返回，
//     与"committed success replay 路径" handler 层补算一致 —— 让 handler 成为 time-derived 字段
//     的单一计算点，避免 service 在两条路径上重复实装（首次成功 + replay）
type OpenChestOutput struct {
	Reward      ChestRewardBrief
	StepAccount StepAccountBrief
	NextChest   ChestBrief // 注意：Status / RemainingSeconds 由 handler 按 (UnlockAt > now ? 1 : 2) 与 max(0, ceil((UnlockAt-now)/1s)) 补算
}

// ChestRewardBrief: 开箱奖励三段嵌套之 reward 段。
type ChestRewardBrief struct {
	UserCosmeticItemID uint64 // **节点 7 阶段固定 0 占位**（V1 §7.2.4h + DB §5.7 注解；节点 8 Story 23.5 改）
	CosmeticItemID     uint64 // 真实 cosmetic_items.id
	Name               string // cosmetic_items.name
	Slot               int8   // cosmetic_items.slot
	Rarity             int8   // cosmetic_items.rarity
	AssetURL           string // cosmetic_items.asset_url
	IconURL            string // cosmetic_items.icon_url
}

// ChestService interface 扩展（追加 OpenChest 方法到既有 interface）：
//
// type ChestService interface {
//   GetCurrent(ctx context.Context, userID uint64) (*ChestBrief, error)  // 既有 20.5
//   OpenChest(ctx context.Context, in OpenChestInput) (*OpenChestOutput, error)  // 本 story 新增
// }

// chestServiceImpl struct 扩展（在既有 struct 上追加字段）：
//
// type chestServiceImpl struct {
//   chestRepo            mysql.ChestRepo            // 既有
//   txMgr                tx.Manager                 // 新增
//   idempotencyRepo      mysql.IdempotencyRepo      // 新增
//   stepAccountRepo      mysql.StepAccountRepo      // 新增
//   cosmeticItemRepo     mysql.CosmeticItemRepo     // 新增
//   chestOpenLogRepo     mysql.ChestOpenLogRepo     // 新增
//   weightedPicker       random.WeightedPicker      // 新增（注入 crypto/rand 或测试 seed）
//   nowFn                func() time.Time           // 新增（注入 time.Now().UTC 默认；单测可覆盖）
// }

// NewChestService 构造签名扩展（向后不兼容；router.go wire 一并扩展）：
//
// func NewChestService(
//   chestRepo mysql.ChestRepo,
//   txMgr tx.Manager,
//   idempotencyRepo mysql.IdempotencyRepo,
//   stepAccountRepo mysql.StepAccountRepo,
//   cosmeticItemRepo mysql.CosmeticItemRepo,
//   chestOpenLogRepo mysql.ChestOpenLogRepo,
//   weightedPicker random.WeightedPicker,
// ) ChestService

// OpenChest 实装（V1 §7.2.5 8 步事务 + handler 内层 rate_limit 不在本 service 做）。
//
// **本 service 不做 rate_limit 检查**（V1 §7.2.5.4 r10 钦定 rate_limit 在 handler 内层）；
// 本 service 入口只做：
//   1. 入参校验（idempotencyKey length 兜底；非业务必走，handler 已校验）
//   2. 步骤 3: 幂等命中预检（autocommit SELECT idempotency 行）→ 命中 success → 直接返 cached OpenChestOutput
//   3. 步骤 5: 业务事务（txMgr.WithTx fn）：5a 预声明 → 5b 短路 / 5c-l 全流程
//
// 步骤 4 (rate_limit) 由 handler 层做；本 service 不感知。
//
// 错误码翻译（service 层完成；handler 仅 c.Error + return）：
//   - chest NotFound (5c) → ErrChestNotFound (4001)
//   - chest 不可解锁 (5d) → ErrChestNotUnlocked (4002)
//   - step_account NotFound (5e) → ErrServiceBusy (1009) —— V1 §7.2 1009 行钦定（数据完整性异常）
//   - step_account.available_steps < 1000 (5e) → ErrInsufficientSteps (3002)
//   - 乐观锁 (5f rows_affected=0) → ErrServiceBusy (1009)
//   - cosmetic_items enabled 为空 (5g) → ErrServiceBusy (1009)
//   - 任何其他 DB 错 → ErrServiceBusy (1009)
//   - 步骤 5b 兜底分支读到 status='pending' → ErrServiceBusy (1009)（V1 §7.2 r11 钦定，**非** 1008）
func (s *chestServiceImpl) OpenChest(ctx context.Context, in OpenChestInput) (*OpenChestOutput, error) {
	// 入参校验（兜底；handler 已校验过 regex + length）
	if in.UserID == 0 {
		return nil, apperror.New(apperror.ErrServiceBusy, "user_id missing")
	}
	if len(in.IdempotencyKey) < idempotencyKeyMinLength || len(in.IdempotencyKey) > idempotencyKeyMaxLength {
		return nil, apperror.New(apperror.ErrInvalidParam, "idempotencyKey length out of range")
	}

	// 步骤 3: committed success 幂等命中预检（autocommit SELECT；MVCC 决定 pending 不可见）
	cached, err := s.idempotencyRepo.FindByUserIDAndKey(ctx, in.UserID, in.IdempotencyKey)
	if err != nil && !stderrors.Is(err, mysql.ErrIdempotencyRecordNotFound) {
		// DB 错（非 NotFound 哨兵）→ 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if cached != nil && cached.Status == mysql.IdempotencyStatusSuccess {
		// committed success replay → 反序列化 response_json + 返回（handler 层补 status / remainingSeconds / requestId）
		// **注**：V1 §7.2.3 钦定本路径**不**进业务事务 + **不**走步骤 4 rate_limit；本 service 直接返
		// reconstructed OpenChestOutput；handler 不需要重新做 rate_limit（路由层未挂 middleware；handler
		// 内层 rate_limit 在 handler 检查 cached 命中后跳过）
		return s.replayFromCachedResponse(cached.ResponseJSON)
	}

	// 未命中或 cached.Status='pending'（理论上不可观察到 pending；MVCC 让 autocommit 看不到首事务的 pending；
	// 但若 driver 异常读到 pending，此处按 1009 兜底）
	if cached != nil && cached.Status == mysql.IdempotencyStatusPending {
		// V1 §7.2 r11 锁定：节点 7 阶段本接口不应触发；保留兜底为 1009
		return nil, apperror.New(apperror.ErrServiceBusy, "idempotency record in pending state (unexpected)")
	}

	// 步骤 5: 业务事务
	var output *OpenChestOutput
	err = s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// 5a: 幂等预声明 INSERT（事务首条语句，借 UNIQUE 做 single-statement 原子 claim）
		affectedRows, err := s.idempotencyRepo.ClaimPending(txCtx, in.UserID, in.IdempotencyKey)
		if err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		if affectedRows == 0 {
			// 5b: 短路分支（行已存在，且首事务已 commit）
			// 在同一事务内 SELECT → status 必然 'success'（V1 §7.2.5b r11 锁定）
			rec, err := s.idempotencyRepo.FindByUserIDAndKey(txCtx, in.UserID, in.IdempotencyKey)
			if err != nil {
				return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
			}
			if rec.Status != mysql.IdempotencyStatusSuccess {
				// 理论不应观察到 pending（详见 V1 §7.2.5b 注解）；按 1009 兜底
				return apperror.New(apperror.ErrServiceBusy, "idempotency record in unexpected state during transaction")
			}
			out, err := s.replayFromCachedResponse(rec.ResponseJSON)
			if err != nil {
				return err
			}
			output = out
			return nil
		}

		// affectedRows = 1 → 走步骤 5c 业务全流程
		out, err := s.runOpenChestTx(txCtx, in)
		if err != nil {
			return err
		}
		output = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

// runOpenChestTx 步骤 5c~5l 业务全流程（事务内调用）。
func (s *chestServiceImpl) runOpenChestTx(txCtx context.Context, in OpenChestInput) (*OpenChestOutput, error) {
	// 5c: SELECT user_chests ... FOR UPDATE
	chest, err := s.chestRepo.FindByUserIDForUpdate(txCtx, in.UserID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrChestNotFound) {
			return nil, apperror.Wrap(err, apperror.ErrChestNotFound, apperror.DefaultMessages[apperror.ErrChestNotFound])
		}
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5d: 判定 unlockable（V1 §7.1.4 同公式：DB status=2 或 (DB status=1 AND unlock_at <= now)）
	now := s.nowFn()
	isUnlockable := chest.Status == 2 || (chest.Status == 1 && !chest.UnlockAt.After(now))
	if !isUnlockable {
		return nil, apperror.New(apperror.ErrChestNotUnlocked, apperror.DefaultMessages[apperror.ErrChestNotUnlocked])
	}

	// 5e: SELECT user_step_accounts ... FOR UPDATE
	account, err := s.stepAccountRepo.FindByUserIDForUpdate(txCtx, in.UserID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrStepAccountNotFound) {
			// V1 §7.2 1009 行钦定：account 行缺失视为数据完整性异常，**非** 3002
			return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if account.AvailableSteps < chestOpenCostSteps {
		return nil, apperror.New(apperror.ErrInsufficientSteps, apperror.DefaultMessages[apperror.ErrInsufficientSteps])
	}

	// 5f: 扣步数（available_steps - 1000, consumed_steps + 1000, version + 1）
	err = s.stepAccountRepo.Spend(txCtx, in.UserID, chestOpenCostSteps, account.Version)
	if err != nil {
		// 乐观锁失败 / DB error 都翻译为 1009（V1 §7.2 1009 行钦定）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5g: 加权抽取 cosmetic_items
	items, err := s.cosmeticItemRepo.ListEnabledForWeightedPick(txCtx)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if len(items) == 0 {
		// seed 未执行 → 1009（V1 §7.2 1009 行钦定）
		return nil, apperror.New(apperror.ErrServiceBusy, "no enabled cosmetic_items for weighted pick (seed not loaded?)")
	}
	weightedItems := make([]random.WeightedItem, len(items))
	for i, item := range items {
		weightedItems[i] = random.WeightedItem{Weight: uint64(item.DropWeight)}
	}
	pickedIndex, err := s.weightedPicker.Pick(weightedItems)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	pickedItem := items[pickedIndex]

	// 5h: 写 chest_open_logs（reward_user_cosmetic_item_id=0 节点 7 阶段占位）
	logRow := &mysql.ChestOpenLog{
		UserID:                   in.UserID,
		ChestID:                  chest.ID,
		CostSteps:                chestOpenCostSteps,
		RewardUserCosmeticItemID: 0, // V1 §7.2.4h 节点 7 阶段占位；节点 8 Story 23.5 改
		RewardCosmeticItemID:     pickedItem.ID,
		RewardRarity:             pickedItem.Rarity,
	}
	if err := s.chestOpenLogRepo.Create(txCtx, logRow); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5i: 刷新下一轮 chest（DELETE 旧 + INSERT 新）
	if err := s.chestRepo.Delete(txCtx, chest.ID); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	nextChest := &mysql.UserChest{
		UserID:        in.UserID,
		Status:        1, // counting
		UnlockAt:      now.Add(chestRefreshNextDelay),
		OpenCostSteps: chestOpenCostSteps,
		Version:       0,
	}
	if err := s.chestRepo.Create(txCtx, nextChest); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5j: 序列化可缓存 response payload
	// **不**含 nextChest.status / nextChest.remainingSeconds（time-derived；handler 层重算）
	// **不**含顶层 requestId（每次请求独立；handler 层填充）
	output := &OpenChestOutput{
		Reward: ChestRewardBrief{
			UserCosmeticItemID: 0, // 节点 7 阶段占位
			CosmeticItemID:     pickedItem.ID,
			Name:               pickedItem.Name,
			Slot:               pickedItem.Slot,
			Rarity:             pickedItem.Rarity,
			AssetURL:           pickedItem.AssetURL,
			IconURL:            pickedItem.IconURL,
		},
		StepAccount: StepAccountBrief{
			TotalSteps:     account.TotalSteps,
			AvailableSteps: account.AvailableSteps - chestOpenCostSteps,
			ConsumedSteps:  account.ConsumedSteps + chestOpenCostSteps,
		},
		NextChest: ChestBrief{
			ID:            nextChest.ID, // GORM Create 后回填
			UnlockAt:      nextChest.UnlockAt.UTC(),
			OpenCostSteps: nextChest.OpenCostSteps,
			// Status / RemainingSeconds 由 handler 层补算
		},
	}

	// 序列化 cacheable response_json（subset of output：不含 time-derived 字段）
	cacheable := buildCacheableResponse(output)
	responseJSON, err := json.Marshal(cacheable)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "marshal response_json failed")
	}

	// 5k: UPDATE idempotency.status='success' + response_json
	if err := s.idempotencyRepo.MarkSuccess(txCtx, in.UserID, in.IdempotencyKey, responseJSON); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 5l: 事务由 WithTx 自动 commit（fn return nil）
	return output, nil
}

// replayFromCachedResponse 反序列化 cached response_json → OpenChestOutput。
// handler 层后续按当前时刻补算 NextChest.Status + RemainingSeconds + 顶层 requestId。
func (s *chestServiceImpl) replayFromCachedResponse(responseJSON []byte) (*OpenChestOutput, error) {
	var cacheable cacheableResponse
	if err := json.Unmarshal(responseJSON, &cacheable); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "unmarshal cached response_json failed")
	}
	return cacheable.toOpenChestOutput(), nil
}

// cacheableResponse / buildCacheableResponse / toOpenChestOutput 由 dev 实装；
// schema 与 OpenChestOutput 同字段集减去 NextChest.Status / NextChest.RemainingSeconds + 顶层 requestId。
// （详见 chest_open_service.go 完整代码）
```

**关键约束**：

- **service 不直接做 rate_limit**：handler 层在调 service 之前显式做 `middleware.CheckRateLimitByUserID(ctx, cfg, userID)`（V1 §7.2.5.4 r10 钦定）；service 只关心 idempotency 命中 + 业务事务
- **`nowFn` 注入而非 `time.Now()` 硬编码**：service struct 字段 `nowFn func() time.Time`；`NewChestService` 内部默认 `func() time.Time { return time.Now().UTC() }`；单测可注入固定 mock 时间
- **`weightedPicker` 注入**：service struct 字段；`NewChestService` 接受 `random.WeightedPicker` 参数；生产 `random.NewCryptoWeightedPicker(rand.Reader)`；单测注入 `mathrand.New(mathrand.NewSource(seed))` 走确定性 source
- **`runOpenChestTx` 是 unexported helper**：步骤 5c~5l 业务全流程从 `OpenChest` 主函数分出，让 5a/5b 短路逻辑与 5c~5l 全流程清晰分离
- **错误码翻译严格按 V1 §7.2 错误码表**：
  - 4001 (chest 不存在) / 4002 (chest 未解锁) / 3002 (steps < 1000) / 1009 (其他全部，含数据完整性异常 / 乐观锁 / DB 错 / 步骤 5b 极端兜底)
  - **禁止**翻译到 1008（V1 §7.2 r11/r15 锁定 1008 在本接口不可达）
  - **禁止**翻译到 4003（V1 §7.2 钦定本接口不触发 4003）
- **`response_json` 缓存严格按 V1 §7.2 r7/r9/r11 + DB §5.16 钦定**：仅含 `{code: 0, message: "ok", data: {reward.*, stepAccount.*, nextChest.{id, unlockAt, openCostSteps}}}`；**禁止**含 `nextChest.status` / `nextChest.remainingSeconds` / `requestId`
- **节点 7 阶段 `userCosmeticItemId = 0` 占位**（V1 §7.2.4h）：service 层硬编码 `UserCosmeticItemID: 0`；handler 层字符串化为 `"0"`
- **加权抽取通过 picker interface 注入**（V1 §7.2 关键约束 + epics.md §20.9 警示）：service 层**不**直接调 `math/rand` 全局源；通过 `random.WeightedPicker.Pick(items)` 拿 selectedIndex
- **`UnlockAt.UTC()` 强制 UTC 视图**（与 home_service / chest_service.GetCurrent 同模式）

### AC3 — chest_handler.go 扩展 + bootstrap/router.go 路由变更

**chest_handler.go 追加 `Open` 方法**：

```go
// Open 处理 POST /api/v1/chest/open（Story 20.6）。
//
// 流程（V1 §7.2.5）：
//  1. 入参解析 + idempotencyKey regex 校验（[A-Za-z0-9_:-] + 1-128 length）
//  2. **handler 内层 rate_limit 检查**（V1 §7.2.5.4 r10）：调
//     middleware.CheckRateLimitByUserID(ctx, cfg, userID)
//     - 但**仅对未命中 committed success replay 的请求做** —— service.OpenChest 内部已先做
//       autocommit SELECT 预检；如果 service 返回的 OpenChestOutput 来自 cached replay 路径，
//       handler **不**调用 rate_limit。
//     - 实装路径：handler 先调 service.OpenChest（service 内做 autocommit 预检 + 事务）；
//       handler **不**单独做 rate_limit，而是**在 service.OpenChest 调用前先做** —— 即如果
//       cached 命中由 service 内的 FindByUserIDAndKey 决定，handler **必须先做 rate_limit 兜底**。
//       但 r10 钦定 "committed success replay 免配额"。
//     - **决策**：handler 入口先做 autocommit idempotency SELECT；如果命中 success，直接调
//       service 走 cached replay 路径（service 内复查 idempotency，幂等安全）；如果未命中，
//       handler 先做 rate_limit 再调 service。这样 handler 拥有"是否需要 rate_limit"的决策点。
//  3. 调 service.OpenChest(ctx, in)
//  4. 成功 → response.Success(c, dto, "ok")；失败 → c.Error(err) + return
//
// **关键差异 vs GetCurrent**：本接口路由层**不**挂 RateLimit middleware（router.go chestOpenGroup
// 仅挂 Auth），rate_limit 在 handler 内层做；V1 §7.2.5.4 r10 钦定。
func (h *ChestHandler) Open(c *gin.Context) {
	// 1. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入）
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	userID, ok := v.(uint64)
	if !ok {
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}

	// 2. 解析 + 校验 idempotencyKey
	var req openChestRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, "request body invalid"))
		return
	}
	if !isValidIdempotencyKey(req.IdempotencyKey) {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "idempotencyKey invalid (must be 1-128 chars, [A-Za-z0-9_:-])"))
		return
	}

	// 3. handler 内层 rate_limit + cached success replay 短路决策
	//    实装路径：handler 调 service.OpenChest 之前，先做 autocommit idempotency SELECT；
	//    - 命中 success → 直接走 service.OpenChest（service 内 autocommit 复查 + cached replay；幂等安全）
	//                    + handler **不**调 CheckRateLimitByUserID（免配额）
	//    - 未命中 → handler **必须先做** CheckRateLimitByUserID 兜底（超限 → 1005）+ 再调 service
	//    - **关键决策**：handler 做"是否需要 rate_limit"的决策点，service 内做"是否走 cached replay"
	//      的实际短路；两层独立判断 idempotency 状态是 OK 的（service 内 SELECT 是兜底，
	//      handler 优先按 rate_limit 决策走）
	cached, cachedErr := h.idempotencyChecker.FindByUserIDAndKey(c.Request.Context(), userID, req.IdempotencyKey)
	if cachedErr != nil && !stderrors.Is(cachedErr, mysql.ErrIdempotencyRecordNotFound) {
		_ = c.Error(apperror.Wrap(cachedErr, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	committedSuccessReplay := cached != nil && cached.Status == mysql.IdempotencyStatusSuccess

	if !committedSuccessReplay {
		// 未命中 success → 必须 rate_limit 兜底
		if rlErr := middleware.CheckRateLimitByUserID(c.Request.Context(), h.rateLimitCfg, userID); rlErr != nil {
			_ = c.Error(rlErr)
			return
		}
	}
	// 命中 success → 跳过 rate_limit，直接走 service（service 内再做 cached replay 解析）

	// 4. 调 service
	requestID := middleware.RequestIDFromCtx(c.Request.Context())
	in := service.OpenChestInput{
		UserID:         userID,
		IdempotencyKey: req.IdempotencyKey,
		RequestID:      requestID,
	}
	out, err := h.svc.OpenChest(c.Request.Context(), in)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// 5. 响应转译（含 NextChest.Status / RemainingSeconds 补算）
	response.Success(c, openChestResponseDTO(out, h.nowFn()), "ok")
}

// openChestRequestDTO V1 §7.2 钦定请求体。
type openChestRequestDTO struct {
	IdempotencyKey string `json:"idempotencyKey" binding:"required"`
}

// openChestResponseDTO 转译 service 输出为 V1 §7.2 wire 格式。
//
// **关键 schema**（V1 §7.2 钦定）：
//   - data.reward.{userCosmeticItemId, cosmeticItemId, name, slot, rarity, assetUrl, iconUrl}
//       - userCosmeticItemId: string "0" 占位（节点 7 阶段）
//       - cosmeticItemId: BIGINT 字符串化
//   - data.stepAccount.{totalSteps, availableSteps, consumedSteps}: number
//   - data.nextChest.{id (string), status (int 1/2), unlockAt (RFC3339), openCostSteps (int), remainingSeconds (int)}
//       - status / remainingSeconds 由本 helper **同源同时刻**按 now 补算（V1 §7.2 r11）
func openChestResponseDTO(out *service.OpenChestOutput, now time.Time) gin.H {
	nextChestStatus := int8(1)
	if !out.NextChest.UnlockAt.After(now) {
		nextChestStatus = 2
	}
	diff := out.NextChest.UnlockAt.Sub(now)
	remainingSeconds := int64(0)
	if diff > 0 {
		// ceil((unlockAt - now) / 1s)
		remainingSeconds = int64((diff + time.Second - 1) / time.Second)
	}

	return gin.H{
		"reward": gin.H{
			"userCosmeticItemId": "0", // V1 §7.2.4h 节点 7 阶段占位
			"cosmeticItemId":     strconv.FormatUint(out.Reward.CosmeticItemID, 10),
			"name":               out.Reward.Name,
			"slot":               out.Reward.Slot,
			"rarity":             out.Reward.Rarity,
			"assetUrl":           out.Reward.AssetURL,
			"iconUrl":            out.Reward.IconURL,
		},
		"stepAccount": gin.H{
			"totalSteps":     out.StepAccount.TotalSteps,
			"availableSteps": out.StepAccount.AvailableSteps,
			"consumedSteps":  out.StepAccount.ConsumedSteps,
		},
		"nextChest": gin.H{
			"id":               strconv.FormatUint(out.NextChest.ID, 10),
			"status":           nextChestStatus,
			"unlockAt":         out.NextChest.UnlockAt.Format(time.RFC3339),
			"openCostSteps":    out.NextChest.OpenCostSteps,
			"remainingSeconds": remainingSeconds,
		},
	}
}

// isValidIdempotencyKey 校验 V1 §7.2 钦定字符集 [A-Za-z0-9_:-] + length 1-128。
// regex 在 package 级 var 编译（避免每次请求 re-parse）。
var idempotencyKeyRegex = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,128}$`)

func isValidIdempotencyKey(key string) bool {
	return idempotencyKeyRegex.MatchString(key)
}
```

**ChestHandler struct 扩展**：

```go
// ChestHandler struct 扩展：
//
// type ChestHandler struct {
//   svc                 service.ChestService              // 既有（已含 Open 方法）
//   idempotencyChecker  mysql.IdempotencyRepo             // 新增：handler 内层做 cached success replay 预检
//   rateLimitCfg        config.RateLimitConfig            // 新增：handler 内层 rate_limit 配置
//   nowFn               func() time.Time                  // 新增：openChestResponseDTO 补算 status / remainingSeconds 用
// }
//
// NewChestHandler 签名扩展：
//
// func NewChestHandler(
//   svc service.ChestService,
//   idempotencyChecker mysql.IdempotencyRepo,
//   rateLimitCfg config.RateLimitConfig,
// ) *ChestHandler {
//   return &ChestHandler{
//     svc: svc,
//     idempotencyChecker: idempotencyChecker,
//     rateLimitCfg: rateLimitCfg,
//     nowFn: func() time.Time { return time.Now().UTC() },
//   }
// }
```

**bootstrap/router.go 关键改动**：

1. wire 新 repo + picker：

```go
// Story 20.6 加：4 个新 repo 实例（cosmeticItemRepo / chestOpenLogRepo / idempotencyRepo）
// + 1 个 weightedPicker
cosmeticItemRepo := repomysql.NewCosmeticItemRepo(deps.GormDB)
chestOpenLogRepo := repomysql.NewChestOpenLogRepo(deps.GormDB)
idempotencyRepo := repomysql.NewIdempotencyRepo(deps.GormDB)
weightedPicker := random.NewCryptoWeightedPicker(cryptorand.Reader)
```

2. 修改既有 chestSvc 构造（向后不兼容；旧签名 1 参数 → 新签名 7 参数）：

```go
// Story 20.5 既有：
// chestSvc := service.NewChestService(chestRepo)
//
// Story 20.6 修改为：
chestSvc := service.NewChestService(
    chestRepo,
    deps.TxMgr,
    idempotencyRepo,
    stepAccountRepo,
    cosmeticItemRepo,
    chestOpenLogRepo,
    weightedPicker,
)
```

3. 修改 chestHandler 构造（向后不兼容；旧签名 1 参数 → 新签名 3 参数）：

```go
chestHandler := handler.NewChestHandler(chestSvc, idempotencyRepo, deps.RateLimitCfg)
```

4. 关键路由架构变更（移除 chest_open 从 authedGroup，新建独立 chestOpenGroup）：

```go
// 既有 authedGroup 保持不变（GET /chest/current 仍在其中）
authedGroup.GET("/chest/current", chestHandler.GetCurrent) // 既有 Story 20.5

// Story 20.6 新增：chestOpenGroup（仅挂 Auth，**不**挂 RateLimit middleware；
// V1 §7.2.5.4 r10 钦定 rate_limit 由 handler 内层做）
chestOpenGroup := api.Group("", middleware.Auth(deps.Signer))
chestOpenGroup.POST("/chest/open", chestHandler.Open)
```

**关键约束**：

- handler 入口 idempotency SELECT 预检 + service 内部再次 SELECT 预检 = **双重 SELECT**：这是协议层钦定的设计（V1 §7.2.5 服务端逻辑步骤 3 + 服务端逻辑步骤 5a）；handler 层 SELECT 决定"是否做 rate_limit"，service 层 SELECT 决定"是否走 cached replay"；两层独立判断幂等状态是契约要求（V1 §7.2.3 + §7.2.5b 钦定）
- handler 内层 rate_limit 是 **opt-in**（仅未命中 committed success 时才调）；V1 §7.2.5.4 r10/r11 钦定
- `openChestResponseDTO` 接受 `now time.Time` 参数（**不**内部 `time.Now()`）—— 让 handler 把 `time.Now().UTC()` 统一作为补算 status / remainingSeconds 的时刻；避免 handler 内多个 time.Now() 取值导致 status=1 + remainingSeconds=0 不可能组合（V1 §7.2 r11 锁定同源同时刻）
- `idempotencyKeyRegex` 是 package-level var（编译期建立）—— 不在每次请求里 `regexp.MustCompile`
- `request_id` 透传：`middleware.RequestIDFromCtx(ctx)` 取（middleware/request_id.go 已落地；handler 注入 service，service 不消费但保留作 future log 关联）
- handler 单测必须覆盖：HappyPath_FirstTime / HappyPath_CachedReplay_SkipsRateLimit / InvalidKey_1002 / RateLimitExceeded_1005 / ChestNotUnlocked_4002 / InsufficientSteps_3002（详见 AC5）

### AC4 — service 单元测试覆盖（≥7 case，stub repo + 注入 picker / nowFn）

新建 `server/internal/service/chest_open_service_test.go`：≥7 case，前缀 `TestChestService_OpenChest_`。

**stub 模式**：

```go
//go:build !integration

package service_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/random"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// stubIdempotencyRepo: mysql.IdempotencyRepo 的 stub（每 case 独立实例）
type stubIdempotencyRepo struct {
	findFn         func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error)
	claimFn        func(ctx context.Context, userID uint64, key string) (int64, error)
	markSuccessFn  func(ctx context.Context, userID uint64, key string, json []byte) error
}

// stubCosmeticItemRepo: mysql.CosmeticItemRepo 的 stub
type stubCosmeticItemRepo struct {
	listFn func(ctx context.Context) ([]mysql.CosmeticItem, error)
}

// stubChestOpenLogRepo: mysql.ChestOpenLogRepo 的 stub
type stubChestOpenLogRepo struct {
	createCalls []*mysql.ChestOpenLog
}

// stubWeightedPicker: random.WeightedPicker 的 stub（注入指定 selectedIndex）
type stubWeightedPicker struct {
	pickFn func(items []random.WeightedItem) (int, error)
}

// stubTxManager: tx.Manager 的 stub —— **关键**：让 fn 直接执行（不真开 DB 事务）
// 与既有 auth_service_test / room_service_test 同模式
type stubTxManager struct {
	withTxFn func(ctx context.Context, fn func(txCtx context.Context) error) error
}

// 各 stub repo 的 method 实装 ...（略）

// 必须覆盖 ≥7 case：
```

**必须覆盖（≥7 case + 推荐扩展 case）**：

1. **`TestChestService_OpenChest_HappyPath_FirstTime`**：
   stub idempotencyRepo.findFn 返 `(nil, ErrIdempotencyRecordNotFound)`（首次到达）
   stub claimFn 返 `(1, nil)`（新行）
   stub chestRepo.findForUpdateFn 返 `&UserChest{ID:5001, Status:1, UnlockAt:now-1min, OpenCostSteps:1000, Version:0}`（unlockable）
   stub stepAccountRepo.findForUpdateFn 返 `&StepAccount{UserID:1001, TotalSteps:1500, AvailableSteps:1500, ConsumedSteps:0, Version:3}`
   stub stepAccountRepo.spendFn 返 `nil`
   stub cosmeticItemRepo.listFn 返 `[{ID:24, Name:"星星围巾", Slot:4, Rarity:2, DropWeight:30, ...}]`
   stub pickerFn 返 `(0, nil)`
   stub chestOpenLogRepo.createFn 返 `nil`（捕获 logRow → 验证 reward_user_cosmetic_item_id=0 + reward_cosmetic_item_id=24）
   stub chestRepo.deleteFn 返 `nil`（验证 chestID=5001）
   stub chestRepo.createFn 返 `nil`（验证新 chest unlock_at ≈ now+10min）
   stub markSuccessFn 返 `nil`（捕获 responseJSON → 验证不含 nextChest.status / nextChest.remainingSeconds / requestId）
   断言：
   - `out.Reward.UserCosmeticItemID == 0`（节点 7 占位）
   - `out.Reward.CosmeticItemID == 24`
   - `out.StepAccount.AvailableSteps == 500`
   - `out.StepAccount.ConsumedSteps == 1000`
   - `out.NextChest.OpenCostSteps == 1000`
   - response_json 序列化结构断言：JSON 字段集仅含 `code / message / data.reward.* / data.stepAccount.* / data.nextChest.{id, unlockAt, openCostSteps}`

2. **`TestChestService_OpenChest_IdempotencyReplay_CachedSuccess`**：
   stub idempotencyRepo.findFn 返 `(&IdempotencyRecord{Status:'success', ResponseJSON:"<valid json>"}, nil)`（命中 cached）
   断言：
   - 返回 `*OpenChestOutput`，字段反序列化自 ResponseJSON
   - **stub claimFn / stub chestRepo / stub stepAccountRepo / stub cosmeticItemRepo 都没有被调用**（service 直接 cached replay，跳过事务）

3. **`TestChestService_OpenChest_ChestNotFound_4001`**：
   stub idempotencyRepo.findFn 返 `(nil, ErrIdempotencyRecordNotFound)`
   stub claimFn 返 `(1, nil)`
   stub chestRepo.findForUpdateFn 返 `(nil, mysql.ErrChestNotFound)`
   断言：`err` 是 `*apperror.AppError` + `Code == 4001`

4. **`TestChestService_OpenChest_ChestNotUnlockable_4002`**：
   stub chestRepo.findForUpdateFn 返 `&UserChest{Status:1, UnlockAt:now+5min, ...}`（counting）
   断言：`err.Code == 4002`

5. **`TestChestService_OpenChest_InsufficientSteps_3002`**：
   stub chestRepo.findForUpdateFn 返 unlockable chest
   stub stepAccountRepo.findForUpdateFn 返 `&StepAccount{AvailableSteps:500, ...}`（< 1000）
   断言：`err.Code == 3002`

6. **`TestChestService_OpenChest_StepAccountNotFound_1009`**：
   stub stepAccountRepo.findForUpdateFn 返 `(nil, mysql.ErrStepAccountNotFound)`
   断言：`err.Code == 1009`（V1 §7.2 钦定数据完整性异常 = 1009 **非** 3002）

7. **`TestChestService_OpenChest_OptimisticLockFails_1009`**：
   stub stepAccountRepo.spendFn 返 `mysql.ErrStepAccountVersionMismatch`
   断言：`err.Code == 1009`

**推荐扩展 case（≥3 case）**：

8. **`TestChestService_OpenChest_NoEnabledCosmetic_1009`**：
   stub cosmeticItemRepo.listFn 返 `([]CosmeticItem{}, nil)`（seed 未执行）
   断言：`err.Code == 1009`

9. **`TestChestService_OpenChest_IdempotencyClaim_ExistingRow_ShortCircuitReplay`**：
   stub idempotencyRepo.findFn 返 `(nil, ErrIdempotencyRecordNotFound)`（autocommit SELECT 未命中）
   stub claimFn 返 `(0, nil)`（事务内 SELECT 后行已存在，首事务已 commit）
   stub idempotencyRepo.findFn 第二次调用（事务内）返 `(&IdempotencyRecord{Status:'success', ResponseJSON:...}, nil)`
   断言：service 走步骤 5b 短路返 cached output；stub chestRepo / stepAccountRepo / cosmeticItemRepo 都没有被调用

10. **`TestChestService_OpenChest_WeightedPickDistribution`**（用确定性 seed math/rand 验证分布）：
    stub cosmeticItemRepo.listFn 返 3 件不同权重的 cosmetic
    stub weightedPicker 用 `random.NewWeightedPickerWithReader(mathrand.New(mathrand.NewSource(42)))`（确定性 source）
    跑 100 次 OpenChest（每次新 idempotencyKey），统计 reward.cosmeticItemId 的分布
    断言：分布大致符合权重比例（±10% 容差，避免脆弱测试）

**关键约束**：

- 每 case 独立 stub 实例（避免 state 串扰，与 step_service_test 同模式）
- **不**真起 DB / 真起 redis（service 单测全部 mock 驱动）
- `stubTxManager.WithTx` 直接执行 fn（不真开事务；fn 内部不依赖 tx 行为，stub 兼容）
- 时间断言用区间（±1s 容差，与 chest_service_test ClockBoundary 同模式）
- NotFound case 用 `errors.As(err, &appErr)` + `appErr.Code == expected`，**不**用 `errors.Is`

### AC5 — handler 单元测试覆盖（≥6 case，stub service + 测试 router）

新建 `server/internal/app/http/handler/chest_open_handler_test.go`：≥6 case，前缀 `TestChestHandler_Open_`。

**stub service 扩展**：

```go
// 既有 stubChestService（chest_handler_test.go 已建，从 Story 20.5）需要扩展加 OpenChestFn 字段
// 或新建独立 stubChestServiceWithOpen
```

**必须覆盖（≥6 case）**：

1. **`TestChestHandler_Open_HappyPath_FirstTime`**：
   mockUserID = 1001
   stub idempotencyChecker.findFn 返 `(nil, ErrIdempotencyRecordNotFound)`（未命中）
   stub service.openFn 返 happy `OpenChestOutput{Reward: {...UserCosmeticItemID:0, CosmeticItemID:24...}, NextChest: {UnlockAt:now+10min...}}`
   POST body: `{"idempotencyKey": "test_key_001"}`
   断言：HTTP 200 + envelope `code=0` + `data.reward.userCosmeticItemId == "0"`（string 占位）+ `data.reward.cosmeticItemId == "24"` + `data.nextChest.status == 1` + `data.nextChest.remainingSeconds in [598, 601]`（按当前时刻补算）

2. **`TestChestHandler_Open_HappyPath_CachedReplay_SkipsRateLimit`**：
   stub idempotencyChecker.findFn 返 `(&IdempotencyRecord{Status:'success'}, nil)`（命中 committed success）
   stub service.openFn 返 `OpenChestOutput{...}`（service 内部走 cached replay）
   **关键断言**：handler **不**调用 rate_limit（在 stub rate_limit 上设置 counter 验证未被调用）+ 仍走 service.openFn → 返回 200 + cached output

3. **`TestChestHandler_Open_RateLimitExceeded_1005`**：
   stub idempotencyChecker.findFn 返 `(nil, ErrIdempotencyRecordNotFound)`（未命中）
   stub `middleware.CheckRateLimitByUserID` 返 `apperror.New(ErrTooManyRequests, "...")`
   断言：HTTP 状态 + envelope `code=1005`；**service.openFn 没有被调用**

4. **`TestChestHandler_Open_InvalidIdempotencyKey_1002`**：
   POST body: `{"idempotencyKey": ""}` 或 `{"idempotencyKey": "key with space"}` 或 `{"idempotencyKey": "<160-char-long-string>"}`
   断言：envelope `code=1002`；service / idempotencyChecker 都没有被调用
   **建议**：跑 3 子 case（空 / 非法字符 / 超长）合并 1 个 test 函数

5. **`TestChestHandler_Open_ServiceReturns4002_HTTP200`**：
   stub idempotencyChecker.findFn 返 NotFound；rate_limit 通过
   stub service.openFn 返 `*apperror.AppError(ErrChestNotUnlocked, ...)`
   断言：HTTP 200 + envelope `code=4002`

6. **`TestChestHandler_Open_ServiceReturns3002_HTTP200`**：
   stub service.openFn 返 `*apperror.AppError(ErrInsufficientSteps, ...)`
   断言：envelope `code=3002`

7. **`TestChestHandler_Open_MissingUserIDInContext_Returns1009`**（推荐扩展，与 GetCurrent 同模式）：
   mockUserID = nil → handler 走 unreachable 兜底 → envelope `code=1009`

8. **`TestChestHandler_Open_NextChestStatusAndRemainingSeconds_DynamicAtRequestTime`**（推荐扩展 r11 锁定）：
   stub service.openFn 返 `OpenChestOutput{NextChest: {UnlockAt: fixed time T}}`
   handler 用 mock `nowFn = func() time.Time { return T - 600s }` → 断 `nextChest.status == 1` + `remainingSeconds == 600`
   再跑同一 stub，mock `nowFn = func() time.Time { return T + 1s }` → 断 `nextChest.status == 2` + `remainingSeconds == 0`
   验证 V1 §7.2 r11 锁定的"同源同时刻"补算

**关键约束**：

- handler 单测必须覆盖"cached replay 路径 skip rate_limit"分支（核心 r10 决策点）
- handler 单测必须覆盖 invalid key 1002 路径（regex 校验）
- handler 单测必须覆盖 nextChest 动态补算正确性（r11 决策点）
- response_json 在 handler 单测层不需要专门 case（service 单测已断言）
- stub 模式：每 case 独立 stub instance
- 响应 envelope 解析复用 chest_handler_test.go 既有 `decodeChestEnvelope` helper

### AC6 — repo 单元测试（sqlmock 驱动；≥6 + 2 + 3 + 2 + 1 = 14 case）

新建 / 扩展以下 repo 测试文件：

1. **`server/internal/repo/mysql/chest_open_idempotency_record_repo_test.go`**（新建，≥6 case）：
   覆盖 `FindByUserIDAndKey` (happy / NotFound / DB error) + `ClaimPending` (新行 / 已存在) + `MarkSuccess` (happy)
   参考 chest_repo_test.go sqlmock 模式 + 既有 step_account_repo_test.go 模式

2. **`server/internal/repo/mysql/chest_repo_test.go`** 扩展（追加 ≥2 case）：
   覆盖 `FindByUserIDForUpdate` (happy / NotFound) + `Delete` (happy)
   参考既有 chest_repo_test.go 的 `TestChestRepo_FindByUserID_*` 测试模式

3. **`server/internal/repo/mysql/step_account_repo_test.go`** 扩展（追加 ≥3 case）：
   覆盖 `FindByUserIDForUpdate` (happy / NotFound) + `Spend` (happy / 乐观锁冲突)

4. **`server/internal/repo/mysql/cosmetic_item_repo_test.go`**（新建，≥2 case）：
   覆盖 `ListEnabledForWeightedPick` (happy 多行 / 空集合)

5. **`server/internal/repo/mysql/chest_open_log_repo_test.go`**（新建，≥1 case）：
   覆盖 `Create` (happy)

**关键约束**：

- 全部 sqlmock 驱动（与既有 repo 测试同模式）
- SQL regex 匹配关键字（FOR UPDATE / ON DUPLICATE KEY UPDATE / INSERT 等）
- 单测 build tag: `//go:build !integration`

### AC7 — random 包单元测试（≥3 case）

新建 `server/internal/pkg/random/weighted_test.go`：

1. `TestWeightedPicker_Pick_SingleItem`：1 item with weight 100 → 返 `(0, nil)`
2. `TestWeightedPicker_Pick_MultipleItems_DistributionWithDeterministicSeed`：3 items weight 10/30/60 → 跑 1000 次确定性 seed → 分布大致符合 10%/30%/60% (±5% 容差)
3. `TestWeightedPicker_Pick_EmptyItems_ReturnsError`：空 slice → 返 `(0, ErrEmptyItems)`
4. （推荐扩展）`TestWeightedPicker_Pick_ZeroTotalWeight_ReturnsError`：全部 weight=0 → 返 `(0, ErrZeroTotalWeight)`

### AC8 — 集成测试（dockertest，≥2 case）

新建 `server/internal/service/chest_open_service_integration_test.go`：

1. **`TestChestOpenServiceIntegration_HappyPath_FullFlow`**：
   - setUp: 启 dockertest MySQL + Redis（复用既有 helper）→ 跑 migrations（含 0014）→ 跑 cosmetic_items seed → 创建 user + 1500 steps + chest unlock_at=now-1min（强制 unlockable）
   - 调 svc.OpenChest(ctx, {UserID, IdempotencyKey: "test_key"})
   - 验证 DB：
     - user_step_accounts.available_steps = 500 / consumed_steps = 1000 / version + 1
     - chest_open_logs 多 1 行（reward_user_cosmetic_item_id=0 占位 + reward_cosmetic_item_id ∈ enabled set + reward_rarity ∈ {1,2,3,4}）
     - 旧 chest 已 DELETE，新 chest INSERT（unlock_at ≈ now+10min, status=1, version=0）
     - chest_open_idempotency_records 多 1 行（status='success' + response_json 完整）
   - 验证返回 OpenChestOutput 字段完整

2. **`TestChestOpenServiceIntegration_IdempotencyReplay_SameKey`**：
   - 第一次 OpenChest 同 happy
   - 第二次同 idempotencyKey 调 OpenChest
   - 验证 DB：
     - user_step_accounts 无新变化（available_steps 仍 500）
     - chest_open_logs 仍 1 行（**没有**多 1 行）
     - chest_open_idempotency_records 仍 1 行
   - 验证返回 OpenChestOutput 与首次结果相同（reward / stepAccount / nextChest.id 一致；nextChest.remainingSeconds 可能因时间漂移略小，但 status 应一致）

**关键约束**：

- 复用 home_service_integration_test.go / step_service_integration_test.go 已建的 dockertest helper（`startMySQLContainer` / `insertUser` / `insertUserChest` / `seedCosmeticItems` 等）
- 本机 Windows docker 不可用 → t.Skip（helper 内已有 skip 逻辑）
- 集成测试 build tag: `//go:build integration`
- **不**做并发场景 / 回滚场景集成测试（那是 Story 20.9 owner）
- **不**做加权抽奖分布的集成测试（service 单测确定性 seed 已覆盖）

### AC9 — `bash scripts/build.sh` 全量绿

完成后必须能跑通：

```bash
bash scripts/build.sh                # vet + build → no failures
bash scripts/build.sh --test         # 全单测过（含本 story 新增 ≥40 case + 既有不回归）
bash scripts/build.sh --race --test  # CI Linux 必过；Windows race skip 不阻塞
bash scripts/build.sh --integration  # 既有集成 + 新 ≥2 case；docker 不可用 → t.Skip
```

### AC10 — 验证清单（人工 + 自动化，15 项核对）

| # | 验证项 | 验证方式 |
|---|---|---|
| 1 | §5.16 migration 0014 schema 1:1 匹配 DB §5.16 + V1 §7.2 钦定（status ENUM 二态机 / response_json JSON / UNIQUE uk_user_id_key） | Read up.sql + diff DB §5.16 |
| 2 | IdempotencyRepo 三方法签名 + 行为严格按 V1 §7.2 r5/r6/r7/r11 | Read chest_open_idempotency_record_repo.go |
| 3 | service.OpenChest 步骤 5a (ClaimPending) 是事务**首条**语句 | Read chest_open_service.go runOpenChestTx 实装 |
| 4 | 5b 短路分支兜底：status='pending' → 1009（**非** 1008） | Read service 实装 + 单测 |
| 5 | 5g 加权抽取通过 random.WeightedPicker.Pick；service 不直接调 math/rand | Grep math/rand 在 chest_open_service.go 应 0 命中 |
| 6 | 5h chest_open_logs.reward_user_cosmetic_item_id 固定 0 占位（节点 7） | Read service.runOpenChestTx 实装 |
| 7 | 5k MarkSuccess + 业务事务在同事务原子 commit | Read txMgr.WithTx fn 闭包结构 |
| 8 | response_json 缓存**不**含 nextChest.status / nextChest.remainingSeconds / requestId | Read buildCacheableResponse + service 单测断言 |
| 9 | handler.Open 内层 rate_limit：未命中 success 时调；命中 success 跳过 | Read chest_handler.go + 单测 case 2 / 3 |
| 10 | handler.openChestResponseDTO 用 now 参数同源同时刻补算 status + remainingSeconds | Read DTO helper + 单测 case 8 |
| 11 | idempotencyKey regex 校验 [A-Za-z0-9_:-]{1,128} | Read isValidIdempotencyKey + 单测 case 4 |
| 12 | router.go: chest/open **不**在 authedGroup（独立 chestOpenGroup 仅挂 Auth） | Read router.go diff |
| 13 | router.go: chestSvc / chestHandler 构造签名扩展正确（7 参数 / 3 参数）；wire 5 新实例 | Read router.go diff |
| 14 | 错误码：4001 / 4002 / 3002 / 1009 / 1002 / 1005；**不**触发 1008 / 4003 | Grep ErrIdempotencyConflict / ErrChestNotOpenable 在 chest_open_service.go 应 0 命中 |
| 15 | `bash scripts/build.sh --test` 全绿（含本 story 新增 ≥40 case） | bash 实跑 |

### AC11 — 不 commit（流水线由 epic-loop 下游收口）

本 story 是 server 业务代码 story，commit 由 epic-loop 流水线在下游 fix-review / story-done sub-agent 阶段统一收口。

- 本 story 的 dev workflow **不** commit / **不** push
- commit message 模板（story-done 阶段使用）：

  ```text
  feat(chest-open): POST /chest/open 开箱事务 + idempotencyKey + 加权抽取（Story 20.6）

  - migration 0014 chest_open_idempotency_records: 落地 §5.16 二态机 schema（pending/success）
    + UNIQUE uk_user_id_key + KEY idx_status_created_at
  - IdempotencyRepo: FindByUserIDAndKey + ClaimPending + MarkSuccess 三方法
    （事务首条预声明 + 同事务最终化原子写）
  - service.OpenChest 实装 8 步事务（V1 §7.2.5）：committed success replay 短路 / 5a 预声明
    / 5c FOR UPDATE chest / 5d unlockable / 5e FOR UPDATE step_account / 5f 扣步数乐观锁
    / 5g 加权抽取 / 5h 写 log / 5i 刷下一轮 chest / 5j 序列化 / 5k MarkSuccess
  - handler.Open 实装：handler 内层 rate_limit（V1 §7.2 r10）+ idempotencyKey regex 校验
    + 同源同时刻补算 nextChest.status / remainingSeconds（V1 §7.2 r11）
  - bootstrap/router.go: 独立 chestOpenGroup（仅挂 Auth；opt-out RateLimit middleware）
    + wire 5 新实例（idempotencyRepo / cosmeticItemRepo / chestOpenLogRepo / weightedPicker）
  - random/weighted: WeightedPicker interface + crypto/rand 注入（替代 math/rand 全局源）
  - chest_repo / step_account_repo / cosmetic_item_repo / chest_open_log_repo 扩展
    （FOR UPDATE / Spend / ListEnabledForWeightedPick / Create）
  - 单测 ≥40 case + 集成 ≥2 case（dockertest happy + idempotency replay）

  契约依据：V1 §7.2 r1~r15 (Story 20.1) + DB §5.16 + §8.3 + epics.md §20.6 行 2899-2930.
  §5.16 migration scope 决策：选项 A（与 service / handler 同 story 一并落地）.

  Story: 20-6-post-chest-open-事务-idempotencykey-加权抽取
  ```

- commit hash 待 story-done 阶段产生后回填到本文件

## Tasks / Subtasks

- [x] **Task 1（AC1）**：§5.16 migration + IdempotencyRepo 落地
  - [x] 1.1 Read DB §5.16 + V1 §7.2 r5/r6/r7/r11 完整决策段
  - [x] 1.2 Write `server/migrations/0014_init_chest_open_idempotency_records.up.sql`（CREATE TABLE 1:1 匹配 §5.16）
  - [x] 1.3 Write `server/migrations/0014_init_chest_open_idempotency_records.down.sql`（DROP TABLE IF EXISTS）
  - [x] 1.4 Write `chest_open_idempotency_record_repo.go`：struct + interface + impl + NewIdempotencyRepo + 3 方法
  - [x] 1.5 追加 errors.go: `ErrIdempotencyRecordNotFound` 哨兵
  - [x] 1.6 Write `chest_open_idempotency_record_repo_test.go`：≥6 case（sqlmock）→ 7 case 全过
  - [x] 1.7 跑 `go test ./internal/repo/mysql/... -run TestIdempotencyRepo -v` 验 6 case 全过

- [x] **Task 2**：chest_repo.go / step_account_repo.go / cosmetic_item_repo.go / chest_open_log_repo.go 扩展
  - [x] 2.1 Read 既有 4 个 repo 文件 + 测试 + room_repo.go FindByIDForUpdate 模式
  - [x] 2.2 chest_repo.go: 追加 FindByUserIDForUpdate + Delete 方法
  - [x] 2.3 step_account_repo.go: 追加 FindByUserIDForUpdate + Spend 方法
  - [x] 2.4 cosmetic_item_repo.go: 落地 CosmeticItemRepo interface + impl + ListEnabledForWeightedPick
  - [x] 2.5 chest_open_log_repo.go: 落地 ChestOpenLogRepo interface + impl + Create
  - [x] 2.6 同步追加各 repo test 文件 case（chest_repo +3 case / step_account +4 case / cosmetic +2 case / chest_open_log +1 case）
  - [x] 2.7 跑 `go test ./internal/repo/mysql/... -v` 全过

- [x] **Task 3（AC7）**：random/weighted 包落地
  - [x] 3.1 Read epics.md §20.9 行 4403 警示 + V1 §7.2.5g 加权抽取语义
  - [x] 3.2 Write `server/internal/pkg/random/weighted.go`：WeightedPicker interface + cryptoWeightedPicker impl + NewCryptoWeightedPicker
  - [x] 3.3 Write `weighted_test.go`：≥3 case（确定性 seed 验证分布）→ 4 case 全过
  - [x] 3.4 跑 `go test ./internal/pkg/random/... -v` 全过

- [x] **Task 4（AC2 + AC4）**：service 层 OpenChest 实装 + 单测（TDD：先写测试驱动 service）
  - [x] 4.1 Read 既有 chest_service.go / step_service.go SyncSteps 模式
  - [x] 4.2 Write `chest_open_service.go`：扩展 ChestService interface + chestServiceImpl + 8 步事务实装
  - [x] 4.3 Write `chest_open_service_test.go`：≥7 case + 推荐 3 case = 10 case（stub repo + picker）→ 13 case 全过
  - [x] 4.4 跑 `go test ./internal/service/... -run TestChestService_OpenChest -v` 全过
  - [x] 4.5 Read 回检：(a) ClaimPending 是事务首条 ✅；(b) 5b 短路兜底走 1009 而非 1008 ✅；(c) weightedPicker 注入不直接调 math/rand ✅；(d) response_json 不含 time-derived 字段 ✅

- [x] **Task 5（AC3 + AC5）**：handler.Open 实装 + 单测
  - [x] 5.1 Read 既有 chest_handler.go GetCurrent + room_handler.go POST 模式
  - [x] 5.2 Extend chest_handler.go: 追加 Open 方法 + openChestRequestDTO + openChestResponseDTO + isValidIdempotencyKey + extend struct fields
  - [x] 5.3 Write `chest_open_handler_test.go`：≥6 case + 推荐 2 case = 10 case
  - [x] 5.4 跑 `go test ./internal/app/http/handler/... -run TestChestHandler_Open -v` 全过
  - [x] 5.5 Read 回检：(a) cached replay 跳过 rate_limit ✅；(b) idempotencyKey regex 校验 ✅；(c) nextChest 动态补算同源同时刻 ✅

- [x] **Task 6（AC3）**：middleware/rate_limit.go 公开 CheckRateLimitByUserID
  - [x] 6.1 Read middleware/rate_limit.go 既有 RateLimit middleware 工厂
  - [x] 6.2 Extract / Export `CheckRateLimitByUserID(ctx, cfg, userID) error` 函数（用 userIDRateChecker struct + sync.Once 单例 + 跨包 ResetForTest hook）
  - [x] 6.3 单测追加 ≥3 case（HappyPath / ExceedQuota / UserIDZero）

- [x] **Task 7（AC3）**：bootstrap/router.go 改造
  - [x] 7.1 wire 4 新实例（idempotencyRepo / cosmeticItemRepo / chestOpenLogRepo / weightedPicker）
  - [x] 7.2 修改 chestSvc / chestHandler 构造签名（7 参数 / 3 参数）
  - [x] 7.3 新建 chestOpenGroup（独立 group，仅 Auth，**不**挂 RateLimit）
  - [x] 7.4 注册 POST /chest/open 到 chestOpenGroup
  - [x] 7.5 跑 `go test ./internal/app/bootstrap/... -v` 全过（router 测试不回归）

- [x] **Task 8（AC8）**：集成测试
  - [x] 8.1 Read home_service_integration_test.go / step_service_integration_test.go helper 命名
  - [x] 8.2 Write `chest_open_service_integration_test.go`：≥2 case（HappyPath_FullFlow + IdempotencyReplay_SameKey）
  - [x] 8.3 验证本机 Windows docker 不可用 → t.Skip 不阻塞；`bash scripts/build.sh --integration` BUILD SUCCESS

- [x] **Task 9（AC9 + AC10）**：全量验证
  - [x] 9.1 `bash scripts/build.sh`（vet + build）过 → BUILD SUCCESS
  - [x] 9.2 `bash scripts/build.sh --test` 全绿（含本 story 新增 ≥40 case）→ all packages PASS
  - [x] 9.3 `bash scripts/build.sh --integration` BUILD SUCCESS（docker 不可用 → t.Skip ok）
  - [x] 9.4 `git status --short` 改动文件清单核对（详见 Dev Agent Record → File List）
  - [x] 9.5 在 Completion Notes List 勾选 AC10 验证清单 15 项

- [x] **Task 10（AC11）**：本 story 不做 git commit
  - [x] 10.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 10.2 commit message 模板保留在 story 文件中
  - [ ] 10.3 commit hash 待 story-done 阶段回填

## Dev Notes

### 关键设计原则

1. **§5.16 migration scope = 选项 A（紧耦合）**：
   - 本 story 一并落地 0014 migration + GORM struct + repo + 单测 + 集成测试
   - 决策理由：集成测试不可分（dockertest case 需要表存在）+ 历史先例（20.4 owner chest_open_logs migration）+ YAGNI（无独立 deliver 价值）
   - 与备选选项 B（独立 20.10 owner）对比：选项 B 增加流水线开销 + story 间依赖 +不交付独立价值
   - **反模式**：拆 migration 单独 story → 20.6 集成测试要等 migration story done 才能跑 → 流水线节奏破坏 + story 间不可独立 deliver

2. **DB 持久化幂等 = 单一可信源（V1 §7.2 r5/r6/r7/r9/r10/r11）**：
   - **r5**: Redis 是非事务存储 → MySQL commit 成功 + Redis SET 失败 case 下 client 卡死；DB 同事务幂等彻底消除该风险
   - **r6**: 预声明 INSERT 在业务事务**首条**语句；事务原子提交；rollback 同步清除 pending 行（消除 r5 设计的"pending 永久卡死"悖论）
   - **r7**: schema 简化二态机 `('pending', 'success')`；**移除** failed 状态；**移除** best-effort post-rollback failed upsert（race condition）
   - **r9**: `response_json` 缓存**不**含 nextChest.status / nextChest.remainingSeconds（time-derived；cached replay 同源同时刻重算）
   - **r10**: rate_limit 挪到 handler 内层；置于幂等命中预检之后；committed success replay 免配额
   - **r11**: 加 r9 漏的 nextChest.status 处理；锁定 MVCC pending 不可见 + 1008 在本接口节点 7 退役
   - **反模式**：本 story 用 Redis 做幂等键 → 违反 r5；预声明 INSERT 在事务外独立 → 违反 r6；schema 含 failed → 违反 r7；缓存 nextChest.status → 违反 r9/r11

3. **InnoDB unique-key X-lock 兜底并发**：
   - 同 idempotencyKey 并发请求：只有一个事务能拿 X-lock 进入业务流程；其他事务在 INSERT 语句上阻塞排队
   - 首事务结束（commit / rollback）→ 其他事务再分支：commit → `affected_rows = 0` 走短路；rollback → `affected_rows = 1` 走全流程
   - **反模式**：在 5a 之前用应用层 mutex / Redis SETNX 兜底并发 → 多余 + 与 InnoDB 锁语义重复 + 引入新失败模式

4. **MVCC 下 pending 不可见 = 硬约束（r11）**：
   - InnoDB 默认 REPEATABLE READ + MVCC：autocommit SELECT 看不到首事务尚未 commit 的 pending 行
   - 步骤 3 (handler 入口 autocommit SELECT) 只能命中 committed success；pending 阶段同 key 重试在步骤 3 表现为"未命中"→ 走 rate_limit 检查
   - **不可降低隔离级别**："READ UNCOMMITTED" 会让"首事务 rollback 后第二请求误以为已 commit"，引入更严重的 bug
   - 协议层**接受** "pending 阶段同 key 重试消耗 rate_limit quota" 作为次优契约（详见 V1 §7.2 r11 决策段）

5. **time-derived 字段同源同时刻重算（r11）**：
   - `nextChest.status` 与 `nextChest.remainingSeconds` 都是 time-derived
   - cached success replay 路径：必须**同源同时刻**用同一 now 快照重算两字段
   - 反例：仅重算 remainingSeconds 而 status 用 cached → 重试发生在新 chest 已到期解锁时刻会返回 `status=1` + `remainingSeconds=0` 不可能组合
   - **反模式**：本 service `response_json` 包含 nextChest.status → 违反 r11

6. **handler 内层 rate_limit（r10）**：
   - 路由层**不**挂 RateLimit middleware；rate_limit 在 handler 内层，**置于幂等命中预检之后**
   - committed success replay 路径：**跳过** rate_limit（V1 §7.2 r10 契约承诺）
   - **反模式**：本 story 在路由层挂 `middleware.RateLimit(...)` → 违反 r10；用户首次 commit 成功后同 key 退避重试会被 rate_limit 卡死，永远读不到 cached success

7. **1008 错误码退役（r11/r15）**：
   - 节点 7 阶段本接口**不**触发 1008（无可达路径）；步骤 5b 兜底分支按 1009 走
   - V1 §7.2 错误码表 r15 已移除 1008 行（dead path 防 client / 测试方实装 1008 分支）
   - 全局错误码定义保留（codes.go ErrIdempotencyConflict=1008），供未来其他幂等接口可能复用
   - **反模式**：本 service 把 5b 兜底分支翻译为 1008 → 违反 r11

8. **加权抽取通过 picker interface 注入（V1 §7.2.5g + epics.md §20.9 警示）**：
   - service 层**不**直接调 `math/rand` 全局源；`random.WeightedPicker` 注入
   - 生产 `random.NewCryptoWeightedPicker(rand.Reader)`；单测注入确定性 seed source
   - 抽奖不应可预测（开箱业务安全性 + 防作弊；math/rand 默认 seed=1 是可预测序列）
   - **反模式**：service 内 `rand.Intn(totalWeight)` → 违反 V1 §7.2 + epics.md §20.9 警示

9. **节点 7 阶段 `userCosmeticItemId = "0"` 占位（V1 §7.2.4h）**：
   - service 层硬编码 `RewardUserCosmeticItemID: 0` + handler 层字符串化为 `"0"`
   - 节点 8 Story 23.5 修改本事务：先 INSERT user_cosmetic_items 拿到真实 id 再填入；本 story 阶段**禁止**预实装该步骤
   - **反模式**：本 story 提前实装 INSERT user_cosmetic_items → 越界 YAGNI；与节点规划不符

10. **`time.Now().UTC()` 统一注入 `nowFn`**：
    - service struct 字段 `nowFn func() time.Time`；构造期默认 `func() time.Time { return time.Now().UTC() }`
    - 单测可注入 mock 时钟；生产用默认实装
    - handler 同样持有 `nowFn` 用于 openChestResponseDTO 同源同时刻补算
    - **反模式**：service 内多处 `time.Now().UTC()` → 单测不可控 + 同源同时刻不可保证

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md`）：

- chest 是节点 7 核心可消费资产；本 story 是 chest **写入 / 状态变更**链路的实装（与 20.5 状态读取链路并列）
- "状态以 server 为准"原则：开箱事务由 server 单一可信源原子提交

**数据库层**（`docs/宠物互动App_数据库设计.md`）：

- §5.6 user_chests：本 story 消费 FOR UPDATE + Spend + Delete + Create
- §5.7 chest_open_logs：本 story 落 INSERT
- §5.8 cosmetic_items：本 story 消费 ListEnabledForWeightedPick + drop_weight 加权
- §5.4 user_step_accounts：本 story 消费 FOR UPDATE + Spend（自定义方法，**非**复用 7.3 UpdateBalance）
- **§5.16 chest_open_idempotency_records**：本 story 落地 migration + 全部读写
- §8.3 开箱事务：本 story 完整实装事务边界

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：

- §7.2 POST /chest/open r1~r15 完整决策（特别 §7.2.3 / §7.2.5a / §7.2.5b / §7.2.5k）
- §3 错误码表：1001 / 1002 / 1005 / 1009 / 3002 / 4001 / 4002

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：

- §5.1 handler：openChestRequestDTO 解析 + idempotencyKey regex 校验 + handler 内层 rate_limit + 调 service
- §5.2 service：8 步事务严格按 V1 §7.2.5；时间字段同源同时刻补算
- §5.3 repo：本 story 落地 5 个 repo 方法 + 1 个新表 + 1 个新包

**ADR 对齐**：

- ADR-0003 / 数据库设计 §3：不建 FK，应用层校验 + 索引兜底
- ADR-0006 三层错误映射：repo 哨兵 → service `*AppError` → handler envelope
- ADR-0007 ctx 传播：service / repo 第一参数 ctx；handler 用 `c.Request.Context()`；事务内调用用 `txCtx`

### 关于 Story 20.6 与 20.5 / 11.3 的关键差异

| 维度 | 20.6 POST /chest/open | 20.5 GET /chest/current | 11.3 POST /rooms |
|------|---|---|---|
| HTTP method | POST | GET | POST |
| 事务 | **8 步事务** | 无 | 4 步事务 |
| 幂等 | **DB 持久化幂等（§5.16）** | n/a（GET 天然幂等） | n/a |
| rate_limit 位置 | **handler 内层（r10 opt-out）** | middleware 链 | middleware 链 |
| FOR UPDATE 行锁 | **chest + step_account 双锁** | 无 | rooms 单锁 |
| 加权随机 | **是（crypto/rand 注入）** | n/a | n/a |
| 错误码全集 | 1001 / 1002 / 1005 / 1009 / 3002 / 4001 / 4002（**禁止** 1008 / 4003） | 1001 / 1005 / 1009 / 4001 | 1001 / 1005 / 1009 / 6005 |
| 新增 migration | **是（0014）** | 否 | 否（0007/0008 由 10.3 落地） |
| 新增 repo | **5 个**（idempotencyRepo + cosmeticItemRepo + chestOpenLogRepo + 扩 chest_repo + 扩 step_account_repo） | 0（仅消费 4.6 / 4.8 已建） | 1（roomRepo） |
| 新增 pkg | **1（internal/pkg/random）** | 0 | 0 |
| 测试规模 | service 7+ + handler 6+ + repo 14 + random 3 + 集成 2 = **≥32** | service 6 + handler 5 + 集成 2 = 13 | service 7 + handler 6 + 集成 2 = 15 |

### Project Structure Notes

**与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 钦定的 server/ 工程结构对齐**：

```
server/
├─ internal/
│  ├─ app/
│  │  ├─ http/
│  │  │  ├─ handler/
│  │  │  │  ├─ chest_handler.go              # **扩展**：追加 Open + DTO helpers
│  │  │  │  └─ chest_open_handler_test.go    # **新建**：≥6 case
│  │  │  └─ middleware/
│  │  │     └─ rate_limit.go                 # **扩展**：导出 CheckRateLimitByUserID
│  │  └─ bootstrap/
│  │     └─ router.go                        # **修改**：wire 5 新实例 + 独立 chestOpenGroup
│  ├─ service/
│  │  ├─ chest_service.go                    # 既有（20.5 落地 GetCurrent + ChestService interface）
│  │  ├─ chest_open_service.go               # **新建**：OpenChest 实装 + DTO 转换
│  │  ├─ chest_open_service_test.go          # **新建**：≥7 case
│  │  └─ chest_open_service_integration_test.go  # **新建**：≥2 dockertest case
│  ├─ repo/
│  │  └─ mysql/
│  │     ├─ chest_repo.go                    # **扩展**：FindByUserIDForUpdate + Delete
│  │     ├─ step_account_repo.go             # **扩展**：FindByUserIDForUpdate + Spend
│  │     ├─ cosmetic_item_repo.go            # **扩展**：落地 CosmeticItemRepo interface + ListEnabledForWeightedPick
│  │     ├─ cosmetic_item_repo_test.go       # **新建**：≥2 case
│  │     ├─ chest_open_log_repo.go           # **扩展**：落地 ChestOpenLogRepo interface + Create
│  │     ├─ chest_open_log_repo_test.go      # **新建**：≥1 case
│  │     ├─ chest_open_idempotency_record_repo.go      # **新建**：完整 IdempotencyRepo
│  │     ├─ chest_open_idempotency_record_repo_test.go # **新建**：≥6 case
│  │     └─ errors.go                        # **扩展**：ErrIdempotencyRecordNotFound 哨兵
│  └─ pkg/
│     ├─ errors/codes.go                     # 既有（4001 / 4002 / 3002 / 1002 / 1005 / 1009 已注册）
│     └─ random/                             # **新建包**
│        ├─ weighted.go                       # **新建**：WeightedPicker interface + impl
│        └─ weighted_test.go                  # **新建**：≥3 case
└─ migrations/
   ├─ 0014_init_chest_open_idempotency_records.up.sql    # **新建**
   └─ 0014_init_chest_open_idempotency_records.down.sql  # **新建**
```

**预期 git status 文件清单（~17 文件）**：

新建（≥12 个）：
1. `server/migrations/0014_init_chest_open_idempotency_records.up.sql`
2. `server/migrations/0014_init_chest_open_idempotency_records.down.sql`
3. `server/internal/repo/mysql/chest_open_idempotency_record_repo.go`
4. `server/internal/repo/mysql/chest_open_idempotency_record_repo_test.go`
5. `server/internal/repo/mysql/cosmetic_item_repo_test.go`
6. `server/internal/repo/mysql/chest_open_log_repo_test.go`
7. `server/internal/pkg/random/weighted.go`
8. `server/internal/pkg/random/weighted_test.go`
9. `server/internal/service/chest_open_service.go`
10. `server/internal/service/chest_open_service_test.go`
11. `server/internal/service/chest_open_service_integration_test.go`
12. `server/internal/app/http/handler/chest_open_handler_test.go`

修改（≥7 个）：
1. `server/internal/repo/mysql/chest_repo.go` — 追加 FindByUserIDForUpdate + Delete
2. `server/internal/repo/mysql/chest_repo_test.go` — 追加测试
3. `server/internal/repo/mysql/step_account_repo.go` — 追加 FindByUserIDForUpdate + Spend
4. `server/internal/repo/mysql/step_account_repo_test.go` — 追加测试
5. `server/internal/repo/mysql/cosmetic_item_repo.go` — 落地 interface + impl + 方法
6. `server/internal/repo/mysql/chest_open_log_repo.go` — 落地 interface + impl + 方法
7. `server/internal/repo/mysql/errors.go` — 追加 ErrIdempotencyRecordNotFound 哨兵
8. `server/internal/service/chest_service.go` — 扩展 interface + struct 字段 + NewChestService 签名
9. `server/internal/app/http/handler/chest_handler.go` — 追加 Open + DTO helpers + 扩展 struct
10. `server/internal/app/http/middleware/rate_limit.go` — 导出 CheckRateLimitByUserID
11. `server/internal/app/bootstrap/router.go` — wire 5 新实例 + 独立 chestOpenGroup
12. 各 middleware / handler / router 既有测试文件可能需要回归调整（因 signature 变更）

流程文件（2 个）：
1. `_bmad-output/implementation-artifacts/sprint-status.yaml` — 20-6 状态流转
2. `_bmad-output/implementation-artifacts/20-6-post-chest-open-事务-idempotencykey-加权抽取.md` — 本文件

**未变更文件（明确 NOT touch；超出范围 → HALT）**：

- `server/cmd/server/main.go`（无新 Deps 字段）
- `server/internal/app/bootstrap/server.go`
- `server/internal/infra/config/*.go`（无新配置项）
- `server/internal/pkg/errors/codes.go`（错误码已注册，**禁止**新增）
- `server/internal/app/http/handler/home_handler.go` / `steps_handler.go` 等其他 handler
- `server/internal/service/auth_service.go` / `home_service.go` / `step_service.go` / `room_service.go` 等其他 service
- `docs/宠物互动App_*.md`（契约**输入**侧）
- `_bmad-output/planning-artifacts/epics.md`
- `_bmad-output/implementation-artifacts/decisions/*.md`（无新决策）
- `local.yaml` / 任一 `*.yaml`（无新配置项）

### References

**优先级 P0（必读）**：

- [Source: docs/宠物互动App_V1接口设计.md#7.2 开启宝箱] — 接口契约定义（行 919-1213）；**特别**：
  - 行 949-995：服务端逻辑步骤 1-8（含 r10/r11 决策）
  - 行 997-1013：事务边界规则 + r5/r6/r7 决策
  - 行 1015-1028：rate_limit r10/r11 决策
  - 行 1030-1046：MVCC pending 不可见 + 1008 退役决策
  - 行 1126-1140：错误码表 + r15 1008 移除注解
- [Source: docs/宠物互动App_数据库设计.md#5.16 chest_open_idempotency_records] — 表结构（行 727-785）+ 设计说明决策史 r5/r6/r7/r9/r10/r11
- [Source: docs/宠物互动App_数据库设计.md#8.3 开箱事务] — 事务边界（行 980-997）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 20.6] — 本 story 钦定 AC（行 2899-2930）
- [Source: _bmad-output/implementation-artifacts/20-1-接口契约最终化.md] — 20.1 r1~r15 review 决策完整记录（本 story 严格遵循）
- [Source: server/internal/service/chest_service.go] — 既有 ChestService interface + chestServiceImpl（本 story 扩展）
- [Source: server/internal/app/http/handler/chest_handler.go] — 既有 ChestHandler（本 story 追加 Open 方法）

**优先级 P1（参考）**：

- [Source: server/internal/service/step_service.go] — SyncSteps 事务模式参考（txMgr.WithTx + 业务步骤 + 错误翻译）
- [Source: server/internal/service/room_service.go] — JoinRoom 多步事务参考（FOR UPDATE + 业务规则失败哨兵 + 多 repo 协作）
- [Source: server/internal/repo/mysql/room_repo.go] — FindByIDForUpdate clause.Locking 模式（本 story chest_repo FOR UPDATE 同模式）
- [Source: server/internal/repo/mysql/chest_repo.go] — 既有 ChestRepo + UserChest struct + Create / FindByUserID（本 story 扩展）
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go] — 既有 CosmeticItem struct（本 story 落地 interface + impl）
- [Source: server/internal/repo/mysql/chest_open_log_repo.go] — 既有 ChestOpenLog struct（本 story 落地 interface + impl）
- [Source: server/internal/app/http/middleware/rate_limit.go] — 既有 RateLimit middleware（本 story 导出 CheckRateLimitByUserID）
- [Source: server/internal/app/bootstrap/router.go] — 既有 router wiring（本 story 修改 chestSvc / chestHandler 构造 + 独立 chestOpenGroup）
- [Source: server/internal/repo/tx/manager.go] — txMgr.WithTx + tx.FromContext 模式
- [Source: server/internal/pkg/errors/codes.go] — 错误码定义（4001 / 4002 / 3002 / 1002 / 1005 / 1009 已注册）
- [Source: _bmad-output/implementation-artifacts/20-5-get-chest-current-接口.md] — 20.5 实装文档（本 story handler / service 模式延续；本 story 关键差异参考表）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-mapping.md] — ADR-0006 三层错误映射
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md] — ADR-0007 ctx 传播

**优先级 P2（背景）**：

- [Source: docs/宠物互动App_时序图与核心业务流程设计.md] — §8 开箱事务时序
- [Source: docs/宠物互动App_总体架构设计.md] — "状态以 server 为准"原则
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5] — 分层职责定义
- [Source: _bmad-output/implementation-artifacts/20-4-chest_open_logs-migration.md] — 前序 story（chest_open_logs migration owner）
- [Source: _bmad-output/implementation-artifacts/20-3-cosmetic_items-seed.md] — 前序 story（cosmetic_items seed owner）
- [Source: _bmad-output/implementation-artifacts/11-4-加入房间事务.md] — 多步事务参考（FOR UPDATE + 业务规则 + 错误翻译）

**Lessons（V1 §7.2 r5/r6/r7/r9/r11 决策来源）**：

- [Source: docs/lessons/2026-05-14-db-same-tx-idempotency-replaces-redis-writeback-fragility-20-1-r5.md] — r5 决策
- [Source: docs/lessons/2026-05-14-idempotency-pre-claim-must-be-inside-business-tx-20-1-r6.md] — r6 决策
- [Source: docs/lessons/2026-05-14-idempotency-no-async-failed-compensation-and-no-cached-requestId-20-1-r7.md] — r7 决策
- [Source: docs/lessons/2026-05-14-time-derived-fields-exhaustive-exclusion-from-idempotency-cache-20-1-r9.md] — r9 决策
- [Source: docs/lessons/2026-05-14-mvcc-invisibility-of-uncommitted-pending-rows-20-1-r11.md] — r11 决策

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash scripts/build.sh --test`: all packages PASS（unit tests ≥40 case 全绿）
- `bash scripts/build.sh --integration`: BUILD SUCCESS（dockertest case 因 Windows docker 不可用 t.Skip；CI Linux 跑）
- `bash scripts/build.sh`: vet + build OK

### Completion Notes List

✅ **AC1**: 0014 migration up/down + IdempotencyRepo（FindByUserIDAndKey + ClaimPending + MarkSuccess）+ 7 sqlmock case 全过。schema 1:1 匹配 DB §5.16 二态机 + UNIQUE uk_user_id_key + idx_status_created_at。

✅ **AC2**: chest_open_service.go 实装 8 步事务（V1 §7.2.5）：committed success replay 短路 / 5a ClaimPending（事务首条）/ 5c chestRepo.FindByUserIDForUpdate / 5d unlockable 判定 / 5e stepAccountRepo.FindByUserIDForUpdate / 5f Spend 乐观锁 / 5g 加权抽取 cosmetic / 5h chest_open_log 写入 / 5i DELETE 旧 chest + INSERT 新 chest / 5j 序列化 / 5k MarkSuccess。response_json 缓存严格不含 nextChest.status / remainingSeconds / requestId（V1 r9/r11）。

✅ **AC3**: chest_handler.Open + router.go chestOpenGroup（仅 Auth，不挂 RateLimit）+ middleware.CheckRateLimitByUserID（userIDRateChecker struct + sync.Once 单例 + 跨包 Reset hook）。同源同时刻补算 nextChest.status / remainingSeconds（openChestResponseDTO 接 now 参数）。

✅ **AC4**: chest_open_service_test.go 13 case 全过：HappyPath_FirstTime / IdempotencyReplay_CachedSuccess / ChestNotFound_4001 / ChestNotUnlockable_4002 / InsufficientSteps_3002 / StepAccountNotFound_1009 / OptimisticLockFails_1009 / NoEnabledCosmetic_1009 / IdempotencyClaim_ExistingRow_ShortCircuitReplay / WeightedPicker_IndexReverseMapsToItem / CachedPending_Returns1009 / UserIDZero_Returns1009 / IdempotencyFindDBError_Returns1009。

✅ **AC5**: chest_open_handler_test.go 10 case 全过（含 InvalidIdempotencyKey 4 子 case 与 NextChestStatusAndRemainingSeconds 2 子 case）。

✅ **AC6**: 14 repo case（idempotency 7 + chest_repo 3 新 + step_account 4 新 + cosmetic 2 + chest_open_log 1）。

✅ **AC7**: random/weighted 包 4 case（SingleItem / DistributionDeterministicSeed / EmptyItems / ZeroTotalWeight）。

✅ **AC8**: chest_open_service_integration_test.go 2 case（HappyPath_FullFlow + IdempotencyReplay_SameKey）；Windows docker 不可用 → t.Skip。

✅ **AC9**: bash scripts/build.sh / --test / --integration 全部 BUILD SUCCESS。

✅ **AC10 验证清单 15 项**：
| # | 验证项 | 结果 |
|---|---|---|
| 1 | §5.16 migration 0014 schema 1:1 匹配（status ENUM 二态机 / response_json JSON / UNIQUE uk_user_id_key） | ✅ |
| 2 | IdempotencyRepo 三方法签名 + 行为严格按 V1 §7.2 r5/r6/r7/r11 | ✅ |
| 3 | service.OpenChest 步骤 5a (ClaimPending) 是事务首条语句 | ✅（chest_open_service.go runOpenChestTx 之前先 ClaimPending） |
| 4 | 5b 短路分支兜底：status='pending' → 1009（**非** 1008） | ✅（service 内显式 apperror.ErrServiceBusy） |
| 5 | 5g 加权抽取通过 random.WeightedPicker.Pick；service 不直接调 math/rand | ✅（Grep 验证）|
| 6 | 5h chest_open_logs.reward_user_cosmetic_item_id 固定 0 占位（节点 7） | ✅（service 内硬编码 0）|
| 7 | 5k MarkSuccess + 业务事务在同事务原子 commit | ✅（runOpenChestTx 最后调 MarkSuccess 后 return nil → txMgr 自动 commit）|
| 8 | response_json 缓存不含 nextChest.status / nextChest.remainingSeconds / requestId | ✅（cacheableNextChestDTO 无该字段 + 单测 HappyPath_FirstTime 断言验证）|
| 9 | handler.Open 内层 rate_limit：未命中 success 时调；命中 success 跳过 | ✅（handler case 2 验证严苛 cfg 下 5 次 cached replay 全过）|
| 10 | handler.openChestResponseDTO 用 now 参数同源同时刻补算 status + remainingSeconds | ✅（DTO 接 now time.Time 参数 + case 8 dynamic 验证）|
| 11 | idempotencyKey regex 校验 [A-Za-z0-9_:-]{1,128} | ✅（idempotencyKeyRegex package-level var + case 4 子测试覆盖空 / 含空格 / 超长 / 非 ASCII）|
| 12 | router.go: chest/open **不**在 authedGroup（独立 chestOpenGroup 仅挂 Auth） | ✅（router.go chestOpenGroup 仅 middleware.Auth）|
| 13 | router.go: chestSvc / chestHandler 构造签名扩展正确；wire 4 新实例 | ✅（cosmeticItemRepo / chestOpenLogRepo / idempotencyRepo / weightedPicker）|
| 14 | 错误码：4001 / 4002 / 3002 / 1009 / 1002 / 1005；**不**触发 1008 / 4003 | ✅（service 翻译路径 Grep ErrIdempotencyConflict / ErrChestNotOpenable 在 chest_open_service.go 0 命中）|
| 15 | `bash scripts/build.sh --test` 全绿 | ✅（all packages PASS）|

✅ **AC11**: 本 story 不 commit；commit hash 待 story-done 阶段回填。

### File List

**新建（≥12 个）**:
1. `server/migrations/0014_init_chest_open_idempotency_records.up.sql`
2. `server/migrations/0014_init_chest_open_idempotency_records.down.sql`
3. `server/internal/repo/mysql/chest_open_idempotency_record_repo.go`
4. `server/internal/repo/mysql/chest_open_idempotency_record_repo_test.go`
5. `server/internal/repo/mysql/cosmetic_item_repo_test.go`
6. `server/internal/repo/mysql/chest_open_log_repo_test.go`
7. `server/internal/pkg/random/weighted.go`
8. `server/internal/pkg/random/weighted_test.go`
9. `server/internal/service/chest_open_service.go`
10. `server/internal/service/chest_open_service_test.go`
11. `server/internal/service/chest_open_service_integration_test.go`
12. `server/internal/app/http/handler/chest_open_handler_test.go`
13. `server/internal/app/http/middleware/rate_limit_checker_test.go`

**修改（≥10 个）**:
1. `server/internal/repo/mysql/chest_repo.go` — 追加 FindByUserIDForUpdate + Delete
2. `server/internal/repo/mysql/chest_repo_test.go` — 追加 3 case
3. `server/internal/repo/mysql/step_account_repo.go` — 追加 FindByUserIDForUpdate + Spend
4. `server/internal/repo/mysql/step_account_repo_test.go` — 追加 4 case
5. `server/internal/repo/mysql/cosmetic_item_repo.go` — 落地 CosmeticItemRepo interface + impl + ListEnabledForWeightedPick
6. `server/internal/repo/mysql/chest_open_log_repo.go` — 落地 ChestOpenLogRepo interface + impl + Create
7. `server/internal/repo/mysql/errors.go` — 追加 ErrIdempotencyRecordNotFound 哨兵
8. `server/internal/service/chest_service.go` — 扩展 ChestService interface + chestServiceImpl 字段 + NewChestService 签名 7 参数
9. `server/internal/app/http/handler/chest_handler.go` — 追加 Open 方法 + DTO helpers + 扩展 struct + 签名 3 参数
10. `server/internal/app/http/middleware/rate_limit.go` — 导出 CheckRateLimitByUserID + userIDRateChecker + ResetForTest hook
11. `server/internal/app/bootstrap/router.go` — wire 4 新实例 + 独立 chestOpenGroup
12. `server/internal/app/http/handler/chest_handler_test.go` — stubChestService 扩展 OpenChest 方法 + NewChestHandler 签名适配
13. `server/internal/service/chest_service_test.go` — newChestServiceForGetCurrent helper 适配 7 参数签名
14. `server/internal/service/chest_service_integration_test.go` — NewChestService 7 参数适配
15. `server/internal/service/auth_service_test.go` — stubStepAccountRepo / stubChestRepo 添加 panic-default 新方法
16. `server/internal/service/auth_service_integration_test.go` — faultChestRepo 适配新方法
17. `server/internal/service/home_service_test.go` — stubHomeStepAccountRepo / stubHomeChestRepo 添加 panic-default 新方法
18. `server/internal/service/step_service_test.go` — stubStepStepAccountRepo 添加 panic-default 新方法

**流程文件（2 个）**:
1. `_bmad-output/implementation-artifacts/sprint-status.yaml` — 20-6 状态流转 ready-for-dev → in-progress → review
2. `_bmad-output/implementation-artifacts/20-6-post-chest-open-事务-idempotencykey-加权抽取.md` — 本文件

### Change Log

| 日期 | 改动 | 状态变更 |
|---|---|---|
| 2026-05-14 | Story 20.6 created (backlog → ready-for-dev)；§5.16 migration scope 决策 = 选项 A（紧耦合，与 service / handler 同 story 落地） | backlog → ready-for-dev |
| 2026-05-15 | dev-story 实装完成：0014 migration + IdempotencyRepo + 4 repo 扩展 + random/weighted 包 + chest_open_service (8 步事务) + handler.Open + middleware.CheckRateLimitByUserID + router chestOpenGroup + 13 service case + 10 handler case + 14 repo case + 4 random case + 2 集成 case；`bash scripts/build.sh --test` / `--integration` 全绿 | in-progress → review |
