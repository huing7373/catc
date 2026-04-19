package service

import (
	"context"
	"errors"
	"sync"
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

type upsertCall struct {
	UserID   ids.UserID
	DeviceID string
	Session  domain.Session
}

type fakeRepo struct {
	findByHash    func(ctx context.Context, hash string) (*domain.User, error)
	findByID      func(ctx context.Context, id ids.UserID) (*domain.User, error)
	insert        func(ctx context.Context, u *domain.User) error
	clearDeletion func(ctx context.Context, id ids.UserID) error

	// Story 1.2 sessions surface. Each is optional; the default
	// happy-path Upsert success / GetSession absent / ListDeviceIDs
	// empty is enough for Story 1.1 tests that do not care about
	// sessions semantics.
	upsertSession         func(ctx context.Context, userID ids.UserID, deviceID string, s domain.Session) error
	upsertSessionIfMatch  func(ctx context.Context, userID ids.UserID, deviceID, expectedJTI string, s domain.Session) error
	getSession            func(ctx context.Context, userID ids.UserID, deviceID string) (domain.Session, bool, error)
	listDeviceIDs         func(ctx context.Context, userID ids.UserID) ([]string, error)

	insertCount        int32
	clearDeletionCount int32

	mu                       sync.Mutex
	upsertSessionCalls       []upsertCall
	upsertSessionCASCalls    []upsertCASCall
}

type upsertCASCall struct {
	UserID      ids.UserID
	DeviceID    string
	ExpectedJTI string
	Session     domain.Session
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

func (f *fakeRepo) UpsertSession(ctx context.Context, userID ids.UserID, deviceID string, s domain.Session) error {
	f.mu.Lock()
	f.upsertSessionCalls = append(f.upsertSessionCalls, upsertCall{UserID: userID, DeviceID: deviceID, Session: s})
	f.mu.Unlock()
	if f.upsertSession == nil {
		return nil
	}
	return f.upsertSession(ctx, userID, deviceID, s)
}

func (f *fakeRepo) UpsertSessionIfJTIMatches(ctx context.Context, userID ids.UserID, deviceID, expectedJTI string, s domain.Session) error {
	f.mu.Lock()
	f.upsertSessionCASCalls = append(f.upsertSessionCASCalls, upsertCASCall{
		UserID: userID, DeviceID: deviceID, ExpectedJTI: expectedJTI, Session: s,
	})
	f.mu.Unlock()
	if f.upsertSessionIfMatch == nil {
		return nil
	}
	return f.upsertSessionIfMatch(ctx, userID, deviceID, expectedJTI, s)
}

func (f *fakeRepo) GetSession(ctx context.Context, userID ids.UserID, deviceID string) (domain.Session, bool, error) {
	if f.getSession == nil {
		return domain.Session{}, false, nil
	}
	return f.getSession(ctx, userID, deviceID)
}

func (f *fakeRepo) ListDeviceIDs(ctx context.Context, userID ids.UserID) ([]string, error) {
	if f.listDeviceIDs == nil {
		return []string{}, nil
	}
	return f.listDeviceIDs(ctx, userID)
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
	refreshExpiry time.Duration // zero → default 30 days
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

func (f *fakeIssuer) RefreshExpiry() time.Duration {
	if f.refreshExpiry == 0 {
		return 30 * 24 * time.Hour
	}
	return f.refreshExpiry
}

// noopRefreshVerifier / noopRefreshBlacklist are happy-path stubs for
// Story 1.1 tests that do not exercise /auth/refresh. Verify always
// errors so any accidental call from SIWA-focused tests surfaces loudly.
type noopRefreshVerifier struct{}

func (noopRefreshVerifier) Verify(_ string) (*jwtx.CustomClaims, error) {
	return nil, errors.New("noopRefreshVerifier: not wired for this test")
}

type noopRefreshBlacklist struct{}

func (noopRefreshBlacklist) IsRevoked(_ context.Context, _ string) (bool, error) { return false, nil }
func (noopRefreshBlacklist) Revoke(_ context.Context, _ string, _ time.Time) error {
	return nil
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
	return NewAuthService(repo, verifier, noopRefreshVerifier{}, issuer, noopRefreshBlacklist{}, clk, "release")
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

	// Story 1.2 AC7: SIWA must write sessions[deviceId].current_jti.
	require.Len(t, repo.upsertSessionCalls, 1)
	assert.Equal(t, res.User.ID, repo.upsertSessionCalls[0].UserID)
	assert.Equal(t, "device-uuid", repo.upsertSessionCalls[0].DeviceID)
	assert.NotEmpty(t, repo.upsertSessionCalls[0].Session.CurrentJTI)
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

	require.Len(t, repo.upsertSessionCalls, 1,
		"Story 1.2 AC7: existing-user SIWA must also write sessions[deviceId]")
	assert.Equal(t, existing.ID, repo.upsertSessionCalls[0].UserID)
	assert.NotEmpty(t, repo.upsertSessionCalls[0].Session.CurrentJTI)
}

func TestSignInWithApple_UpsertSessionError(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		upsertSession: func(_ context.Context, _ ids.UserID, _ string, _ domain.Session) error {
			return errors.New("mongo write timeout")
		},
	}
	verifier := &fakeVerifier{verify: func(_ context.Context, _, _ string) (*jwtx.AppleIdentityClaims, error) {
		return okClaimsFor("apple:user:session-fail"), nil
	}}
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}

	svc := newSvc(t, repo, verifier, issuer)
	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError),
		"Story 1.2 AC7: UpsertSession failure must be fail-closed (INTERNAL_ERROR), not a silent token issue")
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

