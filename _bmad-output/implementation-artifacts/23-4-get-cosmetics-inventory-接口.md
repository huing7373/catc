# Story 23.4: GET /cosmetics/inventory 接口（聚合 + 实例列表；首次落地 UserCosmeticItemRepo interface + 聚合查询方法 + CosmeticService.ListInventory + CosmeticsHandler.GetInventory + 路由挂载 authedGroup + config 三态完整矩阵 A/B/C + 两级确定性全序排序 + ≥5 case 单测 + dockertest 集成测试 + 严格对齐 V1 §8.2 冻结契约）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want **首次落地** `GET /api/v1/cosmetics/inventory` REST 接口：**新建** `server/internal/repo/mysql/user_cosmetic_item_repo.go` 内的 `UserCosmeticItemRepo` interface（Story 23.2 仅落地 `UserCosmeticItem` GORM struct + `TableName()` 最小骨架，**没有** interface / 任何方法 —— 本 story 是该 repo interface 的**首次**落地）+ `ListByUserForInventory(ctx, userID uint64) ([]UserCosmeticItem, error)` 方法（`SELECT id, cosmetic_item_id, status FROM user_cosmetic_items WHERE user_id = ? AND status IN (1, 2)` —— 仅 in_bag + equipped，**不含** consumed(3) / invalid(4)；与 `cosmetic_item_repo.go` `ListEnabledForCatalog` impl 同模式：`tx.FromContext(ctx, r.db).WithContext(ctx)` + 显式 `Select` + `Where` + `Find` + nil slice 兜底）+ `userCosmeticItemRepo` struct + `NewUserCosmeticItemRepo(db *gorm.DB)` 构造 + **扩展既有** `server/internal/repo/mysql/cosmetic_item_repo.go` 的 `CosmeticItemRepo` interface（**新增** `ListByIDsForInventory(ctx, ids []uint64) ([]CosmeticItem, error)` 方法 —— `SELECT id, name, slot, rarity, icon_url, asset_url FROM cosmetic_items WHERE id IN (?)`，**禁止**加 `is_enabled = 1` 过滤（§8.2 契约：实例可见性与配置 enabled 状态完全解耦，已拥有不得静默丢失）；ids 为空 → 直接返 `[]CosmeticItem{}` **不**发空 `IN ()` SQL；**不**复用 `ListEnabledForWeightedPick`（无 ORDER BY、加权语义）/ **不**复用 `ListEnabledForCatalog`（带 `is_enabled=1` 过滤会让 disabled 配置的已拥有项静默隐藏，违背态 B 契约））+ **扩展既有** `server/internal/service/cosmetic_service.go` 的 `CosmeticService` interface（**新增** `ListInventory(ctx, userID uint64) ([]InventoryGroup, error)` 方法 + `InventoryGroup` / `InventoryInstance` service 层 DTO）+ `cosmeticServiceImpl` 加 `userCosmeticItemRepo` 字段 + `NewCosmeticService` 构造扩签名注入 `userCosmeticItemRepo`（**关键回归点**：`NewCosmeticService` 现有 1 参数构造被 router.go line 509 + `cosmetic_service_test.go` + `cosmetic_service_integration_test.go` 调用，扩签名后这三处必须同步改，否则 build 红）+ ListInventory service 实装（查实例 → 按 cosmetic_item_id 聚合 → 关联 cosmetic_items 配置元信息 → **config 三态完整矩阵 A/B/C 全覆盖** → 组装 → **两级确定性全序排序** `groups[]` = `rarity ASC, slot ASC, cosmeticItemId ASC` + 每组 `instances[]` = `userCosmeticItemId ASC`）+ **扩展既有** `server/internal/app/http/handler/cosmetics_handler.go` 加 `GetInventory(c *gin.Context)` 方法（从 `c.Get(middleware.UserIDKey)` 取 userID —— 与 `chest_handler.go` GetCurrent 同模式，**不存在 / 类型断言失败 → 1009 unreachable 兜底**；调 `h.svc.ListInventory(c.Request.Context(), userID)`；ADR-0006 走 `c.Error` + return；ADR-0007 用 `c.Request.Context()`）+ `inventoryResponseDTO` wire 转换（`cosmeticItemId` / `userCosmeticItemId` 字符串化、`slot` / `rarity` / `status` int 直接下发、全 camelCase、`groups: []` 与 `instances: []` 永不 null）+ **扩展** `server/internal/app/bootstrap/router.go`（在 `if deps 完整` 块内复用 line 486 既有 `cosmeticItemRepo` 实例 + 新建 `userCosmeticItemRepo := repomysql.NewUserCosmeticItemRepo(deps.GormDB)` + `cosmeticSvc := service.NewCosmeticService(cosmeticItemRepo, userCosmeticItemRepo)` 扩参 + `authedGroup.GET("/cosmetics/inventory", cosmeticsHandler.GetInventory)` 注册，与 line 531 `/cosmetics/catalog` 同组同模式）+ 扩展单测 `cosmetic_service_test.go`（≥5 case，mocked `CosmeticItemRepo` + `UserCosmeticItemRepo` stub）+ `cosmetics_handler_test.go`（mocked `CosmeticService` stub 加 `listInventoryFn` 字段）+ `cosmetic_item_repo_test.go`（补 `ListByIDsForInventory` repo 层测试）+ 新建 `user_cosmetic_item_repo_test.go`（补 `ListByUserForInventory` repo 层测试）+ 扩展集成测试 `cosmetic_service_integration_test.go`（dockertest 起 mysql:8.0 → migrate up → 手工 INSERT user + 5 个不同 cosmetic 实例 → 跑 `ListInventory` → 验证 groups 数量 + count + instances + 两级排序，**注**：inventory 与 catalog 不同 —— catalog 复用 0012 seed 闭环不手工 INSERT，inventory 必须手工 INSERT user_cosmetic_items 测试实例，因 0012 / 0015 seed 不含 user_cosmetic_items 行），

so that **iOS Epic 24（仓库页 / Story 24.1 RealWardrobeViewModel + Story 24.2 LoadInventoryUseCase）+ Epic 25（仓库链路 E2E 联调）+ Epic 27（穿戴页刷新 inventory）+ Epic 33（合成页选 in_bag 实例）**可以基于一个**已落地、已具备完整测试覆盖（≥5 单测 + handler 单测 + repo 单测 + dockertest 集成）、已通过真实 dockertest 验证聚合 + 两级全序排序 + config 三态、已严格对齐 V1 §8.2 冻结契约（`{groups:[{cosmeticItemId, name, slot, rarity, iconUrl, assetUrl, count, instances:[{userCosmeticItemId, status}]}]}` + `status IN (1,2)` 过滤 + 按 `cosmetic_item_id` 聚合 + `count = instances 数组长度` + **config 三态 A/B/C 全覆盖**（enabled → row 真实值；disabled-but-exists → row 真实值 + 不 log；missing-no-row → 降级占位 + log error）+ **两级确定性全序排序**（`groups[]` = `rarity ASC, slot ASC, cosmeticItemId ASC`、`instances[]` = `userCosmeticItemId ASC`）+ 空背包 `{groups:[]}` 非 null + 错误码 1001/1005/1009 + **不**触发 1002/1003/5001）的 inventory 查询接口并行展开，不再出现"iOS Epic 24 集成时 inventory response 字段名 / camelCase 与 §8.2 漂移 / 两级排序未钉死导致 Wardrobe Tab grid + instance 列表跨请求抖动 / 关联配置加了 `is_enabled=1` 过滤导致 admin 下架配置后已拥有道具静默消失（用户可见数据丢失回归）/ config 缺失 / 禁用时整体报 1009 让只读接口失败 / consumed/invalid 实例误出现在 inventory / count 算成只 in_bag / 空背包返 null 导致 iOS Codable 解析 crash / `NewCosmeticService` 扩签名漏改 router.go 导致 build 红 / 节点 8 联调才发现 server 没挂 /cosmetics/inventory 路由"的返工。

## 故事定位（Epic 23 第四条 = 第二个对外 REST 接口 story；上承 23.2 持久化根基 + 23.3 catalog 接口模板，下启 23.5 开箱补入仓；本接口查 user_cosmetic_items 实例表 + JOIN/关联 cosmetic_items 配置表，是 23.3 catalog 的"user 维度 + 聚合"姊妹接口）

- **Epic 23 进度**：23.1（契约定稿 §8.1 / §8.2 冻结，done）→ 23.2（user_cosmetic_items migration + `UserCosmeticItem` GORM struct **最小骨架**，done）→ 23.3（GET /cosmetics/catalog 接口 —— 首次落地 cosmetic service / handler / `CosmeticItemRepo.ListEnabledForCatalog`，done）→ **23.4（本 story，GET /cosmetics/inventory 接口 —— 首次落地 `UserCosmeticItemRepo` interface + 聚合查询 + config 三态 + 两级排序）** → 23.5（修改 Story 20.6 开箱事务补"入仓" —— 写 user_cosmetic_items 实例）。
- **本 story 数据源 = `user_cosmetic_items` 实例表（§5.9，23.2 落地）+ 关联 `cosmetic_items` 配置表（§5.8，20.2 落地）**。catalog（23.3）= "有哪些装扮配置"（全局静态目录，与 user 无关，不读 userID）；inventory（本 story）= "我拥有哪些实例"（**必须读 userID** + 按 cosmetic_item_id 聚合 + 关联配置取元信息）。**本 story 必须读 userID（从 auth 中间件 c.Get(UserIDKey)）、查 user_cosmetic_items、做聚合 + config 三态 + 两级排序**。
- **本 story 是 iOS Epic 24 / Epic 25 / Epic 27 / Epic 33 的强前置**：
  - **iOS Epic 24（Story 24.1 RealWardrobeViewModel / Story 24.2 LoadInventoryUseCase）**：仓库页主数据源即本接口；iOS `InventoryGroup` / `InventoryInstance` Codable struct 通过本接口 JSON response 依赖 §8.2 锚定字段（`cosmeticItemId` / `name` / `slot` / `rarity` / `iconUrl` / `assetUrl` / `count` / `instances[].userCosmeticItemId` / `instances[].status`，全 camelCase）
  - **Epic 25（仓库链路 E2E）**："开箱 → 入仓（23.5）→ 仓库可见（本接口）"全链路联调依赖本接口正确返回新入仓实例
  - **Epic 27（穿戴流程）**：穿戴 / 卸下后重新调 GET /cosmetics/inventory 刷新（status 1↔2 推进后实例仍在 inventory，因态 A/B 都返回 status IN (1,2)）
  - **Epic 33（合成页）**：合成页选 in_bag 实例需 inventory 的 `instances[].status` 区分（合成只能选 status=1 未装备实例）
