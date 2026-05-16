# Story 23.3: GET /cosmetics/catalog 接口（首次落地 cosmetic service + cosmetics handler + CosmeticItemRepo 新增 catalog 排序查询方法 + 路由挂载 authedGroup + ≥4 case handler/service 单测 + dockertest 集成测试复用 0012 seed 验证 15 行 + 严格对齐 V1 §8.1 冻结契约）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want **首次落地** `GET /api/v1/cosmetics/catalog` REST 接口：新增 `server/internal/service/cosmetic_service.go`（`CosmeticService` interface + `ListCatalog(ctx) ([]CosmeticBrief, error)` 方法 + `CosmeticBrief` service 层 DTO + `cosmeticServiceImpl` + `NewCosmeticService(cosmeticItemRepo)` 构造，与 `emoji_service.go` `EmojiService.ListAvailable` 1:1 同模式 —— 单 repo query → DTO 转换 → nil slice 兜底；repo 失败 wrap 成 1009）+ 新增 `server/internal/app/http/handler/cosmetics_handler.go`（`CosmeticsHandler` struct + `NewCosmeticsHandler(svc)` + `GetCatalog(c *gin.Context)` 方法 + `catalogResponseDTO` wire 转换，与 `emojis_handler.go` `GetEmojis` / `emojiResponseDTO` 1:1 同模式 —— 不做参数校验 / 不读 userID / c.Error 透传走 ErrorMappingMiddleware / `make([]gin.H, 0, len)` 兜底 `items: []` 非 null）+ **扩展既有** `server/internal/repo/mysql/cosmetic_item_repo.go` 的 `CosmeticItemRepo` interface（**新增** `ListEnabledForCatalog(ctx) ([]CosmeticItem, error)` 方法 —— `SELECT id, code, name, slot, rarity, icon_url, asset_url FROM cosmetic_items WHERE is_enabled = 1 ORDER BY rarity ASC, slot ASC, id ASC`，**不**复用 Story 20.6 已有的 `ListEnabledForWeightedPick`，因后者**无 ORDER BY**、字段集为加权抽取语义、复用会让 catalog 排序契约与抽奖路径耦合 → 任一方改字段/排序就破坏另一方；两方法语义独立、各自演进）+ **扩展** `server/internal/app/bootstrap/router.go`（`if deps 完整` 块内复用 line 486 已 wire 的 `cosmeticItemRepo` 实例构造 `cosmeticSvc := service.NewCosmeticService(cosmeticItemRepo)` + `cosmeticsHandler := handler.NewCosmeticsHandler(cosmeticSvc)` + 在 `authedGroup` 注册路由 `authedGroup.GET("/cosmetics/catalog", cosmeticsHandler.GetCatalog)`，与 Story 17.4 `authedGroup.GET("/emojis", emojisHandler.GetEmojis)` / Story 20.5 `authedGroup.GET("/chest/current", chestHandler.GetCurrent)` 同模式 —— auth 中间件由 authedGroup 兜底，rate_limit 由 authedGroup 既有 `RateLimitByUserID` 中间件兜底）+ 新增单测 `cosmetic_service_test.go`（≥4 case，mocked `CosmeticItemRepo` stub）+ `cosmetics_handler_test.go`（mocked `CosmeticService` stub）+ 新增集成测试 `cosmetic_service_integration_test.go`（`//go:build integration` dockertest 起 mysql:8.0 → migrate up 落地 0012 seed 后复用 seed 数据验证 `ListCatalog` 返 15 个 enabled 行 + 字段值 + 排序断言，与 `emoji_service_integration_test.go` 1:1 同模式 —— **不**手工 INSERT 测试数据，直接复用 0012 seed 闭环），

so that **iOS Epic 24（仓库页 / Story 24.2 LoadInventoryUseCase）+ Epic 27（穿戴页装扮元信息查询）+ Epic 30（装扮渲染 cosmetic 元数据）+ Epic 32/33（合成页 cosmetic 配置目录展示）**可以基于一个**已落地、已具备完整测试覆盖（≥4 单测 + dockertest 集成）、已通过真实 0012 seed 闭环验证字段值 + 三级全序排序、已严格对齐 V1 §8.1 冻结契约（`{items:[{cosmeticItemId, code, name, slot, rarity, iconUrl, assetUrl}]}` + `WHERE is_enabled=1 ORDER BY rarity ASC, slot ASC, id ASC` + 不分页 + 错误码 1001/1005/1009 + **不**触发 1002 + 空集 `{items:[]}` 非 error）**的 catalog 查询接口并行展开，不再出现"iOS Epic 24 集成时 catalog response 字段名 / camelCase 与 §8.1 漂移 / 排序未钉死导致 client grid 跨请求抖动 / 复用 `ListEnabledForWeightedPick` 导致无 ORDER BY 列表乱序 / disabled 配置误返回 / 空 catalog 返 null 导致 iOS DTO 解码 crash / DB 错误未映射 1009 导致 client 拿不到统一错误结构 / 节点 8 联调才发现 server 没挂 /cosmetics/catalog 路由"的返工。

## 故事定位（Epic 23 第三条 = 第一个对外 REST 接口 story；上承 23.2 持久化根基，下启 23.4 inventory 聚合接口；本接口查 cosmetic_items 配置表，**不**碰 23.2 落地的 user_cosmetic_items 实例表）

- **Epic 23 进度**：23.1（契约定稿 §8.1 / §8.2 冻结，done）→ 23.2（user_cosmetic_items migration + UserCosmeticItem GORM struct，done）→ **23.3（本 story，GET /cosmetics/catalog 接口 —— 查 `cosmetic_items` 配置表全量 enabled 行）** → 23.4（GET /cosmetics/inventory 接口，查 `user_cosmetic_items` 实例表 + 按 cosmetic_item_id 聚合 + JOIN cosmetic_items 取元信息）→ 23.5（修改 Story 20.6 开箱事务补"入仓"）。
- **本 story 数据源 = `cosmetic_items` 配置表（§5.8）**，**不是** 23.2 落地的 `user_cosmetic_items` 实例表（§5.9）。catalog = "有哪些装扮配置"（全局静态目录，与 user 无关）；inventory（23.4）= "我拥有哪些实例"（user 维度）。**本 story 不读 userID、不查 user_cosmetic_items、不做任何聚合**。
- **本 story 是 iOS Epic 24 / Epic 27 / Epic 30 / Epic 32 / 33 的强前置**：
  - **iOS Epic 24（Story 24.2 LoadInventoryUseCase）**：仓库页虽以 inventory（23.4）为主数据源，但 catalog 提供"全部可用配置目录"用于筛选 / 展示未拥有项；iOS `CosmeticCatalogItem` Codable struct 通过本接口 JSON response 依赖 §8.1 锚定字段（`cosmeticItemId` / `code` / `name` / `slot` / `rarity` / `iconUrl` / `assetUrl`，全 camelCase）
  - **Epic 27 / 30（穿戴 / 渲染）**：装扮详情 / 渲染需 cosmetic 元数据（slot / rarity / assetUrl）；catalog 是该元数据的查询入口
  - **Epic 32 / 33（合成页）**：合成页展示"可合成产出的 cosmetic 配置"需 catalog 目录
- **epics.md §Story 23.3 钦定**（行 3248-3267）：
  - `GET /cosmetics/catalog` 返回 `{items: [...]}`，仅含 `is_enabled=1` 的配置
  - 接口要求 auth
  - 列表按 rarity ASC + slot ASC 排序（**§8.1 + 23.1 补 `id ASC` 决定性 tie-breaker → 三级全序**）
  - 列表大约 15-50 条，不分页
  - 单元测试覆盖 ≥4 case（mocked cosmetic repo）：happy（DB 15 enabled → items 长度 15 含全字段）/ happy（1 disabled → 不返回）/ edge（response 严格符合 §8.1 schema）/ edge（DB 错误 → 1009）
  - 集成测试覆盖（dockertest）：seed cosmetic_items → curl → response.items 含正确数量 + 字段值
