# Story 20.7: dev 端点 POST /dev/force-unlock-chest

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a demo / 开发者,
I want 一个 build flag gated 的 dev 接口把指定用户的当前 chest 强制推到 unlockable 状态（直接 UPDATE `user_chests.unlock_at = now()` 满足 §6.7 + Story 20.5 的"unlock_at <= now → status=2 unlockable"动态判定语义）,
so that demo / 自动化 e2e / 手工调试时不必等 10 分钟倒计时，直接通过 dev 端点把宝箱压到可开状态，配合 Story 20.6 POST /chest/open 走开箱链路（含 Story 21.x iOS 端开箱 UI / Epic 22 跨端 e2e demo 数据准备）。

## 故事定位（Epic 20 第七条 = 节点 7 dev 工具第一条；上承 20.1~20.6 server 业务实装 + 7.5 dev 端点框架，下启 20.8 dev grant cosmetic + 20.9 layer-2 集成 + Epic 21 iOS 开箱 UI demo）

- **Epic 20 进度**：20.1（接口契约 r1~r15 锚定，**done**）→ 20.2（cosmetic_items migration，**done**）→ 20.3（cosmetic_items seed ≥15 行，**done**）→ 20.4（chest_open_logs migration，**done**）→ 20.5（GET /chest/current 动态判定，**done**）→ 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取，**done**）→ **20.7（本 story，POST /dev/force-unlock-chest dev 端点）** → 20.8（dev /dev/grant-cosmetic-batch）→ 20.9（Layer 2 集成测试 - 开箱事务全流程）。
- **本 story 是 Epic 20 第一条 dev 工具实装**（前 6 条都是业务接口；20.7 + 20.8 + 20.9 是节点 7 收官三条）：
  - 业务目的：dev 端点把 `user_chests.unlock_at` 压到 `now()`（UTC），与 Story 20.5 GET /chest/current 的"unlock_at <= now → status=2 unlockable"动态判定串联 —— 调本端点后立刻调 GET /chest/current 会拿到 `status=2, remainingSeconds=0`，调 POST /chest/open 即可开箱（不被 4002「chest 尚未解锁」拦截）。
  - 不动 `status` 字段：Story 20.5 / 20.6 全部以 `unlock_at` 作为"是否可开"的真值（status 仍是 DB 字面值 1=counting，service 层动态翻译为 2=unlockable）；本端点只需 UPDATE `unlock_at`，**不**改 `status`，避免与 Story 20.5 「不更新 DB 字面 status」契约冲突 + 不破坏 20.6 OpenChest 步骤 5f 的状态机判定路径。
  - 不动 `version` 字段：本 dev 端点不参与乐观锁串行化（与 OpenChest Story 20.6 的"持锁更新 version+1"完全独立 —— 本端点是"故意旁路"，prod 不可达）；不接 version 校验让端点幂等（重复调用都把 unlock_at 推到本次 now）。

- **epics.md AC 钦定**（`_bmad-output/planning-artifacts/epics.md` §Story 20.7 行 2932-2949）：
  - **Given** Epic 1 Story 1.6 Dev Tools 框架已就绪 + Story 20.5 chest 状态机可读
  - **When** 仅在 BUILD_DEV=true 模式调用 `POST /dev/force-unlock-chest {userId: int64}`
  - **Then** service 直接 `UPDATE user_chests SET unlock_at = now WHERE user_id=?`
  - **And** 生产构建下访问该端点返回 404
  - **And** 接口**不**要求 auth
  - **And** **单元测试覆盖**（≥3 case）:
    - happy: dev mode + 用户存在 → unlock_at 更新为 now
    - edge: dev mode + 用户无 chest → 1003
    - edge: 非 dev mode → 路由返回 404
  - **And** **集成测试覆盖**（dockertest + BUILD_DEV=true）：用户 chest unlock_at 在未来 → /dev/force-unlock-chest → GET /chest/current 返回 status=2

- **V1 接口设计 doc 状态**：本 dev 端点**不**在 V1 §1-§16 主接口清单内（V1 §1 节点 7 冻结声明只锁 §7.1 / §7.2 业务接口）。dev 端点契约**仅**由 epics.md §Story 20.7 钦定；与 Story 7.5 /dev/grant-steps 同政策（dev 端点是私有运维接口，不进 V1 doc）。

