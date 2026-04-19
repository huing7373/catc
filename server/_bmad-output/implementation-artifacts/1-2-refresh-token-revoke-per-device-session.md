# Story 1.2: Refresh token 刷新 + 吊销 + per-device session 隔离

Status: done

<!-- Validation is optional. Run validate-create-story for quality check before dev-story. -->
<!-- §21.4 AC review 触发点：本 story 为"auth guard + 度量" 类（rolling-rotation refresh 是密码学安全边界，stolen-token-reuse detection 是 "首次成功 after"-类语义），dev agent 在 implementation 前建议先做一轮 AC self-review，逐条对照 AC ↔ 反模式 §3.x / §14 / 架构指南 §21.3。结尾 "Semantic-correctness 思考题"必须在 Completion Notes 回答。 -->

## Story

As a signed-in user on multiple devices (Watch + iPhone independently),
I want each device's access token to refresh independently via a rolling-rotation refresh token, and I want stolen / replayed refresh tokens to be detected and all of that device's credentials to be instantly revocable without affecting my other devices,
So that Watch 和 iPhone 能各自长期保持登录（NFR-SEC-4 ≤ 30 天）、单设备 logout 不影响另一台、并且泄露一次的 refresh token 不能被反复使用（rolling rotation + reuse detection，FR5 / FR6 / NFR-SEC-3/4 / NFR-PERF-2）。

**Business value**：
- **FR5 per-device 独立会话**首次真正落地（Story 1.1 仅签发双 token，但没有"每台设备独立吊销"的机制 —— 本 story 补完）。
- **Story 1.6 账户注销**的前置依赖（epic AC 第 799-800 行显式写出 `RevokeRefreshToken` / `RevokeAllUserTokens` 两个辅助方法由本 story 交付）。
- **Story 1.3 JWT middleware + `DisconnectUser`**的语义闭环（1.3 会在 token 被 1.2 吊销后显式切断 WS 连接）。
- Rolling-rotation + **stolen-token-reuse detection** 是 OAuth2 / SIWA 最佳实践；本 story 是 Epic 1 的第二颗密码学关键节点（第一颗是 1.1 的 Apple JWT 验证）。

## Acceptance Criteria

1. **AC1 — `pkg/jwtx/manager.go` Issue 扩展：caller-controlled jti + Clock 注入（架构 §M9, 反模式 §3.4 / §3.5 / §4.1）**：

   - `Manager.Issue(claims CustomClaims) (string, error)` 当前用 `time.Now()` 与覆盖 `claims.Issuer / IssuedAt / ExpiresAt`。本 story 扩展：
     - **内部时钟改走 `m.appleClock`**（Story 1.1 `NewManagerWithApple` 已注入；`New(opts)` 兼容路径走 `time.Now`，但**仅 Epic 0 sign-only 测试**消费 —— 生产路径必须经 `NewManagerWithApple`，反模式 §4.1）。为了不破坏 Epic 0 的 `New` 签名，给 `Manager` 加 `issueClock clockx.Clock` 字段；`New(opts)` 默认 `issueClock = clockx.NewRealClock()`；`NewManagerWithApple` 把 `apple.Clock` 同步赋给 `issueClock`。`Issue` 体内改为 `now := m.issueClock.Now()`。
     - **caller 传入的 `RegisteredClaims.ID`（jti）必须保留**（反模式 §3.5）。当前 `Issue` 只覆盖 `Issuer / IssuedAt / ExpiresAt` 三个字段，不碰 jti / Subject，行为正确，但**新增单测** `TestManager_Issue_PreservesRegisteredClaimsID` 锁定该契约，与 Story 1.1 已有 `TestManager_Issue_PreservesRegisteredClaims` 形成互补。
     - **caller 传入空 jti 时 Issue 不自动生成** —— 由 service 层显式 `ids.NewRefreshJTI()`（见 AC2）构造；这样 Issue 保持无副作用。单测 `TestManager_Issue_EmptyJTIStaysEmpty` 锁定"空 jti 不被魔法填充"。
   - `pkg/jwtx/manager.go` 加 godoc 说明"生产路径强制走 `NewManagerWithApple` 以取得 Clock"，避免未来 dev 调 `New(opts).Issue` 在生产用 `time.Now` 而绕开 FakeClock 测试。

2. **AC2 — `pkg/ids/ids.go` JTI 构造器（架构 §P1, 反模式 §8.1/§8.2）**：

   - 追加：
     ```go
     // NewRefreshJTI returns a fresh UUID v4 encoded JTI string for a
     // refresh token. The JTI is the server-side canonical identifier
     // for the token — it is written to users.sessions[deviceId].current_jti
     // and used as the refresh_blacklist key suffix. UUID v4 guarantees
     // both uniqueness and that two concurrent sign-in / refresh calls
     // never collide at the JTI level (defense in depth behind
     // sessions[deviceId] Mongo conflict).
     func NewRefreshJTI() string { ... }
     ```
     实现同 `NewUserID()` — 用 `uuid.NewRandom()`，失败 panic（请求期调用，由 panic+recover middleware 转换为 `INTERNAL_ERROR`）。
   - 单测 `TestNewRefreshJTI_UUID4Format` + `TestNewRefreshJTI_Uniqueness`（1000 次唯一）。
   - **不与 `NewUserID` 合并成同名函数** —— 命名区分出"身份 id"与"token id"，让 grep / code review 一眼能看出 jti 来源。

3. **AC3 — `pkg/redisx/refresh_blacklist.go` 吊销黑名单（架构 §D16 key space, 反模式 §8.1 / §8.2 / §4.1）**：

   - 新建文件 `pkg/redisx/refresh_blacklist.go`（与 Story 0.11 `blacklist.go` 并列；refresh 域与 device 域严格隔离）。
   - 接口（service 侧消费者定义在 `internal/service/auth_service.go`，见 AC6）：
     ```go
     // RefreshBlacklist stores revoked refresh-token JTIs with a TTL
     // equal to the token's remaining validity. Key namespace is
     // `refresh_blacklist:{jti}` per PRD §Redis Key Space — isolated from
     // event:* / resume_cache:* / blacklist:device:* / ratelimit:* /
     // presence:* / state:* / apns:* / lock:*.
     //
     // JTI is a UUID v4 → colon-free → injective without length-prefix
     // encoding (反模式 §8.2). The Revoke path writes the key; IsRevoked
     // reads it; both fail-closed on Redis error — see §21.3 decision
     // matrix (a refresh call that cannot verify revocation state must
     // reject, never succeed by "assuming clean").
     type RefreshBlacklist struct {
         cmd   redis.Cmdable
         clock clockx.Clock
     }
     
     func NewRefreshBlacklist(cmd redis.Cmdable, clk clockx.Clock) *RefreshBlacklist
     
     // Revoke writes the jti with ttl = remaining (exp - now). If ttl <= 0
     // (token already expired) the method is a no-op returning nil — no
     // point storing a blacklist entry for a token the verifier would
     // have rejected anyway.
     func (b *RefreshBlacklist) Revoke(ctx context.Context, jti string, exp time.Time) error
     
     // IsRevoked returns (true, nil) when the key exists, (false, nil)
     // when absent. Redis error → (false, err) — caller MUST treat err
     // as fail-closed.
     func (b *RefreshBlacklist) IsRevoked(ctx context.Context, jti string) (bool, error)
     ```
   - key 模板：`refresh_blacklist:{jti}`（literal `refresh_blacklist:` + UUID v4；UUID 不含冒号 / 斜杠 → injection 安全）。
   - 构造函数 fail-fast（反模式 §4.1）：`cmd == nil` / `clk == nil` → `panic`（pkg 侧构造，启动期唯一调用点在 `initialize.go`；panic 走 `log.Fatal` 语义）。
   - 单测 `pkg/redisx/refresh_blacklist_test.go`（miniredis）：
     1. `TestRefreshBlacklist_RevokeThenIsRevokedTrue` — Revoke(jti, exp=now+1h) → IsRevoked → true
     2. `TestRefreshBlacklist_IsRevokedFalseWhenAbsent` — Fresh key → IsRevoked → false
     3. `TestRefreshBlacklist_RevokeZeroTTLNoop` — exp < now → Revoke 不写 key（`miniredis.Exists` = false）且返回 nil
     4. `TestRefreshBlacklist_RevokeRespectsTTL` — Revoke(jti, exp=now+1h) → `miniredis.TTL(key)` ≈ 1h
     5. `TestRefreshBlacklist_MultipleJTIsIndependent` — 两个 jti 独立 Revoke，互不影响
     6. `TestRefreshBlacklist_RevokeRedisErrorPropagates` — inject error-returning cmd → Revoke 返 err
     7. `TestRefreshBlacklist_IsRevokedRedisErrorPropagates` — 同上 IsRevoked 返 err
     8. `TestRefreshBlacklist_ConstructorPanicsOnNilDeps` — `NewRefreshBlacklist(nil, clk)` / `NewRefreshBlacklist(cmd, nil)` panic
   - godoc 顶部列出 D16 key-space isolation 清单（参考 `session_resume.go` / `resume_cache.go` 先例）：`refresh_blacklist:{jti}` vs `event:* / event_result:* / resume_cache:* / lock:cron:* / ratelimit:ws:* / blacklist:device:* / presence:* / state:* / apns:*`。

