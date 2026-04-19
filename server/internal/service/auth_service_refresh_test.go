package service

import (
	"context"
	"errors"
	"sync"
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

func repoErrUserNotFound() error  { return repository.ErrUserNotFound }
func repoErrSessionStale() error  { return repository.ErrSessionStale }

// ---- Fakes ----

type fakeRefreshVerifier struct {
	claims *jwtx.CustomClaims
	err    error
}

func (f *fakeRefreshVerifier) Verify(_ string) (*jwtx.CustomClaims, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

type blRevokeCall struct {
	JTI string
	Exp time.Time
}

type fakeBlacklist struct {
	mu             sync.Mutex
	isRevokedResp  map[string]bool
	isRevokedErr   error
	revokeErrors   map[string]error // keyed by jti; "" key applies to any jti not listed
	revokeAllError error            // applied to every Revoke if non-nil

	revokeCalls []blRevokeCall
}

func newFakeBlacklist() *fakeBlacklist {
	return &fakeBlacklist{
		isRevokedResp: map[string]bool{},
		revokeErrors:  map[string]error{},
	}
}

func (f *fakeBlacklist) IsRevoked(_ context.Context, jti string) (bool, error) {
	if f.isRevokedErr != nil {
		return false, f.isRevokedErr
	}
	return f.isRevokedResp[jti], nil
}

func (f *fakeBlacklist) Revoke(_ context.Context, jti string, exp time.Time) error {
	f.mu.Lock()
	f.revokeCalls = append(f.revokeCalls, blRevokeCall{JTI: jti, Exp: exp})
	f.mu.Unlock()
	if f.revokeAllError != nil {
		return f.revokeAllError
	}
	if err, ok := f.revokeErrors[jti]; ok {
		return err
	}
	return nil
}

// ---- Helpers ----

func newRefreshSvc(
	t *testing.T,
	repo UserRepository,
	refreshV RefreshVerifier,
	issuer JWTIssuer,
	blacklist RefreshBlacklist,
	clk clockx.Clock,
) *AuthService {
	t.Helper()
	return NewAuthService(repo, &fakeVerifier{}, refreshV, issuer, blacklist, clk, "release")
}

func refreshClaims(userID, deviceID, platform, jti string, exp time.Time) *jwtx.CustomClaims {
	return &jwtx.CustomClaims{
		UserID:    userID,
		DeviceID:  deviceID,
		Platform:  platform,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
}

// ---- Cases ----

func TestRefreshToken_HappyPath(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)

	userID := ids.UserID("user-1")
	deviceID := "device-a"
	oldJTI := "jti-old"
	exp := now.Add(10 * 24 * time.Hour)

	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: oldJTI, IssuedAt: now.Add(-time.Hour)}, true, nil
		},
	}
	rv := &fakeRefreshVerifier{claims: refreshClaims(string(userID), deviceID, "watch", oldJTI, exp)}
	issuer := &fakeIssuer{issuedAccess: "NEW-ACCESS", issuedRefresh: "NEW-REFRESH"}
	bl := newFakeBlacklist()

	svc := newRefreshSvc(t, repo, rv, issuer, bl, clk)
	res, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "old-refresh-token"})
	require.NoError(t, err)
	assert.Equal(t, "NEW-ACCESS", res.AccessToken)
	assert.Equal(t, "NEW-REFRESH", res.RefreshToken)

	// Happy path uses the CAS upsert path (round-1 review P1).
	require.Len(t, repo.upsertSessionCASCalls, 1, "happy path must persist new session jti via CAS")
	assert.Equal(t, userID, repo.upsertSessionCASCalls[0].UserID)
	assert.Equal(t, deviceID, repo.upsertSessionCASCalls[0].DeviceID)
	assert.Equal(t, oldJTI, repo.upsertSessionCASCalls[0].ExpectedJTI,
		"CAS expected jti must be the incoming oldJTI")
	assert.NotEmpty(t, repo.upsertSessionCASCalls[0].Session.CurrentJTI)
	assert.NotEqual(t, oldJTI, repo.upsertSessionCASCalls[0].Session.CurrentJTI)

	require.Len(t, bl.revokeCalls, 1, "happy path must blacklist the old jti")
	assert.Equal(t, oldJTI, bl.revokeCalls[0].JTI)
	assert.Equal(t, exp, bl.revokeCalls[0].Exp)
}

