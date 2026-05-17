# Story 23.5: 修改开箱事务 - 创建 user_cosmetic_items 实例 + 补 chest_open_logs.reward_user_cosmetic_item_id（入仓）（在 Story 20.6 已落地的 8 步开箱事务 runOpenChestTx 内、抽奖产出 pickedItem 之后 / 写 chest_open_logs 之前插入 1 条 user_cosmetic_items INSERT 拿真实 id，回填 chest_open_logs.RewardUserCosmeticItemID + OpenChestOutput.Reward.UserCosmeticItemID + cacheableResponse 缓存值，全部在既有 txCtx 同事务原子提交；UserCosmeticItemRepo 新增 CreateInTx 写方法 + chestServiceImpl 注入新 repo 依赖 + NewChestService 扩签名 + router.go / 全部 chest_open 单测 fixture 同步改 + 激活 Story 20.8 dev/grant-cosmetic-batch 真实写库 + 扩 Story 20.9 Layer 2 集成测试新增 user_cosmetic_items 入仓 + 回滚两组场景）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 资产事务负责人,
I want 修改 **Story 20.6 已落地的 `ChestService.OpenChest` 8 步开箱事务**（具体在 `server/internal/service/chest_open_service.go` 的 `runOpenChestTx` 步骤 5g 抽奖产出 `pickedItem` 之后、步骤 5h 写 `chest_open_logs` 之前），**插入 1 条 `user_cosmetic_items` INSERT**（`user_id` / `cosmetic_item_id=pickedItem.ID` / `status=1 in_bag` / `source=1 chest` / `source_ref_id=chest.ID` / `obtained_at=now`），拿到 GORM 回填的 `user_cosmetic_items.id`，把这个**真实 id** 回填三处（① `chest_open_logs.RewardUserCosmeticItemID`（节点 7 阶段固定 `0`）② `OpenChestOutput.Reward.UserCosmeticItemID`（节点 7 阶段固定 `0`）③ `cacheableResponse` 缓存值，保证幂等 replay 也返回真实 id），并为此**扩展既有 `UserCosmeticItemRepo` interface 新增 `CreateInTx` 写方法**（Story 23.4 已落地的同名 repo / 同文件 `user_cosmetic_item_repo.go` 当前只有 `ListByUserForInventory` 只读方法，本 story 在该 interface **追加** `CreateInTx`，**不**改既有只读方法）+ `chestServiceImpl` 加 `userCosmeticItemRepo mysql.UserCosmeticItemRepo` 字段 + `NewChestService` **扩签名**新增第 8 参数 `userCosmeticItemRepo`（**关键回归点**：`NewChestService` 现有 7 参构造被 `router.go` line 493 + `chest_open_service_test.go` `fixtureService` line 219-231 调用，扩签名后这两处 + 所有测试 fixture 必须同步改否则 build 红）+ `router.go` wire 复用 line 517 既有 `userCosmeticItemRepo` 实例注入 `chestSvc`（**注**：line 517 该实例当前只注入 `cosmeticSvc`，本 story 让 `chestSvc` 也复用同实例，**不**新建第二个 `userCosmeticItemRepo`）+ **激活 Story 20.8 `/dev/grant-cosmetic-batch` 真实写库**（`dev_cosmetic_service.go` 当前是节点 7 stub 返 `ErrNotImplemented(1010)`，本 story 移除 stub 分支 → 注入 `cosmeticItemRepo` + `userCosmeticItemRepo` → 实装"按 rarity 随机抽 count 个 cosmetic_item_id + BatchCreate user_cosmetic_items"，详见 AC6）+ 扩 Story 20.6 单测 `chest_open_service_test.go`（新建 `stubUserCosmeticItemRepo` + 既有全部 happy fixture + `fixtureService` 扩参，新增 ≥4 case）+ 扩 Story 20.9 Layer 2 集成测试 `chest_open_service_integration_test.go`（新增"开箱后 user_cosmetic_items 多 1 行 + reward_user_cosmetic_item_id 非零"happy 场景 + "任一步失败时 user_cosmetic_items 也回滚"场景）,

so that **节点 8 §4.8 验收的"开箱 → 入仓 → 仓库可见"全链路**（Story 23.4 `GET /cosmetics/inventory` 能查到新入仓实例 / Epic 24 仓库页能渲染 / Epic 25 E2E 联调通过 / Epic 27 穿戴有实例可穿 / Epic 33 合成有 in_bag 实例可选）成立，并让 Story 20.8 `/dev/grant-cosmetic-batch` 解除 stub 阻塞（节点 11 合成 demo 不必反复开箱凑齐 10 件 common），不再出现"开箱弹了奖励弹窗但仓库永远空（reward 不入仓）/ chest_open_logs.reward_user_cosmetic_item_id 永久为 0 无法关联实例 / 幂等 replay 返回的 userCosmeticItemId 与首次开箱不一致（缓存值没回填真实 id）/ user_cosmetic_items INSERT 写在 txCtx 之外的独立连接导致任一步失败 user_cosmetic_items 不回滚（孤儿实例 + 步数没扣的数据不一致）/ Story 20.6 已历经多轮 race-fix 的扣步数·幂等预声明·乐观锁·刷新下一轮·MarkSuccess 同事务原子性被本次改动破坏导致重复出箱 / NewChestService 扩签名漏改 router.go / fixtureService 导致 build 红 / Story 20.9 既有集成测试场景被改坏 / dev grant-cosmetic-batch 节点 11 仍 501 阻塞合成 demo"的返工。

## 故事定位（Epic 23 第五条 = 最后一条 = 节点 8 的"入仓闭环"关键 story；上承 23.2 持久化根基 + 23.4 已落地 UserCosmeticItemRepo 只读侧 + Story 20.6 已落地 8 步开箱事务，本 story 给该事务补"写 user_cosmetic_items 实例"这一步 + 回填 reward id，让节点 7 故意延期的"入仓"逻辑落地；完成后 epic-23 全 done）

- **Epic 23 进度**：23.1（契约定稿 §8.1 / §8.2 + §7.2.6 节点 7→8 升级路径冻结，done）→ 23.2（user_cosmetic_items migration `0015` + `UserCosmeticItem` GORM struct，done）→ 23.3（GET /cosmetics/catalog，done）→ 23.4（GET /cosmetics/inventory + **首次落地 `UserCosmeticItemRepo` interface（只读 `ListByUserForInventory`）**，done）→ **23.5（本 story = epic-23 最后一条；修改 Story 20.6 开箱事务补"入仓"+ 激活 Story 20.8 dev 写库）**。本 story done 后 epic-23 retrospective 可启动。
- **本 story 修改的核心是 Story 20.6 历经 r1~r15 多轮 race-fix 的开箱事务**（`chest_open_service.go`，388 行）。这是全仓库**事务正确性最敏感的一段代码**：幂等预声明（5a）/ 短路 replay（5b）/ FOR UPDATE 锁（5c·5e）/ 乐观锁扣步数（5f）/ 加权抽取（5g）/ 写日志（5h）/ 刷新下一轮（5i）/ MarkSuccess（5k）全部在 `txMgr.WithTx` 单事务原子提交。**本 story 唯一允许的改动是"在 5g 与 5h 之间插入一步 user_cosmetic_items INSERT + 把拿到的 id 回填 5h 的 logRow + 5j 的 output + buildCacheableResponse 的缓存值"**。**禁止改动 5a~5f / 5i / 5k 的任何逻辑、错误码、错误翻译、事务边界、字段语义**（详见下方"高危纠偏点"）。
- **节点 7 vs 节点 8 阶段差异（契约层钦定，V1 §7.2.6 行 1151-1155 + DB §8.3 行 995 + V1 §7.2.4h 行 985-990）**：
  - 节点 7 阶段（Story 20.6 已交付）：开箱**不入仓** —— `chest_open_logs.reward_user_cosmetic_item_id` 写占位 `0`，`reward.userCosmeticItemId` 返回字符串 `"0"`
  - 节点 8 阶段（**本 story**）：开箱事务补"创建 user_cosmetic_items 实例 → 拿真实 id → 回填 chest_open_logs + response"
  - **契约层升级路径（V1 §7.2.6 行 1155 钦定）**：节点 7 → 节点 8 升级**不**视为契约变更（`userCosmeticItemId` 类型 / 字段名 / 必填性都不变，只是 server 端语义从"占位"变成"真实主键"）；**Story 23.5 落地时应在 V1 §7.2.6 中标注升级日期 + commit hash**（见 AC8 文档同步注意 —— 但本 story 范围红线**不**改 docs；改契约文档归 Story 25.3 文档同步 story；本 story 仅在 Completion Notes 记录"§7.2.6 待 25.3 标注升级日期"）
- **本 story 是 iOS Epic 24 / Epic 25 / Epic 27 / Epic 33 + Story 20.8 的强前置**：
  - **Epic 25（仓库链路 E2E）**："开箱 → 入仓（本 story）→ 仓库可见（23.4 GET /cosmetics/inventory）"全链路；Story 25.1 验证场景 8 钦定"开箱后 mysql 查 user_cosmetic_items 表数量与仓库一致 + chest_open_logs.reward_user_cosmetic_item_id 不为空"
  - **Story 20.8（dev/grant-cosmetic-batch）**：本 story 完成后激活真实写库（`dev_cosmetic_service.go` 行 32-39 + 110-118 注释明确"节点 8 / Epic 23.5 阶段由 23.5 owner 在本 service 内激活"）；节点 11 合成 demo 必备
  - **Epic 24 / 27 / 33**：仓库页 / 穿戴页 / 合成页都需要"开箱真的产出实例"才有数据
