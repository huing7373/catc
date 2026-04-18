package redisx

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// DedupResult captures the outcome of a WS handler invocation so that
// replayed envelopes with the same eventId return byte-identical responses.
//
// Defined in pkg/redisx (and not internal/ws) because Go uses nominal typing
// for interface parameters, and the RedisDedupStore methods must reference the
// same concrete type the ws.DedupStore interface declares. internal/ws re-
// exports DedupResult via a type alias so business code treats it as a
// ws-layer value while the physical location respects the pkg → internal
// dependency direction.
type DedupResult struct {
	OK           bool
	Payload      json.RawMessage
	ErrorCode    string
	ErrorMessage string
}

// ToHash serializes the result into a Redis hash (all values as strings).
func (r DedupResult) ToHash() map[string]string {
	h := map[string]string{
		"ok":           boolToStr(r.OK),
		"errorCode":    r.ErrorCode,
		"errorMessage": r.ErrorMessage,
	}
	if len(r.Payload) > 0 {
		h["payloadJSON"] = string(r.Payload)
	} else {
		h["payloadJSON"] = ""
	}
	return h
}

// DedupResultFromHash rehydrates a DedupResult from a HGETALL map.
func DedupResultFromHash(m map[string]string) DedupResult {
	r := DedupResult{
		OK:           m["ok"] == "true",
		ErrorCode:    m["errorCode"],
		ErrorMessage: m["errorMessage"],
	}
	if p := m["payloadJSON"]; p != "" {
		r.Payload = json.RawMessage(p)
	}
	return r
}

func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// RedisDedupStore implements the WS dispatcher's dedup write-path using Redis.
//
// Keys (separated from lock:cron:* / ratelimit:* / presence:* / blacklist:*
// per D16):
//   - event:{eventID}         → string "processing" | "done"  (SETNX marker)
//   - event_result:{eventID}  → hash{ok,payloadJSON,errorCode,errorMessage}
//
// Both keys share the same TTL.
type RedisDedupStore struct {
	cmd redis.Cmdable
	ttl time.Duration
}

// NewDedupStore constructs a RedisDedupStore. Panics if ttl <= 0 to prevent
// accidentally storing keys forever.
func NewDedupStore(cmd redis.Cmdable, ttl time.Duration) *RedisDedupStore {
	if ttl <= 0 {
		panic("redisx.NewDedupStore: ttl must be > 0")
	}
	return &RedisDedupStore{cmd: cmd, ttl: ttl}
}

// Acquire attempts SET event:{eventID} "processing" NX EX ttl.
func (s *RedisDedupStore) Acquire(ctx context.Context, eventID string) (bool, error) {
	return s.cmd.SetNX(ctx, "event:"+eventID, "processing", s.ttl).Result()
}

// StoreResult writes the result hash, sets its TTL, and marks the event key
// as "done" (preserving TTL). Uses a pipeline — transactional atomicity isn't
// required because the hash write is idempotent and the "done" marker is a
// monotonic state transition.
func (s *RedisDedupStore) StoreResult(ctx context.Context, eventID string, result DedupResult) error {
	h := result.ToHash()
	hashArgs := make([]any, 0, len(h)*2)
	for k, v := range h {
		hashArgs = append(hashArgs, k, v)
	}

	pipe := s.cmd.Pipeline()
	pipe.HSet(ctx, "event_result:"+eventID, hashArgs...)
	pipe.Expire(ctx, "event_result:"+eventID, s.ttl)
	pipe.Set(ctx, "event:"+eventID, "done", s.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// GetResult loads the cached result; found=false when no hash exists (either
// never written or expired).
func (s *RedisDedupStore) GetResult(ctx context.Context, eventID string) (DedupResult, bool, error) {
	m, err := s.cmd.HGetAll(ctx, "event_result:"+eventID).Result()
	if err != nil {
		return DedupResult{}, false, err
	}
	if len(m) == 0 {
		return DedupResult{}, false, nil
	}
	return DedupResultFromHash(m), true, nil
}