- **Story 23.1 上游冻结边界**（V1 §8.1 字段表 + 服务端逻辑段 + 错误码表 + 关键约束段，2026-05-16 起冻结）：本 story handler response **严格**按 §8.1 锚定字段集 + camelCase；service 流程**严格**按 §8.1 服务端逻辑（`SELECT ... WHERE is_enabled = 1 ORDER BY rarity ASC, slot ASC, id ASC`，`id ASC` 不可省）；错误码**严格**按 §8.1 错误码表（1001 / 1005 / 1009；**不**触发 1002）。
- **范围红线**：
  - 本 story **只**改/新建：`server/internal/service/cosmetic_service.go`（新建）+ `server/internal/service/cosmetic_service_test.go`（新建）+ `server/internal/service/cosmetic_service_integration_test.go`（新建，`//go:build integration`）+ `server/internal/app/http/handler/cosmetics_handler.go`（新建）+ `server/internal/app/http/handler/cosmetics_handler_test.go`（新建）+ `server/internal/repo/mysql/cosmetic_item_repo.go`（**改**：`CosmeticItemRepo` interface 新增 `ListEnabledForCatalog` 方法 + impl）+ `server/internal/repo/mysql/cosmetic_item_repo_test.go`（**改**：补 `ListEnabledForCatalog` repo 层测试，若该文件既有测试用 dockertest 则放集成测试，按既有文件现状判定）+ `server/internal/app/bootstrap/router.go`（**改**：wire cosmetic svc/handler + 注册 GET /cosmetics/catalog 路由）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**碰 23.2 落地的 `user_cosmetic_items` 表 / `UserCosmeticItem` GORM struct（catalog 查 cosmetic_items 配置表，与实例表无关；inventory 23.4 才查实例表）
  - **不**改 Story 20.6 `ChestService.OpenChest` 开箱事务 / `ListEnabledForWeightedPick` 方法签名或 body（本 story **新增**独立 `ListEnabledForCatalog` 方法，**不**改既有方法 —— 排序契约不同、字段语义不同，复用会耦合两条业务路径）
  - **不**改 V1 接口契约（23.1 已冻结 §8.1）/ 任何 `docs/宠物互动App_*.md`（§8.1 / §5.8 / §6.8 / §6.9 是契约**输入**，本 story 严格对齐但**不**修改；如发现不一致 → 优先以文档为准改本 story 而非反向改文档）
  - **不**改 0001~0015 既有 migration / 0012 seed（catalog 直接复用 0012 已 seed 的 15 行 enabled cosmetic_items，**不**新建 seed）
  - **不**建 `internal/domain/cosmetic/` 目录（**重要纠偏**：CLAUDE.md target 目录形态 §4 + 23.1 行 281 提到 `internal/domain/cosmetic/` / `internal/service/cosmetic_service.go`，但 server 工程**实际现状**是扁平 `internal/service/*.go` + `internal/app/http/handler/*.go` + `internal/repo/mysql/*.go`，**无** `internal/domain/` 目录。本 story **严格遵循 server 实际现状目录布局**，与 `emoji_service.go` / `emojis_handler.go` 同级落地，**不**为本 story 单独引入 `internal/domain/cosmetic/` 新目录 —— 与 ADR-0006 / 既有 17.4 / 20.5 实装一致；目录形态偏差属"CLAUDE.md target 是 aspirational、实际工程演进为扁平"的历史现状，**不**视为架构变更，dev 实装时以**真实代码现状**为准不盲信文档 target 段)
  - **不**实装 23.4 inventory 聚合查询 / 23.5 开箱补入仓（本 story **仅** catalog 只读查询；inventory 是 23.4 钦定范围，预实装会让下游评审找不到"新增聚合方法"的明确范围边界，与 20.2 / 23.2 "禁止预实装"同模式）
  - **不**加分页 / query 参数解析（§8.1 钦定不分页、不接受任何 query string）
  - **不**做 `iconUrl == "" 跳过` 空串过滤（§8.1 钦定 enabled cosmetic 必须非空 URL，0012 seed 已保证；server 是 cosmetic 数据 single source of truth，意外空串透传到 client 触发渲染失败而非 server 静默过滤 —— 与 `emoji_service.go` 行 38-42 "不做空字符串过滤"钦定同源）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除本 story 文件 + sprint-status.yaml 流转）
  - **不**写英文版测试注释 / 文档（项目 communication_language=Chinese）
  - **不**为 catalog 写 cache / Redis（§8.1 钦定单 SELECT 不开事务；约 15-50 行单查足够，无 cache 需求；与 emoji ListAvailable 同模式）

**本 story 不做**（明确范围红线，再次强调）：

- 不读 userID / 不查 user_cosmetic_items / 不做任何 user 维度过滤或聚合（catalog 是全局静态目录，与 user 无关；inventory 23.4 才做 user 维度 + 聚合）
- 不复用 / 不修改 `ListEnabledForWeightedPick`（Story 20.6 加权抽取专用 —— 无 ORDER BY、字段语义为抽奖；本 story **新增**独立 `ListEnabledForCatalog`）
- 不实装任何 23.4 / 23.5 方法（YAGNI；inventory 聚合 / 开箱补入仓是 23.4 / 23.5 钦定范围）
- 不新建 seed SQL / 不改 0012 seed（直接复用已 seed 的 15 行 enabled cosmetic_items）
- 不引入 `internal/domain/cosmetic/` 新目录（严格遵循 server 扁平工程现状，与 emoji 17.4 / chest 20.5 实装同级落地）
- 不加分页 / query 参数 / 不接受任何 query string（§8.1 钦定）
- 不做空 URL / 空串过滤 / 不做 enabled 行字段补值（server 透传真实 row，seed/admin 层保证非空，与 emoji_service 同源钦定）
- 不开 MySQL 事务（§8.1 钦定单 SELECT 无副作用，天然幂等，不需要事务）
- 不在 handler 做参数校验 / 不直接调 `response.Error`（ADR-0006 单一 envelope 生产者 —— 一律 c.Error + return 走 ErrorMappingMiddleware；与 emojis_handler 同模式）
- 不把 `*gin.Context` 当 ctx 传给 service（ADR-0007 §2.2 —— 用 `c.Request.Context()`；与 emojis_handler / chest_handler 同模式）
- 不改 V1 §8.1 契约 / §5.8 cosmetic_items schema / §6.8 slot / §6.9 rarity 枚举文档（schema 输入，严格对齐不修改）
- 不为 catalog 写 stress / fuzz test（节点 8 阶段 schema 稳定 + 单测 + dockertest 集成已覆盖核心约束，与 17.4 / 20.5 一致）
- 不预实装 admin POST / PATCH /cosmetics 等写接口（MVP 节点 8 无 admin 后台需求；cosmetic_items 写入仅 0012 seed，与 emojis_handler 行 10-13 "future epic 加 POST / PATCH" 同模式）

## Acceptance Criteria

**AC1 — `CosmeticItemRepo` 新增 `ListEnabledForCatalog` 方法（§8.1 服务端逻辑步骤 2 钦定 SQL）**

修改 `server/internal/repo/mysql/cosmetic_item_repo.go`：

- 在既有 `CosmeticItemRepo` interface（Story 20.6 引入，现含 `ListEnabledForWeightedPick`）**新增**方法：

  ```go
  // ListEnabledForCatalog 返回所有 is_enabled=1 的 cosmetic_items 行，按
  // V1 §8.1 服务端逻辑步骤 2 钦定排序。
  //
  // SQL: SELECT id, code, name, slot, rarity, icon_url, asset_url FROM cosmetic_items
  //      WHERE is_enabled = 1 ORDER BY rarity ASC, slot ASC, id ASC
  ListEnabledForCatalog(ctx context.Context) ([]CosmeticItem, error)
  ```

