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

### room.join (bi, v1, auth required)

**MVP only — to be removed wholesale when Epic 4.1 ships.**

Client joins a room by id. The server adds the caller to its in-memory `RoomManager`, evicting them from any previous room they occupied. The response payload carries a snapshot of all **other** members currently in the room (self excluded). No `member.join` broadcast is emitted — Epic 4.1's full presence stack will handle join/leave fan-out.

Dedup: not required.

**Request payload:**

```json
{ "roomId": "<string, 1-64 bytes>" }
```

**Response payload:**

```json
{
  "roomId":  "<string>",
  "members": [
    { "userId": "<string>", "action": "<string, possibly empty>", "tsMs": <int64, 0 if no action yet> }
  ]
}
```

`members` is always serialized as an array (possibly empty `[]`), never `null`.

**Errors:** `VALIDATION_ERROR` — `roomId required` | `roomId exceeds 64 bytes` | `roomId must be valid UTF-8` | `invalid room.join payload`.

### action.update (up, v1, auth required)

**MVP only — to be removed wholesale when Epic 4.1 ships.**

Client publishes its current action string to the room it currently occupies. The server stores the value (for subsequent `room.join` snapshots) and fans out an `action.broadcast` push to every other member. The caller receives an empty ack, not a broadcast of its own message.

Dedup: not required (this story has no idempotency guarantee — clients may replay, and every replay rebroadcasts).

**Request payload:**

```json
{ "action": "<string, 1-64 bytes>" }
```

**Response payload:** `{}` (empty object).

**Errors:**

- `VALIDATION_ERROR` — `action required` | `action exceeds 64 bytes` | `action must be valid UTF-8` | `invalid action.update payload`
- `VALIDATION_ERROR` — `user not in any room` (client called `action.update` before `room.join`)

### action.broadcast (down, v1, auth required)

**MVP only — to be removed wholesale when Epic 4.1 ships.**

Server → client push carrying another member's latest action. Fanned out from `action.update` to every room peer excluding the sender. `action.broadcast` is **never** upstream — clients must not send it, and the dispatcher has no handler for it.

**Push payload:**

```json
{
  "userId": "<string>",
  "action": "<string>",
  "tsMs":   <int64, server clock.Now().UnixMilli() at the moment action.update fired>
}
```

No response shape — downstream pushes carry no `id` and no `ok` field (see **Envelope Shapes → Downstream push** above).
