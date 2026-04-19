package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

// Sentinel errors exported so callers can branch with errors.Is. Wrapping
// at the boundary (vs returning the raw mongo.ErrNoDocuments) keeps the
// service layer free of mongo-driver imports — M7 keeps repos as the
// sole holders of driver knowledge.
var (
	ErrUserNotFound      = errors.New("user repo: user not found")
	ErrUserDuplicateHash = errors.New("user repo: duplicate apple_user_id_hash")
	// ErrSessionStale signals that UpsertSessionIfJTIMatches observed a
	// session.current_jti different from the expected one — i.e. the
	// caller lost a rotation race against a concurrent /auth/refresh
	// (or the user document was absent). The service interprets this
	// as "your refresh token is no longer the live one; treat as
	// revoked" and returns AUTH_REFRESH_TOKEN_REVOKED without burning
	// any jti (no point — whoever won already wrote a new one).
	ErrSessionStale = errors.New("user repo: session current_jti no longer matches expected")
)

const usersCollection = "users"

// MongoUserRepository persists domain.User in the `users` Mongo
// collection. snake_case BSON field names are enforced via the
// per-field bson tags on domain.User; the unique index on
// apple_user_id_hash is the load-bearing constraint that lets Insert
// surface ErrUserDuplicateHash when two concurrent SignInWithApple
// requests for the same Apple account race.
type MongoUserRepository struct {
	coll  *mongo.Collection
	clock clockx.Clock
}

// NewMongoUserRepository wires the repository against an existing Mongo
// database handle. Clock is required (M9) — ClearDeletion uses it to
// stamp updated_at without falling back to time.Now() and tripping the
// build-script M9 guard.
func NewMongoUserRepository(db *mongo.Database, clk clockx.Clock) *MongoUserRepository {
	if db == nil {
		panic("repository.NewMongoUserRepository: db must not be nil")
	}
	if clk == nil {
		panic("repository.NewMongoUserRepository: clock must not be nil")
	}
	return &MongoUserRepository{coll: db.Collection(usersCollection), clock: clk}
}

// EnsureIndexes creates the apple_user_id_hash unique index. Re-runs are
// idempotent: Mongo's CreateOne returns success when an index with the
// same key spec / name already exists. Index name is pinned to
// `apple_user_id_hash_1` per architecture §P1 `{field}_{order}` rule —
// renaming would force a re-build on the next deploy.
func (r *MongoUserRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "apple_user_id_hash", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("apple_user_id_hash_1"),
	})
	if err != nil {
		return fmt.Errorf("user repo: ensure indexes: %w", err)
	}
	return nil
}

// FindByAppleHash returns the user matching hash, or
// (nil, ErrUserNotFound) if no document matched.
func (r *MongoUserRepository) FindByAppleHash(ctx context.Context, hash string) (*domain.User, error) {
	if hash == "" {
		return nil, errors.New("user repo: empty apple_user_id_hash")
	}
	var u domain.User
	err := r.coll.FindOne(ctx, bson.M{"apple_user_id_hash": hash}).Decode(&u)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("user repo: find by apple hash: %w", err)
	}
	return &u, nil
}

// FindByID returns the user matching id, or (nil, ErrUserNotFound) if
// no document matched.
func (r *MongoUserRepository) FindByID(ctx context.Context, id ids.UserID) (*domain.User, error) {
	if id == "" {
		return nil, errors.New("user repo: empty user id")
	}
	var u domain.User
	err := r.coll.FindOne(ctx, bson.M{"_id": string(id)}).Decode(&u)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("user repo: find by id: %w", err)
	}
	return &u, nil
}

// Insert writes u as a new document. Returns ErrUserDuplicateHash when
// the apple_user_id_hash unique index rejects the write — callers can
// resolve that race by re-reading via FindByAppleHash.
func (r *MongoUserRepository) Insert(ctx context.Context, u *domain.User) error {
	if u == nil {
		return errors.New("user repo: insert nil user")
	}
	if u.ID == "" {
		return errors.New("user repo: insert user with empty id")
	}
	if u.AppleUserIDHash == "" {
		return errors.New("user repo: insert user with empty apple_user_id_hash")
	}
	if u.Sessions == nil {
		// Mongo encodes nil maps as null; the seed contract is "{}".
		u.Sessions = map[string]domain.Session{}
	}
	_, err := r.coll.InsertOne(ctx, u)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrUserDuplicateHash
		}
		return fmt.Errorf("user repo: insert: %w", err)
	}
	return nil
}

// validateDeviceID is the repo-side defense-in-depth guard for
// sessions.<deviceId> Mongo paths. Real callers are DTO-validated
// (binding:"uuid") so this guard is never load-bearing; it exists so a
// future programmer error cannot smuggle "." or "$" into the dotted
// path and alter an unrelated field (review-antipatterns §8.2).
func validateDeviceID(deviceID string) error {
	if deviceID == "" {
		return errors.New("user repo: empty device id")
	}
	if strings.ContainsAny(deviceID, ".$") {
		return fmt.Errorf("user repo: device id contains reserved path characters: %q", deviceID)
	}
	return nil
}

// UpsertSession writes sessions.<deviceId>.current_jti +
// sessions.<deviceId>.issued_at via a dotted $set so other
// sessions.<deviceId>.* fields (has_apns_token, owned by Story 1.4)
// are untouched. updated_at is always refreshed via the injected
// clock. Returns ErrUserNotFound when no user document matched.
func (r *MongoUserRepository) UpsertSession(ctx context.Context, userID ids.UserID, deviceID string, s domain.Session) error {
	if userID == "" {
		return errors.New("user repo: upsert session: empty user id")
	}
	if err := validateDeviceID(deviceID); err != nil {
		return err
	}
	now := r.clock.Now()
	setDoc := bson.M{
		"sessions." + deviceID + ".current_jti": s.CurrentJTI,
		"sessions." + deviceID + ".issued_at":   s.IssuedAt,
		"updated_at":                            now,
	}
	res, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": string(userID)},
		bson.M{"$set": setDoc},
	)
	if err != nil {
		return fmt.Errorf("user repo: upsert session: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpsertSessionIfJTIMatches is the rotation-safe variant of
// UpsertSession. The Mongo filter includes
// `sessions.<deviceId>.current_jti: expectedJTI`, so two concurrent
// /auth/refresh requests that both passed the reuse-detection gate
// cannot both overwrite the session — only the one that observes the
// expected jti at UpdateOne time wins. The loser gets ErrSessionStale.
//
// Returns:
//   - nil when the CAS succeeded.
//   - ErrSessionStale when the user document exists but current_jti
//     no longer matches — the caller MUST treat this as "refresh
//     token already consumed", i.e. return AUTH_REFRESH_TOKEN_REVOKED.
//     The repo cannot cheaply distinguish "user missing" from "CAS
//     failed" in a single query; we conflate them here because the
//     service semantic is identical (the token is no longer valid).
//     Production data can never be in "user missing but has an
//     authenticated refresh jti" state — SignInWithApple inserts the
//     row before issuing the token.
func (r *MongoUserRepository) UpsertSessionIfJTIMatches(ctx context.Context, userID ids.UserID, deviceID string, expectedJTI string, s domain.Session) error {
	if userID == "" {
		return errors.New("user repo: upsert session cas: empty user id")
	}
	if err := validateDeviceID(deviceID); err != nil {
		return err
	}
	if expectedJTI == "" {
		// An empty expected jti would match an uninitialized sub-document
		// and silently overwrite it — reject loudly. Legitimate callers
		// always pass claims.ID which comes from a non-empty JWT.
		return errors.New("user repo: upsert session cas: empty expected jti")
	}
	now := r.clock.Now()
	setDoc := bson.M{
		"sessions." + deviceID + ".current_jti": s.CurrentJTI,
		"sessions." + deviceID + ".issued_at":   s.IssuedAt,
		"updated_at":                            now,
	}
	res, err := r.coll.UpdateOne(ctx,
		bson.M{
			"_id": string(userID),
			"sessions." + deviceID + ".current_jti": expectedJTI,
		},
		bson.M{"$set": setDoc},
	)
	if err != nil {
		return fmt.Errorf("user repo: upsert session cas: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrSessionStale
	}
	return nil
}

// GetSession projects only sessions.<deviceId> to keep the payload
// small. Returns ErrUserNotFound if the user document is absent;
// returns (zero, false, nil) when the user exists but has no entry for
// deviceID (distinguishable case — the caller treats absence as
// "session not initialized, reject").
func (r *MongoUserRepository) GetSession(ctx context.Context, userID ids.UserID, deviceID string) (domain.Session, bool, error) {
	if userID == "" {
		return domain.Session{}, false, errors.New("user repo: get session: empty user id")
	}
	if err := validateDeviceID(deviceID); err != nil {
		return domain.Session{}, false, err
	}
	// Projection: only the requested sub-document + _id.
	opts := options.FindOne().SetProjection(bson.M{"sessions." + deviceID: 1})
	var doc struct {
		Sessions map[string]domain.Session `bson:"sessions"`
	}
	err := r.coll.FindOne(ctx, bson.M{"_id": string(userID)}, opts).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return domain.Session{}, false, ErrUserNotFound
		}
		return domain.Session{}, false, fmt.Errorf("user repo: get session: %w", err)
	}
	s, ok := doc.Sessions[deviceID]
	if !ok {
		return domain.Session{}, false, nil
	}
	return s, true, nil
}

// ListDeviceIDs projects the full sessions map and returns its keys.
// Returns []string{} (non-nil) when the user exists but has no
// sessions. Consumed by AuthService.RevokeAllUserTokens (Story 1.6
// account deletion).
func (r *MongoUserRepository) ListDeviceIDs(ctx context.Context, userID ids.UserID) ([]string, error) {
	if userID == "" {
		return nil, errors.New("user repo: list device ids: empty user id")
	}
	opts := options.FindOne().SetProjection(bson.M{"sessions": 1})
	var doc struct {
		Sessions map[string]domain.Session `bson:"sessions"`
	}
	err := r.coll.FindOne(ctx, bson.M{"_id": string(userID)}, opts).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("user repo: list device ids: %w", err)
	}
	out := make([]string, 0, len(doc.Sessions))
	for k := range doc.Sessions {
		out = append(out, k)
	}
	return out, nil
}

// SetSessionHasApnsToken toggles users.sessions.<deviceId>.has_apns_token.
// Introduced by Story 1.4 so ApnsTokenService can mark a session as having
// an APNs token after successful registration.
//
// Implementation: single UpdateOne filtered by _id only. The dotted $set
// on sessions.<deviceId>.has_apns_token either merges into the existing
// sub-document or creates a minimal one if the session is absent. That
// auto-creation is acceptable — GetSession would read back
// current_jti="" / issued_at=zero, and Story 1.2's CAS path already
// rejects empty expected jti (see UpsertSessionIfJTIMatches), so the
// residual sub-doc cannot be replayed into a valid session.
//
// Semantics:
//   - userID missing → ErrUserNotFound (only "real error" branch).
//   - sub-document absent → nil (silent create).
//
// A prior TOCTOU-prone design used a two-step exists-check + update;
// that was replaced with the single UpdateOne to match the AC Review
// requirement for atomicity (no flaky race with Story 1.6 concurrent
// deletion).
func (r *MongoUserRepository) SetSessionHasApnsToken(ctx context.Context, userID ids.UserID, deviceID string, has bool) error {
	if userID == "" {
		return errors.New("user repo: set session has_apns_token: empty user id")
	}
	if err := validateDeviceID(deviceID); err != nil {
		return err
	}
	now := r.clock.Now()
	setDoc := bson.M{
		"sessions." + deviceID + ".has_apns_token": has,
		"updated_at": now,
	}
	res, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": string(userID)},
		bson.M{"$set": setDoc},
	)
	if err != nil {
		return fmt.Errorf("user repo: set session has_apns_token: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrUserNotFound
	}
	return nil
}

