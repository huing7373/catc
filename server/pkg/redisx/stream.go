// Package redisx — Redis primitive wrappers shared across the server.
//
// stream.go provides thin helpers around Redis Streams so that future
// stream-based queues (APNs push in Story 0.13, notification recall, analytics
// etc.) can all reuse the same ergonomics without re-implementing
// XADD / XGROUP / XREADGROUP / XACK call shapes.
//
// # Design
//
// These types are intentionally thin: no dedup, no idempotency, no
// retry/DLQ — those are queue-specific semantics that belong in the
// consumer (e.g. internal/push/apns_worker.go). This file only offers the
// Redis command façade.
//
// # D16 Redis key space (PRD §Redis Key Convention)
//
// Stream keys coexist with — and are kept strictly separate from — other
// Redis namespaces used by the server:
//
//	apns:queue           → Stream (APNs enqueue, Story 0.13)
//	apns:dlq             → Stream (APNs DLQ)
//	apns:retry           → ZSET   (APNs scheduled retries)
//	apns:idem:{key}      → String (APNs enqueue dedup)
//	event:{eventID}      → String (WS dedup marker)
//	event_result:{id}    → Hash   (WS dedup result)
//	resume_cache:{user}  → Hash   (session.resume cache)
//	ratelimit:ws:{user}  → ZSET   (WS connect rate-limit)
//	blacklist:device:{d} → String (device blacklist)
//	lock:cron:{name}     → String (distributed cron lock)
//	refresh_blacklist:{} → String (refresh-token revocation)
//	presence:*           → Hash   (room presence — Epic 4)
//	state:*              → Hash   (cat state — Epic 2)
//
// All APNs stream keys share the `apns:` prefix; callers that instantiate
// this helper must pass validated key strings (no `:` in arbitrary user
// input). See D16 invariants.
package redisx

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// StreamPusher produces entries into a single Redis Stream key.
//
// Intentionally holds no dedup / idempotency state: the caller (e.g.
// RedisStreamsPusher in internal/push/pusher.go) layers those semantics on
// top of XAdd.
type StreamPusher struct {
	cmd       redis.Cmdable
	streamKey string
}

// NewStreamPusher constructs a StreamPusher. Panics on nil cmd or empty
// streamKey — these are startup invariants, not runtime conditions, so a
// clear boot-time crash is preferable to a lazy Enqueue failure.
func NewStreamPusher(cmd redis.Cmdable, streamKey string) *StreamPusher {
	if cmd == nil {
		panic("redisx.NewStreamPusher: cmd must not be nil")
	}
	if streamKey == "" {
		panic("redisx.NewStreamPusher: streamKey must not be empty")
	}
	return &StreamPusher{cmd: cmd, streamKey: streamKey}
}

// XAdd appends an entry to the stream using XADD with the server-generated
// ID (`*`). Returns the assigned entry ID or a non-nil error on Redis
// failure. values is the Redis field-value map — callers encode their
// payload as they see fit (JSON in a single field, multiple flat fields,
// etc.).
func (s *StreamPusher) XAdd(ctx context.Context, values map[string]string) (string, error) {
	args := make(map[string]any, len(values))
	for k, v := range values {
		args[k] = v
	}
	return s.cmd.XAdd(ctx, &redis.XAddArgs{
		Stream: s.streamKey,
		ID:     "*",
		Values: args,
	}).Result()
}

// StreamConsumer wraps the XREADGROUP read-ack loop for a single
// (stream, group, consumer) triple.
type StreamConsumer struct {
	cmd       redis.Cmdable
	streamKey string
	group     string
	consumer  string
	block     time.Duration
	count     int64
}

// NewStreamConsumer constructs a StreamConsumer. Panics on invalid
// arguments — consumer groups require all four identifiers to be set, and
// non-positive block / count would produce a hot loop or a useless read.
func NewStreamConsumer(cmd redis.Cmdable, streamKey, group, consumer string, block time.Duration, count int64) *StreamConsumer {
	if cmd == nil {
		panic("redisx.NewStreamConsumer: cmd must not be nil")
	}
	if streamKey == "" {
		panic("redisx.NewStreamConsumer: streamKey must not be empty")
	}
	if group == "" {
		panic("redisx.NewStreamConsumer: group must not be empty")
	}
	if consumer == "" {
		panic("redisx.NewStreamConsumer: consumer must not be empty")
	}
	if block <= 0 {
		panic("redisx.NewStreamConsumer: block must be > 0")
	}
	if count <= 0 {
		panic("redisx.NewStreamConsumer: count must be > 0")
	}
	return &StreamConsumer{
		cmd:       cmd,
		streamKey: streamKey,
		group:     group,
		consumer:  consumer,
		block:     block,
		count:     count,
	}
}

// EnsureGroup creates the consumer group (with MKSTREAM so the stream
// itself is created on demand) starting at `$` — only new entries after
// the group's creation are delivered to consumers. If the group already
// exists, Redis returns `BUSYGROUP Consumer Group name already exists`
// which we treat as success (idempotent startup across restarts and
// multi-replica deploys).
func (c *StreamConsumer) EnsureGroup(ctx context.Context) error {
	err := c.cmd.XGroupCreateMkStream(ctx, c.streamKey, c.group, "$").Err()
	if err != nil && strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

// Read blocks for up to the configured block duration waiting for up to
// count new entries. On block-timeout the underlying client returns
// redis.Nil — we translate that to a nil-error empty-slice return so
// callers can safely loop without special-casing redis.Nil.
func (c *StreamConsumer) Read(ctx context.Context) ([]redis.XMessage, error) {
	res, err := c.cmd.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.group,
		Consumer: c.consumer,
		Streams:  []string{c.streamKey, ">"},
		Count:    c.count,
		Block:    c.block,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	for _, s := range res {
		if s.Stream == c.streamKey {
			return s.Messages, nil
		}
	}
	return nil, nil
}

// Ack acknowledges a previously-read entry so Redis removes it from the
// pending-entries list (PEL) for this consumer group.
func (c *StreamConsumer) Ack(ctx context.Context, id string) error {
	return c.cmd.XAck(ctx, c.streamKey, c.group, id).Err()
}
