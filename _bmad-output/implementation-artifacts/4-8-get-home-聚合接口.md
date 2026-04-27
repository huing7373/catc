# Story 4.8: GET /home 聚合接口（initial 版含 user + pet + stepAccount + chest）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want App 启动后调 `GET /api/v1/home` 一次拿到主界面所需的全部数据（user / pet / stepAccount / chest / room 五段），
so that 客户端首屏不再串行调 5 个 API（user / pet / step / chest / room），节省 ~800ms 首屏时间 + iOS 端错误处理代码减半（节点 2 §4.2 验收硬指标）；server 端在同一次请求里聚合查 5 张表的数据并按 V1 §5.1 钦定 schema 一次性下发；任一聚合查询失败 → 整体 1009 服务繁忙（**不部分降级** —— 避免主界面渲染异常引发更深的客户端错误链）。

## 故事定位（Epic 4 第七条 = 节点 2 server 第二个真正的业务 handler；上承 4.6 游客登录初始化事务，下启 4.7 Layer 2 集成测试 + 5.5 LoadHomeUseCase）

- **Epic 4 进度**：4.1 (契约定稿，done) → 4.2 (MySQL + tx manager，done) → 4.3 (5 张表 migrations，done) → 4.4 (token util，done) → 4.5 (auth + rate_limit 中间件，done) → 4.6 (游客登录 handler + 首次初始化事务，done) → **4.8 (本 story，GET /home 聚合接口)** → 4.7 (Layer 2 集成测试)。**注意 sprint-status.yaml 物理顺序**：4.8 在 4.7 之前，因为 4.7 是 Epic 4 收尾性 Layer 2 集成测试，需要 4.6 + 4.8 都落地后再做整体回归。
- **epics.md AC 钦定**：`_bmad-output/planning-artifacts/epics.md` §Story 4.8（行 1106-1137）已**精确**列出 AC：
  - `GET /home` service 流程：**一次性查询** users + pets (默认猫) + user_pet_equips（节点 2 阶段为空）+ user_step_accounts + user_chests + users.current_room_id
  - 返回 V1 §5.1 钦定 schema（`user / pet / stepAccount / chest / room` 五段）
  - **pet.equips 节点 2 阶段强制返回 `[]`**（节点 9 由 Story 26.6 填充真实数据）
  - **room.currentRoomId 节点 2 阶段强制返回 `null`**（节点 4 由 Story 11.10 填充）
  - **chest 字段直接复用 Story 20.5 的动态判定逻辑**（节点 7 完成后才有 unlockable 状态判断；节点 2 阶段所有 chest 都是 counting / 倒计时已过的视为 unlockable）
  - 接口要求 **auth**（Bearer token）
  - **单测覆盖 ≥ 4 case**（mocked repo）：登录后立即调（user + 默认 pet + step 全 0 + chest counting）/ chest unlock_at 已过（status=2 unlockable）/ 用户无 pet（pet 字段返回 null）/ 各部分 repo 错误（整体 1009 服务繁忙，**不部分降级**）
  - **集成测试覆盖**（dockertest）：创建 user + 默认 pet + step_account + chest → curl GET /home → response 全部字段正确
- **V1 §5.1 钦定 schema**（`docs/宠物互动App_V1接口设计.md` 行 308-450，已冻结契约）：
  - **路径**：`GET /api/v1/home`，**走** auth 中间件（Bearer token），**走** rate_limit 中间件（userID 维度，与 4.5 已就位的 `RateLimitByUserID` extractor 对接）
  - **响应 data**：见 §5.1 字段表（详见本 story AC2 完整 schema 复刻）；**关键变化点**对比 4.6 / V1 §4.1：
    - `data.pet` **可空**（V1 §5.1 行 335 + Story 4.1 round 5 lesson `2026-04-26-契约schema字段可空性必须显式声明.md` 钦定）
    - `data.pet.currentState` 是新字段（V1 §4.1 没有；本接口必带，节点 2 阶段读 `pets.current_state` 字段，初始为 1 = rest）
    - `data.pet.equips` 节点 2 强制 `[]`（**不**查 user_pet_equips；本 story repo 不引入 EquipRepo）
    - `data.chest.status` 是**动态判定**：DB `user_chests.status` 是 1（counting，登录初始化时写死）/ 2（unlockable，节点 7 chest_service 才会写），节点 2 阶段所有 DB 行 status 字段都是 1，但本 service 必须按 `time.Now().UTC() >= unlock_at ? 2 : 1` 动态计算下发值（V1 §5.1 行 345 + epics.md 行 1133 钦定）
    - `data.chest.remainingSeconds` 是**动态计算**字段：`max(0, int(unlock_at - now))` 秒
    - `data.room.currentRoomId` 节点 2 强制 `null`（**不**读 `users.current_room_id`，节点 2 阶段该字段即便有值也按 null 返）；节点 4 由 Story 11.10 改为读真实 `users.current_room_id`
  - **错误码**：1001 未登录 / token 无效 / 1009 服务繁忙（任一聚合查询失败）
- **数据库设计层**（`docs/宠物互动App_数据库设计.md`）：
  - §5.1 users（id / nickname / avatar_url / current_room_id 等）
  - §5.3 pets（id / pet_type / name / current_state / is_default 等；`is_default=1` 是默认猫筛选条件）
  - §5.4 user_step_accounts（user_id PK / total_steps / available_steps / consumed_steps）
  - §5.6 user_chests（id / user_id UK / status / unlock_at / open_cost_steps）
  - §6.4 pets.current_state 枚举（1=rest, 2=walk, 3=run）
  - §6.7 user_chests.status 枚举（1=counting, 2=unlockable）
- **设计文档 §6.2 钦定 home 模块**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §6.2 行 364-389 钦定 "User / Home 模块"：
  - 关联接口：`GET /api/v1/me` / **`GET /api/v1/home`**
  - 关联表：users / pets / user_step_accounts / user_chests / user_pet_equips / rooms
  - **§6.2 关键钦定**：`/home` 是典型聚合接口，建议**单独有 `home_service.go`**，不要把首页拼装逻辑散在多个 handler 中。
- **下游立即依赖**：
  - **Story 4.7 Layer 2 集成测试**：节点 2 阶段 Layer 2 集成测试覆盖**游客登录 + 首页拉取**全流程（4.6 + 4.8 联动）。本 story 必须保证 service.LoadHome 的失败路径（mock 任一 repo 抛 error）确实返 1009（**不部分降级**），4.7 才能用 fault injection 覆盖回滚 / 边界 / 重入。
  - **Story 5.5 iOS LoadHomeUseCase**：iOS 客户端用 SilentRelogin 拿到 token 后立即调 `GET /home` → 拿 user / pet / stepAccount / chest 渲染主界面；本 story response schema 任何字段名 / 类型 / 可空性变更都要触发 iOS 重写。**关键**：iOS 端 PetDTO 必须按 `Optional<PetDTO>` 解析（V1 §5.1 行 335 钦定 `data.pet` 可空），本 story handler 必须在 pet 缺失时返 `"pet": null`（**不是**返一个 `{}` 占位）。
  - **Story 11.10**（节点 4 房间 epic）：扩展 `room.currentRoomId` 从硬编码 `null` 改为读 `users.current_room_id`；本 story 写的 home_service 必须**预留 room 段落的扩展点**，让 11.10 只需要在 service 层加一行查询 + 替换 `nil` → 真实值，**不需要**改 handler / DTO 结构。
  - **Story 26.6**（节点 9 穿戴 epic）：扩展 `pet.equips` 从硬编码 `[]` 改为读 `user_pet_equips JOIN cosmetic_items`；本 story 必须**预留 equips 段落的扩展点**（DTO 字段名 / 嵌套结构对齐 V1 §5.1 节点 9 阶段示例）。
  - **Story 20.5**（节点 7 chest_service.GetCurrent）：会复用本 story 的"chest.status 动态判定 + remainingSeconds 计算"逻辑；epics.md §Story 4.8 行 1133 钦定 "chest 字段直接复用 Story 20.5 的动态判定逻辑"。本 story 必须**首次落地**该判定逻辑（命名 `chestStatusDynamic` / `computeRemainingSeconds`），20.5 来时直接复用而非重写。
- **范围红线**：本 story **新增** `home_service.go` + `home_handler.go` + 在已有 5 个 mysql repo 上**扩展** Find 方法（`UserRepo.FindByID` 已有，复用；`PetRepo` 加 `FindDefaultByUserIDOptional` 区分"无默认猫"情况；`StepAccountRepo.FindByUserID`、`ChestRepo.FindByUserID` 是新方法）+ wire 进 router `/api/v1` 已认证子组 + handler/service 单测 + dockertest 集成测试；**不**实装 GET /me handler（V1 §4.3 锚定但 Epic 4 范围内不做）；**不**实装 GET /home 节点 9+ 的真实 equips（Story 26.6 落地）；**不**实装真实 currentRoomId 读取（Story 11.10 落地）。

**本 story 不做**（明确范围红线）：

- [skip] **不**实装 GET /me handler（V1 §4.3 锚定但非本 story AC；epics.md §Story 4.8 没有 /me；future Epic 收尾决策）
- [skip] **不**实装 POST /pets/current/state-sync handler（V1 §5.2，节点 5 才上线）
- [skip] **不**查询 user_pet_equips 表（节点 2 阶段强制返回 `[]`；不引入 EquipRepo —— Story 26.6 才需要新建）
- [skip] **不**读 `users.current_room_id` 字段（即使该列已存在，节点 2 阶段强制返回 `null` —— 避免给客户端假信号；Story 11.10 才接真值）
- [skip] **不**实装 chest 开箱后重建逻辑（节点 7 / Epic 20 chest_service 才有 chest_open_logs 表 + opened 后建下一轮 chest）
- [skip] **不**写 idempotencyKey header 处理（GET 是幂等的，HTTP 语义保证；不需要应用层幂等）
- [skip] **不**改 5 张表的 migrations（4.3 已 done；本 story 仅消费）
- [skip] **不**改 4.5 中间件（auth + rate_limit 已 done；本 story 仅 wire 进 router 已认证子组用 RateLimitByUserID）
- [skip] **不**改 4.4 token util（仅消费 `Auth(signer)` 工厂，4.5 已就位）
- [skip] **不**改 4.6 auth_service / auth_handler（不互相依赖；home_service 不调 auth_service）
- [skip] **不**改 4.2 db / tx manager（仅消费 `*gorm.DB`，**不**走 tx —— GET /home 全是只读查询，无需事务）
- [skip] **不**修改 `docs/宠物互动App_*.md` 任一份（V1 §5.1 / 数据库设计 §5/§6 / 设计文档 §6.2 是契约**输入**，本 story 严格对齐但**不**修改）
- [skip] **不**写 README / 部署文档：留 Epic 4 收尾或 Story 6.3 文档同步阶段
- [skip] **不**给 GET /me 提前占位（即使空 handler）—— future story 真正落地时新建
- [skip] **不**用反射 / 依赖注入框架（wire / fx）：沿用 4.6 已建立的显式 DI（bootstrap.NewRouter 内 wire）
- [skip] **不**实装"主界面缓存"（Cache-Control / ETag）—— 节点 2 阶段每次都是直查 MySQL；future Epic 引入 Redis 后再考虑（NFR4 钦定，但节点 2 阶段单实例 + 数据量小，足够直查）
- [skip] **不**用并发查询 / errgroup 拆并行调 4 个 repo —— MVP 阶段单 user 单查询，串行 4 个 repo 调用 < 50ms（GORM 连接池复用），**简单优于过早优化**；future Epic 性能 epic 才引入 errgroup（详见 Dev Notes "查询拆分 vs 串行"段）

## Acceptance Criteria

**AC1 — `internal/app/http/handler/home_handler.go`：首页聚合 handler**

新增 `server/internal/app/http/handler/home_handler.go`，提供：

