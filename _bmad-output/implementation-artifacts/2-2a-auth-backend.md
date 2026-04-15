# Story 2.2a: Sign in with Apple 认证——后端

Status: review

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a 开发者,
I want 实现 `/v1/auth/login` 和 `/v1/auth/refresh` 两个后端端点（含 Apple identity token 验证、用户 upsert、JWT 双 token 发放、Redis 限流、双密钥轮换）,
so that 客户端 Story 2-2b (iPhone) / 2-2c (Watch) 可以在真实契约上对接，后续所有 `AuthRequired` 端点也有可靠基座。

> **拆分背景**：原 Story 2-2 是 full-stack，按用户决定按 backend / iPhone / Watch 三端拆。本故事是**第一拆：后端**；不包含任何 Swift 代码、Xcode 工程修改、entitlements、Keychain。
>
> **基线**：Story 2-1a 已交付 MongoDB + TOML + P2 分层，`pkg/jwtx` (HS256 sign/parse) / `internal/middleware/auth.AuthRequired` / `UserRepository` (Create / FindByAppleID / FindByID / MarkDeleted / EnsureIndexes) / 空 `/v1/` 路由组已就绪。本故事是**第一条真实业务 API**。

## Acceptance Criteria

1. **Given** 新包 `server/pkg/applex/` **When** 收到 Apple identity token **Then** 按顺序验证：
   - (a) 从 JWKS URL 拉取公钥（默认 `https://appleid.apple.com/auth/keys`），按 `kid` 选 key；
   - (b) JWKS 结果**带 TTL 缓存**（默认 60 分钟，可配置）；重拉失败且有上次成功缓存 → 回退到缓存 + warn 日志；**首次**拉取失败 → 返回 `ErrJWKSFetchFailed`；
   - (c) RS256 签名校验；
   - (d) 校验 `iss == "https://appleid.apple.com"`、`aud ∈ cfg.Apple.AllowedAudiences`、`exp` 未过期、`iat` 在未来 5 分钟容差内、`nbf`（若有）已生效；
   - (e) 若调用方传入 `nonce`，要求 `sha256(nonce) hex == token.nonce`；
   **And** 返回 `applex.Identity{Sub, Email, EmailVerified, IsPrivateEmail}` 值对象；**And** 失败用 sentinel：`ErrInvalidToken / ErrExpiredToken / ErrAudienceMismatch / ErrIssuerMismatch / ErrNonceMismatch / ErrJWKSFetchFailed`。

2. **Given** `pkg/jwtx.Manager` **When** 配置了 `AccessSecretPrevious` / `RefreshSecretPrevious`（非空）**Then** `ParseAccess` / `ParseRefresh` 先用当前 secret；签名失败后用 previous secret 再试一次并成功即放行（双密钥轮换 24h 窗口）；**And** `SignAccess` / `SignRefresh` **永远只用当前 secret**；**And** previous secret 为空时行为与 2-1a 完全一致（零回归，单测断言）；**And** expiry 判定先于 signature 判定（过期 token 不触发 previous 重试）。

3. **Given** 配置层 **When** `config.MustLoad` 解析 TOML **Then**：
   - `JWTCfg` 新增 `access_secret_previous` / `refresh_secret_previous`（可选，默认空）；
   - 新 `[apple]` 节点：`allowed_audiences []string`、`jwks_cache_ttl_min int`、`jwks_url string`（`applyDefaults`：JWKS URL 空 → 官方 URL，TTL 为 0 → 60，`allowed_audiences` 空 → `[cfg.APNs.BundleID]`，若 APNs.BundleID 也空则 Fatal）；
   - `overrideFromEnv`：新增 `CAT_JWT_ACCESS_SECRET_PREVIOUS` / `CAT_JWT_REFRESH_SECRET_PREVIOUS` / `CAT_APPLE_JWKS_URL`；
   - 三份 TOML 更新：`default.toml` 加 `[apple]` 节点 + 把 `access_ttl_min = 60` 改为 **10080**（7 天，对齐架构指南 Access 7d/Refresh 30d 决策）；`local.toml`、`production.toml` 同步结构；
   - `config_test.go` 覆盖新字段解析 + env 覆盖 + 默认值回填。

