package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/huing7373/catc/server/internal/domain"
	"github.com/huing7373/catc/server/pkg/ids"
)

// userCacheTTL bounds how long a user document stays in Redis before a
// fresh Mongo read is forced. Writes invalidate via Del.
const userCacheTTL = 30 * time.Minute

// DeletionCoolDown is how long a soft-deleted user can still be revived
// by a fresh Sign-in-with-Apple login. Aligned with Story 2.4.
const DeletionCoolDown = 30 * 24 * time.Hour

// LoginOutcome describes how UpsertOnAppleLogin satisfied the request.
type LoginOutcome string

const (
	// OutcomeExisting means an active user was matched and refreshed.
	OutcomeExisting LoginOutcome = "existing"
	// OutcomeRestored means a soft-deleted user inside the cool-down
	// window was revived and refreshed.
	OutcomeRestored LoginOutcome = "restored"
	// OutcomeCreated means no matching user existed and one was inserted.
	OutcomeCreated LoginOutcome = "created"
)

// userDoc is the private BSON shape of the users collection. It must
// never escape the repository package.
type userDoc struct {
	ID          string     `bson:"_id"`
	AppleID     string     `bson:"apple_id"`
	DisplayName string     `bson:"display_name"`
	DeviceID    string     `bson:"device_id"`
	DnDStart    *time.Time `bson:"dnd_start,omitempty"`
	DnDEnd      *time.Time `bson:"dnd_end,omitempty"`
	IsDeleted   bool       `bson:"is_deleted"`
	// DeletionScheduledAt is the moment the deletion was REQUESTED —
	// the start of the cool-down window. Field name kept for AC #4
	// wording ("deletion_scheduled_at > now-30d"); see MarkDeleted
	// godoc for the contract.
	DeletionScheduledAt *time.Time `bson:"deletion_scheduled_at,omitempty"`
	CreatedAt           time.Time  `bson:"created_at"`
	LastActiveAt        time.Time  `bson:"last_active_at"`
}

func (d *userDoc) toDomain() *domain.User {
	return &domain.User{
		ID:                  ids.UserID(d.ID),
		AppleID:             d.AppleID,
		DisplayName:         d.DisplayName,
		DeviceID:            ids.DeviceID(d.DeviceID),
		DnDStart:            d.DnDStart,
		DnDEnd:              d.DnDEnd,
		IsDeleted:           d.IsDeleted,
		DeletionScheduledAt: d.DeletionScheduledAt,
		CreatedAt:           d.CreatedAt,
		LastActiveAt:        d.LastActiveAt,
	}
}

func docFromUser(u *domain.User) *userDoc {
	return &userDoc{
		ID:                  string(u.ID),
		AppleID:             u.AppleID,
		DisplayName:         u.DisplayName,
		DeviceID:            string(u.DeviceID),
		DnDStart:            u.DnDStart,
		DnDEnd:              u.DnDEnd,
		IsDeleted:           u.IsDeleted,
		DeletionScheduledAt: u.DeletionScheduledAt,
		CreatedAt:           u.CreatedAt,
		LastActiveAt:        u.LastActiveAt,
	}
}

// UserRepository persists users in Mongo with a Redis cache-aside layer.
type UserRepository struct {
	coll *mongo.Collection
	rdb  *redis.Client
}

// NewUserRepo constructs a *UserRepository bound to the "users"
// collection of the supplied database.
func NewUserRepo(cli *mongo.Client, dbName string, rdb *redis.Client) *UserRepository {
	return &UserRepository{
		coll: cli.Database(dbName).Collection("users"),
		rdb:  rdb,
	}
}

// EnsureIndexes creates the indexes required by this repository. It is
// idempotent and safe to call on every boot.
func (r *UserRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "apple_id", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetName("uq_users_apple_id").
				SetPartialFilterExpression(bson.M{"is_deleted": false}),
		},
		{
			Keys:    bson.D{{Key: "last_active_at", Value: 1}},
			Options: options.Index().SetName("idx_users_last_active"),
		},
		{
			Keys: bson.D{{Key: "deletion_scheduled_at", Value: 1}},
			Options: options.Index().
				SetName("idx_users_deletion_scheduled").
				SetSparse(true).
				SetPartialFilterExpression(bson.M{"is_deleted": true}),
		},
	})
	if err != nil {
		return fmt.Errorf("user repo: ensure indexes: %w", err)
	}
	return nil
}