```go
package handler

import (
    "strconv"

    "github.com/gin-gonic/gin"

    "github.com/huing/cat/server/internal/app/http/middleware"
    "github.com/huing/cat/server/internal/pkg/response"
    "github.com/huing/cat/server/internal/service"
)

// HomeHandler 是 /home 路由的 handler。
//
// 节点 2 阶段仅 LoadHome（GET /api/v1/home）；future epic 加 GET /me 等同模块路由。
type HomeHandler struct {
    svc service.HomeService
}

// NewHomeHandler 构造 HomeHandler。
func NewHomeHandler(svc service.HomeService) *HomeHandler {
    return &HomeHandler{svc: svc}
}

// LoadHome 处理 GET /api/v1/home。
//
// 流程：
//  1. 从 gin.Context.Keys 取 userID（middleware.UserIDKey 由 Auth 中间件注入）
//  2. 调 svc.LoadHome(ctx, userID) 一次性聚合查询
//  3. 成功 → response.Success(c, dto, "ok")
//  4. 失败 → c.Error(err) + return（让 ErrorMappingMiddleware 写 envelope）
//
// **关键**：本 handler 不做参数校验（路径 + auth 头由中间件兜底）；userID 必然存在
// （Auth 中间件挂在前；不存在 → 已被 1001 拦截）；类型断言失败 → 走 1009 兜底（unreachable
// 但保险起见，与 4.5 RateLimitByUserID 同模式）。
//
// **ADR-0006 单一 envelope 生产者**：handler 不直接 response.Error 写 envelope。
//
// **ADR-0007 §2.2 ctx 传播**：用 c.Request.Context()，**不**直接传 *gin.Context。
func (h *HomeHandler) LoadHome(c *gin.Context) {
    v, ok := c.Get(middleware.UserIDKey)
    if !ok {
        // unreachable: Auth 中间件挂在前，userID 必然存在
        _ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
        return
    }
    userID, ok := v.(uint64)
    if !ok {
        // unreachable: Auth 中间件 c.Set(UserIDKey, claims.UserID) 永远是 uint64
        _ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
        return
    }

    out, err := h.svc.LoadHome(c.Request.Context(), userID)
    if err != nil {
        _ = c.Error(err)
        return
    }

    response.Success(c, homeResponseDTO(out), "ok")
}

// homeResponseDTO 把 service 输出转成 V1 §5.1 钦定的 wire 格式。
//
// 关键转换：
//   - BIGINT id 用 strconv.FormatUint 转 string（V1 §2.5 钦定）
//   - pet 可空：out.Pet == nil → "pet": null（**不**是 {} 空对象）
//   - chest.unlockAt 用 RFC3339 格式（time.Time.Format(time.RFC3339)）
//   - chest.remainingSeconds 已在 service 层算好（int 秒数；可能为 0）
//   - room.currentRoomId 节点 2 阶段固定 nil（gin.H{"currentRoomId": nil} 序列化为 null）
//
// **V1 §5.1 节点 2 阶段必须严格返回的字段集**（任何缺字段都会导致 iOS DTO 解码失败）：
//   - data.user: id / nickname / avatarUrl
//   - data.pet（可空）: id / petType / name / currentState / equips（[]）
//   - data.stepAccount: totalSteps / availableSteps / consumedSteps
//   - data.chest: id / status / unlockAt / openCostSteps / remainingSeconds
//   - data.room: currentRoomId（null）
func homeResponseDTO(out *service.HomeOutput) gin.H {
    var petDTO any // 显式 any 类型，让 nil 序列化为 JSON null
    if out.Pet != nil {
        petDTO = gin.H{
            "id":           strconv.FormatUint(out.Pet.ID, 10),
            "petType":      out.Pet.PetType,
            "name":         out.Pet.Name,
            "currentState": out.Pet.CurrentState,
            "equips":       []any{}, // 节点 2 阶段强制 []，**不**用 nil（nil 序列化为 null）
        }
    }
    return gin.H{
        "user": gin.H{
            "id":        strconv.FormatUint(out.User.ID, 10),
            "nickname":  out.User.Nickname,
            "avatarUrl": out.User.AvatarURL,
        },
        "pet": petDTO,
        "stepAccount": gin.H{
            "totalSteps":     out.StepAccount.TotalSteps,
            "availableSteps": out.StepAccount.AvailableSteps,
            "consumedSteps":  out.StepAccount.ConsumedSteps,
        },
        "chest": gin.H{
            "id":               strconv.FormatUint(out.Chest.ID, 10),
            "status":           out.Chest.Status,
            "unlockAt":         out.Chest.UnlockAt.Format(time.RFC3339),
            "openCostSteps":    out.Chest.OpenCostSteps,
            "remainingSeconds": out.Chest.RemainingSeconds,
        },
        "room": gin.H{
            "currentRoomId": nil, // 节点 2 阶段强制 null（Story 11.10 节点 4 才接真值）
        },
    }
}
```

**关键设计约束**：

- **handler 层只取 ctx + userID + 调 service + 返 envelope**：不写业务规则（设计文档 §6.2 钦定 home_service.go 单独承载首页拼装逻辑）；不直接 import gorm / mysql 类型
- **userID 从 gin.Context.Keys 取**（用 4.5 已就位的 `middleware.UserIDKey` 常量），**不**从 ctx.Value 取（ADR-0007 §6 钦定不用 ctx.Value 传业务字段）
- **petDTO 用 any 类型 + 显式 nil 让 gin.H 序列化为 JSON null**：`gin.H{"pet": nil}` 序列化为 `"pet": null`；如果用 `gin.H{}`（空 map）会序列化为 `"pet": {}`，违反 V1 §5.1 行 335 钦定的 "可空对象解析"
- **equips 用 `[]any{}` 而非 `nil`**：nil slice 序列化为 JSON null（违反 V1 §5.1 行 340 钦定 "节点 2 阶段强制返回 `[]`"）；`[]any{}` 序列化为 `[]`
- **unlockAt 格式用 `time.RFC3339`**（=`2006-01-02T15:04:05Z07:00`）：与 V1 §2.5 钦定 ISO 8601 UTC 兼容；**不要**用 `time.RFC3339Nano`（毫秒精度对客户端无意义，徒增字节）
- **错误一律 c.Error + return**：handler 不直接 response.Error 写 envelope（ADR-0006 单一生产者；与 4.5 / 4.6 同模式）
- **id 转字符串用 strconv.FormatUint**：而非 `fmt.Sprintf("%d", ...)`（更快 + 不依赖 fmt reflect；与 4.6 同模式）
- **response.Success 第三参数 message 固定 "ok"**：V1 §2.4 envelope.message 在成功时为 "ok"（与 4.6 同模式）

**关键反模式**：

- [bad] **不**在 handler 直接调 `*gorm.DB`（违反设计文档 §5.1）
- [bad] **不**做"先调 svc.GetUser → 再调 svc.GetPet → 再调 svc.GetStep …" 拆分 —— 单一入口 svc.LoadHome 一次拼装；handler 不做编排（编排归 service 层）
- [bad] **不**用 `c.Bind(...)` —— GET 没有 body，没有参数校验需求
- [bad] **不**用 c.JSON(http.StatusBadRequest, ...) 直接写 400：V1 §2.4 钦定业务码与 HTTP status 正交
- [bad] **不**接 `idempotencyKey` header —— GET 是幂等的，HTTP 语义保证
- [bad] **不**返回额外的 device / session 信息：V1 §5.1 response 只含 user / pet / stepAccount / chest / room 五段
- [bad] **不**给 pet=nil 时返 `gin.H{}` —— `{}` 是空对象不是 null，违反 schema 可空声明
- [bad] **不**给 equips 字段返 nil（除非 pet 整个为 nil）—— 节点 2 阶段必须 `[]`
- [bad] **不**返回 `currentRoomId: ""`（空串）—— V1 §5.1 钦定 `string | null`，节点 2 阶段必须 `null`

**AC2 — `internal/service/home_service.go`：首页聚合 service**

新增 `server/internal/service/home_service.go`，提供：

```go
package service

import (
    "context"
    "errors"
    "time"

    apperror "github.com/huing/cat/server/internal/pkg/errors"
    "github.com/huing/cat/server/internal/repo/mysql"
)

// HomeService 是 home handler 的依赖 interface（便于 handler 单测 mock）。
type HomeService interface {
    // LoadHome: 一次性聚合查询主界面所需全部数据。
    //
    // 流程：
    //  1. userRepo.FindByID(ctx, userID) → 拿 user
    //  2. petRepo.FindDefaultByUserID(ctx, userID) → 拿默认 pet（可能 ErrPetNotFound）
    //  3. stepAccountRepo.FindByUserID(ctx, userID) → 拿 step_account
    //  4. chestRepo.FindByUserID(ctx, userID) → 拿 user_chest
    //  5. 在 service 层动态判定 chest.status 与 remainingSeconds（基于 time.Now().UTC() vs unlockAt）
    //  6. 拼装 HomeOutput 返回
    //
    // 错误约定：
    //   - userRepo 失败（含 ErrUserNotFound）→ 1009（user 必须存在；登录后 token 已校验过 userID）
    //   - petRepo NotFound → 不视为错误，HomeOutput.Pet = nil（V1 §5.1 钦定可空）
    //   - petRepo 其他失败 → 1009
    //   - stepAccountRepo / chestRepo 失败（含 NotFound） → 1009（这两张表登录初始化必建，缺即数据脏）
    //
    // **不部分降级**：epics.md §Story 4.8 行 1136 钦定 "各部分 repo 错误 → 整体 1009 服务繁忙，不部分降级，
    // 避免主界面渲染异常"。
    LoadHome(ctx context.Context, userID uint64) (*HomeOutput, error)
}

// HomeOutput 是 service 层 DTO（**不是** wire DTO，handler 转换）。
//
// 字段语义：
//   - User: 必有（登录后 user 必然存在；查询失败 → 1009）
//   - Pet: 可空（用户无默认 pet → nil；V1 §5.1 钦定 edge case）
//   - StepAccount: 必有（登录初始化时已建；缺 → 1009）
//   - Chest: 必有 + Status / RemainingSeconds 已动态计算（不是 DB 原值）
type HomeOutput struct {
    User        UserBrief
    Pet         *PetBrief // 可空
    StepAccount StepAccountBrief
    Chest       ChestBrief
}

type UserBrief struct {
    ID        uint64
    Nickname  string
    AvatarURL string
}

type PetBrief struct {
    ID           uint64
    PetType      int
    Name         string
    CurrentState int
}

type StepAccountBrief struct {
    TotalSteps     uint64
    AvailableSteps uint64
    ConsumedSteps  uint64
}

// ChestBrief 含**动态判定后**的字段（Status / RemainingSeconds 由 service 算，不是 DB 原值）。
type ChestBrief struct {
    ID               uint64
    Status           int       // 1=counting / 2=unlockable（动态）
    UnlockAt         time.Time // UTC（与 V1 §2.5 一致）
    OpenCostSteps    uint32
    RemainingSeconds int64 // max(0, int64(unlockAt - now))
}

// homeServiceImpl 是 HomeService 的默认实装。
type homeServiceImpl struct {
    userRepo        mysql.UserRepo
    petRepo         mysql.PetRepo
    stepAccountRepo mysql.StepAccountRepo
    chestRepo       mysql.ChestRepo
    // **关键**：不依赖 authBindingRepo（auth_binding 表查询是 4.6 / 5.4 的事）
    // 不依赖 txMgr（GET /home 全是只读查询，无事务需求）
    // 不依赖 signer（auth 中间件已校验 token）
}

// NewHomeService 构造 HomeService。
func NewHomeService(
    userRepo mysql.UserRepo,
    petRepo mysql.PetRepo,
    stepAccountRepo mysql.StepAccountRepo,
    chestRepo mysql.ChestRepo,
) HomeService {
    return &homeServiceImpl{
        userRepo:        userRepo,
        petRepo:         petRepo,
        stepAccountRepo: stepAccountRepo,
        chestRepo:       chestRepo,
    }
}

// LoadHome 实装 4 个 repo 串行查询 + chest 动态判定。
//
// **串行 vs 并发**：MVP 阶段用 4 个串行调用（user → pet → step → chest）；不引入 errgroup 并发。
// 理由：
//  1. 单 user 单查询，4 个简单 SELECT < 50ms（GORM 连接池复用）
//  2. errgroup 引入额外复杂度（cancel 传播 / error 收敛），节点 2 不需要
//  3. 节点 36 性能 epic 才考虑并发；MVP 简单优于过早优化
//
// **chest 动态判定**：DB user_chests.status 字段是登录初始化时写死的 1（counting），节点 2 阶段
// 不会被 update（开箱功能在 Epic 20 才上线）。但 V1 §5.1 钦定客户端期望"unlock_at 已过 → status=2 unlockable"，
// 所以 service 必须基于 time.Now().UTC() 与 unlockAt 比较动态计算下发的 status 值（不写回 DB）。
func (s *homeServiceImpl) LoadHome(ctx context.Context, userID uint64) (*HomeOutput, error) {
    // (1) user
    user, err := s.userRepo.FindByID(ctx, userID)
    if err != nil {
        // 即便是 ErrUserNotFound 也包成 1009（auth 中间件已校验 token，user 必须存在；不存在 → 数据脏）
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
    }

    // (2) pet（可空）
    var petBrief *PetBrief
    pet, err := s.petRepo.FindDefaultByUserID(ctx, userID)
    if err != nil {
        if errors.Is(err, mysql.ErrPetNotFound) {
            // 用户无默认 pet（V1 §5.1 edge case）：petBrief 留 nil
            petBrief = nil
        } else {
            return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
        }
    } else {
        petBrief = &PetBrief{
            ID:           pet.ID,
            PetType:      int(pet.PetType),
            Name:         pet.Name,
            CurrentState: int(pet.CurrentState),
        }
    }

    // (3) stepAccount
    stepAccount, err := s.stepAccountRepo.FindByUserID(ctx, userID)
    if err != nil {
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
    }

    // (4) chest
    chest, err := s.chestRepo.FindByUserID(ctx, userID)
    if err != nil {
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
    }

    // (5) chest 动态判定（Story 20.5 会复用本逻辑）
    now := time.Now().UTC()
    chestStatus := chestStatusDynamic(chest.Status, chest.UnlockAt, now)
    remainingSeconds := computeRemainingSeconds(chest.UnlockAt, now)

    return &HomeOutput{
        User: UserBrief{
            ID:        user.ID,
            Nickname:  user.Nickname,
            AvatarURL: user.AvatarURL,
        },
        Pet: petBrief,
        StepAccount: StepAccountBrief{
            TotalSteps:     stepAccount.TotalSteps,
            AvailableSteps: stepAccount.AvailableSteps,
            ConsumedSteps:  stepAccount.ConsumedSteps,
        },
        Chest: ChestBrief{
            ID:               chest.ID,
            Status:           chestStatus,
            UnlockAt:         chest.UnlockAt.UTC(), // 强制 UTC（DB 字段已是 UTC，但 GORM 解析时可能带本地时区）
            OpenCostSteps:    chest.OpenCostSteps,
            RemainingSeconds: remainingSeconds,
        },
    }, nil
}

// chestStatusDynamic 基于当前时间与 unlockAt 动态判定 chest 状态。
//
// 节点 2 阶段：DB 原值固定为 1 (counting)；超过 unlockAt → 下发 2 (unlockable)。
// Story 20.5 节点 7 chest_service.GetCurrent 会复用本函数。
//
// 参数：
//   - dbStatus: DB 中 user_chests.status 原值（节点 2 阶段始终 1）
//   - unlockAt: DB 中 user_chests.unlock_at（UTC）
//   - now: 当前时间（UTC，调用方传入便于单测注入）
//
// 返回：1 (counting) 或 2 (unlockable)
//
// **节点 2 阶段简化**：dbStatus 始终 1，所以函数等价于 `if now >= unlockAt then 2 else 1`。
// 但本函数保留 dbStatus 参数是为节点 7 准备：Story 20.5 chest_service 在用户开箱后会把
// status 写为其他值（如 3=opened，但该值在 V1 §5.1 钦定 /home 永远不返），届时本函数
// 会扩成 switch 语句穷举节点 7 阶段的 status 值集。
func chestStatusDynamic(dbStatus int8, unlockAt, now time.Time) int {
    // 节点 2 阶段：dbStatus 永远 1 (counting)；忽略 dbStatus 参数（保留为 future-proof）
    _ = dbStatus
    if !now.Before(unlockAt) { // now >= unlockAt
        return 2 // unlockable
    }
    return 1 // counting
}

// computeRemainingSeconds 计算距离 unlockAt 的剩余秒数。
//
// 返回值：
//   - 已过 unlockAt：0（**不**返回负数 —— V1 §5.1 钦定 "> 0 表示 counting，≤ 0 表示已可开启"，
//     0 是边界值；客户端按 ≤0 判 unlockable，等价 status=2）
//   - 未过 unlockAt：剩余整秒数（向下取整）
//
// 注意：用 int64 而非 int，避免 32-bit 平台 overflow（unlockAt 可能在远未来）。
func computeRemainingSeconds(unlockAt, now time.Time) int64 {
    diff := unlockAt.Sub(now)
    if diff <= 0 {
        return 0
    }
    return int64(diff.Seconds())
}
```

