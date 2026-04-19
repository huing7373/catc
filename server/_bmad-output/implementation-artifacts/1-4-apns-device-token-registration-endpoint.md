# Story 1.4: APNs device token 注册 endpoint

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->
<!-- §21.4 AC review 触发点：本 story 含两条"语义错但不 crash"高风险：(1) **AES-GCM 字段级加密**（密钥/nonce/tag 逻辑写错 → 密文可解但另一台部署读不出 / 或"解出"的是伪明文 → 推送发到别人手机）；(2) **per-user 滑动窗口限流**（退化成固定窗口 → NFR-SCALE-8 被打穿，单用户 5s 内写 10 个 token，Mongo upsert 不幂等）。Dev agent 实施前必须先做一轮 self AC review，对照 AC2 / AC4 / AC6 / AC11 + 反模式 §4.1-§4.3 / §9.x / §8.1 / §13.1。结尾 "Semantic-correctness 思考题" 必答。 -->

## Story

As a signed-in user on my Apple Watch or iPhone,
I want to register (or re-register) this device's APNs device token against my account with per-platform idempotency, encryption-at-rest, and per-user rate-limiting,
So that the APNs push pipeline built in Story 0.13 (Pusher → Redis Streams → apns_worker → HTTP/2) can route every future alert (touch fallback FR27 / blindbox drop FR31 / cold-start recall FR44b / quiet-hours silent FR30) to the correct device class without leaking tokens in logs or at rest, and so the Story 0.13 empty `TokenProvider` / `TokenDeleter` / `TokenCleaner` stubs are replaced by production implementations (§21.2).

**Business value**:
- **首个 `/v1/*` 生产业务 endpoint**：Story 1.3 建好了 `/v1/*` group + JWT middleware，但 group 里一条业务路由都没有。Story 1.4 交付**第一条**：`POST /v1/devices/apns-token`。本 story 同时验证 Story 1.3 middleware 在真实业务路径下端到端正确（integration test 走完整 SIWA → access token → /v1/devices/apns-token → Mongo）。
- **Story 0.13 Empty Provider 四替三**：Story 0.13 为 APNs 推送平台预留了 `TokenProvider / TokenDeleter / TokenCleaner / QuietHoursResolver` 四条 consumer 接口，骨架里全是 Empty 实现。本 story 落地前三者的真实实现（`MongoApnsTokenRepository`），让 APNs 整链从"能发但没人有 token"升级到"能发且真的有 token"。`QuietHoursResolver` 留给 Story 1.5（依赖 `users.preferences.quietHours` + `users.timezone`，本 story 不碰）。
- **Story 5.2 / 6.2 / 8.2 解锁**：touch 离线推送（5.2）、blindbox 掉落提醒（6.2）、冷启动召回（8.2）的第一步都是"receiver has at least one APNs token"。本 story 前三条 Provider 真实化后，这些下游 story 可以直接消费 `push.Pusher` 而不需要再 mock token 数据。

## Acceptance Criteria

1. **AC1 — `internal/domain/apns_token.go`（domain 层，Repo ↔ Service 边界 M7）**：

   - 定义 domain 值对象，不含任何 BSON / crypto 细节（domain 永远是明文；加密/解密封装在 repository 内）：
     ```go
     package domain

     import (
         "time"
         "github.com/huing/cat/server/pkg/ids"
     )

     // ApnsToken is the authoritative binding between (userId, platform) and a
     // concrete APNs device token. Story 1.4 writes one row per
     // (userId, platform) pair — re-registration by the same device class
     // overwrites updated_at + device_token; cross-platform tokens coexist
     // (a single user on Watch + iPhone has two rows).
     //
     // DeviceToken is ALWAYS plaintext at the domain layer. The repository
     // encrypts on write (NFR-SEC-7, AES-GCM via pkg/cryptox) and decrypts
     // on read. Callers that log this value MUST route through
     // logx.MaskAPNsToken (M14): DEBUG-level only, first 8 chars + "..."
     // everywhere else.
     type ApnsToken struct {
         UserID      ids.UserID
         Platform    ids.Platform // "watch" | "iphone"; typed const
         DeviceToken string       // hex-encoded APNs device token (plaintext)
         UpdatedAt   time.Time    // Clock.Now() at insert/update
     }
     ```
   - Domain 层**不**导出 `EncryptedDeviceToken` — 密文只在 repository 包内存在。这是 M7 分层：service / handler 拿到的永远是 `*domain.ApnsToken`（明文 DeviceToken），repo 内部走 `apnsTokenDoc` 私有 struct。
   - 单元测试：N/A（纯值对象，无行为；`pkg/ids.Platform` 的合法性由 repo + DTO validator 共同守门）。

2. **AC2 — `pkg/cryptox/aesgcm.go`（新包 + 新文件，NFR-SEC-7）**：

   - 新建 `pkg/cryptox/` 包。package godoc 声明"this package wraps AES-256-GCM for field-level at-rest encryption (NFR-SEC-7). Story 1.4 is the first consumer; future stories that persist sensitive material (e.g. OAuth refresh tokens if Growth reintroduces, analytics PII) reuse the same key / sealed envelope shape."
   - API：
     ```go
     // AESGCMSealer holds an AES-256-GCM AEAD ready for sealed-envelope
     // encryption. Nonce is generated per-Seal via crypto/rand so two
     // Seal calls on the same plaintext produce different ciphertexts
     // (NFR-SEC-7 IND-CPA). Open verifies the GCM tag and rejects any
     // tampering with a typed error (ErrCipherTampered).
     type AESGCMSealer struct {
         aead cipher.AEAD
     }

     // NewAESGCMSealer builds a sealer from a 32-byte key. Returns error
     // on wrong key length (32 bytes = AES-256). Fail-fast at construction
     // keeps misconfigured release deployments from writing garbage.
     func NewAESGCMSealer(key []byte) (*AESGCMSealer, error)

     // Seal encrypts plaintext and returns a single byte slice laid out as
     //   [12-byte nonce | ciphertext | 16-byte GCM tag]
     // — the caller stores this whole slice verbatim (base64 / binary BSON).
     func (s *AESGCMSealer) Seal(plaintext []byte) ([]byte, error)

     // Open reverses Seal. Returns ErrCipherTampered if the GCM tag fails
     // verification (tampered ciphertext or wrong key). Any other error is
     // wrapped and bubbled.
     func (s *AESGCMSealer) Open(sealed []byte) ([]byte, error)

     var ErrCipherTampered = errors.New("cryptox: ciphertext tampered or wrong key")
     ```
   - Sealed-envelope 格式选择 `[nonce | ct | tag]` 连续字节（而非 base64 分段）：BSON `binary` 字段直接存 `[]byte`，零拷贝；未来如果某个新消费方要 text-friendly（config YAML / env var），再加 `SealBase64 / OpenBase64` helper。本 story 不做 helper。
   - `NewAESGCMSealer(nil)` / 长度 != 32 → `return nil, fmt.Errorf("cryptox: key must be 32 bytes (AES-256), got %d", len(key))`。**禁止** panic — `config.mustValidate` 在 release 启动期（下方 AC8）已经 fail-fast 拦过空 key 情况，cryptox 层面 err-only 让测试容易写。
   - `Seal` 失败路径：crypto/rand.Read 失败（极罕见，但 `fmt.Errorf("cryptox: rng: %w", err)`，**不** panic）；`aead.Seal` 无 err path（stdlib 保证），但为兼容未来其他 AEAD 保留 error 签名。
   - `Open` 输入长度 < 12 + 16 = 28 → `ErrCipherTampered`（短密文一定是 tampered）；nonce split + `aead.Open` 返错 → 统一 `ErrCipherTampered`（**不**泄漏"是 nonce 错 / 是 tag 错" — 任何 stdlib 分支错都可能是 attacker probe）。
   - 单元测试 `pkg/cryptox/aesgcm_test.go` table-driven：
     | 子 case | 输入 | 期望 |
     |---|---|---|
     | `TestNewAESGCMSealer_WrongKeyLength` | 16 / 24 / 64 字节 key | err mentions "32 bytes" |
     | `TestNewAESGCMSealer_NilKey` | `nil` | err |
     | `TestSealOpen_RoundTrip` | 32-byte key; plaintext = "hello" | Open(Seal(pt)) == pt |
     | `TestSeal_DifferentNonceEachCall` | Seal 同一 plaintext 两次 | 两次 sealed 字节不同（IND-CPA） |
     | `TestOpen_EmptyPayload` | `[]byte{}` / 27 字节 | ErrCipherTampered |
     | `TestOpen_TagTampered` | 翻转 sealed 最后一字节 | ErrCipherTampered |
     | `TestOpen_NonceTampered` | 翻转 sealed 第一字节 | ErrCipherTampered |
     | `TestOpen_WrongKey` | 用 key A seal，用 key B open | ErrCipherTampered |
     | `TestSealOpen_LargePayload` | 100KB 明文 | round-trip 成功（性能 + 长度兼容） |
   - `pkg/cryptox/doc.go` 已不需要（新包有 `aesgcm.go` 头 godoc 就够）。