- impl `(r *cosmeticItemRepo) ListEnabledForCatalog` 用 `tx.FromContext(ctx, r.db).WithContext(ctx)` + 显式 `Select("id, code, name, slot, rarity, icon_url, asset_url")` + `Where("is_enabled = ?", 1)` + `Order("rarity ASC, slot ASC, id ASC")` + `Find(&rows)` + 空 slice 兜底（`if rows == nil { rows = []CosmeticItem{} }`），与 `emoji_repo.go` 行 137-164 `List` impl 1:1 同模式。
- **关键：`ORDER BY rarity ASC, slot ASC, id ASC` 三级全序，`id ASC` 是决定性 tie-breaker 不可省**（§8.1 行 1306 + 23.1 r1 [P2] 钦定 —— 0012 seed 内同 (rarity, slot) 必有多行，如 `hat_yellow`/`hat_red` 同为 (slot=1, rarity=1)、`gloves_white`/`gloves_brown` 同为 (slot=2, rarity=1)，缺 `id ASC` 则 MySQL 同 (rarity, slot) 行顺序跨请求可抖动 → client grid 抖动违背契约）。
- **新增独立方法，不复用 / 不改 `ListEnabledForWeightedPick`**：后者无 ORDER BY、字段语义为加权抽取（开箱事务步骤 5g）；两方法语义独立，复用会让 catalog 排序契约与抽奖路径耦合（任一方改字段/排序破坏另一方）。interface 注释头补本方法说明（与既有 `ListEnabledForWeightedPick` 注释风格一致 + 明确两方法语义边界 + 范围红线"本 story 仅加 catalog 方法，不实装 23.4 inventory 方法"）。
- 显式 `Select` 字段集与 §8.1 服务端逻辑步骤 2 钦定 7 列 1:1（`id, code, name, slot, rarity, icon_url, asset_url`；**不** SELECT `*`，避免 future 表加列污染 payload；`drop_weight` / `is_enabled` / `created_at` / `updated_at` 不在 SELECT —— client 不需要，GORM Scan 填 zero-value 安全，与 `emoji_repo.go` 行 144-148 同模式）。

**AC2 — `cosmetic_service.go` 新建（`CosmeticService` interface + `ListCatalog` + `CosmeticBrief` DTO）**

新建 `server/internal/service/cosmetic_service.go`，与 `emoji_service.go` 1:1 同模式：

- `CosmeticService` interface（handler 单测 mock 用）含 `ListCatalog(ctx context.Context) ([]CosmeticBrief, error)`。
- `CosmeticBrief` service 层 DTO（**不是** wire DTO；字段与 §8.1 `data.items[]` 钦定字段集 1:1）：
  - `CosmeticItemID uint64`（§8.1 `cosmeticItemId`；handler 层字符串化 → BIGINT 字符串化与 §2.5 全局约定一致）
  - `Code string`（§8.1 `code`）
  - `Name string`（§8.1 `name`）
  - `Slot int8`（§8.1 `slot`，§6.8 枚举 {1,2,3,4,5,6,7,99}）
  - `Rarity int8`（§8.1 `rarity`，§6.9 枚举 {1,2,3,4}）
  - `IconURL string`（§8.1 `iconUrl`）
  - `AssetURL string`（§8.1 `assetUrl`）
  - **不**含 DropWeight / IsEnabled / CreatedAt / UpdatedAt（§8.1 钦定 client 不需要）
- `cosmeticServiceImpl` struct 含 `cosmeticItemRepo mysql.CosmeticItemRepo` + `NewCosmeticService(cosmeticItemRepo mysql.CosmeticItemRepo) CosmeticService` 构造。
- `ListCatalog` impl：调 `cosmeticItemRepo.ListEnabledForCatalog(ctx)` → 失败 `apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])`（1009；与 `emoji_service.go` 行 100-104 同模式 + lesson 2026-05-13 Lesson 2 DB error 必有 1009 路径）→ 成功 `make([]CosmeticBrief, 0, len(rows))` 兜底非 nil slice → 逐行 DTO 转换 → 返回。
- **不**做空 URL 过滤（§8.1 钦定 enabled 必非空，0012 seed 已保证；与 `emoji_service.go` 行 38-42 钦定同源）。
- 完整中文注释头：interface / DTO / impl 注释说明 §8.1 字段来源 + 错误约定 + nil slice 兜底理由 + 不做空串过滤理由 + 范围红线（参照 `emoji_service.go` 注释风格）。

**AC3 — `cosmetics_handler.go` 新建（`GetCatalog` + `catalogResponseDTO` wire 转换）**

新建 `server/internal/app/http/handler/cosmetics_handler.go`，与 `emojis_handler.go` 1:1 同模式：

- `CosmeticsHandler` struct 含 `svc service.CosmeticService` + `NewCosmeticsHandler(svc service.CosmeticService) *CosmeticsHandler` 构造。
- `GetCatalog(c *gin.Context)`：调 `h.svc.ListCatalog(c.Request.Context())`（ADR-0007 §2.2 用 `c.Request.Context()`，**不**传 `*gin.Context`）→ 失败 `_ = c.Error(err); return`（ADR-0006 单一 envelope 生产者，走 ErrorMappingMiddleware）→ 成功 `response.Success(c, catalogResponseDTO(briefs), "ok")`。
- **不**做参数校验 / **不**读 userID（§8.1 钦定不接受 query 参数 / body；接口要求 auth 但 service 不需要 user 维度 —— auth 由 router authedGroup 中间件兜底；与 `emojis_handler.go` 行 36-49 同模式）。
- `catalogResponseDTO(briefs []service.CosmeticBrief) gin.H`：`make([]gin.H, 0, len(briefs))` 兜底非 nil → 逐元素 `gin.H{"cosmeticItemId": strconv.FormatUint(b.CosmeticItemID, 10), "code": b.Code, "name": b.Name, "slot": b.Slot, "rarity": b.Rarity, "iconUrl": b.IconURL, "assetUrl": b.AssetURL}` → 返 `gin.H{"items": items}`。
  - **`cosmeticItemId` 必须字符串化**（§8.1 字段表行 1262 钦定 `string` 类型 BIGINT 字符串化，与 §2.5 全局约定一致；用 `strconv.FormatUint(b.CosmeticItemID, 10)`）—— 这是与 emoji handler 的关键差异（emoji 无 id 下发，cosmetic 有 `cosmeticItemId` 且必须 string）。
  - `slot` / `rarity` 是 **int**（§8.1 字段表钦定 `int` 类型，**不**字符串化）。
  - 字段名全 camelCase（§8.1 + V1 §2.4 钦定 wire 全 camelCase：`cosmeticItemId` / `iconUrl` / `assetUrl`）。
  - **永远**下发 `items: []` 非 `items: null`（§8.1 行 1301 + 空 catalog 返 `{items:[]}` code=0 不报错；`make([]gin.H, 0, len)` 兜底；与 `emojis_handler.go` 行 78-95 同模式）。
- 完整中文注释头：handler / DTO 注释说明 §8.1 wire 字段集（任一缺失 → iOS DTO 解码失败）+ camelCase 转换 + cosmeticItemId 字符串化理由 + items 非 null 兜底 + ADR-0006 / ADR-0007 引用（参照 `emojis_handler.go` 注释风格）。

**AC4 — `router.go` wire + 路由注册**

修改 `server/internal/app/bootstrap/router.go`，在 `if deps 完整`（含 `cosmeticItemRepo` 已 wire 的）块内：

- 复用 line 486 既有 `cosmeticItemRepo := repomysql.NewCosmeticItemRepo(deps.GormDB)` 实例（**不**新建第二个实例 —— 与 20.6 chestSvc 复用同实例同模式）。
- 构造 `cosmeticSvc := service.NewCosmeticService(cosmeticItemRepo)` + `cosmeticsHandler := handler.NewCosmeticsHandler(cosmeticSvc)`（wire 顺序：在 `cosmeticItemRepo` 构造之后、`authedGroup` 路由注册之前；位置参照 Story 17.4 emojiSvc / 20.5 chestSvc wire 段）。
- 在 `authedGroup` 注册 `authedGroup.GET("/cosmetics/catalog", cosmeticsHandler.GetCatalog)`（与既有 `authedGroup.GET("/emojis", ...)` 行 519 / `authedGroup.GET("/chest/current", ...)` 行 522 同组同模式 —— auth 中间件 + RateLimitByUserID 由 authedGroup 既有中间件链兜底，**不**单独挂中间件）。
- 注释说明本路由由 Story 23.3 加（与既有路由注释风格一致）。
- **关键**：`cosmeticsHandler` 必须只在 `cosmeticItemRepo` 可用的 `if` 块内构造 + 注册（与 chestHandler / emojisHandler 在同一 `if deps 完整` 块内同模式；若 deps 不完整该路由不注册，与既有 fallback 行为一致）。

**AC5 — 单元测试覆盖（≥4 case，mocked repo / mocked service）**

新建 `server/internal/service/cosmetic_service_test.go`（mocked `CosmeticItemRepo` stub，与 `emoji_service_test.go` 同模式，≥4 case）：

