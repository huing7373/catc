package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/jwtx"
)

// ---- Fakes ----

type fakeRepo struct {
	findByHash    func(ctx context.Context, hash string) (*domain.User, error)
	findByID      func(ctx context.Context, id ids.UserID) (*domain.User, error)
	insert        func(ctx context.Context, u *domain.User) error
	clearDeletion func(ctx context.Context, id ids.UserID) error

	insertCount        int32
	clearDeletionCount int32
}

func (f *fakeRepo) EnsureIndexes(_ context.Context) error { return nil }

func (f *fakeRepo) FindByAppleHash(ctx context.Context, hash string) (*domain.User, error) {
	if f.findByHash == nil {
		return nil, repository.ErrUserNotFound
	}
	return f.findByHash(ctx, hash)
}

func (f *fakeRepo) FindByID(ctx context.Context, id ids.UserID) (*domain.User, error) {
	if f.findByID == nil {
		return nil, repository.ErrUserNotFound
	}
	return f.findByID(ctx, id)
}

func (f *fakeRepo) Insert(ctx context.Context, u *domain.User) error {
	atomic.AddInt32(&f.insertCount, 1)
	if f.insert == nil {
		return nil
	}
	return f.insert(ctx, u)
}

func (f *fakeRepo) ClearDeletion(ctx context.Context, id ids.UserID) error {
	atomic.AddInt32(&f.clearDeletionCount, 1)
	if f.clearDeletion == nil {
		return nil
	}
	return f.clearDeletion(ctx, id)
}

type fakeVerifier struct {
	verify func(ctx context.Context, idToken, expectedNonce string) (*jwtx.AppleIdentityClaims, error)
}

func (f *fakeVerifier) VerifyApple(ctx context.Context, idToken, expectedNonce string) (*jwtx.AppleIdentityClaims, error) {
	return f.verify(ctx, idToken, expectedNonce)
}

type fakeIssuer struct {
	issuedAccess  string
	issuedRefresh string
	failOn        string // "" | "access" | "refresh"
	calls         []string
}

func (f *fakeIssuer) Issue(claims jwtx.CustomClaims) (string, error) {
	f.calls = append(f.calls, claims.TokenType)
	if f.failOn == claims.TokenType {
		return "", errors.New("issue failed: " + claims.TokenType)
	}
	if claims.TokenType == "access" {
		return f.issuedAccess, nil
	}
	return f.issuedRefresh, nil
}

// ---- Helpers ----

func okClaimsFor(sub string) *jwtx.AppleIdentityClaims {
	return &jwtx.AppleIdentityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: sub,
			Issuer:  jwtx.AppleIssuer,
		},
	}
}

func defaultRequest() SignInWithAppleRequest {
	return SignInWithAppleRequest{
		IdentityToken: "id-token",
		DeviceID:      "device-uuid",
		Platform:      ids.PlatformWatch,
		Nonce:         "raw-nonce",
	}
}

func newSvc(t *testing.T, repo UserRepository, verifier AppleVerifier, issuer JWTIssuer) *AuthService {
	t.Helper()
	clk := clockx.NewFakeClock(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))
	return NewAuthService(repo, verifier, issuer, clk, "release")
}

// ---- Cases ----

func TestSignInWithApple_NewUser(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{} // FindByAppleHash returns NotFound by default; Insert succeeds
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:new"), nil
	}}
	issuer := &fakeIssuer{issuedAccess: "ACCESS", issuedRefresh: "REFRESH"}

	svc := newSvc(t, repo, verifier, issuer)
	res, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err)
	assert.True(t, res.IsNewUser)
	assert.NotEmpty(t, res.User.ID)
	assert.Equal(t, "ACCESS", res.AccessToken)
	assert.Equal(t, "REFRESH", res.RefreshToken)
	assert.Equal(t, int32(1), atomic.LoadInt32(&repo.insertCount))
}

func TestSignInWithApple_ExistingUser(t *testing.T) {
	t.Parallel()
	existing := &domain.User{
		ID:              ids.NewUserID(),
		AppleUserIDHash: hexSHA256("apple:user:old"),
	}
	repo := &fakeRepo{
		findByHash: func(_ context.Context, _ string) (*domain.User, error) { return existing, nil },
	}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:old"), nil
	}}
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}

	svc := newSvc(t, repo, verifier, issuer)
	res, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err)
	assert.False(t, res.IsNewUser)
	assert.Equal(t, existing.ID, res.User.ID)
	assert.Equal(t, int32(0), atomic.LoadInt32(&repo.insertCount))
}

func TestSignInWithApple_ResurrectsDeletedUser(t *testing.T) {
	t.Parallel()
	pastTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	existing := &domain.User{
		ID:                  ids.NewUserID(),
		DeletionRequested:   true,
		DeletionRequestedAt: &pastTime,
	}
	repo := &fakeRepo{
		findByHash: func(_ context.Context, _ string) (*domain.User, error) { return existing, nil },
	}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:returning"), nil
	}}
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}

	svc := newSvc(t, repo, verifier, issuer)
	res, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err)
	assert.False(t, res.User.DeletionRequested)
	assert.Nil(t, res.User.DeletionRequestedAt)
	assert.Equal(t, int32(1), atomic.LoadInt32(&repo.clearDeletionCount))
}

