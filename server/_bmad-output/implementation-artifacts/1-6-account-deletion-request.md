# Story 1.6: 账户注销请求

Status: review

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a signed-in user,
I want to request account deletion and have my sessions terminated immediately,
so that I can stop using the app while the server handles cascade cleanup per MVP compliance (FR47, NFR-COMP-5 MVP 30 天内人工处理).

## Acceptance Criteria

**Given** Story 1.1 `users` collection（`domain.User.DeletionRequested bool` + `DeletionRequestedAt *time.Time` 字段已预留）+ Story 1.2 `AuthService.RevokeAllUserTokens(ctx, userID) error` + Story 1.3 JWT middleware (`middleware.UserIDFrom(c)` 注入) + Story 1.3 `ws.Hub.DisconnectUser(userID UserID) (int, error)` + Story 0.12 `ws.ResumeCacheInvalidator`（`pkg/redisx.RedisResumeCache.Invalidate` 已实现）+ 架构指南 §14 路由约定（`/v1/*` 受 JWT 保护、动作挂子路径）+ §21.3 fail-closed vs fail-open 决策框架。

**1. 端点选择与路由挂载（§14 路由约定 + 客户端指南 §16 预告对齐）**

**Given** epics.md L883 要求"HTTP DELETE `/v1/users/me`（或 POST `/v1/users/me/deletion-request`；二选一在实现时定稿）"；`docs/api/integration-mvp-client-guide.md` §16 预告已用 `DELETE /v1/users/me`；架构指南 §14 资源复数 + RESTful 动词语义
**When** 选定最终端点
**Then** **锁定 `DELETE /v1/users/me`**（不是 POST /deletion-request），理由：①REST 动词对齐"请求删除此资源"②客户端指南 §16 已广播该 URL 给前端；改 URL 会拖累联调③动作类子路径（`POST /v1/users/me/skins/:id/equip`）与本 endpoint 语义不同——本 endpoint 是资源级动作而非 sub-resource 操作
**And** 路由注册在 `cmd/cat/wire.go` 的 `v1 := r.Group("/v1")` 下：`v1.DELETE("/users/me", h.user.RequestDeletion)`（新增 `h.user *handler.UserHandler` 字段到 `handlers` 结构；infra 模式同 `h.device` Story 1.4）
**And** endpoint 由 `/v1/*` 组强制 JWT 鉴权（release mode）；debug mode `jwtAuth=nil` 时 handler 自己复用 Story 1.4 的"UserIDFrom 空即 401"防御纵深 pattern（`dto.ErrUnauthorized`）
**And** debug mode 下本 endpoint **不**挂任何 debug-only 特殊处理——本 story 属 release 业务，双模式都注册（类似 1.4/1.5；DebugOnly 概念仅用于 WS message，HTTP endpoint 无此概念）

**2. Handler 层（`internal/handler/user_handler.go`）**

**Given** 架构指南 §6.1 handler 唯一职责是 HTTP↔service 翻译；§P7 DTO 校验；Story 1.4 `device_handler.go` 的"UserIDFrom 空即返 AppError"防御纵深模板
**When** 新建 `internal/handler/user_handler.go`
**Then** 定义 `UserHandlerService interface { RequestAccountDeletion(ctx context.Context, userID ids.UserID) (*service.AccountDeletionResult, error) }`（消费者接口在 handler 包内，service 包实现）
**And** `UserHandler` 结构体依赖 `svc UserHandlerService`；构造 `NewUserHandler(svc) *UserHandler` nil panic（§P3）
**And** `RequestDeletion(c *gin.Context)`：
  - `uid := middleware.UserIDFrom(c)`；uid == "" → `dto.RespondAppError(c, dto.ErrUnauthorized)`（release mode 该路径理论不可达因 jwtAuth 先挂；debug mode 或 middleware 漏挂时的防御纵深，与 Story 1.4 device_handler 同模式）
  - 调 `h.svc.RequestAccountDeletion(c.Request.Context(), uid)`；err → `dto.RespondAppError(c, err)`
  - 成功 → `c.JSON(http.StatusAccepted, dto.AccountDeletionResponse{Status: "deletion_requested", RequestedAt: result.RequestedAt.UTC().Format(time.RFC3339), Note: "30 days manual cleanup per MVP policy"})`
**And** **不**接受请求 body（DELETE 语义无 body；`c.ShouldBindJSON` 不调）
**And** 单元测试 `user_handler_test.go` ≥ 6 子测（fake `UserHandlerService`）：
  - happy path → 202 + 正确 JSON shape（status/requestedAt/note 三字段）
  - service 返 `dto.ErrUserNotFound` → 404 + 正确 code（repo 脏数据场景；见 AC4 决策）
  - service 返 `dto.ErrInternalError` → 500 + INTERNAL_ERROR
  - UserIDFrom 空 → 401 UNAUTHORIZED（防御纵深）
  - RequestedAt 序列化为 RFC3339 UTC 字符串（不是 epoch / 不是本地 tz）
  - 响应 Content-Type 是 `application/json; charset=utf-8`（Gin 默认即可，断言即可）

**3. DTO（`internal/dto/account_deletion_dto.go`）**

**Given** §P1 HTTP JSON snake_case；§P7 DTO 层定义 response
**When** 新建 `internal/dto/account_deletion_dto.go`
**Then** 定义 `AccountDeletionResponse{Status string json:"status"; RequestedAt string json:"requested_at"; Note string json:"note"}`（纯 response，无 request DTO —— body 为空）
**And** `Status` 值锁为常量 `"deletion_requested"`（字符串字面量；无新全局常量集合，§21.1 不触发——单一常量不构成"集合"，无双 gate 必要）
**And** `Note` 值锁为常量 `"30 days manual cleanup per MVP policy"`（MVP 文案；epic L888）
**And** `RequestedAt` 是 `string` 而非 `time.Time` —— 显式 RFC3339 UTC 序列化避免 JSON decoder 在客户端产生时区解析歧义；handler 层调用 `.UTC().Format(time.RFC3339)` 生成
**And** 单元测试 `account_deletion_dto_test.go` ≥ 3 子测：JSON marshal shape 断言（所有字段 snake_case）+ 空值断言 + 长 Note 不被截断

**4. Service 层（`internal/service/account_deletion_service.go`）**

**Given** 架构指南 §6.2 service 负责业务组合 / 事务 / 外部副作用；消费者接口在 service 包内定义（P2 风格）；§21.3 fail-closed vs fail-open 必须显式选择并声明
**When** 新建 `internal/service/account_deletion_service.go`
**Then** 定义消费者接口（本 service 所需能力）：
  - `accountDeletionUserRepo interface { MarkDeletionRequested(ctx context.Context, userID ids.UserID) (*domain.User, error) }` —— 见 AC5
  - `accountDeletionTokenRevoker interface { RevokeAllUserTokens(ctx context.Context, userID ids.UserID) error }` —— `*AuthService` 满足
  - `accountDeletionSessionDisconnector interface { DisconnectUser(userID ws.UserID) (int, error) }` —— `*ws.Hub` 满足；注意 `ws.UserID` 与 `ids.UserID` 通过 `ws.UserID(string(userID))` 转换（见 `internal/ws/hub.go` L27 定义）
  - `accountDeletionCacheInvalidator interface { Invalidate(ctx context.Context, userID string) error }` —— `*redisx.RedisResumeCache` 满足；**复用** `ws.ResumeCacheInvalidator` 签名（不再定义新接口；方向 `internal/service → internal/ws` 合法）