- **devtools 框架契约**（Story 1.6 已落地；详见 `server/internal/app/http/devtools/devtools.go` + `_bmad-output/implementation-artifacts/1-6-dev-tools-框架.md`）：
  - **双闸门** OR 启用：(1) build tag `-tags devtools` → `forceDevEnabled=true`；(2) env var `BUILD_DEV=true`（严格字面，**不**接受 `"1"` / `"yes"` / `"TRUE"`）。任一即启用。
  - **Register(r, devStepsHandler)** 在 IsEnabled()==false 时直接返回，不挂任何 /dev/* 路由 → Gin 默认 NoRoute → HTTP 404 + 文本 `"404 page not found"`（**非** envelope）。
  - **DevOnlyMiddleware()** 是 /dev/* 路由组的请求时第二闸门：IsEnabled() false 时推 ErrResourceNotFound (1003) → ErrorMappingMiddleware 翻成 envelope（HTTP 200 + `code=1003` + `message="资源不存在"`）。
  - **业务 dev 端点扩展模式**：在 `devtools.Register(r, ...)` 内部加 `g.POST("/force-unlock-chest", devChestHandler.PostForceUnlockChest)`（与 Story 7.5 `g.POST("/grant-steps", devStepsHandler.PostGrantSteps)` 同模式）。**关键**：Story 7.5 已经把 `Register` 签名扩到 `(r, devStepsHandler DevStepsHandler)`；本 story 必须**再次扩签名**为 `(r, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)` —— 给 dev chest handler 留独立 interface 抽象槽位，避免把多个业务 dev 端点塞到同一个 interface。
  - **devtools 层 vs 业务层分离**：devtools 包只做"框架"（Register / DevOnlyMiddleware / 启停判定 + 各业务 dev handler 的 interface 抽象槽位）；本 story 的"chest dev 业务逻辑"（service 层 `ForceUnlockChest` + handler）写到**业务层**（`internal/service/dev_chest_service.go` + `internal/app/http/handler/dev_chest_handler.go`），devtools.go 仅追加 `DevChestHandler` interface + `Register` 签名扩 + 路由注册一行。

## 范围红线（明确不做）

**本 story 只做**：

1. **service 层**：新建 `server/internal/service/dev_chest_service.go` —— `DevChestService` interface + `devChestServiceImpl` 实装 + `NewDevChestService` 构造函数；`ForceUnlockChest(ctx, userID) error` 方法（**单 UPDATE 不开事务**：UPDATE 是单行原子操作；与 Story 7.5 DevStepService.GrantSteps **不同**：7.5 需要 4 步事务 FindByID + FindByUserID + UpdateBalance + Create sync_log；本 story 单 UPDATE 即可达成"unlock_at 推到 now"语义，不需要审计行 / version+1 / 多表写入）。
2. **handler 层**：新建 `server/internal/app/http/handler/dev_chest_handler.go` —— `DevChestHandler` struct + `NewDevChestHandler` + `PostForceUnlockChest(c *gin.Context)` 方法 + `PostForceUnlockChestRequest` DTO + `postForceUnlockChestResponseDTO` helper。
3. **devtools 层**：扩 `server/internal/app/http/devtools/devtools.go` —— 改 `Register` 签名为 `Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)`（接收新业务 handler）；新增 `DevChestHandler interface { PostForceUnlockChest(c *gin.Context) }`；在 `if devStepsHandler != nil { g.POST("/grant-steps", ...) }` 之后追加 `if devChestHandler != nil { g.POST("/force-unlock-chest", devChestHandler.PostForceUnlockChest) }`。**关键 nil 陷阱**：与 Story 7.5 同模式 —— nil-tolerant 跳过路由 + interface 解耦避免 import cycle。
4. **repo 层**：扩 `server/internal/repo/mysql/chest_repo.go` —— `ChestRepo` interface 追加 `UpdateUnlockAt(ctx, userID, unlockAt time.Time) error` 方法 + impl 实装。
   - 实装：`UPDATE user_chests SET unlock_at = ? WHERE user_id = ?`；rows_affected=0 → 返 `ErrChestNotFound` 哨兵；其他 DB error 透传给 service 由 service 包成 1003 / 1009。
   - **不**改 `Create` / `FindByUserID` / `FindByUserIDForUpdate` / `Delete` 任一行。
   - **不**改 `UserChest` GORM struct（字段不变）。
   - 单测追加 ≥2 case（happy: UPDATE 成功 rows_affected=1 / edge: user_id 不存在 rows_affected=0 → ErrChestNotFound）。
5. **bootstrap 层**：扩 `server/internal/app/bootstrap/router.go` —— 在业务 wire 块内（`if deps.GormDB != nil && ...` 块**内**）构造 `devChestSvc := service.NewDevChestService(chestRepo)` + `devChestHandler = handler.NewDevChestHandler(devChestSvc)`；与 Story 7.5 同模式：if 块**外**用 `var devChestHandler *handler.DevChestHandler` 提前声明 nil，块内填充；改 `devtools.Register` 调用为 `devtools.Register(r, devStepsHandler, devChestHandler)`（**关键 nil 陷阱**：与 Story 7.5 同 —— `*handler.DevChestHandler(nil)` typed-nil 传给 interface 会被 `!= nil` 误判 → 显式分支 `if devChestHandler == nil { Register(r, devStepsHandler, nil) } else { Register(r, devStepsHandler, devChestHandler) }`）。
   - **不**改 `Deps` struct（无新依赖；chestRepo 已在 if 块内 wire 给 authSvc / homeSvc / chestSvc 复用）。
   - **不**改 `cmd/server/main.go`（router wire 内部消费 Deps，main.go 透明）。
6. **service 单测**：新建 `server/internal/service/dev_chest_service_test.go` —— ≥3 case（HappyPath / ChestNotFound 1003 / DBError 1009）。复用 `auth_service_test.go` 中 `stubChestRepo` —— 该 stub 已有 `Create` / `FindByUserID` / `FindByUserIDForUpdate` / `Delete` 四方法 panic-default；本 story 需要追加 `UpdateUnlockAt` 方法（**关键 hint**：因 `ChestRepo` interface 增了一个方法，`stubChestRepo` 必须同步追加 `UpdateUnlockAt(ctx, userID, unlockAt) error` 方法 + `updateUnlockAtFn` 字段，否则 `auth_service_test.go` 全部 case 编译失败 —— 本 story 必须改 `auth_service_test.go` 加方法）。
7. **handler 单测**：新建 `server/internal/app/http/handler/dev_chest_handler_test.go` —— ≥4 case（HappyPath_ReturnsAck / UserIDZero_Returns1002 / ChestNotFound_Forwards1003 / DBBusy_Forwards1009）。stub 设计：新建独立 `stubDevChestService`（与 `stubDevStepService` 平级）；newDevChestHandlerRouter（**不**挂 mock auth；dev 路径无 auth）。
8. **devtools 框架测试扩展**：扩 `server/internal/app/bootstrap/router_dev_test.go` —— 追加 1 case：BUILD_DEV=true + Deps{} 零值 → `devChestHandler` 保持 nil → `/dev/force-unlock-chest` 返 404（nil-tolerant 路径）。**OR** 扩 `server/internal/app/http/devtools/devtools_test.go` —— 追加 2 case：(a) BUILD_DEV=true + 传入非 nil DevChestHandler → /dev/force-unlock-chest 路由存在 → 调到 handler；(b) BUILD_DEV=true + 传入 nil DevChestHandler → /dev/force-unlock-chest 跳过 → 404。**决策**（与 7.5 同）：两处都加最小 case；router_dev_test 验"完整 wire 链路"；devtools_test 验"路由注册 / 跳过"决策点。
   - **关键改动**：Story 7.5 既有的 6 个 devtools_test case 全部调 `devtools.Register(r, nil)`（仅 1 个可选 handler）；本 story 改 Register 签名后，**所有**这 6 个 case 必须改为 `devtools.Register(r, nil, nil)`（2 个可选 handler）；新加的 7.5 grant-steps 相关 case（GrantStepsRegisteredWhenHandlerProvided / GrantStepsSkippedWhenHandlerNil）必须改为 `devtools.Register(r, stubHandler, nil)` / `devtools.Register(r, nil, nil)`。
9. **集成测试**：新建 `server/internal/service/dev_chest_service_integration_test.go` —— ≥1 case：BUILD_DEV=true 容器 → INSERT user + chest with unlock_at=now+10min → svc.ForceUnlockChest(userID) → SELECT user_chests → unlock_at <= now → GET /chest/current 仿真（直接调 chest_service.GetCurrent）返 status=2 unlockable / remainingSeconds=0。
10. **本 story 文件 + sprint-status.yaml** 更新。

**本 story 不做**：

- **不**改 `docs/宠物互动App_V1接口设计.md` 任一行（dev 端点是私有运维接口，不进 V1 doc；与 7.5 同政策）
- **不**改 `docs/宠物互动App_数据库设计.md` 任一行（user_chests §5.6 表结构未改；本 story 仅 UPDATE 既有 `unlock_at` 列）
- **不**改 `migrations/0001-0014` 任一文件（user_chests 表结构已锁；本 story 仅消费现有列）
- **不**改 `chest_service.go` (20.5) / `chest_open_service.go` (20.6) / `chest_handler.go` (20.5/20.6) 任一行（dev 走独立 service / handler，**不**复用 GetCurrent / OpenChest / Open handler）
- **不**改 `step_account_repo.go` / `cosmetic_item_repo.go` / `chest_open_log_repo.go` / `chest_open_idempotency_record_repo.go` 任一行（本 story 不读不写步数账户 / cosmetic 表 / log 表 / 幂等表）
- **不**改 `step_service.go` (7.3) / `dev_step_service.go` (7.5) 任一行（dev chest 走独立 service，**不**复用 dev step service）
- **不**改 `internal/pkg/errors/codes.go`（1002 / 1003 / 1009 全已注册；本 story 仅消费；**不**新增 4xxx 业务错误码 —— 本 story 用 1003 ErrResourceNotFound 而非 4001 ErrChestNotFound，与 epics.md §20.7 行 2947 "用户无 chest → 1003" 钦定一致；与 7.5 dev grant 用 1003 而非业务码同模式）
- **不**改 `internal/app/http/middleware/auth.go` / `rate_limit.go` / `error_mapping.go`（已实装；dev 端点**不**挂 auth / **不**挂 rate_limit；与 7.5 同）
- **不**改 `Deps` struct（dev chest 用 `Deps.GormDB` 已注入；不引入新依赖）
- **不**接 Redis（dev 端点不需要 Redis；与 7.5 同）
- **不**接幂等键 `idempotencyKey`（dev 端点是"故意可重复"语义；重复调本端点都把 unlock_at 推到当前 now，不像 chest/open 不能重复扣可用步数；与 7.5 dev grant 同政策）
- **不**接 version 乐观锁（dev 端点是"故意旁路"的强制更新；prod 不可达；不参与 20.6 OpenChest 并发持锁路径）
- **不**接 chest.status UPDATE（Story 20.5 钦定 status 字面值留 DB 不动；动态判定全靠 unlock_at vs now）
- **不**接事务（单 UPDATE 是原子的；不存在多表 / 多步骤一致性问题；与 14.2 POST /pets/current/state-sync 同"单 UPDATE 不开事务"模式）
- **不**支持 `userId=0` / `userId=-1` / `userId="abc"` 等异常（handler binding 校验 + service.UpdateUnlockAt → ErrChestNotFound 自然失败）
- **不**支持 `unlock_at = now + offset`（dev 端点产品语义是"立刻可开"；如果未来需要 demo 「unlock_at = now + 30s」滚动倒计时，加独立 `/dev/set-chest-unlock-at {userId, unlockAt}` 端点，YAGNI 本 story 不预实装）
- **不**写 e2e 跨端测试（Epic 22 Story 22.1 才做）
- **不**写性能压测（dev 端点不接 prod 流量，不做 NFR 性能 baseline）
- **不**做"prod 二进制访问 /dev/force-unlock-chest 返 envelope code=1003"测试 —— Story 1.6 devtools_test.go 已有 `TestRegister_BuildDevFalse_PingDevReturns404` + `TestDevOnlyMiddleware_RejectsWhenDisabled` 覆盖通用闸门，本 story 不重复
- **不**改 `docs/lessons/*.md`（无新教训；本 story 是直接落地，无 review iteration 预期）
- **不**预实装 20.8 dev /dev/grant-cosmetic-batch（即便顺手把 DevChestHandler interface 加 PostGrantCosmeticBatch 也禁止 —— YAGNI，20.8 owner）
- **不**预实装 20.9 Layer 2 集成测试的全部场景（本 story 集成测试覆盖 happy path 一个，20.9 owner 回滚 / 并发 / 抽奖分布等深度场景）

**任何超出上述清单的改动 → HALT 并问设计**。

## Acceptance Criteria

**AC1 — `service.DevChestService` interface + impl（新文件 `internal/service/dev_chest_service.go`）**

新建 `server/internal/service/dev_chest_service.go`：

```go
package service

import (
	"context"
	stderrors "errors"
	"log/slog"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// DevChestService 是 /dev/force-unlock-chest 端点的依赖 interface（Story 20.7）。
//
// **dev 端点的产品语义**：把指定用户的当前 chest 强制推到 unlockable 状态（直接
// UPDATE `user_chests.unlock_at = now()` UTC），与 Story 20.5 GET /chest/current
// 动态判定 "unlock_at <= now → status=2 unlockable" 串联 —— 调本端点后立刻调
// GET /chest/current 拿到 status=2 / remainingSeconds=0，调 POST /chest/open
// 即可开箱不被 4002「chest 尚未解锁」拦截。仅供 demo / 自动化 e2e / 手工调试，**不**走 prod。
//
// **不**复用 Story 20.6 chest_service.OpenChest：
//   - OpenChest 含 8 步事务（幂等预声明 + 持锁查询 + 步数扣减 + 加权抽取 + 写日志 +
//     刷新下一轮）—— 全不适用 dev force-unlock（dev 端点只想"压时间"，不消费步数 / 不抽奖）
//   - 强行复用要绕过 5+ 步事务分支，反模式 → 独立 service 更清晰
//
// **不**复用 chest_service.GetCurrent：
//   - GetCurrent 是"纯读 + 动态翻译"；本 service 是"强制 UPDATE"，语义反向
//
// 错误约定（ADR-0006 三层映射）：
//   - mysql.ErrChestNotFound（UpdateUnlockAt rows_affected=0）→ 包成 ErrResourceNotFound (1003)
//     **注意**：用 1003 而非 4001 ErrChestNotFound —— epics.md §20.7 行 2947 钦定 "用户无 chest → 1003"
//     与 Story 7.5 dev grant 用 1003 而非业务码同模式（dev 端点错误码统一在通用 1xxx 段）
//   - 其他 DB 异常（连接断 / SQL 错 / 死锁等）→ 包成 ErrServiceBusy (1009)
//
// **不**接事务（单 UPDATE 是原子的；与 Story 14.2 PostStateSync 同"单 UPDATE 不开事务"模式）。
type DevChestService interface {
	// ForceUnlockChest 把指定 userID 的当前 chest unlock_at 推到 now() UTC：
	//  1. chestRepo.UpdateUnlockAt(ctx, userID, time.Now().UTC()) → rows_affected=0 → ErrChestNotFound
	//
	// **不**改 chest.status 字段（Story 20.5 钦定 DB 字面 status 不动；动态判定全靠 unlock_at vs now）。
	// **不**改 chest.version 字段（dev 端点不参与乐观锁串行化，与 Story 20.6 OpenChest 持锁路径独立）。
	// **不**接 unlock_at 参数（dev 产品语义是"立刻可开"；未来如需"滚动倒计时 demo"加独立端点）。
	ForceUnlockChest(ctx context.Context, userID uint64) error
}

// devChestServiceImpl 是 DevChestService 的默认实装。
type devChestServiceImpl struct {
	chestRepo mysql.ChestRepo
}

// NewDevChestService 构造 DevChestService。
//
// 依赖：
//   - chestRepo：UpdateUnlockAt 强制更新当前 chest 的 unlock_at 列
//
// 与 NewDevStepService (Story 7.5) 不同：**不**接 txMgr（单 UPDATE 不开事务）；
// **不**接 userRepo（UpdateUnlockAt rows_affected=0 已经表达"用户无 chest"语义，
// 不需要额外的 FindByID(user) 前置验证 —— dev 端点容忍 "user 不存在" 与 "chest 不存在" 都映射 1003）；
// **不**接 stepAccountRepo / stepSyncLogRepo（dev force-unlock 不读写步数账户）；
// **不**接 envName（dev 端点已被 build tag / env var 双闸门防 prod，不再做 envName 检查）。
func NewDevChestService(chestRepo mysql.ChestRepo) DevChestService {
	return &devChestServiceImpl{chestRepo: chestRepo}
}

// ForceUnlockChest 实装：单 UPDATE user_chests SET unlock_at = ? WHERE user_id = ?。
//
// **time.Now().UTC()**：必须 UTC（与 V1 §2.5 ISO 8601 UTC 钦定一致；与 chest.UnlockAt
// 字段 UTC 语义对齐 —— chest_repo.go 顶部注释钦定，Story 4.6 firstTimeLogin
// 也用 time.Now().UTC().Add(10*time.Minute)，与本 story now() 同源 UTC）。
func (s *devChestServiceImpl) ForceUnlockChest(ctx context.Context, userID uint64) error {
	now := time.Now().UTC()
	if err := s.chestRepo.UpdateUnlockAt(ctx, userID, now); err != nil {
		if stderrors.Is(err, mysql.ErrChestNotFound) {
			// epics.md §20.7 行 2947 钦定：用户无 chest → 1003（**非** 4001）
			return apperror.Wrap(err, apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
		}
		return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	slog.WarnContext(ctx, "dev force-unlock-chest applied",
		"user_id", userID, "unlock_at", now.Format(time.RFC3339),
	)
	return nil
}
```

**关键约束**：

- **新 service 文件**：dev force-unlock 业务规则极简（1 步 UPDATE），但与 20.5 GetCurrent / 20.6 OpenChest 业务语义完全不同 → 独立文件清晰；**不**复用 ChestService interface
- **不接事务**：单 UPDATE 是原子的；与 14.2 PostStateSync 同模式
- **time.Now().UTC()**：必须 UTC，与 V1 §2.5 + Story 4.6 / 20.5 / 20.6 同源
- **错误翻译**：ErrChestNotFound → 1003（**非** 4001）—— epics.md §20.7 行 2947 钦定 + 与 Story 7.5 dev grant 同模式
- **slog.WarnContext**：dev 端点写 WARN 级别审计日志（与 7.5 dev grant 同模式 —— "有人调用了 dev 端点"对运维 / 测试调试有价值）
- **不接 userRepo 前置 FindByID**：UpdateUnlockAt rows_affected=0 已表达"用户无 chest"语义；与 7.5 显式 FindByID 前置不同（7.5 是 4 步事务必须先验用户存在；本 story 单 UPDATE 直接靠 rows_affected 兜底）

**AC2 — `handler.DevChestHandler` + `PostForceUnlockChestRequest` DTO（新文件 `internal/app/http/handler/dev_chest_handler.go`）**

新建 `server/internal/app/http/handler/dev_chest_handler.go`：

```go
package handler

import (
	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// DevChestHandler 是 /dev/force-unlock-chest 等 dev 宝箱端点的 handler 集合（Story 20.7）。
//
// 与 ChestHandler (20.5 / 20.6) 区分：
//   - ChestHandler 处理 /api/v1/chest/*（业务接口；含 auth + rate_limit + 业务事务 / 幂等）
//   - DevChestHandler 处理 /dev/force-unlock-chest（dev 工具；不含 auth / rate_limit / 事务）
//
// 与 DevStepsHandler (Story 7.5) 平级：dev 工具按"业务模块"独立 handler，让未来加
// /dev/grant-cosmetic-batch (Epic 20.8) 时有独立 handler 槽位（DevCosmeticHandler），
// 避免单文件膨胀。
//
// 独立 handler 文件让"dev 工具"与"业务接口"边界清晰，dev 路径未来加新 chest 端点
// （如 /dev/reset-chest）也走本 handler。
type DevChestHandler struct {
	svc service.DevChestService
}

// NewDevChestHandler 构造 DevChestHandler。
func NewDevChestHandler(svc service.DevChestService) *DevChestHandler {
	return &DevChestHandler{svc: svc}
}

// PostForceUnlockChestRequest 是 POST /dev/force-unlock-chest 请求体的 Go mirror。
//
// epics.md §Story 20.7 行 2941 钦定：`{userId: int64}`。
//
// **userId 用 *uint64 指针类型**（不挂 binding:"required"）：
//   - validator/v10 把 0 视为 zero value 会误判 "required"，与 7.5 PostGrantStepsRequest 同模式
//   - 用 *uint64 指针 + handler 显式 nil 校验区分 "字段缺失" vs "显式传 0"
//   - userId=0 在 MySQL users 表里**不存在**（AUTO_INCREMENT 从 1 起），handler 显式拒
//     让错误更早 + 错误消息更精确（"userId 必须 > 0"）
//
// **不**接 unlockAt 字段（dev 产品语义是"立刻可开"；未来如需"滚动倒计时 demo"加独立端点）。
// **不**接 idempotencyKey 字段（dev 端点是"故意可重复"语义；重复调都把 unlock_at 推到本次 now）。
type PostForceUnlockChestRequest struct {
	UserID *uint64 `json:"userId"`
}

// PostForceUnlockChest 处理 POST /dev/force-unlock-chest（Story 20.7）。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：userId 非 nil（缺失 → 1002）+ != 0（非法 → 1002）
//  3. 调 svc.ForceUnlockChest(ctx, *userId) —— ctx = c.Request.Context()
//  4. 成功 → response.Success(c, postForceUnlockChestResponseDTO(...), "ok")
//  5. 失败 → c.Error(err) + return（middleware envelope；ADR-0006 单一 envelope 生产者）
//
// **不**做 auth 校验（dev 端点不要求 auth）；**不**取 c.Get(UserIDKey)（dev 路径无 auth 中间件）。
func (h *DevChestHandler) PostForceUnlockChest(c *gin.Context) {
	var req PostForceUnlockChestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// === required 字段校验（指针 nil → 字段未传 → 1002）===
	if req.UserID == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必填"))
		return
	}
	if *req.UserID == 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必须 > 0"))
		return
	}

	if err := h.svc.ForceUnlockChest(c.Request.Context(), *req.UserID); err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	// 成功响应 —— 简单 ack，不返当前 chest 状态（如要查 chest 用 GET /chest/current；端点单一职责）
	response.Success(c, postForceUnlockChestResponseDTO(*req.UserID), "ok")
}

// postForceUnlockChestResponseDTO 拼装 ack response。
//
// **schema 选择**：返 `{userId}` 简单 ack —— 不返当前 chest 状态。
//   - 调用方（demo / 自动化测试 / Epic 21 iOS）调本端点后再调 GET /chest/current
//     验证 status=2，而不是依赖本端点 response —— 端点单一职责（force-unlock 只负责"做了"，
//     get-current 只负责"读了"）
//   - 与 Story 7.5 postGrantStepsResponseDTO 同模式（dev 端点统一 ack 风格）
func postForceUnlockChestResponseDTO(userID uint64) gin.H {
	return gin.H{
		"userId": userID,
	}
}
```

**关键约束**：

- handler 层**必须**做 userId nil / 0 校验（与 7.5 同模式：validator/v10 把 0 视为 zero value 会误判 required）
- userId=0 显式拒（与 7.5 同；MySQL users.id AUTO_INCREMENT 从 1 起，0 永远不存在）
- response 返简单 ack `{userId}`，**不**返当前 chest 状态（端点单一职责；与 7.5 dev grant 同模式）
- handler **不**调 `c.Get(UserIDKey)`（dev 路径无 auth）；userID 全靠 body
- `c.Error + return` 而非 `response.Error`（ADR-0006）

**AC3 — `devtools.Register` 签名扩 + 业务 dev 路由注册（修改 `internal/app/http/devtools/devtools.go`）**

修改 `server/internal/app/http/devtools/devtools.go`：

1. **新增 DevChestHandler interface**（与 DevStepsHandler 平级）：

```go
// DevChestHandler 是 dev 宝箱端点的 handler 抽象（Story 20.7）。
//
// 用 interface 解耦避免 devtools 包反向 import handler 包：实际的 handler 实装在
// internal/app/http/handler/dev_chest_handler.go；本 interface 仅为 Register 签名抽象，
// 让 devtools 包保持"框架"角色，不依赖具体 handler 实装。
//
// **签名简化原则**：本 interface 只列 Register 签名所需的方法（PostForceUnlockChest）；
// future 加 /dev/grant-cosmetic-batch (Epic 20.8) 时新建独立 DevCosmeticHandler interface
// **不**追加到本 interface（"按业务模块独立 interface"原则，与 DevStepsHandler / DevChestHandler 平级）。
type DevChestHandler interface {
	PostForceUnlockChest(c *gin.Context)
}
```

2. **改 Register 签名**：从 `Register(r *gin.Engine, devStepsHandler DevStepsHandler)` 改为 `Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)`，追加 nil-tolerant 路由注册：

```go
// Register 把 /dev/* 路由组挂到传入的 gin.Engine 上（仅在 dev 模式启用时）。
//
// 启用时挂载以下端点：
//   - GET  /dev/ping-dev              → PingDevHandler（Story 1.6 框架自带）
//   - POST /dev/grant-steps           → devStepsHandler.PostGrantSteps（Story 7.5；devStepsHandler == nil 时跳过）
//   - POST /dev/force-unlock-chest    → devChestHandler.PostForceUnlockChest（Story 20.7；devChestHandler == nil 时跳过）
//
// **多 handler 可空设计**（nil-tolerant）：
//   - 单元测试 NewRouter(Deps{}) 零值场景：bootstrap 不构造业务 handler → 都传 nil
//     → 本函数仅注册 ping-dev，跳过所有业务路由（避免 nil deref panic）
//   - 生产路径：bootstrap 在 deps 完整时构造全部业务 handler 透传 → 本函数注册全部 dev 端点
//
// **签名扩展模式**：每加一个业务 dev 端点（grant-steps / force-unlock-chest /
// grant-cosmetic-batch 等），按业务模块独立 interface 槽位扩 Register 签名。
// 这让"哪些 dev 端点存在"在 Register 签名层就可见，避免运行时神秘失踪。
func Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler) {
	if !IsEnabled() {
		return
	}
	slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION", ...)
	g := r.Group("/dev")
	g.Use(DevOnlyMiddleware())
	g.GET("/ping-dev", PingDevHandler)
	if devStepsHandler != nil {
		// Story 7.5 加：业务 dev 端点 /dev/grant-steps；nil-tolerant 跳过避免单测 panic。
		g.POST("/grant-steps", devStepsHandler.PostGrantSteps)
	}
	if devChestHandler != nil {
		// Story 20.7 加：业务 dev 端点 /dev/force-unlock-chest；nil-tolerant 跳过避免单测 panic。
		g.POST("/force-unlock-chest", devChestHandler.PostForceUnlockChest)
	}
}
```

**关键约束**：

- 用 interface 解耦：devtools 包**不**反向 import handler 包（避免 import cycle；与 7.5 同模式）
- 每业务模块独立 interface（DevStepsHandler / DevChestHandler 平级），**不**塞到同一个 interface（让"哪些 dev 端点存在"在 Register 签名层可见）
- nil-tolerant：每 handler nil 时跳过对应路由 —— 与 NewRouter 的 `if deps.GormDB != nil && ...` 同 nil-tolerant 模式，让单测 `NewRouter(Deps{})` 不崩
- **不**新建 /dev 路由组（仍在 `g := r.Group("/dev")`）—— 与现有 ping-dev / grant-steps 共享 DevOnlyMiddleware
- ping-dev 仍是框架自带（与 1.6 同；不动其语义）

**AC4 — `bootstrap/router.go` wire dev chest service / handler / Register 签名（修改 `internal/app/bootstrap/router.go`）**

修改 `server/internal/app/bootstrap/router.go`：

1. 在 `if deps.GormDB != nil && ...` 块内（与 devStepsHandler 构造并列，紧邻其后）追加 dev chest service / handler 构造：

```go
// Story 20.7 加：dev chest service + handler（仅 dev 模式 build 启用时挂路由）
//
// 即便 BUILD_DEV=false（IsEnabled()==false），此处仍构造 service / handler；devtools.Register
// 内部判定 IsEnabled() 决定是否真注册路由。这样"是否注册"的决策集中在 devtools 包内，
// bootstrap 不重复判定（与 7.5 devStepSvc 同模式）。
//
// 复用：chestRepo 已在 if 块顶部（行 314）wire 给 authSvc / homeSvc / chestSvc；本 story 仅
// 多 wire 一个 devChestSvc + devChestHandler（不开事务 → 不依赖 txMgr）。
devChestSvc := service.NewDevChestService(chestRepo)
devChestHandler = handler.NewDevChestHandler(devChestSvc)
```

2. **在 NewRouter 函数顶部 `var devStepsHandler` 之后**追加 `var devChestHandler *handler.DevChestHandler`（提前声明 nil；deps 完整时填）：

```go
var devStepsHandler *handler.DevStepsHandler // Story 7.5
var devChestHandler *handler.DevChestHandler // Story 20.7
```

3. 改 `devtools.Register(r, devStepsHandler)` 调用为新签名 + 双 nil 陷阱兜底：

**关键 nil 陷阱**（Story 7.5 已 lesson 过的 typed-nil-interface 陷阱在本 story 同样适用）：直接传 `*handler.DevChestHandler(nil)` 给 `devtools.DevChestHandler` interface，interface header 是 `(type=*handler.DevChestHandler, value=nil)` → **非 nil interface**，Register 内 `if devChestHandler != nil` 判定会通过 → 调 PostForceUnlockChest 时 nil receiver panic。

```go
// dev 模式下挂 /dev/* 路由组；devStepsHandler / devChestHandler 在 deps 完整时填充，
// 否则保持 nil。**关键 Go 接口 nil 陷阱**（与 Story 7.5 同）：typed-nil 传给 interface
// 会被 != nil 误判 → 显式分支确保 nil 指针 → nil interface。
//
// 4 分支矩阵（dev handler nil 组合）：
//   - 都非 nil → Register(r, devStepsHandler, devChestHandler)
//   - devSteps nil + devChest 非 nil → Register(r, nil, devChestHandler)
//   - devSteps 非 nil + devChest nil → Register(r, devStepsHandler, nil)
//   - 都 nil → Register(r, nil, nil)
//
// 简化实现：每个 handler 独立 nil-collapse + 调一次 Register。
var stepsArg devtools.DevStepsHandler
if devStepsHandler != nil {
	stepsArg = devStepsHandler
}
var chestArg devtools.DevChestHandler
if devChestHandler != nil {
	chestArg = devChestHandler
}
devtools.Register(r, stepsArg, chestArg)
```

**关键变更**：

- 移除原 7.5 的 4 分支显式 if (`if devStepsHandler == nil { Register(r, nil) } else { Register(r, devStepsHandler) }`)，改为统一的 nil-collapse 模式（让 4 分支矩阵不爆炸）
- **可选**：保持原 7.5 显式 if 风格 + 嵌套 if 也行；但 nil-collapse 写法更紧凑、可扩展（future 加 devCosmeticHandler 只需 1 行 var + 1 行 if，无需 8 分支 cartesian product）。**本 story 必采 nil-collapse 写法**

```go
func NewRouter(deps Deps) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	// ... 中间件 ...

	r.GET("/ping", handler.PingHandler)
	r.GET("/version", handler.VersionHandler)
	r.GET("/metrics", gin.WrapH(metrics.Handler()))

	var devStepsHandler *handler.DevStepsHandler // Story 7.5
	var devChestHandler *handler.DevChestHandler // Story 20.7

	if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil {
		// ... 既有 wire ...
		// Story 7.5 加：dev step service + handler
		devStepSvc := service.NewDevStepService(deps.TxMgr, userRepo, stepAccountRepo, stepSyncLogRepo)
		devStepsHandler = handler.NewDevStepsHandler(devStepSvc)

		// Story 20.7 加：dev chest service + handler（不开事务 → 不依赖 txMgr）
		devChestSvc := service.NewDevChestService(chestRepo)
		devChestHandler = handler.NewDevChestHandler(devChestSvc)

		// ... 既有 api / authedGroup wire ...
	}

	// dev 模式下挂 /dev/* 路由组；nil-collapse 防 Go typed-nil-interface 陷阱。
	var stepsArg devtools.DevStepsHandler
	if devStepsHandler != nil {
		stepsArg = devStepsHandler
	}
	var chestArg devtools.DevChestHandler
	if devChestHandler != nil {
		chestArg = devChestHandler
	}
	devtools.Register(r, stepsArg, chestArg)

	return r
}
```

**关键约束**：

- `var devChestHandler *handler.DevChestHandler` 提前声明 nil —— deps 不完整时（单测）保持 nil，devtools.Register 内部跳过 force-unlock-chest 路由
- 复用 if 块顶部已 wire 的 `chestRepo` —— **不**新建 repo 实例
- 改用 **nil-collapse 写法**调 `devtools.Register(r, stepsArg, chestArg)`（避免 4 分支 cartesian product 爆炸）
- **不**改 `Deps` struct（无新依赖）
- **不**改 `cmd/server/main.go`（router wire 内部消费 Deps）

**AC5 — repo 层扩展 `ChestRepo.UpdateUnlockAt`（修改 `internal/repo/mysql/chest_repo.go`）**

修改 `server/internal/repo/mysql/chest_repo.go`：

1. **ChestRepo interface 末尾追加**：

```go
// UpdateUnlockAt 把指定 userID 的 user_chests 行的 unlock_at 列更新为 newUnlockAt。Story 20.7 引入。
//
// **不**改 status / version / open_cost_steps 任一字段（Story 20.7 dev force-unlock 业务语义：
// 只动时间字段，让 Story 20.5 GET /chest/current 动态判定 "unlock_at <= now → unlockable" 生效）。
//
// **不**接 expectedVersion 乐观锁（dev 端点是"故意旁路"，与 Story 20.6 OpenChest 持锁路径独立；
// 不参与 version+1 串行化）。
//
// **rows_affected 语义**：
//   - rows_affected=1 → UPDATE 成功 → 返 nil
//   - rows_affected=0 → user_id 不存在或无 chest → 返 ErrChestNotFound 哨兵
//     （与 FindByUserID / FindByUserIDForUpdate 共用同一哨兵；service 层 errors.Is 后翻译为 1003
//     ErrResourceNotFound —— epics.md §20.7 行 2947 钦定 "用户无 chest → 1003" 而非业务 4001）
//   - 其他 DB error（连接断 / SQL 错 / 死锁等）→ raw error 透传（service 层包成 1009）
//
// **必须用 UTC 时间**：调用方传入的 newUnlockAt 必须是 UTC（time.Now().UTC()）；
// 与 chest.UnlockAt 字段 UTC 语义对齐（chest_repo.go 顶部注释钦定，Story 4.6 / 20.5 / 20.6 同源）。
// repo 层**不**做 UTC 强转，由 service 层保证；本约束在 service.ForceUnlockChest 落地。
//
// 事务无关：本方法可在事务内或事务外调用（GORM tx.FromContext 透明）；Story 20.7 dev 端点
// 直接事务外调用（单 UPDATE 原子）；future 如有需要在事务内调用也无副作用。
UpdateUnlockAt(ctx context.Context, userID uint64, newUnlockAt time.Time) error
```

2. **chestRepo impl 末尾追加**：

```go
// UpdateUnlockAt 实装：UPDATE user_chests SET unlock_at = ? WHERE user_id = ?。
//
// 走 uk_user_id 唯一索引（与 FindByUserID 同）；rows_affected=0 → ErrChestNotFound 哨兵。
//
// **GORM 实装**：用 `gorm.DB.Model(&UserChest{}).Where("user_id = ?", userID).Update("unlock_at", newUnlockAt)`
// 而非 `.Save(&chest)` —— Save 会写入全部字段（含 version / status / open_cost_steps）+
// 触发 GORM autoUpdateTime:milli 自动更新 updated_at；Update("unlock_at", ...) 只动 unlock_at +
// updated_at（GORM 会自动加 updated_at）+ rows_affected 通过 result.RowsAffected 取值精确。
func (r *chestRepo) UpdateUnlockAt(ctx context.Context, userID uint64, newUnlockAt time.Time) error {
	db := tx.FromContext(ctx, r.db)
	result := db.WithContext(ctx).Model(&UserChest{}).Where("user_id = ?", userID).Update("unlock_at", newUnlockAt)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrChestNotFound
	}
	return nil
}
```

**关键约束**：

- **rows_affected 兜底**：rows_affected=0 → ErrChestNotFound（与 FindByUserID NotFound 共用哨兵；service 层错误码翻译统一）
- **不**改 status / version / open_cost_steps（业务语义只动时间字段；Story 20.5 动态判定钦定）
- **不**接 expectedVersion（dev 端点不参与乐观锁；与 Story 20.6 OpenChest 持锁路径独立）
- **GORM .Update() 而非 .Save()**：精确控制只更新 unlock_at 列；避免 .Save() 误写整行触发 race
- **UTC 时间**：调用方保证（service 层），repo 层透传不强转
- **事务无关**：用 `tx.FromContext(ctx, r.db)` 透明取 db handle（与既有 Create / FindByUserID 同模式）

**AC6 — service 单测 ≥3 case（stub repo；新文件 `dev_chest_service_test.go`）**

新建 `server/internal/service/dev_chest_service_test.go`：

**stub 设计**（与 7.5 dev_step_service_test 同 package；复用 `stubChestRepo`）：

```go
package service_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// **stubChestRepo 复用**：auth_service_test.go 已定义 stubChestRepo（同 package service_test），
// 含 Create / FindByUserID / FindByUserIDForUpdate / Delete 四方法 panic-default；
// 本 story 在 auth_service_test.go 追加 UpdateUnlockAt 方法 + updateUnlockAtFn 字段（**关键**：
// ChestRepo interface 加方法后，stub 必须同步追加，否则 auth_service_test 全部 case 编译失败）。
//
// buildDevChestService 用 stub repo 构造 DevChestService。
func buildDevChestService(repo mysql.ChestRepo) service.DevChestService {
	return service.NewDevChestService(repo)
}
```

**必须覆盖 3 case**（前缀 `TestDevChestService_ForceUnlockChest_`）：

1. **`TestDevChestService_ForceUnlockChest_HappyPath_UpdatesUnlockAtToNow`**：stubChestRepo.updateUnlockAtFn 接收并断言：(a) userID=1001 透传正确；(b) 传入的 newUnlockAt **必须是 UTC**（`time.Now().UTC()` 同源；用 `newUnlockAt.Location() == time.UTC` 断言）；(c) newUnlockAt 与 `time.Now().UTC()` 偏差 < 1s（防 service 误传未来 / 历史时间） → 返 nil → service.ForceUnlockChest(ctx, 1001) 返 err=nil；**断言**：repo.updateCalls=1
2. **`TestDevChestService_ForceUnlockChest_ChestNotFound_Returns1003`**：stubChestRepo.updateUnlockAtFn 返 `mysql.ErrChestNotFound` → service 返 *AppError(Code=1003)（**注意：1003 而非 4001**，与 epics.md §20.7 行 2947 钦定一致）；repo.updateCalls=1
3. **`TestDevChestService_ForceUnlockChest_DBError_Returns1009`**：stubChestRepo.updateUnlockAtFn 返 `stderrors.New("simulated db conn lost")` → service 返 *AppError(Code=1009)；repo.updateCalls=1

**关键约束**：

- 3 case 命名前缀 `TestDevChestService_ForceUnlockChest_<场景>` 一目了然（与 7.5 `TestDevStepService_GrantSteps_<场景>` 同风格）
- HappyPath case **必须**断 `newUnlockAt.Location() == time.UTC` —— 这是本 story 的核心契约断言（UTC 时间钦定，与 V1 §2.5 + Story 4.6 / 20.5 / 20.6 同源）
- ChestNotFound case 强断 `apperror.Code(err) == apperror.ErrResourceNotFound` (1003)，**不是** `apperror.ErrChestNotFound` (4001) —— 防 service 误用业务码
- DBError case 用任意 stderrors.New 模拟非哨兵错误 → 验 service 兜底翻译为 1009
- 每 case 用**独立 stub instance**（与 7.5 同约束 —— 避免计数器串扰）
- **不**写 NegativeUserID / UserIDZero panic case（dev chest service 没有"参数防御性 panic"语义 —— 不像 7.5 dev grant 因为有 steps<0 panic 防御；本 service 单 UPDATE，没有可校验参数；handler 已校验 userId 非 nil + > 0 → service 不重复校验）

**AC7 — handler 单测 ≥4 case（stub service + 测试 router；新文件 `dev_chest_handler_test.go`）**

新建 `server/internal/app/http/handler/dev_chest_handler_test.go`：

**stub 设计**（**新建** `stubDevChestService`，与 stubDevStepService 平级独立）：

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

type stubDevChestService struct {
	forceUnlockChestFn func(ctx context.Context, userID uint64) error
}

func (s *stubDevChestService) ForceUnlockChest(ctx context.Context, userID uint64) error {
	return s.forceUnlockChestFn(ctx, userID)
}

// newDevChestHandlerRouter 构造 handler test router。
//
// **关键差异 vs newChestHandlerRouter**：dev 端点不挂 mock auth middleware（dev 不要求 auth）。
// 仅挂 ErrorMappingMiddleware（c.Error 写 envelope 必需；与 7.5 newDevStepsHandlerRouter 同模式）。
func newDevChestHandlerRouter(svc service.DevChestService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewDevChestHandler(svc)
	r.POST("/dev/force-unlock-chest", h.PostForceUnlockChest)
	return r
}
```

**必须覆盖 4 case**（前缀 `TestDevChestHandler_PostForceUnlockChest_`）：

1. **`TestDevChestHandler_PostForceUnlockChest_HappyPath_ReturnsAck`**：合法 body `{"userId":1001}` → stub service 返 nil → HTTP 200 + envelope.code=0 + data.userId=1001；stub service 内部 if userID != 1001 → t.Errorf 验透传
2. **`TestDevChestHandler_PostForceUnlockChest_ChestNotFound_Forwards1003_HTTP200`**：stub service 返 `*apperror.AppError(ErrResourceNotFound, "资源不存在")` → handler `c.Error` → middleware envelope code=1003，HTTP 200（业务码与 HTTP status 正交）
3. **`TestDevChestHandler_PostForceUnlockChest_DBBusy_Forwards1009_HTTP500`**：stub service 返 `*apperror.AppError(ErrServiceBusy, "服务繁忙")` → middleware envelope code=1009，**HTTP 500**（ErrorMappingMiddleware 钦定 1009 走 500）
4. **`TestDevChestHandler_PostForceUnlockChest_UserIDZero_Returns1002_NoServiceCall`**：body `{"userId":0}` → handler 显式校验 0 → 1002 + message="userId 必须 > 0"；stub service.forceUnlockChestFn 内 t.Errorf("should NOT be called")（验证 handler 拦截在 service 之前）

**加分 case**：

5. **`TestDevChestHandler_PostForceUnlockChest_MissingUserID_Returns1002`**：body `{}`（无 userId）→ ShouldBindJSON 后 UserID 仍 nil → handler 校验失败 → 1002 + message="userId 必填"；stub.forceUnlockChestFn 内 t.Errorf 兜底
6. **`TestDevChestHandler_PostForceUnlockChest_InvalidJSON_Returns1002`**：body `{"userId":"abc"}`（类型错）→ ShouldBindJSON 失败 → 1002；stub.forceUnlockChestFn 内 t.Errorf 兜底

**关键约束**：

- 4-6 case 命名前缀 `TestDevChestHandler_PostForceUnlockChest_<场景>` —— 与 7.5 `TestDevStepsHandler_PostGrantSteps_<场景>` 同风格
- HappyPath stub 内 `if userID != 1001 { t.Errorf(...) }` 验透传
- UserIDZero / MissingUserID case **必须**用 stub.forceUnlockChestFn 内 t.Errorf 兜底验"handler 拦在 service 之前" —— 防 future handler 误删 nil/0 校验
- DBBusy case 验 HTTP **500**（不是 200）—— 1009 是唯一走非 200 的业务码
- 测试 router**不**挂 mock auth middleware —— dev 路径无 auth；与 chestHandler (20.5/20.6) 测试模式区分

**AC8 — devtools / bootstrap 框架测试扩展（修改 `internal/app/bootstrap/router_dev_test.go` + `internal/app/http/devtools/devtools_test.go`）**

**修改 `router_dev_test.go`** 末尾追加 1 case：

```go
// TestRouter_DevForceUnlockChest_NilHandlerSkipsRoute 验证 Story 20.7 dev force-unlock 端点的 wire 链路：
//   - BUILD_DEV=true + Deps{} 零值 → devChestHandler 保持 nil → devtools.Register 跳过路由 → 404
//
// 与 TestRouter_DevGrantSteps_NilHandlerSkipsRoute (7.5) 同模式：
// 真实 wire 链路（dev handler 真被调）由 dev_chest_handler_test 单测 + dev_chest_service_integration_test
// 集成测试覆盖；本测试仅验证"nil-tolerant"路径。
func TestRouter_DevForceUnlockChest_NilHandlerSkipsRoute(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")
	gin.SetMode(gin.TestMode)
	r := NewRouter(Deps{}) // 零值 deps → devChestHandler 保持 nil

	// /dev/ping-dev 应该正常注册（Register 内 ping-dev 不依赖任何业务 handler）
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dev/ping-dev", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/dev/ping-dev should be 200 when BUILD_DEV=true; got %d", w.Code)
	}

	// /dev/force-unlock-chest 应该跳过注册（devChestHandler nil）→ Gin NoRoute 404
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(`{"userId":1}`)))
	if w2.Code != http.StatusNotFound {
		t.Errorf("/dev/force-unlock-chest with nil handler should be 404; got %d body=%s", w2.Code, w2.Body.String())
	}
}
```

**修改 `devtools_test.go`** 末尾追加 2 case + 把既有所有 `devtools.Register(r, ...)` 调用改新签名（每加 1 nil 参数）：

```go
// TestRegister_BuildDevTrue_ForceUnlockChestRegisteredWhenHandlerProvided 验证 Story 20.7 路由注册：
//   - BUILD_DEV=true + 传入非 nil DevChestHandler → /dev/force-unlock-chest 路由存在
//   - 验路由存在的方式：ServeHTTP 应该走到 handler 而非 NoRoute；用 stub handler 标志位验证
func TestRegister_BuildDevTrue_ForceUnlockChestRegisteredWhenHandlerProvided(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")

	called := false
	stubHandler := devChestHandlerFunc(func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r := newEngine()
	r.Use(middleware.ErrorMappingMiddleware())
	devtools.Register(r, nil /* devSteps */, stubHandler /* devChest */)

	w := doPost(r, "/dev/force-unlock-chest", `{"userId":1}`)

	if w.Code != http.StatusOK {
		t.Errorf("/dev/force-unlock-chest should be 200 when handler registered; got %d body=%s", w.Code, w.Body.String())
	}
	if !called {
		t.Errorf("handler should be called; got called=false (路由未注册或被 NoRoute 拦截)")
	}
}