- **epics.md §Story 23.4 钦定**（行 3269-3293）：
  - 查 `user_cosmetic_items WHERE user_id=? AND status IN (1, 2)`（in_bag + equipped，**不含** consumed）
  - 按 cosmetic_item_id 分组；每组关联 cosmetic_items 拿配置元信息
  - 返回 `{groups: [{cosmeticItemId, name, slot, rarity, iconUrl, assetUrl, count, instances: [{userCosmeticItemId, status}]}]}`
  - 接口要求 auth
  - 用户背包为空 → 返回 `{groups: []}`
  - 性能：单用户预期实例数 < 1000，单 SQL 查询足以，不分页
  - 单元测试覆盖 ≥5 case（mocked repo）：happy（3 hat + 1 scarf → groups=2 第一组 count=3）/ happy（0 件 → groups=[]）/ happy（1 件 equipped → 仍包含 status=2）/ edge（1 件 consumed → 不出现）/ edge（配置不存在的 cosmetic_item_id → skip + log warning）
  - 集成测试覆盖（dockertest）：创建 user + 5 个不同 cosmetic 实例 → curl GET /cosmetics/inventory → 验证 groups 数量 + count + instances
- **关键纠偏：epics.md §Story 23.4 AC 文字（行 3292）说"配置不存在 → skip + log warning"，但 V1 §8.2 冻结契约（行 1342-1353 + 1437）已经把这个 case 收紧为"config 三态完整矩阵"且明确禁止 skip / 禁止 log warning（态 C 是 log error 不是 warning，且态 C **不** skip 而是降级占位保留组）。两者冲突时以 V1 §8.2 冻结契约为准**（CLAUDE.md "状态以 server 为准"+ 23.1 已冻结 §8.2 + 23.3 r1 同源原则"文档/契约不一致时以契约文档为准改 story 而非反向改文档"）。详见下方 AC4 三态矩阵。
- **Story 23.1 上游冻结边界**（V1 §8.2 字段表 + 服务端逻辑段 + config 三态矩阵 + 两级排序 + 错误码表 + 关键约束段，2026-05-16 起冻结，见 §1 行 78-79）：本 story handler response **严格**按 §8.2 锚定字段集 + camelCase；service 流程**严格**按 §8.2 服务端逻辑步骤 1-7（status IN (1,2) 过滤 / 按 cosmetic_item_id 聚合 / config 三态 A·B·C 全覆盖 / 两级全序排序 / 空背包 {groups:[]}）；错误码**严格**按 §8.2 错误码表（1001 / 1005 / 1009；**不**触发 1002 / 1003 / 5001 —— 单组配置缺失/禁用**不**让只读接口失败）。
- **范围红线（只改/新建以下文件）**：
  - **新建** `server/internal/repo/mysql/user_cosmetic_item_repo.go`（`UserCosmeticItemRepo` interface + `ListByUserForInventory` + `userCosmeticItemRepo` struct + `NewUserCosmeticItemRepo`；**注**：`UserCosmeticItem` GORM struct + `TableName()` 已由 23.2 落在**同名文件**，本 story 在该文件**追加** interface / impl，**不**改既有 struct）
  - **新建** `server/internal/repo/mysql/user_cosmetic_item_repo_test.go`（`ListByUserForInventory` repo 层测试）
  - **改** `server/internal/repo/mysql/cosmetic_item_repo.go`（`CosmeticItemRepo` interface **新增** `ListByIDsForInventory` 方法 + impl；**不**改既有 `ListEnabledForWeightedPick` / `ListEnabledForCatalog`）
  - **改** `server/internal/repo/mysql/cosmetic_item_repo_test.go`（补 `ListByIDsForInventory` 测试）
  - **改** `server/internal/service/cosmetic_service.go`（`CosmeticService` interface **新增** `ListInventory` + `InventoryGroup` / `InventoryInstance` DTO + `cosmeticServiceImpl` 加 `userCosmeticItemRepo` 字段 + `NewCosmeticService` 扩签名 + ListInventory impl；**不**改既有 `ListCatalog` / `CosmeticBrief`）
  - **改** `server/internal/service/cosmetic_service_test.go`（扩 `stubCosmeticItemRepo` 加 `ListByIDsForInventory` + 新建 `stubUserCosmeticItemRepo` + ≥5 ListInventory case + 既有 `NewCosmeticService(...)` 调用全部改成 2 参数）
  - **改** `server/internal/app/http/handler/cosmetics_handler.go`（**新增** `GetInventory` 方法 + `inventoryResponseDTO`；**不**改既有 `GetCatalog` / `catalogResponseDTO`）
  - **改** `server/internal/app/http/handler/cosmetics_handler_test.go`（`stubCosmeticService` 加 `listInventoryFn` 字段 + 实现 `ListInventory` 方法 + GetInventory case；既有 stub 现仅实现 `ListCatalog`，加 `ListInventory` 后才满足扩展后的 interface）
  - **改** `server/internal/app/bootstrap/router.go`（新建 `userCosmeticItemRepo` 实例 + `NewCosmeticService` 扩参 + 注册 `/cosmetics/inventory` 路由）
  - **改** `server/internal/service/cosmetic_service_integration_test.go`（扩 `buildCosmeticServiceIntegration` 注入 userCosmeticItemRepo + 改 `NewCosmeticService` 2 参 + 新增 `ListInventory` 集成 case）
  - 本 story 文件 + sprint-status.yaml 流转
  - **不**碰 23.2 落地的 `UserCosmeticItem` GORM struct / `TableName()`（本 story 在同文件追加 interface，不改 struct）
  - **不**改 Story 20.6 `ChestService.OpenChest` 开箱事务 / `ChestService` / `chest_service.go`（开箱补入仓是 23.5 钦定范围；本 story **只**做只读 inventory 查询，**不**写 user_cosmetic_items）
  - **不**实装 23.5 开箱补入仓 / 不写 user_cosmetic_items INSERT（YAGNI；预实装会让下游评审找不到"开箱事务改动"明确边界，与 23.2 / 23.3 "禁止预实装"同模式）
  - **不**激活 Story 20.8 `/dev/grant-cosmetic-batch` 真实写库（dev_cosmetic_service.go 仍走 501 stub —— 那是 23.5 完成后才打开，见其行 120-125 注释；本 story **不**碰 dev_cosmetic_service.go）
  - **不**改 V1 接口契约（23.1 已冻结 §8.2）/ 任何 `docs/宠物互动App_*.md`（§8.2 / §5.9 / §5.8 / §6.8 / §6.9 / §6.10 是契约**输入**，本 story 严格对齐但**不**修改；如发现 epics.md §Story 23.4 AC 文字与 V1 §8.2 冻结契约冲突 → 以 V1 §8.2 为准实装并在 Completion Notes 记录，**不**反向改文档）
  - **不**改 0001~0015 既有 migration / 0012 / 0015 seed（inventory 查 23.2 已建的 user_cosmetic_items 表 + 20.2 已建的 cosmetic_items 表，**不**新建 migration / seed）
  - **不**新建 `internal/domain/cosmetic/` 目录（**沿用 23.3 已确立的目录布局纠偏**：CLAUDE.md target §4 / 23.1 行 281 提 `internal/domain/cosmetic/`，但 server 实际现状为扁平 `internal/service/` + `internal/app/http/handler/` + `internal/repo/mysql/`，**无** `internal/domain/`。本 story **严格遵循 server 实际现状目录布局**，与 23.3 已落地的 `cosmetic_service.go` / `cosmetics_handler.go` / `cosmetic_item_repo.go` 同级追加，**不**为本 story 单独引入 `internal/domain/cosmetic/` —— 与 ADR-0006 / 17.4 / 20.5 / 23.3 实装一致；属"CLAUDE.md target 是 aspirational、实际工程演进为扁平"的历史现状，dev 实装以**真实代码现状**为准不盲信文档 target 段)
  - **不**加分页 / query 参数解析（§8.2 行 1329-1330 钦定不分页、不接受任何 query string；筛选纯客户端由 iOS Story 24.3 做）
  - **不**做 `iconUrl == "" 跳过` / 空串过滤（§8.2 态 A·B 行 server 透传真实非空 URL；态 C 降级占位才允许空串 —— 这是**契约钦定的合法路径**，**不**是要过滤的脏数据）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除本 story 文件 + sprint-status.yaml 流转）
  - **不**写英文版测试注释 / 文档（项目 communication_language=Chinese）
  - **不**为 inventory 写 cache / Redis（§8.2 行 1360 钦定只读单查无事务；单用户 < 1000 实例单查足够；与 catalog / emoji 同模式 —— iOS Story 24.2 钦定不缓存）

**本 story 不做**（明确范围红线，再次强调）：

- 不写 user_cosmetic_items INSERT / 不改 ChestService 开箱事务（开箱补入仓是 23.5 钦定范围；本 story **只读** inventory 查询）
- 不激活 dev_cosmetic_service.go `/dev/grant-cosmetic-batch` 真实写库（23.5 完成后才打开；本 story 不碰该文件）
- 不复用 `ListEnabledForWeightedPick`（无 ORDER BY、加权抽取语义）/ 不复用 `ListEnabledForCatalog`（带 `is_enabled=1` 过滤会让 disabled 配置的已拥有项静默隐藏 → 违背 §8.2 态 B 契约）—— inventory 关联配置**新增独立** `ListByIDsForInventory` 方法（**禁止** `is_enabled=1` 过滤）
- 不引入 `internal/domain/cosmetic/` 新目录（沿用 23.3 已确立扁平工程现状）
- 不加分页 / query 参数 / 不接受任何 query string（§8.2 钦定）
- 不开 MySQL 事务（§8.2 行 1360 钦定只读查询无副作用，天然幂等，不需要事务）
- 不在 handler 做参数校验（GET 无 body / 无 query；userID 由 auth 中间件兜底）/ 不直接调 `response.Error`（ADR-0006 单一 envelope 生产者 —— c.Error + return 走 ErrorMappingMiddleware；与 chest_handler / cosmetics_handler.GetCatalog 同模式）
- 不把 `*gin.Context` 当 ctx 传给 service（ADR-0007 §2.2 —— 用 `c.Request.Context()`；与 chest_handler.GetCurrent / cosmetics_handler.GetCatalog 同模式）
- 不改 V1 §8.2 契约 / §5.9 user_cosmetic_items schema / §5.8 cosmetic_items schema / §6.8 slot / §6.9 rarity / §6.10 status 枚举文档（schema 输入，严格对齐不修改）
- 不为 inventory 写 stress / fuzz test（节点 8 阶段 schema 稳定 + 单测 + dockertest 集成已覆盖核心约束，与 17.4 / 20.5 / 23.3 一致）
- 不预实装 admin POST / PATCH /cosmetics 等写接口（MVP 节点 8 无 admin 后台需求）
- 不把 consumed(3) / invalid(4) 实例放进 inventory（§8.2 行 1340 钦定仅 status IN (1,2)；过滤在 repo SQL `WHERE status IN (1,2)` 层做）

## Acceptance Criteria

**AC1 — `user_cosmetic_item_repo.go` 追加 `UserCosmeticItemRepo` interface + `ListByUserForInventory`（§8.2 服务端逻辑步骤 2）**

在既有 `server/internal/repo/mysql/user_cosmetic_item_repo.go`（23.2 落地，现仅含 `UserCosmeticItem` struct + `TableName()`）**追加**：

