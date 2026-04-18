# Story 0.13: APNs 推送平台（Pusher 接口 + 队列 + 路由 + 410 清理）

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a platform engineer,
I want a unified `Pusher` interface fronting a Redis-Streams-backed APNs queue with per-platform topic routing, exponential-backoff retry, DLQ, HTTP 410 token cleanup, and quiet-hours silent-push downgrade,
So that every future APNs consumer — Story 5.2 touch fallback (FR27) / Story 6.2 blindbox drop / Story 8.2 cold-start recall (FR44b) / Story 5.5 quiet-hours silencing (FR30) — calls one reliable push platform that already satisfies D3, D16, FR43, FR58, NFR-REL-8, NFR-COMP-3, NFR-SEC-7, NFR-SEC-9, NFR-INT-2.

## Acceptance Criteria

1. **AC1 — Pusher 消费方接口（`internal/push/pusher.go`, P2）**：

   - `Pusher` interface is the single entry-point consumed by every future service that sends APNs notifications (touch_service, blindbox_service, cold_start_recall_job, profile/account flows):
     ```go
     type Pusher interface {
         Enqueue(ctx context.Context, userID ids.UserID, p PushPayload) error
     }
     ```
   - `PushPayload` struct, **all fields exported** (service-layer constructs these directly):
     ```go
     type PushKind string
     const (
         PushKindAlert  PushKind = "alert"   // apns-push-type: alert; visible banner/sound
         PushKindSilent PushKind = "silent"  // apns-push-type: background; content-available=1
     )

     type PushPayload struct {
         Kind               PushKind // required; alert | silent
         Title              string   // optional for silent; required for alert (validator returns VALIDATION_ERROR if empty when Kind=alert)
         Body               string   // optional; empty body allowed for alerts with Title-only
         DeepLink           string   // optional; client routes on tap — e.g. "cat://touch?from=<userId>"
         RespectsQuietHours bool     // if true + receiver in quiet window → Kind coerced to silent at consume-time (AC8)
         IdempotencyKey     string   // optional; if non-empty, Enqueue SETNX apns:idem:{key} for 5-min dedupe (AC6)
     }
     ```
   - Interface lives in `internal/push/` **because every future business-layer service imports `internal/push`** — this package is the shared seam, not a WS-only or cron-only concern. Contrast with `ws.ResumeCacheInvalidator` (Story 0.12) which is owned by the WS layer because only WS-layer services revoke it. `Pusher` has fan-in from service, ws/handlers, and cron — co-locating the interface with the implementation package is the right shape (P2 allows interface + impl in the same package when the package IS the abstraction boundary, like `io.Reader` in `io`). The structure satisfies M1 (package = noun = "push").
   - **Constructor returns `*struct`** satisfying the interface via Go structural typing (same pattern as `RedisDedupStore` for `ws.DedupStore`). `Pusher` interface exists so tests can fake it, so services accept interface arguments — but the real constructor returns `*RedisStreamsPusher`.

2. **AC2 — Redis Streams helper（`pkg/redisx/stream.go` — 新建）**：

   - New package file exposing minimal stream primitives used by `push/` and future stream-based queues:
     ```go
     type StreamPusher struct { cmd redis.Cmdable; streamKey string }
     func NewStreamPusher(cmd redis.Cmdable, streamKey string) *StreamPusher
     func (s *StreamPusher) XAdd(ctx context.Context, values map[string]string) (string, error)

     type StreamConsumer struct {
         cmd       redis.Cmdable
         streamKey string
         group     string
         consumer  string
         block     time.Duration
         count     int64
     }
     func NewStreamConsumer(cmd redis.Cmdable, streamKey, group, consumer string, block time.Duration, count int64) *StreamConsumer
     func (c *StreamConsumer) EnsureGroup(ctx context.Context) error     // XGROUP CREATE MKSTREAM, BUSYGROUP tolerated
     func (c *StreamConsumer) Read(ctx context.Context) ([]redis.XMessage, error) // XREADGROUP BLOCK count
     func (c *StreamConsumer) Ack(ctx context.Context, id string) error  // XACK
     ```
   - `EnsureGroup` tolerates the `BUSYGROUP Consumer Group name already exists` error (startup idempotency across restarts and multi-replica): return `nil` if the error message contains `BUSYGROUP`; bubble up any other error (including connection failure).
   - No WithLock / dedup logic inside this file — those belong to consumers. This is deliberately a thin wrapper so future stream-based queues (push, notification recall, analytics) can all reuse it without accidental coupling to APNs semantics.
   - godoc lists D16 key-space neighbors and notes that stream keys (`apns:queue`, `apns:dlq`) coexist with `apns:retry` ZSET and `apns:idem:*` string keys — all prefixed under `apns:`.

3. **AC3 — ApnsSender 接口 + 真实 APNs 客户端（`internal/push/apns_sender.go` + `apns_client.go`）**：

   - `ApnsSender` interface (consumer-side, defined in `internal/push/` alongside the worker that consumes it — same package because worker-sender dependency is purely internal):
     ```go
     type ApnsSender interface {
         Send(ctx context.Context, n *apns2.Notification) (*apns2.Response, error)
     }
     ```
   - `apnsClient` struct wraps `*apns2.Client` (from `github.com/sideshow/apns2`) and implements `ApnsSender.Send` by delegating with `ctx` honoring: if `ctx.Err() != nil` before sending, return `ctx.Err()`; else call `Client.PushWithContext(ctx, n)` (apns2 v0.25+ supports context propagation — see AC14 for version pin).
   - Constructor `NewApnsClient(keyPath, keyID, teamID string, production bool) (*apnsClient, error)`:
     - Reads `.p8` private key from `keyPath` (path from `APNsCfg.KeyPath`; must be absolute, comes from env-bound TOML — G3 fix per epics line 703).
     - Builds `token.AuthToken` with `KeyID=keyID, TeamID=teamID`.
     - Selects `apns2.HostDevelopment` or `apns2.HostProduction` via `production` bool; production=`true` when `cfg.Server.Mode == "release"`, else Development. Log info line at construction: `{action: "apns_client_init", mode: production/dev, topicWatch, topicIphone, keyId}` — masks nothing (no token material in the log; keyId / teamId are not secrets).
     - Errors on: missing `.p8` file (`os.ReadFile` wrapped with `fmt.Errorf("load apns key: %w", err)`), invalid PEM, missing teamID/keyID (validation: empty strings → return error immediately). **Never log the private key.**
   - **Release-mode `.p8` sourcing**：production deployment mounts the `.p8` file into the container via Docker volume; `APNsCfg.KeyPath` points at the mount location. `config/default.toml` and `config/production.toml` must leave `key_path = ""` (no secrets checked in); `initialize()` skips APNs construction when `key_path == ""` and logs a warn that push is disabled — see AC15 for the full startup contract.