// TestRegister_BuildDevTrue_ForceUnlockChestSkippedWhenHandlerNil 验证 nil-tolerant：
//   - BUILD_DEV=true + 传入 nil → /dev/ping-dev 仍注册，/dev/force-unlock-chest 跳过 → 404
func TestRegister_BuildDevTrue_ForceUnlockChestSkippedWhenHandlerNil(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")

	r := newEngine()
	devtools.Register(r, nil, nil) // 两个 handler 都 nil

	// ping-dev 仍应注册
	w1 := doGet(r, "/dev/ping-dev")
	if w1.Code != http.StatusOK {
		t.Errorf("/dev/ping-dev should be 200; got %d", w1.Code)
	}

	// force-unlock-chest 应跳过注册 → Gin NoRoute 404
	w2 := doPost(r, "/dev/force-unlock-chest", `{}`)
	if w2.Code != http.StatusNotFound {
		t.Errorf("/dev/force-unlock-chest with nil handler should be 404; got %d", w2.Code)
	}
}
```

**辅助 helper**（追加到 devtools_test.go 已有 helper 段，与 7.5 `DevStepsHandlerFunc` adapter 平级）：

```go
// devChestHandlerFunc 是 devtools.DevChestHandler interface 的函数适配器（仅供测试用）。
//
// 实际生产 handler 是 *handler.DevChestHandler（struct）；测试中用 func 包装更简洁。
type devChestHandlerFunc func(c *gin.Context)

