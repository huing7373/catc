// Package push — production ApnsSender implementation (AC3 / AC14).
//
// apnsClient wraps github.com/sideshow/apns2 (NFR-INT-2). It is constructed
// once at startup (AC15) with the team's .p8 token-auth key and is shared
// across all worker goroutines — apns2.Client is documented as safe for
// concurrent use over a single HTTP/2 connection.
package push

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

// apnsClient satisfies ApnsSender by delegating to apns2.Client.
// Exposed as *apnsClient (pointer) for structural-typing satisfaction,
// same pattern as RedisDedupStore / RedisResumeCache.
type apnsClient struct {
	inner *apns2.Client
}

// NewApnsClient loads the .p8 token-auth key and constructs an APNs
// client pointed at either the production or sandbox endpoint. production
// is `true` when cfg.Server.Mode == "release".
//
// Validation: keyPath / keyID / teamID must all be non-empty. Missing .p8
// file or invalid PEM bubble the underlying error wrapped with context.
//
// Never logs the private key material. The startup log line surfaces mode,
// keyID, teamID (none of which is secret per M14) plus the two topic
// strings so an operator can see which topology is active.
func NewApnsClient(keyPath, keyID, teamID string, production bool) (*apnsClient, error) {
	if keyPath == "" {
		return nil, fmt.Errorf("apns: key_path must not be empty")
	}
	if keyID == "" {
		return nil, fmt.Errorf("apns: key_id must not be empty")
	}
	if teamID == "" {
		return nil, fmt.Errorf("apns: team_id must not be empty")
	}

	authKey, err := token.AuthKeyFromFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("load apns key: %w", err)
	}
	tok := &token.Token{
		AuthKey: authKey,
		KeyID:   keyID,
		TeamID:  teamID,
	}

	client := apns2.NewTokenClient(tok)
	mode := "development"
	if production {
		client = client.Production()
		mode = "production"
	} else {
		client = client.Development()
	}

	log.Info().
		Str("action", "apns_client_init").
		Str("mode", mode).
		Str("keyId", keyID).
		Str("teamId", teamID).
		Msg("apns client initialised")

	return &apnsClient{inner: client}, nil
}

// Send delegates to apns2.Client.PushWithContext so the provided ctx
// bounds the HTTP/2 request. If ctx is already cancelled we short-circuit
// rather than opening a new stream.
func (c *apnsClient) Send(ctx context.Context, n *apns2.Notification) (*apns2.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.inner.PushWithContext(ctx, n)
}
