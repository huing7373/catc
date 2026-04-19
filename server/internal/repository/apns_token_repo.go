package repository

// Import direction invariant: repository → push (for push.TokenInfo shape).
// push MUST NOT import internal/repository (would create a cycle and couple
// the push platform to the user DB schema). If a future story needs push
// to invoke repo methods, declare a consumer-side interface in push/
// (matching the TokenProvider / TokenDeleter / TokenCleaner pattern in
// internal/push/providers.go) and let the repo satisfy it structurally —
// do NOT reverse the direction. AC14 grep gate:
//
//	! grep -rE "\"github.com/huing/cat/server/internal/repository\"" internal/push/

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/push"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/cryptox"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/logx"
)

// Sentinel errors declared at the repo boundary so service callers can
// branch via errors.Is without importing pkg/cryptox.
var (
	// ErrApnsTokenNotFound is returned by lookup methods that expect a
	// row to exist. Delete / DeleteExpired treat missing rows as a no-op
	// (nil) per the push.TokenDeleter contract, so they never surface
	// this error.
	ErrApnsTokenNotFound = errors.New("apns_token repo: not found")
	// ErrApnsTokenCipherTampered wraps cryptox.ErrCipherTampered. Today
	// it only appears inside ListByUserID / Delete in warn logs (a
	// tampered row is skipped, not returned); the sentinel is exported
	// so future callers that choose to bubble the error can still pattern
	// match via errors.Is.
	ErrApnsTokenCipherTampered = errors.New("apns_token repo: cipher tampered or wrong key")
)

const apnsTokensCollection = "apns_tokens"

// apnsTokenDoc is the BSON shape for one row in the `apns_tokens`
// collection. DeviceToken holds the AES-GCM sealed envelope (nonce |
// ciphertext | tag); plaintext never touches Mongo.
type apnsTokenDoc struct {
	UserID      string    `bson:"user_id"`
	Platform    string    `bson:"platform"`
	DeviceToken []byte    `bson:"device_token"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

// MongoApnsTokenRepository persists domain.ApnsToken with field-level
// AES-GCM encryption on device_token (NFR-SEC-7). A single repository
// instance also satisfies push.TokenProvider / TokenDeleter /
// TokenCleaner through the three adapter methods at the bottom of this
// file — cmd/cat/initialize.go injects `*MongoApnsTokenRepository`
// directly into push.NewAPNsRouter / APNsWorker / cron.NewScheduler,
// swapping out the Story 0.13 Empty* stubs (§21.2 Empty→Real).
type MongoApnsTokenRepository struct {
	coll   *mongo.Collection
	clock  clockx.Clock
	sealer *cryptox.AESGCMSealer
}

// NewMongoApnsTokenRepository constructs the repo. sealer is required —
// nil would silently write plaintext tokens (NFR-SEC-7 violation);
// fail-fast at construction rather than per-Upsert so a wiring bug
// surfaces at startup.
func NewMongoApnsTokenRepository(db *mongo.Database, clk clockx.Clock, sealer *cryptox.AESGCMSealer) *MongoApnsTokenRepository {
	if db == nil {
		panic("repository.NewMongoApnsTokenRepository: db must not be nil")
	}
	if clk == nil {
		panic("repository.NewMongoApnsTokenRepository: clock must not be nil")
	}
	if sealer == nil {
		panic("repository.NewMongoApnsTokenRepository: sealer must not be nil")
	}
	return &MongoApnsTokenRepository{
		coll:   db.Collection(apnsTokensCollection),
		clock:  clk,
		sealer: sealer,
	}
}

// EnsureIndexes creates the (user_id, platform) unique compound index.
// Name pinned to `user_id_1_platform_1` per architecture §P1
// {field}_{order} rule — renaming forces a rebuild on next deploy.
// Idempotent: Mongo returns success when an index with the same key
// spec / name already exists.
func (r *MongoApnsTokenRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "user_id", Value: 1},
			{Key: "platform", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetName("user_id_1_platform_1"),
	})
	if err != nil {
		return fmt.Errorf("apns_token repo: ensure indexes: %w", err)
	}
	return nil
}

// Upsert writes or replaces the row for (t.UserID, t.Platform). The
// service layer is responsible for stamping t.UpdatedAt via its
// injected clock — the repo does not read clock itself, so a FakeClock
// service test drives UpdatedAt deterministically.
//
// The caller is expected to have DTO-validated the token, but the
// repo re-checks the non-empty fields and platform enum as
// defense-in-depth (review-antipatterns §4.1).
func (r *MongoApnsTokenRepository) Upsert(ctx context.Context, t *domain.ApnsToken) error {
	if t == nil {
		return errors.New("apns_token repo: upsert nil token")
	}
	if t.UserID == "" {
		return errors.New("apns_token repo: upsert empty user id")
	}
	if t.DeviceToken == "" {
		return errors.New("apns_token repo: upsert empty device token")
	}
	if t.Platform != ids.PlatformWatch && t.Platform != ids.PlatformIphone {
		return fmt.Errorf("apns_token repo: upsert unknown platform %q", t.Platform)
	}
	sealedBytes, err := r.sealer.Seal([]byte(t.DeviceToken))
	if err != nil {
		return fmt.Errorf("apns_token repo: seal: %w", err)
	}
	doc := apnsTokenDoc{
		UserID:      string(t.UserID),
		Platform:    string(t.Platform),
		DeviceToken: sealedBytes,
		UpdatedAt:   t.UpdatedAt,
	}
	filter := bson.M{"user_id": doc.UserID, "platform": doc.Platform}
	update := bson.M{"$set": doc}
	opts := options.UpdateOne().SetUpsert(true)
	if _, err := r.coll.UpdateOne(ctx, filter, update, opts); err != nil {
		return fmt.Errorf("apns_token repo: upsert: %w", err)
	}
	return nil
}

// ListByUserID returns every domain.ApnsToken for userID with
// DeviceToken fields decrypted to plaintext. Rows whose sealer.Open
// fails with ErrCipherTampered are logged at WARN and skipped —
// fail-open on a per-row basis (AC11) so a single poisoned row does
// not break the whole user's push pipeline. Any non-tamper decrypt
// error aborts the cursor and returns a wrapped error.
func (r *MongoApnsTokenRepository) ListByUserID(ctx context.Context, userID ids.UserID) ([]domain.ApnsToken, error) {
	if userID == "" {
		return nil, errors.New("apns_token repo: list: empty user id")
	}
	cursor, err := r.coll.Find(ctx, bson.M{"user_id": string(userID)})
	if err != nil {
		return nil, fmt.Errorf("apns_token repo: list: %w", err)
	}
	defer cursor.Close(ctx)

	out := make([]domain.ApnsToken, 0)
	for cursor.Next(ctx) {
		var doc apnsTokenDoc
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("apns_token repo: list decode: %w", err)
		}
		plaintext, err := r.sealer.Open(doc.DeviceToken)
		if err != nil {
			if errors.Is(err, cryptox.ErrCipherTampered) {
				logx.Ctx(ctx).Warn().
					Str("userId", doc.UserID).
					Str("platform", doc.Platform).
					Str("action", "apns_token_decrypt_tampered").
					Msg("apns_token_decrypt_tampered")
				continue
			}
			return nil, fmt.Errorf("apns_token repo: list decrypt: %w", err)
		}
		out = append(out, domain.ApnsToken{
			UserID:      ids.UserID(doc.UserID),
			Platform:    ids.Platform(doc.Platform),
			DeviceToken: string(plaintext),
			UpdatedAt:   doc.UpdatedAt,
		})
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("apns_token repo: list cursor: %w", err)
	}
	return out, nil
}

// Delete removes the row for (userID, plaintextToken). Implementation
// decrypts every row for the user and deletes the one whose plaintext
// matches; a Seal-based filter would never match because each Seal
// produces a fresh nonce. Missing row is a no-op (returns nil) per the
// push.TokenDeleter idempotency contract. A tampered sibling row is
// skipped with a warn log — it does NOT block deletion of the valid
// target row (AC3 Delete_SkipsTamperedRowDeletesValidSibling).
func (r *MongoApnsTokenRepository) Delete(ctx context.Context, userID ids.UserID, plaintextToken string) error {
	if userID == "" {
		return errors.New("apns_token repo: delete: empty user id")
	}
	if plaintextToken == "" {
		return errors.New("apns_token repo: delete: empty plaintext token")
	}
	cursor, err := r.coll.Find(ctx, bson.M{"user_id": string(userID)})
	if err != nil {
		return fmt.Errorf("apns_token repo: delete find: %w", err)
	}
	defer cursor.Close(ctx)

	type rowIdent struct {
		ID any `bson:"_id"`
		apnsTokenDoc
	}
	for cursor.Next(ctx) {
		var row rowIdent
		if err := cursor.Decode(&row); err != nil {
			return fmt.Errorf("apns_token repo: delete decode: %w", err)
		}
		plaintext, openErr := r.sealer.Open(row.DeviceToken)
		if openErr != nil {
			if errors.Is(openErr, cryptox.ErrCipherTampered) {
				logx.Ctx(ctx).Warn().
					Str("userId", row.UserID).
					Str("platform", row.Platform).
					Str("action", "apns_token_decrypt_tampered").
					Msg("apns_token_decrypt_tampered")
				continue
			}
			return fmt.Errorf("apns_token repo: delete decrypt: %w", openErr)
		}
		if string(plaintext) != plaintextToken {
			continue
		}
		if _, delErr := r.coll.DeleteOne(ctx, bson.M{"_id": row.ID}); delErr != nil {
			return fmt.Errorf("apns_token repo: delete: %w", delErr)
		}
		return nil
	}
	if err := cursor.Err(); err != nil {
		return fmt.Errorf("apns_token repo: delete cursor: %w", err)
	}
	return nil
}

// DeleteExpired bulk-removes rows with updated_at < cutoff. Does NOT
// decrypt — speed and privacy both benefit from never touching the
// device_token field. Returns the number of deleted rows; cron
// observability uses this (Story 0.13 `apns_token_cleanup` @daily job).
func (r *MongoApnsTokenRepository) DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.coll.DeleteMany(ctx, bson.M{"updated_at": bson.M{"$lt": cutoff}})
	if err != nil {
		return 0, fmt.Errorf("apns_token repo: delete expired: %w", err)
	}
	return res.DeletedCount, nil
}

// --- push.TokenProvider / TokenDeleter / TokenCleaner adapters ---
//
// These three methods let *MongoApnsTokenRepository satisfy the
// Story 0.13 consumer-side interfaces in internal/push/providers.go.
// cmd/cat/initialize.go drops the repo directly into push.NewAPNsRouter
// (as TokenProvider), push.NewAPNsWorker (as TokenDeleter), and
// cron.NewScheduler (as TokenCleaner), swapping out the Empty* stubs
// from Epic 0 (§21.2).

// ListTokens implements push.TokenProvider by projecting each
// domain.ApnsToken to push.TokenInfo (plaintext DeviceToken + Platform
// string). Behavior matches ListByUserID: tampered rows are skipped
// with a warn log; an error aborts.
func (r *MongoApnsTokenRepository) ListTokens(ctx context.Context, userID ids.UserID) ([]push.TokenInfo, error) {
	toks, err := r.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]push.TokenInfo, 0, len(toks))
	for _, t := range toks {
		out = append(out, push.TokenInfo{
			Platform:    string(t.Platform),
			DeviceToken: t.DeviceToken,
		})
	}
	return out, nil
}