- `UserCosmeticItemRepo` interface（service 注入 + 单测 mock 用）含：

  ```go
  // ListByUserForInventory 返回某用户 status IN (1,2)（in_bag + equipped）的所有
  // user_cosmetic_items 实例（GET /cosmetics/inventory 数据源，V1 §8.2 服务端逻辑步骤 2）。
  //
  // SQL: SELECT id, cosmetic_item_id, status FROM user_cosmetic_items
  //      WHERE user_id = ? AND status IN (1, 2)
  //
  // 仅 in_bag(1) + equipped(2)；consumed(3) / invalid(4) 被 SQL 层过滤（§8.2 行 1340）。
  // 空结果 → []UserCosmeticItem{}（非 nil）；service 透传为 {groups:[]}（§8.2 行 1341）。
  ListByUserForInventory(ctx context.Context, userID uint64) ([]UserCosmeticItem, error)
  ```

- impl `(r *userCosmeticItemRepo) ListByUserForInventory`：`tx.FromContext(ctx, r.db).WithContext(ctx)` + 显式 `Select("id, cosmetic_item_id, status")`（**不** SELECT *；client 不需要 source / obtained_at 等列）+ `Where("user_id = ? AND status IN ?", userID, []int8{1, 2})` + `Find(&rows)` + `if rows == nil { rows = []UserCosmeticItem{} }` 兜底 —— 与 `cosmetic_item_repo.go` `ListEnabledForCatalog`（行 149-177）1:1 同模式。
- `userCosmeticItemRepo` struct（`db *gorm.DB`）+ `NewUserCosmeticItemRepo(db *gorm.DB) UserCosmeticItemRepo` 构造（与 `NewCosmeticItemRepo` 同模式）。
- **关键：`status IN (1,2)` 过滤在 repo SQL 层做**（§8.2 行 1340 钦定）—— consumed(3) / invalid(4) 绝不进 inventory。`[]int8{1, 2}` 用 GORM `IN ?` 占位（`status TINYINT` → Go `int8`，与 `UserCosmeticItem.Status int8` 一致）。
- 完整中文注释头：interface / impl 注释说明 §8.2 来源 + status 过滤理由 + nil slice 兜底 + 范围红线（"本 story 仅加 inventory 查询方法；23.5 才加 BatchCreate 写方法"）。
- **不**改既有 `UserCosmeticItem` struct / `TableName()`（23.2 落地，本 story 仅追加 interface / impl）。

**AC2 — `cosmetic_item_repo.go` 的 `CosmeticItemRepo` interface 新增 `ListByIDsForInventory`（§8.2 服务端逻辑步骤 3 config 关联）**

修改 `server/internal/repo/mysql/cosmetic_item_repo.go`：

- 在既有 `CosmeticItemRepo` interface（现含 `ListEnabledForWeightedPick` / `ListEnabledForCatalog`）**新增**方法：

  ```go
  // ListByIDsForInventory 按 id 集合批量查 cosmetic_items 配置元信息
  // （GET /cosmetics/inventory 步骤 3 config 关联，V1 §8.2）。
  //
  // SQL: SELECT id, name, slot, rarity, icon_url, asset_url FROM cosmetic_items
  //      WHERE id IN (?)
  //
  // **禁止加 is_enabled=1 过滤**（§8.2 行 1342 / 1437 契约：实例可见性与配置
  // enabled 状态完全解耦 —— 已拥有道具不得因 admin 下架配置而静默丢失，态 B
  // disabled-but-exists 行必须返回真实 metadata）。
  //
  // ids 为空 → 直接返 []CosmeticItem{}（service 层在 ids 为空时不调本方法，
  // 但本方法仍兜底空 ids → 空 slice，**不**发 `WHERE id IN ()` 空集 SQL）。
  ListByIDsForInventory(ctx context.Context, ids []uint64) ([]CosmeticItem, error)
  ```

- impl `(r *cosmeticItemRepo) ListByIDsForInventory`：`if len(ids) == 0 { return []CosmeticItem{}, nil }` 早返（避免空 `IN ()`）→ `tx.FromContext(ctx, r.db).WithContext(ctx)` + 显式 `Select("id, name, slot, rarity, icon_url, asset_url")`（**不** SELECT code/drop_weight/is_enabled —— inventory 响应 §8.2 不下发 code，不需要）+ `Where("id IN ?", ids)` + `Find(&rows)` + nil slice 兜底 —— 与 `ListEnabledForCatalog` 同模式（差异：无 ORDER BY（service 层做两级排序）、无 `is_enabled=1` 过滤、`WHERE id IN ?`）。
- **关键：禁止 `is_enabled=1` 过滤**（§8.2 行 1437 关键约束：**Story 23.4 实装关联 `cosmetic_items` 时禁止加 `is_enabled = 1` 过滤** —— 否则 disabled 配置下已拥有项被静默隐藏，违背态 B 契约）。interface 注释头补本方法说明 + 三方法语义边界（`ListEnabledForWeightedPick` 加权抽取无序、`ListEnabledForCatalog` catalog 带 `is_enabled=1` 三级排序、`ListByIDsForInventory` inventory 按 id 集合无 `is_enabled` 过滤）。
- **不**复用 `ListEnabledForCatalog`（带 `is_enabled=1` 会让 disabled 配置已拥有项消失）/ **不**复用 `ListEnabledForWeightedPick`（无字段裁剪 + 全表扫，inventory 只需按 id 集合查）。
- 显式 `Select` 6 列（id / name / slot / rarity / icon_url / asset_url；**不** SELECT code —— §8.2 groups[] 不含 code 字段，与 §8.1 catalog 含 code 不同）。

**AC3 — `cosmetic_service.go` 扩展（`ListInventory` + `InventoryGroup` / `InventoryInstance` DTO + 构造扩签名）**

修改 `server/internal/service/cosmetic_service.go`：

- `CosmeticService` interface **新增** `ListInventory(ctx context.Context, userID uint64) ([]InventoryGroup, error)`（保留既有 `ListCatalog` 不动）。
- **新增** service 层 DTO（**不是** wire DTO；字段与 §8.2 `data.groups[]` 钦定字段集 1:1）：

  ```go
  type InventoryGroup struct {
      CosmeticItemID uint64              // §8.2 cosmeticItemId（handler 字符串化）
      Name           string              // §8.2 name
      Slot           int8                // §8.2 slot（§6.8 枚举；int 下发不字符串化）
      Rarity         int8                // §8.2 rarity（§6.9 枚举；int 下发不字符串化）
      IconURL        string              // §8.2 iconUrl（态 A/B 非空；态 C 空串）
      AssetURL       string              // §8.2 assetUrl（态 A/B 非空；态 C 空串）
      Count          int                 // §8.2 count = len(Instances)
      Instances      []InventoryInstance // §8.2 instances
  }
  type InventoryInstance struct {
      UserCosmeticItemID uint64 // §8.2 instances[].userCosmeticItemId（handler 字符串化）
      Status             int8   // §8.2 instances[].status（枚举 {1,2}；int 下发不字符串化）
  }
  ```

- `cosmeticServiceImpl` struct **加** `userCosmeticItemRepo mysql.UserCosmeticItemRepo` 字段（保留既有 `cosmeticItemRepo`）。
- `NewCosmeticService` **扩签名**为 `NewCosmeticService(cosmeticItemRepo mysql.CosmeticItemRepo, userCosmeticItemRepo mysql.UserCosmeticItemRepo) CosmeticService`（**回归点**：现有 1 参调用方 = router.go line 509 + cosmetic_service_test.go 所有 case + cosmetic_service_integration_test.go buildCosmeticServiceIntegration，扩签名后必须**全部同步改 2 参**，否则 build 红 —— 见 AC5/AC6/AC7）。
- `ListInventory(ctx, userID)` impl 流程（严格按 §8.2 服务端逻辑步骤 2-6）：
  1. `instances, err := s.userCosmeticItemRepo.ListByUserForInventory(ctx, userID)` → 失败 `apperror.Wrap(err, apperror.ErrServiceBusy, ...)`（1009；与 ListCatalog 同模式）。
  2. `len(instances) == 0` → 返 `[]InventoryGroup{}, nil`（非 nil 空 slice；§8.2 行 1341 空背包 {groups:[]} code=0 不报错）。
  3. 按 `CosmeticItemID` 聚合：收集去重后的 `cosmeticItemIDs []uint64`。
  4. `configs, err := s.cosmeticItemRepo.ListByIDsForInventory(ctx, cosmeticItemIDs)` → 失败 → 1009。建 `map[uint64]mysql.CosmeticItem`（id → config）。
  5. **config 三态完整矩阵（AC4 详述）**：对每个聚合组按其 cosmetic_item_id 在 config map 命中情况落 A/B/C 三态之一，组装 `InventoryGroup`（态 C 用降级占位 + `slog.ErrorContext` log；态 B 用 row 真实值不 log；态 A 用 row 真实值）。
  6. **两级确定性全序排序（AC4 详述）**：`groups[]` `sort.Slice` by `(Rarity ASC, Slot ASC, CosmeticItemID ASC)`；每组 `Instances[]` `sort.Slice` by `UserCosmeticItemID ASC`。
  7. `Count = len(Instances)`（含 status=1 与 status=2，**不**只算 in_bag；§8.2 行 1374）。
  8. 返回排序后 `[]InventoryGroup`。
- **不**做空 URL 过滤（态 A/B 透传真实非空；态 C 降级占位是契约合法路径）。
- 完整中文注释头：interface / DTO / impl 注释说明 §8.2 来源 + 三态矩阵 + 两级排序契约 + 错误约定 + nil slice 兜底 + 范围红线。

**AC4 — config 三态完整矩阵 A/B/C + 两级确定性全序排序（§8.2 行 1342-1358 + 1437 + 1443 冻结契约，本 story 最核心约束）**

ListInventory 步骤 5 对每个聚合组（按其 `cosmetic_item_id` 在 `ListByIDsForInventory` 返回的 config map 命中情况）**必须**落入以下**三态之一**（互斥且穷尽，**禁止**只处理两态）：

| 态 | 判定条件 | 该组是否出现在 groups[] | 元信息来源 | 日志 |
|---|---|---|---|---|
| **A. enabled** | config map 命中该 id 且 `IsEnabled == 1` | **是** | row 真实值（`Name`/`Slot`/`Rarity`/`IconURL`/`AssetURL`，非空 URL） | 无 |
| **B. disabled-but-exists** | config map 命中该 id 但 `IsEnabled == 0` | **是**（已拥有不得静默丢失） | **row 真实值（与态 A 完全一致，含真实非空 URL）**；**不**用 placeholder | **无**（admin 下架是常规运维，**不** log error） |
| **C. missing-no-row** | config map **不**命中该 id（admin 物理删了 row，但用户已拥有实例仍在 user_cosmetic_items） | **是**（已拥有不得静默丢失） | 降级占位：`Name="未知装扮"` / `Slot=99` / `Rarity=1` / `IconURL=""` / `AssetURL=""` | **log error**（`slog.ErrorContext`，数据治理事件触发告警） |

