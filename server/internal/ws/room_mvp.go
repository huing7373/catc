// Package ws — room_mvp.go hosts the Story 10.1 联调 MVP room layer.
//
// 3 DebugOnly messages: room.join / action.update / action.broadcast are
// bound in cmd/cat/initialize.go's debug branch. Together with the in-memory
// RoomManager below, they give an Apple Watch + iPhone client enough WS
// surface to exercise end-to-end 加入房间 / 上传当前动作 / 下发当前动作 on a
// real device today, without waiting for the full Epic 4 presence / room
// persistence / 4-person cap / disconnect-grace / cross-instance broadcast
// stack.
//
// EXPECTED to be deleted wholesale when Epic 4.1 ships. Epic 4.1 will land a
// new internal/room/ domain package with persistence + 4-person cap + D8
// disconnect grace + session.resume integration + RedisPubSub cross-instance
// broadcast. NOT a seed for Epic 4 — a proper implementation must be
// written from scratch, because everything intentionally skipped below
// lives outside this file and has no partial draft to migrate. Do not
// flip the DebugOnly flag to "promote" this file; `git rm` it instead.
//
// Intentionally not implemented here (→ Epic 4):
//   - persistence (Mongo room collection + roomservice domain → Epic 4.2)
//   - 4-person cap (→ Epic 4.2)
//   - D8 disconnect grace period (→ Epic 4.1 core)
//   - session.resume room snapshot integration (→ Epic 4.5)
//   - cross-instance broadcast via RedisPubSub (→ Epic 4.3)
//   - member.join / member.leave / friend.online / friend.offline pushes
//     (→ Epic 4.1 / 4.4)
//
// The only long-lived contribution from this story is ws.ClientObserver +
// Hub.AddObserver + Hub.notifyDisconnect in hub.go — Epic 4.1 Presence will
// be the second consumer of that same hook, so those extensions stay when
// room_mvp.go is removed.
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
)

// maxFieldBytes bounds room IDs, user IDs, and action strings. UTF-8 byte
// length rather than rune count: the downstream storage is the in-memory map,
// but we still want to refuse "Unicode bombs" at ingest so future
// persistence does not inherit unbounded values.
const maxFieldBytes = 64

// Member is a room participant as tracked in memory.
type Member struct {
	UserID       UserID
	ConnID       ConnID
	LastAction   string // empty ⇒ user has never called action.update
	LastActionTs int64  // Unix ms; 0 ⇒ user has never called action.update
}

// Room is the in-memory container for a set of members. members is keyed by
// UserID; switching rooms is a delete-then-insert, handled by RoomManager
// under its own write lock.
type Room struct {
	ID      RoomID
	members map[UserID]*Member
}

// RoomManager owns the in-memory rooms map and the two lookup indexes
// (userLoc, connMap) that the disconnect path consults without needing a
// full scan. All maps are protected by a single RWMutex; read handlers take
// the read lock, write handlers (join / update / disconnect) take the write
// lock.
type RoomManager struct {
	clock       clockx.Clock
	broadcaster Broadcaster

	mu      sync.RWMutex
	rooms   map[RoomID]*Room
	userLoc map[UserID]RoomID
	connMap map[ConnID]UserID
}

// NewRoomManager constructs the manager. Panics on nil clock or broadcaster
// per §P3 fail-fast startup — in debug mode these are always supplied from
// initialize.go, so a nil here is a programmer error, not a runtime concern.
func NewRoomManager(clock clockx.Clock, broadcaster Broadcaster) *RoomManager {
	if clock == nil {
		panic("ws.NewRoomManager: clock is required")
	}
	if broadcaster == nil {
		panic("ws.NewRoomManager: broadcaster is required")
	}
	return &RoomManager{
		clock:       clock,
		broadcaster: broadcaster,
		rooms:       make(map[RoomID]*Room),
		userLoc:     make(map[UserID]RoomID),
		connMap:     make(map[ConnID]UserID),
	}
}

// joinRequest is the wire shape of the `room.join` request payload
// (iOS JSONDecoder camelCase convention — matches AC1).
type joinRequest struct {
	RoomID string `json:"roomId"`
}

// memberSnapshot is one element of the `room.join.result` members array.
// Self is excluded from the snapshot by the handler — see AC3 step 3.
type memberSnapshot struct {
	UserID       string `json:"userId"`
	LastAction   string `json:"action"`
	LastActionTs int64  `json:"tsMs"`
}

// joinResponse is the wire shape returned by HandleJoin. members is always
// serialized as an array (possibly empty) — never nil — so iOS JSONDecoder
// has a stable shape to decode against (delegated Story 0.14 AC decision).
type joinResponse struct {
	RoomID  string           `json:"roomId"`
	Members []memberSnapshot `json:"members"`
}