- **epics.md §Story 23.5 钦定**（行 3295-3320）：
  - 在抽奖产出 cosmetic_item_id 之后、写 chest_open_logs 之前插入新步骤：INSERT user_cosmetic_items `(user_id, cosmetic_item_id, status=1 in_bag, source=1 chest, source_ref_id=chest_id, obtained_at=now)` → 拿生成的 user_cosmetic_item_id → 写 chest_open_logs 时填这个真实 id（之前是 0）→ response.reward.userCosmeticItemId 也填真实 id
  - **不破坏 Story 20.6 其他逻辑**：扣步数、idempotencyKey、抽奖分布、刷新下一轮 chest 全部不变
  - **不破坏 Layer 2 集成测试 Story 20.9**：现有测试场景仍通过 + 新增"开箱后 user_cosmetic_items 多 1 行"
  - 单测覆盖 ≥4 case；集成测试 dockertest；Layer 2 新增"user_cosmetic_items 也回滚"
  - 本 story 完成后 Story 20.8（`/dev/grant-cosmetic-batch`）打开真实写库逻辑（之前是 placeholder）
- **关键纠偏：epics.md §Story 23.5 AC 文字与 V1 §7.2 / DB §8.3 锚定的事务边界一致；但 source_ref_id 取值有 disambiguation 点**：epics.md 行 3306 写 `source_ref_id=chest_id`，DB §5.9 字段说明行 512 写"来源关联记录 id"。**本 story 取 `source_ref_id = chest.ID`（被开启的那个宝箱实例 id，与 chest_open_logs.chest_id 同值）**，与 epics.md 行 3306 一致，与 `UserCosmeticItem` struct 注释行 27-29（"开箱时=chest_id 非空"）一致。`source=1`（chest，§6.11 枚举 + struct 注释行 24-25 钦定 1=chest）。这两者契约一致无冲突，但 dev **禁止**自行改成 chest_open_logs.id 或 NULL（前者是日志 id 非来源宝箱 id，后者违背"开箱来源可追溯"语义）。
- **范围红线（只改/新建以下文件）**：
  - **改** `server/internal/repo/mysql/user_cosmetic_item_repo.go`（在既有 `UserCosmeticItemRepo` interface **追加** `CreateInTx` 写方法 + impl；**不**改既有 `UserCosmeticItem` struct / `TableName()` / `ListByUserForInventory` interface 注释 / impl / `userCosmeticItemRepo` struct / `NewUserCosmeticItemRepo` 构造）
  - **改** `server/internal/repo/mysql/user_cosmetic_item_repo_test.go`（补 `CreateInTx` repo 层测试；**不**改既有 `ListByUserForInventory` 测试）
  - **改** `server/internal/service/chest_service.go`（`chestServiceImpl` struct 加 `userCosmeticItemRepo mysql.UserCosmeticItemRepo` 字段 + `NewChestService` 扩签名加第 8 参数 + 注释更新；**不**改 `ChestService` interface 方法签名 / `GetCurrent` impl）
  - **改** `server/internal/service/chest_open_service.go`（`runOpenChestTx` 在 5g 之后 5h 之前插入 user_cosmetic_items INSERT 步骤 + 回填 5h `logRow.RewardUserCosmeticItemID` + 5j `output.Reward.UserCosmeticItemID`；`buildCacheableResponse` 已透传 `out.Reward.UserCosmeticItemID` 无需改（验证）；`replayFromCachedResponse` 已透传 `cached.Data.Reward.UserCosmeticItemID` 无需改（验证）；**不**改 5a~5f / 5i / 5k 任何逻辑 / 错误码 / 事务边界）
  - **改** `server/internal/service/chest_open_service_test.go`（新建 `stubUserCosmeticItemRepo`（实现扩展后的 `UserCosmeticItemRepo` interface —— `ListByUserForInventory` panic 不期望 + `CreateInTx` 可注入 fn）+ `happyUserCosmeticItemRepo()` helper + `fixtureService` 扩参第 8 个 + 既有全部 case 的 `fixtureService(...)` 调用同步改 + 新增 ≥4 case）
  - **改** `server/internal/service/chest_open_service_integration_test.go`（新增 happy："1500 步 + force-unlock → 开箱 → user_cosmetic_items 多 1 行 + reward_user_cosmetic_item_id 非零"+ 回滚："任一步 mock 失败 → user_cosmetic_items 也回滚 + 步数不变 + chest 仍 unlockable"；既有场景全部保持通过，**不**改既有 case 断言除非 NewChestService 扩参强制改 fixture 构造）
  - **改** `server/internal/service/chest_service_test.go`（若该文件内有 `NewChestService(...)` 调用，扩参同步改 —— dev 先 grep `NewChestService(` 全仓库找全部调用点再逐一改）
  - **改** `server/internal/service/dev_cosmetic_service.go`（移除节点 7 stub 返 1010 分支 → `devCosmeticServiceImpl` 加 `cosmeticItemRepo` + `userCosmeticItemRepo` 字段 → `NewDevCosmeticService` 扩签名 → `GrantCosmeticBatch` 实装真实写库；详见 AC6）
  - **改** `server/internal/service/dev_cosmetic_service_test.go`（把节点 7 阶段"断言 1010 + HTTP 501"的 case 改为"happy path return nil + repo 被调"；扩 stub）
  - **改** `server/internal/repo/mysql/cosmetic_item_repo.go`（**若** AC6 dev 写库需要"按 rarity 随机抽 count 个"方法且现有 `CosmeticItemRepo` interface 无此方法，**则**新增 `FindRandomByRarity` 方法 + impl；dev 先读 `cosmetic_item_repo.go` 确认现有方法集 —— 现有 `ListEnabledForWeightedPick` / `ListEnabledForCatalog` / `ListByIDsForInventory`，**无**按 rarity 随机抽方法 → 需新增；详见 AC6）
  - **改** `server/internal/repo/mysql/cosmetic_item_repo_test.go`（补 `FindRandomByRarity` 测试，若 AC6 新增该方法）
  - **改** `server/internal/app/bootstrap/router.go`（`NewChestService` wire 扩第 8 参 `userCosmeticItemRepo` —— 复用 line 517 既有实例（**不**新建第二个）；`NewDevCosmeticService` wire 扩参注入 `cosmeticItemRepo` + `userCosmeticItemRepo` —— 见 line 383 注释提示的目标签名）
  - 本 story 文件 + sprint-status.yaml 流转
  - **不**改 Story 20.6 `chest_open_service.go` 5a~5f / 5i / 5k 逻辑 / 错误码 / 错误翻译 / 事务边界 / `cacheableResponse` JSON tag / `OpenChestInput` / `OpenChestOutput` / `ChestRewardBrief` 字段定义（仅回填 `UserCosmeticItemID` 的赋值语句从固定 `0` 改为真实 id；字段本身 Story 20.6 已预留 `UserCosmeticItemID uint64`）
  - **不**改 `ChestService` interface 的 `GetCurrent` / `OpenChest` 方法签名（contract 稳定；扩的是构造函数 `NewChestService` 签名，不是 interface 方法）
  - **不**改 V1 §7.2 / §8.2 契约 / §5.9 user_cosmetic_items schema / §5.7 chest_open_logs schema / 任何 `docs/宠物互动App_*.md`（节点 7→8 升级 V1 §7.2.6 标注升级日期归 Story 25.3 文档同步；本 story 仅在 Completion Notes 记录"待 25.3 标注"）
  - **不**改 0001~0015 既有 migration / seed（user_cosmetic_items 表 23.2 `0015` 已建；chest_open_logs 表 20.4 `0013` 已建；本 story 仅写数据不改 schema）
  - **不**新建 `internal/domain/cosmetic/` 目录（沿用 23.3 / 23.4 已确立扁平工程现状 —— `internal/service/` + `internal/repo/mysql/`，**无** `internal/domain/`；与 ADR-0006 / 23.3 / 23.4 实装一致）
  - **不**写英文版测试注释 / 文档（项目 communication_language=Chinese）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除本 story 文件 + sprint-status.yaml 流转）

**本 story 不做**（明确范围红线，再次强调）：

