package ws

import "golang.org/x/time/rate"

func newConnLimiter(ratePerSec int) *rate.Limiter {
	return rate.NewLimiter(rate.Limit(ratePerSec), ratePerSec)
}
