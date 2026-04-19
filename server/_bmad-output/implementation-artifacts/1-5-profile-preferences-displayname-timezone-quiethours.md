# Story 1.5: Profile 与偏好设置（displayName / timezone / quietHours）

Status: review

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a signed-in user,
I want to set my display name and local timezone + quiet-hours window, and have my client auto-report timezone changes,
so that friends see my cat labeled correctly and nighttime touches are silenced per my local time (FR48, FR49, FR50).

## Acceptance Criteria

**Given** Story 1.3 的 JWT middleware（`middleware.UserIDFrom` / `DeviceIDFrom` / `PlatformFrom` 已注入 context）+ Story 0.10 的 `ws.DedupStore` + Story 0.12 的 `ws.ResumeCacheInvalidator`（`redisx.RedisResumeCache.Invalidate` 已实现）+ Story 0.13 的 `push.QuietHoursResolver` 接口（目前挂 `EmptyQuietHoursResolver`）+ Story 1.1 的 `users` collection + `internal/domain/user.go` 的 `DisplayName *string / Timezone *string / Preferences.QuietHours{Start,End}` 字段（已预留）。

**1. WS 消息注册表登记（§21.1 四步走 gate）**

**Given** `internal/dto/ws_messages.go` 的 `WSMessages` 切片是 WS envelope.type 的唯一真相，且所有 dispatcher 注册必须与之 1:1 匹配（Story 0.14 AC4/AC15 双 gate：单测 `TestWSMessages_ConsistencyWithDispatcher_*` + 启动期 `validateRegistryConsistency`）
**When** 新增 `profile.update` RPC
**Then** 在 `WSMessages` 追加一条 `{Type: "profile.update", Version: "v1", Direction: WSDirectionBi, RequiresAuth: true, RequiresDedup: true, DebugOnly: false, Description: "Client updates profile (displayName / timezone / quietHours); authoritative write, idempotent via envelope.id dedup."}`
**And** 在 `cmd/cat/initialize.go` 用 `dispatcher.RegisterDedup("profile.update", profileHandler.HandleUpdate)` 注册（写类 RPC，NFR-SEC-9 需 eventId 幂等；Direction=Bi 因为 client→server 请求+server→client response）
**And** 更新 `docs/api/ws-message-registry.md` 增 `### profile.update (bi, v1, auth required)` section：描述、请求 payload schema、响应 payload schema、Dedup: required
**And** 更新 `docs/api/openapi.yaml` `/v1/platform/ws-registry` 响应示例（如果列了具体 types）
**And** 运行 `bash scripts/build.sh --test` 确认 `TestWSMessages_ConsistencyWithDispatcher_ReleaseMode` / `_DebugMode` / `TestValidateRegistryConsistency_ReleaseMode` 全绿（双 gate 自证通过）

**2. DTO 与 validator（handler 层 P7）**

**Given** `docs/backend-architecture-guide.md` §P7 要求 DTO 层校验 + Gin binding + validator v10 自动触发
**When** 新建 `internal/dto/profile_dto.go`
**Then** 定义 `ProfileUpdateRequest` 结构：
  - `DisplayName *string` `json:"displayName,omitempty"` —— optional；非 nil 时：trim 前后空白后长度 ∈ [1,32] UTF-8 字符、不含 ASCII 控制字符（`\x00-\x1F` + `\x7F`）、必须 `utf8.ValidString`
  - `Timezone *string` `json:"timezone,omitempty"` —— optional；非 nil 时：必须能被 `time.LoadLocation` 解析（IANA 如 `Asia/Shanghai` / `America/New_York` / `UTC`）
  - `QuietHours *QuietHoursDTO` `json:"quietHours,omitempty"` —— optional；非 nil 时 `Start` 和 `End` 均必须符合 `HH:MM` 24h 格式（`^([01]\d|2[0-3]):[0-5]\d$`，锁死 0-23 小时 + 0-59 分，拒绝 24:00 / 25:00 / 26:90 等）
**And** 定义 `QuietHoursDTO{Start string, End string}` 双 tag `json:"start"/"end"` + `bson:"start"/"end"` 对齐（P2 DTO 双 tag 强制）
**And** payload 至少 1 个顶层字段非 nil（三者全 nil → `VALIDATION_ERROR`："at least one of displayName/timezone/quietHours must be provided"）
**And** Start == End **允许**（表示"24 小时免打扰"；用户想永久静音合法；边界语义锁到 Dev Notes 思考题 #3）
**And** 定义 `ProfileUpdateResponse{User UserPublicProfile}`，其中 `UserPublicProfile{ID, DisplayName?, Timezone?, Preferences{QuietHours{Start,End}}}` 扩 `dto.UserPublic` 补 QuietHours（保持向后兼容 Story 1.1 的 UserPublic JSON 契约，新增嵌套字段）
**And** 单元测试 `internal/dto/profile_dto_test.go` ≥ 12 子测覆盖：每字段 nil/非 nil combination / trim 边界 / 控制字符拒绝 / UTF-8 非法字节拒绝 / 非 IANA 拒绝（`"Pacific/Nope"`）/ HH:MM 边界（`"00:00"`/`"23:59"`/`"24:00"` 拒绝/`"23:5"` 拒绝/`"ab:cd"` 拒绝）/ 空 payload 拒绝

**3. Repository 扩展（single $set partial update）**

**Given** `internal/repository/user_repo.go` 已有 `MongoUserRepository.Insert / FindByID / UpsertSession*` 等方法
**When** 扩 `UpdateProfile(ctx context.Context, userID ids.UserID, p ProfileUpdate) (*domain.User, error)`
**Then** `ProfileUpdate` 是 repository 包内类型 `{DisplayName *string; Timezone *string; QuietHours *domain.QuietHours}`（与 DTO 不同：这里用 `*domain.QuietHours`，handler 层在 service 内转换）
**And** 实现细节：
  - 对三字段**单次** `UpdateOne` + `$set`（不是三次调用；避免 TOCTOU 与 Mongo 写压力翻倍 —— Story 1.4 AC Review 硬化 #5 同款模式）
  - 构建 `setDoc bson.M{"updated_at": clock.Now()}`
  - `DisplayName != nil` → `setDoc["display_name"] = *p.DisplayName`（BSON 字段对齐 §P1 snake_case）
  - `Timezone != nil` → `setDoc["timezone"] = *p.Timezone`
  - `QuietHours != nil` → `setDoc["preferences.quiet_hours.start"] = p.QuietHours.Start` 以及 `setDoc["preferences.quiet_hours.end"] = p.QuietHours.End`（**嵌套字段 dotted $set**，不是整块替换 preferences —— 保证 `preferences` 未来扩展字段不被清零）
  - `UpdateOne` + `ReturnDocument: After` 读回 `*domain.User`（`FindOneAndUpdate` 返回更新后文档）
  - `MatchedCount == 0` → `ErrUserNotFound`
**And** 单元测试不写（repo 层测试依赖 mongo，走 integration）
**And** 集成测试 `internal/repository/user_repo_integration_test.go`（已存在）追加 `TestMongoUserRepo_Integration_UpdateProfile` 6 子测：
  - `PartialDisplayNameOnly`：其他字段保持
  - `PartialTimezoneOnly`：`display_name` 保持 nil / `preferences.quiet_hours` 保持 seed 默认
  - `PartialQuietHoursOnly`：不动 displayName / timezone
  - `AllThreeFields`：一次 UpdateOne 三字段齐改
  - `UserNotFound`：返 `ErrUserNotFound`
  - `PreservesSessionsAndFriendCount`：更新 profile 后 `sessions / friend_count / consents` 原值未被意外重置（嵌套 dotted $set 的核心不变量 —— 这条 locks Dev Notes 思考题 #7）

**4. Service 层（业务组合）**

**Given** 架构指南 §6.3 要求 Service 层做业务组合；§M7 Service ↔ Repository 边界：repo 返 `*domain.User`，service 负责事务 / 调用其他 service / 调用 invalidator
**When** 新建 `internal/service/profile_service.go` 定义 `ProfileService`
**Then** 依赖注入：`userRepo ProfileUpdater`（消费者接口，仅 `UpdateProfile`）+ `invalidator ResumeCacheInvalidator`（消费者接口，仅 `Invalidate(ctx, userID) error`）+ `clock clockx.Clock`
**And** 构造函数 `NewProfileService(repo, invalidator, clk)` 三依赖 nil 任一 panic（§P3 fail-fast startup）
**And** 实现 `Update(ctx, userID ids.UserID, p ProfileUpdate) (*domain.User, error)`：
  - 调 `userRepo.UpdateProfile(ctx, userID, p)` —— 失败直接返（包括 `ErrUserNotFound` 透传；handler 层映射成 `dto.ErrInternalError`，不暴露给客户端是否存在该用户）
  - 成功后**同步**调 `invalidator.Invalidate(ctx, string(userID))`：**失败 log warn 但**不**影响主响应**（fail-open —— resume cache 是性能层，60s TTL 自愈；Story 0.12 / 1.1 已确立该模式）
  - zerolog audit log：`Str("action", "profile_update").Str("userId", userID).Strs("fields", [changed_field_names])`；**fields 列表用 enum 字符串**，**不**记 displayName / timezone / quietHours 的**值**（M13 PII：displayName 必须 `[REDACTED]` 或整条省略）
  - 返 `*domain.User`（来自 `UpdateOne ReturnDocument: After`），handler 层做 DTO 投影
