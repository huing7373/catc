# Story 20.5: GET /chest/current 接口（按 unlock_at 动态判定 unlockable）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 查询我当前宝箱的状态 + 倒计时（id / status / unlockAt / openCostSteps / remainingSeconds 5 字段）,
so that 主界面（首页宝箱组件 Story 21.1）+ 倒计时校对（client 倒到 0 时主动重 GET）+ 开箱按钮可点态判定（仅 status=2 时可点）都能拿到 server 权威的"宝箱状态 + 倒计时"，不依赖客户端本地估算。

## 故事定位（Epic 20 第五条 = 节点 7 第一条 server 业务实装；上承 20.4 chest_open_logs migration，下启 20.6 POST /chest/open 事务 + 20.7 dev force-unlock）

- **Epic 20 进度**：20.1（接口契约最终化，**done**）→ 20.2（cosmetic_items migration，**done**）→ 20.3（cosmetic_items seed ≥15 行，**done**）→ 20.4（chest_open_logs migration + ChestOpenLog GORM struct，**done**）→ **20.5（本 story，GET /chest/current handler + service 纯读 + 动态判定 + 单测 ≥5 + 集成 ≥1）** → 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取）→ 20.7（dev /dev/force-unlock-chest）→ 20.8（dev /dev/grant-cosmetic-batch）→ 20.9（Layer 2 集成测试 - 开箱事务全流程）。
- **本 story 是 Epic 20 最简单的一条 server 实装**：纯读 + 无事务 + 无防作弊 + 无配置 + 无新 repo / 无新 repo 方法 —— 仅扩 `service` 包加新 `ChestService` interface + impl + 加 handler 加路由一行 + 单测 / 集成测试。**核心难度**集中在"DB 原值 → 动态判定 status / remainingSeconds"的语义实装 + V1 §7.1 钦定的扁平响应格式 + 4001 vs 1009 错误码精确区分。
- **下游 20.6 POST /chest/open**：**不**复用本 service 的 `GetCurrent` 方法（开箱事务走 FOR UPDATE 行锁查询，与本 story 的"无锁单查"完全不同语义）；但开箱**成功**响应里有 `data.nextChest.{id, status, unlockAt, openCostSteps, remainingSeconds}` 5 字段（V1 §7.2 钦定与 §7.1 同义对齐 —— 行 1207），**复用**本 story 落地的 `chestStatusDynamic` / `computeRemainingSeconds` 两个 helper（home_service.go 已建，本 story **仅消费**，**不**重新实装）。
- **下游 20.7 dev /dev/force-unlock-chest**：通过 `UPDATE user_chests SET unlock_at = now()` 让 chest 进入 unlockable 态；20.7 自身**不**调本 service，但 20.7 集成测试会 curl GET /chest/current 验证 status=2 已切换（epics.md §20.7 行 2949 钦定）。
- **下游 iOS Epic 21（首页宝箱组件 + 调 GET /chest/current 调用 + 状态展示）**：
  - Story 21.2 GET /chest/current 调用 + 状态展示 + 主动定时纠正 —— **强依赖**本 story
  - Story 21.3 开箱按钮（调 POST /chest/open） + Story 21.5 开箱前主动同步步数 —— 间接依赖（需要 21.2 落地后才能拿到 status=2 触发按钮可点）
- **epics.md §Story 20.5 钦定 AC**（行 2873-2897）：
  - 接口要求 auth（带 Bearer token）
  - 状态动态判定（3 条 case：DB status=1 + unlock_at ≤ now → status=2 / DB status=1 + unlock_at > now → status=1 / DB status=2 → status=2）
  - 用户没有 chest（理论不该发生）→ 4001（**非** 1009 / **非** 1003）
  - **不**更新 DB（动态判定节省写，真状态变更在开箱时 Story 20.6）
  - **单元测试 ≥5 case**（mocked chest repo + clock mock）：counting + unlock_at 在未来 5 分钟 → status=1, remainingSeconds=300 / counting + unlock_at 已过 → status=2, remainingSeconds=0 / status=2 → status=2 / 无 chest → 4001 / clock 跨秒边界 → remainingSeconds 计算精度合理（不出现 -1）
  - **集成测试 ≥1**（dockertest）：创建 user + chest with unlock_at=now+10min → curl GET /chest/current → status=1, remainingSeconds≈600
- **V1 §7.1 已冻结契约**（`docs/宠物互动App_V1接口设计.md` 行 810-915；20.1 已 done）：
  - request：无 body / 无 query；token from `Authorization: Bearer <token>` header
  - response data（成功 code=0）：**扁平** 5 字段 `{id, status, unlockAt, openCostSteps, remainingSeconds}`
    - `id`: string（BIGINT 字符串化，V1 §2.5 钦定）
    - `status`: int 枚举 `1=counting` / `2=unlockable`（**动态判定**，非 DB 原值）
    - `unlockAt`: string ISO 8601 UTC（如 `"2026-04-23T10:20:00Z"`，length=20）
    - `openCostSteps`: int（节点 7 阶段固定 1000）
    - `remainingSeconds`: int（`max(0, ceil((unlockAt - now) / 1s))`；never < 0）
  - **错误码全集**：1001 (auth 中间件) / 1005 (rate_limit) / 1009 (DB / panic) / 4001 (chest 不存在 —— service 层翻译)
  - 认证：Bearer token；限频：默认 60/min/userID（已认证子组）
- **V1 §7.1 关键约束摘录**（行 908-915）：
  - **status 动态判定 vs DB 原始值**：DB `user_chests.status` 节点 7 阶段恒为 1（仅在开箱事务 Story 20.6 才更新）；server 端 `data.status` 是"DB 原值 + 当前时间"动态计算结果 —— 客户端拿到 `status=2` 不能推断 DB 也是 2
  - **remainingSeconds 不会为负**：服务端用 `max(0, ceil((unlock_at - now) / 1s))` 兜底；即便 server 时钟跳变 / `unlock_at` 略早于 now 也只返 0
  - **本地倒计时与 server 状态校对**：client 倒到 0 时**主动**重 GET 一次以校正（避免本地时钟漂移）
  - **首次登录后 1 秒内调本接口**：Story 4.6 登录初始化的 chest `unlock_at = now + 10min`；首次 GET 返回 `status=1, remainingSeconds ≈ 600`
- **数据库 §5.6 钦定**（`docs/宠物互动App_数据库设计.md`）：
  - `user_chests` 表：`uk_user_id` UNIQUE 约束保证一个用户有且只有一行
  - 字段 `status TINYINT NOT NULL` / `unlock_at DATETIME(3) NOT NULL` / `open_cost_steps INT UNSIGNED DEFAULT 1000` / `version BIGINT UNSIGNED`
  - 本 story 仅消费已落地的 0005 migration + GORM `UserChest` struct + `ChestRepo.FindByUserID` 方法（chest_repo.go），**不**改任一行 repo 层
- **数据库 §6.7 status 枚举钦定**：1=counting / 2=unlockable / 3=opening_in_transaction（事务中间态，不下发客户端）...；本 story `data.status` 只下发 1 / 2 两值（V1 §7.1 行 851 钦定客户端可见枚举集）

## 范围红线（明确不做）

**本 story 只做**：
1. 在 `server/internal/service/` 下**新建** `chest_service.go`：定义 `ChestService` interface + `chestServiceImpl` struct + `NewChestService` 构造函数 + `GetCurrent(ctx, userID) (*ChestBrief, error)` 方法实装；
2. **复用** home_service.go 已落地的 `ChestBrief` struct + `chestStatusDynamic` + `computeRemainingSeconds` 两个 helper（**不**新建同义类型 / 同义函数）；
3. 在 `server/internal/app/http/handler/` 下**新建** `chest_handler.go`：定义 `ChestHandler` struct + `NewChestHandler` 构造 + `GetCurrent(c *gin.Context)` 方法 + `getCurrentChestResponseDTO` helper；
4. 修改 `server/internal/app/bootstrap/router.go`：在 `authedGroup.GET("/steps/account", ...)` 一行之后**追加一行** `authedGroup.GET("/chest/current", chestHandler.GetCurrent)`；并在 if 块顶部 wire `chestSvc := service.NewChestService(chestRepo)` + `chestHandler := handler.NewChestHandler(chestSvc)`（**复用** 4.8 已 wire 的 `chestRepo` 实例 —— 不要重新调 `NewChestRepo`）；
5. **新建** `chest_service_test.go`：≥5 case 单测（HappyPath_Counting / HappyPath_Unlockable / HappyPath_DBStatus2 / NotFound_4001 / ClockBoundary）；
6. **新建** `chest_handler_test.go`：≥5 case 单测（HappyPath_FlatSchema / HappyPath_BIGINTIDStringified / 4001_HTTP200 / 1009_HTTP200 / MissingUserID_1009）；
7. **新建** `chest_service_integration_test.go`：≥1 dockertest case（按 epics.md §20.5 钦定模板：创建 user + chest with unlock_at=now+10min → svc.GetCurrent → status=1, remainingSeconds≈600）；
8. 本 story 文件本身 + sprint-status.yaml 流转。

**本 story 不做**：