- 不改 Story 20.6 开箱事务 5a~5f（幂等预声明 / 短路 replay / FOR UPDATE / 乐观锁扣步数）/ 5i（刷新下一轮 chest）/ 5k（MarkSuccess）的任何逻辑 / 错误码 / 错误翻译 / 事务边界 —— 这些是 r1~r15 多轮 race-fix 锁定的不变量，唯一允许的改动是 5g 与 5h 之间插一步 INSERT + 回填 3 处 id
- 不把 user_cosmetic_items INSERT 写在 `txMgr.WithTx` 之外的独立连接 / 独立事务（必须用 `runOpenChestTx` 传入的 `txCtx`，与 5h chest_open_logs / 5f 扣步数 / 5i 刷新 chest 同事务原子提交 —— ADR-0007 §2.4 + DB §8.3"全部同事务"钦定；任一步失败 user_cosmetic_items 必须跟随回滚）
- 不改 `ChestService` interface 方法签名 / `OpenChestInput` / `OpenChestOutput` / `ChestRewardBrief` 字段（`UserCosmeticItemID uint64` Story 20.6 已预留，本 story 仅改其赋值来源）
- 不改 `cacheableResponse` / `cacheableRewardDTO` JSON tag / 结构（`userCosmeticItemId` tag 已存在；缓存写入 / replay 读出已透传该字段 —— 本 story 改完 output.Reward.UserCosmeticItemID 为真实 id 后，buildCacheableResponse 自动缓存真实 id，replayFromCachedResponse 自动返回真实 id，**无需改这两个 helper**，只需验证透传链正确）
- 不改 V1 §7.2 / §8.2 / DB §5.9 / §5.7 / §8.3 契约文档（schema / 契约输入，严格对齐不修改；节点 7→8 升级 §7.2.6 标注归 Story 25.3）
- 不新建 migration / seed（表 23.2 / 20.4 已建）
- 不引入 `internal/domain/cosmetic/` 新目录（沿用 23.3 / 23.4 扁平工程现状）
- 不为 user_cosmetic_items INSERT 做 backfill 历史 chest_open_logs.reward_user_cosmetic_item_id=0 的旧记录（epics.md 行 3204 / 行 3487 / Story 25.3 tech-debt 钦定"接受历史 0/NULL，新记录正常填值"；本 story 只保证新开箱正确，**不**回填历史）
- 不改 Story 20.8 `/dev/grant-cosmetic-batch` 的路由 / handler DTO / 接口签名 / 客户端调用代码（`dev_cosmetic_service.go` 行 39 钦定"接口签名 / 路由 / 客户端调用代码不变 → 兼容已部署 e2e 脚本"；本 story 仅激活 service 层真实写库 + 改构造函数签名 + 改 service 单测）
- 不为开箱事务写 stress / fuzz / 并发压测（节点 8 阶段 schema 稳定 + 单测 + dockertest 集成已覆盖核心约束 + Story 20.6 r1~r15 已锁定并发正确性；与 17.4 / 20.5 / 23.3 / 23.4 一致）
- 不预实装 Epic 26 穿戴事务 / Epic 32-33 合成事务对 user_cosmetic_items 的 status 推进 / consumed 写入（YAGNI；本 story 只写 status=1 in_bag 入仓实例，status 1↔2↔3 推进归后续 epic）

## Acceptance Criteria

**AC1 — `user_cosmetic_item_repo.go` 的 `UserCosmeticItemRepo` interface 追加 `CreateInTx` 写方法（开箱事务步骤"插入实例"数据出口）**

修改 `server/internal/repo/mysql/user_cosmetic_item_repo.go`，在既有 `UserCosmeticItemRepo` interface（Story 23.4 落地，现仅含 `ListByUserForInventory`）**追加**：

```go
// CreateInTx 在事务内插入一行 user_cosmetic_items（开箱事务"创建实例"步骤数据出口，
// epics.md §Story 23.5 + V1 §7.2.4h 节点 8 + DB §8.3 钦定）。
//
// GORM 在成功后回填 item.ID（AUTO_INCREMENT）—— 调用方拿这个真实 id 回填
// chest_open_logs.reward_user_cosmetic_item_id + response.reward.userCosmeticItemId。
//
// **必须在事务内调用**（与同事务的扣步数 / 写 chest_open_logs / 刷新 chest /
// MarkSuccess 一起原子提交；ADR-0007 §2.4 + DB §8.3"全部同事务"——
// 任一步失败本 INSERT 必须跟随回滚，杜绝"孤儿实例 + 步数没扣"数据不一致）。
//
// query 失败 → 返 raw error 透传（service 包成 1009，与同事务其他写步骤一致）。
CreateInTx(ctx context.Context, item *UserCosmeticItem) error
```

- impl `(r *userCosmeticItemRepo) CreateInTx`：`db := tx.FromContext(ctx, r.db)` → `return db.WithContext(ctx).Create(item).Error` —— 与同目录 `chest_open_log_repo.go` `Create`（行 86-89）**1:1 同模式**（`tx.FromContext` 拿事务句柄 → `WithContext` → `Create` → GORM 自动回填 `item.ID`）。
- **关键：必须用 `tx.FromContext(ctx, r.db)`**（事务内调用走 txCtx 注入的 tx 句柄；与 `chest_open_log_repo.go` / `chest_repo.go` Create / `cosmetic_item_repo.go` 同模式）—— 这是"INSERT 与开箱事务同事务原子提交"的技术保证。
- 完整中文注释头：interface 注释说明 epics.md §23.5 + V1 §7.2.4h 节点 8 来源 + "必须事务内调用 + GORM 回填 id + 任一步失败跟随回滚"+ 范围红线（"本 story 加 CreateInTx 入仓写方法；BatchCreate dev 批量发放写方法见 AC6；status 推进 / consumed 写方法归 Epic 26 / 32-33"）。
- **不**改既有 `UserCosmeticItem` struct / `TableName()` / `ListByUserForInventory` interface 注释 + impl / `userCosmeticItemRepo` struct / `NewUserCosmeticItemRepo` 构造（23.2 + 23.4 落地，本 story 仅追加 `CreateInTx`）。

**AC2 — `chest_service.go` 的 `chestServiceImpl` + `NewChestService` 注入 `userCosmeticItemRepo`（依赖扩展）**

修改 `server/internal/service/chest_service.go`：

- `chestServiceImpl` struct **新增字段** `userCosmeticItemRepo mysql.UserCosmeticItemRepo`（放在 `cosmeticItemRepo` 字段附近，注释标"Story 23.5 引入：开箱事务补入仓写 user_cosmetic_items 实例依赖"）。
- `NewChestService` **扩签名**新增第 8 个参数 `userCosmeticItemRepo mysql.UserCosmeticItemRepo`（追加在 `weightedPicker` 之后），并在 return 的 `&chestServiceImpl{...}` 加 `userCosmeticItemRepo: userCosmeticItemRepo,` 字段赋值。
- 更新 `NewChestService` doc 注释：标注"Story 23.5 扩签名为 8 参数（节点 8 入仓）—— 新增 userCosmeticItemRepo（开箱事务创建 user_cosmetic_items 实例）；router.go + 全部 chest_open 测试 fixture 同步扩参"。
- 更新 `chestServiceImpl` struct 顶部字段注释段（行 56-57 附近）说明 Story 23.5 加 `userCosmeticItemRepo`（OpenChest 入仓依赖；GetCurrent 不消费）。
- **关键回归点**：`NewChestService` 现 7 参构造被以下调用点引用，扩签名后**全部必须同步改否则 build 红**：
  - `server/internal/app/bootstrap/router.go` line 493（AC7）
  - `server/internal/service/chest_open_service_test.go` `fixtureService` line 219-231（AC5）
  - **dev 实装前必须 `grep -rn "NewChestService(" server/` 找全部调用点逐一改**（可能还有 `chest_service_test.go` / 其他集成测试）—— 与 Story 23.4 `NewCosmeticService` 扩签名同模式（23.4 行 39-44 范围红线"扩签名后这三处必须同步改否则 build 红"）。
- **不**改 `ChestService` interface 方法签名（`GetCurrent` / `OpenChest` contract 稳定）/ `GetCurrent` impl。

**AC3 — `chest_open_service.go` 的 `runOpenChestTx` 在 5g 之后 5h 之前插入 user_cosmetic_items INSERT + 回填真实 id（核心改动）**

修改 `server/internal/service/chest_open_service.go` 的 `runOpenChestTx`（行 213-335）：

- **改动位置精确锚定**：在 5g 抽奖产出 `pickedItem := items[pickedIndex]`（行 267）**之后**、5h 写 `chest_open_logs`（行 269-280）**之前**插入新步骤 **5g.5（创建 user_cosmetic_items 实例）**：

  ```go
  // 5g.5: 创建 user_cosmetic_items 实例（Story 23.5 节点 8 入仓；epics.md §23.5 +
  // V1 §7.2.4h 节点 8 + DB §8.3"插入一条 user_cosmetic_items"钦定）。
  // **必须在 5h 写 chest_open_logs 之前** —— 要先拿到 user_cosmetic_items.id 才能
  // 回填 chest_open_logs.reward_user_cosmetic_item_id（之前节点 7 阶段固定 0）。
  // 全部在 txCtx 同事务（ADR-0007 §2.4 + DB §8.3）—— 任一步失败本 INSERT 跟随回滚。
  newItem := &mysql.UserCosmeticItem{
      UserID:         in.UserID,
      CosmeticItemID: pickedItem.ID,           // 5g 抽中的配置 id
      Status:         1,                        // 1=in_bag（§6.10 + struct 注释钦定）
      Source:         1,                        // 1=chest（§6.11 + struct 注释钦定）
      SourceRefID:    ptrUint64(chest.ID),      // 被开启的宝箱 id（epics.md 行 3306 + struct 注释行 27-29 钦定；*uint64 非空指针）
      ObtainedAt:     now,                      // 复用 5d 已取的 now := s.nowFn()（同源同时刻，**不**重新 time.Now()）
  }
  if err := s.userCosmeticItemRepo.CreateInTx(txCtx, newItem); err != nil {
      return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
  }
  // newItem.ID 已由 GORM 回填（AUTO_INCREMENT）
  ```

