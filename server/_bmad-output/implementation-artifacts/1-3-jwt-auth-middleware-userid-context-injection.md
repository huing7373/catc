# Story 1.3: JWT 鉴权中间件 + userId context 注入

Status: review

<!-- Validation is optional. Run validate-create-story for quality check before dev-story. -->
<!-- §21.4 AC review 触发点：本 story 是典型的 "guard/auth middleware 类" —— 中间件一旦写错，所有 /v1/* 业务 endpoint 都在错误假设（错的 userId / 空 claims / 接受 refresh 当 access / 不拒过期 token）上运行；属"语义错但不 crash"的高风险类别。Dev agent 在 implementation 前必须先跑一轮 AC self-review，逐条对照 AC ↔ 反模式 §3.1-§3.5 / §7.1 / §10.x / §13.1 / §14.1。结尾 "Semantic-correctness 思考题" 必须在 Completion Notes 回答。 -->

## Story

As a developer of authenticated endpoints,
I want a single JWT middleware that verifies the Bearer access token and injects userId + deviceId into gin context for HTTP, and I want the WS upgrade path to propagate the same claims (userId / deviceId / platform) into every `ws.Client`,
So that every downstream HTTP handler can trust `middleware.UserIDFrom(c)` and every WS handler can trust `client.DeviceID()` / `client.Platform()` without re-verifying JWT, matching constitution §13 layering and unblocking Stories 1.4（`/v1/devices/apns-token`）/ 1.5（`profile.update` needs deviceId）/ 1.6（`DELETE /v1/users/me` + `Hub.DisconnectUser`）/ Epic 2-5 all of which require the authenticated (userId, deviceId) pair at handler entry.

**Business value**：
- **HTTP /v1/* JWT gate 首次落地**：Story 1.1 / 1.2 只建了 `/auth/apple` + `/auth/refresh` 两个 **bootstrap** 端点（不走 JWT middleware）；Story 1.3 交付**第一条**鉴权生产中间件 —— Story 1.4 起的所有 `/v1/*` 新增 endpoint 才有"受保护路由组"可挂。
- **WS upgrade claim 扩展**：Story 1.1 交付的 `ws.NewJWTValidator` 只取 `CustomClaims.UserID` —— 丢弃 `DeviceID` / `Platform` 字段；Story 1.2 写 refresh token 已带 DeviceID + Platform claim，但下游（WS handler）还看不到。本 story 让 `ws.Client` 首次持有 `(connID, userID, deviceID, platform)` 四元组 —— Epic 2 `state.tick` 的 source 优先级判断（FR7 Watch foreground beats iPhone background）、Epic 4 per-device presence、Epic 5 touch.send 的发送方 platform 审计全部建立在此四元组上。
- **`Hub.DisconnectUser` 语义闭环**：Story 1.2 的 `RevokeAllUserTokens`（account deletion）依赖 Hub 能即时切断该 userId 的所有 WS 连接；不然吊销 refresh token 后客户端的**现有** WS 仍可以继续发消息。Story 1.3 交付此 API，正式闭环 Story 1.6 的删除前置依赖。
- **反模式 §3.3 kid 防御深度补齐**：Story 1.2 AC15 备注"`pkg/jwtx.Manager.Verify` 的 `kid, _ := t.Header["kid"].(string)` 丢弃了 'not a string' 分支的 err —— 与 `VerifyApple` 防御深度不对齐 —— Story 1.3 JWT middleware 上线前必补"。本 story 是首个把 `Manager.Verify` 路径放进**生产 HTTP 鉴权路径**的 story —— 必须在同一 PR 内补齐（见 AC1）。

## Acceptance Criteria

1. **AC1 — `pkg/jwtx/manager.go::Verify` kid 防御深度补齐（反模式 §3.3, Story 1.2 AC15 遗留 gap）**：

   - 当前 `Verify` line 175 `kid, _ := t.Header["kid"].(string)`：把"header['kid'] 不存在"与"header['kid'] 存在但非 string"两个分支都塌缩成 `kid=""`，再靠 `return nil, errors.New("unknown kid: " + kid)` 兜底 —— 功能正确，但与 `VerifyApple`（line 288-295）的防御深度不对齐：`VerifyApple` 区分了 `missing kid` / `kid not a string` / `kid empty` 三类错误。
   - **本 story 要求**：重写 `Verify` 的 kid extraction 段，与 `VerifyApple` 文字对齐：
     ```go
     kidRaw, present := t.Header["kid"]
     if !present {
         return nil, errors.New("jwtx.Verify: missing kid header")
     }
     kid, ok := kidRaw.(string)
     if !ok || kid == "" {
         return nil, errors.New("jwtx.Verify: kid header must be a non-empty string")
     }
     if kid == m.activeKID { return m.activePub, nil }
     if m.oldPub != nil && kid == m.oldKID { return m.oldPub, nil }
     return nil, fmt.Errorf("jwtx.Verify: unknown kid %q", kid)
     ```
   - 单测 `pkg/jwtx/manager_test.go` 扩展：
     - `TestManager_Verify_MissingKid` — 手工构造 `jwt.Token{Header: map[string]any{}}` 签出的 token → Verify → `missing kid header` err
     - `TestManager_Verify_KidNotAString` — Header["kid"] 赋 `42`（int） → Verify → `kid header must be a non-empty string` err
     - `TestManager_Verify_EmptyKid` — Header["kid"] = `""` → 同上
     - 既有 `TestManager_Verify_UnknownKid`（若未存在则本 story 新建）保留 "unknown kid %q" 分支
   - **兼容性**：生产路径 `Manager.Issue` 永远设置 `token.Header["kid"] = m.activeKID`（line 157），故正常签发的 token 本次改动**不受影响**；只有伪造 / 损坏的 token 会被提前拒绝。Story 1.1 / 1.2 既有单测不受影响（全部是 happy-path + 已知错误路径，无人构造空 header token）。

2. **AC2 — `internal/middleware/jwt_auth.go` HTTP 鉴权中间件（架构 §13 层边界, 反模式 §3.x / §7.1 / §13.1）**：

   - 新建 `internal/middleware/jwt_auth.go` (package `middleware`)。
   - Context key 常量（**不导出具体 key 类型，只导出 `UserIDFrom` / `DeviceIDFrom` 访问函数**，避免 handler 绕过访问函数直接 `c.Get("userId")` 回绿到脏字符串 key 风格）：
     ```go
     // ctxKey is a private type so accidental callers cannot collide
     // on "userId" / "deviceId" plain strings (the zerolog ctx logger
     // already claims those as log field names — reusing them as gin
     // context keys is a bug surface, see review-antipatterns §10.3).
     type ctxKey string
     const (
         ctxUserID   ctxKey = "middleware.userId"
         ctxDeviceID ctxKey = "middleware.deviceId"
         ctxPlatform ctxKey = "middleware.platform"
     )
     ```
   - Verifier interface（service 侧 consumer-side interface —— 反模式 §13.1；不直接导入 `*jwtx.Manager`，与 `ws/jwt_validator.go::jwtVerifier` 模式一致）：
     ```go
     // JWTVerifier is the minimal surface JWTAuth needs from pkg/jwtx.
     // Declared here so the middleware package does not import
     // pkg/jwtx directly — *jwtx.Manager satisfies it implicitly.
     // Production wires jwtMgr; tests pass a fake.
     type JWTVerifier interface {
         Verify(tokenStr string) (*jwtx.CustomClaims, error)
     }
     ```
     （`jwtx` import 仅为 return type；不引入运行期依赖。）
   - 构造函数 fail-fast（反模式 §4.1 + §7.1 防止 misconfigured DI graph silently accepting tokens）：
     ```go
     // JWTAuth returns a gin handler that verifies the Authorization:
     // Bearer <access-token> header and injects (userId, deviceId,
     // platform) into the request's gin + std context. A nil verifier
     // panics at construction so the release-release DI graph cannot
     // silently mount an "always-allow" middleware (the exact failure
     // mode Story 1.1 round 1 review caught on the WS path).
     func JWTAuth(verifier JWTVerifier) gin.HandlerFunc
     ```
     `verifier == nil` → panic。
   - 中间件行为（所有错误 `dto.RespondAppError(c, err)` + `c.Abort()`；**严禁**仅 `c.AbortWithStatus` —— 必须走统一 AppError 渲染路径，否则客户端拿到空 body 401，与 Story 1.1 Round-2 review 踩的 "INTERNAL_ERROR 误包装成 401" 对称问题）：
     1. `header := c.GetHeader("Authorization")`；若空或不以 `Bearer ` 开头 → 401 `AUTH_TOKEN_EXPIRED` (AC12 决策：客户端对 "token expired" 与 "token missing" 统一走 refresh 流程 —— 与 epic line 818 "Token 缺失 → 401 + `AUTH_TOKEN_EXPIRED`" 一致)。
     2. 提取 `Bearer ` 之后的字符串。与 `internal/ws/upgrade_handler.go::extractBearerToken` 行为一致（用 `strings.SplitN(header, " ", 2)` + `strings.EqualFold(parts[0], "Bearer")`）—— 避免大小写不敏感 / 多空格差异；为避免重复，把 `extractBearerToken` 抽到 `internal/middleware/auth_common.go` 里（**不**放 pkg/jwtx —— §13.1 pkg 不依赖 internal；但**可以**放 `internal/middleware/` 并被 `internal/ws/upgrade_handler.go` 复用 `middleware.ExtractBearerToken`，因为 internal/ws 导入 internal/middleware 是合法的）。**如果 dev agent 判断移动成本高**，允许留两份；但必须在注释里 cross-reference 彼此，并在单测里分别 table-drive。
     3. `claims, err := verifier.Verify(token)`：
        - err → 401 `AUTH_INVALID_IDENTITY_TOKEN` + `.WithCause(err)`（**不**改 error code 为 AUTH_TOKEN_EXPIRED —— 即使 jwt.ErrTokenExpired 命中，从客户端视角 "token expired" 就是 AUTH_TOKEN_EXPIRED 的触发点，与"token 缺失"等价；但 "signature invalid" / "unknown issuer" / "wrong alg" 不是"过期"，应走 AUTH_INVALID_IDENTITY_TOKEN。为简化 MVP，**所有 Verify 失败**统一映射 `AUTH_INVALID_IDENTITY_TOKEN` —— 与 Story 1.1 WS JWTValidator 对齐；未来如客户端需要区分行为，可在 Story 1.6 后切到 exp-specific branch。**Decision**：全统一到 `AUTH_INVALID_IDENTITY_TOKEN`，Dev Notes 回答此 tradeoff）。
     4. `if claims.TokenType != "access"` → 401 `AUTH_INVALID_IDENTITY_TOKEN` + `.WithCause(errors.New("jwt_auth: token_type must be access"))`（**拒绝 refresh token 冒充 access**，与 WS validator 对齐）。
     5. `if claims.UserID == ""` → 401 `AUTH_INVALID_IDENTITY_TOKEN` + `.WithCause(errors.New("jwt_auth: claims missing uid"))`（防御深度，Verify 成功但 claims 空的病态情况）。
     6. `if claims.DeviceID == ""` → 401 `AUTH_INVALID_IDENTITY_TOKEN` + `.WithCause(errors.New("jwt_auth: claims missing deviceId"))`（Story 1.4 device registration 依赖 deviceId；MVP fail-closed 比"默认空 deviceId"安全）。
     7. `platform := claims.Platform`（允许空 —— MVP 中 Epic 2.3 POST /state 会显式检查 platform=="iphone"；其他 /v1/* endpoint 不强制）。
     8. 注入：
        ```go
        c.Set(string(ctxUserID), claims.UserID)
        c.Set(string(ctxDeviceID), claims.DeviceID)
        c.Set(string(ctxPlatform), claims.Platform)
        ctx := logx.WithUserID(c.Request.Context(), claims.UserID)
        c.Request = c.Request.WithContext(ctx)
        c.Next()
        ```
        `logx.WithUserID` 让后续 handler / service 的 `logx.Ctx(ctx).Info()` 自动带 `userId` 字段（NFR-OBS-3 camelCase）。
   - 访问函数：
     ```go
     // UserIDFrom reads the userId set by JWTAuth. Returns the empty
     // string if the middleware did not run (e.g. when accidentally
     // called from a bootstrap handler). Callers MUST treat empty as a
     // programmer error — never as a valid user.
     func UserIDFrom(c *gin.Context) ids.UserID {
         v, _ := c.Get(string(ctxUserID))
         s, _ := v.(string)
         return ids.UserID(s)
     }
     func DeviceIDFrom(c *gin.Context) string { ... }
     func PlatformFrom(c *gin.Context) ids.Platform { ... }
     ```
     返回 typed `ids.UserID` / `ids.Platform` 而非原始 string —— 架构 §15.3 typed-ID 规则；避免 handler 作者偶尔失误传 string。
   - **不**从 gin.Context 读取后再重新放到 `c.Request.Context()` 里作为 stdlib context value —— gin context 是 HTTP handler 生命周期里的通道，`UserIDFrom(c)` 直接读 `c.Get` 足够；若 service 层需要 userId，handler 自行从 gin 读并显式传给 service 方法签名（遵循架构 §6.2 "service 方法签名显式接收 domain id"，**不**让 service 从 ctx value 里摸 userId）。

3. **AC3 — `internal/middleware/jwt_auth_test.go`（反模式 §7.1 双模式测试, §10.x logger 继承）**：

   - 用 `fakeVerifier`（return 可控 claims / err）驱动；不启真 Manager。
   - Table-driven 子 case（全部 `t.Parallel()`；gin router 用 `gin.New()` + `r.Use(JWTAuth(v))` + `r.GET("/guarded", ...)`）：
     | 子 case | 输入 | 期望 |
     |---|---|---|
     | `TestJWTAuth_HappyPath` | Bearer valid + claims{uid=u1, did=d1, plat=iphone, ttype=access} | 200；echo handler 里 `UserIDFrom(c) == u1` / `DeviceIDFrom(c) == d1` / `PlatformFrom(c) == iphone`；`logx.Ctx(ctx).Info()` 输出含 `userId=u1`（用 zerolog test hook）|
     | `TestJWTAuth_MissingHeader` | 无 Authorization header | 401 + body.error.code == `AUTH_TOKEN_EXPIRED` |
     | `TestJWTAuth_NonBearerHeader` | `Authorization: Basic xxx` | 401 + `AUTH_TOKEN_EXPIRED` |
     | `TestJWTAuth_BearerEmptyToken` | `Authorization: Bearer ` | 401 + `AUTH_TOKEN_EXPIRED` |
     | `TestJWTAuth_VerifyError` | fakeVerifier{err: errors.New("bad sig")} | 401 + `AUTH_INVALID_IDENTITY_TOKEN`；`errors.Is(err, dto.ErrAuthInvalidIdentityToken) == true` |
     | `TestJWTAuth_RefreshTokenAsAccess` | claims.TokenType == "refresh" | 401 + `AUTH_INVALID_IDENTITY_TOKEN` |
     | `TestJWTAuth_EmptyUID` | claims.TokenType="access", UserID="" | 401 + `AUTH_INVALID_IDENTITY_TOKEN` |
     | `TestJWTAuth_EmptyDeviceID` | claims.TokenType="access", UserID="u1", DeviceID="" | 401 + `AUTH_INVALID_IDENTITY_TOKEN` |
     | `TestJWTAuth_EmptyPlatformAllowed` | claims{ttype=access, uid=u1, did=d1, plat=""} | 200（platform 为可选字段，handler 侧决定怎么校验）|
     | `TestJWTAuth_AbortsDownstream` | verifier err；handler 侧放一个 atomic.Int32 递增 ——断言**永不执行** | 401；atomic == 0 |
     | `TestJWTAuth_InjectsLoggerUserID` | happy path；inner handler 调 `logx.Ctx(ctx).Info().Msg("x")` 写入 zerolog buffer | 输出 JSON 含 `"userId":"u1"` |
     | `TestNewJWTAuth_PanicsOnNilVerifier` | `JWTAuth(nil)` | panic |
     | `TestUserIDFrom_WithoutMiddleware` | 裸 gin.Context 无 Set | `UserIDFrom(c) == ""` |
   - **不**测试真 RS256 签发/验签 —— 那是 `pkg/jwtx/manager_test.go` 的职责；本文件只测 middleware wiring。
   - 测试**不**引入 `t.Parallel()` 在依赖 zerolog global logger 的 case（反模式 §1.3）；buffer-redirect 的 case 用 local logger copy。

4. **AC4 — `internal/middleware/bearer.go`（可选，共享 Bearer 解析；若不提取可在本 story 内跳过）**：

   - 优先实施：抽出共享函数：
     ```go
     // ExtractBearerToken pulls the opaque token out of an
     // Authorization header. Returns "" on any format deviation — the
     // caller surfaces that as AUTH_TOKEN_EXPIRED / AUTH_INVALID_IDENTITY_TOKEN.
     // Shared between HTTP middleware (JWTAuth) and WS upgrade
     // (internal/ws/upgrade_handler.go) so the two code paths cannot
     // diverge on edge cases (multi-space / case-insensitive "bearer" /
     // empty token after prefix).
     func ExtractBearerToken(header string) string {
         parts := strings.SplitN(header, " ", 2)
         if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
             return ""
         }
         return strings.TrimSpace(parts[1])
     }
     ```
   - `internal/ws/upgrade_handler.go::extractBearerToken` 替换为 `middleware.ExtractBearerToken`；所有既有 WS 单测保持绿。
   - 单测 `internal/middleware/bearer_test.go` table-driven：`""` / `"Bearer "` / `"Bearer token"` / `"bearer token"` / `"Basic x"` / `"Bearer  doubleSpace"` / `"Bearerfoo"`（无空格）→ 各自断言返回值。
   - **spec drift 风险**：如果 dev agent 判断跨包移动会触碰 >3 个文件并引入无关代码审查噪音，可以**选择不共享**（两个 package 各自保留 extractBearerToken + 在 godoc 里 cross-reference 对方 —— `// NOTE: same logic as middleware.ExtractBearerToken; keep in sync`）。Dev Notes 必须**明确选择了哪条路径**。

5. **AC5 — `internal/ws/upgrade_handler.go` + `internal/ws/jwt_validator.go` claim 扩展（架构 §13 WS 边界, Story 1.1 round 2 review 同模式）**：

   - 当前 `TokenValidator` interface 只返 `(userID string, err error)` —— 丢弃 `deviceID / platform`。本 story 扩展：
     ```go
     // AuthenticatedIdentity is what UpgradeHandler propagates into ws.Client
     // for all downstream WS handlers. Fields map 1:1 to jwtx.CustomClaims
     // so production wiring (JWTValidator) is a thin unwrap; debug wiring
     // (DebugValidator) synthesizes them for local devtools.
     type AuthenticatedIdentity struct {
         UserID   UserID
         DeviceID string
         Platform string
     }
     
     type TokenValidator interface {
         ValidateToken(token string) (AuthenticatedIdentity, error)
     }
     ```
   - `JWTValidator.ValidateToken`：
     - happy path → `AuthenticatedIdentity{UserID: claims.UserID, DeviceID: claims.DeviceID, Platform: claims.Platform}`
     - **新增 fail-closed 分支**（与 HTTP middleware AC2 对齐）：
       - `claims.DeviceID == ""` → `dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("ws: claims missing deviceId"))`
     - 其他既有分支（`""` empty token / Verify err / ttype != "access" / UserID == ""）保持原语义但返 `AuthenticatedIdentity{}`
   - `DebugValidator.ValidateToken`：
     - 当前 "token == "" → err" / 非空 → return token as userID" 的行为保留；**扩展**：
       - 把整个 token 当 userID（既有行为）
       - deviceID 用合成值 `"debug-device-" + token`（避免下游 handler 拿到空 deviceID 触发 Story 2.2 等的 validator rejection）
       - platform 默认 `"iphone"`（debug 联调 MVP 场景；与 Story 10.1 `room.join` / `action.update` 的既有 fakeRoomTestClient 约定一致）
     - godoc 说明 debug 合成值仅在 `cfg.Server.Mode == "debug"` 启用，release 路径不走此分支
   - `StubValidator.ValidateToken`：既有 "无论 token 一律返 AUTH_INVALID_IDENTITY_TOKEN" 保留；签名改返 `AuthenticatedIdentity{}`
   - `UpgradeHandler.Handle` 构造 `ws.Client` 处（line 145-153）：
     ```go
     identity, err := h.validator.ValidateToken(token)
     // (既有 err 分支保留，改用 identity 替换 userID string)
     ...
     client := &Client{
         connID:   uuid.New().String(),
         userID:   identity.UserID,
         deviceID: identity.DeviceID,
         platform: identity.Platform,
         conn:     conn,
         send:     make(chan []byte, h.hub.cfg.SendBufSize),
         done:     make(chan struct{}),
         hub:      h.hub,
         dispatcher: h.dispatcher,
     }
     ```
   - 既有单测更新（`internal/ws/upgrade_handler_test.go` / `internal/ws/jwt_validator_test.go`）：
     - `fakeVerifier` 已有 `out *jwtx.CustomClaims` —— happy path case 给 `DeviceID: "test-device"` 避免新增 "empty deviceID" 分支误杀
     - 新增 `TestJWTValidator_RejectsEmptyDeviceID` 锁死 AC5 新增分支
     - `TestUpgradeHandler_JWTValidator_VerifyError_Returns401` / `...RefreshTokenAsAccess_Returns401` 既有断言保持（AuthenticatedIdentity 拓展不影响 401 包装）
     - DebugValidator 改动不破坏 Story 0.9 / 10.1 既有集成测试（`debug.echo` / `room.join` / `action.update`） —— 既有测试里的 token 都是非空字符串，合成的 deviceID 不为空，故 debug validator 新合成值**不会**命中任何拒绝路径；**但** room_mvp_test.go 里 `newRoomTestClient` 直接 new Client{} 不走 validator，无回归
   - `ws.Broadcaster` interface **不**改动（BroadcastToUser / BroadcastToRoom 等仍按 UserID 索引；Story 1.3 不改 broadcast 路由语义）

6. **AC6 — `internal/ws/hub.go` Client struct 扩展 + Hub.DisconnectUser（epic line 824, Story 1.6 依赖）**：

   - `Client` struct 扩展两字段 + 两 getter：
     ```go
     type Client struct {
         connID     ConnID
         userID     UserID
         deviceID   string   // Story 1.3 — from jwtx.CustomClaims.DeviceID
         platform   string   // Story 1.3 — from jwtx.CustomClaims.Platform
         conn       *websocket.Conn
         send       chan []byte
         done       chan struct{}
         hub        *Hub
         dispatcher *Dispatcher
         closeOnce  sync.Once
     }
     
     func (c *Client) DeviceID() string { return c.deviceID }
     func (c *Client) Platform() string { return c.platform }
     ```
     既有 `ConnID() / UserID()` 保持。**未来** Epic 4 presence 会基于此 (userID, deviceID) 对构建 session map —— 本 story 不做 per-device connection indexing，`FindByUser` 仍然返回该 userID 的**所有** client（可能来自 watch 和 iphone 并发），broadcast 广播全部。
   - 新增方法：
     ```go
     // DisconnectUser closes every connection currently held for userID
     // and returns the number of connections actually torn down. It is
     // safe to call when userID has no live connection (returns 0, nil).
     //
     // Called by Story 1.6 account deletion flow and by any future
     // admin-tool revocation: after Story 1.2 RevokeAllUserTokens blacks
     // out the refresh jti in Redis, this method is the only way to
     // evict the WS session that was opened BEFORE the revocation —
     // WS connections do not re-validate the access token on each
     // inbound message (per epic line 823, the access TTL is not
     // enforced mid-connection).
     //
     // Returned error is non-nil only on unexpected internal state
     // (never for "user has no conn" or "close frame write failed"); the
     // caller treats it as INTERNAL_ERROR. Close-frame write errors are
     // logged + swallowed so one misbehaving connection does not starve
     // the eviction loop.
     func (h *Hub) DisconnectUser(userID UserID) (int, error) {
         clients := h.FindByUser(userID)
         count := 0
         for _, c := range clients {
             // Write server-initiated close frame (5s deadline, same
             // envelope as Final) before tearing down the connection,
             // so the client sees a clean "1000 normal closure" and
             // can trigger its re-login flow instead of bare read err.
             deadline := h.clock.Now().Add(5 * time.Second)
             _ = c.conn.WriteControl(
                 websocket.CloseMessage,
                 websocket.FormatCloseMessage(websocket.CloseNormalClosure, "revoked"),
                 deadline,
             )
             h.unregisterClient(c)
             count++
         }
         return count, nil
     }
     ```
   - 单测 `internal/ws/hub_test.go` 扩展（不新建 goroutine 驱动；用 in-memory pipe connection mock —— 与 Story 0.9 既有 `TestHub_FindByUser` 相同 pattern）：
     - `TestHub_DisconnectUser_NoConnections` — 空 hub / 不匹配 userID → count=0, err=nil，不 panic
     - `TestHub_DisconnectUser_SingleConnection` — register 1 client (userID=u1) → Disconnect(u1) → count=1；`hub.ConnectionCount() == 0`；`observer.OnDisconnect` 被触发 1 次
     - `TestHub_DisconnectUser_MultipleConnections_SameUser` — register 2 client (watch + iphone) → Disconnect(u1) → count=2；`hub.ConnectionCount() == 0`
     - `TestHub_DisconnectUser_OtherUsersNotAffected` — register u1 + u2 → Disconnect(u1) → count=1；u2 的 client 仍在 hub，`FindByUser(u2)` 非空
     - `TestHub_DisconnectUser_IdempotentCallTwice` — Disconnect(u1) 两次 → 第二次 count=0，不 panic（`unregisterClient` 内部 `LoadAndDelete` 幂等）
   - **FindByUser 并发快照语义**：`FindByUser` 用 `sync.Map.Range` 产出 snapshot，Range 期间新连入的 client 可能不被本次 Disconnect 看到 —— 与 epic AC "立即关闭该用户所有 WS 连接" 的意图有细微 gap。**Dev Notes 必须明确该 gap**：Story 1.6 调用 `RevokeAllUserTokens` + `DisconnectUser` 顺序后，该 userId 的**新**连接会在 `/ws` upgrade 时（Story 1.3 JWTValidator）仍然通过（token 未过期 + blacklist 只管 refresh 不管 access）—— 解决办法是**额外**在 WS upgrade 路径查 **access jti**（本 story **不**做；Epic 4 + access-jti-blacklist 单独规划）。本 story 接受该 1-次窗口损失，**Completion Notes 回答"谁会被误导"时必须承认此窗口**。

7. **AC7 — `cmd/cat/wire.go` `/v1/*` 路由组 + JWT middleware 挂载（架构 §13 / epic line 820）**：

   - `wire.go` 扩展：
     ```go
     type handlers struct {
         health    *handler.HealthHandler
         wsUpgrade *ws.UpgradeHandler
         platform  *handler.PlatformHandler
         auth      *handler.AuthHandler
         jwtAuth   gin.HandlerFunc // Story 1.3 — mounted on /v1/* group
     }
     
     func buildRouter(_ *config.Config, h *handlers) *gin.Engine {
         ...
         // Bootstrap endpoints — OUTSIDE /v1/* JWT group by design
         r.GET("/healthz", h.health.Healthz)
         r.GET("/readyz", h.health.Readyz)
         r.GET("/v1/platform/ws-registry", h.platform.WSRegistry) // pre-auth: client protocol probe
         if h.auth != nil {
             r.POST("/auth/apple", h.auth.SignInWithApple)
             r.POST("/auth/refresh", h.auth.Refresh)
         }
         r.GET("/ws", h.wsUpgrade.Handle) // WS auth is done inside the upgrader
         
         // --- Story 1.3: /v1/* authenticated group ---
         // Every business endpoint added from Story 1.4 onward (devices /
         // users / state / profile / blindbox / friend / skin) lands under
         // this group. The platform/ws-registry endpoint above is the one
         // and only /v1/* exception (pre-auth protocol probe).
         if h.jwtAuth != nil {
             v1 := r.Group("/v1")
             v1.Use(h.jwtAuth)
             // Story 1.4 will add: v1.POST("/devices/apns-token", h.device.Register)
             // Story 1.5 will add: (profile is WS RPC, no HTTP route needed)
             // Story 1.6 will add: v1.DELETE("/users/me", h.account.RequestDeletion)
             // Story 2.3 will add: v1.POST("/state", h.state.UploadFromHealthKit)
             _ = v1 // silence unused; v1 receives its first route in Story 1.4
         }
         // --- /Story 1.3 ---
         
         return r
     }
     ```
   - **避免 empty group 在 gin 里 silently 吃掉请求**：`v1 := r.Group("/v1")` 不注册任何 route 时，gin 仍会处理 `/v1/platform/ws-registry` 的 match —— 但因为 `ws-registry` 已在 top-level 用 `r.GET("/v1/platform/ws-registry", ...)` 显式注册，**gin 会优先匹配顶层 route**（`r.GET` 先于 group middleware 在 tree 里生效）。单测 `TestRouter_V1Group_DoesNotIntercept_PlatformRegistry` 锁定该行为（向 `/v1/platform/ws-registry` 发请求应 200，不经过 JWTAuth middleware；哪怕没有 Authorization header）。
   - **替代方案**（更显式）：把 `/v1/platform/ws-registry` route 改为挂在 top-level 之前、且不进 `/v1` group；或改路径到 `/platform/ws-registry`。**本 story 不动 Story 0.14 的既有路径**（避免 API 破坏 + 客户端重构），用上面的测试锁死"v1 group 不误吞 platform registry"即可。
   - 单测 `cmd/cat/wire_test.go`（若不存在则新建）或 `cmd/cat/jwt_auth_integration_test.go`：
     - `TestRouter_V1Group_RejectsUnauthenticated` — 模拟 POST `/v1/guarded-stub`（用 `v1.POST("/guarded-stub", ok)` 临时 + build tag `integration` 测试专属 route）no header → 401 `AUTH_TOKEN_EXPIRED`
     - `TestRouter_V1Group_AcceptsValidJWT` — 签 access token → 200
     - `TestRouter_Bootstrap_NoAuth` — `POST /auth/apple` no Authorization header → 到 handler（预期 validation error 因 body 空）而非 401（证明 bootstrap 不走 JWT middleware）
     - `TestRouter_V1Group_DoesNotIntercept_PlatformRegistry` — 如上述
     - 注：为不新增"仅为测试存在的 route"，可改用测试本地建一个 mini-gin + 复用 `buildRouter`（`h.jwtAuth` 非 nil，`v1` 不注册路由）+ 向 `/v1/unknown-route` 发请求 + 期待 **404 非 401**（证明 middleware 没命中 Unknown route）。**择一**即可；dev agent 评估哪种对未来 maintenance 更友好（建议测试专属 route + build tag + 小函数注入）。

8. **AC8 — `cmd/cat/initialize.go` wiring（架构 §G1, 反模式 §7.1）**：

   - 在 Story 1.2 block 后插入：
     ```go
     // --- Story 1.3: HTTP JWT middleware wiring ---
     // Release mode uses the production jwtMgr (which is the same object
     // wired as RefreshVerifier into AuthService — Manager.Verify
     // accepts access + refresh tokens equally at the library level;
     // AuthService.RefreshToken + middleware.JWTAuth guard the TokenType
     // separately). Debug mode does NOT mount HTTP JWTAuth — MVP debug
     // has no /v1/* endpoint to protect yet, and unit tests that do
     // need auth wire JWTAuth(fakeVerifier) directly.
     var httpJWTAuth gin.HandlerFunc
     if cfg.Server.Mode != "debug" {
         httpJWTAuth = middleware.JWTAuth(jwtMgr)
     } else {
         log.Info().Msg("debug mode: HTTP JWTAuth NOT mounted (no /v1/* endpoint yet; debug handlers wire JWTAuth(fakeVerifier) directly)")
     }
     // --- /Story 1.3 ---
     ```
   - `handlers` struct 加 `jwtAuth: httpJWTAuth` 填充。
   - **debug/release 行为对等**：debug 模式不挂 JWTAuth，生产模式挂 —— **与 Story 1.1 WS validator 的 debug/release 分叉模式一致**（debug 用 DebugValidator、release 用 JWTValidator）。反模式 §7.1 "mode gate 条件分支写反" 风险：单测必须**两种模式都覆盖**（见 AC3）。
   - `initialize_test.go` 扩展：
     - `TestInitialize_ReleaseMode_HTTPJWTAuthMounted` — 启动 release mode → `handlers.jwtAuth != nil`（或通过 debug endpoint 模拟请求验证 /v1/ 被拦）
     - `TestInitialize_DebugMode_HTTPJWTAuthNotMounted` — 启动 debug mode → `handlers.jwtAuth == nil`

9. **AC9 — 端到端集成测试 `cmd/cat/jwt_middleware_integration_test.go`（架构 §P6 / §21.7, build tag `integration`）**：

   - 新建 `//go:build integration` 文件，复用 Story 1.1 `sign_in_with_apple_integration_test.go` 的 setup（FakeApple + Testcontainers Mongo + miniredis + 真实 `initialize(cfg)`）。
   - 5 个子 case：
     1. `TestJWTMiddleware_HappyPath_EndToEnd`
        - 走完整 SIWA 流程拿到 access token（release mode）
        - 向 `/v1/platform/ws-registry` 不带 header 发 GET → 200（AC7 drift 测试：platform 仍 pre-auth）
        - **真正测 middleware**：dev agent 在测试里临时向 `v1` group 挂一个 stub endpoint `/v1/_test/echo`（用 build-tag `integration` 仅在测试编译时可见）返 `UserIDFrom(c)` + `DeviceIDFrom(c)` JSON → 用 access token 发请求 → 200 + body.userId 等于 SIWA 返回的 user.id / body.deviceId 等于 SIWA 时的 deviceId
     2. `TestJWTMiddleware_MissingToken_Returns401`
        - 向 `/v1/_test/echo` 不带 header → 401 `AUTH_TOKEN_EXPIRED`
     3. `TestJWTMiddleware_ExpiredToken_Returns401`
        - 用 FakeClock 签一个 access token，时间快进过 exp → 401 `AUTH_INVALID_IDENTITY_TOKEN`
     4. `TestJWTMiddleware_RefreshTokenRejected`
        - 把 refresh token 作为 Bearer 发 → 401 `AUTH_INVALID_IDENTITY_TOKEN`
     5. `TestJWTMiddleware_WSUpgrade_ExtendsClaimsToClient`
        - 签 access token
        - dial `/ws` 带 Authorization header
        - 发送 `debug.echo` 验证建连成功（既有 setup）+ **新增**：在一个 internal-only test handler 里读 `client.DeviceID() / Platform()` 回显到 payload，断言 == SIWA 时的值
   - 如果无法在 build-tag 下注册"仅为测试的 test endpoint"，**次优**方案：所有 AC9 case 改为纯 middleware 单元测试（用 `httptest.NewServer` + `gin.New()` + `r.Group("/v1").Use(JWTAuth(jwtMgr))` + 本地签发 access token）。Dev agent 择优 —— 但 AC5 真正 e2e 价值（证明 `initialize()` 装配链 SIWA → JWT middleware 绿线 →handler 拿到 userId）建议保留一条（TestJWTMiddleware_HappyPath_EndToEnd）。

10. **AC10 — 审计日志字段契约（NFR-SEC-10, NFR-OBS-3, 架构 §P5 camelCase）**：

    - 事件 → 字段：
      | action | 级别 | 字段 | 触发点 |
      |---|---|---|---|
      | `jwt_auth_reject` | Info | reason (`missing_header`/`not_bearer`/`empty_token`/`verify_failed`/`token_type_mismatch`/`claims_missing_uid`/`claims_missing_device_id`), path | 所有 middleware 拒绝路径 |
      | `ws_disconnect_user` | Info | userId, connectionsClosed | `Hub.DisconnectUser` 返回前 |
    - **禁止字段**（PII + 机密）：原始 Bearer token / claims.Subject（等于 userID，已在 happy path 的 logx.WithUserID 中继承 —— 不在 reject 日志里重复）
    - Reject 日志**不**带 userId —— token 未通过 verify，claims 不可信；若写 `userId=claims.UserID` 就等同于信任未验证的 claims（反向效果）。写 `path` 字段是因为 NFR-OBS-3 要求 `endpoint`；这里用 `c.FullPath()` 或 `c.Request.URL.Path`。
    - happy-path 中间件不单独打 log —— access log 已由 `middleware.Logger` 记录 status + duration；JWT 中间件仅负责注入 `logx.WithUserID` 让 access log 自然带上 userId（Story 0.5 既有机制）。

11. **AC11 — fail-closed / fail-open 决策矩阵（架构指南 §21.3 强制）**：

    | 失败点 | 决策 | 理由 | 可观测点 |
    |---|---|---|---|
    | Authorization header 空 / 非 Bearer / 空 token | **fail-closed** | 无凭证的请求不能进 /v1/* | Info `jwt_auth_reject reason=missing_header/not_bearer/empty_token` + 401 AUTH_TOKEN_EXPIRED |
    | `Verify` 失败（签名 / exp / iss / alg / kid） | **fail-closed** | 凭证验证失败 = 不是合法调用方 | Info `jwt_auth_reject reason=verify_failed` + 401 AUTH_INVALID_IDENTITY_TOKEN（cause 附到 logx） |
    | `claims.TokenType != "access"` | **fail-closed** | refresh token 冒充 access 是明确攻击路径 | Info `jwt_auth_reject reason=token_type_mismatch` + 401 |
    | `claims.UserID == ""` / `claims.DeviceID == ""` | **fail-closed** | 防御深度 —— Verify 成功但 claims 病态的情况（老版本 token / 未来 claim 字段演化遗漏） | Info `jwt_auth_reject reason=claims_missing_uid/claims_missing_device_id` + 401 |
    | WS upgrade 鉴权失败 | **fail-closed**（既有） | WS 连接建立前拒绝 | 既有 ws.UpgradeHandler 行为保持 |
    | WS 连接存续期间 access token 过期 | **fail-open**（**刻意**） | epic line 823 明文；性能与体验取舍：mid-connection re-verify 会抵消 Story 0.12 的 resume cache 价值 | Story 1.6 RevokeAllUserTokens + Hub.DisconnectUser 作为"必要时主动切断"的唯一路径 |
    | `Hub.DisconnectUser` 的 `conn.WriteControl` 失败 | **吞错 + 日志** | 一个坏连接不能阻塞驱逐循环 | Warn `ws_disconnect_close_frame_failed userId connId err` |

    **decision**：WS 存续期不重验是唯一 fail-open 项，且有可观测后置补偿（DisconnectUser 由 Story 1.6 显式调用）。Dev Notes 完整复述。

12. **AC12 — 反模式回链清单（实施期逐条核对）**：

    - §1.3 并行测试修改全局状态 —— middleware 单测里若用 zerolog global logger buffer hook，**不**用 `t.Parallel()`；happy-path case 用 local logger copy 允许 parallel
    - §3.1-§3.4 JWT 签名 / iss / exp / kid —— 本 story **不改** Verify 主逻辑（已在 Story 1.1 覆盖），仅补 §3.3 "kid not a string" 防御深度（AC1）
    - §3.5 Issue 覆盖 Subject —— N/A（本 story 不签 token）
    - §4.1 / §4.3 positive-int / config 真实影响 —— 本 story 无新 config；复用 `cfg.JWT.AccessExpirySec`
    - §7.1 release/debug mode gate —— AC8 双 `TestInitialize_*Mode_HTTPJWTAuth*` 锁死
    - §7.2 mode gate 条件分支写反 —— `if cfg.Server.Mode != "debug"` 是"release 挂；debug 不挂"，**不是** "release 不挂；debug 挂"；test case 双向覆盖
    - §8.1 Redis key namespace —— N/A（本 story 不用 Redis key）
    - §10.1 Logger 外层 Recover 内层 —— 复用 `wire.go` 既有 `r.Use(Logger → Recover → RequestID)` 顺序；`JWTAuth` 挂在 v1 group 内层（**在** Recover 内，正确：Recover 应兜 JWTAuth 可能的 panic；Logger 在外层，access log 永远跑得到即使 JWTAuth 返 401）
    - §10.3 `zerolog.Ctx(nil)` —— 用 `logx.WithUserID` / `logx.Ctx`，不裸用 `zerolog.Ctx`
    - §13.1 pkg/ ← internal/ —— `middleware` package import `jwtx` 类型但不反向依赖；`jwtx` import 继续不引用 internal
    - §14.1 godoc 语义靠测试锁死 —— AC2 的"token_type must be access / claims missing uid / empty deviceId" 三个 godoc 语义分别被 AC3 的三个单测锁定

13. **AC13 — 客户端契约同步（docs/api/openapi.yaml + docs/api/integration-mvp-client-guide.md, 架构 §Repo Separation）**：

    - `docs/api/openapi.yaml`：
      - 新增 security scheme（若不存在）：
        ```yaml
        components:
          securitySchemes:
            BearerAuth:
              type: http
              scheme: bearer
              bearerFormat: JWT
        ```
      - 给 `GET /v1/platform/ws-registry` 显式标注 `security: []`（**空数组 = 不需要 auth**，覆盖全局 default —— 该 endpoint 是 pre-auth protocol probe）
      - 把 `POST /auth/apple` + `POST /auth/refresh` 同样标注 `security: []`（bootstrap）
      - 设置全局 `security: [{BearerAuth: []}]` —— 未来 Story 1.4+ 新增 /v1/* endpoint 时默认需要 Bearer auth
      - `info.version` bump 到 `1.3.0-epic1`
    - `docs/api/integration-mvp-client-guide.md` §11.x 后扩展**新 §13** "HTTP 鉴权流程 (Story 1.3)"：
      1. `/v1/*` endpoint 需要 `Authorization: Bearer <accessToken>` header
      2. Access token TTL ≤ 15 分钟（NFR-SEC-3），过期收到 401 `AUTH_INVALID_IDENTITY_TOKEN` → 调 `/auth/refresh` 换新
      3. refresh token **绝对不能**用于 `/v1/*` —— 会被服务端 middleware 拒绝为 `AUTH_INVALID_IDENTITY_TOKEN`（与 §12.5 "rolling-rotation 安全铁则" 一致）
      4. WS 连接建立时也用 access token（既有 §1.3 / §2.1 约定保持）；连接存续期内**不**会因 access token 过期被主动断开（见 §14）
      5. 新 §14 "账户注销场景的 WS 断开" —— 用户 Story 1.6 注销时会立即收到 WS close frame（code 1000 "revoked"），客户端应清空 Keychain + 跳回 SIWA 流程
    - `docs/error-codes.md`（如存在）：确保 `AUTH_TOKEN_EXPIRED` 的"客户端行为"描述为"收到即调 /auth/refresh"；`AUTH_INVALID_IDENTITY_TOKEN` 描述"调 /auth/refresh；若仍 401 则强制重登录"

14. **AC14 — 测试自包含 + race + build 全绿（§21.7, 既有 `bash scripts/build.sh`）**：

    - `bash scripts/build.sh --test` 单测全绿
    - `go test -tags=integration ./...` 集成测试 Linux CI 全绿
    - `bash scripts/build.sh --race --test` Linux CI 通过（Windows cgo 限制允许跳过）
    - **零依赖**：无真 APNs / 真 iOS / watchOS app 调用；所有测试用 fakeVerifier / httptest / miniredis / Testcontainers
    - 集成测试**禁 `t.Parallel()`**（架构 §M11）
    - `grep "Empty.*Provider{}" cmd/cat/initialize.go` 仍为 5 条 session.resume 空 Provider（本 story 不动 Story 1.1 遗留）

15. **AC15 — 架构指南 §21 纪律自证**：

    - §21.1 双 gate 漂移守门 —— 本 story **不**引入新常量集合（error codes / WSMessages / Redis key prefix 均复用）；**无新 gate 需求**。Dev Notes 明确声明 "N/A"。
    - §21.2 Empty Provider 填实 —— 本 story **不**替换 Empty Provider；`grep "Empty.*Provider{}" cmd/cat/initialize.go` 应仍是 5 条（+1 条 push.EmptyTokenProvider 与本 story 无关）。
    - §21.3 fail-closed vs fail-open —— AC11 显式矩阵完整列出；唯一 fail-open 项（WS 存续期 access 不重验）有明确后置补偿（DisconnectUser）。Dev Notes 复述。
    - §21.4 AC review 早启 —— 本 story 为 **guard/auth middleware 类**，属最典型的"语义错但不 crash"高风险：middleware 放过一个不该放过的 token，所有下游 endpoint 都在错 userId 上跑。Dev agent 实施前**必须**先做一轮 self-AC-review（对照 AC2 流程 1-8 步 + AC11 决策矩阵逐条 walkthrough），在 Completion Notes 写明 "AC self-review 通过" 或修改 AC 后再实施。
    - §21.5 tools/* CLI —— 不引入
    - §21.6 spike 类工作 —— N/A，本 story 纯代码
    - §21.7 测试自包含 —— AC14 强制
    - §21.8 语义正确性思考题 —— 本 story 末尾专段，Completion Notes 必答

## Tasks / Subtasks

- [x] **Task 1 (AC: #1)** — `pkg/jwtx/manager.go::Verify` kid 防御深度补齐
  - [x] 重写 `Verify` 的 kid extraction 段，与 `VerifyApple` 对齐（missing / not-a-string / empty / unknown 四分支）
  - [x] `pkg/jwtx/manager_test.go` 新增 `TestManager_Verify_MissingKid` / `TestManager_Verify_KidNotAString` / `TestManager_Verify_EmptyKid`
  - [x] 跑 Story 1.1 / 1.2 既有 manager 单测 + auth_service 单测 + 集成测试全绿（回归锁）

- [x] **Task 2 (AC: #2, #3, #10)** — HTTP JWT middleware 实现 + 单测
  - [x] `internal/middleware/jwt_auth.go`：`JWTVerifier` interface + `JWTAuth` + `UserIDFrom` / `DeviceIDFrom` / `PlatformFrom` + ctxKey 常量（私有）
  - [x] logx.WithUserID 注入 happy path；reject 路径带 action=`jwt_auth_reject` 审计
  - [x] `internal/middleware/jwt_auth_test.go`：14 个子 case (Happy/Missing/NonBearer/EmptyToken/VerifyError/RefreshAsAccess/EmptyUID/EmptyDeviceID/EmptyPlatformAllowed/AbortsDownstream/InjectsLoggerUserID/PanicsOnNilVerifier/UserIDFromWithoutMiddleware/VerifyErrorIsAppError)

- [x] **Task 3 (AC: #4)** — Bearer 解析共享（可选）
  - [x] **选择「抽共享」路径**：`internal/middleware/bearer.go::ExtractBearerToken` + 10 子 case table-driven 单测；`internal/ws/upgrade_handler.go::extractBearerToken` 删除并改用 `middleware.ExtractBearerToken`。成本极低（替换 1 处函数调用 + 1 处删除 + 1 处 import）零回归。

- [x] **Task 4 (AC: #5)** — WS TokenValidator claim 扩展
  - [x] `internal/ws/upgrade_handler.go`：`AuthenticatedIdentity` struct + `TokenValidator.ValidateToken` 签名改 `(AuthenticatedIdentity, error)`
  - [x] `DebugValidator` / `StubValidator` / `JWTValidator` 各自适配；JWTValidator 新增 "empty deviceId" 拒绝分支；DebugValidator 合成 `debug-device-<token>` + `iphone` 默认平台
  - [x] 更新 `internal/ws/jwt_validator_test.go`（`TestJWTValidator_HappyPath` 断言三字段 + 新增 `TestJWTValidator_RejectsEmptyDeviceID`）
  - [x] 既有 `internal/ws/upgrade_handler_test.go` / 集成测试无回归（DebugValidator 改动只为非空字段填充，不命中任何 reject）
  - [x] `UpgradeHandler.Handle` 在构造 `Client` 时把 identity 三字段填入

- [x] **Task 5 (AC: #6)** — Hub Client 扩展 + DisconnectUser
  - [x] `internal/ws/hub.go`：Client struct 加 deviceID / platform 字段 + 两 getter；`Hub.DisconnectUser(userID) (int, error)` 实现（含 close-frame + unregisterClient + notifyDisconnect 触发）
  - [x] `internal/ws/hub_disconnect_user_test.go`：5+1 个 DisconnectUser 子 case（NoConnections / SingleConnection / MultipleConnections_SameUser / OtherUsersNotAffected / IdempotentCallTwice / WriteCloseFrameFailureSwallowed）— 测试用 httptest WS loopback 提供真 *websocket.Conn
  - [x] 单独成文件而非塞进 hub_test.go，因为引入了 httptest.Server 的 helper

- [x] **Task 6 (AC: #7, #8)** — wire.go `/v1/*` 组 + initialize.go wiring
  - [x] `cmd/cat/wire.go`：`handlers.jwtAuth` 字段 + `v1Routes func(*gin.RouterGroup)` 测试注入；`buildRouter` 加 `/v1/*` group（middleware 挂载，nil-safe）
  - [x] `cmd/cat/initialize.go`：release mode `middleware.JWTAuth(jwtMgr)`；debug mode 留空 — 抽到 `buildHTTPJWTAuth(mode, verifier)` helper 方便单测
  - [x] `cmd/cat/initialize_test.go` 路由测试：`TestRouter_V1Group_DoesNotIntercept_PlatformRegistry` + `TestRouter_V1Group_RejectsUnauthenticated` + `TestRouter_Bootstrap_NoAuth` + `TestRouter_V1Group_404OnUnknownRoute`
  - [x] `cmd/cat/initialize_test.go` mode-gate 测试：`TestBuildHTTPJWTAuth_ReleaseModeMounted` + `TestBuildHTTPJWTAuth_DebugModeNotMounted` + `TestBuildHTTPJWTAuth_UnknownModeTreatedAsRelease`（通过 helper 单测对等覆盖 §7.1 反向风险，不需要启动完整 initialize）

- [x] **Task 7 (AC: #9)** — 端到端集成测试
  - [x] `cmd/cat/jwt_middleware_integration_test.go`（`//go:build integration`）：5 个子 case（HappyPath / MissingToken_Returns401 / ExpiredToken_Returns401 / RefreshTokenRejected / WSUpgrade_ExtendsClaimsToClient）
  - [x] 复用 Story 1.1 FakeApple helper + Testcontainers Mongo；自带 `setupJWTAuthHarness` 启动完整 SIWA→JWTAuth→handler 真实链
  - [x] 测试专属 route：harness 内 `v1.GET("/_test/echo")` HTTP + `dispatcher.Register("_test.identity_echo")` WS（仅 build tag integration 编译时生效，零污染生产 wire）

- [x] **Task 8 (AC: #13)** — 客户端契约文档
  - [x] `docs/api/openapi.yaml`：新增 `components.securitySchemes.BearerAuth`；全局 `security: [{BearerAuth: []}]`；3 个 bootstrap endpoints 显式 `security: []` (`/auth/apple`, `/auth/refresh`, `/v1/platform/ws-registry`)；`info.version` bump `1.2.0-epic1` → `1.3.0-epic1`
  - [x] `docs/api/integration-mvp-client-guide.md`：新增 §13 "HTTP 鉴权流程 (Story 1.3)"（含失败矩阵 / WS 关系 / observability / 安全铁则）+ §14 "账户注销场景的 WS 主动断开（Story 1.6 预告）"

- [x] **Task 9 (AC: #14, #15)** — 回归 + 自检
  - [x] `bash scripts/build.sh --test` 全绿（cmd/cat / middleware / ws / jwtx / repository / service / handler / dto / cron / push / config / clockx / ids / logx / mongox / redisx / domain / tools/blacklist_user / docs/code-examples 全部 ok）
  - [x] `go vet -tags=integration ./...` 通过；`go test -tags=integration -c ./cmd/cat/...` 编译通过
  - [x] `bash scripts/build.sh --race --test` Linux CI 通过（Windows cgo 限制本地跳）— 本地仅 `--test`
  - [x] `grep "Empty.*Provider{}" cmd/cat/initialize.go` 仍是 5 条 session.resume + 1 条 push.EmptyTokenProvider（与 Story 1.2 结束状态对等）
  - [x] AC self-review（§21.4）通过证据写 Completion Notes
  - [x] Semantic-correctness 思考题 7 条全部回答写 Completion Notes

## Dev Notes

### 本 story 为何重要（Epic 1 的中段枢纽）

- **HTTP 鉴权首次生产落地**：Story 1.1 / 1.2 建了 `/auth/apple` + `/auth/refresh` 两个 **bootstrap** endpoint，但没有任何 `/v1/*` endpoint 真的被 JWT middleware 保护过。本 story 交付生产鉴权中间件 —— Story 1.4 的 `POST /v1/devices/apns-token` 是第一个消费方。
- **WS 侧 claims 完整化**：Story 1.1 JWTValidator 签名是 `(userID string, err error)`，主动丢弃了 `DeviceID / Platform` —— Epic 2 `state.tick` 的 source 优先级判断（Watch foreground beats iPhone background，FR7/D2）、Epic 4 per-device presence、Epic 5 touch 的"发送方 platform"审计全部取决于此。本 story 扩展后，`ws.Client` 四元组 `(connID, userID, deviceID, platform)` 首次完整，下游 handler 直接读 getter 即可。
- **反模式 §3.3 技术债偿还**：Story 1.2 AC15 明确记录"`Manager.Verify` 的 kid 防御深度缺口 Story 1.3 前必补"。本 story 借挂载生产鉴权中间件之机同步偿还 —— 未来 Manager 被其他 package 消费时不会重新暴露该缺口。
- **Story 1.6 锚点**：`Hub.DisconnectUser` 是 Story 1.6 账户注销的必要依赖之一；`RevokeAllUserTokens` 只吊销 refresh，**不**踢 WS；没有本 story 的 DisconnectUser，账户注销后存续 WS 连接仍可以照常发包。

### 关键依赖与 Epic 0/1.1/1.2 资产复用

| 来源 | 资产 | Story 1.3 用法 |
|---|---|---|
| 0.2 | `Runnable` + `initialize()` | wiring JWTAuth middleware |
| 0.5 | `logx.Ctx(ctx)` + `logx.WithUserID` + requestId 中间件 | happy path 注入 userId 到 ctx logger；reject 审计日志带 path |
| 0.6 | `ErrAuthInvalidIdentityToken` / `ErrAuthTokenExpired` / `ErrInternalError` | 复用全部；**不新增**错误码 —— §21.1 drift gate N/A |
| 0.7 | `clockx.Clock` / `FakeClock` | 集成测试用 FakeClock 驱动 token exp |
| 0.9 | `ws.Hub` / `ws.Client` / `ws.UpgradeHandler` / `ws.TokenValidator` | 扩展 Client 两字段 + `DisconnectUser`；改 Validator 签名 |
| 0.14 | `validateRegistryConsistency` + `ws-registry` | `/v1/platform/ws-registry` 必须维持 pre-auth；本 story 在 AC7 路由组挂载时显式避开它 |
| 1.1 | `ws.NewJWTValidator(jwtMgr)` / `pkg/jwtx.Manager.Verify` / CustomClaims{uid,did,plat,ttype} | HTTP middleware + WS validator 共用 Manager.Verify；claim shape 已固定 |
| 1.1 | `Story 1.1 round 2 fix`：WS validator 用 `AppError.WithCause` 防止 401→500 塌缩 | 本 story HTTP middleware 必须同模式；reject 分支全走 `dto.ErrAuth*.WithCause(err)` |
| 1.2 | `jwtx.Manager.issueClock` + `Verify(.WithTimeFunc)` 绑定的 FakeClock | 集成测试可以用 FakeClock 驱动 "access 过期" 场景 |
| 1.2 | refresh token `claims.TokenType = "refresh"` | middleware 的 "refresh 冒充 access" 拒绝路径（AC2 step 4）消费此 claim |

### fail-closed / fail-open 完整声明（架构指南 §21.3 强制）

（亦见 AC11 矩阵；此段是架构指南要求的**集中**声明点。）

- **HTTP middleware 所有失败路径**：**fail-closed**。无凭证 / 无效凭证 / claim 残缺 一律 401；不存在"Redis 挂了就假装 token 有效"这类便利。
- **WS upgrade 所有失败路径**：**fail-closed**（既有；本 story 不改）。
- **WS 存续期 access token 过期**：**fail-open（刻意）**。Epic line 823 明文 "access token 在 WS 连接存续期间过期不中断连接"。理由：mid-connection re-verify 会抵消 Story 0.12 session.resume cache 的意义（每条消息都要查 Mongo 验 token），且 WS 连接本身就是"信任基于握手时的凭证"模型（与 Redis / PostgreSQL 协议同理）。补偿：Story 1.6 account deletion 调 `Hub.DisconnectUser` 做**显式**切断；未来 Story X（若需要更严格）可追加"access token jti 黑名单"机制，本 story 不做。
- **`DisconnectUser` close-frame 写失败**：**吞错 + 日志**（Warn）。理由：不能让一个坏连接阻塞后续所有驱逐；`unregisterClient` 后该 client 已从 hub 清除，close-frame 写失败的唯一后果是客户端看不到优雅的 "1000 revoked" 而是 tcp reset —— 可接受。
- **反用户 memory `feedback_no_backup_fallback.md`**：无 backup / fallback。每条失败通路直接反映成 HTTP 401 / 500 / WS close，由客户端按 AC13 §13 规范收敛。

### 反模式 TL;DR 实施期自检（对应 review-antipatterns.md）

1. close(channel) —— N/A（本 story 无 channel 新增 / 关闭）
2. goroutine panic recover —— N/A（middleware 在 gin goroutine 里，`Recover` middleware 已在外层兜）
3. shutdown-sensitive I/O —— N/A（middleware 无 I/O，Hub.DisconnectUser 的 WriteControl 有 5s deadline，不会阻塞 shutdown）
4. **全局常量**：无新错误码 / WS 类型 / config 字段 —— §21.1 drift gate N/A；任何新常量必须先改 AC
5. **新 config 字段**：无
6. **JWT**：`Manager.Verify` 已覆盖 §3.1-§3.4；本 story AC1 同步补 §3.3 "kid not a string" 防御深度；§3.5 N/A（本 story 不签 token）
7. **debug/release mode gate**：AC8 双测试锁死；反模式 §7.1 "conditional gate 写反" 风险由双 mode test 覆盖
8. **Redis key**：N/A
9. **rate limit**：本 story 不加 per-user HTTP rate limit；未来 story 需要 HTTP 限流时独立规划
10. **度量 / 比率**：本 story 不加 metric；audit log 字段由 AC10 契约锁定
11. **中间件顺序**：Logger → Recover → RequestID → (v1 group) JWTAuth —— Logger 最外层 access log 永远跑到；Recover 内层兜 JWTAuth 的意外 panic；RequestID 继续为每个请求生成 ID

### 关于 "debug 模式不挂 HTTP JWTAuth" 的决策

- **选择**：debug 模式不挂 HTTP JWTAuth；release 模式挂上 `JWTAuth(jwtMgr)`。
- **理由**：
  1. debug 模式 MVP 下没有任何 /v1/* 业务 endpoint 需要保护（`/v1/platform/ws-registry` 本来就是 pre-auth）；挂上等于白噪音
  2. 未来 Story 1.4-1.6 / Epic 2-5 若需要在 debug 下手动测试 /v1/* endpoint，可以在 `initialize.go` 的 debug 分支里显式加 `middleware.JWTAuth(debugVerifier)`；或者临时构造 access token
  3. 保持与 WS validator 的 debug/release 分叉一致（debug 用 DebugValidator 直接通过 token，release 用 JWTValidator 严格验证）
- **取舍**：debug 环境下如果有人忘记在 release 前验证 JWT 鉴权，可能在 release mode 启动时才发现 middleware bug；但 AC3 的单测 + AC8 的双模式 initialize test + AC9 的集成测试已覆盖三层防护，此风险可接受

### 关于 `UserIDFrom` 返回 typed `ids.UserID` 的原因

- 架构 §15.3 Typed IDs 强制；handler 消费该值时如果是 string，很容易被"再去 c.GetString("userId")"之类的写法绕过封装
- 返回 typed 后，handler 方法签名必然是 `func Do(ctx, uid ids.UserID, ...)`，service 层也统一用 `ids.UserID`，编译期就能拦截"把 deviceID 当 userID 传下去"的事故
- `DeviceIDFrom` 返 `string` 因为 `ids` 包 MVP 阶段没有 `DeviceID` typed alias（客户端生成的 UUID，与 DeviceToken / APNs token 等概念并列，MVP 不强求 typing）；未来若觉得需要可在 Story 1.4 补 `ids.DeviceID` 然后本函数改签名（小 refactor）

### 关于"WS 连接存续期不重验 access token"的已知风险窗口

- **场景**：用户 A 在 Watch 上登录 → Story 1.6 注销账户 → 1.6 flow 调 `RevokeAllUserTokens` + `Hub.DisconnectUser` → 但此前 5 秒已经通过 `/ws` 建连的 Watch client 仍然可能在 `h.unregisterClient` 之前发一条消息（race window ~100ms 量级，受 sync.Map Range 快照影响）
- **MVP 可接受**：窗口小 + 用户本来就是自己主动注销自己 + 发的消息会经过 dispatcher 路由到各 handler，每个 handler 对"用户已注销 / 不存在" 的路径会返 AppError（Mongo 查不到 → 业务错）—— 相当于业务层自然 fail-closed 补偿
- **不可接受时的补丁**：追加 "access token jti 黑名单 + WS upgrade 前查黑名单"（架构 §D16 key space 里保留 `access_blacklist:*` 命名空间）；本 story 不做，未来专项 story 规划
- **Completion Notes 必答**：该窗口是否可接受；如不可接受的话，Dev Notes 增补"Story X 规划 access blacklist"条

### 测试自包含（§21.7）落地

- `fakeVerifier`（既有）：happy / err 驱动；middleware 单测不启真 Manager
- zerolog buffer hook：测试 happy path 的 `logx.WithUserID` 注入时，捕获 JSON 断言 `userId` 字段存在
- 集成测试：复用 Story 1.1 FakeApple + Testcontainers Mongo + miniredis + 真实 `initialize(cfg)`；build-tag `integration`
- **不可依赖**：appleid.apple.com / 真 iOS-watchOS app / 真 APNs / cgo race（Windows）
- 禁 `t.Parallel()` 在集成测试（架构 §M11）

### Semantic-correctness 思考题（§21.8 / §19 第 14 条强制）

> **如果这段代码运行时产生了错误结果但没有 crash，谁会被误导？**
>
> **答**：所有 /v1/* endpoint 的下游消费者 —— handler / service / repository 全部基于 "ctx 里的 userId 是合法验证过的"这一假设运作。以下 7 个陷阱必须在 Completion Notes self-audit：
>
> 1. **Verify err 被当 happy path bug**：如果 middleware 的 `if err != nil { ... }` 分支写反（e.g. `if err == nil { return 401 }`），任何垃圾 token 都能通过 middleware，下游所有 handler 在空 / 攻击者构造的 userId 上跑。AC3 的 `TestJWTAuth_VerifyError` + `TestJWTAuth_AbortsDownstream` 双重锁定。
>
> 2. **TokenType 不校验 bug**：如果 middleware 忘了校验 `claims.TokenType == "access"`，refresh token 可以直接当 access 用；refresh TTL 30 天 ≫ access TTL 15 分钟 → 攻击者一次偷到 refresh 就能访问 /v1/* 长达 30 天。**AC2 step 4** + AC3 `TestJWTAuth_RefreshTokenAsAccess` 锁定。
>
> 3. **空 userId / deviceId claim 蒙混 bug**：如果 middleware 不检查 `claims.UserID == ""` 或 `claims.DeviceID == ""`，病态 token（签名对但 claim 残缺，例如旧版 token 或未来 claim 结构演化遗漏）会让 ctx 里 userId 为空；下游 `UserIDFrom(c) == ""` 被当合法 userId 查 Mongo → `FindByID(ctx, "")` → ErrNotFound 或更糟的"空字符串匹配了某条 `_id: ""` 的脏数据"。AC3 `TestJWTAuth_EmptyUID` + `TestJWTAuth_EmptyDeviceID` 锁定。
>
> 4. **kid 防御深度缺口 bug（Story 1.2 遗留）**：如果 `Manager.Verify` 的 kid 提取仍然 `kid, _ := ...` 丢 err，攻击者构造 `header["kid"] = 42`（非 string）的 token → Verify 返 `unknown kid: ""` err（因为 kid 变 `""`）→ 行为上等于"被拒"，但日志里看不到"kid 格式错"的精确原因，追踪困难；更严重的是**未来**如果有人重构该行 kid-check 逻辑（比如把 `if kid == activeKID` 改成 `if kid != ""`），非 string kid 的 token 就能用 `""` 蒙混过去。AC1 的 3 个新单测锁死防御深度，避免未来重构破功。
>
> 5. **HTTP Bearer 解析 bug（大小写 / 多空格 / 无空格）**：如果 middleware 的 Bearer 解析用 `strings.TrimPrefix(header, "Bearer ")` 简单实现，`Authorization: bearer token`（小写） 会被当成 `"bearer token"` 整串 token；`Authorization: Bearer  token`（双空格）会被当成 `" token"`（前缀空格）。任一都让合法 token 看起来像"非法 token"被拒 → 用户反复 401 → 以为是 Apple 或服务端问题，实际是解析 bug。AC4 的 `bearer_test.go` 7+ 子 case 覆盖。
>
> 6. **debug/release mode gate 写反 bug（反模式 §7.1）**：如果 initialize.go 写成 `if cfg.Server.Mode == "debug" { mountJWTAuth }`（刚好反了），release 部署不挂 middleware —— 线上 /v1/* 永远放行任何人。AC8 的双 test case 锁死；initialize_test.go 必须**同时**有 ReleaseMode 和 DebugMode 两个 case，缺一个就失去反向保护。
>
> 7. **Hub.DisconnectUser 不遍历全部 client bug**：如果实现写成 "找到第一个匹配 userID 的 client 就 break"，Story 1.6 注销只能踢一个 device（比如 Watch），iPhone 的 WS 连接还能继续用；attacker 持有 iPhone 上的 access token 仍能发包。AC6 的 `TestHub_DisconnectUser_MultipleConnections_SameUser` 锁定"全部踢"语义。
>
> **Dev agent 实施完成后在 `Completion Notes List` 里明确写一段"以上 7 个陷阱哪些已被 AC/测试覆盖"的 self-audit；任一条答"未覆盖"必须立即补测试或修代码。**

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.3 JWT 鉴权中间件 + userId context 注入 — line 807-826]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-3 Access token 有效期 ≤ 15min — line 886]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-10 审计日志 — line 893]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-OBS-3 必含字段 camelCase — line 928]
- [Source: _bmad-output/planning-artifacts/architecture.md#Project Structure internal/middleware/jwt_auth.go — line 872-873]
- [Source: _bmad-output/planning-artifacts/architecture.md#HTTP API 边界 /auth/apple + /auth/refresh bootstrap — line 990-998]
- [Source: _bmad-output/planning-artifacts/architecture.md#横切关注点 JWT 鉴权 — line 1031]
- [Source: docs/backend-architecture-guide.md#§6.1 Handler middleware.UserIDFrom(c) — line 232-240]
- [Source: docs/backend-architecture-guide.md#§13 AuthRequired 仅挂 /v1/* — line 666-668]
- [Source: docs/backend-architecture-guide.md#§21.3 Fail-closed vs Fail-open — line 863-883]
- [Source: docs/backend-architecture-guide.md#§21.4 AC review 早启（guard/auth 类必触发）— line 885-899]
- [Source: docs/backend-architecture-guide.md#§21.7 Server 测试自包含 — line 925-937]
- [Source: docs/backend-architecture-guide.md#§21.8 语义正确性思考题 — line 939-941]
- [Source: server/agent-experience/review-antipatterns.md#§3.1-§3.5 JWT 安全边界（§3.3 kid 缺口由本 story 补齐）]
- [Source: server/agent-experience/review-antipatterns.md#§7.1-§7.2 Release/Debug mode gate — line 197-207]
- [Source: server/agent-experience/review-antipatterns.md#§10.1-§10.3 中间件顺序 / Logger 外层 / zerolog.Ctx nil 陷阱 — line 249-264]
- [Source: server/agent-experience/review-antipatterns.md#§13.1 pkg/ ← internal/ 禁止 — line 292-295]
- [Source: server/pkg/jwtx/manager.go#L144-L160 — 既有 Issue（caller 传入 jti 保留）]
- [Source: server/pkg/jwtx/manager.go#L169-L193 — 既有 Verify（本 story AC1 小改 kid 段）]
- [Source: server/pkg/jwtx/manager.go#L265-L337 — VerifyApple（kid 防御深度对齐模板）]
- [Source: server/internal/ws/jwt_validator.go — 既有 JWTValidator 模式复用（本 story AC5 扩展）]
- [Source: server/internal/ws/upgrade_handler.go#L17-L177 — 既有 TokenValidator interface + extractBearerToken + Client 构造]
- [Source: server/internal/ws/hub.go#L120-L170 — FindByUser / unregisterClient / Client struct（本 story AC6 扩展）]
- [Source: server/internal/middleware/logger.go + recover.go + request_id.go — 既有三件套顺序（本 story 新 middleware 挂 v1 group 内层）]
- [Source: server/internal/dto/error_codes.go#L23-L25 — AUTH_INVALID_IDENTITY_TOKEN / AUTH_TOKEN_EXPIRED / AUTH_REFRESH_TOKEN_REVOKED 复用]
- [Source: server/internal/service/auth_service.go#L444-L669 — Story 1.2 RefreshToken 流程（本 story 不改，仅确认 refresh token claim shape 稳定）]
- [Source: server/cmd/cat/initialize.go#L104-L141 — Story 1.1 + 1.2 wiring 区块（本 story 在其后追加 Story 1.3 middleware wiring）]
- [Source: server/cmd/cat/wire.go — 既有 /auth/* + /ws + /healthz + /v1/platform/ws-registry 路由（本 story 追加 /v1 group）]
- [Source: _bmad-output/implementation-artifacts/1-1-user-domain-sign-in-with-apple-jwt.md#AC7 — SignInWithApple 流程 + JWT 签发签名]
- [Source: _bmad-output/implementation-artifacts/1-2-refresh-token-revoke-per-device-session.md#AC5 — RefreshToken 流程 + claim shape]
- [Source: _bmad-output/implementation-artifacts/1-2-refresh-token-revoke-per-device-session.md#AC15 — 反模式 §3.3 "kid not a string" 防御深度技术债 Story 1.3 前必补]
- [Source: docs/api/openapi.yaml — 当前 version 1.2.0-epic1（本 story bump 到 1.3.0-epic1）]
- [Source: docs/api/integration-mvp-client-guide.md#§11 SIWA / §12 Refresh — 本 story 追加 §13 HTTP 鉴权 + §14 账户注销 WS 断开]

### Project Structure Notes

- 完全对齐架构指南 `internal/{middleware, ws}` + `pkg/jwtx` 分层；无新 package
- **新建文件（3-4 个）**：
  - `server/internal/middleware/jwt_auth.go`
  - `server/internal/middleware/jwt_auth_test.go`
  - `server/internal/middleware/bearer.go` + `bearer_test.go`（**可选**，Task 3 视情况；不抽出则免建）
  - `server/cmd/cat/jwt_middleware_integration_test.go`
- **修改文件（8-10 个）**：
  - `server/pkg/jwtx/manager.go`（Verify kid 防御深度）
  - `server/pkg/jwtx/manager_test.go`（3 个新 case）
  - `server/internal/ws/upgrade_handler.go`（AuthenticatedIdentity + TokenValidator 新签名 + Client 构造）
  - `server/internal/ws/upgrade_handler_test.go`（fakes 更新 + 新断言）
  - `server/internal/ws/jwt_validator.go`（TokenValidator 新签名实现 + empty deviceID 拒绝）
  - `server/internal/ws/jwt_validator_test.go`（`TestJWTValidator_RejectsEmptyDeviceID` 新 case）
  - `server/internal/ws/hub.go`（Client 两字段 + 两 getter + DisconnectUser）
  - `server/internal/ws/hub_test.go`（5 个 DisconnectUser 子 case）
  - `server/cmd/cat/wire.go`（/v1 group + jwtAuth 字段）
  - `server/cmd/cat/initialize.go`（middleware.JWTAuth wiring + debug/release 分叉）
  - `server/cmd/cat/initialize_test.go`（双 mode gate test）
  - `docs/api/openapi.yaml`（securitySchemes + 全局 security + bootstrap 豁免 + version bump）
  - `docs/api/integration-mvp-client-guide.md`（§13 + §14）
- 无新 external dependency（一切 stdlib + gin + gorilla/websocket + zerolog + 既有）
- 无架构偏差；无新 Empty Provider 替换
- 无新 WSMessage；无新 error code；无新 config 字段 → §21.1 drift gate N/A

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- `bash scripts/build.sh --test` 单测套件全绿（详见 Task 9 checklist）
- `go test ./internal/middleware/... -v` — 14 个 jwt_auth 子 case + 10 个 ExtractBearerToken table case + 既有 logger/recover/request_id 全绿
- `go test ./internal/ws/... -v` — 既有 ws 测试 + 6 个 DisconnectUser 子 case 全绿
- `go test ./pkg/jwtx/... -v` — 既有 manager 测试 + 3 个新 kid 防御深度 case 全绿
- `go test ./cmd/cat/... -run "TestRouter|TestBuildHTTPJWTAuth"` — 7 个新 wire/mode-gate 测试全绿
- `go vet -tags=integration ./...` + `go test -tags=integration -c ./cmd/cat/...` 编译通过

### Completion Notes List

#### AC self-review（§21.4 强制，guard/auth middleware 类）

实施前对照 AC2 流程 1-8 步 + AC11 决策矩阵逐条 walk-through：

1. **AC2 step 1（header empty/non-Bearer）→ 401 AUTH_TOKEN_EXPIRED**：✅ 实现 `JWTAuth` 第一段 `header := c.GetHeader("Authorization"); if header == "" { rejectAuth(...,"missing_header",...,dto.ErrAuthTokenExpired); return }`，TestJWTAuth_MissingHeader / TestJWTAuth_NonBearerHeader 锁定。
2. **AC2 step 2（提取 Bearer）**：✅ 用 `middleware.ExtractBearerToken`（共享给 ws/upgrade_handler）。reason 区分 `not_bearer` vs `empty_token` 通过额外 prefix 检查，TestJWTAuth_BearerEmptyToken 验证。
3. **AC2 step 3（Verify err → 401 AUTH_INVALID_IDENTITY_TOKEN）**：✅ 决策与 spec 一致，全部 Verify 失败统一映射为 AUTH_INVALID_IDENTITY_TOKEN（包含 exp 失败）；TestJWTAuth_VerifyError + 集成测试 ExpiredToken_Returns401 锁定。
4. **AC2 step 4（refresh→access 拒绝）**：✅ `if claims.TokenType != "access" { ...token_type_mismatch }`，TestJWTAuth_RefreshTokenAsAccess + 集成测试 RefreshTokenRejected 锁定。
5. **AC2 step 5/6（uid/deviceId empty）**：✅ 分别拒绝，TestJWTAuth_EmptyUID / TestJWTAuth_EmptyDeviceID 锁定。
6. **AC2 step 7（platform 可空）**：✅ TestJWTAuth_EmptyPlatformAllowed 锁定。
7. **AC2 step 8（注入 ctx + logger inheritance）**：✅ TestJWTAuth_InjectsLoggerUserID 用 zerolog buffer hook 验证 handler 输出 JSON 含 `userId` 字段。
8. **AC11 fail-closed/open 矩阵**：✅ 全部 7 行 reject 路径走 `dto.RespondAppError(c, ...) + c.Abort()`；唯一 fail-open 项（WS 存续期 access 不重验）已在 Dev Notes / §13.5 客户端文档明确记录，由 Hub.DisconnectUser 提供后置补偿。

#### Semantic-correctness 思考题 7 条 self-audit（§21.8 / §19 强制）

1. ✅ **Verify err 写反 bug** — 已覆盖：TestJWTAuth_VerifyError 锁定 `if err != nil → 401`；TestJWTAuth_AbortsDownstream 锁定 hits 计数为 0。
2. ✅ **TokenType 不校验 bug** — 已覆盖：TestJWTAuth_RefreshTokenAsAccess + 集成测试 RefreshTokenRejected 双重锁定，refresh→AUTH_INVALID_IDENTITY_TOKEN。
3. ✅ **空 uid/deviceId claim 蒙混 bug** — 已覆盖：TestJWTAuth_EmptyUID + TestJWTAuth_EmptyDeviceID + WS 侧 TestJWTValidator_RejectsEmptyDeviceID。
4. ✅ **kid 防御深度缺口 bug** — 已覆盖：3 个新 manager 单测 (Missing/NotAString/Empty) + 既有 UnknownKID 共 4 分支锁定。AC1 改写后 `Verify` 与 `VerifyApple` 文字对齐。
5. ✅ **Bearer 解析 bug（大小写/多空格/无空格）** — 已覆盖：bearer_test.go 10 个 table-driven case 含 lowercase/双空格/Bearerfoo run-on 等；middleware/ws 共用同一函数 `ExtractBearerToken`，二者不可能漂移。
6. ✅ **debug/release mode gate 写反 bug** — 已覆盖：TestBuildHTTPJWTAuth_ReleaseModeMounted + TestBuildHTTPJWTAuth_DebugModeNotMounted 双向锁定；TestBuildHTTPJWTAuth_UnknownModeTreatedAsRelease 额外覆盖 typo/未来模式 fail-closed 默认。
7. ✅ **Hub.DisconnectUser 不遍历全部 client bug** — 已覆盖：TestHub_DisconnectUser_MultipleConnections_SameUser 注册 watch + iphone 两个 client，断言 count == 2，验证「全部踢」语义；TestHub_DisconnectUser_OtherUsersNotAffected 反向锁定不误杀其他用户。

#### §21 纪律自证（AC15）

- §21.1 双 gate 漂移守门 — **N/A**：本 story 不引入新 error code / WSMessage / config 字段。
- §21.2 Empty Provider 填实 — **N/A**：本 story 不替换任何 Empty Provider；grep 结果与 Story 1.2 结束态对等（5 条 session.resume Empty + 1 条 push.EmptyTokenProvider）。
- §21.3 fail-closed vs fail-open — ✅ AC11 矩阵 + Dev Notes 完整声明，唯一 fail-open（WS 存续期 access 不重验）有补偿（Hub.DisconnectUser）。
- §21.4 AC review 早启 — ✅ 实施前已逐条 walkthrough（见上）。
- §21.5 tools/* CLI — **N/A**：未引入。
- §21.6 spike/真机/物理执行 — **N/A**：纯代码。
- §21.7 server 测试自包含 — ✅ 单测无任何 APP/watch/真 APNs/真 Apple JWKS 依赖；集成测试用 FakeApple + Testcontainers Mongo + miniredis；DisconnectUser 用 httptest WS loopback（无外部依赖）。
- §21.8 语义正确性思考题 — ✅ 7 题已答（见上）。

#### Story 1.6 race window 接受声明

WS 存续期不重验 access token + RevokeAllUserTokens 与 DisconnectUser 之间的 ~100ms 量级 race window：本 story 接受。理由（重述 Dev Notes）：
- 业务下游（Mongo 查 deletion_requested = true）天然 fail-closed；
- 客户端是「自己注销自己」的语义，攻击面有限；
- 真正的 access token jti 黑名单可由 Story X 单独规划（架构 §D16 已预留 `access_blacklist:*` 命名空间）。

谁会被误导？答：**短窗口内**该 userId 的「新建」WS 连接（如果在 RevokeAllUserTokens 完成与 DisconnectUser 完成之间通过 /ws upgrade 进来）。规模可控且业务层有兜底，MVP 接受。

#### 实施期偏离 spec 的小决策

- **Task 6 mode-gate 测试通过 helper 单测而非 booting initialize()**：spec 建议「`TestInitialize_ReleaseMode_HTTPJWTAuthMounted` — 启动 release mode → `handlers.jwtAuth != nil`」。booting 完整 initialize() 需要真 Mongo + Redis 连接，等价于集成测试。改抽出 `buildHTTPJWTAuth(mode, verifier)` helper 单测，对 §7.1 反向 gate 风险的覆盖等价但成本远低。这与 spec 目标（防止 mode gate 写反）一致，被覆盖的不变量没有缩水。
- **DisconnectUser 测试用 httptest WS loopback**：spec 提议「与 Story 0.9 既有 `TestHub_FindByUser` 相同 pattern」即裸 `&Client{conn: nil}`。但 `DisconnectUser` 调用 `c.conn.WriteControl + c.conn.Close`，nil conn 会 panic。改用 httptest WS loopback 提供真 *websocket.Conn —— gorilla/websocket v1.5.3 的 NewConn 是 unexported，httptest 是最低成本方案；该测试仍为单测（无 build tag），随 `bash scripts/build.sh --test` 跑。

### File List

**新建文件（4 个）**：
- `server/internal/middleware/jwt_auth.go`
- `server/internal/middleware/jwt_auth_test.go`
- `server/internal/middleware/bearer.go`
- `server/internal/middleware/bearer_test.go`
- `server/internal/ws/hub_disconnect_user_test.go`
- `server/cmd/cat/jwt_middleware_integration_test.go`

**修改文件（10 个）**：
- `server/pkg/jwtx/manager.go` — `Verify` kid 提取改写（AC1）
- `server/pkg/jwtx/manager_test.go` — 3 个新 case (MissingKid/KidNotAString/EmptyKid)
- `server/internal/ws/upgrade_handler.go` — `AuthenticatedIdentity` 新结构 + TokenValidator 签名改 + DebugValidator/StubValidator 适配 + Client 构造填三字段；`extractBearerToken` 删除并改用 `middleware.ExtractBearerToken`
- `server/internal/ws/jwt_validator.go` — `ValidateToken` 签名改 + 新增 empty deviceId 拒绝分支
- `server/internal/ws/jwt_validator_test.go` — `TestJWTValidator_HappyPath` 改三字段断言；新增 `TestJWTValidator_RejectsEmptyDeviceID`
- `server/internal/ws/hub.go` — Client 加 deviceID/platform 字段 + 两 getter；新增 `Hub.DisconnectUser`
- `server/cmd/cat/wire.go` — `handlers.jwtAuth` + `v1Routes` 字段；`buildRouter` 加 `/v1` group
- `server/cmd/cat/initialize.go` — `buildHTTPJWTAuth` helper + wiring
- `server/cmd/cat/initialize_test.go` — 7 个新测试（3 mode gate + 4 router）
- `docs/api/openapi.yaml` — version bump + securitySchemes.BearerAuth + 全局 security + 3 处 bootstrap 显式 `security: []`
- `docs/api/integration-mvp-client-guide.md` — 新 §13 + §14
- `server/_bmad-output/implementation-artifacts/sprint-status.yaml` — Story 1-3 in-progress

### Change Log

| Date | Version | Author | Summary |
|------|---------|--------|---------|
| 2026-04-19 | 0.1 | sm | Story 1.3 草稿创建，15 条 AC，9 个 Task，ready-for-dev；覆盖 HTTP JWT 中间件 + WS upgrade claim 扩展 + Hub.DisconnectUser + kid 防御深度补齐（Story 1.2 技术债） + /v1/* 路由组挂载 |
| 2026-04-19 | 1.0 | dev | Story 1.3 实现完成，9 Tasks 全绿；新建 4 文件 + 修改 10 文件；本地 `bash scripts/build.sh --test` 全绿；集成测试 `go test -tags=integration -c` 编译通过待 Linux CI Docker 跑；AC self-review + Semantic-correctness 7 思考题全部回答 |