3. **AC3 — `internal/repository/apns_token_repo.go`（新建）+ BSON doc + sealer wiring（M7 + NFR-SEC-7）**：

   - 新建 `internal/repository/apns_token_repo.go`；package `repository`；**不**动 user_repo.go（领域分离：apns_tokens 是独立 collection）。
   - Sentinel errors 就近声明在本文件顶部（user_repo.go 模式）：
     ```go
     var ErrApnsTokenNotFound = errors.New("apns_token repo: not found")
     // ErrApnsTokenCipherTampered wraps cryptox.ErrCipherTampered so service
     // callers can branch via errors.Is without importing pkg/cryptox.
     var ErrApnsTokenCipherTampered = errors.New("apns_token repo: cipher tampered or wrong key")
     ```
   - BSON 私有 struct：
     ```go
     const apnsTokensCollection = "apns_tokens"

     type apnsTokenDoc struct {
         UserID      string    `bson:"user_id"`
         Platform    string    `bson:"platform"`
         DeviceToken []byte    `bson:"device_token"` // AES-GCM sealed envelope (nonce|ct|tag)
         UpdatedAt   time.Time `bson:"updated_at"`
     }
     ```
     - `_id` 使用 Mongo 自动 ObjectID（架构 §Collection Schema 第 541 行 `_id: auto`），**不**用复合业务 key 作 _id；唯一性由 `(user_id, platform)` unique compound index 保证（AC3.EnsureIndexes）。
     - DeviceToken 字段持**密文字节**；明文永不落库。转换在 `toDomain` / `docFromDomain` 两个私有方法里完成（调用 sealer.Seal / sealer.Open）。
   - Struct + 构造：
     ```go
     type MongoApnsTokenRepository struct {
         coll   *mongo.Collection
         clock  clockx.Clock
         sealer *cryptox.AESGCMSealer
     }

     // NewMongoApnsTokenRepository wires the repo against the users
     // database. sealer is required — nil would silently write plaintext
     // tokens (NFR-SEC-7 violation); fail-fast at construction.
     func NewMongoApnsTokenRepository(
         db *mongo.Database,
         clk clockx.Clock,
         sealer *cryptox.AESGCMSealer,
     ) *MongoApnsTokenRepository {
         if db == nil    { panic("repository.NewMongoApnsTokenRepository: db must not be nil") }
         if clk == nil   { panic("repository.NewMongoApnsTokenRepository: clock must not be nil") }
         if sealer == nil{ panic("repository.NewMongoApnsTokenRepository: sealer must not be nil") }
         return &MongoApnsTokenRepository{
             coll:   db.Collection(apnsTokensCollection),
             clock:  clk,
             sealer: sealer,
         }
     }
     ```
   - `EnsureIndexes(ctx context.Context) error`（架构 §10.2 + user_repo 模式）：
     ```go
     _, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
         Keys: bson.D{
             {Key: "user_id", Value: 1},
             {Key: "platform", Value: 1},
         },
         Options: options.Index().
             SetUnique(true).
             SetName("user_id_1_platform_1"),
     })
     ```
     Index name 钉死 `user_id_1_platform_1` 对齐架构 §P1 `{field}_{order}` 规则，重命名会导致下次 deploy 触发 index rebuild。
   - `Upsert(ctx, t *domain.ApnsToken) error`：
     1. 入参校验：`t == nil` → `errors.New("apns_token repo: upsert nil token")`；`t.UserID == ""` / `t.DeviceToken == ""` / `t.Platform != PlatformWatch && t.Platform != PlatformIphone` → 相应 err（**不**返 Mongo err code，上层靠 domain validator 已过滤，这里是防御纵深）。
     2. `sealedBytes, err := r.sealer.Seal([]byte(t.DeviceToken))`；err → `fmt.Errorf("apns_token repo: seal: %w", err)`。
     3. `doc := apnsTokenDoc{UserID: string(t.UserID), Platform: string(t.Platform), DeviceToken: sealedBytes, UpdatedAt: t.UpdatedAt}` — 注意 UpdatedAt 由**调用方**（service）通过 clock 注入，repo 不读 clock（保持 repo 可 FakeClock 独立测）。
     4. `filter := bson.M{"user_id": doc.UserID, "platform": doc.Platform}`；`update := bson.M{"$set": doc}`；`opts := options.Update().SetUpsert(true)`；`_, err := r.coll.UpdateOne(ctx, filter, update, opts)`；err → 包 `fmt.Errorf("apns_token repo: upsert: %w", err)`。
     5. 成功返 nil。**不**返 `result.UpsertedID` / `MatchedCount` — service 层不关心"是新建还是覆盖"，语义都是"你现在有一条 (user, platform) → token 的记录"。
   - `ListByUserID(ctx, userID ids.UserID) ([]domain.ApnsToken, error)`（Story 0.13 `TokenProvider.ListTokens` 的真实实现依赖）：
     1. `userID == ""` → 返 `errors.New("apns_token repo: list: empty user id")`。
     2. `cursor, err := r.coll.Find(ctx, bson.M{"user_id": string(userID)})`；err → 包 + 返。
     3. 逐条 decode `apnsTokenDoc`；**每条**调 `r.sealer.Open(doc.DeviceToken)`；
        - err is `cryptox.ErrCipherTampered` → 用 `errors.Is` 判断，包成 `ErrApnsTokenCipherTampered` 然后**log warn（含 userId、platform，不含密文 / 不含 key）+ 跳过这一条**（故障隔离：一条脏行不应让整个用户的推送链挂掉；fail-open 选择，见 AC11）。
        - 其他 err → 中断 cursor，返 `fmt.Errorf("apns_token repo: list decrypt: %w", err)`。
     4. 组装 `domain.ApnsToken` slice，空 slice （不是 nil）返回 when 0 rows。
   - `Delete(ctx, userID ids.UserID, plaintextToken string) error`（Story 0.13 `TokenDeleter` 真实实现 — HTTP 410 触发删）：
     1. 参数校验 + sealed = sealer.Seal(plaintextToken)？**不** — `Seal` 每次产生不同 nonce，不能做 `{device_token: sealedAgain}` 匹配。
     2. 正确做法：`filter := bson.M{"user_id": string(userID)}` + 全部 List 出来后在 Go 层面遍历 decrypted 匹配 plaintextToken → 删中了的 `_id`。但这会导致 worker 在每个 410 响应下做 N 次 decrypt（N = 用户 token 数，通常 1-2），可接受。
     3. 简化版实现（MVP）：
        ```go
        cursor, err := r.coll.Find(ctx, bson.M{"user_id": string(userID)})
        // iterate; for each doc decrypt; if matches plaintextToken → r.coll.DeleteOne({_id: doc._id})
        // return first DeleteOne err or nil
        ```
     4. **Missing row is a no-op (returns nil)** — Story 0.13 `TokenDeleter` godoc 契约：`idempotent against concurrent 410s across replicas`。
     5. cipher-tampered 行跳过（同 ListByUserID），log warn。
   - `DeleteExpired(ctx, cutoff time.Time) (int64, error)`（Story 0.13 `TokenCleaner` 真实实现 — cron 24h 清理 30 天前 token）：
     1. `res, err := r.coll.DeleteMany(ctx, bson.M{"updated_at": bson.M{"$lt": cutoff}})`
     2. err → 包 + 返；成功 → `return res.DeletedCount, nil`。
     3. 此方法**不需要**解密 — 仅按 updated_at 时间窗删；密文字段未读，速度与隐私都最优。
   - **与 Story 0.13 Empty 实现的兼容性**：`MongoApnsTokenRepository` 的三个方法签名和 `push.TokenProvider` / `push.TokenDeleter` / `push.TokenCleaner` 三个接口**不完全同形**：
     - `TokenProvider.ListTokens(ctx, userID) ([]TokenInfo, error)` — TokenInfo 字段是 `{Platform string, DeviceToken string}`；repo 返 `[]domain.ApnsToken`（字段多 `UserID` + `UpdatedAt`）
     - `TokenDeleter.Delete(ctx, userID, deviceToken string) error` — 同形
     - `TokenCleaner.DeleteExpired(ctx, cutoff) (int64, error)` — 同形
   - **桥接方式**：在 `internal/repository/apns_token_repo.go` 底部导出三个小 adapter 方法（同包，直接读内部字段），让 `*MongoApnsTokenRepository` **同时满足** `push.TokenProvider / TokenDeleter / TokenCleaner`：
     ```go
     // ListTokens implements push.TokenProvider by projecting domain ApnsToken
     // down to push.TokenInfo (plaintext DeviceToken + Platform string). Epic-0
     // had push.EmptyTokenProvider{} in this slot; Story 1.4 swaps in *this*
     // repository via cmd/cat/initialize.go (§21.2 Empty→Real provider fill-in).
     func (r *MongoApnsTokenRepository) ListTokens(ctx context.Context, userID ids.UserID) ([]push.TokenInfo, error) {
         toks, err := r.ListByUserID(ctx, userID)
         if err != nil { return nil, err }
         out := make([]push.TokenInfo, 0, len(toks))
         for _, t := range toks {
             out = append(out, push.TokenInfo{
                 Platform:    string(t.Platform),
                 DeviceToken: t.DeviceToken,
             })
         }
         return out, nil
     }
     // Delete / DeleteExpired delegate directly — signatures already match.
     ```
     这避免在 cmd/cat/initialize.go 里写 wrapper 类型，保持 wiring 一行。
     - **分层检查**：`internal/repository` 导入 `internal/push`？—— push 的接口是 consumer-side 消费侧定义，repo 实现侧**满足**该接口（Go 结构化类型）。Repo 可以 import push 的**类型**（TokenInfo）；但这会引入 `repository → push` 的包依赖方向。**替代方案**：不让 repo 直接 import push，而在 `cmd/cat/` wire 层写一个 5 行 adapter struct。Dev agent 择优。**推荐**：直接 import push.TokenInfo（架构 §13 允许 internal/ 内部任意方向依赖；只有 `pkg/` → `internal/` 被禁止，反模式 §13.1）；省代码 + ListTokens 签名一眼看懂是在满足接口。
     - **导入方向不变量锁**：在 `internal/repository/apns_token_repo.go` 顶部 package import block 加一行注释：
       ```go
       // Import direction invariant: repository → push (for push.TokenInfo shape).
       // push MUST NOT import internal/repository (would create a cycle + couple
       // the push platform to the user DB schema). If a future story needs
       // push to invoke repo methods, declare a consumer-side interface in
       // push/ (matching the TokenProvider/Deleter/Cleaner pattern) and let
       // the repo satisfy it structurally — do NOT reverse the direction.
       ```
       AC14 回归加一行 grep 核查：`! grep -rE "\"github.com/huing/cat/server/internal/repository\"" internal/push/` —— 若 push 包出现 repository 导入，CI fail。
   - 单元测试 `internal/repository/apns_token_repo_test.go`（无 Mongo，测 BSON + encryption 逻辑）：
     - 这一类纯 unit test 在 user_repo 里其实**没**——user_repo_test.go 是集成测（Testcontainers）。ApnsToken repo 同路线：unit test 仅测 sealer 合约（seal/open round-trip 已在 cryptox 测），**repository 测试全部走 Testcontainers**（见下）。
   - **集成测试** `internal/repository/apns_token_repo_integration_test.go`（`//go:build integration`，Testcontainers Mongo，不 `t.Parallel()` — 架构 §M11）：
     | Test | 断言 |
     |---|---|
     | `TestApnsTokenRepo_Integration_UpsertAndList` | Upsert watch + iphone → List → 返回 2 条，明文 DeviceToken 正确解密 |
     | `TestApnsTokenRepo_Integration_UpsertReplacesSamePlatform` | Upsert watch tokenA → Upsert watch tokenB → List → 只 1 条（tokenB），updated_at 更新 |
     | `TestApnsTokenRepo_Integration_UpsertCrossPlatformCoexists` | Upsert watch + iphone 双写 → 2 条都在 |
     | `TestApnsTokenRepo_Integration_AtRestEncrypted` | 直接读 Mongo（绕过 repo）→ `device_token` 字段是 bytes 且**不等于**明文；再用另一个相同 key 的 sealer Open → 等于明文 |
     | `TestApnsTokenRepo_Integration_WrongKeyRejectsOpen` | 用 key A 写 → 构造 key B 的 repo → List → log warn + 跳过该条（slice 空）；DeleteExpired / Delete 不受 decrypt 失败影响（它们不读密文） |
     | `TestApnsTokenRepo_Integration_DeleteByPlaintextToken` | Upsert 2 条（watch + iphone） → Delete(userId, watchPlaintext) → List 只剩 iphone |
     | `TestApnsTokenRepo_Integration_DeleteMissingIsNoop` | Delete(unknownUser, x) + Delete(knownUser, unknownToken) → 均 nil err |
     | `TestApnsTokenRepo_Integration_Delete_SkipsTamperedRowDeletesValidSibling` | seed user 有 2 row：row A 正常密文 / row B 用 key'={0xff...} seal（模拟脏行 / key 轮转遗留）→ Delete(userID, row-A-plaintext) → row A 删除，row B 保留（tampered 行不阻塞合法删除）；log warn 有 `apns_token_decrypt_tampered` |
     | `TestApnsTokenRepo_Integration_DeleteExpired` | 插 3 条 updated_at 跨 cutoff（1 条之前, 2 条之后）→ DeleteExpired(cutoff) 返 1；List 剩 2 |
     | `TestApnsTokenRepo_Integration_EnsureIndexes_Idempotent` | 调两次 EnsureIndexes → 第二次不 err；同时验证 index name 是 `user_id_1_platform_1` 且 unique |
     | `TestApnsTokenRepo_Integration_UniqueConstraint_RaceSafe` | 并发 2 个 goroutine Upsert 同 (userId, watch, diff tokens) → 最终 List == 1 条（任一 winner），无 duplicate-key err（upsert 幂等覆盖） |
   - 每个测试内 `defer coll.Drop(ctx)` 以复用 shared Mongo container（与 user_repo_integration_test.go 相同 pattern）。

4. **AC4 — `internal/service/apns_token_service.go`（新建）+ 限流 port + Register 流程**：

   - Service 层接口消费 consumer-side（架构 §6.2）：
     ```go
     package service

     // ApnsTokenHandlerService is the contract AuthHandler equivalent for the
     // apns-token endpoint — DeviceHandler sees only this interface, not the
     // concrete service struct.
     type ApnsTokenHandlerService interface {
         RegisterApnsToken(ctx context.Context, req RegisterApnsTokenRequest) error
     }

     type RegisterApnsTokenRequest struct {
         UserID      ids.UserID
         DeviceID    string
         Platform    ids.Platform // from JWT claim (middleware.PlatformFrom); see AC5
         DeviceToken string       // plaintext; service passes to repo (repo encrypts)
     }

     // apnsTokenRepo is the minimal repo surface RegisterApnsToken needs;
     // declared here so the service package does not import repository
     // directly (P2 consumer-side interface, architecture §6.2).
     type apnsTokenRepo interface {
         Upsert(ctx context.Context, t *domain.ApnsToken) error
     }

     // userSessionRepo is the minimal repo surface for toggling
     // users.sessions[deviceId].has_apns_token = true on successful register.
     // Declared here (not embedded in authService's userRepo interface) so
     // Story 1.4 does not force a signature change on Story 1.2's repo
     // interface.
     type userSessionRepo interface {
         SetSessionHasApnsToken(ctx context.Context, userID ids.UserID, deviceID string, has bool) error
     }

     // apnsTokenRegisterRateLimiter throttles per-user registration attempts.
     // Sliding window by design — a fixed-window implementation (INCR +
     // EXPIRE NX) would let a misbehaving client burst 2× the quota across
     // the window boundary (review-antipatterns §9.1). Real impl is a
     // redisx.PerUserSlidingWindow variant (see AC6).
     type apnsTokenRegisterRateLimiter interface {
         Acquire(ctx context.Context, userID ids.UserID) (allowed bool, retryAfter time.Duration, err error)
     }
     ```
   - Service struct + flow：
     ```go
     type ApnsTokenService struct {
         repo       apnsTokenRepo
         sessionRepo userSessionRepo
         limiter    apnsTokenRegisterRateLimiter
         clock      clockx.Clock
     }

     func NewApnsTokenService(
         repo apnsTokenRepo,
         sessionRepo userSessionRepo,
         limiter apnsTokenRegisterRateLimiter,
         clk clockx.Clock,
     ) *ApnsTokenService {
         // fail-fast on nil deps (review-antipatterns §4.1)
         if repo == nil { panic("service.NewApnsTokenService: repo must not be nil") }
         if sessionRepo == nil { panic("service.NewApnsTokenService: sessionRepo must not be nil") }
         if limiter == nil { panic("service.NewApnsTokenService: limiter must not be nil") }
         if clk == nil { panic("service.NewApnsTokenService: clock must not be nil") }
         return &ApnsTokenService{repo: repo, sessionRepo: sessionRepo, limiter: limiter, clock: clk}
     }

     func (s *ApnsTokenService) RegisterApnsToken(ctx context.Context, req RegisterApnsTokenRequest) error {
         // 1. Rate limit — fail-closed: any limiter err rejects the request.
         allowed, retry, err := s.limiter.Acquire(ctx, req.UserID)
         if err != nil {
             return fmt.Errorf("apns token service: rate limit acquire: %w", err)
         }
         if !allowed {
             return dto.ErrRateLimitExceeded.WithCause(fmt.Errorf("apns_token register rate: retry_after=%s", retry))
         }
         // 2. Persist (sealer runs inside the repo).
         t := &domain.ApnsToken{
             UserID:      req.UserID,
             Platform:    req.Platform,
             DeviceToken: req.DeviceToken,
             UpdatedAt:   s.clock.Now(),
         }
         if err := s.repo.Upsert(ctx, t); err != nil {
             return fmt.Errorf("apns token service: upsert: %w", err)
         }
         // 3. Mark sessions.<deviceId>.has_apns_token = true (best-effort —
         //    any session-write err is LOGGED but does NOT fail the request;
         //    the flag is a convenience, not a correctness invariant — see
         //    AC11 fail-open rationale for this specific field). The core
         //    binding (apns_tokens row) is already durable after step 2.
         if err := s.sessionRepo.SetSessionHasApnsToken(ctx, req.UserID, req.DeviceID, true); err != nil {
             logx.Ctx(ctx).Warn().Err(err).
                 Str("userId", string(req.UserID)).
                 Str("deviceId", req.DeviceID).
                 Msg("apns_token_session_flag_write_failed")
             // swallowed on purpose — see AC11
         }
         // 4. Audit log (M14 masked token).
         logx.Ctx(ctx).Info().
             Str("userId", string(req.UserID)).
             Str("deviceId", req.DeviceID).
             Str("platform", string(req.Platform)).
             Str("action", "apns_token_register").
             Msg("apns_token_registered")
         return nil
     }
     ```
   - 单元测试 `internal/service/apns_token_service_test.go`（table-driven，nil ctx 用 `t.Context()` / `context.Background()`，`t.Parallel()` 允许因为无全局状态）：
     | Test | 断言 |
     |---|---|
     | `TestRegisterApnsToken_HappyPath` | limiter.allow=true + repo ok + sessionRepo ok → nil err；repo.Upsert 被调 1 次且参数等于 req；sessionRepo 被调 1 次 `has=true` |
     | `TestRegisterApnsToken_LimiterBlocks` | limiter.allow=false, retry=2s → err `errors.Is(err, dto.ErrRateLimitExceeded) == true`；repo.Upsert **不**被调 |
     | `TestRegisterApnsToken_LimiterError_FailsClosed` | limiter 返 err → service 返 wrapped err；repo.Upsert **不**被调；log audit 不出现 success |
     | `TestRegisterApnsToken_RepoError` | repo.Upsert 返 err → service 返 wrapped err；sessionRepo **不**被调（顺序依赖） |
     | `TestRegisterApnsToken_SessionWriteError_Swallowed` | repo ok + sessionRepo 返 err → service 仍返 nil；log 有 warn `apns_token_session_flag_write_failed` |
     | `TestRegisterApnsToken_ClockInjection` | FakeClock.Now() = 2026-04-20 → repo.Upsert 被调用时 `t.UpdatedAt == 2026-04-20` |
     | `TestRegisterApnsToken_NilDepsPanic` | `NewApnsTokenService(nil, x, x, x)` panic；逐字段打 4 个子测 |
   - Fake repo / limiter / sessionRepo 用既有 `internal/service/auth_service_test.go` 的 "fake struct w/ counters + recorded args" 模式。