**关键设计约束**：

- **service 层是聚合 + 动态判定的归属**：4 个 repo 串行调用 + chest 动态判定（status + remainingSeconds）+ 拼装 DTO；不依赖 txMgr（只读查询无事务）
- **依赖 4 个 repo（不要 5 个）**：authBindingRepo 不需要（home 不查 binding 表）；signer 不需要（auth 中间件已校验 token）
- **pet 可空 = 唯一不视为错误的 NotFound**：V1 §5.1 行 335 钦定 pet 容器可空；其他 3 张表（user / step_account / chest）的 NotFound 都包成 1009（数据脏）
- **chest.status 是动态判定结果（不是 DB 原值）**：V1 §5.1 行 345 钦定客户端按 `time.Now()` vs `unlockAt` 看到 1/2；service 必须算好下发；DB user_chests.status 字段节点 2 阶段恒为 1，本接口**不**修改 DB
- **chestStatusDynamic / computeRemainingSeconds 函数命名稳定**：Story 20.5 复用 → 函数签名 + 行为契约**冻结**；节点 7 扩 status 值集时只能在 chestStatusDynamic 内加 switch case，**不能改函数签名**
- **chest.UnlockAt 强制 .UTC()**：DB DATETIME(3) 不带时区，GORM 解析回 Go time.Time 时可能带 local 时区（取决于 driver loc 参数）；service 层强制 .UTC() 防止 wire 端漂移
- **HomeOutput.Pet 是指针**（`*PetBrief`）：nil 表示可空；与 Go 惯用的可空表达式一致；handler 用 `out.Pet == nil` 判断走 null 分支
- **errors.Is(err, mysql.ErrPetNotFound) 而非 errors.Is(err, gorm.ErrRecordNotFound)**：repo 层已翻译为哨兵 error（4.6 已落地 ErrPetNotFound 哨兵）；service 用 sentinel 而非 gorm 内部 error，遵循 ADR-0006 三层映射

**关键反模式**：

- [bad] **不**用 `errgroup` 并发查询 4 个 repo（MVP 阶段串行简单 + 性能足够）
- [bad] **不**包事务（只读查询无意义；浪费连接池 + 锁开销）
- [bad] **不**把 chest.status 写回 DB（节点 2 阶段动态判定，DB 原值不变；写回会引入并发 race + 与 Epic 20 chest_service 边界冲突）
- [bad] **不**把 pet NotFound 包成 1009（V1 §5.1 钦定 pet 可空，包成 1009 会让客户端 edge case 测试失败 → epics.md §Story 4.8 AC 失败）
- [bad] **不**把 user / step / chest NotFound 视为可空（这三张表登录初始化必建；缺即数据脏，必须 1009 让客户端 SilentRelogin 兜底）
- [bad] **不**调 4.6 auth_service 的方法（home_service 不应跨 service 调用；如需共享 user 数据 → 走 userRepo）
- [bad] **不**把 chestStatusDynamic / computeRemainingSeconds 写在 chestRepo 里（动态判定是业务逻辑，归属 service 层；repo 只做 CRUD）
- [bad] **不**用 `pet.UnlockAt.Add(0).UTC()` 类奇怪写法 —— 直接 `.UTC()` 即可

**AC3 — 在已有 mysql repo 上扩展 Find 方法**

修改 `server/internal/repo/mysql/step_account_repo.go`：加 `FindByUserID`：

```go
type StepAccountRepo interface {
    Create(ctx context.Context, a *StepAccount) error

    // FindByUserID 查指定 user 的步数账户。
    // NotFound → ErrStepAccountNotFound 哨兵；其他 DB error 透传。
    FindByUserID(ctx context.Context, userID uint64) (*StepAccount, error)
}

func (r *stepAccountRepo) FindByUserID(ctx context.Context, userID uint64) (*StepAccount, error) {
    db := tx.FromContext(ctx, r.db)
    var a StepAccount
    err := db.WithContext(ctx).Where("user_id = ?", userID).First(&a).Error
    if err != nil {
        if stderrors.Is(err, gorm.ErrRecordNotFound) {
            return nil, ErrStepAccountNotFound
        }
        return nil, err
    }
    return &a, nil
}
```

修改 `server/internal/repo/mysql/chest_repo.go`：加 `FindByUserID`：

```go
type ChestRepo interface {
    Create(ctx context.Context, c *UserChest) error

    // FindByUserID 查指定 user 的当前宝箱（user_chests.uk_user_id 唯一约束保证 ≤ 1 行）。
    // NotFound → ErrChestNotFound 哨兵；其他 DB error 透传。
    FindByUserID(ctx context.Context, userID uint64) (*UserChest, error)
}

func (r *chestRepo) FindByUserID(ctx context.Context, userID uint64) (*UserChest, error) {
    db := tx.FromContext(ctx, r.db)
    var c UserChest
    err := db.WithContext(ctx).Where("user_id = ?", userID).First(&c).Error
    if err != nil {
        if stderrors.Is(err, gorm.ErrRecordNotFound) {
            return nil, ErrChestNotFound
        }
        return nil, err
    }
    return &c, nil
}
```

修改 `server/internal/repo/mysql/errors.go`：加两个新哨兵：

```go
var (
    // 已有：ErrUserNotFound / ErrUsersGuestUIDDuplicate / ErrAuthBindingNotFound /
    //       ErrAuthBindingDuplicate / ErrPetNotFound

    // 新增 by Story 4.8：
    ErrStepAccountNotFound = stderrors.New("mysql: step_account not found")
    ErrChestNotFound       = stderrors.New("mysql: chest not found")
)
```

**关键设计约束**：

- **user / pet repo 不动**：4.6 已落地 `UserRepo.FindByID` / `PetRepo.FindDefaultByUserID`，本 story 仅消费
- **step_account_repo / chest_repo 加 FindByUserID 方法**：interface 扩展（**不**新建 V2 interface）+ impl 加方法 + sentinel error 加两个
- **每个 Find 方法都用 sentinel error 翻译 NotFound**：与 4.6 同模式（ADR-0006 三层映射；让 service 用 errors.Is 而非 gorm 内部 error）
- **chest.FindByUserID 走 uk_user_id 唯一索引**：性能 ≤ 1ms（user_id 是唯一索引，单行查询）
- **step_account.FindByUserID 走 PK = user_id**：性能 ≤ 1ms（PK 直查）
- **不引入 EquipRepo**：节点 2 阶段不查 user_pet_equips 表（service 直接返 `[]`）；Story 26.6 才引入

**AC4 — bootstrap router 挂 GET /home + auth + rate_limit**

修改 `server/internal/app/bootstrap/router.go`，在已有 `if deps.GormDB != nil && ...` 块内追加 home wire：

```go
if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil {
    // 5 repo（已有 4.6 的 wire；本 story 仅扩展）
    userRepo := mysql.NewUserRepo(deps.GormDB)
    authBindingRepo := mysql.NewAuthBindingRepo(deps.GormDB)
    petRepo := mysql.NewPetRepo(deps.GormDB)
    stepAccountRepo := mysql.NewStepAccountRepo(deps.GormDB)
    chestRepo := mysql.NewChestRepo(deps.GormDB)

    // auth service（4.6 已 wire）
    authSvc := service.NewAuthService(
        deps.TxMgr, deps.Signer,
        userRepo, authBindingRepo, petRepo, stepAccountRepo, chestRepo,
    )
    authHandler := handler.NewAuthHandler(authSvc)

    // ★ 4.8 新增：home service + handler（不依赖 authBindingRepo / txMgr / signer）
    homeSvc := service.NewHomeService(userRepo, petRepo, stepAccountRepo, chestRepo)
    homeHandler := handler.NewHomeHandler(homeSvc)

    api := r.Group("/api/v1")

    // /auth 子组：RateLimitByIP（4.6 已 wire）
    authGroup := api.Group("/auth", middleware.RateLimit(deps.RateLimitCfg, middleware.RateLimitByIP))
    authGroup.POST("/guest-login", authHandler.GuestLogin)

    // ★ 4.8 新增：已认证子组（先 auth 校验 token + 注 userID，再 RateLimitByUserID 限频）
    authedGroup := api.Group("",
        middleware.Auth(deps.Signer),
        middleware.RateLimit(deps.RateLimitCfg, middleware.RateLimitByUserID),
    )
    authedGroup.GET("/home", homeHandler.LoadHome)
}
```

**关键设计约束**：

- **挂在已认证子组**：先 `middleware.Auth(deps.Signer)` 校验 Bearer token + 注入 userID 到 c.Keys；再 `middleware.RateLimit(cfg, RateLimitByUserID)` 按 userID 维度限频；handler 假设 userID 必然存在
- **路由路径精确为 `/api/v1/home`**：注意 authedGroup 的 prefix 是 `""`（空串），所以路径是 `api.prefix + ""+ "/home" = "/api/v1/home"`，与 V1 §5.1 钦定一致
- **不挂 /me / 其他已认证路由**：本 story 范围只 /home；future story（GET /me）来时往 authedGroup 加 route 即可
- **deps 完整性 if-guard 复用 4.6 已建立的 pattern**：`deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil` 才 wire（单测 Deps{} 零值场景不挂业务路由）；deps 不变（已有 4 字段，4.8 不新增）

**关键反模式**：

- [bad] **不**给 /home 单独挂一个 group：复用 authedGroup 让 future GET /me 等同模块路由共享 auth + rate_limit 配置
- [bad] **不**用 RateLimitByIP（IP 维度会让多 user 共享 NAT IP 时互相影响 → 用户体验差；userID 维度精确限制单用户）
- [bad] **不**改 4.6 的 /auth 子组 wire（保持 4.6 已 done 的 wire 不变；本 story 仅追加）
- [bad] **不**引入新的 Deps 字段（不需要 RedisClient / etc；4 字段够用）

**AC5 — handler 单测覆盖 ≥ 4 case**

新建 `server/internal/app/http/handler/home_handler_test.go`，含以下 case：

1. **Happy path（节点 2 阶段首登）** — `TestHomeHandler_HappyPath_FirstLogin_ReturnsCompleteSchema`
   - 给 stub HomeService 返回 `HomeOutput{User, Pet (默认猫), StepAccount全0, Chest counting+10min}`
   - 用 mocked Auth 中间件（c.Set(UserIDKey, uint64(1001))）
   - 期望 200 + envelope code=0 + 完整 V1 §5.1 schema:
     - user.id="1001" / nickname / avatarUrl=""
     - pet.id="2001" / petType=1 / name="默认小猫" / currentState=1 / equips=[]
     - stepAccount.totalSteps=0 / availableSteps=0 / consumedSteps=0
     - chest.id="5001" / status=1 / unlockAt=ISO8601 UTC / openCostSteps=1000 / remainingSeconds≈600
     - room.currentRoomId=null

