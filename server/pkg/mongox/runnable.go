package mongox

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Runnable wraps a *mongo.Client so the App container can Final() it
// during graceful shutdown. Start is a no-op because the connection is
// established in MustConnect.
type Runnable struct {
	cli *mongo.Client
}

// NewRunnable returns a lifecycle adapter for cli.
func NewRunnable(cli *mongo.Client) *Runnable { return &Runnable{cli: cli} }

// Name identifies this component in shutdown logs.
func (r *Runnable) Name() string { return "mongo" }

// Start is a no-op. Mongo is already connected when the App starts.
func (r *Runnable) Start(ctx context.Context) error { return nil }

// Final disconnects the Mongo client. Safe to call multiple times.
func (r *Runnable) Final(ctx context.Context) error {
	if r.cli == nil {
		return nil
	}
	err := r.cli.Disconnect(ctx)
	r.cli = nil
	return err
}