func TestSignInWithApple_ConcurrentRaceResolved(t *testing.T) {
	t.Parallel()
	winner := &domain.User{
		ID:              ids.NewUserID(),
		AppleUserIDHash: hexSHA256("apple:user:race"),
	}
	calls := 0
	repo := &fakeRepo{
		findByHash: func(_ context.Context, _ string) (*domain.User, error) {
			calls++
			if calls == 1 {
				return nil, repository.ErrUserNotFound
			}
			return winner, nil
		},
		insert: func(_ context.Context, _ *domain.User) error {
			return repository.ErrUserDuplicateHash
		},
	}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:race"), nil
	}}
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}

	svc := newSvc(t, repo, verifier, issuer)
	res, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err)
	assert.False(t, res.IsNewUser, "race-resolved user must surface as existing")
	assert.Equal(t, winner.ID, res.User.ID)
	assert.Equal(t, 2, calls, "second FindByAppleHash must be called after duplicate-key Insert")
}

func TestSignInWithApple_VerifyError(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return nil, errors.New("apple: alg mismatch")
	}}
	issuer := &fakeIssuer{}

	svc := newSvc(t, repo, verifier, issuer)
	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrAuthInvalidIdentityToken))
	assert.Equal(t, int32(0), atomic.LoadInt32(&repo.insertCount), "insert must NOT happen on verify failure")
}

func TestSignInWithApple_FindByAppleHashError(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		findByHash: func(_ context.Context, _ string) (*domain.User, error) { return nil, errors.New("mongo down") },
	}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:1"), nil
	}}
	issuer := &fakeIssuer{}

	svc := newSvc(t, repo, verifier, issuer)
	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError))
}

func TestSignInWithApple_InsertNonDuplicateError(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		insert: func(_ context.Context, _ *domain.User) error { return errors.New("mongo write conflict") },
	}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:1"), nil
	}}
	issuer := &fakeIssuer{}

	svc := newSvc(t, repo, verifier, issuer)
	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError))
}

func TestSignInWithApple_ClearDeletionError(t *testing.T) {
	t.Parallel()
	pastTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	existing := &domain.User{
		ID:                  ids.NewUserID(),
		DeletionRequested:   true,
		DeletionRequestedAt: &pastTime,
	}
	repo := &fakeRepo{
		findByHash:    func(_ context.Context, _ string) (*domain.User, error) { return existing, nil },
		clearDeletion: func(_ context.Context, _ ids.UserID) error { return errors.New("mongo write fail") },
	}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:returning"), nil
	}}
	issuer := &fakeIssuer{}

	svc := newSvc(t, repo, verifier, issuer)
	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError),
		"ClearDeletion failure must NOT silently issue tokens (story dev notes fail-closed matrix)")
}

func TestSignInWithApple_AccessIssueError(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:1"), nil
	}}
	issuer := &fakeIssuer{failOn: "access", issuedRefresh: "R"}

	svc := newSvc(t, repo, verifier, issuer)
	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError))
}

func TestSignInWithApple_RefreshIssueError(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:1"), nil
	}}
	issuer := &fakeIssuer{issuedAccess: "A", failOn: "refresh"}

	svc := newSvc(t, repo, verifier, issuer)
	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError))
}

func TestSignInWithApple_RejectsEmptyNonce(t *testing.T) {
	t.Parallel()
	svc := newSvc(t, &fakeRepo{}, &fakeVerifier{}, &fakeIssuer{})
	req := defaultRequest()
	req.Nonce = ""
	_, err := svc.SignInWithApple(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrValidationError))
}

func TestSignInWithApple_RejectsBadPlatform(t *testing.T) {
	t.Parallel()
	svc := newSvc(t, &fakeRepo{}, &fakeVerifier{}, &fakeIssuer{})
	req := defaultRequest()
	req.Platform = "android"
	_, err := svc.SignInWithApple(context.Background(), req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrValidationError))
}

func TestSignInWithApple_PassesHashedNonceToVerifier(t *testing.T) {
	t.Parallel()
	var passedNonce string
	verifier := &fakeVerifier{verify: func(_ context.Context, _, expectedNonce string) (*jwtx.AppleIdentityClaims, error) {
		passedNonce = expectedNonce
		return okClaimsFor("apple:user:1"), nil
	}}
	repo := &fakeRepo{}
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}
	svc := newSvc(t, repo, verifier, issuer)

	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err)
	expected := hexSHA256("raw-nonce")
	assert.Equal(t, expected, passedNonce, "service must SHA-256 the raw nonce before passing to VerifyApple")
}

func TestSignInWithApple_IssuedClaimsCarryDeviceAndPlatform(t *testing.T) {
	t.Parallel()
	var captured []jwtx.CustomClaims
	captureIssuer := &capturingIssuer{
		out: &captured,
		access:  "A",
		refresh: "R",
	}
	repo := &fakeRepo{}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:1"), nil
	}}
	svc := newSvc(t, repo, verifier, captureIssuer)

	res, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err)
	require.Len(t, captured, 2)
	for _, c := range captured {
		assert.Equal(t, "device-uuid", c.DeviceID)
		assert.Equal(t, "watch", c.Platform)
		assert.Equal(t, string(res.User.ID), c.UserID)
		assert.Equal(t, string(res.User.ID), c.Subject,
			"RegisteredClaims.Subject must equal UserID for downstream audit consumers (§3.5)")
	}
	assert.Equal(t, "access", captured[0].TokenType)
	assert.Equal(t, "refresh", captured[1].TokenType)
}

type capturingIssuer struct {
	out     *[]jwtx.CustomClaims
	access  string
	refresh string
}

func (c *capturingIssuer) Issue(claims jwtx.CustomClaims) (string, error) {
	*c.out = append(*c.out, claims)
	if claims.TokenType == "access" {
		return c.access, nil
	}
	return c.refresh, nil
}