// FindByID fetches a non-deleted user by id. Returns ErrNotFound if
// absent.
func (r *UserRepository) FindByID(ctx context.Context, uid ids.UserID) (*domain.User, error) {
	if u, ok := r.getCache(ctx, uid); ok {
		return u, nil
	}
	var d userDoc
	err := r.coll.FindOne(ctx, bson.M{"_id": string(uid), "is_deleted": false}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: find by id: %w", err)
	}
	u := d.toDomain()
	r.setCache(ctx, u)
	return u, nil
}

// FindByAppleID fetches a non-deleted user by Sign-in-with-Apple id.
func (r *UserRepository) FindByAppleID(ctx context.Context, appleID string) (*domain.User, error) {
	var d userDoc
	err := r.coll.FindOne(ctx, bson.M{"apple_id": appleID, "is_deleted": false}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user repo: find by apple id: %w", err)
	}
	return d.toDomain(), nil
}

// Create inserts a new user. Duplicate apple_id yields ErrConflict.
func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	if u.LastActiveAt.IsZero() {
		u.LastActiveAt = u.CreatedAt
	}
	_, err := r.coll.InsertOne(ctx, docFromUser(u))
	if mongo.IsDuplicateKeyError(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("user repo: create: %w", err)
	}
	return nil
}

// UpdateDisplayName atomically updates the user's display_name.
func (r *UserRepository) UpdateDisplayName(ctx context.Context, uid ids.UserID, name string) error {
	res, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": string(uid), "is_deleted": false},
		bson.M{"$set": bson.M{"display_name": name}})
	if err != nil {
		return fmt.Errorf("user repo: update display_name: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	r.invalidate(ctx, uid)
	return nil
}

// UpsertOnAppleLogin reconciles a Sign-in-with-Apple login against the
// users collection, returning a domain.User plus the path taken
// (existing / restored / created).
//
// Order of attempts:
//  1. ACTIVE: match {apple_id, is_deleted=false} → refresh
//     last_active_at + device_id → OutcomeExisting.
//  2. RESTORED: match {apple_id, is_deleted=true, deletion_scheduled_at >
//     now-DeletionCoolDown} → clear is_deleted/deletion_scheduled_at +
//     refresh → OutcomeRestored.
//  3. CREATED: insert a fresh user (uuid id, empty display name) →
//     OutcomeCreated.
//  4. CONFLICT GUARD: if step 3 races another concurrent registration we
//     re-query and finish as OutcomeExisting; if the second lookup also
//     fails we surface ErrConflict.
//
// nowFn lets tests inject a deterministic clock; production callers pass
// time.Now.
func (r *UserRepository) UpsertOnAppleLogin(
	ctx context.Context,
	appleID string,
	deviceID string,
	nowFn func() time.Time,
) (*domain.User, LoginOutcome, error) {
	now := nowFn().UTC()

	// 1. Active hit.
	activeSet := bson.M{
		"$set": bson.M{
			"last_active_at": now,
			"device_id":      deviceID,
		},
	}
	returnAfter := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var d userDoc
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"apple_id": appleID, "is_deleted": false},
		activeSet,
		returnAfter,
	).Decode(&d)
	if err == nil {
		u := d.toDomain()
		r.invalidate(ctx, u.ID)
		return u, OutcomeExisting, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, "", fmt.Errorf("user repo: upsert active: %w", err)
	}

	// 2. Cool-down restore.
	//
	// Convention (see MarkDeleted godoc): deletion_scheduled_at stores
	// the moment the deletion was REQUESTED — i.e. the start of the
	// cool-down window, not the future purge time. A doc qualifies for
	// restore while its deletion request is younger than DeletionCoolDown.
	cooldownStart := now.Add(-DeletionCoolDown)
	restore := bson.M{
		"$set": bson.M{
			"is_deleted":     false,
			"last_active_at": now,
			"device_id":      deviceID,
		},
		"$unset": bson.M{"deletion_scheduled_at": ""},
	}
	err = r.coll.FindOneAndUpdate(ctx,
		bson.M{
			"apple_id":              appleID,
			"is_deleted":            true,
			"deletion_scheduled_at": bson.M{"$gt": cooldownStart},
		},
		restore,
		returnAfter,
	).Decode(&d)
	if err == nil {
		u := d.toDomain()
		r.invalidate(ctx, u.ID)
		return u, OutcomeRestored, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, "", fmt.Errorf("user repo: upsert restore: %w", err)
	}

	// 3. Create.
	newUser := &domain.User{
		ID:           ids.UserID(uuid.NewString()),
		AppleID:      appleID,
		DisplayName:  "",
		DeviceID:     ids.DeviceID(deviceID),
		CreatedAt:    now,
		LastActiveAt: now,
	}
	_, err = r.coll.InsertOne(ctx, docFromUser(newUser))
	if err == nil {
		return newUser, OutcomeCreated, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return nil, "", fmt.Errorf("user repo: upsert create: %w", err)
	}

	// 4. Concurrent-registration race: re-query and treat as existing.
	existing, err := r.FindByAppleID(ctx, appleID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, "", ErrConflict
		}
		return nil, "", fmt.Errorf("user repo: upsert dupkey re-find: %w", err)
	}
	if _, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": string(existing.ID), "is_deleted": false},
		activeSet,
	); err != nil {
		return nil, "", fmt.Errorf("user repo: upsert dupkey refresh: %w", err)
	}
	existing.LastActiveAt = now
	existing.DeviceID = ids.DeviceID(deviceID)
	r.invalidate(ctx, existing.ID)
	return existing, OutcomeExisting, nil
}