- **关键纠偏（覆盖 epics.md §Story 23.4 AC 文字陷阱）**：epics.md 行 3292 写"配置不存在 → skip + log warning"，但 V1 §8.2 冻结契约（行 1342-1353 + 1437）已收紧：态 C **不 skip**（降级占位**保留**该组，因"已拥有实例不得静默丢失"是用户可见数据丢失回归），**且 log error 不是 warning**（物理删仍有用户持有的 cosmetic_items 行是需运维介入的数据治理事件）。**dev 必须按 V1 §8.2 三态矩阵实装，禁止按 epics.md "skip + warning" 实装** —— 这是冲突点，以契约文档为准（CLAUDE.md "状态以 server 为准" + 23.3 r1 同源原则）。
- **`ListByIDsForInventory` 禁止 `is_enabled=1` 过滤**（AC2 已钉死）：若误加，态 B disabled 配置的已拥有项会从 config map 消失被误判为态 C（错误降级 + 错误 log error）—— 实际应是态 B（row 真实值 + 不 log）。故 service 区分态 A/B 靠 config map 命中后读 `CosmeticItem.IsEnabled`，区分态 B/C 靠 config map 是否命中。
- **态 C 降级占位值必须落既有枚举值域**：`Slot=99`（§6.8 other）/ `Rarity=1`（§6.9 common）—— **不**扩展 schema；client 据此渲染"配置已下架的已拥有道具"占位卡。
- **态 C log 用 `slog.ErrorContext(ctx, ...)`**（参照 dev_cosmetic_service.go 行 120 `slog.WarnContext` 但本处是 **Error** 级别）：log 字段含 `cosmetic_item_id` + `user_id` + 提示运维补 down-migration / 恢复配置。
- **两级确定性全序排序（§8.2 行 1355-1358 + 1443 契约必需，非可选优化）**：
  - `groups[]`：`sort.Slice` 按 `(Rarity ASC, Slot ASC, CosmeticItemID ASC)` —— 与 §8.1 catalog `rarity ASC, slot ASC, id ASC` 同根因风格对齐；`CosmeticItemID ASC` 是决定性 tie-breaker（同 (rarity, slot) 必有多配置）。**态 C 组用降级占位的 `Rarity=1, Slot=99` 参与排序**（占位值落枚举内，排序仍全序确定）。
  - 每组 `Instances[]`：`sort.Slice` 按 `UserCosmeticItemID ASC`（`user_cosmetic_items.id` §5.9 全局唯一主键，单值即决定性全序 tie-breaker；`Status` **不**参与排序）。
  - **理由（为何契约必需）**：iOS Epic 24 每次打开 Wardrobe Tab 重新 GET inventory 渲染聚合 grid + 实例列表（Story 24.2 不缓存），若两级排序未钉死则相同库存跨请求重排、client UI 抖动。两级 tie-breaker 均到唯一键级别保证全序唯一。
  - **不**依赖 SQL / map 迭代顺序（Go map 迭代无序 + GORM Find 无 ORDER BY），**必须**在 service 层显式 `sort.Slice`（§8.2 行 1360：无论实装用 JOIN 还是分步查，两级排序是契约必需，不得依赖 DB 天然顺序）。

**AC5 — `cosmetics_handler.go` 扩展（`GetInventory` + `inventoryResponseDTO`）**

修改 `server/internal/app/http/handler/cosmetics_handler.go`：

- `CosmeticsHandler` **新增** `GetInventory(c *gin.Context)` 方法（保留既有 `GetCatalog`）：
  - 从 `c.Get(middleware.UserIDKey)` 取 userID —— **与 chest_handler.go GetCurrent（行 60-72）1:1 同模式**：`v, ok := c.Get(middleware.UserIDKey)`；`!ok` → `c.Error(apperror.New(apperror.ErrServiceBusy, ...))` + return（unreachable，auth 中间件已挂前，1009 兜底）；`userID, ok := v.(uint64)`；`!ok` → 同样 1009 兜底。
  - 调 `h.svc.ListInventory(c.Request.Context(), userID)`（ADR-0007 §2.2 用 `c.Request.Context()`，**不**传 `*gin.Context`）。
  - 失败 → `_ = c.Error(err); return`（ADR-0006 单一 envelope 生产者，走 ErrorMappingMiddleware）。
  - 成功 → `response.Success(c, inventoryResponseDTO(groups), "ok")`。
  - **关键差异 vs GetCatalog**：GetCatalog **不**读 userID（catalog 全局静态）；GetInventory **必须**读 userID（user 维度）。注意 import `middleware`（既有 cosmetics_handler.go 现未 import middleware，本 story GetInventory 首次需要）。
- `inventoryResponseDTO(groups []service.InventoryGroup) gin.H`：
  - `make([]gin.H, 0, len(groups))` 兜底非 nil → 逐组：
    ```go
    instances := make([]gin.H, 0, len(g.Instances))
    for _, ins := range g.Instances {
        instances = append(instances, gin.H{
            "userCosmeticItemId": strconv.FormatUint(ins.UserCosmeticItemID, 10), // string
            "status":             ins.Status,                                     // int
        })
    }
    groupsOut = append(groupsOut, gin.H{
        "cosmeticItemId": strconv.FormatUint(g.CosmeticItemID, 10), // string
        "name":           g.Name,
        "slot":           g.Slot,    // int
        "rarity":         g.Rarity,  // int
        "iconUrl":        g.IconURL,
        "assetUrl":       g.AssetURL,
        "count":          g.Count,   // int
        "instances":      instances,
    })
    ```
  - 返 `gin.H{"groups": groupsOut}`。
  - **`cosmeticItemId` / `userCosmeticItemId` 必须字符串化**（§8.2 行 1368 / 1376 钦定 string，BIGINT 字符串化，与 §2.5 全局约定一致；`strconv.FormatUint`）。
  - **`slot` / `rarity` / `status` / `count` 是 int 直接下发**（§8.2 行 1370-1377 钦定 int，**不**字符串化）。
  - 字段名全 camelCase（§8.2 + V1 §2.4：`cosmeticItemId` / `iconUrl` / `assetUrl` / `userCosmeticItemId`）。
  - **永远**下发 `groups: []` 非 null + 每组 `instances: []` 非 null（§8.2 行 1440 防 Swift Codable 解析 nil；`make([]gin.H, 0, len)` 兜底；与 catalogResponseDTO items 非 null 同模式）。
- 完整中文注释头：handler / DTO 注释说明 §8.2 wire 字段集（任一缺失 → iOS Codable 解码失败）+ camelCase + 双 id 字符串化理由 + groups/instances 非 null 兜底 + ADR-0006 / ADR-0007 引用（参照既有 GetCatalog 注释风格）。

**AC6 — `router.go` wire + 路由注册**

修改 `server/internal/app/bootstrap/router.go`，在既有 `if deps 完整`（含 line 486 `cosmeticItemRepo`）块内：

- 新建 `userCosmeticItemRepo := repomysql.NewUserCosmeticItemRepo(deps.GormDB)`（位置：在 line 486 `cosmeticItemRepo` 构造之后、line 509 `cosmeticSvc` 构造之前）。
- 改 line 509：`cosmeticSvc := service.NewCosmeticService(cosmeticItemRepo, userCosmeticItemRepo)`（从 1 参扩到 2 参 —— 复用 line 486 既有 `cosmeticItemRepo` 实例 + 新 `userCosmeticItemRepo`，**不**新建第二个 cosmeticItemRepo）。
- 在 line 531 `authedGroup.GET("/cosmetics/catalog", ...)` 之后**新增** `authedGroup.GET("/cosmetics/inventory", cosmeticsHandler.GetInventory)`（同组同模式 —— auth + RateLimitByUserID 由 authedGroup 既有中间件链兜底，对应 §8.2 错误码 1001 / 1005，**不**单独挂中间件）。
- 注释说明本 repo 实例 + 路由由 Story 23.4 加（与既有 23.3 注释风格一致）。
- **关键**：`userCosmeticItemRepo` + `/cosmetics/inventory` 路由必须只在 `cosmeticItemRepo` 可用的同一 `if deps 完整` 块内（与 23.3 / chestHandler / emojisHandler 同块同模式；deps 不完整则该路由不注册，与既有 fallback 行为一致）。

**AC7 — 单元测试覆盖（≥5 case service + handler + repo）**

修改 `server/internal/service/cosmetic_service_test.go`：

- 扩既有 `stubCosmeticItemRepo`（若不存在则新建；现 ListCatalog 测试用的 stub）加 `listByIDsForInventoryFn` + `ListByIDsForInventory` 方法实现。
- 新建 `stubUserCosmeticItemRepo`（`listByUserForInventoryFn func(ctx, userID) ([]mysql.UserCosmeticItem, error)` + `ListByUserForInventory` 方法）。
- **既有所有 `NewCosmeticService(repo)` 调用全部改成 `NewCosmeticService(repo, ucRepo)` 2 参**（ListCatalog 既有 case 传一个 noop stubUserCosmeticItemRepo —— 否则编译不过 / 既有测试回归）。
- `ListInventory` ≥5 case（mocked 双 repo stub，对齐 epics.md 行 3287-3292 + V1 §8.2 三态/排序）：
  1. **happy（3 hat cosmeticId=12 + 1 scarf cosmeticId=24 → groups 长度=2，第一组 count=3 + instances 长度=3）**：stub userRepo 返 4 实例（3 个 cosmetic_item_id=12 status=1，1 个 cosmetic_item_id=24 status=1）+ stub cosmeticRepo `ListByIDsForInventory` 返 2 配置行（均 is_enabled=1，态 A）→ 验证 groups=2、第一组（按排序 rarity/slot/id）count=3 instances=3、第二组 count=1。
  2. **happy（0 件 → groups=[]）**：stub userRepo 返 `[]UserCosmeticItem{}` → ListInventory 返 `len==0` 且 `!= nil` 的 `[]InventoryGroup`（保证 handler 下发 groups:[]）。
  3. **happy（1 件 status=equipped → 仍包含 status=2）**：stub userRepo 返 1 实例 status=2 + config 态 A → group count=1 instances[0].Status==2（验证 equipped 不被过滤，repo SQL `status IN (1,2)` 已含 2）。
  4. **edge（态 C：config map 不命中 cosmetic_item_id → 不 skip，降级占位保留组 + log error）**：stub userRepo 返 1 实例 cosmetic_item_id=999 + stub cosmeticRepo `ListByIDsForInventory` 返 `[]`（不含 999）→ ListInventory 返 groups 长度=1（**不** skip！）第一组 `Name=="未知装扮" && Slot==99 && Rarity==1 && IconURL=="" && AssetURL=="" && Count==1`（断言态 C 降级占位 + 保留组；可选用注入 logger 断言 error 级别 log，或注释说明 log 行为由集成测试 / 人工验证覆盖）。
  5. **edge（态 B：config 命中但 is_enabled=0 → row 真实值，不 placeholder，不 log）**：stub userRepo 返 1 实例 cosmetic_item_id=50 + stub cosmeticRepo 返该配置行 `IsEnabled=0` 但 `Name="旧帽子" IconURL="https://x/i" AssetURL="https://x/a"` → ListInventory 返 group `Name=="旧帽子" && IconURL=="https://x/i"`（**用 row 真实值，非占位**；区分态 B vs 态 C 关键 case）。
  6. **edge（两级全序排序）**：stub userRepo 返多组乱序实例（不同 rarity/slot/id + 同组内 userCosmeticItemId 乱序）→ 断言 groups 按 `(rarity, slot, cosmeticItemId)` 升序 + 每组 instances 按 userCosmeticItemId 升序（逐对相邻 lexicographic 断言）。
  7. **edge（userRepo DB 错误 → 1009）**：stub userRepo 返 `errors.New("db down")` → ListInventory 返 `*apperror.AppError` `Code==ErrServiceBusy`（1009；`errors.As` 断言）。
  8. **edge（cosmeticRepo `ListByIDsForInventory` DB 错误 → 1009）**：userRepo 正常返实例 + cosmeticRepo 返 error → ListInventory 返 1009（config 关联失败也走 1009 路径）。