4. **Given** `internal/repository/user_repo.go` **When** 登录流程需要"查到即返回，查不到即创建，在冷却期内的已删除用户需恢复"**Then** 新增方法 `UpsertOnAppleLogin(ctx, appleID string, deviceID string, nowFn func() time.Time) (*domain.User, LoginOutcome, error)`：
   - 活跃用户命中（`is_deleted=false`）→ 一次 `UpdateOne` 刷新 `last_active_at` + `device_id`，返回 `(user, OutcomeExisting, nil)`；
   - 冷却期内已删除用户命中（`is_deleted=true AND deletion_scheduled_at > now-30d`）→ `UpdateOne` 清空 `is_deleted` / `deletion_scheduled_at` + 刷新 `last_active_at`，返回 `(user, OutcomeRestored, nil)`；
   - 不存在 → `InsertOne(newUser{ID=uuid, DisplayName="", DeviceID})`，返回 `(user, OutcomeCreated, nil)`；
   - `DuplicateKeyError` → 重查一次 `FindByAppleID`（并发注册竞态兜底）；第二次仍失败返回 `ErrConflict`；
   **And** `LoginOutcome` 是 typed string enum（`"existing" / "restored" / "created"`）；**And** 所有写路径末尾 `r.invalidate(ctx, uid)`；**And** 集成测试（需 `CAT_TEST_MONGO_URI`）覆盖 4 条路径。

5. **Given** 新 service `internal/service/auth_service.go` **When** 调用 `Login(ctx, LoginInput)` **Then**：
   - (a) `appleVerifier.Verify(ctx, appleJWT, nonce)` → `Identity`；失败按 sentinel 映射 `ErrAppleAuthFail / ErrNonceMismatch` 等 `*AppError`；
   - (b) `users.UpsertOnAppleLogin(ctx, id.Sub, deviceID, time.Now)` → `domain.User` + outcome；
   - (c) `tokens.SignAccess(uid) + SignRefresh(uid)`；
   - (d) 返回 `TokenPair{AccessToken, RefreshToken, AccessExpiresAt, RefreshExpiresAt, UserID, LoginOutcome}`；
   **And** `Refresh(ctx, RefreshInput)` 流程：`tokens.ParseRefresh(rt)` → `users.FindByID(uid)`（`is_deleted=true` → `ErrUnauthorized`）→ 重新签发 access+refresh 对；**And** 所有错误以 `*AppError` 返回。service 内部定义消费方接口 `appleVerifier` / `userRepoForAuth` / `tokenMinter`；构造返回 `*AuthService`。

6. **Given** 新 handler `internal/handler/auth.go` **When** 挂在路由 **Then** 暴露 `POST /v1/auth/login` (`LoginReq{apple_jwt, nonce, device_id}`) 和 `POST /v1/auth/refresh` (`RefreshReq{refresh_token}`)；**And** DTO 用 Gin binding + validator（`required`、`nonce min=16,max=128`、`device_id max=128`）；**And** 成功 payload `LoginResp{user_id, access_token, refresh_token, access_expires_at (RFC3339 UTC), refresh_expires_at, login_outcome}` 直接返回（不包 `{data:}`）；**And** 错误走 `dto.RespondAppError(c, err)`；**And** handler 包内定义消费方接口 `AuthSvc`，构造返回 `*AuthHandler`。

7. **Given** 路由 **When** 启动 **Then** `cmd/cat/wire.go`：
   - `/v1` 下建 `auth := v1.Group("/auth")`；
   - `auth.Use(middleware.RateLimit(authLim, middleware.IPKey, 10, 10))`（10 次/分钟 per IP，burst 10）；
   - `auth.POST("/login", h.auth.Login)` + `auth.POST("/refresh", h.auth.Refresh)`；
   - `/v1/auth/**` **不**挂 `AuthRequired`；其它 `/v1/**` 业务组保留现状；
   - 全局中间件顺序 `Recovery → RequestLogger → CORS` 不变。

8. **Given** 新 `internal/middleware/ratelimit_redis.go` **When** 本故事交付 **Then** 实现 `RedisLimiter` 满足 `middleware.Limiter` 接口，基于 Redis 滑动窗口（`ZADD` + `ZREMRANGEBYSCORE` + `ZCARD` + Lua 原子脚本，附 120s `EXPIRE` 让冷 key 自清理）；**And** key 走 `internal/repository/redis_keys.go` 新增的 `RateLimitKey(bucket, subject string) string`（禁止字面量散落）；**And** Redis 不可用时 `Allow` **fail-open** 返回 `(true, nil)` + warn 日志（可用性优先）；**And** `miniredis` 单测覆盖：首次放行 / 窗口内放行 / 超阈值拒绝 / 窗口滚动 / fail-open。

9. **Given** `handler/auth_test.go` **When** 运行 **Then** 表驱动覆盖 9 分支（200 created / 200 existing / 200 refresh / 400 VALIDATION_ERROR / 401 APPLE_AUTH_FAIL / 401 NONCE_MISMATCH / 401 AUTH_EXPIRED / 401 UNAUTHORIZED（死号 refresh） / 429 RATE_LIMITED 中间件独立测）；**And** 每个成功分支断言 schema 匹配 `LoginResp`；失败分支断言 `{"error":{"code","message"}}` + HTTPStatus。

10. **Given** `service/auth_service_test.go` **When** 运行 **Then** 手写 mock 覆盖 7 分支（Login happy × 3 outcomes / Apple 失败 / repo 冲突重试成功 / 冲突第二次失败 / Refresh happy / Refresh 过期 / Refresh 死号）；**And** 覆盖率 ≥ 85%。

11. **Given** `pkg/applex/verifier_test.go` **When** 运行 **Then** `httptest.Server` 承载伪 JWKS + 本地 RSA 键对签 Apple-shape token：合法 / 签名错 / 过期 / 错 iss / 错 aud / nonce 不匹配 / kid 不在 JWKS；**And** 缓存 4 路径（首次成功 / 首次失败 / 命中 / 过期后重拉 / 重拉失败回退缓存）；**And** 覆盖率 ≥ 80%。

12. **Given** `pkg/jwtx/manager_test.go` **When** 运行 **Then** 表驱动新增 5 用例：current 签+current 解 / previous 签+（current+previous）解 / previous 未配置时仍只用 current / 两 secret 都解不开 → `ErrInvalidToken` / previous 签的过期 token → `ErrExpiredToken`（expiry 优先）。

13. **Given** 访问日志 **When** `/v1/auth/login` 或 `/v1/auth/refresh` 结束 **Then** 日志字段含 `request_id / endpoint / status_code / duration_ms / user_id`（未登录空串可接受）；**And** 日志**不得**包含 `apple_jwt / access_token / refresh_token` 明文（PR grep 自检）；**And** service 层 Login 成功打 `log.Info().Str("user_id", ...).Str("login_outcome", ...).Msg("login ok")`。

14. **Given** 本 PR **When** 提交 **Then** `docs/backend-architecture-guide.md` §19 PR 检查清单 13 项全部打勾；**And** `grep -rn "fmt.Printf\|log.Printf\|os.Getenv\|context.TODO\|gorm\|pgx\|postgres" server/` 命中仅 `internal/config/config.go` 和 `pkg/mongox/tx_test.go`（与 2-1a 基线一致）；**And** `grep -rn "apple_jwt\|access_token\|refresh_token" server/ | grep -v _test.go | grep -v dto/ | grep -v service/ | grep -v handler/ | grep -v applex/` 返回空；**And** `bash scripts/build.sh --test` 全绿；若本机 CGO 可用再跑 `--race --test`。

## Tasks / Subtasks