// actionUpdateRequest is the `action.update` request payload shape.
type actionUpdateRequest struct {
	Action string `json:"action"`
}

// actionBroadcastPush is the `action.broadcast` downstream push payload.
type actionBroadcastPush struct {
	UserID string `json:"userId"`
	Action string `json:"action"`
	TsMs   int64  `json:"tsMs"`
}

// HandleJoin implements the room.join RPC. See AC3 for the four-step flow.
func (rm *RoomManager) HandleJoin(ctx context.Context, client *Client, env Envelope) (json.RawMessage, error) {
	var req joinRequest
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, validationError("invalid room.join payload")
		}
	}
	if err := validateField(req.RoomID, "roomId"); err != nil {
		return nil, err
	}

	userID := client.UserID()
	connID := client.ConnID()

	rm.mu.Lock()
	// Step 1: if the user is already in another room, leave it. Remember
	// to drop the previous connMap entry too — otherwise a stale
	// OnDisconnect on the old conn would find userLoc still pointing at
	// the new room and evict the user under their new active conn.
	if oldRoomID, ok := rm.userLoc[userID]; ok && oldRoomID != RoomID(req.RoomID) {
		if oldRoom, exists := rm.rooms[oldRoomID]; exists {
			if oldMember, has := oldRoom.members[userID]; has && oldMember.ConnID != connID {
				delete(rm.connMap, oldMember.ConnID)
			}
		}
		rm.leaveRoomLocked(userID, oldRoomID)
	}

	// Step 2: ensure the target room exists and the user is a member.
	roomID := RoomID(req.RoomID)
	room, ok := rm.rooms[roomID]
	if !ok {
		room = &Room{ID: roomID, members: make(map[UserID]*Member)}
		rm.rooms[roomID] = room
	}
	if existing, already := room.members[userID]; !already {
		room.members[userID] = &Member{
			UserID:       userID,
			ConnID:       connID,
			LastAction:   "",
			LastActionTs: 0,
		}
	} else {
		// Idempotent same-room join — refresh the connID (client may have
		// reconnected while we slept) but preserve LastAction state. Also
		// purge the previous connMap entry: leaving it behind lets the old
		// conn's eventual OnDisconnect look up userLoc, find the (still
		// populated) room, and evict the user's new active session.
		if existing.ConnID != connID {
			delete(rm.connMap, existing.ConnID)
		}
		existing.ConnID = connID
	}
	rm.userLoc[userID] = roomID
	rm.connMap[connID] = userID

	// Step 3: snapshot other members (exclude self). Build under the lock;
	// the slice is about to escape to the handler caller but its entries
	// are copies, not pointers into the map.
	snapshot := make([]memberSnapshot, 0, len(room.members))
	for uid, m := range room.members {
		if uid == userID {
			continue
		}
		snapshot = append(snapshot, memberSnapshot{
			UserID:       string(uid),
			LastAction:   m.LastAction,
			LastActionTs: m.LastActionTs,
		})
	}
	rm.mu.Unlock()

	// Step 4: respond with a nil-safe members array (iOS decode stability).
	resp := joinResponse{
		RoomID:  string(roomID),
		Members: snapshot,
	}
	log.Ctx(ctx).Debug().
		Str("room_id", string(roomID)).
		Str("user_id", string(userID)).
		Int("peers", len(snapshot)).
		Msg("room.join")

	return json.Marshal(resp)
}

