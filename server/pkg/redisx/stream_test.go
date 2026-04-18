package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamPusher_XAddReturnsID(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)

	p := NewStreamPusher(cmd, "apns:queue")
	id, err := p.XAdd(context.Background(), map[string]string{
		"userId": "u1",
		"msg":    `{"hello":1}`,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, id, "XADD should return a non-empty ID")
}

func TestStreamConsumer_EnsureGroup_IdempotentOnBusygroup(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	ctx := context.Background()

	c := NewStreamConsumer(cmd, "apns:queue", "apns_workers", "w-1", 100*time.Millisecond, 10)
	require.NoError(t, c.EnsureGroup(ctx), "first EnsureGroup must succeed")
	require.NoError(t, c.EnsureGroup(ctx), "second EnsureGroup must be idempotent (BUSYGROUP tolerated)")
}

func TestStreamConsumer_ReadAndAck_RoundTrip(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	ctx := context.Background()

	c := NewStreamConsumer(cmd, "apns:queue", "apns_workers", "w-1", 200*time.Millisecond, 10)
	require.NoError(t, c.EnsureGroup(ctx))

	p := NewStreamPusher(cmd, "apns:queue")
	_, err := p.XAdd(ctx, map[string]string{"msg": "hello"})
	require.NoError(t, err)

	msgs, err := c.Read(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "hello", msgs[0].Values["msg"])

	require.NoError(t, c.Ack(ctx, msgs[0].ID))
}

func TestStreamConsumer_ReadBlockTimeout_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	ctx := context.Background()

	c := NewStreamConsumer(cmd, "apns:queue", "apns_workers", "w-1", 50*time.Millisecond, 10)
	require.NoError(t, c.EnsureGroup(ctx))

	msgs, err := c.Read(ctx)
	require.NoError(t, err, "BLOCK timeout must surface as nil-error empty-slice")
	assert.Empty(t, msgs)
}
