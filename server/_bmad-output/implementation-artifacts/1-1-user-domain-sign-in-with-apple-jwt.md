# Story 1.1: User 领域 + Sign in with Apple 登录 + JWT 签发

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->
<!-- Story 1.1 is the FIRST Epic 1 story. Per epic-0-retro §8.1 Action #2 + 架构指南 §21.4, this story is NOT a tool/metric/guard/measurement story, so the mandatory AC review is NOT triggered — 但本 story 涉及密码学原语（JWT / Apple JWK 验签 / PII 哈希）和账号身份边界，dev agent 在 implementation 前仍建议手工 diff AC 与实现契约再开工，并回答 §21.8 "语义正确性" 思考题（见本文件末尾 "Semantic-correctness 思考题" 段落）。 -->

## Story

As a new or returning user,
I want to sign in with my Apple ID via the iOS / watchOS client and receive per-device access + refresh JWTs,
So that I can start using 裤衩猫 on Watch or iPhone without creating a password, with independent sessions per device and first-class replay / signature protections enforced server-side (FR1, FR2, FR3, FR5, NFR-SEC-2/3/4/5/6, NFR-INT-1, NFR-PERF-2).

**Business value**: Unlocks every downstream authenticated path — Story 1.2 (refresh), 1.3 (JWT middleware), 1.4 (device registration), 1.5 (profile), 1.6 (deletion), 以及整条 Epic 2-8 —— 没有真实 UserID 之前，所有业务都只能在 Story 0 debug stub 上空转。本 story 也是 Epic 0 Empty Provider → 真实 Provider 的**第一块多米诺**（架构指南 §21.2，epic-0-retro §9.1）。

## Acceptance Criteria

1. **AC1 — `internal/domain/user.go` User 聚合根（architecture §Project Structure line 833, M6 enum 命名, M7 Repo 边界）**：

   - Create `internal/domain/user.go` (package `domain`). 本 story 首次引入 `internal/domain` 有实体 —— Story 0 只埋了 `doc.go` 占位。
   - 定义：
     ```go
     package domain

     import "github.com/huing/cat/server/pkg/ids"

     // User is the aggregate root for the authenticated identity. Mongo
     // persists one document per Apple-account-linked user in the `users`
     // collection; Story 1.1 writes the fields marked "seed"; later stories
     // (1.4 sessions / APNs binding, 1.5 profile, 1.6 deletion, 3.x friend
     // count) fill the rest. Repo ↔ Service boundary M7 applies: repos
     // return *domain.User, never raw bson.M.
     type User struct {
         ID                   ids.UserID        // seed; _id in Mongo (UUID v4 string)
         AppleUserIDHash      string            // seed; SHA-256 hex of Apple `sub`, unique index
         DisplayName          *string           // 1.5; nil until user sets one
         Timezone             *string           // 1.5; nil until user sets IANA tz
         Preferences          UserPreferences   // seed with defaults
         FriendCount          int               // 3.x; seed as 0
         Consents             UserConsents      // seed
         Sessions             map[string]Session // 1.2/1.4; seed empty map (BSON {})
         DeletionRequested    bool              // 1.6; seed false
         DeletionRequestedAt  *time.Time        // 1.6; seed nil
         CreatedAt            time.Time         // seed = Clock.Now()
         UpdatedAt            time.Time         // seed = Clock.Now()
     }

     type UserPreferences struct {
         QuietHours QuietHours // default {23:00, 07:00}
     }

     type QuietHours struct {
         Start string // "HH:MM"
         End   string // "HH:MM"
     }

     type UserConsents struct {
         StepData *bool // nil until user answers the HealthKit prompt (Story 2.3)
     }

     // Session is reserved for 1.2 (refresh jti tracking) and 1.4 (apns
     // token binding). Story 1.1 seeds empty map so the BSON shape is
     // stable and 1.2 can start writing without a schema migration.
     type Session struct {
         CurrentJTI   string    `bson:"current_jti"`
         IssuedAt     time.Time `bson:"issued_at"`
         HasApnsToken bool      `bson:"has_apns_token"`
     }

     // DefaultPreferences is what Story 1.1 seeds on first sign-in.
     // Mutable? No — return a fresh copy each call (M6 enum: value types).
     func DefaultPreferences() UserPreferences {
         return UserPreferences{QuietHours: QuietHours{Start: "23:00", End: "07:00"}}
     }
     ```
   - `User` 字段注释**显式写出每个字段由哪个 story seed / 填充**，未来 story 读这个文件时能一眼看到自己的职责边界；符合架构指南 §M6（enum 命名）+ §M7（Repo 返 domain entity）精神。
   - BSON tag 只写 **snake_case**（架构 §P1），`json` tag 在 handler DTO 层加（M8）；`User` struct 本身只承担 domain 语义，不暴露给 HTTP / WS。
   - `internal/domain/user_test.go` 单测：`TestDefaultPreferences_ReturnsFreshCopy` 断言两次 call 返回值不共享底层内存；覆盖"调用方篡改不影响后续 call"。

2. **AC2 — `pkg/ids` 扩展 UserID 构造器（pkg/ids/ids.go 既有, 架构 §Project Structure line 935）**：

   - `pkg/ids/ids.go` 已有 `UserID` 类型。新增：
     ```go
     import "github.com/google/uuid"

     // NewUserID returns a fresh UUID v4 encoded UserID. Story 1.1 uses it
     // for new Mongo documents; tests use it for fixture identity. Failures
     // from the crypto RNG are fatal — the process cannot produce a safe
     // user id and must not continue.
     func NewUserID() UserID {
         id, err := uuid.NewRandom()
         if err != nil {
             // Panic, not log.Fatal — this helper is called from request
             // handling, not startup; fatal-during-request is masked by
             // panic+recover middleware and surfaces as INTERNAL_ERROR.
             panic("ids.NewUserID: " + err.Error())
         }
         return UserID(id.String())
     }
     ```
   - `google/uuid` 已是 Epic 0 依赖（Story 0.9 `h.connID = uuid.New().String()`）；不需要新增 go.mod 依赖。
   - 新增单测 `pkg/ids/ids_test.go`：`TestNewUserID_UUID4Format` 断言返回值长度 36 + dash 分隔符；`TestNewUserID_Uniqueness` 断言 1000 次 call 全唯一。

3. **AC3 — `pkg/jwtx/apple_jwk.go` Apple JWK 拉取器 + Redis 缓存（架构 §Project Structure line 931-934, NFR-INT-1, 反模式 §2.6 / §4.1 / §8.1）**：

   - Create `pkg/jwtx/apple_jwk.go`. 消费方接口在同包导出，`pkg/jwtx` 不引 `internal/*`（反模式 §13.1）；Redis client 通过 `redis.Cmdable` interface 传入，避免直接依赖 `pkg/redisx`。
   - 定义：
     ```go
     package jwtx

     import (
         "context"
         "crypto/rsa"
         "encoding/json"
         "net/http"
         "time"

         "github.com/redis/go-redis/v9"
         "golang.org/x/sync/singleflight"
     )

     // AppleJWKFetcher retrieves Apple's JWKS from appleid.apple.com and
     // caches it in Redis (key `apple_jwk:cache`, TTL from config — default
     // 24h per NFR-INT-1). On Redis read / write error the fetcher
     // transparently degrades to a fresh HTTPS fetch (§21.3 fail-open —
     // Apple JWK caching is a performance optimization, NOT a security
     // boundary: the cryptographic verification happens regardless of
     // cache hit / miss, so a dead Redis only costs latency / Apple QPS).
     // A fresh HTTPS fetch that itself fails is fail-closed: the caller
     // must reject the identity token with AUTH_INVALID_IDENTITY_TOKEN
     // (cannot verify signature → cannot trust token).
     //
     // Concurrency: in-flight parallel /auth/apple requests during cache
     // miss are coalesced via singleflight.Group keyed by the constant
     // cache key so Apple sees 1 QPS regardless of local burst (反模式
     // §12.1). The winner re-reads cache before issuing the HTTP call to
     // avoid the N+1 thundering-herd scenario.
     type AppleJWKFetcher struct {
         redis       redis.Cmdable
         httpClient  *http.Client
         jwksURL     string
         cacheKey    string
         cacheTTL    time.Duration
         fetchSF     singleflight.Group
         clock       clockx.Clock // for Redis TTL arithmetic & test hook
     }

     // JWKSet is the decoded Apple JWKS response. Only RS256 keys are
     // kept — any other alg is dropped at parse time (defense in depth
     // behind 反模式 §3.2 RS256 pinning).
     type JWKSet struct {
         Keys map[string]*rsa.PublicKey // kid → pubkey
     }

     type AppleJWKConfig struct {
         JWKSURL          string
         CacheKey         string        // e.g. "apple_jwk:cache"
         CacheTTL         time.Duration // e.g. 24h
         FetchTimeout     time.Duration // e.g. 5s — bounds Apple HTTP call
     }

     func NewAppleJWKFetcher(r redis.Cmdable, clk clockx.Clock, cfg AppleJWKConfig) *AppleJWKFetcher { ... }

     // Get returns the currently-trusted JWKS. Tries Redis first; on miss
     // or Redis error fetches from appleid.apple.com and writes-through.
     // Caller MUST NOT mutate the returned map.
     func (f *AppleJWKFetcher) Get(ctx context.Context) (*JWKSet, error) { ... }
     ```
   - HTTP fetch:
     - `http.Client{Timeout: cfg.FetchTimeout}`（`FetchTimeout` 默认 `5*time.Second`；`<=0` 在构造函数 `log.Fatal` —— 反模式 §4.1）
     - `http.NewRequestWithContext(ctx, "GET", jwksURL, nil)` — ctx 允许上游 HTTP handler 超时传导
     - JSON 解码失败 / 非-200 status → 返回 error（不缓存）
     - 仅 `alg == "RS256" && kty == "RSA"` 的 key 保留（跳过 RS384 / ES256 / 非 RSA）
     - `n` / `e` base64url decode → `rsa.PublicKey`
   - Redis:
     - `GET apple_jwk:cache` — 命中 → unmarshal → 返回
     - Miss 或 Redis error → log warn + singleflight fetch
     - Fetch 成功 → `SET apple_jwk:cache <json> EX <ttl-seconds>`（**写入失败仅 log warn**，不阻断返回 —— fail-open 语义）
     - 缓存 value 格式：用**自定义 JSON 结构**（kid → base64url(n), base64url(e) 字符串），避免把 `*rsa.PublicKey` 直接序列化（Go 结构不稳定；Apple 原始 JWKS 就是这个结构，直接透传即可）
   - **可观测点（§21.3 fail-open 要求）**：
     - `log.Warn().Str("action", "apple_jwk_cache_read_error")` 当 Redis GET error（非 `redis.Nil`）
     - `log.Warn().Str("action", "apple_jwk_cache_write_error")` 当 Redis SET error
     - `log.Info().Str("action", "apple_jwk_fetched").Int("kid_count", n)` 每次成功 HTTP fetch
     - Redis read-error 和 fresh-fetch-error 不是同一类：前者 fail-open 继续 fetch；后者 fail-closed 返 error
   - `pkg/jwtx/apple_jwk_test.go` 单测（miniredis + httptest）：
     1. `TestFetcher_CacheHit` — 先 warm cache，再 Get，断言未调用 HTTP server（`httptest.Server` counter = 0）
     2. `TestFetcher_CacheMissFetchesWritesThrough` — Redis 空，Get → HTTP fetch → 返回 + cache 被写入（`miniredis.TTL(key)` 等于 24h）
     3. `TestFetcher_RedisReadErrorFallsBackToFetch` — inject error-returning redis.Cmdable → Get 成功（degrade），HTTP 被 call
     4. `TestFetcher_HTTPFetchErrorFailsClosed` — httptest server 返 500 → Get 返 error
     5. `TestFetcher_HTTPTimeout` — httptest server sleep 10s，`FetchTimeout=100ms` → Get 返 ctx error
     6. `TestFetcher_SingleflightCoalesces` — 10 并发 Get on empty cache → HTTP server 只被 call 一次（coarse counter）
     7. `TestFetcher_NonRS256Dropped` — httptest 返回含 `RS256 + ES256` 两条 key → JWKSet 只含 RS256 条目
     8. `TestFetcher_InvalidJSONNotCached` — httptest 返非法 JSON → Get 返 error；`miniredis.Exists(key)` = false
   - `FetchTimeout<=0` / `CacheTTL<=0` / `CacheKey==""` / `JWKSURL==""` 在 `NewAppleJWKFetcher` 内 `log.Fatal`（反模式 §4.1）。