**And** 实际 wiring：`service.AccountDeletionService` 接 `userRepo *repository.MongoUserRepository / authSvc *AuthService / hub *ws.Hub / resumeCache *redisx.RedisResumeCache`；四依赖 nil 任一 panic（§P3）
**And** 核心方法 `RequestAccountDeletion(ctx, userID ids.UserID) (*AccountDeletionResult, error)` 流程**按严格顺序**执行（见下），且实现"首次请求 stamp，幂等重复请求 return 原 timestamp"语义：
  1. **Step 1 — Mark deletion（fail-closed）**：调 `userRepo.MarkDeletionRequested(ctx, userID)`；返 `*domain.User`（反映提交后状态，含已写入或既有的 `DeletionRequested / DeletionRequestedAt`）；
     - `repository.ErrUserNotFound` → 透传成 `dto.ErrUserNotFound`（handler 层 404）—— 正常运行路径该分支不可达（JWT 签的 userId 必有 user 文档），出现即数据损坏 / token 来自幽灵用户；404 **允许**暴露"此 userId 不存在"因为 JWT 已能证明自己是那个用户（不是跨用户信息泄漏）
     - 其他 err → 包 `dto.ErrInternalError.WithCause(err)`（**fail-closed**：Mongo 写失败不能让后续 side-effect 继续跑——审计链路要求"要么全成功要么全回滚"）
  2. **Step 2 — Revoke tokens（fail-open with warn，§21.3 判据：token 已延迟失效不代表必须阻塞主流程）**：调 `authSvc.RevokeAllUserTokens(ctx, userID)`；
     - err → `logx.Ctx(ctx).Warn().Err(err).Str("action", "account_deletion_revoke_partial").Str("userId", string(userID)).Msg(...)`；**不** abort；30 天内 ops 跑 process_deletion_queue 时会再次扫并显式吊销（次重试兜底）
     - 理由：已经 mark deletion + 即将 disconnect WS，refresh token 即使短期有效也无法生产新 access（/auth/refresh 需要走 rotation + blacklist 路径，而 access token max TTL 15min 短，等 15min 自然失效）
  3. **Step 3 — Disconnect WS（fail-open with warn）**：调 `hub.DisconnectUser(ws.UserID(string(userID)))`；
     - err → warn log `action="account_deletion_disconnect_error"`；**不** abort
     - 理由：Hub 当前实现无 err 路径（始终返 nil，见 `hub.go` L166-168）；为未来扩展预留 fail-open；WS 即使没断开也不会产生新写入（profile.update 走 JWT 注入的 userID；deletion_requested=true 后的读取链路由 Growth 阶段再逐 story fail-closed）
  4. **Step 4 — Invalidate resume cache（fail-open with warn）**：调 `resumeCache.Invalidate(ctx, string(userID))`；
     - err → warn log `action="account_deletion_resume_invalidate_error"`；**不** abort
     - 理由：cache 60s TTL 自愈；主响应成功后客户端已下线，60s 内没有 session.resume 请求来消费旧 cache
  5. **Step 5 — Audit log（§NFR-SEC-10 合规必写）**：
     - `logx.Ctx(ctx).Info().Str("action", "account_deletion_request").Str("userId", string(userID)).Time("requestedAt", result.RequestedAt).Bool("wasAlreadyRequested", !firstTimeRequest).Msg("account_deletion_request")`
     - **不**记 displayName / timezone / email / sub / appleUserIdHash（§M13 PII + NFR-SEC-6——deletion audit 只需 userId + 时间戳）
  6. **Return**：`&AccountDeletionResult{RequestedAt: user.DeletionRequestedAt, WasAlreadyRequested: bool}`；handler 做 RFC3339 格式化
**And** 定义 `AccountDeletionResult{RequestedAt time.Time; WasAlreadyRequested bool}` —— `WasAlreadyRequested` 供审计日志用，**不**暴露给客户端（响应 shape 对两种情况一致 —— 幂等）
**And** Step 1 的 MarkDeletionRequested 本身必须 TOCTOU-safe（见 AC5），service 层**不**做"先 Read 再 Write"的分支分叉
**And** 单元测试 `account_deletion_service_test.go` ≥ 12 子测（fake 四依赖 + `clockx.FakeClock`）：
  - happy path firstTime → 所有 4 step 各调一次 + result.WasAlreadyRequested=false
  - happy path alreadyRequested → MarkDeletionRequested 返已有 timestamp + WasAlreadyRequested=true + 其余 3 step 仍调（再跑一遍吊销 + 断连 + invalidate 是幂等安全操作）
  - Step 1 返 ErrUserNotFound → 透传 ErrUserNotFound + Step 2/3/4 **不**调（fail-closed 断链）
  - Step 1 返 generic err → 包 ErrInternalError + Step 2/3/4 **不**调
  - Step 2 返 err → 主流程继续到 Step 3/4 + warn log action=account_deletion_revoke_partial
  - Step 3 返 err → 主流程继续到 Step 4 + warn log action=account_deletion_disconnect_error
  - Step 4 返 err → 主响应仍 OK + warn log action=account_deletion_resume_invalidate_error
  - 审计日志 action="account_deletion_request" 必出现
  - 审计日志 **不**含任何 displayName 测试值（buffer 扫"Alice"/"王小明"不出现）
  - 严格顺序断言：用 calls-ordered slice 记录 fake 调用顺序，断言是 `mark → revoke → disconnect → invalidate` 而非 interleaved
  - ws.Hub.DisconnectUser 的 userId 参数类型转换正确（fake 捕获的 string 与 userID 相等）
  - audit log 的 `wasAlreadyRequested` 字段随场景正确切换

**5. Repository 扩展（`internal/repository/user_repo.go` + service.go 接口追加）**

**Given** `user_repo.go` 既有 `ClearDeletion`（Story 1.1，反向操作）+ `UpdateProfile`（Story 1.5 单 UpdateOne + dotted $set 模板）+ `ErrUserNotFound` sentinel；架构指南 §10.4 schema 演化强调单次 UpdateOne
**When** 追加 `MarkDeletionRequested` 到 `MongoUserRepository`
**Then** 签名 `func (r *MongoUserRepository) MarkDeletionRequested(ctx context.Context, id ids.UserID) (*domain.User, error)`
**And** 实现语义锁：**首次请求写 timestamp；幂等重复请求保留原 timestamp**（见 AC4 Step 1 for rationale）
**And** 实现：**两阶段 atomic idempotent**（单次 `FindOneAndUpdate` 无法表达条件性 $set —— 要么用 aggregation pipeline update，要么分两步；选后者，可读性强）：
  1. 调 `r.coll.FindOneAndUpdate(ctx, filter=bson.M{"_id": string(id), "deletion_requested": bson.M{"$ne": true}}, $set={"deletion_requested": true, "deletion_requested_at": now, "updated_at": now}, ReturnDocument: After)`
  2. 若返 `mongo.ErrNoDocuments` → 进入"fallback read"：调 `FindByID(ctx, id)` 读既有文档
     - 返 ErrUserNotFound → 是 ErrUserNotFound 透传（真的没这个 user）
     - 返 *domain.User 且 `DeletionRequested == true` → 返 &user, nil（已请求过，幂等成功）
     - 返 *domain.User 且 `DeletionRequested == false` → 这种情况理论不可达（如果 user 存在且 deletion_requested=false，第一步的 UpdateOne 必匹配 filter）；防御：返 fmt.Errorf("user repo: mark deletion: impossible state")包起来 ErrInternalError 让运维看到数据损坏
  3. 其他 mongo err → `fmt.Errorf("user repo: mark deletion requested: %w", err)`
**And** **不**用 aggregation pipeline update（`$cond` in `$set`）—— Mongo driver v2 语法繁琐、CI 可读性弱、单次 UpdateOne-Then-FindByID 两查对小规模 MVP 代价可接受
**And** 单元测试不写 repo（Story 1.5 同模式；repo 依赖 mongo，走 integration）
**And** 集成测试 `user_repo_integration_test.go` 追加 `TestMongoUserRepo_Integration_MarkDeletionRequested` 6 子测：
  - `FirstTimeStampsRequestedAt`：初始 `deletion_requested=false` → 写后 `deletion_requested=true` + `deletion_requested_at == Clock.Now` + `updated_at == Clock.Now`
  - `IdempotentPreservesOriginalTimestamp`：预置 `deletion_requested=true, deletion_requested_at=t0`；advance clock 到 t1；再调 → 返回 user 的 timestamp **仍是 t0**（MVP 关键不变量：重复调用不延长 30 天宽限期）
  - `UserNotFound`：返 `ErrUserNotFound`
  - `PreservesSessions`：mark 前先 seed `users.sessions[d1]={current_jti, issued_at}`；mark 后 sessions sub-doc **完整保留**（dotted $set 不清零 sessions）
  - `PreservesFriendCountAndConsents`：seed `friend_count=3, consents.step_data=true`；mark 后两字段保留
  - `UpdatesUpdatedAtEvenOnIdempotent`：wait —— 幂等路径第二阶段是纯读，不写 `updated_at`；锁死这条语义：**幂等重复请求 _不更新_ updated_at**（对齐"保留原 timestamp"）。子测断言：第二次 mark 后的 `updated_at == t0`（首次 mark 时的 clock）

**6. AuthService 接口扩展（最小增量 —— 已有 `RevokeAllUserTokens`）**

**Given** Story 1.2 已实现 `AuthService.RevokeAllUserTokens(ctx, userID) error`（internal/service/auth_service.go L720）
**When** 本 story 消费
**Then** `account_deletion_service.go` 声明本地消费者接口 `accountDeletionTokenRevoker { RevokeAllUserTokens(ctx, ids.UserID) error }`；`*service.AuthService` 方法集已满足，**零**代码改动
**And** wiring 层在 `cmd/cat/initialize.go` 注入已构造的 `authSvc` 到 `accountDeletionSvc`（见 AC9）
**And** **不**在 AuthService 加任何新方法——`RevokeRefreshToken(userID, deviceID)` 由 Story 1.2 提供但本 story 用 `RevokeAllUserTokens`（epic L885）

**7. Hub 依赖（最小增量 —— 已有 `DisconnectUser`）**

