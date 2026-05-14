# Story 20.8: dev 端点 POST /dev/grant-cosmetic-batch（节点 7 阶段骨架；节点 8 / Epic 23.2 完成后激活真实写库）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a demo / 开发者,
I want 一个 build flag gated 的 dev 接口给指定用户批量发放指定品质的 cosmetic_items 占位（节点 7 阶段仅 stub —— 路由 + handler 框架 + service 占位 + 全套单测，**不**真实写库；handler 内部真实写库逻辑等 Epic 23 Story 23.2 user_cosmetic_items 表 migration 落地后再激活）,
so that 节点 7 阶段提前把"端点骨架"立起来让 Story 25.1 节点 8 跨端 e2e（验证场景 6）/ Story 34.1 节点 11 跨端 e2e（准备步骤）可以基于稳定的 URL + schema 提前编排测试脚本；节点 8 Story 23.2 完成后只需在本 service 内开启"真实写库"分支，**不**改契约 / 路由 / 客户端调用代码。

## 故事定位（Epic 20 第八条 = 节点 7 dev 工具第二条；上承 20.7 dev force-unlock-chest，下启 20.9 Layer 2 集成测试 + 节点 8 Epic 23.2 user_cosmetic_items migration + 节点 11 合成 demo 准备）

- **Epic 20 进度**：20.1（接口契约，**done**）→ 20.2（cosmetic_items migration，**done**）→ 20.3（cosmetic_items seed ≥15 行，**done**）→ 20.4（chest_open_logs migration，**done**）→ 20.5（GET /chest/current 动态判定，**done**）→ 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取，**done**）→ 20.7（POST /dev/force-unlock-chest，**done**）→ **20.8（本 story，POST /dev/grant-cosmetic-batch 节点 7 阶段骨架）** → 20.9（Layer 2 集成测试 - 开箱事务全流程）。
- **本 story 是 Epic 20 第二条 dev 工具实装**（前 7 条业务接口 + 1 条 dev 工具 force-unlock-chest）：
  - 业务目的：dev 端点接收 `{userId, rarity, count}` 三参数，按 rarity 从 cosmetic_items 池中随机抽 count 个 cosmetic_item_id 创建 user_cosmetic_items 实例 —— **但本 story 节点 7 阶段不真实写库**（user_cosmetic_items 表节点 8 Story 23.2 才建）。
  - **节点 7 vs 节点 8 阶段实装策略**（**选项 C** —— epics.md §20.8 行 2964 钦定；**fix-review r1 锁定 explicit-failure 语义** —— 详见 `docs/lessons/2026-05-15-stub-endpoint-explicit-failure.md`）：
    - **节点 7 阶段（本 story 范围）**：路由 `/dev/grant-cosmetic-batch` 注册 + handler 框架（DTO + 参数校验 + ShouldBindJSON + 1002 拦截）+ service 接口 + **stub explicit-failure 实装**（**`slog.WarnContext` 输出 WARN 日志 + return `apperror.New(ErrServiceBusy, "dev/grant-cosmetic-batch not yet implemented (node-7 stub; awaits Story 23.5 to activate)")`** —— middleware 自动翻 HTTP **503** + envelope.code=**1009**；**绝不返 200 success**，避免 silent false-positive 让 e2e / demo 在"调用成功 + 仓库空"的矛盾态里调试很久才发现根因；service 接口签名 final，handler / route 与未来激活后完全一致）+ 全套单测 mock（service 3 case：HappyPathStub_ReturnsServiceBusy_LogsWarn / BoundaryCases_AlwaysReturnsServiceBusy / StubIgnoresInvalidParams_StillReturnsServiceBusy；handler 6 case：HappyPath_ServiceReturnsServiceBusy_Forwards503 / 各种参数错误 / ServiceError_Forwards1009）+ devtools.Register 签名再扩 `(r, devSteps, devChest, devCosmetic)` + bootstrap wire devCosmeticHandler。**不**写真实写库逻辑、**不**新建 user_cosmetic_items_repo（节点 8 Story 23.2 owner）。
    - **节点 8 / Epic 23 阶段（**不**在本 story 范围）**：Story 23.2 落地 user_cosmetic_items migration → Story 23.5 落地 user_cosmetic_item_repo.Create / BatchCreate 后，**本 service 内部"打开开关"** —— 把 service stub 实装替换为"用 cosmetic_item_repo.FindRandomByRarity + user_cosmetic_item_repo.BatchCreate"真实写库（接口签名 / 路由 / 客户端调用代码不变 → 兼容已部署的 e2e 脚本）。本 story 在 service 注释 + handler doc + Completion Notes 里明确标注"等节点 8 / Epic 23.5 后激活真实写库的 todo"。
  - **设计参考**：
    - epics.md §Story 20.8 行 2953-2970 钦定 AC + "节点 7 阶段实装策略"
    - Story 7.5 /dev/grant-steps 老 dev endpoint 模式（事务 + 多 repo wire）
    - **Story 20.7 /dev/force-unlock-chest 最新 dev endpoint 模式**（devtools.Register 签名扩 + nil-collapse wire + interface 解耦 + dev 路径无 auth）
    - docs/宠物互动App_数据库设计.md §5.9 user_cosmetic_items 表结构（节点 8 真实写库时参考；本 story stub 不消费）

- **epics.md AC 钦定**（`_bmad-output/planning-artifacts/epics.md` §Story 20.8 行 2953-2970）：
  - **Given** Epic 1 Dev Tools 框架已就绪 + Story 20.3 cosmetic_items 已 seed + **依赖 user_cosmetic_items 表（节点 8 Story 23.2 完成后才能真实写库）**
  - **When** 仅在 BUILD_DEV=true 模式调用 `POST /dev/grant-cosmetic-batch {userId, rarity, count}`
  - **Then** service 直接 INSERT 多条 user_cosmetic_items（按 rarity 从 cosmetic_items 中随机抽 count 个 cosmetic_item_id 创建实例）
  - **And** **节点 7 阶段实装策略**：路由注册 + handler 框架 + 单测 mock 都完成；handler 内部真实写库逻辑等 Story 23.2 user_cosmetic_items 表 migration 完成后开放（在 Epic 23 sprint 内打开开关）
  - **And** 生产构建下访问该端点返回 404
  - **And** **单元测试覆盖**（≥3 case）:
    - happy: dev mode + rarity=1, count=10 → DB user_cosmetic_items 多 10 行（cosmetic_item_id 来自 common 池随机）—— **节点 7 阶段降级为 stub explicit-failure：service 返 *AppError(ErrServiceBusy=1009) + middleware 翻 HTTP 503；节点 8 激活真实写库后转为完整断言（service 返 nil + 200）**
    - edge: dev mode + rarity=99（非法）→ 1002（handler 层拦截，不到 service）
    - edge: dev mode + count=0 → 1002（handler 层拦截，不到 service）
  - **And** **集成测试覆盖**（节点 8 完成后跑）: /dev/grant-cosmetic-batch {rarity:1, count:10} → DB 验证 10 行新实例 —— **本 story 阶段不写集成测试**（user_cosmetic_items 表未建 → dockertest migrate 失败 + 本来就在节点 8 sprint 内激活，节点 7 提前写无价值）

- **V1 接口设计 doc 状态**：本 dev 端点**不**在 V1 §1-§16 主接口清单内（V1 §1 节点 7 冻结声明只锁 §7.1 / §7.2 业务接口）。dev 端点契约**仅**由 epics.md §Story 20.8 钦定；与 Story 7.5 /dev/grant-steps + Story 20.7 /dev/force-unlock-chest 同政策（dev 端点是私有运维接口，不进 V1 doc）。