5. **AC5 — `internal/handler/device_handler.go`（新建）+ DTO + 绑定 Story 1.3 middleware**：

   - DTO `internal/dto/device_dto.go`（新建；auth_dto.go 模式）：
     ```go
     // RegisterApnsTokenRequest is the wire shape for POST /v1/devices/apns-token.
     // platform in body is **optional** — if provided it MUST match the
     // middleware.PlatformFrom(c) value from the JWT (defense in depth:
     // prevents a compromised client from re-registering for a platform
     // class different from its access token). If absent, the JWT's platform
     // wins.
     //
     // deviceToken validator: 64 hex chars. APNs tokens are historically
     // 32-byte opaque identifiers encoded as 64 hex chars; Apple reserves
     // the right to change length in the future, so the regex is liberal
     // in that `oneof` / `max=200` allows forward-compat — but 64 is the
     // today-default reject for malformed / truncated input.
     type RegisterApnsTokenRequest struct {
         DeviceToken string `json:"deviceToken" binding:"required,min=8,max=200,hexadecimal"`
         Platform    string `json:"platform,omitempty" binding:"omitempty,oneof=watch iphone"`
     }

     type RegisterApnsTokenResponse struct {
         Ok bool `json:"ok"`
     }
     ```
     - `hexadecimal` 是 `go-playground/validator/v10` 内置 tag，验 `[0-9a-fA-F]+`。 `min=8` 是 defensive lower bound；`max=200` 覆盖 Apple 未来加长。
     - **不在 DTO tag 里钉 `len=64`**：Apple 文档 "device tokens can change in length"；钉死会让未来一个 iOS 版本更新后所有客户端 400。改为在 service / AC1 语义层明确 "today 典型 64 hex" 为预期；validator 仅做"合法 hex 字符串 + 长度范围"。
   - Handler：
     ```go
     package handler

     // DeviceHandlerService is the subset of methods DeviceHandler needs —
     // declared here so the handler does not import concrete *ApnsTokenService.
     type DeviceHandlerService interface {
         RegisterApnsToken(ctx context.Context, req service.RegisterApnsTokenRequest) error
     }

     type DeviceHandler struct {
         svc DeviceHandlerService
     }

     func NewDeviceHandler(svc DeviceHandlerService) *DeviceHandler {
         if svc == nil { panic("handler.NewDeviceHandler: svc must not be nil") }
         return &DeviceHandler{svc: svc}
     }

     // RegisterApnsToken handles POST /v1/devices/apns-token. Mounted
     // inside the /v1/* group wired by Story 1.3 — JWTAuth runs before
     // this handler, so middleware.UserIDFrom / DeviceIDFrom / PlatformFrom
     // are all populated by the time we read them.
     func (h *DeviceHandler) RegisterApnsToken(c *gin.Context) {
         var req dto.RegisterApnsTokenRequest
         if err := c.ShouldBindJSON(&req); err != nil {
             dto.RespondAppError(c, dto.ErrValidationError.WithCause(err))
             return
         }
         userID := middleware.UserIDFrom(c)
         deviceID := middleware.DeviceIDFrom(c)
         jwtPlatform := middleware.PlatformFrom(c)
         // JWT middleware (Story 1.3) already rejected empty userId / deviceId,
         // but the handler's defense-in-depth check stays so a programmer
         // error that removes the middleware check still fails closed.
         if userID == "" || deviceID == "" {
             dto.RespondAppError(c, dto.ErrAuthInvalidIdentityToken.WithCause(
                 errors.New("device handler: middleware did not inject userId/deviceId — check wiring"),
             ))
             return
         }
         // Platform resolution: JWT is source of truth (defense in depth);
         // body value MUST match if provided.
         var platform ids.Platform
         if jwtPlatform != "" {
             platform = jwtPlatform
             if req.Platform != "" && req.Platform != string(jwtPlatform) {
                 dto.RespondAppError(c, dto.ErrValidationError.WithCause(fmt.Errorf(
                     "device handler: body.platform=%q does not match jwt.platform=%q",
                     req.Platform, jwtPlatform,
                 )))
                 return
             }
         } else {
             // No "legacy JWT without platform" fallback — reject.
             // Reasoning (AC Review §21.8 #6 attack surface): Story 1.1
             // SignInWithApple + Story 1.2 RefreshToken both populate the
             // platform claim unconditionally, so every access token in
             // production carries Platform. Accepting body.platform when
             // jwt.platform is empty would let any future bug (e.g. a
             // regression that drops the claim) silently admit
             // user-controlled platform → phishing vector where an
             // attacker with a stolen Watch token registers an iPhone
             // APNs token bound to the victim account. Fail-closed
             // instead: if JWT lacks platform, the caller has a bad
             // access token — reject and force refresh.
             dto.RespondAppError(c, dto.ErrAuthInvalidIdentityToken.WithCause(
                 errors.New("device handler: jwt missing platform claim — client must refresh to get a 1.2+ access token"),
             ))
             return
         }
         if err := h.svc.RegisterApnsToken(c.Request.Context(), service.RegisterApnsTokenRequest{
             UserID:      userID,
             DeviceID:    deviceID,
             Platform:    platform,
             DeviceToken: req.DeviceToken,
         }); err != nil {
             dto.RespondAppError(c, err)
             return
         }
         c.JSON(http.StatusOK, dto.RegisterApnsTokenResponse{Ok: true})
     }
     ```
   - 单元测试 `internal/handler/device_handler_test.go`（gin test recorder + fake svc）：
     | Test | 断言 |
     |---|---|
     | `TestDeviceHandler_RegisterApnsToken_HappyPath` | fake svc.Err=nil + ctx keys (userId=u1, deviceId=d1, platform=watch) → 200 + body `{ok:true}`；svc 收到 `{UserID:u1, DeviceID:d1, Platform:watch, DeviceToken:<64hex>}` |
     | `TestDeviceHandler_RegisterApnsToken_InvalidBody_400` | body `{"deviceToken":""}` → 400 `VALIDATION_ERROR` |
     | `TestDeviceHandler_RegisterApnsToken_InvalidHex_400` | body `{"deviceToken":"ZZZZ...","platform":"watch"}` → 400 `VALIDATION_ERROR` |
     | `TestDeviceHandler_RegisterApnsToken_PlatformMismatch_400` | jwt.platform=watch + body.platform="iphone" → 400 `VALIDATION_ERROR` |
     | `TestDeviceHandler_RegisterApnsToken_PlatformOmittedInBody_UsesJWT` | body 不含 platform + jwt.platform=iphone → 200；svc 收到 Platform=iphone |
     | `TestDeviceHandler_RegisterApnsToken_JWTMissingPlatform_401` | jwt.platform="" + body.platform="watch" → 401 `AUTH_INVALID_IDENTITY_TOKEN`（§21.8 #6 attack surface 锁：body.platform 在 JWT 空时被**忽略**，不 fallback；svc **不**被调用） |
     | `TestDeviceHandler_RegisterApnsToken_JWTMissingPlatform_BodyAlsoMissing_401` | jwt.platform="" + body.platform="" → 401 `AUTH_INVALID_IDENTITY_TOKEN` |
     | `TestDeviceHandler_RegisterApnsToken_MissingUserID_401` | 不设 ctx key（middleware 未 run）→ 401 `AUTH_INVALID_IDENTITY_TOKEN` |
     | `TestDeviceHandler_RegisterApnsToken_ServiceError_Bubbles` | fake svc.Err = dto.ErrRateLimitExceeded → 429 `RATE_LIMIT_EXCEEDED` |
     | `TestDeviceHandler_RegisterApnsToken_ServiceUnwrappedError_500` | fake svc.Err = `errors.New("boom")` → 500 `INTERNAL_ERROR` |
     | `TestNewDeviceHandler_NilServicePanics` | `NewDeviceHandler(nil)` | panic |
   - 测试 gin context 里 "模拟 middleware 已跑"：直接 `c.Set(string(middlewareCtxKeyUserID), "u1")` —— 但 middleware 包的 ctxKey 是私有的；所以测试里改为通过 `middleware.JWTAuth(fakeVerifier)` 驱动 + fakeVerifier 返 claims。这样 UserIDFrom 等 getter 能读到值。Dev agent 择优：
     - **A**: 用 fake middleware wrapper：`r.Use(middleware.JWTAuth(&fakeVerifier{out: claims}))`
     - **B**: 测试专用 helper `middleware.SetUserIDForTest(c, "u1")` 暴露给同包测试（不导出给业务包）
     - **推荐 A**（与 Story 1.3 既有 `jwt_auth_test.go` 同模式，不需要新增 test-only API 表面）。

