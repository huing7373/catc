package clockx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ Clock = (*RealClock)(nil)
	_ Clock = (*FakeClock)(nil)
)

func TestFakeClock(t *testing.T) {
	t.Parallel()

	initial := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		advances []time.Duration
		want     time.Time
	}{
		{
			name:     "initial time equals constructor arg",
			advances: nil,
			want:     initial,
		},
		{
			name:     "advance 15s",
			advances: []time.Duration{15 * time.Second},
			want:     initial.Add(15 * time.Second),
		},
		{
			name:     "multiple advances accumulate",
			advances: []time.Duration{10 * time.Second, 5 * time.Second},
			want:     initial.Add(15 * time.Second),
		},
		{
			name:     "advance zero is no-op",
			advances: []time.Duration{0},
			want:     initial,
		},
		{
			name:     "negative advance moves time backward",
			advances: []time.Duration{10 * time.Second, -5 * time.Second},
			want:     initial.Add(5 * time.Second),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fc := NewFakeClock(initial)
			for _, d := range tt.advances {
				fc.Advance(d)
			}
			assert.Equal(t, tt.want, fc.Now())
		})
	}
}

func TestRealClock_ReturnsUTC(t *testing.T) {
	t.Parallel()

	clk := NewRealClock()
	before := time.Now().UTC()
	got := clk.Now()
	after := time.Now().UTC()

	require.Equal(t, time.UTC, got.Location())
	assert.False(t, got.Before(before), "clock.Now() should not be before test start")
	assert.False(t, got.After(after), "clock.Now() should not be after test end")
}