- **不**新增任何 repo / repo 方法（`ChestRepo.FindByUserID` 已在 4.6 / 4.8 实装，仅消费）
- **不**新增任何 migration / DDL / seed（0005 已锁定）
- **不**新增任何 ChestService interface 方法（仅 `GetCurrent` 一个；20.6 才加 `OpenChest` 等）
- **不**接事务（GET /chest/current 全只读，**不**调 `txMgr.WithTx`；与 home_service.LoadHome 同模式 —— 单表 PK 查询不上事务）
- **不**接 Redis（节点 7 阶段本接口无幂等需求；GET 天然幂等）
- **不**修改 `home_service.go` 任一行（`ChestBrief` / `chestStatusDynamic` / `computeRemainingSeconds` 在 home_service.go 已落地 + 注释钦定 "Story 20.5 复用"，本 story 严格按"消费方"姿态使用，**不**反向迁移到 chest_service.go）
- **不**修改 `chest_repo.go` 任一行（仅消费 `FindByUserID` + `ErrChestNotFound` 哨兵）
- **不**修改 `internal/pkg/errors/codes.go`（`ErrChestNotFound = 4001` 已在 codes.go 注册，仅消费）
- **不**修改 V1 §7.1 接口契约任一字（20.1 已冻结）
- **不**修改 0001-0013 任一 migration（已锁定）
- **不**修改 `docs/宠物互动App_*.md` 任一份（契约**输入**侧）
- **不**修改 `cmd/server/main.go` / `internal/app/bootstrap/server.go`（除 router.go 一行 wire + 4 行实例构造外）
- **不**修改 `Deps` struct（GET /chest/current 不引入新依赖；复用 `deps.GormDB`）
- **不**修改 `internal/app/http/middleware/auth.go` / `rate_limit.go` / `error_mapping.go`（已实装；本接口与 GET /home 共享 `authedGroup` 中间件链）
- **不**写 e2e 跨端测试（Epic 22 才做：Story 22.1 跨端集成测试场景）
- **不**修改 `_bmad-output/implementation-artifacts/decisions/*.md`（无新决策）
- **不**写"prod 部署 / 运维化"改造（保留最小集；与 7.4 / 17.4 / 11.6 同模式）
- **不**预实装 Story 20.6 / 20.7 / 20.8 任一行（即使顺手把 `OpenChest` 方法签名加到 interface 里也禁止 —— YAGNI，让 20.6 评审找不到"新增方法"明确边界）
- **不**写 clock interface（节点 7 阶段 service 单测直接传 `now time.Time` 参数注入；不引入 `Clock` 包；与 home_service 同模式 —— 调用方 service.GetCurrent 内部 `time.Now().UTC()` 取当前时间，单测通过"重置 helper 内部时钟"或"通过 chest.UnlockAt 相对偏移"避开 time.Now 不可控）

**任何超出上述清单的改动 → HALT 并问设计**。

## Acceptance Criteria

### AC1 — 新建 `chest_service.go`（`ChestService` interface + `chestServiceImpl` + `GetCurrent` 实装）

新建 `server/internal/service/chest_service.go`：

```go
package service

import (
	"context"
	stderrors "errors"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// ChestService 是 chest handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**：handler 单测注入 stub struct，与 7.4 / 17.4 / 11.6 同模式。
//
// 节点 7 阶段仅 GetCurrent；future Story 20.6 落地 POST /chest/open 时**另起独立 interface**
// （或在本 interface 末尾追加 OpenChest 方法 + 复用 chestServiceImpl）—— 本 story 不预实装。
type ChestService interface {
	// GetCurrent 处理 GET /api/v1/chest/current 业务（Story 20.5）。
	//
	// 流程（V1 §7.1.4 + 数据库设计 §5.6）：
	//  1. chestRepo.FindByUserID(ctx, userID) → user_chest（必有；登录初始化已建）
	//  2. service 层动态判定 chest.status / remainingSeconds（基于 time.Now().UTC() vs unlockAt）
	//     **复用** home_service.go 已落地的 chestStatusDynamic + computeRemainingSeconds 两个 helper
	//     （home_service.go 顶部注释钦定 "Story 20.5 chest_service.GetCurrent 复用"）
	//  3. 拼装 ChestBrief 返回（**复用** home_service.go 已落地的 ChestBrief struct）
	//
	// 错误约定（ADR-0006 三层映射）：
	//   - mysql.ErrChestNotFound（理论不该发生 —— Story 4.6 五表事务必建一行）→
	//     **包成 ErrChestNotFound (4001)**（V1 §7.1.6 钦定 4001，**不**包成 1009 / 1003 ——
	//     V1 §7.1 行 904 钦定 "user 在 user_chests 表中无任何行" 是 4001 而非 1009）
	//   - 其他 DB 异常（连接断 / SQL 错 / 死锁等）→ 包成 ErrServiceBusy (1009)
	//
	// **不**接事务（纯读，与 home_service.LoadHome / step_service.GetAccount 同模式）。
	// **不**更新 DB（动态判定，节省写；真状态变更在开箱事务 Story 20.6）。
	GetCurrent(ctx context.Context, userID uint64) (*ChestBrief, error)
}

// chestServiceImpl 是 ChestService 的默认实装。
//
// 依赖（DI 注入；bootstrap.NewRouter 内 wire）：
//   - chestRepo: mysql.ChestRepo（4.6 已实装）
//
// **不**依赖：
//   - txMgr（GET /chest/current 全只读，无事务）
//   - userRepo / petRepo / stepAccountRepo（chest 单表查询不聚合其他实体）
//   - signer（auth 中间件已校验 token，handler 已注入 userID）
type chestServiceImpl struct {
	chestRepo mysql.ChestRepo
}

// NewChestService 构造 ChestService。
func NewChestService(chestRepo mysql.ChestRepo) ChestService {
	return &chestServiceImpl{chestRepo: chestRepo}
}

// GetCurrent 实装：单表查询 user_chests → 动态判定 → ChestBrief。
//
// **关键**：
//   - chest 必有（登录初始化已建）；查不到 → V1 §7.1.6 钦定 4001
//   - chest_status_dynamic 在 home_service.go 顶部注释钦定 "Story 20.5 复用，签名 + 行为冻结"
//   - computeRemainingSeconds 同上
//
// **time.Now().UTC()**：必须 UTC（与 V1 §2.5 ISO 8601 UTC 钦定一致；
// 与 chest.UnlockAt 字段 UTC 语义对齐 —— chest_repo.go 顶部注释钦定）。
func (s *chestServiceImpl) GetCurrent(ctx context.Context, userID uint64) (*ChestBrief, error) {
	chest, err := s.chestRepo.FindByUserID(ctx, userID)
	if err != nil {
		// 理论不该发生（Story 4.6 五表事务必建一行）→ 但 V1 §7.1.6 钦定 4001，按契约下发。
		if stderrors.Is(err, mysql.ErrChestNotFound) {
			return nil, apperror.Wrap(err, apperror.ErrChestNotFound, apperror.DefaultMessages[apperror.ErrChestNotFound])
		}
		// 其他 DB 异常（含 driver / 网络 / 慢查询）→ 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	now := time.Now().UTC()
	chestStatus := chestStatusDynamic(chest.Status, chest.UnlockAt, now)
	remainingSeconds := computeRemainingSeconds(chest.UnlockAt, now)

	return &ChestBrief{
		ID:               chest.ID,
		Status:           chestStatus,
		UnlockAt:         chest.UnlockAt.UTC(), // 强制 UTC 视图（防 GORM driver loc=Local 漂移；与 home_service.LoadHome 同模式）
		OpenCostSteps:    chest.OpenCostSteps,
		RemainingSeconds: remainingSeconds,
	}, nil
}
```

**关键约束**：

- **复用 `ChestBrief`**（home_service.go 已定义；本 service 直接复用）—— **不**新建 GetCurrentOutput / ChestCurrentBrief 等同义类型
- **复用 `chestStatusDynamic` / `computeRemainingSeconds`**（home_service.go 已定义，签名 + 行为已冻结；home_service.go 顶部注释钦定 "Story 20.5 复用"）—— **不**新建同义 helper / **不**重新实装
- **`stderrors.Is` 区分 NotFound 哨兵**：与 7.4 / 11.6 service 同模式（`stderrors` alias 因 `errors` 已被 `apperror` 占用 —— 看 imports 段写法）
- **NotFound → 4001**（V1 §7.1.6 钦定）；**不**包成 1009（与 home_service.LoadHome 不同 —— LoadHome 是聚合接口 → 任一失败 1009 不部分降级；本 service 是单查接口，按 V1 钦定的 4001 精确下发）；**不**包成 1003（1003 是通用 "资源不存在"，4001 是 chest 业务专用；用 4001 让 client 能区分 "chest 缺失" 与 "其他资源缺失"）
- **time.Now().UTC()** 必须 UTC（与 V1 §2.5 + chest_repo.go 顶部注释一致）
- **不**接事务（纯读，与 home_service.LoadHome / step_service.GetAccount 同模式）

### AC2 — 新建 `chest_handler.go`（`ChestHandler` + `GetCurrent` + `getCurrentChestResponseDTO`）

新建 `server/internal/app/http/handler/chest_handler.go`：

```go
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// ChestHandler 是 /api/v1/chest/* 路由的 handler 集合。
//
// 节点 7 阶段：GetCurrent（GET /chest/current，Story 20.5）；
// future Story 20.6 加 PostOpen（POST /chest/open）。
type ChestHandler struct {
	svc service.ChestService
}

// NewChestHandler 构造 ChestHandler。
func NewChestHandler(svc service.ChestService) *ChestHandler {
	return &ChestHandler{svc: svc}
}

// GetCurrent 处理 GET /api/v1/chest/current（Story 20.5）。
//
// 流程：
//  1. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入；不存在 → 1009 unreachable 兜底）
//  2. 调 svc.GetCurrent(ctx, userID) —— ctx = c.Request.Context()（**不**用 *gin.Context；ADR-0007 §2.2）
//  3. 成功 → response.Success(c, dto, "ok")；失败 → c.Error(err) + return（middleware envelope）
//
// **不**做参数校验（GET 无 body / 无 query；userID 由 auth 中间件兜底）；与 home_handler.LoadHome /
// steps_handler.GetAccount 同模式。
//
// **不**直接调 response.Error 写 envelope（ADR-0006 单一 envelope 生产者；与 4.6 / 4.8 / 7.4 同模式）。
func (h *ChestHandler) GetCurrent(c *gin.Context) {
	// 从 auth 中间件取 userID（与 home_handler.LoadHome / steps_handler.GetAccount 同模式）
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		// unreachable: Auth 中间件挂在前；保险兜底走 1009
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	userID, ok := v.(uint64)
	if !ok {
		// unreachable: Auth 中间件 c.Set(UserIDKey, claims.UserID) 永远是 uint64
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}

	out, err := h.svc.GetCurrent(c.Request.Context(), userID)
	if err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	response.Success(c, getCurrentChestResponseDTO(out), "ok")
}

// getCurrentChestResponseDTO 把 service 输出转成 V1 §7.1 wire 格式。
//
// **关键 schema**（V1 §7.1 行 848-854 钦定）：扁平 5 字段：
//   - id: string（BIGINT 字符串化，V1 §2.5 + §7.1 行 850 钦定）
//   - status: int 1 / 2（**动态判定后**值，非 DB 原值）
//   - unlockAt: string ISO 8601 UTC（如 "2026-04-23T10:20:00Z"；time.RFC3339 格式）
//   - openCostSteps: int（节点 7 阶段固定 1000）
//   - remainingSeconds: int（≥ 0；service 层已用 max(0, ...) 兜底）
//
// **与 home_handler chest 块字段一致**（home_handler.go 行 141-147）：
// 节点 2 阶段 GET /home 已下发同一组字段；本 story 下发同一组字段；future 任一字段调整需要同步 V1 §5.1 + §7.1。
func getCurrentChestResponseDTO(out *service.ChestBrief) gin.H {
	return gin.H{
		"id":               strconv.FormatUint(out.ID, 10),
		"status":           out.Status,
		"unlockAt":         out.UnlockAt.Format(time.RFC3339),
		"openCostSteps":    out.OpenCostSteps,
		"remainingSeconds": out.RemainingSeconds,
	}
}
```

**关键约束**：

- handler 不做参数校验（GET 无 body / query；userID 走 auth 中间件）—— 与 home_handler.LoadHome / steps_handler.GetAccount 同模式
- userID 取出与类型断言用 `c.Get(middleware.UserIDKey)` + `v.(uint64)` —— 与所有 authedGroup 内 handler 完全同模式
- `c.Error + return` 而非 `response.Error`（ADR-0006）
- DTO 字段命名严格 `id` / `status` / `unlockAt` / `openCostSteps` / `remainingSeconds`（V1 §7.1.2 + §7.1.3 钦定 camelCase）
- `data.id` 是 string（V1 §2.5 + §7.1 行 850 钦定 BIGINT 字符串化；**不**下发 number —— 客户端 JS Number 不能精确表示 > 2^53 的 uint64）
- 其他 4 个字段是 number / string（**不**字符串化）
- `unlockAt` 用 `time.RFC3339`（输出 `"2026-04-23T10:20:00Z"`）—— 与 home_handler.LoadHome 行 144 同模式
- **不**复用 `home_handler` 的 chest 块（home_handler 直接 inline gin.H 在 LoadHome 内）—— 本 story 独立 helper 函数，让 chest_handler 自包含；当 V1 §7.1 与 §5.1 chest schema 出现差异时（虽然当前一致）便于独立演化

### AC3 — `bootstrap/router.go` wire chestSvc + chestHandler + 追加路由一行

修改 `server/internal/app/bootstrap/router.go`，三处改动（全部在 `if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil { ... }` 块内）：

1. **复用** 既有 `chestRepo := repomysql.NewChestRepo(deps.GormDB)`（行 312 已建）；**不**重新构造
2. 在 `petsHandler := handler.NewPetsHandler(petSvc)` 一行之后**追加** 2 行（service + handler 构造；位置可任选 GormDB 块内任一处但建议靠近其他 service 构造段，按依赖顺序排列）：

   ```go
   // Story 20.5 加：chest service + handler（GET /chest/current，单查不开事务 + 动态判定）。
   // 复用 4.8 已 wire 的 chestRepo 实例（避免重复构造同 db handle 的多个 repo）。
   chestSvc := service.NewChestService(chestRepo)
   chestHandler := handler.NewChestHandler(chestSvc)
   ```

3. 在 `authedGroup.GET("/steps/account", stepsHandler.GetAccount)` 那一行之后**追加一行**：

   ```go
   authedGroup.GET("/chest/current", chestHandler.GetCurrent) // Story 20.5 加
   ```

**关键约束**：

- **复用** 既有 `chestRepo` 实例（行 312 已建给 authSvc / homeSvc 用）—— **不**调 `repomysql.NewChestRepo(deps.GormDB)` 再造一份；同 11.3 r4 review 钦定的"双实例引入隐性 race / 测试 mock 不一致"反模式
- **不**改 `Deps` struct（无新依赖；`deps.GormDB` / `deps.Signer` / `deps.TxMgr` 已存在）
- **不**改 `cmd/server/main.go`（无新配置 / 依赖透传）
- 路由挂在 `authedGroup`（与 GET /home / GET /steps/account 共享 auth + rate_limit by userID 中间件 —— V1 §7.1 行 820-821 钦定需要 Bearer + 60/min/userID）
- 路径 `/chest/current` 不带 `/api/v1` 前缀（前缀由 `api := r.Group("/api/v1")` + `authedGroup := api.Group(...)` 自动加）
- HTTP method `GET`（V1 §7.1 行 818 钦定）；**不**用 POST
- 顺序：建议放在 `authedGroup.GET("/steps/account", ...)` 之后，`authedGroup.POST("/rooms", ...)` 之前，按"模块分组（auth/home/emojis/steps/chest/rooms/pets）+ 模块内 GET 优先 POST 在后"原则排列；本约束**非**强制（gin Router 不依赖路由声明顺序），仅可读性建议

### AC4 — service 单元测试覆盖（≥5 case，stub chest repo）

新建 `server/internal/service/chest_service_test.go`：≥5 case，前缀 `TestChestService_GetCurrent_`。

**stub repo 模式**（参考 step_service_test.go 已建的 stubStepAccountRepo 模式）：

```go
//go:build !integration

package service

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// stubChestRepo 是 mysql.ChestRepo 的 stub 实装，仅服务于 chest_service_test.go。
//
// **每 case 独立 stub instance**（避免 createCalls 计数串扰）—— 与 step_service_test 同模式。
type stubChestRepo struct {
	findByUserIDFn func(ctx context.Context, userID uint64) (*mysql.UserChest, error)
	createFn       func(ctx context.Context, c *mysql.UserChest) error
}

func (s *stubChestRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
	return s.findByUserIDFn(ctx, userID)
}

func (s *stubChestRepo) Create(ctx context.Context, c *mysql.UserChest) error {
	if s.createFn != nil {
		return s.createFn(ctx, c)
	}
	return nil
}
```

**必须覆盖 ≥5 case**（epics.md §20.5 钦定）：

1. **`TestChestService_GetCurrent_HappyPath_Counting_Returns300s`**：
   stub `findByUserIDFn` 返 `&mysql.UserChest{ID: 5001, Status: 1, UnlockAt: time.Now().UTC().Add(5*time.Minute), OpenCostSteps: 1000, Version: 0}` → service 返 `*ChestBrief{ID: 5001, Status: 1, OpenCostSteps: 1000, RemainingSeconds: ≈ 300（允许 ±1 秒抖动，因 time.Now() 取两次）}`
   断言：`brief.Status == 1` + `brief.RemainingSeconds >= 299 && brief.RemainingSeconds <= 300`（用 `>=, <=` 区间断言，避免 `==` 强等导致单测偶发 flake）

2. **`TestChestService_GetCurrent_HappyPath_Unlockable_DBStatus1_UnlockAtPassed_Returns0s`**：
   stub 返 `&mysql.UserChest{ID: 5001, Status: 1, UnlockAt: time.Now().UTC().Add(-5*time.Minute), OpenCostSteps: 1000}` → service 返 `Status: 2, RemainingSeconds: 0`（**动态判定**核心断言：DB status=1 + unlock_at < now → 下发 status=2）

3. **`TestChestService_GetCurrent_HappyPath_DBStatus2_Returns2_0s`**：
   stub 返 `&mysql.UserChest{ID: 5001, Status: 2, UnlockAt: time.Now().UTC().Add(-10*time.Minute), OpenCostSteps: 1000}` → service 返 `Status: 2, RemainingSeconds: 0`（DB 原值已是 2，service 透传 + remainingSeconds=0）

4. **`TestChestService_GetCurrent_ChestNotFound_Returns4001`**：
   stub 返 `(nil, mysql.ErrChestNotFound)` → service 返 `(nil, *apperror.AppError with Code == apperror.ErrChestNotFound (4001))`
   断言：
   ```go
   var appErr *apperror.AppError
   if !errors.As(err, &appErr) { t.Fatalf(...) }
   if appErr.Code != apperror.ErrChestNotFound { t.Errorf(...) }  // 4001，**非** 1003 / 1009
   ```

5. **`TestChestService_GetCurrent_ClockBoundary_RemainingSecondsNotNegative`**：
   stub 返 `&mysql.UserChest{Status: 1, UnlockAt: time.Now().UTC().Add(-1*time.Millisecond)}` → service 返 `Status: 2, RemainingSeconds: 0`（**不**负数；验证 `computeRemainingSeconds` 的 `max(0, ...)` 兜底）