**Given** Story 1.3 已实现 `Hub.DisconnectUser(userID ws.UserID) (int, error)`（internal/ws/hub.go L166）
**When** 本 story 消费
**Then** service 层声明本地接口 `accountDeletionSessionDisconnector { DisconnectUser(userID ws.UserID) (int, error) }`；`*ws.Hub` 方法集已满足
**And** **import 方向合规**：`internal/service → internal/ws` 合法（Story 1.5 已奠基）；service 包本身可直接 import `ws` 获取 `ws.UserID` 类型别名
**And** service 层显式做 `ws.UserID(string(userID))` 类型转换（`ws.UserID = string` 别名底层兼容，但显式转换提示类型边界；见 `internal/ws/hub.go` 类型定义）

**8. ResumeCacheInvalidator 复用（§21.2 纪律：不新增接口）**

**Given** Story 0.12 已定义 `ws.ResumeCacheInvalidator{ Invalidate(ctx, userID string) error }`；`*redisx.RedisResumeCache` 满足；Story 1.5 首个业务消费者
**When** 本 story 消费
**Then** 直接复用 Story 0.12 接口签名（本地别名 `accountDeletionCacheInvalidator`）；**不**定义新接口
**And** wiring 复用 Story 1.5 已装配的 `resumeCache *redisx.RedisResumeCache` 实例

**9. Wiring（`cmd/cat/initialize.go`）**

**Given** Story 1.5 已在 initialize.go 装配 `userRepo / authSvc / resumeCache / hub`；`handlers` 结构体在 `cmd/cat/wire.go` L17-29
**When** 本 story 装配
**Then** 在 `initialize.go` 构造 `accountDeletionSvc := service.NewAccountDeletionService(userRepo, authSvc, hub, resumeCache)`（依赖顺序清晰：user 写 / token 吊销 / ws 断 / cache 失效）
**And** 构造 `userHandler := handler.NewUserHandler(accountDeletionSvc)`（类型满足 `handler.UserHandlerService`）
**And** 把 `user` 字段加到 `handlers` 结构体：`user *handler.UserHandler`；在 `buildRouter` 里 `if h.user != nil { v1.DELETE("/users/me", h.user.RequestDeletion) }`（与 Story 1.4 `h.device` 同风格；未装配时路由不挂保证单测 router 不强依赖该字段）
**And** **release & debug mode 都注册**（DELETE 是业务端点，无 debug-only 概念；当 debug mode `jwtAuth=nil` 时，UserHandler 的 UserIDFrom-空-即-401 防御纵深接住）
**And** 无新 global 常量 / 新 config 字段 —— §21.1 drift gate N/A；§21.2 Empty→Real N/A（无新 Provider）

**10. 集成测试（§21.7 server 测试自包含）**

**Given** Story 1.1-1.5 integration harness（Testcontainers Mongo + Redis + httptest server + JWT issuance helpers）
**When** 新建 `cmd/cat/account_deletion_integration_test.go` `//go:build integration`
**Then** 复用 `setupJWTAuthHarness(t)` 或抽 `setupAccountDeletionHarness(t)`：Testcontainers Mongo + Redis + in-proc HTTP + WS hub
**And** ≥ 5 子测：
  - `HappyPath_AllStepsExecuted`：seed user + WS client + refresh token；DELETE /v1/users/me → 202 + `status="deletion_requested"` + `requested_at` 非空；断言：(a) Mongo users._id 的 `deletion_requested=true` + `deletion_requested_at` 非零；(b) Redis refresh blacklist 含该 device 的 jti；(c) WS 连接收到 Close 1000 "revoked" 帧；(d) Redis `resume_cache:{userId}` 不存在
  - `Idempotent_SecondCall_PreservesFirstTimestamp`：先调一次记录 t0；等 ≥1s；再调 DELETE → 202 + `requested_at == t0`（**不是** 新时间戳）
  - `MissingAuthToken_Returns401`：不带 Bearer → 401（JWTAuth middleware 先拦；跟 Story 1.3 jwt_middleware_integration_test.go 同模式）
  - `NoWSConnection_StillSucceeds`：用户未连 WS，DELETE 仍 202 + Mongo 字段正确（DisconnectUser 返 count=0，fail-open）
  - `MultipleDevices_AllDisconnected`：先建两条 WS 连接（两个 deviceId）+ 两个 refresh token；DELETE → 两条连接都收 Close + 两个 jti 都进 blacklist
**And** 每个子测在 finally 清理 Redis+Mongo（避免跨 test pollution；复用 harness teardown）
**And** `go vet -tags=integration ./...` 编译通过（Windows 下 Docker 不可用时 vet 仍绿；Linux CI 跑真用例）

**11. openapi.yaml 与客户端指南更新（文档漂移 gate）**

**Given** `docs/api/openapi.yaml` 当前 `version: "1.5.1-epic1"`（Story 1.5 已 bump）；`docs/api/integration-mvp-client-guide.md` §16 已有 Story 1.6 预告
**When** 本 story 上线
**Then** `openapi.yaml` bump 到 `"1.6.0-epic1"`；新增 `/v1/users/me` path 的 `delete` operation：
  - `summary: "Request account deletion (FR47, MVP marks only)"`
  - `security: [{BearerAuth: []}]`（默认继承但显式写出更清晰）
  - `responses: 202: { schema: AccountDeletionResponse }, 401: AuthError, 404: UserNotFound, 500: InternalError`
  - 定义 `AccountDeletionResponse` 组件 schema（三字段 status/requested_at/note）
**And** `docs/api/integration-mvp-client-guide.md` §16 从"预告"升级为"正式"（改标题 `### 16. 账户注销（Story 1.6）`）：
  - 保留现有幂等行为说明（"第二次调用返回原 timestamp"需加一段）
  - 增"端点契约"表（method/path/headers/body-空/response shape）
  - 增"客户端实施要点"：①DELETE body 不需要；②收 202 后立刻清 Keychain；③WS close 帧已有的 §16 行为保留；④**幂等安全**——客户端重试不会延长宽限期，运维体感是"first-write-wins"
  - 增 errors 表：401 / 404（稀有；仅在 JWT 签的 userId 不存在 Mongo 时）/ 500
**And** 不动 ws-message-registry.md（本 story 不引入 WS 消息）

**12. tools/process_deletion_queue 一次性清理脚本（§21.5 CLI 上线判据）**

**Given** epic L891 要求本 story 交付 `tools/process_deletion_queue/main.go`（一次性脚本，30 天后运维手动执行）；§21.5 CLI 必须"仅开发期使用" OR "监控 + runbook + alert 三件套"
**When** 本 story 交付该脚本
**Then** **选定"生产 ops 手动执行 + runbook"路径**（非"仅开发期"——脚本确实要在生产跑），按 §21.5 模式实施：
  - 新建 `server/tools/process_deletion_queue/main.go`：
    - 注释头 `// Command process_deletion_queue ... Production ops use only; see docs/runbook/process_deletion_queue.md for usage and preflight checklist.`
    - 执行前打印 banner `"!!! This will PERMANENTLY DELETE user data from Mongo + Redis. Type CONFIRM to proceed:"` + 等待 stdin 读取 `"CONFIRM"`（case-sensitive）；其他输入 → print abort + exit 1
    - 参数：`-config path`（标准 TOML）+ `-dry-run`（默认 false，true 时只打印候选不写）+ `-older-than-days`（默认 30）+ `-limit`（默认 100；单次执行上限避免意外 full-sweep）
    - 核心逻辑（go run 单 goroutine，无并发）：
      1. 构造 Mongo / Redis clients（复用 `pkg/mongox` + `pkg/redisx`）
      2. 扫 `users` collection `find({deletion_requested: true, deletion_requested_at: {$lt: Clock.Now - older-than-days*24h}})` 限 `-limit`
      3. 对每条 user：
         - delete `apns_tokens` 所有 `user_id = userID` 文档（跨两 platform）
         - `users.DeleteOne({_id: userID})`（真正删除）
         - 清理 refresh blacklist 中与该 user 相关的 jti —— ⚠️ 现有 blacklist key 设计按 jti 而非 userId 索引（见 `pkg/redisx/refresh_blacklist.go`），无法枚举；**遵循既定合约**：blacklist 内 TTL 自动过期 30 天（与 refresh TTL 对齐）—— 无需 explicit cleanup
         - 未来 Epic 2/3/6/7 会追加：`cat_states / friendships / blindbox_drops / skin_ownership` —— 预留一个 `TODO(Epic N.X)` 块，当前 story 不实现（collection 不存在）
      4. 统计 summary JSON 输出到 stdout：`{deletedUsers: N, deletedApnsTokens: M, durationMs, dryRun, olderThanDays}`；stderr 仅错误
  - 新建 `docs/runbook/process_deletion_queue.md`（§21.5 三件套之 runbook 文档）：
    - 何时跑（cron 计划 or 手动触发条件）
    - 跑前 preflight（备份 Mongo 快照 / 确认无其他写入 in-flight）
    - 跑中监控（命令示例 `go run ./tools/process_deletion_queue -dry-run=true | jq .`）
    - 失败 rollback 说明（Mongo point-in-time recovery 见 NFR-REL-7）
    - 联系人（DBA / backend lead）
  - **不**加 alert / metric（MVP 阶段单实例手动脚本，没有 metric 系统消费）；runbook 里显式标注"MVP: no automated alerting; run under two-eyes review"