func TestRefreshToken_RotationRaceLost(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	oldJTI := "jti-old"
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", oldJTI, now.Add(time.Hour))}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			// Both racing requests observed current_jti=oldJTI here.
			return domain.Session{CurrentJTI: oldJTI, IssuedAt: now}, true, nil
		},
		upsertSessionIfMatch: func(_ context.Context, _ ids.UserID, _ string, _ string, _ domain.Session) error {
			// By the time this loser reached Mongo, the winner had already
			// rotated → CAS fails.
			return repoErrSessionStale()
		},
	}
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}
	bl := newFakeBlacklist()

	svc := newRefreshSvc(t, repo, rv, issuer, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "racing"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrAuthRefreshTokenRevoked),
		"CAS loser gets AUTH_REFRESH_TOKEN_REVOKED — single-use invariant preserved")

	// The loser DID revoke oldJTI (idempotent with the winner) — that's
	// acceptable because oldJTI is single-use anyway.
	require.Len(t, bl.revokeCalls, 1)
	assert.Equal(t, oldJTI, bl.revokeCalls[0].JTI)
}

func TestRefreshToken_InvalidSignature(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Now().UTC())
	rv := &fakeRefreshVerifier{err: errors.New("bad signature")}

	svc := newRefreshSvc(t, &fakeRepo{}, rv, &fakeIssuer{}, newFakeBlacklist(), clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "bad"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrAuthInvalidIdentityToken))
}

func TestRefreshToken_WrongTokenType(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))
	claims := refreshClaims("user-1", "device-a", "watch", "jti-old", clk.Now().Add(time.Hour))
	claims.TokenType = "access" // wrong!
	rv := &fakeRefreshVerifier{claims: claims}

	svc := newRefreshSvc(t, &fakeRepo{}, rv, &fakeIssuer{}, newFakeBlacklist(), clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "access-token"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrAuthInvalidIdentityToken))
}

func TestRefreshToken_BlacklistHit(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", "jti-old", now.Add(time.Hour))}
	bl := newFakeBlacklist()
	bl.isRevokedResp["jti-old"] = true
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}

	repo := &fakeRepo{}
	svc := newRefreshSvc(t, repo, rv, issuer, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "revoked"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrAuthRefreshTokenRevoked))
	assert.Equal(t, []string(nil), issuer.calls, "blacklisted token must NOT reach Issue")
	assert.Empty(t, repo.upsertSessionCalls, "blacklisted token must NOT update sessions")
	assert.Empty(t, repo.upsertSessionCASCalls, "blacklisted token must NOT update sessions (CAS path)")
}

func TestRefreshToken_BlacklistRedisError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", "jti-old", now.Add(time.Hour))}
	bl := newFakeBlacklist()
	bl.isRevokedErr = errors.New("redis dial tcp: connection refused")

	svc := newRefreshSvc(t, &fakeRepo{}, rv, &fakeIssuer{}, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "x"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError),
		"Redis error must surface as INTERNAL_ERROR (fail-closed, NOT 'assume clean')")
}

func TestRefreshToken_SessionNotInitialized(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", "jti-old", now.Add(time.Hour))}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{}, false, nil
		},
	}

	svc := newRefreshSvc(t, repo, rv, &fakeIssuer{}, newFakeBlacklist(), clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "x"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrAuthRefreshTokenRevoked))
}

func TestRefreshToken_ReuseDetected(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", "jti-old", now.Add(time.Hour))}
	// session.CurrentJTI points at a DIFFERENT jti — a legitimate refresh
	// already rotated the token and this call is a replay.
	currentJTI := "jti-current-live"
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: currentJTI, IssuedAt: now}, true, nil
		},
	}
	bl := newFakeBlacklist()
	issuer := &fakeIssuer{refreshExpiry: 30 * 24 * time.Hour}

	svc := newRefreshSvc(t, repo, rv, issuer, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "replayed"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrAuthRefreshTokenRevoked))

	// The critical assertion: the live jti (NOT the replayed one) must
	// be burned so the attacker cannot continue.
	require.Len(t, bl.revokeCalls, 1)
	assert.Equal(t, currentJTI, bl.revokeCalls[0].JTI,
		"reuse detection must burn the CURRENT live jti (OAuth2 RFC 6819 §5.2.2.3)")
	// Burn TTL should be ≈ configured refresh expiry from now
	assert.InDelta(t,
		float64(now.Add(30*24*time.Hour).Unix()),
		float64(bl.revokeCalls[0].Exp.Unix()),
		2.0, "reuse-detection burn TTL is configured RefreshExpiry")
	// No new tokens issued, no session mutated.
	assert.Empty(t, issuer.calls)
	assert.Empty(t, repo.upsertSessionCalls)
	assert.Empty(t, repo.upsertSessionCASCalls)
}