**Bonus（可选 ≥1 case）**：

6. **`TestChestService_GetCurrent_RepoOtherDBError_Returns1009`**（建议加）：
   stub 返 `(nil, stderrors.New("db connection lost"))`（非 ErrChestNotFound 哨兵）→ service 返 `*apperror.AppError with Code == apperror.ErrServiceBusy (1009)`
   防御断言：未识别的 raw error 必须翻译为 1009；如果实装把 raw error 透传给 handler 会让 envelope 走 `default` 兜底也是 1009 但语义模糊（ADR-0006）

**关键约束**：

- 5 case 命名前缀 `TestChestService_GetCurrent_<场景>` —— 与 7.4 / 11.6 / 17.4 case 命名同风格
- **不**用真实 time（time.Now().UTC().Add(...)）—— service 内部 `time.Now().UTC()` 取一次，stub 设置 unlock_at 时再 time.Now() 取一次，两次间隔 < 1ms 内可控；断言用区间（`>= X-1 && <= X`）避开 ±1s 抖动
- **不**引入 clock interface（本 story 范围红线已钦定）
- **不**在 chest_service_test.go 内重复定义 `ChestBrief` / `chestStatusDynamic` / `computeRemainingSeconds`（这三个已在 home_service.go 同包内定义，test 直接引用）
- **HappyPath_Counting 断言**：用区间 `>= 299 && <= 300`，因 stub setup 时 `time.Now().UTC().Add(5*time.Minute)` 与 service.GetCurrent 内 `time.Now().UTC()` 间隔可能 < 1ms 但有理论抖动；区间断言更稳健
- **NotFound case 断言**：必须用 `errors.As(err, &appErr)` 取出 *AppError 后 `appErr.Code == apperror.ErrChestNotFound`（即 4001），不直接断 err == apperror.ErrChestNotFound（4001 是 int code，*AppError 是 wrapper struct）

### AC5 — handler 单元测试覆盖（≥5 case，stub service + 测试 router）

新建 `server/internal/app/http/handler/chest_handler_test.go`：≥5 case，前缀 `TestChestHandler_GetCurrent_`。

**stub service 模式**（参考 steps_handler_test.go 已建的 stubStepService 模式）：

```go
//go:build !integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/service"
)

type stubChestService struct {
	getCurrentFn func(ctx context.Context, userID uint64) (*service.ChestBrief, error)
}

func (s *stubChestService) GetCurrent(ctx context.Context, userID uint64) (*service.ChestBrief, error) {
	return s.getCurrentFn(ctx, userID)
}

// newChestHandlerRouter 构造测试 router：挂 ErrorMappingMiddleware + 注入 mockUserID + 注册 GET /chest/current。
//
// 模式与 newStepsHandlerRouter 同（steps_handler_test.go 已建）。
func newChestHandlerRouter(t *testing.T, h *ChestHandler, mockUserID *uint64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	if mockUserID != nil {
		uid := *mockUserID
		r.Use(func(c *gin.Context) {
			c.Set(middleware.UserIDKey, uid)
			c.Next()
		})
	}
	r.GET("/api/v1/chest/current", h.GetCurrent)
	return r
}

// chestEnvelope 是 V1 §2.4 钦定统一 envelope 的 Go mirror。
type chestEnvelope struct {
	Code      int                    `json:"code"`
	Message   string                 `json:"message"`
	Data      map[string]any         `json:"data"`
	RequestID string                 `json:"requestId"`
}

func decodeChestEnvelope(t *testing.T, body []byte) chestEnvelope {
	t.Helper()
	var env chestEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v\nbody: %s", err, string(body))
	}
	return env
}
```

**必须覆盖 ≥5 case**：

1. **`TestChestHandler_GetCurrent_HappyPath_FlatSchema`**：
   mockUserID = 1001；stub service 返 `&service.ChestBrief{ID: 5001, Status: 1, UnlockAt: time.Date(2026, 4, 23, 10, 20, 0, 0, time.UTC), OpenCostSteps: 1000, RemainingSeconds: 253}` → HTTP 200 + envelope `code=0` + **扁平** data：
   - `data.id == "5001"` （string 字符串化，**非** number）
   - `data.status == 1`
   - `data.unlockAt == "2026-04-23T10:20:00Z"` （RFC3339 / ISO 8601 UTC）
   - `data.openCostSteps == 1000`
   - `data.remainingSeconds == 253`
   **关键断言**：`data` map 没有任何 `chest` 子对象（与 §5.1 GET /home 的 `data.chest.{...}` 嵌套不同；§7.1 是扁平）

2. **`TestChestHandler_GetCurrent_BIGINTIDStringified`**：
   stub 返 `ID: math.MaxUint64`（极端 BIGINT 值）或 `ID: 18446744073709551615` → 断 `data.id == "18446744073709551615"`（string；如果是 JS Number 会精度损失）；JSON Unmarshal 时若不是 string 会失败
   防御断言：用 `data["id"]` 取出 + `.(string)` 类型断言成功

3. **`TestChestHandler_GetCurrent_ServiceReturns4001_ForwardsAsCode4001_HTTP200`**：
   stub service 返 `*apperror.AppError(ErrChestNotFound, "当前宝箱不存在")` → handler `c.Error` → middleware envelope `code=4001`，HTTP **200**（V1 §2.4 钦定业务错也走 HTTP 200，**不**是 4xx）
   断言：`w.Code == http.StatusOK` + `env.Code == 4001`

4. **`TestChestHandler_GetCurrent_ServiceReturns1009_ForwardsAsCode1009_HTTP200`**：
   stub service 返 `*apperror.AppError(ErrServiceBusy, "服务繁忙")` → envelope `code=1009`，HTTP 200

5. **`TestChestHandler_GetCurrent_MissingUserIDInContext_Returns1009`**：
   `mockUserID = nil`（不注入 userID 到 c.Keys）→ handler 走 unreachable 兜底分支 → envelope `code=1009`
   与 home_handler_test / steps_handler_test 同模式 fail-safe

**关键约束**：

- 5 case 命名前缀 `TestChestHandler_GetCurrent_<场景>` —— 与 7.4 / 17.4 case 命名同风格
- 单测启动的 router **必须挂 ErrorMappingMiddleware**（newChestHandlerRouter 已挂）
- 注入 userID 通过 `mockUserID *uint64` 参数（nil 不挂 mock，模拟 unreachable 分支）
- HappyPath case **必须断扁平结构**（顶级 `data.id` / `data.status` 等；如果误访问 `data.chest` 应该 nil）—— 这是 V1 §7.1 vs §5.1 schema 差异的核心断言点
- `data.id` 是 string 必须断言 `.(string)` 类型断言成功（防 future regression 把 `id` 改回 uint64 数字）
- **不**真起 auth 中间件 / signer
- **不**断言 stub 内部行为（GetCurrent 入参只有 userID，stub 函数被调即说明 wire 链路正确，但**可**像 7.4 happy case 一样在 stub 内 `if userID != 1001 { t.Errorf(...) }` 验 ID 透传）

### AC6 — 集成测试（dockertest，≥1 case）

新建 `server/internal/service/chest_service_integration_test.go`：≥1 case。

**实装模式**（参考 home_service_integration_test.go / step_service_integration_test.go 已建的 dockertest helper）：

```go
//go:build integration

package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
	// 视既有 helper 位置调整：buildXxxIntegration 是哪个文件 owner 的就 import 哪个；
	// 通常 home_service_integration_test.go 内已有 startMySQLContainer / insertUser /
	// insertUserChest 等 helper —— 本 story 复用，**不**重新建一套。
)

func TestChestServiceIntegration_GetCurrent_HappyPath_Counting_Returns600s(t *testing.T) {
	sqlDB, gormDB, cleanup := startMySQLContainer(t) // 复用 home_service_integration_test.go helper
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-chest-get-1", "用户chest", "")

	// 关键：unlock_at = now + 10min（节点 7 Story 4.6 firstTimeLogin 钦定的初始倒计时）
	unlockAt := time.Now().UTC().Add(10 * time.Minute)
	insertUserChest(t, sqlDB, userID, 1 /* status=counting */, unlockAt, 1000 /* open_cost_steps */)

	chestRepo := mysql.NewChestRepo(gormDB)
	svc := service.NewChestService(chestRepo)

	ctx := context.Background()
	out, err := svc.GetCurrent(ctx, userID)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if out.Status != 1 {
		t.Errorf("Status = %d, want 1 (counting)", out.Status)
	}
	if out.OpenCostSteps != 1000 {
		t.Errorf("OpenCostSteps = %d, want 1000", out.OpenCostSteps)
	}
	// remainingSeconds ≈ 600（10min = 600s）；service 内部 time.Now() 与本测试 setup time.Now() 间隔 < 1s
	if out.RemainingSeconds < 595 || out.RemainingSeconds > 601 {
		t.Errorf("RemainingSeconds = %d, want ≈ 600 (allow [595, 601] for clock jitter)", out.RemainingSeconds)
	}
}
```

**关键约束**：

- **复用** home_service_integration_test.go / step_service_integration_test.go 已建的 helper（`startMySQLContainer` / `insertUser` / `insertUserChest`）—— **不**重新建一套；如 helper 跨文件不可见，按既有 same-package _test 模式（如 `helpers_test.go` 共享 fixture）查找正确导入路径
  - **先 Read** `internal/service/home_service_integration_test.go` 找 helper 真实命名 + 包路径（可能在 service 包内 `package service_test` 或 helpers 文件）；如果命名是 `seedUserChest` / `createUserChest` 等同义，使用真实命名 —— 本 AC 模板用 `insertUserChest` 是示例命名
