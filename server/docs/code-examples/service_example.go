package examples

import (
	"context"
	"time"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/logx"
)

type ExampleService struct {
	clock clockx.Clock
}

func NewExampleService(clock clockx.Clock) *ExampleService {
	return &ExampleService{clock: clock}
}

func (s *ExampleService) ProcessItem(ctx context.Context, itemID string) (time.Time, error) {
	now := s.clock.Now()

	logx.Ctx(ctx).Info().
		Str("action", "processItem").
		Str("itemId", itemID).
		Time("timestamp", now).
		Msg("starting item processing")

	return now, nil
}