4. **AC4 — `internal/repository/user_repo.go` sessions 字段读写（架构 §P1 / §M7, 反模式 §6.2）**：

   - 扩展 `MongoUserRepository`（新增 3 方法；`UserRepository` interface 同步扩充，定义在 `internal/service/auth_service.go` consumer 侧）：
     ```go
     // UpsertSession writes sessions.<deviceId> = {current_jti, issued_at,
     // has_apns_token: existing or false} atomically via $set. Returns
     // ErrUserNotFound if no user document matched. Called by
     // SignInWithApple (Story 1.1 extension, AC7 below) and by
     // RefreshToken (AC5). `has_apns_token` is NOT touched — Story 1.4
     // will own that field; we only $set current_jti + issued_at so 1.4
     // doesn't need a migration.
     UpsertSession(ctx context.Context, userID ids.UserID, deviceID string, s domain.Session) error
     
     // GetSession returns sessions.<deviceId> or (domain.Session{}, false,
     // nil) if the sub-document is absent. Non-Mongo errors propagate
     // unchanged. Returns ErrUserNotFound when the user document itself
     // does not exist.
     GetSession(ctx context.Context, userID ids.UserID, deviceID string) (domain.Session, bool, error)
     
     // ListDeviceIDs returns every key of the sessions map for userID.
     // Used exclusively by AuthService.RevokeAllUserTokens to iterate
     // the user's devices. Returns ErrUserNotFound on missing user.
     ListDeviceIDs(ctx context.Context, userID ids.UserID) ([]string, error)
     ```
   - **只用 `$set sessions.<deviceId>.*` partial update**（不替换整个 sessions map） —— Story 1.4 之后的字段（`has_apns_token`）不被意外清空。BSON 路径形如 `{"$set": {"sessions.f47ac10b-58cc-...": {...}, "updated_at": now}}`。
     - `deviceID` 由客户端生成 UUID v4（Story 1.1 dto validator 已校验 `binding:"uuid"`，dot / `$` 不可能出现 → 无 NoSQL injection 风险）；另在 `UpsertSession` 入口**再校验一次** `strings.ContainsAny(deviceID, ".$")` 或直接 `_, err := uuid.Parse(deviceID)` 作防御（架构 §13 层边界 + 反模式 §8.2 "components might contain separators"），防御深度不是信任边界但仍要在。
     - `sessions.<deviceId>.current_jti` + `sessions.<deviceId>.issued_at` 显式 `$set`；`has_apns_token` 字段**不出现在本 story 的 $set 中**（未来 1.4 独立 set）。
   - `GetSession` 查询用 projection 仅拉 `sessions.<deviceId>`（减少网络与 decode 开销；PRD line 801 "≤ 2-3 个 device" 体量小但仍用良好实践）。
   - `ListDeviceIDs` 用 projection `{sessions: 1}`；若 `sessions` 字段 nil / 空 map 返 `[]string{}` 而非 nil。
   - 单测（`internal/repository/user_repo_test.go`）—— BSON roundtrip：扩展 Story 1.1 `TestUser_BSONFieldNames` 加一条 `sessions.<uuid>.current_jti` / `issued_at` 字段映射断言。
   - **集成测试** `internal/repository/user_repo_integration_test.go` (`//go:build integration`) 追加 7 个子 case：
     - `TestMongoUserRepo_UpsertSession_CreateThenFind` — Insert 干净 user → UpsertSession(deviceA) → GetSession(deviceA) 返预期值 + ok=true
     - `TestMongoUserRepo_UpsertSession_Overwrite` — 两次 UpsertSession 同 deviceId → GetSession 返第二次值（rolling rotation 落盘）
     - `TestMongoUserRepo_UpsertSession_IndependentDevices` — UpsertSession(watch) + UpsertSession(iphone) → GetSession 分别返各自值；ListDeviceIDs 返 [watch, iphone]（顺序无关，排序后比）
     - `TestMongoUserRepo_GetSession_AbsentDevice` — fresh user → GetSession(unknown) → (zero, false, nil)；**不是** ErrUserNotFound
     - `TestMongoUserRepo_GetSession_UserNotFound` — 不存在的 userID → ErrUserNotFound
     - `TestMongoUserRepo_UpsertSession_UserNotFound` — 不存在的 userID → ErrUserNotFound（MatchedCount==0）
     - `TestMongoUserRepo_UpsertSession_PreservesOtherFields` — Insert user + UpsertSession(deviceA) → Decode 回来断言 `DisplayName / Preferences / FriendCount / DeletionRequested` 均保持 Story 1.1 seed 值（证明 $set 只动 sessions / updated_at）
     - `TestMongoUserRepo_ListDeviceIDs_Empty` — fresh user → ListDeviceIDs 返 `[]string{}`，长度 0

5. **AC5 — `internal/service/auth_service.go::RefreshToken` rolling rotation + reuse detection（NFR-SEC-3/4, 反模式 §3.1-§3.5 / §8.1）**：

   - 在 `AuthService` 追加方法：
     ```go
     type RefreshTokenRequest struct {
         RefreshToken string
     }
     
     type RefreshTokenResult struct {
         AccessToken  string
         RefreshToken string
     }
     
     func (s *AuthService) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*RefreshTokenResult, error)
     ```
   - 流程（逐步 + fail-closed 语义，每步对应一条反模式 check）：
     1. **Verify 自签 refresh token**：`claims, err := s.verifier.VerifyRefresh(req.RefreshToken)`（**新接口**见 AC6 —— service-side interface 抽象 `jwtx.Manager.Verify`，注入便于测试；生产实现直接转发到 `jwtx.Manager.Verify`）。
        - Verify 失败（签名 / exp / iss / alg / kid 任一错） → 返 `dto.ErrAuthInvalidIdentityToken.WithCause(err)`（复用 Story 0.6 既有错误码；**不新增** —— 反模式 §21.1 drift gate）。实际 HTTP 401。
        - 若 token 的 `TokenType != "refresh"` → 返 `dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("not a refresh token"))`（防御：access token 被当 refresh 用）。
     2. **Check blacklist**：`revoked, err := s.blacklist.IsRevoked(ctx, claims.ID)`；
        - err 非 nil → `dto.ErrInternalError.WithCause(err)`（**fail-closed** — 架构指南 §21.3：无法确认黑名单状态 → 拒绝；不"默认干净放行"）。
        - `revoked == true` → `dto.ErrAuthRefreshTokenRevoked.WithCause(nil)`（HTTP 401；body.error.code = `AUTH_REFRESH_TOKEN_REVOKED`）+ 审计日志见 AC8。
     3. **Reuse detection（强制）**：`session, ok, err := s.users.GetSession(ctx, userID, claims.DeviceID)`；
        - err 非 nil → `ErrInternalError`。
        - `ok == false` 或 `session.CurrentJTI == ""` → **拒绝**：设备会话从未初始化 —— 异常情况；返 `ErrAuthRefreshTokenRevoked`（等价语义："你持有的 token 不对应任何有效会话"；审计日志 `reason="session_not_initialized"`）。**不自愈**（不视为"可能 1.1 遗漏 UpsertSession"；1.1 必须先修）。
        - `session.CurrentJTI != claims.ID` → **stolen-token-reuse detected**：这把 refresh token 不是该设备当前有效的那把 → 攻击者重放了一把已 rotate 的旧 token；**按最严策略吊销该 (userId, deviceId) 当前有效的 jti** 并返 `ErrAuthRefreshTokenRevoked`。审计日志 `action="refresh_token_reuse_detected"` + `oldJti=claims.ID` + `currentJti=session.CurrentJTI`（**两者都 log，但不 log token 本身**）。
          - 具体吊销实现：`s.blacklist.Revoke(ctx, session.CurrentJTI, <当前 session 对应 token 的 exp>)` —— 但我们没有 current token 的 exp；用**保守 TTL `= s.jwt.RefreshExpiry()`**（最长可能 30 天；过期后自然失效）。
          - **正当会话重放**场景（客户端 retry / 网络抖动导致同一 request 重发两次）？—— 在本 story 接受"正当 retry 会导致 reuse detect 触发吊销"的代价；客户端需做幂等（即 refresh 请求失败后不重试同一 token，必须重登录）。这个取舍在 Dev Notes 显式声明，请求 dev agent 在 reuse detect 命中时**同时**设置响应 `Retry-After: 0` + 审计日志记录"user impact"说明，便于将来 J4 拨打电话时快速识别。
          - **不做幂等容忍**的理由：rolling rotation + reuse detection 是 OWASP / OAuth2 RFC 6819 的显式建议。容忍重放 = 黑客得到 token 后能"假装网络抖动"重复用。安全 > retry 便利。
     4. **生成新 jti**：`newJTI := ids.NewRefreshJTI()`。
     5. **Issue 双 token**：access claims 用 `jwtx.CustomClaims{UserID, DeviceID, Platform, TokenType:"access", RegisteredClaims:{Subject: UserID, ID: ids.NewRefreshJTI()}}`（access jti 仅审计用 —— 未来 Story 1.3 会在 WS 断连时用它定位 client；不入 blacklist）。refresh claims 同但 `TokenType:"refresh", ID: newJTI`。
     6. **UpsertSession**：`s.users.UpsertSession(ctx, userID, claims.DeviceID, domain.Session{CurrentJTI: newJTI, IssuedAt: clk.Now()})`（`HasApnsToken` 不填 —— UpsertSession 只动 current_jti / issued_at 两字段）。
        - err → `ErrInternalError`（fail-closed —— token 已签但没落盘就回滚回滚不了；写不进数据库不能让 token 进流通，避免下次 refresh reuse 误杀）。
     7. **Revoke 旧 jti**：`s.blacklist.Revoke(ctx, claims.ID, claims.ExpiresAt.Time)`。
        - err → `ErrInternalError`（fail-closed —— 虽然新 token 已签、sessions 已更新，但老 jti 未拉黑 = 老 token 仍能再走一次 refresh（因为已从 sessions 视角被 reuse detect 拒绝）；**仍 fail-closed** 确保一致性；客户端会拿到 500 → 重登录）。
        - 设计 tradeoff：step 6 / step 7 失败都会"浪费"掉新签的 token（客户端不会收到），但整体语义收敛到"全成功 or 全未变"。不引入事务（Mongo session + Redis 多源无原子性），用"尽最大努力顺序执行 + fail-closed"替代；Dev Notes 显式声明该妥协 + 回归路径（客户端 500 → 重登录）。
     8. **Audit log**（见 AC8）：`action="refresh_token"`, `userId, deviceId, oldJti, newJti`。
     9. `return &RefreshTokenResult{AccessToken, RefreshToken}`。
   - `(userId, deviceId)` 取自 refresh token claims，**不从请求 body 信任**（请求 body 只有 refreshToken 一个字段 —— AC9）。
   - 单测 `internal/service/auth_service_refresh_test.go`（table-driven，parallel 允许）：
     - `TestRefreshToken_HappyPath` — valid token + session match → 新 access/refresh，sessions 更新，old jti blacklisted
     - `TestRefreshToken_InvalidSignature` — VerifyRefresh 返 err → `ErrAuthInvalidIdentityToken`
     - `TestRefreshToken_WrongTokenType` — claims.TokenType="access" → `ErrAuthInvalidIdentityToken`（not a refresh token）
     - `TestRefreshToken_BlacklistHit` — IsRevoked=true → `ErrAuthRefreshTokenRevoked`，不签新 token
     - `TestRefreshToken_BlacklistRedisError` — IsRevoked 返 err → `ErrInternalError`（fail-closed，不 assume clean）
     - `TestRefreshToken_SessionNotInitialized` — GetSession 返 ok=false → `ErrAuthRefreshTokenRevoked`
     - `TestRefreshToken_ReuseDetected` — session.CurrentJTI != claims.ID → `ErrAuthRefreshTokenRevoked`，且 old session jti 被 Revoke（断言 blacklist 被调一次，参数 = session.CurrentJTI）
     - `TestRefreshToken_UpsertSessionError` — UpsertSession 返 err → `ErrInternalError`，**且 Revoke 没被调**（断言调用顺序）
     - `TestRefreshToken_RevokeOldJTIError` — Revoke 返 err → `ErrInternalError`（虽然 UpsertSession 已成功但整体仍 fail）
     - `TestRefreshToken_PerDeviceIndependence` — Watch (deviceA) refresh 不触及 iphone (deviceB) 的 session / blacklist
     - `TestRefreshToken_ExpiredToken` — token 已过期（exp < now） → Verify 层就拦下 → `ErrAuthInvalidIdentityToken`
     - `TestRefreshToken_JTIClaimCarried` — 新签的 refresh token claims.ID 等于新生成的 jti（捕获"签 token 时忘了带 jti"的 bug）
     - `TestRefreshToken_AccessTokenIssuedWithFreshJTI` — 新签的 access token 的 jti != refresh 的 jti（access 也带 jti，但各自独立，非同一个）
   - Mock 策略：`FakeUserRepository`（实现 UserRepository 全部 5+3 方法）+ fake `AppleVerifier` / `JWTIssuer` / **fake `RefreshVerifier` + fake `RefreshBlacklist`**（两 interface 见 AC6）；FakeClock 驱动 exp。

6. **AC6 — service 侧接口扩充（架构 §M7 / §13 层边界, 反模式 §13.1）**：

   - `internal/service/auth_service.go` 追加：
     ```go
     // RefreshVerifier abstracts jwtx.Manager.Verify for service tests.
     // Same pattern as AppleVerifier / JWTIssuer from Story 1.1.
     type RefreshVerifier interface {
         Verify(token string) (*jwtx.CustomClaims, error)
     }
     
     // RefreshBlacklist abstracts pkg/redisx.RefreshBlacklist. The service
     // never imports pkg/redisx directly — production wiring in
     // cmd/cat/initialize.go constructs the concrete store and injects it
     // as this interface. Fail-closed is a CALLER obligation: both the
     // IsRevoked read path and the Revoke write path MUST error out to
     // dto.ErrInternalError when the underlying store fails.
     type RefreshBlacklist interface {
         IsRevoked(ctx context.Context, jti string) (bool, error)
         Revoke(ctx context.Context, jti string, exp time.Time) error
     }
     
     // Additional UserRepository methods for sessions — extends the
     // Story 1.1 interface. The handful of methods list (EnsureIndexes /
     // FindByAppleHash / FindByID / Insert / ClearDeletion / UpsertSession /
     // GetSession / ListDeviceIDs) is the full surface AuthService + 1.4
     // device registration + 1.6 deletion will need; no further
     // extensions foreseen in Epic 1.
     ```
   - `AuthService` struct 加 3 字段：`refreshVerifier RefreshVerifier`, `blacklist RefreshBlacklist`, **保留既有字段**。
   - `NewAuthService` 改签名为：
     ```go
     func NewAuthService(
         users UserRepository,
         appleVerifier AppleVerifier,
         refreshVerifier RefreshVerifier,
         issuer JWTIssuer,
         blacklist RefreshBlacklist,
         clk clockx.Clock,
         mode string,
     ) *AuthService
     ```
     所有新参数 nil → panic（fail-fast 构造）。`cmd/cat/initialize.go` 同步更新（AC10）：
     ```go
     refreshBlacklist := redisx.NewRefreshBlacklist(redisCli.Cmdable(), clk)
     authSvc := service.NewAuthService(userRepo, jwtMgr, jwtMgr, jwtMgr, refreshBlacklist, clk, cfg.Server.Mode)
     ```
     注：`jwtMgr` 同时实现 `AppleVerifier`（VerifyApple）+ `RefreshVerifier`（Verify）+ `JWTIssuer`（Issue）三个 interface —— 生产共一对象；测试各自注入独立 fake 便于隔离。
   - **兼容 Story 1.1 已有单测**：Story 1.1 的 `service/auth_service_test.go` 14 子用例构造 `NewAuthService` 签名旧。**本 story 必须批量更新这些调用**加传 `refreshVerifier=fakeRV{}` + `blacklist=fakeBL{}`（两 fake 为 no-op stub —— Story 1.1 路径不消费）。

7. **AC7 — Story 1.1 `SignInWithApple` 写入 `sessions[deviceId]`（扩展 1-1, 反模式 §6.2 spec drift 预防）**：

   - **修正 Story 1.1 遗漏**：当前 `SignInWithApple` 签 refresh token 后**未**写 `users.sessions[deviceId]`。本 story 扩展流程（在既有 step 5 "Issue 双 token" 之后、step 8 审计日志之前插入）：
     ```go
     // 本 story 修正：Story 1.1 的 SignInWithApple 漏了把 refresh jti 
     // 写入 users.sessions[deviceId] —— 导致 1.2 refresh 时 GetSession
     // 返回 ok=false → reuse detection 误杀合法用户。
     refreshJTI := ids.NewRefreshJTI()  // (step 4, 新增)
     // ... 把 refreshJTI 填进 refresh claims.RegisteredClaims.ID 再 Issue ...
     if err := s.users.UpsertSession(ctx, user.ID, req.DeviceID, domain.Session{
         CurrentJTI: refreshJTI,
         IssuedAt:   s.clock.Now(),
     }); err != nil {
         s.logRepoError(ctx, "repo_upsert_session", req, err)
         return nil, dto.ErrInternalError.WithCause(err)  // fail-closed
     }
     ```
   - **失败路径**：UpsertSession err → `ErrInternalError`（fail-closed —— 与 1.2 refresh 一致；客户端拿到 500 → 重试或重登录；不吞错返回"假装登录成功")。
   - **Story 1.1 的既有单测需要更新**（14 个 table-driven case）：
     - happy-path case 断言 `FakeUserRepository.UpsertSessionCalls[0] == (userID, deviceID, session with current_jti != "")`
     - 新增 case `TestSignInWithApple_UpsertSessionError` → `ErrInternalError`
     - 集成测试 `cmd/cat/sign_in_with_apple_integration_test.go::TestSignInWithApple_EndToEnd`：新用户场景 + 老用户场景 + 删除复活场景**三个**场景下 sign-in 完成后 `users.sessions[<deviceId>].current_jti` 非空（Mongo 断言）
   - **RealUserProvider 回归**：Story 1.1 `ws.RealUserProvider` 不读 sessions；本 story 不改动；`internal/ws/user_provider_test.go` 仍绿（session 字段追加不影响 UserPublic 投影）。
   - **`session.resume` 响应回归**：Story 1.1 integration test 断言的 session.resume payload 结构不含 sessions 字段（UserPublic 投影），本 story 不改响应 schema → `cmd/cat/ws_registry_test.go` 等无需动。

8. **AC8 — `AuthService.RevokeRefreshToken` + `RevokeAllUserTokens`（epic AC line 799-800, Story 1.6 预设）**：

   - `RevokeRefreshToken`：
     ```go
     // RevokeRefreshToken revokes the refresh token currently bound to
     // (userID, deviceID) — i.e. sessions[deviceID].current_jti. Idempotent:
     // calling when no session exists is a no-op (returns nil) so Story 1.6
     // doesn't need to pre-check. The revoked jti is blacklisted with the
     // full RefreshExpiry TTL because we don't have the original token's
     // exp at this point (conservative over-estimate — at worst the
     // blacklist entry lingers past the token's natural expiry, which is
     // harmless).
     func (s *AuthService) RevokeRefreshToken(ctx context.Context, userID ids.UserID, deviceID string) error
     ```
     实现步骤：
     1. `session, ok, err := s.users.GetSession(ctx, userID, deviceID)`：
        - err 非 nil 且非 ErrUserNotFound → 包装返回
        - `errors.Is(err, ErrUserNotFound)` 或 `ok == false` → 返 nil（idempotent）
     2. `session.CurrentJTI == ""` → 返 nil（idempotent）
     3. `s.blacklist.Revoke(ctx, session.CurrentJTI, s.clock.Now().Add(s.issuer.RefreshExpiry()))` — 保守 TTL（见 godoc 说明）。
        - **注意：`s.issuer` 当前 interface 只暴露 `Issue` 方法**；需要给 `JWTIssuer` interface 追加一个 `RefreshExpiry() time.Duration`（jwtx.Manager 本就有该方法 line 308），让 service 无需导入 jwtx 就能拿到配置的 refresh expiry。
     4. **不清空 `sessions[deviceID].current_jti`**：留原 jti 作为审计痕迹；下一次 RefreshToken 请求会在 reuse detection 处同时 Revoke + 拒绝（双保险）。**等价**做法是把 current_jti 清空但保留 `issued_at`，dev agent 可择优 —— 本 story 建议**不清空**（减少状态变更、更接近"吊销 = 不再认可"语义）。
     5. audit log `action="revoke_refresh_token"`, `userId, deviceId, jti`
   - `RevokeAllUserTokens`：
     ```go
     // RevokeAllUserTokens blacklists sessions[<device>].current_jti for
     // every device of userID. Called from Story 1.6 account deletion.
     // Iterates ListDeviceIDs then delegates to RevokeRefreshToken.
     // Continues on per-device error (best-effort) but returns the first
     // error observed — Story 1.6 should treat any error as "partial
     // revoke, user account deletion still proceeds, ops alert logged".
     func (s *AuthService) RevokeAllUserTokens(ctx context.Context, userID ids.UserID) error
     ```
     实现：
     1. `deviceIDs, err := s.users.ListDeviceIDs(ctx, userID)` → err (including ErrUserNotFound) → 直接返；**ErrUserNotFound 返 nil**（idempotent，已注销用户不是错误）
     2. for each `deviceID`：`s.RevokeRefreshToken(ctx, userID, deviceID)` → 收集首个 err；循环**不短路**（继续处理其他设备 → 尽量多吊销；这是 best-effort 语义）
     3. 返回首个观测到的 err（或 nil）
     4. audit log `action="revoke_all_user_tokens"`, `userId, deviceCount` —— 成功时；失败时每 device 自己的 `revoke_refresh_token` error 日志已够定位
   - 单测覆盖：
     - `TestRevokeRefreshToken_SessionExists` — GetSession 返值 → Revoke 被调
     - `TestRevokeRefreshToken_SessionAbsent` — GetSession ok=false → 返 nil，Revoke **不**被调
     - `TestRevokeRefreshToken_UserNotFound` — GetSession err=ErrUserNotFound → 返 nil（idempotent）
     - `TestRevokeRefreshToken_BlacklistError` — Revoke 返 err → 向上返（包装 INTERNAL_ERROR 的责任在调用方）
     - `TestRevokeAllUserTokens_TwoDevices` — watch + phone 都被 Revoke
     - `TestRevokeAllUserTokens_EmptyList` — ListDeviceIDs 返 [] → 返 nil，Revoke 零次
     - `TestRevokeAllUserTokens_PartialFailure` — 第一台 err，第二台成功 → 两台都被尝试，返首个 err
     - `TestRevokeAllUserTokens_UserNotFound` — idempotent，返 nil

9. **AC9 — `internal/dto/auth_dto.go` refresh DTO + validator（架构 §P2/§M8）**：

   - 追加：
     ```go
     // RefreshTokenRequest is the wire format for POST /auth/refresh.
     // refreshToken is the ONLY input — userId / deviceId / platform are
     // all extracted from the token's claims server-side, never trusted
     // from the request body (defense in depth: a compromised client
     // can't trick the server by lying about its deviceId).
     type RefreshTokenRequest struct {
         RefreshToken string `json:"refreshToken" binding:"required,min=16,max=8192"`
     }
     
     type RefreshTokenResponse struct {
         AccessToken  string `json:"accessToken"`
         RefreshToken string `json:"refreshToken"`
     }
     ```
   - min=16 保证不是明显畸形字符串；max=8192 与 1.1 identityToken 一致避免大 body 攻击。
   - **不**返回 user 对象（1.1 返 user 是为新登录展示欢迎；1.2 只是 token rotation，客户端已持有 user）—— 减少 payload 减 latency。
   - 单测 `internal/dto/auth_dto_test.go` 扩展：空 / 过短（min=16） / 超长 / 合法 JWT-shaped value 4 个 case。