**And** 单元测试 `internal/service/profile_service_test.go` ≥ 8 子测（fake `ProfileUpdater` + fake `ResumeCacheInvalidator` + `FakeClock`）：
  - happy path 三字段
  - repo 返 `ErrUserNotFound` 透传
  - repo 返 generic err 透传
  - invalidator 返 err → 主响应仍成功 + warn log（用 `zerolog` in-memory sink 捕获 `action="resume_cache_invalidate_error"`）
  - fields 日志 enum 正确（仅 displayName）
  - fields 日志 enum 正确（displayName + quietHours）
  - Audit log **不**含 displayName 原文（scan 字符串保证 redacted）
  - `Clock.Now()` 被用（而非 `time.Now()` 裸调；M9 CI 拦截外 per-test verifier）

**5. WS handler（dispatcher 适配）**

**Given** `internal/ws/dispatcher.go` 的 `HandlerFunc(ctx, *Client, Envelope) (json.RawMessage, error)` + `RegisterDedup` 自动裹 dedup middleware（Story 0.10）+ `Client.UserID()` 返已鉴权 userID
**When** 新建 `internal/ws/profile_handler.go`
**Then** 定义 `ProfileHandler` 结构体，依赖 `profileUpdater` 消费者接口（在 internal/ws 内**本地定义**，签名 `Update(ctx, userID ids.UserID, p service.ProfileUpdate) (*domain.User, error)` —— 同 session_resume.go 的 `userByIDLookup` 模式）
**And** 实际注入 `*service.ProfileService` 满足该接口
**And** 构造函数 `NewProfileHandler(updater)` nil panic
**And** `HandleUpdate(ctx, client *Client, env Envelope) (json.RawMessage, error)`：
  - `env.Payload` 解 `ProfileUpdateRequest`；非法 JSON → `validationError("invalid profile.update payload")`（复用 room_mvp.go 的 helper 模式）
  - 在 handler 内部**手动**跑 validator（WS 路径没有 Gin binding 自动触发）：调 `dto.ValidateProfileUpdateRequest(req) error`（在 profile_dto.go 定义纯 Go 校验函数，handler 测试覆盖；不依赖 go-playground/validator 因 HH:MM 正则 / IANA LoadLocation 都是需自己写的语义校验）
  - 调 `service.ProfileService.Update(ctx, client.UserID(), toDomainPartial(req))` —— 转换辅助 `toDomainPartial(req ProfileUpdateRequest) ProfileUpdate`（trim displayName 前后空白后再 *string；QuietHoursDTO → domain.QuietHours）
  - 成功 → `json.Marshal(ProfileUpdateResponse{User: userPublicProfileFromDomain(u)})`
  - service 返 `ErrUserNotFound` → 映射成 `dto.ErrInternalError`（handler 不应暴露"该 userId 不存在"，这只在数据损坏时发生）
  - service 返其他 err → `dto.ErrInternalError.WithCause(err)`
**And** Dispatcher 注册通过 `RegisterDedup`（§21.1 step 2），dedup middleware 的 `EVENT_PROCESSING` / replay-cache 由 Story 0.10 自动处理；本 handler **不**自己写 dedup 逻辑
**And** 单元测试 `internal/ws/profile_handler_test.go` ≥ 10 子测（fake `profileUpdater`）：
  - happy path 三字段 → OK + payload shape
  - 非法 JSON → VALIDATION_ERROR
  - 空 payload（无字段）→ VALIDATION_ERROR
  - displayName trim 后空 → VALIDATION_ERROR
  - displayName 控制字符 → VALIDATION_ERROR
  - timezone 非 IANA → VALIDATION_ERROR
  - quietHours 非 HH:MM → VALIDATION_ERROR
  - service 返 `ErrUserNotFound` → INTERNAL_ERROR（不泄漏 user 存在性）
  - service 返 generic err → INTERNAL_ERROR
  - PII mask 正确：log 输出不含 displayName 原文

**6. QuietHoursResolver 真实实现（§21.2 Empty→Real 第 4 个 APNs Provider 填实）**

**Given** `internal/push/providers.go` 的 `QuietHoursResolver interface { Resolve(ctx, userID ids.UserID) (quiet bool, err error) }` + 现挂 `EmptyQuietHoursResolver{}`（恒返 `(false, nil)`）+ APNsWorker 在 consume 时调 `.Resolve` 决定是否降级 alert→silent（见 `internal/push/apns_worker.go` L267-275）
**When** 新建 `internal/push/real_quiet_hours_resolver.go` 实现 `RealQuietHoursResolver`
**Then** 依赖：`userLookup` 消费者接口（本地定义签名 `FindByID(ctx, ids.UserID) (*domain.User, error)`）+ `clockx.Clock`
**And** 构造 `NewRealQuietHoursResolver(lookup, clk)` nil panic
**And** `Resolve(ctx, userID)`：
  - `FindByID` 失败：`ErrUserNotFound` → 返 `(false, nil)`（fail-open，合约保证 §21.3 + providers.go godoc 注释 L83-90：missing user → not quiet）
  - 其他 err → 返 `(false, err)`（APNsWorker 会 warn log 后 fail-open 投递 alert —— 保持既有合约）
  - `user.Timezone == nil || *user.Timezone == ""` → 返 `(false, nil)`（fail-open："用户没设时区，按不 quiet 发"）
  - `time.LoadLocation(*user.Timezone)` 失败（历史脏数据）→ 返 `(false, nil)` + warn log `action="quiet_hours_bad_timezone"` userId（不 err，避免整链路阻塞）
  - 加载 user.Preferences.QuietHours.Start / End 解析 HH:MM → `(hStart, mStart)` / `(hEnd, mEnd)`；解析失败 → 返 `(false, nil)` + warn（同上 fail-open）
  - `nowLocal := clock.Now().In(loc)`；取 `nowH, nowM = nowLocal.Hour(), nowLocal.Minute()`
  - 比较：
    - 若 `start == end` → **永久静默**，返 `(true, nil)`（AC2 允许 start==end；表示"24 小时免打扰"）
    - 若 `start < end`（同日窗口，例如 `10:00-15:00`）→ `(nowH*60+nowM) ∈ [start, end)` → quiet
    - 若 `start > end`（跨日窗口，例如 `23:00-07:00`）→ `(nowH*60+nowM) ∈ [start, 24:00) ∪ [00:00, end)` → quiet（语义：23:00 到次日 07:00 期间）
  - **边界**：区间**左闭右开**（`[start, end)`）；Start 的那一分钟就算 quiet，End 的那一分钟**不**算 quiet。（让"07:00 解除"在 07:00:00 立刻生效直观）
**And** 单元测试 `internal/push/real_quiet_hours_resolver_test.go` ≥ 12 子测（fake `userLookup` + `clockx.FakeClock`）：
  - user 不存在 → `(false, nil)`
  - user.Timezone nil → `(false, nil)`
  - user.Timezone 非法字符串 → `(false, nil)` + warn
  - quietHours 非 HH:MM（脏数据）→ `(false, nil)` + warn
  - 同日窗口 `10:00-15:00`，now = 12:00 → quiet
  - 同日窗口 `10:00-15:00`，now = 09:59 → not quiet
  - 同日窗口边界 `10:00-15:00`，now = 10:00 → quiet（左闭）
  - 同日窗口边界 `10:00-15:00`，now = 15:00 → **not** quiet（右开）
  - 跨日窗口 `23:00-07:00`，now(local) = 23:00 → quiet
  - 跨日窗口 `23:00-07:00`，now(local) = 03:00 → quiet
  - 跨日窗口 `23:00-07:00`，now(local) = 07:00 → not quiet
  - 跨日窗口 `23:00-07:00`，now(local) = 22:59 → not quiet
  - `start == end`（`22:00-22:00`）→ 永远 quiet
  - Timezone 差异：同一 UTC 时刻下 `America/New_York` 用户与 `Asia/Shanghai` 用户得到不同结论（锁死 `time.Now().In(loc)` 的逻辑确实生效）

**7. initialize.go wiring（§21.2 执行 Empty→Real）**

**Given** `cmd/cat/initialize.go` L108 目前 `push.NewAPNsWorker(..., push.EmptyQuietHoursResolver{}, apnsTokenRepo, clk)`
**When** 本 story 实施
**Then** 构造 `realQuietHoursResolver := push.NewRealQuietHoursResolver(userRepo, clk)`（`userRepo` 来自 Story 1.1 已创建的 `MongoUserRepository`）
**And** 把 `push.EmptyQuietHoursResolver{}` 替换为 `realQuietHoursResolver`
**And** 在被替换位置保留注释 `// QuietHoursResolver real impl — removed Empty via Story 1.5 (§21.2 Empty→Real Provider 填实).`（Epic 0 retro action #4：保留 `// removed by Story X.Y` 注释）
**And** 构造 `profileSvc := service.NewProfileService(userRepo, resumeCache, clk)` —— `resumeCache` 来自 Story 0.12 已创建的 `*redisx.RedisResumeCache`（同时实现 `ws.ResumeCache` 与 `ws.ResumeCacheInvalidator`）
**And** 构造 `profileHandler := ws.NewProfileHandler(profileSvc)`
**And** `dispatcher.RegisterDedup("profile.update", profileHandler.HandleUpdate)`（release & debug 模式都注册 —— WSMessage.DebugOnly=false）
**And** 新增 `initialize_test.go` 子测 `TestInitialize_ProfileUpdateDispatcherRegistered_ReleaseMode` 与 `_DebugMode`（双模式验证 dispatcher 注册；§7.1 双 gate 纪律）

