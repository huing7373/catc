//go:build integration

package mongox_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/huing/cat/server/pkg/mongox"
)

func TestMustConnect_Integration(t *testing.T) {
	ctx := context.Background()

	container, err := mongodb.Run(ctx, "mongo:7")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	cli := mongox.MustConnect(mongox.ConnectOptions{
		URI:        uri,
		DB:         "testdb",
		TimeoutSec: 10,
	})
	t.Cleanup(func() { _ = cli.Final(ctx) })

	t.Run("HealthCheck", func(t *testing.T) {
		err := cli.HealthCheck(ctx)
		assert.NoError(t, err)
	})

	t.Run("DB", func(t *testing.T) {
		db := cli.DB()
		require.NotNil(t, db)
		assert.Equal(t, "testdb", db.Name())
	})

	t.Run("Raw", func(t *testing.T) {
		raw := cli.Raw()
		require.NotNil(t, raw)
		err := raw.Ping(ctx, nil)
		assert.NoError(t, err)
	})
}

func TestWithTx_Integration(t *testing.T) {
	ctx := context.Background()

	container, err := mongodb.Run(ctx, "mongo:7", mongodb.WithReplicaSet("rs0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	uri, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	cli, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Disconnect(ctx) })

	err = mongox.WithTx(ctx, cli, func(sc context.Context) error {
		coll := cli.Database("testdb").Collection("test")
		_, err := coll.InsertOne(sc, map[string]string{"key": "value"})
		return err
	})
	assert.NoError(t, err)
}