- **devtools 框架契约**（Story 1.6 已落地；7.5 / 20.7 已 evolve；详见 `server/internal/app/http/devtools/devtools.go` + `_bmad-output/implementation-artifacts/20-7-dev-端点-post-dev-force-unlock-chest.md` AC3）：
  - **双闸门** OR 启用：(1) build tag `-tags devtools` → `forceDevEnabled=true`；(2) env var `BUILD_DEV=true`（严格字面，**不**接受 `"1"` / `"yes"` / `"TRUE"`）。任一即启用。
  - **Register(r, devStepsHandler, devChestHandler)** 当前签名（Story 20.7 后）→ 本 story 再扩签名为 **`Register(r, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler, devCosmeticHandler DevCosmeticHandler)`** —— 给 dev cosmetic handler 留独立 interface 抽象槽位。
  - **DevOnlyMiddleware()** 是 /dev/* 路由组的请求时第二闸门：IsEnabled() false 时推 ErrResourceNotFound (1003) → ErrorMappingMiddleware 翻成 envelope（HTTP 200 + `code=1003` + `message="资源不存在"`）。
  - **业务 dev 端点扩展模式**：在 `devtools.Register(r, ...)` 内部加 `g.POST("/grant-cosmetic-batch", devCosmeticHandler.PostGrantCosmeticBatch)`（与 7.5 grant-steps / 20.7 force-unlock-chest 同模式）。
  - **devtools 层 vs 业务层分离**：devtools 包只做"框架"（Register + DevOnlyMiddleware + 启停判定 + 各业务 dev handler 的 interface 抽象槽位）；本 story 的"cosmetic dev 业务逻辑"（service 层 `GrantCosmeticBatch` + handler）写到**业务层**（`internal/service/dev_cosmetic_service.go` + `internal/app/http/handler/dev_cosmetic_handler.go`），devtools.go 仅追加 `DevCosmeticHandler` interface + `Register` 签名扩 + 路由注册一行。

## 范围红线（明确不做）

**本 story 只做（节点 7 阶段骨架范围）**：

1. **service 层**：新建 `server/internal/service/dev_cosmetic_service.go` —— `DevCosmeticService` interface + `devCosmeticServiceImpl` 实装 + `NewDevCosmeticService` 构造函数；`GrantCosmeticBatch(ctx, userID, rarity, count) error` 方法（**节点 7 阶段 stub explicit-failure 实装**：`slog.WarnContext` 输出"endpoint called in node-7 stub phase, returns 503 by design"+ **return `apperror.New(ErrServiceBusy, "dev/grant-cosmetic-batch not yet implemented (node-7 stub; awaits Story 23.5 to activate)")`**；middleware 自动翻 HTTP 503 + envelope.code=1009；service 接口签名 final，节点 8 后只在 impl 内部把"return 1009" 替换成"真实写库 + happy path return nil"，**不**改 interface 签名 / 不改 handler / 不改 route）。
2. **handler 层**：新建 `server/internal/app/http/handler/dev_cosmetic_handler.go` —— `DevCosmeticHandler` struct + `NewDevCosmeticHandler` + `PostGrantCosmeticBatch(c *gin.Context)` 方法 + `PostGrantCosmeticBatchRequest` DTO（含 `userId *uint64` + `rarity *int8` + `count *int32` 三字段，指针类型 + 手动 nil/范围校验；与 7.5 / 20.7 同模式）+ `postGrantCosmeticBatchResponseDTO` helper（返 `{userId, rarity, count}` 简单 ack）。
3. **devtools 层**：扩 `server/internal/app/http/devtools/devtools.go` —— 改 `Register` 签名为 `Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler, devCosmeticHandler DevCosmeticHandler)`（接收第三个业务 handler）；新增 `DevCosmeticHandler interface { PostGrantCosmeticBatch(c *gin.Context) }`；在 `if devChestHandler != nil { g.POST("/force-unlock-chest", ...) }` 之后追加 `if devCosmeticHandler != nil { g.POST("/grant-cosmetic-batch", devCosmeticHandler.PostGrantCosmeticBatch) }`。**关键 nil 陷阱**：与 7.5 / 20.7 同模式 —— nil-tolerant 跳过路由 + interface 解耦避免 import cycle。
4. **bootstrap 层**：扩 `server/internal/app/bootstrap/router.go` —— 在业务 wire 块**外**（NewRouter 函数顶部，紧邻 `var devChestHandler` 之后）追加 `var devCosmeticHandler *handler.DevCosmeticHandler // Story 20.8` 提前声明 nil；在 `if deps.GormDB != nil && ...` 块**内**（紧邻 `devChestHandler = handler.NewDevChestHandler(devChestSvc)` 之后）追加 `devCosmeticSvc := service.NewDevCosmeticService()` + `devCosmeticHandler = handler.NewDevCosmeticHandler(devCosmeticSvc)`（**节点 7 阶段 stub 无 repo 依赖** —— 不需要传 chestRepo / cosmeticItemRepo / userCosmeticItemRepo；节点 8 激活时再扩 constructor 签名加 repo 依赖）；扩 nil-collapse Register 调用为 `Register(r, stepsArg, chestArg, cosmeticArg)`。
   - **不**改 `Deps` struct（节点 7 阶段 stub 不需要新依赖；节点 8 激活时再决定是否加新 repo 字段到 Deps，由 Story 23.5 owner）。
   - **不**改 `cmd/server/main.go`（router wire 内部消费 Deps，main.go 透明）。
5. **repo 层**：**完全不动**（节点 7 阶段 stub service 不调任何 repo；节点 8 激活时 Story 23.5 owner 在 user_cosmetic_item_repo.go 新建 BatchCreate 方法 + cosmetic_item_repo.go 扩 FindRandomByRarity 方法 + 本 service 内部消费）。本 story **不**新建 user_cosmetic_item_repo.go，**不**改 cosmetic_item_repo.go。
6. **service 单测**：新建 `server/internal/service/dev_cosmetic_service_test.go` —— ≥3 case（HappyPathStub_ReturnsNil_LogsWarn / InvalidRarity_DefersToHandler / InvalidCount_DefersToHandler）。本 story 阶段 stub service 内只做 slog.WarnContext + return nil；**无 repo stub 需要**（service 不调 repo）。**单测约定**：HappyPath 验"slog WARN 被触发 + return nil"；rarity / count 边界 case 验"service 不做参数防御"（这与 7.5 dev grant 的"steps<0 panic 防御"模式不同 —— 因为本 story 节点 7 阶段 service 是 stub，没有真实业务防御性 panic；handler 已校验 rarity ∈ [1,4] + count ∈ [1,100]）。
7. **handler 单测**：新建 `server/internal/app/http/handler/dev_cosmetic_handler_test.go` —— ≥5 case（HappyPath_ReturnsAck / RarityInvalid_99_Returns1002 / RarityZero_Returns1002 / CountZero_Returns1002 / CountTooLarge_101_Returns1002）+ 2 加分 case（MissingFields_Returns1002 / InvalidJSON_Returns1002 / ServiceError_Forwards1009）。stub 设计：新建独立 `stubDevCosmeticService`（与 stubDevStepService / stubDevChestService 平级）。
8. **devtools 框架测试扩展**：扩 `server/internal/app/http/devtools/devtools_test.go` —— 追加 `devCosmeticHandlerFunc` adapter + 2 case（GrantCosmeticBatchRegisteredWhenHandlerProvided / GrantCosmeticBatchSkippedWhenHandlerNil）+ 把既有所有 `devtools.Register(r, X, Y)` 调用改为 `devtools.Register(r, X, Y, nil)`（编译期错误兜底，不漏改 —— 7.5 / 20.7 已 lesson 同模式）。
9. **bootstrap 框架测试扩展**：扩 `server/internal/app/bootstrap/router_dev_test.go` —— 追加 1 case：BUILD_DEV=true + Deps{} 零值 → devCosmeticHandler 保持 nil → /dev/grant-cosmetic-batch 返 404（nil-tolerant 路径；与 7.5 / 20.7 同模式）。
10. **集成测试**：**本 story 不写**（user_cosmetic_items 表未建 → dockertest migrate 跑不通；本来就在节点 8 sprint 内由 Story 23.5 落地真实写库时一起加 dev_cosmetic_service_integration_test.go —— epics.md §20.8 行 2970 钦定"集成测试覆盖（节点 8 完成后跑）"）。本 story 在 Completion Notes 里明确标注"集成测试 deferred to Story 23.5 阶段"。
11. **本 story 文件 + sprint-status.yaml** 更新。

**本 story 不做**：

- **不**改 `docs/宠物互动App_V1接口设计.md` 任一行（dev 端点是私有运维接口，不进 V1 doc；与 7.5 / 20.7 同政策）
- **不**改 `docs/宠物互动App_数据库设计.md` 任一行（user_cosmetic_items §5.9 表结构由 Epic 23.2 落地；本 story 不消费）
- **不**新建 `migrations/0015_init_user_cosmetic_items.up.sql`（节点 8 Story 23.2 owner —— 严禁本 story 越权落地 migration；侵入 Epic 23 scope 反模式；选项 B 已被明确拒绝）
- **不**新建 `server/internal/repo/mysql/user_cosmetic_item_repo.go`（节点 8 Story 23.5 owner —— 节点 7 阶段 stub service 不调 repo）
- **不**改 `server/internal/repo/mysql/cosmetic_item_repo.go`（节点 8 Story 23.5 / 节点 11 Story 32.4 在用到时再扩 FindRandomByRarity 方法；本 story stub 不调）
- **不**改 `chest_service.go` (20.5) / `chest_open_service.go` (20.6) / `dev_chest_service.go` (20.7) 任一行（dev cosmetic 走独立 service / handler，**不**复用）
- **不**改 `dev_step_service.go` (7.5) / `dev_chest_service.go` (20.7) / `step_service.go` 任一行（dev cosmetic 走独立 service，**不**复用其他 dev service）
- **r1 阶段**：不改 `internal/pkg/errors/codes.go`（1002 / 1003 / 1009 全已注册；仅消费）
- **r2 阶段（review round 2 修复）**：在 `internal/pkg/errors/codes.go` 新增 `ErrNotImplemented = 1010`
  + DefaultMessages["接口未实装"]，并改 `internal/app/http/middleware/error_mapping.go` 让 1010 走 HTTP 501 + WARN log。
  这是**通用基建**，本 story stub 端点是首个消费者；未来 dev / preview 端点同模式复用
- **不**改 `internal/app/http/middleware/auth.go` / `rate_limit.go` / `error_mapping.go`（已实装；dev 端点**不**挂 auth / **不**挂 rate_limit；与 7.5 / 20.7 同）
- **不**改 `Deps` struct（节点 7 阶段 stub 无新依赖；节点 8 激活时由 Story 23.5 决定）
- **不**接 Redis（dev 端点不需要 Redis；与 7.5 / 20.7 同）
- **不**接幂等键 `idempotencyKey`（dev 端点是"故意可重复"语义；与 7.5 / 20.7 同政策）
- **不**接 version 乐观锁（节点 7 阶段 stub 不写库；节点 8 激活后 user_cosmetic_items 表 INSERT 也不参与乐观锁）
- **不**接事务（节点 7 阶段 stub 无写操作；节点 8 激活后 BatchCreate 单 INSERT statement 是原子的；如未来扩展为"BatchCreate user_cosmetic_items + 写 admin_grant 审计行"则节点 8 owner 再决定是否要事务）
- **不**做"真实写库"分支实装（路由 + handler + service stub + 单测都做，但 service 内部"调 cosmetic_item_repo.FindRandomByRarity + user_cosmetic_item_repo.BatchCreate"两步路径**严禁**在本 story 落地 —— 节点 8 Story 23.5 owner 在 user_cosmetic_items migration done 后激活）
- **不**支持 `userId=0` / `rarity=0` / `rarity>4` / `count=0` / `count>100` 等异常（handler 显式 1002 拦截）
- **不**写 e2e 跨端测试（Epic 22 Story 22.1 / Epic 25 Story 25.1 / Epic 34 Story 34.1 才做）
- **不**写性能压测（dev 端点不接 prod 流量）
- **不**写集成测试（user_cosmetic_items 表节点 8 才建 —— dockertest migrate 跑不通；与 epics.md §20.8 行 2970 钦定"集成测试节点 8 后跑"一致 —— 本 story 阶段写集成测试无价值且会破 build）
- **不**改 `docs/lessons/*.md`（无新教训；本 story 是 7.5 / 20.7 dev endpoint 模式 + 节点 7/8 阶段切片的直接落地）
- **不**预实装节点 8 Story 23.2 user_cosmetic_items migration（即便顺手把表也建上也禁止 —— YAGNI + 选项 B 越权 + Story 23.2 owner 抢工范畴）
- **不**预实装节点 8 Story 23.5 真实写库（包括 user_cosmetic_item_repo.BatchCreate / cosmetic_item_repo.FindRandomByRarity 等任何节点 8 才该有的 repo 方法）

**任何超出上述清单的改动 → HALT 并问设计**。

## Acceptance Criteria

**AC1 — `service.DevCosmeticService` interface + 节点 7 阶段 stub impl（新文件 `internal/service/dev_cosmetic_service.go`）**

> **fix-review r1 锁定**：service stub 改为 **explicit-failure**（return `apperror.New(ErrServiceBusy, "...")` →
> middleware 自动翻 HTTP 503），**绝不返 nil**。silent false-positive 会让 e2e / demo 调试链路无故拉长。
>
> **fix-review r2 锁定**：r1 用的 ErrServiceBusy (1009) 触发两个 P2：
>   1. middleware 把 1009 映射到 HTTP 500（非 503）—— e2e 工具按 503 检测会失败
>   2. 1009 路径走 ERROR log → 每次 stub 调用记 ERROR → 污染监控 + 假告警
>
> r2 引入新错误码 **`apperror.ErrNotImplemented = 1010`** → middleware 翻 **HTTP 501 (Not Implemented)** +
> **WARN log**（不污染监控）。HTTP 501 是标准"Not Implemented"语义，e2e 工具能正确识别"endpoint 未激活"。
> 详见 `docs/lessons/2026-05-15-stub-endpoint-not-implemented-error-code.md`。

新建 `server/internal/service/dev_cosmetic_service.go`：

```go
package service

import (
	"context"
	"log/slog"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// DevCosmeticService 是 /dev/grant-cosmetic-batch 端点的依赖 interface（Story 20.8）。
//
// **dev 端点的产品语义**：给指定用户批量发放指定品质的 cosmetic_items 实例（按 rarity 从 cosmetic_items
// 池中随机抽 count 个 cosmetic_item_id 创建 user_cosmetic_items 实例），让节点 11 合成 demo
// 不必反复开箱凑齐 10 件 common。仅供 demo / 自动化 e2e / 手工调试，**不**走 prod。
//
// # 节点 7 vs 节点 8 阶段实装策略（**选项 C**，epics.md §20.8 行 2964 钦定）
//
// **节点 7 阶段（本 story 范围）：stub 显式失败实装**
//   - 路由 /dev/grant-cosmetic-batch + handler 框架（DTO + 1002 参数校验）+ service 接口 final
//   - service 实装内部 slog.WarnContext + return apperror.ErrServiceBusy (1009) → middleware 翻 HTTP 503
//   - 全套单测（service 3 case + handler 6 case + devtools 2 case + bootstrap 1 case），断言 1009/503
//   - **不**新建 user_cosmetic_items_repo / **不**新建 migration / **不**改 cosmetic_item_repo
//
// **节点 8 / Epic 23.5 阶段（**不**在本 story 范围 —— 由 23.5 owner 在本 service 内激活）：真实写库**
//   - Story 23.2 落地 user_cosmetic_items migration + 23.5 落地 user_cosmetic_item_repo.BatchCreate
//     + cosmetic_item_repo.FindRandomByRarity（若不存在则同步落地）后
//   - 修改本 service 实装：移除"stub 返 1009"分支 → 加 cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count)
//     + userCosmeticItemRepo.BatchCreate(ctx, userID, []cosmeticItemIDs) 两步写库 → 成功 return nil
//   - 修改 NewDevCosmeticService 构造函数签名加新 repo 依赖
//   - 同步把 service 单测里 1009 断言换成"happy path return nil + repo BatchCreate 被调"
//   - **接口签名 / 路由 / 客户端调用代码不变** —— 兼容已部署的 e2e 脚本
//
// # 错误约定（ADR-0006 三层映射）
//
// **节点 7 阶段（本 story）**：
//   - rarity / count 越界由 handler 1002 拦截，service 不收到
//   - service stub 实装 **永远 return ErrServiceBusy (1009)**：endpoint 物理可达但功能未激活
//     → middleware 翻 HTTP 503，调用方明确知道"endpoint not yet active"
//
// **节点 8 阶段（激活后）**：
//   - 真实写库 happy path → return nil
//   - mysql.ErrCosmeticItemNotFound（FindRandomByRarity 没数据 —— 理论 Story 20.3 seed ≥15 行不该发生）
//     → 包成 ErrServiceBusy (1009)（seed 数据完整性异常）
//   - userCosmeticItemRepo.BatchCreate 失败 → 包成 ErrServiceBusy (1009)
//   - userRepo.FindByID 验用户存在（可选，节点 8 owner 决定）→ ErrUserNotFound → ErrResourceNotFound (1003)
type DevCosmeticService interface {
	// GrantCosmeticBatch 给指定 userID 批量发放 count 个 rarity 品质的 cosmetic_items 实例。
	//
	// **节点 7 阶段 stub 行为**：slog.WarnContext 记录调用 + return apperror.ErrServiceBusy (1009)
	// → middleware 自动翻 HTTP 503。endpoint 物理可达（路由 / handler / DTO 校验完整），但 service
	// 层显式拒绝 —— 让调用方立刻知道"endpoint not yet active in node-7 phase"，避免 silent false-positive。
	//
	// **节点 8 激活后行为**：事务内或事务外（节点 8 owner 决定）：
	//  1. cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count) 返回 count 个 cosmetic_item_id（来自 enabled 池）
	//  2. userCosmeticItemRepo.BatchCreate(ctx, userID, cosmeticItemIDs, source=2 admin_grant)
	//     → INSERT 多条 user_cosmetic_items 行（status=1 in_bag / source=2 / source_ref_id=NULL / obtained_at=now）
	//  3. happy path return nil；任一步失败 wrap 成 1009
	//
	// 参数：
	//   - userID：目标用户 ID（handler 已校验 > 0）
	//   - rarity：装扮品质，1=common / 2=rare / 3=epic / 4=legendary（§6.9 钦定；handler 已校验 ∈ [1,4]）
	//   - count：发放数量，1 ≤ count ≤ 100（handler 已校验；上限 100 防 demo 误传 1e6 砸 DB）
	//
	// **不**接 cosmeticItemID 参数（dev 产品语义是"按品质随机抽"，不是"指定 cosmetic 发放"；
	// 未来如需"指定 cosmetic 发放"加独立 /dev/grant-cosmetic-by-id 端点，YAGNI 本 story 不预实装）。
	GrantCosmeticBatch(ctx context.Context, userID uint64, rarity int8, count int32) error
}

// devCosmeticServiceImpl 是 DevCosmeticService 的节点 7 阶段 stub 实装。
//
// **节点 7 阶段**：无 repo 依赖（不写库；显式返 1009）。
// **节点 8 激活后**：在本 struct 加 cosmeticItemRepo + userCosmeticItemRepo 字段（节点 8 owner 改）。
type devCosmeticServiceImpl struct{}

// NewDevCosmeticService 构造 DevCosmeticService 节点 7 阶段 stub。
//
// **节点 7 阶段**：无参数（stub 不需要 repo）。
// **节点 8 激活时**：节点 8 owner 改签名加 repo 依赖，如：
//
//	func NewDevCosmeticService(cosmeticItemRepo mysql.CosmeticItemRepo,
//	    userCosmeticItemRepo mysql.UserCosmeticItemRepo) DevCosmeticService { ... }
//
// 接口签名 / 路由 / 客户端调用代码不变 → 兼容已部署的 e2e 脚本。
func NewDevCosmeticService() DevCosmeticService {
	return &devCosmeticServiceImpl{}
}

// GrantCosmeticBatch 节点 7 阶段 stub 实装：WARN 日志 + return ErrServiceBusy (1009)。
//
// **设计原则**：stub endpoint **绝不返 success** —— silent false-positive 会让 e2e / demo
// 链路在"调用成功 + 仓库空"的矛盾态里调试很久才发现根因。显式返 1009 → middleware 翻 HTTP 503，
// 调用方立刻看到"endpoint not yet active in node-7 phase"。
//
// WARN log 级别保留（不是 ERROR）—— 节点 7 阶段被调用是预期"未激活路径"，不是系统错误。
//
// **节点 8 激活后** 替换为：
//
//	cosmeticItemIDs, err := s.cosmeticItemRepo.FindRandomByRarity(ctx, rarity, count)
//	if err != nil { return apperror.Wrap(err, apperror.ErrServiceBusy, "...") }
//	if err := s.userCosmeticItemRepo.BatchCreate(ctx, userID, cosmeticItemIDs, ...); err != nil {
//	    return apperror.Wrap(err, apperror.ErrServiceBusy, "...")
//	}
//	slog.InfoContext(ctx, "dev grant cosmetic batch applied", ...)
//	return nil
func (s *devCosmeticServiceImpl) GrantCosmeticBatch(ctx context.Context, userID uint64, rarity int8, count int32) error {
	slog.WarnContext(ctx, "dev grant-cosmetic-batch called in node-7 stub phase, returns 503 by design (endpoint not yet active; awaits Story 23.5 to activate after Story 23.2 user_cosmetic_items migration)",
		"user_id", userID, "rarity", rarity, "count", count,
		"phase", "node-7-stub",
		"todo", "activate real writes in Story 23.5 (after Story 23.2 user_cosmetic_items migration)",
	)
	return apperror.New(apperror.ErrServiceBusy, "dev/grant-cosmetic-batch not yet implemented (node-7 stub; awaits Story 23.5 to activate)")
}
```

**关键约束**：

- **新 service 文件**：dev cosmetic 业务规则与 7.5 / 20.7 完全不同 → 独立文件清晰；**不**复用 ChestService / DevChestService / DevStepService
- **stub 无 repo 依赖**：节点 7 阶段 NewDevCosmeticService 无参数；节点 8 激活时由 23.5 owner 加 repo 依赖（接口签名不变）
- **WARN 日志格式**：明确标注 `phase="node-7-stub"` + `todo="activate ... in Story 23.5"` —— 让运维 / 开发 / 自动化测试能 grep `phase=node-7-stub` 找出还在 stub 状态的 dev 端点（与 7.5 / 20.7 slog.WarnContext 同模式 + 加强结构化）
- **stub explicit failure**：永远 return `apperror.New(ErrServiceBusy, ...)` → middleware 翻 HTTP 503。**禁止**返 nil success（fix-review r1 锁定；silent false-positive 让 e2e 调试链路无故拉长）。即便 service 收到非法 rarity / count 也仍返 1009 —— handler 已 1002 拦截，service 不防御参数但保持 stub 显式拒绝
- **接口文档清晰标注"节点 7 vs 节点 8"**：让节点 8 激活时 owner 不必再读 epic / sprint-status 就能知道激活路径（节点 8 激活时把"return 1009"替换成"真实写库 + return nil"）

**AC2 — `handler.DevCosmeticHandler` + `PostGrantCosmeticBatchRequest` DTO（新文件 `internal/app/http/handler/dev_cosmetic_handler.go`）**

新建 `server/internal/app/http/handler/dev_cosmetic_handler.go`：

```go
package handler

import (
	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// DevCosmeticHandler 是 /dev/grant-cosmetic-batch 等 dev 装扮端点的 handler 集合（Story 20.8）。
//
// 与 ChestHandler (20.5 / 20.6) / DevStepsHandler (7.5) / DevChestHandler (20.7) 区分：
//   - DevCosmeticHandler 处理 /dev/grant-cosmetic-batch（dev 工具；不含 auth / rate_limit / 事务）
//   - 与 DevStepsHandler / DevChestHandler 平级：dev 工具按"业务模块"独立 handler，让未来加
//     /dev/grant-cosmetic-by-id 或其他 cosmetic 相关 dev 端点时有独立 handler 槽位，避免单文件膨胀。
//
// **节点 7 阶段 stub**：handler 路径完整（参数校验 / DTO / response 全实装），底层 service 是 stub。
//                       节点 8 激活时 handler **不**改 —— service 内部从 stub 切真实写库即可。
type DevCosmeticHandler struct {
	svc service.DevCosmeticService
}

// NewDevCosmeticHandler 构造 DevCosmeticHandler。
func NewDevCosmeticHandler(svc service.DevCosmeticService) *DevCosmeticHandler {
	return &DevCosmeticHandler{svc: svc}
}

// PostGrantCosmeticBatchRequest 是 POST /dev/grant-cosmetic-batch 请求体的 Go mirror。
//
// epics.md §Story 20.8 行 2962 钦定：`{userId, rarity, count}`。
//
// **userId 用 *uint64 指针类型** + **rarity 用 *int8 指针类型** + **count 用 *int32 指针类型**：
//   - validator/v10 把 0 视为 zero value 会误判 "required"，与 7.5 PostGrantStepsRequest / 20.7
//     PostForceUnlockChestRequest 同模式
//   - 用指针 + handler 显式 nil 校验区分 "字段缺失" vs "显式传 0"
//   - userId=0 / rarity=0 / count=0 在业务上都是非法值，handler 显式拒让错误更早 + 错误消息更精确
//
// **rarity 范围**（数据库设计.md §6.9）：1=common / 2=rare / 3=epic / 4=legendary —— handler 校验 ∈ [1,4]
// **count 范围**：1 ≤ count ≤ 100 —— handler 校验；上限 100 防 demo 误传 1e6 砸 DB（节点 11 合成 demo 凑 10 件 common 用 count=10 / 12 即可）
//
// **不**接 cosmeticItemId 字段（dev 产品语义是"按品质随机抽"，不是"指定 cosmetic 发放"）。
// **不**接 idempotencyKey 字段（dev 端点是"故意可重复"语义）。
type PostGrantCosmeticBatchRequest struct {
	UserID *uint64 `json:"userId"`
	Rarity *int8   `json:"rarity"`
	Count  *int32  `json:"count"`
}

// PostGrantCosmeticBatch 处理 POST /dev/grant-cosmetic-batch（Story 20.8 节点 7 阶段 stub）。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：
//     - userId 非 nil + != 0（1002）
//     - rarity 非 nil + ∈ [1,4]（1002）
//     - count 非 nil + ∈ [1,100]（1002）
//  3. 调 svc.GrantCosmeticBatch(ctx, *userId, *rarity, *count) —— ctx = c.Request.Context()
//     **节点 7 阶段 service 是 stub return nil**；handler 与 stub / 激活后行为完全兼容
//  4. 成功 → response.Success(c, postGrantCosmeticBatchResponseDTO(...), "ok")
//  5. 失败 → c.Error(err) + return（middleware envelope；ADR-0006 单一 envelope 生产者）
//
// **不**做 auth 校验（dev 端点不要求 auth）；**不**取 c.Get(UserIDKey)（dev 路径无 auth 中间件）。
func (h *DevCosmeticHandler) PostGrantCosmeticBatch(c *gin.Context) {
	var req PostGrantCosmeticBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// === userId 校验 ===
	if req.UserID == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必填"))
		return
	}
	if *req.UserID == 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必须 > 0"))
		return
	}

	// === rarity 校验（§6.9 枚举：1=common / 2=rare / 3=epic / 4=legendary）===
	if req.Rarity == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "rarity 必填"))
		return
	}
	if *req.Rarity < 1 || *req.Rarity > 4 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "rarity 必须 ∈ [1,4]"))
		return
	}

	// === count 校验（1 ≤ count ≤ 100）===
	if req.Count == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "count 必填"))
		return
	}
	if *req.Count < 1 || *req.Count > 100 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "count 必须 ∈ [1,100]"))
		return
	}

	if err := h.svc.GrantCosmeticBatch(c.Request.Context(), *req.UserID, *req.Rarity, *req.Count); err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	// 成功响应 —— 简单 ack，不返实际创建的 user_cosmetic_items 实例 id 列表
	// （节点 7 阶段 stub 不写库，没有实例 id 可返；节点 8 激活后由 23.5 owner 决定是否扩 response schema
	//  返实例 id 列表 —— 兼容性：先返简单 ack，等节点 11 demo / e2e 真有调用方需要再扩）
	response.Success(c, postGrantCosmeticBatchResponseDTO(*req.UserID, *req.Rarity, *req.Count), "ok")
}

// postGrantCosmeticBatchResponseDTO 拼装 ack response。
//
// **schema 选择**：返 `{userId, rarity, count}` 简单 ack —— 不返实际创建的实例 id 列表（节点 7 阶段 stub
// 没有实例 id；节点 8 激活后由 23.5 owner 决定是否扩 schema）。
//
// 与 Story 7.5 postGrantStepsResponseDTO / Story 20.7 postForceUnlockChestResponseDTO 同模式
//（dev 端点统一 ack 风格）。
func postGrantCosmeticBatchResponseDTO(userID uint64, rarity int8, count int32) gin.H {
	return gin.H{
		"userId": userID,
		"rarity": rarity,
		"count":  count,
	}
}
```

**关键约束**：

- handler 层**必须**做 userId / rarity / count nil + 范围校验（与 7.5 / 20.7 同模式：validator/v10 把 0 视为 zero value 会误判 required）
- rarity ∈ [1,4]、count ∈ [1,100] 显式拒（防 demo 误传非法值砸下游 / 节点 8 激活后砸 DB）
- response 返简单 ack `{userId, rarity, count}`，**不**返实例 id 列表（端点单一职责；节点 8 激活后 schema 兼容性扩展由 23.5 owner 决定）
- handler **不**调 `c.Get(UserIDKey)`（dev 路径无 auth）；userID 全靠 body
- `c.Error + return` 而非 `response.Error`（ADR-0006）

**AC3 — `devtools.Register` 签名扩 + 业务 dev 路由注册（修改 `internal/app/http/devtools/devtools.go`）**

修改 `server/internal/app/http/devtools/devtools.go`：

1. **新增 DevCosmeticHandler interface**（与 DevStepsHandler / DevChestHandler 平级）：

```go
// DevCosmeticHandler 是 dev 装扮端点的 handler 抽象（Story 20.8 节点 7 阶段 stub）。
//
// 用 interface 解耦避免 devtools 包反向 import handler 包：实际的 handler 实装在
// internal/app/http/handler/dev_cosmetic_handler.go；本 interface 仅为 Register 签名抽象，
// 让 devtools 包保持"框架"角色，不依赖具体 handler 实装。
//
// **签名简化原则**：本 interface 只列 Register 签名所需的方法（PostGrantCosmeticBatch）；
// future 加 /dev/grant-cosmetic-by-id 等同 cosmetic 业务模块的 dev 端点时，可考虑加到本 interface
// （同业务模块原则）；其他业务模块（如 /dev/grant-room）新建独立 DevRoomHandler interface。
type DevCosmeticHandler interface {
	PostGrantCosmeticBatch(c *gin.Context)
}
```

2. **改 Register 签名**：从 `Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)` 改为 `Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler, devCosmeticHandler DevCosmeticHandler)`，追加 nil-tolerant 路由注册：

```go
// Register 把 /dev/* 路由组挂到传入的 gin.Engine 上（仅在 dev 模式启用时）。
//
// 启用时挂载以下端点：
//   - GET  /dev/ping-dev               → PingDevHandler（Story 1.6 框架自带）
//   - POST /dev/grant-steps            → devStepsHandler.PostGrantSteps（Story 7.5；devStepsHandler == nil 时跳过）
//   - POST /dev/force-unlock-chest     → devChestHandler.PostForceUnlockChest（Story 20.7；devChestHandler == nil 时跳过）
//   - POST /dev/grant-cosmetic-batch   → devCosmeticHandler.PostGrantCosmeticBatch（Story 20.8 节点 7 stub；devCosmeticHandler == nil 时跳过）
//
// **多 handler 可空设计**（nil-tolerant）：
//   - 单元测试 NewRouter(Deps{}) 零值场景：bootstrap 不构造业务 handler → 都传 nil
//     → 本函数仅注册 ping-dev，跳过所有业务路由（避免 nil deref panic）
//   - 生产路径：bootstrap 在 deps 完整时构造全部业务 handler 透传 → 本函数注册全部 dev 端点
//
// **签名扩展模式**：每加一个业务 dev 端点（grant-steps / force-unlock-chest / grant-cosmetic-batch /
// ...），按业务模块独立 interface 槽位扩 Register 签名。
// 这让"哪些 dev 端点存在"在 Register 签名层就可见，避免运行时神秘失踪。
func Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler, devCosmeticHandler DevCosmeticHandler) {
	if !IsEnabled() {
		return
	}
	slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION", ...)
	g := r.Group("/dev")
	g.Use(DevOnlyMiddleware())
	g.GET("/ping-dev", PingDevHandler)
	if devStepsHandler != nil {
		g.POST("/grant-steps", devStepsHandler.PostGrantSteps)
	}
	if devChestHandler != nil {
		g.POST("/force-unlock-chest", devChestHandler.PostForceUnlockChest)
	}
	if devCosmeticHandler != nil {
		// Story 20.8 加：业务 dev 端点 /dev/grant-cosmetic-batch；nil-tolerant 跳过避免单测 panic。
		g.POST("/grant-cosmetic-batch", devCosmeticHandler.PostGrantCosmeticBatch)
	}
}
```

**关键约束**：

- 用 interface 解耦：devtools 包**不**反向 import handler 包（避免 import cycle；与 7.5 / 20.7 同模式）
- 每业务模块独立 interface（DevStepsHandler / DevChestHandler / DevCosmeticHandler 平级），**不**塞到同一个 interface
- nil-tolerant：每 handler nil 时跳过对应路由 —— 与 NewRouter 的 `if deps.GormDB != nil && ...` 同 nil-tolerant 模式
- **不**新建 /dev 路由组（仍在 `g := r.Group("/dev")`）—— 与现有 ping-dev / grant-steps / force-unlock-chest 共享 DevOnlyMiddleware

**AC4 — `bootstrap/router.go` wire dev cosmetic service / handler / Register 签名（修改 `internal/app/bootstrap/router.go`）**

修改 `server/internal/app/bootstrap/router.go`：

1. **在 NewRouter 函数顶部 `var devChestHandler` 之后**追加 `var devCosmeticHandler *handler.DevCosmeticHandler // Story 20.8`：

```go
var devStepsHandler *handler.DevStepsHandler    // Story 7.5
var devChestHandler *handler.DevChestHandler    // Story 20.7
var devCosmeticHandler *handler.DevCosmeticHandler // Story 20.8（节点 7 阶段 stub；节点 8 激活后保持声明位置不变）
```

2. **在 `if deps.GormDB != nil && ...` 块内**（紧邻 `devChestHandler = handler.NewDevChestHandler(devChestSvc)` 之后）追加 dev cosmetic service / handler 构造：

```go
// Story 20.8 加：dev cosmetic service + handler（节点 7 阶段 stub —— 无 repo 依赖）
//
// **节点 7 阶段**：stub service 不写库，NewDevCosmeticService() 无参数。
// **节点 8 / Story 23.5 激活后**：23.5 owner 改 constructor 签名加 repo 依赖，如：
//
//	devCosmeticSvc := service.NewDevCosmeticService(cosmeticItemRepo, userCosmeticItemRepo)
//
// router.go wire 顺序不变；只改 constructor 签名 + 加 repo 依赖（chestRepo 已在 if 块顶部 wire，
// cosmeticItemRepo 待 23.5 owner 新建实例 + 加 Deps 字段）。
devCosmeticSvc := service.NewDevCosmeticService()
devCosmeticHandler = handler.NewDevCosmeticHandler(devCosmeticSvc)
```

3. 改 `devtools.Register` 调用为 nil-collapse 三参数：

```go
// dev 模式下挂 /dev/* 路由组；devStepsHandler / devChestHandler / devCosmeticHandler 在 deps
// 完整时填充，否则保持 nil。**关键 Go 接口 nil 陷阱**（与 7.5 / 20.7 同 lesson）：typed-nil
// 传给 interface 会被 != nil 误判 → nil-collapse 写法保护（每加 1 个 handler 只需 +2 行）。
//
// 8 分支矩阵被 nil-collapse 化解 —— Story 20.7 选 nil-collapse 替换 7.5 的 4 分支显式 if 即为此 evolve。
var stepsArg devtools.DevStepsHandler
if devStepsHandler != nil {
	stepsArg = devStepsHandler
}
var chestArg devtools.DevChestHandler
if devChestHandler != nil {
	chestArg = devChestHandler
}
var cosmeticArg devtools.DevCosmeticHandler
if devCosmeticHandler != nil {
	cosmeticArg = devCosmeticHandler
}
devtools.Register(r, stepsArg, chestArg, cosmeticArg)
```

**关键约束**：

- `var devCosmeticHandler *handler.DevCosmeticHandler` 提前声明 nil —— deps 不完整时（单测）保持 nil，devtools.Register 内部跳过 grant-cosmetic-batch 路由
- 节点 7 阶段 `NewDevCosmeticService()` 无参数（stub 无依赖）；节点 8 激活时改签名加 repo 依赖，**不**改 router.go wire 顺序 / 不改 nil-collapse 段
- nil-collapse 写法防 typed-nil-interface 陷阱（与 20.7 同 lesson；新加第三个 handler 只需 +2 行 var + 1 行 `Register(r, ..., cosmeticArg)` 参数）
- **不**改 `Deps` struct（节点 7 阶段 stub 不需要 cosmeticItemRepo / userCosmeticItemRepo；节点 8 激活时由 Story 23.5 owner 决定是否加新字段到 Deps）
- **不**改 `cmd/server/main.go`（router wire 内部消费 Deps）

**AC5 — service 单测 ≥3 case（无 repo stub；新文件 `dev_cosmetic_service_test.go`）**

新建 `server/internal/service/dev_cosmetic_service_test.go`：

**stub 设计**：本 story 节点 7 阶段 stub service **无 repo 依赖** → 单测**不需要 stub repo**，直接构造 `service.NewDevCosmeticService()` 调 GrantCosmeticBatch 验证"return nil + 触发 slog WARN"即可。

**必须覆盖 3 case**（前缀 `TestDevCosmeticService_GrantCosmeticBatch_`，**fix-review r1 锁定 explicit-failure 断言**）：

1. **`TestDevCosmeticService_GrantCosmeticBatch_HappyPathStub_ReturnsServiceBusy_LogsWarn`**：调 `svc.GrantCosmeticBatch(ctx, userID=1001, rarity=1, count=10)` → 返 **`*apperror.AppError` 且 Code == 1009 + Message 含"node-7 stub"或"not yet implemented"**（验 stub explicit-failure 行为）+ 用 `slogtest` 捕获日志验"WARN 含 `phase=node-7-stub` + `todo` 字段 + `user_id` / `rarity` / `count` 透传字段"。
2. **`TestDevCosmeticService_GrantCosmeticBatch_BoundaryCases_AlwaysReturnsServiceBusy`**：表驱动测试，覆盖 rarity ∈ {1,2,3,4} × count ∈ {1,10,100} 共 12 个组合 → 全部返 1009（验 stub 无差别 reject，节点 8 激活前没有合法 happy path）。
3. **`TestDevCosmeticService_GrantCosmeticBatch_StubIgnoresInvalidParams_StillReturnsServiceBusy`**：调 svc 传 rarity=99 / count=0 / userID=0 等"handler 应该拦截但 service 不防御"的参数 → 全部仍返 1009（验"stub 不做 service 层参数防御，所有调用都失败"）。

**关键约束**：

- 3 case 命名前缀 `TestDevCosmeticService_GrantCosmeticBatch_<场景>` 一目了然（与 7.5 / 20.7 同风格）
- **无需 stub repo**（stub service 不调 repo）—— 与 7.5 / 20.7 单测大量 stub repo 不同
- HappyPath case **必须**验 1009 + WARN 日志结构化字段（透传 user_id/rarity/count + phase=node-7-stub + todo）；用 helper `assertServiceBusyError` 统一封装断言（详见 `dev_cosmetic_service_test.go` 实装）
- BoundaryCases 表驱动避免 12 个独立 test func；rarity / count 矩阵全覆盖；全部断言 1009
- StubIgnoresInvalidParams 验证"stub 不做 service 层防御性 panic + 仍返 1009"
- **节点 8 激活时** 这 3 case 必须改写：把 1009 断言换成"happy path return nil + repo BatchCreate 被调"语义；本 story 的 `assertServiceBusyError` helper 删除

**AC6 — handler 单测 ≥5 case（stub service + 测试 router；新文件 `dev_cosmetic_handler_test.go`）**

新建 `server/internal/app/http/handler/dev_cosmetic_handler_test.go`：

**stub 设计**（**新建** `stubDevCosmeticService`，与 stubDevStepService / stubDevChestService 平级独立）：

```go
package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/service"
)

type stubDevCosmeticService struct {
	grantCosmeticBatchFn func(ctx context.Context, userID uint64, rarity int8, count int32) error
}

func (s *stubDevCosmeticService) GrantCosmeticBatch(ctx context.Context, userID uint64, rarity int8, count int32) error {
	return s.grantCosmeticBatchFn(ctx, userID, rarity, count)
}

// newDevCosmeticHandlerRouter 构造 handler test router。
//
// **关键差异 vs newChestHandlerRouter**：dev 端点不挂 mock auth middleware（dev 不要求 auth）。
// 仅挂 ErrorMappingMiddleware（c.Error 写 envelope 必需；与 7.5 / 20.7 newXxxHandlerRouter 同模式）。
func newDevCosmeticHandlerRouter(svc service.DevCosmeticService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewDevCosmeticHandler(svc)
	r.POST("/dev/grant-cosmetic-batch", h.PostGrantCosmeticBatch)
	return r
}
```

**必须覆盖 5 case**（前缀 `TestDevCosmeticHandler_PostGrantCosmeticBatch_`，**fix-review r1 锁定 explicit-failure 断言**）：

1. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_HappyPath_ServiceReturnsServiceBusy_Forwards503`**：合法 body `{"userId":1001, "rarity":1, "count":10}` → handler 把三参数透传到 stub service → stub service 模拟节点 7 阶段实装返 `*AppError(ErrServiceBusy, "...not yet implemented...")` → HTTP **503** + envelope.code=**1009** + message 含"node-7 stub"或"not yet implemented"；stub service 内部 if 参数不等于预期 → t.Errorf 验透传。**节点 8 激活后**本 case 改回 HTTP 200 + envelope.code=0 + data 透传 happy path 语义。
2. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_RarityInvalid_99_Returns1002_NoServiceCall`**：body `{"userId":1001, "rarity":99, "count":10}` → handler 显式校验 99 ∉ [1,4] → 1002 + message="rarity 必须 ∈ [1,4]"；stub service.grantCosmeticBatchFn 内 t.Errorf("should NOT be called")（验证 handler 拦截在 service 之前）
3. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_RarityZero_Returns1002_NoServiceCall`**：body `{"userId":1001, "rarity":0, "count":10}` → handler 显式校验 rarity=0 ∉ [1,4] → 1002（验"0 不被当作合法值放行"）；stub.grantCosmeticBatchFn 内 t.Errorf 兜底
4. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_CountZero_Returns1002_NoServiceCall`**：body `{"userId":1001, "rarity":1, "count":0}` → handler 显式校验 count=0 ∉ [1,100] → 1002 + message="count 必须 ∈ [1,100]"；stub.grantCosmeticBatchFn 内 t.Errorf 兜底
5. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_CountTooLarge_101_Returns1002_NoServiceCall`**：body `{"userId":1001, "rarity":1, "count":101}` → handler 显式校验 count=101 > 100 → 1002（验"上限保护"）；stub.grantCosmeticBatchFn 内 t.Errorf 兜底

**加分 case**（≥7 case 更佳）：

6. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_MissingUserID_Returns1002`**：body `{"rarity":1, "count":10}`（无 userId）→ ShouldBindJSON 后 UserID 仍 nil → handler 校验失败 → 1002 + message="userId 必填"；stub.grantCosmeticBatchFn 内 t.Errorf 兜底
7. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_MissingRarity_Returns1002`**：body `{"userId":1001, "count":10}`（无 rarity）→ Rarity nil → 1002 + message="rarity 必填"
8. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_MissingCount_Returns1002`**：body `{"userId":1001, "rarity":1}`（无 count）→ Count nil → 1002 + message="count 必填"
9. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_InvalidJSON_Returns1002`**：body `{"userId":"abc"}`（类型错）→ ShouldBindJSON 失败 → 1002
10. **`TestDevCosmeticHandler_PostGrantCosmeticBatch_ServiceError_Forwards1009`**：stub service 返 `*apperror.AppError(ErrServiceBusy, "服务繁忙")`（模拟节点 8 激活后 BatchCreate 失败场景）→ middleware envelope code=1009 + HTTP **500**（占位测试，验 handler 转发 service error 路径在节点 8 激活后仍能工作）

**关键约束**：

- 5-10 case 命名前缀 `TestDevCosmeticHandler_PostGrantCosmeticBatch_<场景>` —— 与 7.5 / 20.7 同风格
- HappyPath stub 内 `if userID != 1001 || rarity != 1 || count != 10 { t.Errorf(...) }` 验三参数透传
- 所有"handler 应拦截"case **必须**用 stub.grantCosmeticBatchFn 内 t.Errorf 兜底验"handler 拦在 service 之前" —— 防 future handler 误删 nil/范围校验
- ServiceError case 验 HTTP **500**（不是 200）—— 1009 是唯一走非 200 的业务码（即便节点 7 阶段 stub 永远 nil，这个 case 验"激活后 service 返 1009 能 forward"路径完整）
- 测试 router **不**挂 mock auth middleware —— dev 路径无 auth；与 7.5 / 20.7 dev handler 测试同模式

**AC7 — devtools 框架测试扩展（修改 `internal/app/http/devtools/devtools_test.go`）**

修改 `server/internal/app/http/devtools/devtools_test.go` 末尾追加 2 case + 把既有所有 `devtools.Register(r, X, Y)` 调用改新签名（每加 1 nil 参数）：

```go
// TestRegister_BuildDevTrue_GrantCosmeticBatchRegisteredWhenHandlerProvided 验证 Story 20.8 路由注册：
//   - BUILD_DEV=true + 传入非 nil DevCosmeticHandler → /dev/grant-cosmetic-batch 路由存在
//   - 验路由存在的方式：ServeHTTP 应该走到 handler 而非 NoRoute；用 stub handler 标志位验证
func TestRegister_BuildDevTrue_GrantCosmeticBatchRegisteredWhenHandlerProvided(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")

	called := false
	stubHandler := devCosmeticHandlerFunc(func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r := newEngine()
	r.Use(middleware.ErrorMappingMiddleware())
	devtools.Register(r, nil /* devSteps */, nil /* devChest */, stubHandler /* devCosmetic */)

	w := doPost(r, "/dev/grant-cosmetic-batch", `{"userId":1, "rarity":1, "count":10}`)

	if w.Code != http.StatusOK {
		t.Errorf("/dev/grant-cosmetic-batch should be 200 when handler registered; got %d body=%s", w.Code, w.Body.String())
	}
	if !called {
		t.Errorf("handler should be called; got called=false (路由未注册或被 NoRoute 拦截)")
	}
}

