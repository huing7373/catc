package mongox

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// WithTx runs fn inside a MongoDB session-based transaction and returns
// the error produced by fn. The sessCtx passed to fn carries the session
// and should be propagated to every repository call that must join the
// transaction.
//
// Usage:
//
//	err := mongox.WithTx(ctx, cli, func(sessCtx context.Context) error {
//	    if err := repoA.Save(sessCtx, a); err != nil { return err }
//	    return repoB.Save(sessCtx, b)
//	})
func WithTx(ctx context.Context, cli *mongo.Client, fn func(sessCtx context.Context) error) error {
	sess, err := cli.StartSession()
	if err != nil {
		return err
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(sessCtx context.Context) (any, error) {
		return nil, fn(sessCtx)
	})
	return err
}