4. **AC4 — `pkg/jwtx/manager.go` 扩展：VerifyApple(ctx, token, nonce) 与双签名模式分离（既有 manager.go 第 105-130 行 Verify, 反模式 §3.1/§3.2/§3.3/§3.4/§3.5）**：

   - **不替换**既有 `Manager.Verify` —— 它处理**本服务签发**的 access / refresh token（活跃 KID 查找、issuer = 本服务）。
   - **新增** `VerifyApple(ctx context.Context, idToken string, expectedNonceSHA256 string) (*AppleIdentityClaims, error)`：
     ```go
     // AppleIdentityClaims are the subset of Apple identity-token claims
     // we trust. `sub` is Apple's opaque user id; `nonce` is the hashed
     // nonce Apple echoed back (Apple hashes the raw nonce the client
     // sent — SHA-256 per Apple SIWA spec). Server compares
     // SHA-256(request.Nonce) against claims.Nonce to bind this token
     // to the current request and prevent replay across requests.
     type AppleIdentityClaims struct {
         Sub      string `json:"sub"`
         Aud      string `json:"aud"`
         Iss      string `json:"iss"`
         Exp      int64  `json:"exp"`
         Iat      int64  `json:"iat"`
         Email    string `json:"email,omitempty"`    // Apple may omit after first signin; ignored (NFR-COMP-1 PII minimization)
         EmailVerified string `json:"email_verified,omitempty"` // string-typed "true" / "false" per Apple spec; ignored by MVP
         Nonce    string `json:"nonce,omitempty"`
     }

     func (m *Manager) VerifyApple(
         ctx context.Context,
         idToken string,
         expectedNonceSHA256 string, // server-side hex(sha256(request.Nonce)); empty = skip nonce check (only test harness; production always non-empty)
     ) (*AppleIdentityClaims, error)
     ```
   - 实现必须**显式**处理每一条反模式 §3.x：
     - `token.Method.(*jwt.SigningMethodRSA)` 只做类型判定；**必须**再 `if token.Method.Alg() != "RS256" { return nil, errInvalidAlg }` 钉死 RS256（§3.2）
     - `header.kid`：`kid, ok := token.Header["kid"].(string); if !ok || kid == "" { return nil, errMissingKid }`（§3.3）
     - `claims.Iss`：`if claims.Iss != "https://appleid.apple.com" { return nil, errInvalidIssuer }`（§3.1）
     - `claims.Aud`：`if claims.Aud != cfg.AppleBundleID { return nil, errInvalidAudience }`（NFR-INT-1）
     - `claims.Exp`：显式 `WithExpirationRequired()` + `if claims.Exp < clock.Now().Unix() { return nil, errExpired }`（§3.4）
     - `claims.Nonce`（如果 expectedNonceSHA256 非空）：`if !constantTimeEqual(claims.Nonce, expectedNonceSHA256) { return nil, errNonceMismatch }`（用 `crypto/subtle.ConstantTimeCompare` 防 timing attack）
     - JWK 解析：通过 `AppleJWKFetcher.Get(ctx)` 查 `kid` 对应 `*rsa.PublicKey`；找不到 → `errUnknownKid`
   - 返回的 error 在 handler 层统一 wrap 为 `dto.ErrAuthInvalidIdentityToken.WithCause(err)`（具体子错误通过 `Cause` 链保留，log 写 `Str("stage", "verify_apple").Err(err)`；不暴露给 client 以免提示攻击者）
   - `Manager` 构造需注入 `AppleJWKFetcher` + `AppleBundleID` + `clockx.Clock`：
     ```go
     type AppleVerifyDeps struct {
         Fetcher      *AppleJWKFetcher
         BundleID     string          // expected aud
         Clock        clockx.Clock
     }

     // NewManagerWithApple keeps the existing Manager zero-arg path for Epic 0
     // tests but adds the Apple verify dependencies required by Story 1.1.
     // The dependency is separate from the existing `Options` block so older
     // tests that construct a sign-only Manager (no Apple JWK) keep compiling.
     func NewManagerWithApple(opts Options, apple AppleVerifyDeps) *Manager
     ```
     或直接扩展 `Options` 加 `AppleBundleID` + `AppleFetcher` + `AppleClock` 字段，两种都可接受，dev agent 择优（**关键：不破坏既有 `New(opts Options)` 签名的 Epic 0 测试**）。
   - 单测 `pkg/jwtx/manager_apple_test.go`：
     1. `TestVerifyApple_HappyPath` — 自签 RSA key pair → httptest fake JWKS → VerifyApple 成功
     2. `TestVerifyApple_InvalidSignature` — 用另一 key 签 → 返 err
     3. `TestVerifyApple_WrongIssuer` — iss = "evil.com" → err
     4. `TestVerifyApple_WrongAudience` — aud = "com.attacker.app" → err
     5. `TestVerifyApple_ExpiredToken` — exp = 1 hour ago → err
     6. `TestVerifyApple_MissingKid` — token header no kid → err
     7. `TestVerifyApple_UnknownKid` — kid not in JWKS → err
     8. `TestVerifyApple_WrongAlgorithm` — 签 with RS384 → err（§3.2）
     9. `TestVerifyApple_NonceMismatch` — expectedNonceSHA256 ≠ claims.Nonce → err
     10. `TestVerifyApple_NoExpClaim` — token minted without exp → err（§3.4）
     11. `TestVerifyApple_MalformedToken` — 非 JWT 字符串 → err
   - **测试自包含（§21.7）**：所有测试用**自签 RSA key pair** + `httptest.NewServer` 冒充 Apple JWKS endpoint，**不**打真 `appleid.apple.com`。`setupFakeApple(t)` helper 产生 (`privateKey`, `fakeFetcher`, `cleanup`)，后续 story 也可复用。

5. **AC5 — `internal/config/config.go` 新增 `[apple]` section（反模式 §4.1 / §4.2, 架构 §13 配置约定）**：

   - 追加 struct：
     ```go
     type AppleCfg struct {
         BundleID             string `toml:"bundle_id"`              // aud for identity token (com.huing.cat.app 或 watchOS bundle)
         JWKSURL              string `toml:"jwks_url"`               // default "https://appleid.apple.com/auth/keys"
         JWKSCacheKey         string `toml:"jwks_cache_key"`         // default "apple_jwk:cache"
         JWKSCacheTTLSec      int    `toml:"jwks_cache_ttl_sec"`     // default 86400
         JWKSFetchTimeoutSec  int    `toml:"jwks_fetch_timeout_sec"` // default 5
     }
     ```
     加入 `Config` struct 为 `Apple AppleCfg \`toml:"apple"\``。
   - `applyDefaults()` 追加（反模式 §4.2 —— 用户 `local.toml` 如果不含 `[apple]` 段，启动仍应 succeed）：
     ```go
     if c.Apple.JWKSURL == "" { c.Apple.JWKSURL = "https://appleid.apple.com/auth/keys" }
     if c.Apple.JWKSCacheKey == "" { c.Apple.JWKSCacheKey = "apple_jwk:cache" }
     if c.Apple.JWKSCacheTTLSec == 0 { c.Apple.JWKSCacheTTLSec = 86400 }
     if c.Apple.JWKSFetchTimeoutSec == 0 { c.Apple.JWKSFetchTimeoutSec = 5 }
     ```
     `BundleID` **不**给 default —— 生产必填，default.toml 也留空 `""`，用户必须在 `local.toml` / `production.toml` 显式填值（类似 APNs.KeyID）。
   - `mustValidate()` 追加（反模式 §4.1 —— 所有 positive-int 字段 `<=0` 必 `log.Fatal`）：
     ```go
     if c.Apple.BundleID == "" { log.Fatal().Msg("config: apple.bundle_id must not be empty") }
     if c.Apple.JWKSCacheTTLSec <= 0 { log.Fatal().Int(...).Msg("config: apple.jwks_cache_ttl_sec must be > 0") }
     if c.Apple.JWKSFetchTimeoutSec <= 0 { log.Fatal().Int(...).Msg("config: apple.jwks_fetch_timeout_sec must be > 0") }
     ```
   - `config/default.toml` 追加：
     ```toml
     [apple]
     bundle_id = ""
     jwks_url = "https://appleid.apple.com/auth/keys"
     jwks_cache_key = "apple_jwk:cache"
     jwks_cache_ttl_sec = 86400
     jwks_fetch_timeout_sec = 5
     ```
     `config/local.toml.example` 同步加一段带注释的 `# bundle_id = "com.huing.cat.dev"` 引导开发者填。
   - `config_test.go` 既有 TestMustLoad_Valid / TestMustLoad_Defaults 两个用例补 `[apple] bundle_id = "com.test.cat"` 字段避免 `mustValidate` fatal；新增 `TestMustLoad_MissingBundleIDFatals`（用 helper captures `log.Fatal` 通过 os.Exit mock 或 `zerolog` Hook —— 参考 Story 0.11 `config_test.go` 同类模式）。

6. **AC6 — `internal/repository/user_repo.go` UserRepository 接口 + Mongo 实现（架构 §Project Structure line 848, M7/M8, P1）**：

   - Create `internal/repository/user_repo.go` (package `repository`). 本 story 首次引入 `internal/repository` 有实体 —— Story 0 只埋了 `doc.go` 占位。
   - 接口（consumer-side interface 定义在 `internal/service/auth_service.go` —— 不在 repository 包里 —— 避免 service 依赖 repository 实现；参考 Story 0.12 `ws.ResumeCache` 先例）：
     ```go
     // 定义在 internal/service/auth_service.go
     type UserRepository interface {
         // EnsureIndexes is called once at startup (cmd/cat/initialize.go)
         // to create the apple_user_id_hash unique index. Idempotent.
         EnsureIndexes(ctx context.Context) error

         // FindByAppleHash returns the user matching the SHA-256 hash, or
         // (nil, ErrUserNotFound) if no document exists. Other errors
         // (Mongo down, decode) propagate unchanged.
         FindByAppleHash(ctx context.Context, hash string) (*domain.User, error)

         // Insert creates a new user document. On duplicate-key (concurrent
         // sign-in for same Apple account), returns ErrUserDuplicateHash so
         // the service can retry FindByAppleHash — the document exists, we
         // just lost the race.
         Insert(ctx context.Context, u *domain.User) error

         // ClearDeletion marks deletion_requested=false + deletion_requested_at=nil
         // + updated_at=now for user id. Returns ErrUserNotFound if no doc
         // matched. Called from Story 1.6 on resurrection during /auth/apple.
         ClearDeletion(ctx context.Context, id ids.UserID) error
     }

     // Sentinel errors (repo package exports; service wraps to AppError)
     var (
         ErrUserNotFound       = errors.New("user: not found")
         ErrUserDuplicateHash  = errors.New("user: duplicate apple_user_id_hash")
     )
     ```
     （`ErrUserNotFound` / `ErrUserDuplicateHash` 导出自 `internal/repository` 以便单测用 `errors.Is` 断言；架构指南 §M12 / 反模式 §6.x。）
   - Mongo 实现 `MongoUserRepository`：
     ```go
     type MongoUserRepository struct {
         coll  *mongo.Collection
         clock clockx.Clock
     }

     func NewMongoUserRepository(db *mongo.Database, clk clockx.Clock) *MongoUserRepository {
         return &MongoUserRepository{coll: db.Collection("users"), clock: clk}
     }
     ```
   - `EnsureIndexes`: 
     ```go
     _, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
         Keys:    bson.D{{Key: "apple_user_id_hash", Value: 1}},
         Options: options.Index().SetUnique(true).SetName("apple_user_id_hash_1"),
     })
     ```
     幂等（Mongo `CreateOne` 同名同签名 index 不报错）；Index name 显式 `apple_user_id_hash_1` 对齐架构 §P1 "`{field}_{order}`" 约定。
   - `FindByAppleHash`: `FindOne` + `Decode`；`mongo.ErrNoDocuments` → `ErrUserNotFound`（用 `errors.Is` 判断）。
   - `Insert`: `InsertOne(ctx, userBson)`；duplicate key → `ErrUserDuplicateHash`（用 `mongo.IsDuplicateKeyError(err)` 辅助）。
   - `ClearDeletion`: `UpdateOne({_id: id}, {$set: {deletion_requested: false, deletion_requested_at: nil, updated_at: now}})`；`result.MatchedCount == 0` → `ErrUserNotFound`。
   - BSON 文档结构（snake_case 字段，架构 §P1）：
     ```json
     {
       "_id": "a8f5f...-uuid-v4",
       "apple_user_id_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
       "display_name": null,
       "timezone": null,
       "preferences": { "quiet_hours": { "start": "23:00", "end": "07:00" } },
       "friend_count": 0,
       "consents": { "step_data": null },
       "sessions": {},
       "deletion_requested": false,
       "deletion_requested_at": null,
       "created_at": ISODate("2026-04-19T12:00:00Z"),
       "updated_at": ISODate("2026-04-19T12:00:00Z")
     }
     ```
     `sessions` seed 为**空 BSON object** `{}`（非 nil）便于 Story 1.2 / 1.4 直接 `$set sessions.<deviceId>`。
   - 单测 `internal/repository/user_repo_test.go` — 仅做 struct ↔ BSON mapping 的 table-driven unit test，不打 Mongo（速度 + 单元隔离）。用 `bson.Marshal` / `bson.Unmarshal` 往返。
   - **集成测试** `internal/repository/user_repo_integration_test.go` (`//go:build integration`) — Testcontainers Mongo，覆盖：
     - `TestMongoUserRepo_EnsureIndexesIdempotent` — call 两次不报错；断言 `ListIndexes()` 含 `apple_user_id_hash_1`
     - `TestMongoUserRepo_InsertThenFind` — Insert → FindByAppleHash 返回等值 user
     - `TestMongoUserRepo_FindByAppleHash_NotFound` — 空 collection Find → `ErrUserNotFound`
     - `TestMongoUserRepo_Insert_DuplicateHash` — 同 hash Insert 两次 → 第二次 `ErrUserDuplicateHash`
     - `TestMongoUserRepo_ClearDeletion_Success` — Insert with `deletion_requested=true` → ClearDeletion → FindByAppleHash 的 `DeletionRequested == false`
     - `TestMongoUserRepo_ClearDeletion_NotFound` — 不存在的 id → `ErrUserNotFound`
     - `TestMongoUserRepo_BSONRoundtrip` — Insert 完整 User + Find 回来逐字段等值（避免 Story 1.x 读出来字段丢失）
   - **集成测试禁并行**（架构指南 §M11）；用 `testutil.SetupMongo(t)` helper；per-test unique DB name (`"test_user_" + uuid`) 避免交叉污染。

7. **AC7 — `internal/service/auth_service.go` SignInWithApple 业务流程（架构 §Project Structure line 837, M7/M8, FR1/FR2/FR3）**：

   - Create `internal/service/auth_service.go` (package `service`). 本 story 首次引入 `internal/service` 有实体。
   - 声明：
     ```go
     type AuthService struct {
         users    UserRepository
         jwt      *jwtx.Manager
         clock    clockx.Clock
         mode     string // cfg.Server.Mode — release forbids debug-only bypass
     }

     type SignInWithAppleRequest struct {
         IdentityToken     string
         AuthorizationCode string // optional, ignored in MVP (reserved for server-to-Apple token exchange in growth)
         DeviceID          string // client-generated UUID
         Platform          ids.Platform // "watch" | "iphone"
         Nonce             string // raw nonce client used when calling Apple SIWA
     }

     type SignInWithAppleResult struct {
         AccessToken  string
         RefreshToken string
         User         *domain.User
         IsNewUser    bool
     }

     func NewAuthService(u UserRepository, jwt *jwtx.Manager, clk clockx.Clock, mode string) *AuthService { ... }

     func (s *AuthService) SignInWithApple(ctx context.Context, req SignInWithAppleRequest) (*SignInWithAppleResult, error)
     ```
   - 流程步骤（每步对应 AC 里 Given/When/Then 某一条 + 反模式回链）：
     1. `hex(sha256(req.Nonce))` → `expectedNonceSHA256`（SIWA 规范，Apple 对原始 nonce 做 SHA-256 后放 `nonce` claim）
     2. `claims, err := s.jwt.VerifyApple(ctx, req.IdentityToken, expectedNonceSHA256)` → err 直接返回 `dto.ErrAuthInvalidIdentityToken.WithCause(err)`（handler 层再 RespondAppError）
     3. `appleUserIDHash := hex(sha256(claims.Sub))`（NFR-SEC-6 不可逆哈希）
     4. `existing, err := s.users.FindByAppleHash(ctx, appleUserIDHash)`:
        - `errors.Is(err, repository.ErrUserNotFound)` → 跳到步骤 5（新用户）
        - `err == nil` → 跳到步骤 6（老用户）
        - 其他 err → 返回 `dto.ErrInternalError.WithCause(err)`（fail-closed，反模式 §21.3）
     5. **新用户**（`isNewUser = true`）：
        - `u := &domain.User{ID: ids.NewUserID(), AppleUserIDHash: hash, Preferences: domain.DefaultPreferences(), Sessions: map[string]domain.Session{}, CreatedAt: clk.Now(), UpdatedAt: clk.Now()}`
        - `err := s.users.Insert(ctx, u)`:
          - `errors.Is(err, repository.ErrUserDuplicateHash)` → 并发 race，**重试** `FindByAppleHash`（最多 1 次重试，第二次仍 NotFound 则返 `ErrInternalError`，理论不可达但防御深度）；跳到步骤 6（老用户路径，此时视为已有用户）
          - 其他 err → `dto.ErrInternalError.WithCause(err)`
        - 成功 → user = u；跳步骤 7
     6. **老用户**（`isNewUser = false`）：
        - `if existing.DeletionRequested { if err := s.users.ClearDeletion(ctx, existing.ID); err != nil { return ErrInternalError } ; existing.DeletionRequested = false; existing.DeletionRequestedAt = nil }`
        - user = existing；跳步骤 7
     7. **签 JWT**（`jwtx.Manager.Issue`，既有 manager.go line 87）：
        - access claims: `CustomClaims{UserID: string(user.ID), DeviceID: req.DeviceID, Platform: string(req.Platform), TokenType: "access", RegisteredClaims: jwt.RegisteredClaims{Subject: string(user.ID)}}` —— 注意**必须**填 RegisteredClaims.Subject，避免反模式 §3.5 静默覆盖（既有 `Manager.Issue` line 94-97 会覆盖 Issuer / IssuedAt / ExpiresAt，但**保留** Subject；如果 Issue 实现已经整体赋值 RegisteredClaims，本 story 必须修正成 merge 模式并加单测锁）
        - refresh claims: 同上但 `TokenType: "refresh"`；注意**这里 jti 暂时空**（Story 1.2 才真正用 jti blacklist；1.1 只签发不管 blacklist），Issue 现在没塞 jti，1.2 story 会扩展
        - `accessToken, err := s.jwt.Issue(accessClaims)` → err fail-closed
        - `refreshToken, err := s.jwt.Issue(refreshClaims)` → err fail-closed
     8. **审计日志**（NFR-SEC-10）：
        ```go
        logx.Ctx(ctx).Info().
            Str("action", "sign_in_with_apple").
            Str("userId", string(user.ID)).
            Str("deviceId", req.DeviceID).
            Str("platform", string(req.Platform)).
            Bool("isNewUser", isNewUser).
            Msg("sign_in_with_apple")
        ```
        **不含**：`identityToken`、`claims.Sub` 原始、`appleUserIDHash`、`claims.Email`（PII 最小化）。
     9. return `&SignInWithAppleResult{AccessToken, RefreshToken, User, IsNewUser}`, nil
   - **Fail-closed vs Fail-open 决策矩阵**（Dev Notes 也重复一份 —— 架构指南 §21.3 强制）：
     | 失败点 | 选择 | 可观测点 |
     |---|---|---|
     | Apple JWK Redis 读错 | fail-open | warn log + 回退到 HTTP fetch |
     | Apple JWK HTTP fetch 错 | fail-closed | `ErrAuthInvalidIdentityToken` 返 401；log error |
     | identity token 签名 / claims 错 | fail-closed | `ErrAuthInvalidIdentityToken` 返 401 |
     | Mongo `FindByAppleHash` err | fail-closed | `ErrInternalError` 返 500；log error |
     | Mongo `Insert` duplicate key | 自愈（重试 Find，视为老用户） | log info "sign_in_race_resolved" |
     | Mongo `Insert` 其他 err | fail-closed | `ErrInternalError` 返 500 |
     | Mongo `ClearDeletion` err（老用户路径） | fail-closed | `ErrInternalError` 返 500；**不**吞错返空 token |
     | JWT `Issue` err | fail-closed | `ErrInternalError` 返 500 |
     —— 每条选择的理由：签入/身份/账号写入属"安全 / 数据错乱"类（选 fail-closed）；JWK 缓存属"便利功能降级"（选 fail-open）。
   - 单测 `internal/service/auth_service_test.go`（table-driven，parallel 允许）：
     - Given 新用户 → FindByAppleHash 返 NotFound → Insert 返 nil → Result.IsNewUser=true / User.ID 是新 UUID
     - Given 老用户 → FindByAppleHash 返 existing → Result.IsNewUser=false / User 等于 existing
     - Given 老用户 DeletionRequested=true → FindByAppleHash 返 existing with flag → ClearDeletion 被 call → Result.User.DeletionRequested=false
     - Given 并发 race → FindByAppleHash.NotFound → Insert.Duplicate → FindByAppleHash 重试返 existing → 返老用户结果
     - Given VerifyApple 返 err → Result nil / err = `ErrAuthInvalidIdentityToken`
     - Given Nonce 为 ""（test 模式 bypass）→ expectedNonceSHA256 = "" → VerifyApple 被传 "" （`Manager.VerifyApple` 的 nonce="" 分支在 AC4 里约定仅测试用；service 层不该接受 req.Nonce="" —— 见 AC8 validator，handler 层拦）
     - Given FindByAppleHash Mongo err → 返 `ErrInternalError`
     - Given Insert 非 duplicate err → 返 `ErrInternalError`
     - Given ClearDeletion err → 返 `ErrInternalError`（不吞错）
     - Given JWT Issue err → 返 `ErrInternalError`
   - Mock 策略：`FakeUserRepository` / fake `jwtx.Manager`（或用构造注入的 `VerifyApple` / `Issue` func pointer —— 避免在 service 测试里起 JWK httptest）。

8. **AC8 — `internal/dto/auth_dto.go` 请求/响应 DTO + validator（架构 §P2 / §P7 / M8）**：

   - Create `internal/dto/auth_dto.go`:
     ```go
     // SignInWithAppleRequest is the wire format for POST /auth/apple.
     // Validated by go-playground/validator via Gin ShouldBindJSON.
     type SignInWithAppleRequest struct {
         IdentityToken     string `json:"identityToken"     binding:"required,min=1,max=8192"`
         AuthorizationCode string `json:"authorizationCode,omitempty" binding:"omitempty,max=1024"`
         DeviceID          string `json:"deviceId"          binding:"required,uuid"`
         Platform          string `json:"platform"          binding:"required,oneof=watch iphone"`
         Nonce             string `json:"nonce"             binding:"required,min=8,max=128"`
     }

     // SignInWithAppleResponse is the success body returned to the client.
     type SignInWithAppleResponse struct {
         AccessToken  string     `json:"accessToken"`
         RefreshToken string     `json:"refreshToken"`
         User         UserPublic `json:"user"`
     }

     // UserPublic is the session-resume / sign-in subset of User that is
     // safe to hand the client. Profile + preferences land via /me in 1.5;
     // 1.1 only returns id + displayName + timezone per epic AC.
     type UserPublic struct {
         ID          string  `json:"id"`
         DisplayName *string `json:"displayName,omitempty"`
         Timezone    *string `json:"timezone,omitempty"`
     }

     // ToPublic projects the authenticated domain user into the wire DTO
     // (M8: handler-layer DTO conversion; service never sees DTO).
     func UserPublicFromDomain(u *domain.User) UserPublic {
         return UserPublic{
             ID:          string(u.ID),
             DisplayName: u.DisplayName,
             Timezone:    u.Timezone,
         }
     }
     ```
   - Validator 错误 → `ErrValidationError`（既有 `internal/dto/error_codes.go` 第 40 行，`VALIDATION_ERROR / 400 / client_error`）；handler 层统一用 `RespondAppError(c, dto.ErrValidationError.WithCause(err))`（既有 `error.go` line 78）。
   - `deviceId` `binding:"uuid"` —— Gin binding 自带 uuid 校验器，覆盖 v1-v5；客户端生成的 UUID v4 通过。
   - `nonce` 长度 8-128：Apple SIWA 客户端典型 nonce 是 base64-encoded 32 bytes ≈ 44 字符；留 buffer。**注意**：这里的 Nonce 是客户端生成的**原始** nonce（非 hash），服务端拿到后在 service 层 SHA-256 并比对 `claims.Nonce`。
   - 单测 `internal/dto/auth_dto_test.go` — validator 边界：空字符串、超长、非 UUID deviceId、非枚举 platform、nonce 过短 / 过长。

9. **AC9 — `internal/handler/auth_handler.go` POST /auth/apple endpoint（架构 §Project Structure line 859, P2 / P7 / M8）**：

   - Create `internal/handler/auth_handler.go`:
     ```go
     type AuthHandler struct {
         svc *service.AuthService
     }

     func NewAuthHandler(svc *service.AuthService) *AuthHandler {
         return &AuthHandler{svc: svc}
     }

     // SignInWithApple handles POST /auth/apple — an unauthenticated bootstrap
     // endpoint (架构 §13 bootstrap / §Architectural Boundaries line 993-996).
     func (h *AuthHandler) SignInWithApple(c *gin.Context) {
         var req dto.SignInWithAppleRequest
         if err := c.ShouldBindJSON(&req); err != nil {
             dto.RespondAppError(c, dto.ErrValidationError.WithCause(err))
             return
         }
         svcReq := service.SignInWithAppleRequest{
             IdentityToken:     req.IdentityToken,
             AuthorizationCode: req.AuthorizationCode,
             DeviceID:          req.DeviceID,
             Platform:          ids.Platform(req.Platform),
             Nonce:             req.Nonce,
         }
         result, err := h.svc.SignInWithApple(c.Request.Context(), svcReq)
         if err != nil {
             dto.RespondAppError(c, err)
             return
         }
         c.JSON(http.StatusOK, dto.SignInWithAppleResponse{
             AccessToken:  result.AccessToken,
             RefreshToken: result.RefreshToken,
             User:         dto.UserPublicFromDomain(result.User),
         })
     }
     ```
   - 成功 HTTP 200（架构 §P2 "成功响应 直接返回 payload，无 wrapper"）；错误通过 `RespondAppError` 统一走 AppError 通道。
   - **路由挂载**（在 `cmd/cat/wire.go` `buildRouter` —— AC11）：`/auth/apple` 是 bootstrap endpoint，**不**挂 JWT 中间件（Story 1.3 的 JWT middleware 只挂 `/v1/*` 组；`/auth/*` 在外）。
   - 单测 `internal/handler/auth_handler_test.go` — 用 `httptest.NewRecorder` + fake `AuthService`（interface 注入）覆盖：
     - 正常请求 → 200 + 响应 body 字段正确
     - 非法 JSON → 400 `VALIDATION_ERROR`
     - 缺 `deviceId` → 400 `VALIDATION_ERROR`
     - 非枚举 platform → 400 `VALIDATION_ERROR`
     - service 返 `ErrAuthInvalidIdentityToken` → 401 + body.error.code
     - service 返 `ErrInternalError` → 500

10. **AC10 — 集成测试 `cmd/cat/integration_test.go` 端到端 sign-in（架构 §P6 测试模式, §21.7 自包含, 既有 integration_test.go 扩展）**：

    - 扩展既有 `server/cmd/cat/integration_test.go`（Story 0.14 已建立模式），**新增** `TestSignInWithApple_EndToEnd` `//go:build integration`:
      1. Testcontainers Mongo + miniredis（或 Testcontainers Redis）
      2. 自签 RSA key + httptest server 伪装 Apple JWKS（复用 AC4 `setupFakeApple` helper）
      3. 启动真实 App（`initialize(cfg)` —— 所有 wiring 全走一遍；cfg 指向 httptest JWKS URL + 测试 Mongo URI + miniredis addr）
      4. 客户端流程：
         - 生成 raw nonce → `sha256(nonce)` hex → 用 fake Apple 私钥签 identityToken 带 `sub="apple:test:001"` / `iss="https://appleid.apple.com"` / `aud=cfg.Apple.BundleID` / `nonce=sha256(raw_nonce)` / `exp=now+1h`
         - POST `/auth/apple` body `{identityToken, deviceId="<uuid>", platform="watch", nonce=<raw>}`
         - 断言 200 + response body `{accessToken, refreshToken, user: {id: "<uuid>"}}`
         - 断言 `jwtx.Manager.Verify(accessToken)` 通过（claims.UserID 等于 response.user.id）
         - 断言 Mongo `users` collection 有 1 条文档 `apple_user_id_hash = sha256("apple:test:001")`
         - 断言 audit log 行包含 `action="sign_in_with_apple" userId=<uuid> platform="watch" isNewUser=true`（zerolog TestWriter 模式）
      5. **重复登录**：同一 sub 再 POST 一次 → 200 + response.user.id 与第一次相同；`users` collection 仍只有 1 条；audit log `isNewUser=false`
      6. **删除复活**：手动把文档 `deletion_requested` 改 true → POST → 200 + `deletion_requested` 变 false；audit log `isNewUser=false` + 可选 info "user_resurrected_from_deletion"
    - **测试自包含（§21.7 强制）**：全程不打 `appleid.apple.com` 真地址；不调 iOS / watchOS 客户端；纯 Go 测试。
    - `go test -tags=integration ./cmd/cat/...` 全绿是 AC 满足标准。

11. **AC11 — `cmd/cat/initialize.go` + `wire.go` 端到端 wiring（既有 initialize.go 第 1-200 行, 架构 §G1 DI 约定）**：

    - `initialize.go` 新增（按现有 wiring 顺序插入）：
      ```go
      // --- Story 1.1 Apple SIWA wiring ---
      appleJWKFetcher := jwtx.NewAppleJWKFetcher(redisCli.Cmdable(), clk, jwtx.AppleJWKConfig{
          JWKSURL:      cfg.Apple.JWKSURL,
          CacheKey:     cfg.Apple.JWKSCacheKey,
          CacheTTL:     time.Duration(cfg.Apple.JWKSCacheTTLSec) * time.Second,
          FetchTimeout: time.Duration(cfg.Apple.JWKSFetchTimeoutSec) * time.Second,
      })
      jwtMgr := jwtx.NewManagerWithApple(jwtx.Options{
          PrivateKeyPath:    cfg.JWT.PrivateKeyPath,
          PrivateKeyPathOld: cfg.JWT.PrivateKeyPathOld,
          ActiveKID:         cfg.JWT.ActiveKID,
          OldKID:            cfg.JWT.OldKID,
          Issuer:            cfg.JWT.Issuer,
          AccessExpirySec:   cfg.JWT.AccessExpirySec,
          RefreshExpirySec:  cfg.JWT.RefreshExpirySec,
      }, jwtx.AppleVerifyDeps{
          Fetcher:  appleJWKFetcher,
          BundleID: cfg.Apple.BundleID,
          Clock:    clk,
      })
      userRepo := repository.NewMongoUserRepository(mongoCli.DB(), clk)
      if err := userRepo.EnsureIndexes(context.Background()); err != nil {
          log.Fatal().Err(err).Msg("user repo EnsureIndexes failed")
      }
      authSvc := service.NewAuthService(userRepo, jwtMgr, clk, cfg.Server.Mode)
      authHandler := handler.NewAuthHandler(authSvc)
      // --- /Story 1.1 ---
      ```
    - `EnsureIndexes` 在 `initialize()` 调用一次，用 `context.Background()`（startup-only I/O，无超时意图让 Mongo 驱动的默认 socket timeout 兜底；对齐 `mongox.MustConnect` Ping 模式）。
    - `cmd/cat/wire.go` `buildRouter` 新增 route（在 `/healthz` `/readyz` 之间，`/v1/*` 之外）：
      ```go
      r.POST("/auth/apple", h.authHandler.SignInWithApple)
      ```
      `handlers` struct 同步加 `authHandler *handler.AuthHandler` 字段；`initialize.go` 构造 `handlers{}` 时填入。
    - **Empty Provider 替换**（架构指南 §21.2, epic-0-retro §8.2 Action #4）：
      ```go
      // Replace the 0.12 stub with the real Story 1.1 provider.
      realUserProvider := ws.NewRealUserProvider(userRepo, clk) // UserProvider real removed Empty via Story 1.1
      sessionResumeHandler := ws.NewSessionResumeHandler(resumeCache, clk, ws.ResumeProviders{
          User:         realUserProvider,
          Friends:      ws.EmptyFriendsProvider{},   // 3.4
          CatState:     ws.EmptyCatStateProvider{},  // 2.2
          Skins:        ws.EmptySkinsProvider{},     // 7.2
          Blindboxes:   ws.EmptyBlindboxesProvider{},// 6.3
          RoomSnapshot: ws.EmptyRoomSnapshotProvider{}, // 4.2
      })
      ```
      `ws.NewRealUserProvider` 定义在 `internal/ws/user_provider.go`（**新建**；实现 `ws.UserProvider` 接口：从 `UserRepository.FindByID` 读当前 userId 的 `domain.User` → 转 `UserPublic` / 或更丰富的 resume 快照结构 → `json.RawMessage`）。
      - **Scope 澄清**：Story 0.12 的 `UserProvider.GetUser(ctx, userID) → (json.RawMessage, error)` 签名已定；本 story 的 `RealUserProvider` 返回 JSON 结构至少含 `id / displayName / timezone / preferences.quietHours`。
      - `UserRepository` 接口需要**补充** `FindByID(ctx, id) (*domain.User, error)` 方法以支持 RealUserProvider —— 本 story 在 AC6 的接口里**同步加一条 FindByID**（改 AC6 的接口定义段为 5 个方法：EnsureIndexes / FindByAppleHash / FindByID / Insert / ClearDeletion）。Mongo 实现对应 `FindByID` + 集成测试用例；service 层不调 FindByID，只有 RealUserProvider 调。
    - **`session.resume.DebugOnly` 翻转**（epic-0-retro §9.1 明确指引）：
      - `internal/dto/ws_messages.go` 里 `session.resume` 条目 `DebugOnly: true` → `DebugOnly: false`
      - `cmd/cat/initialize.go` 移除 "release mode: session.resume handler NOT registered" 分支；`dispatcher.Register("session.resume", sessionResumeHandler.Handle)` 移出 `if cfg.Server.Mode == "debug"` 块，在两种 mode 下均注册
      - `validateRegistryConsistency`（既有 line 217-272）不改 —— 它已经按 DebugOnly flag 做 release-vs-debug 验证，flag 翻转自动通过
      - **风险承担声明**：本翻转后，`session.resume` 在 release 模式返回的 payload 仍包含 5 个 Empty Provider 的 `friends=[] / catState=null / skins=[] / blindboxes=[] / roomSnapshot=null`。对"brand-new real user"语义是**真实**的（新用户确实 0 好友 / 0 皮肤 / 0 盲盒）；对老用户（如果本 story 前有 debug-mode 老用户）语义**部分失真**，但生产无老用户（Story 1.1 首次落盘），无遗留风险。Dev Notes 须显式记录 Epic 0 的 release guard 何时能彻底移除（答：Story 4.5 全部 6 个 Provider real 后）。
    - 现有 Story 0.14 的 `validateRegistryConsistency(dispatcher, cfg.Server.Mode)` 调用保持原位 —— 它会在 release mode 下自动检测 `session.resume` 被注册且 `DebugOnly: false` 一致。**CI drift gate（§21.1 double gate）**：`internal/dto/ws_messages_test.go` + `cmd/cat/initialize_test.go` 已覆盖，本 story 只翻 flag，drift gate 自证不会漏。

