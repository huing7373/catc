package mongox

import (
	"context"
	"testing"
)

type runnable interface {
	Name() string
	Start(ctx context.Context) error
	Final(ctx context.Context) error
}

func TestClient_ImplementsRunnable(t *testing.T) {
	t.Parallel()
	var _ runnable = (*Client)(nil)
}

func TestClient_Name(t *testing.T) {
	t.Parallel()
	c := &Client{}
	if c.Name() != "mongo" {
		t.Errorf("expected Name() = %q, got %q", "mongo", c.Name())
	}
}

func TestClient_Start_Noop(t *testing.T) {
	t.Parallel()
	c := &Client{}
	if err := c.Start(context.Background()); err != nil {
		t.Errorf("expected Start() = nil, got %v", err)
	}
}