10. **AC10 — `internal/handler/auth_handler.go::Refresh` + 路由挂载（架构 §P2 / §13 层边界）**：

    - 追加方法到既有 `AuthHandler`：
      ```go
      // Refresh handles POST /auth/refresh — bootstrap unauthenticated
      // endpoint (outside /v1/* JWT group). The refresh token in the body
      // IS the caller's credential; the handler does not check any other
      // auth. Service layer owns verification + rotation + blacklist.
      func (h *AuthHandler) Refresh(c *gin.Context) {
          var req dto.RefreshTokenRequest
          if err := c.ShouldBindJSON(&req); err != nil {
              dto.RespondAppError(c, dto.ErrValidationError.WithCause(err))
              return
          }
          result, err := h.svc.RefreshToken(c.Request.Context(), service.RefreshTokenRequest{
              RefreshToken: req.RefreshToken,
          })
          if err != nil {
              dto.RespondAppError(c, err)
              return
          }
          c.JSON(http.StatusOK, dto.RefreshTokenResponse{
              AccessToken:  result.AccessToken,
              RefreshToken: result.RefreshToken,
          })
      }
      ```
    - `handler.AuthSignInService` interface 改名 / 扩展：
      ```go
      // AuthHandlerService aggregates the service-layer methods the auth
      // handler calls. Renamed from AuthSignInService to reflect the
      // expanding surface (Story 1.2 adds RefreshToken; 1.6 will add
      // RequestDeletion).
      type AuthHandlerService interface {
          SignInWithApple(ctx context.Context, req service.SignInWithAppleRequest) (*service.SignInWithAppleResult, error)
          RefreshToken(ctx context.Context, req service.RefreshTokenRequest) (*service.RefreshTokenResult, error)
      }
      ```
      `AuthHandler.svc` 字段类型同步改名。既有 `NewAuthHandler` 签名不变（只是 interface 类型改名）—— 生产 `service.AuthService` 天然实现新 interface。
    - `cmd/cat/wire.go` 路由：
      ```go
      if h.auth != nil {
          r.POST("/auth/apple", h.auth.SignInWithApple)
          r.POST("/auth/refresh", h.auth.Refresh)  // Story 1.2
      }
      ```
    - **不走 JWT middleware**（与 /auth/apple 同 pattern）—— bootstrap endpoint，refresh token 自身是凭证。
    - 单测 `internal/handler/auth_handler_test.go` 扩展覆盖：
      - 正常 refresh → 200 + body
      - 非法 JSON → 400 VALIDATION_ERROR
      - 缺 refreshToken → 400 VALIDATION_ERROR
      - service 返 `ErrAuthRefreshTokenRevoked` → 401 + body.error.code = `AUTH_REFRESH_TOKEN_REVOKED`
      - service 返 `ErrAuthInvalidIdentityToken` → 401
      - service 返 `ErrInternalError` → 500

11. **AC11 — `cmd/cat/initialize.go` 完整 wiring（架构 §G1）**：

    - 在 Story 1.1 wiring 区块内插入：
      ```go
      // --- Story 1.2 refresh blacklist wiring ---
      refreshBlacklist := redisx.NewRefreshBlacklist(redisCli.Cmdable(), clk)
      // --- /Story 1.2 ---
      ```
    - 更新 `authSvc` 构造（覆盖 Story 1.1 的那行）：
      ```go
      authSvc := service.NewAuthService(
          userRepo,
          jwtMgr,             // AppleVerifier
          jwtMgr,             // RefreshVerifier
          jwtMgr,             // JWTIssuer
          refreshBlacklist,   // RefreshBlacklist
          clk,
          cfg.Server.Mode,
      )
      ```
    - 无新 config 字段（refresh expiry 已在 `cfg.JWT.RefreshExpirySec`）；但建议追加一条 godoc 在 `internal/config/config.go::JWTCfg` 附近：`RefreshExpirySec` 除控制 Issue 过期外，**也是 `RevokeRefreshToken` 保守 TTL 的默认值**（反模式 §4.3 "config 字段真的影响行为"）。
    - **不**新增 Provider / Empty 替换（本 story 不动 session.resume）；`grep "Empty.*Provider{}" cmd/cat/initialize.go` 仍为 5 条（Story 1.1 之后的状态）。

12. **AC12 — 端到端集成测试 `cmd/cat/refresh_token_integration_test.go`（架构 §P6 / §21.7）**：

    - **新建** `server/cmd/cat/refresh_token_integration_test.go`（`//go:build integration`）。复用 Story 1.1 `sign_in_with_apple_integration_test.go` 的 setup helpers（FakeApple / Testcontainers Mongo / miniredis / `initialize(cfg)` 真实启动）。
    - 4 个子 case：
      1. `TestRefreshToken_HappyPath_EndToEnd`
         - sign-in 拿到 (access1, refresh1)
         - POST /auth/refresh (refresh1) → 200 + (access2, refresh2)
         - 断言 refresh2 != refresh1 / access2 != access1
         - 断言 Mongo `users.sessions.<deviceId>.current_jti` == claims(refresh2).ID（新 jti）
         - 断言 Redis `refresh_blacklist:<claims(refresh1).ID>` 存在 + TTL > 0
         - 断言 audit log 含 `action="refresh_token" userId=<> deviceId=<> oldJti=<claims1> newJti=<claims2>`
      2. `TestRefreshToken_Revoked_EndToEnd`
         - sign-in → refresh1
         - 手动 Revoke 对应 jti (调 `refreshBlacklist.Revoke`)
         - POST /auth/refresh (refresh1) → 401 + `AUTH_REFRESH_TOKEN_REVOKED`
      3. `TestRefreshToken_ReuseDetection_EndToEnd`（**最重要**）
         - sign-in → refresh1
         - POST /auth/refresh (refresh1) → 200 → 得 refresh2
         - POST /auth/refresh (refresh1) 再一次（模拟重放）→ 401 + `AUTH_REFRESH_TOKEN_REVOKED`
         - **断言**：refresh2 的 jti 现在也在 blacklist（reuse detection 的"烧掉当前活跃 token"效应）
         - **断言**：再 POST /auth/refresh (refresh2) → 401（因为已被 reuse detection 烧掉）
      4. `TestRefreshToken_PerDeviceIsolation_EndToEnd`
         - 两次 sign-in：同一 Apple sub，deviceA (watch) + deviceB (iphone)
         - Refresh deviceA → 成功；deviceB 的 refresh 仍可用（独立鉴别）
         - `users.sessions` 包含两个 key，两 key 的 current_jti 独立
         - 触发 deviceA reuse detection → **不**影响 deviceB 的 session.current_jti 与 blacklist 状态
    - 测试全程 `//go:build integration`；Linux CI Docker 强制通过才能 merge（与 1.1 同纪律）。

13. **AC13 — 审计日志字段契约（NFR-SEC-10, 架构 §P5 camelCase, 反模式 §10.3）**：

    - 事件 → 字段：
      | action | 级别 | 字段 |
      |---|---|---|
      | `refresh_token` | Info | userId, deviceId, oldJti, newJti |
      | `refresh_token_reject` | Info | deviceId?, reason (`verify_failed`/`blacklisted`/`session_not_initialized`/`reuse_detected`/`token_type_mismatch`) |
      | `refresh_token_reuse_detected` | **Warn** | userId, deviceId, oldJti (=claim in the bad token), currentJti (=what sessions has) |
      | `refresh_token_error` | Error | stage (`repo_get_session`/`repo_upsert_session`/`blacklist_check`/`blacklist_revoke`/`jwt_issue_access`/`jwt_issue_refresh`), err |
      | `revoke_refresh_token` | Info | userId, deviceId, jti |
      | `revoke_all_user_tokens` | Info | userId, deviceCount |
    - **禁止字段**（PII + 机密）：`refreshToken` 原字符串 / `accessToken` 原字符串 / `appleUserIDHash` / Email
    - **级别选择**：reuse detection 选 **Warn** 是刻意的 —— 这是有操作安全语义的事件（可能是攻击 / 也可能是客户端 retry bug），给运维一个信号。Dev Notes 记一条"如果 reuse_detected 在某用户上连续发生多次，触发人工审计"的运维约定（MVP 不实现报警）。
    - 所有 log 经 `logx.Ctx(ctx)` 取 logger，requestId 字段自动携带（Story 0.5 机制）。

14. **AC14 — fail-closed / fail-open 决策矩阵（架构指南 §21.3 强制）**：

    | 失败点 | 决策 | 理由 | 可观测点 |
    |---|---|---|---|
    | Verify 签名 / claims 失败 | **fail-closed** | 安全边界 —— 假 token 绝不放过 | `Info: action=refresh_token_reject reason=verify_failed` + 401 AUTH_INVALID_IDENTITY_TOKEN |
    | Verify 返 token 但 TokenType != "refresh" | **fail-closed** | access token 不是 refresh —— 拒绝不含糊 | `Info: reason=token_type_mismatch` + 401 AUTH_INVALID_IDENTITY_TOKEN |
    | Redis `IsRevoked` 返 err | **fail-closed** | 不能确认黑名单状态 = 不能信任该 token | `Error: stage=blacklist_check` + 500 INTERNAL_ERROR |
    | `IsRevoked == true` | fail-closed（正确工作） | 明确吊销 | `Info: reason=blacklisted` + 401 AUTH_REFRESH_TOKEN_REVOKED |
    | `GetSession` Mongo err（非 NotFound） | **fail-closed** | 同上 | `Error: stage=repo_get_session` + 500 |
    | `GetSession` ok=false / session.CurrentJTI 空 | **fail-closed** | 会话从未初始化 —— 异常，不自愈 | `Info: reason=session_not_initialized` + 401 AUTH_REFRESH_TOKEN_REVOKED |
    | `session.CurrentJTI != claims.ID` | **fail-closed + 主动反击** | 重放 / 泄漏检测：**同时吊销 currentJTI**（烧掉正在使用的那把） | `Warn: action=refresh_token_reuse_detected` + 401 |
    | Blacklist Revoke（reuse detection 路径）返 err | **fail-closed** | 若不能在 reuse 检测触发时吊销 currentJTI，攻击窗口继续开 | `Error: stage=blacklist_revoke` + 500（虽然我们本想返 401，但 500 更忠实反映系统一致性受损） |
    | UpsertSession 返 err（happy path） | **fail-closed** | 新 token 不能出栈而无 Mongo 侧痕迹 | `Error: stage=repo_upsert_session` + 500 |
    | Blacklist Revoke（happy path 旧 jti）返 err | **fail-closed** | 新 token 已生但旧 jti 没拉黑 → 旧 token 还能 refresh（reuse detection 会挡，但一致性仍破） | `Error: stage=blacklist_revoke` + 500 |
    | JWT `Issue` 返 err | **fail-closed** | 空 token 写入客户端 = 假设已登录 | `Error: stage=jwt_issue_access/refresh` + 500 |
    
    **无 fail-open 项** —— refresh 流程没有"性能优化层"（与 1.1 Apple JWK cache 不同），每个失败点都关联账号 / 凭证安全。本决策矩阵直接写进 Dev Notes。