- 本机 Windows docker 不可用 → t.Skip（与 4.3 / 7.4 / 11.6 同模式；helper 内已有 skip 逻辑自动继承）
- envName 不传 / 用默认（chest service 不依赖 envName，与 step_service 不同 —— chest 无 prod cap）
- **不**新增独立的 chest_handler_integration_test（handler 集成由 service 集成 + handler 单测已覆盖；e2e 走 Epic 22.1 跨端测试）
- **不**新增 NotFound / 1009 集成 case —— 单测已用 stub 覆盖该返回路径，集成测试再造一次"故意不 INSERT chest"成本 > 收益（与 7.4 同决策模式）
- **可选** 集成 bonus case：`TestChestServiceIntegration_GetCurrent_UnlockAtPassed_DynamicReturns2`（INSERT chest with unlock_at = now - 5min → svc.GetCurrent → Status=2 + RemainingSeconds=0）—— 验证"DB 原值 status=1 但下发 status=2"动态判定的端到端正确性；时间成本极低，建议加上但**不**强制

### AC7 — `bash scripts/build.sh` 全量绿

完成后必须能跑通：

```bash
bash scripts/build.sh                      # vet + build → no failures
bash scripts/build.sh --test               # 全单测过（service 5+ + handler 5+ 新 case + 既有全过）
bash scripts/build.sh --race --test        # CI Linux 必过；Windows race skip 不阻塞
bash scripts/build.sh --integration        # 既有集成 + 新 1 case；docker 不可用 → t.Skip
```

**关键约束**：

- 本 story 引入的新代码全部含单测；`go test ./internal/service/... -run TestChestService_GetCurrent -v` 必须 5 case 全过
- `go test ./internal/app/http/handler/... -run TestChestHandler_GetCurrent -v` 必须 5 case 全过
- `--race` 在 Windows skip（ADR-0001 §3.5）；CI Linux 跑
- **不**改 `scripts/build.sh` 自身

### AC8 — 验证清单（人工 + 自动化）

完成后**人工**核对以下 10 项（结果记到 Completion Notes List）：

| # | 验证项 | 验证方式 |
|---|---|---|
| 1 | service 层 GetCurrent 流程严格按 V1 §7.1.4 实装：FindByUserID → 动态判定 status / remainingSeconds → 拼装 ChestBrief；NotFound 哨兵 → 4001（**非** 1003 / **非** 1009） | Read chest_service.go + diff against V1 §7.1.4 / §7.1.6 |
| 2 | service 层**复用** home_service.go 的 `ChestBrief` struct + `chestStatusDynamic` + `computeRemainingSeconds`（**未**重复定义同义类型 / 同义函数） | Grep `ChestBrief` / `chestStatusDynamic` / `computeRemainingSeconds` 全 repo 出现位置（应仅 home_service.go 定义 + home_service.go / chest_service.go 消费） |
| 3 | handler `getCurrentChestResponseDTO` 返**扁平** gin.H（顶级 `id` / `status` / `unlockAt` / `openCostSteps` / `remainingSeconds`）；**无** `chest` 子对象 | Read chest_handler.go + diff against V1 §7.1 行 848-854 |
| 4 | `data.id` 是 `strconv.FormatUint(out.ID, 10)`（string 字符串化），**非** uint64 数字（V1 §2.5 + §7.1 行 850 钦定） | Read chest_handler.go DTO 段 |
| 5 | `data.unlockAt` 用 `time.RFC3339`（"2026-04-23T10:20:00Z" 格式，与 home_handler 同模式） | Read chest_handler.go + diff against home_handler.go 行 144 |
| 6 | handler 复用 `c.Get(middleware.UserIDKey)` + `v.(uint64)` 兜底（与 home_handler / steps_handler 完全同模式） | Read chest_handler.GetCurrent |
| 7 | bootstrap/router.go 仅追加：(1) 2 行 chestSvc + chestHandler 构造；(2) 1 行 `authedGroup.GET("/chest/current", chestHandler.GetCurrent)`；**未**新建 chestRepo 实例（复用 4.8 已 wire 的实例） | Read router.go diff |
| 8 | service 单测 NotFound case 断 `*apperror.AppError.Code == ErrChestNotFound (4001)`，**非** 1003 / 1009 | Read chest_service_test.go NotFound case |
| 9 | handler 单测 happy case 断 `data.id` 是 `string` + 无 `data.chest` 子对象（扁平 schema + BIGINT 字符串化两个核心断言） | Read chest_handler_test.go happy case |
| 10 | `bash scripts/build.sh --test` 全绿（含本 story 新增 ≥10 case）；`git status --short` 改动文件清单匹配预期范围（4 新文件 + 1 修改文件 + 1 story + 1 sprint-status = 7 文件） | bash 实跑 + git status |

### AC9 — 不 commit（流水线由 epic-loop 下游收口）

epics.md §Story 20.5 AC 钦定"≥5 单测 + ≥1 集成测试覆盖"，**不**钦定"git commit 单独提交"。本 story 是 server 业务代码 story，commit 由 epic-loop 流水线在下游 fix-review / story-done sub-agent 阶段统一收口。

- 本 story 的 dev workflow **不** commit / **不** push
- commit message 模板（story-done 阶段使用）：

  ```text
  feat(chest-current): GET /chest/current 当前宝箱状态查询接口（Story 20.5）

  - service chest_service.GetCurrent 实装：单表查询 user_chests → 动态判定
    status / remainingSeconds（复用 home_service.go 的 chestStatusDynamic +
    computeRemainingSeconds 两个 helper）→ ChestBrief；NotFound → 4001
    （V1 §7.1.6 钦定）；其他 DB 错 → 1009
  - handler chest_handler.GetCurrent + getCurrentChestResponseDTO（扁平 5 字段
    schema：id / status / unlockAt / openCostSteps / remainingSeconds；
    BIGINT id 字符串化 V1 §2.5）
  - bootstrap/router.go wire authedGroup.GET /chest/current 一行 + chestSvc /
    chestHandler 实例构造（复用 4.8 已 wire 的 chestRepo 实例）
  - 单测 ≥10 case（service ≥5 + handler ≥5）+ 集成测试 ≥1 case

  依据 epics.md §Story 20.5 + V1 §7.1 + apperror.ErrChestNotFound.

  Story: 20-5-get-chest-current-接口
  ```

- commit hash 待 story-done 阶段产生后回填到本文件

## Tasks / Subtasks

- [x] **Task 1（AC1）**：新建 `chest_service.go`
  - [x] 1.1 Read `step_service.go` 完整文件，理解 "interface + impl + NewXxx 构造 + stderrors.Is 错误识别" 的 service 文件骨架
  - [x] 1.2 Read `home_service.go` 全文，**特别**核对 (a) `ChestBrief` struct 定义（应在 home_service.go 行 76-87）；(b) `chestStatusDynamic` 函数（行 232-255）；(c) `computeRemainingSeconds` 函数（行 257-273）—— 确认这三个 symbol 在 service package 内可直接引用
  - [x] 1.3 Read `chest_repo.go` 全文，理解 `ChestRepo` interface 形态 + `FindByUserID` 返回的 `*UserChest` 字段命名（`ID / UserID / Status / UnlockAt / OpenCostSteps / Version / CreatedAt / UpdatedAt`）
  - [x] 1.4 Read `internal/pkg/errors/codes.go` + `apperror.go`，确认 `ErrChestNotFound = 4001` 已注册 + `DefaultMessages[ErrChestNotFound] = "当前宝箱不存在"` 已定义
  - [x] 1.5 Write `chest_service.go`：完整按 AC1 模板写 `ChestService` interface + `chestServiceImpl` + `NewChestService` + `GetCurrent` 实装
  - [x] 1.6 Read 回检：(a) 复用 `ChestBrief`（不新建类型）；(b) 复用 `chestStatusDynamic` + `computeRemainingSeconds`（不新建函数）；(c) NotFound 翻译为 `apperror.ErrChestNotFound (4001)`，**非** 1009 / 1003；(d) 其他 DB 错翻译为 `apperror.ErrServiceBusy (1009)`；(e) 不调 stepSyncLogRepo / 不调 txMgr.WithTx；(f) `stderrors.Is` 区分哨兵 error；(g) `time.Now().UTC()` 用 UTC

- [x] **Task 2（AC4）**：service 单测 ≥5 case（stub chest repo；TDD：先写测试驱动 service 实装）
  - [x] 2.1 Read `step_service_test.go` 完整文件，理解 stub repo 模式 + `stderrors.As / .Is` 断言 *AppError 模式
  - [x] 2.2 Read `home_service_test.go` 完整文件（特别 chest 相关 case），理解既有 `chestStatusDynamic` / `computeRemainingSeconds` 测试覆盖；本 story 单测**不**重复覆盖这两个 helper 行为（home_service_test 已覆盖），仅覆盖 `chest_service.GetCurrent` 的 "调 helper + 拼装 brief + 错误翻译" 业务流程
  - [x] 2.3 Write `chest_service_test.go`：5 case + bonus 1 case = 6 case（HappyPath_Counting / HappyPath_Unlockable_DBStatus1_UnlockAtPassed / HappyPath_DBStatus2 / ChestNotFound_4001 / ClockBoundary / RepoOtherDBError_1009）；stub instance 每 case 独立
  - [x] 2.4 跑 `go test ./internal/service/... -run TestChestService_GetCurrent -v` 验证 6 case 全过
  - [x] 2.5 Read 回检：(a) 每 case 独立 stub instance；(b) NotFound case 用 `errors.As(err, &appErr)` + `appErr.Code == apperror.ErrChestNotFound`；(c) HappyPath_Counting 用区间断言 `>= 299 && <= 300` 避抖动；(d) 不引入 clock interface