// TestSignInWithApple_Resurrection_ClearsAccessBlacklist locks the
// Story 1.6 round-1 resurrection hook: when the blacklist remover is
// installed, SIWA resurrection MUST call Remove(userID). Without
// this, a user who DELETEs and then SIWAs back within 15 minutes
// would see 401s on /v1/* until the blacklist TTL expires.
func TestSignInWithApple_Resurrection_ClearsAccessBlacklist(t *testing.T) {
	t.Parallel()
	pastTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	uid := ids.NewUserID()
	existing := &domain.User{
		ID:                  uid,
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

	bl := &fakeAccessBlacklistRemover{}
	svc := newSvc(t, repo, verifier, issuer)
	svc.SetAccessBlacklistRemover(bl)

	_, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err)

	require.Len(t, bl.removed, 1, "resurrection MUST call blacklist.Remove exactly once")
	assert.Equal(t, string(uid), bl.removed[0],
		"Remove MUST be called with the same userId whose row was just un-deleted")
}

// TestSignInWithApple_Resurrection_BlacklistRemoveError_DoesNotFailFlow
// verifies fail-open on the resurrection hook — a Redis outage during
// Remove emits a warn log but the SIWA flow still returns tokens.
func TestSignInWithApple_Resurrection_BlacklistRemoveError_DoesNotFailFlow(t *testing.T) {
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

	bl := &fakeAccessBlacklistRemover{err: errors.New("redis down")}
	svc := newSvc(t, repo, verifier, issuer)
	svc.SetAccessBlacklistRemover(bl)

	res, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err, "fail-open: SIWA succeeds even if blacklist Remove errors")
	require.NotNil(t, res)
	assert.Equal(t, "A", res.AccessToken)
}

// TestSignInWithApple_NilAccessBlacklistRemover_ResurrectionStillWorks
// sanity-checks that a nil remover (legacy test harnesses) does not
// crash the resurrection path — the Remove call is simply skipped.
func TestSignInWithApple_NilAccessBlacklistRemover_ResurrectionStillWorks(t *testing.T) {
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
	// Deliberately do NOT call SetAccessBlacklistRemover — accessBlacklist stays nil.

	res, err := svc.SignInWithApple(context.Background(), defaultRequest())
	require.NoError(t, err)
	assert.False(t, res.User.DeletionRequested)
}

type fakeAccessBlacklistRemover struct {
	removed []string
	err     error
}

func (f *fakeAccessBlacklistRemover) Remove(_ context.Context, userID string) error {
	f.removed = append(f.removed, userID)
	return f.err
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

func (c *capturingIssuer) RefreshExpiry() time.Duration {
	return 30 * 24 * time.Hour
}
