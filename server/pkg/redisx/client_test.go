package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestConnectAndHealthCheck_Miniredis(t *testing.T) {
	srv := miniredis.RunT(t)
	cli, err := Connect(Config{Addr: srv.Addr()})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := HealthCheck(ctx, cli); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestConnect_EmptyAddr(t *testing.T) {
	if _, err := Connect(Config{}); err == nil {
		t.Fatal("expected error for empty addr")
	}
}

func TestRunnable_FinalIdempotent(t *testing.T) {
	srv := miniredis.RunT(t)
	cli, err := Connect(Config{Addr: srv.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunnable(cli)
	ctx := context.Background()
	if err := r.Final(ctx); err != nil {
		t.Fatalf("Final #1: %v", err)
	}
	// Second call must not panic or fail.
	if err := r.Final(ctx); err != nil {
		t.Fatalf("Final #2: %v", err)
	}
}

func TestHealthCheck_NilClient(t *testing.T) {
	if err := HealthCheck(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil client")
	}
}
