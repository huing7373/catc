package examples

import (
	"context"

	"github.com/huing/cat/server/internal/push"
	"github.com/huing/cat/server/pkg/ids"
)

// ExamplePusherUsage_Alert shows how a service sends an alert-kind APNs
// notification through the Pusher façade (Story 0.13). This is the
// pattern future Story 5.2 TouchService.SendTouch will follow exactly:
//
//	1. Service accepts a push.Pusher (interface) via DI.
//	2. Caller builds a PushPayload with Kind=Alert + Title (+ optional
//	   Body / DeepLink / RespectsQuietHours / IdempotencyKey).
//	3. Enqueue returns immediately after XADD — no waiting for APNs.
//
// The service remains fire-and-forget relative to APNs: a Mongo write
// should be committed BEFORE the Enqueue call so a txn abort cannot
// produce an orphan push (D10).
func ExamplePusherUsage_Alert(ctx context.Context, p push.Pusher, receiver ids.UserID) error {
	return p.Enqueue(ctx, receiver, push.PushPayload{
		Kind:               push.PushKindAlert,
		Title:              "Alice",
		Body:               "sent you a touch",
		DeepLink:           "cat://touch?from=user-456",
		RespectsQuietHours: true,          // honours FR30 per-receiver
		IdempotencyKey:     "touch_abc123", // 5-min SETNX dedupe (NFR-SEC-9)
	})
}

// ExamplePusherUsage_Silent shows the content-available path used by
// state-sync or background-refresh flows. Silent push sets PushType=
// background at the worker, no Title / Body / Sound — NFR-COMP-3 demands
// that background push carry exactly this shape.
func ExamplePusherUsage_Silent(ctx context.Context, p push.Pusher, receiver ids.UserID) error {
	return p.Enqueue(ctx, receiver, push.PushPayload{
		Kind:     push.PushKindSilent,
		DeepLink: "cat://state-sync",
	})
}