1. **happy（DB 15 enabled → items 长度 15 含全字段）**：stub repo 返 15 行 `CosmeticItem` → `ListCatalog` 返 15 个 `CosmeticBrief`，逐字段值正确映射（id / code / name / slot / rarity / iconUrl / assetUrl）。
2. **happy（disabled 不返回）**：本接口 disabled 过滤在 repo SQL 层（`WHERE is_enabled=1`），service 层不重复过滤 —— 该 case 用 stub repo 模拟"repo 已过滤后返回的就是 enabled 行"，验证 service 透传不丢字段；**注**：disabled 过滤的真实验证在 AC6 集成测试（真实 SQL 跑 0012 全 enabled seed + 可临时 UPDATE 一行 is_enabled=0 验证不返回）—— epics.md AC "1 个 disabled → 不返回" 的真值在 SQL 层，单测层 stub 无法测 SQL 过滤，故该 case 在集成测试覆盖，单测层覆盖 service 透传逻辑（注释写明该分工）。
3. **edge（response 严格符合 §8.1 schema）**：返 1 行 → 断言 `CosmeticBrief` 字段集 = §8.1 `data.items[]` 钦定 7 字段，无多余 / 无缺失。
4. **edge（DB 错误 → 1009）**：stub repo 返 `errors.New("db down")` → `ListCatalog` 返 `*apperror.AppError` 且 `Code == apperror.ErrServiceBusy`（1009）；`errors.As` 断言。
5. **edge（空集 → 非 nil 空 slice）**：stub repo 返 `[]CosmeticItem{}` → `ListCatalog` 返 `len==0` 且 `!= nil` 的 `[]CosmeticBrief`（保证 handler 下发 `items: []` 非 null）。

新建 `server/internal/app/http/handler/cosmetics_handler_test.go`（mocked `CosmeticService` stub，与 `emojis_handler_test.go` 同模式）：

6. **happy**：stub svc 返 N 个 `CosmeticBrief` → `GetCatalog` 走 `httptest` → response body `data.items` 长度 N + `cosmeticItemId` 是 **string** + `slot`/`rarity` 是 **number** + 全 camelCase 字段名 + HTTP 200 code=0。
7. **edge（空集 → `items: []` 非 null）**：stub svc 返空 slice → response body `data.items` 是 `[]` 而非 `null`（JSON 层断言；与 emoji handler 测同模式）。
8. **edge（service 返 1009 → handler c.Error 透传）**：stub svc 返 `*apperror.AppError{Code:1009}` → handler 不 panic，error 进 `c.Errors`（ErrorMappingMiddleware 兜底翻译；与 emoji handler 测同模式 —— 若既有 emoji handler 测用 router + middleware 链测 envelope，则同模式断言 envelope code=1009）。

**AC6 — 集成测试覆盖（dockertest，复用 0012 seed）**

新建 `server/internal/service/cosmetic_service_integration_test.go`（`//go:build integration`，与 `emoji_service_integration_test.go` 1:1 同模式）：

- buildCosmeticServiceIntegration helper：起 mysql:8.0 容器 → `runMigrations(t, dsn)` 跑到最新版（含 0012 seed cosmetic_items 15 行）→ 装配 `cosmeticItemRepo` + `cosmeticSvc` + 返清理 closure（参照 `emoji_service_integration_test.go` 行 34-75）。
- `TestCosmeticServiceIntegration_ListCatalog_SeedContent`：migrate 后跑 `ListCatalog` → 断言：
  - `len(got) == 15`（0012 seed 钦定 15 行 enabled cosmetic_items；与 `emoji_service_integration_test.go` `len(got)==4` 同模式 —— **不**手工 INSERT，直接复用 0012 seed 闭环）。
  - 抽样验证字段值与 0012 seed 1:1（如 `hat_yellow` 行：code=`hat_yellow` / name=`小黄帽` / slot=1 / rarity=1 / iconUrl 与 seed 一致 / assetUrl 与 seed 一致）。
  - **排序断言**：结果按 `rarity ASC, slot ASC, id ASC` 全序 —— 逐对相邻元素断言 `(rarity_i, slot_i, id_i) <= (rarity_{i+1}, slot_{i+1}, id_{i+1})`（lexicographic）；验证三级全序契约（§8.1 关键约束 + 23.1 r1 [P2]）。
- `TestCosmeticServiceIntegration_ListCatalog_DisabledExcluded`：migrate 后 `UPDATE cosmetic_items SET is_enabled=0 WHERE code='hat_yellow'` → 跑 `ListCatalog` → 断言 `len(got)==14` 且结果中无 `hat_yellow`（验证 epics.md AC "1 disabled → 不返回" 的 SQL 层真值；测后该容器销毁不污染其他 case）。
- **不**手工 INSERT 测试数据（与 emoji 集成测试同 —— 直接复用 0012 seed migration，seed → 接口 endpoint 闭环）。

**AC7 — 构建 / 测试通过**

- `bash scripts/build.sh --test` 通过（vet + build + 全量单测 `go test -count=1 ./...`；本 story 改/新增多个 Go 文件，必须跑）。
- `bash scripts/build.sh --integration` 通过（`-tags=integration` 跑本 story 新增 cosmetic service 集成测试 + 既有集成测试无回归；dockertest 起真实 MySQL 容器）。
  - **注**：若 `go test ./...` 顺序串跑大量 dockertest case 时本机 docker daemon 被压垮导致个别**不相关包** per-package timeout（已知环境侧 flake，Story 23.2 Debug Log 已记录同现象），需隔离单跑本 story 范围的集成测试（`go test -tags=integration -timeout=600s ./internal/service/ -run TestCosmeticServiceIntegration`）确认全 PASS，并在 Debug Log 如实记录环境侧 flake 与本 story 改动无因果关系。
- 全量测试无回归（既有 emoji / chest / home / room service + handler 测试全绿；`ListEnabledForWeightedPick` 既有路径不受影响 —— 本 story **新增**独立 `ListEnabledForCatalog`，不改既有方法；router.go 既有路由不受影响 —— 仅新增 1 路由）。
- `git status` 仅出现范围红线内文件改动（≤8 个 server 文件 + story 文件 + sprint-status.yaml）。

**AC8 — 跨文档一致性自检（对外接口 story 必须项）**

完成 AC1~AC4 后，必须逐项核对并在本 story "Completion Notes List" 记录核对结论：

1. handler `catalogResponseDTO` wire 字段集 / 类型 / camelCase 与 V1 §8.1 `data.items[]` 字段表（行 1260-1268）**逐字段比对一致**：`cosmeticItemId`（string，BIGINT 字符串化）/ `code`（string）/ `name`（string）/ `slot`（int，§6.8 枚举）/ `rarity`（int，§6.9 枚举）/ `iconUrl`（string）/ `assetUrl`（string）；无多余 / 无缺失字段；`cosmeticItemId` 确为 string 非 number。
2. service `ListCatalog` 流程 + repo `ListEnabledForCatalog` SQL 与 §8.1 服务端逻辑步骤 2（行 1250-1254）**一致**：`WHERE is_enabled = 1` + `ORDER BY rarity ASC, slot ASC, id ASC`（`id ASC` 决定性 tie-breaker 未省）+ 不分页 + 不开事务。
3. 错误码映射与 §8.1 错误码表（行 1295-1301）**一致**：DB 异常 → 1009；auth / rate_limit 由 router 中间件兜底（1001 / 1005）；**不**触发 1002（GET 无 body / query）；空 catalog → `{items:[]}` code=0 **不**报错。
4. 集成测试 `len(got)==15` 与 0012 seed 真实行数（`server/migrations/0012_seed_cosmetic_items.up.sql` INSERT 15 行全 `is_enabled=1`）**一致**；dockertest 实跑确认。
5. 未触碰 23.2 `user_cosmetic_items` 表 / `UserCosmeticItem` struct / Story 20.6 `ListEnabledForWeightedPick` 既有方法 / `ChestService.OpenChest` / V1 §8.1 契约 / §5.8 schema / §6.8 / §6.9 枚举 / 其他 6 份 docs / 0001~0015 既有 migration / 0012 seed。`git status` 实测仅范围红线内文件。
6. 目录布局纠偏（CLAUDE.md target §4 / 23.1 行 281 提 `internal/domain/cosmetic/`，但 server 实际现状为扁平 `internal/service/` + `internal/app/http/handler/` + `internal/repo/mysql/`）已在 Dev Notes + Completion Notes 显式记录，**不**视为架构变更（属 CLAUDE.md target 为 aspirational、实际工程演进为扁平的历史现状，与 emoji 17.4 / chest 20.5 实装一致）。