修改 `server/internal/app/http/handler/cosmetics_handler_test.go`：

- 既有 `stubCosmeticService` 加 `listInventoryFn func(ctx, userID) ([]service.InventoryGroup, error)` 字段 + `ListInventory` 方法实现（否则 stub 不满足扩展后的 `CosmeticService` interface → 既有 GetCatalog 测试编译红）。
- `GetInventory` ≥3 case：
  9. **happy**：stub svc 返 N 组（含 instances）→ `GetInventory` 走 httptest（需在 router 上注入 userID 到 c.Keys —— 参照 chest_handler_test / steps_handler_test 的 userID 注入中间件模式）→ response body `data.groups` 长度 N + `cosmeticItemId`/`userCosmeticItemId` 是 **string**（raw JSON 断言）+ `slot`/`rarity`/`status`/`count` 是 **number** + 全 camelCase + HTTP 200 code=0。
  10. **edge（空背包 → groups:[] 非 null + 每组 instances:[] 非 null）**：stub svc 返 `[]InventoryGroup{}` → response `data.groups` 是 `[]` 而非 `null`（raw JSON 层断言）。
  11. **edge（缺 userID in context → 1009 unreachable 兜底）**：构造不注入 userID 的 router → `GetInventory` → envelope code=1009（参照 chest_open_handler_test.go case 7 "MissingUserIDInContext_Returns1009"）。
  12. **edge（service 返 1009 → handler c.Error 透传）**：stub svc 返 `*apperror.AppError{Code:1009}` → handler 不 panic，error 进 c.Errors → envelope code=1009。

修改 `server/internal/repo/mysql/cosmetic_item_repo_test.go`：补 `ListByIDsForInventory` 测试（既有文件用 sqlmock 单测 → 同模式补：HappyPath（IN 多 id 返多行）+ EmptyIDs（ids=[] 早返空 slice 不发 SQL）+ NoMatch（IN 不命中返空）；若既有用 dockertest 则按既有文件现状判定放集成测试）。

新建 `server/internal/repo/mysql/user_cosmetic_item_repo_test.go`：`ListByUserForInventory` 测试（与 cosmetic_item_repo_test.go 同测试风格 —— sqlmock 单测或 dockertest，按既有 mysql repo 测试现状判定：HappyPath（user 有多实例 status 1/2 → 返全部）+ StatusFiltered（构造 status=3 行 → SQL `WHERE status IN (1,2)` 不返）+ EmptyResult（user 无实例 → 返空 slice 非 nil））。

**AC8 — 集成测试覆盖（dockertest，手工 INSERT user_cosmetic_items）**

修改 `server/internal/service/cosmetic_service_integration_test.go`：

- 扩 `buildCosmeticServiceIntegration` helper：装配时**新增** `userCosmeticItemRepo := mysql.NewUserCosmeticItemRepo(gormDB)` + 改 `service.NewCosmeticService(cosmeticItemRepo, userCosmeticItemRepo)` 2 参（**回归点**：既有 catalog 集成 case 复用同 helper，扩签名后 helper 内 NewCosmeticService 调用必须同步改 2 参，否则既有 catalog 集成测试编译红）。helper 返回值若需暴露 raw *sql.DB 给 INSERT，沿用既有 `sqlDB *sql.DB` 返回（DisabledExcluded case 已用 raw SQL UPDATE，本 story INSERT 同模式复用）。
- 新增 `TestCosmeticServiceIntegration_ListInventory_GroupsAndInstances`：migrate 后用 raw `sqlDB.Exec` 手工 INSERT 1 个 user + 5 个不同 cosmetic 实例（关联 0012 seed 已存在的 cosmetic_item_id，如 hat_yellow / hat_red 等真实 id；含不同 status 1/2 + 同 cosmetic 多实例验证聚合）→ 跑 `svc.ListInventory(ctx, userID)` → 断言：
  - groups 数量 = 不同 cosmetic_item_id 数；含同 cosmetic 多实例组 `Count == 实例数` + `len(Instances) == Count`。
  - 两级排序：groups 按 `(rarity, slot, cosmeticItemId)` 升序、每组 instances 按 userCosmeticItemId 升序（逐对断言）。
  - status=2(equipped) 实例仍在 inventory；构造 1 个 status=3(consumed) 实例 → 验证**不**出现在任何 group（SQL `status IN (1,2)` 过滤真值）。
- 新增 `TestCosmeticServiceIntegration_ListInventory_EmptyBag`：migrate 后不 INSERT 任何 user_cosmetic_items → `ListInventory(ctx, 任意userID)` → `len(got)==0 && got != nil`（空背包 {groups:[]} 真值）。
- （可选增强）`TestCosmeticServiceIntegration_ListInventory_DisabledConfigStillVisible`：INSERT 实例关联某 cosmetic → `UPDATE cosmetic_items SET is_enabled=0 WHERE id=?` → `ListInventory` 仍返回该组且用 row 真实值（态 B 真值；验证 `ListByIDsForInventory` 无 `is_enabled=1` 过滤）。
- **注**：inventory 集成测试**必须**手工 INSERT user_cosmetic_items（与 catalog 集成测试**不同** —— catalog 复用 0012 seed 闭环不手工 INSERT，但 0012/0015 seed 不含 user_cosmetic_items 行，inventory 无数据可复用，故必须手工 INSERT；这是与 23.3 catalog 集成测试的关键差异，在测试注释头写明）。

**AC9 — 构建 / 测试通过**

- `bash scripts/build.sh --test` 通过（vet + build + 全量单测 `go test -count=1 ./...`；本 story 改/新增多个 Go 文件 + `NewCosmeticService` 扩签名影响 router.go / 既有测试，必须跑确认无 build 红 / 无回归）。
- `bash scripts/build.sh --integration` 通过（`-tags=integration` 跑本 story 新增 inventory 集成测试 + 既有 catalog 集成测试无回归；dockertest 起真实 MySQL 容器）。
  - **注**：若 `go test ./...` 串跑大量 dockertest case 时本机 docker daemon 被压垮导致个别**不相关包** per-package timeout（已知环境侧 flake，Story 23.2 / 23.3 Debug Log 已记录同现象），隔离单跑本 story 范围（`go test -tags=integration -timeout=600s ./internal/service/ -run TestCosmeticServiceIntegration`）确认全 PASS，并在 Debug Log 如实记录环境侧 flake 与本 story 改动无因果关系。
- 全量测试无回归（既有 catalog / emoji / chest / home / room service + handler 测试全绿；`ListEnabledForWeightedPick` / `ListEnabledForCatalog` 既有路径不受影响 —— 本 story **新增**独立方法 + 扩 `NewCosmeticService` 签名同步改全部调用方；router.go 既有路由不受影响 —— 仅新增 1 路由）。
- `git status` 仅出现范围红线内文件改动（≤10 个 server 文件 + story 文件 + sprint-status.yaml）。

**AC10 — 跨文档一致性自检（对外接口 story 必须项）**

完成 AC1~AC6 后，必须逐项核对并在本 story "Completion Notes List" 记录核对结论：

1. handler `inventoryResponseDTO` wire 字段集 / 类型 / camelCase 与 V1 §8.2 `data.groups[]` + `instances[]` 字段表（行 1366-1377）**逐字段比对一致**：`cosmeticItemId`(string) / `name`(string) / `slot`(int,§6.8) / `rarity`(int,§6.9) / `iconUrl`(string) / `assetUrl`(string) / `count`(int) / `instances[].userCosmeticItemId`(string) / `instances[].status`(int,{1,2})；无多余 / 无缺失；两个 id 字段确为 string 非 number；**不**含 code 字段（§8.2 groups[] 无 code，与 §8.1 catalog 有 code 不同）。
2. service `ListInventory` + repo `ListByUserForInventory` / `ListByIDsForInventory` 与 §8.2 服务端逻辑步骤 2-6（行 1336-1360）**一致**：`WHERE user_id=? AND status IN (1,2)` + 按 cosmetic_item_id 聚合 + config 关联**无** `is_enabled=1` 过滤 + 三态 A/B/C 全覆盖 + 两级全序排序 + 不分页 + 不开事务。
3. **config 三态矩阵实装与 §8.2 行 1342-1353 + 1437 表格逐态比对**：态 A enabled→row 真实值无 log；态 B disabled-but-exists→row 真实值（非 placeholder）无 log；态 C missing-no-row→降级占位（Name="未知装扮"/Slot=99/Rarity=1/空 URL）+ `slog.ErrorContext` log；三态均保留组**不** skip。确认**未**按 epics.md 行 3292 "skip + warning" 实装（已识别冲突，以 V1 §8.2 为准）。
4. **两级排序与 §8.2 行 1355-1358 + 1443 比对**：`groups[]` = `rarity ASC, slot ASC, cosmeticItemId ASC`（态 C 用占位 rarity=1/slot=99 归位）；`instances[]` = `userCosmeticItemId ASC`；service 层 `sort.Slice` 显式排序**不**依赖 SQL/map 迭代顺序。
5. 错误码映射与 §8.2 错误码表（行 1426-1432）**一致**：DB 异常（userRepo / cosmeticRepo 任一）→ 1009；auth/rate_limit 由 router 中间件兜底（1001/1005）；缺 userID → 1009 unreachable 兜底；**不**触发 1002（GET 无 body/query）/ 1003 / 5001（单组配置缺失/禁用**不**报错）；空背包 → {groups:[]} code=0。
6. `NewCosmeticService` 扩签名后全部调用方（router.go line 509 + cosmetic_service_test.go 全部 case + cosmetic_service_integration_test.go buildCosmeticServiceIntegration）**已同步改 2 参**；`git status` + `bash scripts/build.sh --test` 实测无 build 红 / 无回归。
7. 未触碰 23.2 `UserCosmeticItem` struct / `TableName()` / Story 20.6 `ListEnabledForWeightedPick` / 23.3 `ListEnabledForCatalog` / `ChestService.OpenChest` / dev_cosmetic_service.go / V1 §8.2 契约 / §5.9 / §5.8 / §6.8/6.9/6.10 枚举 / 其他 docs / 0001~0015 既有 migration / 0012/0015 seed。`git status` 实测仅范围红线内文件。
8. 目录布局纠偏（沿用 23.3：CLAUDE.md target §4 提 `internal/domain/cosmetic/`，实际扁平 `internal/service/` + `internal/app/http/handler/` + `internal/repo/mysql/`）已在 Dev Notes + Completion Notes 显式记录，**不**视为架构变更（与 17.4 / 20.5 / 23.3 一致）。

## Tasks / Subtasks