func TestRefreshToken_ReuseDetected_BurnRevokeError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", "jti-old", now.Add(time.Hour))}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: "jti-current-live", IssuedAt: now}, true, nil
		},
	}
	bl := newFakeBlacklist()
	bl.revokeAllError = errors.New("redis down")

	svc := newRefreshSvc(t, repo, rv, &fakeIssuer{}, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "replayed"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError),
		"reuse-detection burn failure ⇒ INTERNAL_ERROR (attack window still open, not a silent 401)")
}

func TestRefreshToken_UpsertSessionError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	oldJTI := "jti-old"
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", oldJTI, now.Add(time.Hour))}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: oldJTI, IssuedAt: now}, true, nil
		},
		// Non-CAS generic Mongo error (round-1 review P2: order changed
		// to Revoke → UpsertSession; a post-revoke UpsertSession Mongo
		// error is the accepted rare compound-failure tail).
		upsertSessionIfMatch: func(_ context.Context, _ ids.UserID, _ string, _ string, _ domain.Session) error {
			return errors.New("mongo write conflict")
		},
	}
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}
	bl := newFakeBlacklist()

	svc := newRefreshSvc(t, repo, rv, issuer, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "x"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError))
	// Revoke happens BEFORE UpsertSession after the P2 reorder — the
	// compound failure (Revoke OK + UpsertSession fails) locks the user
	// out via blacklist on retry, which is the accepted trade-off for
	// the P2 fix (single transient Revoke failure no longer forces
	// re-login).
	assert.Len(t, bl.revokeCalls, 1, "Revoke runs BEFORE UpsertSession after P2 reorder")
}

func TestRefreshToken_RevokeOldJTIError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	oldJTI := "jti-old"
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", oldJTI, now.Add(time.Hour))}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: oldJTI, IssuedAt: now}, true, nil
		},
	}
	bl := newFakeBlacklist()
	bl.revokeErrors[oldJTI] = errors.New("redis SET OOM")
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}

	svc := newRefreshSvc(t, repo, rv, issuer, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "x"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrInternalError),
		"Revoke failure ⇒ fail-closed 500; session unchanged so the client can retry with the same oldJTI")

	// After the P2 reorder Revoke is step 6 and UpsertSession is step 7,
	// so a Revoke failure MUST short-circuit before the session update —
	// that's the whole point: a transient blacklist write failure must
	// not permanently rotate the user's session.
	assert.Len(t, repo.upsertSessionCASCalls, 0,
		"session must NOT be rotated when Revoke failed (round-1 review P2)")
}

func TestRefreshToken_PerDeviceIndependence(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)

	userID := ids.UserID("user-1")
	watchJTI := "jti-watch-old"
	phoneJTI := "jti-phone-old"

	type sessionKey struct{ device string }
	sessions := map[sessionKey]domain.Session{
		{device: "device-watch"}: {CurrentJTI: watchJTI, IssuedAt: now.Add(-time.Hour)},
		{device: "device-phone"}: {CurrentJTI: phoneJTI, IssuedAt: now.Add(-time.Hour)},
	}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, d string) (domain.Session, bool, error) {
			s, ok := sessions[sessionKey{device: d}]
			return s, ok, nil
		},
	}
	rv := &fakeRefreshVerifier{claims: refreshClaims(string(userID), "device-watch", "watch", watchJTI, now.Add(time.Hour))}
	issuer := &fakeIssuer{issuedAccess: "A", issuedRefresh: "R"}
	bl := newFakeBlacklist()

	svc := newRefreshSvc(t, repo, rv, issuer, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "watch-token"})
	require.NoError(t, err)

	// Watch device session was upserted via CAS; phone was NOT touched.
	require.Len(t, repo.upsertSessionCASCalls, 1)
	assert.Equal(t, "device-watch", repo.upsertSessionCASCalls[0].DeviceID)
	assert.Equal(t, watchJTI, repo.upsertSessionCASCalls[0].ExpectedJTI)

	// Blacklist saw only the watch jti revoked.
	require.Len(t, bl.revokeCalls, 1)
	assert.Equal(t, watchJTI, bl.revokeCalls[0].JTI)
}

