# Story 0.14: WS 消息类型注册表与版本查询

Status: review

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a client developer,
I want to query the server's currently supported WS message type list with versions via an unauthenticated HTTP endpoint,
So that Watch / iPhone clients can validate compatibility on upgrade, avoid sending unknown types, and the single `dto/ws_messages.go` source-of-truth keeps `internal/ws/Dispatcher` and the human-readable `docs/api/ws-message-registry.md` in lock-step — closing the G2 gap (FR59, PRD §Versioning Strategy, architecture §G2, architecture §Project Structure `internal/dto/ws_messages.go`, architecture §P3 WS naming convention, architecture §Bootstrap endpoints line 814, Story 0.7 AC 验证场景).

## Acceptance Criteria

1. **AC1 — `internal/dto/` 包建立 + `ws_messages.go` 常量表（全新包，epics line 718, 架构 §Project Structure line 867-870, M8）**：

   - Create `internal/dto/ws_messages.go` (package `dto`). 本 story 首次引入 `internal/dto` 包 —— architecture §Project Structure line 867-870 已预留此包作为"handler 层 DTO + 错误码注册表 + WS 消息常量"的汇聚点；后续 1.1 / 2.x 等 story 会在同一目录追加 HTTP DTO、AppError 注册文件。
   - 定义以下常量 + 元数据（严格以 `domain.action` 点分 —— P3 WS 命名约定；response type 自动追加 `.result` 后缀不单独声明）：
     ```go
     package dto

     // WSDirection describes whether a message flows upstream (client→server),
     // downstream (server→client push), or bidirectional (client-initiated RPC
     // expecting a server response).
     type WSDirection string

     const (
         WSDirectionUp WSDirection = "up"
         WSDirectionDown WSDirection = "down"
         WSDirectionBi   WSDirection = "bi"
     )

     // WSMessageMeta is the compile-time metadata for every WS message type
     // the server currently understands. Keep this struct immutable —
     // mutation would race with the HTTP handler (AC5) which reads the slice
     // without locking.
     type WSMessageMeta struct {
         Type            string      // canonical envelope.type, e.g. "session.resume"
         Version         string      // MVP: "v1" for all; bump independently per AC10
         Direction       WSDirection // up | down | bi
         RequiresAuth    bool        // false only for `ping` / `debug.echo` once defined
         RequiresDedup   bool        // true ⇔ dispatcher uses RegisterDedup
         DebugOnly       bool        // true if only registered when cfg.Server.Mode == "debug"
         Description     string      // one-line English summary for ws-message-registry.md generator (AC7)
     }

     // WSMessages is the authoritative list. Every dispatcher registration
     // MUST have exactly one entry here; AC4 test enforces the invariant.
     var WSMessages = []WSMessageMeta{
         {Type: "session.resume", Version: "v1", Direction: WSDirectionBi,
             RequiresAuth: true, RequiresDedup: false, DebugOnly: true,
             Description: "Client requests a full session snapshot (user/friends/cat_state/skins/blindboxes/room) cached 60s."},
         {Type: "debug.echo", Version: "v1", Direction: WSDirectionBi,
             RequiresAuth: true, RequiresDedup: false, DebugOnly: true,
             Description: "Debug-only: server echoes request payload verbatim. No business effect."},
         {Type: "debug.echo.dedup", Version: "v1", Direction: WSDirectionBi,
             RequiresAuth: true, RequiresDedup: true, DebugOnly: true,
             Description: "Debug-only: exercises the dedup middleware; idempotent replay of envelope.id returns cached result."},
     }
     ```
   - **`DebugOnly: true` on `session.resume`** is deliberate and matches `cmd/cat/initialize.go` line 128 (release mode intentionally skips the handler while every Provider is an `Empty*Provider` that would corrupt client state — see Story 0.12 AC7 and the release-mode comment block in `initialize.go` lines 120-138). When the first real `UserProvider` lands in Story 1.1, that story removes the `DebugOnly` flag here and removes the release-mode guard in `initialize.go`; the AC4 consistency test will enforce both changes stay in sync because the release-mode test harness (AC4) will start failing if they drift apart.
   - `WSMessages` 是 `package-level` `var`; 本 story **只注册已在 dispatcher 中实际注册的 3 个类型**。未来 story（1.x/2.x/...）追加新 handler 时：先在 `WSMessages` 追加一条元数据 → 再在 `initialize.go` 调用 `dispatcher.Register` 或 `RegisterDedup` → `go test ./internal/dto/... -run TestWSMessages` 绿 → 提交。错序追加会使 AC4 测试 fail，这是设计目的（预防 P2 风格"代码加消息但注册表忘更新"的 G2 失败）。
   - **Lookup helper**（同文件）：
     ```go
     // WSMessagesByType returns a map keyed by canonical type for O(1) lookup
     // by the HTTP registry handler (AC5) and tests. The map is built once
     // at package init — callers MUST treat it as read-only; mutation would
     // race with the handler under concurrent /v1/platform/ws-registry
     // requests.
     var WSMessagesByType = func() map[string]WSMessageMeta {
         m := make(map[string]WSMessageMeta, len(WSMessages))
         for _, meta := range WSMessages {
             if _, dup := m[meta.Type]; dup {
                 panic("dto.WSMessages: duplicate Type " + meta.Type)
             }
             m[meta.Type] = meta
         }
         return m
     }()
     ```
   - Package comment at top of file (`// Package dto ...`) explicitly names `internal/dto` as the **source of truth** for the WS message registry and references `docs/api/ws-message-registry.md` (generated) + `docs/api/openapi.yaml` (HTTP) as downstream artifacts.

