package examples

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

func TestExampleService_ProcessItem(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	fc := clockx.NewFakeClock(fixedTime)
	svc := NewExampleService(fc)

	got, err := svc.ProcessItem(context.Background(), "item-123")
	require.NoError(t, err)
	assert.Equal(t, fixedTime, got)

	fc.Advance(30 * time.Second)

	got2, err := svc.ProcessItem(context.Background(), "item-456")
	require.NoError(t, err)
	assert.Equal(t, fixedTime.Add(30*time.Second), got2)
}