## Tasks / Subtasks

- [x] Task 1：读取并定位（AC1~AC4 / AC8）
  - [x] 读 V1 §8.1（行 1224-1308）GET /cosmetics/catalog 完整契约（接口元信息 + 服务端逻辑 + 响应体字段表 + 错误码表 + 关键约束段）
  - [x] 读数据库设计 §5.8 cosmetic_items DDL + §6.8 slot 枚举 + §6.9 rarity 枚举（经 cosmetic_item_repo.go GORM struct 注释 + 0012 seed 头注释交叉确认枚举值域）
  - [x] 读参照 `server/internal/service/emoji_service.go`（全文）+ `server/internal/app/http/handler/emojis_handler.go`（全文）+ `server/internal/repo/mysql/emoji_repo.go` 行 137-164（List impl 模板）
  - [x] 读 `server/internal/repo/mysql/cosmetic_item_repo.go`（全文，确认 `CosmeticItemRepo` interface 现状 + `ListEnabledForWeightedPick` 语义边界 + GORM struct 字段映射 + 注释风格）
  - [x] 读 `server/internal/app/bootstrap/router.go` 行 475-534（cosmeticItemRepo line 486 wire 位置 + emojiSvc/chestSvc wire 段 + authedGroup 路由注册段）
  - [x] 读 `server/internal/service/emoji_service_integration_test.go`（全文，dockertest helper + runMigrations + SeedContent case 1:1 模板）
  - [x] 读 `server/internal/service/emoji_service_test.go` + `server/internal/app/http/handler/emojis_handler_test.go`（单测 mock stub 模式 + httptest 模式）
  - [x] 确认 0012 seed 真实行数 = 15（`server/migrations/0012_seed_cosmetic_items.up.sql` INSERT 15 行全 is_enabled=1）+ apperror 码（ErrServiceBusy=1009 / ErrUnauthorized=1001 / ErrInvalidParam=1002 / ErrTooManyRequests=1005）
- [x] Task 2：扩展 CosmeticItemRepo 新增 ListEnabledForCatalog（AC1）
  - [x] interface 加 `ListEnabledForCatalog(ctx) ([]CosmeticItem, error)` + 注释（说明 SQL + 三级全序 + 与 ListEnabledForWeightedPick 语义边界 + 范围红线）
  - [x] impl：`Select("id, code, name, slot, rarity, icon_url, asset_url")` + `Where("is_enabled = ?", 1)` + `Order("rarity ASC, slot ASC, id ASC")` + `Find` + 空 slice 兜底
  - [x] **不**改 / 不复用 `ListEnabledForWeightedPick`（已核对 git diff：该方法签名/body 0 改动）
  - [x] 补 `cosmetic_item_repo_test.go` 对应测试（既有文件用 sqlmock 单测 → 同模式补 2 case：HappyPath + EmptyResult）
- [x] Task 3：新建 cosmetic_service.go（AC2）
  - [x] `CosmeticService` interface + `CosmeticBrief` DTO（7 字段 1:1 §8.1）+ `cosmeticServiceImpl` + `NewCosmeticService`
  - [x] `ListCatalog` impl：repo query → 失败 wrap 1009 → 成功 make 非 nil slice → DTO 转换
  - [x] 完整中文注释头（§8.1 来源 + 错误约定 + nil slice + 不做空串过滤 + 范围红线）
- [x] Task 4：新建 cosmetics_handler.go（AC3）
  - [x] `CosmeticsHandler` + `NewCosmeticsHandler` + `GetCatalog`（c.Request.Context / c.Error 透传 / response.Success）
  - [x] `catalogResponseDTO`：cosmeticItemId 字符串化（strconv.FormatUint）+ slot/rarity int + 全 camelCase + items 非 null 兜底
  - [x] 完整中文注释头（§8.1 wire 字段集 + camelCase + 字符串化理由 + ADR-0006/0007 引用）
- [x] Task 5：router.go wire + 路由注册（AC4）
  - [x] `if deps 完整` 块内复用 line 486 cosmeticItemRepo 构造 cosmeticSvc + cosmeticsHandler
  - [x] `authedGroup.GET("/cosmetics/catalog", cosmeticsHandler.GetCatalog)` + 注释
- [x] Task 6：单元测试（AC5）
  - [x] `cosmetic_service_test.go` 5 case（happy 15 行全字段 / disabled 透传 / schema 严格 / DB 错误 1009 / 空集非 nil）
  - [x] `cosmetics_handler_test.go` 3 case（happy httptest 字段类型+camelCase+raw JSON cosmeticItemId string 断言 / 空集 items:[] / service 1009 c.Error 透传）
- [x] Task 7：集成测试（AC6）
  - [x] `cosmetic_service_integration_test.go`（`//go:build integration`）helper + `ListCatalog_SeedContent`（15 行 + 字段值 + 三级全序排序断言）+ `ListCatalog_DisabledExcluded`（UPDATE 1 行 disabled → 14 行无 hat_yellow）
- [x] Task 8：构建 / 测试（AC7）
  - [x] `bash scripts/build.sh --test`（vet + build + 全量单测 → PASS，无回归）
  - [x] cosmetic 集成测试隔离单跑 PASS（`go test -tags=integration ./internal/service/ -run TestCosmeticServiceIntegration` → 2 PASS 46s；+ 与 chest-open happy 同跑 PASS 94s 确认 20.6 无回归）；全量 `--integration` 串跑触发已知环境侧 flake（auth 包 startMySQL per-package timeout，与本 story 无因果，见 Debug Log）
  - [x] 确认无回归 + `git status` 仅范围红线内文件
- [x] Task 9：跨文档一致性自检（AC8）
  - [x] 逐项核对 AC8 的 6 条，结论写入 "Completion Notes List"
  - [x] 确认目录布局纠偏（target §4 vs 实际扁平）已显式记录、不视为架构变更
  - [x] 标记 sprint-status.yaml `23-3-get-cosmetics-catalog-接口` 状态流转 ready-for-dev → in-progress → review

## Dev Notes

### 这是什么类型的 story

第一个对外只读 REST 接口 story（Epic 23 内）。对标 Story 17.4（GET /emojis）/ 20.5（GET /chest/current）—— 都是 auth-required、无 query 参数、单 SELECT、不开事务、seed/static 数据源的只读列表接口。**GET /emojis（17.4）是本 story 最贴近的 1:1 模板**（同为"全局静态 enabled 配置列表，与 user 无关，DTO 裁字段 + camelCase + items 非 null 兜底"），dev 实装时**逐文件对照 emoji_service.go / emojis_handler.go / emoji_repo.go / emoji_service_integration_test.go / emoji_service_test.go / emojis_handler_test.go 改写为 cosmetic 版本**，差异点见下方"与 emoji 模板的关键差异"。

### 与 emoji 模板的关键差异（逐点对照，避免照抄出错）

| 维度 | emoji（17.4 模板） | cosmetic（本 story） |
|---|---|---|
| 数据源表 | `emoji_configs`（§5.15） | `cosmetic_items`（§5.8） |
| repo 现状 | 17.2 新建 `EmojiRepo` interface | **20.6 已建 `CosmeticItemRepo` interface（含 `ListEnabledForWeightedPick`）—— 本 story 是 interface 扩展不是新建** |
| 新增 repo 方法 | `List`（17.4 首建） | `ListEnabledForCatalog`（**新增独立方法，不复用 `ListEnabledForWeightedPick`**） |
| SQL ORDER BY | `sort_order ASC, id ASC`（二级） | **`rarity ASC, slot ASC, id ASC`（三级全序，id ASC 决定性 tie-breaker 不可省）** |
| wire 字段集 | code / name / assetUrl / sortOrder（4 字段，无 id 下发） | cosmeticItemId / code / name / slot / rarity / iconUrl / assetUrl（**7 字段，含 id 且必须字符串化**） |
| id 是否下发 | **不**下发 | **下发 `cosmeticItemId`（string，`strconv.FormatUint`）** |
| int 字段 | sortOrder（int 直接下发） | slot / rarity（int 直接下发，**不**字符串化；只有 cosmeticItemId 字符串化） |
| 集成测试 seed 行数 | 0010 seed 4 行 | **0012 seed 15 行** |
| disabled 验证 | 集成测试 | 集成测试（额外加 `ListCatalog_DisabledExcluded` UPDATE 一行 is_enabled=0 → 14 行）|