func TestRefreshToken_JTIClaimCarried(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	oldJTI := "jti-old"
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", oldJTI, now.Add(time.Hour))}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: oldJTI, IssuedAt: now}, true, nil
		},
	}

	var captured []jwtx.CustomClaims
	captureIssuer := &capturingIssuer{out: &captured, access: "A", refresh: "R"}
	bl := newFakeBlacklist()

	svc := newRefreshSvc(t, repo, rv, captureIssuer, bl, clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "x"})
	require.NoError(t, err)
	require.Len(t, captured, 2)

	// Both tokens must carry a jti; the refresh token's jti must match
	// what landed in sessions[deviceId].current_jti.
	assert.NotEmpty(t, captured[0].RegisteredClaims.ID, "access token must carry a jti")
	assert.NotEmpty(t, captured[1].RegisteredClaims.ID, "refresh token must carry a jti")
	assert.NotEqual(t, captured[0].RegisteredClaims.ID, captured[1].RegisteredClaims.ID,
		"access and refresh jtis must be independent (review-antipatterns §3.5)")

	require.Len(t, repo.upsertSessionCASCalls, 1)
	assert.Equal(t, captured[1].RegisteredClaims.ID, repo.upsertSessionCASCalls[0].Session.CurrentJTI,
		"session.current_jti must equal the refresh token's jti — rolling-rotation invariant")
}

func TestRefreshToken_AccessTokenIssuedWithFreshJTI(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	oldJTI := "jti-old"
	rv := &fakeRefreshVerifier{claims: refreshClaims("user-1", "device-a", "watch", oldJTI, now.Add(time.Hour))}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: oldJTI, IssuedAt: now}, true, nil
		},
	}

	var captured []jwtx.CustomClaims
	captureIssuer := &capturingIssuer{out: &captured, access: "A", refresh: "R"}

	svc := newRefreshSvc(t, repo, rv, captureIssuer, newFakeBlacklist(), clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: "x"})
	require.NoError(t, err)
	require.Len(t, captured, 2)
	assert.NotEqual(t, oldJTI, captured[0].RegisteredClaims.ID, "new access jti must differ from the incoming refresh jti")
	assert.NotEqual(t, oldJTI, captured[1].RegisteredClaims.ID, "new refresh jti must differ from the incoming jti")
}

func TestRefreshToken_EmptyInput(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Now().UTC())
	svc := newRefreshSvc(t, &fakeRepo{}, &fakeRefreshVerifier{}, &fakeIssuer{}, newFakeBlacklist(), clk)
	_, err := svc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: ""})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrValidationError))
}

// ---- RevokeRefreshToken / RevokeAllUserTokens ----

func TestRevokeRefreshToken_SessionExists(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: "jti-live", IssuedAt: now}, true, nil
		},
	}
	bl := newFakeBlacklist()
	issuer := &fakeIssuer{refreshExpiry: 30 * 24 * time.Hour}
	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, issuer, bl, clk)

	require.NoError(t, svc.RevokeRefreshToken(context.Background(), "user-1", "device-a"))
	require.Len(t, bl.revokeCalls, 1)
	assert.Equal(t, "jti-live", bl.revokeCalls[0].JTI)
}

func TestRevokeRefreshToken_SessionAbsent(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Now().UTC())
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{}, false, nil
		},
	}
	bl := newFakeBlacklist()
	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, &fakeIssuer{}, bl, clk)

	require.NoError(t, svc.RevokeRefreshToken(context.Background(), "user-1", "device-absent"))
	assert.Empty(t, bl.revokeCalls, "no session ⇒ no Revoke call (idempotent)")
}

func TestRevokeRefreshToken_UserNotFound(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Now().UTC())
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{}, false, repoErrUserNotFound()
		},
	}
	bl := newFakeBlacklist()
	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, &fakeIssuer{}, bl, clk)

	require.NoError(t, svc.RevokeRefreshToken(context.Background(), "user-gone", "device-a"),
		"user-not-found must be idempotent (nil)")
}