- **`SourceRefID` 是 `*uint64`**（`UserCosmeticItem` struct 行 68 `SourceRefID *uint64`，NULL 可空列）—— 开箱来源宝箱非空，必须传 `&chest.ID` 的指针。`chest.ID` 是 5c `FindByUserIDForUpdate` 返回的 `chest *mysql.UserChest`，在 `runOpenChestTx` 作用域内可见。dev 用本地 helper 或 `func ptrUint64(v uint64) *uint64 { return &v }`（**注**：`chest.ID` 是循环外稳定变量直接取址安全；若仓库已有同义 helper 复用，dev 先 grep `*uint64` / `ptrUint64` / `func ptr` 找现有 helper，无则在本文件加最小 helper）。
- **`ObtainedAt` 必须复用 5d 已取的 `now`**（`now := s.nowFn()` 行 224）—— **不**在 5g.5 重新 `time.Now()`；与同事务"同源同时刻"原则一致（V1 §7.2 r11；与 5i `now.Add(chestRefreshNextDelay)` 同 now 源）。`CreatedAt` / `UpdatedAt` 留空由 DB `DEFAULT CURRENT_TIMESTAMP(3)` 兜底（与 `UserCosmeticItem` struct tag 一致；GORM 不传则走 DB default）；`ConsumedAt` 留 nil（未消耗，§5.9"未消耗时为空"）。
- **5h 回填 `logRow.RewardUserCosmeticItemID`**：把 行 274 `RewardUserCosmeticItemID: 0,` 改为 `RewardUserCosmeticItemID: newItem.ID,`，注释从"节点 7 阶段占位"更新为"Story 23.5 节点 8 回填真实 user_cosmetic_items.id"。
- **5j 回填 `output.Reward.UserCosmeticItemID`**：把 行 300 `UserCosmeticItemID: 0, // 节点 7 阶段占位` 改为 `UserCosmeticItemID: newItem.ID, // Story 23.5 节点 8 真实 user_cosmetic_items.id`。
- **`buildCacheableResponse`（行 340-366）+ `replayFromCachedResponse`（行 370-397）无需改**：它们已透传 `out.Reward.UserCosmeticItemID` / `cached.Data.Reward.UserCosmeticItemID`（行 346 / 377）。改完 5j 后 buildCacheableResponse 自动把真实 id 写入 `chest_open_idempotency_records.response_json`，幂等 replay 自动返回真实 id（**验证点**：dev 写一条幂等 replay 单测断言 replay 返回的 `UserCosmeticItemID` == 首次开箱写入的真实 id，详见 AC5 case 4）。dev **禁止**为了"保险"去改这两个 helper（它们透传逻辑已正确，改动只会引入回归）。
- **绝对禁止改动**：5a 幂等预声明 / 5b 短路 replay / 5c·5e FOR UPDATE / 5d unlockable 判定 / 5f 乐观锁扣步数 / 5g 加权抽取 / 5i 刷新下一轮 chest / 5k MarkSuccess 的任何逻辑、错误码、`apperror.Wrap` 翻译、事务边界、变量。新步骤 5g.5 的错误**必须**与同事务其他写步骤一致包成 `ErrServiceBusy (1009)`（V1 §7.2 "任何其他 DB 错 → 1009"行钦定；与 5h / 5i / 5k 完全一致），**不**引入新错误码。
- 文件顶部决策注释段（行 14-22）追加一行：`//   - 23.5: 节点 8 入仓 —— 5g 与 5h 之间插 user_cosmetic_items INSERT，回填 reward id 三处（log/output/cache 透传），同 txCtx 原子提交`。

**AC4 — 不破坏 Story 20.6 既有逻辑（回归保护硬性验收）**

完成本 story 后，**Story 20.6 的全部既有行为必须保持不变**：

- 扣步数（5f available_steps -1000 / consumed_steps +1000 / version +1）逻辑、错误翻译（乐观锁失败 → 1009）不变。
- idempotencyKey 幂等：5a ClaimPending / 5b 短路 replay / 步骤 3 committed success 预检 / cached pending → 1009 全部不变。**幂等命中 replay 返回的 `userCosmeticItemId` 必须与首次开箱写入的真实 id 一致**（因 5j 回填 + buildCacheableResponse 透传 + MarkSuccess 把含真实 id 的 response_json 缓存）。
- 抽奖分布：5g `ListEnabledForWeightedPick` + `weightedPicker.Pick` 不变（5g.5 用 5g 已抽中的 `pickedItem.ID`，**不**重新抽）。
- 刷新下一轮 chest：5i DELETE 旧 + INSERT 新 `UnlockAt: now.Add(chestRefreshNextDelay)` 不变。
- 错误码：4001（chest not found）/ 4002（not unlockable）/ 3002（insufficient steps）/ 1009（DB 异常 / 乐观锁 / cosmetic 空 / pending 兜底）全部不变；5g.5 新增的 INSERT 失败包成 1009（与既有 DB 错一致，**不**新增错误码）。
- `server/internal/service/chest_open_service_test.go` 既有全部 case（`TestChestService_OpenChest_HappyPath_FirstTime` / `IdempotencyReplay_CachedSuccess` / `ChestNotFound_4001` / `ChestNotUnlockable_4002` / `InsufficientSteps_3002` / `StepAccountNotFound_1009` / `OptimisticLockFails_1009` / `NoEnabledCosmetic_1009` / `IdempotencyClaim_ExistingRow_ShortCircuitReplay` / `WeightedPicker_IndexReverseMapsToItem` / `CachedPending_Returns1009` / `UserIDZero_Returns1009` / `IdempotencyFindDBError_Returns1009` 等）**全部仍通过**（fixtureService 扩参后调整 stub 注入即可；断言逻辑不改 —— happy path 现在还会调 `userCosmeticItemRepo.CreateInTx`，注入 happy stub 返 nil + 回填 ID 即可）。
- `bash server/scripts/build.sh --test` 全绿（vet + build + 全部单测；含本 story 新增 case + 既有 case）。

**AC5 — 扩 Story 20.6 单测 `chest_open_service_test.go`（≥4 新 case + 既有 case 适配）**

修改 `server/internal/service/chest_open_service_test.go`：

- **新建 `stubUserCosmeticItemRepo`**（实现扩展后的 `mysql.UserCosmeticItemRepo` interface —— 含 `ListByUserForInventory`（`panic("not expected (chest_open_service_test 仅测开箱入仓写路径，不走 GET /cosmetics/inventory)")` —— 与既有 `stubCosmeticItemRepo.ListEnabledForCatalog` panic 行 78 同模式）+ `CreateInTx`（含可注入 `createInTxFn func(ctx, item *mysql.UserCosmeticItem) error` 字段；默认行为：回填 `item.ID = <固定测试 id 如 90001>` 并 return nil））。
- **新增 `happyUserCosmeticItemRepo()` helper**（返回 `*stubUserCosmeticItemRepo`，`createInTxFn` 回填 `item.ID = 90001` + return nil；与 `happyLogRepo()` 行 268-270 同模式）。
- **`fixtureService`（行 219-231）扩参第 8 个** `ucRepo mysql.UserCosmeticItemRepo`，并把 `service.NewChestService(...)` 调用扩第 8 参 `ucRepo`。**既有全部调用 `fixtureService(...)` 的 case 同步扩参**（传 `happyUserCosmeticItemRepo()` 即可让既有 happy case 仍绿；错误分支 case 在 INSERT 之前就 return 的传 happy stub 不影响）。
- **新增 ≥4 case**（与 epics.md 行 3313-3316 钦定 1:1）：
  1. `TestChestService_OpenChest_CreatesUserCosmeticItem_Node8`：happy 开箱成功 → 断言 `stubUserCosmeticItemRepo.CreateInTx` 被调一次 + 收到的 `item.UserID == in.UserID` / `item.CosmeticItemID == pickedItem.ID` / `item.Status == 1` / `item.Source == 1` / `*item.SourceRefID == chest.ID` / `item.ObtainedAt == 5d 的 now`（用 nowFn 控制可断言）+ output.Reward.UserCosmeticItemID == 90001（stub 回填值）非零。
  2. `TestChestService_OpenChest_LogFilledWithRealUserCosmeticItemID_Node8`：happy 开箱 → 断言 `stubChestOpenLogRepo.Create` 收到的 `logRow.RewardUserCosmeticItemID == 90001`（非零，回填真实 id）。
  3. `TestChestService_OpenChest_UserCosmeticItemInsertFails_RollsBack_1009`：`stubUserCosmeticItemRepo.createInTxFn` 返 DB error → 断言整体 return `apperror` code == 1009（ErrServiceBusy）+ （在 stubTxMgr 支持 rollback 断言时）事务回滚 / chest_open_logs.Create **未被调用**（5g.5 失败应在 5h 之前 return，logRepo.Create 不该被触发）—— 验证 INSERT 失败时后续步骤不执行 + 整体回滚语义。
  4. `TestChestService_OpenChest_IdempotencyReplay_ReturnsRealUserCosmeticItemID_Node8`：扩展既有 `IdempotencyReplay_CachedSuccess` 模式 —— 构造 cached `response_json` 含 `userCosmeticItemId: "90001"`（或先跑一次 happy 开箱拿到 MarkSuccess 写入的 responseJSON，再用同 key replay）→ 断言 replay 返回的 `output.Reward.UserCosmeticItemID == 90001`（验证缓存透传链：5j 回填 → buildCacheableResponse → MarkSuccess 缓存 → replayFromCachedResponse 读出，幂等 replay 与首次一致，**不**回 0）。