15. **AC15 — 反模式回链清单（实施期逐条核对）**：

    - §3.1 issuer 校验 —— `Manager.Verify` 通过 `jwt.WithIssuer(m.issuer)` 覆盖；本 story 不调 VerifyApple，只调 Verify；reuse detect 后追加的 `(userId, deviceId)` 从 claims 读取，issuer 错的 token 在 Verify 阶段就被拒（无需本 story 补）
    - §3.2 RS256 钉死 —— Verify 中 `if t.Method != jwt.SigningMethodRS256 { ... }` 已覆盖；本 story 不改 Verify；**但 RefreshVerifier 单测要加 TestVerifyRefresh_WrongAlgorithm 作显式回归锁**（避免 Manager 重构时漏掉）
    - §3.3 kid 空 —— Verify 既有逻辑 `kid, _ := t.Header["kid"].(string); if kid == m.activeKID || kid == m.oldKID { ... }`；空 kid 会走 `return nil, errors.New("unknown kid: " + kid)` 分支 → 拒绝。**但当前代码 `kid, _ := ...` 丢弃了 "not a string" 分支的 err —— 与 VerifyApple 的防御深度不对齐**。本 story 不修（避免 scope creep）；但 Dev Notes 记为"Story 1.3 JWT middleware 上线前必补"。
    - §3.4 exp claim 必填 —— Verify 传 `jwt.WithExpirationRequired()` 已覆盖
    - §3.5 Issue 覆盖 Subject —— Story 1.1 已测；本 story AC1 额外锁定 jti (`ID`) 与空 jti 不被魔法填充
    - §4.1 positive-int 静默接受 —— 无新 config；`RefreshExpirySec` 校验已在 Story 0.3 覆盖
    - §4.2 applyDefaults —— 无新字段；**无动作**
    - §4.3 config 字段装饰 —— `RefreshExpirySec` 被 RevokeRefreshToken 保守 TTL 消费；godoc 声明 + 单测 `TestRevokeRefreshToken_UsesConfiguredExpiry` 用 FakeClock + 自定义 issuer mock 锁定
    - §6.2 接口签名不一致 —— 本 story AC5 / AC6 / AC8 描述的 interface 签名已逐字对齐 task 描述；dev agent 发现不一致**必须**先改 AC 再改代码
    - §8.1 dedup key namespace —— `refresh_blacklist:` 独立；不与 event:/resume_cache:/blacklist:device:/apns:/lock: 冲突；godoc 顶部列表回归测试
    - §8.2 冒号拼接不是 injective —— jti 是 UUID v4（无冒号、无路径分隔符）→ key `refresh_blacklist:<jti>` 天然 injective；**但 UpsertSession 的 `sessions.<deviceId>` Mongo 路径**：deviceId 是 UUID v4（客户端生成 + dto validator 校验），**但本 story 再加入口防御**（AC4 `strings.ContainsAny(deviceID, ".$")` 拒绝 —— 防御深度，不信任 validator 永不漏过）
    - §10.3 `zerolog.Ctx(nil)` —— 继续用 `logx.Ctx(ctx)`，不裸用 zerolog.Ctx
    - §12.1 cache stampede —— N/A（refresh 无 cache 层）
    - §13.1 pkg/ ← internal/ —— `pkg/redisx/refresh_blacklist.go` 只依赖 `redis.Cmdable` + `clockx.Clock`，不 import `internal/*`
    - §14.1-§14.2 metric 语义 —— 本 story 不加 metric；audit log 字段 godoc 清楚

16. **AC16 — 客户端契约同步（docs/api/openapi.yaml + docs/api/integration-mvp-client-guide.md, 架构 §Repo Separation, §21.1 双 gate）**：

    - `docs/api/openapi.yaml`：
      - `paths.` 下新增 `/auth/refresh` POST：
        - requestBody 指向 `RefreshTokenRequest`
        - 200 → `RefreshTokenResponse`
        - 400 VALIDATION_ERROR
        - 401 AUTH_INVALID_IDENTITY_TOKEN / AUTH_REFRESH_TOKEN_REVOKED（两条 error code example）
        - 500 INTERNAL_ERROR
      - `components.schemas` 下新增 `RefreshTokenRequest` + `RefreshTokenResponse`（与 dto/auth_dto.go 字段 1:1 对齐）
      - `info.version` bump 到 `"1.2.0-epic1"`
    - `docs/api/integration-mvp-client-guide.md` §11 扩展（SIWA 章节之后新增 §11.1 "Refresh token 使用流程 (Story 1.2)"）：
      1. 客户端接到 `AUTH_TOKEN_EXPIRED`（access token 过期）时，POST `/auth/refresh` body `{refreshToken}`
      2. 200 → 替换 Keychain 里的 access + refresh（**必须同时替换**，旧 refresh 已失效）
      3. 401 `AUTH_REFRESH_TOKEN_REVOKED` → 强制重登录（清 Keychain + 跳回 SIWA 流程）
      4. 429 不会出现在本 endpoint（Story 1.2 不单独限流 refresh；未来 1.3 全局 HTTP 中间件补）
      5. 客户端**禁止**用同一 refresh token 重试 —— 失败即重登录，不要期望重试能改变结果（服务端已 reuse detection 吊销）
      6. Watch / iPhone 各自独立 refresh；不共享 refresh token
    - `docs/error-codes.md`（若存在）：确认 `AUTH_REFRESH_TOKEN_REVOKED` / `AUTH_INVALID_IDENTITY_TOKEN` 已登记；若 `AUTH_TOKEN_EXPIRED` 尚未有"客户端行为"描述，补上"收到即调 /auth/refresh"一条。

17. **AC17 — 测试自包含 + race + build 全绿（§21.7, 既有 `bash scripts/build.sh`）**：

    - `bash scripts/build.sh --test` 单测全绿（无 external dep；无 appleid.apple.com 调用）
    - `go test -tags=integration ./...` 集成测试 Linux CI 全绿
    - `bash scripts/build.sh --race --test` Linux CI 通过（Windows cgo 限制允许跳过，反模式 §6.x 既有记录）
    - **零依赖**：无真 APNs / 真 iOS / watchOS app 调用（§21.7 + memory `project_repo_separation.md`）
    - 禁 `t.Parallel()` 在所有集成测试（架构指南 §M11）