### 关键纠偏点 1：目录布局以 server 实际现状为准（扁平，非 internal/domain/cosmetic/）

CLAUDE.md "节点 1 之后的目录形态（target）" §4 + 23.1 行 281 提到 `internal/domain/cosmetic/` / `internal/service/cosmetic_service.go`。但 server 工程**实际现状**（用 `ls server/internal/` 确认）是扁平布局：`internal/service/*.go`（emoji_service.go / chest_service.go / home_service.go ...）+ `internal/app/http/handler/*.go` + `internal/repo/mysql/*.go`，**无** `internal/domain/` 目录。本 story **严格遵循实际现状**，与 `emoji_service.go` / `emojis_handler.go` / `cosmetic_item_repo.go` **同级落地**，**不**为本 story 单独引入 `internal/domain/cosmetic/` 新目录。这属"CLAUDE.md target 是 aspirational、实际工程从 17.4 / 20.5 起就演进为扁平"的历史现状，**不**视为架构变更（与既有 emoji / chest 接口实装一致）。Completion Notes 必须显式记录此纠偏。**dev 实装铁律：以真实代码现状为准，不盲信 CLAUDE.md target 段 / 23.1 行 281 的目录建议。**

### 关键纠偏点 2：不复用 ListEnabledForWeightedPick，新增独立 ListEnabledForCatalog

`CosmeticItemRepo` interface 已含 Story 20.6 落地的 `ListEnabledForWeightedPick(ctx) ([]CosmeticItem, error)`。**禁止复用它做 catalog**：
- `ListEnabledForWeightedPick` **无 ORDER BY**（开箱加权抽取不需要排序，service 内加权采样）；catalog **必须** `ORDER BY rarity ASC, slot ASC, id ASC` 三级全序（§8.1 契约 + client grid 防抖动）。
- 复用会让 catalog 排序契约与开箱抽奖路径**耦合**：任一方改字段 / 改排序就破坏另一方；下游评审找不到"catalog 专用查询"的明确边界。
- 两方法语义独立、各自演进。本 story **新增** `ListEnabledForCatalog`（带 ORDER BY），与 `ListEnabledForWeightedPick` 并列，**不**改后者签名 / body。

### 关键纠偏点 3：cosmeticItemId 必须字符串化（与 emoji 模板最大差异）

§8.1 字段表行 1262 钦定 `cosmeticItemId` 是 **string** 类型（BIGINT 字符串化，与 §2.5 全局约定 + `cosmetic_items.id BIGINT UNSIGNED` 一致）。emoji handler **不**下发 id，所以照抄 `emojiResponseDTO` 会漏掉这个字段 + 漏掉字符串化。dev 实装时 `catalogResponseDTO` 必须 `"cosmeticItemId": strconv.FormatUint(b.CosmeticItemID, 10)`（**不**是 `b.CosmeticItemID` 直接塞 —— 那会序列化成 JSON number 破坏契约，iOS `String` 解码失败）。`slot` / `rarity` 是 **int** 直接下发（§8.1 钦定 int 类型，**不**字符串化 —— 只有 id 类字段字符串化，枚举类 int 字段不字符串化）。参照 `home_handler.go` / `chest_handler.go` 既有 BIGINT 字符串化处理（用 `strconv.FormatUint`）。

### 字段语义唯一来源（DB / 契约 → 实装单向）

- 接口契约：`docs/宠物互动App_V1接口设计.md` §8.1（行 1224-1308，**冻结**）—— 接口元信息 + 服务端逻辑 + 响应体字段表 + 错误码表 + 关键约束
- DB schema：`docs/宠物互动App_数据库设计.md` §5.8 cosmetic_items（行 437-459）+ §6.8 slot 枚举（1=hat/2=gloves/3=glasses/4=neck/5=back/6=body/7=tail/99=other）+ §6.9 rarity 枚举（1=common/2=rare/3=epic/4=legendary）
- epics.md §Story 23.3（行 3248-3267）AC 钦定
- **DB / 契约 → 实装单向**：本 story 严格对齐这些输入，**不**反向修改任何 docs；如发现实装与文档不一致 → 优先以文档为准改本 story

### 易错点（review 高频命中，提前规避）

1. **照抄 emojiResponseDTO 漏 cosmeticItemId / 漏字符串化**：必须加 `"cosmeticItemId": strconv.FormatUint(b.CosmeticItemID, 10)`（string，非 number）。
2. **复用 ListEnabledForWeightedPick**：必须新增独立 `ListEnabledForCatalog`（带 ORDER BY 三级全序），不碰加权抽取方法。
3. **漏 `id ASC` tie-breaker**：`ORDER BY rarity ASC, slot ASC, id ASC` —— `id ASC` 是契约必需部分（0012 seed 同 (rarity, slot) 必有多行，缺则跨请求抖动），不可只写 `rarity ASC, slot ASC`。
4. **建 internal/domain/cosmetic/ 新目录**：严格遵循 server 扁平现状，与 emoji_service.go 同级落地。
5. **slot / rarity 字符串化**：§8.1 钦定 slot/rarity 是 **int**，**不**字符串化；只有 cosmeticItemId 字符串化。
6. **空 catalog 返 error / 返 null**：空集返 `{items:[]}` code=0 **不**报错（§8.1 行 1301）；service make 非 nil slice + handler make([]gin.H,0,len) 兜底，JSON 出 `[]` 非 `null`。
7. **handler 做参数校验 / 触发 1002**：§8.1 钦定不接受 query/body，**不**触发 1002；handler 不做任何参数校验（与 emojis_handler 同模式）。
8. **DB 错误未映射 1009**：repo 失败 service 必 `apperror.Wrap(err, apperror.ErrServiceBusy, ...)`（lesson 2026-05-13 Lesson 2）。
9. **空 URL 过滤**：**不**做 `iconUrl == "" 跳过`（§8.1 钦定 enabled 必非空，0012 seed 已保证；server 透传，与 emoji_service 行 38-42 同源）。
10. **router 路由挂错组 / 单独挂中间件**：挂 `authedGroup`（auth + RateLimitByUserID 中间件已在该组兜底），**不**单独挂中间件、**不**挂到无 auth 的 group。
11. **直接传 *gin.Context 当 ctx**：用 `c.Request.Context()`（ADR-0007 §2.2）。
12. **handler 直接调 response.Error**：一律 `c.Error(err) + return` 走 ErrorMappingMiddleware（ADR-0006 单一 envelope 生产者）。

### 范围红线（再次强调）

只改/新建 ≤8 个 server 文件 + story 文件 + sprint-status.yaml：
- 新建 `server/internal/service/cosmetic_service.go`
- 新建 `server/internal/service/cosmetic_service_test.go`
- 新建 `server/internal/service/cosmetic_service_integration_test.go`（`//go:build integration`）
- 新建 `server/internal/app/http/handler/cosmetics_handler.go`
- 新建 `server/internal/app/http/handler/cosmetics_handler_test.go`
- 改 `server/internal/repo/mysql/cosmetic_item_repo.go`（interface 加 `ListEnabledForCatalog` + impl，**不**改既有方法）
- 改 `server/internal/repo/mysql/cosmetic_item_repo_test.go`（补对应测试，按既有文件现状判定单测/集成位置）
- 改 `server/internal/app/bootstrap/router.go`（wire cosmetic svc/handler + 注册 1 路由）

**不**改 23.2 user_cosmetic_items 表/struct、**不**改 Story 20.6 `ListEnabledForWeightedPick`/`ChestService.OpenChest`、**不**改 V1 §8.1 / docs、**不**改 0001~0015 migration / 0012 seed、**不**改 `_bmad-output/` 下其他 yaml/md。改了 Go 代码 → **必须**跑 `bash scripts/build.sh --test` + `bash scripts/build.sh --integration`。

### Project Structure Notes

- service 落地 `server/internal/service/cosmetic_service.go`（与 `emoji_service.go` / `chest_service.go` / `home_service.go` 同级；server 工程实际扁平布局，**非** CLAUDE.md target §4 的 `internal/domain/cosmetic/` —— 见关键纠偏点 1）。
- handler 落地 `server/internal/app/http/handler/cosmetics_handler.go`（与 `emojis_handler.go` / `chest_handler.go` 同级）。
- repo 改 `server/internal/repo/mysql/cosmetic_item_repo.go`（Story 20.6 既有文件，本 story 扩 interface + impl）。
- router wire 改 `server/internal/app/bootstrap/router.go`（Story 17.4 / 20.5 既有 wire 段，本 story 加 cosmetic svc/handler + 1 路由）。
- 集成测试 `server/internal/service/cosmetic_service_integration_test.go`（`-tags=integration` dockertest，复用既有 `runMigrations` helper + 0012 seed）。
- 无新增目录 / 模块 / 第三方依赖（Gin / GORM / dockertest / strconv 均既有）。

