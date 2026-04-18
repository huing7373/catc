# WS Message Type Registry

> **Source of truth:** `server/internal/dto/ws_messages.go` (Go constants + metadata).
>
> **CI drift guards:**
> - `server/internal/dto/ws_messages_test.go` — compile-time invariants + dispatcher parity (Story 0.14 AC4/AC12)
> - `server/cmd/cat/initialize_test.go` — `validateRegistryConsistency` fail-fast (Story 0.14 AC15)
> - `server/cmd/cat/ws_registry_test.go` — `TestWSRegistryEndpoint_*` wire shape (Story 0.14 AC9)
>
> **Adding a new message** (keep these four in lock-step — out-of-order
> edits fail CI by design):
>
> 1. Append an entry to `dto.WSMessages` in `server/internal/dto/ws_messages.go`.
> 2. Register the handler in `server/cmd/cat/initialize.go` (`dispatcher.Register` or `dispatcher.RegisterDedup`).
> 3. Add a `### <type>` section to this file under **Message Types**.
> 4. Run `bash scripts/build.sh --test` — green before PR.
>
> <!-- TODO: regenerate from WSMessages when a future story lands the codegen script. -->

## Envelope Shapes

**Upstream request** (client → server):

```json
{
  "id": "<client-generated unique id>",
  "type": "<domain.action>",
  "payload": { ... }
}
```

**Downstream response** (server → client, in reply to upstream request):

```json
{
  "id": "<echo of request id>",
  "ok": true,
  "type": "<domain.action>.result",
  "payload": { ... }
}
```

or on error:

```json
{
  "id": "<echo>",
  "ok": false,
  "type": "<domain.action>.result",
  "error": { "code": "UPPER_SNAKE", "message": "..." }
}
```

**Downstream push** (server → client, unsolicited):

```json
{
  "type": "<domain.action>",
  "payload": { ... }
}
```

## Version Strategy

- All messages MVP-ship at `version: "v1"`.
- Breaking changes coexist as a new entry with the same `type` and `version: "v2"`, with a 30-day transition period during which both are accepted. See Story 0.14 AC10.

## Message Types

> Note: every type listed below is currently `DebugOnly` — they are registered by `cmd/cat/initialize.go` only when `[server].mode = "debug"`. Release-mode clients receive an empty `messages` array from `GET /v1/platform/ws-registry`. Epic 1+ stories remove the `DebugOnly` flag as real providers replace the Empty\*Provider stubs.

### session.resume (bi, v1, auth required)

Client-initiated RPC asking the server for a full session snapshot after reconnect. Response payload is a 6-field composite cached 60s (Story 0.12):

- `user` — profile & preferences
- `friends` — friend list with online/offline
- `cat_state` — current FSM state + step counters
- `skins` — owned + equipped skin ids
- `blindboxes` — pending rewards
- `room` — current room snapshot (if any)

Dedup: **not** required (idempotent read).

**Request payload:** empty or omitted.

**Response payload:**

```json
{
  "user":       { ... },
  "friends":    [ ... ],
  "catState":   { ... },
  "skins":      { ... },
  "blindboxes": [ ... ],
  "room":       { ... } | null
}
```

### debug.echo (bi, v1, auth required)

Debug-only: server echoes the request payload verbatim. No business effect; used to validate round-trip framing.

Dedup: not required.

**Request payload:** arbitrary JSON.

**Response payload:** identical to request payload.

### debug.echo.dedup (bi, v1, auth required)

Debug-only: same behavior as `debug.echo`, but the dispatcher wraps the handler in the dedup middleware (`RegisterDedup`). Replaying the same `envelope.id` within the configured TTL returns the cached first-invocation result without re-invoking the handler. Used to validate the idempotency write-path.

Dedup: **required**.

**Request payload:** arbitrary JSON.

**Response payload:** first-invocation payload, cached.