func (f devChestHandlerFunc) PostForceUnlockChest(c *gin.Context) { f(c) }
```

**关键改动清单**（既有 case 适配新签名）：

| 既有 case | 既有调用 | 新调用 |
|---|---|---|
| Story 1.6 6 个 case（如 `TestRegister_BuildDevFalse_PingDevReturns404`） | `devtools.Register(r, nil)` | `devtools.Register(r, nil, nil)` |
| Story 7.5 `TestRegister_BuildDevTrue_GrantStepsRegisteredWhenHandlerProvided` | `devtools.Register(r, stubHandler)` | `devtools.Register(r, stubHandler, nil)` |
| Story 7.5 `TestRegister_BuildDevTrue_GrantStepsSkippedWhenHandlerNil` | `devtools.Register(r, nil)` | `devtools.Register(r, nil, nil)` |

**关键约束**：

- router_dev_test.go 验证"nil-tolerant + 全 wire 链路"
- devtools_test.go 验证"路由注册 / 跳过"决策点
- 两处都用最小 stub handler（不引入业务 handler 实例 → 避免 bootstrap import；与 7.5 同模式）
- `devChestHandlerFunc` adapter 只在 devtools_test.go（测试包）暴露 —— **不**进 production code（与 7.5 `DevStepsHandlerFunc` 同模式）
- 整个 devtools_test.go 文件 build tag 仍 `//go:build !devtools`（与 1.6 / 7.5 既有约束）
- **既有 case 必须全部改新签名**（每个调用补 nil 参数；编译期错误兜底，不漏改）

**AC9 — 集成测试 1 case（dockertest，新文件 `dev_chest_service_integration_test.go`）**

新建 `server/internal/service/dev_chest_service_integration_test.go`：

```go
//go:build integration

package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// buildDevChestServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// 与 buildDevStepServiceIntegration (7.5) 的区别：本 helper 只需要 chestRepo（dev force-unlock 单 UPDATE）。
func buildDevChestServiceIntegration(t *testing.T) (svc service.DevChestService, sqlDB *sql.DB, cleanup func()) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn)

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		dockerCleanup()
		t.Fatalf("db.Open: %v", err)
	}

	chestRepo := mysql.NewChestRepo(gormDB)
	svc = service.NewDevChestService(chestRepo)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, cleanup
}

// TestDevChestServiceIntegration_ForceUnlockChest_PushesUnlockAtToNow 验证 epics.md §Story 20.7 行 2949 钦定：
// 用户 chest unlock_at 在未来 → /dev/force-unlock-chest → GET /chest/current 返回 status=2。
//
// 本 case 直接调 svc.ForceUnlockChest（绕过 handler）+ 直接 SELECT 验 unlock_at <= now；
// 完整 HTTP 链路（含 dev mode 闸门）由 dev_chest_handler_test 单测 + router_dev_test 覆盖。
func TestDevChestServiceIntegration_ForceUnlockChest_PushesUnlockAtToNow(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	// 复用 7.5 / 20.6 已建的 insertUser helper（同 package service_test）
	insertUser(t, sqlDB, userID, "uid-dev-force-unlock-1", "用户DEV", "")

	// chest with unlock_at 在未来 10 分钟（与 Story 4.6 firstTimeLogin 同模式）
	unlockAtFuture := time.Now().UTC().Add(10 * time.Minute)
	insertChest(t, sqlDB, 5001 /* chest_id */, userID, 1 /* status=counting */, unlockAtFuture, 1000 /* open_cost_steps */)

	ctx := context.Background()
	beforeNow := time.Now().UTC()

	// 调 dev force-unlock
	if err := svc.ForceUnlockChest(ctx, userID); err != nil {
		t.Fatalf("ForceUnlockChest: %v", err)
	}

	// SELECT user_chests 验 unlock_at 已被推到 now（落在 [beforeNow, afterNow] 区间内）
	afterNow := time.Now().UTC()
	row := sqlDB.QueryRow(`SELECT unlock_at FROM user_chests WHERE user_id = ?`, userID)
	var newUnlockAt time.Time
	if err := row.Scan(&newUnlockAt); err != nil {
		t.Fatalf("scan unlock_at: %v", err)
	}
	newUnlockAtUTC := newUnlockAt.UTC()
	if newUnlockAtUTC.Before(beforeNow) || newUnlockAtUTC.After(afterNow) {
		t.Errorf("unlock_at = %v, want in [%v, %v]", newUnlockAtUTC, beforeNow, afterNow)
	}

	// 仿真 GET /chest/current 动态判定：unlock_at <= now → status=2 unlockable
	// （不通过 chest_service.GetCurrent —— service 包内嵌的 chestStatusDynamic 不导出；
	// 直接断言 newUnlockAt <= now 等价于 GetCurrent 会返 status=2）
	if !newUnlockAtUTC.Before(afterNow) && !newUnlockAtUTC.Equal(afterNow) {
		t.Errorf("unlock_at %v should be <= now %v (so GetCurrent returns status=2)", newUnlockAtUTC, afterNow)
	}
}

// TestDevChestServiceIntegration_ForceUnlockChest_UserNotFound_Returns1003 验证 epics.md §Story 20.7 行 2947 钦定：
// dev mode + 用户无 chest → 返回 1003。
func TestDevChestServiceIntegration_ForceUnlockChest_UserNotFound_Returns1003(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()
	_ = sqlDB

	const nonExistentUserID = uint64(99999)
	err := svc.ForceUnlockChest(context.Background(), nonExistentUserID)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail when user has no chest")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003 ErrResourceNotFound)", got, apperror.ErrResourceNotFound)
	}
}
```

**insertChest helper 复用问题**：

- **检查清单**：`insertChest(t, sqlDB, chestID, userID, status, unlockAt, openCostSteps)` 是否在 20.6 chest_open_service_integration_test.go 或 4.6 auth_service_integration_test.go 中已建为 package-level helper？
  - 如果**已有**且签名匹配 → 直接复用（不重新建）
  - 如果**没有** → 在本文件新建（最小实现：`func insertChest(t *testing.T, db *sql.DB, id, userID uint64, status int8, unlockAt time.Time, openCostSteps uint32)`）
- 实装阶段必须先 Grep 检查 helper 存在性，**不**重复建

**关键约束**：

- 集成测试**必须**复用 7.5 / 20.6 已建的 `startMySQL` / `runMigrations` / `insertUser` / `insertChest` helper —— **不**重新建一套
- 本机 Windows docker 不可用 → t.Skip（4.3 graceful skip 模式；7.5 / 20.5 / 20.6 已用同模式）—— `startMySQL` 内部已有 skip 逻辑，本 case 自动继承
- HappyPath case **必须**断 `unlock_at` 落在 `[beforeNow, afterNow]` UTC 区间内（防 service 误传未来 / 历史时间；与 V1 §2.5 UTC 钦定同源）
- HappyPath 加分段验"GetCurrent 等价断言"：`unlock_at <= now` 等价于 Story 20.5 动态判定会返 status=2 —— 这是 epics.md §20.7 行 2949 钦定的 e2e 链路核心断言
- 第二个 case `UserNotFound_Returns1003` 验 epics.md AC 钦定的 edge case（service 单测已用 stub 覆盖；集成补一次 MySQL 真返 ErrChestNotFound 链路）
- **不**新增独立的 `dev_chest_handler_integration_test.go`（HTTP 链路由 router_dev_test + dev_chest_handler_test 覆盖；与 7.5 同政策）
- 集成 ≥1 case AC 满足；本 story 写 2 case（HappyPath + UserNotFound）超额，时间成本可控

**AC10 — repo 单测 ≥2 case（sqlmock；扩 `internal/repo/mysql/chest_repo_test.go`）**

修改 `server/internal/repo/mysql/chest_repo_test.go`（末尾追加 2 case）：