- 全部新 case 用中文注释 + 中文 `t.Errorf` 文案（项目 communication_language=Chinese）。

**AC6 — 激活 Story 20.8 `/dev/grant-cosmetic-batch` 真实写库（解除节点 7 stub 阻塞）**

修改 `server/internal/service/dev_cosmetic_service.go`（行 19-39 + 110-128 注释已明确钦定"节点 8 / Epic 23.5 阶段由 23.5 owner 在本 service 内激活"）：

- `devCosmeticServiceImpl` struct（行 81，现 `struct{}` 无字段）**加字段** `cosmeticItemRepo mysql.CosmeticItemRepo` + `userCosmeticItemRepo mysql.UserCosmeticItemRepo`。
- `NewDevCosmeticService`（行 92-94，现无参）**扩签名** `NewDevCosmeticService(cosmeticItemRepo mysql.CosmeticItemRepo, userCosmeticItemRepo mysql.UserCosmeticItemRepo) DevCosmeticService`（与行 88-89 注释钦定的目标签名一致）。
- `GrantCosmeticBatch`（行 119-128，现 stub 返 `ErrNotImplemented(1010)`）**实装真实写库**（行 110-118 注释钦定的激活逻辑）：
  1. 调 `cosmeticItemRepo` 按 rarity 随机抽 count 个 cosmetic_item_id。**dev 先 `grep -n "func (r \*cosmeticItemRepo)" server/internal/repo/mysql/cosmetic_item_repo.go` 确认现有方法集**（当前 `ListEnabledForWeightedPick` / `ListEnabledForCatalog` / `ListByIDsForInventory` —— **无**按 rarity 随机抽方法）。**需新增 `CosmeticItemRepo.FindRandomByRarity(ctx, rarity int8, count int32) ([]uint64, error)` 方法 + impl**（SQL: `SELECT id FROM cosmetic_items WHERE rarity = ? AND is_enabled = 1 ORDER BY RAND() LIMIT ?`；返回 cosmetic_item_id slice）+ 补 `cosmetic_item_repo_test.go` 测试。**注**：dev 端点是 grant 任意品质，需 `is_enabled = 1` 过滤（与 catalog/weighted-pick 一致，发放的是可用配置）。若 dev 评估出更优等价方案（如复用既有方法 + Go 层过滤），在 Completion Notes 记录理由。
  2. 对每个 cosmetic_item_id 调 `userCosmeticItemRepo.CreateInTx`（**AC1 已落地的方法复用** —— 但 dev grant 是事务外批量发放，可不开事务逐条 Create，或循环内调 CreateInTx（`tx.FromContext` 在无 txCtx 时走 `r.db` 直连，行为正确）；`source=3 admin_grant`（§6.11 枚举 + `dev_cosmetic_service.go` 行 64 注释钦定 source=2 —— **disambiguation**：行 64 注释写 source=2，但 §6.11 + `UserCosmeticItem` struct 行 24-25 钦定 2=compose / 3=admin_grant；dev 发放语义是 admin_grant 应取 **source=3**；行 64 注释 source=2 与 §6.11 冲突，以 §6.11 枚举为准取 source=3，在 Completion Notes 记录此 disambiguation）；`source_ref_id=NULL`（dev 发放无来源记录，传 nil）；`status=1 in_bag`；`obtained_at` 留空走 DB default 或传 `time.Now().UTC()`）。
  3. happy → return nil；`FindRandomByRarity` 无数据（理论 20.3 seed ≥15 行不该发生）→ 包 `ErrServiceBusy(1009)`；`CreateInTx` 失败 → 包 `ErrServiceBusy(1009)`（行 50-52 注释钦定）。`slog.InfoContext` 记录"dev grant cosmetic batch applied"（行 117 钦定，替换原 WarnContext stub log）。
- **不**改 `DevCosmeticService` interface 的 `GrantCosmeticBatch` 方法签名（行 74）/ 路由 / handler DTO / 客户端调用（行 39 钦定"接口签名 / 路由 / 客户端调用代码不变"）。
- 修改 `dev_cosmetic_service_test.go`：把节点 7"断言 ErrNotImplemented(1010) + HTTP 501"的 service 层 case 改为"happy path return nil + cosmeticItemRepo.FindRandomByRarity + userCosmeticItemRepo.CreateInTx 被调"（行 38 钦定）+ 扩 stub 实现新 repo 依赖 + 加"FindRandomByRarity 空 → 1009"/"CreateInTx 失败 → 1009"case。handler / devtools / bootstrap 层若有断言 501 的 case 同步评估（行 29 提到"全套单测 service 3 + handler 6 + devtools 2 + bootstrap 1"——dev grep `1010` / `ErrNotImplemented` / `501` 找全部相关 case 改）。

**AC7 — `router.go` wire 扩参（NewChestService 第 8 参 + NewDevCosmeticService 扩参）**

修改 `server/internal/app/bootstrap/router.go`：

- `NewChestService(...)` 调用（line 493-501）**扩第 8 参** `userCosmeticItemRepo` —— **复用 line 517 既有 `userCosmeticItemRepo := repomysql.NewUserCosmeticItemRepo(deps.GormDB)` 实例**（line 517 当前只注入 `cosmeticSvc` line 518；本 story 让 `chestSvc` 也复用同一实例，**不**新建第二个 `userCosmeticItemRepo`，与 line 505-507 注释"复用既有 cosmeticItemRepo 实例，不新建第二个，与 chestSvc 复用同实例同模式"完全同模式）。**注意顺序**：line 517 的 `userCosmeticItemRepo` 定义在 line 493 `NewChestService` 调用**之后**——dev 需把 `userCosmeticItemRepo := repomysql.NewUserCosmeticItemRepo(deps.GormDB)` **上移**到 line 493 `NewChestService` 调用之前（与 line 486-489 其他 repo 实例同段构造），再在 NewChestService 扩参引用；line 518 `NewCosmeticService(cosmeticItemRepo, userCosmeticItemRepo)` 引用同实例不受影响。更新 line 491-492 注释为"Story 20.6 扩 7 参；Story 23.5 扩 8 参（加 userCosmeticItemRepo 入仓）"。
- `NewDevCosmeticService(...)` 调用 **扩参** 注入 `cosmeticItemRepo`（复用 line 486 既有实例）+ `userCosmeticItemRepo`（复用同上实例）—— line 383 注释 `// devCosmeticSvc := service.NewDevCosmeticService(cosmeticItemRepo, userCosmeticItemRepo)` 已是目标签名提示。dev grep `NewDevCosmeticService(` 找到 router.go 现有调用点改之。
- `bash server/scripts/build.sh --test` 全绿验证 wire 正确（vet + build + 全部单测）。

**AC8 — 扩 Story 20.9 Layer 2 集成测试 + 全量验证（dockertest）**

修改 `server/internal/service/chest_open_service_integration_test.go`：

- **新增 happy 集成场景**（epics.md 行 3317-3318 钦定）：dockertest 起 `mysql:8.0` → migrate up（含 `0013` chest_open_logs + `0015` user_cosmetic_items）→ 创建 user + 1500 步 + force-unlock chest → 调 `OpenChest` → 断言：`user_cosmetic_items` 表多 1 行（`SELECT COUNT(*) ... WHERE user_id=?` == 1 + 该行 `cosmetic_item_id` == 抽中配置 / `status==1` / `source==1` / `source_ref_id==chest.ID`）+ `chest_open_logs` 多 1 行且 `reward_user_cosmetic_item_id` == 该 user_cosmetic_items.id（**非零**）+ output.Reward.UserCosmeticItemID == 该 id。可选：再调 23.4 `GET /cosmetics/inventory` service 路径（或直接查表）验证该实例在 inventory 可见（epics.md 行 3318"GET /cosmetics/inventory 返回该实例"——若集成测试基建支持则做，否则在 Completion Notes 记"留 Story 25.1 E2E 验证 inventory 可见"）。
- **新增回滚集成场景**（epics.md 行 3319 钦定"user_cosmetic_items 也回滚"）：构造"5g.5 之后某步失败"（如 mock 或制造 chest_open_logs.Create 失败 / MarkSuccess 失败 / 5i 刷新失败）→ 断言事务整体回滚：`user_cosmetic_items` 表**无新增行**（COUNT == 0）+ `user_step_accounts.available_steps` **不变**（步数没扣）+ chest 仍 unlockable（未被刷新）+ 返回 1009。**这是本 story 最高危的回归保护**——验证"user_cosmetic_items INSERT 与开箱事务同 txCtx 原子，任一步失败全回滚"（DB §8.3"全部同事务"+ epics.md 行 3319）。
- **Story 20.9 既有全部集成场景保持通过**（除 `NewChestService` 扩参强制改 fixture 构造外，**不**改既有 case 断言逻辑；既有 happy 场景现在还会写 1 行 user_cosmetic_items，断言加这一行不影响原有 chest_open_logs / step_account 断言）。
- **全量验证（dev 必跑，AC 完成判定）**：
  - `bash server/scripts/build.sh --test`（vet + build + 全部单测全绿，含 AC5 新 case + AC6 dev 改造 case + 既有全部 case）
  - `bash server/scripts/build.sh --integration`（`-tags=integration` 集成测试全绿，含本 AC 新增 2 场景 + Story 20.9 既有全部场景）