// TestRegister_BuildDevTrue_GrantCosmeticBatchSkippedWhenHandlerNil 验证 nil-tolerant：
//   - BUILD_DEV=true + 传入 nil → /dev/ping-dev 仍注册，/dev/grant-cosmetic-batch 跳过 → 404
func TestRegister_BuildDevTrue_GrantCosmeticBatchSkippedWhenHandlerNil(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")

	r := newEngine()
	devtools.Register(r, nil, nil, nil) // 三个 handler 都 nil

	// ping-dev 仍应注册
	w1 := doGet(r, "/dev/ping-dev")
	if w1.Code != http.StatusOK {
		t.Errorf("/dev/ping-dev should be 200; got %d", w1.Code)
	}

	// grant-cosmetic-batch 应跳过注册 → Gin NoRoute 404
	w2 := doPost(r, "/dev/grant-cosmetic-batch", `{}`)
	if w2.Code != http.StatusNotFound {
		t.Errorf("/dev/grant-cosmetic-batch with nil handler should be 404; got %d", w2.Code)
	}
}
```

**辅助 helper**（追加到 devtools_test.go 已有 helper 段，与 7.5 `DevStepsHandlerFunc` / 20.7 `devChestHandlerFunc` adapter 平级）：

```go
// devCosmeticHandlerFunc 是 devtools.DevCosmeticHandler interface 的函数适配器（仅供测试用）。
//
// 实际生产 handler 是 *handler.DevCosmeticHandler（struct）；测试中用 func 包装更简洁。
type devCosmeticHandlerFunc func(c *gin.Context)

