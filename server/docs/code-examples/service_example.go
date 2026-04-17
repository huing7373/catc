package examples

import (
	"context"

	"github.com/huing/cat/server/pkg/logx"
)

// ExampleService demonstrates structured logging in a service layer function.
//
//	The context carries requestId and userId injected by middleware.
//	Service code uses logx.Ctx(ctx) to obtain the enriched logger.
func ExampleService(ctx context.Context, itemID string) error {
	logx.Ctx(ctx).Info().
		Str("action", "processItem").
		Str("itemId", itemID).
		Msg("starting item processing")

	// business logic ...

	logx.Ctx(ctx).Info().
		Str("action", "processItem").
		Str("itemId", itemID).
		Msg("item processed")

	return nil
}