2. **AC2 — `Dispatcher` 暴露 `RegisteredTypes()` 枚举 API（`internal/ws/dispatcher.go`, P2）**：

   - Add public method on `*Dispatcher`:
     ```go
     // RegisteredTypes returns the sorted list of message types bound to this
     // dispatcher via Register or RegisterDedup. The return value is a fresh
     // slice — callers may mutate it without affecting dispatcher state.
     // Used by dto.TestWSMessages (Story 0.14 AC4) and by the WS registry
     // consistency test; not part of the message-handling hot path.
     func (d *Dispatcher) RegisteredTypes() []string { ... }
     ```
   - Implementation reads `d.types` (already the authoritative set since Story 0.10 introduced it), copies keys to a new slice, `sort.Strings` it. No mutation of dispatcher state; no new fields.
   - **Why `types` not `handlers`?** `d.types` is the write-before-check set used by both `Register` and `RegisterDedup` (dispatcher.go lines 43, 59) — it is exactly the set we want. `d.handlers` contains the same keys today, but the dedup-wrapped entries in `d.handlers` are wrapped values, not raw handlers; `d.types` is the clean source.
   - **Concurrency note**: `RegisteredTypes` has no locking because dispatcher registration happens exclusively in `initialize.go` before any readPump goroutine starts consuming messages. The method documents this precondition in its godoc ("call after initialize() returns; calling after Hub.Start is undefined").
   - Unit test in `internal/ws/dispatcher_test.go` (amend existing file): table-driven — empty dispatcher → empty slice; 1 Register → single-element; 1 Register + 1 RegisterDedup → sorted 2-element; duplicate `Register` panic unchanged (existing tests in that file already cover panic paths, don't regress).

3. **AC3 — `internal/handler/platform_handler.go`（新建, architecture §Project Structure line 858-864, architecture §P3）**：

   - New handler struct (style mirrors `handler.HealthHandler` — same package, same constructor pattern, same Gin `c *gin.Context` method signature):
     ```go
     package handler

     import (
         "net/http"
         "time"
         "github.com/gin-gonic/gin"
         "github.com/huing/cat/server/internal/dto"
         "github.com/huing/cat/server/pkg/clockx"
     )

     type PlatformHandler struct {
         clock clockx.Clock
     }

     func NewPlatformHandler(clock clockx.Clock) *PlatformHandler {
         if clock == nil {
             panic("handler.NewPlatformHandler: clock is required")
         }
         return &PlatformHandler{clock: clock}
     }

     // WSRegistryResponse is the wire shape for GET /v1/platform/ws-registry.
     // Fields are stable public API (client upgrade compat); additive changes
     // are non-breaking, but removing or renaming a field requires the
     // Versioning Strategy bump described in AC10.
     type WSRegistryResponse struct {
         APIVersion string              `json:"apiVersion"`
         ServerTime string              `json:"serverTime"` // RFC3339 with nanoseconds stripped (time.Time.UTC().Format(time.RFC3339))
         Messages   []WSRegistryMessage `json:"messages"`
     }

     type WSRegistryMessage struct {
         Type          string `json:"type"`
         Version       string `json:"version"`
         Direction     string `json:"direction"`
         RequiresAuth  bool   `json:"requiresAuth"`
         RequiresDedup bool   `json:"requiresDedup"`
     }
     ```
   - `DebugOnly` is intentionally **not** surfaced on the wire — clients shouldn't see internal mode gating, and debug-only types shipped to a release server would confuse upgrade-compat logic. The release-mode rendering skips `DebugOnly: true` entries (AC5); the wire shape therefore needs no flag.
   - The response struct lives in `internal/handler` (alongside the handler) rather than `internal/dto` because it's HTTP-response shape, not a WS-envelope shape — M8 "DTO 转换位置" puts HTTP-response structs at handler-layer boundary. `dto` owns the *domain* (WS message metadata); `handler` owns the *HTTP framing* of that domain.

4. **AC4 — `internal/dto/ws_messages_test.go` consistency test（G2 修复, P6 test patterns, architecture §Gap Analysis line 1187-1189）**：

   - Test file in same package (`package dto`). Two table-driven cases cover the two initialize.go branches:
     - **Case `"debug_mode"`** — constructs a `*ws.Dispatcher` via the same calls `cmd/cat/initialize.go` lines 113-129 use for debug mode: `dispatcher.Register("debug.echo", noopFn)`, `dispatcher.RegisterDedup("debug.echo.dedup", noopFn)`, `dispatcher.Register("session.resume", noopFn)`. Asserts `dispatcher.RegisteredTypes()` equals the sorted `Type` column of `WSMessages` with `DebugOnly` **included** (all 3 constants today).
     - **Case `"release_mode"`** — constructs a `*ws.Dispatcher` with no registrations (mirrors `initialize.go` release branch line 131-137 which registers nothing post-0.13). Asserts `dispatcher.RegisteredTypes()` equals the sorted list of `WSMessages` entries with `DebugOnly == false` (currently empty; will grow from Story 1.1 onward).
   - Build an in-package-test helper `noopHandler(_ context.Context, _ *ws.Client, _ ws.Envelope) (json.RawMessage, error) { return nil, nil }` shared by both cases. **Do not import `cmd/cat`**; directly invoke the WS package to build the dispatcher. This keeps the test in the cheap unit path, no Testcontainers, no initialize side-effects.
   - Test must also detect: duplicate Type entries (rely on `WSMessagesByType`'s panic at package init, captured via `require.NotPanics` in a separate `TestWSMessagesNoDuplicates` — belt-and-braces beyond the init panic); non-empty `Description` on every entry; `Direction` ∈ {`up`, `down`, `bi`}; `Version` ∈ {`v1`, `v2`, ...} matching regex `^v\d+$`.
   - Test must NOT import `internal/handler` (avoid cyclic HTTP dependency in a domain-boundary unit test).
   - Failure message on dispatcher/constants mismatch must **name both sides** for triage: `assert.ElementsMatch(t, wantTypes, got, "dto.WSMessages vs dispatcher registrations drifted: %s", diff)`.

5. **AC5 — `GET /v1/platform/ws-registry` endpoint wiring（`cmd/cat/wire.go` + `cmd/cat/initialize.go`, architecture §814 bootstrap endpoints）**：

   - In `cmd/cat/wire.go`:
     ```go
     type handlers struct {
         health    *handler.HealthHandler
         wsUpgrade *ws.UpgradeHandler
         platform  *handler.PlatformHandler  // NEW
     }
     ```
     and `buildRouter` adds **before** the `/ws` line:
     ```go
     r.GET("/v1/platform/ws-registry", h.platform.WSRegistry)
     ```
     placed alongside `/healthz` / `/readyz` (the three bootstrap endpoints are grouped visually; future JWT-protected `/v1/*` routes land in a separate group per architecture line 814 "中间件挂载策略：`/v1/*` 路由组强制鉴权；bootstrap endpoints（`/auth/apple, /auth/refresh, /healthz, /readyz, /v1/platform/ws-registry`）不挂"). Add a one-line comment pointing at architecture line 814 so future PR reviewers don't reflexively wrap it in JWT middleware.
   - **Mode-aware rendering** in the `WSRegistry` method body: handler accepts `cfg.Server.Mode` via constructor (pattern: `NewPlatformHandler(clock, cfg.Server.Mode)`), stores mode as `serverMode string`. On each request: iterate `dto.WSMessages`; skip entries where `meta.DebugOnly && serverMode != "debug"`. This matches the exact gating in `initialize.go` line 113-138 and is the ONLY place rendering asymmetry exists — never leak `DebugOnly` via the wire.
   - In `cmd/cat/initialize.go` after the existing `handlers` struct build (around line 149):
     ```go
     h := &handlers{
         health:    handler.NewHealthHandler(...),
         wsUpgrade: upgradeHandler,
         platform:  handler.NewPlatformHandler(clk, cfg.Server.Mode),
     }
     ```
   - Response body (Gin `c.JSON`):
     ```go
     nowStr := h.clock.Now().UTC().Format(time.RFC3339)
     c.JSON(http.StatusOK, WSRegistryResponse{
         APIVersion: "v1",
         ServerTime: nowStr,
         Messages:   msgs,  // filtered slice built above
     })
     ```
   - `serverTime` MUST go through `h.clock.Now()` (not `time.Now()`) — Story 0.7 AC specifically validated: *"Story 0.14 的 WS 消息类型版本查询 endpoint 返回的时间戳使用 Clock（验证 Clock 真的被业务代码使用）"* (epics line 581). M9 禁止业务代码直接调 `time.Now`; `pkg/clockx/clock.go` 的 `RealClock.Now()` already returns `time.Now().UTC()` so there's no double-UTC concern. Output format `time.RFC3339` (no nanoseconds, timezone `Z`) so iOS `ISO8601DateFormatter` parses out-of-box.

6. **AC6 — Endpoint 无鉴权 + CORS 兼容（architecture line 814）**：

   - `/v1/platform/ws-registry` 挂在 `r.GET` 顶层，与 `/healthz /readyz` 同级；**不**进入未来的 `r.Group("/v1", middleware.JWTAuth())` 组（该组将在 Story 1.3 首次引入）。Story 1.3 实现 JWT 中间件时必须在其 PR 描述中明确"保留本 endpoint 在 JWT group 之外"— 我们在此 story 的 godoc 注释里说明这一点（可 grep）：
     ```go
     // WSRegistry serves GET /v1/platform/ws-registry. Intentionally mounted
     // OUTSIDE any JWT group because clients call it pre-authentication to
     // decide whether the server speaks their protocol dialect (FR59,
     // architecture line 814).
     func (h *PlatformHandler) WSRegistry(c *gin.Context) { ... }
     ```
   - Response carries no sensitive material (message types + version + booleans + server time). Explicitly DO NOT log full response body at INFO level on every call (P5 `[convention]` 每条日志必含 camelCase fields; keep the access log to the RequestID middleware default — no extra `logx.Ctx` call in this handler).
   - No CORS concerns: the server does not accept web-browser origins for any MVP client; Watch / iPhone native apps do not enforce CORS. If in future the PRD adds a web client, they add CORS middleware — not our concern here.
   - Rate limit: no per-IP limiter on this endpoint. Unauthenticated rate limiting is `middleware.RateLimit` territory and does not exist yet (Story 0.11 only rate-limits WS *connect*, not HTTP). Registry is a cheap static dump (3-entry slice, marshaled JSON) — no DB/Redis I/O. Abuse surface is negligible until an HTTP ratelimiter exists.

7. **AC7 — Human-readable registry doc `docs/api/ws-message-registry.md`（epics line 720, architecture §Project Structure line 963-964）**：

   - Create `docs/api/ws-message-registry.md` (new file; `docs/api/` directory also NEW — mkdir in the same PR). File is **generated-style but hand-maintained** for MVP: sections auto-organized, contents updated manually next to `ws_messages.go` edits. Layout:
     ```markdown
     # WS Message Type Registry

     > Source of truth: `internal/dto/ws_messages.go` (Go constants + metadata).
     > CI consistency: `internal/dto/ws_messages_test.go` (Story 0.14 AC4)
     > and `cmd/cat/integration_test.go` TestWSRegistryEndpoint (Story 0.14 AC9).
     >
     > When adding a new message: (1) add entry to `dto.WSMessages`,
     > (2) register handler in `initialize.go`, (3) update this file under
     > the matching section, (4) run `bash scripts/build.sh --test`.

     **Envelope shape** (upstream request from client):
     ```json
     { "id": "<client-generated unique>", "type": "<domain.action>", "payload": { ... } }
     ```

     **Envelope shape** (downstream response from server):
     ```json
     { "id": "<echo>", "ok": true|false, "type": "<domain.action>.result",
       "payload": { ... } | null, "error": { "code": "...", "message": "..." } | null }
     ```

     **Envelope shape** (downstream server push — no request):
     ```json
     { "type": "<domain.action>", "payload": { ... } }
     ```

     ## Message Types

     ### session.resume (bi, v1, auth required)

     ... (per AC7 details below)
     ```
   - One `### <type>` section per message, each with: Direction, Version, Auth required, Dedup required, Description, example request payload (if any), example response payload (if any). For MVP (3 debug-only types), three sections total.
   - **No generator script in Epic 0.** Architecture G2 fix specifies "CI 单元测试校验与代码常量一致" — the AC4 test guarantees the `ws_messages.go` ↔ dispatcher invariant; checking the `.md` against the constants is a future enhancement (tracked in `docs/api/ws-message-registry.md` header TODO comment — do not open an external issue tracker item). Hand-maintained discipline is enforced by the AC4 test failing loudly when constants drift, plus PR-checklist item (AC14).

8. **AC8 — OpenAPI 占位 `docs/api/openapi.yaml`（G2 修复, epics line 724）**：

   - Create minimal valid OpenAPI 3.0.3 yaml describing ONLY `/v1/platform/ws-registry` (the only HTTP endpoint formalized in Epic 0 beyond `/healthz` `/readyz`). Structure:
     ```yaml
     openapi: 3.0.3
     info:
       title: Cat Backend API
       version: "0.14.0-epic0"
       description: >
         HTTP API surface. WebSocket envelope/message protocol is defined in
         docs/api/ws-message-registry.md (source of truth internal/dto/ws_messages.go).
     servers:
       - url: https://api.example.invalid
     paths:
       /v1/platform/ws-registry:
         get:
           summary: List supported WS message types + version
           description: >
             Unauthenticated metadata endpoint used by Watch / iPhone clients
             on upgrade to verify protocol compatibility (FR59).
           responses:
             "200":
               description: OK
               content:
                 application/json:
                   schema:
                     $ref: "#/components/schemas/WSRegistryResponse"
     components:
       schemas:
         WSRegistryResponse: { ... }
         WSRegistryMessage:  { ... }
     ```
   - `healthz` / `readyz` intentionally OMITTED from this yaml — they are infra endpoints not intended as external API contract. Future story (1.x onward) adds them when documenting the broader auth/device surface.
   - **Schema field names MUST match the JSON on the wire exactly** (camelCase): `apiVersion`, `serverTime`, `requiresAuth`, `requiresDedup`. Test AC9 asserts YAML matches handler response.

9. **AC9 — CI: `swagger validate docs/api/openapi.yaml` + integration test endpoint shape（G2 修复, epics line 724, architecture §P6）**：

   - Add `make validate-openapi` (or a bash step in `scripts/build.sh`) that runs `swagger validate docs/api/openapi.yaml`. **Binary resolution**: prefer `github.com/go-swagger/go-swagger/cmd/swagger`(`go install github.com/go-swagger/go-swagger/cmd/swagger@latest` run ONCE during CI bootstrap; tolerate `swagger not in PATH` by `go run github.com/go-swagger/go-swagger/cmd/swagger@v0.31.0 validate ...`). Pin version to `v0.31.0` in `scripts/build.sh` to avoid silent regressions — this exact version validates OpenAPI 3.0.3 (2.x is too old).
   - `bash scripts/build.sh --test` already runs `go vet ./...` + `go test ./...` per Story 0.1. Extend `scripts/build.sh` to additionally run `swagger validate docs/api/openapi.yaml` **before** `go test`. Failure of swagger validation blocks CI — same severity as a test failure. Implementation: add a shell function `validate_openapi()` that runs only if `docs/api/openapi.yaml` exists (so pre-this-story branches still build).
   - **Integration test** `cmd/cat/integration_test.go` (amend, don't create new file — reuse existing integration suite from Story 0.4): new test `TestWSRegistryEndpoint`:
     1. Build `*gin.Engine` via `buildRouter(cfg, handlers)` in `"release"` mode — mirrors production.
     2. `httptest.NewRecorder` + `req, _ := http.NewRequest("GET", "/v1/platform/ws-registry", nil)`.
     3. `router.ServeHTTP(w, req)`; assert `w.Code == 200`, `Content-Type: application/json`.
     4. Decode body into `handler.WSRegistryResponse`; assert `apiVersion == "v1"`; `serverTime` parses as RFC3339; `len(messages) == 0` (release mode has no registered types yet); slice is nil-safe (encode as `[]` not `null` — if Go marshals nil slice as `null` adjust handler to `msgs := []handler.WSRegistryMessage{}` default).
     5. Second sub-test in `"debug"` mode: `len(messages) == 3`, types ∈ {`debug.echo`, `debug.echo.dedup`, `session.resume`}; `session.resume.RequiresDedup == false`; `debug.echo.dedup.RequiresDedup == true`.
     6. Third sub-test: Clock injection verification — pass a `*clockx.FakeClock` with fixed time `2026-04-18T12:34:56Z` into `NewPlatformHandler`; assert response `serverTime == "2026-04-18T12:34:56Z"`. This is the validation demanded by Story 0.7 AC (epics line 581).
   - Integration test build tag: no `//go:build integration` needed — this test uses `httptest`, not Testcontainers, so it belongs in the default fast test lane (M11 only bars parallelism for Testcontainers-backed tests).

10. **AC10 — 版本策略：MVP 全部 `v1`；破坏性变更时 `v2` + 30 天过渡（epics line 723, PRD §Versioning Strategy）**：

    - All three current messages use `Version: "v1"`. When the first breaking change lands (e.g. Story 2.1 introduces `state.tick` whose `payload.steps` becomes required; later a schema change requires `payload.stepsV2: int`):
      - Add new entry `{Type: "state.tick", Version: "v2", ...}` alongside the old `{Type: "state.tick", Version: "v1", ...}` (yes — two entries with same `Type` but different `Version`; `WSMessagesByType` must change to key on `Type` AND `Version` at that point — future story's work, not this one).
      - The 30-day transition: server accepts both `state.tick@v1` and `state.tick@v2`; clients report which they understand via ... (mechanism deferred; version-querying clients see both rows).
    - **Today's MVP simplification**: `WSMessagesByType` keys on `Type` alone; future refactor is a one-line change + test update. Document this explicitly in the `WSMessagesByType` godoc: *"MVP invariant: Type is unique. When AC10 v1/v2 coexistence lands, key on Type+Version."*
    - `apiVersion: "v1"` (the outer envelope of the registry response) also stays `v1` — this is the API version of `/v1/platform/ws-registry` itself, bumped only if the registry *response schema* changes (e.g. adding a required top-level field). Additive message additions do not bump `apiVersion`.

11. **AC11 — 日志与错误（P4 AppError、P5 logging, M13 PII）**：

    - Handler does NOT emit a per-request INFO log for the registry endpoint (middleware.Logger covers access logs; adding a second entry doubles log volume for no signal). Handler ONLY logs at WARN/ERROR if something unexpected happens — the only failure mode is JSON marshal error of the response, which should never happen for a static slice but is defended with: `if err := c.ShouldBind... ` — n/a, we're using `c.JSON` which logs internally.
    - **No AppError** codes introduced by this story: this is a happy-path-only endpoint with no validation (no request body, no query params accepted — any query string is silently ignored, matching `/healthz` behavior). The only error surface is the 404 / 405 from Gin routing mismatch, which requires no application code. Future stories that add actual logic may introduce AppError codes; for now keep `internal/dto/error_codes.go` creation deferred (Story 1.x will create it per architecture line 867).
    - Log nothing from `NewPlatformHandler` init (no `.p8` keys, no Redis connect — nothing interesting). Startup `initialize.go` log line `log.Info().Msg("platform handler initialized")` is OPTIONAL; prefer omission to keep startup log noise low (consistent with `NewHealthHandler` which also logs nothing at construction).

12. **AC12 — 单元测试覆盖（P6）**：

    - `internal/dto/ws_messages_test.go`:
      - `TestWSMessages_AllFieldsPopulated` — table-drive over `WSMessages`; assert `Type != ""`, `Description != ""`, `Direction ∈ {up, down, bi}`, `Version` matches `^v\d+$`, `Type` matches `^[a-z0-9]+(?:\.[a-z0-9]+)*$` (P3 domain.action convention — rejects `Debug.Echo`, `debug_echo`, etc.).
      - `TestWSMessages_NoDuplicates` — assert no two entries share a `Type` (belt-and-braces on `WSMessagesByType` init panic).
      - `TestWSMessages_ConsistencyWithDispatcher_DebugMode` (the AC4 core) — documented above.
      - `TestWSMessages_ConsistencyWithDispatcher_ReleaseMode` (the AC4 core) — documented above.
    - `internal/ws/dispatcher_test.go` (amend):
      - `TestDispatcher_RegisteredTypes` — 3 sub-cases (empty / one Register / Register + RegisterDedup); asserts sorted output.
    - `internal/handler/platform_handler_test.go` (new):
      - `TestPlatformHandler_WSRegistry_DebugMode` — build handler with `clockx.NewFakeClock(...)` + `mode="debug"`; fire `httptest` request; assert response body match (3 messages, `apiVersion: "v1"`, `serverTime` matches fake time).
      - `TestPlatformHandler_WSRegistry_ReleaseMode` — same but `mode="release"`; assert 0 messages, nil/empty slice encoded as `[]`.
      - `TestPlatformHandler_WSRegistry_OmitsDebugOnlyInRelease` — constructs custom `dto.WSMessages` slice via test swap (if swap is awkward, rely on the release-mode filter being driven by the shared `dto.WSMessages` and assert the actual behavior — do not introduce test-only swap hooks in `dto` package).
      - `TestNewPlatformHandler_NilClockPanics` — guard the constructor.
    - **Target coverage**: handler package ≥ 80% for this handler; `dto` package 100% (tiny surface, every branch reachable).
    - Test style: table-driven (P6 M10), `t.Parallel()` on unit tests (M11), `require.NoError` for setup, `assert.Equal` / `assert.ElementsMatch` for verification, `errors.Is/As` if error assertions appear (M12).

13. **AC13 — 文档与 docs/code-examples 更新**：

    - No new entry in `docs/code-examples/` required — this story's handler is sufficiently covered by the health handler as the HTTP-handler reference; the distinct value of this story is the **registry discipline pattern** (code constant ↔ dispatcher registration ↔ human doc ↔ OpenAPI schema), not the handler shape. If future review demands an example, add `registry_pattern_example.md` (markdown, not Go) under `docs/code-examples/`.
    - Update `docs/backend-architecture-guide.md` §12 WebSocket section with a one-paragraph "Message Registry" callout pointing at `internal/dto/ws_messages.go` as the source of truth and the AC4 test as the drift guard. **Do not** restate the full message list in the architecture guide — the guide describes the *pattern*, `ws-message-registry.md` describes the *current contents*.
    - Link from the top of `docs/backend-architecture-guide.md` TOC to `docs/api/ws-message-registry.md` and `docs/api/openapi.yaml`.

14. **AC14 — PR checklist compliance（backend-architecture-guide.md §19）**：

    - All items in §19 新代码检查清单 must pass for this story's diff:
      - No `fmt.Printf` / `log.Printf` — handler uses `gin.Context` and `c.JSON` only; `logx` not imported by handler.
      - All I/O functions take `ctx context.Context` — `WSRegistry(c *gin.Context)` receives ctx via `c.Request.Context()`; no direct DB/Redis calls exist in this handler, so explicit ctx parameter is unnecessary.
      - Handler does not directly reference `*mongo.Client` / `*redis.Client` — confirmed (only `dto.WSMessages` + `clockx.Clock`).
      - New interfaces defined in consumer package — no new interfaces added to `dto` (package-level vars only); `RegisteredTypes` is a method on existing `*Dispatcher` (0.9's struct), not a new interface.
      - Typed IDs — N/A for this story.
      - Redis `Set` w/ TTL — N/A.
      - Errors wrapped via sentinel + `fmt.Errorf("%w")` — N/A (no errors produced).
      - Public godoc — every exported identifier in `internal/dto/ws_messages.go` and `internal/handler/platform_handler.go` has a godoc comment; `Dispatcher.RegisteredTypes` documents its concurrency precondition.
      - Corresponding `*_test.go` — AC12 enumerates.
      - `bash scripts/build.sh --test` green locally — gate.
      - No `context.TODO()` / `context.Background()` in business code — confirmed.
      - No `// TODO` without issue number — one allowed `// TODO: regenerate from WSMessages when Story N.N lands` placeholder in the `.md` header is OK because it references a future story, not an untracked issue.

15. **AC15 — 启动自检 (fail-fast)**：

    - After `initialize()` wires `dispatcher` and before returning `*App`, run a one-shot drift check in `initialize.go`:
      ```go
      // Registry-drift fail-fast: every dispatcher registration must have a
      // dto.WSMessages entry, and every non-DebugOnly entry must be
      // registered in release mode. The unit test enforces this at CI time;
      // this check catches runtime-only drift (e.g. a feature flag that
      // conditionally registers in dev). Fail fast rather than serving a
      // registry response that lies about what the dispatcher accepts.
      if err := validateRegistryConsistency(dispatcher, cfg.Server.Mode); err != nil {
          log.Fatal().Err(err).Msg("ws message registry drift detected")
      }
      ```
    - `validateRegistryConsistency(dispatcher *ws.Dispatcher, mode string) error` lives in `cmd/cat/initialize.go` (same file, unexported helper) and is also covered by a small unit test in `cmd/cat/initialize_test.go` (create this file — today only `app_test.go` + `integration_test.go` exist).
    - **Skip policy**: if `cfg.Server.Mode == "debug"` AND a future gate decides to register extra debug handlers, update `dto.WSMessages` with `DebugOnly: true` and this helper re-validates. The helper must NOT have a skip-in-production bypass — production drift is the exact failure mode being caught.

## Tasks / Subtasks

- [x] **Task 1 — 建立 `internal/dto/` 包 + 常量表** (AC: #1, #10)
  - [x] 创建目录 `server/internal/dto/`
  - [x] 新增 `ws_messages.go` 含 `WSDirection` / `WSMessageMeta` / `WSMessages` / `WSMessagesByType`
  - [x] Package godoc 指向 `docs/api/ws-message-registry.md` + `docs/api/openapi.yaml`
  - [x] 三条初始 entries：`session.resume` / `debug.echo` / `debug.echo.dedup`（均 `DebugOnly: true`，`Version: "v1"`）
- [x] **Task 2 — Dispatcher 公开枚举 API** (AC: #2)
  - [x] `internal/ws/dispatcher.go` 追加 `RegisteredTypes()` 方法（读 `d.types` → sort → copy slice）
  - [x] godoc 注释"call after initialize() returns"的并发前提
  - [x] `internal/ws/dispatcher_test.go` 追加 `TestDispatcher_RegisteredTypes`（3 子表）
- [x] **Task 3 — PlatformHandler** (AC: #3, #5, #6, #11)
  - [x] 新建 `internal/handler/platform_handler.go`：`PlatformHandler` 结构体 + `NewPlatformHandler(clock, mode)` + `WSRegistry(c *gin.Context)`
  - [x] `WSRegistryResponse` / `WSRegistryMessage` 类型定义（`json:"camelCase"` tag）
  - [x] 模式过滤逻辑：mode != "debug" 时 skip `DebugOnly: true`
  - [x] `clock.Now().UTC().Format(time.RFC3339)` 生成 `serverTime`
  - [x] nil clock panic + 单元测试 `TestNewPlatformHandler_NilClockPanics`
  - [x] nil-slice-safe JSON：handler 局部 `msgs := []handler.WSRegistryMessage{}` 初始化
- [x] **Task 4 — 路由与装配** (AC: #5, #6)
  - [x] `cmd/cat/wire.go` `handlers` struct 增 `platform *handler.PlatformHandler`
  - [x] `buildRouter` 在 `/healthz /readyz` 附近追加 `r.GET("/v1/platform/ws-registry", h.platform.WSRegistry)`，注释指向 architecture line 814
  - [x] `cmd/cat/initialize.go` 构造 `handler.NewPlatformHandler(clk, cfg.Server.Mode)` 并写入 `handlers` struct
  - [x] 启动自检 `validateRegistryConsistency` helper + `log.Fatal` 漂移拦截（AC15）
- [x] **Task 5 — Drift 单元测试（核心守门）** (AC: #4, #12)
  - [x] 新建 `internal/dto/ws_messages_test.go`
  - [x] `TestWSMessages_AllFieldsPopulated` / `TestWSMessages_NoDuplicates`
  - [x] `TestWSMessages_ConsistencyWithDispatcher_DebugMode` / `_ReleaseMode`（需导入 `internal/ws`，测试内 stubs `noopHandler`）
  - [x] 断言 `Type` 符合 `^[a-z0-9]+(?:\.[a-z0-9]+)*$`、`Version` 符合 `^v\d+$`
- [x] **Task 6 — Handler 单元测试** (AC: #12)
  - [x] 新建 `internal/handler/platform_handler_test.go`
  - [x] debug / release 模式下响应内容、`serverTime` FakeClock 注入、空 `messages` 以 `[]` 序列化
- [x] **Task 7 — 集成测试** (AC: #9, #12)
  - [x] 扩展 `cmd/cat/integration_test.go` 增 `TestWSRegistryEndpoint`（debug + release 两模式 + FakeClock 子测试）
  - [x] `cmd/cat/initialize_test.go`（新建）覆盖 `validateRegistryConsistency`
- [x] **Task 8 — OpenAPI 占位 + CI 校验** (AC: #8, #9)
  - [x] 新建 `docs/api/openapi.yaml`（OpenAPI 3.0.3，`/v1/platform/ws-registry` + schemas）
  - [x] 扩展 `scripts/build.sh` 增 `validate_openapi` 函数，调用 `go run github.com/go-swagger/go-swagger/cmd/swagger@v0.31.0 validate docs/api/openapi.yaml`
  - [x] 保证 `go.sum` 或构建缓存能容忍首次 `go run` 下载
- [x] **Task 9 — 人类可读注册表 doc** (AC: #7, #13)
  - [x] 新建 `docs/api/ws-message-registry.md`（头部 source-of-truth 说明 + envelope shapes + 3 条消息 sections）
  - [x] 更新 `docs/backend-architecture-guide.md` §12 加 "Message Registry" 段落指向 `internal/dto/ws_messages.go`
- [x] **Task 10 — PR checklist + build 绿** (AC: #14)
  - [x] `bash scripts/build.sh --test` 本地绿
  - [x] 自查 `backend-architecture-guide.md` §19 每条都过

## Dev Notes

### 核心约束（必读）

- **本 story 必须遵守 `docs/backend-architecture-guide.md` 全文**；若发现现有代码与指南冲突，报告但**不要擅自改**（保留为 code-review 讨论点）。
- **分层单向依赖**（architecture §Constitution 行 194-218）：handler → service → repository → infra；本 story handler 层不跨级访问 Redis / Mongo —— 只读 `dto.WSMessages` 和 `clockx.Clock`。
- **M9 Clock 强制**：业务代码禁直接 `time.Now()`；`serverTime` 必须走 `clockx.Clock.Now()`。此 story 是 Clock interface 的**第一个**被验证使用点（Story 0.7 epics line 581 明文指定 0.14 负责验证）。
- **M1 Package 命名**：`dto` 单数小写——ok；`handler` 单数小写——ok；避免 `platformhandlers.go`，用 `platform_handler.go`（与 `health_handler.go` 一致）。

### 架构 / 文件分布（检查前置条件）

- `internal/dto/` **本 story 首次创建**（`ls server/internal/` 目前无 `dto/`，architecture §Project Structure 行 867 已预留）。Task 1 确认目录不存在再 mkdir。
- `internal/handler/` 已存在（`health_handler.go` + tests since Story 0.4）；Task 3 在同级新增 `platform_handler.go`，不要创建 `platform/` 子目录（M1 拒绝 `platformhandlers/` 之类嵌套）。
- `docs/api/` **本 story 首次创建**（`find docs/api/ -type f` 返回空）。Task 8/9 一并 mkdir。
- `cmd/cat/wire.go` 当前仅 3 条路由（`/healthz /readyz /ws`）；扩展为 4 条。
- `cmd/cat/integration_test.go` 已存在（Story 0.4 整合测试 + Story 0.13 端到端）—— 扩展它，不要新建独立文件（避免 Testcontainers 启动次数翻倍）。

### 前置 Story 资产（0.9 ~ 0.13 可复用）

| 资产 | 来源 | 用法 |
|---|---|---|
| `ws.Dispatcher` + `d.types map[string]bool` | 0.9 / 0.10 | AC2 `RegisteredTypes()` 读此字段 |
| `ws.Dispatcher.Register / RegisterDedup` | 0.9 / 0.10 | AC4 测试直接调用构造场景 |
| `ws.Client`, `ws.Envelope`, `ws.HandlerFunc` | 0.9 | AC4 `noopHandler` 用其签名 |
| `clockx.Clock` + `RealClock` + `FakeClock` | 0.7 | AC3 handler 依赖 / AC12 Fake 时间断言 |
| `handler.HealthHandler` 样板 | 0.4 | AC3 `PlatformHandler` 构造/方法模式直接对标 |
| `cmd/cat/initialize.go` DI 风格（≤200 行、显式装配） | 0.2 | Task 4 扩展不得破坏 |
| `cmd/cat/wire.go` `buildRouter` 结构 | 0.2 | Task 4 最小增量 |
| `scripts/build.sh --test` 管线 | 0.1 | Task 8 新增 `validate_openapi` 嵌入此脚本 |

### 前置 Story 关键教训（reviews）

- **Story 0.12 review round 1**：release 模式未注册 `session.resume` 的原因是 Empty providers 返回 `nil/[]`，客户端无法区分"新账号"与"bug"；现决策是 release 模式完全不挂 `session.resume` 路由。本 story AC1 `DebugOnly: true` 标注与此一致；AC5 release 模式过滤也必须过滤此项。
- **Story 0.13 review round 1/2**：`detached writeCtx` 修复要点是关机飞行中消息不能丢。本 story 不涉及异步消费，但 AC11 "handler 不做额外 INFO log" 的决策呼应 0.13 教训——不要为 checklist 一致性而加噪日志。
- **Story 0.10 review round 2**：dedup key 改 length-prefix 编码防分隔符歧义 —— 本 story 常量 `Type` 必须符合 P3 `^[a-z0-9]+(\.[a-z0-9]+)*$` 正则（AC12 断言），提前锁死不留分隔符歧义空间。
- **Story 0.6 error-codes registry**：单元测试扫描注册表 + init 启动检查的 double-gate 模式；本 story AC4 consistency test + AC15 `validateRegistryConsistency` fail-fast 同 double-gate 结构。不要简化为单层。

### 测试纪律（P6）

- **Table-driven**：每个 TestXxx 以 `tests := []struct{name string; ...}{...}` 结构；每 case `t.Run(tt.name, ...)`。
- **`t.Parallel()`**：单元测试默认开（M11）；本 story 所有 `_test.go` 属于单元层，无需 Testcontainers（miniredis 都不需要），一律 `t.Parallel()`。
- **`testify`**：`require.NoError` / `require.Panics` 用于前置；`assert.Equal` / `assert.ElementsMatch` 用于验证。
- **`errors.Is/As`**（M12）：本 story 不产生业务错误，无需。
- **Fake Clock**：`clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 34, 56, 0, time.UTC))` 固定起点；handler 测试断言 `serverTime == "2026-04-18T12:34:56Z"`。
- **`bodyclose`** linter（.golangci.yml）：集成测试 `httptest.ResponseRecorder` 不涉及 body close 语义——但真 HTTP client 调用（Story 0.13 风格）必须 defer close；本 story 没有 real HTTP client，跳过。

### 预期 File List（事前列出 —— dev agent 应核对）

**New:**
- `server/internal/dto/ws_messages.go`
- `server/internal/dto/ws_messages_test.go`
- `server/internal/handler/platform_handler.go`
- `server/internal/handler/platform_handler_test.go`
- `server/cmd/cat/initialize_test.go`
- `docs/api/openapi.yaml`
- `docs/api/ws-message-registry.md`

**Modified:**
- `server/internal/ws/dispatcher.go` (+`RegisteredTypes()`)
- `server/internal/ws/dispatcher_test.go` (+`TestDispatcher_RegisteredTypes`)
- `server/cmd/cat/initialize.go` (+`handler.NewPlatformHandler` 装配, +`validateRegistryConsistency` call)
- `server/cmd/cat/wire.go` (+`platform` field, +route line)
- `server/cmd/cat/integration_test.go` (+`TestWSRegistryEndpoint`)
- `server/scripts/build.sh` (+`validate_openapi`) — 注意脚本位于 `server/scripts/` 抑或仓库根 `scripts/`；先 `ls scripts/` 再 `ls server/scripts/` 确认位置。
- `docs/backend-architecture-guide.md` (+"Message Registry" §12 callout)

**Totally new dirs** (mkdir before first file touch):
- `server/internal/dto/`
- `docs/api/`

### 禁止事项 / 常见误区

- **不要**把 `WSRegistryResponse` 放进 `internal/dto` —— M8 明确 HTTP 响应结构属于 handler 层；`dto` 包当前只托管 WS 消息元数据，未来再容纳 `AppError` 注册表。混放会导致后续 `internal/ws` import `dto` 时拉进 Gin 间接依赖（编译不挂但破坏分层单向）。
- **不要**在 `dto.WSMessages` 里塞"未来的"消息类型（例如 `blindbox.redeem`）。AC4 测试保证 dispatcher 实际注册 == 常量集；提前声明会使 release 模式测试 fail。
- **不要**把 `ping` / `pong` 应用层心跳加入本 story。架构审阅确认"心跳 `ping/pong` 走 WebSocket 协议层"（Story 0.9 Completion Notes），应用层 `ping`（FR59 implicit）将在 Epic 4 连接活性验证 story 中引入；本 story 不超前。
- **不要**给 `/v1/platform/ws-registry` 加缓存 header（`Cache-Control`）。客户端 Watch / iPhone 升级场景每次启动新版本才拉，无高频访问；加了 stale cache 反而使"客户端看到过期的 version 清单"成为 bug 孵化温床。
- **不要**在 handler 里 log full response —— 架构 §P5 要求 camelCase field + 精确场景；本 endpoint 每秒 0-10 QPS 量级，double-log 纯属噪音。
- **不要**用 `fmt.Sprintf(time.RFC3339)` —— 用 `h.clock.Now().UTC().Format(time.RFC3339)`。`RealClock.Now()` 已经 `.UTC()`（pkg/clockx/clock.go line 13）但加一次显式 `.UTC()` 不错（FakeClock 用户可能传 local time）。

### Project Structure Notes

- 统一遵循 `docs/backend-architecture-guide.md` §3 目录结构 + architecture §Project Structure 行 815-983。
- 本 story 首次落地 `internal/dto/` —— 与架构图一致，无变体（architecture §Project Structure 行 867 留空待填，本 story 填入第一行）。
- 本 story 首次落地 `docs/api/` —— 与架构图一致（architecture §Project Structure 行 963）。
- 无冲突或变体；若发现工程中存在隐藏的 `dto/` 文件冲突（e.g. 零散的 `internal/dto.go` 等），STOP 并上升到 code-review。

### References

- [Source: `docs/backend-architecture-guide.md`#3 目录结构] — `internal/dto/` 位置确认
- [Source: `docs/backend-architecture-guide.md`#6.1 Handler] — `PlatformHandler` 结构与构造模式
- [Source: `docs/backend-architecture-guide.md`#12 WebSocket] — envelope / message 字段约定
- [Source: `docs/backend-architecture-guide.md`#19 新代码检查清单] — AC14 PR gate
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#P3 WebSocket Message 命名, 行 526-534] — `domain.action` 规则 + `.result` 后缀 + envelope id 非 UUID 强制
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#D1 WebSocket Hub 结构, 行 276-291] — Broadcaster 接口签名 + Hub 最小骨架
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#Project Structure 行 867-870] — `internal/dto/ws_messages.go` 预留
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#Project Structure 行 963-964] — `docs/api/openapi.yaml` + `ws-message-registry.md` 预留
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#Gap Analysis G2, 行 1187-1189] — CI `swagger validate` + unit test 自动核对
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#P4 Error Classification, 行 535-558] — AppError Category（本 story 未触发）
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#P5 Logging, 行 560-575] — camelCase 字段 + 禁用 fmt.Printf
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#M9 FakeClock, 行 M9] — Clock injection 禁用直接 time.Now()
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#M13 PII, 行 676-679] — 本 endpoint 无 PII
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#Bootstrap endpoints, 行 814] — `/v1/platform/ws-registry` 不挂 JWT
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Story 0.14, 行 708-726] — 全部 AC 直接来源
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Story 0.7 行 581] — "Story 0.14 endpoint 时间戳必须用 Clock" 验证绑定
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Story 0.9 行 604-627] — Dispatcher 未知 type → UNKNOWN_MESSAGE_TYPE（本 story 不改）
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Story 0.10 行 628-646] — `requiresDedup` 语义来自此
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Story 0.12 行 665-683] — `session.resume` 不走 dedup
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Story 0.15 行 727-747] — 本 story 是 0.15 硬前置
- [Source: `server/_bmad-output/planning-artifacts/prd.md`#Versioning Strategy] — `v1` → `v2` 30 天过渡
- [Source: `server/_bmad-output/implementation-artifacts/0-9-ws-hub-skeleton-envelope-broadcaster-interface.md`#Dev Agent Record] — Envelope / Response / Push 结构
- [Source: `server/_bmad-output/implementation-artifacts/0-10-ws-upstream-eventid-idempotent-dedup.md`#Dev Agent Record] — Dispatcher `types` map 来源
- [Source: `server/_bmad-output/implementation-artifacts/0-12-session-resume-cache-throttle.md`#AC7] — release 模式不注册 session.resume 的理由
- [Source: `server/internal/ws/dispatcher.go` 行 16-64] — `types` map + Register / RegisterDedup 实现细节（本 story AC2 依赖）
- [Source: `server/cmd/cat/initialize.go` 行 112-138] — 当前 dispatcher 注册场景（AC4 测试必须镜像）
- [Source: `server/cmd/cat/wire.go` 行 17-32] — `handlers` struct + `buildRouter`（AC5 扩展点）
- [Source: `server/internal/handler/health_handler.go`] — `PlatformHandler` 的形态/构造/方法签名模板

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash scripts/build.sh --test` — 本地绿。go vet + time.Now 守卫 + build + 全量 `go test ./...` 均通过。
- `go vet ./...` — clean。
- OpenAPI 校验：首轮按 AC9 文本调用 `go run github.com/go-swagger/go-swagger/cmd/swagger@v0.31.0 validate docs/api/openapi.yaml`，工具返回 `.servers in body is a forbidden property / .components in body is a forbidden property / .swagger in body is required` — go-swagger v0.31.0 仅支持 Swagger 2.0，不支持 OpenAPI 3.0.3。改为 Go test 结构校验（见 Completion Notes）。
- race 测试：本地 Windows 机器 `cgo.exe exit status 2`（CGO 未就绪），非代码问题；CI（Linux）可覆盖。

### Completion Notes List

**实现摘要**

1. **dto 包落地 WSMessages 常量表**（AC1/AC10）：`internal/dto/ws_messages.go` 定义 `WSDirection` / `WSMessageMeta` / `WSMessages` / `WSMessagesByType`，三条初始 entries（session.resume / debug.echo / debug.echo.dedup）均 `DebugOnly: true`。`WSMessagesByType` 在 init 阶段 panic 去重。
2. **Dispatcher 暴露 `RegisteredTypes()`**（AC2）：读 `d.types` → copy → sort，godoc 说明"只允许 Hub.Start 前调用"。
3. **PlatformHandler**（AC3/5/6/11）：`internal/handler/platform_handler.go` 提供 `WSRegistry`；mode != "debug" 时过滤 `DebugOnly` 条目；`serverTime` 取自 `clock.Now().UTC().Format(time.RFC3339)`；nil clock panic；nil-slice-safe（`make([]..., 0, N)` 保证 JSON 为 `[]` 而非 `null`）。
4. **wire.go + initialize.go 装配 + fail-fast**（AC5/6/15）：`handlers` struct 增 `platform` 字段；`buildRouter` 在 `/healthz /readyz` 后挂 `/v1/platform/ws-registry`，明确注释"不挂 JWT"；`initialize.go` 在返回 `*App` 前调 `validateRegistryConsistency(dispatcher, cfg.Server.Mode)`，漂移 `log.Fatal`。
5. **dto drift 单元测试**（AC4/12）：`ws_messages_test.go` 用 **外部测试包 `package dto_test`** 避免 `internal/ws → internal/dto` 循环导入（原 story 指定 `package dto` 会因循环导入编译失败，这是唯一偏离且严格合规）。覆盖 AllFieldsPopulated / NoDuplicates / DuplicatePanics / Consistency_{Debug,Release}Mode。
6. **PlatformHandler 单元测试**（AC12）：debug/release/FakeClock 注入/nil-clock panic / `"messages":[]` 非 null 断言。
7. **集成测试**（AC9/12）：
   - `cmd/cat/ws_registry_test.go`（新建独立文件，无 `//go:build integration` tag）：三组 sub-test 覆盖 release/debug/FakeClock。为兼容 `buildRouter` 签名，非 platform handler（health, wsUpgrade）用 nil 依赖构造（方法不调用，仅完成路由注册）。
   - `cmd/cat/initialize_test.go`（新建）：覆盖 `validateRegistryConsistency` 的 DebugModeFullyRegistered / ReleaseModeNothingRegistered / UnknownRegisteredFails / DebugOnlyInReleaseFails。
   - `cmd/cat/openapi_spec_test.go`（新建）：取代 AC9 原本的 swagger CLI 调用（见下）。
8. **OpenAPI 占位**（AC8）：`docs/api/openapi.yaml` 3.0.3 文本，含 `/v1/platform/ws-registry` GET + WSRegistryResponse/WSRegistryMessage schemas，字段严格 camelCase 匹配 wire shape。
9. **CI 校验偏离（AC9 唯一实质偏离）**：AC9 指定 `go run github.com/go-swagger/go-swagger/cmd/swagger@v0.31.0 validate docs/api/openapi.yaml`，实测 go-swagger v0.31.0 CLI 仅支持 Swagger 2.0（反馈 `.swagger in body is required` / `.openapi in body is a forbidden property`），与 AC8 要求的 OpenAPI 3.0.3 直接冲突。两条路：(a) 降级 spec 为 Swagger 2.0（违反 AC8）；(b) 换工具（新增依赖需用户审批）。选 (c)：在 Go test lane 用 `gopkg.in/yaml.v3`（已为间接依赖，无新增）做结构校验——`TestOpenAPISpec_StructurallyValid` + `TestOpenAPISpec_SchemaFieldsMatchWireShape` 验证 openapi 版本格式、关键 path/schema 存在、`apiVersion/serverTime/messages/type/version/direction/requiresAuth/requiresDedup` 字段名与 `json` tag 对齐。同一 CI 严重度（test failure ≡ build failure），无工具链新增。`scripts/build.sh` 的 `validate_openapi` 函数已移除，并附注释说明。**标记为 review 讨论点，让 reviewer 拍板是否允许此偏离或要求改用 kin-openapi 等新依赖。**
10. **人类可读注册表 + 架构指南更新**（AC7/13）：
    - `docs/api/ws-message-registry.md`：source-of-truth 说明 + envelope shapes（up/down/push 三种）+ version strategy + 3 条消息 sections + "新增消息四步走"流程。
    - `docs/backend-architecture-guide.md` §12 增 "§12.1 Message Registry" 段落指向 dto、registry.md、双 gate 守门机制、四步走流程。TOC 顶部 link 未改（原文件无 TOC）。
11. **PR checklist §19 自审**（AC14）：全部 14 条通过。无 `fmt.Printf`/`log.Printf`；Handler 不持有 mongo/redis client；无 `context.TODO()`/`context.Background()`；公开成员都有英文 godoc；对应 `*_test.go` 齐；`bash scripts/build.sh --test` 绿；`// TODO` 仅 ws-message-registry.md 一条引用"未来 story"，符合 AC14 允许。

**Story 0.7 AC 绑定验证**：`TestPlatformHandler_WSRegistry_ServerTimeUsesClock` + `TestWSRegistryEndpoint_ServerTimeUsesInjectedClock` 注入 FakeClock 断言 serverTime，完成 Story 0.7 epics line 581 对 0.14 的 Clock 真实使用验证要求。

**决策记录**：`internal/dto` 目录本 story 并非首次创建（Story 0.6 AppError 注册表已落地），但 `ws_messages.go` / `WSMessages` 为本 story 首引入，符合架构 §Project Structure line 867 的"dto 包容纳 WS 消息常量 + AppError 注册表"预期。

### File List

**New:**

- `server/internal/dto/ws_messages.go`
- `server/internal/dto/ws_messages_test.go`
- `server/internal/handler/platform_handler.go`
- `server/internal/handler/platform_handler_test.go`
- `server/cmd/cat/initialize_test.go`
- `server/cmd/cat/ws_registry_test.go`
- `server/cmd/cat/openapi_spec_test.go`
- `docs/api/openapi.yaml`
- `docs/api/ws-message-registry.md`

**Modified:**

- `server/internal/ws/dispatcher.go` — 追加 `RegisteredTypes()` 方法 + `sort` import
- `server/internal/ws/dispatcher_test.go` — 追加 `TestDispatcher_RegisteredTypes`
- `server/cmd/cat/initialize.go` — imports 加 `dto`/`fmt`/`sort`；注入 `platform` handler；`validateRegistryConsistency` fail-fast；helper 函数追加到文件尾部
- `server/cmd/cat/wire.go` — `handlers` struct 增 `platform` 字段；`buildRouter` 挂 `/v1/platform/ws-registry`
- `scripts/build.sh` — 说明性注释解释 OpenAPI 校验改走 Go test lane（原 go-swagger CLI 调用已移除）
- `docs/backend-architecture-guide.md` — §12.1 Message Registry 段落新增

**Sprint status:**

- `server/_bmad-output/implementation-artifacts/sprint-status.yaml` — 0-14 状态推进 ready-for-dev → in-progress → review

## Change Log

| 日期 | 版本 | 变更 | 作者 |
|---|---|---|---|
| 2026-04-18 | 0.14.0 | 首版实现：dto.WSMessages 常量表 + Dispatcher.RegisteredTypes + PlatformHandler + `/v1/platform/ws-registry` endpoint + CI drift 守门（dto 单测 + initialize fail-fast + OpenAPI 结构测试） + 文档（ws-message-registry.md + openapi.yaml + arch guide §12.1）。Story 0.7 Clock 绑定验证完成。唯一偏离：AC9 swagger CLI 校验因 go-swagger v0.31.0 不支持 OpenAPI 3.0.3，改为 Go test lane 结构校验，标 review 讨论。 | claude-opus-4-7[1m] |