// HandleActionUpdate implements action.update. See AC4 for the five-step flow.
// Self receives no action.broadcast (strict filter in step 3).
func (rm *RoomManager) HandleActionUpdate(ctx context.Context, client *Client, env Envelope) (json.RawMessage, error) {
	var req actionUpdateRequest
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, validationError("invalid action.update payload")
		}
	}
	if err := validateField(req.Action, "action"); err != nil {
		return nil, err
	}

	userID := client.UserID()

	rm.mu.Lock()
	roomID, inRoom := rm.userLoc[userID]
	if !inRoom {
		rm.mu.Unlock()
		return nil, validationError("user not in any room")
	}
	room, ok := rm.rooms[roomID]
	if !ok {
		// Defensive — userLoc references a nonexistent room. Clean up and
		// surface the same error the client sees when they never joined.
		delete(rm.userLoc, userID)
		rm.mu.Unlock()
		return nil, validationError("user not in any room")
	}
	member, ok := room.members[userID]
	if !ok {
		delete(rm.userLoc, userID)
		rm.mu.Unlock()
		return nil, validationError("user not in any room")
	}

	nowMs := rm.clock.Now().UnixMilli()
	member.LastAction = req.Action
	member.LastActionTs = nowMs

	// Step 3: snapshot other members' UserIDs for broadcast (copy out; do
	// not hold the lock across broadcaster calls).
	others := make([]UserID, 0, len(room.members))
	for uid := range room.members {
		if uid == userID {
			continue
		}
		others = append(others, uid)
	}
	rm.mu.Unlock()

	// Step 4: fan out. Marshal the push body once, dispatch to every peer.
	// Use BroadcastToUser rather than BroadcastToRoom — the latter is a D6
	// no-op that Epic 4.3 will wire up; touching it here would poach that
	// story's scope. N ≤ 4 in practice makes O(N) fan-out a non-concern.
	pushBody := actionBroadcastPush{
		UserID: string(userID),
		Action: req.Action,
		TsMs:   nowMs,
	}
	payload, err := json.Marshal(pushBody)
	if err != nil {
		// json.Marshal on a plain struct with string/int64 fields cannot
		// fail in practice; log and continue so the ack still ships.
		log.Ctx(ctx).Error().Err(err).Msg("action.broadcast: marshal push body failed")
		return json.Marshal(struct{}{})
	}
	push := NewPush("action.broadcast", payload)
	pushBytes, err := json.Marshal(push)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("action.broadcast: marshal envelope failed")
		return json.Marshal(struct{}{})
	}
	for _, peer := range others {
		if err := rm.broadcaster.BroadcastToUser(ctx, peer, pushBytes); err != nil {
			log.Ctx(ctx).Warn().Err(err).
				Str("peer_user_id", string(peer)).
				Msg("action.broadcast: BroadcastToUser failed")
		}
	}

	// Step 5: empty ack.
	return json.Marshal(struct{}{})
}

// OnDisconnect implements ClientObserver. Called by Hub after a client's
// connection is removed from the registry (at-most-once per client).
func (rm *RoomManager) OnDisconnect(connID ConnID, userID UserID) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Cross-check: connMap must agree with the userID the Hub passed.
	// Divergence would mean two clients sharing a connID, which the Hub's
	// sync.Map invariants forbid — but guard anyway so the delete cannot
	// clobber a concurrent (connID, user') bookkeeping entry.
	if tracked, ok := rm.connMap[connID]; ok && tracked != userID {
		// Keep connMap honest; do not touch room state for a stranger.
		delete(rm.connMap, connID)
		return
	}
	delete(rm.connMap, connID)

	roomID, inRoom := rm.userLoc[userID]
	if !inRoom {
		// Client disconnected without ever joining a room — nothing to do.
		return
	}
	// Guard against stale disconnects: if the user has since rejoined
	// under a new conn, the room's Member.ConnID reflects the new conn,
	// not the one we were told just disconnected. Evicting here would
	// clobber an active session and force the client to rejoin before
	// peers see their actions again.
	room, ok := rm.rooms[roomID]
	if !ok {
		delete(rm.userLoc, userID)
		return
	}
	member, ok := room.members[userID]
	if !ok {
		delete(rm.userLoc, userID)
		return
	}
	if member.ConnID != connID {
		return
	}
	rm.leaveRoomLocked(userID, roomID)
}

// leaveRoomLocked removes user from the given room and GC's the room if it
// becomes empty. MUST be called with rm.mu held for writing. Does NOT touch
// rm.connMap — disconnect/join paths handle that separately because they
// have different authoritative sources.
func (rm *RoomManager) leaveRoomLocked(userID UserID, roomID RoomID) {
	room, ok := rm.rooms[roomID]
	if !ok {
		delete(rm.userLoc, userID)
		return
	}
	delete(room.members, userID)
	delete(rm.userLoc, userID)
	if len(room.members) == 0 {
		delete(rm.rooms, roomID)
	}
}

// validateField rejects empty-or-oversized UTF-8 strings. Keeping the
// message identical across fields (only the field name varies) makes wire
// errors easy to correlate against client validation.
func validateField(s, name string) error {
	if s == "" {
		return validationError(name + " required")
	}
	if len(s) > maxFieldBytes {
		return validationError(fmt.Sprintf("%s exceeds %d bytes", name, maxFieldBytes))
	}
	if !utf8.ValidString(s) {
		return validationError(name + " must be valid UTF-8")
	}
	return nil
}

// validationError wraps dto.ErrValidationError with a message that
// identifies the offending field / reason. Reuses the existing error code
// (does not pollute the Story 0.6 registry with new codes).
func validationError(msg string) error {
	e := *dto.ErrValidationError
	e.Message = msg
	return &e
}

// Interface assertions keep signatures honest against the ws contracts. If
// HandleJoin/HandleActionUpdate ever drift from HandlerFunc, or if
// RoomManager drifts from ClientObserver, these lines fail to compile at
// the nearest `go build`.
var (
	_ HandlerFunc    = (*RoomManager)(nil).HandleJoin
	_ HandlerFunc    = (*RoomManager)(nil).HandleActionUpdate
	_ ClientObserver = (*RoomManager)(nil)
)