// MarkDeleted flips is_deleted and records the moment the deletion
// request was made (Story 2.4 cool-down anchor).
//
// CONTRACT: deletedAt is the time the user requested deletion, normally
// time.Now().UTC() at the call site — NOT a future purge time. The
// cool-down window starts here; UpsertOnAppleLogin restores the user
// while now-deletedAt < DeletionCoolDown. Passing a future timestamp
// (e.g. now.Add(30d)) would incorrectly extend the restore window to
// roughly 60 days. The Story 2.4 sweeper is the consumer that decides
// when actual purge happens; it derives the deadline as
// deletedAt + DeletionCoolDown.
func (r *UserRepository) MarkDeleted(ctx context.Context, uid ids.UserID, deletedAt time.Time) error {
	res, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": string(uid), "is_deleted": false},
		bson.M{"$set": bson.M{"is_deleted": true, "deletion_scheduled_at": deletedAt}})
	if err != nil {
		return fmt.Errorf("user repo: mark deleted: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	r.invalidate(ctx, uid)
	return nil
}

// --- cache helpers ---

func (r *UserRepository) getCache(ctx context.Context, uid ids.UserID) (*domain.User, bool) {
	if r.rdb == nil {
		return nil, false
	}
	raw, err := r.rdb.Get(ctx, userCacheKey(uid)).Bytes()
	if err != nil {
		return nil, false
	}
	var d userDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, false
	}
	return d.toDomain(), true
}

func (r *UserRepository) setCache(ctx context.Context, u *domain.User) {
	if r.rdb == nil {
		return
	}
	raw, err := json.Marshal(docFromUser(u))
	if err != nil {
		return
	}
	// Explicit TTL (30min): required by the Redis discipline rule.
	_ = r.rdb.Set(ctx, userCacheKey(u.ID), raw, userCacheTTL).Err()
}

func (r *UserRepository) invalidate(ctx context.Context, uid ids.UserID) {
	if r.rdb == nil {
		return
	}
	_ = r.rdb.Del(ctx, userCacheKey(uid)).Err()
}