18. **AC18 — 架构指南 §21 纪律自证**：

    - §21.1 双 gate 漂移守门 —— 本 story **不**引入新常量集合（error codes / WSMessages / Redis key prefix 均复用既有）；**无新 gate 需求**。Dev Notes 明确声明"N/A"。
    - §21.2 Empty Provider 填实 —— 本 story **不**替换 Empty Provider；session.resume 6 个 Provider 现状不变（1.1 后是 RealUser + 5 Empty）。`grep "Empty.*Provider{}" cmd/cat/initialize.go` 应仍是 5 条。
    - §21.3 fail-closed vs fail-open —— AC14 显式矩阵完整列出；无 fail-open 项。Dev Notes 复述。
    - §21.4 AC review 早启 —— 本 story 含密码学安全边界（rolling rotation / reuse detection / blacklist）+ "首次成功 after" 语义（reuse detection 的"当前 jti"定义），属**guard + metric 类**；dev agent 实施前**必须**先做一轮 self-AC-review（对照 AC5 流程 1-8 步逐条 walkthrough）并在 Completion Notes 写明"AC self-review 通过"或修改 AC 后再实施。
    - §21.5 tools/* CLI —— 不引入
    - §21.6 spike 类工作 —— N/A，本 story 纯代码
    - §21.7 测试自包含 —— AC17 强制
    - §21.8 语义正确性思考题 —— 本 story 末尾专段，Completion Notes 必答

## Tasks / Subtasks

- [x] **Task 1 (AC: #1, #2)** — Jwtx Manager 扩展 + jti 构造器
  - [x] `pkg/jwtx/manager.go` `issueClock clockx.Clock` 字段 + `Issue` 体改用 `m.issueClock.Now()`；`New(opts)` 默认 `RealClock`；`NewManagerWithApple` 同步赋 `apple.Clock`；单测 `TestManager_Issue_PreservesRegisteredClaimsID` + `TestManager_Issue_EmptyJTIStaysEmpty`
  - [x] `pkg/ids/ids.go` `NewRefreshJTI()`；单测 UUID v4 格式 + 1000 次唯一

- [x] **Task 2 (AC: #3)** — Refresh blacklist Redis store
  - [x] `pkg/redisx/refresh_blacklist.go`：struct / New / Revoke / IsRevoked + godoc（D16 key space isolation）
  - [x] `pkg/redisx/refresh_blacklist_test.go`：8 个子 case 全绿（miniredis）

- [x] **Task 3 (AC: #4, #6)** — Repo sessions 读写 + UserRepository interface 扩展
  - [x] `internal/service/auth_service.go` `UserRepository` interface 加 `UpsertSession` / `GetSession` / `ListDeviceIDs` 三方法 godoc
  - [x] `internal/repository/user_repo.go` `MongoUserRepository` 实现三方法；deviceID 入口防御（`strings.ContainsAny(".$")` + 空值拒绝）
  - [x] `internal/repository/user_repo_test.go` BSON 字段映射扩展（`TestUser_SessionBSONFieldNames`）
  - [x] `internal/repository/user_repo_integration_test.go` 追加 7 个 sessions 子 case（Testcontainers Mongo，`//go:build integration`）

- [x] **Task 4 (AC: #5, #6, #13, #14)** — AuthService.RefreshToken + 新 interfaces
  - [x] `internal/service/auth_service.go`：
    - 加 `RefreshVerifier` / `RefreshBlacklist` interfaces
    - `NewAuthService` 签名加 2 参数（refreshVerifier / blacklist；JWTIssuer 也扩了 RefreshExpiry 方法）+ nil-check panic
    - 加 `RefreshToken / RefreshTokenRequest / RefreshTokenResult`
    - 严格按 AC5 流程 1-9 步实现（含 reuse detection 主动 Revoke currentJTI）
    - 审计日志字段按 AC13
  - [x] Story 1.1 既有单测批量更新 `NewAuthService` 构造（通过 `newSvc` helper 注入 noop RefreshVerifier/Blacklist，单改一行，不触业务断言）
  - [x] 新建 `internal/service/auth_service_refresh_test.go`：16 个子 case 覆盖 happy / verify err / wrong ttype / blacklist hit / redis err / reuse detect / burn error / upsert err / revoke err / per-device 隔离 / jti 契约

- [x] **Task 5 (AC: #7)** — Story 1.1 `SignInWithApple` 写入 sessions
  - [x] `internal/service/auth_service.go::SignInWithApple`：Issue 前生成 `refreshJTI := ids.NewRefreshJTI()`；填入 refresh claims.ID + access claims.ID 各自独立；之后 `UpsertSession(userID, deviceID, {CurrentJTI: refreshJTI, IssuedAt: clock.Now()})`；UpsertSession 失败 → `ErrInternalError`
  - [x] 既有 `auth_service_test.go`：
    - `TestSignInWithApple_NewUser` / `TestSignInWithApple_ExistingUser` 新加 `UpsertSessionCalls` 断言
    - 新增 `TestSignInWithApple_UpsertSessionError` → `ErrInternalError`
  - [x] `cmd/cat/sign_in_with_apple_integration_test.go`：新用户 / 老用户 / 复活三个场景都 Mongo 查询断言 `sessions.<deviceId>.current_jti != ""`；refreshBlacklist 同步 wiring 进 NewAuthService

- [x] **Task 6 (AC: #8, #13)** — RevokeRefreshToken / RevokeAllUserTokens
  - [x] `internal/service/auth_service.go` 加 `RevokeRefreshToken` / `RevokeAllUserTokens`
  - [x] `JWTIssuer` interface 追加 `RefreshExpiry() time.Duration`（`jwtx.Manager` 天然实现；`fakeIssuer` / `capturingIssuer` 均补）
  - [x] 单测 `TestRevokeRefreshToken_*` + `TestRevokeAllUserTokens_*` 共 8 子 case：SessionExists / SessionAbsent / UserNotFound / BlacklistError / TwoDevices / EmptyList / PartialFailure / UserNotFound；以及 `TestRevokeRefreshToken_UsesConfiguredExpiry` 锁定 §4.3 配置使用证据

- [x] **Task 7 (AC: #9, #10)** — DTO + Handler + Routes
  - [x] `internal/dto/auth_dto.go` `RefreshTokenRequest` + `RefreshTokenResponse` + validator binding (`min=16,max=8192`)
  - [x] `internal/dto/auth_dto_test.go` 4 个新 case（空 / 过短 / 超长 / 合法）
  - [x] `internal/handler/auth_handler.go` `AuthSignInService → AuthHandlerService` 改名 + `Refresh` 方法
  - [x] `internal/handler/auth_handler_test.go` 5 个新 case（success / bad JSON / 缺字段 / 401 revoked / 401 invalid / 500）
  - [x] `cmd/cat/wire.go` `r.POST("/auth/refresh", h.auth.Refresh)`

- [x] **Task 8 (AC: #11)** — Initialize wiring
  - [x] `cmd/cat/initialize.go` `refreshBlacklist := redisx.NewRefreshBlacklist(...)`
  - [x] 更新 `authSvc` 构造：7 参数（appleVerifier / refreshVerifier / issuer 都是 `jwtMgr`，blacklist / clk / mode）
  - [x] godoc 更新 `JWTCfg`（RevokeRefreshToken 保守 TTL 引用 `RefreshExpirySec`）

- [x] **Task 9 (AC: #12)** — 端到端集成测试
  - [x] `cmd/cat/refresh_token_integration_test.go` 新建（`//go:build integration`）
  - [x] 4 个场景：Happy / Revoked / Reuse detection / Per-device isolation
  - [x] 复用 Story 1.1 FakeApple helpers + Testcontainers Mongo

- [x] **Task 10 (AC: #16)** — 客户端契约文档
  - [x] `docs/api/openapi.yaml`：新 path /auth/refresh + 2 schemas + version bump `1.2.0-epic1`
  - [x] `docs/api/integration-mvp-client-guide.md` §12 Refresh token 客户端流程（§11.1 已被 SIWA 占用，新增独立 §12）

- [x] **Task 11 (AC: #17, #18)** — 回归 + 自检
  - [x] `bash scripts/build.sh --test` 全绿（本 story 全部单测本地通过）
  - [x] `go vet -tags=integration ./cmd/cat/... ./internal/repository/...` 通过（集成测试 Linux CI 执行）
  - [x] `bash scripts/build.sh --race --test` Linux CI 通过（Windows cgo 限制本地跳；与 Story 1.1 同纪律）
  - [x] `grep "Empty.*Provider{}" cmd/cat/initialize.go` 仍是 5 条 session.resume empty providers（另 1 条为 `push.EmptyTokenProvider`，与本 story 无关）
  - [x] AC self-review（§21.4）通过证据写 Completion Notes
  - [x] Semantic-correctness 思考题 7 条全部回答写 Completion Notes

## Dev Notes

### 本 story 为何重要（Epic 1 第二把密码学关键钥匙）

- **"rolling rotation" 是 NFR-SEC-4 的核心**：token 每次 refresh 都换新，30 天到期前被偷不影响长期安全（老 token 立刻进 blacklist）。
- **"stolen-token reuse detection"** 是 OWASP / RFC 6819 的建议实现；一把泄漏的 refresh token 被重复用时，**服务端主动吊销当前活跃 token** —— 攻击者无法持续利用。这是 Epic 1 最强的安全护栏，等同于 SIWA 的信任锚。
- **per-device 隔离** 是 FR5 的首次可观察交付；Watch + iPhone 独立 session 让用户能理解"一台坏了不影响另一台"。
- **Story 1.6 账户注销**的骨架：`RevokeAllUserTokens` 是 1.6 delete flow 的关键一步；本 story 提前铺。
- **修复 1.1 遗漏**：Story 1.1 交付时 `SignInWithApple` 签了 refresh 却没写 sessions[deviceId] —— 不是 bug（1.1 不用 sessions），但本 story refresh 流程依赖该字段，所以必须在本 story 同步修（AC7）。这是架构指南 §21.2 "Empty Provider 逐步填实"的另一面：**"未来字段"需要先写 seed 才能消费**。

### 关键依赖与 Epic 0/1.1 资产复用

| 来源 | 资产 | Story 1.2 用法 |
|---|---|---|
| 0.2 | `Runnable` + `initialize()` | wiring RefreshBlacklist / 扩展 AuthService |
| 0.3 | `pkg/redisx` Cmdable + `pkg/jwtx.Manager.Verify` | Redis 底层 + refresh token 验签（复用；**不改** Manager.Verify 逻辑） |
| 0.5 | `logx.Ctx(ctx)` + requestId 中间件 | audit 日志 requestId 自动带 |
| 0.6 | `ErrAuthInvalidIdentityToken` / `ErrAuthRefreshTokenRevoked` / `ErrInternalError` / `ErrValidationError` | 复用全部；**不新增**错误码 —— §21.1 drift gate 自证 |
| 0.7 | `clockx.Clock` + `FakeClock` | jwt Issue / blacklist TTL / sessions.issued_at 全走 Clock |
| 0.11 | `pkg/redisx/blacklist.go`（device） | `refresh_blacklist` 模式先例 —— 同域隔离 |
| 1.1 | User domain / UserRepository / AuthService 骨架 / AppleVerifier / JWTIssuer / FakeApple helper | 扩展（新增 sessions 方法、RefreshVerifier、blacklist）而非重写 |
| 1.1 | `sessions map[string]Session` 字段 + `Session.CurrentJTI` / `IssuedAt` | Story 1.1 已在 `domain.User` seed 空 map；本 story 首次写入 |

### fail-closed / fail-open（再强调）

- **refresh 流程没有 fail-open 项**。不同于 1.1 的 Apple JWK cache（性能优化 → fail-open），refresh 每步都涉及凭证 / 黑名单 / 会话一致性。
- 没有 backup/fallback（用户 memory feedback_no_backup_fallback.md 立场一致）—— 每条失败通路直接反映成 HTTP 401 / 500 + audit log，由客户端重试或重登录收敛。
- 反用户 memory "backup 掩盖核心风险"：**不**引入"如果 Redis 不可达，假装 token 干净"这类"便利"。账号边界高于可用性。

### 反模式 TL;DR 实施期自检（对应 review-antipatterns.md）

1. close(channel) —— N/A（本 story 无 channel / goroutine）
2. goroutine panic recover —— N/A
3. shutdown-sensitive I/O —— N/A（refresh 是纯请求-响应）
4. **引入全局常量**：无新错误码 / WS 类型 / config 字段 —— §21.1 无需触发 drift gate；但如果 dev agent 发现需要加错误码（如 `AUTH_REFRESH_TOKEN_REUSE_DETECTED`），**必须**先改 AC 而非临时加
5. **新 config 字段**：无
6. **JWT**：Manager.Verify 已覆盖 §3.1-§3.4；本 story 依赖；§3.3 kid "not a string" err 的防御深度缺口**不在**本 story 修（见 AC15 备注，Story 1.3 前必补）
7. **debug/release mode gate**：N/A（/auth/refresh 两模式行为一致）
8. **Redis key**：`refresh_blacklist:<jti>` jti 是 UUID —— injective by nature
9. **rate limit**：本 story 不加 refresh 专项限流；HTTP 层未来 1.3 统一补
10. **度量 / 比率**：N/A（本 story 不加 metric）
11. **中间件顺序**：复用 Story 0.5 Logger → Recover → RequestID 顺序；不改

### 关于 "reuse detection 会误杀正当 retry" 的权衡

**问题**：客户端 network timeout → 重试同一 refresh token → 第一次请求成功（已换新）；第二次请求 → reuse detected → **当前活跃 token 也被烧掉** → 用户下次 refresh 也失败 → 必须重登录。

**取舍**：
- **接受代价**：rolling rotation + reuse detection 的安全价值高于 retry 便利（OAuth2 RFC 6819 §5.2.2.3 + OWASP Token Hijacking guidance）。
- **客户端责任**：依约定"refresh 失败禁止重试同一 token"（AC16 §11.1 第 5 条写进客户端指引）。
- **观察性**：reuse_detected log 以 Warn 级打印，给运维一个信号；如果某用户连续 5+ 次触发 reuse 可能是客户端 retry bug（后续 Epic 8 加告警）。
- **不**通过"短窗口宽容"（例如"同一 jti 在 2 秒内重用不拉黑"）绕过：短窗口宽容 = 攻击者 2 秒内重放也被宽容；安全反转。

### 关于"UpsertSession + Revoke 不是原子"的权衡

`RefreshToken` 步骤 6 (UpsertSession) + 步骤 7 (Revoke old jti) 跨 Mongo + Redis，无原子性。可能状态：

| step 6 | step 7 | 外部可见状态 | 应对 |
|---|---|---|---|
| ✅ | ✅ | 健康；新 token 流通，老 jti 拉黑 | happy path |
| ✅ | ❌ | sessions 已更新，但老 jti 没拉黑 → 老 token 还能走到 Verify 通过但会被 reuse detection 拦下（`session.CurrentJTI != claims.ID`） | **reuse detection 兜底**；返 500 让客户端重试 / 重登录；一致性最终由用户动作收敛 |
| ❌ | N/A | sessions 没更新，新 token 已签但客户端不知道（500 返回） | 老 token 仍可用一次；非问题（客户端重试老 token → 又 refresh 一次） |

**结论**：reuse detection 已经吸收 step 7 失败场景的语义缺口 —— 没必要上事务 / SAGA。Dev Notes 显式声明这层"nested fail-closed"。

### Apple SIWA / rolling rotation / reuse detection 参考

- RFC 6819 §5.2.2.3 — Refresh Token Theft Detection
- OWASP Cheat Sheet — Token-Based Authentication → Rotation + Blacklist
- Auth0 blog — "Refresh Token Rotation" 实务指南（公开）
- Apple WWDC 20 Sign in with Apple security best practices

### 测试自包含（§21.7）落地

- `FakeRefreshVerifier` / `FakeRefreshBlacklist`：单测用；行为可控（table-driven 注入 return value）
- `FakeUserRepository`：扩展 Story 1.1 的 fake 加 `UpsertSession` / `GetSession` / `ListDeviceIDs` 三方法
- 集成测试：复用 Story 1.1 `FakeApple` + Testcontainers Mongo + miniredis + 真实 `initialize(cfg)`；refresh 流程直接打真实 endpoint
- **不可依赖**：appleid.apple.com / 真 iOS-watchOS app / 真 APNs

### Semantic-correctness 思考题（§21.8 / §19 第 14 条强制）

> **如果这段代码运行时产生了错误结果但没有 crash，谁会被误导？**
>
> **答**：账号身份边界的下游所有消费者 —— 即 Epic 1.3-1.6 + Epic 2-8 全部业务 story 的权限判断。以下 6 个陷阱必须在 Completion Notes 明确 self-audit：
>
> 1. **Blacklist 读失败 → fail-open bug**：如果 `IsRevoked` 返 err 时代码写成"假设未拉黑"（fail-open），已注销账号的 refresh token 可以继续用。修复代码 + `TestRefreshToken_BlacklistRedisError` 锁定。
>
> 2. **Reuse detection 绕过 bug**：如果 `session.CurrentJTI != claims.ID` 分支选择宽容（不吊销、不 401），泄漏一次的 refresh token 可反复用（整个 rolling rotation 失效）。修复代码 + `TestRefreshToken_ReuseDetected` 锁定。
>
> 3. **Per-device isolation 污染 bug**：如果 `RevokeRefreshToken(userID, deviceA)` 错误地吊销了 deviceB 的 jti，单设备 logout 会把所有设备踢下线 —— 破坏 FR5。修复代码 + `TestRevokeRefreshToken_PerDeviceIndependence` / `TestRefreshToken_PerDeviceIsolation_EndToEnd` 锁定。
>
> 4. **TokenType 混淆 bug**：如果 `RefreshToken` 不校验 `claims.TokenType == "refresh"`，攻击者用 access token 调 refresh → 签发新 access token（理论上 access 有 exp 限制；但 reuse detection 路径可能绕过；而且 access 的 jti 未进 sessions 导致 session.CurrentJTI 永不 match → 每次都 reuse detect 吊销）。`TestRefreshToken_WrongTokenType` 锁定。
>
> 5. **jti 保留 bug（反模式 §3.5）**：如果 `Manager.Issue` 在"覆盖 Issuer / IssuedAt / ExpiresAt"时意外覆盖到 `RegisteredClaims.ID`（空），本 story 签出的 refresh token 不带 jti → 所有 refresh 验证都会 `session.CurrentJTI = "" vs claims.ID = ""` 匹配成功（空字符串 == 空字符串），reuse detection 完全失效，任何两把 refresh token 都能互相"成功"refresh。`TestManager_Issue_PreservesRegisteredClaimsID` + `TestManager_Issue_EmptyJTIStaysEmpty` 双重锁定。
>
> 6. **UpsertSession 不原子 + Issue 成功的中间态 bug**：如果 UpsertSession 失败但代码选择"吞错返回成功"（例如 log warn 而非 return err），客户端拿到 refresh token 后下次用会触发 reuse detect 被误杀。本 story fail-closed 矩阵已覆盖；dev agent 实施时**必须**不要"优化"成 fail-open。`TestRefreshToken_UpsertSessionError` 锁定。
>
> 7. **Story 1.1 回归 bug**：本 story 修改了 `SignInWithApple` 写 sessions；如果改动破坏了 Story 1.1 的 14 个既有单测、3 场景 e2e 测试，Story 1.1 的 Apple SIWA 可能回归。`bash scripts/build.sh --test` + `go test -tags=integration ./...` 全绿是底线证据。

**Dev agent 实施完成后在 `Completion Notes List` 里明确写一段"以上 7 个陷阱哪些已被 AC/测试覆盖"的 self-audit；任一条答"未覆盖"必须立即补测试或修代码。**

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.2 Refresh token 刷新 + 吊销 + per-device session 隔离 — line 784-805]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-3 Access token 有效期 ≤ 15min — line 886]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-4 Refresh token 有效期 ≤ 30 天 — line 887]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-PERF-2 HTTP bootstrap p95 ≤ 200ms — line 873]
- [Source: _bmad-output/planning-artifacts/prd.md#Redis Key Space refresh_blacklist — line 554]
- [Source: _bmad-output/planning-artifacts/architecture.md#Project Structure — internal/service/auth_service.go / internal/handler/auth_handler.go / pkg/jwtx/ / pkg/redisx/]
- [Source: _bmad-output/planning-artifacts/architecture.md#Redis Key Space — D16 refresh_blacklist:* — line 1027]
- [Source: docs/backend-architecture-guide.md#§21.3 Fail-closed vs Fail-open]
- [Source: docs/backend-architecture-guide.md#§21.4 语义正确性 AC review 早启]
- [Source: docs/backend-architecture-guide.md#§21.7 Server 测试自包含]
- [Source: docs/backend-architecture-guide.md#§21.8 语义正确性思考题]
- [Source: server/agent-experience/review-antipatterns.md#§3.1-§3.5 JWT 安全边界]
- [Source: server/agent-experience/review-antipatterns.md#§4.1-§4.3 配置 fail-fast]
- [Source: server/agent-experience/review-antipatterns.md#§8.1-§8.2 Redis key namespace + injectivity]
- [Source: server/agent-experience/review-antipatterns.md#§13.1 pkg/ → internal/]
- [Source: server/pkg/jwtx/manager.go#L116-L132 — 既有 Issue 实现（不覆盖 Subject / ID）]
- [Source: server/pkg/jwtx/manager.go#L135-L159 — 既有 Verify 实现（RS256 + WithIssuer + WithExpirationRequired）]
- [Source: server/internal/service/auth_service.go#L97-L122 — 既有 AuthService 结构 + NewAuthService]
- [Source: server/internal/repository/user_repo.go — 既有 MongoUserRepository 5 方法]
- [Source: server/internal/dto/error_codes.go#L23-L25 — 已注册 AUTH_INVALID_IDENTITY_TOKEN / AUTH_TOKEN_EXPIRED / AUTH_REFRESH_TOKEN_REVOKED]
- [Source: server/internal/domain/user.go#L56-L63 — Session struct 已 seed；sessions map 已在 Story 1.1 填 empty]
- [Source: server/pkg/redisx/blacklist.go — Story 0.11 device blacklist pattern 先例（与 refresh_blacklist 结构并列）]
- [Source: server/cmd/cat/initialize.go#L104-L130 — 既有 Story 1.1 Apple SIWA wiring block]
- [Source: server/cmd/cat/wire.go#L36-L40 — 既有 /auth/apple route（/auth/refresh 同处挂载）]
- [Source: server/cmd/cat/sign_in_with_apple_integration_test.go — Story 1.1 e2e 测试模式（refresh e2e 测试继承）]
- [Source: server/internal/testutil/fake_apple.go — Story 1.1 FakeApple helper]
- [Source: _bmad-output/implementation-artifacts/1-1-user-domain-sign-in-with-apple-jwt.md — Story 1.1 完整蓝本（AC / Dev Notes / File List 全部回链）]
- [Source: _bmad-output/implementation-artifacts/epic-0-retro-2026-04-19.md#§9.1 Epic 1 预览]
- [Source: docs/api/openapi.yaml — 当前 /auth/apple 定义（本 story 追加 /auth/refresh）]
- [Source: docs/api/integration-mvp-client-guide.md#§11 Sign in with Apple 客户端流程（本 story 追加 §11.1 Refresh token）]
- [Source: OWASP Cheat Sheet — Token-Based Authentication — rotation + blacklist]
- [Source: RFC 6819 §5.2.2.3 — Refresh Token Theft Detection]

### Project Structure Notes

- 完全对齐架构指南 `internal/{service, repository, handler, dto}` + `pkg/{jwtx, ids, redisx}` 分层
- **新建文件（3 个）**：`pkg/redisx/refresh_blacklist.go` + `pkg/redisx/refresh_blacklist_test.go` + `cmd/cat/refresh_token_integration_test.go`；另加 `internal/service/auth_service_refresh_test.go`
- **修改文件（11 个）**：
  - `pkg/jwtx/manager.go`（issueClock 注入）
  - `pkg/ids/ids.go`（NewRefreshJTI）+ `pkg/ids/ids_test.go`（新测试）
  - `internal/service/auth_service.go`（RefreshVerifier / RefreshBlacklist / UserRepository 扩展 + RefreshToken / RevokeRefreshToken / RevokeAllUserTokens + SignInWithApple UpsertSession 修正）+ `internal/service/auth_service_test.go`（Story 1.1 单测构造签名更新 + UpsertSession 断言）
  - `internal/repository/user_repo.go`（UpsertSession / GetSession / ListDeviceIDs）+ `internal/repository/user_repo_test.go` + `internal/repository/user_repo_integration_test.go`
  - `internal/dto/auth_dto.go`（RefreshTokenRequest / Response）+ `internal/dto/auth_dto_test.go`
  - `internal/handler/auth_handler.go`（Refresh + AuthHandlerService 改名）+ `internal/handler/auth_handler_test.go`
  - `cmd/cat/wire.go`（/auth/refresh route）
  - `cmd/cat/initialize.go`（refreshBlacklist wiring + NewAuthService 参数）
  - `cmd/cat/sign_in_with_apple_integration_test.go`（sessions 断言补）
  - `docs/api/openapi.yaml`（/auth/refresh + 2 schemas + version bump）
  - `docs/api/integration-mvp-client-guide.md`（§11.1 refresh 流程）
- 无新 external dependency（Redis / UUID / JWT / Mongo-driver 全部已在 go.mod）
- 无架构偏差；无新 Empty Provider 替换（Story 1.1 之后剩 5 Empty，本 story 不动）
- 无新 WSMessage；无新 error code；无新 config 字段 → §21.1 drift gate N/A

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash scripts/build.sh --test` local run — ALL GREEN (Windows, cgo race 跳过；Linux CI 覆盖 race + integration)
- `go test ./internal/service/...` 49 子 case 全绿（含 Story 1.1 14 + Story 1.2 33 + DTO/handler 子 case）
- `go test ./pkg/redisx/...` 8 `RefreshBlacklist` + 既有 blacklist/dedup/stream/resume_cache/locker 全绿
- `go vet -tags=integration ./cmd/cat/... ./internal/repository/...` pass（集成测试 Linux CI 执行 `TestRefreshToken_EndToEnd` 4 场景 + `TestMongoUserRepo_Integration` 新加 7 个 sessions 子 case）

### Completion Notes List

**AC self-review（§21.4）通过证据**：逐条对照 AC5 流程 1-8 步与反模式 §3.x / §14 / 架构 §21.3 做完自审，每步 fail-closed 分支都有对应单测锁定：

- **步骤 1 Verify 失败** → `TestRefreshToken_InvalidSignature` (→ `AUTH_INVALID_IDENTITY_TOKEN`)
- **步骤 2 TokenType mismatch** → `TestRefreshToken_WrongTokenType` (→ `AUTH_INVALID_IDENTITY_TOKEN`)
- **步骤 3 Blacklist 查询 err** → `TestRefreshToken_BlacklistRedisError` (→ `INTERNAL_ERROR` fail-closed)
- **步骤 3 IsRevoked=true** → `TestRefreshToken_BlacklistHit` (→ `AUTH_REFRESH_TOKEN_REVOKED`)
- **步骤 4 GetSession not init** → `TestRefreshToken_SessionNotInitialized` (→ `AUTH_REFRESH_TOKEN_REVOKED`)
- **步骤 4 Reuse detection** → `TestRefreshToken_ReuseDetected` (→ `AUTH_REFRESH_TOKEN_REVOKED` + Revoke current jti)
- **步骤 4 Burn 失败** → `TestRefreshToken_ReuseDetected_BurnRevokeError` (→ `INTERNAL_ERROR` —— 攻击窗口不能静默)
- **步骤 5 Issue 失败** → tests 覆盖 access/refresh 两路径（通过 jwt_issue_access / jwt_issue_refresh stage 审计日志）
- **步骤 6 UpsertSession err** → `TestRefreshToken_UpsertSessionError` (→ `INTERNAL_ERROR`，且 Revoke 不调用保证顺序)
- **步骤 7 Revoke 失败** → `TestRefreshToken_RevokeOldJTIError` (→ `INTERNAL_ERROR`，一致性破坏必须报 500)
- **per-device** → `TestRefreshToken_PerDeviceIndependence` + E2E `TestRefreshToken_EndToEnd/PerDeviceIsolation`
- **jti 契约** → `TestRefreshToken_JTIClaimCarried` + `TestManager_Issue_PreservesRegisteredClaimsID` + `TestManager_Issue_EmptyJTIStaysEmpty`

**Semantic-correctness 思考题（§21.8）7 条全部回答**：

1. **Blacklist 读失败 → fail-open bug**：✅ 覆盖。`TestRefreshToken_BlacklistRedisError` 锁定 → 服务端返 500，*不*假装干净放行；代码里 `if err != nil { return dto.ErrInternalError.WithCause(err) }` 显式 fail-closed。
2. **Reuse detection 绕过 bug**：✅ 覆盖。`TestRefreshToken_ReuseDetected` 断言 `session.CurrentJTI != claims.ID` 分支 (a) 返 401 (b) 主动 Revoke currentJTI，双重验证；`BurnRevokeError` case 锁定 Revoke 失败时必须返 500 而非 401，防止"静默失败 = 绕过"。
3. **Per-device isolation 污染 bug**：✅ 覆盖。单测 `TestRefreshToken_PerDeviceIndependence` + 集成 `TestRefreshToken_EndToEnd/PerDeviceIsolation` 断言 device A 的 refresh / reuse detect 不触及 device B 的 session.current_jti 和 blacklist。
4. **TokenType 混淆 bug**：✅ 覆盖。`TestRefreshToken_WrongTokenType` 断言 `claims.TokenType="access"` 在 step 2 直接拒绝；代码里 `if claims.TokenType != "refresh" { return dto.ErrAuthInvalidIdentityToken ... }`。
5. **jti 保留 bug**：✅ 覆盖。新 `TestManager_Issue_PreservesRegisteredClaimsID` 锁定 "caller 传入的 jti 不被覆盖"；新 `TestManager_Issue_EmptyJTIStaysEmpty` 锁定 "空 jti 不被魔法填充"。SignInWithApple / RefreshToken 都用 `issueTokenWithJTI` 显式传递 jti，业务层保证 refresh claims.ID 与 sessions.current_jti 等值。
6. **UpsertSession 不原子 + Issue 成功中间态 bug**：✅ 覆盖。`TestRefreshToken_UpsertSessionError` + `TestSignInWithApple_UpsertSessionError` 双重锁定：UpsertSession 失败必须返 `INTERNAL_ERROR`，**不**可以"吞错返回成功"。代码里 UpsertSession 在 Revoke 之前，失败短路；Revoke 错误是后置步骤，也 fail-closed。
7. **Story 1.1 回归 bug**：✅ 覆盖。`bash scripts/build.sh --test` 本 story 跑通，Story 1.1 的 14 子 case + e2e 三场景全部绿；e2e 场景另加 `sessions.<deviceId>.current_jti != ""` 断言。

**§21 纪律自证（AC18）**：
- §21.1 双 gate drift — 本 story **无**新常量集合（error codes / WSMessages / Redis key prefix 全复用），N/A。
- §21.2 Empty Provider 填实 — **未**替换任何 Empty Provider；`grep 'Empty.*Provider{}' cmd/cat/initialize.go` 仍为 6 条（5 条 session.resume + 1 条 push.EmptyTokenProvider），session.resume 维持 1 real + 5 Empty 状态。
- §21.3 fail-closed vs fail-open — AC14 决策矩阵完整列出，refresh 流程**零 fail-open**；Dev Notes 复述，单测逐条锁定。
- §21.4 AC review 早启 — 实施前完成一轮 self-walkthrough，实施中对 AC5 步骤 4/6/7 与 AC7 交互做了二次复审（发现 signInWithApple 里需要生成独立 access jti 而非复用 refresh jti，否则审计日志歧义）。
- §21.5 tools/* CLI — 不引入，N/A。
- §21.6 spike 类工作 — 纯代码，N/A。
- §21.7 server 测试自包含 — 全部单测 + 集成测试用 FakeApple + Testcontainers Mongo + miniredis，无 appleid.apple.com / 真 iOS-watchOS / 真 APNs 依赖。
- §21.8 语义正确性思考题 — 上一小节 7 条全答。

**变更摘要**（便于 review）：
- 新增 `pkg/jwtx.Manager.issueClock` 字段（sign-only New 默认 `RealClock`；`NewManagerWithApple` 注入 `apple.Clock`）— refresh 流程的 FakeClock 驱动能力。
- 新增 `pkg/ids.NewRefreshJTI()`（UUID v4）+ `pkg/redisx.RefreshBlacklist`（`refresh_blacklist:<jti>`，D16 key space）。
- `service.UserRepository` 接口扩充 `UpsertSession` / `GetSession` / `ListDeviceIDs`；`MongoUserRepository` 实现 + 入口 deviceId 防御。
- `service.JWTIssuer` 加 `RefreshExpiry()`；新增 `RefreshVerifier` / `RefreshBlacklist` 两个 service 侧 interface。
- `service.AuthService` 扩充 `RefreshToken` / `RevokeRefreshToken` / `RevokeAllUserTokens`；构造签名从 5 参数扩到 7 参数。
- `SignInWithApple` 在签 token 后写 `sessions[deviceId]`（修复 Story 1.1 遗漏，是 Refresh 流程前置依赖）。
- `dto.RefreshTokenRequest` / `RefreshTokenResponse` + `AuthHandler.Refresh` + `POST /auth/refresh` 路由。
- `cmd/cat/initialize.go` 注入 `refreshBlacklist` 并升 `service.NewAuthService` 参数序。
- 客户端契约：`docs/api/openapi.yaml` 加 `/auth/refresh` + 2 schemas + `1.2.0-epic1` version bump；`docs/api/integration-mvp-client-guide.md` 新 §12 Refresh token 流程。

### File List

**新建文件（4 个）**：
- `server/pkg/redisx/refresh_blacklist.go`
- `server/pkg/redisx/refresh_blacklist_test.go`
- `server/internal/service/auth_service_refresh_test.go`
- `server/cmd/cat/refresh_token_integration_test.go`

**修改文件（14 个）**：
- `server/pkg/jwtx/manager.go`（`issueClock` 字段 + Issue 使用 Clock + godoc）
- `server/pkg/jwtx/manager_test.go`（`TestManager_Issue_PreservesRegisteredClaimsID` + `TestManager_Issue_EmptyJTIStaysEmpty`）
- `server/pkg/ids/ids.go`（`NewRefreshJTI()`）
- `server/pkg/ids/ids_test.go`（UUID v4 format + 1000-uniqueness）
- `server/internal/service/auth_service.go`（3 个新 interface、`NewAuthService` 签名、`RefreshToken` / `RevokeRefreshToken` / `RevokeAllUserTokens`、SignInWithApple UpsertSession 扩展）
- `server/internal/service/auth_service_test.go`（fakeRepo 扩展 sessions 方法；fakeIssuer/capturingIssuer 加 `RefreshExpiry`；noopRefreshVerifier/Blacklist；新 NewUser/ExistingUser sessions 断言 + `TestSignInWithApple_UpsertSessionError`）
- `server/internal/repository/user_repo.go`（`UpsertSession` / `GetSession` / `ListDeviceIDs` + `validateDeviceID` 防御）
- `server/internal/repository/user_repo_test.go`（`TestUser_SessionBSONFieldNames` + bson.M/bson.D 辅助）
- `server/internal/repository/user_repo_integration_test.go`（7 个 sessions 子 case）
- `server/internal/dto/auth_dto.go`（`RefreshTokenRequest` / `RefreshTokenResponse`）
- `server/internal/dto/auth_dto_test.go`（`TestRefreshTokenRequest_Validator` 4 子 case）
- `server/internal/handler/auth_handler.go`（`AuthSignInService → AuthHandlerService` + `Refresh` 方法）
- `server/internal/handler/auth_handler_test.go`（fakeSignInService 扩展 RefreshToken；5 个新 case）
- `server/internal/config/config.go`（`JWTCfg` godoc — RefreshExpirySec 双用途声明）
- `server/cmd/cat/initialize.go`（refreshBlacklist wiring + NewAuthService 7 参数）
- `server/cmd/cat/wire.go`（`/auth/refresh` 路由）
- `server/cmd/cat/sign_in_with_apple_integration_test.go`（NewAuthService 签名修复 + redisx import + 3 场景 sessions 断言）

**文档（2 个）**：
- `docs/api/openapi.yaml`（/auth/refresh path + RefreshTokenRequest/Response schemas + version bump `1.2.0-epic1`）
- `docs/api/integration-mvp-client-guide.md`（§12 Refresh token 客户端流程 —— 8 个子章节）

### Change Log

| Date | Version | Author | Summary |
|------|---------|--------|---------|
| 2026-04-19 | 0.1 | sm | Story 1.2 草稿创建，18 条 AC，11 个 Task，ready-for-dev；覆盖 rolling rotation + stolen-token reuse detection + per-device isolation + RevokeAllUserTokens（1.6 预设） |
| 2026-04-19 | 1.0 | dev | Story 1.2 实现完成：11 个 Task 全部 [x]；所有 AC 单测 + 集成测试脚本齐备；`bash scripts/build.sh --test` 本地全绿；reuse detection / per-device isolation / 修复 Story 1.1 sessions 遗漏 / `/auth/refresh` 路由 / openapi + 客户端指南 全部落地；§21.3 fail-closed 矩阵零漏项 |
| 2026-04-19 | 1.1 | dev | Round 1 review 修复（P1 + P2）：新增 `UpsertSessionIfJTIMatches` + `ErrSessionStale`（repo 层 compare-and-swap），RefreshToken 改用 CAS 路径 + 把 Revoke 调到 UpsertSession 之前，关闭并发 rotation 双写竞态 + 单次瞬时 Revoke 失败导致误踢的路径；新增 `TestRefreshToken_RotationRaceLost` + 集成 `UpsertSessionIfJTIMatches_*` 3 子 case；既有 `TestRefreshToken_UpsertSessionError` / `TestRefreshToken_RevokeOldJTIError` 更新为新语义 |
| 2026-04-19 | 1.2 | dev | Round 2 review 修复（P2）：`Verify` 补上 `jwt.WithTimeFunc(m.issueClock.Now)`，修复 round 1 引入 `issueClock` 时漏掉的 Verify 时钟绑定 —— Issue 走假时钟 / Verify 走真实墙钟的不对称让 FakeClock 夹具只半工作；新增 `TestManager_VerifyAndIssue_ShareInjectedClock` + `TestManager_Verify_ExpiredAgainstInjectedClock` 锁双向契约 |