func (f devCosmeticHandlerFunc) PostGrantCosmeticBatch(c *gin.Context) { f(c) }
```

**关键改动清单**（既有 case 适配新签名）：

| 既有 case | 既有调用 | 新调用 |
|---|---|---|
| Story 1.6 所有 case | `devtools.Register(r, nil, nil)` | `devtools.Register(r, nil, nil, nil)` |
| Story 7.5 `GrantStepsRegisteredWhenHandlerProvided` | `devtools.Register(r, stubHandler, nil)` | `devtools.Register(r, stubHandler, nil, nil)` |
| Story 7.5 `GrantStepsSkippedWhenHandlerNil` | `devtools.Register(r, nil, nil)` | `devtools.Register(r, nil, nil, nil)` |
| Story 20.7 `ForceUnlockChestRegisteredWhenHandlerProvided` | `devtools.Register(r, nil, stubHandler)` | `devtools.Register(r, nil, stubHandler, nil)` |
| Story 20.7 `ForceUnlockChestSkippedWhenHandlerNil` | `devtools.Register(r, nil, nil)` | `devtools.Register(r, nil, nil, nil)` |

**关键约束**：

- devtools_test.go 验证"路由注册 / 跳过"决策点
- 用最小 stub handler（不引入业务 handler 实例 → 避免 bootstrap import；与 7.5 / 20.7 同模式）
- `devCosmeticHandlerFunc` adapter 只在 devtools_test.go（测试包）暴露 —— **不**进 production code
- 整个 devtools_test.go 文件 build tag 仍 `//go:build !devtools`（与 1.6 / 7.5 / 20.7 既有约束）
- **既有 case 必须全部改新签名**（每个调用补 nil 参数；编译期错误兜底，不漏改 —— 与 20.7 已 lesson 同模式）

**AC8 — bootstrap 框架测试扩展（修改 `internal/app/bootstrap/router_dev_test.go`）**