- [x] **Task 3（AC2）**：新建 `chest_handler.go`
  - [x] 3.1 Read `steps_handler.go` 完整文件，**特别**：(a) `GetAccount` 方法骨架（行 272-294）；(b) `getAccountResponseDTO` 扁平 helper（行 296-314）；(c) `c.Get(middleware.UserIDKey)` + `v.(uint64)` userID 取出模式（行 274-285）
  - [x] 3.2 Read `home_handler.go` 完整文件，**特别**：(a) chest 块 DTO（行 141-147）—— 字段命名 + `strconv.FormatUint` + `time.RFC3339` 用法；本 story DTO 与 home_handler chest 块**完全同字段**，但实装为独立 helper（不复用 inline 代码）
  - [x] 3.3 Write `chest_handler.go`：完整按 AC2 模板写 `ChestHandler` + `NewChestHandler` + `GetCurrent` + `getCurrentChestResponseDTO`
  - [x] 3.4 Read 回检：(a) UserID 取出与类型断言用 `c.Get(middleware.UserIDKey) + v.(uint64)`；(b) ctx 用 `c.Request.Context()`；(c) `c.Error + return` 不直接 response.Error；(d) DTO 扁平（**无** chest 子对象，与 home_handler 嵌套结构不同）；(e) `data.id` 用 `strconv.FormatUint(out.ID, 10)` 字符串化；(f) `data.unlockAt` 用 `time.RFC3339`

- [x] **Task 4（AC5）**：handler 单测 ≥5 case（stub service + 测试 router）
  - [x] 4.1 Read `steps_handler_test.go` 完整文件，理解 stubStepService + newStepsHandlerRouter + decodeStepsEnvelope helper 模式
  - [x] 4.2 Write `chest_handler_test.go`：5 case（HappyPath_FlatSchema / BIGINTIDStringified / 4001 / 1009 / MissingUserID）；stub service + newChestHandlerRouter + decodeChestEnvelope helper（参考 AC5 模板）
  - [x] 4.3 跑 `go test ./internal/app/http/handler/... -run TestChestHandler_GetCurrent -v` 验证 5 case 全过
  - [x] 4.4 Read 回检：(a) HappyPath case 强断 `data["chest"] == nil`（扁平 schema）+ `data["id"]` 是 string 类型；(b) 4001 case 验 HTTP 200 + envelope.code=4001；(c) MissingUserID case 用 mockUserID = nil；(d) BIGINTIDStringified case 用 math.MaxUint64 或类似极端值

- [x] **Task 5（AC3）**：bootstrap/router.go 三处改动
  - [x] 5.1 Read 现有 `router.go` 完整文件（确认行 312 已建 `chestRepo := repomysql.NewChestRepo(deps.GormDB)`；行 470 已建 `authedGroup.GET("/steps/account", stepsHandler.GetAccount)`）
  - [x] 5.2 Edit 在 `petsHandler := handler.NewPetsHandler(petSvc)` 一行之后**追加** 3 行（含 // Story 20.5 加 注释 + chestSvc + chestHandler 构造）—— **复用** 既有 chestRepo 实例
  - [x] 5.3 Edit 在 `authedGroup.GET("/steps/account", stepsHandler.GetAccount)` 那一行之后**追加一行** `authedGroup.GET("/chest/current", chestHandler.GetCurrent) // Story 20.5 加`
  - [x] 5.4 Read 回检：(a) Deps struct 未改；(b) main.go 未改；(c) chestRepo 仅一处构造（行 312）；(d) chestSvc / chestHandler 构造 2 行；(e) authedGroup.GET 一行新增；(f) 仅 router.go 一个文件改

- [x] **Task 6（AC6）**：集成测试 ≥1 case（dockertest）
  - [x] 6.1 Read `home_service_integration_test.go` 完整文件，找出 `startMySQLContainer` / `insertUser` / `insertUserChest` 等 helper 的真实命名 + 文件位置 + 包路径
  - [x] 6.2 Write `chest_service_integration_test.go`：按 AC6 模板（HappyPath_Counting + 可选 bonus UnlockAtPassed_DynamicReturns2）；`//go:build integration` tag；package 路径与既有集成测试一致（如 `service_test` 或 `service`）
  - [x] 6.3 验证本机 Windows docker 不可用 → t.Skip 不阻塞（startMySQLContainer 内已有 skip 逻辑；本机实跑确认 SKIP 输出正常 —— `bash scripts/build.sh --integration` 必须 BUILD SUCCESS）

- [x] **Task 7（AC7 / AC8）**：全量验证
  - [x] 7.1 `bash scripts/build.sh`（vet + build）过 → BUILD SUCCESS
  - [x] 7.2 `bash scripts/build.sh --test` 全绿（含本 story 新增 ≥10 case）
  - [ ] 7.3 `bash scripts/build.sh --race --test`（本地未跑，等 CI Linux 验证；Windows race tag skip ok）
  - [x] 7.4 `bash scripts/build.sh --integration`（docker 不可用 → t.Skip ok；BUILD SUCCESS）
  - [x] 7.5 `git status --short` 改动文件清单核对（实际 4 新文件 + 1 修改 + 2 流程文件 = 7 文件；与预期范围一致）
  - [x] 7.6 在下方 Completion Notes List 勾选 AC8 验证清单 10 项

- [x] **Task 8（AC9）**：本 story 不做 git commit
  - [x] 8.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 8.2 commit message 模板保留在 story 文件中
  - [x] 8.3 commit hash 待 story-done 阶段回填

## Dev Notes

### 关键设计原则

1. **复用 home_service.go 已有 helper（不重复实装）**：
   `ChestBrief` / `chestStatusDynamic` / `computeRemainingSeconds` 三个 symbol 在 home_service.go 顶部注释明确钦定 "Story 20.5 chest_service.GetCurrent 会**复用**本函数（函数签名 + 行为契约**冻结**）"（home_service.go 行 234-235 + 行 266）。本 story 严格按"消费方"姿态使用，**不**反向迁移到 chest_service.go。**反模式**：本 story 把 `chestStatusDynamic` 复制到 chest_service.go → 两份实装会偏移 → 节点 7 阶段如果某个 case 行为不一致就会出现 "/home 显示 status=1 但 /chest/current 显示 status=2" 的诡异 bug。

2. **NotFound 翻译为 4001（不是 1003 / 不是 1009）**：
   V1 §7.1.6 行 904 钦定 "user 在 user_chests 表中无任何行 → 4001"。区别于 7.4 GET /steps/account 的 NotFound → 1003 决策 —— step_account 是通用资源（NotFound 走通用 1003）；chest 是业务领域专用资源（NotFound 走业务码 4001 让 client 能精确分流到"显示宝箱缺失提示 + 引导重新登录"路径，而非通用 1003）。**反模式**：本 story 也包成 1009 → 客户端无法区分"chest 数据完整性损坏 vs 通用 DB 异常"，运维排查更难（4001 = 查 4.6 firstTimeLogin 是否漏跑；1009 = 查 MySQL 健康）。

3. **扁平响应 schema（不复用 home_handler 的 chest 嵌套块）**：
   V1 §7.1 钦定 `data.{id, status, unlockAt, ...}` 扁平 5 字段；V1 §5.1 GET /home 钦定 `data.chest.{id, status, ...}` 嵌套。两端字段值完全相同（同源 ChestBrief）但嵌套层级不同 —— 与 7.4 vs 7.3 schema 差异同源逻辑（聚合接口嵌套子对象 vs 纯查接口扁平字段）。**反模式**：本 story DTO 包一层 `"chest": gin.H{...}` → 客户端 Codable 解析失败 + 与 V1 §7.1 钦定不符。

4. **动态判定 status / remainingSeconds（不更新 DB）**：
   V1 §7.1 行 909-910 钦定 "server 端 status 由 'DB 原值 + 当前时间' 动态计算，**不**等同于 DB user_chests.status 列值"。每次 GET 都更新 DB 状态会严重放大写入压力（用户每秒可能调一次 GET）+ 无业务意义（status=unlockable 是查询时态判定，无副作用）。**反模式**：本 service 在 GET 路径里做 `UPDATE user_chests SET status = 2` → 每秒 1 次 UPDATE 写库压力放大 60× + 与开箱事务的 status 更新冲突。

5. **remainingSeconds 不为负**：
   `computeRemainingSeconds` 已用 `if diff <= 0 { return 0 }` 兜底；本 service 直接调即可。**反模式**：本 service 自己再算一次 `int64(unlockAt.Sub(now).Seconds())` → 可能返负数（如果 unlock_at 已过）→ 客户端按 UInt 解析会 crash（V1 §7.1 行 911 钦定 client 应按 Int 解析，但 server 端必须保证 ≥ 0）。

6. **time.Now().UTC()（UTC 必须）**：
   chest.UnlockAt 在 chest_repo.go 顶部注释钦定为 UTC（与 V1 §2.5 ISO 8601 UTC 对齐）；service 内 `now` 必须也用 UTC 取，否则 `unlockAt.UTC()` vs `time.Now()`（local loc）做减法会得到错误的 diff 秒数（虽然 time.Sub 内部用 wall + monotonic 不依赖 loc，但 unlock_at 解析时 loc 如果是 Local 而 now 是 Local，跨时区部署会偏；统一 UTC 是最稳的安全默认）。