- [x] Task 1：读取并定位（AC1~AC6 / AC10）
  - [x] 读 V1 §8.2（行 1313-1444）GET /cosmetics/inventory 完整契约（接口元信息 + 服务端逻辑步骤 1-7 + config 三态矩阵表 + 两级排序段 + 响应体字段表 + 错误码表 + 关键约束段）
  - [x] 读 §1 行 78-79（§8.2 冻结边界 —— 哪些算契约变更哪些不算）
  - [x] 读数据库设计 §5.9 user_cosmetic_items DDL + §6.10 status 枚举（1=in_bag/2=equipped/3=consumed/4=invalid）+ §5.8 cosmetic_items + §6.8 slot + §6.9 rarity
  - [x] 读 epics.md 行 3269-3293（§Story 23.4 AC）+ **识别行 3292 "skip + log warning" 与 V1 §8.2 三态矩阵冲突点**（以 V1 §8.2 为准）
  - [x] 读参照 `server/internal/repo/mysql/cosmetic_item_repo.go`（全文，`CosmeticItemRepo` interface + `ListEnabledForCatalog` impl 模板 + 注释风格）
  - [x] 读 `server/internal/repo/mysql/user_cosmetic_item_repo.go`（全文，23.2 落地的 `UserCosmeticItem` struct + `TableName()`；确认**无** interface / 方法，本 story 首次加）
  - [x] 读 `server/internal/service/cosmetic_service.go`（全文，`CosmeticService` / `ListCatalog` / `CosmeticBrief` / `NewCosmeticService` 现状 —— 确认扩签名影响面）
  - [x] 读 `server/internal/app/http/handler/cosmetics_handler.go`（全文，`GetCatalog` / `catalogResponseDTO` 模板）+ `chest_handler.go` 行 59-81（GetCurrent userID 提取 1:1 模板）
  - [x] 读 `server/internal/app/bootstrap/router.go` 行 483-534（cosmeticItemRepo wire + cosmeticSvc 构造 + authedGroup 路由段）
  - [x] 读 `server/internal/service/cosmetic_service_test.go` + `cosmetics_handler_test.go` + `cosmetic_item_repo_test.go` + `cosmetic_service_integration_test.go` + `chest_open_service_test.go` + `chest_open_service_integration_test.go`（确认 stub 模式 + `NewCosmeticService` / `CosmeticItemRepo` 调用点全集 + handler userID 注入测试模式 + dockertest helper）
  - [x] 确认 apperror 码（ErrServiceBusy=1009 / ErrUnauthorized=1001 / ErrTooManyRequests=1005 / ErrInvalidParam=1002 不触发）+ `middleware.UserIDKey` + `slog.ErrorContext` 用法
- [x] Task 2：新建 UserCosmeticItemRepo interface + ListByUserForInventory（AC1）
  - [x] 在既有 user_cosmetic_item_repo.go 追加 interface + impl + struct + 构造函数（**不**改 23.2 struct/TableName）
  - [x] SQL `Select("id, cosmetic_item_id, status")` + `Where("user_id = ? AND status IN ?", userID, []int8{1,2})` + nil slice 兜底
  - [x] 完整中文注释（§8.2 来源 + status 过滤理由 + 范围红线）
- [x] Task 3：CosmeticItemRepo 新增 ListByIDsForInventory（AC2）
  - [x] interface 加方法 + 注释（说明 SQL + **禁止 is_enabled=1 过滤**理由 + 三方法语义边界）
  - [x] impl：`if len(ids)==0 早返` + `Select` 列（含 is_enabled 供 service 区分态 A/B；无 code）+ `Where("id IN ?", ids)` + nil slice 兜底
  - [x] **不**改 / 不复用 `ListEnabledForWeightedPick` / `ListEnabledForCatalog`（git diff 仅新增方法 + interface 注释扩充）
- [x] Task 4：扩展 cosmetic_service.go（AC3 + AC4）
  - [x] `CosmeticService` 加 `ListInventory` + `InventoryGroup` / `InventoryInstance` DTO + struct 加 userCosmeticItemRepo 字段
  - [x] `NewCosmeticService` 扩签名 2 参（所有调用方同步改）
  - [x] `ListInventory` impl：查实例 → 空返 [] → 聚合 → 关联配置 → **config 三态 A/B/C 全覆盖**（态 C `slog.ErrorContext` log + 降级占位保留组，**不** skip）→ **两级 sort.Slice 全序排序** → Count=len(Instances)
  - [x] 完整中文注释（§8.2 来源 + 三态矩阵 + 两级排序契约必需 + epics.md 冲突纠偏注明 + 范围红线）
- [x] Task 5：扩展 cosmetics_handler.go（AC5）
  - [x] `GetInventory`：c.Get(UserIDKey) 取 userID（!ok/类型断言失败 → 1009 兜底，chest_handler 同模式）+ c.Request.Context() + c.Error 透传 + response.Success
  - [x] `inventoryResponseDTO`：双 id 字符串化 + slot/rarity/status/count int + 全 camelCase + groups/instances 非 null 兜底 + **不**下发 code
  - [x] import middleware + apperror（既有文件未 import，GetInventory 首次需要）
  - [x] 完整中文注释（§8.2 wire 字段集 + 双 id 字符串化 + 非 null 兜底 + ADR-0006/0007）
- [x] Task 6：router.go wire + 路由注册（AC6）
  - [x] 新建 `userCosmeticItemRepo := repomysql.NewUserCosmeticItemRepo(deps.GormDB)`
  - [x] 改 cosmeticSvc 构造 `NewCosmeticService(cosmeticItemRepo, userCosmeticItemRepo)` 2 参
  - [x] `authedGroup.GET("/cosmetics/inventory", cosmeticsHandler.GetInventory)` + 注释
- [x] Task 7：单元测试（AC7）
  - [x] cosmetic_service_test.go：扩 stub repo（加 ListByIDsForInventory）+ 新建 stubUserCosmeticItemRepo + 既有 NewCosmeticService 调用全改 2 参 + 8 ListInventory case（含态 C 不 skip 保留组、态 B row 真实值、两级排序、双 DB 错误 1009）
  - [x] cosmetics_handler_test.go：stubCosmeticService 加 listInventoryFn + ListInventory 方法 + 4 GetInventory case（happy raw JSON 双 id string、空背包 groups/instances:[]、缺 userID 1009、service 1009 透传）
  - [x] cosmetic_item_repo_test.go：补 ListByIDsForInventory（HappyPath 含 is_enabled=0 行不被过滤 / EmptyIDs 不发 SQL / NoMatch）
  - [x] 新建 user_cosmetic_item_repo_test.go：ListByUserForInventory（HappyPath / StatusFilteredInSQL / EmptyResult）
  - [x] chest_open_service_test.go：既有 stubCosmeticItemRepo 加 ListByIDsForInventory（satisfy 扩展后 interface 编译；防御性 panic）
- [x] Task 8：集成测试（AC8）
  - [x] 扩 buildCosmeticServiceIntegration（注入 userCosmeticItemRepo + NewCosmeticService 2 参）
  - [x] ListInventory_GroupsAndInstances（手工 INSERT user + hat_yellow x3 含 equipped + hat_red x1 + hat_chef status=3 → groups + count + 两级排序 + consumed 不出现）
  - [x] ListInventory_EmptyBag（不 INSERT → {groups:[]} 非 nil）
  - [x] ListInventory_DisabledConfigStillVisible（态 B 真值 —— UPDATE is_enabled=0 后仍可见 + row 真实值）
  - [x] chest_open_service_integration_test.go：faultCosmeticItemRepoOnList 加 ListByIDsForInventory（integration tag 下 satisfy 扩展后 interface 编译）
- [x] Task 9：构建 / 测试（AC9）
  - [x] `bash scripts/build.sh --test`（vet + build + 全量单测 → PASS，确认 NewCosmeticService 扩签名无 build 红 / 无回归）
  - [x] `go vet -tags devtools ./internal/service/`（devtools tag 下也编译通过 exit 0；lexLEService 独立命名避开 23.3 itoa 撞名）
  - [x] inventory 集成测试隔离单跑 PASS（`go test -tags=integration -timeout=600s ./internal/service/ -run TestCosmeticServiceIntegration` → 5/5 PASS，含 2 catalog 既有 case 无回归 + 3 新 inventory case）
  - [x] 确认无回归 + `git status` 仅范围红线内文件
- [x] Task 10：跨文档一致性自检（AC10）
  - [x] 逐项核对 AC10 的 8 条，结论写入 "Completion Notes List"
  - [x] 显式记录 epics.md 行 3292 "skip+warning" vs V1 §8.2 三态矩阵冲突纠偏（以 §8.2 为准）+ 目录布局纠偏（沿用 23.3）
  - [x] 标记 sprint-status.yaml `23-4-get-cosmetics-inventory-接口` 状态流转 ready-for-dev → in-progress → review

## Dev Notes

### 这是什么类型的 story

第二个对外只读 REST 接口 story（Epic 23 内）。**GET /cosmetics/catalog（23.3）是本 story 最贴近的同模块姊妹模板**（同为 cosmetic 模块、auth-required、无 query 参数、单查、不开事务、§8.x 冻结契约），但本 story **复杂度显著高于 23.3**：

| 维度 | catalog（23.3 模板） | inventory（本 story） |
|---|---|---|
| 数据源 | `cosmetic_items` 配置表（与 user 无关） | `user_cosmetic_items` 实例表（**读 userID**）+ 关联 `cosmetic_items` |
| 读 userID | **不**读（全局静态目录） | **必须**读（c.Get(UserIDKey)，chest_handler.GetCurrent 模板） |
| repo 现状 | `CosmeticItemRepo` 已存在（20.6），加 1 方法 | `UserCosmeticItemRepo` **不存在**（23.2 仅 struct），**首次建 interface** + 加 `CosmeticItemRepo.ListByIDsForInventory` |
| 聚合 | 无（平铺 items） | **按 cosmetic_item_id 聚合**（groups + count + instances） |
| config 关联 | 无 | **config 三态完整矩阵 A/B/C**（最核心难点） |
| 排序 | repo SQL `ORDER BY rarity,slot,id` 一级 | **service 层 sort.Slice 两级全序**（groups + instances；不能靠 SQL） |
| 构造签名 | `NewCosmeticService(repo)` | **扩签名 `NewCosmeticService(repo, ucRepo)`**（回归点） |
| 集成测试数据 | 复用 0012 seed（不手工 INSERT） | **必须手工 INSERT** user_cosmetic_items（seed 无该表数据） |

dev 实装时**逐文件对照 cosmetic_service.go / cosmetics_handler.go / cosmetic_item_repo.go / cosmetic_service_integration_test.go 的 catalog 实装改写为 inventory 版本** + userID 提取对照 `chest_handler.go` GetCurrent，差异点见上表。

### 最核心难点（按风险排序）

