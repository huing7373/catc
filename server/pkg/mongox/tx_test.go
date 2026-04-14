package mongox

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestConnect_FailsFast_WithUnreachableURI ensures Connect returns an
// error (rather than blocking) when Mongo is unreachable. This runs in
// every CI environment regardless of whether real Mongo is available.
func TestConnect_FailsFast_WithUnreachableURI(t *testing.T) {
	_, err := Connect(Config{
		URI:        "mongodb://127.0.0.1:1", // deterministic unreachable port
		Database:   "cat_test",
		TimeoutSec: 1,
	})
	if err == nil {
		t.Fatal("expected error for unreachable mongo")
	}
}

// TestWithTx_CommitsAndRollsBack requires a real Mongo instance with
// replica set support. We skip unless CAT_TEST_MONGO_URI is set.
func TestWithTx_CommitsAndRollsBack(t *testing.T) {
	uri := os.Getenv("CAT_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("CAT_TEST_MONGO_URI not set — skipping real-mongo transaction test")
	}
	cli, err := Connect(Config{URI: uri, Database: "cat_test", TimeoutSec: 5})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Commit path.
	err = WithTx(ctx, cli, func(sc context.Context) error { return nil })
	if err != nil {
		t.Errorf("commit path: unexpected error: %v", err)
	}

	// Rollback path.
	sentinel := errTestRollback
	err = WithTx(ctx, cli, func(sc context.Context) error { return sentinel })
	if err == nil {
		t.Error("expected propagated error from rollback path, got nil")
	}
}

var errTestRollback = &testErr{"rollback"}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }

func TestRedactURI(t *testing.T) {
	cases := map[string]string{
		"mongodb://user:pass@host/db": "mongodb://****@host/db",
		"mongodb://host/db":           "mongodb://host/db",
		"plain-string":                "plain-string",
	}
	for in, want := range cases {
		if got := redactURI(in); got != want {
			t.Errorf("redactURI(%q) = %q, want %q", in, got, want)
		}
	}
}