修改 `server/internal/app/bootstrap/router_dev_test.go` 末尾追加 1 case：

```go
// TestRouter_DevGrantCosmeticBatch_NilHandlerSkipsRoute 验证 Story 20.8 dev grant-cosmetic-batch
// 端点的 wire 链路：
//   - BUILD_DEV=true + Deps{} 零值 → devCosmeticHandler 保持 nil → devtools.Register 跳过路由 → 404
//
// 与 TestRouter_DevGrantSteps_NilHandlerSkipsRoute (7.5) / TestRouter_DevForceUnlockChest_NilHandlerSkipsRoute
//（20.7）同模式：真实 wire 链路（dev handler 真被调）由 dev_cosmetic_handler_test 单测覆盖；
// 本测试仅验证"nil-tolerant"路径。
func TestRouter_DevGrantCosmeticBatch_NilHandlerSkipsRoute(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")
	gin.SetMode(gin.TestMode)
	r := NewRouter(Deps{}) // 零值 deps → devCosmeticHandler 保持 nil

	// /dev/ping-dev 应该正常注册
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dev/ping-dev", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/dev/ping-dev should be 200 when BUILD_DEV=true; got %d", w.Code)
	}

	// /dev/grant-cosmetic-batch 应该跳过注册（devCosmeticHandler nil）→ Gin NoRoute 404
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/dev/grant-cosmetic-batch", strings.NewReader(`{"userId":1, "rarity":1, "count":10}`)))
	if w2.Code != http.StatusNotFound {
		t.Errorf("/dev/grant-cosmetic-batch with nil handler should be 404; got %d body=%s", w2.Code, w2.Body.String())
	}
}
```

**关键约束**：

- router_dev_test.go 验证"nil-tolerant + 全 wire 链路"
- 与 20.7 `TestRouter_DevForceUnlockChest_NilHandlerSkipsRoute` 平级独立 case
- 整个 router_dev_test.go 文件 build tag 仍 `//go:build !devtools`（与 1.6 / 7.5 / 20.7 既有约束）

**AC9 — `bash scripts/build.sh` 全量绿**

完成后必须能跑通：

```bash
bash scripts/build.sh                      # vet + build → no failures
bash scripts/build.sh --test               # 全单测过（service 3 + handler 5-10 + devtools 2 + bootstrap 1 = 11-16 新 case + 既有全过）
bash scripts/build.sh --race --test        # CI Linux 必过；Windows race skip 不阻塞
bash scripts/build.sh --integration        # 既有集成（含 20.7 dev_chest 2 case）全过；本 story **不**新增集成测试
bash scripts/build.sh --devtools           # 验证 build tag 路径不出错（构造 build/catserver-dev[.exe]）—— Register 新签名兼容
```

**关键约束**：

- 本 story 引入的新代码全部含单测；`go test ./internal/service/... -run TestDevCosmeticService -v` 必须 3 个 case 全过
- `go test ./internal/app/http/handler/... -run TestDevCosmeticHandler -v` 必须 5-10 个 case 全过
- `go test ./internal/app/http/devtools/... -run TestRegister_BuildDevTrue_GrantCosmeticBatch -v` 必须 2 个 case 全过
- `go test ./internal/app/bootstrap/... -run TestRouter_DevGrantCosmeticBatch -v` 必须 1 个 case 全过
- `--race` 在 Windows skip（ADR-0001 §3.5）；CI Linux 跑
- `--devtools` 必须能通过（forceDevEnabled=true 路径不破 + 不引入 build error；本 story 改 Register 签名后必须验证）
- **不**改 `scripts/build.sh` 自身

**AC10 — 验证清单（人工 + 自动化）**

完成后**人工**核对以下 10 项（结果记到 Completion Notes List）：

| # | 验证项 | 验证方式 |
|---|---|---|
| 1 | service 层 `GrantCosmeticBatch` 是 **stub** —— 仅 `slog.WarnContext(ctx, "...STUB...phase=node-7-stub...todo=...23.5...")` + return nil；**无** repo 调用 | Read `dev_cosmetic_service.go` GrantCosmeticBatch 实装段 |
| 2 | service 接口 doc 注释明确"节点 7 vs 节点 8 阶段切片策略"（含节点 8 激活路径示例代码） | Read `dev_cosmetic_service.go` 顶部 interface doc |
| 3 | handler `PostGrantCosmeticBatchRequest` 用 *uint64 / *int8 / *int32 指针；userId / rarity / count 缺失或非法值显式 1002 拦截 | Read `dev_cosmetic_handler.go` PostGrantCosmeticBatch |
| 4 | handler 校验 rarity ∈ [1,4]、count ∈ [1,100]；**不**调 `c.Get(UserIDKey)`（dev 路径无 auth 中间件） | Read `dev_cosmetic_handler.go` |
| 5 | devtools.Register 签名扩 `(r, devStepsHandler, devChestHandler, devCosmeticHandler)`；nil 时跳过 grant-cosmetic-batch 路由（保留 ping-dev + grant-steps + force-unlock-chest） | Read `devtools.go` Register 实装 |
| 6 | router.go wire `var devCosmeticHandler` 提前声明 nil + if 块内构造（**节点 7 阶段 `NewDevCosmeticService()` 无参数**）+ Register 留 if 块外（nil-collapse 三参数写法） | Read `router.go` diff |
| 7 | **未新建** `migrations/0015_init_user_cosmetic_items.up.sql` / **未新建** `user_cosmetic_item_repo.go` / **未改** `cosmetic_item_repo.go`（验证选项 B 越权红线） | `git status --short` 检查无 migrations 改动 + 无 user_cosmetic_item_repo.go 文件 + cosmetic_item_repo.go 无 diff |
| 8 | devtools_test.go 既有 case 全部改 3 参数新签名（每个 `devtools.Register(r, X, Y)` 加 `, nil` 第 3 参数）；编译通过 | Read devtools_test.go diff + `go test ./internal/app/http/devtools/...` 全绿 |
| 9 | 既有所有 dev endpoint test（7.5 grant-steps / 20.7 force-unlock-chest）仍全过；既有 chest / step / auth / room 等 test 不受影响 | `bash scripts/build.sh --test` 全绿；`git diff --stat` 改动文件清单匹配预期范围（5 新 + 3 改） |
| 10 | `bash scripts/build.sh --test` 全绿；`bash scripts/build.sh --devtools` 不破；`bash scripts/build.sh --integration` 既有集成全过（本 story 不新增集成测试 → 不阻塞） | bash 实跑 + git status |

**AC11 — 不 commit（流水线由 epic-loop 下游收口）**

epics.md §Story 20.8 AC 钦定"≥3 单测"，**不**钦定"git commit 单独提交"。本 story 是 server 业务代码 story，commit 由 epic-loop 流水线在下游 fix-review / story-done sub-agent 阶段统一收口。

- 本 story 的 dev workflow **不** commit / **不** push
- commit message 模板（story-done 阶段使用）：

  ```text
  feat(dev-grant-cosmetic-batch): POST /dev/grant-cosmetic-batch dev 端点节点 7 stub（Story 20.8）

  - service dev_cosmetic_service.GrantCosmeticBatch 节点 7 阶段 stub 实装：
    仅 slog.WarnContext + return nil；接口签名 final，节点 8 / Story 23.5 落地后
    在本 service 内激活真实写库逻辑（cosmetic_item_repo.FindRandomByRarity +
    user_cosmetic_item_repo.BatchCreate）
  - handler dev_cosmetic_handler.PostGrantCosmeticBatch + PostGrantCosmeticBatchRequest
    （userId / rarity / count 指针类型；rarity ∈ [1,4]、count ∈ [1,100] 显式 1002）
    + postGrantCosmeticBatchResponseDTO 简单 ack
  - devtools.Register 签名扩 (r, devStepsHandler, devChestHandler, devCosmeticHandler)；
    nil-tolerant 跳过 grant-cosmetic-batch 路由；devtools_test.go 既有 case 全部改 3 参数新签名
  - bootstrap/router.go wire devCosmeticSvc + devCosmeticHandler；nil-collapse 三参数 Register
  - 单测 11-14 case（service 3 + handler 5-10 + devtools 2 + bootstrap 1）
  - **不**新建 migration / **不**新建 user_cosmetic_item_repo（侵入 Epic 23.2 / 23.5 scope）
  - 集成测试 deferred to Story 23.5 阶段（user_cosmetic_items 表节点 8 才建）

  依据 epics.md §Story 20.8 + Story 20.7 dev 端点扩展模式 + 节点 7 vs 节点 8 阶段切片策略（选项 C）。

  Story: 20-8-dev-端点-post-dev-grant-cosmetic-batch
  ```

- commit hash 待 story-done 阶段产生后回填到本文件

## Tasks / Subtasks

- [x] **Task 1（AC1）**：新建 `internal/service/dev_cosmetic_service.go` —— DevCosmeticService interface + 节点 7 阶段 stub impl
  - [x] 1.1 Read `dev_chest_service.go` (20.7) 完整文件理解 service 模式（doc 注释 / 错误翻译 / slog.WarnContext / 节点切片策略类比）
  - [x] 1.2 Read `dev_step_service.go` (7.5) 复习 service 模式（用以对比 7.5 真实写库与本 story stub 的差异）
  - [x] 1.3 Write 新文件 `dev_cosmetic_service.go` —— DevCosmeticService interface + devCosmeticServiceImpl + NewDevCosmeticService 无参数 + GrantCosmeticBatch stub 实装（slog.WarnContext + return nil）
  - [x] 1.4 Read 回检：(1) **无** repo 依赖（stub）；(2) WARN 日志含 `phase="node-7-stub"` + `todo` 字段；(3) interface doc 明确"节点 7 vs 节点 8 阶段切片策略"含激活路径示例；(4) GrantCosmeticBatch 永远 return nil

- [x] **Task 2（AC2）**：新建 `internal/app/http/handler/dev_cosmetic_handler.go` —— DevCosmeticHandler + DTO + handler
  - [x] 2.1 Read `dev_chest_handler.go` (20.7) 完整文件复习 handler 模式（指针类型 + 手动 nil 校验 + c.Error + ctx 传播）
  - [x] 2.2 Write 新文件 `dev_cosmetic_handler.go` —— DevCosmeticHandler + NewDevCosmeticHandler + PostGrantCosmeticBatchRequest（3 个指针字段）+ PostGrantCosmeticBatch + postGrantCosmeticBatchResponseDTO
  - [x] 2.3 Read 回检：(1) UserID/Rarity/Count 指针类型（无 binding:"required"）；(2) userId/rarity/count nil 显式 1002；(3) rarity ∈ [1,4] / count ∈ [1,100] 显式 1002；(4) 不调 c.Get(UserIDKey)；(5) c.Error + return；(6) response 返简单 ack `{userId, rarity, count}`

- [x] **Task 3（AC3）**：扩 `internal/app/http/devtools/devtools.go` —— DevCosmeticHandler interface + Register 签名扩
  - [x] 3.1 Read `devtools.go` (Story 20.7 后) 完整文件理解现有 Register / DevStepsHandler / DevChestHandler interface
  - [x] 3.2 Edit 在 devtools.go 加 `DevCosmeticHandler interface { PostGrantCosmeticBatch(c *gin.Context) }` 类型（位置：紧邻 DevChestHandler interface 之后）
  - [x] 3.3 Edit 改 `Register` 签名为 `Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler, devCosmeticHandler DevCosmeticHandler)`，在 `if devChestHandler != nil { g.POST("/force-unlock-chest", ...) }` 之后追加 `if devCosmeticHandler != nil { g.POST("/grant-cosmetic-batch", devCosmeticHandler.PostGrantCosmeticBatch) }`
  - [x] 3.4 Read 回检：(1) DevCosmeticHandler interface 解耦避免 import cycle；(2) nil-tolerant 跳过 grant-cosmetic-batch；(3) ping-dev / grant-steps / force-unlock-chest 仍在；(4) build tag 不影响

- [x] **Task 4（AC4）**：扩 `internal/app/bootstrap/router.go` —— wire dev cosmetic service + handler + Register 签名 + nil-collapse 三参数
  - [x] 4.1 Read 现有 `router.go` 理解 if 块结构 + 20.7 devChestHandler wire 模式
  - [x] 4.2 Edit 在 NewRouter 函数顶部 `var devChestHandler *handler.DevChestHandler` 之后追加 `var devCosmeticHandler *handler.DevCosmeticHandler // Story 20.8`
  - [x] 4.3 Edit 在 `if deps.GormDB != nil && ...` 块内、`devChestHandler` 构造之后追加 `devCosmeticSvc := service.NewDevCosmeticService()`（节点 7 阶段无参数）+ `devCosmeticHandler = handler.NewDevCosmeticHandler(devCosmeticSvc)`
  - [x] 4.4 Edit 改 `devtools.Register` 调用为 nil-collapse 三参数写法（追加 `var cosmeticArg devtools.DevCosmeticHandler; if devCosmeticHandler != nil { cosmeticArg = devCosmeticHandler }; Register(r, stepsArg, chestArg, cosmeticArg)`）
  - [x] 4.5 Read 回检：(1) deps 完整时 devCosmeticHandler 非 nil；(2) deps 零值（单测）时保持 nil；(3) nil-collapse 三参数写法防 typed-nil-interface 陷阱；(4) Deps struct **未改**；(5) NewDevCosmeticService() 无参数 stub 模式（节点 8 激活时改签名加 repo 依赖）

- [x] **Task 5（AC5）**：新建 `internal/service/dev_cosmetic_service_test.go` —— ≥3 case
  - [x] 5.1 Read `dev_chest_service_test.go` (20.7) 复习 stub 测试模式（但本 story 无 repo stub）
  - [x] 5.2 Write 新文件 `dev_cosmetic_service_test.go` —— 3 case（HappyPathStub_ReturnsNil_LogsWarn / BoundaryCases_AlwaysReturnsNil 表驱动 / StubIgnoresInvalidParams_ReturnsNilForRarity99）
  - [x] 5.3 Read 回检：(1) **无 stub repo**（service stub 不调 repo）；(2) HappyPath 验 return nil + slog WARN 触发（slog 断言可用 string contains 简化）；(3) 表驱动 12 组合全 return nil；(4) StubIgnoresInvalidParams 验"service 不做参数防御性 panic"

- [x] **Task 6（AC6）**：新建 `internal/app/http/handler/dev_cosmetic_handler_test.go` —— ≥5 case
  - [x] 6.1 Read `dev_chest_handler_test.go` (20.7) 复习 stubDevChestService / newDevChestHandlerRouter 模式
  - [x] 6.2 Write 新文件 `dev_cosmetic_handler_test.go` —— stubDevCosmeticService + newDevCosmeticHandlerRouter（**不**挂 mock auth）+ 5-10 case（HappyPath / RarityInvalid_99 / RarityZero / CountZero / CountTooLarge_101 / MissingUserID / MissingRarity / MissingCount / InvalidJSON / ServiceError）
  - [x] 6.3 Read 回检：(1) HappyPath stub 内验三参数透传；(2) 所有 1002 case stub.grantCosmeticBatchFn 主动 t.Errorf 兜底；(3) ServiceError → HTTP 500 + envelope 1009