// ProfileUpdate is the repo-layer shape for UpdateProfile. Every field
// is a pointer: non-nil ⇒ "client asked to change this"; nil ⇒ "leave
// alone". The handler/service layer is responsible for already having
// run DTO validation (HH:MM / IANA / displayName trim) before calling
// this method; the repo treats inputs as authoritative and copies them
// into the $set document.
type ProfileUpdate struct {
	DisplayName *string
	Timezone    *string
	QuietHours  *domain.QuietHours
}

// UpdateProfile applies the partial update in a single Mongo round-trip
// (FindOneAndUpdate with ReturnDocument: After). The partial shape
// prevents accidentally clobbering unrelated nested fields —
// preferences.quiet_hours.{start,end} uses *dotted* $set so future
// preferences.* additions (Epic 5 touch mute, Epic 7 skin slot) survive
// a displayName-only update.
//
// A single-call UpdateOne is deliberate (see Story 1.4 AC Review
// hardening #5): a two-step exists-check + update would TOCTOU-race
// with Story 1.6 concurrent account deletion. FindOneAndUpdate uses
// Mongo's server-side atomic update and returns the post-update
// document in one network round-trip.
//
// Returns ErrUserNotFound when no document matched. The caller (service
// layer) typically maps this to ErrInternalError at the handler boundary
// so clients cannot probe for user-existence.
func (r *MongoUserRepository) UpdateProfile(ctx context.Context, userID ids.UserID, p ProfileUpdate) (*domain.User, error) {
	if userID == "" {
		return nil, errors.New("user repo: update profile: empty user id")
	}
	if p.DisplayName == nil && p.Timezone == nil && p.QuietHours == nil {
		return nil, errors.New("user repo: update profile: no fields to update")
	}

	setDoc := bson.M{"updated_at": r.clock.Now()}
	if p.DisplayName != nil {
		setDoc["display_name"] = *p.DisplayName
	}
	if p.Timezone != nil {
		setDoc["timezone"] = *p.Timezone
	}
	if p.QuietHours != nil {
		// Dotted $set — NEVER replace `preferences` wholesale. Future
		// preferences.* additions would otherwise get clobbered by a
		// profile.update that only carries quietHours. (Story 1.5
		// Semantic-correctness #7.)
		setDoc["preferences.quiet_hours.start"] = p.QuietHours.Start
		setDoc["preferences.quiet_hours.end"] = p.QuietHours.End
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updated domain.User
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": string(userID)},
		bson.M{"$set": setDoc},
		opts,
	).Decode(&updated)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("user repo: update profile: %w", err)
	}
	return &updated, nil
}

// ClearDeletion clears the deletion_requested flag and stamps
// updated_at with the clock. Returns ErrUserNotFound if no row matched
// (the caller should treat this as a programming error — the service
// only calls ClearDeletion after observing the row via
// FindByAppleHash).
func (r *MongoUserRepository) ClearDeletion(ctx context.Context, id ids.UserID) error {
	if id == "" {
		return errors.New("user repo: clear deletion: empty user id")
	}
	now := r.clock.Now()
	res, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": string(id)},
		bson.M{
			"$set": bson.M{
				"deletion_requested": false,
				"updated_at":         now,
			},
			"$unset": bson.M{
				"deletion_requested_at": "",
			},
		},
	)
	if err != nil {
		return fmt.Errorf("user repo: clear deletion: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrUserNotFound
	}
	return nil
}