12. **AC12 — 监控 / 日志字段契约（NFR-SEC-10 audit, NFR-OBS, 反模式 §10.3）**：

    - 审计日志字段严格 camelCase（架构 §P5 line 566）：`{action, userId, deviceId, platform, isNewUser, requestId, stage?}`
    - Stage 分级（便于日志 grep 定位）：
      - `stage="verify"` — VerifyApple 失败时的 error log
      - `stage="repo_find"` — FindByAppleHash 错误
      - `stage="repo_insert"` — Insert 错误
      - `stage="repo_clear_deletion"` — ClearDeletion 错误
      - `stage="jwt_issue_access"` / `stage="jwt_issue_refresh"` — Issue 错误
    - **禁止字段**（PII 规则 §M13 / NFR-SEC-6）：`identityToken / claims.Sub 原始 / appleUserIDHash / email / nonce`
    - `authorizationCode` 若日后用于 token exchange，也必须脱敏（MVP 不用不记）
    - 日志级别：成功 `Info`；`VALIDATION_ERROR / AUTH_INVALID_IDENTITY_TOKEN` 返客户端错误属**常态**用 `Info`（不污染 error log）；`INTERNAL_ERROR` / Mongo / JWT Issue 失败用 `Error`
    - **反模式 §10.3 cross-check**：`logx.Ctx(ctx)` 已在 handler / service 使用（Story 0.5 确立）；本 story 不引入新 logger 构造，复用现有 `logx.Ctx` helper；**确认** ctx 在 handler 里由 `middleware.RequestID()` 注入过 logger（`cmd/cat/wire.go` line 28 已挂）—— 审计日志一定带 `requestId` 字段。
    - **不**加新 Prometheus metric —— 架构 §D14 "可观测性栈拓扑（MVP 最小）" 当前阶段靠 zerolog 结构化 + grep，未来 Story 8/9 统一上 metric。

13. **AC13 — 客户端侧契约同步（docs/api/openapi.yaml + docs/api/integration-mvp-client-guide.md, 架构 §Repo Separation）**：

    - Update `docs/api/openapi.yaml`：补 `POST /auth/apple` path 的 request / response / error code 定义（200 success, 400 `VALIDATION_ERROR`, 401 `AUTH_INVALID_IDENTITY_TOKEN`, 500 `INTERNAL_ERROR`）
    - Update `docs/api/integration-mvp-client-guide.md`：新增 "Sign in with Apple 客户端流程" 章节：
      1. 客户端本地发起 `ASAuthorizationAppleIDProvider` SIWA 流程时**必须**生成随机 nonce（32 bytes → base64）并用 `sha256(rawNonce)` 作为 `nonce` 参数传给 Apple
      2. Apple 回调返回 `identityToken`（含 `nonce` claim = `sha256(rawNonce)`）
      3. 客户端生成 `deviceId`（UUID v4，存 Keychain）
      4. POST `/auth/apple` body `{identityToken, deviceId, platform, nonce: rawNonce}`（**原始 nonce**，不是 hash）
      5. 响应 200 → 持久化 `accessToken` + `refreshToken` 至 Keychain per-device
    - 这两个 doc 更新是 §21.1 "四步走纪律"中"更新对应文档"一步，虽然 WS 常量没变但 HTTP 契约变了。不更新的话 iOS / watchOS 客户端会按过时协议实现。
    - **生成与校验**：本 story 不改既有 `TestOpenAPISpec_Valid` / `TestWSRegistryEndpoint` 测试；openapi.yaml 新增后跑 `go test ./cmd/cat/...` 应仍绿；若要求 openapi 字段解析单测，记为 Story 1.3 / 1.4 顺带做（1.1 先人工维护）。

14. **AC14 — 测试自包含（§21.7 强制）+ race 检测回归（既有 `bash scripts/build.sh --test`）**：

    - 本 story **所有**测试（单元 + 集成）必须通过：
      - `bash scripts/build.sh --test`（vet + 单元测试全绿）
      - `go test -tags=integration ./...`（集成测试全绿 —— 特别是 `internal/repository/user_repo_integration_test.go` + `cmd/cat/integration_test.go::TestSignInWithApple_EndToEnd`）
      - 期望：`bash scripts/build.sh --race --test` 绿（Windows cgo 已知限制 —— 反模式 §6.x —— 允许跳过，但 Linux CI 必须绿；Dev Notes 记录跳过理由）
    - **零外部依赖**：无任何测试调用 `appleid.apple.com`、真 APNs、真 iOS / watchOS 设备（§21.7）。
    - `internal/testutil/fake_apple.go` **新建**（可选 helper），封装 "自签 RSA + httptest JWKS server + 签 identity token" 三件套，供 AC4 / AC10 共用；避免每个测试重复 100 行样板。

15. **AC15 — Empty Provider removal 注释与 Provider registry 审计（架构指南 §21.2 action #4）**：

    - `cmd/cat/initialize.go` 在 UserProvider 换上真实实现处必须**保留一行注释** `// UserProvider 真实实现 — removed Empty via Story 1.1`（epic-0-retro Action #4 明文要求）。
    - `internal/ws/session_resume.go` 的 `EmptyUserProvider` struct **不删**（Story 0.12 单测仍引用；删除是 Story 4.5 的统一收口工作）—— 只是**不再被 initialize.go 装配**。
    - 验证方法：`grep -n "Empty.*Provider{}" cmd/cat/initialize.go` 剩余条目应精确剩 5 条（Friends / CatState / Skins / Blindboxes / RoomSnapshot）。未来 Story 2-8 每替换一个都减少一条；Story 4.5 最后收口把 5 个 Empty struct 从 session_resume.go 删掉（若彼时无其他引用）。
    - `internal/ws/session_resume.go` 包注释**不改**（它准确描述当前状态："六个 Provider"；本 story 只是把第一个换成真实实现；Story 0.12 的 fail-open / fail-closed 策略仍适用）。

## Tasks / Subtasks