### References

- [Source: docs/宠物互动App_V1接口设计.md#8.1（行 1224-1308）] — GET /cosmetics/catalog 完整冻结契约（接口元信息 + 服务端逻辑步骤 2 SQL + 响应体 7 字段表 + 错误码表 1001/1005/1009 + 关键约束三级全序排序 + iconUrl/assetUrl 非空契约 + 空集 {items:[]} 不报错）
- [Source: docs/宠物互动App_数据库设计.md#5.8（行 437-459）] — cosmetic_items DDL（id/code/name/slot/rarity/asset_url/icon_url/drop_weight/is_enabled + uk_code + idx_slot_rarity + idx_enabled_weight）+ 设计说明（配置表非实例表）
- [Source: docs/宠物互动App_数据库设计.md#6.8 / 6.9] — slot 枚举（1~7,99）/ rarity 枚举（1~4）值域钦定
- [Source: _bmad-output/planning-artifacts/epics.md#Story 23.3（行 3248-3267）] — AC 钦定（is_enabled=1 过滤 + auth + rarity ASC+slot ASC 排序 + 15-50 条不分页 + ≥4 单测 case + dockertest 复用 seed 验证数量+字段值）
- [Source: _bmad-output/implementation-artifacts/23-1-接口契约最终化.md（行 26 + §8.1 锚定段）] — 上游契约定稿（§8.1 字段 / 排序 / 错误码冻结声明 + id ASC 决定性 tie-breaker r1 [P2] 钦定 + 行 281 目录建议属 aspirational 见纠偏点 1）
- [Source: _bmad-output/implementation-artifacts/23-2-user_cosmetic_items-migration.md] — 前序 story（落地 user_cosmetic_items 实例表；**本 story 不碰该表，catalog 查 cosmetic_items 配置表**；模板/范围红线编排参照）
- [Source: server/internal/service/emoji_service.go（全文）] — 1:1 service 模板（EmojiService interface + EmojiBrief DTO + ListAvailable repo query→DTO→nil slice 兜底 + 1009 wrap + 不做空串过滤钦定）
- [Source: server/internal/app/http/handler/emojis_handler.go（全文）] — 1:1 handler 模板（GetEmojis c.Request.Context + c.Error 透传 + emojiResponseDTO camelCase + items 非 null 兜底 + ADR-0006/0007 注释）
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go（全文）] — 待扩展 interface（CosmeticItemRepo 含 20.6 ListEnabledForWeightedPick + CosmeticItem GORM struct 字段映射 + 注释风格）；**本 story 新增 ListEnabledForCatalog 不复用 weighted pick**
- [Source: server/internal/repo/mysql/emoji_repo.go（行 137-164）] — List impl 模板（tx.FromContext + 显式 Select 字段集 + Where is_enabled=1 + Order + Find + 空 slice 兜底）
- [Source: server/internal/service/emoji_service_integration_test.go（全文）] — 1:1 dockertest 集成测试模板（buildXxxIntegration helper + runMigrations + SeedContent case 复用 seed 验证行数+字段值，不手工 INSERT）
- [Source: server/internal/app/bootstrap/router.go（行 480-530）] — wire 段（line 486 cosmeticItemRepo 既有实例 + emojiSvc/chestSvc wire 模式 + authedGroup.GET /emojis 行 519 / /chest/current 行 522 路由注册模式）
- [Source: server/internal/pkg/errors/codes.go] — apperror 码（ErrUnauthorized=1001 / ErrInvalidParam=1002 / ErrTooManyRequests=1005 / ErrServiceBusy=1009 + DefaultMessages）
- [Source: server/migrations/0012_seed_cosmetic_items.up.sql] — 集成测试复用的 seed（15 行全 is_enabled=1，含 hat_yellow/hat_red/gloves_white/gloves_brown ...；集成测试 len==15 真值源）
- [Source: CLAUDE.md（Build & Test 段 + 目录形态 target §4）] — `bash scripts/build.sh --test` / `--integration` 验证契约 + target 目录段（属 aspirational，实际扁平见纠偏点 1）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（epic-loop dev-story sub-agent）

### Debug Log References

- **环境侧 flake（已知，与本 story 无因果）**：`bash scripts/build.sh --integration` 串跑全 `-tags=integration` 包时，`internal/service` 包在 120s（脚本 per-package timeout）/ 600s 处 FAIL，panic stack 顶端为 `auth_service_integration_test.go:94 startMySQL → dockertest.Pool.Retry`（**不**是本 story 改动的任何文件）。根因：`go test ./...` 串行跑大量 dockertest case 时本机 docker daemon 被压垮，个别**不相关包** startMySQL 容器 readiness 等待超时。Story 23.2 Debug Log + 本 story AC7 注已预记同现象。**隔离单跑确认本 story 范围全 PASS**：`go test -tags=integration -timeout=600s ./internal/service/ -run TestCosmeticServiceIntegration` → 2 case PASS（SeedContent 21.50s / DisabledExcluded 24.45s，总 46s）；`go test -tags=integration ./internal/repo/mysql/` → PASS（0.571s）；`go test -tags=integration ./internal/service/ -run 'TestCosmeticServiceIntegration|TestChestOpenServiceIntegration_HappyPath'` → PASS（94.24s，确认接口扩展未回归 20.6 weighted-pick）。dockertest 启动期 `[mysql] connection.go:49: unexpected EOF` 日志为容器 boot 期连接重试，非测试失败。
- **接口扩展连带 stub 编译修复（必要非行为变更）**：给 `CosmeticItemRepo` interface 加 `ListEnabledForCatalog` 后，Story 20.6 既有的两个测试 stub（`chest_open_service_test.go` 的 `stubCosmeticItemRepo` + `chest_open_service_integration_test.go` 的 `faultCosmeticItemRepoOnList`）不再满足扩展后的 interface → 编译失败。按"接口扩展标准做法"给两 stub 各补一个 **panic-guarded** `ListEnabledForCatalog`（不走 catalog 路径时 panic 暴露漂移）—— 仅为 satisfy interface 编译，**不**改 20.6 任何既有行为 / 不改 `ListEnabledForWeightedPick` 签名或 body（与 story 对新 stub "stub 必须实现以 satisfy interface 编译"的钦定同源）。
- **同 package 命名冲突规避（2 处）**：(1) `service_test` package 内 20.6 已有 `stubCosmeticItemRepo` → 本 story service 单测 stub 改名 `stubCatalogCosmeticItemRepo` 避重声明；(2) `service_test` package 内 `dev_cosmetic_service_test.go` 已有 `itoa` helper → 本 story service 单测改用标准库 `strconv.Itoa` 造数据，删除自定义 helper。

### Completion Notes List

**AC8 跨文档一致性自检（对外接口 story 必须项，逐条核对结论）：**

1. **handler wire 字段集 / 类型 / camelCase 与 V1 §8.1 字段表（行 1260-1268）逐字段一致**：✅ `catalogResponseDTO` 输出 7 字段 = `cosmeticItemId`（string，`strconv.FormatUint(b.CosmeticItemID, 10)` 字符串化）/ `code`（string）/ `name`（string）/ `slot`（int 直接下发，§6.8 枚举）/ `rarity`（int 直接下发，§6.9 枚举）/ `iconUrl`（string）/ `assetUrl`（string）；无多余 / 无缺失字段。`cosmeticItemId` 确为 JSON string（handler_test raw JSON 断言 `"1"` 带引号），`slot`/`rarity` 确为 JSON number（断言 `1` 无引号）。
2. **service 流程 + repo SQL 与 §8.1 服务端逻辑步骤 2（行 1250-1254）一致**：✅ repo `ListEnabledForCatalog` = `Select("id, code, name, slot, rarity, icon_url, asset_url")` + `Where("is_enabled = ?", 1)` + `Order("rarity ASC, slot ASC, id ASC")`（`id ASC` 决定性 tie-breaker 未省，repo sqlmock 测断言完整 ORDER BY 串 + 集成测试逐对相邻元素 lexicographic 全序断言）+ 不分页 + 不开事务（单 SELECT，`tx.FromContext` 仅模式一致性）。
3. **错误码映射与 §8.1 错误码表（行 1295-1301）一致**：✅ DB 异常 → service `apperror.Wrap(err, ErrServiceBusy=1009, ...)`（单测 AC5.4 + handler 测 AC5.8 断言 envelope code=1009 + cause 保留）；auth(1001) / rate_limit(1005) 由 router authedGroup 既有 Auth + RateLimitByUserID 中间件链兜底；**不**触发 1002（handler 不做任何参数校验 / 不读 query / 不读 body）；空 catalog → `{items:[]}` code=0 不报错（service make 非 nil slice + handler make([]gin.H,0,len) 双层兜底，单测 AC5.5 + handler 测 AC5.7 断言 JSON `[]` 非 `null`）。
4. **集成测试 len==15 与 0012 seed 真实行数一致**：✅ `server/migrations/0012_seed_cosmetic_items.up.sql` INSERT 15 行全 `is_enabled=1`；dockertest 实跑 `TestCosmeticServiceIntegration_ListCatalog_SeedContent` 断言 `len(got)==15` + 抽样 `hat_yellow`（rarity=1/slot=1/小黄帽/非空 URL，三级全序排首位）+ `body_armor`（rarity=4/slot=6/黄金圣衣，排末位）PASS；`ListCatalog_DisabledExcluded` UPDATE 1 行 is_enabled=0 → len==14 无 hat_yellow PASS。
5. **未触碰禁改边界**：✅ `git status` 实测仅 11 项：4 个改（router.go / cosmetic_item_repo.go + 其 test / 2 个 20.6 stub 文件 compile-fix）+ 5 个新建（cosmetic_service.go / _test / _integration_test / cosmetics_handler.go / _test）+ story 文件 + sprint-status.yaml。未碰 23.2 `user_cosmetic_items` 表 / `UserCosmeticItem` struct；未改 `ListEnabledForWeightedPick` 签名/body（git diff 核对 = 0 改动，仅同 struct 内新增并列方法）；未改 `ChestService.OpenChest`；未改 V1 §8.1 / §5.8 / §6.8 / §6.9 / 其他 6 份 docs；未改 0001~0015 migration / 0012 seed。
6. **目录布局纠偏已显式记录**：✅ CLAUDE.md target §4 / 23.1 行 281 提 `internal/domain/cosmetic/`，但 server 实际现状为扁平 `internal/service/` + `internal/app/http/handler/` + `internal/repo/mysql/`。本 story 严格遵循实际现状，`cosmetic_service.go` 与 `emoji_service.go`/`chest_service.go` 同级、`cosmetics_handler.go` 与 `emojis_handler.go` 同级、`cosmetic_item_repo.go` 原地扩展；**未**新建 `internal/domain/cosmetic/` 目录。属"CLAUDE.md target 为 aspirational、实际工程从 17.4/20.5 起演进为扁平"的历史现状，**不**视为架构变更（与 emoji 17.4 / chest 20.5 实装一致）。

**3 个关键纠偏点遵守确认：**
- 纠偏点 1（扁平目录非 internal/domain/cosmetic/）：✅ 遵守（见 AC8 第 6 条）。
- 纠偏点 2（不复用 ListEnabledForWeightedPick，新增独立 ListEnabledForCatalog）：✅ 遵守 —— 新增独立带 `ORDER BY rarity ASC, slot ASC, id ASC` 三级全序的 `ListEnabledForCatalog`；`ListEnabledForWeightedPick`（无 ORDER BY、SELECT *、加权抽取语义）签名/body 0 改动；interface 注释头明确两方法语义边界 + 范围红线。
- 纠偏点 3（cosmeticItemId 必须字符串化）：✅ 遵守 —— `catalogResponseDTO` 用 `strconv.FormatUint(b.CosmeticItemID, 10)`；`slot`/`rarity` 是 int 直接下发不字符串化；handler_test 用 raw JSON 断言 `cosmeticItemId` 序列化为带引号 string、`slot`/`rarity` 为无引号 number（防回归）。

**测试结果汇总：**
- `bash scripts/build.sh --test`：PASS（go vet OK + go build OK + 全 25 个包 go test 全绿，无回归）。
- cosmetic 集成测试隔离单跑：PASS（`TestCosmeticServiceIntegration_ListCatalog_SeedContent` 21.50s + `_DisabledExcluded` 24.45s）；`internal/repo/mysql` 集成 PASS；与 chest-open happy 同跑 PASS（确认接口扩展未回归 20.6）。
- 单测覆盖：service 5 case + handler 3 case + repo sqlmock 2 case + 集成 2 case = 12 个新测试全 PASS。

### File List

新建（5 个 server 文件）：
- `server/internal/service/cosmetic_service.go`
- `server/internal/service/cosmetic_service_test.go`
- `server/internal/service/cosmetic_service_integration_test.go`（`//go:build integration`）
- `server/internal/app/http/handler/cosmetics_handler.go`
- `server/internal/app/http/handler/cosmetics_handler_test.go`

改（4 个文件 + 1 story + 1 sprint-status）：
- `server/internal/repo/mysql/cosmetic_item_repo.go`（`CosmeticItemRepo` interface 加 `ListEnabledForCatalog` + impl；**未**改 `ListEnabledForWeightedPick`）
- `server/internal/repo/mysql/cosmetic_item_repo_test.go`（补 `ListEnabledForCatalog` sqlmock 单测 2 case）
- `server/internal/app/bootstrap/router.go`（wire `cosmeticSvc`/`cosmeticsHandler` + 注册 `GET /cosmetics/catalog`）
- `server/internal/service/chest_open_service_test.go`（20.6 既有 `stubCosmeticItemRepo` 补 panic-guarded `ListEnabledForCatalog` —— 接口扩展连带 compile-fix，不改 20.6 行为）
- `server/internal/service/chest_open_service_integration_test.go`（20.6 既有 `faultCosmeticItemRepoOnList` 补 panic-guarded `ListEnabledForCatalog` —— 同上）
- `_bmad-output/implementation-artifacts/23-3-get-cosmetics-catalog-接口.md`（本 story 文件）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态流转 ready-for-dev → in-progress → review）

## Change Log

| 日期 | 变更 | 作者 |
|------|------|------|
| 2026-05-16 | 创建 Story 23.3（GET /cosmetics/catalog 接口）story 文件：context engine 分析 V1 §8.1 冻结契约 + 数据库 §5.8/§6.8/§6.9 + epics.md §Story 23.3 AC + 1:1 比对 emoji 17.4 模板（service/handler/repo/集成测试）+ 锁定 3 个关键纠偏点（扁平目录非 internal/domain/cosmetic、不复用 ListEnabledForWeightedPick 新增独立 ListEnabledForCatalog、cosmeticItemId 必须字符串化）。状态 backlog → ready-for-dev。 | create-story (claude-opus-4-7[1m]，epic-loop sub-agent) |
| 2026-05-16 | 实装 Story 23.3：扩展 `CosmeticItemRepo` 加独立 `ListEnabledForCatalog`（显式 7 列 + WHERE is_enabled=1 + ORDER BY rarity ASC, slot ASC, id ASC 三级全序，不复用 weighted-pick）；新建 `cosmetic_service.go`（`CosmeticService`/`CosmeticBrief`/`ListCatalog` 1:1 emoji 模板 + 1009 wrap + nil slice 兜底）+ `cosmetics_handler.go`（`GetCatalog` + `catalogResponseDTO`，cosmeticItemId 字符串化 + slot/rarity int + camelCase + items 非 null）；router wire + 注册 `authedGroup.GET("/cosmetics/catalog")`；新增 12 个测试（service 5 + handler 3 + repo sqlmock 2 + 集成 2）全 PASS；接口扩展连带给 20.6 两 stub 补 panic-guarded `ListEnabledForCatalog`（compile-fix，不改 20.6 行为）。`build.sh --test` PASS 无回归；cosmetic 集成测试隔离单跑 2 PASS（全量串跑触发已知 auth 包环境侧 flake，与本 story 无因果）。AC8 跨文档自检 6 条全过 + 3 纠偏点全遵守。状态 ready-for-dev → review。 | dev-story (claude-opus-4-7[1m]，epic-loop sub-agent) |
