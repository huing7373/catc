package redisx

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisBlacklist implements the device blacklist write path (Add/Remove/TTL)
// and read path (IsBlacklisted) using Redis string keys with TTL.
//
// Key space (separated from lock:cron:* / ratelimit:ws:* / event:* per D16):
//
//	blacklist:device:{userID}  →  "1"  (string with TTL)
//
// IsBlacklisted satisfies the ws.Blacklist interface via Go structural typing;
// the write methods are consumed directly by tools/blacklist_user and are
// deliberately *not* part of the ws interface — exposing them there would
// drag operational concerns into the protocol layer (P2).
type RedisBlacklist struct {
	cmd redis.Cmdable
}

// NewBlacklist constructs a RedisBlacklist. No TTL is baked in; Add takes a
// per-call TTL so the ops CLI can express "24h default" while future
// fraud-detection services can pass shorter windows.
func NewBlacklist(cmd redis.Cmdable) *RedisBlacklist {
	return &RedisBlacklist{cmd: cmd}
}

// blacklistKey builds the Redis key. userID is drawn from the JWT subject
// and has a controlled format (ObjectID hex / UUID / debug token — no ":"
// separators), so single-segment concatenation is collision-free and no
// length-prefix encoding is required (contrast with dedup keys, which join
// multiple free-form fields).
func blacklistKey(userID string) string {
	return "blacklist:device:" + userID
}

// IsBlacklisted returns true iff an un-expired entry exists for userID.
func (b *RedisBlacklist) IsBlacklisted(ctx context.Context, userID string) (bool, error) {
	n, err := b.cmd.Exists(ctx, blacklistKey(userID)).Result()
	if err != nil {
		return false, err
	}
	return n >= 1, nil
}

// Add records userID as blacklisted for ttl. ttl must be > 0 — permanent
// blacklists are rejected so every entry is auditable and eventually
// self-expiring (NFR-SEC-10).
func (b *RedisBlacklist) Add(ctx context.Context, userID string, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("blacklist: ttl must be > 0")
	}
	return b.cmd.Set(ctx, blacklistKey(userID), "1", ttl).Err()
}

// Remove clears a blacklist entry. Missing keys are a no-op (DEL returns 0
// without error).
func (b *RedisBlacklist) Remove(ctx context.Context, userID string) error {
	return b.cmd.Del(ctx, blacklistKey(userID)).Err()
}

// TTL reports the remaining lifetime of a blacklist entry.
//
// Return shape: (ttl, exists, err)
//   - exists=false, ttl=0: no entry (DEL / TTL expired / never set)
//   - exists=true, ttl>0: normal case
//   - exists=true, ttl=0: entry exists with no TTL (Redis returns -1); this
//     should not happen via Add but is surfaced as TTL=0/exists=true so
//     callers can still detect the blacklisted state.
func (b *RedisBlacklist) TTL(ctx context.Context, userID string) (time.Duration, bool, error) {
	d, err := b.cmd.TTL(ctx, blacklistKey(userID)).Result()
	if err != nil {
		return 0, false, err
	}
	// go-redis v9 TTL sentinel values (raw time.Duration of -2 / -1 ns,
	// not seconds): -2 = key missing, -1 = key exists with no TTL.
	switch d {
	case time.Duration(-2):
		return 0, false, nil
	case time.Duration(-1):
		return 0, true, nil
	default:
		return d, true, nil
	}
}