- [x] **Task 7（AC7）**：扩 `internal/app/http/devtools/devtools_test.go`
  - [x] 7.1 Read `devtools_test.go` (Story 20.7 后) 复习 newEngine / doGet / doPost helper + 7.5 / 20.7 既有 case 调用 `devtools.Register(r, ...)` 的位置
  - [x] 7.2 Edit `devtools_test.go` —— 末尾追加 `devCosmeticHandlerFunc` adapter + 2 case（GrantCosmeticBatchRegisteredWhenHandlerProvided / GrantCosmeticBatchSkippedWhenHandlerNil）
  - [x] 7.3 Edit `devtools_test.go` —— 把既有所有 `devtools.Register(r, X, Y)` 调用改为 `devtools.Register(r, X, Y, nil)`（编译期错误兜底，不漏改）—— 包括 1.6 / 7.5 / 20.7 全部 case
  - [x] 7.4 Read 回检：(1) devCosmeticHandlerFunc adapter 与 7.5 DevStepsHandlerFunc / 20.7 devChestHandlerFunc 平级；(2) 2 个新 case 用 nil + nil + stubHandler 三参数；(3) 既有 case 全部加 ", nil" 第 3 参数

- [x] **Task 8（AC8）**：扩 `internal/app/bootstrap/router_dev_test.go` —— 1 case
  - [x] 8.1 Read `router_dev_test.go` (Story 20.7 后) 复习 BUILD_DEV setenv + ServeHTTP 模式
  - [x] 8.2 Edit 末尾追加 `TestRouter_DevGrantCosmeticBatch_NilHandlerSkipsRoute`（NewRouter(Deps{}) → grant-cosmetic-batch 404；ping-dev 200）
  - [x] 8.3 Read 回检：(1) BUILD_DEV setenv 必须 "true"（严格字面）；(2) NewRouter(Deps{}) 零值 → devCosmeticHandler nil；(3) /dev/ping-dev 200（验框架仍工作）+ /dev/grant-cosmetic-batch 404（验跳过）

- [x] **Task 9（AC10）**：本 story 不写集成测试
  - [x] 9.1 验证 epics.md §Story 20.8 行 2970 钦定"集成测试覆盖（节点 8 完成后跑）"—— 本 story 阶段 user_cosmetic_items 表未建，dockertest migrate 跑不通
  - [x] 9.2 在 Completion Notes 列明确标注"集成测试 deferred to Story 23.5 阶段（user_cosmetic_items 表节点 8 才建）"
  - [x] 9.3 验证 `bash scripts/build.sh --integration` 既有集成（含 20.7 dev_chest 2 case）全过 / 既有 chest_open 集成全过 / 不破

- [x] **Task 10（AC9 / AC10）**：全量验证
  - [x] 10.1 `bash scripts/build.sh`（vet + build）过 → BUILD SUCCESS
  - [x] 10.2 `bash scripts/build.sh --test` 全绿（新增 11-16 case：service 3 + handler 5-10 + devtools 2 + bootstrap 1）
  - [x] 10.3 `bash scripts/build.sh --race --test`（Windows race skip ok；本地未跑，CI Linux 跑）—— 不阻塞；CI 兜底
  - [x] 10.4 `bash scripts/build.sh --integration`（既有集成全过；本 story 不新增）
  - [x] 10.5 `bash scripts/build.sh --devtools`（验证 build tag 路径不破 + Register 三参数新签名兼容）
  - [x] 10.6 `git status --short` 改动文件清单核对（实际新 4 文件 + 改 3-4 文件 + sprint-status + story 文件 = 9+ 文件；**不**新增 migrations / **不**新增 user_cosmetic_item_repo.go / **不**改 cosmetic_item_repo.go）
  - [x] 10.7 在下方 Completion Notes List 勾选 AC10 验证清单 10 项

- [x] **Task 11（AC11）**：本 story 不做 git commit
  - [x] 11.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 11.2 commit message 模板保留在 story 文件中
  - [x] 11.3 commit hash 待 story-done 阶段回填 —— story-done 阶段执行

## Dev Notes

### 关键设计原则

1. **节点 7 阶段 stub 策略（选项 C）vs 选项 B 越权**：epics.md §20.8 行 2964 钦定"节点 7 阶段实装策略：路由注册 + handler 框架 + 单测 mock 都完成；handler 内部真实写库逻辑等 Story 23.2 user_cosmetic_items 表 migration 完成后开放"。选项 B（在本 story 内同时落地 user_cosmetic_items migration + repo + 真实写库）会侵入 Epic 23.2 + 23.5 scope，反 BMAD"节点顺序不可乱跳"原则。本 story 严格选项 C：**只做端点骨架** + service 内部 stub return nil + WARN 日志标注未来激活路径，让节点 8 Story 23.5 owner 只需在本 service 内激活"真实写库分支"即可（接口签名 / 路由 / 客户端调用代码不变 → 兼容已部署的 e2e 脚本）。

2. **dev 端点独立 service / handler 文件**（不复用 ChestService.OpenChest / DevChestService.ForceUnlockChest / DevStepService.GrantSteps）：dev grant cosmetic 的产品语义是"按品质随机抽 + 批量发放装扮"，与开箱抽奖（消耗步数 + 单次抽取）/ 强制解锁宝箱（动 unlock_at）/ 直接补步数（写 step_account + sync_log）业务完全独立。独立 service 是"按职责而非按表分服务"的合理切分（与 NewDevStepService / NewDevChestService 同思路）。

3. **stub service 无 repo 依赖**：节点 7 阶段 stub 不调任何 repo（无写库 / 无读库），所以 `NewDevCosmeticService()` 无参数。节点 8 激活时由 Story 23.5 owner 改 constructor 签名加 cosmeticItemRepo + userCosmeticItemRepo 依赖。**关键**：接口签名 / handler 调用 / route 路径**不变** —— 兼容已部署的 e2e 脚本。

4. **错误码用 1002 拦截，service stub 永远 return nil**：handler 已校验 userId/rarity/count；service stub 不做参数防御性 panic（与 7.5 dev grant 的 steps<0 panic 模式不同 —— 因为 7.5 真实写库需要兜底防御；本 story stub 没有真实下游，没必要 panic）。节点 8 激活后由 23.5 owner 决定是否加防御性 panic。

5. **devtools.Register 签名扩展模式**（Story 1.6 → 7.5 → 20.7 → 20.8 evolve 链）：
   - 1.6：`Register(r)` 单参数（仅 ping-dev 框架自带）
   - 7.5：`Register(r, devStepsHandler DevStepsHandler)` 加 grant-steps 业务 handler
   - 20.7：`Register(r, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)` 加 force-unlock-chest
   - **20.8（本 story）**：`Register(r, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler, devCosmeticHandler DevCosmeticHandler)` 加 grant-cosmetic-batch
   - 每加一个业务 dev 端点都是"独立 interface + Register 签名追加一个槽位"模式。这让"哪些 dev 端点存在"在 Register 签名层就可见，避免运行时神秘失踪 + 每业务模块独立维护边界。

6. **router.go nil-collapse 写法 evolve**（7.5 → 20.7 → 20.8）：
   - 7.5：4 分支显式 if（`if devStepsHandler == nil { Register(r, nil) } else { Register(r, devStepsHandler) }`）
   - 20.7：改用 nil-collapse 防 4 分支爆炸（`var stepsArg / chestArg + Register(r, stepsArg, chestArg)`）
   - **20.8**：三参数 nil-collapse（`var stepsArg / chestArg / cosmeticArg + Register(r, stepsArg, chestArg, cosmeticArg)`）—— 每加 1 个 handler 只需 +2 行，无指数爆炸
   - **关键 Go 知识**：`var stepsArg devtools.DevStepsHandler` 声明零值是真正的 nil interface（type=nil, value=nil），不是 typed-nil 陷阱场景

7. **interface 解耦避免 import cycle**：devtools 包**不**反向 import handler 包（handler import service / repo / middleware；devtools 是基础设施层在 handler 之下）→ 用 interface 解耦（DevCosmeticHandler interface 在 devtools 包内定义，handler 包内 `*handler.DevCosmeticHandler` 自动实现）。

8. **handler 用指针 + 手动校验**（与 7.5 / 20.7 同模式）：UserID / Rarity / Count 全用指针类型 + 手动 nil 校验，**不**挂 `binding:"required"` —— validator/v10 把 0 视为 zero value 会误判 required（Go validator/v10 著名陷阱）。0 在三个字段上都是非法值（userId 从 1 起、rarity ∈ [1,4]、count ∈ [1,100]），显式拒。

9. **count 上限 100 防 demo 误传砸 DB**：节点 11 合成 demo 凑 10 件 common 用 count=10 / 12 即可；上限 100 足够覆盖所有合理 demo 场景，远低于 user_cosmetic_items 单 INSERT 1000 行的 GORM 默认 batch_size。防"误传 count=1e6 砸 DB"。