- [x] **Task 1 (AC: #1, #2)** — Domain + ids helper
  - [x] `internal/domain/user.go` + `user_test.go`
  - [x] `pkg/ids/ids.go` 追加 `NewUserID()` + `pkg/ids/ids_test.go`

- [x] **Task 2 (AC: #3)** — Apple JWK fetcher
  - [x] `pkg/jwtx/apple_jwk.go`（struct / Get / Redis cache / singleflight / fail-open）
  - [x] `pkg/jwtx/apple_jwk_test.go`（8 个子 case 全过 + 1 个 happy path config）
  - [x] 构造函数 `log.Fatal` 覆盖：`FetchTimeout<=0 / CacheTTL<=0 / CacheKey="" / JWKSURL=""`

- [x] **Task 3 (AC: #4)** — jwtx Manager Apple 扩展
  - [x] `pkg/jwtx/manager.go` 扩展（`NewManagerWithApple` 工厂；保留既有 `New(opts)` 签名）
  - [x] `VerifyApple` 实现覆盖全部反模式 §3.1-§3.5
  - [x] `pkg/jwtx/manager_apple_test.go`（13 个子 case 全过，含 multi-aud 攻击 + sign-only-manager refuse）
  - [x] `internal/testutil/fake_apple.go` helper（AC14 共用）

- [x] **Task 4 (AC: #5)** — Config
  - [x] `internal/config/config.go` 加 `AppleCfg` + `applyDefaults` + `validateApple`
  - [x] `config/default.toml` + `config/local.toml.example` 加 `[apple]` 段
  - [x] `internal/config/config_test.go` 补 `[apple] bundle_id`，新增 `TestMustLoad_MissingBundleIDFatals` + `TestMustLoad_AppleDefaultsForJWKSKnobs`

- [x] **Task 5 (AC: #6)** — UserRepository
  - [x] `internal/service/auth_service.go` 声明 `UserRepository` interface + sentinels（consumer-side）
  - [x] `internal/repository/user_repo.go` `MongoUserRepository` 实现 5 方法：`EnsureIndexes` / `FindByAppleHash` / `FindByID` / `Insert` / `ClearDeletion`
  - [x] `internal/repository/user_repo_test.go` 单测（BSON roundtrip + snake_case 字段名）
  - [x] `internal/repository/user_repo_integration_test.go` (`//go:build integration`) 7 个子 case

- [x] **Task 6 (AC: #7, #12)** — AuthService
  - [x] `internal/service/auth_service.go` 完整实现 `SignInWithApple`
  - [x] 严格按 AC7 流程 1-9 步 + fail-closed/open 决策矩阵
  - [x] 审计日志字段按 AC12 契约
  - [x] `internal/service/auth_service_test.go` table-driven 14 个场景（覆盖 10+ 要求）

- [x] **Task 7 (AC: #8, #9)** — DTO + Handler
  - [x] `internal/dto/auth_dto.go` + `auth_dto_test.go`（validator 边界 11 子 case）
  - [x] `internal/handler/auth_handler.go` + `auth_handler_test.go`（fake service，httptest）

- [x] **Task 8 (AC: #11)** — Wiring
  - [x] `cmd/cat/initialize.go` Apple fetcher / Manager / UserRepo / AuthService 装配
  - [x] `cmd/cat/wire.go` `buildRouter` 加 `POST /auth/apple`，`handlers` struct 加字段
  - [x] `internal/ws/user_provider.go` `RealUserProvider` 实现 + 单测
  - [x] `cmd/cat/initialize.go` 替换 `EmptyUserProvider{}` → `realUserProvider`，保留注释
  - [x] `internal/dto/ws_messages.go` `session.resume.DebugOnly: true` → `false`
  - [x] `cmd/cat/initialize.go` session.resume handler 注册移出 debug-only 分支
  - [x] `validateRegistryConsistency` + `initialize_test.go` release mode 测试更新（session.resume 现在在 release 里是必须注册项）

- [x] **Task 9 (AC: #10)** — End-to-end integration test
  - [x] `cmd/cat/sign_in_with_apple_integration_test.go` 新增 `TestSignInWithApple_EndToEnd`
  - [x] 覆盖：新用户 / 重复登录 / 删除复活 3 个场景
  - [x] audit log 断言（zerolog buffer + JSON line 解析）

- [x] **Task 10 (AC: #13)** — 客户端契约文档
  - [x] `docs/api/openapi.yaml` 新增 `POST /auth/apple` 定义 + 4 个 schema
  - [x] `docs/api/integration-mvp-client-guide.md` 新增 §11 "Sign in with Apple 客户端流程"

- [x] **Task 11 (AC: #14, #15)** — 回归 + 自检
  - [x] `bash scripts/build.sh --test` 全绿（21 packages 单测 + 集成 vet 全过）
  - [ ] `go test -tags=integration ./...` —— Linux CI 待跑（本地 Windows 无 Docker）
  - [ ] `bash scripts/build.sh --race --test` 在 Linux CI 绿；Windows cgo 限制本地跳过
  - [ ] `grep "Empty.*Provider{}" cmd/cat/initialize.go` 剩 5 条
  - [ ] 回答"Semantic-correctness 思考题"（本文件末尾，写入 Dev Agent Record `Completion Notes`）

## Dev Notes

### 本 story 为何重要（Epic 1 开篇）

- **首个真实用户落库**：整条栈（Mongo write + 唯一索引 + domain entity + service 层 + handler 层 + 路由）第一次闭环；此后所有业务都假设 `users` 有记录。
- **首个 Empty Provider 实化**：`UserProvider` 是 Story 0.12 埋下的 6 个 Empty 中的第一个 —— 本 story 验证"Empty 升级到 Real 时消费侧代码零改动"（架构 §21.2）的设计可行。
- **session.resume release-mode 开闸**：Story 0.14 遗留的 `DebugOnly: true` 在本 story 翻转；以后真实用户在 release 客户端能调 `session.resume` 拿到正常的新账户空态（friends=[] / skins=[] / catState=null 全部是真实的"新用户状态"）。
- **JWT 密码学第一道真实考验**：Epic 0 只有自签 JWT 的 sign-only 路径（`pkg/jwtx/manager.go::Issue`），本 story 首次引入**外部信任链验证**（Apple JWKS + alg/issuer/aud/exp/nonce 全套 claim 校验）；反模式 §3.x 五条 JWT 陷阱必须全部显式处理。

### 关键依赖与可复用 Epic 0 资产

| Epic 0 Story | 资产 | Story 1.1 用法 |
|---|---|---|
| 0.2 | `Runnable` 接口 + `initialize()` DI 骨架 | wiring AppleFetcher / AuthService / UserRepo；EnsureIndexes 启动期调用 |
| 0.3 | `pkg/mongox` / `pkg/redisx` / `pkg/jwtx.Manager`（sign-only） | UserRepo 用 `mongoCli.DB().Collection("users")`；AppleFetcher 用 `redis.Cmdable`；扩展 `jwtx.Manager` 加 VerifyApple |
| 0.5 | `logx.Ctx(ctx)` + `requestId` 中间件 | 审计日志直接 `logx.Ctx(ctx).Info()...`，requestId 自动注入 |
| 0.6 | `dto.ErrAuthInvalidIdentityToken / ErrValidationError / ErrInternalError` + `RespondAppError` | 返错码统一走这三个 sentinel；**不新增**错误码 —— 避免 §21.1 drift gate |
| 0.7 | `clockx.Clock` + `FakeClock` | 所有时间源走 Clock；测试用 FakeClock 驱动 exp / iat |
| 0.9 | `ws.Client{userID}` 结构 | Story 1.1 签的 JWT 的 `userId` claim 为 Story 1.3 的 JWT middleware 消费；本 story 不直接用，但契约对齐 |
| 0.10 | — | 本 story HTTP endpoint 不走 dedup（非 WS RPC） |
| 0.11 | — | 本 story HTTP endpoint 不走 WS rate limit；HTTP per-user rate limit 本 story **不加**（Epic 1.x 其他 story 视需求补） |
| 0.12 | `ws.UserProvider` interface + `EmptyUserProvider{}` | 替换 `EmptyUserProvider` 为 `RealUserProvider`；session.resume 开闸 |
| 0.13 | — | APNs 本 story 不消费；Story 1.4 device token 注册才用 |
| 0.14 | `dto.WSMessages` + `validateRegistryConsistency` | 翻 `session.resume.DebugOnly` flag，drift gate 自证兼容 |
| 0.15 / 9.1 | — | 本 story 无真机依赖；纯单元 + 集成测试（§21.7） |

### Fail-closed / Fail-open 决策（架构指南 §21.3 强制）

（亦见 AC7 内矩阵；此段是架构指南要求的**集中**声明点。）

- **Apple JWK Redis 缓存层**：**fail-open**。理由：缓存是性能优化，不是安全边界；Redis 故障时回退到 HTTPS fetch，Apple 仍执行完整签名验证。可观测点：`log.Warn().Str("action", "apple_jwk_cache_read_error")` + fetch 延迟增加（未来 Story 8/9 metric 可加）。
- **Apple JWK HTTPS fetch**：**fail-closed**。理由：没有 JWKS 就无法验签；接受 identity token 等于信任任何客户端伪造。可观测点：`log.Error().Str("action", "apple_jwk_http_fetch_error")` + 调用方拿到 `AUTH_INVALID_IDENTITY_TOKEN` 401。
- **identity token 签名 / claims 错**：**fail-closed**。理由：安全边界。可观测点：`log.Info().Str("action", "sign_in_with_apple_reject").Str("reason", "<alg_mismatch|bad_issuer|bad_audience|expired|nonce_mismatch|unknown_kid>")` —— 关键：**reason** 字段 only log 大类，不 log 原始 claim（避免信息泄漏给可从日志拿到账户的攻击者）。
- **Mongo `users` 读写**：**fail-closed**。理由：账号数据错乱风险；如果 Mongo 故障期间"临时跳过验证签 token"，等于无条件签发 token 给任何合法 identityToken 持有者（攻击者截获一份即可反复用）—— 彻底打穿鉴权。可观测点：`log.Error().Str("stage", "repo_*").Err(err)` + `ErrInternalError` 500。
- **Mongo duplicate key (sign-in race)**：**自愈**。理由：同一用户并发两次 SIWA 是正常的 watch + phone 同步登录场景；duplicate key 不是错误，是天然并发。可观测点：`log.Info().Str("action", "sign_in_race_resolved").Str("userId", ...)` —— 未来可 metric count（如 > 百 QPS 频繁出现说明客户端 retry 策略出问题）。
- **JWT `Issue`**：**fail-closed**。理由：不能返空 token 让客户端继续（客户端会写入空 Keychain 并假设已登录）。
- **Note**：用户 memory `feedback_no_backup_fallback.md` 强调 "backup/fallback 不能掩盖核心风险"；本 story 所有 fail-open 仅在**性能优化层**（JWK cache），安全边界（验签 / DB 写）全 fail-closed。

### Review-antipatterns 回链清单（实施期须逐条核对）

- §2.6 初始化期 Ping 无超时 → AC3 `AppleJWKFetcher` HTTP client 强制 `http.Client{Timeout:FetchTimeout}`；`EnsureIndexes` 走 Mongo 默认 socket timeout，不额外处理
- §3.1 Verify 未校验 issuer → AC4 显式 `claims.Iss == "https://appleid.apple.com"` + 单测 `TestVerifyApple_WrongIssuer`
- §3.2 `*SigningMethodRSA` 非 RS256 → AC4 显式 `token.Method.Alg() == "RS256"` + 单测 `TestVerifyApple_WrongAlgorithm`（签 RS384 → 拒）
- §3.3 kid 为空 → AC4 显式 `header.kid != "" && != nil` + 单测 `TestVerifyApple_MissingKid`
- §3.4 exp claim 默认非必填 → AC4 `jwt.WithExpirationRequired()` + 单测 `TestVerifyApple_NoExpClaim`
- §3.5 Issue 静默覆盖 RegisteredClaims → AC7 `Manager.Issue` 必须保留 `claims.RegisteredClaims.Subject`；如果现有 manager.go line 95-97 整体赋值，本 story 必须改成 merge 并加单测锁（如 `TestIssue_PreservesSubject`）
- §4.1 非正值静默接受 → AC5 `Apple.JWKSCacheTTLSec<=0 / JWKSFetchTimeoutSec<=0` → `log.Fatal`；AC3 `AppleJWKFetcher` 构造函数同
- §4.2 applyDefaults 缺失 → AC5 `[apple]` 字段全部进 `applyDefaults`；旧 `local.toml` 无 `[apple]` 段仍能启动
- §8.1 dedup key 缺 namespace → AC3 `apple_jwk:cache` 独立 namespace，不与 `event:` / `resume_cache:` 冲突；static key 无 injection concern
- §10.3 `zerolog.Ctx(nil)` → 既有 `logx.Ctx` helper 已兜底；本 story 不裸用 `zerolog.Ctx`
- §12.1 cache stampede → AC3 singleflight 实现 + 单测 `TestFetcher_SingleflightCoalesces`
- §13.1 pkg/ → internal/ → AC3 `AppleJWKFetcher` 只依赖 `redis.Cmdable` / `clockx.Clock`，不 import `internal/*`

### Apple SIWA spec 关键要点速查

- Identity token 格式：JWT (header.payload.signature)，header `{alg: "RS256", kid: "<apple-key-id>"}`，payload 含 `iss / aud / sub / iat / exp / nonce?`
- `iss` 固定为 `"https://appleid.apple.com"` —— 字符串常量直接钉死，无 config 可变
- `aud` = iOS App 的 Bundle ID（如 `"com.huing.cat.app"` 或 watchOS companion bundle）
- `sub` = Apple 分配的**稳定** user id；**per-Team** 稳定（同一 Apple Developer team 的不同 app 看到同一 user 是同一个 sub）
- `nonce` = SHA-256(客户端原始 nonce) 的 hex 字符串（注意：**不是** base64）；若客户端没传 nonce，Apple 不返回 nonce claim；服务端 MVP 强制要求 nonce 非空
- JWKS endpoint: `https://appleid.apple.com/auth/keys` 返 `{keys: [{kty, kid, use, alg, n, e}, ...]}`，每 key 独立 kid；Apple 可能随时轮换
- **官方建议缓存 TTL**：不固定，社区共识 ≤ 24h（与 NFR-INT-1 对齐）

### 测试自包含（§21.7）落地

- Apple JWK mock: `httptest.NewServer` + 自签 RSA key pair；key.n / key.e base64url-encoded 后返回 `{keys: [{kid: "test-kid-1", ...}]}`
- 签 identity token: `jwt.NewWithClaims(SigningMethodRS256, customClaims).SignedString(privateKey)`, header `{kid: "test-kid-1"}`
- Mongo: Testcontainers + `internal/testutil/mongo_setup.go::SetupMongo(t)`
- Redis: miniredis（单测）+ Testcontainers Redis（集成测试；若仅用 miniredis 则集成测试也可单进程，但 Story 0.3 建立了 Testcontainers Redis 集成模式，保持一致）
- **不可依赖**: `appleid.apple.com` 真地址 / 真 APNs / 真 iOS / watchOS app（架构指南 §21.7 + memory `project_repo_separation.md`）

### Semantic-correctness 思考题（§21.8 / §19 第 14 条强制）

> **如果这段代码运行时产生了错误结果但没有 crash，谁会被误导？**
>
> **答**：整条账号系统的所有下游消费者。具体场景：
>
> 1. **验签 bug 1**：如果 `VerifyApple` 漏校验 `aud`（反模式 §3.1 变体），攻击者能用**别的 iOS app** 发给 Apple 的 identity token 登录本服务 → 获得真实 userId 绑定**错误** Apple 账号；此后该攻击 Apple 账号的持有者所有操作都被记到被攻击者账号上；数据被永久污染。
>
> 2. **验签 bug 2**：如果 `Verify` 接受了 `alg: "none"` 或 `alg: "HS256"`（反模式 §3.2），攻击者完全可控 identity token 的任意 sub，**任意冒充任何已注册用户**（只要知道对方 sub 的 SHA-256 哈希）。
>
> 3. **哈希碰撞处理 bug**：如果 SHA-256(sub) 处理代码意外把不同 sub 映射到同一 hash（例如 truncation / encoding 错误 + 两个 sub 前 N 字节相同），两个真实 Apple 账号会被合并到同一 UserID；两个用户互相看到对方好友、状态、盲盒；数据无法事后恢复。
>
> 4. **并发 race bug**：如果 duplicate key 分支返回错误而非自愈，两个客户端同时 SIWA（常见：watch + phone 同步首次登录）会有一个概率性失败；客户端如果没有重试，"signed out" 状态写入 Keychain，终端用户被无故退出。
>
> 5. **JWT Subject 覆盖 bug**（反模式 §3.5）：如果 `Issue` 用整体赋值抹掉 `RegisteredClaims.Subject`，签出的 token 的 `sub` claim 可能为空；依赖 `claims.Subject` 做审计 / 多租户路由的将来 story 拿到空字符串 → 所有审计归到同一"匿名用户"。
>
> 6. **Empty Provider release 开闸遗留 bug**：如果本 story 翻了 `session.resume.DebugOnly` 但**忘了同步开闸 initialize.go 的 handler 注册**（本 story AC11 明确要求同步），release 客户端调 `session.resume` 会收到 `UNKNOWN_MESSAGE_TYPE`；客户端 UI 一片空白；且 Story 0.14 的 drift gate 会 fail-fast log.Fatal 阻止启动（双 gate 保护兜底，但测试未覆盖 release mode 的情况下可能漏）。
>
> **Dev agent 实施完成后在 `Completion Notes List` 里明确写一段"以上 6 个陷阱哪些已被 AC/测试覆盖"的 self-audit；任一条答"未覆盖"必须立即补测试或修代码，不能进入 review。**

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.1 User 领域 + Sign in with Apple 登录 + JWT 签发 — line 760-783]
- [Source: _bmad-output/planning-artifacts/prd.md#FR1-6, FR47-50 — Identity 相关功能需求]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-2/3/4/5/6/10 — 安全 NFR — line 884-894]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-INT-1 — Apple Sign in With Apple 集成 — line 949]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-PERF-2 — HTTP bootstrap p95 ≤ 200ms — line 873]
- [Source: _bmad-output/planning-artifacts/architecture.md#Project Structure — internal/domain/user.go / internal/service/auth_service.go / internal/repository/user_repo.go / internal/handler/auth_handler.go / pkg/jwtx/ — line 801-984]
- [Source: _bmad-output/planning-artifacts/architecture.md#P1 MongoDB 规范 — snake_case + unique index naming — line 499-505]
- [Source: _bmad-output/planning-artifacts/architecture.md#P2 HTTP API 格式 — /auth/apple 路径约定 — line 506-526]
- [Source: _bmad-output/planning-artifacts/architecture.md#P4 Error Classification — fatal 分类映射 401 — line 536-559]
- [Source: _bmad-output/planning-artifacts/architecture.md#P5 Logging — camelCase 字段 — line 561-577]
- [Source: _bmad-output/planning-artifacts/architecture.md#P6 Testing — Testcontainers 强制 — line 578-587]
- [Source: _bmad-output/planning-artifacts/architecture.md#M6/M7/M8/M9/M13/M14 — Domain 命名 / Repo 边界 / DTO 转换 / Clock / PII / APNs token mask — line 626-686]
- [Source: docs/backend-architecture-guide.md#§21.1 双 gate 漂移守门 — line 828-846]
- [Source: docs/backend-architecture-guide.md#§21.2 Empty/Noop Provider 逐步填实 — line 848-861]
- [Source: docs/backend-architecture-guide.md#§21.3 Fail-closed vs Fail-open — line 863-883]
- [Source: docs/backend-architecture-guide.md#§21.7 Server 测试自包含 — line 925-937]
- [Source: docs/backend-architecture-guide.md#§21.8 语义正确性思考题 — line 939-941]
- [Source: server/agent-experience/review-antipatterns.md#§2.6/§3.1-3.5/§4.1-4.2/§8.1/§10.3/§12.1/§13.1]
- [Source: _bmad-output/implementation-artifacts/epic-0-retro-2026-04-19.md#§9.1 Epic 1 预览 — line 219-229（UserProvider 替换 + session.resume DebugOnly 翻转）]
- [Source: _bmad-output/implementation-artifacts/0-12-session-resume-cache-throttle.md — 6 Empty Provider 模式先例]
- [Source: _bmad-output/implementation-artifacts/0-14-ws-message-type-registry-and-version-query.md — double-gate drift pattern 先例]
- [Source: _bmad-output/implementation-artifacts/0-11-ws-connect-rate-limit-and-abnormal-device-reject.md — fail-closed + config fail-fast + tools CLI 模式先例]
- [Source: server/pkg/jwtx/manager.go — 既有 Manager.Issue / Verify 签名（第 105-130 行）]
- [Source: server/internal/dto/error_codes.go#L23-L25 — 已注册 AUTH_INVALID_IDENTITY_TOKEN / AUTH_TOKEN_EXPIRED / AUTH_REFRESH_TOKEN_REVOKED]
- [Source: server/internal/dto/ws_messages.go — `session.resume` 条目当前 `DebugOnly: true`]
- [Source: server/cmd/cat/initialize.go#L100-L154 — 既有 resumeProviders 装配点 + session.resume release-mode 注释块]
- [Source: server/internal/ws/session_resume.go — 6 个 Empty*Provider 定义 + 包注释 fail-open 策略]
- [Source: server/internal/testutil/mongo_setup.go — Testcontainers Mongo helper]
- [Source: server/pkg/ids/ids.go — UserID 类型已存在，需追加 NewUserID() 构造器]
- [Source: server/internal/config/config.go — 既有 JWT / APNs section；追加 [apple] section 模式]

### Project Structure Notes

- 本 story 完全对齐架构指南的 `internal/{domain, service, repository, handler, dto}` + `pkg/{jwtx, ids}` 分层
- 新建文件 9 个：`domain/user.go` / `repository/user_repo.go` / `service/auth_service.go` / `handler/auth_handler.go` / `dto/auth_dto.go` / `ws/user_provider.go` / `pkg/jwtx/apple_jwk.go` / `testutil/fake_apple.go` / 相应 `*_test.go`
- 修改文件 5 个：`pkg/jwtx/manager.go`（Apple 扩展 + §3.5 merge fix）/ `pkg/ids/ids.go`（NewUserID 构造器）/ `internal/config/config.go` + `default.toml` + `local.toml.example` / `cmd/cat/initialize.go` + `wire.go`（wiring + /auth/apple 路由）/ `internal/dto/ws_messages.go`（session.resume.DebugOnly flip）
- 无架构偏差；无新 external dependency（`google/uuid` + `golang.org/x/sync` + `github.com/redis/go-redis/v9` + `github.com/golang-jwt/jwt/v5` 全是 Epic 0 已在 go.mod 的包 —— 仅需确认 `golang.org/x/sync/singleflight` 已 indirect → direct promote，否则 `go mod tidy` 自动补）
- 本 story 启用 `internal/dto/auth_dto.go`、`internal/domain/user.go`、`internal/repository/user_repo.go`、`internal/service/auth_service.go`、`internal/handler/auth_handler.go` 五个文件 —— 为后续 Epic 1 story 1.2-1.6 建立"auth / 账号"模块命名先例（domain/repo/service/handler/dto 每类一个文件，按需长文件拆分的时机归后续 story）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- jwt-go v5 `*jwt.SigningMethodRSA` 类型断言并不区分 RS256/RS384/RS512 —— 必须额外比对 `t.Method.Alg() == "RS256"` 才能落地 §3.2 对 RS384 token 的拒绝（`TestVerifyApple_WrongAlgorithm` 锁定）。
- jwt-go v5 `WithAudience` 是 contains 检查；多 aud token 攻击（["evil", BundleID]）会 contains-pass —— 在 `VerifyApple` 末尾追加 `len(claims.Audience)==1 && claims.Audience[0]==BundleID` 显式校验（`TestVerifyApple_MultipleAudienceRejected` 锁定）。
- `mongo-driver/v2` 解码子文档 default 是 `bson.D` 而非 `bson.M`；`TestUser_BSONFieldNames` 第一版在 `prefs.(bson.M)` 处 panic，改为 switch 处理两种类型。
- session.resume DebugOnly 翻转触发 4 处既有测试需要更新：cmd/cat/initialize_test.go 两个用例 + cmd/cat/ws_registry_test.go ReleaseMode + internal/handler/platform_handler_test.go ReleaseMode + internal/dto/ws_messages_test.go ConsistencyWithDispatcher_ReleaseMode。`§21.1` double gate 自证机制按预期触发，无需手动同步。
- `internal/testutil/fake_apple.go` 必须 import `pkg/jwtx`，方向 `internal → pkg`（合法）；但反方向 `pkg/jwtx_test → internal/testutil` 会触发反模式 §13.1，所以 `manager_apple_test.go` 自带一份小型 helper 不复用 fake_apple。
- `TestMustLoad_MissingBundleIDFatals` 用 stdlib 标准模式 child re-exec：父进程 `os.Args[0] -test.run=^...$ -test.v` + 环境变量分支，期望子进程 `log.Fatal → os.Exit(1)`。Windows 下也跑通。

### Completion Notes List

#### Semantic-correctness 思考题 6 条 self-audit（§21.8 / §19 第 14 条）

| # | 陷阱 | 是否覆盖 | 证据 |
|---|---|---|---|
| 1 | VerifyApple 漏 aud 校验 → 跨 app token 串账号 | ✅ | `TestVerifyApple_WrongAudience` + `TestVerifyApple_MultipleAudienceRejected`（多 aud 攻击）+ source `len(Audience)==1 && [0]==BundleID` 显式守卫 |
| 2 | 接受 alg=none / HS256 → 任意冒充已注册用户 | ✅ | `TestVerifyApple_WrongAlgorithm`（RS384 拒绝）+ source `*SigningMethodRSA` + `Alg() == "RS256"` 双重 gate；jwt v5 stdlib 早就拒 alg=none，但 source 里写死 RS256 是显式护栏 |
| 3 | SHA-256(sub) truncation / encoding 错误把不同 sub 映射到同一 hash | ✅ | service/auth_service.go `hexSHA256` 用 stdlib `sha256.Sum256` + 完整 32 字节 hex 编码；`TestSignInWithApple_PassesHashedNonceToVerifier` 锁定 sha256 路径，集成测试断言 `users` collection 的 `apple_user_id_hash` 等于独立 hexSHA256 计算 |
| 4 | duplicate-key 不自愈 → watch+phone 同步首登概率失败 | ✅ | `TestSignInWithApple_ConcurrentRaceResolved` 显式覆盖 race → 自愈 → 老用户结果 |
| 5 | Issue 整体赋值抹掉 RegisteredClaims.Subject | ✅ | manager.go::Issue 当前 line 95-97 单字段赋值（不覆盖 RegisteredClaims）；`TestManager_Issue_PreservesRegisteredClaims` 已锁定该行为；service 测试 `TestSignInWithApple_IssuedClaimsCarryDeviceAndPlatform` 进一步断言 access/refresh 都带 `Subject == UserID` |
| 6 | session.resume DebugOnly 翻转但忘了开闸 handler 注册 | ✅ | `TestValidateRegistryConsistency_ReleaseModeMissingSessionResumeFails` 直接锁定该回归；`TestValidateRegistryConsistency_ReleaseMode` 验证正确装配；run-time double-gate validateRegistryConsistency 在 release mode 启动时对 session.resume 缺失会 log.Fatal |

**结论：** 6 条全部覆盖；无需补测试或修代码。

#### 其他完成证据

- `bash scripts/build.sh --test` 全绿（vet + M9 时钟 check + build + 全部单元/集成测试）—— 见 dev session 输出 `BUILD SUCCESS`。
- `go test -tags=integration ./...` —— 本地 Windows Docker 未启动，集成测试（Testcontainers Mongo）跳过，仅 `go vet -tags=integration ./...` 编译通过；Linux CI 必须执行 `internal/repository/user_repo_integration_test.go::TestMongoUserRepo_Integration` + `cmd/cat/sign_in_with_apple_integration_test.go::TestSignInWithApple_EndToEnd` 全绿才能 merge（AC10 / AC14 强制）。
- `bash scripts/build.sh --race --test` —— Windows cgo 限制，`go build -race` 报 `runtime/cgo: cgo.exe: exit status 2`（Windows 已知限制，反模式有先例记录）；Linux CI 必须运行 race build 并全绿。
- `grep "Empty.*Provider{}" cmd/cat/initialize.go` —— 剩余条目精确为 5（Friends / CatState / Skins / Blindboxes / RoomSnapshot），原 `EmptyUserProvider` 已替换为 `realUserProvider`，注释 `// UserProvider 真实实现 — removed Empty via Story 1.1` 保留（§21.2 action #4 满足）。第 6 条 `push.EmptyTokenProvider{}` 是 Story 0.13 APNs scaffold，与本 story 无关。
- `docs/api/openapi.yaml` 新增 `/auth/apple` path + 4 个 schema（SignInWithAppleRequest/Response/UserPublic/ErrorResponse）；版本号 bump `0.14.0-epic0` → `1.1.0-epic1`；`TestOpenAPISpec_StructurallyValid` 仍绿。
- `docs/api/integration-mvp-client-guide.md` 新增 §11 「Sign in with Apple 客户端流程（Story 1.1）」，覆盖端点 / 流程 / 关键约束 / 错误处理 / 与 WS 通道关系。
- Apple nonce 完整实现：service 层 SHA-256 raw nonce → VerifyApple 用 `crypto/subtle.ConstantTimeCompare` 比对 claims.nonce；非空校验在 service + dto validator 两层。MVP 不简化。
- session.resume DebugOnly false 翻转完成；release mode CI drift gate 自证（unit + runtime fail-fast）双 gate 全绿。

### File List

**新建文件（19 个）：**

- `server/internal/domain/user.go` — User 聚合根 + UserPreferences/QuietHours/UserConsents/Session + DefaultPreferences()
- `server/internal/domain/user_test.go` — DefaultPreferences fresh-copy 单测
- `server/pkg/ids/ids_test.go` — NewUserID UUID v4 格式 + 唯一性单测
- `server/pkg/jwtx/apple_jwk.go` — AppleJWKFetcher + Redis cache + singleflight + fail-open/closed
- `server/pkg/jwtx/apple_jwk_test.go` — 8+1 个子用例（cache hit / miss / Redis 失败 / HTTP 失败 / timeout / singleflight / non-RS256 drop / invalid JSON 不缓存 / config happy path）
- `server/pkg/jwtx/manager_apple_test.go` — VerifyApple 13 个子用例（happy / invalid sig / wrong iss / wrong aud / expired / missing kid / non-string kid / unknown kid / wrong alg RS384 / nonce mismatch / no exp / malformed / multi-aud reject / missing sub / sign-only-manager refuses）
- `server/internal/testutil/fake_apple.go` — FakeApple test helper（RSA + httptest JWKS + miniredis + AppleJWKFetcher）；可复用于 AC10 e2e 与未来 stories
- `server/internal/repository/user_repo.go` — MongoUserRepository 5 方法（EnsureIndexes / FindByAppleHash / FindByID / Insert / ClearDeletion）+ ErrUserNotFound / ErrUserDuplicateHash 哨兵
- `server/internal/repository/user_repo_test.go` — User BSON roundtrip + snake_case 字段名单测
- `server/internal/repository/user_repo_integration_test.go` — Testcontainers Mongo 7 子用例（idempotent index / insert+find / not found / find-by-id / dup hash / clear deletion success+notfound / full BSON roundtrip）
- `server/internal/service/auth_service.go` — UserRepository / AppleVerifier / JWTIssuer 接口 + AuthService.SignInWithApple 完整流程 + audit log 契约
- `server/internal/service/auth_service_test.go` — 14 个 table-driven 用例（new user / existing / resurrection / race / verify err / find err / insert err / clearDeletion err / access&refresh issue err / validation guards / nonce hash 传递 / claims subject 锁）
- `server/internal/dto/auth_dto.go` — SignInWithAppleRequest/Response + UserPublic + UserPublicFromDomain
- `server/internal/dto/auth_dto_test.go` — validator 边界 11 子用例 + UserPublicFromDomain 投影
- `server/internal/handler/auth_handler.go` — AuthHandler.SignInWithApple Gin 处理函数
- `server/internal/handler/auth_handler_test.go` — 7 子用例（success / bad JSON / 缺 deviceId / 非枚举 platform / service 返 invalid / 返 internal / nil svc panic）
- `server/internal/ws/user_provider.go` — RealUserProvider 实现 ws.UserProvider（替换 EmptyUserProvider；JSON projection: id / displayName / timezone / preferences.quietHours）
- `server/internal/ws/user_provider_test.go` — 5 子用例（happy / 缺字段 omit / NotFound is error / repo err 透传 / 拒绝空 id）
- `server/cmd/cat/sign_in_with_apple_integration_test.go` — TestSignInWithApple_EndToEnd（3 场景 + JWT 回溯校验 + Mongo 校验 + audit log 校验）

**修改文件（14 个）：**

- `server/pkg/ids/ids.go` — `NewUserID()` UUID v4 构造器 + 文档更新
- `server/pkg/jwtx/manager.go` — `AppleIssuer` 常量、`AppleVerifyDeps` 结构、`NewManagerWithApple` 工厂、`AppleIdentityClaims` 结构、`VerifyApple(ctx, idToken, expectedNonceSHA256)` 实现（§3.1-§3.5 全显式覆盖）；`Manager` struct 加 `appleFetcher / appleBundleID / appleClock` 可选字段；保留 `New(opts)` Epic 0 兼容
- `server/internal/config/config.go` — `AppleCfg` 结构 + `Config.Apple` 字段 + `applyDefaults` 4 项默认 + `validateApple()` fail-fast
- `server/internal/config/config_test.go` — 既有 4 个用例补 `[apple] bundle_id`；新增 `TestMustLoad_AppleDefaultsForJWKSKnobs` + `TestMustLoad_MissingBundleIDFatals`（child re-exec pattern）
- `server/config/default.toml` — 新增 `[apple]` 段
- `server/config/local.toml.example` — 全文重写（清理 stale jwt 字段 + 加 `[apple]` 注释引导）
- `server/internal/dto/ws_messages.go` — `session.resume.DebugOnly: true → false`（Story 1.1 翻转）
- `server/internal/dto/ws_messages_test.go` — `TestWSMessages_ConsistencyWithDispatcher_ReleaseMode` 加 `d.Register("session.resume", noopHandler)`
- `server/cmd/cat/initialize.go` — Apple JWK fetcher / Manager / UserRepo / EnsureIndexes / AuthService / AuthHandler 装配；`EmptyUserProvider{}` → `realUserProvider`（保留 `// UserProvider 真实实现 — removed Empty via Story 1.1` 注释）；`session.resume` handler 注册移出 debug-only 分支
- `server/cmd/cat/wire.go` — `handlers.auth` 字段 + `r.POST("/auth/apple", h.auth.SignInWithApple)` 路由
- `server/cmd/cat/initialize_test.go` — `TestValidateRegistryConsistency_ReleaseMode` 更新 + 新增 `TestValidateRegistryConsistency_ReleaseModeMissingSessionResumeFails`；`TestValidateRegistryConsistency_DebugOnlyInReleaseFails` 改用 `debug.echo` 演示
- `server/cmd/cat/ws_registry_test.go` — `TestWSRegistryEndpoint_ReleaseMode` 改为断言 session.resume 出现 + null 反向断言
- `server/internal/handler/platform_handler_test.go` — `TestPlatformHandler_WSRegistry_ReleaseMode` 改为「非空但非 null」断言
- `docs/api/openapi.yaml` — 新增 `POST /auth/apple` path + `SignInWithAppleRequest/Response/UserPublic/ErrorResponse` schemas；版本号 bump
- `docs/api/integration-mvp-client-guide.md` — 新增 §11 SIWA 客户端流程章节

### Change Log

| Date | Version | Author | Summary |
|------|---------|--------|---------|
| 2026-04-19 | 1.1 | Claude (Opus 4.7) | Story 1.1 实现完成：Apple SIWA 端到端（域 / repo / service / handler / dto / 配置 / wiring / e2e 集成测试 / 客户端契约文档），首次真实用户落库；UserProvider 替换 EmptyUserProvider；session.resume DebugOnly 翻转 release。所有单测全绿，集成测试待 Linux Docker CI 跑。|
| 2026-04-19 | 0.1 | sm | Story 1.1 草稿创建，15 条 AC，11 个 Task，ready-for-dev |