func TestRevokeRefreshToken_BlacklistError(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Now().UTC())
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: "jti-live", IssuedAt: time.Now()}, true, nil
		},
	}
	bl := newFakeBlacklist()
	bl.revokeAllError = errors.New("redis down")

	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, &fakeIssuer{}, bl, clk)
	err := svc.RevokeRefreshToken(context.Background(), "user-1", "device-a")
	require.Error(t, err)
}

func TestRevokeAllUserTokens_TwoDevices(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	sessions := map[string]domain.Session{
		"device-watch": {CurrentJTI: "jti-w", IssuedAt: now},
		"device-phone": {CurrentJTI: "jti-p", IssuedAt: now},
	}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, d string) (domain.Session, bool, error) {
			s, ok := sessions[d]
			return s, ok, nil
		},
		listDeviceIDs: func(_ context.Context, _ ids.UserID) ([]string, error) {
			return []string{"device-watch", "device-phone"}, nil
		},
	}
	bl := newFakeBlacklist()
	issuer := &fakeIssuer{refreshExpiry: 30 * 24 * time.Hour}
	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, issuer, bl, clk)

	require.NoError(t, svc.RevokeAllUserTokens(context.Background(), "user-1"))
	require.Len(t, bl.revokeCalls, 2)
	jtis := []string{bl.revokeCalls[0].JTI, bl.revokeCalls[1].JTI}
	assert.ElementsMatch(t, []string{"jti-w", "jti-p"}, jtis)
}

func TestRevokeAllUserTokens_EmptyList(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Now().UTC())
	repo := &fakeRepo{
		listDeviceIDs: func(_ context.Context, _ ids.UserID) ([]string, error) {
			return []string{}, nil
		},
	}
	bl := newFakeBlacklist()
	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, &fakeIssuer{}, bl, clk)

	require.NoError(t, svc.RevokeAllUserTokens(context.Background(), "user-1"))
	assert.Empty(t, bl.revokeCalls)
}

func TestRevokeAllUserTokens_PartialFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	sessions := map[string]domain.Session{
		"device-bad":  {CurrentJTI: "jti-bad", IssuedAt: now},
		"device-good": {CurrentJTI: "jti-good", IssuedAt: now},
	}
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, d string) (domain.Session, bool, error) {
			s, ok := sessions[d]
			return s, ok, nil
		},
		listDeviceIDs: func(_ context.Context, _ ids.UserID) ([]string, error) {
			return []string{"device-bad", "device-good"}, nil
		},
	}
	bl := newFakeBlacklist()
	bl.revokeErrors["jti-bad"] = errors.New("redis timeout")
	issuer := &fakeIssuer{refreshExpiry: 30 * 24 * time.Hour}

	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, issuer, bl, clk)
	err := svc.RevokeAllUserTokens(context.Background(), "user-1")
	require.Error(t, err, "first per-device failure must surface")
	require.Len(t, bl.revokeCalls, 2, "best-effort: second device must still be attempted")
}

func TestRevokeAllUserTokens_UserNotFound(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Now().UTC())
	repo := &fakeRepo{
		listDeviceIDs: func(_ context.Context, _ ids.UserID) ([]string, error) {
			return nil, repoErrUserNotFound()
		},
	}
	bl := newFakeBlacklist()
	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, &fakeIssuer{}, bl, clk)

	require.NoError(t, svc.RevokeAllUserTokens(context.Background(), "user-gone"))
	assert.Empty(t, bl.revokeCalls)
}

func TestRevokeRefreshToken_UsesConfiguredExpiry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(now)
	repo := &fakeRepo{
		getSession: func(_ context.Context, _ ids.UserID, _ string) (domain.Session, bool, error) {
			return domain.Session{CurrentJTI: "jti-live", IssuedAt: now}, true, nil
		},
	}
	bl := newFakeBlacklist()
	customExpiry := 7 * 24 * time.Hour
	issuer := &fakeIssuer{refreshExpiry: customExpiry}
	svc := newRefreshSvc(t, repo, &fakeRefreshVerifier{}, issuer, bl, clk)

	require.NoError(t, svc.RevokeRefreshToken(context.Background(), "user-1", "device-a"))
	require.Len(t, bl.revokeCalls, 1)
	assert.Equal(t, now.Add(customExpiry).Unix(), bl.revokeCalls[0].Exp.Unix(),
		"Revoke must pass exp = now + JWTIssuer.RefreshExpiry()")
}