- **节点 7→8 升级文档标注**（V1 §7.2.6 行 1155 钦定"Story 23.5 落地时应在 §7.2.6 标注升级日期 + commit hash"）：本 story 范围红线**不**改 docs（改契约文档归 Story 25.3 文档同步 story）；dev 在 Completion Notes 记录"V1 §7.2.6 节点 7→8 升级日期 + commit hash 待 Story 25.3 文档同步 story 标注（本 story 仅落地实装，不改 docs，与范围红线一致）"。

## Tasks / Subtasks

- [x] **Task 1 — repo 写方法**（AC1）
  - [x] `user_cosmetic_item_repo.go` 的 `UserCosmeticItemRepo` interface 追加 `CreateInTx(ctx, item *UserCosmeticItem) error` + 完整中文注释头
  - [x] impl `(r *userCosmeticItemRepo) CreateInTx`：`tx.FromContext(ctx, r.db).WithContext(ctx).Create(item).Error`（与 `chest_open_log_repo.go` Create 1:1 同模式）
  - [x] 不动既有 struct / TableName / ListByUserForInventory / NewUserCosmeticItemRepo
- [x] **Task 2 — service 依赖扩展**（AC2）
  - [x] `grep -rn "NewChestService(" server/` 找全部调用点
  - [x] `chest_service.go`：`chestServiceImpl` 加 `userCosmeticItemRepo` 字段 + `NewChestService` 扩第 8 参 + 注释更新
- [x] **Task 3 — 开箱事务核心改动**（AC3, AC4）
  - [x] `chest_open_service.go` `runOpenChestTx`：5g `pickedItem` 之后 5h 之前插入 5g.5 user_cosmetic_items INSERT（UserID/CosmeticItemID=pickedItem.ID/Status=1/Source=1/SourceRefID=&chest.ID/ObtainedAt=now 复用 5d now）+ CreateInTx 错误包 1009
  - [x] 5h `logRow.RewardUserCosmeticItemID: 0` → `newItem.ID` + 注释更新
  - [x] 5j `output.Reward.UserCosmeticItemID: 0` → `newItem.ID` + 注释更新
  - [x] 验证 `buildCacheableResponse` / `replayFromCachedResponse` 透传链正确（**不改**这两个 helper）
  - [x] 文件顶部决策注释段加 23.5 行
  - [x] 绝对不动 5a~5f / 5i / 5k
- [x] **Task 4 — 单测扩展**（AC5）
  - [x] 新建 `stubUserCosmeticItemRepo`（ListByUserForInventory panic + CreateInTx 可注入 fn 回填 ID）+ `happyUserCosmeticItemRepo()` helper
  - [x] `fixtureService` 扩第 8 参 + 既有全部 fixtureService 调用点同步扩参
  - [x] 新增 ≥4 case（创建实例断言字段 / log 回填真实 id / INSERT 失败回滚 1009 / 幂等 replay 返回真实 id）
- [x] **Task 5 — 激活 dev grant-cosmetic-batch**（AC6）
  - [x] `grep -n "func (r \*cosmeticItemRepo)" cosmetic_item_repo.go` 确认现有方法集
  - [x] 新增 `CosmeticItemRepo.FindRandomByRarity` 方法 + impl + 测试（`WHERE rarity=? AND is_enabled=1 ORDER BY RAND() LIMIT ?`）
  - [x] `dev_cosmetic_service.go`：struct 加 2 repo 字段 + `NewDevCosmeticService` 扩参 + `GrantCosmeticBatch` 实装真实写库（source=3 admin_grant，记 disambiguation）+ slog.InfoContext
  - [x] `dev_cosmetic_service_test.go`：1010/501 断言 case 改为 happy + 1009 失败 case；grep `1010`/`ErrNotImplemented`/`501` 找全部相关 case 改
- [x] **Task 6 — wire**（AC7）
  - [x] `router.go`：`userCosmeticItemRepo` 实例上移到 NewChestService 之前 + NewChestService 扩第 8 参 + NewDevCosmeticService 扩参注入 cosmeticItemRepo + userCosmeticItemRepo
- [x] **Task 7 — 集成测试 + 全量验证**（AC8）
  - [x] `chest_open_service_integration_test.go`：新增 happy（开箱后 user_cosmetic_items 多 1 行 + reward id 非零）+ 回滚（任一步失败 user_cosmetic_items 也回滚 + 步数不变 + chest 未刷新 + 1009）
  - [x] Story 20.9 既有集成场景全部保持通过
  - [x] `bash server/scripts/build.sh --test` 全绿
  - [x] `bash server/scripts/build.sh --integration` 全绿（service 包；`internal/infra/migrate` 包 dockertest 容器启动 120s 超时是该包独立测试基建问题，与本 story 无关 —— 见 Completion Notes）
  - [x] Completion Notes 记录 V1 §7.2.6 升级日期标注待 Story 25.3 + source disambiguation

## Dev Notes

### 高危纠偏点（防返工 + 防后续 review over-correction chain —— 本节是本 story 最重要的部分）

1. **本 story 改的是全仓库事务正确性最敏感代码 —— Story 20.6 历经 r1~r15 多轮 race-fix 的开箱事务。唯一允许的改动是在 `runOpenChestTx` 的 5g（`pickedItem := items[pickedIndex]`，行 267）与 5h（写 chest_open_logs，行 269）之间插一步 INSERT + 回填 3 处 id。** 5a 幂等预声明 / 5b 短路 replay / 5c·5e FOR UPDATE / 5d unlockable / 5f 乐观锁扣步数 / 5i 刷新下一轮 / 5k MarkSuccess 的逻辑、错误码、`apperror.Wrap` 翻译、事务边界、变量**一律不许动**。这些是 r1~r15 锁定的不变量（详见 `chest_open_service.go` 行 14-22 决策段 + DB §8.3 行 997 r5/r6/r7/r11 锁定段）。

2. **必须用 `txCtx` 不是外层 `ctx`**：5g.5 的 `s.userCosmeticItemRepo.CreateInTx(txCtx, newItem)` 必须传 `runOpenChestTx` 形参 `txCtx`（**不**是 OpenChest 的外层 ctx）—— ADR-0007 §2.4 + `runOpenChestTx` 行 211-212 注释钦定"本函数内所有 repo 调用必须用传入的 txCtx"。传错 ctx 会让 INSERT 走独立连接脱离事务 → 任一步失败 user_cosmetic_items 不回滚 → 孤儿实例 + 步数没扣的数据不一致（这正是 DB §8.3 行 999-1006 列举的灾难）。AC8 回滚集成场景就是验证这一点。

3. **buildCacheableResponse / replayFromCachedResponse 不许改**：很多 LLM 会"为了保险"去改这两个 helper 加 userCosmeticItemId 处理 —— 它们的 `cacheableRewardDTO.UserCosmeticItemID` 字段 + JSON tag `userCosmeticItemId`（行 98）+ 透传赋值（行 346 / 377）Story 20.6 **已全部就位**。改完 5j `output.Reward.UserCosmeticItemID = newItem.ID` 后，缓存链自动正确（output → buildCacheableResponse 缓存真实 id → MarkSuccess 写 response_json → replayFromCachedResponse 读出真实 id）。改这两个 helper 只会引入回归。AC5 case 4 是这条链的验证测试。**若后续 review 提"buildCacheableResponse 没显式处理 userCosmeticItemId" → 那是误报**：透传逻辑 Story 20.6 已正确，本 story 不需要也不应该改它（防 over-correction：review 若要求改，dev 应拿 AC5 case 4 绿测 + 本注释反驳，**不**盲目顺从）。

4. **source / source_ref_id 取值有契约 disambiguation**：
   - 开箱入仓（AC3）：`source=1`（chest，§6.11 + struct 注释行 24-25）/ `source_ref_id=&chest.ID`（被开启宝箱 id，epics.md 行 3306 + struct 注释行 27-29；`*uint64` 非空指针）。`SourceRefID` 是 `*uint64` 必须传指针，**不**传 0 / NULL。
   - dev grant（AC6）：`source=3`（admin_grant，§6.11 + struct 注释行 24-25 钦定 3=admin_grant）—— **注意** `dev_cosmetic_service.go` 行 64 注释写 `source=2`，但 §6.11 枚举 2=compose / 3=admin_grant，dev 发放语义是 admin_grant 应取 **source=3**。这是文档内部不一致，**以 §6.11 + struct 注释为准取 source=3**，在 Completion Notes 记录此 disambiguation（与 23.4 r1 同源原则"契约/文档不一致时以更权威的枚举定义为准，记录 disambiguation，不反向改文档"）。