**8. ResumeCacheInvalidator 契约复用 — 无新接口**

**Given** `internal/ws/session_resume.go` 已定义 `ResumeCacheInvalidator interface { Invalidate(ctx, userID string) error }`；`pkg/redisx.RedisResumeCache.Invalidate` 已实现
**When** Profile service 做 cache 失效
**Then** 直接复用既有接口；**不**定义新接口
**And** Service 构造函数参数类型用 `ws.ResumeCacheInvalidator`（注意 import 方向：`internal/service → internal/ws` 是合法的；`internal/ws → internal/service` 才是禁止的）
**And** **不**改 `internal/ws/session_resume.go`；**不**改 `pkg/redisx/resume_cache.go`

**9. PII 日志与 §M13 合规**

**Given** 架构指南 §M13：`displayName` 是 PII 必须 `[REDACTED]`；`pkg/logx.MaskPII` helper 可用
**When** 本 story 任何代码路径触及 displayName 值
**Then** **所有** `logx.Ctx(ctx).*()` / `log.*()` 调用**禁**记 displayName 原值 —— 哪怕 DEBUG level
**And** 只记 **field enum**（枚举字符串 `"displayName"` / `"timezone"` / `"quietHours"`），让运维知道"哪个字段改了"但看不到改成什么
**And** timezone / quietHours **允许**记原值（非 PII；时区和作息不像 displayName 那样有人格识别风险，且运维排错时需要看原值）
**And** CI 自测 `profile_service_test.go` 里 `TestProfileService_Update_DoesNotLogDisplayNameValue` 扫写入 zerolog 的 buffer 字符串，断言**不**包含测试用例的 `"Alice"` / `"王小明"` 等 displayName 测试值

**10. Fail-closed / Fail-open 矩阵（§21.3 强制声明）**

| 场景 | 策略 | 故障时行为 | 可观测 |
|---|---|---|---|
| `userRepo.UpdateProfile` Mongo 写失败 | **fail-closed** | 返 `ErrInternalError` 给客户端（5xx） | audit log + zerolog error |
| `ResumeCacheInvalidator.Invalidate` Redis 失败 | **fail-open** | 主响应仍成功；用户看到 profile 已更新 | warn log `action="resume_cache_invalidate_error"` + TTL 60s 自愈 |
| `RealQuietHoursResolver` Mongo 失败 | **fail-open** | 返 `(false, err)`；APNsWorker warn 后投 alert | APNsWorker 的 `action="apns_quiet_resolve_error"` warn log |
| `RealQuietHoursResolver` user missing / timezone nil / IANA 非法 / HH:MM 非法 | **fail-open (no err)** | 返 `(false, nil)`；APNsWorker 投 alert | bad_timezone / bad_quiet_hours warn log |
| handler / service validation 失败 | **fail-closed** | 返 `VALIDATION_ERROR`（400 类） | WS 错误 envelope + log（仅错误类型，不含 payload 原值） |
| DispatcherDedup middleware `EVENT_PROCESSING` | 继承 Story 0.10 | 返 `EVENT_PROCESSING`（client 退避） | dedup ZADD / GET 成对 |

**And** 文档对齐：上述矩阵必须在实施完成后纳入 Completion Notes 自审（Story 1.1 / 1.4 同款 self-audit 节奏）

**11. 集成测试（§21.7 server 测试自包含）**

**Given** Story 1.1-1.4 既有 integration harness（`cmd/cat/sign_in_with_apple_integration_test.go` / `refresh_token_integration_test.go` / `jwt_middleware_integration_test.go` / `apns_token_integration_test.go`）用 Testcontainers Mongo + miniredis / Testcontainers Redis
**When** 新建 `cmd/cat/profile_update_integration_test.go` `//go:build integration`
**Then** 扩展 harness（新增 `setupProfileHarness(t)` 或复用 `setupJWTAuthHarness`）使用 WS upgrade + handshake：
  - 启动 Testcontainers Mongo + Redis + in-process HTTP + WS Hub + 全 dispatcher 注册（包括本 story 新增）
  - 用 `signInWithApple` helper 获 access token + userId
  - gorilla/websocket Dial `ws://.../ws` + Authorization: Bearer
  - 发 `{"id":"...","type":"profile.update","payload":{"displayName":"Alice","timezone":"Asia/Shanghai","quietHours":{"start":"23:00","end":"07:00"}}}`
  - 断言响应 `ok:true` + payload.user 字段正确
  - 断言 Mongo `users._id=<userID>` 文档 `display_name / timezone / preferences.quiet_hours.start / .end` 已落库
  - 断言 Redis `resume_cache:<userID>` 已被 DEL（Invalidate 生效）
**And** 6 子测覆盖：
  - `HappyPath_AllThreeFields`
  - `Partial_DisplayNameOnly`（timezone / quietHours 保持 seed）
  - `Partial_TimezoneOnly`
  - `ResumeCacheInvalidated_VerifiesRedisDel`（先注入假 cache 再调 update 验证 DEL）
  - `ReplayEventIDReturnsCachedResponse`（dedup middleware 生效，Story 0.10 合约）
  - `InvalidPayload_ReturnsValidationError`（timezone=`Pacific/Nope` → WS 错误 envelope `VALIDATION_ERROR`）
**And** 额外增 `cmd/cat/quiet_hours_resolver_integration_test.go` 1 子测 `TestQuietHoursResolver_Integration_EndToEnd`：
  - 注入 user 文档（`timezone="Asia/Shanghai"`, `quietHours={start:"23:00", end:"07:00"}`）
  - 调 `realQuietHoursResolver.Resolve(ctx, userID)` 并**控制 Clock** 至 UTC 16:00（= 上海时间次日 00:00）→ 应 quiet
  - 改 Clock 至 UTC 00:00（= 上海 08:00）→ 应 not quiet
  - 验证"time.Now().In(loc)"真实生效 / 不是 UTC 比较
**And** 运行 `go vet -tags=integration ./...` 通过（Windows 下 Docker 不可用时 vet 编译仍需绿；实际执行依赖 Linux CI）

**12. openapi.yaml 与客户端指南更新（文档漂移 gate）**

**Given** `docs/api/openapi.yaml` 当前 `version: "1.4.0-epic1"`；`docs/api/integration-mvp-client-guide.md` 最新节是 §14（Story 1.4 APNs 注册）
**When** 本 story 上线
**Then** `openapi.yaml` bump 到 `1.5.0-epic1`；`/v1/platform/ws-registry` 响应示例追加 `profile.update` 条目（如果既有测试断言了具体类型数）
**And** `docs/api/ws-message-registry.md` 追加 `### profile.update (bi, v1, auth required)` section，内容：
  - Description：用户更新 displayName / timezone / quietHours 部分字段（至少一个）
  - Dedup: required（见 Story 0.10）
  - Request payload schema（含三字段 optional + HH:MM 格式说明 + IANA tz 说明 + displayName 长度 1-32）
  - Response payload schema（UserPublicProfile 含 preferences.quietHours）
  - 错误码：`VALIDATION_ERROR`（payload 非法）/ `EVENT_PROCESSING`（dedup 冲突）/ `INTERNAL_ERROR`
**And** `integration-mvp-client-guide.md` 追加 `## §15 Profile 更新与时区上报（Story 1.5）`：
  - WS RPC `profile.update` 使用说明
  - FR50 实现：客户端 iOS/watchOS 监听 `TimeZone.current` 变化，触发本 RPC（仅携带 `timezone` 字段）；服务端不区分是"设置"还是"时区自动上报" —— 都走同一个 endpoint
  - 默认 quietHours `23:00-07:00`（Story 1.1 seed），用户首次 override 前客户端 UI 可显示默认值

**13. §21.1 常量 / 注册表漂移守门双 gate 自证（机械化验证）**

**Given** Story 0.14 AC4/AC15 既有双 gate 机制：单测遍历 WSMessages 与 Dispatcher 注册的对称性 + 启动期 `validateRegistryConsistency` fail-fast
**When** 本 story 追加 `profile.update` WSMessage + dispatcher.RegisterDedup 注册
**Then** 以下测试**不修改任何断言**仍全绿，证明新类型被双 gate 接住：
  - `TestWSMessages_ConsistencyWithDispatcher_ReleaseMode`
  - `TestWSMessages_ConsistencyWithDispatcher_DebugMode`
  - `TestValidateRegistryConsistency_ReleaseMode`
  - `TestValidateRegistryConsistency_DebugMode`
**And** 主动增一条 fail-fast 自证测试 `TestWSMessages_ProfileUpdate_Entry`：断言 `dto.WSMessagesByType["profile.update"]` 存在且 `RequiresDedup=true / RequiresAuth=true / DebugOnly=false / Direction=Bi`（防止未来 refactor 误翻 flag）

**14. §21.8 Semantic-correctness 思考题（§19 PR checklist 第 14 条强制）**

实施前需在 Dev Notes Semantic-correctness 段落逐条回答：本 story 的代码运行时产生错误结果但没 crash，**谁会被误导**？详见下文 Dev Notes §Semantic-correctness 思考题（9 条陷阱）。实施后在 Completion Notes List 逐条 self-audit 覆盖状态。

---

## Tasks / Subtasks