7. **不接事务（纯读）**：
   与 home_service.LoadHome / step_service.GetAccount 同模式 —— 单表 PK 查询不上事务。理由：(1) 单表 PK 查询本身原子；(2) 事务上下文成本（连接占用 / 锁追踪）对纯读没有收益；(3) 反模式：把所有 service 方法都包 `txMgr.WithTx` 会让事务边界泄露到无意义的地方，掩盖真实事务边界（20.6 开箱事务的 FOR UPDATE 行锁 + 多步骤一致性才是事务的用途）。

8. **复用 chestRepo 实例（router.go 行 312 已建）**：
   4.6 / 4.8 wire 阶段已构造 `chestRepo := repomysql.NewChestRepo(deps.GormDB)` 给 authSvc / homeSvc 用；本 story chestSvc 也复用同一实例。**反模式**：本 story 再调一次 `repomysql.NewChestRepo(deps.GormDB)` → 双实例引入隐性 race / 测试 mock 不一致（11.3 r4 review 钦定的反模式）+ 同一 db handle 多个 wrapper 实例无 ROI。

9. **不预实装 OpenChest 方法**：
   Story 20.6 才实装 POST /chest/open；本 story `ChestService` interface 只声明 `GetCurrent` 一个方法。即使顺手把 `OpenChest(ctx, input) (output, error)` 方法签名加到 interface 里也禁止 —— YAGNI；让 20.6 评审找不到"新增方法"的明确范围边界（与 11.2 / 17.2 / 20.2 "禁止预实装" 同模式）。

