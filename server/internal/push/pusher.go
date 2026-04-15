// Package push abstracts push notification delivery. The APNs
// implementation is provided but intentionally returns
// ErrNotImplemented until Story 5-6 wires real credentials.
package push

import (
	"context"
	"errors"
)

// ErrNotImplemented signals that the push backend is known but not
// operational yet. Callers must treat this as a soft failure and log,
// not abort, the parent business flow.
var ErrNotImplemented = errors.New("push: not implemented")

// Payload is the minimal APNs-style payload used by the cat backend.
// Larger structures are translated by the concrete Pusher.
type Payload struct {
	Title    string
	Body     string
	Category string
	Data     map[string]any
}

// Pusher is the contract service code depends on. Services hold this
// interface, never a concrete APNs client.
type Pusher interface {
	Send(ctx context.Context, deviceToken string, payload Payload) error
}

// Config mirrors config.APNsCfg without importing it.
type Config struct {
	KeyID    string
	TeamID   string
	BundleID string
	KeyPath  string
}

// APNsPusher is the production Pusher. Story 5-6 will flesh out the
// real sideshow/apns2 client; the skeleton preserves the wiring and
// error contract today.
type APNsPusher struct {
	cfg Config
}

// NewAPNsPusher validates cfg surface and returns a Pusher.
func NewAPNsPusher(cfg Config) *APNsPusher {
	return &APNsPusher{cfg: cfg}
}

// Send is a placeholder that returns ErrNotImplemented. It intentionally
// does NOT panic so that the calling flow can degrade gracefully.
func (p *APNsPusher) Send(ctx context.Context, deviceToken string, payload Payload) error {
	if deviceToken == "" {
		return errors.New("push: empty device token")
	}
	return ErrNotImplemented
}

// NullPusher is a silent implementation used in tests and in local
// development when APNs credentials are unavailable.
type NullPusher struct{}

// Send records nothing and returns nil.
func (NullPusher) Send(ctx context.Context, deviceToken string, payload Payload) error {
	return nil
}