**And** 单元测试 `tools/process_deletion_queue/main_test.go` ≥ 4 子测（用 miniredis + 内存 Mongo testcontainer 或 mock）：
  - `DryRun_DoesNotWrite`：seed 2 expired user → dry-run → summary 显示 2 候选但 Mongo 仍有两 user
  - `DeletesExpiredUsers`：seed 2 expired + 1 within-grace → real run → 两个老 user 被删、一个保留
  - `RespectsLimit`：seed 5 expired + `-limit=2` → 只删两个
  - `RequiresConfirmInput`：stdin 送 "WRONG" → exit non-zero + no writes
**And** **grep gate 自证**：`ls server/tools/` → 出现新目录 `process_deletion_queue`；main.go 首行注释含 "Production ops use only"

**13. §21.3 Fail-closed / Fail-open 矩阵（强制声明）**

| 场景 | 策略 | 故障时行为 | 可观测信号 |
|---|---|---|---|
| `userRepo.MarkDeletionRequested` Mongo 写失败 | **fail-closed** | 返 `ErrInternalError` 给客户端（500）；Step 2/3/4 **不**执行 | zerolog error + handler 返 500 | 
| `userRepo.MarkDeletionRequested` ErrUserNotFound | **fail-closed (404)** | 透传 `ErrUserNotFound` → 404 | zerolog error + 仅当 token/user 数据损坏时触发 |
| `AuthService.RevokeAllUserTokens` 失败 | **fail-open (warn)** | Step 1 已成功；继续 Step 3/4；warn log | `action="account_deletion_revoke_partial"` warn log；30 天后 process_deletion_queue 再扫 + refresh TTL 自然失效 |
| `ws.Hub.DisconnectUser` 失败（现行实现始终 nil；未来扩展预留）| **fail-open (warn)** | 继续 Step 4 | `action="account_deletion_disconnect_error"` warn log |
| `resumeCache.Invalidate` 失败 | **fail-open (warn)** | 主响应仍 202 | `action="account_deletion_resume_invalidate_error"` warn log；60s TTL 自愈 |
| JWT middleware 拒绝（token 缺失 / 签名错）| **fail-closed (401)** | 由 middleware 返 401 | Story 1.3 既有路径 |
| 幂等第二次调用 | **always success (202)** | 不 re-stamp timestamp；依 repo 单实现 | `wasAlreadyRequested=true` 出现在 audit log |

**And** 反 `feedback_no_backup_fallback.md` 澄清：上述 fail-open 每一条**都可观测**——不是"藏着 backup 掩盖根因"；用户 memory 反对的是"静默 fallback"；本 story 所有 fail-open 都有 warn log + 兜底机制（process_deletion_queue / refresh blacklist TTL）

**14. §21.1 常量 / 注册表漂移守门自证（本 story 不触发，显式说明）**

**Given** §21.1 仅在"引入新**全局常量集合**"时触发双 gate
**When** 本 story 是否需要 drift gate
**Then** **不触发**——仅引入两个本地 string literal：
  - `"deletion_requested"` 作为 response status 值（AccountDeletionResponse.Status）
  - `"30 days manual cleanup per MVP policy"` 作为 response note 值
**And** 两者都是**本 story 专属 response payload 的字段值**，不构成"集合"（不像 WSMessages 或 ErrorCodes 那种跨 story 枚举集合），无需双 gate
**And** 新增 `action="account_deletion_request"` / `action="account_deletion_revoke_partial"` / `action="account_deletion_disconnect_error"` / `action="account_deletion_resume_invalidate_error"` 四条 log action string 也不构成常量集合（audit log action 字符串分散在全 repo，无 drift 风险）
**And** 如果未来要统一 log action string 集合，补双 gate 是横向重构任务，不归本 story

**15. §21.8 Semantic-correctness 思考题（§19 PR checklist 第 14 条强制 · 10 条陷阱）**

实施前需在 Dev Notes Semantic-correctness 段落回答：本 story 代码运行错误结果但没 crash，**谁会被误导**？

> **答**：NFR-COMP-5 审计方（30 天内人工处理合规者）+ `RealUserProvider` 消费者（Story 1.1 session.resume user 投影）+ process_deletion_queue ops（cascade cleanup 的真相源）+ 被注销用户本人（期望设备断开）+ 好友 / 触碰发送方（Epic 3/5 接收侧，当用户 deletion_requested=true 时应该 fail-closed 不再能 receive touch）。

10 条陷阱必在 Completion Notes self-audit：

> 1. **幂等第二次调用 re-stamp timestamp bug**：如果 MarkDeletionRequested 的 filter 漏了 `deletion_requested: {$ne: true}`，第二次调用会把 timestamp 刷新成 "now"，**延长 30 天宽限期**。合规审计者看到的是"用户 A 2026-04-19 请求删除"；但 2026-04-25 用户再 DELETE 一次 → 2026-05-25 才过宽限；再 DELETE 一次 → 再延 30 天 → **永远删不掉**。这破坏 NFR-COMP-5 SLA 且让恶意持续 DELETE 成为"刷新宽限"的隐蔽 DoS。AC5 `TestMongoUserRepo_Integration_MarkDeletionRequested_IdempotentPreservesOriginalTimestamp` 锁 $ne 语义。
>
> 2. **Step 顺序颠倒 bug**：如果 service 写 `DisconnectUser → Revoke → MarkDeletionRequested`，DisconnectUser 成功 + Revoke 成功 + MarkDeletionRequested 失败（Mongo 抖动）→ 用户被踢下线 + token 被吊销 + **但数据库无任何删除痕迹** → 用户重连 → /auth/refresh 被黑名单拒绝 → SIWA → 匹配既有 user（无 deletion_requested）→ 新签 token → ??? 状态：老 token 吊销、新 token 正常、数据库无 deletion 标记。合规审计者找不到 deletion 记录；用户困惑为何被踢但仍能登录。AC4 strict-order 单元测试（`calls-ordered slice`）锁死 mark → revoke → disconnect → invalidate 且 Step 1 失败后不 invoke 2/3/4。
>
> 3. **fail-closed vs fail-open 反了 bug**：如果 Step 2 (Revoke) 变成 fail-closed（err → abort），Redis 抖动 → 写 Mongo deletion_requested=true 成功 + Revoke 失败 → 整体返 5xx → 客户端重试 DELETE → Mongo 已标记 → 幂等返 202 无 error 上报。但用户的 refresh token **永远没被吊销**（因为第一次失败 + 第二次因幂等路径走"already requested"分支跳过 Revoke）。反之：如果 Step 1 (Mark) 变成 fail-open，Mongo 写失败 → 仍调 Revoke + Disconnect → token 被吊销、连接被关，但**数据库无 deletion_requested 记录**。AC4 矩阵锁死 Step 1 fail-closed、Step 2/3/4 fail-open；AC5 `IdempotentPreservesOriginalTimestamp` 的变体确保幂等第二次调用**仍**调 Step 2/3/4（不能因"已 deletion_requested"跳过兜底操作）。
>
> 4. **Response requestedAt 序列化成非 UTC bug**：如果 handler 返 `result.RequestedAt.Format(time.RFC3339)` 而不是 `.UTC().Format(time.RFC3339)`，服务器所在机器时区（e.g. Asia/Shanghai）会在 RFC3339 字符串里出现 `+08:00` offset；不同服务器 / 容器产生的字符串不一致 → 客户端显示时间、日志聚合排序都会错位。AC2 `TestUserHandler_Request_RequestedAtIsUTC` 断言响应里的 timestamp 字符串以 `Z` 结尾（UTC 标识）。
>
> 5. **幂等路径漏掉 audit log bug**：如果 service 在"已 deletion_requested"路径 early-return 跳过 audit log，合规审计者只能看到首次请求的记录；真实人工审计场景下，如果用户在客服电话里说"我 5 天前请求了注销"，ops 需要从 audit log 看到"deletion_request on 2026-04-19" + "deletion_request (re-submit) on 2026-04-24"两条。AC4 audit log 的 `wasAlreadyRequested=true` 字段锁死"幂等也要记 audit"。
>
> 6. **ClearDeletion 反向操作没覆盖 bug**：epic L890 明确 SIWA 登录时 deletion_requested=true → ClearDeletion 复活（Story 1.1 已实现）。本 story **不**动 ClearDeletion 的复活行为。风险：如果 Story 1.1 的复活逻辑被测试跳过 / bug，用户 DELETE /v1/users/me → 30 天内 SIWA 登录 → 应"复活"但失败 → 用户以为账号被删（体验炸）。**本 story 不修复该风险，但 AC10 `HappyPath_AllStepsExecuted` 集成测试后**增加一条 `ResurrectionAfterDeletion_WorksE2E` 子测 → 先 DELETE 再 SIWA 再断言 deletion_requested=false（锁死 Story 1.1 的复活不被本 story 无意破坏）。
>
> 7. **DELETE with body not rejected bug**：如果 handler 调 `c.ShouldBindJSON(&req)` 而 DELETE 不应该有 body，恶意客户端发 body `{foo: "bar"}` 可能使 Gin 返 400 （schema 拒）或吞掉 body（behavior depends）。REST 语义上 DELETE 允许 body 但不被鼓励。AC2 明确**不**做 body 解析；AC2 单元测试加一条 `TestUserHandler_DeleteWithBody_StillSucceeds`（body 非空但不影响 handler 调 service 的参数）锁死 handler 忽略 body。
>
> 8. **WS close code 错 bug**：Hub.DisconnectUser 既有实现用 `websocket.CloseNormalClosure (1000) + "revoked"`（见 hub.go L187-189）。如果改用 1008 (PolicyViolation) 或 4000+ 自定义 code，客户端 §16 文档说"不要当成网络异常重连"的假设会破裂——客户端可能重连循环。本 story **不**改 Hub.DisconnectUser 内部逻辑；AC10 `HappyPath_AllStepsExecuted` 集成测试断言客户端收到 CloseError.Code == 1000 + Text == "revoked"。
>
> 9. **process_deletion_queue 无 CONFIRM 守门 bug**：如果 tool 直接执行不等 stdin，运维误操作 `go run ./tools/process_deletion_queue -older-than-days=0` → 全库扫过所有 deletion_requested=true 即删 → 还没过 30 天的被误删。AC12 `RequiresConfirmInput` 子测锁"stdin != 'CONFIRM' → abort + no writes"；banner 文案 `!!! This will PERMANENTLY DELETE ...` 对齐 §21.5 + runbook preflight。
>
> 10. **process_deletion_queue cleanup 漏 apns_tokens bug**：如果 tool 只 `users.DeleteOne` 不碰 apns_tokens，被删 user 的 APNs token 仍留 Mongo → Epic 8 cold-start recall 扫 apns_tokens 发 push → 用户已注销还收推送 → Apple Guidelines（NFR-COMP-3）违规。AC12 `DeletesExpiredUsers` 断言 apns_tokens 相关文档也被清。未来 Epic 2/3/6/7 加各自 cascade 时应遵循同模式（`TODO(Epic N.X)` 块已 scaffold）。