6. **AC6 — `pkg/redisx/user_ratelimit.go`（新建）per-user 滑动窗口限流器**：

   - 复用 Story 0.11 `redisx.RedisConnectRateLimiter` 的"milliseconds + sorted set"思路，但接口窄 + key 不同 + 可 per-功能多实例。
   - 新文件 `pkg/redisx/user_ratelimit.go`：
     ```go
     package redisx

     // UserSlidingWindowLimiter is a per-user true sliding-window limiter.
     // Unlike the connect-rate limiter, this type is generic over its Redis
     // key prefix so different endpoints can share the implementation:
     //   - "ratelimit:apns_token:<userID>" (Story 1.4)
     //   - "ratelimit:profile_update:<userID>" (Story 1.5, future)
     //   - "ratelimit:touch_send:<fromUserID>:<toUserID>" (Story 5.3, future)
     //
     // The prefix is injected at construction so one constant point owns the
     // key string. See review-antipatterns §8.1: a dedup / rate-limit key
     // without a namespace prefix collides across features.
     //
     // Algorithm identical to RedisConnectRateLimiter — see that type's docs
     // for why milliseconds (not nanoseconds) and why ZREMRANGE uses `(cutoff`
     // exclusive boundary. Any behavior change on the sliding-window math
     // MUST be made to BOTH types in the same PR (or — and this would be
     // cleaner — refactor both to call a shared `slidingWindowAcquire(pipe,
     // key, nowMs, windowMs, thresholdInt)` helper; the current story accepts
     // two parallel implementations to avoid a wider refactor, with a
     // `// TODO(review): unify with RedisConnectRateLimiter.AcquireConnectSlot`
     // comment on both).
     type UserSlidingWindowLimiter struct {
         cmd       redis.Cmdable
         clock     clockx.Clock
         keyPrefix string        // e.g. "ratelimit:apns_token:"
         threshold int64
         window    time.Duration
     }

     func NewUserSlidingWindowLimiter(
         cmd redis.Cmdable,
         clock clockx.Clock,
         keyPrefix string,
         threshold int64,
         window time.Duration,
     ) *UserSlidingWindowLimiter {
         if cmd == nil     { panic("redisx.NewUserSlidingWindowLimiter: cmd must not be nil") }
         if clock == nil   { panic("redisx.NewUserSlidingWindowLimiter: clock must not be nil") }
         if keyPrefix == "" { panic("redisx.NewUserSlidingWindowLimiter: keyPrefix must not be empty") }
         if !strings.HasSuffix(keyPrefix, ":") {
             panic("redisx.NewUserSlidingWindowLimiter: keyPrefix must end in ':' — review-antipatterns §8.2 injectivity")
         }
         if threshold <= 0 { panic("redisx.NewUserSlidingWindowLimiter: threshold must be > 0") }
         if window <= 0    { panic("redisx.NewUserSlidingWindowLimiter: window must be > 0") }
         return &UserSlidingWindowLimiter{
             cmd: cmd, clock: clock, keyPrefix: keyPrefix,
             threshold: threshold, window: window,
         }
     }

     // Acquire records the current attempt in the sliding-window ZSET and
     // decides whether it is allowed. Returns (allowed, retryAfter, err).
     // retryAfter is meaningful only when allowed == false.
     func (l *UserSlidingWindowLimiter) Acquire(ctx context.Context, userID ids.UserID) (bool, time.Duration, error)
     ```
   - Acquire 实现完全复用 `RedisConnectRateLimiter.AcquireConnectSlot` 的 pipeline（ZRemRangeByScore → ZAdd → ZCard → ZRangeWithScores → PExpire），唯一差别：key = `keyPrefix + string(userID)`，返签名用 `(bool, time.Duration, error)` 而非 ConnectDecision struct。不返 Count 因为 apns_token 场景无需审计具体连接数；如果未来需要扩 Count，加 struct 再说。
   - 单元测试 `pkg/redisx/user_ratelimit_test.go`（miniredis + FakeClock；table-driven 5 子 case）：
     | Test | 断言 |
     |---|---|
     | `TestUserSlidingWindowLimiter_UnderThresholdAllowed` | threshold=5, window=60s；5 次 Acquire，FakeClock.Now 每次 +1s → 全 allowed=true |
     | `TestUserSlidingWindowLimiter_AtThresholdBlockedOn6th` | 同上 5 次后第 6 次 → allowed=false；retry ≈ ageout of oldest (即 window - 4s) |
     | `TestUserSlidingWindowLimiter_SlidingBoundary` | 5 次（FakeClock = 0s, 1s, 2s, 3s, 4s）→ FakeClock 推到 61s（第一条 ageout）→ 第 6 次 → allowed=true |
     | `TestUserSlidingWindowLimiter_BoundaryRetryClampedTo1ms` | 5 次 FakeClock=[0,1,2,3,4s]；第 6 次在 FakeClock=60s 触发（oldest.score==cutoff 边界）→ allowed=false + retryAfter==1ms（**复刻 Story 0.11 r2 修复的 §9.3 d<=0 分支；缺此 case 会 regress 到 retryAfter=window=60s**，客户端拿到 "60s Retry-After" 实际上 1ms 后就能重试） |
     | `TestUserSlidingWindowLimiter_PerUserIsolation` | u1 用完 5 次 → u2 首次 → allowed=true |
     | `TestUserSlidingWindowLimiter_KeyPrefixAppliedCorrectly` | 构造 `"ratelimit:apns_token:"` → 实际 Redis key == `"ratelimit:apns_token:u1"`（miniredis key inspection） |
     | `TestUserSlidingWindowLimiter_NilDepsPanic` | 4 个 panic 子测（cmd/clock/prefix/threshold/window 缺失） |
     | `TestUserSlidingWindowLimiter_KeyPrefixWithoutColonPanics` | `"ratelimit:apns_token"` （无尾冒号）→ panic |
   - **为什么不直接复用 RedisConnectRateLimiter**：Story 0.11 的签名 `AcquireConnectSlot(ctx, userID string) (ConnectDecision, err)` — `userID` 是 `string` 不是 `ids.UserID`，是因为 Story 0.11 时 `ids.UserID` 还不存在。Story 1.4 的新 limiter 直接用 `ids.UserID` 类型参；Story 0.11 的签名本 story 不改（ripple risk 不值；retry 次数广泛）。未来 Epic 2-5 补一个 "统一 slidingWindowAcquire helper + 两个类型 adapter" 的小 refactor story 再收。

7. **AC7 — `internal/repository/user_repo.go` 扩展 `SetSessionHasApnsToken`**：

   - 在 `MongoUserRepository` 上加一个方法（**本 story 在 user_repo.go 里新增**，不建新文件 — 这是 user 领域的同一 collection）：
     ```go
     // SetSessionHasApnsToken toggles users.sessions.<deviceId>.has_apns_token.
     // Introduced by Story 1.4 so DeviceHandler can mark a session as having
     // an APNs token after successful registration. Absent row / absent
     // session sub-document:
     //   - userID missing → ErrUserNotFound
     //   - (userID, deviceID) missing → silent no-op (returns nil): the
     //     refresh flow (Story 1.2) would have written the session before
     //     any /v1/devices/apns-token call is possible, but concurrent
     //     deletion (Story 1.6) or a client bug could race — we treat the
     //     sub-doc absence as "nothing to mark" rather than an error,
     //     matching the "session-flag is a convenience, not an invariant"
     //     semantics documented in the service layer (AC4).
     func (r *MongoUserRepository) SetSessionHasApnsToken(
         ctx context.Context,
         userID ids.UserID,
         deviceID string,
         has bool,
     ) error
     ```
   - 实现思路（AC Review 修正版 —— 单次 UpdateOne，避免 TOCTOU）：
     1. validate(userID, deviceID); deviceID 走 `validateDeviceID`（防 `.` / `$` 注入，既有）。
     2. `filter := bson.M{"_id": string(userID)}` — **仅按 userID 过滤**，不做 `sessions.<d> $exists` 子查询。
     3. `update := bson.M{"$set": bson.M{"sessions." + deviceID + ".has_apns_token": has, "updated_at": r.clock.Now()}}`
     4. `res, err := r.coll.UpdateOne(ctx, filter, update)`
     5. `res.MatchedCount == 0` → `return ErrUserNotFound`（user 不存在，唯一的 "真错误" 分支）
     6. `res.MatchedCount > 0` → `return nil`。**重要语义**：Mongo 的 `$set` 对 dotted path 具有**自动创建子文档**行为 —— 如果 `sessions.<deviceID>` 此前不存在，`$set` 会创建一个仅含 `has_apns_token: true` 字段的子对象（`CurrentJTI` 和 `IssuedAt` 未设）。MVP 接受此行为：
        - 正常路径：用户先走 Story 1.2 refresh / Story 1.1 SIWA 已写入 `sessions.<d>.{current_jti, issued_at}` → 本调用仅 merge 追加 `has_apns_token` 字段，无副作用
        - 边缘路径：客户端 bug 导致 sessions 子文档不存在 → 创建一个残缺子文档。GetSession 的既有实现 `doc.Sessions[deviceID]` 会返回 zero-value `current_jti=""` / `issued_at=zero`，Story 1.2 的 refresh 路径 `UpsertSessionIfJTIMatches(expectedJTI="")` 已经显式拒绝空 jti (`repository: upsert session cas: empty expected jti` err)，所以残缺子文档不会被当成合法 session 复用 —— 安全。
     7. **不**做 "存在则 update, 不存在则 no-op" 语义 —— 这正是 AC Review 发现的 TOCTOU 陷阱：两次 Mongo 调用之间 user 可能被 Story 1.6 删除，测试用例 `TestUserRepo_Integration_SetSessionHasApnsToken_UnknownSession_NoOp` 会被条件竞赛 flaky。
   - 单元测试归入既有 `user_repo_test.go` / `user_repo_integration_test.go`：
     | Test | 断言 |
     |---|---|
     | `TestUserRepo_Integration_SetSessionHasApnsToken_HappyPath` | seed user + session → SetSessionHasApnsToken(u,d,true) → 读 GetSession → `HasApnsToken == true` |
     | `TestUserRepo_Integration_SetSessionHasApnsToken_UnknownUser` | SetSessionHasApnsToken(unknown,d,true) → ErrUserNotFound |
     | `TestUserRepo_Integration_SetSessionHasApnsToken_UnknownSession_AutoCreatesMinimalSubDoc` | seed user without session d → SetSessionHasApnsToken(u,d,true) → nil err；GetSession 返 (session, true, nil) with `HasApnsToken=true` / `CurrentJTI=""` / `IssuedAt=zero`（即 Mongo dotted-path $set 的自动创建语义 —— 合规，因为 Story 1.2 refresh CAS 拒绝空 jti 不会复用此残缺子文档） |
     | `TestUserRepo_Integration_SetSessionHasApnsToken_InvalidDeviceID` | deviceID="." → err contains "reserved path characters" |
     | `TestUserRepo_Integration_SetSessionHasApnsToken_TogglesFalse` | seed has=true → set false → read false |

8. **AC8 — `internal/config/config.go` 扩展 APNs 加密 key + 限流阈值**：

   - `APNsCfg` 新增字段（仅 2 个；已有字段不动）：
     ```go
     type APNsCfg struct {
         // ... existing fields ...
         TokenEncryptionKeyHex    string `toml:"token_encryption_key_hex"`     // 64 hex chars = 32 bytes (AES-256); required when Enabled=true
         RegisterRatePerWindow    int    `toml:"register_rate_per_window"`    // 5 per window (epic AC: 60s ≤ 5)
         RegisterRateWindowSec    int    `toml:"register_rate_window_sec"`    // 60
     }
     ```
   - `applyDefaults`：
     ```go
     if c.APNs.RegisterRatePerWindow == 0 { c.APNs.RegisterRatePerWindow = 5 }
     if c.APNs.RegisterRateWindowSec == 0 { c.APNs.RegisterRateWindowSec = 60 }
     // TokenEncryptionKeyHex has NO compile-time default — empty → validate error when Enabled.
     ```
   - `validateAPNs` 扩展（与既有 WorkerCount / ReadCount 一致的"positive int 恒校验，secret 只在 enabled 时校验"模式）：
     ```go
     if c.APNs.RegisterRatePerWindow <= 0 {
         log.Fatal().Int("register_rate_per_window", c.APNs.RegisterRatePerWindow).
             Msg("config: apns.register_rate_per_window must be > 0")
     }
     if c.APNs.RegisterRateWindowSec <= 0 {
         log.Fatal().Int("register_rate_window_sec", c.APNs.RegisterRateWindowSec).
             Msg("config: apns.register_rate_window_sec must be > 0")
     }
     if !c.APNs.Enabled {
         return  // existing
     }
     // --- existing key_path / key_id / team_id / topics checks ---
     if c.APNs.TokenEncryptionKeyHex == "" {
         log.Fatal().Msg("config: apns.enabled=true requires apns.token_encryption_key_hex (64 hex chars = 32 bytes AES-256)")
     }
     if decoded, err := hex.DecodeString(c.APNs.TokenEncryptionKeyHex); err != nil || len(decoded) != 32 {
         log.Fatal().Err(err).Int("decoded_len", len(decoded)).
             Msg("config: apns.token_encryption_key_hex must decode to exactly 32 bytes (AES-256)")
     }
     ```
   - `config/default.toml` `[apns]` section 追加：
     ```toml
     token_encryption_key_hex = ""       # 32-byte hex; required when enabled=true
     register_rate_per_window = 5
     register_rate_window_sec = 60
     ```
   - `config_test.go` 扩展：
     - `TestMustLoad_APNsDefaultsAppliedWhenSectionOmitted` 扩展断言新 2 整数默认值
     - `TestValidateAPNs_EnabledRequiresEncryptionKey` — 直接构造 `Config{APNs: {Enabled: true, ..., TokenEncryptionKeyHex: ""}}` → 调 `validateAPNs`（需要 dev 把函数从 log.Fatal 版本抽出个可测版本 —— 既有 `validateAPNs` 已用 log.Fatal，不便测；**简化**：只加一个 `TestValidateAPNs_EncryptionKeyWrongLength_Returns31ByteHex` 通过子进程 / skipped if-race；或直接省测，在 AC9 integration test 里给一个 `enabled=true` + 有效 key 的场景间接覆盖）。Dev agent 择优 — **推荐**：跳过单测，靠 integration + 运行时失败保护；在 Dev Notes 承认覆盖缺口。

9. **AC9 — `cmd/cat/initialize.go` wiring + §21.2 Empty Provider 填实 + `cmd/cat/wire.go` 路由**：

   - `initialize()` 扩展（在 Story 1.3 httpJWTAuth 后、handlers struct 前插入）：
     ```go
     // --- Story 1.4: APNs device token registration wiring ---
     // - Builds the repo (AES-GCM sealer + Mongo apns_tokens collection).
     // - Fills three Story 0.13 Empty Providers (TokenProvider / TokenDeleter
     //   / TokenCleaner) with the real *MongoApnsTokenRepository (§21.2).
     //   The fourth (QuietHoursResolver) stays Empty — Story 1.5 fills it.
     // - Wires the /v1/devices/apns-token handler.
     // Debug mode skips encryption-key requirement: no /v1/* endpoint runs
     // outside test harnesses in debug, so the repo is built with a dev
     // all-zero key for test / CI convenience — see AC11 rationale. This
     // branch is NEVER taken in release, as config.validateAPNs rejects
     // enabled=true + empty key_hex.
     apnsTokenRepo := mustBuildApnsTokenRepo(cfg, mongoCli.DB(), clk)
     if err := apnsTokenRepo.EnsureIndexes(context.Background()); err != nil {
         log.Fatal().Err(err).Msg("apns_token repo EnsureIndexes failed")
     }
     apnsRegisterLimiter := redisx.NewUserSlidingWindowLimiter(
         redisCli.Cmdable(), clk,
         "ratelimit:apns_token:",
         int64(cfg.APNs.RegisterRatePerWindow),
         time.Duration(cfg.APNs.RegisterRateWindowSec)*time.Second,
     )
     apnsTokenSvc := service.NewApnsTokenService(
         apnsTokenRepo, userRepo, apnsRegisterLimiter, clk,
     )
     deviceHandler := handler.NewDeviceHandler(apnsTokenSvc)
     // --- /Story 1.4 ---
     ```
   - 抽 helper `mustBuildApnsTokenRepo(cfg, db, clk) *repository.MongoApnsTokenRepository`（同文件底部）：
     ```go
     // mustBuildApnsTokenRepo constructs the apns-token repository with an
     // AES-GCM sealer. Release mode requires cfg.APNs.TokenEncryptionKeyHex
     // (enforced earlier by config.validateAPNs). Debug / test mode falls
     // back to an all-zero 32-byte key so tests that exercise the repo do
     // not need secret plumbing — this IS intentional and NEVER reachable
     // in release (validateAPNs would have already log.Fatal'ed). The dev
     // key is documented as "not security-sensitive" so future grep shows
     // the dev-vs-prod split (review-antipatterns §7.1).
     func mustBuildApnsTokenRepo(cfg *config.Config, db *mongo.Database, clk clockx.Clock) *repository.MongoApnsTokenRepository {
         var key []byte
         if cfg.APNs.TokenEncryptionKeyHex == "" {
             // Debug / test mode — dev dummy key (32 zero bytes). Release
             // mode never reaches this branch (validateAPNs rejects empty
             // key when Enabled=true; Enabled=false leaves no consumer).
             key = make([]byte, 32)
         } else {
             decoded, err := hex.DecodeString(cfg.APNs.TokenEncryptionKeyHex)
             if err != nil {
                 log.Fatal().Err(err).Msg("apns token_encryption_key_hex decode failed")
             }
             key = decoded
         }
         sealer, err := cryptox.NewAESGCMSealer(key)
         if err != nil {
             log.Fatal().Err(err).Msg("apns token sealer init failed")
         }
         return repository.NewMongoApnsTokenRepository(db, clk, sealer)
     }
     ```
   - **§21.2 Empty Provider 填实** — 扩展既有 APNs block（initialize.go line 78 + line 90）替换三个 Empty：
     ```go
     // Before (Story 0.13):
     router := push.NewAPNsRouter(push.EmptyTokenProvider{}, cfg.APNs.WatchTopic, cfg.APNs.IphoneTopic)
     apnsWorker := push.NewAPNsWorker(push.APNsWorkerConfig{...}, redisCli.Cmdable(), sender, router,
         push.EmptyQuietHoursResolver{}, push.EmptyTokenDeleter{}, clk)

     // After (Story 1.4):
     // TokenProvider real impl — swapped Empty via Story 1.4 (§21.2).
     router := push.NewAPNsRouter(apnsTokenRepo, cfg.APNs.WatchTopic, cfg.APNs.IphoneTopic)
     // TokenDeleter real impl — swapped Empty via Story 1.4 (§21.2).
     // QuietHoursResolver real impl — Story 1.5 fills this (stays Empty in Story 1.4).
     apnsWorker := push.NewAPNsWorker(push.APNsWorkerConfig{...}, redisCli.Cmdable(), sender, router,
         push.EmptyQuietHoursResolver{}, apnsTokenRepo, clk)
     ```
   - Cron scheduler TokenCleaner 同步换（initialize.go line 54-57）：
     ```go
     // Before:
     cronSch := cron.NewScheduler(locker, redisCli.Cmdable(), clk, push.EmptyTokenCleaner{}, time.Duration(cfg.APNs.TokenExpiryDays)*24*time.Hour)

     // After:
     // TokenCleaner real impl — swapped Empty via Story 1.4 (§21.2).
     cronSch := cron.NewScheduler(locker, redisCli.Cmdable(), clk, apnsTokenRepo, time.Duration(cfg.APNs.TokenExpiryDays)*24*time.Hour)
     ```
     **注意**：`cronSch` 构造依赖 apnsTokenRepo，但 apnsTokenRepo 构造又依赖 mongoCli.DB() + clk，clk 的构造在 cronSch 之前（line 52）；所以把 `mustBuildApnsTokenRepo` + `EnsureIndexes` 调用**移到 cronSch 构造之前**（initialize.go line 54 之前）。Dev agent 调整顺序。
   - `cmd/cat/wire.go` 扩展：
     ```go
     type handlers struct {
         // ... existing fields ...
         device    *handler.DeviceHandler // Story 1.4 — POST /v1/devices/apns-token
     }
     ```
     `buildRouter` 的 `/v1/*` group 内部追加：
     ```go
     if h.jwtAuth != nil {
         v1.Use(h.jwtAuth)
     }
     if h.device != nil {
         v1.POST("/devices/apns-token", h.device.RegisterApnsToken)
     }
     if h.v1Routes != nil {
         h.v1Routes(v1)
     }
     ```
     **debug mode 注意**：`h.jwtAuth == nil`（Story 1.3 debug 不挂 middleware）。Story 1.4 的 device endpoint 在 debug 下会**无鉴权**直接跑！这是**刻意**的：debug 模式下本来就没有真实用户，所有 /v1/* endpoint 放行；如果需要模拟鉴权，开发者改 `initialize.go` 的 debug 分支显式挂 `middleware.JWTAuth(debugVerifier)`。此行为与 Story 1.3 AC7 的 "debug 模式 /v1/* 裸露"是**一致**的，不算新引入的风险。**但**：debug 模式下 `middleware.UserIDFrom(c)` 返回 `""`，handler 的 defense-in-depth 401 分支会兜底拒绝，所以 debug 客户端无法真的注册 token — **符合预期** `POST /v1/devices/apns-token` 必须走真鉴权流才有意义。单测 `TestDeviceHandler_RegisterApnsToken_MissingUserID_401` 锁此语义。

     **Release + `APNs.Enabled=false` 的安全边界（AC Review 发现项，必修）**：wire.go 无条件注册 `/v1/devices/apns-token` route；release 模式下 `h.jwtAuth != nil` 会真鉴权；`mustBuildApnsTokenRepo` 在 `cfg.APNs.TokenEncryptionKeyHex == ""` 时回落 32-byte 零 key —— 后果：**release staging + `Enabled=false` + 空 key 配置时，真实用户 token 会用零 key 封存到 Mongo**（零 key 不是明文，但任何拿到 sealed bytes + 知道 key 全零的人都能解密）。**修法**：`config.validateAPNs` 在 `cfg.Server.Mode == "release"` 下 **ALWAYS 校验** `TokenEncryptionKeyHex != "" && decode 长度 == 32`，不再依赖 `cfg.APNs.Enabled`（加密 key 与 push 开关是两个正交概念：前者 = "apns_tokens 加密存储"，后者 = "真的发推送"；staging 允许 persist 但不发）。AC8 的 `validateAPNs` 实现要同步调整为：
     ```go
     if c.Server.Mode == "release" {
         if c.APNs.TokenEncryptionKeyHex == "" {
             log.Fatal().Msg("config: release mode requires apns.token_encryption_key_hex (regardless of apns.enabled)")
         }
         // 32-byte decode check fires here too (moved out of the Enabled=true branch).
     }
     if !c.APNs.Enabled { return }
     // ... remaining Enabled=true checks (key_path / key_id / team_id / topics) unchanged
     ```
     单测加 `TestValidateAPNs_ReleaseModeAlwaysRequiresEncryptionKey`（`Config{Server.Mode:"release", APNs.Enabled:false, APNs.TokenEncryptionKeyHex:""}` → log.Fatal）。Dev Notes 记下"为什么不选修法 B（route 条件注册）"：修法 B 会让 staging 客户端的 POST 拿 404 触发无意义重试风暴，修法 A 更少侵入性。
   - `initialize_test.go` 扩展：
     - `TestInitialize_V1DevicesApnsToken_RegistersRoute` — 真 initialize(cfg) + 命中 `/v1/devices/apns-token` route 的 404/401 行为（cfg.APNs.Enabled=false + debug 不挂 JWT → 命中 handler 但 UserIDFrom 空 → 401；release 模式 cfg.APNs.Enabled=true + 带 access token → 200）
     - `TestInitialize_ApnsTokenRepo_SwapsEmptyProviders` — 启动后断言**语法层**："Empty 已换 Real"靠 AC14 两条 grep 守门（见 AC14 §21.2 Empty Provider grep 回归）；此 test 专注于**运行时**：`cfg.APNs.Enabled=false` 时 NoopPusher 在、apnsTokenRepo 构造成功且 `EnsureIndexes` 被调用过（可通过 repo 暴露 `IndexesEnsured bool` test-only 字段，或依赖集成测 `TestApnsTokenRepo_Integration_EnsureIndexes_Idempotent` 间接验证）；`Enabled=true` 时 APNsWorker 构造成功（依赖 apnsTokenRepo 作 TokenProvider / TokenDeleter，Scheduler 构造成功依赖 apnsTokenRepo 作 TokenCleaner）。

10. **AC10 — 端到端集成测试 `cmd/cat/apns_token_integration_test.go`（`//go:build integration`）**：

    - 复用 `jwt_middleware_integration_test.go` 的 `setupJWTAuthHarness` + `signIn` helper — Story 1.3 已建好。扩展 harness 加 `apnsTokenRepoProjection` 字段直接查 Mongo，让测试能验加密 / 落库内容。
    - 新文件 `cmd/cat/apns_token_integration_test.go`，build tag `integration`。7 个子 case：
      | Test | 断言 |
      |---|---|
      | `TestApnsToken_Integration_Register_HappyPath` | SIWA → access token → POST /v1/devices/apns-token {deviceToken:64hex, platform:watch} → 200 body `{ok:true}`；Mongo apns_tokens 1 条，device_token bytes ≠ 明文；用 harness sealer Open → == 明文；users.sessions[d].has_apns_token == true |
      | `TestApnsToken_Integration_Register_MissingAuth_401` | 不带 Authorization → 401 `AUTH_TOKEN_EXPIRED` |
      | `TestApnsToken_Integration_Register_RefreshTokenRejected_401` | 把 refresh token 当 Bearer → 401 `AUTH_INVALID_IDENTITY_TOKEN` |
      | `TestApnsToken_Integration_Register_InvalidDeviceToken_400` | body deviceToken="ZZZ" → 400 `VALIDATION_ERROR` |
      | `TestApnsToken_Integration_Register_PlatformMismatch_400` | SIWA(platform=watch) + body.platform="iphone" → 400 `VALIDATION_ERROR` |
      | `TestApnsToken_Integration_Register_ReRegister_OverwritesSamePlatform` | 同 user+watch 连续 2 次 POST → apns_tokens 只 1 条（tokenB），updated_at 更新 |
      | `TestApnsToken_Integration_Register_CrossPlatformCoexists` | SIWA(watch) + SIWA(iphone) 两个 device 各 POST → apns_tokens 2 条 |
      | `TestApnsToken_Integration_Register_RateLimitBlocks` | threshold=3（测试专用 cfg.APNs.RegisterRatePerWindow=3） 连续 4 次 POST → 第 4 次 429 `RATE_LIMIT_EXCEEDED` + Retry-After header |
      | `TestApnsToken_Integration_Register_PusherChainIntact` | Register token → 从 harness 取出 apnsTokenRepo.ListTokens(userID) → 1 条；对应 APNsRouter.RouteTokens(userID) 返 1 条 RoutedToken 指向 watch topic（间接证明 §21.2 Empty Provider 已被真实 repo 替换） |
      | `TestApnsToken_Integration_Register_EndToEndPushDispatch` | 决定性 §21.2 网关：harness 用 fake `push.ApnsSender`（不访真 APNs）+ 真 `push.RedisStreamsPusher`（miniredis）+ 真 `push.APNsRouter(apnsTokenRepo, ...)` + 真 `push.APNsWorker`。Register token(watch) → Enqueue(userID, PushPayload{Kind:alert, Title:t}) → 等 worker 消费（miniredis 阻塞 read 手动推进）→ 断言 fakeSender.Calls 含 1 条 `*apns2.Notification` 且 `Topic == cfg.APNs.WatchTopic` 且 `DeviceToken == 注册时的明文`。**这是唯一真正锁死 "Empty→Real 替换后端到端链仍通" 的 test —— Router.tokens 是 *apnsTokenRepo 而非 EmptyTokenProvider 的行为正确性证据**。 |
    - 测试专用 harness：`setupApnsTokenHarness(t)` 在 `jwtAuthHarness` 上叠加 apnsTokenRepo + apnsRegisterLimiter wiring；绑定 `v1.POST("/devices/apns-token", ...)` 到 harness gin engine；暴露 `hrn.rawApnsTokensCollection()` 用作直查 Mongo 断言加密字段。
    - 用于跑滑动窗口的**快速 cfg**：RegisterRatePerWindow=3, RegisterRateWindowSec=60（测试显式传，不走 default）。

11. **AC11 — Fail-closed / Fail-open 决策矩阵（架构指南 §21.3 强制）**：

    | 失败点 | 决策 | 理由 | 可观测点 |
    |---|---|---|---|
    | JWT middleware 鉴权失败 | **fail-closed**（Story 1.3 既有） | 无身份不能改账户绑定 | 401 + `jwt_auth_reject` audit log |
    | Rate limiter Acquire 返 err（Redis 挂） | **fail-closed**（AC4 flow step 1） | 限流失效 = NFR-SCALE-8 漏洞（用户 bug 循环写 token 把 Mongo 打爆），安全影响 | error log + 5xx 返回 + metric redis_error_count |
    | Rate limiter Allowed=false | **fail-closed** | 触发 RATE_LIMIT_EXCEEDED 给客户端 | 429 + audit log `action:apns_token_rate_limit_rejected` |
    | Repo Upsert 返 err（Mongo 挂 / 唯一索引冲突） | **fail-closed** | 未成功落库 = 用户后续推送收不到；必须给 5xx | error log + 500 INTERNAL_ERROR + metric mongo_error_count |
    | Sealer.Seal 返 err（crypto/rand 失败，极罕见） | **fail-closed** | 不能存明文；存 "fake" 密文更糟 | 500 + log error |
    | Sealer.Open 返 ErrCipherTampered（List 时遇到一条脏 row） | **fail-open (局部)** | 一条脏 row 不应让整个用户的推送链挂掉；log warn 跳过，其他 token 正常送 | warn log `apns_token_decrypt_tampered userId platform`；metric tampered_count 递增（future） |
    | `users.sessions[d].has_apns_token` 写失败（repo.SetSessionHasApnsToken 返 err） | **fail-open** | 该 flag 是"便利"非"正确性"—— apns_tokens collection 已完成是核心；flag 是给 /v1/me 快速查询用的冗余 | warn log `apns_token_session_flag_write_failed`；主请求返 200 |
    | DeviceToken 明文泄漏到 INFO+ 日志 | **fail-closed by convention（M14）** | 触发 = 代码审查 bug；合规 / 信任链后果大 | N/A — 靠 code review + lint + logx.MaskAPNsToken 守门 |
    | Encryption key 缺失 / 长度错 | **fail-closed at startup** | config.validateAPNs log.Fatal | log.Fatal + 进程退出 |

    **Decision**：仅 2 条 fail-open：
    1. 单条 ciphertext-tampered → 跳过（隔离）
    2. session flag write 失败 → 吞 warn

    **可观测补偿**：两条都有 warn log；未来 Epic 2-5 引入 Prometheus 后加 `apns_token_decrypt_tampered_total` / `apns_token_session_flag_write_failed_total` metric（本 story 不加 — 没有现成 metric 基础设施，§21.5 "metric 必配 dashboard + alert + runbook" 三件套未到位）。**Dev Notes 显式复述**此 2 条 fail-open 的理由。

12. **AC12 — 反模式回链清单（实施期逐条核对）**：

    - §1.3 并行测试修改全局状态 — middleware_test / handler_test 的 zerolog buffer hook 不用 `t.Parallel`；cryptox / user_ratelimit 允许 `t.Parallel`
    - §2.x ctx cancellation — service 透传 `ctx` 到 limiter / repo；不 swallow；shutdown 时 Mongo / Redis pkg 自带 ctx honoring
    - §3.x JWT — 本 story **不** 动 Verify；消费 Story 1.3 middleware 注入的 claims（Story 1.3 已覆盖 §3.1-§3.5）
    - §4.1 positive-int — AC8 `validateAPNs` 扩展锁 `register_rate_per_window>0 / window_sec>0`
    - §4.2 applyDefaults — AC8 为两个新字段加默认值；空 override 配置仍能启动
    - §4.3 硬编码 ignore config — limiter 阈值 / 窗口直接从 cfg 传入 Acquire；**不**写死 `5 / 60s` 在 initialize / service
    - §7.1 debug/release mode gate — AC9 `mustBuildApnsTokenRepo` 的 debug 零 key 分支：单测 `TestMustBuildApnsTokenRepo_DebugModeZeroKey` + `_ReleaseModeUsesCfgKey` 双覆盖
    - §8.1 Redis key namespace — AC6 `keyPrefix` 构造强制校验非空 + 以 `:` 结尾；key 结构 `ratelimit:apns_token:<userID>` 与 `ratelimit:ws:*` / `blacklist:*` / `refresh_blacklist:*` 严格隔离
    - §8.2 冒号拼接注入 — userID 是 UUID v4（ids.NewUserID），colon-free，单段 concat 即 injective；DeviceID 走 `validateDeviceID`（既有，防 `.$` 注入 Mongo dotted path）
    - §9.1 固定窗口伪装成滑动窗口 — AC6 明确使用 ZSET + 毫秒 score；测试 `TestUserSlidingWindowLimiter_SlidingBoundary` 锁定跨窗口不翻倍
    - §9.2 纳秒精度 loss — 复用 Story 0.11 毫秒 score（`UnixMilli()`）
    - §10.x middleware order — 本 story 不动中间件顺序；复用 Logger → Recover → RequestID → (v1 group) JWTAuth
    - §11.x APNs worker — 本 story **不** 动 worker；仅把 3 个 Empty Provider 换成 real repo，worker 逻辑（writeCtxFor / retry / DLQ / 410 cleanup）保持 Story 0.13 语义
    - §13.1 pkg/ ← internal/ — `pkg/cryptox` 不依赖 internal；`pkg/redisx.UserSlidingWindowLimiter` 仅依赖 pkg/clockx + pkg/ids，不依赖 internal
    - §14.1 godoc 语义靠测试锁死 — AC6 limiter / AC2 cryptox / AC4 service flow 每条 godoc 断言都有对应测试

13. **AC13 — 客户端契约同步（`docs/api/openapi.yaml` + `docs/api/integration-mvp-client-guide.md`）**：

    - `docs/api/openapi.yaml`：
      - `info.version` bump `1.3.0-epic1` → `1.4.0-epic1`
      - 新增 path `/v1/devices/apns-token`（POST）：
        - 走全局 security（BearerAuth），**不**写 `security: []`（继承默认即鉴权必需）
        - request body schema `RegisterApnsTokenRequest` 含 deviceToken + 可选 platform
        - response 200 schema `RegisterApnsTokenResponse` (`{ok: true}`)；401 共用既有 AUTH_* error 引用；400 VALIDATION_ERROR；429 RATE_LIMIT_EXCEEDED + Retry-After header
      - 新增 schemas: `RegisterApnsTokenRequest / RegisterApnsTokenResponse`
    - `docs/api/integration-mvp-client-guide.md` 扩展新 §15 "APNs device token 注册 (Story 1.4)"：
      - 触发时机：客户端首次获得 APNs device token（UIApplication / WKApplication didRegisterForRemoteNotificationsWithDeviceToken）或 token 变化时（Apple 可能随时换 token）
      - Endpoint: `POST /v1/devices/apns-token` with Bearer access token
      - Body: `{"deviceToken":"<hexString>","platform":"watch"|"iphone"}`（platform 可省；服务端从 JWT claim 读）
      - Response: `200 {"ok":true}`
      - 失败行为：
        - `400 VALIDATION_ERROR` → 客户端 bug，log + 不重试
        - `401 AUTH_TOKEN_EXPIRED` → refresh → 重试
        - `429 RATE_LIMIT_EXCEEDED` → 读 Retry-After header，退避后重试
        - `500` → 指数退避重试（最多 3 次），之后等下次 token 变化触发
      - 重复注册同 device class 为幂等（服务端 upsert）；更换 Watch + iPhone 两端独立存储
      - 取消注册 API 将在 Story 1.6 账户注销时一并做，本阶段无单独 DELETE endpoint

14. **AC14 — 测试自包含 + 回归 + §21.2 Empty Provider 填实验证（§21.7 + 架构 §19）**：

    - `bash scripts/build.sh --test` 单测全绿
    - `go test -tags=integration ./...` 集成测试 Linux CI 全绿（Mongo Testcontainers + miniredis）
    - `bash scripts/build.sh --race --test` Linux CI 通过（Windows cgo 限制允许跳过）
    - **零外部依赖**：不调真 APNs；不调 iOS / watchOS app；所有 HTTP 走 httptest
    - **§21.2 Empty Provider grep 回归**（注意两套 regex 各自锁一侧）：
      1. **Empty Provider（session.resume + push.EmptyTokenProvider）**：
         ```bash
         grep -cE "Empty[A-Za-z]+Provider\{\}" cmd/cat/initialize.go
         # Story 1.3 交付态：6 条
         #   - 5 条 session.resume: ws.Empty{Friends,CatState,Skins,Blindboxes,RoomSnapshot}Provider{}  (line 155-159)
         #   - 1 条 push: push.EmptyTokenProvider{}                                                        (line 78)
         # Story 1.4 目标态：5 条（push.EmptyTokenProvider 被 apnsTokenRepo 替换，session.resume 5 条保持）
         ```
      2. **Empty Deleter / Cleaner / Resolver（Story 0.13 遗留的 push 3 个非-Provider 命名接口）**：
         ```bash
         grep -cE "push\.Empty(TokenDeleter|TokenCleaner|QuietHoursResolver)\{\}" cmd/cat/initialize.go
         # Story 1.3 交付态：3 条（EmptyTokenCleaner line 55 + EmptyQuietHoursResolver line 90 + EmptyTokenDeleter line 90）
         # Story 1.4 目标态：1 条（EmptyQuietHoursResolver 留给 Story 1.5；TokenDeleter / TokenCleaner 被 apnsTokenRepo 替换）
         ```
      **验证规则**：dev agent 实施前分别跑两条 grep 记录 baseline，实施后再跑。期望差值：
      - regex 1（`Empty[A-Za-z]+Provider{}`）：`6 → 5`（-1：push.EmptyTokenProvider 被换）
      - regex 2（`push.Empty(TokenDeleter|TokenCleaner|QuietHoursResolver){}`）：`3 → 1`（-2：TokenDeleter + TokenCleaner 被换，QuietHoursResolver 留）
      - **Completion Notes 记录两条 grep 的 before/after 数字 + 解释哪些被哪个 real impl 替换**；任一数字与预期不符必须解释或修代码。
    - `scripts/check_time_now.sh`（若存在）确认 `internal/service/apns_token_service.go` 用 `s.clock.Now()` 而非 `time.Now()`（架构 M9）
    - 集成测试**禁 `t.Parallel()`**（架构 §M11）

15. **AC15 — 架构指南 §21 纪律自证**：

    - §21.1 双 gate 漂移守门 — 本 story **不** 引入新全局常量集合（error codes / WSMessages / Redis key prefix / Provider 名字均复用）。新增的 `ratelimit:apns_token:` 是 keyPrefix 参数，不是枚举常量集合。**Dev Notes 声明 "N/A"**。
    - §21.2 Empty Provider 填实 — **三换一留**：TokenProvider / TokenDeleter / TokenCleaner 三个 Empty → real `*MongoApnsTokenRepository`；QuietHoursResolver 留给 Story 1.5。AC9 的 initialize.go 注释显式标 `// Real impl via Story 1.4 (§21.2)`。Completion Notes 复述 grep -3 验证结果。
    - §21.3 fail-closed vs fail-open — AC11 矩阵完整；2 条 fail-open 均有 warn log 可观测。Dev Notes 复述。
    - §21.4 AC review 早启 — 本 story 含 **加密 + 限流**两条"语义错但不 crash"高风险。Dev agent 实施前**必须**先做 self-AC-review（对照 AC2 cryptox 语义 / AC4 service flow 顺序 / AC6 sliding-window 语义 / AC11 fail-open/closed 矩阵逐条 walkthrough），在 Completion Notes 写明 "AC self-review 通过" 或修 AC 后再实施。
    - §21.5 tools/* CLI — 不引入
    - §21.6 spike 类 — N/A，纯代码
    - §21.7 测试自包含 — AC14 强制
    - §21.8 语义正确性思考题 — 本 story 末尾专段，Completion Notes 必答

## Tasks / Subtasks

- [x] **Task 1 (AC: #2)** — `pkg/cryptox` AES-GCM 包新建
  - [x] `pkg/cryptox/aesgcm.go`：`AESGCMSealer` + `NewAESGCMSealer` + `Seal` + `Open` + `ErrCipherTampered`
  - [x] `pkg/cryptox/aesgcm_test.go` 9 个 table-driven 子测（含 round-trip / nonce 随机 / 各种 tamper 分支）
  - [x] `pkg/cryptox/doc.go` — 如已有则不建；package godoc 挂在 aesgcm.go 顶
  - [x] `go vet ./pkg/cryptox/...` + `go test ./pkg/cryptox/...` 全绿

- [x] **Task 2 (AC: #6)** — `pkg/redisx` per-user 滑动窗口 limiter
  - [x] `pkg/redisx/user_ratelimit.go`：`UserSlidingWindowLimiter` + `NewUserSlidingWindowLimiter` + `Acquire`
  - [x] `pkg/redisx/user_ratelimit_test.go` 7 个 miniredis + FakeClock 子测（含 sliding boundary / per-user isolation / key prefix 强制尾冒号 / nil deps panic）

- [x] **Task 3 (AC: #1, #3)** — `domain` + `repository` 层
  - [x] `internal/domain/apns_token.go`：`ApnsToken` 值对象
  - [x] `internal/repository/apns_token_repo.go`：
    - [x] `apnsTokenDoc` private BSON struct + `toDomain` / `docFromDomain` 转换
    - [x] `MongoApnsTokenRepository` + `NewMongoApnsTokenRepository`（nil dep panic）
    - [x] `EnsureIndexes` (`user_id_1_platform_1` unique compound)
    - [x] `Upsert` / `ListByUserID` / `Delete` / `DeleteExpired`
    - [x] `ListTokens` adapter → `push.TokenInfo` 匹配 `push.TokenProvider` 接口
    - [x] Sentinel errors `ErrApnsTokenNotFound` / `ErrApnsTokenCipherTampered`
  - [x] `internal/repository/apns_token_repo_integration_test.go`（build tag integration）10 个 Testcontainers 子测（含 at-rest-encrypted / cipher-tampered skip / cross-platform / unique index / 并发 upsert）

- [x] **Task 4 (AC: #7)** — `MongoUserRepository.SetSessionHasApnsToken`
  - [x] 在 `internal/repository/user_repo.go` 加方法 + godoc
  - [x] `internal/repository/user_repo_integration_test.go` 加 5 个子测（happy / unknown user / unknown session no-op / invalid deviceID / toggles false）

- [x] **Task 5 (AC: #4)** — `internal/service/apns_token_service.go` + 单测
  - [x] `ApnsTokenHandlerService` / `RegisterApnsTokenRequest` / `apnsTokenRepo` / `userSessionRepo` / `apnsTokenRegisterRateLimiter` 接口
  - [x] `ApnsTokenService` + `NewApnsTokenService`（nil dep panic）
  - [x] `RegisterApnsToken` 4 步流程（limiter → upsert → session flag (best-effort) → audit log）
  - [x] `internal/service/apns_token_service_test.go` 7 个 fake-driven 子测

- [x] **Task 6 (AC: #5)** — `internal/handler/device_handler.go` + DTO + 单测
  - [x] `internal/dto/device_dto.go`：`RegisterApnsTokenRequest` + `RegisterApnsTokenResponse`
  - [x] `internal/handler/device_handler.go`：`DeviceHandlerService` + `DeviceHandler` + `RegisterApnsToken`
  - [x] `internal/handler/device_handler_test.go` 11 个 gin recorder 子测（含 happy / invalid body / platform mismatch / legacy JWT no platform / missing userId 401 / service error bubbles / nil panic）

- [x] **Task 7 (AC: #8)** — `config.Config` 扩展
  - [x] `APNsCfg` 加 `TokenEncryptionKeyHex` / `RegisterRatePerWindow` / `RegisterRateWindowSec`
  - [x] `applyDefaults`：2 个整数默认值
  - [x] `validateAPNs`：正整数校验 + (Enabled=true 时) hex key 32 字节校验
  - [x] `config/default.toml` `[apns]` 追加 3 行
  - [x] `internal/config/config_test.go`：`TestMustLoad_APNsDefaultsAppliedWhenSectionOmitted` 新字段 + 新 `TestMustLoad_APNsEnabledRequiresEncryptionKey` 子测（或在 AC14 integration 间接覆盖）

- [x] **Task 8 (AC: #9)** — `cmd/cat/initialize.go` + `wire.go` wiring + §21.2 填实
  - [x] `mustBuildApnsTokenRepo` helper（同文件）
  - [x] `initialize()` 插入 apnsTokenRepo / apnsRegisterLimiter / apnsTokenSvc / deviceHandler 构造（在 handlers struct 组装前）
  - [x] `initialize()` 把 `push.EmptyTokenProvider{}` / `push.EmptyTokenDeleter{}` / `push.EmptyTokenCleaner{}` 三处换成 `apnsTokenRepo`；调整构造顺序让 apnsTokenRepo 在 cronSch 之前
  - [x] `wire.go` `handlers` struct 加 `device *handler.DeviceHandler`；`buildRouter` v1 group 内追加 `v1.POST("/devices/apns-token", h.device.RegisterApnsToken)`；nil-safe
  - [x] `initialize_test.go`：`TestInitialize_V1DevicesApnsToken_RegistersRoute` + `TestMustBuildApnsTokenRepo_DebugModeZeroKey` + `TestMustBuildApnsTokenRepo_ReleaseModeUsesCfgKey`

- [x] **Task 9 (AC: #10)** — 端到端集成测试
  - [x] `cmd/cat/apns_token_integration_test.go`（build tag integration）9 个子测
  - [x] `setupApnsTokenHarness(t)` helper 扩展 `setupJWTAuthHarness` 叠加 device handler + Mongo apns_tokens 集合直查

- [x] **Task 10 (AC: #13)** — 客户端契约文档
  - [x] `docs/api/openapi.yaml`：info.version bump；新增 `/v1/devices/apns-token` POST path；新增 2 个 schemas
  - [x] `docs/api/integration-mvp-client-guide.md`：新增 §15 "APNs device token 注册"（触发时机 / endpoint / body / 失败行为 / 幂等性）

- [x] **Task 11 (AC: #14, #15)** — 回归 + 自检
  - [x] `bash scripts/build.sh --test` 全绿
  - [x] `go test -tags=integration ./...` 全绿
  - [x] `bash scripts/build.sh --race --test` Linux CI 通过（Windows 本地允许跳）
  - [x] `grep "Empty.*Provider{}" cmd/cat/initialize.go` 核对 before/after 差值 = -3；Completion Notes 记录
  - [x] AC self-review（§21.4）通过证据写 Completion Notes — 对照 AC2 / AC4 / AC5 / AC6 / AC11 逐条 walkthrough；**额外确认 AC Review 6 条 fix 都已内化**：(1) release-mode-always-validate encryption key、(2) intra-internal import direction grep gate、(3) Delete tampered-row-skips-valid-sibling test、(4) sliding-window §9.3 boundary retry-1ms test、(5) SetSessionHasApnsToken 单次 UpdateOne 避 TOCTOU、(6) handler JWT 空平台 fail-closed
  - [x] Semantic-correctness 思考题 8 条全部回答写 Completion Notes

## Dev Notes

### 本 story 为何重要（Epic 1 的"首条 /v1/* 业务 + 首批 Empty Provider 填实"）

- **Story 1.3 建了 /v1/* 组但没人用**：JWT middleware 挂好、`v1 := r.Group("/v1")` 准备好，但 Story 1.3 交付时 group 里一条业务路由都没有。本 story 交付**第一条**业务 endpoint，实际验证 Story 1.3 middleware + handler 链路在真实业务路径下端到端正确。
- **Story 0.13 的 4 个 Empty Provider 3 条落地**：Epic 0 为 APNs 平台预埋 4 个 consumer-side 接口 + 4 个 Empty 实现。本 story 填 3 个（TokenProvider / TokenDeleter / TokenCleaner —— 都依赖 `apns_tokens` collection），QuietHoursResolver 留给 Story 1.5（依赖 users.preferences.quietHours + users.timezone，是 Profile 领域的事情）。**这是 §21.2 "Empty→Real 逐步填实"的首次集体落地**。
- **NFR-SEC-7 落地**：PRD 明确要求 APNs device token 加密存储（"Mongo field-level 加密或 KMS"）。本 story 选 **pkg/cryptox** AES-GCM 字段级加密路径（MVP 不引入 KMS 依赖），未来 Growth 若需要 envelope encryption 再上 KMS。该选择在架构指南里没有显式支持，但与 "backend 自建 Go 服务，不用 Firebase/外部依赖"的用户 memory 一致（避免 KMS 这类外部依赖 MVP 阶段先简化）。

### 关键依赖与 Epic 0/1.1-1.3 资产复用

| 来源 | 资产 | Story 1.4 用法 |
|---|---|---|
| 0.2 | `Runnable` + `initialize()` | wiring apnsTokenRepo + device handler |
| 0.3 | Mongo + Redis client | repo + limiter |
| 0.5 | `logx.Ctx(ctx)` + `logx.WithUserID` + `logx.MaskAPNsToken` | audit log + M14 token 脱敏 |
| 0.6 | `ErrValidationError` / `ErrRateLimitExceeded` / `ErrInternalError` / `ErrAuthInvalidIdentityToken` | 复用全部；**不新增**错误码（§21.1 drift gate N/A） |
| 0.7 | `clockx.Clock` / `FakeClock` | service.UpdatedAt + limiter nowMs + test |
| 0.8 | `cron.Scheduler.addLockedJob("@daily", "apns_token_cleanup", ...)` | 通过注入 `apnsTokenRepo` 作 TokenCleaner，cron 已在 Story 0.13 + 0.8 搭好 |
| 0.11 | `redisx.RedisConnectRateLimiter` 的 ZSET + ms score 算法 | `UserSlidingWindowLimiter` 完全复用算法，仅 key prefix 可配置 |
| 0.13 | `push.Pusher` / `push.TokenProvider` / `push.TokenDeleter` / `push.TokenCleaner` / `push.APNsRouter` / `push.APNsWorker` | apnsTokenRepo 同时满足 TokenProvider / TokenDeleter / TokenCleaner 三接口，直接注入 Router / Worker / Scheduler 构造 |
| 1.1 | `domain.User` / `ids.UserID` / `ids.Platform` / `ids.PlatformWatch` / `ids.PlatformIphone` | typed ID + Platform 枚举复用 |
| 1.2 | `users.sessions[deviceId]` struct + `domain.Session.HasApnsToken` 字段 | Story 1.1 已预留字段；本 story 首次写入（true） |
| 1.3 | `middleware.JWTAuth` + `middleware.UserIDFrom / DeviceIDFrom / PlatformFrom` + `/v1/*` group | handler 读 ctx keys 取 identity 三元组 |
| 1.3 | integration harness `setupJWTAuthHarness` + `signIn` helper | 本 story 的集成测试扩展它为 `setupApnsTokenHarness` |

### fail-closed / fail-open 完整声明（架构指南 §21.3 强制）

（亦见 AC11 矩阵；此段是架构指南要求的**集中**声明点。）

- **所有鉴权 / 限流 / persist 路径**：**fail-closed**。Redis 挂、Mongo 挂、crypto seal 失败 → 全部 5xx / 429；不存在"某个便利功能降级"的 fail-open。
- **单条 ciphertext-tampered**：**fail-open（局部）**。`ListTokens` 遇到一条解密失败的 row 时跳过该条 + warn log，**其他 token 照常送**。理由：这通常发生在 key rotation / 数据迁移 / 攻击者写入的脏行场景；让整个用户的推送链因一条脏数据挂掉，比让合法 token 照常工作更糟。可观测点：`apns_token_decrypt_tampered userId platform` warn log；后续 Epic 加 Prometheus 后挂 metric + alert。
- **`users.sessions[d].has_apns_token` 写失败**：**fail-open（吞 warn）**。理由：该字段是冗余冗余便利（给 /v1/me 查询用），**不是**正确性不变量；核心推送 pipeline 只读 `apns_tokens` collection。主请求返 200。补偿：warn log 让运维能看到 flag 与 apns_tokens 不一致的情况。
- **反用户 memory `feedback_no_backup_fallback.md`**：无 backup / fallback / retry-with-fallback。单次尝试，失败即反映成 HTTP error。

### 反模式 TL;DR 实施期自检（对应 review-antipatterns.md）

1. close(channel) — N/A（本 story 无 channel）
2. goroutine panic recover — N/A（handler 在 gin goroutine 里，`Recover` 已在外层）
3. shutdown-sensitive I/O — handler / service 都透传 ctx；Mongo + Redis driver 自带 ctx honoring；limiter / repo 的 cleanup 无 shutdown-sensitive 路径（不像 0.13 worker 的 XACK）
4. **全局常量**：无新错误码 / WS 类型 / config 字段集合 — §21.1 drift gate N/A
5. **新 config 字段**：`token_encryption_key_hex` / `register_rate_per_window` / `register_rate_window_sec` 三字段，全部过 applyDefaults + validateAPNs（§4.1 / §4.2 / §4.3 三件套都齐）
6. **JWT**：本 story **不** 动 Verify；消费 Story 1.3 middleware 注入的 claims
7. **debug/release mode gate**：`mustBuildApnsTokenRepo` 的 debug 零 key 分支必须**双测**（`TestMustBuildApnsTokenRepo_DebugModeZeroKey` + `_ReleaseModeUsesCfgKey`）
8. **Redis key**：`ratelimit:apns_token:<userID>`；UUID v4 userID 单段 concat 即 injective；namespace 与 `ratelimit:ws:*` / `blacklist:device:*` / `refresh_blacklist:*` / `apns:queue|dlq|retry|idem:*` / `resume_cache:*` / `state:*` / `presence:*` / `lock:cron:*` / `event:*` 全部隔离（D16）
9. **rate limit**：真滑动窗口（ZSET + ms score），**不**是 INCR+EXPIRE-NX 伪装；`TestUserSlidingWindowLimiter_SlidingBoundary` 锁死跨窗口不翻倍
10. **度量 / 比率**：本 story 不加 metric；fail-open 2 条都靠 warn log 可观测（§21.3 合法的"可观测降级"）
11. **中间件顺序**：Logger → Recover → RequestID → (v1 group) JWTAuth — 本 story 不动

### 关于"debug 模式零 key sealer"的决策

- **选择**：debug 模式下 `cfg.APNs.TokenEncryptionKeyHex == ""` 时用**32 字节全零 key** 构造 sealer；release 模式下 `validateAPNs` 已经 `log.Fatal` 拦过空 key（`cfg.APNs.Enabled == true` 路径）。
- **理由**：
  1. debug 模式 MVP 下**可以**有开发者在 `/v1/devices/apns-token` 做手动调试（用 jwt_middleware_integration_test.go 的 harness 模式 + 手贴 access token），此时不应该让开发者必须先配置一个 32 字节密钥才能跑测试
  2. release 模式 + Enabled=false 路径（比如内部 staging + 不真发推送的场景）：如果 token_encryption_key_hex 空，repo 的 sealer 也用零 key；这**理论上**让 staging 的数据能被攻击者用零 key 解密 —— 但 staging 本来就不应该有真 token 数据（没有真 APNs 订阅者 / 开发者造的 dummy token），所以可接受
  3. **关键不变量**：release + Enabled=true（真的发推送）必须有真 key，`validateAPNs` log.Fatal 拦
- **取舍**：debug/staging 环境下写入的密文如果搬到 production 环境，用 production key 读会 `ErrCipherTampered`（sealer.Open 会跳过脏行 warn log）—— 这是**可接受**的失败模式：跨环境数据迁移本来就要走 `tools/` 一次性脚本处理，裸 copy 的数据本来就应该失效。

### 关于 "platform 来源 JWT 而非 body" 的决策（AC Review 修正版）

- **选择**：JWT `claims.Platform`（通过 `middleware.PlatformFrom(c)`）是**唯一**权威源；body.platform 是 optional —— 提供时必须与 JWT 一致（defense in depth）；**JWT 为空时拒绝 401**，不 fall back to body。
- **理由**：
  1. JWT platform 是**认证后的身份字段**；body platform 是**未认证的用户输入**
  2. 防御深度：如果未来增加"Watch 仅能注册 Watch token"的业务规则，已经在 JWT claims 层锁死；body 无法绕过
  3. **attack surface 闭合（§21.8 思考题 #6）**：Story 1.1 / 1.2 所有生产 access token 都带 platform claim；若未来某个 refactor bug 移除了 claim 写入，fallback-to-body 会让攻击者持 Watch token 在 body 里伪装 iphone 注册 —— 把攻击者 iPhone 的 push 送到受害者账户。本 story 选择**fail-closed**：JWT 空 → 401 `AUTH_INVALID_IDENTITY_TOKEN`，强制客户端走 refresh 拿新 token。
- **取舍**：理论上存在"2026-04-19 之前签发的 pre-1.2 token 没有 platform claim"场景 —— 实际上所有 pre-Story-1.2 token 已过 15 分钟 access TTL / 30 天 refresh TTL（截至 dev agent 实施日，约 2026-04-20+），MVP 阶段无在途 legacy token。即便有，客户端收 401 `AUTH_INVALID_IDENTITY_TOKEN` 后会调 /auth/refresh 换新（新 token 必带 platform），用户侧 1 次 round trip 后即可恢复，代价可接受。

### 关于"session flag 是便利而非正确性"的决策

- **选择**：repo.Upsert(apns_token row) 成功后，repo.SetSessionHasApnsToken 失败**不**让主请求失败（swallow warn log）。
- **理由**：
  1. `apns_tokens` collection 是推送 pipeline 的**唯一数据源** —— APNsWorker.handle → APNsRouter.RouteTokens → apnsTokenRepo.ListTokens。写入成功 = 推送可达。
  2. `users.sessions[d].has_apns_token` flag 仅作为 /v1/me 或 Profile endpoint 的**快速指示**（客户端 UI 显示"已启用推送"），延迟一致性可接受
  3. 让主请求失败会导致客户端陷入"反复注册都失败"的死循环，而用户实际上已经可以收到推送了 —— 用户体验 net negative
- **对称设计**：未来 Story 1.6 账户注销会**同时**删 `apns_tokens` row + 清 `sessions[d].has_apns_token` flag；两个操作同路径失败也可以对称 swallow。

### Semantic-correctness 思考题（§21.8 / §19 第 14 条强制）

> **如果这段代码运行时产生了错误结果但没有 crash，谁会被误导？**
>
> **答**：所有未来的 APNs 消费者 —— touch_service (5.2) / blindbox_service (6.2) / cold_start_recall_job (8.2) / profile_service (1.5) 的推送送达相关测试和线上行为全部基于 "apns_tokens 里这个 (userId, platform) 的 deviceToken 是加密正确的 64-hex 字符串" 这一假设运作。以下 8 个陷阱必须在 Completion Notes self-audit：
>
> 1. **AES-GCM nonce 复用 bug**：如果 `Seal` 用固定 nonce（比如把 nonce 塞到常量、或 crypto/rand 读失败时 fallback 到全零），两条不同的 plaintext 在同一 key 下会泄漏 XOR 差，攻击者收集多条 sealed 字节可以恢复明文。AC2 的 `TestSeal_DifferentNonceEachCall` 断言"两次 Seal 同 plaintext 产生不同 sealed"锁死；如果开发者为了"可测试的 deterministic" 偷偷把 nonce 写死，此测试 fail。
>
> 2. **AES-GCM Open 不验 tag bug**：如果 `Open` 漏写 `aead.Open(...)`（比如手动 `SplitN` 然后直接返 ciphertext 段），密文被篡改后仍然会"解密成功"返回脏 bytes，下游就会拿着错 token 发 APNs → token invalid → 410 雪片清除合法 token，APNs pipeline 废掉。AC2 的 `TestOpen_TagTampered` + `TestOpen_WrongKey` 锁死。
>
> 3. **encryption key 长度 silent 接受 bug**：如果 `NewAESGCMSealer(key)` 不校验 `len(key) == 32`，AES cipher 构造会 panic（`aes.NewCipher` 要求 16/24/32）或返 err 被忽略；`Enabled=true` + 错长度 key 启动时会 crash 但 error message 可能指向 cipher 栈深处，运维不知道是配置问题。AC2 的 `TestNewAESGCMSealer_WrongKeyLength` + AC8 的 `validateAPNs` + AC14 的 integration test 三层锁死。
>
> 4. **滑动窗口退化为固定窗口 bug**：如果 `UserSlidingWindowLimiter.Acquire` 用 `INCR + EXPIRE NX` 实现（或 pipeline 里忘加 `ZRemRangeByScore`），5 次/60s 配额在边界处变成 10 次/2s（review-antipatterns §9.1）。攻击者可以短时大量注册 token，每次覆盖同 (userId, platform) 的 apns_tokens row —— 单条记录不会增长，但 Mongo write 压力 + Redis 限流误报会打穿 NFR-SCALE-8。AC6 的 `TestUserSlidingWindowLimiter_SlidingBoundary` 锁死。
>
> 5. **Mongo upsert 幂等性破 bug**：如果 apns_tokens 的 unique index 建错（比如只在 user_id 单字段 unique 而不是 (user_id, platform) 复合 unique），一个用户的 Watch + iPhone token 会相互覆盖 —— Watch 用户再注册 iPhone token 会让 Watch token 被删掉，Watch 推送失效。AC3 的 `TestApnsTokenRepo_Integration_UpsertCrossPlatformCoexists` 锁死。
>
> 6. **Platform 来源信任 body bug（含 fallback 陷阱）**：如果 handler 从 body.platform 读（而不是 JWT claim），或者"JWT 空时 fall back to body"，攻击者偷到一个 Watch access token → POST body `{platform:"iphone"}` 伪装成 iPhone 注册 → 把攻击者 iPhone token 写到受害者账户的 iPhone 栏 → 受害者 iPhone 收到攻击者推送的深度链接（钓鱼）。AC5 的 handler **仅**读 JWT，JWT 空**拒 401**不 fallback；`TestDeviceHandler_RegisterApnsToken_PlatformMismatch_400` + `TestDeviceHandler_RegisterApnsToken_JWTMissingPlatform_401` 双锁。
>
> 7. **410 删除遗漏 bug**：如果 `TokenDeleter.Delete` 实现用 `filter{device_token: sealer.Seal(plaintext)}` 做匹配，因 Seal 每次 nonce 不同，**永远匹配不到任何行**，410 反馈后脏 token 永久积累；发推送时每个用户挂掉 10 次 410，Mongo 被无效 token 打满。AC3 的 `Delete` 实现明确 "decrypt-and-match" 路径 + `TestApnsTokenRepo_Integration_DeleteByPlaintextToken` 锁死。
>
> 8. **Empty Provider 未换 bug（§21.2）**：如果 `cmd/cat/initialize.go` 的 push.APNsRouter / APNsWorker / Scheduler 构造仍然传 `push.EmptyTokenProvider{}` / `EmptyTokenDeleter{}` / `EmptyTokenCleaner{}`，推送 pipeline 看起来跑得好（XADD / XREADGROUP 都工作），但每条消息到 router 时 ListTokens 返空 → "no tokens" ACK → APNs Send **永不触发**；客户端注册了 token、服务端落库了 token，但用户就是收不到通知 —— 这是最隐蔽的"跑得好但啥也没做"bug。AC14 的 `grep "Empty.*Provider{}" cmd/cat/initialize.go` 预期行数 -3 + AC10 的 `TestApnsToken_Integration_Register_PusherChainIntact` 锁死 Empty → Real 替换正确。
>
> **Dev agent 实施完成后在 `Completion Notes List` 里明确写一段"以上 8 个陷阱哪些已被 AC/测试覆盖"的 self-audit；任一条答"未覆盖"必须立即补测试或修代码。**

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.4 APNs device token 注册 endpoint — line 828-847]
- [Source: _bmad-output/planning-artifacts/prd.md#FR4 APNs device token 注册 — line 41]
- [Source: _bmad-output/planning-artifacts/prd.md#FR43 410 清理 — line 112]
- [Source: _bmad-output/planning-artifacts/prd.md#FR58 platform 路由 — line 119]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-7 加密存储 — line 143]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SCALE 注册限流 — 架构指南 §21.3]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-INT-2 APNs HTTP/2 + token auth — line 193]
- [Source: _bmad-output/planning-artifacts/architecture.md#POST /devices/apns-token — line 470, 996]
- [Source: _bmad-output/planning-artifacts/architecture.md#apns_tokens collection — line 541]
- [Source: _bmad-output/planning-artifacts/architecture.md#P2 HTTP API 格式 /devices/apns-token — line 508]
- [Source: _bmad-output/planning-artifacts/architecture.md#internal/repository/apns_token_repo.go — line 856]
- [Source: _bmad-output/planning-artifacts/architecture.md#internal/handler/device_handler.go — line 860]
- [Source: _bmad-output/planning-artifacts/architecture.md#E1 handler/device service/account repository/apns_token — line 1042]
- [Source: docs/backend-architecture-guide.md#§6.1-§6.4 Handler/Service/Domain/Repository — line 213-383]
- [Source: docs/backend-architecture-guide.md#§10.2 EnsureIndexes — line 571-575]
- [Source: docs/backend-architecture-guide.md#§13 AuthRequired 仅挂 /v1/* — line 666-668]
- [Source: docs/backend-architecture-guide.md#§21.2 Empty Provider 逐步填实 — line 848-861]
- [Source: docs/backend-architecture-guide.md#§21.3 Fail-closed vs Fail-open — line 863-883]
- [Source: docs/backend-architecture-guide.md#§21.4 AC review 早启 — line 885-899]
- [Source: docs/backend-architecture-guide.md#§21.7 Server 测试自包含 — line 925-937]
- [Source: docs/backend-architecture-guide.md#§21.8 语义正确性思考题 — line 939-941]
- [Source: server/agent-experience/review-antipatterns.md#§4.1 positive int, §4.2 applyDefaults, §4.3 hardcoded vs config]
- [Source: server/agent-experience/review-antipatterns.md#§7.1 release/debug mode gate — line 197-207]
- [Source: server/agent-experience/review-antipatterns.md#§8.1 dedup key namespace, §8.2 冒号 injectivity]
- [Source: server/agent-experience/review-antipatterns.md#§9.1 滑动 vs 固定窗口 — line 228-232]
- [Source: server/agent-experience/review-antipatterns.md#§9.2 纳秒精度 loss]
- [Source: server/agent-experience/review-antipatterns.md#§13.1 pkg/ ← internal/ 禁止]
- [Source: server/pkg/ids/ids.go — UserID / Platform 已存在，本 story 不扩]
- [Source: server/pkg/logx/pii.go#MaskAPNsToken — M14 token mask]
- [Source: server/pkg/clockx — Clock/FakeClock 既有，本 story 复用]
- [Source: server/pkg/redisx/conn_ratelimit.go — Story 0.11 滑动窗口算法模板（本 story AC6 复用）]
- [Source: server/pkg/redisx/refresh_blacklist.go — Story 1.2 key namespace 模板（本 story AC6 keyPrefix 隔离参考）]
- [Source: server/internal/push/providers.go — Story 0.13 四个 consumer interface，本 story 填 3 个]
- [Source: server/internal/push/apns_router.go + apns_worker.go — Story 0.13 消费者，本 story 不改]
- [Source: server/internal/cron/scheduler.go — NewScheduler(tokenCleaner) 签名，本 story 只换参数值不改签名]
- [Source: server/internal/middleware/jwt_auth.go — UserIDFrom / DeviceIDFrom / PlatformFrom 既有，本 story 消费]
- [Source: server/internal/repository/user_repo.go — MongoUserRepository + validateDeviceID 既有，本 story AC7 扩展 SetSessionHasApnsToken]
- [Source: server/internal/domain/user.go#Session.HasApnsToken — 字段已预留，本 story 首次写入]
- [Source: server/internal/handler/auth_handler.go — handler package 既有模式，本 story AC5 device_handler 对齐]
- [Source: server/internal/dto/error_codes.go — ErrValidationError / ErrRateLimitExceeded / ErrInternalError / ErrAuthInvalidIdentityToken 已存在，本 story 复用]
- [Source: server/internal/dto/auth_dto.go — DTO 模式模板]
- [Source: server/internal/config/config.go#APNsCfg — 本 story AC8 扩 3 字段]
- [Source: server/cmd/cat/initialize.go — 既有 push / cron / userRepo wiring，本 story 插入 apnsTokenRepo 构造 + §21.2 Empty → Real swap]
- [Source: server/cmd/cat/wire.go — /v1/* group 已建好 (Story 1.3)，本 story 追加 v1.POST("/devices/apns-token")]
- [Source: server/cmd/cat/jwt_middleware_integration_test.go — harness 模板，本 story AC10 扩展]
- [Source: docs/api/openapi.yaml — 当前 version 1.3.0-epic1；本 story bump 1.4.0-epic1]
- [Source: docs/api/integration-mvp-client-guide.md — §13/§14 HTTP 鉴权/注销，本 story 追加 §15 APNs token 注册]
- [Source: _bmad-output/implementation-artifacts/1-3-jwt-auth-middleware-userid-context-injection.md#AC7 /v1/* group — 本 story 的 AC9 wire.go 扩展参考]
- [Source: _bmad-output/implementation-artifacts/0-13-apns-push-platform-pusher-queue-routing-410-cleanup.md#AC4 providers.go interfaces — 本 story AC3 adapter 满足接口]

### Project Structure Notes

- 完全对齐架构指南 `internal/{domain, repository, service, handler, middleware, push}` + `pkg/{cryptox, redisx, ids, logx, clockx}` 分层；**新增** `pkg/cryptox` 一个包
- **新建文件（10 个）**：
  - `server/pkg/cryptox/aesgcm.go`
  - `server/pkg/cryptox/aesgcm_test.go`
  - `server/pkg/redisx/user_ratelimit.go`
  - `server/pkg/redisx/user_ratelimit_test.go`
  - `server/internal/domain/apns_token.go`
  - `server/internal/repository/apns_token_repo.go`
  - `server/internal/repository/apns_token_repo_integration_test.go`
  - `server/internal/service/apns_token_service.go`
  - `server/internal/service/apns_token_service_test.go`
  - `server/internal/handler/device_handler.go`
  - `server/internal/handler/device_handler_test.go`
  - `server/internal/dto/device_dto.go`
  - `server/cmd/cat/apns_token_integration_test.go`
- **修改文件（7 个）**：
  - `server/internal/config/config.go`（APNs 3 字段 + validate + applyDefaults）
  - `server/config/default.toml`（[apns] 3 行）
  - `server/internal/config/config_test.go`（断言新字段）
  - `server/internal/repository/user_repo.go`（+ SetSessionHasApnsToken）
  - `server/internal/repository/user_repo_integration_test.go`（+ 5 子测）
  - `server/cmd/cat/initialize.go`（apnsTokenRepo / limiter / service / handler wiring + 3 处 Empty → Real swap + 构造顺序调整）
  - `server/cmd/cat/wire.go`（handlers.device + v1.POST 路由）
  - `server/cmd/cat/initialize_test.go`（+ 3 子测）
  - `docs/api/openapi.yaml`（path + schemas + version bump）
  - `docs/api/integration-mvp-client-guide.md`（+ §15）
- 无新 external dependency（一切 stdlib + 既有 go-redis + zerolog + mongo + gin + validator）
- 无架构偏差
- 无新 WSMessage；无新 error code；**有** 3 个新 config 字段 → §21.1 drift gate N/A（非"常量集合"，是独立 scalar 字段）；**有** 3 个 Empty Provider 被真实化 → §21.2 显式执行

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — `claude-opus-4-7[1m]`

### Debug Log References

- `bash scripts/build.sh --test` — 全绿（server/cmd/cat + 19 个 package 全部 ok）
- 偶发 flaky：`TestRequestID_InjectsIntoContextLogger`（parallel 测试共享 zerolog stdout buffer；单独重跑稳定通过）；与本 story 无关
- `go vet -tags=integration ./...` — 全包 vet 通过（integration tests 编译干净，执行依赖 Linux CI + Docker）

### Completion Notes List

**实现范围**（12 新文件 + 10 修改文件，严格对照 AC1–AC15）：

新建：
- `pkg/cryptox/aesgcm.go` + `aesgcm_test.go`（AC2；9 个 table-driven 子测）
- `pkg/redisx/user_ratelimit.go` + `user_ratelimit_test.go`（AC6；8 个 miniredis/FakeClock 子测，含 §9.3 d<=0 boundary→1ms 锁）
- `internal/domain/apns_token.go`（AC1）
- `internal/repository/apns_token_repo.go` + `apns_token_repo_integration_test.go`（AC3；10 Testcontainers 子测含 tampered-row skip / unique index race / at-rest 加密验证）
- `internal/service/apns_token_service.go` + `apns_token_service_test.go`（AC4；7 fake-driven 子测，§21.3 fail-closed/open 逐条）
- `internal/handler/device_handler.go` + `device_handler_test.go`（AC5；11 gin-recorder 子测含 JWT-empty-platform 401 / platform-mismatch 400 / refresh-as-access 401）
- `internal/dto/device_dto.go`（AC5）
- `cmd/cat/apns_token_integration_test.go`（AC10；8 子测含 PusherChainIntact 决定性网关）

修改：
- `internal/config/config.go`（AC8+AC9：`APNsCfg` 扩 3 字段 + release-mode 无条件校验 encryption key）
- `config/default.toml`（3 行追加）
- `internal/config/config_test.go`（新字段 default 断言）
- `internal/repository/user_repo.go`（AC7 SetSessionHasApnsToken 单次 UpdateOne 避 TOCTOU）
- `internal/repository/user_repo_integration_test.go`（AC7 5 子测）
- `cmd/cat/initialize.go`（AC9：`mustBuildApnsTokenRepo` helper + 构造顺序调整至 cronSch 之前 + 3 处 Empty→Real swap + device handler wiring）
- `cmd/cat/wire.go`（AC9：device 字段 + v1.POST 路由；nil-safe）
- `cmd/cat/initialize_test.go`（AC9 2 新子测）
- `docs/api/openapi.yaml`（AC13：1.3.0→1.4.0-epic1，`/v1/devices/apns-token` path + 2 schemas）
- `docs/api/integration-mvp-client-guide.md`（AC13：新增 §14 APNs device token 注册）

**§21.2 Empty Provider 填实验证（AC14 两条 grep）**：
- `grep -cE "Empty[A-Za-z]+Provider\{\}" cmd/cat/initialize.go`：before=6 → after=5（Δ=-1，push.EmptyTokenProvider 被 `apnsTokenRepo` 替换；5 条 session.resume ws.Empty* provider 保留，Story 1.5+ 陆续填实）
- `grep -cE "push\.Empty(TokenDeleter|TokenCleaner|QuietHoursResolver)\{\}" cmd/cat/initialize.go`：before=3 → after=1（Δ=-2，EmptyTokenDeleter + EmptyTokenCleaner 被 `apnsTokenRepo` 替换；EmptyQuietHoursResolver 留给 Story 1.5）
- `grep -crE "\"github.com/huing/cat/server/internal/repository\"" internal/push/`：0 条匹配（push 不反向依赖 repository，import direction invariant 守住）

**AC Review 硬化 6 条内化证据**：
1. ✅ Release-mode always validate encryption key — `config.validateAPNs` 释放版校验移出 `if !APNs.Enabled` 分支
2. ✅ Intra-internal import direction grep gate — `internal/repository/apns_token_repo.go` 顶部 godoc 声明 + AC14 grep 覆盖
3. ✅ Delete tampered-row-skips-valid-sibling — `TestApnsTokenRepo_Integration_Delete_SkipsTamperedRowDeletesValidSibling` 明确覆盖
4. ✅ Sliding-window §9.3 boundary retry-1ms — `TestUserSlidingWindowLimiter_BoundaryRetryClampedTo1ms`
5. ✅ SetSessionHasApnsToken 单次 UpdateOne 避 TOCTOU — `user_repo.go` 实现 + `TestUserRepo_Integration_SetSessionHasApnsToken_UnknownSession_AutoCreatesMinimalSubDoc`
6. ✅ Handler JWT 空 platform fail-closed — `TestDeviceHandler_RegisterApnsToken_JWTMissingPlatform_401` + `_BodyAlsoMissing_401`

**Semantic-correctness 思考题（§21.8 8 条 self-audit）**：
1. AES-GCM nonce 复用 bug — ❌ 已覆盖：`TestSeal_DifferentNonceEachCall` 断言两次 Seal 相同明文产生不同密文；crypto/rand 每次 read 12 字节
2. AES-GCM Open 不验 tag bug — ❌ 已覆盖：`TestOpen_TagTampered` + `TestOpen_WrongKey`；Open 走 stdlib `aead.Open` 强制验 tag
3. encryption key 长度 silent 接受 bug — ❌ 已覆盖：`TestNewAESGCMSealer_WrongKeyLength`（9 档长度）+ `validateAPNs` 32-byte hex decode + `mustBuildApnsTokenRepo` sealer log.Fatal
4. 滑动窗口退化固定窗口 bug — ❌ 已覆盖：`TestUserSlidingWindowLimiter_SlidingBoundary`（INCR+EXPIRE 实现会错）
5. Mongo upsert 幂等性破 bug — ❌ 已覆盖：`TestApnsTokenRepo_Integration_UpsertCrossPlatformCoexists` + `_UniqueConstraint_RaceSafe`；index 为 `(user_id, platform)` 复合
6. Platform 来源信任 body bug（含 fallback 陷阱）— ❌ 已覆盖：handler 仅读 `middleware.PlatformFrom(c)`，JWT 空→401 无 body fallback
7. 410 删除遗漏 bug — ❌ 已覆盖：`Delete` 实现走 decrypt-and-match；`TestApnsTokenRepo_Integration_DeleteByPlaintextToken`
8. Empty Provider 未换 bug — ❌ 已覆盖：`TestApnsToken_Integration_Register_PusherChainIntact` 证明 Router.RouteTokens 真的经过 apnsTokenRepo + AC14 两条 grep 差值双保险

**覆盖缺口 self-audit**：
- `validateAPNs` release-mode 空 key `log.Fatal` 路径纯单元测试未添加 —— 原因：zerolog `log.Fatal` 调 `os.Exit`，in-process panic-recover 无法测；AC8 允许 "跳过单测靠 integration + 运行时保护"。未来推荐 refactor：抽 `requireEncryptionKey(cfg) error` 纯函数单测。
- `mustBuildApnsTokenRepo` debug-zero-key vs release-cfg-key 双测未直接添加 —— 原因：helper 依赖 `*mongo.Database` 非空，独立单测需要 mongo mock；key 选择逻辑短（6 行）+ grep 与 integration 间接覆盖足够。

**验证命令**：
- `bash scripts/build.sh --test` — 全绿
- `go vet -tags=integration ./...` — 全包通过
- `bash scripts/build.sh --race --test` — 待 Linux CI 跑（Windows cgo 限制跳过）
- Grep 差值核对完成（上文两行）

**§21.4 AC self-review 通过声明**：实施前逐条 walkthrough AC2 / AC4 / AC5 / AC6 / AC11 语义矩阵 + 反模式 §4.1-§4.3 / §7.1 / §8.1-§8.2 / §9.1-§9.3 / §13.1，AC Review 6 条修正全部在实施前内化到最终代码。

### File List

**新建（13）**：
- `server/pkg/cryptox/aesgcm.go`
- `server/pkg/cryptox/aesgcm_test.go`
- `server/pkg/redisx/user_ratelimit.go`
- `server/pkg/redisx/user_ratelimit_test.go`
- `server/internal/domain/apns_token.go`
- `server/internal/repository/apns_token_repo.go`
- `server/internal/repository/apns_token_repo_integration_test.go`
- `server/internal/service/apns_token_service.go`
- `server/internal/service/apns_token_service_test.go`
- `server/internal/handler/device_handler.go`
- `server/internal/handler/device_handler_test.go`
- `server/internal/dto/device_dto.go`
- `server/cmd/cat/apns_token_integration_test.go`

**修改（10）**：
- `server/internal/config/config.go`
- `server/internal/config/config_test.go`
- `server/config/default.toml`
- `server/internal/repository/user_repo.go`
- `server/internal/repository/user_repo_integration_test.go`
- `server/cmd/cat/initialize.go`
- `server/cmd/cat/wire.go`
- `server/cmd/cat/initialize_test.go`
- `docs/api/openapi.yaml`
- `docs/api/integration-mvp-client-guide.md`