5. **NewChestService 扩签名是已知回归点**：现 7 参构造被 router.go line 493 + chest_open_service_test.go fixtureService line 219-231 + 可能的 chest_service_test.go / 集成测试调用。**必须 `grep -rn "NewChestService(" server/` 找全调用点逐一扩参**，否则 build 红（与 Story 23.4 NewCosmeticService 扩签名同模式 —— 23.4 把这列为"关键回归点"并要求三处同步改）。

6. **激活 dev grant-cosmetic-batch 是本 story 范围内钦定，不是 scope creep**：epics.md 行 3320 + `dev_cosmetic_service.go` 行 32-39 / 110-118 注释明确"节点 8 / Epic 23.5 阶段由 23.5 owner 在本 service 内激活"。**不做这一步会让节点 11 合成 demo 永久 501 阻塞**（合成需 10 件 common，dev grant 是唯一不用反复开箱凑齐的途径）。但**不**改其路由 / handler DTO / 接口签名 / 客户端调用（行 39 钦定"兼容已部署 e2e 脚本"），只改 service 层 + 构造函数 + service 单测。

7. **不 backfill 历史 chest_open_logs.reward_user_cosmetic_item_id=0**：epics.md 行 3204 / 3487 + Story 25.3 tech-debt 钦定"接受历史 0/NULL，新记录正常填值"。本 story 只保证**新开箱**正确，**不**写迁移脚本回填节点 7 阶段产生的 0 值历史记录（那是另一个 ticket / 已接受的已知现象）。

8. **节点 7→8 升级 V1 §7.2.6 标注归 Story 25.3**：V1 §7.2.6 行 1155 钦定"Story 23.5 落地时应在 §7.2.6 标注升级日期 + commit hash"，但本 story 范围红线**不**改 docs（改契约文档统一归 Story 25.3 文档同步 story，与 epics.md §25.3 行 3482-3485 一致）。dev 在 Completion Notes 记录"待 Story 25.3 标注"即可，**不**在本 story 改 `docs/宠物互动App_V1接口设计.md`。

### 相关架构模式与约束

- **事务边界**（DB §8.3 + ADR-0007 §2.4）：开箱事务"预声明 idempotency + 扣步数 + 抽奖 + **创建 user_cosmetic_items（本 story 新增）** + 写 chest_open_logs + 刷新 chest + MarkSuccess"全部在 `txMgr.WithTx` 单事务原子提交；repo 调用必用 `txCtx`；任一步失败全回滚。
- **幂等键**（V1 §7.2 r5/r6/r7/r11 + CLAUDE.md"幂等键"）：`/chest/open` 的 idempotencyKey 幂等由 Story 20.6 5a/5b/步骤3 已实装；本 story 不动，但需保证 replay 返回的 userCosmeticItemId 与首次一致（缓存透传链）。
- **状态以 server 为准**（CLAUDE.md）：reward 入仓后 user_cosmetic_items.id 是最终态；client 只在内部 log 不做业务分支（V1 §7.2.6 行 1154）。
- **错误码统一**（V1 §3 + ADR-0006）：5g.5 INSERT 失败包 `ErrServiceBusy(1009)`（与同事务其他 DB 错一致，**不**新增错误码）；repo 返 raw error → service `apperror.Wrap` → handler `c.Error` + return。
- **ctx 必传**（CLAUDE.md + ADR-0007）：`CreateInTx(ctx, ...)` 第一参 ctx；service 内用 txCtx；repo 用 `tx.FromContext(ctx, r.db).WithContext(ctx)`。

### Source tree（本 story 触碰文件清单）

- 改 `server/internal/repo/mysql/user_cosmetic_item_repo.go`（+`CreateInTx`）
- 改 `server/internal/repo/mysql/user_cosmetic_item_repo_test.go`（+`CreateInTx` 测试）
- 改 `server/internal/repo/mysql/cosmetic_item_repo.go`（+`FindRandomByRarity`，AC6）
- 改 `server/internal/repo/mysql/cosmetic_item_repo_test.go`（+`FindRandomByRarity` 测试）
- 改 `server/internal/service/chest_service.go`（`chestServiceImpl` +字段 + `NewChestService` 扩 8 参）
- 改 `server/internal/service/chest_open_service.go`（`runOpenChestTx` 5g.5 INSERT + 5h/5j 回填）
- 改 `server/internal/service/chest_open_service_test.go`（`stubUserCosmeticItemRepo` + fixtureService 扩参 + ≥4 新 case）
- 改 `server/internal/service/chest_open_service_integration_test.go`（+ happy + 回滚 2 场景）
- 改 `server/internal/service/chest_service_test.go`（若有 `NewChestService(` 调用则扩参）
- 改 `server/internal/service/dev_cosmetic_service.go`（激活真实写库，AC6）
- 改 `server/internal/service/dev_cosmetic_service_test.go`（1010 断言改 happy）
- 改 `server/internal/app/bootstrap/router.go`（NewChestService 扩 8 参 + NewDevCosmeticService 扩参）
- `_bmad-output/implementation-artifacts/23-5-*.md`（本 story 文件）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态流转）

### Testing standards

- 测试栈：`bash server/scripts/build.sh --test`（vet + build + `go test -count=1 ./...`）；集成 `bash server/scripts/build.sh --integration`（`-tags=integration` dockertest 起 `mysql:8.0`）。契约来源 ADR-0001 §3.5 + `1-7-重做-scripts-build-sh.md`。
- 单测：mocked repo（`stubUserCosmeticItemRepo` / 既有 `stubChestOpenLogRepo` / `stubIdempotencyRepo` 等）；stub 未期望方法 `panic("... not expected ...")`（与 `chest_open_service_test.go` 行 78 既有同模式）。
- 集成测：dockertest migrate up `0013` + `0015` → 手工 INSERT user/step/chest → 跑 `OpenChest` → 查表断言（与 `chest_open_service_integration_test.go` 既有同模式）。
- 中文注释 + 中文 `t.Errorf` 文案（communication_language=Chinese）。
- 覆盖率：epics.md §23.5 钦定单测 ≥4 case + 集成 dockertest happy + Layer 2 回滚场景。

### Project Structure Notes