10. **GORM driver loc=Local 漂移防御**：
    `chest.UnlockAt.UTC()` 调用强制 UTC 视图（chest_repo.go 顶部注释 + home_service.go 行 216 同模式）—— 即便 DSN loc=Local 导致 GORM 解析 DATETIME(3) 时带 Local loc 标签，本 service 在拼装 ChestBrief 时调 `.UTC()` 把 loc 字段重置为 UTC，向 handler 下传的 time.Time 一定是 UTC loc；handler `time.RFC3339` Format 输出的字符串末尾是 "Z" 而非 "+08:00" 等本地时区后缀（与 V1 §2.5 钦定一致）。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md`）：

- chest 是节点 7 核心可消费资产；本 story 是 chest **状态读取**链路的实装（与 20.6 开箱写入链路并列）
- "状态以 server 为准"原则：客户端任何时候可调本接口拿权威 chest 状态 + 倒计时，不依赖本地缓存

**数据库层**（`docs/宠物互动App_数据库设计.md`）：

- §5.6 user_chests：本 story 消费（`ChestRepo.FindByUserID` 单行查询）
- §6.7 status 枚举：本 story `data.status` 下发 1 / 2 两值（动态判定后）
- 索引：`uk_user_id` UNIQUE 保证查询 ≤ 1 行；走 PK，无需新增索引

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：

- §7.1 接口元信息：HTTP GET / Path /api/v1/chest/current / 需要 Bearer / 限频 60/min/userID / 节点 7
- §7.1.4 服务端逻辑：(1) 认证 & 限频；(2) 单表查询；(3) 动态判定 3 条 case；(4) 响应
- §7.1.2 响应体 schema：**扁平** 5 字段（id / status / unlockAt / openCostSteps / remainingSeconds）
- §7.1.6 错误码：1001 by auth / 1005 by rate_limit / 1009 by service DB 异常兜底 / 4001 by service NotFound 翻译
- §7.1 行 908-915 关键约束：status 动态判定 / remainingSeconds 非负 / 本地倒计时校对 / 首次登录后初值

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：

- §5.1 handler 层：本 story GetCurrent 严格按 handler 职责（DTO 转换 + 不直接接触 *gorm.DB）
- §5.2 service 层：本 story GetCurrent 严格按 service 职责（业务规则薄：单查 + 动态判定 + 错误翻译）
- §5.3 repo 层：本 story 仅消费已实装的 `chest_repo.FindByUserID`，**不**改 repo 层
- §6 internal/repo/mysql/ 目录已锚定；本 story 不新增 mysql repo 文件

**ADR 对齐**：

- ADR-0006 三层错误映射：repo 返哨兵 ErrChestNotFound → service 翻译为 *AppError(4001) → handler c.Error + middleware envelope
- ADR-0007 ctx 传播：service / repo 第一参数 ctx；handler 用 `c.Request.Context()` 不直接传 *gin.Context
- ADR-0006 单一 envelope 生产者：handler 一律 `c.Error + return`，由 ErrorMappingMiddleware 写 envelope

### 关于 Story 20.5 与 7.4 / 11.6 的关键差异

| 维度 | 20.5 GET /chest/current | 7.4 GET /steps/account | 11.6 GET /rooms/current |
|------|---|---|---|
| HTTP method | GET | GET | GET |
| body | 无 | 无 | 无 |
| 事务 | 无 | 无 | 无 |
| 响应 schema | **扁平** 5 字段 | **扁平** 3 字段 | **扁平** 1 字段（currentRoomId） |
| 字段需动态计算 | **是**（status / remainingSeconds） | 否（三档值原值透传） | 否（DB null / 值透传） |
| 错误码全集 | 1001 / 1005 / 1009 / **4001** | 1001 / 1003 / 1005 / 1009 | 1001 / 1005 / 1009 |
| NotFound 翻译 | **4001**（业务专用） | 1003（通用资源） | n/a（CurrentRoomID 字段 nil 即可，无 NotFound 路径） |
| BIGINT 字段 | id 字符串化 | 无 BIGINT id 字段 | currentRoomId 字符串化 |
| 时间字段 | unlockAt RFC3339 | 无时间字段 | 无时间字段 |
| 新建文件 | chest_service.go / chest_handler.go / chest_service_test.go / chest_handler_test.go / chest_service_integration_test.go = 5 个 | step_service.go 末尾追加 + 同模式 | room_service.go 末尾追加 + 同模式 |
| 改动文件数 | 4 新 + 1 改（router）+ 2 流程 = 7 | 4 改 + 1 集成新 + 2 流程 = 7 | 4 改 + 1 集成新 + 2 流程 = 7 |
| 复用既有 helper | **是**（chestStatusDynamic / computeRemainingSeconds / ChestBrief） | 是（StepAccountBrief） | n/a |
| 测试规模 | service 5+ + handler 5+ + 集成 1+ = 11+ | service 4 + handler 4 + 集成 1 = 9 | service 3 + handler 3 + 集成 1 = 7 |

本 story 与 7.4 GET /steps/account 是**最相似**的两条 story（同 "纯读 GET + 扁平响应 + auth + rate_limit + dockertest 集成"），但有 3 个关键差异：

1. **动态判定字段**：本 story 有 `status / remainingSeconds` 需要 service 层算（不是 DB 原值透传）—— 7.4 三档值是 DB 原值透传
2. **业务专用错误码**：4001 vs 1003（业务码 vs 通用码）
3. **新建独立 service / handler 文件**：7.4 是在 step_service.go / steps_handler.go **末尾追加**（共享文件）；本 story **新建** chest_service.go / chest_handler.go（chest 模块从零开始，节点 7 阶段 owner）

### Project Structure Notes

**与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 钦定的 server/ 工程结构对齐**：

```
server/
├─ internal/
│  ├─ app/
│  │  ├─ http/
│  │  │  ├─ handler/
│  │  │  │  ├─ chest_handler.go              # 本 story 新建
│  │  │  │  └─ chest_handler_test.go         # 本 story 新建（≥5 case）
│  │  │  └─ middleware/
│  │  │     └─ auth.go / rate_limit.go / ...  # 已实装；GET /chest/current 与 GET /home 共享同一中间件链
│  │  └─ bootstrap/
│  │     └─ router.go                         # 4.6 / 4.8 已 wire chestRepo；本 story 追加 2 行实例构造 + 1 行路由
│  ├─ service/
│  │  ├─ home_service.go                      # 4.8 已建 ChestBrief + chestStatusDynamic + computeRemainingSeconds；本 story 仅消费
│  │  ├─ chest_service.go                     # 本 story 新建
│  │  ├─ chest_service_test.go                # 本 story 新建（≥5 case）
│  │  └─ chest_service_integration_test.go    # 本 story 新建（≥1 dockertest case）
│  ├─ repo/
│  │  └─ mysql/
│  │     ├─ chest_repo.go                     # 4.6 已建 ChestRepo + FindByUserID + Create + UserChest struct；本 story 仅**消费**
│  │     └─ errors.go                         # 4.6 已建 ErrChestNotFound 哨兵；本 story 仅**消费**
│  └─ pkg/
│     └─ errors/codes.go                      # ErrChestNotFound = 4001 已注册；本 story 仅**消费**
└─ migrations/                                # 0005 已建 user_chests 表；本 story **不**改
```

**变更范围（预期 git status 文件清单，7 文件）**：

1. `server/internal/service/chest_service.go` — **新建** ChestService interface + impl + GetCurrent
2. `server/internal/service/chest_service_test.go` — **新建** ≥5 case + stub chest repo
3. `server/internal/service/chest_service_integration_test.go` — **新建** ≥1 dockertest case
4. `server/internal/app/http/handler/chest_handler.go` — **新建** ChestHandler + GetCurrent + getCurrentChestResponseDTO
5. `server/internal/app/http/handler/chest_handler_test.go` — **新建** ≥5 case + stub chest service
6. `server/internal/app/bootstrap/router.go` — **追加** 2 行实例构造 + 1 行路由
7. `_bmad-output/implementation-artifacts/sprint-status.yaml` — 20-5 状态 backlog → ready-for-dev → in-progress → review → done
8. `_bmad-output/implementation-artifacts/20-5-get-chest-current-接口.md` — 本 story 文件本身

**未变更文件（明确 NOT touch；超出范围 → HALT）**：

- `server/internal/service/home_service.go` —— ChestBrief / chestStatusDynamic / computeRemainingSeconds 在此 + 注释钦定"复用"，本 story 严格消费
- `server/internal/repo/mysql/chest_repo.go` / `errors.go`
- `server/internal/pkg/errors/codes.go` / `apperror.go`
- `server/internal/app/http/handler/home_handler.go`（GET /home 已下发同字段，不复用 inline 块）
- `server/internal/app/http/middleware/*.go`
- `server/cmd/server/main.go` / `internal/app/bootstrap/server.go` / `internal/infra/config/*.go`
- `server/migrations/*.sql`
- `docs/宠物互动App_*.md`（契约**输入**侧）
- `_bmad-output/implementation-artifacts/decisions/*.md`（无新决策）
- `local.yaml` / 任一 `*.yaml`（无新配置项）

### References

**优先级 P0（必读）**：

- [Source: docs/宠物互动App_V1接口设计.md#7.1 获取当前宝箱] — 接口契约定义（行 810-915）；**特别**注意行 836-839 status 动态判定 3 条 case + 行 908-915 关键约束 + 行 851 status 枚举钦定 1 / 2 + 行 850 BIGINT id 字符串化
- [Source: docs/宠物互动App_数据库设计.md#5.6 user_chests] — 表结构（4.6 firstTimeLogin 已建行；本 story 仅查询消费）
- [Source: docs/宠物互动App_数据库设计.md#6.7 user_chests.status 枚举] — status TINYINT 枚举值定义
- [Source: _bmad-output/planning-artifacts/epics.md#Story 20.5] — 本 story 钦定 AC（行 2873-2897）：5 case 单测 / 1 集成 / 4001 错误码
- [Source: server/internal/service/home_service.go] — `ChestBrief` struct + `chestStatusDynamic` + `computeRemainingSeconds` 三个 symbol 定义；本 story 严格"消费方"姿态使用，**不**重复定义
- [Source: server/internal/repo/mysql/chest_repo.go] — `ChestRepo.FindByUserID` 已实装 + `UserChest` struct + `ErrChestNotFound` 哨兵；本 story **仅消费**
- [Source: server/internal/pkg/errors/codes.go] — `ErrChestNotFound = 4001` + `DefaultMessages` 已注册；本 story **仅消费**

**优先级 P1（参考）**：

- [Source: server/internal/service/step_service.go] — service 文件骨架参考（interface + impl + NewXxx + GetAccount 单查模式 / 无事务模式）
- [Source: server/internal/app/http/handler/steps_handler.go] — handler 文件骨架参考（GetAccount 扁平 DTO + userID 取出 + ctx 传播 + c.Error 模式）
- [Source: server/internal/app/http/handler/home_handler.go] — chest 块 DTO 字段命名参考（行 141-147；本 story DTO 字段值完全相同，但扁平 vs 嵌套不同）
- [Source: server/internal/service/step_service_test.go] — service 单测 stub 模式 + *AppError 断言模式
- [Source: server/internal/app/http/handler/steps_handler_test.go] — handler 单测 stub service + 测试 router + decodeEnvelope 模式
- [Source: server/internal/service/home_service_integration_test.go] — 集成测试 dockertest helper 参考（startMySQLContainer / insertUser / insertUserChest 等）
- [Source: server/internal/app/bootstrap/router.go] — 4.6 / 4.8 已 wire `chestRepo`（行 312）+ authedGroup 中间件链；本 story 追加 chestSvc / chestHandler 构造 + 路由一行
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-mapping.md] — ADR-0006 三层错误映射
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md] — ADR-0007 ctx 传播
- [Source: _bmad-output/implementation-artifacts/7-4-get-steps-account-接口.md] — 7.4 完整实装文档（最相似前 story；本 story 复用其落地的所有 stub 模式 + decodeEnvelope 模式）

**优先级 P2（背景）**：

- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5] — 分层职责定义
- [Source: docs/宠物互动App_总体架构设计.md] — "状态以 server 为准"原则
- [Source: docs/宠物互动App_V1接口设计.md#7.2 开启宝箱] — 下游 20.6 接口契约（行 919-1080）；§7.2 行 1207 钦定 `nextChest.{...}` 与 §7.1 同字段对齐
- [Source: _bmad-output/implementation-artifacts/20-4-chest_open_logs-migration.md] — 前序 story（chest_open_logs migration 落地，与本 story 同节点 7 阶段并列；本 story **不**触碰 chest_open_logs，仅 20.6 触碰）
- [Source: _bmad-output/implementation-artifacts/4-8-get-home-聚合接口.md] — Story 4.8 chest 块实装文档（home_service ChestBrief 落地的源 story；本 story 复用其落地的 helper）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

2026-05-14 dev-story 实装：

- 起步检测 `stubChestRepo` 已在 `auth_service_test.go` 第 101 行定义（package service_test 内）—— **不**新建同义 stub，重构 chest_service_test.go 直接复用，避免 stubChestRepo redeclared 编译错。
- `bash scripts/build.sh` BUILD SUCCESS。
- `bash scripts/build.sh --test` 全绿（service 6 case + handler 5 case = 11 case 新增，全过 + 既有全部不回归）。
- `bash scripts/build.sh --integration` BUILD SUCCESS（本机 Windows docker 不可用 → startMySQL 内 t.Skip 兜底；test "ok"）。
- **handler 单测关键发现**：error_mapping.go 钦定 ErrServiceBusy(1009) → HTTP 500；业务码（含 4001）→ HTTP 200；handler 单测 1009 case 必须断 `w.Code == http.StatusInternalServerError`（与 steps_handler_test.go GetAccount_MissingUserID / GetAccount_BusyErr case 同模式；story 文件 AC5 模板里的 "4001 vs 1009 都走 HTTP 200" 不准确，按实装的 ErrorMappingMiddleware 行为为准）。
- File List 总计 8 文件（5 新 server + 1 改 server + 2 流程），与 story Project Structure Notes 预期一致。

### Completion Notes List

AC8 验证清单（10 项人工核对）：

- [x] **#1** service 层 GetCurrent 流程严格按 V1 §7.1.4：FindByUserID → 动态判定 → 拼装 ChestBrief；NotFound 哨兵 → 4001
- [x] **#2** service 层复用 home_service.go 的 `ChestBrief` + `chestStatusDynamic` + `computeRemainingSeconds`（未重复定义）
- [x] **#3** handler DTO 扁平 5 字段（无 chest 子对象）
- [x] **#4** `data.id` 用 `strconv.FormatUint` 字符串化
- [x] **#5** `data.unlockAt` 用 `time.RFC3339`
- [x] **#6** handler 复用 `c.Get(middleware.UserIDKey) + v.(uint64)` 兜底
- [x] **#7** router.go 三处改动符合 AC3（实例构造复用 chestRepo）
- [x] **#8** service 单测 NotFound case 断 4001（非 1003 / 非 1009）
- [x] **#9** handler 单测 happy case 断 `data` 无 chest 子对象 + `data.id` 是 string
- [x] **#10** `bash scripts/build.sh --test` 全绿（含本 story 新增 ≥10 case）

### File List

实装文件（5 个 server 文件 = 4 新建 + 1 修改）：

- `server/internal/service/chest_service.go` — **新建** ChestService interface + chestServiceImpl + NewChestService + GetCurrent 实装
- `server/internal/service/chest_service_test.go` — **新建** 6 case 单测（HappyPath_Counting / HappyPath_Unlockable_DBStatus1_UnlockAtPassed / HappyPath_DBStatus2 / ChestNotFound_4001 / ClockBoundary / RepoOtherDBError_1009）；复用 auth_service_test.go 已定义的 stubChestRepo
- `server/internal/app/http/handler/chest_handler.go` — **新建** ChestHandler + NewChestHandler + GetCurrent + getCurrentChestResponseDTO
- `server/internal/app/http/handler/chest_handler_test.go` — **新建** 5 case 单测（HappyPath_FlatSchema / BIGINTIDStringified / 4001_HTTP200 / 1009_HTTP500 / MissingUserID_1009）
- `server/internal/app/bootstrap/router.go` — **修改** 追加 2 行实例构造（chestSvc + chestHandler，复用既有 chestRepo）+ 1 行路由（authedGroup.GET /chest/current）

集成测试（1 个新文件）：

- `server/internal/service/chest_service_integration_test.go` — **新建** 2 dockertest case（HappyPath_Counting / UnlockAtPassed_DynamicReturns2）

流程文件（2 个修改）：

- `_bmad-output/implementation-artifacts/sprint-status.yaml` — 20-5 状态 backlog → ready-for-dev → in-progress → review；last_updated 同步
- `_bmad-output/implementation-artifacts/20-5-get-chest-current-接口.md` — 本文件（Status / Tasks / Dev Agent Record / File List / Change Log 更新）

**总计 8 文件**，与 story Project Structure Notes 钦定预期一致。

### Change Log

| 日期 | 改动 | 状态变更 |
|---|---|---|
| 2026-05-14 | Story 20.5 created (backlog → ready-for-dev) | backlog → ready-for-dev |
| 2026-05-14 | dev-story 实装完成：service + handler + router + 6 单测 + 5 handler 测 + 2 集成 case；build + test + integration 全 BUILD SUCCESS | ready-for-dev → in-progress → review |