- [x] **Task 1: DTO 层** (AC: #2)
  - [x] 新建 `internal/dto/profile_dto.go`：`ProfileUpdateRequest / QuietHoursDTO / ProfileUpdateResponse / UserPublicProfile / ValidateProfileUpdateRequest(req) error`
  - [x] HH:MM 正则锁死 `^([01]\d|2[0-3]):[0-5]\d$`
  - [x] IANA tz 用 `time.LoadLocation` 验证（若失败直接返 validation err，不 fallback）
  - [x] displayName trim + 控制字符 reject + UTF-8 valid + 长度 [1,32]
  - [x] "至少一个字段" 校验
  - [x] 单测 `profile_dto_test.go` ≥ 12 子测

- [x] **Task 2: Repository 扩展** (AC: #3)
  - [x] 在 `user_repo.go` 追加 `ProfileUpdate` 结构 + `UpdateProfile` 方法
  - [x] 使用 `FindOneAndUpdate` + `ReturnDocument: After` + dotted `$set` on `preferences.quiet_hours.{start,end}`
  - [x] 集成测试 `user_repo_integration_test.go` 追加 `TestMongoUserRepo_Integration_UpdateProfile` 6 子测

- [x] **Task 3: Service 层** (AC: #4, #9, #10)
  - [x] 新建 `internal/service/profile_service.go`
  - [x] `ProfileUpdater` + `ResumeCacheInvalidator` 消费接口
  - [x] `Update` 组合 repo.Update + invalidator.Invalidate（fail-open）
  - [x] audit log（fields enum；**不**记 displayName 原值）
  - [x] 单测 `profile_service_test.go` ≥ 8 子测（含 DoesNotLogDisplayNameValue PII 断言）

- [x] **Task 4: WS handler** (AC: #5)
  - [x] 新建 `internal/ws/profile_handler.go`
  - [x] `HandleUpdate` satisfies `HandlerFunc`
  - [x] envelope.Payload decode / validator / service / response marshal 链路
  - [x] err mapping（ErrUserNotFound / generic → ErrInternalError；不暴露存在性）
  - [x] 单测 `profile_handler_test.go` ≥ 10 子测

- [x] **Task 5: QuietHoursResolver 真实实现** (AC: #6)
  - [x] 新建 `internal/push/real_quiet_hours_resolver.go`
  - [x] 含时区 / HH:MM / 跨日窗口的逻辑（左闭右开）
  - [x] 所有非致命解析错 fail-open
  - [x] 单测 `real_quiet_hours_resolver_test.go` ≥ 12 子测（含跨时区 / 边界 / 跨日 / 永久静默 start==end / 脏数据）

- [x] **Task 6: WSMessage 注册 + docs 漂移 gate** (AC: #1)
  - [x] `dto/ws_messages.go` 新增 `profile.update` 条目
  - [x] `docs/api/ws-message-registry.md` 新增 section
  - [x] `docs/api/openapi.yaml` version bump → `1.5.0-epic1`
  - [x] `docs/api/integration-mvp-client-guide.md` 新增 §15
  - [x] `TestWSMessages_ProfileUpdate_Entry` 自证测试 (AC #13)

- [x] **Task 7: initialize.go wiring + Empty→Real swap** (AC: #7)
  - [x] 构造 `profileSvc / profileHandler / realQuietHoursResolver`
  - [x] `RegisterDedup("profile.update", profileHandler.HandleUpdate)`
  - [x] 替换 `push.EmptyQuietHoursResolver{}` → `realQuietHoursResolver` + 留 `// removed via Story 1.5` 注释
  - [x] `initialize_test.go` 新增 `TestInitialize_ProfileUpdateDispatcherRegistered_*Mode` 双子测

- [x] **Task 8: 集成测试** (AC: #11)
  - [x] `cmd/cat/profile_update_integration_test.go` 6 子测（Testcontainers Mongo + Redis）
  - [x] `cmd/cat/quiet_hours_resolver_integration_test.go` 1 子测（真 Mongo + 真 clock 控制）
  - [x] `go vet -tags=integration ./...` 通过

- [x] **Task 9: §21.2 Empty→Real grep gate 自证** (AC: #7)
  - [x] `grep -cE "Empty[A-Za-z]+Provider\{\}" cmd/cat/initialize.go`：before → after Δ=-1（EmptyQuietHoursResolver 消失）
  - [x] `grep -cE "push\.Empty(QuietHoursResolver)\{\}" cmd/cat/initialize.go`：after=0

- [x] **Task 10: §21.8 self-audit + §21.3 fail matrix + build** (AC: #10, #14)
  - [x] Completion Notes List 9 条陷阱逐条标注已覆盖 / 未覆盖 + 证据
  - [x] `bash scripts/build.sh --test` 全绿
  - [x] Windows 下 race build 跳过（cgo 已知限制），Linux CI 必跑
  - [x] PR checklist §19 14 条逐条回答

---

## Dev Notes

### 本 story 为何重要（Epic 1 profile / 时区的基石 + QuietHoursResolver 最后一块拼图）

- **APNs 推送 quiet hours 合规落地**：PRD §用户安全"跨时区免打扰"是 MVP 强制要求；Epic 0 Story 0.13 只占了接口位 `QuietHoursResolver`，挂 `EmptyQuietHoursResolver`（恒 false = 永远 alert 不静音）。本 story 是**最后一个**APNs Empty Provider 的替换（Story 1.4 填了 3 个，本 story 填第 4 个 = 4/4 —— Story 0.13 的 4 个 APNs 消费接口全部 real 化）。未填实前，夜间 23:00-07:00 的触碰推送会响铃吵醒用户，PRD NFR-COMP 合规签不下去。
- **WS 首个"真业务写"RPC**：Epic 0 / Story 1.1-1.4 的 WS 路径只有 `session.resume`（读）+ `debug.echo*` + `room.join/action.update`（DebugOnly）。**本 story 上线 Epic 1 第一个 release-mode 的 WS 写类 RPC** `profile.update`，首次实战验证 Story 0.10 dedup middleware 在真实业务上的表现，首次跑通 `RegisterDedup` → envelope.id 幂等 → service 写 Mongo → 失效 resume cache 的完整链路。
- **FR50 时区自动上报的服务端端点**：客户端 iOS/watchOS 监听 `TimeZone.current` 变化 → 复用本 endpoint 自动推送新时区。服务端不区分"设置"还是"自动"，简化契约。
- **session.resume 的 user 字段**：Story 1.1 的 `RealUserProvider` 已经把 `displayName / timezone / preferences.quietHours` 投影到 session.resume 的 `user` 字段。本 story 一旦更新，必须失效 resume cache（60s TTL 自愈，但友好体验需立即生效）—— 把 `ResumeCacheInvalidator` 的首次业务消费者上线。

### 关键依赖与 Epic 0/1.1-1.4 资产复用

| 来源 | 资产 | Story 1.5 用法 |
|---|---|---|
| 0.5 | `logx.Ctx(ctx)` + `logx.WithUserID` + `logx.MaskPII` | audit log + M13 displayName 脱敏 |
| 0.6 | `ErrValidationError` / `ErrInternalError` / `ErrEventProcessing` | 复用全部；**不**新增错误码（§21.1 drift gate N/A） |
| 0.7 | `clockx.Clock` / `FakeClock` | service.UpdatedAt + QuietHoursResolver `clock.Now().In(loc)` + test |
| 0.10 | `Dispatcher.RegisterDedup` + `dedupMiddleware` | profile.update 上游幂等自动生效 |
| 0.12 | `ws.ResumeCacheInvalidator` + `redisx.RedisResumeCache.Invalidate` | service Update 成功后同步失效 |
| 0.13 | `push.QuietHoursResolver` interface | **本 story 填实 real impl，替换 EmptyQuietHoursResolver** |
| 0.14 | `WSMessages` + `validateRegistryConsistency` | profile.update 注册双 gate 自证 |
| 1.1 | `domain.User` + `DisplayName / Timezone / Preferences.QuietHours` 字段（已预留）+ `MongoUserRepository` | 字段首次业务写入；repo 扩 `UpdateProfile` |
| 1.1 | `dto.UserPublic / UserPublicFromDomain` | 扩展成 `UserPublicProfile`（嵌套 preferences.quietHours）保持向后兼容 |
| 1.1 | `ws.RealUserProvider` 投影 user.Preferences.QuietHours | 本 story 更新后失效 resume cache 让下次 resume 反映新值 |
| 1.2 | `users.sessions[deviceId]` struct | 本 story **不**动 sessions 字段（嵌套 dotted $set 纪律锁） |
| 1.3 | `middleware.JWTAuth` + `ws.JWTValidator` → `Client.UserID()` | handler 读 `client.UserID()`（WS 路径；HTTP 不走） |
| 1.4 | `repo.SetSessionHasApnsToken` 单次 UpdateOne 模式 | `UpdateProfile` 复用同款单 UpdateOne + dotted $set 模式；避免 TOCTOU |

### §21.1 常量 / 注册表漂移守门（本 story 四步走）

按 Epic 0 retro action #3，引入全局常量集合必须跟上双 gate；本 story 引入一条 WS 消息类型 `profile.update`：

1. **加常量**：`dto/ws_messages.go` `WSMessages` 追加条目
2. **在使用点注册**：`cmd/cat/initialize.go` `dispatcher.RegisterDedup("profile.update", ...)`
3. **更新文档**：`docs/api/ws-message-registry.md` 新增 section + `integration-mvp-client-guide.md` §15
4. **跑 CI**：`bash scripts/build.sh --test` —— 既有 `TestWSMessages_ConsistencyWithDispatcher_*` / `TestValidateRegistryConsistency_*` 零改动通过

额外自证：AC13 新增 `TestWSMessages_ProfileUpdate_Entry` 锁死本条目的 meta（DebugOnly=false / RequiresDedup=true / RequiresAuth=true / Direction=Bi）。

### §21.2 Empty→Real Provider 填实（Epic 1 收官 APNs 四件套）

Epic 0 Story 0.13 留下 4 个 APNs 消费接口的 Empty 占位。Epic 1 story 分布：

| Provider | Story 替换 | 状态 |
|---|---|---|
| `EmptyTokenProvider` | 1.4 | ✅ done |
| `EmptyTokenDeleter` | 1.4 | ✅ done |
| `EmptyTokenCleaner` | 1.4 | ✅ done |
| `EmptyQuietHoursResolver` | **1.5** | ⏳ **本 story** |

Epic 0 retro 明确 "1.5 (Quiet Hours)"。本 story 把最后一个 APNs Provider 填实 = **Epic 1 的 APNs 真实化完全收口**；`internal/push/providers.go` 的 4 个 Empty 类型将**全部**不再被 initialize.go wire —— 保留定义仅供测试 fake 使用。

session.resume 的 6 个 Provider 中 `UserProvider` 已 Story 1.1 填实，其余 5 个（Friends/CatState/Skins/Blindboxes/RoomSnapshot）待 Epic 2/3/4/6/7；本 story **不**动。

### §21.3 Fail-closed / Fail-open 完整声明

见 AC10 矩阵（六行）；此段是架构指南要求的**集中**声明点。

- **所有 profile 写路径**：**fail-closed**。Mongo 挂 → 5xx；validator 挂 → 400。绝无"降级为内存写"。
- **resume cache invalidate 失败**：**fail-open（吞 warn）**。cache 是性能层；60s TTL 自愈；主响应不因此失败。可观测：warn log `action="resume_cache_invalidate_error"`。
- **RealQuietHoursResolver 脏数据路径**（timezone 非法 / HH:MM 非法 / user missing）：**fail-open (false, nil)**。理由见 providers.go L83-90 既定合约"silenced-but-wanted 比 loud-but-should-be-silent 更糟"。可观测：warn log + 既有 APNsWorker fail-open 路径。
- **反用户 memory `feedback_no_backup_fallback.md`**：上述 fail-open 是**场景化选择**，不是"掩盖根因的 backup"——每一条失败都在日志里可见；resume cache 失败不会让 displayName 永久错，TTL 60s 自愈；脏 quietHours 不会永久不静音，用户下次更新即覆盖。

### §21.4 AC review 早启（本 story 符合标准）

Profile 是"用户可见的业务写 RPC"；同时触发：(1) 新全局常量（WS msg type）(2) 新 Empty→Real (3) 跨时区 / quiet-hours 语义复杂度。按 Epic 0 retro action #2，本类 AC 在实施前应做 AC review。推荐操作：dev session 开始前另开一个 Claude session，调用 `bmad-review-adversarial-general` 或 `bmad-review-edge-case-hunter` skill 喂本 story 文件做 AC 评审；特别关注 AC2（validator 边界）/ AC6（时区 / HH:MM 跨日 / start==end）/ AC14（Semantic-correctness 9 思考题）。

### 反模式 TL;DR 实施期自检（对应 `server/agent-experience/review-antipatterns.md`）

1. close(channel) — N/A（无 channel）
2. goroutine panic recover — N/A（handler 在 ws readPump goroutine；dispatcher 既有 recover 不在本 story 范围）
3. shutdown-sensitive I/O — handler / service / resolver 都透传 ctx；Mongo + Redis driver 自带 ctx honoring
4. **全局常量**：**有** 1 个新 WS msg type → §21.1 四步走（已在 AC1/AC13）
5. **新 config 字段**：**无**（不引入 config 字段——量化参数全复用 Story 0.x / 1.x）
6. **JWT**：**不**动 Verify；消费 Story 1.3 middleware 已注入的 `client.UserID()`
7. **debug/release mode gate**：`profile.update` 两模式都注册（DebugOnly=false），AC7 双模式启动测试 `TestInitialize_ProfileUpdateDispatcherRegistered_*Mode` 双测
8. **Redis key**：`resume_cache:{userId}` 既有 key，本 story 只 Invalidate（DEL）；不新增 key space
9. **rate limit**：N/A（WS 既有 per-conn rate limit Story 0.9 M15 + dedup Story 0.10 两层防护足够；profile.update 不引入专属限流，因业务期望 ≤ 1 次/小时/用户）
10. **度量 / 比率**：本 story 不加 metric；fail-open 路径全部 warn log 可观测
11. **中间件顺序**：N/A（WS 路径不经 gin middleware；dispatcher 内顺序既定）
12. **errors.Is/As**（M12）：service 用 `errors.Is(err, repository.ErrUserNotFound)` 分支，不用字符串比较
13. **分层违规（§13.1）**：`internal/push/real_quiet_hours_resolver.go` 声明本地 `userLookup` 接口，**不**反向 import `internal/repository`；service `internal/service → internal/ws` 合法方向；grep 自证见 AC7 Completion Notes
14. **度量 ratio（§14.x）**：N/A

### 关于 "displayName 记 enum 不记值" 的决策

- **选择**：audit log 记 `fields=["displayName", "timezone"]`（变更字段名枚举）；**不**记 displayName 的改前 / 改后值
- **理由**：
  1. displayName 是人格识别 PII（§M13 明确等同邮箱）
  2. 运维排错时：知道"用户 X 改了 displayName 和 timezone"足够追溯；需要改前 / 改后值时可读 Mongo 版本历史或 Change Stream（Epic 7+ 后上线）
  3. 记 enum 让日志文件泄漏或 ELK 误授权时 **不**暴露用户姓名
- **取舍**：audit log 能力略弱；代价是 PII 零泄漏。符合用户 memory `project_go_backend.md` / `feedback_no_backup_fallback.md` 的隐私原则（禁"为了便利而泄漏原值"）

### 关于 "resume cache invalidate fail-open" 的决策

- **选择**：service 写 Mongo 成功、Redis DEL 失败 → **主响应仍 OK** + warn log；客户端看到 profile 已更新
- **理由**：
  1. resume cache 是纯性能层（Story 0.12 AC4 明确）；TTL 60s，最长 60 秒后下次 session.resume 自动拿新 user
  2. 主响应失败会让客户端以为没写成功 → 触发重试 → 写入重复 / 用户困惑 → 但 Mongo 已落库 → 数据不一致
  3. warn log 有可观测（Story 0.12 既定模式，session.resume 自己 cache Get/Put 失败也是 warn + fall-through）
- **与"反 backup/fallback"的澄清**：这**不是**掩盖根因 —— Redis 连续挂掉会在 healthz + zerolog error 阶上暴露；是**性能层**的透明降级，不是"backup 藏着核心写入风险"

### 关于 "HH:MM 边界左闭右开" 的决策

- **选择**：quiet window `[start, end)`。`07:00` 的那一分钟**不**算 quiet（闹钟解除）；`23:00` 那一分钟开始 quiet
- **理由**：
  1. 符合闹钟 / 日历 UI 直觉："设 07:00 结束静默" = "07:00 响"
  2. 边界含闭不闭的分歧必须测试锁死（AC6 子测 `boundary_10:00_start_included` / `boundary_15:00_end_excluded`）；否则运维排查"为啥 07:00 还收不到推送"会找两天
- **取舍**：左闭右开与右闭左开差 1 分钟；选左闭右开因为更符合`start <= t < end` 标准数学区间

### 关于 "start == end 解释为永久静默" 的决策

- **选择**：`start == end` → 永远 quiet（返 true）
- **理由**：
  1. 用户拨两端相同通常是"勿扰模式永久开"；对 MVP 用户行为是合法操作
  2. 若禁止 `start == end`，validator 必须在 DTO 层拒绝；但"start == end 也被拒"会让 UI 必须知道这条业务规则，耦合度增大
  3. 替代："永不 quiet"（返 false）语义上等价于"没设 quiet hours"，不如让 `start == end` 表达"永久静默"来得有用
- **取舍**：若未来需要"禁 quiet"开关，添加 `preferences.quietHoursEnabled bool` 字段；本 MVP 不引入该开关

### Semantic-correctness 思考题（§21.8 / §19 第 14 条强制 · 9 条陷阱）

> **如果这段代码运行时产生了错误结果但没有 crash，谁会被误导？**
>
> **答**：所有 APNs 推送接收者（1.4 注册的 token） + `session.resume` 消费者（Watch 首屏 + Epic 3/4/6/7 未来好友投影） + FR50 自动时区上报链路。以下 9 个陷阱必须在 Completion Notes self-audit：
>
> 1. **时区 LoadLocation 用错参数 bug**：如果 `time.LoadLocation(*user.Timezone)` 后 resolver 不用 `.In(loc)` 而用 `clock.Now()` 直接 Hour() —— 就成了"按服务器 UTC 比较"，美东用户 23:00 本地但服务器 03:00 UTC → 比较 03:00 是否在 `[23:00, 07:00)` → false → 用户半夜收推送。AC6 的 `TestRealQuietHoursResolver_Timezone_NewYork_vs_Shanghai` 锁死 `.In(loc)` 被用。
>
> 2. **HH:MM 正则过松 bug**：如果正则写成 `^\d{1,2}:\d{1,2}$`，用户输入 `25:90` 通过 validator 但 Resolve 时 parse 成 hours=25 → hours*60+min 超出 1440 边界 → 比较逻辑下所有时刻都落在 `[1500, 1440)`（跨日窗口误判为同日）→ 结果：无论真实时刻几点都**not** quiet，永远响铃。AC2 的 `^([01]\d|2[0-3]):[0-5]\d$` 锁死 00:00-23:59 范围 + `TestValidateProfileUpdateRequest_HHMM_InvalidRanges` 6 子测。
>
> 3. **跨日窗口比较方向错 bug**：如果 resolver 写 `if start > end { return nowMin >= start && nowMin < end }`（即 AND 不是 OR），跨日窗口（23:00-07:00）永远 false —— 因为没有一个时刻 `>= 23:00 AND < 07:00`；用户半夜 03:00 收到触碰推送响铃。AC6 的 `TestRealQuietHoursResolver_CrossMidnight_*` 3 子测（23:00 / 03:00 / 07:00）锁死 OR 逻辑。
>
> 4. **`start == end` 分支遗漏 bug**：如果代码只有 `start < end` / `start > end` 两分支（`if/else`），`start == end` 落到 `start > end` 分支 → `nowMin >= start OR nowMin < end` → `nowMin >= X OR nowMin < X` → 第一条 `nowMin >= X` 在任一刻都为 true（假设 start = 22:00：22:00 起都 true；11:00 false）→ 半数时间错判为 quiet。AC2 / AC6 的 `TestRealQuietHoursResolver_StartEqualsEnd_AlwaysQuiet` 显式锁死 `start == end → true`。
>
> 5. **resume cache 失效漏一次 bug**：如果 service.Update 只在三字段**全**非 nil 时才 Invalidate（错写成 `if p.DisplayName != nil && p.Timezone != nil && p.QuietHours != nil`），单字段更新后 resume cache 保留旧值 60s；好友 Watch 在 60s 内做 session.resume 看到改前 displayName / timezone / quietHours。AC4 的 `TestProfileService_Update_InvalidatesCacheOnAnyFieldChange` 锁死"单字段也 invalidate"。
>
> 6. **displayName 日志泄漏 bug**：如果 audit log 写 `Str("displayName", *p.DisplayName)` 而不是 `Strs("fields", ["displayName"])`，log 聚合平台（Loki / Splunk）会持久化 displayName 原文；跑路员工或被攻击者拉 log 可反查用户姓名。M13 + NFR-COMP 合规红线。AC9 的 `TestProfileService_Update_DoesNotLogDisplayNameValue` 显式扫 buffer 禁断言"Alice" 不出现。
>
> 7. **嵌套 $set 变成整块替换 bug**：如果 repo.UpdateProfile 写 `setDoc["preferences"] = bson.M{"quiet_hours": bson.M{"start": ..., "end": ...}}` 而不是 `"preferences.quiet_hours.start": ...` 两条，未来 Epic 1.6 若在 preferences 下加 `deletionGraceDays` 或 Epic 3 加 `friendVisibilityRules` —— profile.update 会把那些字段整块清零。AC3 的 `TestMongoUserRepo_Integration_UpdateProfile_PreservesSessionsAndFriendCount` + dotted 字段路径锁死。
>
> 8. **displayName trim 遗漏 bug**：如果 validator 只 `len(trim(s)) >= 1` 但 $set 写未 trim 的 s，用户输入 `"   "` → len after trim = 0 被拒；但输入 `" Alice "` → len after trim = 5 通过，落库 `" Alice "`（前后空格）→ 好友看到带空格的奇怪名字。AC5 的 `toDomainPartial` 明确 trim after validate（handler 层；AC2 validator 只判"trim 后是否空"）。service/repo 层拿到的是已 trim 值。
>
> 9. **dedup middleware 忘记挂 bug**：如果 dispatcher 用 `Register` 而不是 `RegisterDedup` 注册 profile.update，客户端 retry 同一 envelope.id 会做两次 UpdateOne + 两次 Invalidate —— 数据库压力 + log 重复 + 客户端迷惑（第二次可能看到 _id 不匹配类的 err，但实际 $set 是幂等的所以大概率静默不显）。AC1 明确 `RegisterDedup`；AC13 `TestWSMessages_ProfileUpdate_Entry` 断言 `RequiresDedup == true`；AC11 集成测试 `ReplayEventIDReturnsCachedResponse` 端到端验证。
>
> **Dev agent 实施完成后在 `Completion Notes List` 里明确写一段"以上 9 个陷阱哪些已被 AC/测试覆盖"的 self-audit；任一条答"未覆盖"必须立即补测试或修代码。**

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.5 Profile 与偏好设置 — line 849-872]
- [Source: _bmad-output/planning-artifacts/prd.md#FR48 修改 displayName — line 829]
- [Source: _bmad-output/planning-artifacts/prd.md#FR49 修改 timezone + 免打扰 — line 830]
- [Source: _bmad-output/planning-artifacts/prd.md#FR50 客户端主动上报时区变更 — line 831]
- [Source: _bmad-output/planning-artifacts/prd.md#跨时区免打扰 — line 350]
- [Source: _bmad-output/planning-artifacts/prd.md#profile.update 消息类型 — line 500]
- [Source: _bmad-output/planning-artifacts/architecture.md#§P1 MongoDB 规范 — line 499-505]
- [Source: _bmad-output/planning-artifacts/architecture.md#§P3 WebSocket Message 命名 — line 527-534]
- [Source: _bmad-output/planning-artifacts/architecture.md#§P7 Request DTO 校验 — line 588-593]
- [Source: _bmad-output/planning-artifacts/architecture.md#§M13 PII 日志规则 — line 677-681]
- [Source: _bmad-output/planning-artifacts/architecture.md#§M9 Clock interface — line 647]
- [Source: docs/backend-architecture-guide.md#§6.1-§6.4 Handler/Service/Domain/Repository 分层]
- [Source: docs/backend-architecture-guide.md#§10.2 EnsureIndexes]
- [Source: docs/backend-architecture-guide.md#§13 AuthRequired — /v1/* 与 WS 挂载]
- [Source: docs/backend-architecture-guide.md#§21.1 双 gate 漂移守门]
- [Source: docs/backend-architecture-guide.md#§21.2 Empty Provider 逐步填实]
- [Source: docs/backend-architecture-guide.md#§21.3 Fail-closed vs Fail-open]
- [Source: docs/backend-architecture-guide.md#§21.4 AC review 早启]
- [Source: docs/backend-architecture-guide.md#§21.7 Server 测试自包含]
- [Source: docs/backend-architecture-guide.md#§21.8 语义正确性思考题]
- [Source: server/agent-experience/review-antipatterns.md#TL;DR 自检清单]
- [Source: server/agent-experience/review-antipatterns.md#§4.1 positive int / §4.2 applyDefaults]
- [Source: server/agent-experience/review-antipatterns.md#§7.1 release/debug mode gate]
- [Source: server/agent-experience/review-antipatterns.md#§8.1 key namespace]
- [Source: server/agent-experience/review-antipatterns.md#§13.1 pkg/ ← internal/ 禁止]
- [Source: server/internal/domain/user.go#User.DisplayName/Timezone/Preferences.QuietHours — 字段已预留（Story 1.1 seed）]
- [Source: server/internal/domain/user.go#DefaultPreferences — 23:00-07:00 默认]
- [Source: server/internal/repository/user_repo.go#SetSessionHasApnsToken — Story 1.4 单次 UpdateOne + dotted $set 模板]
- [Source: server/internal/ws/session_resume.go#ResumeCacheInvalidator — Story 0.12 既定接口]
- [Source: server/internal/ws/dispatcher.go#RegisterDedup — Story 0.10 消费入口]
- [Source: server/internal/ws/room_mvp.go#validateField / validationError — Story 10.1 helper 模板]
- [Source: server/internal/ws/user_provider.go#RealUserProvider — Story 1.1 session.resume user 字段 provider（本 story 成功后被 invalidate）]
- [Source: server/internal/push/providers.go#QuietHoursResolver + EmptyQuietHoursResolver — Story 0.13 合约 + 待替换]
- [Source: server/internal/push/apns_worker.go#L267-275 quiet Resolve 调用点]
- [Source: server/internal/dto/auth_dto.go#UserPublic / UserPublicFromDomain — Story 1.1 JSON 契约（本 story 扩展向后兼容）]
- [Source: server/internal/dto/ws_messages.go#WSMessages — Story 0.14 注册表]
- [Source: server/internal/dto/error_codes.go#ErrValidationError / ErrInternalError / ErrEventProcessing — 复用既有]
- [Source: server/internal/middleware/jwt_auth.go#UserIDFrom / DeviceIDFrom — Story 1.3（HTTP 路径；本 story WS 路径用 client.UserID()）]
- [Source: server/cmd/cat/initialize.go#L108 push.EmptyQuietHoursResolver{} → 待替换]
- [Source: server/cmd/cat/initialize.go#L169-178 session.resume providers wiring]
- [Source: server/cmd/cat/initialize.go#validateRegistryConsistency — Story 0.14 运行时 gate]
- [Source: server/pkg/logx/pii.go#MaskPII — §M13 helper（profile service audit log 不记 displayName 原值由测试断言）]
- [Source: server/pkg/clockx — Clock/FakeClock 既有，本 story 复用]
- [Source: server/pkg/redisx/resume_cache.go#Invalidate — 既有，本 story 首个业务消费者]
- [Source: docs/api/openapi.yaml — 当前 version 1.4.0-epic1；本 story bump 1.5.0-epic1]
- [Source: docs/api/ws-message-registry.md — 本 story 追加 profile.update section]
- [Source: docs/api/integration-mvp-client-guide.md — 本 story 追加 §15 profile/timezone 上报]
- [Source: _bmad-output/implementation-artifacts/epic-0-retro-2026-04-19.md#§5.1 预留项 / 8.2 action #4 Empty Provider 注释]
- [Source: _bmad-output/implementation-artifacts/1-1-user-domain-sign-in-with-apple-jwt.md#domain.User 字段布局]
- [Source: _bmad-output/implementation-artifacts/1-4-apns-device-token-registration-endpoint.md#AC7 SetSessionHasApnsToken 单 UpdateOne 模板 + §21.2 Empty→Real 替换套路]
- [Source: _bmad-output/implementation-artifacts/0-10-ws-upstream-eventid-idempotent-dedup.md#RegisterDedup 合约]
- [Source: _bmad-output/implementation-artifacts/0-12-session-resume-cache-throttle.md#ResumeCacheInvalidator + fail-open]
- [Source: _bmad-output/implementation-artifacts/0-13-apns-push-platform-pusher-queue-routing-410-cleanup.md#QuietHoursResolver interface + fail-open 合约]
- [Source: _bmad-output/implementation-artifacts/0-14-ws-message-type-registry-and-version-query.md#WSMessages 双 gate 自证]

### Project Structure Notes

- 完全对齐架构指南 `internal/{domain, repository, service, handler, middleware, push, ws, dto}` + `pkg/{logx, clockx, redisx, ids}` 分层；**无新增 pkg 目录**
- **新建文件（11 个）**：
  - `server/internal/dto/profile_dto.go`
  - `server/internal/dto/profile_dto_test.go`
  - `server/internal/service/profile_service.go`
  - `server/internal/service/profile_service_test.go`
  - `server/internal/ws/profile_handler.go`
  - `server/internal/ws/profile_handler_test.go`
  - `server/internal/push/real_quiet_hours_resolver.go`
  - `server/internal/push/real_quiet_hours_resolver_test.go`
  - `server/cmd/cat/profile_update_integration_test.go`
  - `server/cmd/cat/quiet_hours_resolver_integration_test.go`
  - （可选）若 service 单独依赖接口定义多，可单独 `server/internal/service/profile_service_interfaces.go` —— 但参考 Story 1.4 风格放在 profile_service.go 顶部即可
- **修改文件（7 个）**：
  - `server/internal/dto/ws_messages.go`（追加 profile.update entry）
  - `server/internal/dto/ws_messages_test.go`（TestWSMessages_ProfileUpdate_Entry 自证）
  - `server/internal/repository/user_repo.go`（+ UpdateProfile + ProfileUpdate 结构）
  - `server/internal/repository/user_repo_integration_test.go`（+ 6 子测）
  - `server/cmd/cat/initialize.go`（profile wiring + EmptyQuietHoursResolver 替换 + `// removed via Story 1.5` 注释）
  - `server/cmd/cat/initialize_test.go`（+ 双模式 TestInitialize_ProfileUpdateDispatcherRegistered）
  - `docs/api/openapi.yaml`（version bump 1.4.0-epic1 → 1.5.0-epic1 + 如有 WSRegistry 示例追加 profile.update）
  - `docs/api/ws-message-registry.md`（追加 ### profile.update section）
  - `docs/api/integration-mvp-client-guide.md`（追加 §15 Profile 与时区上报）
- 无新 external dependency（一切 stdlib `time.LoadLocation` + `regexp` / `unicode/utf8` + 既有 go-redis + mongo + gin + zerolog）
- 无架构偏差
- 无新 WSMessage type 字段集合 / error code 集合；**有** 1 个 WS msg type 新增 → §21.1 drift gate 走四步走；**有** 1 个 Empty Provider 被真实化 → §21.2 显式执行
- 无新 config 字段

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] via Claude Code

### Debug Log References

- 单元测试全绿：`go test -count=1 ./internal/dto/ ./internal/repository/ ./internal/service/ ./internal/ws/ ./internal/push/ ./cmd/cat/`
- `go vet -tags=integration ./...` 无输出 → 集成测试编译 clean，等待 Linux CI Docker 执行真跑
- `internal/middleware` pre-existing flaky test `TestRequestID_InjectsIntoContextLogger` 5 次中偶发 1-2 次 FAIL，已在 HEAD 前验证同样 flaky（未修改 middleware 代码），不归本 story 负责 — 详见下文 Known Flakes

### Completion Notes List

#### 架构与分层决策

1. **Import cycle 破解**：`internal/push` 不能 import `internal/repository`（后者已经 import `internal/push` 以拿 `push.TokenInfo`）。`RealQuietHoursResolver` 的 `quietHoursUserLookup` 接口采用 `(user, found, err)` 形状（而不是基于 `errors.Is(err, repository.ErrUserNotFound)` 的错误匹配），配合 `cmd/cat/wire_profile.go` 的 `quietHoursUserLookupAdapter` 把 `repository.ErrUserNotFound → found=false, err=nil`。这让 push 层对 repo 无感，保住了 §13.1 依赖方向纪律。
2. **Profile handler → service import 方向**：`internal/ws` 不能 import `internal/service`（§M8 / review-antipatterns §13.1）。`ws.ProfileHandler` 通过本地定义的 `profileUpdater` 接口 + `ws.ProfileUpdateInput` 类型与 service 解耦；`cmd/cat/wire_profile.go` 的 `profileServiceHandlerAdapter` 在 wiring 层做一次性字段拷贝。
3. **userRepo 构造顺序前移**：把 `userRepo` 的构造从 Story 1.1 区块移到 APNs worker wiring 之前（新独立区块，EnsureIndexes 幂等无 side effect），使 `RealQuietHoursResolver` 能在 APNs worker 构造时被直接 wire 进去。

#### §21.2 Empty→Real grep gate 自证

| Pattern | Before (HEAD^) | After (HEAD) | Δ | 说明 |
|---|---|---|---|---|
| `Empty[A-Za-z]+(Provider\|Resolver\|Cleaner\|Deleter)\{\}` in initialize.go | 6 | 5 | **-1** ✓ | `push.EmptyQuietHoursResolver{}` 消失；5 个 session.resume Empty 保留（Epic 2/3/4/6/7 替换） |
| `push\.EmptyQuietHoursResolver\{\}` in initialize.go | 1 | **0** ✓ | -1 | 完全替换为 `realQuietHoursResolver` |

Epic 0 Story 0.13 的 4 个 APNs Empty Provider 已**全部**替换为 Real：
- `EmptyTokenProvider` → Story 1.4 ✅
- `EmptyTokenDeleter` → Story 1.4 ✅
- `EmptyTokenCleaner` → Story 1.4 ✅
- `EmptyQuietHoursResolver` → **Story 1.5（本 story）** ✅

#### §21.1 常量 / 注册表漂移守门双 gate 自证

1. `dto/ws_messages.go` 新增 `profile.update` 条目（`RequiresDedup=true / RequiresAuth=true / DebugOnly=false / Direction=Bi`）
2. `cmd/cat/initialize.go` 调 `dispatcher.RegisterDedup("profile.update", profileHandler.HandleUpdate)`（release & debug 双模式）
3. `docs/api/ws-message-registry.md` 追加 `### profile.update (bi, v1, auth required)` section；`docs/api/integration-mvp-client-guide.md` 追加 §15（原 §15 账户注销预告挪到 §16）
4. Zero-assertion-modification tests 全部通过：
   - `TestWSMessages_ConsistencyWithDispatcher_ReleaseMode/DebugMode`
   - `TestValidateRegistryConsistency_ReleaseMode/DebugModeFullyRegistered`
   - 新增 `TestWSMessages_ProfileUpdate_Entry` 锁死 meta 四字段
   - 新增 `TestInitialize_ProfileUpdateDispatcherRegistered_ReleaseMode/DebugMode` 双模式注册断言

#### §21.3 Fail-closed / Fail-open 矩阵自审

| 场景 | 策略 | 实际实现 | 可观测信号 |
|---|---|---|---|
| `userRepo.UpdateProfile` Mongo 写失败 | fail-closed | `INTERNAL_ERROR`（非 `ErrUserNotFound` 被 `fmt.Errorf` wrap 再返） | handler zerolog error 带 `code=INTERNAL_ERROR` |
| `ResumeCacheInvalidator.Invalidate` Redis 失败 | fail-open | 主响应仍 OK + `action="resume_cache_invalidate_error"` warn log；60s TTL 自愈 | warn log 里带 userId + err + 注释"cache will self-heal in 60s" |
| `RealQuietHoursResolver` Mongo FindByID 失败（非 NotFound）| fail-open (err 上抛) | 返 `(false, err)`；APNsWorker warn 后投 alert | err 上抛，APNsWorker 现有 fail-open 路径捕获 |
| `RealQuietHoursResolver` user missing | fail-open (false, nil) | 适配器把 `repository.ErrUserNotFound → found=false`；resolver 返 `(false, nil)` | 无 warn（missing user 是合法业务状态，不记 log 噪声） |
| `RealQuietHoursResolver` timezone 非 IANA | fail-open (false, nil) | warn log `action="quiet_hours_bad_timezone"` | warn log 带 userId + timezone |
| `RealQuietHoursResolver` HH:MM 非法 | fail-open (false, nil) | warn log `action="quiet_hours_bad_quiet_hours"` | warn log 带 userId + start/end |
| handler / service validation 失败 | fail-closed | 返 `VALIDATION_ERROR`（400 类；WS error envelope） | 错误码清晰，`payload` 值**不**入 log（PII 合规） |
| Dispatcher dedup middleware | 继承 Story 0.10 | 继承 | 同 Story 0.10 |

#### §21.8 Semantic-correctness 9 条思考题 self-audit

| # | 陷阱 | 被哪个 AC / 测试锁定 | 状态 |
|---|---|---|---|
| 1 | 时区 LoadLocation 后不 `.In(loc)` | `TestRealQuietHoursResolver_Timezone_NewYork_vs_Shanghai`（同 UTC 时刻不同结论锁死 `.In(loc)`）+ 集成测试 `TestQuietHoursResolver_Integration_EndToEnd` 两阶段 clock advance | ✅ covered |
| 2 | HH:MM 正则过松 | `dto/profile_dto.go` 正则 `^([01]\d\|2[0-3]):[0-5]\d$`；`TestValidateProfileUpdateRequest_QuietHours_InvalidFormat` 8 子测含 `24:00 / 25:90 / 23:5 / ab:cd / 23:60 / 7:00 / 空` | ✅ covered |
| 3 | 跨日窗口 AND 不是 OR | `TestRealQuietHoursResolver_Overnight_MidOfNight`（03:00 必须 quiet）+ `_Overnight_StartIncluded`（23:00 必须 quiet）+ `_Overnight_EndBoundaryExcluded`（07:00 必须 NOT quiet）三子测；`isQuiet` 函数实现注释明确 OR | ✅ covered |
| 4 | `start == end` 分支遗漏 | `isQuiet` 顶部显式 `if startMin == endMin { return true }`；`TestRealQuietHoursResolver_StartEqualsEnd_AlwaysQuiet` 4 个不同时刻循环断言 always-quiet | ✅ covered |
| 5 | resume cache 失效遗漏单字段路径 | `TestProfileService_Update_InvalidatesCacheOnAnyFieldChange` 3 子测（displayName-only / timezone-only / quietHours-only 各验 `inv.calls == 1`） | ✅ covered |
| 6 | displayName log 泄漏 | `TestProfileService_Update_DoesNotLogDisplayNameValue` 扫 `Alice / 王小明 / Bob / 🐈Suki` 4 个值确保不出现；服务代码用 `Strs("fields", [...])` 而非 `Str("displayName", value)` | ✅ covered |
| 7 | 嵌套 $set 变成整块替换 | `repository/user_repo.go` UpdateProfile 使用 `preferences.quiet_hours.start/end` dotted set；`TestMongoUserRepo_Integration_UpdateProfile_PreservesSessionsAndFriendCount` 验证 update quietHours 后 `sessions / friend_count / consents.step_data` 全部保留 | ✅ covered（integration） |
| 8 | displayName trim 遗漏 | Handler 层 `toHandlerInput` 做 trim after validate；`TestProfileHandler_DisplayNameTrimmedBeforeService` 锁 "  Alice  " → service 收到 "Alice"；集成测试 `TestProfileUpdate_Integration_Partial_DisplayNameOnly` 验证 "  Bob  " 落 Mongo 是 "Bob" | ✅ covered |
| 9 | dedup middleware 忘记挂 | initialize.go 用 `RegisterDedup` 而非 `Register`；`TestWSMessages_ProfileUpdate_Entry` 断言 `RequiresDedup=true`；集成测试 `TestProfileUpdate_Integration_ReplayEventIDReturnsCachedResponse` 端到端验证 replay 不双写 | ✅ covered |

**结论**：9 条陷阱全部有 AC 或测试锁定，无遗漏。

#### §19 PR Checklist 14 条（由背景知识记忆 `review-antipatterns.md` 提炼）

1. **close(channel)**：N/A — 本 story 不引入 channel
2. **goroutine panic recover**：N/A — handler 在既有 readPump goroutine，dispatcher recover 既有
3. **shutdown-sensitive I/O**：handler / service / resolver 全路径传 `ctx`；Mongo + Redis driver 自带 ctx 尊重
4. **全局常量**：`profile.update` 新 WS msg type → §21.1 四步走全落地（见上）
5. **新 config 字段**：无 — 未引入任何 `cfg.*` 字段
6. **JWT**：无改动 — 复用 Story 1.3 `client.UserID()`
7. **debug/release mode gate**：`RequiresDedup=true, DebugOnly=false` → 双模式都注册，初始化测试双模式覆盖
8. **Redis key**：无新 key — 只复用 `resume_cache:{userId}` (DEL) 与 dedup (Story 0.10)
9. **rate limit**：N/A — 既有 per-conn + dedup 两层防护充足；profile.update 不引入专属限流（业务期望 ≤ 1 次/hr/user）
10. **度量 / 比率**：N/A — 本 story 不加 metric，fail-open 路径全部 warn log
11. **中间件顺序**：N/A — WS 路径不经 gin middleware
12. **errors.Is/As (§M12)**：service 层用 `errors.Is(err, repository.ErrUserNotFound)`，不用字符串比较
13. **分层违规 (§13.1)**：
    - `internal/push/real_quiet_hours_resolver.go` 声明**本地** `quietHoursUserLookup` 接口，**不**反向 import `internal/repository`（cycle 破解见"架构决策 #1"）
    - `internal/ws/profile_handler.go` 声明**本地** `profileUpdater` 接口 + `ProfileUpdateInput` 类型，**不** import `internal/service`
    - `internal/service/profile_service.go` → `internal/ws.ResumeCacheInvalidator` 方向合法
14. **度量 ratio (§14.x)**：N/A

#### 反模式 TL;DR 实施期自检

| # | 反模式 | 本 story 处理 |
|---|---|---|
| §4.1/4.2 positive int / applyDefaults | N/A（无新 cfg） | — |
| §7.1 release/debug mode gate | DebugOnly=false 两模式都注册；`TestInitialize_ProfileUpdateDispatcherRegistered_*Mode` 双测 | ✅ |
| §8.1 key namespace | 复用既有 `resume_cache:` key space | ✅ |
| §8.2 path injection | `preferences.quiet_hours.start/end` 字段名是代码常量，非用户输入，不受 injection 影响 | ✅ |
| §9.1 sliding window | N/A（未加限流） | — |
| §13.1 pkg/ ← internal/ 禁止 | N/A（未改 pkg/） | — |
| service → ws 方向 | 合法（§M8） | ✅ |
| push → repository 禁止 | 本地接口 + adapter 破除 cycle | ✅ |

#### Known Flakes（非本 story 负责）

- `internal/middleware/TestRequestID_InjectsIntoContextLogger`：`t.Parallel()` + 全局 `log.Logger` 突变组合，与同包并行 tests 竞争；5 次运行偶发 1-2 次 FAIL。在 HEAD（未引入本 story 的代码）上同样 flaky。不属于本 story 回归。

### File List

**新建（11 个）：**
- `server/internal/dto/profile_dto.go`
- `server/internal/dto/profile_dto_test.go`
- `server/internal/service/profile_service.go`
- `server/internal/service/profile_service_test.go`
- `server/internal/ws/profile_handler.go`
- `server/internal/ws/profile_handler_test.go`
- `server/internal/push/real_quiet_hours_resolver.go`
- `server/internal/push/real_quiet_hours_resolver_test.go`
- `server/cmd/cat/wire_profile.go`（两个 wiring 适配器：`quietHoursUserLookupAdapter` + `profileServiceHandlerAdapter`）
- `server/cmd/cat/profile_update_integration_test.go`（6 子测，build tag integration）
- `server/cmd/cat/quiet_hours_resolver_integration_test.go`（1 子测，build tag integration）

**修改（9 个）：**
- `server/internal/dto/ws_messages.go`（追加 `profile.update` 条目）
- `server/internal/dto/ws_messages_test.go`（AC13 `TestWSMessages_ProfileUpdate_Entry` + 双模式 consistency 测试加 profile.update RegisterDedup）
- `server/internal/repository/user_repo.go`（新增 `ProfileUpdate` 结构 + `UpdateProfile` 方法）
- `server/internal/repository/user_repo_integration_test.go`（追加 6 子测）
- `server/cmd/cat/initialize.go`（userRepo 构造前移 + `realQuietHoursResolver` 替换 `EmptyQuietHoursResolver` + profile 服务/handler wiring + RegisterDedup("profile.update")）
- `server/cmd/cat/initialize_test.go`（release/debug mode drift 测试追加 profile.update；新增 `TestInitialize_ProfileUpdateDispatcherRegistered_*Mode` 双模式注册断言）
- `docs/api/ws-message-registry.md`（追加 `### profile.update (bi, v1, auth required)` section）
- `docs/api/openapi.yaml`（version bump `1.4.0-epic1 → 1.5.0-epic1`）
- `docs/api/integration-mvp-client-guide.md`（新增 §15 Profile 更新与时区自动上报；原 §15 账户注销预告挪到 §16）

### Change Log

| Date | Version | Change |
|---|---|---|
| 2026-04-19 | 1.5.0-epic1 | Story 1.5 implementation (profile.update WS RPC + RealQuietHoursResolver + UpdateProfile repo method + §21.2 Empty→Real 4/4 APNs Provider 收口) |