4. **AC4 — Token / quiet-hours / cleanup provider interfaces（`internal/push/providers.go` — 新建）**：

   - Four consumer-side interfaces, all defined here so Story 1.4 / 1.5 / future stories provide real implementations while Epic 0 ships Empty stubs (exact pattern as Story 0.12's 6 providers):
     ```go
     type TokenInfo struct {
         Platform    string // "watch" | "iphone"  (ids.Platform typed const — see AC4.note)
         DeviceToken string // unencrypted hex string (repository decrypts on read per Story 1.4 AC)
     }
     type TokenProvider interface {
         ListTokens(ctx context.Context, userID ids.UserID) ([]TokenInfo, error)
     }
     type TokenDeleter interface {
         // Delete the specific (userID, deviceToken) pair after APNs 410.
         // Missing record is a no-op (returns nil). Returns error only on Mongo I/O failure.
         Delete(ctx context.Context, userID ids.UserID, deviceToken string) error
     }
     type TokenCleaner interface {
         // Delete apns_tokens with updatedAt < cutoff. Returns count deleted.
         DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
     }
     type QuietHoursResolver interface {
         // Resolve whether userID is currently inside their local quiet window.
         // Missing user / missing timezone / missing quietHours → returns (false, nil) — fail-open
         // silencing (erring on the side of delivering the push rather than silencing it).
         // Story 1.5 real impl reads users.preferences.quietHours + users.timezone.
         Resolve(ctx context.Context, userID ids.UserID) (quiet bool, err error)
     }
     ```
   - Epic 0 Empty implementations in the same file (kept together for grep-discoverability; future stories will leave the interfaces in place and add real impls in `internal/repository/apns_token_repo.go` / `internal/repository/user_repo.go`):
     ```go
     type EmptyTokenProvider struct{}
     func (EmptyTokenProvider) ListTokens(context.Context, ids.UserID) ([]TokenInfo, error) { return nil, nil }
     type EmptyTokenDeleter struct{}
     func (EmptyTokenDeleter) Delete(context.Context, ids.UserID, string) error { return nil }
     type EmptyTokenCleaner struct{}
     func (EmptyTokenCleaner) DeleteExpired(context.Context, time.Time) (int64, error) { return 0, nil }
     type EmptyQuietHoursResolver struct{}
     func (EmptyQuietHoursResolver) Resolve(context.Context, ids.UserID) (bool, error) { return false, nil }
     ```
   - **`ids.Platform`**：`pkg/ids/ids.go` currently only has a `doc.go` placeholder (per file list). Add `type Platform string; const (PlatformWatch Platform = "watch"; PlatformIphone Platform = "iphone")` to `pkg/ids/ids.go` — this is the first real ids content; future stories (1.1 UserID, 1.4 ApnsToken) will append. `TokenInfo.Platform` is `string` (not `ids.Platform`) to keep the JSON/Mongo shape simple and avoid forcing Story 1.4 to change repo types.
   - **`ids.UserID`**：also does not yet exist. Add `type UserID string` to `pkg/ids/ids.go`. The Pusher interface signature accepts `ids.UserID` because (a) architecture §15.3 mandates typed IDs, (b) consumers (future `TouchService.SendTouch`) already hold `ids.UserID`, (c) adding it now is a 2-line change that prevents Story 1.1 having to rewrite every `Pusher.Enqueue(ctx, string(u), p)` call site. Internal use within `push/` passes `string(userID)` to Redis field writes.

5. **AC5 — APNs router (platform-aware topic selection) — `internal/push/apns_router.go`**：

   - `APNsRouter` struct persists per-platform topic bundles from config + a `TokenProvider`:
     ```go
     type APNsRouter struct {
         tokens      TokenProvider
         watchTopic  string
         iphoneTopic string
     }
     func NewAPNsRouter(tokens TokenProvider, watchTopic, iphoneTopic string) *APNsRouter
     ```
   - `(r *APNsRouter) RouteTokens(ctx, userID) ([]RoutedToken, error)` returns `[]RoutedToken{DeviceToken, Topic, Platform}`. Empty slice when user has no registered tokens (MVP stubs return no tokens — worker silently ACKs and moves on).
   - **Topic routing rule (FR58)**：`TokenInfo.Platform == "watch"` → `watchTopic`; `"iphone"` → `iphoneTopic`; any other value → zerolog warn `{action:"apns_route_unknown_platform", userId, platform}` + skip that token (do NOT route to the other bundle — unknown platform usually signals a corrupted row or future enum expansion; silent-skip prevents pushing to the wrong device class and is reversible once Story 1.4 validates at write-time). Still return the rest of the tokens if any are valid.
   - **MVP topic values**：`cfg.APNs.WatchTopic = "<bundleID>.watchkitapp"` (or the native watchOS app bundle — TBD per Apple config; `default.toml` ships empty string). `cfg.APNs.IphoneTopic = "<bundleID>"`. Worker rejects empty topics at Enqueue path (AC15 startup validation). Routing logic does not care what the exact string is — it just passes it to `apns2.Notification.Topic`.
   - godoc documents the invariant "one notification per (user, token) — never multicast a single `apns2.Notification` across tokens" (required because each Notification binds exactly one DeviceToken per APNs protocol).

6. **AC6 — `RedisStreamsPusher.Enqueue` 实现（`internal/push/pusher.go` 同文件内）**：

   - Construction:
     ```go
     func NewRedisStreamsPusher(
         stream *redisx.StreamPusher,
         idemCmd redis.Cmdable,
         clock clockx.Clock,
         idemTTL time.Duration,
     ) *RedisStreamsPusher
     ```
     - All params required; nil / non-positive → panic at construction (Epic 0 precedent — no "disabled" mode).
   - `Enqueue(ctx, userID, p)` flow:
     1. Validate `p.Kind` is `PushKindAlert | PushKindSilent` else return `dto.ErrValidationError.WithCause(...)`.
     2. If `p.Kind == PushKindAlert && p.Title == ""` → return `dto.ErrValidationError` (AC1 note).
     3. If `p.IdempotencyKey != ""` → `SET apns:idem:{key} "1" NX EX idemTTL`; on NX-false (duplicate suppressed), return nil + log info `{action:"apns_enqueue_idem_dedup", userId, idemKey}`. On Redis error, log warn and **continue to XADD** (fail-open for idempotency — duplicate push is user-visible annoyance, lost push is worse; see AC11 fail-open vs fail-closed classification).
     4. Marshal payload to JSON: `msgJSON, _ := json.Marshal(queueMessage{UserID, Payload: p, Attempt: 0, EnqueuedAtMs: clock.Now().UnixMilli()})` — define private `queueMessage` struct in same file.
     5. `XADD apns:queue * userId <id> msg <json> attempt 0` (3 fields; `msg` is the full queueMessage JSON; `userId` / `attempt` duplicated for Redis-CLI readability during ops debugging).
     6. Return XADD error (caller bubbles; service layer typically logs and continues since APNs is fire-and-forget relative to main business write).
   - **`Enqueue` is non-blocking and returns immediately after XADD** — no waiting for APNs send (D3, NFR-REL-5 "权威写必入 Mongo；推送异步不阻塞").
   - Log line on success: `info` level, `action:"apns_enqueue", userId, kind, idemKey (hash-masked — see M14/M13: IdempotencyKey can embed business identifiers like blindboxId, so mask via logx.MaskPII when non-empty; **not** logx.MaskAPNsToken — IdempotencyKey is not a device token), streamId` (the XADD-returned ID).

7. **AC7 — APNsWorker Runnable（`internal/push/apns_worker.go`）**：

   - `APNsWorker` implements `Runnable` (`Name()`, `Start(ctx)`, `Final(ctx)`), named `"apns_worker"`.
   - Constructor:
     ```go
     func NewAPNsWorker(
         cfg APNsWorkerConfig,
         streamCmd redis.Cmdable,  // XREADGROUP, XACK, ZADD, ZRANGE, XADD (retry/dlq)
         sender ApnsSender,
         router *APNsRouter,
         quiet QuietHoursResolver,
         deleter TokenDeleter,
         clock clockx.Clock,
     ) *APNsWorker

     type APNsWorkerConfig struct {
         InstanceID      string        // uuid from cmd/cat/initialize (shared with Locker)
         StreamKey       string        // cfg.APNs.StreamKey → "apns:queue"
         DLQKey          string        // "apns:dlq"
         RetryZSetKey    string        // "apns:retry"
         ConsumerGroup   string        // "apns_workers"
         WorkerCount     int           // cfg.APNs.WorkerCount
         ReadBlock       time.Duration // default 1s
         ReadCount       int64         // default 10
         RetryBackoffsMs []int         // default [1000, 3000, 9000]
         MaxAttempts     int           // default 3 (epics 3 retries + 1 initial = 3 total? See AC10 note)
     }
     ```
     - All nil/zero → panic per Epic 0 precedent.
   - `Start(ctx)`:
     1. `streamConsumer.EnsureGroup(ctx)` — creates `apns:queue` stream + `apns_workers` group; tolerates BUSYGROUP.
     2. Spawn `WorkerCount` goroutines, each running `w.loop(ctx, consumerName)` where `consumerName = cfg.InstanceID + "-" + index`.
     3. Spawn one retry-promoter goroutine running `w.promoteRetries(ctx)` on 100ms ticker.
     4. Return nil. If `EnsureGroup` fails, return the error — `App.Run` will cancel and exit (graceful-shutdown path).
   - `Final(ctx)`: triggered by cancellation of the parent ctx inside `Start`; wait for all spawned goroutines to drain (sync.WaitGroup Wait, bounded by a 5-second context — if the APNs sender has a long in-flight `Send`, we cut our losses at 5s and log a warn). Idempotent.
   - Worker `loop`:
     ```
     for ctx not done:
       msgs, err := consumer.Read(ctx)   // XREADGROUP BLOCK 1s COUNT 10
       if err == redis.Nil { continue }   // BLOCK timeout
       if err != nil { log error; sleep 100ms via clock-aware ticker; continue }
       for each m in msgs: w.handle(ctx, consumerName, m)
     ```
   - Each handle() flow is AC8. All goroutines respond to `ctx.Done()` before blocking reads return — `XREADGROUP BLOCK 1s` bounds worst-case shutdown lag.

8. **AC8 — Worker handle flow (the core state machine per message)**：

   ```
   handle(ctx, consumerName, m):
     qm, err := decodeQueueMessage(m.Values["msg"])
     if err != nil { ack + XADD dlq {reason:"decode_error"}; return }

     // Quiet-hours downgrade happens at consume time (user may have flipped quiet since Enqueue).
     if qm.Payload.RespectsQuietHours {
       quiet, qerr := w.quiet.Resolve(ctx, qm.UserID)
       if qerr != nil { log warn "quiet_resolve_error"; treat as false (fail-open) }
       if quiet { qm.Payload.Kind = PushKindSilent }
     }

     tokens, err := w.router.RouteTokens(ctx, qm.UserID)
     if err != nil { retryOrDLQ(qm, err, "route_error"); return }
     if len(tokens) == 0 { ack + log info {action:"apns_no_tokens", userId}; return }  // user has no registered devices yet

     allFatal := true
     anyRetryable := false
     for each rt in tokens:
       n := buildNotification(rt, qm.Payload)  // sets Topic=rt.Topic, DeviceToken=rt.DeviceToken, PushType, APS payload
       resp, err := w.sender.Send(ctx, n)
       if err != nil: anyRetryable = true; allFatal = false; continue  // transport error → retry eligible
       switch resp.StatusCode:
         case 200:
           allFatal = false
           log info {action:"apns_send_ok", userId, platform, maskedToken}
         case 410:
           // Unregistered / BadDeviceToken — delete the token and DO NOT retry this msg against this token.
           _ = w.deleter.Delete(ctx, qm.UserID, rt.DeviceToken)
           log info {action:"apns_token_410_deleted", userId, platform, reason: resp.Reason}
           // 410 alone does not mark the whole message retryable — other tokens may still succeed.
         case 400, 403, 404, 413:
           // Permanent payload / auth errors — do not retry. Log error. Continue to other tokens.
           log error {action:"apns_send_fatal", userId, statusCode, reason}
         case 429, 500, 503:
           anyRetryable = true; allFatal = false
           log warn {action:"apns_send_retryable", userId, statusCode, reason}
         default:
           anyRetryable = true; allFatal = false
           log warn {action:"apns_send_unknown_status", userId, statusCode, reason}

     if anyRetryable && qm.Attempt < cfg.MaxAttempts-1:
       scheduleRetry(ctx, qm)   // ZADD apns:retry with dueAt = clock.Now()+backoff[qm.Attempt], XACK
     else if anyRetryable:      // exhausted
       xaddDLQ(ctx, qm, "retries_exhausted"); XACK
     else:
       XACK  // all tokens either succeeded or permanent-failed; retrying won't help
   ```

   - `buildNotification(rt, payload)`:
     - `Topic = rt.Topic`
     - `DeviceToken = rt.DeviceToken`
     - `Priority = apns2.PriorityHigh` when Kind=alert, `apns2.PriorityLow` when Kind=silent
     - `PushType = apns2.PushTypeAlert` when Kind=alert, `apns2.PushTypeBackground` when Kind=silent (NFR-COMP-3 — background push MUST carry PushType=background, not Alert with sound=0, else APNs rejects)
     - `Expiration = clock.Now().Add(1 * time.Hour)` (APNs will discard the notification after 1h — reasonable for touch/blindbox/recall which are time-sensitive)
     - APS payload built with `payload.NewPayload()`:
       - Kind=alert: `.AlertTitle(Title).AlertBody(Body).Sound("default")`. If `DeepLink != ""`, `.Custom("deepLink", DeepLink)`.
       - Kind=silent: `.ContentAvailable()`. No alert / sound / badge fields (NFR-COMP-3).
   - `scheduleRetry(ctx, qm)`:
     1. Increment `qm.Attempt`.
     2. `dueAtMs := clock.Now().UnixMilli() + cfg.RetryBackoffsMs[qm.Attempt-1]` (Attempt=1 → first retry uses 1000ms, Attempt=2 → 3000ms, Attempt=3 → 9000ms).
     3. Re-marshal qm to JSON.
     4. `ZADD apns:retry dueAtMs msgJSON` + `XACK apns:queue apns_workers m.ID` in a pipeline.
     5. Log info `{action:"apns_retry_scheduled", userId, attempt, dueAtMs}`.

9. **AC9 — Retry promoter goroutine**：

   - `promoteRetries(ctx)` loop, one-shot per worker:
     ```
     ticker := time.NewTicker(100ms)  // real wall-clock ticker is fine; clock.Now() reads drive the scoring
     defer ticker.Stop()
     for ctx not done:
       select { case <-ctx.Done(): return; case <-ticker.C: }
       nowMs := clock.Now().UnixMilli()
       // ZRANGEBYSCORE apns:retry 0 <nowMs> LIMIT 0 100 — members whose dueAt passed
       due, err := redisCmd.ZRangeByScore(ctx, retryKey, &redis.ZRangeBy{Min:"0", Max:strconv.FormatInt(nowMs, 10), Count: 100}).Result()
       if err != nil { log warn; continue }
       for each msgJSON in due:
         pipe:
           ZREM apns:retry msgJSON           // exactly-once promotion
           XADD apns:queue * userId <id> msg <json> attempt <n>
         exec; log info {action:"apns_retry_promoted", userId, attempt}
     ```
   - ZREM before XADD within a pipeline guarantees at-most-once re-XADD across the retry ZSET — if pipeline fails mid-execution, either ZREM didn't happen (retry stays in ZSET and will be re-promoted next tick) or XADD didn't happen (retry is gone but no stream entry — acceptable because the worker will eventually give up when retries stop being promoted; see Dev Notes for the "lost retry" failure mode analysis).
   - **ZREM-first vs ZPOPMIN rejected**：ZPOPMIN returns lowest-score members but gives us no control over the "score ≤ now" filter — we'd pop future-dated retries and have to put them back, which races. ZRANGEBYSCORE + ZREM keeps the filter and the removal atomic per-member at the cost of two commands. The pipeline makes it one round-trip.
   - Ticker uses real `time.NewTicker` (not Clock-driven) because it's a scheduling primitive, not a timestamp source — same separation P2/cron uses. Tests inject `clockx.FakeClock` to control `nowMs` and advance the clock; the 100ms tick is fine in tests (tests can `time.Sleep(150ms)` or use shorter tick injection — see AC13).

10. **AC10 — Exponential backoff exhaustion & DLQ**：

    - `cfg.MaxAttempts = 3` means total send attempts = 3 (initial + 2 retries), but epics says "失败重试指数退避 3 次（基准 1s / 3s / 9s）". Interpret epics literally: initial (attempt=0) + 3 retries (attempts 1/2/3) = 4 total send invocations. Use `MaxAttempts = 4` default with backoff `[1000, 3000, 9000]` (3 entries, used for attempts 1→3; attempt 0 is the initial non-retry). The scheduleRetry branch checks `qm.Attempt < cfg.MaxAttempts - 1` ⇔ `qm.Attempt < 3` before reincrementing; when Attempt=3 re-increments to 4 ⇒ `MaxAttempts` reached → DLQ. Config key name: `retry_backoff_ms = [1000, 3000, 9000]` (TOML array). Empty array → applyDefaults fills defaults.
    - `xaddDLQ(ctx, qm, reason)`: `XADD apns:dlq * userId <id> msg <json> reason <str> attempts <n> failedAtMs <nowMs>`. Log error `{action:"apns_dlq", userId, reason, attempts}`.
    - DLQ is **not auto-drained**; ops tool `tools/apns_dlq_dump` (future, not this story) will stream/inspect. DLQ stream key has no XGROUP — it's append-only audit.

11. **AC11 — Fail-open vs fail-closed policy (vs Story 0.11/0.12 precedents)**：

    | Concern | Mode | Reason |
    |---|---|---|
    | `Enqueue` XADD error | **fail-closed** (return err to caller) | The caller's business write already committed; surfacing the error lets the service decide whether to double-write, log-and-continue, or rollback. We do NOT swallow it here — that would violate "backup 掩盖核心风险" (user feedback). |
    | `Enqueue` idempotency SETNX Redis error | **fail-open** (continue to XADD) | A duplicate push is user-annoying; a lost push is user-breaking. See AC6.3. |
    | Worker `TokenProvider.ListTokens` error | **retry via ZSET** | Mongo transient — retry gets another shot. Terminal after MaxAttempts → DLQ. |
    | Worker `QuietHoursResolver` error | **fail-open to loud push** | User will hear the notification rather than lose it; better than false-silencing. Matches AC4 interface godoc. |
    | Worker `APNs.Send` 410 | **delete token + continue** (not retry) | FR43 mandate; other tokens in the same message may still succeed. |
    | Worker `APNs.Send` 4xx non-410 | **no retry** (log error) | Permanent. Per-token basis — other tokens continue. |
    | Worker `APNs.Send` 5xx / transport | **retry** | Transient. |
    | Cron `TokenCleaner.DeleteExpired` error | **log + skip** | Cron is idempotent; next 24h run retries. |

    - Dev Notes must reference this table and compare: Story 0.11 blacklist/ratelimit are safety gates → fail-closed. Story 0.12 resume cache is a performance gate → fail-open. **Story 0.13 is a delivery pipeline** — different parts are safety (XADD to service) vs performance (quiet resolver); this table captures each classification.

12. **AC12 — Cron job `apns_token_cleanup_job` (`internal/cron/apns_token_cleanup_job.go` — 新建)**：

    - Signature: `func apnsTokenCleanupJob(ctx context.Context, cleaner push.TokenCleaner, clock clockx.Clock) error`.
    - Logic: `cutoff := clock.Now().AddDate(0, 0, -30)`; `count, err := cleaner.DeleteExpired(ctx, cutoff); log info {action:"apns_token_cleanup", deletedCount, cutoff}; return err`.
    - Registered in `internal/cron/scheduler.go` `registerJobs()` via `s.addLockedJob("@daily", "apns_token_cleanup", ...)` — spec `@daily` (midnight UTC). `Scheduler` constructor is extended with a `push.TokenCleaner` dep:
      ```go
      func NewScheduler(locker *redisx.Locker, redisCmd redis.Cmdable, clock clockx.Clock, tokenCleaner push.TokenCleaner) *Scheduler
      ```
      `initialize.go` passes `push.EmptyTokenCleaner{}` in Epic 0. Story 1.4 swaps in the real repository.
    - **Backward compatibility**：`NewScheduler` gains a 4th parameter; `internal/cron/scheduler_test.go` and `setupTestScheduler` are updated to pass `push.EmptyTokenCleaner{}`. All existing cron tests continue to pass. If you prefer a struct-param constructor for forward-compat, refactor at your discretion (match 0.12 `ResumeProviders` precedent for >=3 param constructors) — but Scheduler is pkg-internal and 4 positional args is still readable.
    - Unit test `internal/cron/apns_token_cleanup_job_test.go`:
      - FakeClock fixed at 2026-04-18 → cutoff assertion: `time.Date(2026,3,19,...)`.
      - Fake cleaner tracks calls; asserts cutoff is exactly 30-days-prior.
      - Error path: fake returns error → job returns error (scheduler logs and continues — AC11 "log + skip").

13. **AC13 — Configuration（`internal/config/config.go`）**：

    - Extend `APNsCfg`:
      ```go
      type APNsCfg struct {
          KeyID           string   `toml:"key_id"`
          TeamID          string   `toml:"team_id"`
          BundleID        string   `toml:"bundle_id"`
          KeyPath         string   `toml:"key_path"`
          WatchTopic      string   `toml:"watch_topic"`       // empty in default.toml; operator provides per environment
          IphoneTopic     string   `toml:"iphone_topic"`
          StreamKey       string   `toml:"stream_key"`        // "apns:queue"
          DLQKey          string   `toml:"dlq_key"`           // "apns:dlq"
          RetryZSetKey    string   `toml:"retry_zset_key"`    // "apns:retry"
          ConsumerGroup   string   `toml:"consumer_group"`    // "apns_workers"
          WorkerCount     int      `toml:"worker_count"`      // 2
          IdemTTLSec      int      `toml:"idem_ttl_sec"`      // 300
          ReadBlockMs     int      `toml:"read_block_ms"`     // 1000
          ReadCount       int      `toml:"read_count"`        // 10
          RetryBackoffsMs []int    `toml:"retry_backoffs_ms"` // [1000, 3000, 9000]
          MaxAttempts     int      `toml:"max_attempts"`      // 4 (initial + 3 retries)
          TokenExpiryDays int      `toml:"token_expiry_days"` // 30 — cron cutoff window
          Enabled         bool     `toml:"enabled"`           // false in default/test; true in production
      }
      ```
    - `applyDefaults` adds per-field zero-value fill (same `applyDefaults` pattern Story 0.11/0.12 established to keep override configs booting when fields are added):
      ```go
      if c.APNs.StreamKey == "" { c.APNs.StreamKey = "apns:queue" }
      if c.APNs.DLQKey == "" { c.APNs.DLQKey = "apns:dlq" }
      if c.APNs.RetryZSetKey == "" { c.APNs.RetryZSetKey = "apns:retry" }
      if c.APNs.ConsumerGroup == "" { c.APNs.ConsumerGroup = "apns_workers" }
      if c.APNs.WorkerCount == 0 { c.APNs.WorkerCount = 2 }
      if c.APNs.IdemTTLSec == 0 { c.APNs.IdemTTLSec = 300 }
      if c.APNs.ReadBlockMs == 0 { c.APNs.ReadBlockMs = 1000 }
      if c.APNs.ReadCount == 0 { c.APNs.ReadCount = 10 }
      if len(c.APNs.RetryBackoffsMs) == 0 { c.APNs.RetryBackoffsMs = []int{1000, 3000, 9000} }
      if c.APNs.MaxAttempts == 0 { c.APNs.MaxAttempts = 4 }
      if c.APNs.TokenExpiryDays == 0 { c.APNs.TokenExpiryDays = 30 }
      ```
    - `mustValidate`:
      - If `c.APNs.Enabled == true`: require `KeyPath != ""`, `KeyID != ""`, `TeamID != ""`, `WatchTopic != ""`, `IphoneTopic != ""` else `log.Fatal` (fail-fast on incomplete production config).
      - Always validate: `WorkerCount > 0`, `IdemTTLSec > 0`, `ReadBlockMs > 0`, `ReadCount > 0`, `len(RetryBackoffsMs) > 0`, `MaxAttempts > 0`, `TokenExpiryDays > 0`. **Explicit 0/-1 is forbidden** — users who want to disable push set `enabled = false`; they should not try to disable via 0 worker count (see 0.11/0.12 precedent for this validation philosophy).
    - `config/default.toml` `[apns]` section rewritten:
      ```toml
      [apns]
      key_id = ""
      team_id = ""
      bundle_id = ""
      key_path = ""
      watch_topic = ""
      iphone_topic = ""
      stream_key = "apns:queue"
      dlq_key = "apns:dlq"
      retry_zset_key = "apns:retry"
      consumer_group = "apns_workers"
      worker_count = 2
      idem_ttl_sec = 300
      read_block_ms = 1000
      read_count = 10
      retry_backoffs_ms = [1000, 3000, 9000]
      max_attempts = 4
      token_expiry_days = 30
      enabled = false
      ```
    - `internal/config/config_test.go`:
      - `TestMustLoad_ValidConfig` — add assertions for new `APNsCfg` fields.
      - `TestMustLoad_OverrideWithoutWSSection` (analogous) — assert applyDefaults populates APNs field defaults when `[apns]` omitted entirely.
      - New `TestMustLoad_APNsEnabledRequiresKeyPath` — load config with `enabled = true` and empty `key_path` → expect `log.Fatal` (use `gotestutil` or capture via subprocess — **or** simpler: leave mustValidate assertions to be exercised by `bash scripts/build.sh --test` running real config files in CI; write a regular test that constructs a `Config{}` and calls `c.mustValidate()` — but mustValidate calls log.Fatal which kills the test binary. Dev: extract the check into `c.validate() error` that mustValidate wraps, then test `validate()` directly. Align with existing style: `mustValidate` is already `log.Fatal`-only; the cleanest approach is to test APNs field defaults + add a `Config.Validate() error` helper that both mustValidate and tests use — **but this is refactoring existing config.go**. Alternatively, skip the "enabled requires KeyPath" test and rely on runtime + ops review. Dev's call — if adding `Validate() error` doesn't cascade into touching 0.11/0.12 assertions, do it; otherwise document the gap in Completion Notes and rely on startup smoke test).

14. **AC14 — Dependency pin**：

    - Add `github.com/sideshow/apns2 v0.25.0` to `go.mod`. Verify this version supports `Client.PushWithContext` (v0.25+). If not available, fall back to `Client.Push(n)` inside a goroutine-bounded goroutine with `ctx.Done()` select — document the fallback in Completion Notes.
    - `go mod tidy` + `bash scripts/build.sh --test` must not regress any existing test. If `sideshow/apns2` pulls a new indirect dep (e.g. `golang.org/x/net` bump), that's fine.

15. **AC15 — Assembly in `cmd/cat/initialize.go`**：

    - New code lives after existing cron scheduler construction, before `wsHub` (push setup is a peer-level dependency):
      ```go
      // Push platform — APNs worker + Pusher facade.
      var pusher push.Pusher   // interface-typed so downstream accepts interfaces
      if cfg.APNs.Enabled {
          sender, err := push.NewApnsClient(
              cfg.APNs.KeyPath, cfg.APNs.KeyID, cfg.APNs.TeamID,
              cfg.Server.Mode == "release",
          )
          if err != nil {
              log.Fatal().Err(err).Msg("apns client init failed")
          }
          streamPusher := redisx.NewStreamPusher(redisCli.Cmdable(), cfg.APNs.StreamKey)
          pusher = push.NewRedisStreamsPusher(
              streamPusher, redisCli.Cmdable(), clk,
              time.Duration(cfg.APNs.IdemTTLSec)*time.Second,
          )
          tokenProvider := push.EmptyTokenProvider{}
          tokenDeleter := push.EmptyTokenDeleter{}
          quietResolver := push.EmptyQuietHoursResolver{}
          router := push.NewAPNsRouter(tokenProvider, cfg.APNs.WatchTopic, cfg.APNs.IphoneTopic)
          worker := push.NewAPNsWorker(push.APNsWorkerConfig{
              InstanceID:      locker.InstanceID(),
              StreamKey:       cfg.APNs.StreamKey,
              DLQKey:          cfg.APNs.DLQKey,
              RetryZSetKey:    cfg.APNs.RetryZSetKey,
              ConsumerGroup:   cfg.APNs.ConsumerGroup,
              WorkerCount:     cfg.APNs.WorkerCount,
              ReadBlock:       time.Duration(cfg.APNs.ReadBlockMs)*time.Millisecond,
              ReadCount:       int64(cfg.APNs.ReadCount),
              RetryBackoffsMs: cfg.APNs.RetryBackoffsMs,
              MaxAttempts:     cfg.APNs.MaxAttempts,
          }, redisCli.Cmdable(), sender, router, quietResolver, tokenDeleter, clk)
          // Append worker to App.runs so graceful shutdown stops it after WS hub.
          // (Or pass to NewApp — see exact integration in app.go.)
      } else {
          pusher = push.NoopPusher{}   // no-op impl satisfying Pusher; safe to hand to future services
          log.Info().Msg("apns disabled (cfg.apns.enabled=false): push notifications will not be sent")
      }
      _ = pusher  // Epic 0: no service consumes yet; future Story 5.2/6.2/8.2/1.5 will.
      ```
    - Cron scheduler construction gains `push.TokenCleaner`:
      ```go
      cronSch := cron.NewScheduler(locker, redisCli.Cmdable(), clk, push.EmptyTokenCleaner{})
      ```
    - `push.NoopPusher` is added to `internal/push/pusher.go`:
      ```go
      type NoopPusher struct{}
      func (NoopPusher) Enqueue(context.Context, ids.UserID, PushPayload) error { return nil }
      ```
      Justification: Epic 0 `pusher` variable has no consumer; a nil interface is dangerous. `NoopPusher` gives us an always-safe placeholder for debug/test configs with `enabled=false`.
    - **Worker wiring into App.runs**: extend `cmd/cat/app.go` only if needed (currently `NewApp(mongo, redis, cron, wsHub, http)` — add worker positionally between cron and wsHub so `Final` order is HTTP → wsHub → worker → cron → redis → mongo, matching architecture §Graceful Shutdown 顺序 line 218: "HTTP Shutdown → WS Hub Stop → WS 现有连接发 close frame → cron.Stop → APNs worker 处理完"). Actually re-read: shutdown order specifies **cron before APNs worker**. So App.runs forward order is `mongo, redis, cron, wsHub, worker, http` — Final is reverse: http → worker → wsHub → cron → redis → mongo. That matches the spec.

16. **AC16 — Tests**：

    **Unit tests (each with `t.Parallel()`, miniredis-driven where possible):**

    a) `internal/push/pusher_test.go`:
       - `TestEnqueue_XAddsToStream` — ensures XADD happens with correct fields.
       - `TestEnqueue_IdempotencyKeySkipsDuplicate` — second Enqueue with same key returns nil + no second XADD (assert stream length = 1 via miniredis `mr.Stream(key)`).
       - `TestEnqueue_IdempotencySetnxError_FallsThrough` — fake Redis returning error on SET NX → XADD still happens (fail-open).
       - `TestEnqueue_AlertWithoutTitle_ValidationError` — `PushPayload{Kind: PushKindAlert, Title: ""}` → returns `VALIDATION_ERROR`.
       - `TestEnqueue_InvalidKind_ValidationError`.
       - `TestEnqueue_NilConstructorArgs_Panic` (table-driven).

    b) `internal/push/apns_worker_test.go`:
       - `TestHandle_AllTokensSucceed_Acks` — fake sender returns 200 for all tokens → XACK called, no retry, no DLQ.
       - `TestHandle_NoTokens_AcksAndLogs`.
       - `TestHandle_410Response_DeletesTokenAndAcks` — fake sender returns `{StatusCode: 410, Reason: "Unregistered"}` → deleter.Delete called with (userID, token) → XACK.
       - `TestHandle_500Response_SchedulesRetry` — retry ZSET gains entry; XACK on queue.
       - `TestHandle_RetryBackoffCorrect` — 1st attempt fails → dueAt = now+1000ms; 2nd → now+3000ms; 3rd → now+9000ms. Uses FakeClock for deterministic dueAtMs.
       - `TestHandle_MaxAttemptsExceeded_DLQ` — attempt=3 fails → xadd apns:dlq + XACK (no new retry).
       - `TestHandle_QuietHours_CoercesKindToSilent` — resolver returns true + original Kind=alert → built notification has PushType=background.
       - `TestHandle_QuietResolverError_FailsOpenToLoud` — resolver errs → Kind stays alert.
       - `TestHandle_UnknownPlatform_SkipsToken` — router logs warn and that token is not sent.
       - `TestHandle_DecodeError_DLQ` — invalid `msg` JSON in stream → DLQ with reason:"decode_error".
       - `TestHandle_4xxNon410_NoRetry` — 403 → no retry ZSET, XACK.

    c) `internal/push/apns_router_test.go`:
       - `TestRouteTokens_WatchGoesToWatchTopic`.
       - `TestRouteTokens_IphoneGoesToIphoneTopic`.
       - `TestRouteTokens_MixedPlatforms`.
       - `TestRouteTokens_UnknownPlatformSkipped`.
       - `TestRouteTokens_EmptyProviderReturnsEmpty`.

    d) `internal/push/apns_sender_test.go` (test the wrapper — real `apns2.Client` construction with a test key file):
       - `TestNewApnsClient_MissingKeyPath_Error`.
       - `TestNewApnsClient_InvalidPEM_Error`.
       - `TestNewApnsClient_ValidKey_Success` (fixture `.p8` in `testdata/`; generate via openssl once, commit under `internal/push/testdata/test_key.p8` — note this is an ECDSA P-256 key usable only for local/unit testing, not a real Apple key). **File content must be labeled non-production in a top `README` inside testdata/**.

    e) `pkg/redisx/stream_test.go`:
       - `TestStreamPusher_XAddReturnsID`.
       - `TestStreamConsumer_EnsureGroup_IdempotentOnBusygroup` — call twice, second returns nil.
       - `TestStreamConsumer_ReadAndAck_RoundTrip`.
       - `TestStreamConsumer_ReadBlockTimeout_ReturnsEmpty`.

    f) `internal/config/config_test.go`: APNs field assertions in existing `TestMustLoad_ValidConfig`; new `TestMustLoad_APNsDefaultsAppliedWhenSectionOmitted`.

    g) `internal/cron/apns_token_cleanup_job_test.go`:
       - `TestApnsTokenCleanupJob_CutoffIs30DaysPrior`.
       - `TestApnsTokenCleanupJob_CleanerError_Propagates`.

    **Integration tests (`//go:build integration`, miniredis for Redis, fake ApnsSender, NOT t.Parallel — M11):**

    h) `internal/push/apns_worker_integration_test.go`:
       - `TestIntegration_APNs_EndToEnd_Success` — Enqueue → start worker → fake sender returns 200 → assert XACK via miniredis XPENDING emptiness + sender.Calls == 1.
       - `TestIntegration_APNs_RetryPromotion_Succeeds` — fake returns 500 once then 200 → advance FakeClock by 1.1s → promoter rediscovers → second Send succeeds → final XACK.
       - `TestIntegration_APNs_MaxRetries_DLQ` — fake always returns 500 → after 1+3 attempts and clock advances totaling > 13s (1s+3s+9s), DLQ stream gets one entry.
       - `TestIntegration_APNs_410_DeletesToken` — fake sender returns 410 → fake deleter.Calls contains (userID, token); no retry.
       - `TestIntegration_APNs_IdempotencyDedupes` — two Enqueue with same idempKey within 5 min → only one DLQ entry (force fake sender to fail so message reaches DLQ) proves second enqueue never hit the stream.

    - Fake sender pattern (reused across tests):
      ```go
      type fakeSender struct {
          mu        sync.Mutex
          responses []*apns2.Response  // returned in order
          errors    []error             // matching-index; nil means use responses[i]
          calls     []*apns2.Notification
      }
      func (f *fakeSender) Send(ctx context.Context, n *apns2.Notification) (*apns2.Response, error) { ... }
      ```
    - `conn.Close()`/scheduler Final/worker Final called in test cleanup to avoid goroutine leaks.

17. **AC17 — Regression protection (Stories 0.1 – 0.12 unchanged)**：

    - `NewDispatcher` / `NewUpgradeHandler` / `NewSessionResumeHandler` signatures unchanged.
    - `NewScheduler` signature **adds** `push.TokenCleaner`; update `internal/cron/scheduler_test.go` and `cmd/cat/initialize.go` accordingly. All existing cron tests pass.
    - `cmd/cat/app.go` `NewApp` signature unchanged (runs slice still grows) — OR extended; whichever matches current positional pattern.
    - `bash scripts/build.sh --test` all green.
    - `go test -tags=integration ./internal/... ./pkg/...` all green.
    - `scripts/check_time_now.sh` clean: `internal/push/*.go` and `internal/cron/apns_token_cleanup_job.go` must use `clock.Now()` exclusively. Promoter goroutine's `time.NewTicker` is a scheduler primitive, not a timestamp — the check_time_now.sh regex targets `time.Now()` calls only, so ticker use is fine. Add inline comment justifying the ticker if reviewer flags it.
    - `internal/dto/error_codes_test.go::TestErrorCodesMd_ConsistentWithRegistry` unchanged (this story does **not** register new error codes — we reuse `ErrValidationError` + `ErrInternalError`).

18. **AC18 — Documentation**：

    - `docs/code-examples/pusher_usage_example.go` (new) — shows the standard service-layer pattern:
      ```go
      // ExamplePusherUsage shows how a service sends an APNs notification.
      // Future Story 5.2 TouchService will follow this pattern exactly.
      func ExamplePusherUsage(ctx context.Context, p push.Pusher) error {
          return p.Enqueue(ctx, "user-123", push.PushPayload{
              Kind:               push.PushKindAlert,
              Title:              "Alice",
              Body:               "sent you a touch",
              DeepLink:           "cat://touch?from=user-456",
              RespectsQuietHours: true,
              IdempotencyKey:     "touch_envelope_abc123",
          })
      }
      ```
    - `docs/error-codes.md` — no changes (no new codes).
    - godoc for `internal/push/pusher.go` file-level references FR27, FR30, FR43, FR44b, FR58, NFR-REL-8, NFR-COMP-3, NFR-SEC-7, NFR-SEC-9, D3, D16 + explains fail-open/fail-closed matrix from AC11.
    - godoc for `pkg/redisx/stream.go` references D3 + key space separation from event/lock/ratelimit/blacklist/presence/state/resume_cache/refresh_blacklist.

## Tasks / Subtasks

- [x] Task 1: `pkg/redisx/stream.go` stream primitives (AC: #2)
  - [x] 1.1 `StreamPusher` + `NewStreamPusher` + `XAdd` method
  - [x] 1.2 `StreamConsumer` + `NewStreamConsumer` + `EnsureGroup`/`Read`/`Ack` methods; BUSYGROUP-tolerant
  - [x] 1.3 godoc file-level references D3 + key space separation; single-segment key hygiene note
  - [x] 1.4 `pkg/redisx/stream_test.go` miniredis-driven (AC16.e) — 4 tests

- [x] Task 2: `pkg/ids/ids.go` minimal typed IDs (AC: #4)
  - [x] 2.1 `type UserID string` and `type Platform string` + `PlatformWatch`/`PlatformIphone` constants
  - [x] 2.2 file-level godoc explaining why `string` alias and not struct (architecture §15.3)
  - [x] 2.3 No test file needed (types-only file); `go vet ./pkg/ids/...` suffices

- [x] Task 3: `internal/push/providers.go` consumer interfaces + Empty impls (AC: #4)
  - [x] 3.1 `TokenInfo` struct
  - [x] 3.2 Four interfaces: `TokenProvider`, `TokenDeleter`, `TokenCleaner`, `QuietHoursResolver`
  - [x] 3.3 Four Empty* impls (no I/O, no state)
  - [x] 3.4 godoc per interface explains Epic 0 stub vs real-impl owner story (1.4 / 1.5 / etc.)

- [x] Task 4: `internal/push/apns_sender.go` + `apns_client.go` (AC: #3, #14)
  - [x] 4.1 Add `github.com/sideshow/apns2` to go.mod via `go get github.com/sideshow/apns2@v0.25.0`; `go mod tidy`
  - [x] 4.2 `ApnsSender` interface
  - [x] 4.3 `apnsClient` concrete impl wrapping `*apns2.Client`; constructor validates keyPath/keyID/teamID non-empty; loads `.p8`; picks dev/prod endpoint by `production bool`
  - [x] 4.4 Log line `{action: "apns_client_init", mode, topicWatch, topicIphone, keyId}` — no secret material
  - [x] 4.5 `internal/push/testdata/test_key.p8` fixture (ECDSA P-256; generated via openssl + pkcs8 wrap); README.md inside testdata/ labels as non-prod
  - [x] 4.6 `internal/push/apns_sender_test.go` — 3 tests (AC16.d)

- [x] Task 5: `internal/push/apns_router.go` (AC: #5)
  - [x] 5.1 `APNsRouter` struct + `NewAPNsRouter` constructor
  - [x] 5.2 `RoutedToken{DeviceToken, Topic, Platform}` struct
  - [x] 5.3 `RouteTokens(ctx, userID)` — platform → topic dispatch; unknown platform logs warn and skips
  - [x] 5.4 `internal/push/apns_router_test.go` — 5 tests (AC16.c) + 1 provider-error extra

- [x] Task 6: `internal/push/pusher.go` Pusher interface + PushPayload + RedisStreamsPusher + NoopPusher (AC: #1, #6, #11)
  - [x] 6.1 `Pusher` interface + `PushPayload` struct + `PushKind` typed string
  - [x] 6.2 `queueMessage` private struct + JSON marshal
  - [x] 6.3 `RedisStreamsPusher` + `NewRedisStreamsPusher` (panic on nil/zero args)
  - [x] 6.4 `Enqueue`: validate → SETNX idemKey → XADD (fail-open idem, fail-closed XADD)
  - [x] 6.5 `NoopPusher{}` for disabled-push mode
  - [x] 6.6 Log lines per M13/M14/P5 (camelCase, userId ok, idemKey via MaskPII when non-empty)
  - [x] 6.7 `internal/push/pusher_test.go` — 6 tests (AC16.a) + 1 NoopPusher extra

- [x] Task 7: `internal/push/apns_worker.go` (AC: #7, #8, #9, #10, #11)
  - [x] 7.1 `APNsWorker` struct + `APNsWorkerConfig` + `NewAPNsWorker` with nil/zero panic guards
  - [x] 7.2 Runnable methods: `Name()="apns_worker"`, `Start(ctx)`, `Final(ctx)`; ensure EnsureGroup idempotent; spawn WorkerCount loops + 1 promoter
  - [x] 7.3 `loop(ctx, consumerName)` — XREADGROUP loop with ctx cancellation
  - [x] 7.4 `handle(ctx, msg)` — full decode → quiet → route → per-token send → classify (200/410/4xx/5xx/transport) → retry/DLQ/ack
  - [x] 7.5 `buildNotification(rt, payload)` — sets Topic, DeviceToken, Priority, PushType, Expiration, APS payload per Kind
  - [x] 7.6 `scheduleRetry(ctx, qm)` — ZADD apns:retry (dueAt) + XACK in pipeline
  - [x] 7.7 `promoteRetries(ctx)` — ticker loop + exported `PromoteOnce` for tests; ZRANGEBYSCORE 0 nowMs → pipeline(ZREM + XADD) per due member
  - [x] 7.8 `xaddDLQ(ctx, qm, reason)` + ctx-aware sync.WaitGroup shutdown with 5s cutoff
  - [x] 7.9 zerolog structured per-handler outcome (`action: "apns_send_ok"` etc.); deviceToken masked via `logx.MaskAPNsToken`; userId never masked (opaque)
  - [x] 7.10 `internal/push/apns_worker_test.go` — 11 unit tests (AC16.b)
  - [x] 7.11 `internal/push/apns_worker_integration_test.go` `//go:build integration` — 5 integration tests (AC16.h)

- [x] Task 8: `internal/cron/apns_token_cleanup_job.go` (AC: #12)
  - [x] 8.1 Extend `NewScheduler` signature with `push.TokenCleaner`; update `scheduler.go` to call cleanup job via `addLockedJob`
  - [x] 8.2 `apnsTokenCleanupJob(ctx, cleaner, clock) error` — 30-day cutoff → DeleteExpired → log
  - [x] 8.3 Update `setupTestScheduler` + existing cron tests to pass `push.EmptyTokenCleaner{}`; all cron tests remain green
  - [x] 8.4 `internal/cron/apns_token_cleanup_job_test.go` — 2 tests (AC16.g)

- [x] Task 9: Config (`internal/config/config.go` + `config/default.toml`) (AC: #13)
  - [x] 9.1 Extend `APNsCfg` with 13 new fields
  - [x] 9.2 `applyDefaults` — 11 zero-value fills
  - [x] 9.3 `mustValidate` — enabled-gated required-field checks + positive-integer checks (extracted as `validateAPNs()` helper to keep the chain readable)
  - [x] 9.4 `config/default.toml` `[apns]` section rewritten
  - [x] 9.5 `internal/config/config_test.go` — extend valid-config test + new "defaults when section omitted" test

- [x] Task 10: Assembly (`cmd/cat/initialize.go`, `cmd/cat/app.go` if needed) (AC: #15)
  - [x] 10.1 Construct APNs sender/router/pusher/worker gated by `cfg.APNs.Enabled`; NoopPusher fallback when disabled
  - [x] 10.2 `pusher` variable typed `push.Pusher` (interface) — ready for future Story 5.2/6.2/8.2 consumption
  - [x] 10.3 Worker appended to `App.runs` between wsHub and http so Final order matches architecture §Graceful Shutdown line 218
  - [x] 10.4 NewScheduler call updated to pass `push.EmptyTokenCleaner{}`
  - [x] 10.5 Import statements, `initialize.go` kept under 200 lines

- [x] Task 11: Documentation (AC: #18)
  - [x] 11.1 `docs/code-examples/pusher_usage_example.go` — PushKindAlert sample + PushKindSilent sample
  - [x] 11.2 File-level godoc on every new file referencing FR / NFR / D / P / M IDs

- [x] Task 12: Regression + build (AC: #17)
  - [x] 12.1 `bash scripts/build.sh --test` all green
  - [x] 12.2 `go test -tags=integration ./internal/... ./pkg/...` — push + cron + config + ws integration all green (pre-existing `pkg/mongox` / `pkg/redisx` testcontainers tests require Docker on host; unchanged by this story)
  - [x] 12.3 `bash scripts/check_time_now.sh` clean
  - [x] 12.4 `TestErrorCodesMd_ConsistentWithRegistry` green (no new codes)
  - [x] 12.5 `go mod tidy` — diff shows `github.com/sideshow/apns2 v0.25.0` + its indirect `golang-jwt/jwt/v4 v4.4.1` added only

## Dev Notes

### Architecture Constraints (MANDATORY)

- **P2 Consumer-side interfaces** — `Pusher` / `ApnsSender` / `TokenProvider` / `TokenDeleter` / `TokenCleaner` / `QuietHoursResolver` all defined in `internal/push/` because **this package is the push abstraction**. This is the same co-location pattern `io.Reader`/`io.Writer` use — interface and primary impl live together because the package IS the abstraction, not because the impl consumes it. Contrast with Story 0.12 where `ResumeCache` lives in `internal/ws/` (WS consumer) while impl lives in `pkg/redisx/` (dependency direction matters). For push, services outside `internal/push/` are the consumers; they import `internal/push.Pusher` without pulling in Redis internals.
- **D3 APNs 队列化** — Redis Streams (`apns:queue`) + consumer group (`apns_workers`) + worker Runnable. Multi-replica ready: consumer group auto-partitions (NFR-SCALE-3). Epic 0 ships one replica but the wiring must not hard-code assumptions about single-worker (e.g. XREAD across goroutines sharing one consumer name is wrong — each goroutine gets `instanceID-<idx>` as unique consumer).
- **D16 Redis key space** — `apns:queue / apns:retry / apns:dlq / apns:idem:{key}` all prefixed under `apns:`; strictly separate from `resume_cache:{user} / event:* / event_result:* / lock:cron:* / ratelimit:ws:* / blacklist:device:* / presence:* / state:* / refresh_blacklist:*`. Prefix conflicts are a D16 violation.
- **P1 Mongo 规范** — this story does **not** create collections; Story 1.4 creates `apns_tokens`. The `TokenProvider`/`TokenDeleter`/`TokenCleaner` interfaces are the contract Story 1.4 implements.
- **P4 Error Classification** — `Enqueue` validation errors reuse `dto.ErrValidationError`; transient failures reuse `dto.ErrInternalError`; no new error code registered (this story follows the 0.12 discipline — don't blow up `error_codes.go`). Category=`CategoryClientError` for validation, `CategoryRetryable` for transport.
- **P5 Logging** — camelCase (`userId` / `platform` / `statusCode` / `reason` / `attempt` / `dueAtMs` / `idemKey` / `streamId`); `logx.Ctx(ctx)` for request-correlated lines; `log.Info` / `log.Error` (global) only for worker goroutine lines that run without a request context (which is the norm for worker — there is no upstream request for async consumption). Worker log lines still carry `userId` via the dequeued message.
- **M9 Clock interface** — every timestamp in `internal/push/` and `internal/cron/apns_token_cleanup_job.go` uses `clock.Now()`. `time.NewTicker` in the promoter is a scheduling primitive (not a timestamp source) — `scripts/check_time_now.sh` greps `time\.Now()` literals and will not flag `time.NewTicker(...)`. Add an inline comment at the ticker line `// real ticker is a scheduling primitive (not a timestamp) per M9 exemption — scoring uses clock.Now().UnixMilli()` for reviewer clarity.
- **M13 / M14 PII** — `userId` is opaque (log freely); `deviceToken` masked via `logx.MaskAPNsToken` everywhere (8-char prefix + `...`); `IdempotencyKey` masked via `logx.MaskPII` when non-empty (it can embed business IDs like `blindboxId`, which are not secrets but also not useful in plain audit trail). Never log: the `.p8` private key, the APS payload body when it might contain user content (MVP bodies are all server-side-constructed display strings, so OK for now — future i18n content story may revisit).
- **M15 per-connection rate limit** — WS layer only; unrelated to push. Document here so a reviewer doesn't ask "why no rate limit on Enqueue?" — push enqueue is called from authenticated server-side code paths (no client-driven enqueue), so the service layer's own rate limits (per-sender touch, cold-start cron throttle) are where limits land.
- **D10 Transaction boundaries** — `Pusher.Enqueue` is **not** wrapped in the caller's Mongo transaction. Reason: Redis and Mongo are separate systems; if the Mongo txn aborts after Enqueue succeeds we'd have an orphan push. The service layer pattern for this is "write Mongo inside txn, then enqueue after commit" (touch_service Story 5.2 will demonstrate). `IdempotencyKey` from the eventId ties the push to the committed envelope, so retry-safe re-enqueue is idempotent.
- **NFR-REL-5** — queue writes to Redis are persistent across container restarts (go-redis with real Redis — AOF/RDB). miniredis does NOT persist across `mr.Close()` but tests stop each miniredis cleanly so this is fine.
- **NFR-REL-8** — exponential backoff 3 retries, HTTP 410 cleanup — fully enforced by AC8/AC10.
- **NFR-SEC-9** — idempotency via 5-min `apns:idem:{key}` SETNX window; defaults 300s TTL.
- **NFR-COMP-3** — silent push uses `apns-push-type: background`, no alert fields (PayloadBuilder's `ContentAvailable()`); quiet-hours downgrade coerces alert→silent at consume time (receiver-local timezone).

### 关键实现细节

**Pusher 接口落地在 `internal/push/` 的理由（对比 Story 0.12 的 `ResumeCache` 在 `internal/ws/`）：**

Story 0.12 `ResumeCache` 的唯一消费者是 `internal/ws/handlers/session_resume.go`，所以接口定义在 `internal/ws/`（消费方包）而 impl 在 `pkg/redisx/` 保持单向依赖。对 Pusher，消费者横跨 `internal/service/touch` / `internal/service/blindbox` / `internal/cron/cold_start_recall`，把接口放到任何一个消费包都会让其他两个消费者反向 import（service → service）违反 P2。

解法：把接口放到 `internal/push/`（abstraction package 本身），让所有消费者 `import "github.com/huing/cat/server/internal/push"` 只拿到接口 + payload struct。impl 也在同包，但消费者不 import impl —— 他们 import 的是"push 抽象"。这与 Go 标准库 `io.Reader` / `io.Writer` 在 `io` 包的决策同构：包即抽象，不是某个消费者的"拥有"。

**为什么 `RespectsQuietHours` 在 payload 里（而不是所有 push 都默认尊重）：**

FR27 touch 降级和 FR44b 冷启动召回都需要尊重 quiet hours（NFR-COMP-3）。但未来可能有 "安全告警"、"账号异常"、"强合规通知" 等必须 alert 即使 quiet 的场景——如果把"尊重 quiet hours"写死在 Pusher 里，那些场景就只能绕过 Pusher 直接调 apns2，重复整个队列路由栈。让调用方在 payload 里显式表达"我尊重 quiet hours"是更正的封装。

**为什么 quiet hours resolver 在 consume 时（worker）而不是 enqueue 时（pusher）：**

Enqueue 到 worker 的时间差可能达到 repeat backoff 窗口（最坏 1+3+9 = 13s），期间用户可能调整 timezone / quietHours。在 consume 时读最新状态就避免了"发送时用户已进入 quiet 但推送还按 alert 发出"。Story 1.5 profile.update 会显式失效 `resume_cache:{userId}`——但 push 队列没有类似失效机制，只能在最后一刻读。

**为什么 retry 用 ZSET 而不是 time.Sleep：**

- **多副本就绪（NFR-SCALE-3）**：ZSET 存在 Redis，重启后 retry 可继续；`time.Sleep` 在进程内，重启丢。
- **不阻塞 worker**：一个消息 Sleep 9s 期间该 worker goroutine 无法处理其他消息；用 ZSET + 非阻塞 promoter 让 worker 始终可用。
- **可测试**：FakeClock advance + 单独调 `promoteRetries(ctx)` 或小 ticker（tests 用 50ms tick + 短等待）精确控制时序。
- **坏处**：ZSET 需要额外 ZRANGEBYSCORE/ZREM 操作，每 100ms 多 1 个 Redis 命令。对 MVP 量级（<100 push/s）可忽略。

**为什么 promoter ticker 用 `time.NewTicker` 而不走 clockx.Clock：**

`clockx.Clock` 接口只有 `Now()`，没有 `NewTicker` / `Sleep`——这是有意的（FakeClock 的推进是测试中的 explicit step，不是后台 ticker）。在 promoter 里用 `time.NewTicker` 但**用 `clock.Now().UnixMilli()` 做 scoring**，这样"什么时候 tick"是 real-clock 决定、"该不该 promote"是 injected-clock 决定。tests 把 clock 往前推 1.1s 然后 sleep(150ms) 等 ticker 自然 tick，promoter 会看到 `nowMs` 已经超过 dueAt 并 promote。若 100ms ticker 的抖动让测试 flake，整合测试可单独调 `worker.RunPromoterOnce(ctx)`（在 worker 上 expose 一个 test-only 方法）。

**为什么 `MaxAttempts = 4` 而不是 `3`：**

epics 原文 "失败重试指数退避 3 次（基准 1s / 3s / 9s）"。"3 次" 指**重试**次数（即 initial 失败之后的 3 次），所以总 Send 次数 = initial (attempt=0) + 3 retries (attempts 1/2/3) = 4。`MaxAttempts = 4`。backoff 数组长度 = 3（对应 attempts 1/2/3）。`scheduleRetry` 中 `qm.Attempt < cfg.MaxAttempts-1` 保证 attempt=3 是最后一次通过 retry path 被 send 的；下一次失败（attempt 增到 4）会触发 DLQ 分支。

**为什么 APNs 410 不触发 retry 但其他 5xx 触发：**

410 "Unregistered/BadDeviceToken" 是**永久**失败——token 已经失效，重试只会得到同样的 410。FR43 要求立即删除。其他 5xx / transport 错误是**瞬时**——APNs 服务抖动、网络闪断，retry 有成功可能。4xx non-410（400/403/413）也是**永久**（payload 格式错 / auth 错 / 超大），重试无用，记 error 不 retry。

**为什么每个 token 独立判断 retry 合格性：**

一个用户可能同时注册 Watch + iPhone token。Watch 返回 410（设备卸载）应立即删除 Watch token；iPhone 返回 200 应正常 ACK——这两件事在同一条队列消息里。`handle` 的 token 遍历逻辑：聚合"有 retryable 错的 token 存在"（anyRetryable=true）→ 整条消息重试；如果所有 token 要么成功要么永久失败，就 XACK。**但 retry 会重发给那些已经成功的 token**——这是 MVP 的合理权衡：用户收到重复通知（尤其在高并发）是 uncomfortable 但 recoverable；引入 per-token retry state 就需要 ZSET 里不是存 msg JSON 而是 (msg, token) 元组，工作量翻倍。用 `IdempotencyKey` 作为 APNs collapse-id（MVP 未启用，但未来可）可在 APNs 侧去重——留给 Story 5.2 评估。

**404 status code 归类（AC8 代码中未显式列）：**

APNs 404 通常是 bad topic 或 bad endpoint 配置——属 config 错误，永久，不 retry。AC8 switch 的 `4xx non-410` 默认分支已涵盖。

**`.p8` 私钥从环境变量读（G3 修复） vs TOML：**

epics AC "证书存放（G3 修复）：`.p8` 私钥路径从环境变量读取". 但 TOML 字段 `key_path` 是字符串——操作上等价："环境变量"意味着 Docker / k8s secret 挂载文件到容器路径，`key_path = "/etc/secrets/apns.p8"` 指向挂载点。config 文件本身**不含密钥内容**，只含路径；密钥内容靠部署系统管理。这符合"config/ 不含密钥"的约束。test 环境 `key_path = "internal/push/testdata/test_key.p8"`。

**Worker shutdown 与 in-flight APNs send：**

`apns2.Client.PushWithContext(ctx, ...)` 在 ctx 取消时会 abort HTTP/2 请求。`App.Run` 的 Final 链传入 5s 超时；`APNsWorker.Final` 等 sync.WaitGroup 但 bound 在 5s。因此单条 APNs 请求如果超过 5s 会被 abort，worker Final 直接 log warn 继续。这符合 architecture §Graceful Shutdown "预算 ≤ 30s" 中 APNs worker 的预算子项（line 218 "APNs worker 处理完"）。

### Source Tree — 要创建/修改的文件

**新建：**
- `server/internal/push/pusher.go` — Pusher interface + PushPayload + PushKind + queueMessage + RedisStreamsPusher + NoopPusher
- `server/internal/push/pusher_test.go` — unit tests (AC16.a, 6 tests)
- `server/internal/push/apns_sender.go` — ApnsSender interface
- `server/internal/push/apns_client.go` — apnsClient (sideshow/apns2 wrapper) + NewApnsClient
- `server/internal/push/apns_sender_test.go` — unit tests (AC16.d, 3 tests)
- `server/internal/push/apns_router.go` — APNsRouter + RoutedToken + RouteTokens
- `server/internal/push/apns_router_test.go` — unit tests (AC16.c, 5 tests)
- `server/internal/push/apns_worker.go` — APNsWorker Runnable + loop/handle/scheduleRetry/promoteRetries/xaddDLQ
- `server/internal/push/apns_worker_test.go` — unit tests (AC16.b, 11 tests)
- `server/internal/push/apns_worker_integration_test.go` — `//go:build integration` (AC16.h, 5 tests)
- `server/internal/push/providers.go` — TokenInfo + 4 interfaces + 4 Empty impls
- `server/internal/push/testdata/test_key.p8` — ECDSA P-256 test key (non-prod, openssl generated)
- `server/internal/push/testdata/README.md` — labels the key as non-prod
- `server/pkg/redisx/stream.go` — StreamPusher + StreamConsumer helpers
- `server/pkg/redisx/stream_test.go` — unit tests (AC16.e, 4 tests)
- `server/pkg/ids/ids.go` — UserID + Platform typed strings (replaces the placeholder `doc.go` content — leave `doc.go` as package comment only)
- `server/internal/cron/apns_token_cleanup_job.go` — cleanup cron job
- `server/internal/cron/apns_token_cleanup_job_test.go` — unit tests (AC16.g, 2 tests)
- `server/docs/code-examples/pusher_usage_example.go` — example service consumption pattern

**修改：**
- `server/internal/config/config.go` — extend APNsCfg (+13 fields) + applyDefaults (+11 lines) + mustValidate (+enabled-gated checks)
- `server/internal/config/config_test.go` — extend valid-config test + new "defaults on omit" test
- `server/config/default.toml` — rewrite `[apns]` section with 18 fields
- `server/internal/cron/scheduler.go` — NewScheduler gains `push.TokenCleaner` param + registerJobs adds `apns_token_cleanup`
- `server/internal/cron/scheduler_test.go` — setupTestScheduler passes EmptyTokenCleaner
- `server/cmd/cat/initialize.go` — push platform wiring (AC15); NewScheduler call updated
- `server/cmd/cat/app.go` — (if needed) insert worker into runs order; check NewApp signature
- `server/go.mod` + `server/go.sum` — `github.com/sideshow/apns2` v0.25.0

**不修改（回归保护）：**
- `internal/ws/*` — Stories 0.9 / 0.10 / 0.11 / 0.12 untouched
- `internal/dto/error.go` / `error_codes.go` / `error_codes_test.go` — no new codes
- `pkg/redisx/{blacklist,conn_ratelimit,dedup,locker,resume_cache,client}.go` — no changes
- `pkg/clockx/clock.go` — unchanged (ticker exemption documented in Dev Notes)
- `internal/config/config_test.go` existing tests retain green
- `internal/handler/health_handler.go` — no new dimension (cron_scheduler already covers apns_token_cleanup last tick)
- `docs/error-codes.md` — unchanged

### Testing Standards

- Unit tests at `t.Parallel()` with miniredis; shared fakes (`fakeSender` / `fakeTokenProvider` / `fakeDeleter` / `fakeQuiet` / `fakeCleaner`) in each test file (DRY inside the test file; cross-file sharing unnecessary).
- Integration tests `//go:build integration`, miniredis-backed (stream / zset / string all supported in miniredis v2.37+), NOT `t.Parallel()` per M11.
- `clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))` as the standard clock fixture across push + cron tests (matches Story 0.7 / 0.8 / 0.12 precedent).
- `apns2.Notification` / `apns2.Response` construction in test code uses the library types directly; fakeSender returns pre-built `*apns2.Response` objects.
- `testify`: `require.NoError` / `assert.Equal` / `assert.JSONEq` / `require.Panics`; `require.Len` for counting ZSET entries / stream XPENDING results.
- Graceful cleanup: every integration test `t.Cleanup(func(){ worker.Final(ctx) })` and miniredis close. Ensures no goroutine leaks across subsequent tests (0.12 precedent).

### Previous Story Intelligence (Stories 0.12 / 0.11 / 0.10 / 0.9 / 0.8 / 0.7)

- **Interface + impl in same package when package IS the abstraction (new shape for 0.13)** — all prior stories (0.9-0.12) put the interface in the consumer package (`internal/ws`) and impl in `pkg/redisx`. Story 0.13 is the first that breaks this pattern intentionally because push has fan-in from multiple consumer packages (service / cron) — co-locating Pusher in `internal/push/` is the correct analog to `io.Reader` in `io`. Dev Notes document this call so future reviewers don't mis-apply the 0.12 pattern.
- **applyDefaults backward compat (0.11 round 1, commit `6724015`)** — any new config section / field must be filled by `applyDefaults` so override configs that omit `[apns]` still boot. 11 defaults in this story; do NOT skip any.
- **Empty* providers as "off" state (0.12)** — Epic 0 ships 4 Empty* impls; Story 1.4/1.5 swap real impls in initialize.go. **Do NOT** allow nil interfaces in constructor args — Empty is the correct expression of "disabled".
- **"No backup mask core risk" user feedback** — applies to **safety gates** (like 0.11 blacklist). Does NOT apply uniformly to 0.13 — this pipeline mixes safety (Enqueue success signaling to caller) and performance (quiet resolver, idempotency fallback). AC11 table makes the classification explicit.
- **Config validation discipline (0.11 / 0.12)** — positive-integer fields fail on `<=0` (not just `<0`); operators disable features via explicit flags (`cfg.APNs.Enabled = false`), not via zeroing a numeric. `enabled = false` is the designed off-switch.
- **Dispatcher.Register vs RegisterDedup (0.10 / 0.12)** — N/A to push (no WS RPC registered in this story). Pusher is called server-internally.
- **Construction-site explicit DI (all prior stories)** — no DI framework; `initialize.go` explicit. Check line count ≤200 post-change; if exceeded, consider splitting a `init_push.go` helper called from `initialize()`.
- **P2 consumer interfaces pattern** — `TokenProvider` / `TokenDeleter` / `TokenCleaner` / `QuietHoursResolver` mirror 0.12's 6 Provider interfaces. Each future owner (Story 1.4 / 1.5) adds a real impl without touching push package structure.
- **miniredis v2.37** — supports Streams (XADD / XREADGROUP / XACK / XGROUP / XPENDING) and ZSET (ZADD / ZRANGEBYSCORE / ZREM / ZCARD). Verified via prior stories + miniredis release notes. No new version needed.
- **`logx.MaskAPNsToken` exists in `pkg/logx/pii.go`** (M14) — use it for every `deviceToken` log field. `logx.MaskPII` for any PII-class string (e.g. IdempotencyKey containing business IDs).
- **`clockx.FakeClock.Advance(d)` available** (0.7) — use in unit tests to assert exact retry dueAt math.

### Git Intelligence (最近 5 commits)

```
6057141 chore: mark Story 0.12 done — session.resume 缓存节流骨架...
6e65c4f fix(review): 0-12 round 1 — singleflight 合并并发 miss；release 模式不注册 session.resume
537d750 chore: mark Story 0.11 done — WS 建连频率限流 + 异常设备黑名单
963c737 fix(review): 0-11 round 2 — 边界 Retry-After 回退修复；ZSET score 改 Unix ms
6724015 fix(review): 0-11 round 1 — 配置 applyDefaults 兼容现有 override；rate limit 改真滑动窗口
```

Key observations:

- One "done" commit per story; review fixes follow as `fix(review): X-N round M` with specific technical summary. This story when complete must follow the same format.
- **ZSET scores as Unix ms** (0.11 round 2 via commit `963c737`) — the same convention applies to `apns:retry` ZSET in this story (dueAt in UnixMilli, not UnixNano / not RFC3339 string). Consistency matters for cross-story debugging.
- **Singleflight for concurrent misses** (0.12 round 1) — not directly applicable to push (every Enqueue is independent; IdempotencyKey handles duplicate suppression at its layer). But note the pattern: when multiple goroutines race on the same cache miss in 0.12, singleflight collapsed them. If the promoter ever sees the same retry message in multiple ticks (shouldn't — ZREM makes it exclusive), no singleflight needed.
- **Release-mode guards for incomplete implementations** (0.12 round 1) — `session.resume` is NOT registered in release mode because Empty* providers would corrupt client state. Analogous guard for 0.13: `cfg.APNs.Enabled = false` in default/test configs; release deploy must explicitly opt in with real `.p8` + topics + (once Story 1.4 lands) real TokenProvider. Until Story 1.4 provides real `TokenProvider`, even if `enabled=true` the worker will silently ACK every message (empty token list → no Send → ACK). This is explicit and documented; no operator surprise.
- **Infrastructure lands in `pkg/`; consumption lands in `internal/`** — stream.go is the thin Redis primitive in `pkg/redisx/`; push consumes it via composition.

### Latest Tech Information

- **`github.com/sideshow/apns2` v0.25.0** (Feb 2024, latest stable) — supports `Client.PushWithContext(ctx, n) (*Response, error)` for context cancellation. Import paths:
  ```go
  import (
      "github.com/sideshow/apns2"
      "github.com/sideshow/apns2/payload"
      "github.com/sideshow/apns2/token"
  )
  ```
  - Notification fields used: `Topic`, `DeviceToken`, `Payload`, `PushType` (`apns2.PushTypeAlert` / `apns2.PushTypeBackground`), `Priority` (`apns2.PriorityHigh` 10 / `apns2.PriorityLow` 5), `Expiration` (time.Time; APNs drops notification after this wall-clock instant).
  - Response fields: `StatusCode int`, `Reason string` (e.g. `"Unregistered"` for 410, `"PayloadTooLarge"` for 413), `ApnsID string`, `Timestamp apns2.Time` (for 410 responses, the last-valid time of the token).
  - Token-based auth: `token.AuthToken{AuthKey: ecdsaPrivKey, KeyID: keyID, TeamID: teamID}` where `AuthKey` is parsed via `token.AuthKeyFromFile(keyPath) (*ecdsa.PrivateKey, error)`.
  - Client construction: `apns2.NewTokenClient(authToken).Development()` or `.Production()`.
- **`github.com/redis/go-redis/v9` v9.18.0** (already depended) — `XAdd(ctx, &redis.XAddArgs{...}).Result() (string, error)`, `XReadGroup(ctx, &redis.XReadGroupArgs{...}).Result() ([]redis.XStream, error)`, `XAck(ctx, stream, group, id).Result()`, `XGroupCreateMkStream(ctx, stream, group, start).Err()`, `ZAdd`, `ZRangeByScore`, `ZRem`. All stable API.
- **`miniredis/v2` v2.37** (already depended) — Streams (`XADD` / `XREADGROUP` / `XACK` / `XGROUP CREATE MKSTREAM`) and ZSET commands supported. BUSYGROUP error matches real Redis behavior.
- **`github.com/robfig/cron/v3` v3.0.1** (already depended) — `"@daily"` is a supported cron spec alias = `"0 0 0 * * *"` (midnight UTC). No new feature needed.
- **No new external dependencies beyond `sideshow/apns2`**. `go mod tidy` diff should show exactly 1 module added (apns2) plus its transitive deps.

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.13 — AC 完整定义（lines 684-706）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.3 — Redis 客户端 + MustConnect（lines 484-502）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.7 — Clock interface（line 564-582）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.8 — Cron scheduler + withLock（lines 583-602）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.4 — APNs device token 注册 endpoint（lines 822-841）— 提供 TokenProvider/Deleter/Cleaner 实现]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.5 — quietHours / timezone（lines 843-866）— 提供 QuietHoursResolver 实现]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 5.2 — touch offline → APNs fallback（lines 1367-1397）— Pusher 首个消费者]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 5.5 — 跨时区免打扰（lines 1466-1496）— QuietHoursResolver 消费者]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 6.2 — Blindbox drop push（line 1589）— Pusher 消费者]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 8.2 — 冷启动召回推送（lines 1838-1882）— Pusher 消费者]
- [Source: _bmad-output/planning-artifacts/prd.md#FR27 — touch WS 在线 / APNs 离线（lines 72-73）]
- [Source: _bmad-output/planning-artifacts/prd.md#FR30 — quiet hours silent（line 76）]
- [Source: _bmad-output/planning-artifacts/prd.md#FR43 — APNs 410 token 清理（line 106）]
- [Source: _bmad-output/planning-artifacts/prd.md#FR44b — cold-start recall push（line 108）]
- [Source: _bmad-output/planning-artifacts/prd.md#FR58 — APNs 按 platform 路由（line 113）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-REL-8 — 指数退避 3 次 + 410 清理（line 920）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-COMP-3 — APNs Guidelines（line 940）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-7 — APNs token 加密存储（line 890）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-9 — 幂等性去重 5 分钟（line 892）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-INT-2 — sideshow/apns2 + token auth（line 950）]
- [Source: _bmad-output/planning-artifacts/prd.md#Redis Key Convention — D16 隔离（lines 547-554）]
- [Source: _bmad-output/planning-artifacts/architecture.md#D3 — APNs 推送队列化 OP-5（lines 308-317）]
- [Source: _bmad-output/planning-artifacts/architecture.md#External Integrations — sideshow/apns2 HTTP/2（line 1069）]
- [Source: _bmad-output/planning-artifacts/architecture.md#Source Tree — internal/push/ + pkg/redisx/stream.go（lines 905-930）]
- [Source: _bmad-output/planning-artifacts/architecture.md#Graceful Shutdown 顺序（line 218）— HTTP → wsHub → cron → APNs worker → redis → mongo]
- [Source: _bmad-output/planning-artifacts/architecture.md#M14 APNs token 脱敏（lines 683-685）]
- [Source: _bmad-output/planning-artifacts/architecture.md#M9 Clock interface（lines 647-660）]
- [Source: _bmad-output/planning-artifacts/architecture.md#P2 HTTP/WS API 格式（lines 506-526）]
- [Source: _bmad-output/planning-artifacts/architecture.md#P4 Error Classification（lines 536-559）]
- [Source: _bmad-output/planning-artifacts/architecture.md#P5 Logging camelCase（lines 561-576）]
- [Source: docs/backend-architecture-guide.md#§17.2 推送（lines 752-754）— Pusher 接口 + 不阻塞主业务]
- [Source: docs/backend-architecture-guide.md#§5 Runnable 生命周期 — Name/Start/Final 幂等]
- [Source: docs/backend-architecture-guide.md#§11 Redis 缓存 — 显式 TTL + key 集中]
- [Source: docs/backend-architecture-guide.md#§15.3 Typed IDs — UserID / Platform]
- [Source: docs/backend-architecture-guide.md#§18 P2 坏味道不抄 — 仓库返回结构体 / 接口消费方]
- [Source: internal/config/config.go — APNsCfg 现状 lines 70-75（4 字段，本 story 扩到 18 字段）]
- [Source: internal/config/config.go#applyDefaults lines 110-123 — 新增字段追加模式]
- [Source: internal/config/config.go#mustValidate lines 125-151 — 正整数校验模式]
- [Source: config/default.toml#[apns] lines 41-45 — 待重写]
- [Source: internal/cron/scheduler.go — NewScheduler 签名 / addLockedJob / registerJobs]
- [Source: internal/cron/heartbeat_tick_job.go — job 函数签名 + clock.Now() 模式]
- [Source: internal/cron/scheduler_test.go — setupTestScheduler 模式]
- [Source: pkg/redisx/dedup.go — pipeline 使用模式]
- [Source: pkg/redisx/conn_ratelimit.go — ZSET 操作 + clock 注入]
- [Source: pkg/redisx/resume_cache.go — key helper + pipeline HSET + godoc key space 模式]
- [Source: pkg/redisx/locker.go — Lua CAS + SETNX 模式（apns:idem 参考）]
- [Source: pkg/clockx/clock.go — RealClock / FakeClock / Advance API]
- [Source: pkg/logx/pii.go — MaskAPNsToken / MaskPII（M14 / M13）]
- [Source: pkg/logx/logx.go — logx.Ctx(ctx) 继承 requestId]
- [Source: internal/dto/error_codes.go — ErrValidationError / ErrInternalError 复用]
- [Source: internal/dto/error.go — AppError.WithCause / Category 枚举]
- [Source: cmd/cat/initialize.go — DI 装配模式 / Cron scheduler / wsHub 相对位置]
- [Source: cmd/cat/app.go — Runnable 接口 / Final 逆序]
- [Source: _bmad-output/implementation-artifacts/0-12-session-resume-cache-throttle.md — Provider interface + Empty impl 模式 + applyDefaults 向后兼容]
- [Source: _bmad-output/implementation-artifacts/0-11-ws-connect-rate-limit-and-abnormal-device-reject.md — ZSET score Unix ms / fail-closed 安全关模式]
- [Source: _bmad-output/implementation-artifacts/0-10-ws-upstream-eventid-idempotent-dedup.md — SETNX + pipeline + DedupResult 模式]
- [Source: _bmad-output/implementation-artifacts/0-8-cron-scheduler-and-distributed-lock.md — withLock + 多副本 cron]
- [Source: _bmad-output/implementation-artifacts/0-7-clock-interface-and-virtual-clock.md — Clock / FakeClock]

### Project Structure Notes

- **新增模块 `internal/push/`**：首个实质内容包（之前只有 `doc.go` 占位）。包含 7 个 `.go` 文件 + 1 个 `testdata/` 目录。这与 architecture.md line 905-909 完全一致。
- **`pkg/ids/ids.go`**：从占位升级为真实类型定义。未来 Story 1.1 / 1.4 / 3.1 / 4.2 / 6.1 / 7.1 等会继续追加 `FriendID / SkinID / BlindboxID / InviteTokenID / ConnID / RoomID / ApnsTokenID` 等（architecture line 936）。本 story 不新增不相关 typed IDs（YAGNI）。
- **`internal/cron/`**：从 1 job 扩到 2 jobs；Scheduler 构造签名增 1 参数是有意的——后续 Story 2.5 / 6.2 / 8.1 还会分别加 state_decay / blindbox_drop / cold_start_recall job，每次都会增依赖。当前 constructor positional 参数数量预计最多 6-8 个；考虑 Story 2.5 左右切 `SchedulerConfig` struct 参数。本 story 暂不重构——留待下一个增量 job 时评估。
- **Graceful shutdown 顺序**：architecture.md line 218 明确 "HTTP Shutdown → WS Hub Stop → WS 现有连接发 close frame → cron.Stop → APNs worker 处理完 → repo 关闭 → zerolog flush"。`cmd/cat/app.go` 里 Runnable 注册正向顺序应为 `mongo, redis, cron, wsHub, worker, http` → Final 逆序 `http → worker → wsHub → cron → redis → mongo` 恰好对齐。**注意**：如果 worker 排在 wsHub 之前（例如 `mongo, redis, cron, worker, wsHub, http`），Final 逆序会变成 `http → wsHub → worker → cron → ...`，这也符合 line 218。两种顺序都技术上 OK，但严格遵循 "cron.Stop → APNs worker 处理完" 意味着 `worker` Final **晚于** `cron` Final —— 既 worker 在 runs 里**排在 cron 之后**。这意味着正确排序是 `mongo, redis, cron, worker, wsHub, http` 或 `mongo, redis, cron, wsHub, worker, http`。第二种更自然（"worker" 在 "wsHub" 之后启动但之前 Final，避免 wsHub 广播需要 push 的场景——但本 story worker 不被 wsHub 调用，所以两种等价）。Dev 选第二种匹配 epics line 218 字面顺序。
- **Interface 位置决策对未来 story 的影响**：若未来发现 push 包过大（>8 文件、需要 sub-package），可重构为 `internal/push/{pusher,worker,router,providers}/`，接口仍留在 `internal/push/` 顶层。本 story 保持扁平。
- 与 `docs/backend-architecture-guide.md` 对齐：本 story 的 `internal/push/` 是 §17.2 "推送封装为 Pusher 接口；service 依赖接口" 的落地。
- No architectural deviation; no new external system; single new third-party Go dep (`sideshow/apns2`).

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- miniredis consumer groups: `XGROUP CREATE ... $` ignores entries added
  before the group exists. Tests (unit + integration) therefore
  `EnsureGroup` during `setupWorkerEnv` before any primeQueue call —
  this matches the production `Start()` ordering where EnsureGroup
  precedes any Enqueue. Documented in `apns_worker_test.go`.
- First flake in `TestIntegration_APNs_IdempotencyDedupes` traced to the
  intermediate `waitFor(ZCARD == 0)` racing with the worker's immediate
  retry re-schedule. Replaced the pattern with a single
  `driveAllRetriesToDLQ` helper that only waits on DLQ + ZCARD=1
  boundaries — deterministic across scheduler jitter.
- `.p8` test fixture needs PKCS#8 format (`-----BEGIN PRIVATE KEY-----`),
  not SEC1 (`-----BEGIN EC PRIVATE KEY-----`). The `sideshow/apns2`
  token loader delegates to Go's std-lib which accepts both, but PKCS#8
  matches Apple-issued keys. See `internal/push/testdata/README.md` for
  the regeneration recipe.

### Completion Notes List

- Pusher interface + `PushPayload` / `PushKind` co-located in
  `internal/push/` — the package IS the abstraction (same shape as
  `io.Reader` in `io`). Consumers import `internal/push` and receive a
  cohesive API surface without pulling in Redis internals. This is
  intentionally different from Story 0.12's `ws.ResumeCache` (which
  lives in the sole consumer package) because Pusher has fan-in from
  service / cron / future WS handlers. Dev Notes document the decision
  so future reviewers don't mis-apply the 0.12 pattern.
- Fail-open / fail-closed matrix (AC11) implemented faithfully:
  idempotency SETNX / quiet-hours resolver errors fail open (continue);
  XADD / validation errors fail closed (return to caller); APNs 410
  deletes the token per-token without marking the message retryable; 5xx
  + transport errors mark retryable; 4xx non-410 are permanent. Per-token
  classification is aggregated into a single message-level `anyRetryable`
  flag so one transient failure across N tokens still retries the whole
  message (trade-off accepted in Dev Notes — per-token retry state is
  deferred).
- `APNsWorker.PromoteOnce(ctx)` exported (alongside the private ticker
  goroutine) so integration tests drive promotion deterministically with
  FakeClock + explicit `Advance`; production still uses the 100ms
  ticker. The promoter pipeline is ZREM+XADD (at-most-once promotion per
  member), per Dev Notes rationale.
- `MaxAttempts = 4` (initial send at `attempt=0` + 3 retries via backoff
  table `[1000, 3000, 9000]`). `scheduleRetry` increments `Attempt`
  before ZADD; `retryOrDLQ` uses `qm.Attempt+1 < MaxAttempts` to decide
  between another retry and the DLQ. Integration test
  `TestIntegration_APNs_MaxRetries_DLQ` verifies the 4-send + 1-DLQ
  outcome.
- `pkg/ids/ids.go` grew from the empty `doc.go` placeholder to typed
  `UserID` + `Platform` strings. Pusher's interface signature accepts
  `ids.UserID` so downstream Story 1.1 call sites stay typed from day
  one. `TokenInfo.Platform` intentionally kept as `string` to avoid
  coupling Story 1.4's repository types to `pkg/ids`.
- `NewScheduler` grew a 4th positional parameter (`push.TokenCleaner`).
  Epic 0 threads `push.EmptyTokenCleaner{}`; Story 1.4 will swap in the
  real impl with no further signature change. If a future story adds a
  5th dep we should re-evaluate struct-param form (per 0.12 precedent).
- `cmd/cat/initialize.go` stays under 200 lines. Worker is inserted into
  `App.runs` at position `mongo, redis, cron, wsHub, worker, http`; the
  reverse Final order matches architecture §Graceful Shutdown line 218
  (HTTP → wsHub → worker → cron → redis → mongo).
- `bash scripts/check_time_now.sh` passes: all timestamps in
  `internal/push/*.go` and `internal/cron/apns_token_cleanup_job.go`
  flow through `clock.Now()`. The `time.NewTicker(100*time.Millisecond)`
  in the retry promoter is a scheduling primitive (not a timestamp
  source) per M9 exemption — inline comment + Dev Notes document the
  exemption.

### File List

**New:**

- `server/pkg/redisx/stream.go`
- `server/pkg/redisx/stream_test.go`
- `server/pkg/ids/ids.go`
- `server/internal/push/providers.go`
- `server/internal/push/apns_sender.go`
- `server/internal/push/apns_client.go`
- `server/internal/push/apns_sender_test.go`
- `server/internal/push/apns_router.go`
- `server/internal/push/apns_router_test.go`
- `server/internal/push/pusher.go`
- `server/internal/push/pusher_test.go`
- `server/internal/push/apns_worker.go`
- `server/internal/push/apns_worker_test.go`
- `server/internal/push/apns_worker_integration_test.go`
- `server/internal/push/testdata/test_key.p8`
- `server/internal/push/testdata/README.md`
- `server/internal/cron/apns_token_cleanup_job.go`
- `server/internal/cron/apns_token_cleanup_job_test.go`
- `server/docs/code-examples/pusher_usage_example.go`

**Modified:**

- `server/internal/config/config.go` — `APNsCfg` +14 fields, `applyDefaults`
  +11 fills, new `validateAPNs()` helper + `mustValidate` hook
- `server/internal/config/config_test.go` — extend valid-config assertions
  and add APNs defaults-when-section-omitted test
- `server/config/default.toml` — `[apns]` section extended to 18 fields
- `server/internal/cron/scheduler.go` — `NewScheduler` signature +
  `push.TokenCleaner`; `registerJobs` now registers `apns_token_cleanup`
- `server/internal/cron/scheduler_test.go` — `setupTestScheduler`
  passes `push.EmptyTokenCleaner{}`
- `server/cmd/cat/initialize.go` — push platform wiring gated by
  `cfg.APNs.Enabled`; NoopPusher fallback; `NewScheduler` call updated
- `server/go.mod` + `server/go.sum` — `github.com/sideshow/apns2
  v0.25.0` (direct) + `golang-jwt/jwt/v4 v4.4.1` (indirect)

### Change Log

| Date       | Version | Author | Summary |
|------------|---------|--------|---------|
| 2026-04-18 | 0.1     | huing  | Initial implementation — APNs push platform: Pusher interface + Redis-Streams queue + per-platform topic routing + 3-retry exponential backoff + DLQ + 410 cleanup + 30-day token expiry cron + quiet-hours consume-time downgrade + idempotency gate (5-min SETNX). |
