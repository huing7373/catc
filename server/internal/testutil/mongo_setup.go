package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// SetupMongo starts a MongoDB container via Testcontainers and returns
// a connected client plus a cleanup function. The caller must NOT call
// t.Parallel() — container ports can conflict.
func SetupMongo(t *testing.T) (*mongo.Client, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := mongodb.Run(ctx, "mongo:7")
	require.NoError(t, err)

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	cli, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	require.NoError(t, cli.Ping(ctx, nil))

	cleanup := func() {
		_ = cli.Disconnect(ctx)
		_ = container.Terminate(ctx)
	}
	return cli, cleanup
}