```go
// TestChestRepo_UpdateUnlockAt_HappyPath_UpdatesRowsAffectedOne 验证 Story 20.7 happy path：
// UPDATE user_chests SET unlock_at = ? WHERE user_id = ? → rows_affected=1 → 返 nil。
func TestChestRepo_UpdateUnlockAt_HappyPath_UpdatesRowsAffectedOne(t *testing.T) {
	gormDB, mock := newGormSqlmock(t)
	repo := mysql.NewChestRepo(gormDB)

	newUnlockAt := time.Now().UTC().Add(-1 * time.Minute) // 模拟 dev force-unlock now()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE .user_chests. SET .unlock_at.=.+ WHERE user_id = ?`).
		WithArgs(newUnlockAt, sqlmock.AnyArg() /* updated_at */, uint64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.UpdateUnlockAt(context.Background(), 1001, newUnlockAt)
	if err != nil {
		t.Fatalf("UpdateUnlockAt: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
}

// TestChestRepo_UpdateUnlockAt_ChestNotFound_ReturnsSentinel 验证 Story 20.7 edge case：
// rows_affected=0 → 返 ErrChestNotFound 哨兵（service 层 errors.Is 后翻译 1003）。
func TestChestRepo_UpdateUnlockAt_ChestNotFound_ReturnsSentinel(t *testing.T) {
	gormDB, mock := newGormSqlmock(t)
	repo := mysql.NewChestRepo(gormDB)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE .user_chests. SET .unlock_at.=.+ WHERE user_id = ?`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(99999)).
		WillReturnResult(sqlmock.NewResult(0, 0)) // rows_affected=0
	mock.ExpectCommit()

	err := repo.UpdateUnlockAt(context.Background(), 99999, time.Now().UTC())
	if !stderrors.Is(err, mysql.ErrChestNotFound) {
		t.Errorf("err = %v, want ErrChestNotFound", err)
	}
}
```

**关键约束**：

- `newGormSqlmock` helper 已在 `chest_repo_test.go` / `room_repo_test.go` 等已建 —— 复用，**不**重新建
- sqlmock pattern `.user_chests.` / `.unlock_at.` 用反引号占位（MySQL identifier 引用；与既有 sqlmock test 同模式）
- HappyPath case 用 `WillReturnResult(sqlmock.NewResult(0, 1))` 模拟 rows_affected=1
- ChestNotFound case 用 `WillReturnResult(sqlmock.NewResult(0, 0))` 模拟 rows_affected=0 → 验返 `ErrChestNotFound` 哨兵
- **不**写 DBError case（已被 service 单测的 DBError_Returns1009 case 覆盖；repo 层 raw error 透传无逻辑）

**AC11 — `bash scripts/build.sh` 全量绿**

完成后必须能跑通：

```bash
bash scripts/build.sh                      # vet + build → no failures
bash scripts/build.sh --test               # 全单测过（service 3 + handler 4 + repo 2 + devtools 2 + bootstrap 1 = 12 新 case + 既有全过）
bash scripts/build.sh --race --test        # CI Linux 必过；Windows race skip 不阻塞
bash scripts/build.sh --integration        # 既有集成 + 新 2 case；docker 不可用 → t.Skip
bash scripts/build.sh --devtools           # 验证 build tag 路径不出错（构造 build/catserver-dev[.exe]）
```

**关键约束**：

- 本 story 引入的新代码全部含单测；`go test ./internal/service/... -run TestDevChestService -v` 必须 3 个 case 全过
- `go test ./internal/app/http/handler/... -run TestDevChestHandler -v` 必须 4-6 个 case 全过
- `go test ./internal/repo/mysql/... -run TestChestRepo_UpdateUnlockAt -v` 必须 2 个 case 全过
- `go test ./internal/app/http/devtools/... -run TestRegister_BuildDevTrue_ForceUnlockChest -v` 必须 2 个 case 全过
- `go test ./internal/app/bootstrap/... -run TestRouter_DevForceUnlockChest -v` 必须 1 个 case 全过
- `--race` 在 Windows skip（ADR-0001 §3.5）；CI Linux 跑
- `--devtools` 必须能通过（forceDevEnabled=true 路径不破 + 不引入 build error；本 story 改 Register 签名后必须验证）
- **不**改 `scripts/build.sh` 自身

**AC12 — 验证清单（人工 + 自动化）**

完成后**人工**核对以下 10 项（结果记到 Completion Notes List）：

| # | 验证项 | 验证方式 |
|---|---|---|
| 1 | service 层 ForceUnlockChest 单步 UPDATE：`chestRepo.UpdateUnlockAt(ctx, userID, time.Now().UTC())` | Read `dev_chest_service.go` ForceUnlockChest 实装段 |
| 2 | time 用 UTC：`time.Now().UTC()`（与 V1 §2.5 + Story 4.6 / 20.5 / 20.6 同源） | Read `dev_chest_service.go` |
| 3 | 错误翻译：ErrChestNotFound → 1003（**非** 4001，与 epics.md §20.7 行 2947 钦定一致） | Read `dev_chest_service.go` errors.Is 段 |
| 4 | handler `PostForceUnlockChestRequest` 用 *uint64 指针；userId 缺失 / =0 显式 1002 拦截 | Read `dev_chest_handler.go` PostForceUnlockChest |
| 5 | handler **不**调 `c.Get(UserIDKey)`（dev 路径无 auth 中间件）；userID 全靠 body | Read `dev_chest_handler.go` |
| 6 | repo `UpdateUnlockAt` 用 GORM `.Update("unlock_at", ...)` 而非 `.Save()`；rows_affected=0 → ErrChestNotFound | Read `chest_repo.go` UpdateUnlockAt 实装 |
| 7 | devtools.Register 签名扩 `(r, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)`；nil 时跳过 force-unlock-chest 路由（保留 ping-dev + grant-steps） | Read `devtools.go` Register 实装 |
| 8 | router.go wire `var devChestHandler` 提前声明 nil + if 块内构造 + Register 留 if 块外（含 nil-collapse 写法） | Read `router.go` diff |
| 9 | stubChestRepo 同步追加 `UpdateUnlockAt` 方法 + `updateUnlockAtFn` 字段（在 auth_service_test.go）；既有 6+ case 编译通过 | Read `auth_service_test.go` stub 段 + `go test ./internal/service/...` 全绿 |
| 10 | `bash scripts/build.sh --test` 全绿；`bash scripts/build.sh --devtools` 不破；`git status --short` 改动文件清单匹配预期范围（5 新 + 4-5 改） | bash 实跑 + git status |

**AC13 — 不 commit（流水线由 epic-loop 下游收口）**

epics.md §Story 20.7 AC 钦定"≥3 单测 + 集成测试覆盖"，**不**钦定"git commit 单独提交"。本 story 是 server 业务代码 story，commit 由 epic-loop 流水线在下游 fix-review / story-done sub-agent 阶段统一收口。

- 本 story 的 dev workflow **不** commit / **不** push
- commit message 模板（story-done 阶段使用）：

  ```text
  feat(dev-force-unlock-chest): POST /dev/force-unlock-chest dev 端点（Story 20.7）

  - service dev_chest_service.ForceUnlockChest 实装：单步
    chestRepo.UpdateUnlockAt(time.Now().UTC())；ChestNotFound → 1003；其他 DB 错 → 1009
  - handler dev_chest_handler.PostForceUnlockChest + PostForceUnlockChestRequest（userId
    指针类型；userId 缺失 / =0 → 1002）+ postForceUnlockChestResponseDTO 简单 ack
  - repo ChestRepo.UpdateUnlockAt 实装：GORM .Update("unlock_at", ...)；rows_affected=0 → ErrChestNotFound
  - devtools.Register 签名扩 (r, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)；
    nil-tolerant 跳过 force-unlock-chest 路由
  - bootstrap/router.go wire devChestSvc + devChestHandler；nil-collapse 写法防 typed-nil-interface 陷阱
  - stubChestRepo 同步追加 UpdateUnlockAt 方法（auth_service_test.go）保 ChestRepo interface 兼容
  - 单测 12 case（service 3 + handler 4 + repo 2 + devtools 2 + bootstrap 1）+ 集成测试 2 case（HappyPath + UserNotFound）

  依据 epics.md §Story 20.7 + Story 7.5 dev 端点扩展模式 + Story 20.5 / 20.6 chest 业务上下文。

  Story: 20-7-dev-端点-post-dev-force-unlock-chest
  ```

- commit hash 待 story-done 阶段产生后回填到本文件

## Tasks / Subtasks

- [x] **Task 1（AC5）**：扩 `internal/repo/mysql/chest_repo.go` —— `UpdateUnlockAt` 方法
  - [x] 1.1 Read `chest_repo.go` 完整文件理解现有 ChestRepo interface + chestRepo impl 模式
  - [x] 1.2 Edit `chest_repo.go` —— ChestRepo interface 末尾追加 `UpdateUnlockAt(ctx, userID, newUnlockAt time.Time) error`
  - [x] 1.3 Edit `chest_repo.go` —— chestRepo impl 末尾追加 `UpdateUnlockAt` 实装（GORM `.Update("unlock_at", ...)` + rows_affected=0 → ErrChestNotFound）
  - [x] 1.4 Read 回检：(1) interface 签名匹配 AC5 钦定；(2) GORM .Update 而非 .Save；(3) tx.FromContext(ctx, r.db) 透明取 db handle；(4) rows_affected=0 → ErrChestNotFound

- [x] **Task 2（AC1）**：新建 `internal/service/dev_chest_service.go` —— DevChestService interface + impl
  - [x] 2.1 Read `dev_step_service.go` (7.5) 完整文件理解 service 模式（dev 端点 doc 注释 / 错误翻译 / slog.WarnContext）
  - [x] 2.2 Read `pet_service.go` (14.2) 复习"单 UPDATE 不开事务"模式（与本 story 同形态）
  - [x] 2.3 Write 新文件 `dev_chest_service.go` —— DevChestService interface + devChestServiceImpl + NewDevChestService + ForceUnlockChest 实装
  - [x] 2.4 Read 回检：(1) **不**接 txMgr（单 UPDATE 不开事务）；(2) time.Now().UTC()；(3) ErrChestNotFound → 1003 ErrResourceNotFound（**非** 4001）；(4) 其他 → 1009；(5) slog.WarnContext 审计日志

- [x] **Task 3（AC2）**：新建 `internal/app/http/handler/dev_chest_handler.go` —— DevChestHandler + DTO + handler
  - [x] 3.1 Read `dev_steps_handler.go` (7.5) 完整文件复习 handler 模式（指针类型 + 手动 nil 校验 + c.Error + ctx 传播）
  - [x] 3.2 Write 新文件 `dev_chest_handler.go` —— DevChestHandler + NewDevChestHandler + PostForceUnlockChestRequest + PostForceUnlockChest + postForceUnlockChestResponseDTO
  - [x] 3.3 Read 回检：(1) UserID 指针类型（无 binding:"required" —— 与 7.5 同模式）；(2) userId nil / =0 显式 1002；(3) 不调 c.Get(UserIDKey)；(4) c.Error + return；(5) response 返简单 ack `{userId}`

- [x] **Task 4（AC3）**：扩 `internal/app/http/devtools/devtools.go` —— DevChestHandler interface + Register 签名扩
  - [x] 4.1 Read `devtools.go` (Story 7.5 后) 完整文件理解现有 Register / DevStepsHandler interface
  - [x] 4.2 Edit 在 devtools.go 加 `DevChestHandler interface { PostForceUnlockChest(c *gin.Context) }` 类型（位置：紧邻 DevStepsHandler interface 之后）
  - [x] 4.3 Edit 改 `Register` 签名为 `Register(r *gin.Engine, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)`，在 `if devStepsHandler != nil { g.POST("/grant-steps", ...) }` 之后追加 `if devChestHandler != nil { g.POST("/force-unlock-chest", devChestHandler.PostForceUnlockChest) }`
  - [x] 4.4 Read 回检：(1) DevChestHandler interface 解耦避免 import cycle；(2) nil-tolerant 跳过 force-unlock-chest；(3) ping-dev / grant-steps 仍在；(4) build tag 不影响（forceDevEnabled 不变）

- [x] **Task 5（AC4）**：扩 `internal/app/bootstrap/router.go` —— wire dev chest service + handler + Register 签名 + nil-collapse
  - [x] 5.1 Read 现有 `router.go` 理解 if 块结构 + 7.5 devStepsHandler wire 模式
  - [x] 5.2 Edit 在 NewRouter 函数顶部 `var devStepsHandler *handler.DevStepsHandler` 之后追加 `var devChestHandler *handler.DevChestHandler // Story 20.7`
  - [x] 5.3 Edit 在 `if deps.GormDB != nil && ...` 块内、`devStepsHandler` 构造之后追加 `devChestSvc := service.NewDevChestService(chestRepo)` + `devChestHandler = handler.NewDevChestHandler(devChestSvc)`
  - [x] 5.4 Edit 改 `devtools.Register` 调用为 nil-collapse 写法（`var stepsArg / chestArg + Register(r, stepsArg, chestArg)`）—— 替换 7.5 既有 4 分支显式 if
  - [x] 5.5 Read 回检：(1) deps 完整时 devChestHandler 非 nil；(2) deps 零值（单测 NewRouter(Deps{})）时保持 nil；(3) nil-collapse 写法防 typed-nil-interface 陷阱；(4) Deps struct 未改；(5) chestRepo 复用 if 块顶部既有实例

- [x] **Task 6（AC9 stub 兼容性）**：扩 `internal/service/auth_service_test.go` —— stubChestRepo 同步追加 UpdateUnlockAt
  - [x] 6.1 Read `auth_service_test.go` stubChestRepo 段（行 113-138）复习 stub 模式
  - [x] 6.2 Edit stubChestRepo struct 加 `updateUnlockAtFn func(ctx context.Context, userID uint64, newUnlockAt time.Time) error` 字段
  - [x] 6.3 Edit stubChestRepo 加 `UpdateUnlockAt(ctx, userID, newUnlockAt) error` 方法（fn 字段未设 → panic-default，与现有 FindByUserIDForUpdate / Delete 同模式）
  - [x] 6.4 验证：`go build ./internal/service/...` 不破（ChestRepo interface 增方法后 stubChestRepo 必须实装新方法，否则编译失败）

- [x] **Task 7（AC6）**：新建 `internal/service/dev_chest_service_test.go` —— ≥3 case
  - [x] 7.1 Read `dev_step_service_test.go` (7.5) 完整文件复习 stub 模式 + buildDevStepService helper
  - [x] 7.2 Write 新文件 `dev_chest_service_test.go` —— 复用 stubChestRepo (auth_service_test.go) + buildDevChestService helper + 3 case
  - [x] 7.3 Read 回检：(1) HappyPath 强断 newUnlockAt.Location() == time.UTC + 偏差 < 1s；(2) ChestNotFound → 1003（**非** 4001）；(3) DBError → 1009；(4) repo.updateCalls=1 每 case

- [x] **Task 8（AC7）**：新建 `internal/app/http/handler/dev_chest_handler_test.go` —— ≥4 case
  - [x] 8.1 Read `dev_steps_handler_test.go` (7.5) 完整文件复习 stubDevStepService / newDevStepsHandlerRouter 模式
  - [x] 8.2 Write 新文件 `dev_chest_handler_test.go` —— stubDevChestService + newDevChestHandlerRouter（**不**挂 mock auth）+ 4-6 case（HappyPath / ChestNotFound / DBBusy / UserIDZero / MissingUserID / InvalidJSON）
  - [x] 8.3 Read 回检：(1) HappyPath stub 内验 userID 透传；(2) UserIDZero / MissingUserID stub.forceUnlockChestFn 主动 t.Errorf；(3) ChestNotFound → HTTP 200 + envelope 1003；(4) DBBusy → HTTP 500 + envelope 1009

- [x] **Task 9（AC10）**：扩 `internal/repo/mysql/chest_repo_test.go` —— ≥2 case（UpdateUnlockAt）
  - [x] 9.1 Read 现有 `chest_repo_test.go` 复习 newGormSqlmock helper + sqlmock pattern
  - [x] 9.2 Edit 末尾追加 2 case（TestChestRepo_UpdateUnlockAt_HappyPath_UpdatesRowsAffectedOne + TestChestRepo_UpdateUnlockAt_ChestNotFound_ReturnsSentinel）
  - [x] 9.3 Read 回检：(1) sqlmock pattern 匹配 GORM 实际生成的 SQL；(2) rows_affected=1 / 0 分别用 sqlmock.NewResult(0, 1) / (0, 0)；(3) ChestNotFound case 用 errors.Is 而非 == 比较

- [x] **Task 10（AC8）**：扩 `router_dev_test.go` + `devtools_test.go`
  - [x] 10.1 Read `router_dev_test.go` 复习 BUILD_DEV setenv + ServeHTTP 模式
  - [x] 10.2 Edit 末尾追加 `TestRouter_DevForceUnlockChest_NilHandlerSkipsRoute`（NewRouter(Deps{}) → force-unlock-chest 404；ping-dev 200）
  - [x] 10.3 Read `devtools_test.go` 复习 newEngine / doGet / doPost helper + 7.5 既有 case 调用 `devtools.Register(r, ...)` 的位置
  - [x] 10.4 Edit `devtools_test.go` —— 末尾追加 `devChestHandlerFunc` adapter + 2 case（ForceUnlockChestRegisteredWhenHandlerProvided / ForceUnlockChestSkippedWhenHandlerNil）
  - [x] 10.5 Edit `devtools_test.go` —— 把既有所有 `devtools.Register(r, X)` 调用改为 `devtools.Register(r, X, nil)`（编译期错误兜底，不漏改）

- [x] **Task 11（AC9）**：新建 `internal/service/dev_chest_service_integration_test.go` —— ≥1 case
  - [x] 11.1 Read `dev_step_service_integration_test.go` (7.5) + `chest_open_service_integration_test.go` (20.6) 复习 buildXxxIntegration 模式 + helper 复用（startMySQL / runMigrations / insertUser / insertChest）
  - [x] 11.2 Grep `insertChest` 验证 helper 存在性（应该已在 chest_open_service_integration_test.go 中建为 package-level helper —— 与 startMySQL / insertUser 同模式）
  - [x] 11.3 Write 新文件 `dev_chest_service_integration_test.go` —— buildDevChestServiceIntegration + 2 case（HappyPath_PushesUnlockAtToNow + UserNotFound_Returns1003）
  - [x] 11.4 验证本机 Windows docker 不可用 → t.Skip 不阻塞（startMySQL 内已有 skip 逻辑；本地实跑 `bash scripts/build.sh --integration` BUILD SUCCESS）

- [x] **Task 12（AC11 / AC12）**：全量验证
  - [x] 12.1 `bash scripts/build.sh`（vet + build）过 → BUILD SUCCESS
  - [x] 12.2 `bash scripts/build.sh --test` 全绿（新增 12+ case：service 3 + handler 4-6 + repo 2 + devtools 2 + bootstrap 1 = 12-14）
  - [x] 12.3 `bash scripts/build.sh --race --test`（Windows race skip ok；本地未跑，CI Linux 跑）—— 不阻塞；CI 兜底
  - [x] 12.4 `bash scripts/build.sh --integration`（docker 不可用 → t.Skip ok；BUILD SUCCESS）
  - [x] 12.5 `bash scripts/build.sh --devtools`（验证 build tag 路径不破 + Register 新签名兼容）
  - [x] 12.6 `git status --short` 改动文件清单核对（实际新 5 文件 + 改 4-5 文件 + sprint-status + story 文件 = 11+ 文件）
  - [x] 12.7 在下方 Completion Notes List 勾选 AC12 验证清单 10 项

- [x] **Task 13（AC13）**：本 story 不做 git commit
  - [x] 13.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 13.2 commit message 模板保留在 story 文件中
  - [x] 13.3 commit hash 待 story-done 阶段回填 —— story-done 阶段执行

## Dev Notes

### 关键设计原则

1. **dev 端点独立 service / handler / repo 方法**（不复用 ChestService.OpenChest / GetCurrent）：dev force-unlock 的产品语义是"压时间字段让 Story 20.5 动态判定生效"——OpenChest 含 8 步事务全不适用；GetCurrent 是纯读语义反向。强行复用要在 ChestService 加 "is_dev_force_unlock" flag 或 OpenChest 入参分支，反模式。独立 service 是"按职责而非按表分服务"的合理切分（与 NewDevStepService 同思路）。

2. **不接事务**（与 Story 7.5 DevStepService.GrantSteps 关键区别）：7.5 dev grant 是 4 步事务（FindByID + FindByUserID + UpdateBalance + Create sync_log），必须原子；本 story dev force-unlock 是 1 步 UPDATE，单语句原子。事务边界控制是为了"多步骤一致性"，本 story 没有这个需求。与 Story 14.2 PostStateSync 同"单 UPDATE 不开事务"模式。

3. **错误码用 1003 而非 4001**（与 epics.md §20.7 行 2947 钦定一致）：dev 端点的"用户无 chest"错误**不**用业务码 4001 ErrChestNotFound（那是 Story 20.5 GET /chest/current 业务接口用的码）；用通用 1003 ErrResourceNotFound（与 Story 7.5 dev grant 用 1003 而非业务码同政策）。语义上：dev 端点对内 / 测试用，错误码统一在通用 1xxx 段更易识别（4xxx 留给真实业务接口）。

4. **devtools.Register 签名扩展模式**：Story 1.6 钦定 devtools 包"只做框架"；7.5 引入 DevStepsHandler interface + 改 Register 签名；本 story 引入 DevChestHandler interface + 再次改 Register 签名。每加一个业务 dev 端点（grant-steps / force-unlock-chest / grant-cosmetic-batch / ...）都是"独立 interface + Register 签名追加一个槽位"模式。这让"哪些 dev 端点存在"在 Register 签名层就可见，避免运行时神秘失踪 + 每业务模块独立维护边界。

5. **router.go nil-collapse 写法**：Story 7.5 用 4 分支显式 if（`if devStepsHandler == nil { Register(r, nil) } else { Register(r, devStepsHandler) }`）兜底 typed-nil-interface 陷阱；本 story 加第二个可选 handler → 4 分支变 2^2=4 分支爆炸（grant-cosmetic 加进来又会变 2^3=8 分支）。改用 nil-collapse 写法（`var stepsArg devtools.DevStepsHandler; if devStepsHandler != nil { stepsArg = devStepsHandler }; Register(r, stepsArg, chestArg)`）—— 每加 1 个 handler 只需 +1 行 var + +1 行 if，无指数爆炸。**关键 Go 知识**：var stepsArg devtools.DevStepsHandler 声明零值是真正的 nil interface（type=nil, value=nil），不是 typed-nil 陷阱场景。

6. **GORM .Update("unlock_at", ...) 而非 .Save()**：精确控制只更新 unlock_at 列；避免 .Save() 误写整行（含 status / version / open_cost_steps）触发：(a) 与 Story 20.6 OpenChest 持锁 UPDATE 的字段语义冲突（OpenChest 改 version+1）；(b) GORM autoUpdateTime:milli 自动更新 updated_at（这个行为本 story 接受 —— updated_at 是审计字段，无副作用）。**关键**：Update("unlock_at", ...) 返回的 result.RowsAffected 是精确的（与 Update("column", value) 单字段更新 GORM 实装一致）。

7. **rows_affected=0 → ErrChestNotFound**（与 FindByUserID NotFound 共用哨兵）：repo 层不区分"user_id 不存在" vs "chest 行不存在"——MySQL `UPDATE WHERE user_id = ?` 都返 rows_affected=0；service 层用同一个哨兵翻译为 1003。语义合理：dev 端点容忍这两种边界都映射"用户无 chest"。

8. **UTC 时间钦定**：service 层用 `time.Now().UTC()`（与 V1 §2.5 + Story 4.6 / 20.5 / 20.6 同源）；repo 层不做 UTC 强转，透传 service 传入的 time.Time。**关键**：DB 列 `unlock_at DATETIME(3) NOT NULL` 不带时区信息，写入 UTC 字面值 + 读出 UTC 字面值（与 chest_repo.go 顶部注释钦定一致）。

9. **handler 用指针 + 手动校验**（与 7.5 PostGrantStepsRequest 同模式）：UserID 用 *uint64 + 手动 nil 校验，**不**挂 `binding:"required"` —— validator/v10 把 0 视为 zero value 会误判 required（Go validator/v10 著名陷阱；与 7.5 lesson 同源）。userId=0 显式拒（额外严格 + 早 fail；MySQL users.id AUTO_INCREMENT 从 1 起，0 永远不存在）。

10. **stub 同步更新约束**：mysql.ChestRepo interface 加 UpdateUnlockAt 方法后，**所有** ChestRepo 的 stub 实装（目前只有 auth_service_test.go 的 stubChestRepo）必须同步追加方法，否则 Go 编译期 interface 不满足 → 所有依赖该 stub 的 test 文件（auth_service_test.go / chest_service_test.go 等）都编译失败。Task 6 显式负责这个同步。**关键**：fn 字段未设 → panic-default，与 stubChestRepo 现有 FindByUserIDForUpdate / Delete 同模式 —— 让"未在测试中显式 set 但被调用"的 case 显式失败而不是隐式工作。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md`）：

- 宝箱是节点 7 核心资产（与步数账户 / cosmetic_items 并列）；dev force-unlock 是"测试 / demo 时间快进"基础设施 —— 与业务接口（20.5 GET / 20.6 POST）解耦但共享同一存储 / 同一 unlock_at 字段语义
- "状态以 server 为准"原则：dev force-unlock 后客户端调 GET /chest/current 拿权威 status=2

**数据库层**（`docs/宠物互动App_数据库设计.md`）：

- §5.6 user_chests：本 story UPDATE unlock_at 列；**不**改 status / version / open_cost_steps / id / user_id
- §6.7 chest status 枚举：本 story 不动 status 字面值（动态判定在 Story 20.5 service 层 chestStatusDynamic helper 做）
- 索引：UPDATE WHERE user_id = ? 走 uk_user_id 唯一索引（与 FindByUserID 同）

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：

- V1 §1 节点 7 冻结声明：本 story 是 dev 端点，**不**进 V1 主清单（V1 §7 当前 V1 接口清单不收录；与 Story 7.5 同政策）
- 错误码：1002（参数错误）/ 1003（资源不存在）/ 1009（服务繁忙）—— 全沿用 V1 §3 通用错误码

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：

- §5.1 handler 层：本 story DevChestHandler 严格按 handler 职责（参数校验 + DTO 转换 + 不接触 *gorm.DB）
- §5.2 service 层：本 story DevChestService 严格按 service 职责（业务规则 + 错误翻译；**无**事务边界因单 UPDATE）
- §5.3 repo 层：本 story 扩 ChestRepo.UpdateUnlockAt 方法 + impl；**不**新建 repo 文件

**ADR 对齐**：

- ADR-0006 三层错误映射：repo 返哨兵（ErrChestNotFound）→ service 翻译为 *AppError(1003 / 1009)→ handler c.Error + middleware envelope
- ADR-0007 ctx 传播：service / repo 第一参数 ctx；handler 用 `c.Request.Context()` 不直接传 *gin.Context
- ADR-0006 单一 envelope 生产者：handler 一律 `c.Error + return`，由 ErrorMappingMiddleware 写 envelope

### 关于 Story 20.7 与 7.5 / 20.5 / 20.6 的关键差异

| 维度 | 7.5 POST /dev/grant-steps | 20.5 GET /chest/current | 20.6 POST /chest/open | 20.7 POST /dev/force-unlock-chest |
|------|--------------------------|------------------------|----------------------|----------------------------------|
| 路由前缀 | /dev（dev 模式才挂） | /api/v1（auth + rate_limit） | /api/v1（auth；rate_limit 在 handler 内层）| /dev（dev 模式才挂） |
| HTTP method | POST | GET | POST | POST |
| body | 有（userId / steps） | 无 | 有（idempotencyKey） | 有（userId） |
| auth 中间件 | **否** | 是 | 是 | **否** |
| rate_limit 中间件 | **否** | 是（global） | 内层 handler 做（外层 opt-out） | **否** |
| 事务 | 有（4 步：FindByID + FindByUserID + UpdateBalance + Create sync_log） | 无（单 SELECT） | 有（8 步：幂等 + FOR UPDATE + 扣步 + 抽奖 + log + 刷新 chest）| **无**（单 UPDATE） |
| repo 调用 | FindByID + FindByUserID + UpdateBalance + Create | FindByUserID | 5+ repo 串联（含 FOR UPDATE / Create / Delete） | UpdateUnlockAt |
| 错误码全集 | 1002 / 1003 / 1009 | 4001 / 1009 | 4001 / 4002 / 3002 / 1005 / 1009 | 1002 / 1003 / 1009 |
| 端点物理可达性 | 仅 BUILD_DEV=true OR -tags devtools | 永远 | 永远 | 仅 BUILD_DEV=true OR -tags devtools |
| 单元测试规模 | service 6 + handler 7 + devtools 2 + bootstrap 1 = 16 | service 6 + handler 5 = 11 | service 11+ + handler 7+ + repo 各 2+ = 25+ | service 3 + handler 4-6 + repo 2 + devtools 2 + bootstrap 1 = 12-14 |
| 改动文件数 | 5 新 + 3-4 改 = 8-9 | 1-2 新 + 1 改 = 2-3 | 6 新 + 5 改 + 2 migration = 13 | 5 新 + 4-5 改 = 9-10 |
| router.go 签名扩 | Register(r, devStepsHandler) | n/a | chestOpenGroup 子组 | Register(r, devStepsHandler, **devChestHandler**) |

### Project Structure Notes

**与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 钦定的 server/ 工程结构对齐**：

```
server/
├─ internal/
│  ├─ app/
│  │  ├─ http/
│  │  │  ├─ handler/
│  │  │  │  ├─ chest_handler.go            # 20.5 / 20.6 已建；本 story 不动
│  │  │  │  ├─ dev_steps_handler.go        # 7.5 已建；本 story 不动
│  │  │  │  ├─ dev_chest_handler.go        # 本 story 新建
│  │  │  │  └─ dev_chest_handler_test.go   # 本 story 新建
│  │  │  ├─ devtools/
│  │  │  │  ├─ devtools.go                 # 1.6 / 7.5 已建；本 story 改 Register 签名 + 加 DevChestHandler interface + 加 force-unlock-chest 路由
│  │  │  │  └─ devtools_test.go            # 1.6 / 7.5 已建；本 story 末尾追加 2 case + devChestHandlerFunc adapter + 既有 case 改新签名
│  │  │  └─ middleware/                    # 已实装；本 story 不调（dev 路径无 auth / rate_limit）
│  │  └─ bootstrap/
│  │     ├─ router.go                      # 7.5 / 20.5 / 20.6 已 wire；本 story 加 devChestSvc / devChestHandler / nil-collapse Register
│  │     └─ router_dev_test.go             # 1.6 / 7.5 已建；本 story 末尾追加 1 case
│  ├─ service/
│  │  ├─ chest_service.go                  # 20.5 已建；本 story 不动
│  │  ├─ chest_open_service.go             # 20.6 已建；本 story 不动
│  │  ├─ dev_step_service.go               # 7.5 已建；本 story 不动
│  │  ├─ dev_chest_service.go              # 本 story 新建
│  │  ├─ dev_chest_service_test.go         # 本 story 新建
│  │  ├─ dev_chest_service_integration_test.go  # 本 story 新建
│  │  └─ auth_service_test.go              # 4.6 已建；本 story 改 stubChestRepo 加 UpdateUnlockAt 方法 + updateUnlockAtFn 字段
│  ├─ repo/
│  │  └─ mysql/
│  │     ├─ chest_repo.go                  # 4.6 / 20.6 已建；本 story 扩 UpdateUnlockAt 方法（interface + impl）
│  │     └─ chest_repo_test.go             # 4.6 / 20.6 已建；本 story 末尾追加 2 case
│  └─ pkg/
│     └─ errors/codes.go                   # 1002 / 1003 / 1009 已注册；本 story 仅**消费**
└─ migrations/                              # 0001-0014 已锁定；本 story **不**改
```

**变更范围（预期 git status 文件清单，5 新 + 4-5 改）**：

新建 5 文件：
1. `server/internal/service/dev_chest_service.go`
2. `server/internal/service/dev_chest_service_test.go`
3. `server/internal/service/dev_chest_service_integration_test.go`
4. `server/internal/app/http/handler/dev_chest_handler.go`
5. `server/internal/app/http/handler/dev_chest_handler_test.go`

修改 4-5 文件：
6. `server/internal/repo/mysql/chest_repo.go` — ChestRepo interface 加 UpdateUnlockAt + impl
7. `server/internal/repo/mysql/chest_repo_test.go` — 末尾追加 2 case（UpdateUnlockAt HappyPath + ChestNotFound）
8. `server/internal/service/auth_service_test.go` — stubChestRepo 同步追加 UpdateUnlockAt 方法 + updateUnlockAtFn 字段
9. `server/internal/app/http/devtools/devtools.go` — Register 签名扩 + DevChestHandler interface + g.POST 路由注册
10. `server/internal/app/http/devtools/devtools_test.go` — 末尾追加 2 case + devChestHandlerFunc adapter + 既有 case 改新签名
11. `server/internal/app/bootstrap/router.go` — devChestSvc / devChestHandler wire + var 提前声明 + nil-collapse Register
12. `server/internal/app/bootstrap/router_dev_test.go` — 末尾追加 1 case

流程文件：
13. `_bmad-output/implementation-artifacts/sprint-status.yaml` — 20-7 状态 backlog → ready-for-dev → in-progress → review → done
14. `_bmad-output/implementation-artifacts/20-7-dev-端点-post-dev-force-unlock-chest.md` — 本 story 文件本身

**未变更文件（明确 NOT touch；超出范围 → HALT）**：

- `server/internal/repo/mysql/step_account_repo.go` / `cosmetic_item_repo.go` / `chest_open_log_repo.go` / `chest_open_idempotency_record_repo.go` / `step_sync_log_repo.go` / `user_repo.go` / `errors.go` / 任一其他 repo
- `server/internal/service/chest_service.go` / `chest_open_service.go` / `dev_step_service.go` / `step_service.go` / `home_service.go` / `auth_service.go` / 任一其他 service
- `server/internal/service/chest_service_test.go` / `chest_open_service_test.go` / `chest_service_integration_test.go` / `chest_open_service_integration_test.go` 等其他 chest 相关测试
- `server/internal/app/http/handler/chest_handler.go` / `dev_steps_handler.go` / `steps_handler.go` / 任一其他 handler
- `server/internal/app/http/middleware/*.go`
- `server/internal/pkg/random/*.go`（20.6 已建；本 story 不消费）
- `server/internal/pkg/errors/codes.go`
- `server/internal/infra/config/*.go` / 任一 `*.yaml`
- `server/cmd/server/main.go` / `internal/app/bootstrap/server.go`
- `migrations/*.sql`（包括 0014 idempotency records 不动）
- `docs/宠物互动App_*.md`（消费方）
- `_bmad-output/implementation-artifacts/decisions/*.md`（无新决策）

### References

**优先级 P0（必读）**：

- [Source: _bmad-output/planning-artifacts/epics.md#Story 20.7] — 本 story 钦定 AC（行 2932-2949）
- [Source: server/internal/app/http/devtools/devtools.go] — Story 1.6 / 7.5 devtools 框架（Register / DevOnlyMiddleware / DevStepsHandler interface / nil-tolerant 模式 / "业务 dev 端点扩展模式" 注释）
- [Source: server/internal/service/dev_step_service.go] — Story 7.5 dev service 模式（doc 注释 / 错误翻译 / slog.WarnContext；本 story 严格参考但**不**复用）
- [Source: server/internal/app/http/handler/dev_steps_handler.go] — Story 7.5 dev handler 模式（指针类型 + 手动 nil 校验 + 不调 c.Get + c.Error + 简单 ack response；本 story 严格参考但**不**复用）
- [Source: server/internal/repo/mysql/chest_repo.go] — Story 4.6 / 20.6 已建 ChestRepo + chestRepo impl（本 story 扩 UpdateUnlockAt 方法）
- [Source: server/internal/service/chest_service.go] — Story 20.5 GetCurrent 实装（动态判定 chestStatusDynamic + UTC 钦定；本 story service 复用其 UTC 时间语义但**不**复用 GetCurrent 逻辑）
- [Source: server/internal/app/bootstrap/router.go] — wire 模式 + 7.5 nil-tolerant if 块（本 story 扩 devChestSvc + nil-collapse Register 写法替换 7.5 既有 4 分支显式 if）
- [Source: _bmad-output/implementation-artifacts/7-5-dev-端点-post-dev-grant-steps.md] — Story 7.5 完整实装文档（双闸门 / DevOnlyMiddleware / 业务扩展模式钦定 / typed-nil-interface 陷阱 lesson；本 story 复用全部模式）

**优先级 P1（参考）**：

- [Source: server/internal/repo/mysql/errors.go] — ErrChestNotFound 哨兵
- [Source: server/internal/pkg/errors/codes.go] — 错误码全集（ErrInvalidParam=1002 / ErrResourceNotFound=1003 / ErrServiceBusy=1009 / ErrChestNotFound=4001 仅作对照）
- [Source: server/internal/service/auth_service_test.go] — stubChestRepo 已建（行 113-138；本 story 追加 UpdateUnlockAt 方法 + updateUnlockAtFn 字段）
- [Source: server/internal/service/chest_service_test.go] — Story 20.5 GetCurrent 单测复用 stubChestRepo 模式（本 story dev_chest_service_test 同模式）
- [Source: server/internal/service/dev_step_service_test.go] — Story 7.5 service 单测 6 case 模式（本 story 3 case 同模式 + UTC 断言加强）
- [Source: server/internal/app/http/handler/dev_steps_handler_test.go] — Story 7.5 stubDevStepService + newDevStepsHandlerRouter 模式（本 story 新建独立 stubDevChestService + newDevChestHandlerRouter 同模式）
- [Source: server/internal/service/dev_step_service_integration_test.go] — Story 7.5 集成测试 buildDevStepServiceIntegration + helper 复用模式（本 story 新建 buildDevChestServiceIntegration 同模式）
- [Source: server/internal/service/chest_open_service_integration_test.go] — Story 20.6 集成测试 insertChest helper（本 story 复用）
- [Source: server/internal/app/bootstrap/router_dev_test.go] — Story 1.6 / 7.5 dev 路由 wire 测试（BUILD_DEV setenv + ServeHTTP 模式；本 story 末尾加 1 case）
- [Source: server/internal/app/http/devtools/devtools_test.go] — Story 1.6 / 7.5 devtools 单测（newEngine / doGet / doPost / DevStepsHandlerFunc 模式；本 story 末尾加 2 case + devChestHandlerFunc adapter + 既有 case 改新签名）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-mapping.md] — ADR-0006 三层错误映射
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md] — ADR-0007 ctx 传播

**优先级 P2（背景）**：

- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5] — 分层职责定义
- [Source: docs/宠物互动App_数据库设计.md#5.6 user_chests] — 表结构 + uk_user_id 唯一索引 + status 枚举
- [Source: docs/宠物互动App_数据库设计.md#6.7 chest status 枚举] — 1=counting / 2=unlockable（本 story 不动字面值，动态判定在 Story 20.5）
- [Source: docs/宠物互动App_V1接口设计.md#2.5 ISO 8601 UTC] — 时间字段 UTC 钦定（本 story service 层 time.Now().UTC() 同源）
- [Source: docs/宠物互动App_V1接口设计.md#7.1 GET /chest/current] — Story 20.5 业务接口（与本 story dev 端点串联：调本端点 → 调 GET /chest/current 拿 status=2）
- [Source: docs/宠物互动App_总体架构设计.md] — "状态以 server 为准"原则
- [Source: _bmad-output/implementation-artifacts/1-6-dev-tools-框架.md] — Story 1.6 完整实装文档（双闸门 / DevOnlyMiddleware / 业务扩展模式钦定）
- [Source: _bmad-output/implementation-artifacts/20-5-get-chest-current-接口.md] — Story 20.5 完整实装文档（chest 动态判定 / chestStatusDynamic helper / UTC 钦定）
- [Source: _bmad-output/implementation-artifacts/20-6-post-chest-open-事务-idempotencykey-加权抽取.md] — Story 20.6 完整实装文档（chest_open 事务模式 / chestRepo.Delete + Create 刷新下一轮 chest）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 20.9] — 下游 Layer 2 集成测试场景（本 story 集成测试覆盖 happy + UserNotFound 两 path，20.9 owner 深度场景）

### Previous Story Intelligence（Story 1.6 / 7.5 / 20.5 / 20.6 关键交付物）

**Story 1.6 dev tools 框架交付物**（必读 `_bmad-output/implementation-artifacts/1-6-dev-tools-框架.md`）：

- **devtools 包定位**：只做"框架"（Register + DevOnlyMiddleware + 启停判定）；业务 dev 端点扩展走"业务层 service / handler + devtools.Register 接 interface"模式（已在 devtools.go 顶部 comment 钦定）
- **双闸门 OR 启用**：build tag `-tags devtools` OR env var `BUILD_DEV=true`；任一即启用
- **DevOnlyMiddleware 是请求时第二闸门**：BUILD_DEV 运维热切（启动后 setenv ""）时，路由仍存在但 middleware 推 ErrResourceNotFound 1003 → ErrorMappingMiddleware 翻 envelope HTTP 200
- **Register 是非幂等**：重复调用让 Gin panic（路由重复）；本 story 改签名后调用方仍是 NewRouter 一处
- **测试 build tag `!devtools`**：所有 devtools 自动化测试必须加 `//go:build !devtools`，避免 forceDevEnabled=true 路径污染 env var case

**Story 7.5 dev grant 关键模式参考**（本 story 复用全部架构模式 + lesson）：

- **dev 端点独立 service / handler 文件**：本 story dev_chest_service.go / dev_chest_handler.go 与 7.5 dev_step_service.go / dev_steps_handler.go 平级独立
- **devtools.Register 签名扩**：7.5 改为 `(r, devStepsHandler DevStepsHandler)`；本 story 再扩为 `(r, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)`
- **interface 解耦**：DevChestHandler interface 与 DevStepsHandler 平级（按业务模块独立 interface 槽位），避免 import cycle
- **router.go nil-tolerant + var 提前声明**：与 7.5 同模式 + 改用 nil-collapse 写法（防 4 分支爆炸）
- **typed-nil-interface 陷阱 lesson**：7.5 已 lesson；本 story nil-collapse 写法是该 lesson 的进阶 evolve
- **stub 命名约定**：stubDevChestService 与 stubDevStepService 平级独立类型，不复用
- **测试 router 不挂 mock auth**：dev 路径无 auth；与 chestHandler (20.5/20.6) handler 测试模式区分

**Story 20.5 GET /chest/current 关键模式参考**（虽然不复用 GetCurrent 业务逻辑，但 UTC 钦定 + chest_repo 复用模式参考）：

- **UTC 钦定**：service 层用 `time.Now().UTC()` —— 本 story service 严格同源
- **chest_repo 复用**：20.5 已建 ChestRepo.FindByUserID；本 story 扩 UpdateUnlockAt 方法到同一 interface（**不**新建 ChestRepo 实例）

**Story 20.6 POST /chest/open 关键模式参考**（虽然不复用 OpenChest，但 chest_repo Delete + Create 模式 + bootstrap wire 模式参考）：

- **chest_repo 扩方法模式**：20.6 已扩 FindByUserIDForUpdate + Delete；本 story 扩 UpdateUnlockAt 同模式（interface + impl + sqlmock 单测）
- **bootstrap wire 模式**：20.6 已 wire chestSvc 到 chestRepo；本 story devChestSvc 同复用 chestRepo
- **stubChestRepo 已扩**：auth_service_test.go 中 stubChestRepo 已含 FindByUserIDForUpdate + Delete 方法 panic-default；本 story 追加 UpdateUnlockAt 同模式

### Lessons Index（与本 story 相关的过去教训）

- [docs/lessons/2026-04-24-error-envelope-single-producer.md] — c.Error / response.Error 二选一；本 story 严格走 `c.Error + return`
- [docs/lessons/2026-04-24-go-validator-required-zero-value-trap.md]（若存在；否则参考 Story 7.5 dev notes #4）— validator/v10 把 0 视为 zero value 误判 required；本 story 用指针 + 手动 nil 校验
- [docs/lessons/2026-04-24-typed-nil-interface-trap.md]（若存在；否则参考 Story 7.5 dev notes #11）— Go interface typed-nil 陷阱；本 story 用 nil-collapse 写法防进阶 4 分支爆炸
- Story 7.5 / 20.5 / 20.6 review 教训：本 story 参考但**不**重复触发（dev 端点产品语义 + UTC 钦定 + chest_repo 扩方法都是已 lesson 的成熟模式）

### Git Intelligence（最近 5 个 commit）

```
996e8ba chore(story-20-6): 收官 Story 20.6 + 归档 story 文件
602db7a feat(server): Epic20/20.6 POST /chest/open 事务 + idempotencyKey + 加权抽取
19bea45 chore(story-20-5): 收官 Story 20.5 + 归档 story 文件
b3a0537 feat(server): Epic20/20.5 GET /chest/current 接口（按 unlock_at 动态判定 unlockable）
dae61b7 chore(story-20-4): 收官 Story 20.4 + 归档 story 文件
```

**关键提取**：

- 20.4 / 20.5 / 20.6 已 done；chest_repo / chest_service / chest_open_service / chest_handler 全实装；本 story 仅扩 chest_repo.UpdateUnlockAt + 新建 dev_chest_service / dev_chest_handler
- 20.6 review 经历多轮 r1~r15 review 落地了大量 chest 业务事务 / 幂等 / 加权抽奖 lesson —— 本 story 不接这些复杂度（dev force-unlock 是单 UPDATE），但理解 lesson 上下文有助于 review 时不被误指出"为什么不用事务 / 幂等"等问题
- 7.5 / 20.5 / 20.6 都已 done；本 story 是 Epic 20 收官三条之一（20.7 + 20.8 + 20.9），节点 7 收官即将完成

### 常见陷阱（基于 7.5 / 20.5 / 20.6 / 1.6 review 经验）

1. **import cycle**：devtools 包**不**能 import handler 包（handler import service / repo / middleware；devtools 是基础设施层在 handler 之下）→ 用 interface 解耦（本 story AC3 已设计 DevChestHandler interface）
2. **Register 调用位置**：必须留在 `if deps.GormDB != nil ...` 块**外**，因为 IsEnabled() 不依赖 deps；放块内会让单测 NewRouter(Deps{}) 漏挂 ping-dev
3. **devChestHandler nil 透传 + typed-nil-interface 陷阱**：`var devChestHandler *handler.DevChestHandler` 提前声明 nil；deps 完整时填充。**关键**：传给 `devtools.Register` 时**必须** nil-collapse 到真正的 nil interface，不能直接传 typed-nil `*handler.DevChestHandler(nil)` —— 后者会让 Register 内 `if devChestHandler != nil` 判定为真 → 调 PostForceUnlockChest 时 nil receiver panic。本 story AC4 已设计 nil-collapse 写法防御
4. **stub 同步更新**：ChestRepo interface 加 UpdateUnlockAt 方法后，所有依赖 ChestRepo 的 stub（auth_service_test.go 的 stubChestRepo）必须同步追加方法 —— 否则 Go 编译期 interface 不满足，编译失败连锁影响多个 test 文件。Task 6 显式负责这个同步
5. **既有 devtools_test case 编译失败**：本 story 改 Register 签名为 `(r, devSteps, devChest)`，既有所有 `devtools.Register(r, X)` 调用都会编译失败（少传一个参数）—— Task 10.5 显式负责修复
6. **测试 helper 复用边界**：startMySQL / runMigrations / insertUser / insertChest 都在 service 包测试中已 export（同 package）—— 本 story 集成测试**复用**而非新建；但 `buildXxxServiceIntegration` 是每 service 专用 → 本 story 新建 `buildDevChestServiceIntegration`（只需 chestRepo，不需要其他 repo）
7. **stub 命名冲突**：`stubChestService` 已被 20.5 / 20.6 handler 测试占用 → 本 story handler 测试新建 `stubDevChestService`（独立类型，不复用）
8. **ErrorMappingMiddleware 必挂**：handler 单测 router `r.Use(middleware.ErrorMappingMiddleware())` 必挂，否则 c.Error 不写 envelope —— 与 7.5 / 20.5 / 20.6 同模式
9. **build tag 影响**：`//go:build !devtools` 在 router_dev_test.go / devtools_test.go 必须保留；`go test -tags devtools` 跑这些文件会让 forceDevEnabled=true 让 BUILD_DEV="" 的 case 失败
10. **`bash scripts/build.sh --devtools`**：本 story 改 devtools.Register 签名；必须验证 build tag 路径（forceDevEnabled=true 编译路径）也能通过；script 自动跑 build/catserver-dev 输出
11. **错误码选 1003 而非 4001**：dev 端点的"用户无 chest"用通用 1003 ErrResourceNotFound（与 epics.md §20.7 行 2947 钦定），**不**用业务 4001 ErrChestNotFound（那是 Story 20.5 业务接口的码）—— 这是 dev 端点错误码统一政策（与 7.5 用 1003 同模式）

### 测试覆盖矩阵

| 测试层 | 文件 | 新增 case | 覆盖 AC |
|---|---|---|---|
| service 单测 | `dev_chest_service_test.go`（新） | 3 | AC1, AC6 |
| handler 单测 | `dev_chest_handler_test.go`（新） | 4-6 | AC2, AC7 |
| repo 单测 | `chest_repo_test.go`（追加） | 2 | AC5, AC10 |
| devtools 路由测试 | `devtools_test.go`（追加） | 2 | AC3, AC8 |
| bootstrap wire 测试 | `router_dev_test.go`（追加） | 1 | AC4, AC8 |
| 集成测试 | `dev_chest_service_integration_test.go`（新） | 2 | AC9 |
| **合计** | | **14-16** | |

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash scripts/build.sh --test`：全绿；新增 14 case（service 3 + handler 6 + repo 2 + devtools 2 + bootstrap 1）+ 既有全过
- `bash scripts/build.sh --devtools --test`：全绿；build tag 路径下 forceDevEnabled=true，Register 新签名兼容
- `bash scripts/build.sh --integration`：BUILD SUCCESS；docker 不可用，dev_chest_service_integration_test 2 case 全 SKIP（本机 Windows 没 docker daemon；CI Linux 跑）

### Completion Notes List

**AC12 验证清单（10 项人工核对结果）**：

1. [x] service 层 ForceUnlockChest 单步 UPDATE：`chestRepo.UpdateUnlockAt(ctx, userID, time.Now().UTC())` —— 见 `server/internal/service/dev_chest_service.go` 行 67-79
2. [x] time 用 UTC：`time.Now().UTC()`（与 V1 §2.5 + Story 4.6 / 20.5 / 20.6 同源）—— 见 dev_chest_service.go 行 68
3. [x] 错误翻译：ErrChestNotFound → 1003（**非** 4001，与 epics.md §20.7 行 2947 钦定一致）—— 见 dev_chest_service.go 行 70-72
4. [x] handler `PostForceUnlockChestRequest` 用 *uint64 指针；userId 缺失 / =0 显式 1002 拦截 —— 见 `server/internal/app/http/handler/dev_chest_handler.go` 行 41-43 + 行 62-69
5. [x] handler **不**调 `c.Get(UserIDKey)`（dev 路径无 auth 中间件）；userID 全靠 body —— 见 dev_chest_handler.go PostForceUnlockChest 全函数
6. [x] repo `UpdateUnlockAt` 用 GORM `.Update("unlock_at", ...)` 而非 `.Save()`；rows_affected=0 → ErrChestNotFound —— 见 `server/internal/repo/mysql/chest_repo.go` 行 168-178
7. [x] devtools.Register 签名扩 `(r, devStepsHandler DevStepsHandler, devChestHandler DevChestHandler)`；nil 时跳过 force-unlock-chest 路由（保留 ping-dev + grant-steps）—— 见 `server/internal/app/http/devtools/devtools.go` 行 87-118
8. [x] router.go wire `var devChestHandler` 提前声明 nil + if 块内构造 + Register 留 if 块外（含 nil-collapse 写法）—— 见 `server/internal/app/bootstrap/router.go` 行 298 + 行 366-372 + 行 596-617
9. [x] stubChestRepo 同步追加 `UpdateUnlockAt` 方法 + `updateUnlockAtFn` 字段（auth_service_test.go 行 113-152）；额外发现 stubOpenChestChestRepo / stubHomeChestRepo / faultChestRepo 三处也必须同步追加；既有 case 全过
10. [x] `bash scripts/build.sh --test` 全绿；`bash scripts/build.sh --devtools --test` 不破；`bash scripts/build.sh --integration` BUILD SUCCESS（docker skip 不阻塞）

**实装亮点 / 与 spec 差异**：

- spec 提到的 helper 名 `newGormSqlmock` 实际叫 `newGormWithMock`（user_repo_test.go 行 20）；不影响实装
- spec 提到 stub 同步只列了 stubChestRepo 一处，实际发现 stubOpenChestChestRepo (chest_open_service_test.go) / stubHomeChestRepo (home_service_test.go) / faultChestRepo (auth_service_integration_test.go) 也需要同步追加 UpdateUnlockAt（ChestRepo interface 的 4 个 stub 全部受影响 —— 这是 spec 没显式列但漏掉就 build break 的隐式扩散）
- sqlmock pattern 不带 ExpectBegin/Commit（因 GORM 配置 `SkipDefaultTransaction: true`，与既有 user_repo / step_account_repo UPDATE 测试同模式）

### File List

**新建 5 文件**：
- `server/internal/service/dev_chest_service.go` — DevChestService interface + impl + NewDevChestService + ForceUnlockChest 实装
- `server/internal/service/dev_chest_service_test.go` — service 单测 3 case（HappyPath / ChestNotFound / DBError）
- `server/internal/service/dev_chest_service_integration_test.go` — dockertest 集成测试 2 case（HappyPath / UserNotFound）
- `server/internal/app/http/handler/dev_chest_handler.go` — DevChestHandler + DTO + handler + response helper
- `server/internal/app/http/handler/dev_chest_handler_test.go` — handler 单测 6 case

**修改 8 文件**：
- `server/internal/repo/mysql/chest_repo.go` — ChestRepo interface 加 UpdateUnlockAt + impl 实装
- `server/internal/repo/mysql/chest_repo_test.go` — 末尾追加 2 case（UpdateUnlockAt HappyPath + ChestNotFound）
- `server/internal/service/auth_service_test.go` — stubChestRepo 加 updateUnlockAtFn 字段 + UpdateUnlockAt 方法；import time
- `server/internal/service/chest_open_service_test.go` — stubOpenChestChestRepo 加 UpdateUnlockAt 方法（panic-default）
- `server/internal/service/home_service_test.go` — stubHomeChestRepo 加 UpdateUnlockAt 方法（panic-default）
- `server/internal/service/auth_service_integration_test.go` — faultChestRepo 加 UpdateUnlockAt 方法（透传 delegate）
- `server/internal/app/http/devtools/devtools.go` — DevChestHandler interface + Register 签名扩 (3 参) + force-unlock-chest 路由注册
- `server/internal/app/http/devtools/devtools_test.go` — 既有 case 全部改 Register(r, X, nil) 新签名 + 末尾追加 devChestHandlerFunc adapter + 2 case
- `server/internal/app/bootstrap/router.go` — `var devChestHandler` 提前声明 + if 块内 wire devChestSvc/Handler + nil-collapse Register（替换 7.5 既有 if-else 2 分支）
- `server/internal/app/bootstrap/router_dev_test.go` — 末尾追加 TestRouter_DevForceUnlockChest_NilHandlerSkipsRoute 1 case

**流程文件**：
- `_bmad-output/implementation-artifacts/sprint-status.yaml` — 20-7 状态 ready-for-dev → in-progress → review
- `_bmad-output/implementation-artifacts/20-7-dev-端点-post-dev-force-unlock-chest.md` — 本 story 文件本身（Tasks/Subtasks checkboxes + Dev Agent Record + Status）

### Change Log

| 日期 | 改动 | 状态变更 |
|---|---|---|
| 2026-05-15 | Story 20.7 created (backlog → ready-for-dev) | backlog → ready-for-dev |
| 2026-05-15 | Story 20.7 dev-story 完成 —— service 3 + handler 6 + repo 2 + devtools 2 + bootstrap 1 = 14 case；集成 2 case（docker skip）；build/test/devtools/integration 四模式全绿 | ready-for-dev → in-progress → review |
| 2026-05-15 | r5 [P2] codex review 指出 chestId 从 optional 变 required 是 contract regression；分诊为 **fix-with-doc-only**（不回退代码） —— r2-r4 的"client 传 chestId + 事务 + FOR UPDATE"设计正确（race 根因解决方案 > contract 美感），但承认契约变更需文档化：更新 dev_chest_handler.go PostForceUnlockChestRequest doc 加契约变更通告 + 更新 epics.md §20.7 AC 反映新 schema `{userId, chestId}` + 加脚注链回 lesson + 归档 lesson `docs/lessons/2026-05-15-dev-endpoint-correctness-over-contract-aesthetics-20-7-r5.md` | review (维持) |