2. **chest unlockAt 已过 → status=2 (unlockable)** — `TestHomeHandler_ChestUnlocked_StatusIs2_RemainingSecondsIs0`
   - stub HomeService 返回 `Chest{Status: 2, UnlockAt: 1 分钟前, RemainingSeconds: 0}`
   - 期望 chest.status=2 / chest.remainingSeconds=0

3. **pet 为 nil（无默认 pet）→ "pet": null** — `TestHomeHandler_NoDefaultPet_PetFieldIsNull`
   - stub HomeService 返回 `HomeOutput{Pet: nil, ...}`
   - 期望 envelope.data.pet == nil（解码后 Go 端 PetField 是 *struct 且 == nil）
   - **关键断言**：用 `bytes.Contains(body, []byte(`"pet":null`))` 验证 JSON 字面量含 `"pet":null` 而非 `"pet":{}`

4. **service 返 1009 → handler 透传** — `TestHomeHandler_ServiceError_Returns1009`
   - stub HomeService 返回 `apperror.New(ErrServiceBusy, ...)`
   - 期望 envelope.code=1009 / message="服务繁忙"

5. **userID 缺失 / 类型错（unreachable 但保险） → 1009** — `TestHomeHandler_NoUserIDInContext_Returns1009`
   - **不**挂 mock auth middleware，直接调 LoadHome
   - 期望 envelope.code=1009（unreachable path 但本 story 测一遍）

每个 case 完整结构：构造 stubHomeService → newHomeHandlerRouter → httptest.NewRequest("GET", "/api/v1/home") → ServeHTTP → 解 envelope → 断言。

```go
type stubHomeService struct {
    loadHomeFn func(ctx context.Context, userID uint64) (*service.HomeOutput, error)
}

func (s *stubHomeService) LoadHome(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
    return s.loadHomeFn(ctx, userID)
}

// newHomeHandlerRouter 构造 handler test router；含 ErrorMappingMiddleware（必挂，否则 c.Error 不写 envelope）
// + mock auth middleware（直接 c.Set UserIDKey，不走真实 Bearer token 校验，避免在 handler 单测引入 4.4 / 4.5 联动）
func newHomeHandlerRouter(svc service.HomeService, mockUserID *uint64) *gin.Engine {
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
    h := handler.NewHomeHandler(svc)
    r.GET("/api/v1/home", h.LoadHome)
    return r
}
```

**关键约束**：

- **每个断言要严格按 V1 §5.1 schema**：字段名 + 类型 + 可空性都验
- **case 3 必须验 `"pet":null` 字面量（不是 `"pet":{}`）**：V1 §5.1 行 335 钦定可空，最常见的 LLM bug 是返 `{}` 占位
- **case 1 chest.unlockAt 验 ISO8601 UTC 字面量**：`"unlockAt":"2026-04-23T10:20:00Z"` 格式（验前 17 位 + Z 结尾，避免毫秒漂移）
- **stub HomeService 不真起 mysql**：每 case 直接构造 HomeOutput 返回；与 4.6 stubAuthService 同模式

**AC6 — service 单测覆盖 ≥ 4 case**

新建 `server/internal/service/home_service_test.go`，含以下 case：

1. **Happy: 4 repo 全成功 → HomeOutput 字段语义正确** — `TestHomeService_LoadHome_AllReposOK_ReturnsCompleteOutput`
   - stub 4 repo 各返成功值；user.ID=1001 / pet.ID=2001 / pet.CurrentState=1 / step全0 / chest.Status=1 / unlockAt=now+10min
   - 期望 HomeOutput.User.ID=1001 / Pet != nil / Pet.CurrentState=1 / StepAccount全0 / Chest.Status=1 / RemainingSeconds≈600

2. **chest unlockAt 已过 → 动态判定 Status=2 / RemainingSeconds=0** — `TestHomeService_LoadHome_ChestUnlocked_DynamicStatusIs2`
   - stub chest repo 返 UnlockAt=now-1min / Status=1（DB 原值仍是 1）
   - 期望 HomeOutput.Chest.Status == 2 / RemainingSeconds == 0
   - **关键**：DB 原值 Status=1 但 service 必须返 2（验证动态判定逻辑）

3. **pet NotFound → HomeOutput.Pet == nil（不视为错误）** — `TestHomeService_LoadHome_PetNotFound_PetIsNilNotError`
   - stub petRepo 返 (nil, ErrPetNotFound)
   - 期望 err == nil / HomeOutput.Pet == nil

4. **user repo 失败 → 整体 1009** — `TestHomeService_LoadHome_UserRepoFails_Returns1009`
   - stub userRepo 返 error("DB 异常")
   - 期望 err.(*apperror).Code == ErrServiceBusy(1009)

5. **step_account NotFound → 整体 1009（不视为可空）** — `TestHomeService_LoadHome_StepAccountNotFound_Returns1009`
   - stub stepAccountRepo 返 (nil, ErrStepAccountNotFound)
   - 期望 err.(*apperror).Code == ErrServiceBusy

6. **chest NotFound → 整体 1009（不视为可空）** — `TestHomeService_LoadHome_ChestNotFound_Returns1009`
   - stub chestRepo 返 (nil, ErrChestNotFound)
   - 期望 err.(*apperror).Code == ErrServiceBusy

7. **pet repo 非 NotFound 错误 → 1009（不被错认为可空）** — `TestHomeService_LoadHome_PetRepoOtherError_Returns1009`
   - stub petRepo 返 (nil, errors.New("connection lost"))
   - 期望 err.(*apperror).Code == ErrServiceBusy（不是 nil pet）

```go
// stub 4 repo（与 4.6 同模式：每个 stub 只实装本 service 用到的方法）
type stubUserRepo struct {
    findByIDFn func(ctx context.Context, id uint64) (*mysql.User, error)
}
func (s *stubUserRepo) Create(ctx context.Context, u *mysql.User) error { return nil }
func (s *stubUserRepo) UpdateNickname(ctx context.Context, id uint64, n string) error { return nil }
func (s *stubUserRepo) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
    return s.findByIDFn(ctx, id)
}

// 类似定义 stubPetRepo / stubStepAccountRepo / stubChestRepo（每个有对应的 fn 字段）
```

**关键约束**：

- **case 2 测试动态判定**：用 `time.Now().Add(-time.Minute)` 作为 unlockAt，DB Status 仍传 1，验证 service 算出 Status=2 / RemainingSeconds=0
- **case 3 / 5 / 6 区分 NotFound 处理**：pet NotFound → Pet=nil (OK)；step / chest NotFound → 1009（必有的表 NotFound = 数据脏）
- **case 4 / 7 区分错误类型**：errors.Is(err, ErrPetNotFound) 走可空分支；非 ErrPetNotFound 错误走 1009 分支

**AC7 — repo 层单测扩展（每个新方法 ≥ 1 case）**

修改 `server/internal/repo/mysql/step_account_repo_test.go`：加：

1. **TestStepAccountRepo_FindByUserID_HappyPath**: sqlmock 返 1 行 → 验证返回字段
2. **TestStepAccountRepo_FindByUserID_NotFound**: sqlmock 返 0 行 → 验证返 ErrStepAccountNotFound

修改 `server/internal/repo/mysql/chest_repo_test.go`：加：

1. **TestChestRepo_FindByUserID_HappyPath**: sqlmock 返 1 行 + UTC unlockAt → 验证返回字段
2. **TestChestRepo_FindByUserID_NotFound**: sqlmock 返 0 行 → 验证返 ErrChestNotFound

**关键约束**：

- **sqlmock 验证 SQL pattern**：`SELECT \* FROM .user_chests. WHERE user_id = ?` 类似（GORM 会引号包表名）；用 `regexp.QuoteMeta` 或 `mock.ExpectQuery` 的模糊匹配
- **happy case 验证 ID 字段填充**：sqlmock.NewRows + .AddRow → repo 解出 *UserChest 字段值
- **每个 NotFound case 用 sqlmock.ErrNoRows 触发**：`mock.ExpectQuery(...).WillReturnError(sql.ErrNoRows)` 或返 0 行 → GORM 自动转 gorm.ErrRecordNotFound → repo 翻译为 sentinel

**AC8 — dockertest 集成测试 ≥ 1 case**

新建 `server/internal/service/home_service_integration_test.go`（build tag `integration`）：

1. **TestHomeService_LoadHome_HappyPath** —
   - 起 mysql:8.0 容器（复用 4.6 已建的 startMySQL/runMigrations helper）
   - 手工 INSERT users(1) + pets(1, is_default=1, current_state=1) + user_step_accounts(全0) + user_chests(status=1, unlock_at=now+10min UTC, open_cost_steps=1000)
   - 调 svc.LoadHome(ctx, 1)
   - 断言 HomeOutput 全字段：user.ID=1 / pet != nil / pet.PetType=1 / pet.CurrentState=1 / step.全0 / chest.Status=1 / chest.RemainingSeconds∈[598, 602]（容忍秒级时钟漂移）

2. **TestHomeService_LoadHome_ChestUnlocked_StatusIs2** —
   - 同上但 INSERT user_chests 时 unlock_at = now()-1min UTC
   - 调 svc.LoadHome → 期望 chest.Status=2 / chest.RemainingSeconds=0

3. **TestHomeService_LoadHome_NoPet_PetIsNil** —
   - INSERT users + step_accounts + chests，**不**INSERT pets
   - 调 svc.LoadHome → 期望 err == nil / HomeOutput.Pet == nil

```go
//go:build integration

package service_test

// 复用 4.6 的 startMySQL / migrationsPath / runMigrations helper（同包内可直接调；
// 不抽包，避免范围扩散，与 4.2/4.3/4.6 同模式）

func TestHomeService_LoadHome_HappyPath(t *testing.T) {
    if testing.Short() {
        t.Skip("integration test")
    }
    dsn, dockerCleanup := startMySQL(t)
    defer dockerCleanup()
    runMigrations(t, dsn)

    sqlDB, err := sql.Open("mysql", dsn)
    if err != nil { t.Fatal(err) }
    defer sqlDB.Close()

    gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{})
    if err != nil { t.Fatal(err) }

    // 手工 INSERT 测试数据（不调 4.6 auth_service.GuestLogin —— 解耦）
    unlockAt := time.Now().UTC().Add(10 * time.Minute)
    _, err = sqlDB.Exec(`INSERT INTO users (id, guest_uid, nickname, avatar_url, status) VALUES (?, ?, ?, ?, ?)`,
        1, "test-uid", "用户1", "", 1)
    if err != nil { t.Fatal(err) }
    _, err = sqlDB.Exec(`INSERT INTO pets (id, user_id, pet_type, name, current_state, is_default) VALUES (?, ?, ?, ?, ?, ?)`,
        2001, 1, 1, "默认小猫", 1, 1)
    if err != nil { t.Fatal(err) }
    _, err = sqlDB.Exec(`INSERT INTO user_step_accounts (user_id, total_steps, available_steps, consumed_steps, version) VALUES (?, ?, ?, ?, ?)`,
        1, 0, 0, 0, 0)
    if err != nil { t.Fatal(err) }
    _, err = sqlDB.Exec(`INSERT INTO user_chests (id, user_id, status, unlock_at, open_cost_steps, version) VALUES (?, ?, ?, ?, ?, ?)`,
        5001, 1, 1, unlockAt, 1000, 0)
    if err != nil { t.Fatal(err) }

    // 构造 service
    userRepo := mysql.NewUserRepo(gormDB)
    petRepo := mysql.NewPetRepo(gormDB)
    stepRepo := mysql.NewStepAccountRepo(gormDB)
    chestRepo := mysql.NewChestRepo(gormDB)
    svc := service.NewHomeService(userRepo, petRepo, stepRepo, chestRepo)

    out, err := svc.LoadHome(context.Background(), 1)
    if err != nil { t.Fatalf("LoadHome err = %v", err) }

    // 断言 user
    if out.User.ID != 1 { t.Errorf("User.ID = %d, want 1", out.User.ID) }
    if out.User.Nickname != "用户1" { t.Errorf("User.Nickname = %q, want 用户1", out.User.Nickname) }

    // 断言 pet
    if out.Pet == nil { t.Fatal("Pet should not be nil") }
    if out.Pet.PetType != 1 { t.Errorf("Pet.PetType = %d, want 1", out.Pet.PetType) }
    if out.Pet.CurrentState != 1 { t.Errorf("Pet.CurrentState = %d, want 1", out.Pet.CurrentState) }

    // 断言 step
    if out.StepAccount.TotalSteps != 0 { t.Error("StepAccount.TotalSteps not 0") }

    // 断言 chest（动态字段）
    if out.Chest.Status != 1 { t.Errorf("Chest.Status = %d, want 1 (counting)", out.Chest.Status) }
    if out.Chest.RemainingSeconds < 598 || out.Chest.RemainingSeconds > 602 {
        t.Errorf("Chest.RemainingSeconds = %d, want ∈[598, 602]", out.Chest.RemainingSeconds)
    }
}
```

**关键约束**：

- **build tag `integration` + docker 不可用时 t.Skip**：与 4.2/4.3/4.6 同模式（CI 跑 / 本地 dev 不依赖 docker）
- **手工 INSERT 测试数据而非调 auth_service.GuestLogin**：解耦 home_service 测试与 auth_service；homemod test 只验 home 链路
- **chest.RemainingSeconds 用 ∈[598, 602] 容忍秒级时钟漂移**：dockertest 容器 + GORM round-trip 可能有 1-2s 延迟，硬等于 600 会 flaky
- **chest.Status=1 (counting)** vs case 2 用 unlockAt=now-1min 验 Status=2 (unlockable)

