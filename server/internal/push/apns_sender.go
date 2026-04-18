// Package push — ApnsSender abstraction (AC3).
//
// The interface is the seam tests use to fake APNs without opening an
// HTTP/2 connection. apnsClient (in apns_client.go) is the production
// implementation wrapping github.com/sideshow/apns2 per NFR-INT-2.
package push

import (
	"context"

	"github.com/sideshow/apns2"
)

// ApnsSender sends a single Notification to APNs and returns the response
// or a transport error.
//
// Context cancellation MUST be honoured — the worker's graceful shutdown
// depends on in-flight sends returning promptly when the parent ctx is
// cancelled (Story 0.13 App.Final 5 s budget).
type ApnsSender interface {
	Send(ctx context.Context, n *apns2.Notification) (*apns2.Response, error)
}