10. **集成测试 deferred to Story 23.5**：epics.md §20.8 行 2970 钦定"集成测试覆盖（节点 8 完成后跑）"—— 本 story 阶段 user_cosmetic_items 表未建，dockertest migrate 跑不通；本来就在节点 8 sprint 内由 Story 23.5 落地真实写库时一起加 `dev_cosmetic_service_integration_test.go`。本 story 不写集成测试不是偷工，是与 epics AC 钦定一致 + 表未建时写集成测试无价值。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md`）：

- cosmetic 是节点 7 开始引入的核心资产（与步数账户 / 宝箱并列）；dev grant-cosmetic-batch 是"测试 / demo 凑齐数量"基础设施 —— 与业务接口（GET /cosmetics/catalog / inventory 节点 8）解耦但共享同一 cosmetic_items 池 / user_cosmetic_items 表（节点 8 激活后）
- "状态以 server 为准"原则：dev grant-cosmetic-batch 后客户端调 GET /cosmetics/inventory（节点 8）拿权威实例列表

**数据库层**（`docs/宠物互动App_数据库设计.md`）：

- §5.8 cosmetic_items：本 story stub **不**消费；节点 8 激活后由 23.5 owner 加 FindRandomByRarity 方法消费
- §5.9 user_cosmetic_items：节点 8 Story 23.2 才落地；本 story stub **不**消费
- §6.9 rarity 枚举：1=common / 2=rare / 3=epic / 4=legendary —— handler 校验 ∈ [1,4]
- §6.10 user_cosmetic_items.status：节点 8 激活后由 23.5 owner 决定 INSERT 时 status=1 (in_bag) 还是其他
- §6.11 user_cosmetic_items.source：节点 8 激活后 source=2 (admin_grant) 钦定（与 step_sync_logs.source=2 同语义；dev 端点统一 admin_grant）

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：

- V1 §1 节点 7 冻结声明：本 story 是 dev 端点，**不**进 V1 主清单（V1 §7 当前 V1 接口清单不收录；与 Story 7.5 / 20.7 同政策）
- 错误码：1002（参数错误）/ 1003（资源不存在，节点 8 激活后）/ 1009（服务繁忙，节点 8 激活后）—— 全沿用 V1 §3 通用错误码

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：

- §5.1 handler 层：本 story DevCosmeticHandler 严格按 handler 职责（参数校验 + DTO 转换 + 不接触 *gorm.DB）
- §5.2 service 层：本 story DevCosmeticService 严格按 service 职责（节点 7 阶段 stub 是"通过 service 层占位让 architecture 顺畅"，节点 8 激活后承担真实业务规则）
- §5.3 repo 层：本 story **不**改任何 repo（节点 7 阶段 stub 不读不写库）；节点 8 由 23.5 owner 扩 cosmetic_item_repo + 新建 user_cosmetic_item_repo

**ADR 对齐**：

- ADR-0006 三层错误映射：repo 返哨兵 → service 翻译为 *AppError → handler c.Error + middleware envelope（节点 7 阶段 stub 不触发；节点 8 激活后启用）
- ADR-0007 ctx 传播：service / repo 第一参数 ctx；handler 用 `c.Request.Context()` 不直接传 *gin.Context
- ADR-0006 单一 envelope 生产者：handler 一律 `c.Error + return`，由 ErrorMappingMiddleware 写 envelope

### 关于 Story 20.8 与 7.5 / 20.7 的关键差异

| 维度 | 7.5 POST /dev/grant-steps | 20.7 POST /dev/force-unlock-chest | 20.8 POST /dev/grant-cosmetic-batch（**本 story**） |
|------|--------------------------|-----------------------------------|--------------------------------------------------|
| 路由前缀 | /dev | /dev | /dev |
| HTTP method | POST | POST | POST |
| body 字段 | userId, steps | userId, chestId | userId, rarity, count |
| auth 中间件 | **否** | **否** | **否** |
| rate_limit 中间件 | **否** | **否** | **否** |
| 事务 | 有（4 步） | 有（r4 [P2] 改造后 2 步）| **无**（节点 7 stub）；节点 8 激活后由 23.5 owner 决定 |
| repo 调用 | userRepo.FindByID + stepAccountRepo.FindByUserID + stepAccountRepo.UpdateBalance + stepSyncLogRepo.Create | chestRepo.FindByIDForUpdate + chestRepo.UpdateUnlockAtByID | **无**（节点 7 stub）；节点 8 激活后 cosmeticItemRepo.FindRandomByRarity + userCosmeticItemRepo.BatchCreate |
| 错误码全集 | 1002 / 1003 / 1009 | 1002 / 1003 / 1009 | 1002（节点 7 全部 case）；1009（节点 8 激活后） |
| 端点物理可达性 | 仅 BUILD_DEV=true OR -tags devtools | 仅 BUILD_DEV=true OR -tags devtools | 仅 BUILD_DEV=true OR -tags devtools |
| 节点阶段切片 | n/a | n/a | **节点 7 stub / 节点 8 激活**（选项 C） |
| 单元测试规模 | service 6 + handler 7 + devtools 2 + bootstrap 1 = 16 | service 3 + handler 6 + repo 2 + devtools 2 + bootstrap 1 = 14 | service 3 + handler 5-10 + devtools 2 + bootstrap 1 = 11-16 |
| 改动文件数 | 5 新 + 3-4 改 = 8-9 | 5 新 + 4-5 改 = 9-10 | 4 新 + 3 改 = 7-8 |
| router.go 签名扩 | Register(r, devStepsHandler) | Register(r, devStepsHandler, **devChestHandler**) | Register(r, devStepsHandler, devChestHandler, **devCosmeticHandler**) |
| 集成测试 | 1 case（dockertest） | 2 case（dockertest） | **0 case（deferred to Story 23.5）** |
| 是否依赖未建表 | 否 | 否 | **是（user_cosmetic_items 节点 8 才建）** |

### Project Structure Notes

**与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 钦定的 server/ 工程结构对齐**：

```
server/
├─ internal/
│  ├─ app/
│  │  ├─ http/
│  │  │  ├─ handler/
│  │  │  │  ├─ chest_handler.go              # 20.5 / 20.6 已建；本 story 不动
│  │  │  │  ├─ dev_steps_handler.go          # 7.5 已建；本 story 不动
│  │  │  │  ├─ dev_chest_handler.go          # 20.7 已建；本 story 不动
│  │  │  │  ├─ dev_cosmetic_handler.go       # 本 story 新建
│  │  │  │  └─ dev_cosmetic_handler_test.go  # 本 story 新建
│  │  │  ├─ devtools/
│  │  │  │  ├─ devtools.go                   # 1.6 / 7.5 / 20.7 已建；本 story 改 Register 签名 + 加 DevCosmeticHandler interface + 加 grant-cosmetic-batch 路由
│  │  │  │  └─ devtools_test.go              # 1.6 / 7.5 / 20.7 已建；本 story 末尾追加 2 case + devCosmeticHandlerFunc adapter + 既有 case 改 3 参数新签名
│  │  │  └─ middleware/                      # 已实装；本 story 不调（dev 路径无 auth / rate_limit）
│  │  └─ bootstrap/
│  │     ├─ router.go                        # 7.5 / 20.5-20.7 已 wire；本 story 加 devCosmeticSvc / devCosmeticHandler / nil-collapse 三参数 Register
│  │     └─ router_dev_test.go               # 1.6 / 7.5 / 20.7 已建；本 story 末尾追加 1 case
│  ├─ service/
│  │  ├─ chest_service.go                    # 20.5 已建；本 story 不动
│  │  ├─ chest_open_service.go               # 20.6 已建；本 story 不动
│  │  ├─ dev_step_service.go                 # 7.5 已建；本 story 不动
│  │  ├─ dev_chest_service.go                # 20.7 已建；本 story 不动
│  │  ├─ dev_cosmetic_service.go             # 本 story 新建（节点 7 阶段 stub）
│  │  └─ dev_cosmetic_service_test.go        # 本 story 新建
│  ├─ repo/
│  │  └─ mysql/
│  │     ├─ cosmetic_item_repo.go            # 20.6 已建（仅 ListEnabledForWeightedPick）；本 story **不动**
│  │     └─ chest_repo.go                    # 20.7 后已含 UpdateUnlockAt/UpdateUnlockAtByID/FindByIDForUpdate；本 story **不动**
│  └─ pkg/
│     └─ errors/codes.go                     # 1002 / 1003 / 1009 已注册；本 story 仅**消费**
└─ migrations/                                # 0001-0014 已锁定；本 story **不**改 / **不**新建 0015
```

**变更范围（预期 git status 文件清单，4 新 + 3 改）**：

新建 4 文件：
1. `server/internal/service/dev_cosmetic_service.go`
2. `server/internal/service/dev_cosmetic_service_test.go`
3. `server/internal/app/http/handler/dev_cosmetic_handler.go`
4. `server/internal/app/http/handler/dev_cosmetic_handler_test.go`

修改 3 文件：
5. `server/internal/app/http/devtools/devtools.go` — Register 签名扩 + DevCosmeticHandler interface + g.POST 路由注册
6. `server/internal/app/http/devtools/devtools_test.go` — 末尾追加 2 case + devCosmeticHandlerFunc adapter + 既有 case 改 3 参数新签名
7. `server/internal/app/bootstrap/router.go` — devCosmeticSvc / devCosmeticHandler wire + var 提前声明 + nil-collapse 三参数 Register
8. `server/internal/app/bootstrap/router_dev_test.go` — 末尾追加 1 case

流程文件：
9. `_bmad-output/implementation-artifacts/sprint-status.yaml` — 20-8 状态 backlog → ready-for-dev → in-progress → review → done
10. `_bmad-output/implementation-artifacts/20-8-dev-端点-post-dev-grant-cosmetic-batch.md` — 本 story 文件本身

**未变更文件（明确 NOT touch；超出范围 → HALT）**：

- `server/migrations/*.sql`（**严禁**新建 0015_init_user_cosmetic_items —— Story 23.2 owner）
- `server/internal/repo/mysql/user_cosmetic_item_repo.go`（**严禁**新建 —— Story 23.5 owner）
- `server/internal/repo/mysql/cosmetic_item_repo.go`（**严禁**扩 FindRandomByRarity 方法 —— Story 23.5 / 32.4 owner）
- `server/internal/repo/mysql/chest_repo.go` / 任一其他 repo
- `server/internal/service/chest_service.go` / `chest_open_service.go` / `dev_step_service.go` / `dev_chest_service.go` / `step_service.go` / `home_service.go` / `auth_service.go` / 任一其他 service
- `server/internal/service/auth_service_test.go` / `chest_service_test.go` / `chest_open_service_test.go` 等其他既有 test（本 story 不增 stubChestRepo / stubUserRepo / 任一其他 stub 方法，因为 stub service 不调 repo）
- `server/internal/app/http/handler/chest_handler.go` / `dev_steps_handler.go` / `dev_chest_handler.go` / `steps_handler.go` / 任一其他 handler
- `server/internal/app/http/middleware/*.go`
- `server/internal/pkg/errors/codes.go`
- `server/internal/pkg/random/*.go`（20.6 已建；本 story 不消费）
- `server/internal/infra/config/*.go` / 任一 `*.yaml`
- `server/cmd/server/main.go` / `internal/app/bootstrap/server.go`
- `docs/宠物互动App_*.md`（消费方）
- `_bmad-output/implementation-artifacts/decisions/*.md`（无新决策）

### References

**优先级 P0（必读）**：

- [Source: _bmad-output/planning-artifacts/epics.md#Story 20.8] — 本 story 钦定 AC（行 2953-2970）含"节点 7 阶段实装策略：路由注册 + handler 框架 + 单测 mock 都完成；handler 内部真实写库逻辑等 Story 23.2 user_cosmetic_items 表 migration 完成后开放"
- [Source: server/internal/app/http/devtools/devtools.go] — Story 1.6 / 7.5 / 20.7 devtools 框架（Register / DevOnlyMiddleware / DevStepsHandler interface / DevChestHandler interface / nil-tolerant 模式 / "业务 dev 端点扩展模式" 注释）
- [Source: server/internal/service/dev_chest_service.go] — Story 20.7 最新 dev service 模式（doc 注释 / 错误翻译 / slog.WarnContext / interface doc 详尽演进历史；本 story 严格参考 doc 风格但 stub 实装简化）
- [Source: server/internal/app/http/handler/dev_chest_handler.go] — Story 20.7 最新 dev handler 模式（指针类型 + 手动 nil 校验 + 不调 c.Get + c.Error + 简单 ack response；本 story 严格参考）
- [Source: server/internal/service/dev_step_service.go] — Story 7.5 老 dev service 模式（4 步事务 + 多 repo wire；本 story 不复用此模式，因为是 stub）
- [Source: server/internal/app/http/handler/dev_steps_handler.go] — Story 7.5 老 dev handler 模式（与 20.7 同模式但更简单）
- [Source: server/internal/app/bootstrap/router.go] — wire 模式 + 20.7 nil-collapse 两参数 Register（本 story 扩为三参数）
- [Source: _bmad-output/implementation-artifacts/20-7-dev-端点-post-dev-force-unlock-chest.md] — Story 20.7 完整实装文档（devtools.Register 签名扩 + nil-collapse 模式 + interface 解耦 lesson；本 story 复用全部模式 + 扩第三个 handler 槽位）
- [Source: _bmad-output/implementation-artifacts/7-5-dev-端点-post-dev-grant-steps.md] — Story 7.5 完整实装文档（dev 端点基础模式 + 双闸门 + DevOnlyMiddleware + typed-nil-interface 陷阱 lesson）

**优先级 P1（参考）**：

- [Source: server/internal/pkg/errors/codes.go] — 错误码全集（ErrInvalidParam=1002 / ErrResourceNotFound=1003 / ErrServiceBusy=1009）
- [Source: server/internal/service/dev_chest_service_test.go] — Story 20.7 service 单测模式（本 story service stub 单测无 repo stub 更简单）
- [Source: server/internal/app/http/handler/dev_chest_handler_test.go] — Story 20.7 handler 单测模式（stubDevChestService + newDevChestHandlerRouter；本 story 新建独立 stubDevCosmeticService + newDevCosmeticHandlerRouter 同模式）
- [Source: server/internal/app/http/devtools/devtools_test.go] — Story 1.6 / 7.5 / 20.7 devtools 单测（newEngine / doGet / doPost / DevStepsHandlerFunc / devChestHandlerFunc 模式；本 story 末尾加 2 case + devCosmeticHandlerFunc adapter + 既有 case 改 3 参数新签名）
- [Source: server/internal/app/bootstrap/router_dev_test.go] — Story 1.6 / 7.5 / 20.7 dev 路由 wire 测试（BUILD_DEV setenv + ServeHTTP 模式；本 story 末尾加 1 case）
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go] — Story 20.2 / 20.6 已建 CosmeticItem struct + ListEnabledForWeightedPick 方法（本 story stub 不消费；节点 8 Story 23.5 / 32.4 扩 FindRandomByRarity 方法）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-mapping.md] — ADR-0006 三层错误映射
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md] — ADR-0007 ctx 传播

**优先级 P2（背景）**：

- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5] — 分层职责定义
- [Source: docs/宠物互动App_数据库设计.md#5.8 cosmetic_items] — 表结构 + 字段（节点 7 stub 不消费；激活时参考 §5.8）
- [Source: docs/宠物互动App_数据库设计.md#5.9 user_cosmetic_items] — 表结构（节点 8 Story 23.2 才建；本 story stub 不消费；激活时参考 §5.9 INSERT schema）
- [Source: docs/宠物互动App_数据库设计.md#6.9 cosmetic_items.rarity] — 枚举：1=common / 2=rare / 3=epic / 4=legendary（handler 校验 ∈ [1,4]）
- [Source: docs/宠物互动App_数据库设计.md#6.10 user_cosmetic_items.status] — 枚举：节点 8 激活时由 Story 23.5 owner 决定 INSERT status 值
- [Source: docs/宠物互动App_数据库设计.md#6.11 user_cosmetic_items.source] — 枚举：节点 8 激活时 source=2 admin_grant 钦定
- [Source: docs/宠物互动App_总体架构设计.md] — "状态以 server 为准"原则
- [Source: _bmad-output/implementation-artifacts/1-6-dev-tools-框架.md] — Story 1.6 完整实装文档（双闸门 / DevOnlyMiddleware / 业务扩展模式钦定）
- [Source: _bmad-output/implementation-artifacts/20-6-post-chest-open-事务-idempotencykey-加权抽取.md] — Story 20.6 完整实装文档（chest_open 事务模式；本 story 不复用此事务模式但理解上下文）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 23.2] — 节点 8 user_cosmetic_items migration owner（本 story 严禁越权落地）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 23.5] — 节点 8 修改开箱事务 + 真实写入 user_cosmetic_items（**本 story stub 激活的下游 owner**）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 25.1] — 节点 8 跨端 e2e，验证场景 6 调 /dev/grant-cosmetic-batch（**本 story 端点骨架的下游消费方**）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 34.1] — 节点 11 合成 demo 准备步骤调 /dev/grant-cosmetic-batch {rarity:1, count:12}（**本 story 端点骨架的下游消费方**）

### Previous Story Intelligence（Story 1.6 / 7.5 / 20.7 关键交付物）

**Story 1.6 dev tools 框架交付物**：

- **devtools 包定位**：只做"框架"（Register + DevOnlyMiddleware + 启停判定）；业务 dev 端点扩展走"业务层 service / handler + devtools.Register 接 interface"模式
- **双闸门 OR 启用**：build tag `-tags devtools` OR env var `BUILD_DEV=true`；任一即启用
- **DevOnlyMiddleware 是请求时第二闸门**
- **Register 是非幂等**：重复调用让 Gin panic
- **测试 build tag `!devtools`**：所有 devtools 自动化测试必须加 `//go:build !devtools`

**Story 7.5 dev grant-steps 关键模式参考**：

- **dev 端点独立 service / handler 文件**：本 story dev_cosmetic_service.go / dev_cosmetic_handler.go 与 7.5 / 20.7 平级独立
- **devtools.Register 签名扩**：每加一个业务 dev 端点签名追加一个 interface 槽位
- **interface 解耦**：DevCosmeticHandler interface 与 DevStepsHandler / DevChestHandler 平级
- **typed-nil-interface 陷阱 lesson**：7.5 lesson；20.7 evolve 为 nil-collapse；本 story 三参数 nil-collapse

**Story 20.7 dev force-unlock-chest 关键模式参考**（**本 story 最重要的参考**）：

- **dev endpoint 最新模式**：interface doc 详尽演进历史（r0~r4 + race 修复路径 + 选项决策）— 本 story doc 风格严格参考
- **nil-collapse 三参数写法**：本 story 从两参数（20.7）扩为三参数（20.8）
- **devtools_test.go 既有 case 适配**：20.7 已 lesson"改 Register 签名后所有既有 case 编译失败"，本 story 把所有 `Register(r, X, Y)` 改 `Register(r, X, Y, nil)`
- **bootstrap router_dev_test.go 单 case 模式**：本 story 复用此模式新加 1 case

### Lessons Index（与本 story 相关的过去教训）

- [docs/lessons/2026-04-24-error-envelope-single-producer.md] — c.Error / response.Error 二选一；本 story 严格走 `c.Error + return`
- Story 7.5 dev notes #4：validator/v10 把 0 视为 zero value 误判 required；本 story 用指针 + 手动 nil 校验
- Story 7.5 dev notes #11：Go interface typed-nil 陷阱；本 story 用 nil-collapse 三参数写法防御
- Story 20.7 r5 lesson：dev endpoint correctness > contract aesthetics（虽然本 story 节点 7 stub 不涉及契约迭代，但理解此 lesson 有助于评估"节点 8 激活后是否改 response schema"决策）
- Story 1.6 / 7.5 / 20.7 review 教训：本 story 参考但**不**重复触发（dev 端点产品语义 + 双闸门 + interface 解耦 + nil-collapse 都是已 lesson 的成熟模式）

### Git Intelligence（最近 5 个 commit）

```
ebe8762 chore(lessons): backfill 3cd2ef4 commit hash for SDK/runtime mismatch lesson
3cd2ef4 docs(lessons): 沉淀 epic-18 retro A1 修复 — Xcode SDK/sim-runtime 版本错位的根因诊断
6a04d9f chore(epic-18): 收官 Epic 18 retrospective + sprint-status 标记 retrospective done
48acf83 docs(lessons): 补充 SwiftUI PreferenceKey merge vs replace & owner-side expire lesson（18-4 r1）
e747017 chore(story-18-4): 收官 Story 18.4 + 归档 story 文件
```

（git log 显示最近主要是 iOS Epic 18 收官；Epic 20 server 部分在更早的 commit —— 20.7 done 在主线之前；本 story 是 Epic 20 第二条 dev 工具收尾）

**关键提取**：

- 20.7 已 done；devtools.Register 已扩为 `(r, devSteps, devChest)` 两参数；本 story 再扩为三参数
- 7.5 / 20.7 全 done；本 story 是 Epic 20 收官第二条 dev 工具，下一条 20.9 是 Layer 2 集成测试，节点 7 server 部分即将完成
- 节点 7 阶段 cosmetic_items 表已 seed（20.2 / 20.3 done）；user_cosmetic_items 表节点 8 才建，本 story stub 与此 timing 严格对齐

### 常见陷阱（基于 7.5 / 20.7 review 经验）

1. **import cycle**：devtools 包**不**能 import handler 包 → 用 interface 解耦（本 story AC3 已设计 DevCosmeticHandler interface）
2. **Register 调用位置**：必须留在 `if deps.GormDB != nil ...` 块**外**，因为 IsEnabled() 不依赖 deps；放块内会让单测 NewRouter(Deps{}) 漏挂 ping-dev
3. **typed-nil-interface 陷阱**：`var devCosmeticHandler *handler.DevCosmeticHandler` 提前声明 nil；deps 完整时填充。**关键**：传给 `devtools.Register` 时**必须** nil-collapse 到真正的 nil interface，不能直接传 typed-nil `*handler.DevCosmeticHandler(nil)`。本 story AC4 已设计 nil-collapse 三参数写法防御
4. **既有 devtools_test case 编译失败**：本 story 改 Register 签名为 `(r, devSteps, devChest, devCosmetic)`，既有所有 `devtools.Register(r, X, Y)` 调用都会编译失败（少传一个参数）—— Task 7.3 显式负责修复
5. **跨 story 越权红线**：
   - **不**新建 `migrations/0015_init_user_cosmetic_items.up.sql`（Story 23.2 owner）
   - **不**新建 `user_cosmetic_item_repo.go`（Story 23.5 owner）
   - **不**扩 `cosmetic_item_repo.go` 加 FindRandomByRarity 方法（Story 23.5 / 32.4 owner）
   - 如 review 时发现这些文件被改 → HALT 并问设计（与"任何超出范围红线 → HALT"原则一致）
6. **stub 单测无 repo stub**：与 7.5 / 20.7 单测大量 stub repo 不同，本 story service stub 不调 repo → 单测**不需要新增任何 stub 方法**（auth_service_test.go 的 stubChestRepo / stubUserRepo / 任一其他 stub 不动）。新手陷阱：误以为"按 7.5 / 20.7 模式必须加 stub" → 本 story 节点 7 阶段 stub 不需要
7. **集成测试 deferred 红线**：epics.md §20.8 行 2970 钦定"集成测试覆盖（节点 8 完成后跑）"—— 本 story **严禁**写集成测试（user_cosmetic_items 表未建 → dockertest migrate 失败 + 与 epics AC 不一致 + 浪费工时）
8. **ErrorMappingMiddleware 必挂**：handler 单测 router `r.Use(middleware.ErrorMappingMiddleware())` 必挂，否则 c.Error 不写 envelope —— 与 7.5 / 20.7 同模式
9. **build tag 影响**：`//go:build !devtools` 在 router_dev_test.go / devtools_test.go 必须保留；`go test -tags devtools` 跑这些文件会让 forceDevEnabled=true 让 BUILD_DEV="" 的 case 失败
10. **`bash scripts/build.sh --devtools`**：本 story 改 devtools.Register 签名；必须验证 build tag 路径（forceDevEnabled=true 编译路径）也能通过；script 自动跑 build/catserver-dev 输出
11. **节点 7 阶段错误码限定**：节点 7 stub 阶段 service 永远 return nil，handler 仅返 1002（参数错）—— **不**返 1003 / 1009；这些错误码留给节点 8 激活后由 23.5 owner 接入
12. **未来 schema 变更预留**：节点 8 激活时 23.5 owner 可能想扩 response schema 返实际创建的 user_cosmetic_item_id 列表 —— 本 story handler doc 已标注"节点 8 激活后由 23.5 owner 决定是否扩 schema" + 当前返简单 ack；契约兼容性优先（与 20.7 r5 lesson "dev endpoint correctness > contract aesthetics" 一致 —— 简单 ack 不破任何调用方）

### 测试覆盖矩阵

| 测试层 | 文件 | 新增 case | 覆盖 AC |
|---|---|---|---|
| service 单测 | `dev_cosmetic_service_test.go`（新） | 3 | AC1, AC5 |
| handler 单测 | `dev_cosmetic_handler_test.go`（新） | 5-10 | AC2, AC6 |
| devtools 路由测试 | `devtools_test.go`（追加） | 2 | AC3, AC7 |
| bootstrap wire 测试 | `router_dev_test.go`（追加） | 1 | AC4, AC8 |
| 集成测试 | **deferred to Story 23.5** | 0（本 story 阶段） | AC9（部分；节点 8 激活后补） |
| **合计** | | **11-16** | |

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash scripts/build.sh` → BUILD SUCCESS（vet + build → `build/catserver.exe`）
- `bash scripts/build.sh --test` → OK: all tests passed（含本 story 新增 15 case：service 3 + handler 11 + devtools 2 + bootstrap 1，**整 server 模块**全过 / 无 regression）
- `bash scripts/build.sh --devtools --test` → BUILD SUCCESS + all tests passed（验证 `-tags devtools` 路径 + Register 三参数新签名兼容；`devtools` 测试包在 `!devtools` build tag 下被跳过，符合既有约定）
- `go test ./internal/service/... -run TestDevCosmeticService -v` → 3 case + 19 子 case 全 PASS
- `go test ./internal/app/http/handler/... -run TestDevCosmeticHandler -v` → 11 case 全 PASS
- `go test ./internal/app/http/devtools/... -run TestRegister_BuildDevTrue_GrantCosmeticBatch -v` → 2 case 全 PASS
- `go test ./internal/app/bootstrap/... -run TestRouter_DevGrantCosmeticBatch -v` → 1 case PASS

### Completion Notes List

**AC10 验证清单（10 项人工核对结果，dev-story 阶段填充）**：

1. [x] service 层 `GrantCosmeticBatch` 是 stub —— 仅 slog.WarnContext + return nil；**无** repo 调用
2. [x] service 接口 doc 注释明确"节点 7 vs 节点 8 阶段切片策略"含节点 8 激活路径示例代码
3. [x] handler `PostGrantCosmeticBatchRequest` 用 *uint64 / *int8 / *int32 指针；userId / rarity / count 缺失或非法值显式 1002 拦截
4. [x] handler 校验 rarity ∈ [1,4]、count ∈ [1,100]；**不**调 `c.Get(UserIDKey)`
5. [x] devtools.Register 签名扩 (r, devStepsHandler, devChestHandler, devCosmeticHandler)；nil 时跳过路由
6. [x] router.go wire `var devCosmeticHandler` 提前声明 + if 块内构造（NewDevCosmeticService() 无参数）+ nil-collapse 三参数 Register
7. [x] **未新建** migrations/0015 / **未新建** user_cosmetic_item_repo.go / **未改** cosmetic_item_repo.go
8. [x] devtools_test.go 既有 case 全部改 3 参数新签名；编译通过
9. [x] 既有所有 dev endpoint test 仍全过；既有 chest / step / auth / room 等 test 不受影响
10. [x] bash scripts/build.sh --test 全绿；--devtools 不破；--integration 既有集成全过

**集成测试 deferred 标注**：

epics.md §Story 20.8 行 2970 钦定"集成测试覆盖（节点 8 完成后跑）"—— 本 story 阶段 user_cosmetic_items 表未建，dockertest migrate 跑不通。Story 23.5 阶段 owner 在 user_cosmetic_items migration done 后激活本 service 真实写库逻辑时，一起加 `dev_cosmetic_service_integration_test.go`（含 HappyPath_GrantsRecordsInDB / UserNotFound / DBError 等 case）。

**实装亮点 / 与 spec 差异**：

- service / handler / devtools / bootstrap 4 处骨架 + 4 处单测全按 spec 落地；**无 spec 差异**
- handler 单测共 11 case（spec 钦定 ≥5 必须 + ≥7 加分 → 实际落地 10 case + 1 加分 UserIDZero）：HappyPath / RarityInvalid_99 / RarityZero / CountZero / CountTooLarge_101 / MissingUserID / MissingRarity / MissingCount / InvalidJSON / UserIDZero / ServiceError_Forwards1009_HTTP500
- service 单测共 3 case（spec 钦定 3 case）：HappyPathStub_ReturnsNil_LogsWarn（含 phase=node-7-stub 字段断言）/ BoundaryCases 表驱动（rarity×count = 12 子 case）/ StubIgnoresInvalidParams（7 子 case 含 rarity=99/0/-1、count=0/-1/1000、userID=0）
- nil-collapse 三参数 Register 写法严格按 spec AC4 落地（`var stepsArg/chestArg/cosmeticArg` + `Register(r, stepsArg, chestArg, cosmeticArg)`）；防 typed-nil-interface 陷阱
- `NewDevCosmeticService()` 无参数（stub 无 repo 依赖，节点 8 / Story 23.5 激活时改签名加 `cosmeticItemRepo, userCosmeticItemRepo`）
- 集成测试 deferred to Story 23.5（epics.md §20.8 行 2970 钦定；user_cosmetic_items 表节点 8 才建）—— 本 story 阶段 `--integration` 不新增 case，既有集成测试全过

### File List

新建 4 文件：

1. `server/internal/service/dev_cosmetic_service.go` — DevCosmeticService interface + devCosmeticServiceImpl + NewDevCosmeticService（节点 7 阶段 stub 实装 `slog.WarnContext` + `return nil`）
2. `server/internal/service/dev_cosmetic_service_test.go` — 3 case（HappyPathStub_ReturnsNil_LogsWarn / BoundaryCases_AlwaysReturnsNil 表驱动 / StubIgnoresInvalidParams_ReturnsNilForRarity99）
3. `server/internal/app/http/handler/dev_cosmetic_handler.go` — DevCosmeticHandler + PostGrantCosmeticBatchRequest（指针 3 字段）+ PostGrantCosmeticBatch + postGrantCosmeticBatchResponseDTO
4. `server/internal/app/http/handler/dev_cosmetic_handler_test.go` — 11 case（含 stubDevCosmeticService + newDevCosmeticHandlerRouter helper）

修改 3 文件（server code）：

5. `server/internal/app/http/devtools/devtools.go` — 新增 `DevCosmeticHandler` interface；`Register` 签名扩为四参数；`if devCosmeticHandler != nil { g.POST("/grant-cosmetic-batch", ...) }` 路由注册
6. `server/internal/app/http/devtools/devtools_test.go` — 末尾追加 `devCosmeticHandlerFunc` adapter + 2 case；既有 4 处 `devtools.Register(r, X, Y)` 全改三参数（补 `, nil` 第 3 参数）
7. `server/internal/app/bootstrap/router.go` — 顶部 `var devCosmeticHandler *handler.DevCosmeticHandler` 声明；if 块内 `devCosmeticSvc := service.NewDevCosmeticService()` + `devCosmeticHandler = handler.NewDevCosmeticHandler(devCosmeticSvc)`；末尾 nil-collapse `cosmeticArg` + `devtools.Register(r, stepsArg, chestArg, cosmeticArg)`
8. `server/internal/app/bootstrap/router_dev_test.go` — 末尾追加 1 case `TestRouter_DevGrantCosmeticBatch_NilHandlerSkipsRoute`

流程文件（2）：

9. `_bmad-output/implementation-artifacts/sprint-status.yaml` — `20-8` 状态 `ready-for-dev` → `in-progress` → `review`
10. `_bmad-output/implementation-artifacts/20-8-dev-端点-post-dev-grant-cosmetic-batch.md` — 本 story 文件（Tasks/Subtasks 勾选 + Debug Log + Completion Notes + File List + Change Log + Status）

**严格无侵入区**（spec 钦定）：

- **未新建** `server/migrations/0015_init_user_cosmetic_items.up.sql`（Story 23.2 owner）
- **未新建** `server/internal/repo/mysql/user_cosmetic_item_repo.go`（Story 23.5 owner）
- **未改** `server/internal/repo/mysql/cosmetic_item_repo.go`（Story 23.5 / 32.4 owner —— 加 FindRandomByRarity 方法）
- **未改** `server/internal/app/bootstrap/Deps` struct（节点 7 阶段 stub 无新依赖）
- **未改** `server/cmd/server/main.go` / `internal/app/bootstrap/server.go`
- **未改** 任一既有 service / handler / repo / middleware / errors / random / 任一 docs / 任一 lessons / 任一 ADR

### Change Log

| 日期 | 改动 | 状态变更 |
|---|---|---|
| 2026-05-15 | Story 20.8 created (backlog → ready-for-dev) | backlog → ready-for-dev |
| 2026-05-15 | dev-story 实装：service / handler / devtools / bootstrap 4 处骨架 + 4 处单测（service 3 + handler 11 + devtools 2 + bootstrap 1 = 17 case 全过）；`bash scripts/build.sh --test` / `--devtools --test` 全绿；遵守 spec 严格无侵入区（不动 migrations / user_cosmetic_item_repo / cosmetic_item_repo / Deps struct） | in-progress → review |
| 2026-05-15 | fix-review r3：dev 端点 metrics 豁免（更根因解）—— 在 `server/internal/infra/metrics/http.go` 加 `isDevPath` 私有 helper + `ObserveHTTP` 入口短路返回，让 `/dev/*` 路径**完全不计入** `cat_api_requests_total` / `cat_api_request_duration_seconds`；连带把 20.7 force-unlock-chest / 7.5 grant-steps / 未来新增 dev 端点一起豁免。新增 3 个测试（DevEndpoint_NotCounted 含 stub 501 核心 case / DevPrefixDiscipline 防 over-match / IsDevPath helper 单测）；归档 lesson `docs/lessons/2026-05-15-dev-endpoint-metrics-exempt-20-8-r3.md` | review (no transition) |