1. **config 三态矩阵 A/B/C 必须全覆盖，且与 epics.md AC 文字冲突**：epics.md 行 3292 写"配置不存在 → skip + log warning"，V1 §8.2 冻结契约（行 1342-1353 + 1437）已收紧为三态矩阵且明确**禁止 skip**（态 C 降级占位**保留**组，因"已拥有不得静默丢失"是用户可见数据丢失回归）+ **log error 不是 warning**。**dev 必须按 V1 §8.2 实装，识别并记录此冲突**（以契约文档为准 —— CLAUDE.md "状态以 server 为准" + 23.3 r1 同源原则）。漏处理任一态（尤其漏态 B / 把态 C 写成 skip）= 契约违背 + review 必拦。
2. **`ListByIDsForInventory` 禁止 `is_enabled=1` 过滤**：这是态 B 契约的根因 —— 若误加（如照抄 `ListEnabledForCatalog`），admin 下架配置后已拥有道具会从 inventory 消失（用户可见数据丢失回归）。故本 story **新增独立** `ListByIDsForInventory`（无 `is_enabled` 过滤），**不**复用 `ListEnabledForCatalog`。区分态 A/B 靠 config map 命中后读 `CosmeticItem.IsEnabled`，区分态 B/C 靠 config map 是否命中（必须 SELECT `is_enabled` 列）。
3. **两级确定性全序排序必须 service 层 `sort.Slice` 显式做**：§8.2 行 1355-1358 + 1443 钦定 `groups[]`=`rarity,slot,cosmeticItemId` + `instances[]`=`userCosmeticItemId`，**不**得依赖 SQL/Go map 迭代顺序（Go map range 无序 + GORM Find 无 ORDER BY → 跨请求重排 → iOS Wardrobe Tab UI 抖动）。态 C 组用降级占位 rarity=1/slot=99 参与排序。
4. **`NewCosmeticService` 扩签名是 build 回归点**：现 1 参构造被 router.go line 509 + cosmetic_service_test.go 全部 case + cosmetic_service_integration_test.go helper 调用，扩 2 参后**全部同步改**否则 build 红。`bash scripts/build.sh --test` 必跑确认。
5. **`UserCosmeticItem` struct 注释明示本 story 范围**：23.2 落地的 struct 注释（user_cosmetic_item_repo.go 行 44-52）明确写"本 story 阶段**不**新建 UserCosmeticItemRepo interface ... 23.4（GET /cosmetics/inventory 聚合查询）... 提供字段映射"——即本 story 是 `ListByUserForInventory` 的预期 owner，但 **23.5 才加 BatchCreate 写方法**（本 story 只加只读查询方法，不预实装写）。

### 关键契约锚点（V1 §8.2 行号速查，dev 实装时逐条对照）

- 行 1329-1330：不分页 + 无 query 参数
- 行 1339-1341：步骤 2 SQL `WHERE user_id=? AND status IN (1,2)` + 空背包 {groups:[]}
- 行 1342-1353：步骤 3 config 三态矩阵 A/B/C 完整表 + 态 B 用 row 真实值理由 + 态 C 降级占位值 + 态 C log error 非 warning
- 行 1354-1358：步骤 4 组装 + 步骤 5 两级确定性全序排序（groups + instances 排序键）
- 行 1359-1360：步骤 6 空背包 {groups:[]} + 步骤 7 不开事务（无论 JOIN 还是分步查，两级排序契约必需）
- 行 1366-1377：响应体字段表（9 字段；两 id string，slot/rarity/status/count int；**无 code 字段**）
- 行 1426-1432：错误码表（1001/1005/1009；不触发 1002；空背包/三态均不报错）
- 行 1436-1444：关键约束段（聚合维度 / 可见性+三态 / status 过滤 / count 语义 / 空背包 / userCosmeticItemId 同义 / URL 非空 / 两级排序 / 不分页 全部 freeze）

### 实装模式锚点（既有代码 1:1 参照）

- repo 层 `ListByUserForInventory` / `ListByIDsForInventory` impl → 抄 `cosmetic_item_repo.go` `ListEnabledForCatalog`（行 149-177）：`tx.FromContext(ctx, r.db).WithContext(ctx)` + 显式 `Select` + `Where` + `Find` + nil slice 兜底；差异：无 `is_enabled=1` 过滤、无 ORDER BY、`Where` 条件不同
- service `ListInventory` 错误 wrap → 抄 `cosmetic_service.go` `ListCatalog`（行 99-124）：repo 失败 `apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])` + `make([]T, 0, len)` 兜底非 nil
- handler `GetInventory` userID 提取 → 抄 `chest_handler.go` `GetCurrent`（行 59-81）：`c.Get(middleware.UserIDKey)` + `!ok` / 类型断言失败 → `apperror.New(apperror.ErrServiceBusy, ...)` 1009 兜底 + `c.Request.Context()` + `c.Error` 透传 + `response.Success`
- handler `inventoryResponseDTO` 字符串化 + camelCase + 非 null 兜底 → 抄 `cosmetics_handler.go` `catalogResponseDTO`（行 98-121）：`strconv.FormatUint` + `make([]gin.H, 0, len)` + camelCase 字段名
- router wire → 抄 `router.go` 行 504-531（23.3 cosmeticSvc/handler wire + authedGroup.GET 注册）
- 集成测试 helper → 抄 `cosmetic_service_integration_test.go` `buildCosmeticServiceIntegration`（行 46+）；handler 单测 userID 注入 → 找 chest_handler_test / steps_handler_test 的 userID 注入中间件模式（`r.Use(func(c){ c.Set(middleware.UserIDKey, uint64(...)) })`）
- 态 C log → 抄 `dev_cosmetic_service.go` 行 120 `slog.WarnContext(ctx, msg, k, v...)` 风格但用 **`slog.ErrorContext`**（Error 级别）

### Project Structure Notes

- **目录布局纠偏（沿用 23.3 已确立结论）**：CLAUDE.md target 目录形态 §4 + 23.1 行 281 提 `internal/domain/cosmetic/`，但 server 工程**实际现状**为扁平 `internal/service/*.go` + `internal/app/http/handler/*.go` + `internal/repo/mysql/*.go`，**无** `internal/domain/`。本 story **严格遵循 server 实际现状**，与 23.3 落地的 `cosmetic_service.go` / `cosmetics_handler.go` / `cosmetic_item_repo.go` 同级追加，**不**引入 `internal/domain/cosmetic/`。与 ADR-0006 / 17.4 / 20.5 / 23.3 实装一致；属"CLAUDE.md target 是 aspirational、实际工程演进为扁平"的历史现状，**不**视为架构变更。
- `user_cosmetic_item_repo.go` 是 23.2 已创建文件（现仅 struct + TableName），本 story 在**同文件追加** interface / impl（**不**新建文件 —— 与 cosmetic_item_repo.go 同文件含 struct + interface + impl 的组织模式一致）。
- 端独立原则（CLAUDE.md）：本 story 是纯 server 只读接口，测试自包含（dockertest 起真实 MySQL，**不**依赖 iPhone App / 真机）；跨端契约通过 V1 §8.2（已 23.1 冻结）同步，**不**通过共享代码。

### References

