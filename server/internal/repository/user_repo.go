package repository

import (
	"context"
	"errors"
	"fmt"

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
