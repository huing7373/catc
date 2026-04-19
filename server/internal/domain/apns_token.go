package domain

import (
	"time"

	"github.com/huing/cat/server/pkg/ids"
)

// ApnsToken is the authoritative binding between (userId, platform) and
// a concrete APNs device token. Story 1.4 writes one row per (userId,
// platform) pair into the `apns_tokens` Mongo collection —
// re-registration by the same device class overwrites updated_at +
// device_token; cross-platform tokens coexist (a single user on Watch +
// iPhone has two rows).
//
// DeviceToken is ALWAYS plaintext at the domain layer. The repository
// encrypts on write (NFR-SEC-7, AES-GCM via pkg/cryptox) and decrypts on
// read. Callers that ever log this value MUST route through
// logx.MaskAPNsToken — DEBUG-level only, first 8 chars + "..." everywhere
// else.
type ApnsToken struct {
	UserID      ids.UserID
	Platform    ids.Platform
	DeviceToken string
	UpdatedAt   time.Time
}