- 沿用 23.3 / 23.4 已确立扁平工程现状（`internal/service/` + `internal/repo/mysql/` + `internal/app/http/handler/`，**无** `internal/domain/`）；CLAUDE.md target §4 提 `internal/domain/cosmetic/` 是 aspirational，实际工程演进为扁平，dev 以真实代码现状为准（与 ADR-0006 / 17.4 / 20.5 / 23.3 / 23.4 一致）。
- `CreateInTx` 与 `chest_open_log_repo.go` `Create` 同文件级别同模式（`tx.FromContext` + `WithContext` + `Create` + GORM 回填 ID）。
- 无 GORM AutoMigrate（ADR-0003 §3.2）；schema 真相源是 `0015_init_user_cosmetic_items.up.sql`，struct 仅字段映射。

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 23.5（行 3295-3320）] —— 本 story AC 主来源
- [Source: _bmad-output/planning-artifacts/epics.md#Epic 23 / Story 25.1 验证场景 8 / Story 25.3 tech-debt（行 3206-3208 / 3456 / 3486-3487）]
- [Source: docs/宠物互动App_V1接口设计.md#§7.2.4h 节点 7 vs 8（行 985-990）] —— 开箱日志步骤 h 节点 7→8 切换钦定
- [Source: docs/宠物互动App_V1接口设计.md#§7.2.6 节点 7→8 升级路径（行 1151-1155）] —— userCosmeticItemId 占位→真实主键，升级非契约变更，§7.2.6 标注归 25.3
- [Source: docs/宠物互动App_V1接口设计.md#§8.2 instances[].userCosmeticItemId（行 1376）] —— 节点 8 起开箱 INSERT 的 user_cosmetic_items.id 与 inventory 同义
- [Source: docs/宠物互动App_数据库设计.md#§5.9 user_cosmetic_items（行 483-514）] —— 表结构 + 字段语义
- [Source: docs/宠物互动App_数据库设计.md#§8.3 开箱事务（行 980-1006）] —— 全部同事务 + 节点 7→8 差异 + r5/r6/r7/r11 锁定灾难列举
- [Source: docs/宠物互动App_数据库设计.md#§6.10 status / §6.11 source（行 863-872）] —— status 1=in_bag / source 1=chest / 3=admin_grant 枚举
- [Source: docs/宠物互动App_时序图与核心业务流程设计.md#8 开箱流程（行 240-296）] —— 时序图含"插入 user_cosmetic_items(新实例)"步骤
- [Source: server/internal/service/chest_open_service.go（行 213-335 runOpenChestTx 5c~5l）] —— 改动位置精确锚定
- [Source: server/internal/repo/mysql/user_cosmetic_item_repo.go（23.4 落地，行 91-164）] —— interface 追加 CreateInTx 基础
- [Source: server/internal/repo/mysql/chest_open_log_repo.go（行 86-89 Create）] —— CreateInTx 1:1 同模式参照
- [Source: server/internal/service/dev_cosmetic_service.go（行 19-39 / 110-128）] —— Story 20.8 激活钦定 + 目标签名
- [Source: server/internal/app/bootstrap/router.go（行 483-519）] —— NewChestService / NewCosmeticService / userCosmeticItemRepo wire
- [Source: server/internal/service/chest_open_service_test.go（行 211-293 fixtureService + happy stubs）] —— stub / fixture 扩参参照
- [Source: _bmad-output/implementation-artifacts/23-4-get-cosmetics-inventory-接口.md] —— UserCosmeticItemRepo 首次落地 + 扩签名回归点同模式
- [Source: ADR-0007 §2.4] —— txCtx 传播；[Source: ADR-0006] —— 三层错误映射；[Source: ADR-0003 §3.2] —— 禁止 AutoMigrate

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（续做：上一个 dev-story sub-agent 因 quota limit 在测试对账阶段被打断，本次续完收尾）

### Debug Log References

- `bash scripts/build.sh --test`：vet + build + 全量单测全绿（含 AC5 4 新 case + AC6 5 case + 既有全部 case）
- `cd server && go vet -tags devtools ./...` / `./internal/service/`：exit 0
- `cd server && go vet -tags integration ./...`：exit 0
- `go test -tags=integration ./internal/service/`：含本 story AC8 新增 happy（HappyPath_FullFlow 扩 user_cosmetic_items 断言）+ 新增专项回滚 case（UserCosmeticItemInsert_RollsBackWhenLaterStepFails）+ Story 20.9 既有 5 个回滚/happy case 全绿
- `go test ./internal/repo/mysql/ -run CreateInTx|FindRandomByRarity`：4 个新 repo 单测全绿

### Completion Notes List

**续做范围说明**：本 story 生产代码（AC1 repo / AC2 chest_service / AC3 开箱事务核心 / AC6 dev_cosmetic_service / AC7 router）在被打断前已正确完成，本次续做未推翻重做，仅核对不变量后保留。续做补完的部分：

1. **测试编译错误对账（quota 打断遗留）**：
   - `stubUserCosmeticItemRepo` 在 `cosmetic_service_test.go`（23.4）与 `chest_open_service_test.go`（23.5 半成品）**重复声明** → 合并为单一统一定义（含 `listByUserForInventoryFn` + `createInTxFn` + `createInTxCall` 三字段，`ListByUserForInventory` nil 则 panic / `CreateInTx` 默认回填 90001）。**关键**：统一定义放在**无 build tag** 的 `cosmetic_service_test.go`（而非半成品错放的 `//go:build !integration` 的 `chest_open_service_test.go`）—— 否则 `-tags integration` 构建找不到该类型（半成品 build tag 错位，本次修正）。
   - `stubCosmeticItemRepo`（chest_open_test）/ `stubCatalogCosmeticItemRepo`（cosmetic_test）/ `faultCosmeticItemRepoOnList`（integration）补 `FindRandomByRarity` panic 桩以 satisfy 扩展后 interface 编译。
2. **AC8 集成测试补完（半成品完全没做）**：`HappyPath_FullFlow` 既有断言 `Reward.UserCosmeticItemID == 0（节点 7 占位）`是陈旧断言，节点 8 会失败 → 改为断言真实 id 非零 + 新增 user_cosmetic_items 多 1 行 + 字段（cosmetic_item_id/status=1/source=1/source_ref_id=5001）+ chest_open_logs.reward_user_cosmetic_item_id == 该 id（三处一致）；新增专项回滚 case `UserCosmeticItemInsert_RollsBackWhenLaterStepFails`（5h 失败 → 5g.5 已 INSERT 的 user_cosmetic_items COUNT=0 + 步数不变 + chest 仍 unlockable + 1009）。
3. **AC1/AC6 repo 单测补完（半成品完全没做）**：`user_cosmetic_item_repo_test.go` 加 `CreateInTx` happy + DB error 透传 2 case；`cosmetic_item_repo_test.go` 加 `FindRandomByRarity` happy + 空集 2 case。

**source disambiguation（AC4/AC6 钦定）**：开箱入仓 `source=1`(chest) / `source_ref_id=&chest.ID`；dev grant `source=3`(admin_grant) —— `dev_cosmetic_service.go` 原行 64 注释写 source=2 与 §6.11 枚举（2=compose / 3=admin_grant）冲突，以 §6.11 枚举为准取 source=3，**不**反向改文档（与 23.4 r1 同源原则）。已在 `dev_cosmetic_service.go` 注释 + 本 Completion Notes 双记录。

**V1 §7.2.6 节点 7→8 升级标注**：本 story 范围红线**不**改 docs；V1 §7.2.6 节点 7→8 升级日期 + commit hash 待 Story 25.3 文档同步 story 标注（本 story 仅落地实装，与范围红线一致）。

**5 条关键不变量核对结论（续做时已逐条核对半成品，全部遵守）**：
1. ✅ 唯一许可改动边界：5g.5 仅插在 5g(`pickedItem`)与 5h(写 log)之间 + 回填 3 处（5h logRow / 5j output / cache 透传）；5a~5f / 5i / 5k 逻辑/错误码/事务边界**未动**。
2. ✅ 事务边界铁律：5g.5 `CreateInTx(txCtx, newItem)` 用 `runOpenChestTx` 形参 `txCtx`（非外层 ctx）；AC8 专项回滚 case 实测 5h 失败时 user_cosmetic_items COUNT=0（证明同事务原子回滚，无孤儿实例）。
3. ✅ buildCacheableResponse / replayFromCachedResponse **未改**（透传链 20.6 已就位）；AC5 case4 IdempotencyReplay_ReturnsRealUserCosmeticItemID_Node8 绿测验证缓存回真实 id。
4. ✅ 契约 disambiguation：开箱 source=1/source_ref_id=&chest.ID；dev grant source=3（§6.11 枚举为准，注释+Notes 双记录，未反向改文档）。
5. ✅ 已知回归点：`grep NewChestService(` / `NewDevCosmeticService(` 全调用点（router.go ×2 + 5 个测试文件）已同步扩参；激活 Story 20.8 dev grant 真实写库；**未** backfill 历史 reward_id=0；**未**改 V1 §7.2.6 docs。

**已知非阻塞观察（移交 review 评估）**：
- `internal/app/http/handler/dev_cosmetic_handler_test.go` 仍有 `TestDevCosmeticHandler_..._ServiceReturnsNotImplemented_Forwards501`（注入返 1010 的 stub service 测 handler 透传 1010→501 通用机制）—— 该 case 用 mock service 注入，**不**依赖真实 service stub 行为，仍编译通过且绿（验证 handler 的 1010→501 通用映射，是有效 case）；test 名/语义带"节点 7"色彩但非 build/正确性阻塞。AC6 行 180"handler 层若有断言 501 的 case 同步评估"——评估结论：用 mock 注入的通用 handler 映射测试可保留，未改（保守不动无关 case）。
- `internal/infra/migrate` 包集成测试 dockertest 起 mysql 容器 120s 超时失败 —— 是该包**独立**测试基建在本机 docker 慢启动下的偶发问题，与本 story 改动**无关**（本 story 改 service / repo 层；service 包集成测试全绿）。

### File List

- `server/internal/repo/mysql/user_cosmetic_item_repo.go`（AC1：interface 追加 `CreateInTx` + impl）
- `server/internal/repo/mysql/user_cosmetic_item_repo_test.go`（AC1：补 `CreateInTx` happy + DB error 2 单测）
- `server/internal/repo/mysql/cosmetic_item_repo.go`（AC6：interface 追加 `FindRandomByRarity` + impl）
- `server/internal/repo/mysql/cosmetic_item_repo_test.go`（AC6：补 `FindRandomByRarity` happy + 空集 2 单测）
- `server/internal/service/chest_service.go`（AC2：`chestServiceImpl` +字段 + `NewChestService` 扩 8 参）
- `server/internal/service/chest_open_service.go`（AC3：`runOpenChestTx` 5g.5 INSERT + 5h/5j 回填 + 顶部决策注释）
- `server/internal/service/chest_open_service_test.go`（AC5：复用统一 stub + `stubCosmeticItemRepo` 补 `FindRandomByRarity` panic + `fixtureService` 扩参 + 4 新 case；统一 `stubUserCosmeticItemRepo` 移至 cosmetic_service_test.go）
- `server/internal/service/cosmetic_service_test.go`（去重宿主：统一 `stubUserCosmeticItemRepo` 定义 + `stubCatalogCosmeticItemRepo` 补 `FindRandomByRarity` panic）
- `server/internal/service/chest_open_service_integration_test.go`（AC8：HappyPath 扩 user_cosmetic_items 断言 + 修陈旧节点 7 断言 + 新增专项回滚 case + `faultCosmeticItemRepoOnList` 补 `FindRandomByRarity` panic）
- `server/internal/service/chest_service_test.go`（AC2：`NewChestService` 扩 8 参同步改）
- `server/internal/service/chest_service_integration_test.go`（AC2：`NewChestService` 扩 8 参同步改）
- `server/internal/service/dev_cosmetic_service.go`（AC6：移除 1010 stub → 真实写库 + struct/构造扩参 + source=3 disambiguation）
- `server/internal/service/dev_cosmetic_service_test.go`（AC6：1010 断言改 happy + 1009 失败 case + 扩 stub）
- `server/internal/app/bootstrap/router.go`（AC7：`cosmeticItemRepo`/`userCosmeticItemRepo` 实例上移 + `NewChestService` 扩 8 参 + `NewDevCosmeticService` 扩参）
- `_bmad-output/implementation-artifacts/23-5-*.md`（本 story 文件流转）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（23-5 状态流转 → review）