- [Source: docs/宠物互动App_V1接口设计.md#8.2 GET /api/v1/cosmetics/inventory]（行 1313-1444：接口元信息 + 服务端逻辑步骤 1-7 + config 三态矩阵 A/B/C + 两级全序排序 + 响应体字段表 + 错误码表 + 关键约束）
- [Source: docs/宠物互动App_V1接口设计.md#1 契约冻结边界]（行 78-79：§8.2 自 2026-05-16 冻结 + 哪些算/不算契约变更）
- [Source: docs/宠物互动App_数据库设计.md#5.9 user_cosmetic_items]（行 483-522：表 DDL + 字段说明）
- [Source: docs/宠物互动App_数据库设计.md#6.10 user_cosmetic_items.status]（status 枚举 1=in_bag/2=equipped/3=consumed/4=invalid）
- [Source: docs/宠物互动App_数据库设计.md#5.8 cosmetic_items + #6.8 slot + #6.9 rarity]（关联配置元信息 + slot/rarity 枚举值域）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 23.4]（行 3269-3293：AC —— **注**：行 3292 "skip+warning" 与 V1 §8.2 三态矩阵冲突，以 §8.2 为准）
- [Source: _bmad-output/implementation-artifacts/23-3-get-cosmetics-catalog-接口.md]（catalog 接口实装模板 + 目录布局纠偏结论 + cosmetic 模块代码风格）
- [Source: server/internal/service/cosmetic_service.go]（ListCatalog / CosmeticBrief / NewCosmeticService 现状 —— 扩签名影响面）
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go]（CosmeticItemRepo interface + ListEnabledForCatalog impl 模板 + 三方法语义边界注释风格）
- [Source: server/internal/repo/mysql/user_cosmetic_item_repo.go]（23.2 落地 UserCosmeticItem struct + TableName + 行 44-52 范围红线注释明示本 story 是 ListByUserForInventory owner）
- [Source: server/internal/app/http/handler/chest_handler.go]（行 59-81：GetCurrent userID 提取 + 1009 unreachable 兜底 1:1 模板）
- [Source: server/internal/app/http/handler/cosmetics_handler.go]（行 98-121：catalogResponseDTO 字符串化 + camelCase + 非 null 兜底模板）
- [Source: server/internal/app/bootstrap/router.go]（行 483-534：cosmeticItemRepo wire + cosmeticSvc 构造 + authedGroup 路由注册段）
- [Source: server/internal/service/cosmetic_service_integration_test.go]（buildCosmeticServiceIntegration dockertest helper + catalog 集成 case 模板）
- [Source: server/internal/pkg/errors/codes.go]（ErrServiceBusy=1009 / ErrUnauthorized=1001 / ErrTooManyRequests=1005 / ErrInvalidParam=1002）
- [Source: CLAUDE.md]（工作纪律：状态以 server 为准 / ctx 必传 ADR-0007 / 错误码统一 / 端独立原则 / build & test 脚本契约）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（bmad-dev-story workflow，epic-loop 派出子任务）

### Debug Log References

- `bash scripts/build.sh --test`：vet + build（commit=a4cecfa）+ 全量单测 `go test -count=1 ./...` 全 `ok`，BUILD SUCCESS。
- `go vet -tags devtools ./internal/service/`：exit 0（devtools tag 下编译通过；本 story service_test 新增 `lexLEService` 用独立命名避开 23.3 出现过的 itoa 撞名中间态；`buildCosmeticInventoryService` / `stubUserCosmeticItemRepo` 等新符号无 devtools tag 下重声明冲突）。
- `go test -tags=integration -timeout=600s ./internal/service/ -run TestCosmeticServiceIntegration -v`：5/5 PASS（ListCatalog_SeedContent / ListCatalog_DisabledExcluded 既有 catalog 无回归 + ListInventory_GroupsAndInstances / ListInventory_EmptyBag / ListInventory_DisabledConfigStillVisible 新增）。日志中大量 `[mysql] connection.go:49: unexpected EOF` 为 dockertest MySQL 容器 readiness 轮询的正常现象（非测试失败；与 Story 23.2/23.3 Debug Log 记录同源环境侧行为），与本 story 改动无因果关系。
- **中间发现并修复的回归点**：`NewCosmeticService` 扩 2 参后，除 story 钦定 4 个回归点外，**integration build tag** 下的 `chest_open_service_integration_test.go` 的 `faultCosmeticItemRepoOnList` 也实现 `CosmeticItemRepo` interface，需补 `ListByIDsForInventory`（否则 `--integration` 编译红）。标准 `--test` 不编译 integration tag 文件故未在第一次 build 暴露，集成测试隔离单跑时首次暴露并已修复（防御性 panic stub，不改 20.6 任何既有行为）。这与 23.3 经验一致：`CosmeticItemRepo` interface 扩方法时所有 build tag 下的 stub 都要同步。

### Completion Notes List

实装完成，所有 AC1~AC9 满足，AC10 跨文档一致性自检逐条结论如下：

1. **handler wire 字段集 vs §8.2 行 1366-1377 逐字段一致**：`inventoryResponseDTO` 输出 `cosmeticItemId`(string,strconv.FormatUint) / `name`(string) / `slot`(int,§6.8) / `rarity`(int,§6.9) / `iconUrl`(string) / `assetUrl`(string) / `count`(int) / `instances[].userCosmeticItemId`(string) / `instances[].status`(int,{1,2})；无多余无缺失；两 id 确为 string（handler 单测 raw JSON 断言 `"12"` / `"90001"` 带引号）；slot/rarity/status/count 确为 number（raw JSON 无引号）；**不**含 code 字段（handler 单测 allowed 集严格断言无 code，与 §8.1 catalog 含 code 区分）。✅
2. **service/repo vs §8.2 服务端逻辑步骤 2-6 一致**：repo `ListByUserForInventory` = `WHERE user_id=? AND status IN (1,2)` 显式 3 列；service 按 cosmetic_item_id 聚合；config 关联走 `ListByIDsForInventory`（**无** is_enabled=1 过滤）；三态 A/B/C 全覆盖；两级 sort.Slice 全序；不分页；不开事务。✅
3. **config 三态矩阵 vs §8.2 行 1342-1353 + 1437 逐态比对**：态 A enabled（map 命中+IsEnabled==1）→ row 真实值无 log；态 B disabled-but-exists（map 命中+IsEnabled==0）→ **row 真实值（非 placeholder）无 log**；态 C missing-no-row（map 不命中）→ 降级占位（Name="未知装扮"/Slot=99/Rarity=1/IconURL=""/AssetURL=""）+ `slog.ErrorContext`；三态均保留组**不** skip。**已识别 epics.md 行 3292 "skip + log warning" 与 §8.2 三态矩阵冲突，按 V1 §8.2 实装（态 C 不 skip + log error 非 warning），未按 epics.md "skip + warning"**（CLAUDE.md "状态以 server 为准" + 23.3 r1 同源原则）。单测 case TestCosmeticService_ListInventory_StateC_MissingConfig_PlaceholderNotSkip（断言 len(got)==1 保留组 + 降级占位）/ StateB_DisabledButExists_UsesRealValues（断言 row 真实值非占位）严格覆盖；集成测试 DisabledConfigStillVisible 验证态 B 真实 SQL 真值。✅
4. **两级排序 vs §8.2 行 1355-1358 + 1443 比对**：`groups[]` = `sort.Slice` by (Rarity ASC, Slot ASC, CosmeticItemID ASC)；每组 `instances[]` = `sort.Slice` by UserCosmeticItemID ASC；service 层显式 sort.Slice **不**依赖 SQL/map 迭代顺序（repo 无 ORDER BY，注释明示两级排序在 service 做）；态 C 组用占位 Rarity=1/Slot=99 参与排序。单测 TestCosmeticService_ListInventory_TwoLevelDeterministicTotalOrder（4 组乱序 → wantOrder + 逐对 lexLE + 同组 instances id 升序）+ 集成测试逐对断言覆盖。✅
5. **错误码映射 vs §8.2 行 1426-1432 一致**：userRepo / cosmeticRepo 任一 DB 异常 → `apperror.Wrap(..., ErrServiceBusy)` = 1009（含 cause 保留 errors.Is 穿透）；auth(1001)/rate_limit(1005) 由 authedGroup 中间件兜底；缺 userID → 1009 unreachable 兜底（chest_handler 同模式）；**不**触发 1002（GET 无 body/query）/ 1003 / 5001（单组态 B/C 不报错保留组）；空背包 → {groups:[]} code=0。单测 case 7/8（双 DB 错误 1009）+ handler case 11/12（缺 userID 1009 + service 1009 透传）覆盖。✅
6. **NewCosmeticService 扩签名调用方全同步改**：router.go cosmeticSvc 构造（新建 userCosmeticItemRepo + 2 参）+ cosmetic_service_test.go（buildCosmeticService noop user stub + 新 buildCosmeticInventoryService）+ cosmetic_service_integration_test.go buildCosmeticServiceIntegration（注入 userCosmeticItemRepo + 2 参）全部已改；**额外**发现 chest_open_service_test.go `stubCosmeticItemRepo` + chest_open_service_integration_test.go `faultCosmeticItemRepoOnList` 实现 `CosmeticItemRepo` interface 需补 `ListByIDsForInventory`（已补防御性 panic stub）。`bash scripts/build.sh --test` + `--integration` 隔离单跑实测无 build 红 / 无回归。✅
7. **未触碰禁改项**：23.2 `UserCosmeticItem` struct / `TableName()` 未改（仅同文件追加 interface/impl）；20.6 `ListEnabledForWeightedPick` / 23.3 `ListEnabledForCatalog` impl 未改（仅扩 interface 注释 + 新增独立 `ListByIDsForInventory`）；`ChestService.OpenChest` / chest_service.go / dev_cosmetic_service.go 未碰；V1 §8.2 / §5.9 / §5.8 / §6.8/6.9/6.10 / 其他 docs 未改；0001~0015 migration / 0012/0015 seed 未改。`git status` 实测仅范围红线内文件（10 server 文件 + 1 新 repo 测试文件 + story 文件 + sprint-status.yaml）。✅
8. **目录布局纠偏（沿用 23.3）**：CLAUDE.md target §4 / 23.1 行 281 提 `internal/domain/cosmetic/`，实际工程为扁平 `internal/service/` + `internal/app/http/handler/` + `internal/repo/mysql/`；本 story 严格遵循实际现状与 23.3 落地的 cosmetic_service.go / cosmetics_handler.go / cosmetic_item_repo.go 同级追加，**不**引入 `internal/domain/cosmetic/`。属"CLAUDE.md target 是 aspirational、实际工程演进为扁平"历史现状，**不**视为架构变更（与 ADR-0006/17.4/20.5/23.3 一致）。✅

**关键纠偏点逐条确认（create-story 锚定的 4 个高危点）**：
- 纠偏点 1（契约权威 = V1 §8.2 三态矩阵，禁 skip）：三态 A/B/C 全实现，态 C 降级占位**保留组不 skip** + `slog.ErrorContext` log error（非 epics.md warning）。✅
- 纠偏点 2（ListByIDsForInventory 禁 is_enabled=1 过滤）：新增独立 repo 方法，SQL `SELECT ... WHERE id IN ?` **无** is_enabled=1 过滤；含 is_enabled 列供 service 区分态 A/B；集成测试 DisabledConfigStillVisible 验证 admin 下架后已拥有项仍可见。✅
- 纠偏点 3（两级确定性全序 service 层 sort.Slice）：groups (rarity,slot,cosmeticItemId) + instances userCosmeticItemId，service 层显式 sort.Slice 不依赖 SQL/map 顺序。✅
- 纠偏点 4（NewCosmeticService 扩签名 build 回归）：router.go + cosmetic_service_test.go + 集成测试 helper + chest_open 双 stub（含 integration tag）全部同步改，`--test` + `--integration` 均无 build 红。✅

### File List

**新建**：
- `server/internal/repo/mysql/user_cosmetic_item_repo_test.go`（ListByUserForInventory sqlmock 单测 3 case）

**修改**：
- `server/internal/repo/mysql/user_cosmetic_item_repo.go`（追加 UserCosmeticItemRepo interface + ListByUserForInventory impl + userCosmeticItemRepo struct + NewUserCosmeticItemRepo；**不**改 23.2 struct/TableName）
- `server/internal/repo/mysql/cosmetic_item_repo.go`（CosmeticItemRepo interface 新增 ListByIDsForInventory + impl + interface 注释扩三方法语义边界；**不**改既有 2 方法 impl）
- `server/internal/repo/mysql/cosmetic_item_repo_test.go`（补 ListByIDsForInventory 3 case：HappyPath 含 is_enabled=0 行不被过滤 / EmptyIDs 不发 SQL / NoMatch）
- `server/internal/service/cosmetic_service.go`（CosmeticService 加 ListInventory + InventoryGroup/InventoryInstance DTO + 态 C 占位常量 + cosmeticServiceImpl 加 userCosmeticItemRepo 字段 + NewCosmeticService 扩 2 参 + ListInventory impl 三态矩阵 + 两级 sort.Slice）
- `server/internal/service/cosmetic_service_test.go`（stubCatalogCosmeticItemRepo 加 ListByIDsForInventory + 新建 stubUserCosmeticItemRepo + buildCosmeticService 改 2 参 + 新 buildCosmeticInventoryService + 8 ListInventory case + lexLEService helper）
- `server/internal/service/cosmetic_service_integration_test.go`（buildCosmeticServiceIntegration 注入 userCosmeticItemRepo + 2 参 + cosmeticIDByCode/insertUserCosmeticItem/findGroupByCID helper + 3 inventory 集成 case）
- `server/internal/service/chest_open_service_test.go`（既有 stubCosmeticItemRepo 加 ListByIDsForInventory 防御性 panic —— satisfy 扩展后 interface 编译）
- `server/internal/service/chest_open_service_integration_test.go`（faultCosmeticItemRepoOnList 加 ListByIDsForInventory 防御性 panic —— integration tag 下 satisfy 扩展后 interface 编译）
- `server/internal/app/http/handler/cosmetics_handler.go`（CosmeticsHandler 加 GetInventory + inventoryResponseDTO + import middleware/apperror；**不**改既有 GetCatalog/catalogResponseDTO）
- `server/internal/app/http/handler/cosmetics_handler_test.go`（stubCosmeticService 加 listInventoryFn + ListInventory + buildCosmeticsInventoryHandlerRouter + 4 GetInventory case）
- `server/internal/app/bootstrap/router.go`（新建 userCosmeticItemRepo 实例 + NewCosmeticService 扩 2 参 + 注册 `/cosmetics/inventory` 路由）
- `_bmad-output/implementation-artifacts/23-4-get-cosmetics-inventory-接口.md`（本 story 文件流转）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态流转 ready-for-dev → in-progress → review）

### Change Log

- 2026-05-16：Story 23.4 实装完成 —— 首次落地 GET /api/v1/cosmetics/inventory 接口（UserCosmeticItemRepo interface + ListByUserForInventory + CosmeticItemRepo.ListByIDsForInventory + CosmeticService.ListInventory config 三态矩阵 A/B/C + 两级确定性全序排序 + CosmeticsHandler.GetInventory + router 挂载 + 8 service 单测 + 4 handler 单测 + 3 repo 单测 + 3 dockertest 集成测试）。严格对齐 V1 §8.2 冻结契约；识别并按 §8.2 三态矩阵纠偏 epics.md 行 3292 "skip + warning"（态 C 不 skip + log error）。`bash scripts/build.sh --test` + `go vet -tags devtools ./internal/service/` + inventory 集成测试隔离单跑（5/5 PASS 含 catalog 无回归）均通过。Status: ready-for-dev → in-progress → review。