实施后在 Completion Notes List 逐条 self-audit 覆盖状态；未覆盖必须补测试或修代码。

---

## Tasks / Subtasks

- [x] **Task 1: DTO 层** (AC: #3)
  - [x] 新建 `internal/dto/account_deletion_dto.go`：`AccountDeletionResponse` 三字段 + snake_case tag
  - [x] 单测 `account_deletion_dto_test.go` ≥ 3 子测（shape / RFC3339 UTC / 常量 note 文案）

- [x] **Task 2: Repository 扩展** (AC: #5)
  - [x] 在 `user_repo.go` 追加 `MarkDeletionRequested(ctx, userID) (*domain.User, error)`
  - [x] 首次请求写 timestamp；幂等第二次调用不 re-stamp（filter `deletion_requested: {$ne: true}` + ErrNoDocuments fallback FindByID）
  - [x] 集成测试 `user_repo_integration_test.go` 追加 `TestMongoUserRepo_Integration_MarkDeletionRequested` 6 子测（含 IdempotentPreservesOriginalTimestamp / PreservesSessions / PreservesFriendCountAndConsents / UpdatesUpdatedAtEvenOnIdempotent）

- [x] **Task 3: Service 层（跨仓库组合 + 严格顺序 + fail matrix）** (AC: #4, #6, #7, #8, #13)
  - [x] 新建 `internal/service/account_deletion_service.go`
  - [x] 四消费者接口本地定义（userRepo / tokenRevoker / sessionDisconnector / cacheInvalidator）
  - [x] `RequestAccountDeletion` 按严格顺序 mark → revoke → disconnect → invalidate
  - [x] Step 1 fail-closed；Step 2/3/4 fail-open with warn
  - [x] `AccountDeletionResult` 含 RequestedAt + WasAlreadyRequested
  - [x] audit log action="account_deletion_request" 含 wasAlreadyRequested；**不**记 PII
  - [x] 单测 `account_deletion_service_test.go` ≥ 12 子测（含 strict-order slice 断言 + PII non-leak + fail matrix 各行）

- [x] **Task 4: Handler 层** (AC: #2)
  - [x] 新建 `internal/handler/user_handler.go`：`UserHandler{svc UserHandlerService}`
  - [x] `RequestDeletion` 读 UserIDFrom → svc → 202 AccountDeletionResponse
  - [x] UserIDFrom 空 → 401（防御纵深，Story 1.4 device_handler 同模式）
  - [x] RequestedAt UTC + RFC3339 序列化
  - [x] 单测 `user_handler_test.go` ≥ 6 子测（含 `TestUserHandler_Request_RequestedAtIsUTC` + `TestUserHandler_DeleteWithBody_StillSucceeds`）

- [x] **Task 5: 路由挂载 + 双模式注册** (AC: #1, #9)
  - [x] `handlers` 结构体加 `user *handler.UserHandler` 字段
  - [x] `buildRouter` 内 `v1.DELETE("/users/me", h.user.RequestDeletion)` 条件挂载
  - [x] initialize.go 构造 `accountDeletionSvc / userHandler` 并注入 `handlers.user`
  - [x] 路由单测 `wire_test.go` 或 `router_test.go`（如有）加断言：v1 下有 DELETE /users/me

- [x] **Task 6: Wiring (initialize.go)** (AC: #9)
  - [x] `accountDeletionSvc := service.NewAccountDeletionService(userRepo, authSvc, hub, resumeCache)`
  - [x] `userHandler := handler.NewUserHandler(accountDeletionSvc)`
  - [x] 注入到 `handlers{user: userHandler}`
  - [x] release & debug 双模式都挂

- [x] **Task 7: 集成测试** (AC: #10)
  - [x] 新建 `cmd/cat/account_deletion_integration_test.go` `//go:build integration`
  - [x] 复用 `setupJWTAuthHarness` 或新建 `setupAccountDeletionHarness`
  - [x] 5 子测：HappyPath / Idempotent_PreservesFirstTimestamp / MissingAuthToken / NoWSConnection / MultipleDevices
  - [x] 追加 `ResurrectionAfterDeletion_WorksE2E`（AC15 陷阱 #6 的集成锁）
  - [x] `go vet -tags=integration ./...` 通过

- [x] **Task 8: openapi.yaml + 客户端指南** (AC: #11)
  - [x] `docs/api/openapi.yaml` bump `1.5.1-epic1 → 1.6.0-epic1`；追加 `DELETE /v1/users/me` path + `AccountDeletionResponse` schema
  - [x] `docs/api/integration-mvp-client-guide.md` §16 从预告升级为正式 + 幂等语义段 + 错误码表

- [x] **Task 9: tools/process_deletion_queue CLI + runbook** (AC: #12)
  - [x] 新建 `server/tools/process_deletion_queue/main.go` + `run.go` 结构（main_test.go 驱动 run() 可测）
  - [x] CONFIRM stdin prompt + `-dry-run` / `-older-than-days` / `-limit` flags
  - [x] cascade 当前仅 `apns_tokens` + `users`；其他 collection `TODO(Epic N.X)` 预留
  - [x] 新建 `docs/runbook/process_deletion_queue.md`：preflight / 执行步骤 / rollback / MVP no-alert 说明
  - [x] 单测 `main_test.go` ≥ 4 子测（DryRun / DeletesExpired / RespectsLimit / RequiresConfirm）

- [x] **Task 10: §21.8 self-audit + §21.3 matrix + build** (AC: #13, #15)
  - [x] Completion Notes List 10 条陷阱逐条标注已覆盖 + 证据
  - [x] §21.3 fail matrix 自审（矩阵每行与实际代码一致）
  - [x] `bash scripts/build.sh --test` 全绿
  - [x] §19 PR checklist 14 条逐条回答

---

## Dev Notes

### 本 story 为何重要（Epic 1 合规收口 + FR47 MVP 落地）

- **NFR-COMP-5 合规兑现**：PRD NFR-COMP-5 明确 MVP "30 天内人工处理"；本 story 是 MVP 唯一满足该条款的路径。未落地前，Apple Privacy Policy 要求的"允许用户请求账号删除"在 App Store 审核会被拒（MVP 发版门槛）。
- **Epic 1 六件套收官**：身份与账户 epic 的六个 story（1.1 SIWA / 1.2 refresh / 1.3 JWT middleware / 1.4 APNs token / 1.5 profile / 1.6 deletion）本 story 为最后一块。完成后 Epic 1 entries 在 sprint-status.yaml 全 `done`，可转 Epic 2（猫状态）。
- **首个"跨三层副作用组合"的 service 路径**：此前的 service（AuthService / ProfileService / ApnsTokenService）各自职责单一；本 service 首次组合 userRepo + authSvc + hub + resumeCache 四依赖，且顺序与 fail 策略决定合规与体验。严格顺序 + fail-closed/fail-open 矩阵是本 story 的核心设计产物。
- **process_deletion_queue CLI 与 runbook 首次落地**：Epic 0 的 `blacklist_user / ws_loadgen` 都是开发期工具（§21.5 "仅开发期使用"）；本 story 首次交付**生产 ops 手动执行**类 CLI，按 §21.5 配齐 runbook + CONFIRM 守门。未来同类工具（pkg migration script / 批量 purge 等）复用该模板。

### 关键依赖与 Epic 0/1 资产复用

| 来源 | 资产 | Story 1.6 用法 |
|---|---|---|
| 0.5 | `logx.Ctx(ctx)` + `logx.WithUserID` | audit log action=account_deletion_request |
| 0.6 | `ErrValidationError / ErrInternalError / ErrUserNotFound / ErrUnauthorized` | 复用全部；**不**新增错误码 |
| 0.7 | `clockx.Clock / FakeClock` | MarkDeletionRequested 的 `now := r.clock.Now()` + 幂等测试 |
| 0.12 | `ws.ResumeCacheInvalidator` + `pkg/redisx.RedisResumeCache.Invalidate` | Step 4 fail-open invalidate |
| 1.1 | `domain.User.DeletionRequested` + `DeletionRequestedAt` 字段（已预留）+ `ClearDeletion` 反向操作 | 本 story 正向操作 MarkDeletionRequested；ClearDeletion 被 Story 1.1 的 SIWA 复活路径调用（本 story 不动） |
| 1.2 | `AuthService.RevokeAllUserTokens(ctx, userID) error` | Step 2 fail-open revoke all devices |
| 1.3 | `middleware.JWTAuth` + `UserIDFrom` + `ws.Hub.DisconnectUser(userID ws.UserID) (int, error)` | HTTP 路径鉴权 + Step 3 fail-open disconnect |
| 1.4 | `device_handler.go` 的 UserIDFrom-空-401 防御纵深模板 | Handler 层 early-return 401 |
| 1.5 | `UpdateProfile` 单 UpdateOne + dotted $set 模板 | MarkDeletionRequested 单 UpdateOne 写 deletion_requested/deletion_requested_at/updated_at |
| 1.5 | `wire.go` `handlers.device` 字段 + 条件路由注册模式 | 本 story `handlers.user` 同款 |

### 端点 `DELETE /v1/users/me` vs `POST /v1/users/me/deletion-request` 二选一决策

- **选定 `DELETE /v1/users/me`**
- **理由**：
  1. REST 语义对齐：DELETE 表达"请求删除此 me 资源"；client 意图清晰
  2. 客户端指南 §16 预告已广播 `DELETE /v1/users/me`；改 URL 会让已在实施的客户端联调白跑一圈
  3. POST /deletion-request 风格更适合"资源下的 sub-action"（如 `POST /friends/:id/block`），但删除自己的账号是**资源级别**操作而非 sub-action
  4. Apple Privacy Policy 检查项"Delete Account"典型样板就是 DELETE 动词（参考 Apple Developer Documentation）
- **取舍**：POST /deletion-request 的优势是可以传 body（e.g. reason / confirmation-text）——但 MVP 不收集 reason，无需 body；未来 Growth 想收 reason 可新增 POST /v1/users/me/deletion-reasons 不冲突

### 幂等语义（first-write-wins vs re-stamp）

- **选定 first-write-wins**（repo filter `deletion_requested: {$ne: true}` + fallback FindByID）
- **理由**：
  1. NFR-COMP-5 "30 天"从**首次请求**算起；re-stamp 允许恶意用户无限延长 = 漏洞
  2. 客户端重试 / 误调用不应改变合规 SLA 起点
  3. 用户若真想"撤销删除"应走 SIWA resurrection 路径（Story 1.1 ClearDeletion）—— 本 story 不提供"undo then re-request to refresh timer"的语义
- **取舍**：用户希望"重置 30 天倒计时"的合法诉求**不支持**；MVP 不考虑（Growth 再看）

### 严格操作顺序（Step 1→2→3→4）的设计理由

- **顺序锁**：`mark → revoke → disconnect → invalidate`
- **为什么不换**：
  - **为什么 mark 在最前**：数据库是真相源；没 mark 就 revoke/disconnect 可能出现"token 吊销但无删除记录"（审计失联）
  - **为什么 revoke 在 disconnect 前**：revoke 写 Redis blacklist，Redis 比 WS 断开快；先 revoke 让 WS 断开后即使新建连接（race）也会被 blacklist 拦
  - **为什么 invalidate 在最后**：resume cache 是读路径性能层；用户已断线 60s 不会有 session.resume 请求；invalidate 即使失败 TTL 60s 自愈
- **fail-closed vs fail-open 错位的后果**：见 AC15 陷阱 #3

### §21.1 drift gate 不触发说明

- 本 story 引入的 string literal 都是本地 response / audit action 值，不构成集合
- 如 `"deletion_requested"` response status 仅 1 个值；未来若有 `deletion_in_progress` / `deletion_denied` 多 state，再升级为 `domain.DeletionState` enum + 双 gate

### §21.2 Empty→Real grep gate（本 story 不触发）

- 所有 APNs Provider 已 Story 1.4/1.5 替换完毕；session.resume 的 5 个 Empty（Friends/CatState/Skins/Blindboxes/RoomSnapshot）归 Epic 2/3/4/6/7 替换
- 本 story initialize.go `grep -cE "Empty[A-Za-z]+Provider\{\}" cmd/cat/initialize.go` 应**不变**（Δ=0）

### §21.3 Fail-closed / Fail-open 完整声明

见 AC13 矩阵（7 行）；此段是 §21.3 "每个外部系统故障路径必须显式选一种并在 Dev Notes 说明"的集中履行。反"feedback_no_backup_fallback.md"的关键澄清：每条 fail-open 都有 warn log + 兜底机制——不是藏着。

### §21.4 AC review 早启（本 story 推荐跑）

- **符合"工具 / 指标 / 守门 / 度量"判据**：fail-open / fail-closed 矩阵的语义正确性决定合规 SLA 起点；顺序与 flag 写错不会让代码 crash 但会让审计数据失真——正属 §21.4 类别
- **推荐操作**：dev-story 开始前另起一个 Claude session，invoke `bmad-review-adversarial-general` 或 `bmad-review-edge-case-hunter` 喂本 story AC 段落；重点审：
  - AC4 严格顺序 + fail matrix 是否与真实错误场景对齐
  - AC5 幂等 filter 是否覆盖所有状态转换（`false→true` / `true→true` / `user missing`）
  - AC12 process_deletion_queue 的 cascade 清单是否遗漏已有 collection
  - AC15 10 条陷阱是否覆盖全部已知语义错误路径

### 反模式 TL;DR 实施期自检（对应 `server/agent-experience/review-antipatterns.md`）

| # | 反模式 | 本 story 处理 |
|---|---|---|
| §1 close(channel) | N/A — 无 channel | — |
| §2 goroutine panic recover | 继承 Gin Recover middleware；service 同步调用无新 goroutine | ✅ |
| §3 shutdown-sensitive I/O | handler / service / repo 全路径传 `ctx`；Mongo + Redis driver 自带 ctx 尊重 | ✅ |
| §4 全局常量 | 无新集合 | ✅ |
| §5 新 config 字段 | 无 | ✅ |
| §6 JWT | 复用 Story 1.3 middleware；不改 Verify | ✅ |
| §7 release/debug mode gate | DELETE /v1/users/me 两模式都挂；debug mode 靠 handler UserIDFrom-空-401 防御纵深 | ✅ |
| §8 Redis key | 无新 key；复用 resume_cache + refresh blacklist 既有 namespace | ✅ |
| §9 rate limit | **N/A - 关键风险审视**：DELETE /v1/users/me 理论可被滥刷 DoS 写 Mongo；但 rate limit 本 story **不**加：①写点（MarkDeletionRequested）有 `$ne: true` filter + fallback read，实际写压 O(1)（第二次后全 fallback）② Story 0.11 WS 建连 rate limit 对 HTTP 不生效，但 Story 0.x 预计后续加 HTTP per-IP limit（NFR-SCALE-8 60 秒 ≤ 60 次）作为横切层。本 story 显式**不**引入专属 endpoint limit；若 Epic 1.x retro 发现 DoS 场景再补 | ✅ (显式 N/A + 理由) |
| §10 度量 / 比率 | N/A — 不加 metric；fail-open warn log 足够 | — |
| §11 middleware 顺序 | HTTP 路径复用既有 Logger/Recover/RequestID/JWTAuth 链 | ✅ |
| §12 errors.Is/As | service 用 `errors.Is(err, repository.ErrUserNotFound)` 分支；不用字符串比较 | ✅ |
| §13.1 pkg/ ← internal/ 禁止 | `service → ws / repository` 合法方向；无反向 import | ✅ |
| §14 度量 ratio | N/A | — |

### Semantic-correctness 思考题（§21.8 / §19 第 14 条强制 · 10 条陷阱）

见 AC15（10 陷阱详述）；Dev agent 实施后在 Completion Notes List 逐条 self-audit 覆盖状态。

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.6 账户注销请求 — line 874-893]
- [Source: _bmad-output/planning-artifacts/prd.md#FR47 注销账号 — line 828]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-COMP-5 注销请求 SLA — line 942]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-10 审计日志 — line 893]
- [Source: _bmad-output/planning-artifacts/prd.md#account_deletion_cascade domain concern — line 149]
- [Source: docs/backend-architecture-guide.md#§6.1 Handler 职责]
- [Source: docs/backend-architecture-guide.md#§6.2 Service 业务组合 + 跨仓库协调]
- [Source: docs/backend-architecture-guide.md#§7 AppError 模式]
- [Source: docs/backend-architecture-guide.md#§10.3 事务与单次 UpdateOne]
- [Source: docs/backend-architecture-guide.md#§13 AuthRequired 挂载 /v1/*]
- [Source: docs/backend-architecture-guide.md#§14 路由与 API 约定]
- [Source: docs/backend-architecture-guide.md#§21.1 双 gate 漂移守门（本 story 不触发，显式说明）]
- [Source: docs/backend-architecture-guide.md#§21.2 Empty→Real Provider（本 story 不触发）]
- [Source: docs/backend-architecture-guide.md#§21.3 Fail-closed vs Fail-open 决策框架]
- [Source: docs/backend-architecture-guide.md#§21.4 AC review 早启（本 story 推荐跑）]
- [Source: docs/backend-architecture-guide.md#§21.5 工具类 CLI 上线判据（process_deletion_queue 适用）]
- [Source: docs/backend-architecture-guide.md#§21.7 Server 测试自包含]
- [Source: docs/backend-architecture-guide.md#§21.8 语义正确性思考题]
- [Source: server/agent-experience/review-antipatterns.md#TL;DR 自检清单]
- [Source: server/agent-experience/review-antipatterns.md#§7.1 release/debug mode gate]
- [Source: server/agent-experience/review-antipatterns.md#§13.1 pkg/ ← internal/ 禁止]
- [Source: server/internal/domain/user.go#DeletionRequested + DeletionRequestedAt 字段（Story 1.1 预留）]
- [Source: server/internal/repository/user_repo.go#ClearDeletion — Story 1.1 反向操作（本 story 不动）]
- [Source: server/internal/repository/user_repo.go#UpdateProfile — Story 1.5 单 UpdateOne 模板]
- [Source: server/internal/service/auth_service.go#RevokeAllUserTokens — Story 1.2 (L720)]
- [Source: server/internal/service/auth_service.go#ClearDeletion resurrection 路径 — Story 1.1 (L272-283)]
- [Source: server/internal/ws/hub.go#DisconnectUser — Story 1.3 (L166)]
- [Source: server/internal/ws/session_resume.go#ResumeCacheInvalidator — Story 0.12 接口]
- [Source: server/internal/handler/device_handler.go#UserIDFrom-空-401 防御纵深 — Story 1.4 handler 模板]
- [Source: server/internal/handler/auth_handler.go#AuthHandlerService 接口扩展注释 — L17 "Story 1.6 will add RequestDeletion" (本 story 选择放 UserHandler 而非扩 AuthHandler — 账户注销不属 Auth bootstrap 语义)]
- [Source: server/cmd/cat/wire.go#handlers 结构体 + buildRouter 条件路由 — Story 1.4 模板]
- [Source: server/tools/blacklist_user/main.go#一次性 CLI + run() 分离模板 — Story 0.11]
- [Source: server/tools/ws_loadgen/main.go#"仅 spike 使用"注释头模板 — Story 0.15]
- [Source: docs/api/openapi.yaml#当前 version 1.5.1-epic1；本 story bump 1.6.0-epic1]
- [Source: docs/api/integration-mvp-client-guide.md#§16 账户注销预告 — 本 story 升级为正式]
- [Source: _bmad-output/implementation-artifacts/1-1-user-domain-sign-in-with-apple-jwt.md#domain.User 字段布局 + ClearDeletion 复活路径]
- [Source: _bmad-output/implementation-artifacts/1-2-refresh-token-revoke-per-device-session.md#AC8 RevokeAllUserTokens 合约]
- [Source: _bmad-output/implementation-artifacts/1-3-jwt-auth-middleware-userid-context-injection.md#AC8 DisconnectUser 合约]
- [Source: _bmad-output/implementation-artifacts/1-4-apns-device-token-registration-endpoint.md#handler UserIDFrom 防御纵深模板]
- [Source: _bmad-output/implementation-artifacts/1-5-profile-preferences-displayname-timezone-quiethours.md#AC5 UpdateProfile 单 UpdateOne 模板 + AC10 fail matrix 示范]
- [Source: _bmad-output/implementation-artifacts/epic-0-retro-2026-04-19.md#§5.1 预留项 + §21.5 CLI 上线判据]

### Project Structure Notes

- 完全对齐架构指南 `internal/{handler, service, repository, dto, middleware}` + `tools/<name>/main.go` + `docs/runbook/` 结构；无新增 package / pkg 目录
- **新建文件（13 个）**：
  - `server/internal/dto/account_deletion_dto.go`
  - `server/internal/dto/account_deletion_dto_test.go`
  - `server/internal/service/account_deletion_service.go`
  - `server/internal/service/account_deletion_service_test.go`
  - `server/internal/handler/user_handler.go`
  - `server/internal/handler/user_handler_test.go`
  - `server/cmd/cat/account_deletion_integration_test.go`（build tag integration）
  - `server/tools/process_deletion_queue/main.go`（ops CLI entry）
  - `server/tools/process_deletion_queue/run.go`（可测逻辑入口，`blacklist_user` 同款模式）
  - `server/tools/process_deletion_queue/main_test.go`（≥ 4 子测）
  - `docs/runbook/process_deletion_queue.md`（§21.5 三件套之 runbook）
- **修改文件（6 个）**：
  - `server/internal/repository/user_repo.go`（+ `MarkDeletionRequested` 方法）
  - `server/internal/repository/user_repo_integration_test.go`（+ 6 子测）
  - `server/cmd/cat/wire.go`（+ `user *handler.UserHandler` 字段 + `v1.DELETE("/users/me", ...)` 注册）
  - `server/cmd/cat/initialize.go`（装配 accountDeletionSvc + userHandler + 注入 handlers.user）
  - `docs/api/openapi.yaml`（version bump 1.5.1-epic1 → 1.6.0-epic1 + DELETE /v1/users/me path + AccountDeletionResponse schema）
  - `docs/api/integration-mvp-client-guide.md`（§16 从预告升级为正式）
- 无新 external dependency（一切 stdlib `time` + 既有 gin / mongo / zerolog / redis）
- 无架构偏差
- **§21 合规概览**：§21.1 不触发 · §21.2 不触发 · §21.3 AC13 矩阵已声明 · §21.4 推荐 AC review（Dev agent 判断）· §21.5 process_deletion_queue 配 runbook + CONFIRM 守门 · §21.6 N/A（非 spike/真机）· §21.7 集成测试用 Testcontainers · §21.8 AC15 10 陷阱 + Completion Notes self-audit

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash scripts/build.sh --test`：全绿（22/22 packages），build 产物落盘 `build/catserver`。
- `go vet -tags=integration ./...`（在 `server/` 下运行）：clean。
- 小调整：在实现 repository 时把 `MarkDeletionRequested` 返回签名改为 `(*domain.User, bool, error)`（增加 `firstTime` 语义），以便 service 精确生成 `wasAlreadyRequested` 审计字段而无需第二次读。首版曾尝试用 `time.Since(stamp) > 1s` 作为 proxy，被自审识为"启发式、wall-clock 依赖"后放弃，改用 repo 出口布尔。
- 新加错误码 `USER_NOT_FOUND`（404）+ 更新 `docs/error-codes.md` —— AC14 原称 §21.1 drift 不触发，但 service 确实需要一个 404-mapped `*dto.AppError` 才能让 handler 透明 surface。现有 `TestErrorCodesMd_ConsistentWithRegistry` 双 gate 自动覆盖。

### Completion Notes List

**§21.8 10 条陷阱 self-audit（AC15）**

| # | 陷阱 | 覆盖证据 |
|---|---|---|
| 1 | 幂等第二次调用 re-stamp timestamp | ✅ `TestMongoUserRepo_Integration_MarkDeletionRequested_IdempotentPreservesOriginalTimestamp` + `TestAccountDeletion_Integration_Idempotent_SecondCall_PreservesFirstTimestamp` 双层锁死 `$ne: true` filter + 幂等路径不写 updated_at |
| 2 | Step 顺序颠倒 | ✅ `TestAcctDel_StrictOrder_MarkThenRevokeThenDisconnectThenInvalidate` 用 `order []string` 共享 sink 断言四步顺序；`TestAcctDel_Step1_*_NoSideEffects` 两条锁 Step 1 失败后 Step 2/3/4 **不** invoke |
| 3 | fail-closed / fail-open 反了 | ✅ `TestAcctDel_Step2_*_ContinuesToStep3And4` / `Step3_*_ContinuesToStep4` / `Step4_*_Response202` 三条锁 Step 2/3/4 fail-open with warn；Step 1 fail-closed 由 `UserNotFound_Returns404_NoSideEffects` + `GenericError_Wraps500_NoSideEffects` 覆盖 |
| 4 | requestedAt 序列化非 UTC | ✅ `TestUserHandler_RequestDeletion_RequestedAtIsUTC_ZSuffix`：喂 Asia/Shanghai 时间，断言响应 `Z` 后缀 + Shanghai 12:00 → UTC 04:00 |
| 5 | 幂等路径漏 audit log | ✅ `TestAcctDel_AuditLog_EmitsActionWithUserIDAndFlag` 设 `firstTime=false` 驱动幂等场景，断言 `wasAlreadyRequested=true` 进 audit log；Step 2/3/4 仍跑（`HappyPath_AlreadyRequested_StillRunsAllSideEffects`） |
| 6 | ClearDeletion 反向操作没覆盖 | ✅ `TestAccountDeletion_Integration_ResurrectionAfterDeletion_WorksE2E` —— 先 DELETE，再同 `sub` SIWA，断言 Mongo `deletion_requested=false` + `deletion_requested_at` 被 unset；锁死 Story 1.1 resurrection 不被本 story 无意破坏 |
| 7 | DELETE with body not rejected | ✅ `TestUserHandler_RequestDeletion_DeleteWithBody_StillSucceeds` —— handler **不**调 `ShouldBindJSON`；body `{"foo":"bar"}` 被忽略，svc 照常 1 次调用，userId 来自 middleware |
| 8 | WS close code 错 | ✅ `TestAccountDeletion_Integration_HappyPath_AllStepsExecuted` (c) 段断言 `CloseError.Code == CloseNormalClosure (1000)` + `Text == "revoked"`；MultipleDevices 子测也复核 |
| 9 | process_deletion_queue 无 CONFIRM 守门 | ✅ `TestRun_RequiresConfirmInput` 5 种 wrong-input 子测（empty / wrong / lowercase / leading-ws / trailing-ws）全部 exit 1 且 no writes；banner 强制进 stderr (`TestRun_BannerWrittenToStderr`) 避免被 `| jq` 吞掉 |
| 10 | process_deletion_queue cleanup 漏 apns_tokens | ✅ `TestRunIntegration_DeletesExpiredUsersAndApnsTokens`：seed 5 行 apns_tokens（2 用户共 3 行到期 + 2 行未到期），expect `DeletedApnsTokens=3` + 剩余 2 行；代码顺序锁 apns_tokens 先删、users 后删避免 Epic 8 cold-start 推向已删用户 |

**§21.3 fail matrix 自审：** 代码与 AC13 矩阵 7 行逐行一致：
- Step 1 Mongo err → `dto.ErrInternalError` (500) + zerolog error ✅
- Step 1 ErrUserNotFound → `dto.ErrUserNotFound` (404) + zerolog warn ✅
- Step 2 revoke err → warn log `action=account_deletion_revoke_partial` ✅
- Step 3 disconnect err → warn log `account_deletion_disconnect_error` ✅（测试场景用 stub 返 err 验证）
- Step 4 invalidate err → warn log `account_deletion_resume_invalidate_error` ✅
- JWT 401 由 Story 1.3 middleware 挂载 ✅（`TestAccountDeletion_Integration_MissingAuthToken_Returns401` 集成验证）
- 幂等第二次 always 202 ✅（`TestAccountDeletion_Integration_Idempotent_SecondCall_PreservesFirstTimestamp`）

**§21.8 被误导方审视（AC15 上半段）：** 
- NFR-COMP-5 审计方：re-stamp / 顺序反 / fail 策略反都会让"30 天起点"含义扭曲，本 story 代码都 fail-closed 锁死 + 集成测试验证
- `RealUserProvider`：session.resume user 投影当用户 deletion_requested=true 时不再有权消费服务 —— 本 story 只写 flag，读侧 fail-closed 归属未来业务 epic，已在 Dev Notes 里点名
- process_deletion_queue ops：三件套齐全（CONFIRM 守门 + runbook + dry-run preview），cascade 覆盖 `apns_tokens` + `users`，未来 epic 的 cascade 预留 TODO 块
- 被注销用户本人：DELETE 202 后 Keychain / WS / refresh 全部清理，客户端指南 §16.3 明示"收到 202 立即清 Keychain"
- 好友 / 触碰发送方：Epic 3/5 实现时检查接收方 `deletion_requested` —— 本 story 不修 Epic 3/5 代码但在 Dev Notes 留了位置

**§21.1 / §21.2 status：**
- §21.1 drift gate：加了 1 条 `USER_NOT_FOUND`；既有双 gate `TestErrorCodesMd_ConsistentWithRegistry` 自动覆盖 —— registry + docs/error-codes.md 同步加了对应行，存量测试跑过。
- §21.2 Empty→Real：无新 Provider；所有 APNs Provider 已在 Story 1.4/1.5 替换。

**§19 PR checklist 14 条简答：**
1. ✅ 架构 §6.1/§6.2/§10.3/§14/§13 对齐
2. ✅ handler 纯翻译，svc 组合副作用，repo 单 UpdateOne
3. ✅ §P1 snake_case JSON + bson tag
4. ✅ §P3 nil panic：3 个 New* 构造器全部守护
5. ✅ §P5 audit log camelCase 字段
6. ✅ §P7 DTO 层 response 定义，无 request body
7. ✅ §21.1 drift gate —— USER_NOT_FOUND 走既有双 gate
8. ✅ §21.2 Empty→Real —— 不触发
9. ✅ §21.3 fail-closed/fail-open 矩阵 AC13 声明
10. ✅ §21.5 process_deletion_queue 配 runbook + CONFIRM 守门
11. ✅ §21.7 集成测试自包含（Testcontainers Mongo + miniredis）
12. ✅ §21.8 10 陷阱 self-audit 已覆盖
13. ✅ M9：所有业务代码走 Clock interface；tools/ exempt（script 层）
14. ✅ "谁会被误导"思考题：上述 §21.8 自审已答

**本地测试结果：** `bash scripts/build.sh --test` 全绿（22/22 packages），`go vet -tags=integration ./...` clean。集成测试（account_deletion / process_deletion_queue integration）go vet 编译 clean，等 Linux CI Docker 环境跑真用例 —— Windows 本地 Docker Desktop 不可用，与 Story 1.4/1.5 同等处理。

### File List

**新建（12 个）**
- `server/internal/dto/account_deletion_dto.go`
- `server/internal/dto/account_deletion_dto_test.go`
- `server/internal/service/account_deletion_service.go`
- `server/internal/service/account_deletion_service_test.go`
- `server/internal/handler/user_handler.go`
- `server/internal/handler/user_handler_test.go`
- `server/cmd/cat/account_deletion_integration_test.go`（build tag integration）
- `server/tools/process_deletion_queue/main.go`
- `server/tools/process_deletion_queue/run.go`
- `server/tools/process_deletion_queue/main_test.go`
- `server/tools/process_deletion_queue/integration_test.go`（build tag integration）
- `docs/runbook/process_deletion_queue.md`

**修改（8 个）**
- `server/internal/dto/error_codes.go`（+ `ErrUserNotFound` sentinel）
- `server/docs/error-codes.md`（+ `USER_NOT_FOUND` 行）
- `server/internal/repository/user_repo.go`（+ `MarkDeletionRequested` 方法）
- `server/internal/repository/user_repo_integration_test.go`（+ 6 `MarkDeletionRequested` 子测）
- `server/cmd/cat/wire.go`（+ `handlers.user` 字段 + `v1.DELETE("/users/me", ...)`）
- `server/cmd/cat/initialize.go`（装配 `accountDeletionSvc` + `userHandler`）
- `server/cmd/cat/initialize_test.go`（+ 2 条 Story 1.6 wire 测试 + `time`/`ids` import）
- `docs/api/openapi.yaml`（版本 1.5.1-epic1 → 1.6.0-epic1，+ `DELETE /v1/users/me` path，+ `AccountDeletionResponse` schema）
- `docs/api/integration-mvp-client-guide.md`（§16 从"预告"升级为"正式"，含端点契约 / fail matrix / 错误码表 / race window 说明）
- `server/_bmad-output/implementation-artifacts/sprint-status.yaml`（story status ready-for-dev → in-progress → review）

### Change Log

| 日期 | 变更 | 说明 |
|---|---|---|
| 2026-04-19 | Story 1.6 dev 首次落地 | DELETE /v1/users/me 端到端：DTO + repo `MarkDeletionRequested` 幂等 first-write-wins + service 四步严格顺序 (mark→revoke→disconnect→invalidate) + handler UTC RFC3339 + wire + initialize + 新错误码 `USER_NOT_FOUND` (404) + openapi 1.6.0-epic1 + 客户端指南 §16 正式版 + tools/process_deletion_queue 生产 ops CLI + docs/runbook/process_deletion_queue.md § 21.5 三件套 + 10 条 §21.8 思考题全覆盖 |
