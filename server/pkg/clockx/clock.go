package clockx

import "time"

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func NewRealClock() *RealClock { return &RealClock{} }

func (RealClock) Now() time.Time { return time.Now().UTC() }

type FakeClock struct {
	now time.Time
}

func NewFakeClock(initial time.Time) *FakeClock {
	return &FakeClock{now: initial}
}

func (c *FakeClock) Now() time.Time { return c.now }

func (c *FakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }
