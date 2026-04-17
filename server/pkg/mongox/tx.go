package mongox

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// WithTx wraps a function in a MongoDB transaction.
// The callback receives a session context that propagates the transaction.
func WithTx(ctx context.Context, cli *mongo.Client, fn func(ctx context.Context) error) error {
	sess, err := cli.StartSession()
	if err != nil {
		return err
	}
	defer sess.EndSession(context.Background())

	_, err = sess.WithTransaction(ctx, func(sc context.Context) (any, error) {
		return nil, fn(sc)
	})
	return err
}