**AC9 — 全量验证**

完成所有 task 后跑：

1. `bash scripts/build.sh` → BUILD SUCCESS（go vet + go build）
2. `bash scripts/build.sh --test` → all tests pass（含 ≥4 handler + ≥4 service + ≥4 repo + 既有 4.6 / 4.5 / 4.4 / 4.3 / 4.2 / 4.1 / 4.0 测试全绿）
3. `bash scripts/build.sh --integration` → 集成测试通过（含 4.6 已有 4 case + 本 story 新增 ≥1 case；docker daemon 不可用 → t.Skip 优雅退出）
4. `go mod tidy` → go.mod / go.sum 无变化（仅消费已有依赖）
5. `git status --short` 抽检：仅影响：
   - `server/internal/service/home_service.go` + `home_service_test.go` + `home_service_integration_test.go`
   - `server/internal/app/http/handler/home_handler.go` + `home_handler_test.go`
   - `server/internal/repo/mysql/step_account_repo.go` + `step_account_repo_test.go`（FindByUserID 扩展）
   - `server/internal/repo/mysql/chest_repo.go` + `chest_repo_test.go`（FindByUserID 扩展）
   - `server/internal/repo/mysql/errors.go`（新加 ErrStepAccountNotFound / ErrChestNotFound）
   - `server/internal/app/bootstrap/router.go`（NewRouter 内追加 home wire + authedGroup）
   - `_bmad-output/implementation-artifacts/sprint-status.yaml`（4-8 状态推进）
   - `_bmad-output/implementation-artifacts/4-8-get-home-聚合接口.md`（本 story 文件，dev 完成后填 Tasks/Dev Agent Record/File List/Completion Notes）
   - `docs/*` 全部不变 / `docs/lessons/` 不变 / `configs/local.yaml` 不变 / `cmd/server/main.go` 不变 / `iphone/` / `ios/` 不变

**关键约束**：

- **不 commit**：epic-loop 流水线钦定 dev-story 阶段不 commit；commit 留给 story-done 阶段
- **不 push**：story-done 阶段也不 push（用户控制 push 时机）

## Tasks / Subtasks

- [x] **Task 1（AC3）**：repo 层扩展 — 加 FindByUserID 方法 + 哨兵 error
  - [x] 1.1 编辑 `server/internal/repo/mysql/errors.go`：加 `ErrStepAccountNotFound` / `ErrChestNotFound` 哨兵 error
  - [x] 1.2 编辑 `server/internal/repo/mysql/step_account_repo.go`：interface 加 `FindByUserID(ctx, userID) (*StepAccount, error)` 签名 + impl（PK=user_id 单行查询；NotFound → ErrStepAccountNotFound）
  - [x] 1.3 编辑 `server/internal/repo/mysql/chest_repo.go`：interface 加 `FindByUserID(ctx, userID) (*UserChest, error)` 签名 + impl（uk_user_id 唯一索引；NotFound → ErrChestNotFound）
  - [x] 1.4 godoc 注释（与 4.6 同模式：FindByUserID 走索引 / sentinel error 翻译 / .WithContext(ctx) 必须）
- [x] **Task 2（AC2）**：home_service.go 实装
  - [x] 2.1 新建 `server/internal/service/home_service.go`：定义 `HomeService` interface + `HomeOutput` / `UserBrief` / `PetBrief` / `StepAccountBrief` / `ChestBrief` DTO
  - [x] 2.2 实装 `homeServiceImpl.LoadHome`：4 个 repo 串行 + chest 动态判定 + 拼装 DTO
  - [x] 2.3 实装 `chestStatusDynamic` / `computeRemainingSeconds` 包级 helper（Story 20.5 复用 → 函数签名冻结）
  - [x] 2.4 godoc 注释（service 层职责 + chest 动态判定 + Pet 可空语义 + 不部分降级原则）
- [x] **Task 3（AC1）**：home_handler.go 实装
  - [x] 3.1 新建 `server/internal/app/http/handler/home_handler.go`：`HomeHandler` struct + `NewHomeHandler`
  - [x] 3.2 实装 `LoadHome`：从 c.Get(UserIDKey) 取 userID + 类型断言 + 调 svc.LoadHome + 返 envelope
  - [x] 3.3 实装 `homeResponseDTO`：转 BIGINT id 为 string + petDTO 可空（nil → JSON null）+ equips 用 `[]any{}` 而非 nil + chest.unlockAt 用 RFC3339 + room.currentRoomId 节点 2 强制 nil
  - [x] 3.4 godoc 注释（V1 §5.1 schema 严格对齐 + ADR-0006 单一 envelope 生产者 + ADR-0007 ctx 传播）
- [x] **Task 4（AC5）**：handler 单测 ≥4 case（实际计划 5 case）
  - [x] 4.1 新建 `server/internal/app/http/handler/home_handler_test.go`
  - [x] 4.2 实现 `stubHomeService` stub + `newHomeHandlerRouter` helper（含 mock Auth 中间件 set UserIDKey）
  - [x] 4.3 `TestHomeHandler_HappyPath_FirstLogin_ReturnsCompleteSchema`（断言完整 V1 §5.1 schema 节点 2 阶段值）
  - [x] 4.4 `TestHomeHandler_ChestUnlocked_StatusIs2_RemainingSecondsIs0`
  - [x] 4.5 `TestHomeHandler_NoDefaultPet_PetFieldIsNull`（**关键**：bytes.Contains 验证 `"pet":null` 字面量）
  - [x] 4.6 `TestHomeHandler_ServiceError_Returns1009`
  - [x] 4.7 `TestHomeHandler_NoUserIDInContext_Returns1009`（unreachable 兜底）
- [x] **Task 5（AC6）**：service 单测 ≥4 case（实际计划 7 case）
  - [x] 5.1 新建 `server/internal/service/home_service_test.go`
  - [x] 5.2 实现 4 个 stub repo（stubUserRepo / stubPetRepo / stubStepAccountRepo / stubChestRepo）
  - [x] 5.3 `TestHomeService_LoadHome_AllReposOK_ReturnsCompleteOutput`
  - [x] 5.4 `TestHomeService_LoadHome_ChestUnlocked_DynamicStatusIs2`（验证动态判定逻辑）
  - [x] 5.5 `TestHomeService_LoadHome_PetNotFound_PetIsNilNotError`
  - [x] 5.6 `TestHomeService_LoadHome_UserRepoFails_Returns1009`
  - [x] 5.7 `TestHomeService_LoadHome_StepAccountNotFound_Returns1009`（不视为可空）
  - [x] 5.8 `TestHomeService_LoadHome_ChestNotFound_Returns1009`（不视为可空）
  - [x] 5.9 `TestHomeService_LoadHome_PetRepoOtherError_Returns1009`（区分 NotFound vs 其他错误）
- [x] **Task 6（AC7）**：repo 单测扩展（每方法 ≥1 case）
  - [x] 6.1 编辑 `server/internal/repo/mysql/step_account_repo_test.go`：加 FindByUserID happy + NotFound 共 2 case
  - [x] 6.2 编辑 `server/internal/repo/mysql/chest_repo_test.go`：加 FindByUserID happy + NotFound 共 2 case
- [x] **Task 7（AC4）**：bootstrap router wire
  - [x] 7.1 编辑 `server/internal/app/bootstrap/router.go`：在已有 `if deps.GormDB != nil && ...` 块内追加 `homeSvc := service.NewHomeService(...)` + `homeHandler := handler.NewHomeHandler(...)` + `authedGroup := api.Group("", middleware.Auth(deps.Signer), middleware.RateLimit(...))` + `authedGroup.GET("/home", homeHandler.LoadHome)`
  - [x] 7.2 验证已有 router_test.go / server_test.go / router_dev_test.go 等仍绿（Deps{} 零值路径不挂业务路由 → 不受影响）
- [x] **Task 8（AC8）**：dockertest 集成测试
  - [x] 8.1 新建 `server/internal/service/home_service_integration_test.go`（build tag `integration`）
  - [x] 8.2 复用 4.6 的 startMySQL / migrationsPath / runMigrations helper（同包内可直接调）
  - [x] 8.3 `TestHomeService_LoadHome_HappyPath`（手工 INSERT 4 表数据 → 调 svc.LoadHome → 断言全字段）
  - [x] 8.4 `TestHomeService_LoadHome_ChestUnlocked_StatusIs2`（unlock_at=now-1min → 动态判定 Status=2）
  - [x] 8.5 `TestHomeService_LoadHome_NoPet_PetIsNil`（不 INSERT pets → Pet=nil 不视为错误）
- [x] **Task 9（AC9）**：全量验证
  - [x] 9.1 `bash /c/fork/cat/scripts/build.sh` → BUILD SUCCESS
  - [x] 9.2 `bash /c/fork/cat/scripts/build.sh --test` → all tests pass（≥15 新增 case + 既有全部 4.x test 仍绿）
  - [x] 9.3 `bash /c/fork/cat/scripts/build.sh --integration` → 含本 story 新增 ≥3 集成 case 全绿（docker 不可用 t.Skip）
  - [x] 9.4 `go mod tidy` → go.mod / go.sum 无变化（仅消费已有依赖）
  - [x] 9.5 抽检 `git status --short`：仅影响 AC9 列出的文件清单；docs/* 全部未改 / docs/lessons/ 未改 / configs/local.yaml 未改
- [x] **Task 10**：本 story 不做 git commit
  - [x] 10.1 epic-loop 流水线约束遵守：dev-story 阶段不 commit
  - [x] 10.2 commit message 模板留给 story-done 阶段：

    ```text
    feat(home): GET /home 聚合接口（Story 4.8）

    - internal/service/home_service.go: HomeService.LoadHome 实装（4 repo 串行 + chest 动态判定 + Pet 可空）
    - internal/app/http/handler/home_handler.go: LoadHome handler 严格对齐 V1 §5.1 schema（pet 可空 / equips=[] / room.currentRoomId=null）
    - internal/repo/mysql/step_account_repo.go + chest_repo.go: 加 FindByUserID 方法 + 2 个新哨兵 error
    - internal/app/bootstrap/router.go: wire authedGroup（Auth + RateLimitByUserID）+ GET /home route
    - 单测 ≥15 case（handler 5 + service 7 + repo 4）
    - dockertest 集成测试 ≥3 case（happy / chest 解锁 / 无 pet）

    依据 epics.md §Story 4.8 + V1 §5.1 + 数据库设计 §5/§6 + 设计文档 §6.2。

    Story: 4-8-get-home-聚合接口
    ```

## Dev Notes

### 关键设计原则

1. **§AR3 状态以 server 为准**：本接口聚合下发主界面所需全部数据，client 收到后渲染主界面；client **不**做 chest 倒计时本地纠正（在 chest 状态切换的瞬间会拉一次）—— 服务端 `time.Now().UTC()` 是唯一时钟源。
2. **不部分降级**：epics.md §Story 4.8 行 1136 钦定"各部分 repo 错误 → 整体 1009 服务繁忙"。原因：主界面是用户进入 App 的第一屏，部分降级（如 user 拿到了但 chest 没拿到 → 渲染半截界面）会让客户端处理逻辑爆炸（5 路 partial state 组合）；整体 1009 让 client SilentRelogin 后重试，简单可靠。
3. **chest 状态动态判定**：DB user_chests.status 字段节点 2 阶段恒为 1（counting），但客户端期望"unlock_at 已过 → status=2 unlockable"；service 必须基于 time.Now() 动态算下发 status；**不**写回 DB（节点 7 / Epic 20 chest_service 才负责 chest_open_logs + 重建下一轮 chest）。
4. **pet 可空语义**：V1 §5.1 行 335 钦定 `data.pet | object | null`；用户无默认 pet 时返 `"pet": null`（**不**返 `{}`）；server 实装严格对齐。理由：iOS Codable 按 `Optional<PetDTO>` 解析，Go 用 `*PetDTO`，wire 必须明确 null 否则 decode 失败。
5. **room.currentRoomId 节点 2 强制 null**：节点 2 阶段没有房间功能（Epic 11 才落地），即便 users.current_room_id 字段已存在也不读；强制 null 给客户端明确信号"当前不在任何房间"；Story 11.10（节点 4）才接真值。
6. **service / repo 分层严格**：service 持业务规则（chest 动态判定 / pet 可空语义 / 串行查询编排）；repo 只做单表 CRUD + sentinel error（NotFound 翻译）；handler 只做 ctx + userID 提取 + DTO 转换 + envelope 返回（设计文档 §5 + §6.2 钦定）。
7. **不依赖 ctx 取消业务流程**：repo 调 `.WithContext(ctx)` 让 GORM 感知 client 断开（client 拒收时 GORM 自动 abort 查询）；service 不主动 select ctx.Done()（4 个 repo 调用串行 < 50ms 不需要主动检查）；handler 拿 `c.Request.Context()` 透传（ADR-0007 §2.2）。
8. **handler 假设 userID 必然存在**：依赖 router 挂的 Auth 中间件链；在 if-else fallback 失败也走 1009（unreachable 但保险），与 4.5 RateLimitByUserID 同模式（`v, ok := c.Get(UserIDKey)` + 断言失败 → fallback）。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md` §3）：
- 节点 2 阶段 home 是用户进入 App 后看到的第一屏（启动 → 自动游客登录 → SilentRelogin 拿 token → GET /home → 渲染主界面）；service 层是聚合查询的归属（设计文档 §6.2 钦定）
- 5 段数据（user / pet / step / chest / room）覆盖主界面 4 个核心区块（user 卡 / pet 显示区 / 宝箱卡 / 步数卡）+ room 占位（节点 4 才用）

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：
- §2.3 钦定 `Authorization: Bearer` 头格式（本接口**走** auth 中间件 → 必须带 Bearer token）
- §2.4 钦定 envelope `{code, message, data, requestId}`（response.Success / ErrorMappingMiddleware 一致）
- §2.5 钦定 BIGINT id 转 string（避免 JS Number.MAX_SAFE_INTEGER）+ datetime ISO 8601 UTC + 长度按字符数
- §3 钦定错误码 1001 / 1009（本接口可能触发的两个）
- §5.1 钦定 schema（已在 Story 4.1 锚定 + 冻结，本 story 严格对齐；行 308-450）
- §5.1 Future Fields 钦定 `pet.equips[]` 节点 9 / `pet.equips[].renderConfig` 节点 10 / `room.currentRoomId` 节点 4 各自 future story 接入

**数据库设计层**（`docs/宠物互动App_数据库设计.md`）：
- §3.1 钦定主键 BIGINT UNSIGNED → Go uint64（与 4.6 User.ID / Pet.ID / UserChest.ID 一致）
- §3.2 钦定时间字段 DATETIME(3) 毫秒精度（chest.unlockAt 字段下发时按 RFC3339 不带毫秒）
- §5.1 users 表：本 story 仅 FindByID（4.6 已落地）
- §5.3 pets 表：本 story 仅 FindDefaultByUserID（4.6 已落地）；is_default=1 是默认猫筛选条件
- §5.4 user_step_accounts 表：本 story 加 FindByUserID（PK=user_id 单行查询）
- §5.6 user_chests 表：本 story 加 FindByUserID（uk_user_id 唯一索引）
- §6.4 pets.current_state 枚举：1=rest, 2=walk, 3=run（节点 2 阶段读 DB 原值，初始为 1）
- §6.7 user_chests.status 枚举：1=counting, 2=unlockable（节点 2 阶段 service 动态判定）

**Go 项目结构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：
- §4 钦定目录树：`internal/app/http/handler/home_handler.go` / `internal/service/home_service.go` / `internal/repo/mysql/{user,pet,step_account,chest}_repo.go`
- §5.1 handler 层：参数解析 + 调 service + envelope 返回（**不**写业务）
- §5.2 service 层：核心业务 + 跨 repo 编排（如 4 个 repo 串行查询）
- §5.3 repo 层：CRUD + 错误识别（NotFound → sentinel）
- **§6.2 User / Home 模块钦定**：`/home` 单独 home_service.go，**不**散在 user_service.go / pet_service.go 等

**ADR 对接**：
- ADR-0001 测试栈：单测 + 集成测试（dockertest）双层；Windows race skip；与 4.2 / 4.3 / 4.6 同模式
- ADR-0003 ORM 选型：GORM v1.25.x；本 story 严格用 GORM API
- ADR-0006 错误三层映射：repo 哨兵 → service apperror.Wrap → handler c.Error → middleware envelope；本 story 是 home 模块第一次跑通该框架
- ADR-0007 ctx 传播：service / repo 第一参数 ctx；repo 必 `.WithContext(ctx)`；handler 用 `c.Request.Context()`

### 与已 done 的 4.5 / 4.6 的衔接

**4.5 实装**（本 story 复用）：
- `internal/app/http/middleware/auth.go`：`Auth(signer)` 工厂 + `UserIDKey` 常量；本 story 在 router authedGroup 挂 `middleware.Auth(deps.Signer)` + handler 用 `c.Get(middleware.UserIDKey)` 取 userID
- `internal/app/http/middleware/rate_limit.go`：`RateLimit(cfg, RateLimitByUserID)` 工厂；本 story 在 authedGroup 挂用 userID 维度限频

**4.6 实装**（本 story 复用）：
- `internal/repo/mysql/user_repo.go`：`UserRepo.FindByID`（已有，本 story 直接消费）
- `internal/repo/mysql/pet_repo.go`：`PetRepo.FindDefaultByUserID`（已有，已含 ErrPetNotFound 哨兵）
- `internal/repo/mysql/step_account_repo.go`：`StepAccountRepo.Create`（已有，本 story 加 `FindByUserID` 方法）
- `internal/repo/mysql/chest_repo.go`：`ChestRepo.Create`（已有，本 story 加 `FindByUserID` 方法）
- `internal/repo/mysql/errors.go`：`ErrUserNotFound` / `ErrPetNotFound` 等哨兵（已有，本 story 加 `ErrStepAccountNotFound` / `ErrChestNotFound`）
- `internal/repo/tx/manager.go`：本 story 不用（GET /home 全是只读查询，无事务需求）；但 repo 层 `tx.FromContext(ctx, r.db)` 仍兼容（即使 ctx 没 tx，fallback 到 r.db 单连接）

**本 story 新增解耦的 path**：
- `internal/service/home_service.go`：节点 2 第二个真实 service；后续 Epic 7 step_service / Epic 11 room_service / Epic 20 chest_service / 等等同包平级
- `internal/app/http/handler/home_handler.go`：第二个真实业务 handler（4.6 auth_handler 是第一个）
- `chestStatusDynamic` / `computeRemainingSeconds` 包级 helper：Story 20.5 chest_service.GetCurrent 复用；本 story 落地后函数签名 + 行为契约**冻结**

### 与下游 4.7 / 5.5 / 11.10 / 20.5 / 26.6 的接口

**4.7 落地时会做**：
- 用 dockertest 起真 MySQL 跑 Layer 2 Epic 4 整体集成测试（4.6 + 4.8 联动）
- 覆盖场景：登录初始化（4.6 firstTimeLogin）→ 立即拉首页（4.8 LoadHome）→ 验证首页字段对齐 4.6 写入的 5 行数据
- **本 story 必须保证**：home_service.LoadHome 与 auth_service.GuestLogin 写入的数据**字段语义对齐**（如 chest.UnlockAt 是 UTC 字面量；step_account 全 0；pet.is_default=1 / current_state=1）；4.7 才能跑通 happy 链路

**5.5 LoadHomeUseCase（iOS）落地时会做**：
- 调 `GET /api/v1/home` → 解析 V1 §5.1 schema → 把 user / pet / stepAccount / chest 注入主界面 ViewModel
- **本 story 必须保证**：response 严格匹配 V1 §5.1 schema（id 是 string / pet 可空必带 null 字面量 / equips 必带 [] 字面量 / unlockAt 是 RFC3339 UTC / room.currentRoomId 是 null）；任一字段名 / 类型 / 可空性偏离 → iOS Codable decode 失败

**11.10（节点 4 房间 epic）落地时会做**：
- 修改 home_service.LoadHome：从读硬编码 `nil` 改为读 `users.current_room_id`（已有列，节点 2 阶段不读）
- 在 homeResponseDTO 把 `room.currentRoomId` 从硬编码 nil 改为转 BIGINT id 为 string（uint64 → strconv.FormatUint）
- **本 story 必须保证**：room 字段段落在 service / handler 都是**预留扩展点**（不要把 room 字段段落耦合到其他四段，让 11.10 改动最小化）

**20.5（节点 7 chest_service.GetCurrent）落地时会做**：
- 实装 `chest_service.GetCurrent(ctx, userID)` → 复用本 story 的 `chestStatusDynamic` / `computeRemainingSeconds` helper
- **本 story 必须保证**：两个 helper 函数签名 + 行为契约**冻结**；future 节点 7 chest_service 有新 status 值（如 3 = opening 中间态）时只能在 chestStatusDynamic 内加 switch case，**不能**改函数签名；GET /home 永远不返 opened 状态（V1 §5.1 钦定）

**26.6（节点 9 穿戴 epic）落地时会做**：
- 实装查询 user_pet_equips JOIN cosmetic_items → 把 pet.equips 从 `[]` 改为真实数据
- **本 story 必须保证**：handler `homeResponseDTO` 内 equips 字段段落是**预留扩展点**（用 `[]any{}` 而非内联到 PetDTO 各 field 旁），future 26.6 改动只需要把 `[]any{}` 替换为 `[]equipDTO{...}`；DTO 嵌套结构对齐 V1 §5.1 节点 9 阶段示例（`{slot, userCosmeticItemId, cosmeticItemId, name, rarity, assetUrl}`）

### 关键决策点（实装时注意）

1. **petDTO 用 `any` 类型让 nil 序列化为 JSON null**：

   ```go
   var petDTO any // 关键：声明 any 而非 gin.H
   if out.Pet != nil {
       petDTO = gin.H{"id": ..., ...}
   }
   // out.Pet == nil 时 petDTO 仍是 any(nil)；JSON 序列化为 null
   return gin.H{"pet": petDTO, ...}
   ```

   **错误写法**：`var petDTO gin.H = nil`（虽然语义上是 nil，但 gin.H 是 map 类型，`map[string]interface{}(nil)` 序列化是 `null` 行为正确，但写 `gin.H{}` 时序列化为 `{}` 错误，需要严格区分）；用 `any` 类型最不容易出错。

2. **equips 用 `[]any{}` 而非 nil**：

   ```go
   "equips": []any{},  // 序列化为 [] ✓
   "equips": nil,       // 序列化为 null ✗（违反 V1 §5.1 行 340 钦定节点 2 阶段强制 []）
   ```

3. **chest.UnlockAt 强制 .UTC()**：

   ```go
   UnlockAt: chest.UnlockAt.UTC(),  // ✓ 强制 UTC 时区视图
   UnlockAt: chest.UnlockAt,         // ✗ GORM 解析时可能带 local 时区（取决于 driver loc 参数）
   ```

   GORM 默认从 MySQL DATETIME 字段解出 Go time.Time 时**不带时区**（zero offset）；但 driver 配置 `loc=Local` 时会带本地时区 → 序列化为 RFC3339 时下发 `2026-04-23T18:20:00+08:00` → 客户端按 UTC 解析多 8 小时偏差。`.UTC()` 强制统一为 UTC 视图。

4. **chest.unlockAt 序列化用 `time.RFC3339`**（=`2006-01-02T15:04:05Z07:00`）：

   ```go
   "unlockAt": out.Chest.UnlockAt.Format(time.RFC3339),  // ✓ "2026-04-23T10:20:00Z"
   "unlockAt": out.Chest.UnlockAt,                        // ✗ Go 默认 String() 是 "2026-04-23 10:20:00.123 +0000 UTC"
   "unlockAt": out.Chest.UnlockAt.Format(time.RFC3339Nano), // ✗ 带 .000Z 毫秒后缀，徒增字节
   ```

5. **room.currentRoomId 节点 2 强制 nil**：

   ```go
   "room": gin.H{"currentRoomId": nil},  // ✓ 序列化为 {"currentRoomId": null}
   "room": gin.H{"currentRoomId": ""},   // ✗ 空串不是 null（违反 V1 §5.1 钦定 string|null）
   "room": gin.H{},                       // ✗ 缺字段（iOS DTO 解码可能 fail）
   ```

6. **userID 从 c.Get(UserIDKey) 取 + 类型断言双保险**：

   ```go
   v, ok := c.Get(middleware.UserIDKey)
   if !ok { /* 返 1009 */ }
   userID, ok := v.(uint64)
   if !ok { /* 返 1009 */ }
   ```

   两层断言：`c.Get` 返 `interface{}, bool`；`v.(uint64)` 是类型断言。两者都失败的场景理论 unreachable（Auth 中间件挂前必然 set uint64），但保险起见兜底。

7. **chestStatusDynamic 函数签名冻结**：

   ```go
   // 节点 2 阶段：dbStatus 永远 1；忽略；只看 unlockAt 与 now 比较
   // 节点 7 阶段：会扩 switch dbStatus { case 1: ...; case 3: ...; }
   func chestStatusDynamic(dbStatus int8, unlockAt, now time.Time) int { ... }
   ```

   即使节点 2 阶段 dbStatus 是 unused 参数，**也保留**该签名 —— Story 20.5 落地时直接复用，不需改签名。

8. **computeRemainingSeconds 边界条件**：`diff <= 0` 返 0（**不**返负数）；V1 §5.1 行 348 钦定 "> 0 表示 counting，≤ 0 表示已可开启"，0 是边界值。

9. **pet 处理三态**：

   ```go
   // (a) repo 返 sentinel ErrPetNotFound → service 视为 Pet=nil（不视为错误）
   // (b) repo 返其他 error → service 视为 1009 错误
   // (c) repo 返 *Pet → service 拼装 PetBrief 返回
   pet, err := s.petRepo.FindDefaultByUserID(ctx, userID)
   if err != nil {
       if errors.Is(err, mysql.ErrPetNotFound) {
           petBrief = nil  // (a)
       } else {
           return nil, apperror.Wrap(err, ...)  // (b)
       }
   } else {
       petBrief = &PetBrief{...}  // (c)
   }
   ```

   **关键**：(a) 与 (b) 区分必须用 errors.Is，不能用 `if err == ErrPetNotFound`（如果 repo 用 fmt.Errorf 包装则直接比较失败）。

10. **stub repo 模式（与 4.6 同模式）**：每个 stub 只实装 home_service 用到的方法（不需要全部 method）；Stub 必须满足完整 interface 以编译通过：

    ```go
    type stubStepAccountRepo struct {
        findByUserIDFn func(ctx context.Context, userID uint64) (*mysql.StepAccount, error)
    }
    // 必须实装 Create（即使本 service 不调用）以满足 StepAccountRepo interface
    func (s *stubStepAccountRepo) Create(ctx context.Context, a *mysql.StepAccount) error { return nil }
    func (s *stubStepAccountRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
        return s.findByUserIDFn(ctx, userID)
    }
    ```

11. **handler 单测 mock Auth 中间件 set UserIDKey**：

    ```go
    func newHomeHandlerRouter(svc service.HomeService, mockUserID *uint64) *gin.Engine {
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
        h := handler.NewHomeHandler(svc)
        r.GET("/api/v1/home", h.LoadHome)
        return r
    }
    ```

    **关键**：handler 单测**不**挂真实 Auth 中间件（避免引入 4.4 signer / 4.5 Auth 联动）；只在 TestHomeHandler_NoUserIDInContext_Returns1009 case 不传 mockUserID（验证 unreachable 兜底）。

12. **集成测试手工 INSERT vs 调 4.6 auth_service**：

    手工 INSERT 4 表数据（不调 auth_service.GuestLogin）—— 解耦 home_service 测试与 auth_service；理由：

    - home_service 集成测试只验 home 链路（4 repo 串行 + chest 动态判定）
    - 调 auth_service 引入 4.6 实装变更敏感性（auth_service 改了字段映射 → home 集成测试也跟着炸）
    - 手工 INSERT 用 sqlDB.Exec 直接 SQL；与 4.6 集成测试同模式

13. **chest.RemainingSeconds 集成测试容忍 ∈[598, 602]**：

    dockertest 容器 + GORM round-trip 可能有 1-2s 延迟；硬等于 600 会 flaky；用区间断言 5s 容忍窗口 = `[unlockAt - 600s, unlockAt - 598s]` 范围内通过。

14. **GORM AUTO_INCREMENT 行为差异（与 4.6 一致但需注意）**：用 `sqlDB.Exec` 手工 INSERT 显式指定 id 列时，AUTO_INCREMENT 不会跳过显式值；后续 GORM Create（如果有）会按 max(id)+1 续号。本 story 集成测试只手工 INSERT 不 GORM Create，无此问题。

15. **Windows 平台 race 测试**：与 4.2-4.6 一致 —— Windows ThreadSanitizer skip；Linux / CI race 路径不受影响。

16. **GORM v1.25.x 的 First / Take / Find 区别**：本 story 用 First（按 PK / 唯一索引取一行）；NotFound 自动转 gorm.ErrRecordNotFound（与 4.6 同模式）；不要用 Find（返 0 行不报错，违反预期）。

### Project Structure Notes

预期文件 / 目录变化：

**新增**：
- `server/internal/service/home_service.go` + `home_service_test.go`
- `server/internal/service/home_service_integration_test.go`（build tag `integration`）
- `server/internal/app/http/handler/home_handler.go` + `home_handler_test.go`

**修改**：
- `server/internal/repo/mysql/errors.go`（加 `ErrStepAccountNotFound` / `ErrChestNotFound`）
- `server/internal/repo/mysql/step_account_repo.go`（interface 加 `FindByUserID` + impl）
- `server/internal/repo/mysql/step_account_repo_test.go`（加 2 case）
- `server/internal/repo/mysql/chest_repo.go`（interface 加 `FindByUserID` + impl）
- `server/internal/repo/mysql/chest_repo_test.go`（加 2 case）
- `server/internal/app/bootstrap/router.go`（NewRouter 内追加 home wire + authedGroup）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（4-8: backlog → ready-for-dev → in-progress → review；由 dev-story 流程内推动）
- `_bmad-output/implementation-artifacts/4-8-get-home-聚合接口.md`（本 story 文件，dev 完成后填 Tasks/Dev Agent Record/File List/Completion Notes）

**不影响其他目录**：
- `server/internal/repo/mysql/user_repo.go` 不变（4.6 已落地 FindByID，本 story 仅消费）
- `server/internal/repo/mysql/pet_repo.go` 不变（4.6 已落地 FindDefaultByUserID，本 story 仅消费）
- `server/internal/repo/mysql/auth_binding_repo.go` 不变（home 不查 binding 表）
- `server/internal/service/auth_service.go` 不变（home_service 不调 auth_service）
- `server/internal/app/http/handler/auth_handler.go` 不变（home_handler 不依赖 auth_handler）
- `server/internal/pkg/auth/` 不变（4.4 已落地；本 story 通过 Auth 中间件间接消费 Signer）
- `server/internal/pkg/errors/` 不变（apperror 框架 1.8 已落地；本 story 仅消费 codes + Wrap + DefaultMessages）
- `server/internal/pkg/response/` 不变（envelope helper 已有；本 story 仅消费 Success）
- `server/internal/infra/db/` 不变（4.2 已落地）
- `server/internal/infra/migrate/` 不变（4.3 已落地）
- `server/internal/cli/` 不变（4.3 已落地）
- `server/internal/repo/tx/` 不变（4.2 已落地；本 story repo 调 tx.FromContext(ctx, r.db) 兜底，但实际 ctx 不带 tx）
- `server/internal/app/http/middleware/` 不变（4.5 已落地；本 story 仅 wire Auth + RateLimitByUserID）
- `server/internal/infra/config/` 不变（4.5 已落地 RateLimitConfig）
- `server/migrations/` 不变（4.3 已落地，本 story 不加 migration）
- `server/configs/local.yaml` 不变（不引入新配置项）
- `server/cmd/server/main.go` 不变（4.5 已构造 Deps；本 story 不加新 deps 字段）
- `iphone/` / `ios/` 不变（server-only story）
- `docs/宠物互动App_*.md` 全部 7 份不变（消费方）
- `docs/lessons/` 不变（review 阶段写新 lesson 由 fix-review 处理）
- `README.md` / `server/README.md` 不变（Epic 4 收尾或 Epic 36 部署 story 才统一更新）

### chest 动态判定 vs DB 写回的边界讨论

**为什么本 story 不写回 DB user_chests.status**？

- 节点 2 阶段不需要：用户每次 GET /home 都是直查 DB + service 算 status；写回反而引入并发 race（多个 GET 并发 → 多次 UPDATE → 锁开销）
- 节点 7 / Epic 20 chest_service.GetCurrent 才负责"unlockable 状态稳定下来后写回 DB"（如果有这个需求；目前 Epic 20 epics.md 也没要求 GET /chest/current 写回，倾向于一直动态判定）
- 与 V1 §5.1 行 345 钦定一致："服务端**动态判定**为 `2`（unlockable）"

**chest 写回的真实场景**：

只有用户**开箱**（POST /chest/open，节点 7 / Story 20.6）时 chest_service 会重建下一轮 chest（INSERT new row + UPDATE old row status）；节点 2 阶段完全不写 chest 表（除了 4.6 登录初始化时 INSERT 一次）。

### 查询拆分 vs 串行

**为什么用串行 4 个 repo 调用而非并发 errgroup**？

- 单 user 单查询：4 个简单 SELECT < 50ms（GORM 连接池复用）
- errgroup 引入复杂度：cancel 传播 + error 收敛 + goroutine 调度开销
- 节点 2 阶段单实例部署：QPS 估算 < 100 / s（每用户每分钟最多 1-2 次主动刷新主界面）；串行查询不会成为瓶颈
- 节点 36 性能 epic 才考虑并发优化（如果 50ms 串行 timing 真成为瓶颈的话）

**MVP 简单优于过早优化**：4 个串行调用 + chest 动态判定的 LoadHome 函数总长 < 80 行；引入 errgroup 直接翻倍。

### chest.UnlockAt UTC vs Local 漂移防御

**两层 UTC 强制**（防 GORM 解析漂移）：

1. **DB 写入侧**（4.6 已落地）：service.firstTimeLogin 用 `time.Now().UTC().Add(10*time.Minute)`
2. **DB 读取侧**（本 story）：service.LoadHome 拼装 ChestBrief 时 `chest.UnlockAt.UTC()`

为什么读取侧也要强制 UTC？GORM driver 配置 `loc=Local`（local timezone）时，从 MySQL DATETIME(3) 解出的 Go time.Time 会带 local timezone（即使 DB 存的是 UTC 字面量）；本 story handler `Format(time.RFC3339)` 会按 timezone 输出（"2026-04-23T18:20:00+08:00"）→ 客户端按 UTC 解析多 8 小时偏差。

**当前 4.2 db.Open 配置**：`loc=Local` 仍是默认（参见 `internal/infra/db/mysql.go`）；本 story 在 service 层强制 .UTC() 兜底；future 节点 36 部署 epic 可统一切到 driver `loc=UTC`，届时本 story `.UTC()` 可移除（但保留也无害）。

### 集成测试性能

每个 dockertest case 起一次 mysql:8.0 容器（~10-30s 冷启）；3 case 共 ~30-90s。优化方向（**本 story 不做**，留给 future epic）：

- 多 case 共享一个容器（用 `t.Run` subtest + setup/teardown 在 outer test）
- 用 `mysql:8.0-alpine` 减少镜像体积
- CI 用 docker layer cache

本 story 集成测试遵循 4.2 / 4.3 / 4.6 同模式（每 case 独立容器），优先简单 + 一致性。

### V1 §5.1 schema 字段对照表（本 story 严格对齐用）

| V1 §5.1 字段 | service HomeOutput 字段 | handler DTO 字段 | 节点 2 阶段值 | 备注 |
|---|---|---|---|---|
| data.user.id | UserBrief.ID (uint64) | "id" (string) | 真实 | strconv.FormatUint |
| data.user.nickname | UserBrief.Nickname | "nickname" | 真实 | "用户{id}" 格式 |
| data.user.avatarUrl | UserBrief.AvatarURL | "avatarUrl" | "" | 节点 2 阶段空串 |
| data.pet | *PetBrief（可空） | gin.H 或 nil | 真实 / null | nil → JSON null |
| data.pet.id | PetBrief.ID | "id" | 真实 | strconv.FormatUint |
| data.pet.petType | PetBrief.PetType (int) | "petType" (number) | 1 | 节点 2 固定 |
| data.pet.name | PetBrief.Name | "name" | "默认小猫" | V1 §4.1 钦定 |
| data.pet.currentState | PetBrief.CurrentState (int) | "currentState" (number) | 1 | DB pets.current_state 原值 |
| data.pet.equips | (handler 直接构造) | "equips" ([]any{}) | [] | 节点 2 强制 [] |
| data.stepAccount.totalSteps | StepAccountBrief.TotalSteps (uint64) | "totalSteps" (number) | 0 | DB user_step_accounts.total_steps |
| data.stepAccount.availableSteps | StepAccountBrief.AvailableSteps | "availableSteps" | 0 | 同上 |
| data.stepAccount.consumedSteps | StepAccountBrief.ConsumedSteps | "consumedSteps" | 0 | 同上 |
| data.chest.id | ChestBrief.ID (uint64) | "id" (string) | 真实 | strconv.FormatUint |
| data.chest.status | ChestBrief.Status (int) | "status" (number) | 1 | service 动态判定 |
| data.chest.unlockAt | ChestBrief.UnlockAt (time.Time) | "unlockAt" (string ISO 8601 UTC) | 真实 | RFC3339 + .UTC() |
| data.chest.openCostSteps | ChestBrief.OpenCostSteps (uint32) | "openCostSteps" (number) | 1000 | DB user_chests.open_cost_steps |
| data.chest.remainingSeconds | ChestBrief.RemainingSeconds (int64) | "remainingSeconds" (number) | 0~600 | service 动态计算 |
| data.room.currentRoomId | (handler 直接构造) | "currentRoomId" (nil) | null | 节点 2 强制 null |

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.8 (行 1106-1137)] — 本 story 钦定 AC 来源（GET /home 五段聚合 + ≥4 单测 + dockertest 集成测试 + 节点 2 阶段 equips=[] / room.currentRoomId=null / chest 动态判定）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Epic 4 Overview (行 927-931)] — 节点 2 第一个业务 epic / 执行顺序 4.1 → 4.2 → 4.3 → 4.4 → 4.5 → 4.6 → **4.8** → 4.7
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.6 (行 1051-1082)] — 上游 story；本 story 消费 4.6 写入的 5 行数据（user / pet / step_account / chest）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.7 (行 1084-1104)] — 下游 Layer 2 集成测试；本 story 必须保证 LoadHome 失败路径返 1009 不部分降级
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 11.10] — 下游 room.currentRoomId 真实数据；本 story 必须预留 room 段扩展点
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 20.5] — 下游 chest_service.GetCurrent；本 story 落地的 chestStatusDynamic / computeRemainingSeconds 函数签名冻结供 20.5 复用
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 26.6] — 下游 pet.equips 真实数据；本 story 必须预留 equips 段扩展点
- [Source: `_bmad-output/planning-artifacts/epics.md` §AR3 (行 158)] — 状态以 server 为准；client 不做 chest 倒计时本地纠正
- [Source: `docs/宠物互动App_V1接口设计.md` §2.3 (行 41-44)] — Authorization: Bearer 头格式（本接口需要）
- [Source: `docs/宠物互动App_V1接口设计.md` §2.4 (行 47-63)] — envelope 结构 + 业务码与 HTTP status 正交
- [Source: `docs/宠物互动App_V1接口设计.md` §2.5 (行 65-72)] — BIGINT id 转 string + datetime ISO 8601 UTC
- [Source: `docs/宠物互动App_V1接口设计.md` §3 (行 76-118)] — 错误码 1001 / 1009 定义
- [Source: `docs/宠物互动App_V1接口设计.md` §5.1 (行 308-450)] — GET /home 完整 schema + Future Fields + 节点 2 阶段值
- [Source: `docs/宠物互动App_数据库设计.md` §3.1 (行 73-167)] — 主键 BIGINT UNSIGNED → uint64
- [Source: `docs/宠物互动App_数据库设计.md` §3.2] — 时间字段 DATETIME(3) 毫秒精度
- [Source: `docs/宠物互动App_数据库设计.md` §5.1 (行 173-207)] — users 表 schema
- [Source: `docs/宠物互动App_数据库设计.md` §5.3 (行 246-277)] — pets 表 + UNIQUE(user_id, is_default) 约束
- [Source: `docs/宠物互动App_数据库设计.md` §5.4 (行 280-314)] — user_step_accounts 表（user_id 主键）
- [Source: `docs/宠物互动App_数据库设计.md` §5.6 (行 362-395)] — user_chests 表 + UNIQUE(user_id) 约束
- [Source: `docs/宠物互动App_数据库设计.md` §6.4 (行 749-755)] — pets.current_state 枚举
- [Source: `docs/宠物互动App_数据库设计.md` §6.7 (行 771-776)] — user_chests.status 枚举（仅 1/2，无 opened）
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 (行 122-201)] — 目录树锚定 internal/{app/http/handler,service,repo/mysql}
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §5.1 (行 215-241)] — Handler 层职责
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §5.2 (行 242-267)] — Service 层职责（业务规则归属）
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §5.3 (行 269-...)] — Repository 层职责（CRUD）
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.2 (行 364-389)] — User / Home 模块钦定 home_service.go 单独承载
- [Source: `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md` §2 / §3] — 三层错误映射：repo 哨兵 → service apperror.Wrap → handler c.Error → middleware envelope
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` §2.1 / §2.2 / §2.3] — service / repo 第一参数 ctx；handler 用 c.Request.Context()；repo 必 .WithContext
- [Source: `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.1 / §3.5] — 单测 + 集成测试双层；Windows race skip
- [Source: `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md`] — GORM v1.25.x 选型；本 story 严格用 GORM API
- [Source: `_bmad-output/implementation-artifacts/4-1-接口契约最终化.md` §AC5] — V1 §5.1 schema 锚定（本 story 严格对齐）
- [Source: `_bmad-output/implementation-artifacts/4-2-mysql-接入.md`] — 上游 story；本 story 复用 db.Open + tx.FromContext 模式
- [Source: `_bmad-output/implementation-artifacts/4-3-五张表-migrations.md`] — 上游 story；本 story 消费 5 张表的 schema
- [Source: `_bmad-output/implementation-artifacts/4-4-token-util.md`] — 上游 story；本 story 通过 Auth 中间件间接消费 Signer
- [Source: `_bmad-output/implementation-artifacts/4-5-auth-rate_limit-中间件.md`] — 上游 story；本 story wire `middleware.Auth` + `middleware.RateLimit(cfg, RateLimitByUserID)`
- [Source: `_bmad-output/implementation-artifacts/4-6-游客登录接口-首次初始化事务.md`] — 上游 story；本 story 复用 4.6 已落地的 5 个 mysql repo + sentinel error pattern
- [Source: `docs/lessons/2026-04-26-v1接口设计-home-chest-status-必须严格按节点阶段限定状态空间.md`] — V1 §5.1 chest.status 节点 2 阶段只允许 1/2，**不**返 opened
- [Source: `docs/lessons/2026-04-26-契约schema字段可空性必须显式声明.md`] — V1 §5.1 data.pet 容器可空必须显式声明；本 story handler `"pet": null` 严格对齐
- [Source: `docs/lessons/2026-04-24-error-envelope-single-producer.md`] — ErrorMappingMiddleware 是唯一 envelope 生产者；本 story handler 严禁直接调 response.Error
- [Source: `docs/lessons/2026-04-24-middleware-canonical-decision-key.md`] — c.Keys 显式 canonical key 模式；本 story handler 用 `middleware.UserIDKey` 而非新建 ctx key
- [Source: `docs/lessons/2026-04-26-multi-table-tx-must-cover-all-unique-constraint-races.md`] — 4.6 review lesson：多表事务必须穷举所有唯一约束 race；本 story 不开事务但需理解上游 4.6 已正确落地
- [Source: `docs/lessons/2026-04-26-rate-limit-xff-spoof-and-buckets-cas.md`] — 4.5 review lesson：IP 限频用 RemoteIP；本 story authedGroup 用 RateLimitByUserID 维度（先 Auth 后 RateLimit）
- [Source: `docs/lessons/2026-04-26-jwt-required-claim-and-sign-policy-enforcement.md`] — JWT 必填 claim；本 story Auth 中间件已校验 token claims，handler 假设 userID 必然存在
- [Source: `CLAUDE.md` §"工作纪律"] — "状态以 server 为准 / ctx 必传"；本 story service 是 home 状态权威源
- [Source: `CLAUDE.md` §"Build & Test"] — 写完 / 改完 Go 代码后跑 `bash scripts/build.sh --test` 验证
- [Source: `MEMORY.md` "No Backup Fallback"] — 反对 fallback 掩盖核心风险；本 story 不部分降级（任一 repo 失败 → 1009）
- [Source: `MEMORY.md` "Repo Separation"] — server 测试自包含，不调 APP / watch；本 story 单测 + dockertest 集成，不依赖任何端联调

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Anthropic Opus 4.7, 1M context)

### Debug Log References

- 单元测试运行：`go test ./internal/service/... -run 'HomeService' -v` → 8/8 PASS
- 单元测试运行：`go test ./internal/app/http/handler/... -run 'HomeHandler' -v` → 5/5 PASS
- 单元测试运行：`go test ./internal/repo/mysql/... -run 'FindByUserID' -v` → 4/4 PASS（step_account 2 + chest 2）
- 全量测试：`bash /c/fork/cat/scripts/build.sh --test` → BUILD SUCCESS + all tests pass（含 4.6 既有测试不受影响）
- 集成测试编译：`go vet -tags=integration ./internal/service/...` → 通过（docker 不可用环境会 t.Skip 优雅退出，与 4.2/4.3/4.6 同模式）
- `go mod tidy` → go.mod / go.sum 无变化（仅消费已有依赖）

### Completion Notes List

- **AC1 home_handler.go**：实装 LoadHome 严格对齐 V1 §5.1 schema：BIGINT id 转 string、pet 可空（`var petDTO any` + nil → JSON null）、equips 用 `[]any{}`（**不**用 nil 防序列化为 null）、unlockAt 用 RFC3339、room.currentRoomId 节点 2 强制 nil。userID 提取双层断言（c.Get + 类型断言）兜底走 1009。错误一律 c.Error + return 透传给 ErrorMappingMiddleware（ADR-0006 单一 envelope 生产者）。
- **AC2 home_service.go**：4 repo 串行（user → pet → step → chest）+ chest 动态判定（chestStatusDynamic / computeRemainingSeconds 包级 helper，函数签名冻结供 Story 20.5 复用）。**不部分降级**：user/step/chest 任一 NotFound 都包成 1009；仅 pet ErrPetNotFound 视为 Pet=nil 不报错（V1 §5.1 钦定可空）。chest.UnlockAt.UTC() 强制 UTC 视图防 GORM driver loc=Local 漂移。
- **AC3 repo 扩展**：StepAccountRepo + ChestRepo interface 各加 FindByUserID 方法；errors.go 加 ErrStepAccountNotFound / ErrChestNotFound 哨兵；用 `tx.FromContext(ctx, r.db)` + `.WithContext(ctx)` 与 4.6 同模式（ADR-0007 §2.3）。stderrors.Is(gorm.ErrRecordNotFound) 翻译为 sentinel error。
- **AC4 router wire**：在已有 `if deps.GormDB != nil && ...` 块内追加 homeSvc + homeHandler + authedGroup（Auth + RateLimitByUserID）+ GET /home route。**不**改 4.6 /auth 子组 wire，**不**新增 Deps 字段。
- **AC5 handler 单测 5 case**：HappyPath（完整 V1 §5.1 schema 字段断言 + 字面量 `"equips":[]` / `"currentRoomId":null`） / chest unlocked → status=2 / pet=nil → `"pet":null` 字面量验证（**关键**：bytes.Contains 防 LLM 误返 `{}`） / service 1009 透传 / userID 缺失兜底 1009。
- **AC6 service 单测 8 case**：4 repo 全成功 / chest 动态判定 / pet NotFound → Pet=nil 不报错 / user 失败 → 1009 / userNotFound → 1009 / step NotFound → 1009 / chest NotFound → 1009 / pet 非 NotFound → 1009 区分。**关键 race**：user/step/chest NotFound 都包 1009（不视为可空），仅 pet 视为可空。
- **AC7 repo 单测 4 case**：StepAccountRepo + ChestRepo 各 happy + NotFound 共 4 case，sqlmock 验 SQL pattern + sentinel error 翻译。
- **AC8 dockertest 集成测试 3 case**：复用 4.6 startMySQL/runMigrations helper（同 service_test 包内直调）；手工 INSERT 4 表数据（不调 auth_service.GuestLogin —— 解耦 home / auth 测试）。HappyPath（容忍 RemainingSeconds ∈[598,602] 抗时钟漂移） / chest unlocked → Status=2 / 不 INSERT pets → Pet=nil 不报错。
- **关键决策点**：
  1. **stub repo 命名加 Home 前缀**（stubHomeUserRepo 等）避免与 4.6 auth_service_test.go 同包内 stubUserRepo 命名冲突 —— 不影响 home 链路独立验证；4.6 的 stub 也扩展了 FindByUserID 签名（panic 兜底，因为 auth_service 不调本方法）确保 interface 兼容。
  2. **chestStatusDynamic 保留 dbStatus 参数**（节点 2 阶段未使用，`_ = dbStatus`），是为 Story 20.5 节点 7 chest_service 准备 —— future 扩 switch 不改函数签名（行为契约冻结）。
  3. **petDTO 用 `var petDTO any`** 而非 `var petDTO gin.H`，让 nil 序列化为 JSON `null` —— 与 V1 §5.1 行 335 钦定可空对象语义一致；Story 4.6 round 5 lesson 明确反对 `gin.H{}` 占位。
  4. **不部分降级**：user / step / chest 任一 NotFound 都包成 1009 透传给 client，让 SilentRelogin 兜底，**不**返半截 HomeOutput（避免主界面渲染异常引发更深的客户端错误链；epics.md §Story 4.8 行 1136）。
- **范围红线全部遵守**：未实装 GET /me / bind-wechat / state-sync；未查 user_pet_equips；未读 users.current_room_id；未引入并发 errgroup；未开事务；未改 V1 接口设计文档 / 数据库设计文档 / 设计文档 / lessons。

### File List

**新增**：
- `server/internal/service/home_service.go`
- `server/internal/service/home_service_test.go`
- `server/internal/service/home_service_integration_test.go`（build tag `integration`）
- `server/internal/app/http/handler/home_handler.go`
- `server/internal/app/http/handler/home_handler_test.go`

**修改**：
- `server/internal/repo/mysql/errors.go`（加 `ErrStepAccountNotFound` / `ErrChestNotFound` 哨兵 error + godoc）
- `server/internal/repo/mysql/step_account_repo.go`（interface 加 `FindByUserID` + impl + import stderrors）
- `server/internal/repo/mysql/step_account_repo_test.go`（加 happy + NotFound 共 2 case + import stderrors）
- `server/internal/repo/mysql/chest_repo.go`（interface 加 `FindByUserID` + impl + import stderrors）
- `server/internal/repo/mysql/chest_repo_test.go`（加 happy + NotFound 共 2 case + import stderrors）
- `server/internal/app/bootstrap/router.go`（NewRouter 内追加 homeSvc + homeHandler wire + authedGroup（Auth + RateLimitByUserID）+ GET /home route）
- `server/internal/service/auth_service_test.go`（4.6 stubStepAccountRepo / stubChestRepo 加 FindByUserID 方法兜底，让 interface 扩展后 auth_service 测试仍编译通过）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（4-8 状态：ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/4-8-get-home-聚合接口.md`（本 story 文件，dev 完成后填 Tasks/Dev Agent Record/File List/Change Log/Status）

### Change Log

- 2026-04-27 实装 GET /api/v1/home 聚合接口（Story 4.8）：5 段 schema（user / pet 可空 / stepAccount / chest 动态判定 / room null）+ 4 repo 串行 + chestStatusDynamic / computeRemainingSeconds 包级 helper（Story 20.5 复用预留）+ ≥17 单测 + ≥3 dockertest 集成测试。范围严格对齐 epics.md §Story 4.8 + V1 §5.1。状态推进：ready-for-dev → review。