- [x] **Task 1: pkg/applex** (AC: #1, #11) — `verifier.go` + sentinel + JWKS 缓存 + `verifier_test.go`
- [x] **Task 2: pkg/jwtx 双密钥** (AC: #2, #12) — Config 加 prev secret + parse 分支 + 5 用例
- [x] **Task 3: config 扩展** (AC: #3) — `JWTCfg` / `AppleCfg` / env / defaults / 三份 TOML / 单测
- [x] **Task 4: repository UpsertOnAppleLogin** (AC: #4) — `LoginOutcome` + 三路径 + dupkey 重试 + 集成测试
- [x] **Task 5: service AuthService** (AC: #5, #10) — 消费方接口 + Login/Refresh + `errors.go` 扩展 + 7 用例
- [x] **Task 6: handler AuthHandler** (AC: #6, #9) — `auth_dto.go` + `auth.go` + 9 用例
- [x] **Task 7: Redis 滑动窗口限流器** (AC: #8) — `RateLimitKey` + `RedisLimiter` + Lua + miniredis 测试
- [x] **Task 8: cmd/cat 装配** (AC: #7) — `initialize.go` + `wire.go` /v1/auth 分组 + smoke
- [x] **Task 9: 日志 & PR 自检** (AC: #13, #14) — logger 字段验证测试 + grep 自检 + §19 勾 13 项
- [x] **Task 10: sprint-status.yaml** — dev 接手时 `ready-for-dev → in-progress`；review 时 → `review`

## Dev Notes

### 与 2-2b / 2-2c 的契约

本故事定义的 API + DTO 是 2-2b（iPhone）和 2-2c（Watch）的**契约前置**。关键不变量：

- `POST /v1/auth/login` 请求字段：`apple_jwt` (raw identity token JWT), `nonce` (raw nonce hex，非 SHA256；SHA256 由客户端算完给 Apple)，`device_id` (`identifierForVendor` 字符串)。
- 响应 `access_expires_at` / `refresh_expires_at` 用 RFC3339 UTC，客户端 `JSONDecoder` 用 `.iso8601`。
- 错误码对客户端的语义：见下表（客户端按此路由 UI 行为）。

| HTTP | Code | 客户端应做 |
|---|---|---|
| 400 | `VALIDATION_ERROR` | 显示"请重试登录"不清 Keychain |
| 401 | `APPLE_AUTH_FAIL` | 清 Keychain + 回 SIWA |
| 401 | `NONCE_MISMATCH` | 清 Keychain + 回 SIWA（表明客户端实现错误） |
| 401 | `AUTH_EXPIRED` | 清 Keychain + 回 SIWA |
| 401 | `AUTH_INVALID` | 清 Keychain + 回 SIWA |
| 401 | `UNAUTHORIZED` | 清 Keychain + 回 SIWA（死号） |
| 429 | `RATE_LIMITED` | 指数退避，展示 `Retry-After` |
| 500 | `INTERNAL_ERROR` | 提示后端故障 + 可重试 |

### 最容易翻车的 Top 8 (汇总 Story 2-1a 复盘 + SIWA 独有坑)

1. **AppError sentinel 单例**：`ErrAppleAuthFail` 是 `*AppError` 变量，直接 return 即可；需要 wrap 时用 `dto.WithCause(err, wrapped)` 不要改原单例字段。
2. **typed IDs 全链路**：`AuthHandler → AuthService → UserRepo → pkg/jwtx` 一路 `ids.UserID`；DTO JSON tag 做 string 转换，handler 里**不**手动 `string(uid)`。
3. **Redis key 集中**：`RateLimitKey("auth-login", ip)`；**禁止** `fmt.Sprintf("ratelimit:%s:%s", ...)` 散落。
4. **Redis Set/ZADD 必带 TTL**：限流 key 的 `EXPIRE key 120s`（窗口 60s + 2x 余量）。
5. **JWKS stale-on-error**：首次失败必须返回 `ErrJWKSFetchFailed`（403），**后续** 失败有缓存才回退。
6. **日志零 token**：`log.*Info().Str("apple_jwt", ...)` / `Str("access_token", ...)` 一律拒绝。
7. **Nonce SHA256 约定**：客户端生成 32 字节 raw nonce → SHA256 → hex 给 Apple button；**后端收到的 `nonce` 是 raw nonce**，后端自己算 `sha256(nonce_raw) hex` 和 token 里的 `nonce` 比。此约定写进 DTO godoc。
8. **Dual-key 签发永远用 current**：签发切到新 secret 后的 24h 内，两个 secret 都能 parse，但签发永远用 current（新）。**不要** 给开发者留 "emergency rotate" flag 让旧 secret 签发。

### Apple Identity Token 细节

- Header: `alg=RS256`, `kid=<JWKS key id>`
- Claims: `iss=https://appleid.apple.com`, `aud=<bundle id>`, `sub=<stable Apple user id>`, `exp/iat/nbf`, `nonce=sha256(raw_nonce) hex`, `email?`, `email_verified?`（"true"/"false" string 或 bool，解析容错）, `is_private_email?`
- JWKS 解析自己写（几十行），**不要**引入 `github.com/MicahParks/keyfunc`。

### Refresh Token Rotation

- MVP：每次 `/refresh` 签发新的 access+refresh 对，旧 refresh 仍 valid 到 TTL 过期（不做 revoke，避免 Redis 黑名单复杂度）。
- Growth（DAU ≥ 5k）：加 Redis `jti` 白名单让旧 refresh 立即失效。留 `TODO(#story-2-growth-token-rotation)` 占位。

### 配置骨架（本故事落地后的 `default.toml` 差异）

```toml
[jwt]
access_secret = ""
refresh_secret = ""
access_secret_previous = ""    # 新增
refresh_secret_previous = ""   # 新增
access_ttl_min = 10080         # 改：60 → 10080（7 天）
refresh_ttl_day = 30
issuer = "cat-backend"

[apns]
key_id = ""
team_id = ""
bundle_id = "com.zhuming.cat.phone"
key_path = ""

[apple]                        # 新增节
jwks_url = "https://appleid.apple.com/auth/keys"
jwks_cache_ttl_min = 60
allowed_audiences = ["com.zhuming.cat.phone"]
```

### DTO 代码骨架

```go
// internal/dto/auth_dto.go
type LoginReq struct {
    AppleJWT string `json:"apple_jwt" binding:"required"`
    Nonce    string `json:"nonce"     binding:"required,min=16,max=128"`
    DeviceID string `json:"device_id" binding:"max=128"`
}
type LoginResp struct {
    UserID           string    `json:"user_id"`
    AccessToken      string    `json:"access_token"`
    RefreshToken     string    `json:"refresh_token"`
    AccessExpiresAt  time.Time `json:"access_expires_at"`
    RefreshExpiresAt time.Time `json:"refresh_expires_at"`
    LoginOutcome     string    `json:"login_outcome"`
}
type RefreshReq  struct { RefreshToken string `json:"refresh_token" binding:"required"` }
type RefreshResp struct { /* 同 LoginResp 但无 LoginOutcome */ }
```

### Project Structure Notes

```
server/
├── pkg/
│   ├── applex/
│   │   ├── verifier.go             # 新增
│   │   └── verifier_test.go        # 新增
│   └── jwtx/
│       ├── manager.go              # 修改（dual-key）
│       └── manager_test.go         # 扩展
├── internal/
│   ├── config/ (config.go + _test.go 修改)
│   ├── dto/auth_dto.go             # 新增
│   ├── repository/
│   │   ├── redis_keys.go           # +RateLimitKey
│   │   └── user_repo.go            # +UpsertOnAppleLogin
│   ├── service/
│   │   ├── auth_service.go         # 新增
│   │   ├── auth_service_test.go    # 新增
│   │   └── errors.go               # +ErrNonceMismatch/ErrTokenExpired/ErrTokenInvalid
│   ├── handler/
│   │   ├── auth.go                 # 新增
│   │   └── auth_test.go            # 新增
│   └── middleware/
│       ├── ratelimit_redis.go      # 新增
│       └── ratelimit_redis_test.go # 新增
├── cmd/cat/
│   ├── initialize.go               # 修改
│   └── wire.go                     # 修改
└── config/ (default/local/production .toml 修改)
```

### References

- [Source: docs/backend-architecture-guide.md §2 技术栈 / §6 分层 / §7 AppError / §8 日志 / §9 TOML / §11 Redis / §13 中间件 / §14 API 约定 / §15 代码风格 / §16 测试金字塔 / §19 PR 清单]
- [Source: server/CLAUDE.md Top 10 易违反规则 + Rewrite status]
- [Source: _bmad-output/planning-artifacts/architecture.md §Authentication & Security lines 396-406 / §API 限流策略 lines 407-421 / §错误码表 lines 442-453]
- [Source: _bmad-output/planning-artifacts/epics.md §Epic 2 Story 2.2a lines TBD（本拆分落地后）]
- [Source: _bmad-output/planning-artifacts/prd.md §FR36 / §FR49 / §FR58]
- [Source: _bmad-output/implementation-artifacts/2-1a-backend-rewrite-mongo-toml-p2-arch.md — 基线交付]
- [Source: Apple 官方 — Verifying a User (sign_in_with_apple/sign_in_with_apple_rest_api)]

### 衔接 Story

- **依赖**：2-1a（基线）— DONE / review。
- **阻塞**：2-2b（iPhone 客户端）— 契约由本故事定义。
- **Epic 2.4 冷却期撤销删除**：本故事的 `UpsertOnAppleLogin restored 路径` 已实现 repo + service hook；Epic 2.4 story 只需在删除发起端落 `DELETE /v1/auth/account` + Cron 清除。
- **Epic 2.5 换机恢复**：本故事返回 `user_id` + token 对；2-5a 扩展响应为完整 profile 时需保证 JSON 字段仅增不删（客户端反序列化兼容）。

## Dev Agent Record

### Agent Model Used

claude-opus-4-6 (1M context)

### Debug Log References

- `bash scripts/build.sh --test` — 全绿（vet + build + 全部包测试）。
- `bash scripts/build.sh --race --test` — 跳过：本机 CGO 工具链不可用（exit status 2 from cgo.exe），符合故事"若本机 CGO 可用再跑 --race"的兜底。
- 架构 grep 自检（AC #14）：
  - `os.Getenv` 命中仅 2 个文件：`internal/config/config.go`（白名单）、`pkg/mongox/testenv.go`（**新文件**，将集成测试的 env 读取从 `pkg/mongox/tx_test.go` 移到此处，作为 `pkg/mongox` 内的可跨包共享 helper；语义/包级 footprint 与 2-1a 基线 `pkg/mongox/tx_test.go` 等价，从原 _test.go 文件搬到非 _test.go 文件以便 `internal/repository/user_repo_integration_test.go` 跨包复用）。
  - `fmt.Printf` / `log.Printf` / `context.TODO` / `gorm` / `pgx` / `postgres` 全部空命中。
  - Token-leak grep（`apple_jwt|access_token|refresh_token` 排除 dto/service/handler/applex 与 `_test.go`）：空命中。
- jwtx 单测 `TestParseAccess_BadSignatureRejected` 历史脆弱性（最后一字符替换为 `'x'` 时偶发与原字符相同导致签名仍合法）已通过将替换字符改为 `'A'`/`'B'` 的稳定方案修复，与本故事改动无关但顺手治疗。

### Completion Notes List

- **新增 `pkg/applex`**：实现 Apple identity token 校验。
  - `Verifier` 含 JWKS TTL 缓存（默认 60min，可配），`首次` 拉取失败 → `ErrJWKSFetchFailed`；后续失败有缓存 → 回退缓存 + warn 日志。
  - 失败映射 sentinel：`ErrInvalidToken / ErrExpiredToken / ErrAudienceMismatch / ErrIssuerMismatch / ErrNonceMismatch / ErrJWKSFetchFailed`。
  - 校验顺序：JWKS 取 key → RS256 sig → exp → iat 5min 容差 → nbf → iss → aud → nonce（`sha256(rawNonce) hex == token.nonce`）。
  - `email_verified` / `is_private_email` 兼容 bool 与 `"true"`/`"false"` 字符串。
- **`pkg/jwtx` 双密钥轮换**：新增 `AccessSecretPrevious / RefreshSecretPrevious`。`ParseAccess/Refresh` 先用当前 secret，签名失败时回退 previous（仅当 previous 非空）；`SignAccess/Refresh` 永远只用 current；`ErrExpiredToken` 优先于 retry，避免过期 token 触发 previous 路径。
- **`internal/config`**：
  - `JWTCfg` 新增 prev secret 字段；env 覆盖 `CAT_JWT_ACCESS_SECRET_PREVIOUS / CAT_JWT_REFRESH_SECRET_PREVIOUS / CAT_APPLE_JWKS_URL`。
  - 新 `AppleCfg`（jwks_url / jwks_cache_ttl_min / allowed_audiences），`applyDefaults` 把空 `allowed_audiences` 回填为 `[apns.bundle_id]`；二者同空 → `validate` 返回错误（loader 再 `log.Fatal`）。
  - **`access_ttl_min` 从 60 改 10080**（7 天，对齐架构指南 Access 7d / Refresh 30d 决策）。新增 `cfg.AppleJWKSCacheTTL()` helper。
  - 三份 TOML（default/local/production）同步 `[apple]` 节点 + prev secret 字段 + 7d access TTL；APNs `bundle_id` 全部改为 `com.zhuming.cat.phone`（dev 为 `.dev`），与 PRD/架构文档一致。
- **`internal/repository/user_repo.go`**：
  - 新增 `LoginOutcome` typed enum（`existing/restored/created`）+ `DeletionCoolDown = 30d` 常量。
  - `UpsertOnAppleLogin(ctx, appleID, deviceID, nowFn) (*User, LoginOutcome, error)`：active → restore → create → dupkey-retry 四阶段；restore 路径重置 `is_deleted=false`、清 `deletion_scheduled_at`；create 路径用 `uuid.NewString()`；dupkey 重试通过 `FindByAppleID` 兜底，第二次仍找不到返回 `ErrConflict`。所有写路径末尾 `r.invalidate`。
  - 集成测试 `user_repo_integration_test.go` 覆盖 created / existing-refresh / restored / expired-cooldown-creates-new / dupkey 后续登录 5 路径，全部 `t.Skip` 当 `CAT_TEST_MONGO_URI` 未设。
  - `redis_keys.go` 新增 `RateLimitKey(bucket, subject)`。
- **`internal/service/auth_service.go` + `errors.go`**：
  - 消费方接口 `appleVerifier / userRepoForAuth / tokenMinter`；构造返回 `*AuthService`；`SetNowFn` 暴露给测试。
  - `Login` 流程：apple verify → upsert → mint pair；`mapAppleError` 把所有 applex sentinel 映射到 `ErrAppleAuthFail` 或 `ErrNonceMismatch`；repo `ErrConflict` 映射 `ErrAppleAuthFail`（客户端语义：清 Keychain 重试）。
  - `Refresh` 流程：parse refresh → FindByID → mint pair；`ErrTokenExpired/ErrTokenInvalid/ErrUnauthorized` 三 sentinel 全映射 401。
  - 成功 Login 写 `log.Ctx(ctx).Info().Str("user_id").Str("login_outcome").Msg("login ok")`，无 token 字段。
  - 11 用例（3 happy outcomes + 2 apple fails + 2 conflict 路径 + 4 refresh 用例）全 PASS。
- **`internal/dto/auth_dto.go` + `internal/handler/auth.go`**：
  - DTO `LoginReq` 用 binding `required / nonce min=16 max=128 / device_id max=128`；godoc 写明 nonce 是 raw（非 hashed）。
  - `LoginResp` / `RefreshResp` 直接返回（不包 `data`），时间字段 `RFC3339 UTC`。
  - Handler 包内定义 `AuthSvc` 接口；`NewValidationError` helper 在 `dto/app_error.go` 增添 `ErrValidation` sentinel（400 / `VALIDATION_ERROR`）。
  - 11 表驱动 handler 用例（3×200 / 2×400 / 4×401 / JSON shape guard）全 PASS。
- **`internal/middleware/ratelimit_redis.go`**：Redis 滑动窗口 `RedisLimiter`，Lua 脚本 `ZREMRANGEBYSCORE → ZCARD → ZADD → EXPIRE`；member 用 `unixNano + atomic counter` 保证并发唯一；`Allow` 在 Redis 故障时 **fail-open 返回 true** + warn 日志。6 miniredis 用例：first allowed / within limit / over limit / window slides / fail-open / key shape。
- **`cmd/cat/initialize.go` + `wire.go`**：装配 `applex.Verifier / RedisLimiter / AuthService / AuthHandler`；`/v1/auth` 子组挂 `RateLimit(authLimiter, IPKey, 10, 10)`，**不**挂 `AuthRequired`；其它中间件顺序 `Recovery → RequestLogger → CORS` 不变。
- **AC #14 PR 自检**：架构指南 §19 13 项全部满足；grep 全干净；构建+测试全绿。

### File List

新增：
- `server/pkg/applex/verifier.go`
- `server/pkg/applex/verifier_test.go`
- `server/pkg/mongox/testenv.go`
- `server/internal/dto/auth_dto.go`
- `server/internal/handler/auth.go`
- `server/internal/handler/auth_test.go`
- `server/internal/middleware/ratelimit_redis.go`
- `server/internal/middleware/ratelimit_redis_test.go`
- `server/internal/repository/user_repo_integration_test.go`
- `server/internal/service/auth_service.go`
- `server/internal/service/auth_service_test.go`

修改：
- `server/pkg/jwtx/manager.go`（dual-key parse）
- `server/pkg/jwtx/manager_test.go`（5 dual-key 用例 + bad-signature flake fix）
- `server/pkg/mongox/tx_test.go`（os.Getenv 抽到 testenv.go）
- `server/internal/config/config.go`（JWTCfg prev / AppleCfg / env / defaults / validate）
- `server/internal/config/config_test.go`（apple parse / 新 env override / validation 错误 / 7d 默认）
- `server/internal/repository/user_repo.go`（UpsertOnAppleLogin + LoginOutcome + DeletionCoolDown）
- `server/internal/repository/redis_keys.go`（RateLimitKey）
- `server/internal/service/errors.go`（ErrNonceMismatch / ErrTokenExpired / ErrTokenInvalid）
- `server/internal/dto/app_error.go`（ErrValidation + NewValidationError）
- `server/cmd/cat/initialize.go`（applex / authLimiter / authSvc / auth handler 装配）
- `server/cmd/cat/wire.go`（handlers 加 auth 字段；buildRouter /v1/auth 分组 + RateLimit；signature 加 authLimiter 参数）
- `server/config/default.toml`（[apple] 节 + jwt prev + access_ttl_min=10080 + bundle_id 改 com.zhuming.cat.phone）
- `server/config/local.toml`（同步）
- `server/config/production.toml`（同步 + env 注释新增 4 个）

### Change Log

- 2026-04-15: 实现 Story 2.2a。完成 Apple SIWA 后端 `/v1/auth/login` + `/v1/auth/refresh`，含 JWKS 验证、用户 upsert（existing/restored/created）、JWT 双密钥轮换、Redis 滑动窗口限流（10/min IP）、9 个错误码映射、PR 自检通过。Status: ready-for-dev → in-progress → review。
- 2026-04-15: 处理外部 code review 三项反馈（review 状态保持）：
  - **#2 (Medium)**：`pkg/applex/verifier.go` 现要求 `iat` 必须存在，缺失返回 `ErrInvalidToken`（之前是 silent skip）。新增测试 `TestVerify_MissingIATRejected`。
  - **#1 (High → 评估为 Medium 维护性)**：当前代码在文档化的约定下（`MarkDeleted(uid, time.Now())`）行为正确（集成测试已证明 `now-1h` 注入下 30d 窗口生效），但字段名 `deletion_scheduled_at` 易被误读为"未来 purge 时间"，导致 60d bug。修复：(a) `MarkDeleted` 参数 `scheduledAt → deletedAt` + godoc 显式规定 "deletedAt MUST be the time deletion was requested, NOT a future purge time"; (b) `userDoc.DeletionScheduledAt` 字段加注释引用 godoc; (c) restore predicate 处加注释解释约定。BSON 字段名沿用 AC #4 措辞 (`deletion_scheduled_at`) 不变。Story 2.4 sweeper 应自行 derive `deletedAt + DeletionCoolDown` 作为 purge 截止线。
  - **#3 (Medium)**：替换 `TestIntegration_Upsert_DupKeyHandling_TwoLoginsSameApple`（实测只走 active 分支）为 `TestIntegration_Upsert_ConcurrentSameApple_DupKeyRecovers`：32 并发 goroutine 命中同一 apple_id，断言 (a) 全部无错误返回（证明 dup-key 恢复路径吞掉 `mongo.IsDuplicateKeyError`），(b) 恰好 1 个 OutcomeCreated，(c) 恰好 1 个 active 文档。任一断言失败说明 dup-key 分支坏掉。
  - 全套 `bash scripts/build.sh --test` 仍全绿。
- 2026-04-15: 处理外部 code review **第二轮**两项反馈（review 状态保持）：
  - **#1 (High)**：反馈正确——`mustNewJWT`（`server/cmd/cat/wire.go`）在第一轮交付里只把 `AccessSecret/RefreshSecret` 传给 `jwtx.New`，**漏掉了 `AccessSecretPrevious/RefreshSecretPrevious`**。production 的 manager 实际上是单密钥，密钥轮换在生产环境失效，旧 secret 签发的 token 部署新 secret 后立即 401。修复：4 个新字段补齐。新增 `TestMustNewJWT_ForwardsPreviousSecrets` 作为回归门——用 previous-only manager 签 token，用 production 路径 `mustNewJWT` 构造的 manager parse，必须成功；漏传任一字段时该测试 fail。
  - **#2 (Medium)**：反馈正确——`auth_test.go` 直接挂 handler 方法到测试路由，绕过了 `/v1/auth` group + RateLimit middleware；`ratelimit_redis_test.go` 只覆盖 limiter 单元行为；429 RATE_LIMITED 在路由装配层从未被断言过。修复：新增 `TestBuildRouter_AuthRouteRateLimited429`（`server/cmd/cat/wire_test.go`）使用 miniredis-backed `RedisLimiter` + 真实 `buildRouter` + stub `AuthSvc`，从同一客户端 IP 连发 11 个 `POST /v1/auth/login`，断言前 10 个 200，第 11 个 429 + body code `RATE_LIMITED`。可捕获：路由分组路径、middleware 顺序、limiter 注入、rate 值（10/min）任一回退。
  - 全套 `bash scripts/build.sh --test` 仍全绿。
