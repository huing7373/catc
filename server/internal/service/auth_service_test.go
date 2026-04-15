package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huing7373/catc/server/internal/domain"
	"github.com/huing7373/catc/server/internal/dto"
	"github.com/huing7373/catc/server/internal/repository"
	"github.com/huing7373/catc/server/pkg/applex"
	"github.com/huing7373/catc/server/pkg/ids"
	"github.com/huing7373/catc/server/pkg/jwtx"
)

// --- hand-written mocks (consumer-side interfaces) ---

type mockAppleVerifier struct {
	verifyFn func(ctx context.Context, idToken, rawNonce string) (*applex.Identity, error)
}

func (m *mockAppleVerifier) Verify(ctx context.Context, idToken, rawNonce string) (*applex.Identity, error) {
	return m.verifyFn(ctx, idToken, rawNonce)
}

type mockAuthRepo struct {
	upsertFn func(ctx context.Context, appleID, deviceID string, nowFn func() time.Time) (*domain.User, repository.LoginOutcome, error)
	findFn   func(ctx context.Context, id ids.UserID) (*domain.User, error)
}

func (m *mockAuthRepo) UpsertOnAppleLogin(ctx context.Context, appleID, deviceID string, nowFn func() time.Time) (*domain.User, repository.LoginOutcome, error) {
	return m.upsertFn(ctx, appleID, deviceID, nowFn)
}

func (m *mockAuthRepo) FindByID(ctx context.Context, id ids.UserID) (*domain.User, error) {
	return m.findFn(ctx, id)
}

type mockMinter struct {
	signAccessFn   func(uid ids.UserID) (string, error)
	signRefreshFn  func(uid ids.UserID) (string, error)
	parseRefreshFn func(token string) (ids.UserID, error)
}

func (m *mockMinter) SignAccess(uid ids.UserID) (string, error)  { return m.signAccessFn(uid) }
func (m *mockMinter) SignRefresh(uid ids.UserID) (string, error) { return m.signRefreshFn(uid) }
func (m *mockMinter) ParseRefresh(t string) (ids.UserID, error)  { return m.parseRefreshFn(t) }

func newMintsAlways(t *testing.T) *mockMinter {
	t.Helper()
	return &mockMinter{
		signAccessFn:  func(uid ids.UserID) (string, error) { return "AT-" + string(uid), nil },
		signRefreshFn: func(uid ids.UserID) (string, error) { return "RT-" + string(uid), nil },
	}
}

func fixedTime() time.Time { return time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC) }

func newAuthSvc(t *testing.T, ap *mockAppleVerifier, repo *mockAuthRepo, mint *mockMinter) *AuthService {
	t.Helper()
	svc := NewAuthService(ap, repo, mint, time.Hour, 30*24*time.Hour)
	svc.SetNowFn(fixedTime)
	return svc
}

// --- Login: 3 happy outcomes ---

func TestLogin_Happy_Created(t *testing.T) {
	ap := &mockAppleVerifier{verifyFn: func(context.Context, string, string) (*applex.Identity, error) {
		return &applex.Identity{Sub: "apple-1"}, nil
	}}
	repo := &mockAuthRepo{upsertFn: func(_ context.Context, appleID, deviceID string, _ func() time.Time) (*domain.User, repository.LoginOutcome, error) {
		if appleID != "apple-1" || deviceID != "dev" {
			t.Errorf("upsert args: %q %q", appleID, deviceID)
		}
		return &domain.User{ID: "u1"}, repository.OutcomeCreated, nil
	}}
	svc := newAuthSvc(t, ap, repo, newMintsAlways(t))

	pair, err := svc.Login(context.Background(), LoginInput{AppleJWT: "x", Nonce: "n", DeviceID: "dev"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.LoginOutcome != repository.OutcomeCreated {
		t.Errorf("outcome: %q", pair.LoginOutcome)
	}
	if pair.AccessToken != "AT-u1" || pair.RefreshToken != "RT-u1" {
		t.Errorf("tokens: %+v", pair)
	}
	if !pair.AccessExpiresAt.Equal(fixedTime().Add(time.Hour)) {
		t.Errorf("access expiry: %v", pair.AccessExpiresAt)
	}
	if !pair.RefreshExpiresAt.Equal(fixedTime().Add(30 * 24 * time.Hour)) {
		t.Errorf("refresh expiry: %v", pair.RefreshExpiresAt)
	}
}

func TestLogin_Happy_Existing(t *testing.T) {
	ap := &mockAppleVerifier{verifyFn: func(context.Context, string, string) (*applex.Identity, error) {
		return &applex.Identity{Sub: "apple-2"}, nil
	}}
	repo := &mockAuthRepo{upsertFn: func(context.Context, string, string, func() time.Time) (*domain.User, repository.LoginOutcome, error) {
		return &domain.User{ID: "u2"}, repository.OutcomeExisting, nil
	}}
	svc := newAuthSvc(t, ap, repo, newMintsAlways(t))

	pair, err := svc.Login(context.Background(), LoginInput{AppleJWT: "x"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.LoginOutcome != repository.OutcomeExisting {
		t.Errorf("outcome: %q", pair.LoginOutcome)
	}
}

func TestLogin_Happy_Restored(t *testing.T) {
	ap := &mockAppleVerifier{verifyFn: func(context.Context, string, string) (*applex.Identity, error) {
		return &applex.Identity{Sub: "apple-3"}, nil
	}}
	repo := &mockAuthRepo{upsertFn: func(context.Context, string, string, func() time.Time) (*domain.User, repository.LoginOutcome, error) {
		return &domain.User{ID: "u3"}, repository.OutcomeRestored, nil
	}}
	svc := newAuthSvc(t, ap, repo, newMintsAlways(t))

	pair, err := svc.Login(context.Background(), LoginInput{AppleJWT: "x"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.LoginOutcome != repository.OutcomeRestored {
		t.Errorf("outcome: %q", pair.LoginOutcome)
	}
}

// --- Login: failure branches ---

func TestLogin_AppleVerifyFails_BadSignature(t *testing.T) {
	ap := &mockAppleVerifier{verifyFn: func(context.Context, string, string) (*applex.Identity, error) {
		return nil, applex.ErrInvalidToken
	}}
	repo := &mockAuthRepo{upsertFn: func(context.Context, string, string, func() time.Time) (*domain.User, repository.LoginOutcome, error) {
		t.Fatalf("upsert must NOT be called when Apple verify fails")
		return nil, "", nil
	}}
	svc := newAuthSvc(t, ap, repo, newMintsAlways(t))

	_, err := svc.Login(context.Background(), LoginInput{AppleJWT: "x"})
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "APPLE_AUTH_FAIL" || ae.HTTPStatus != 401 {
		t.Fatalf("expected APPLE_AUTH_FAIL/401, got %v", err)
	}
}

func TestLogin_AppleVerifyFails_NonceMismatch(t *testing.T) {
	ap := &mockAppleVerifier{verifyFn: func(context.Context, string, string) (*applex.Identity, error) {
		return nil, applex.ErrNonceMismatch
	}}
	svc := newAuthSvc(t, ap, &mockAuthRepo{}, newMintsAlways(t))

	_, err := svc.Login(context.Background(), LoginInput{AppleJWT: "x"})
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "NONCE_MISMATCH" {
		t.Fatalf("expected NONCE_MISMATCH, got %v", err)
	}
}

func TestLogin_RepoConflict_RetrySuccess_ServiceSeesExisting(t *testing.T) {
	// Repo absorbs the dup-key race internally and returns existing.
	// Service should treat this as an ordinary OutcomeExisting login.
	ap := &mockAppleVerifier{verifyFn: func(context.Context, string, string) (*applex.Identity, error) {
		return &applex.Identity{Sub: "apple-race"}, nil
	}}
	repo := &mockAuthRepo{upsertFn: func(context.Context, string, string, func() time.Time) (*domain.User, repository.LoginOutcome, error) {
		return &domain.User{ID: "u-race"}, repository.OutcomeExisting, nil
	}}
	svc := newAuthSvc(t, ap, repo, newMintsAlways(t))

	pair, err := svc.Login(context.Background(), LoginInput{AppleJWT: "x"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.LoginOutcome != repository.OutcomeExisting {
		t.Errorf("outcome: %q", pair.LoginOutcome)
	}
}

func TestLogin_RepoConflict_RetryFails_MapsToAuthFail(t *testing.T) {
	ap := &mockAppleVerifier{verifyFn: func(context.Context, string, string) (*applex.Identity, error) {
		return &applex.Identity{Sub: "apple-stuck"}, nil
	}}
	repo := &mockAuthRepo{upsertFn: func(context.Context, string, string, func() time.Time) (*domain.User, repository.LoginOutcome, error) {
		return nil, "", repository.ErrConflict
	}}
	svc := newAuthSvc(t, ap, repo, newMintsAlways(t))

	_, err := svc.Login(context.Background(), LoginInput{AppleJWT: "x"})
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "APPLE_AUTH_FAIL" {
		t.Fatalf("expected APPLE_AUTH_FAIL on conflict, got %v", err)
	}
}

// --- Refresh ---

func TestRefresh_Happy(t *testing.T) {
	mint := newMintsAlways(t)
	mint.parseRefreshFn = func(string) (ids.UserID, error) { return "u-r", nil }
	repo := &mockAuthRepo{findFn: func(context.Context, ids.UserID) (*domain.User, error) {
		return &domain.User{ID: "u-r"}, nil
	}}
	svc := newAuthSvc(t, &mockAppleVerifier{}, repo, mint)

	pair, err := svc.Refresh(context.Background(), RefreshInput{RefreshToken: "rt"})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if pair.AccessToken != "AT-u-r" || pair.RefreshToken != "RT-u-r" {
		t.Errorf("tokens: %+v", pair)
	}
	if pair.LoginOutcome != "" {
		t.Errorf("refresh outcome should be empty, got %q", pair.LoginOutcome)
	}
}

func TestRefresh_Expired(t *testing.T) {
	mint := newMintsAlways(t)
	mint.parseRefreshFn = func(string) (ids.UserID, error) { return "", jwtx.ErrExpiredToken }
	svc := newAuthSvc(t, &mockAppleVerifier{}, &mockAuthRepo{}, mint)

	_, err := svc.Refresh(context.Background(), RefreshInput{RefreshToken: "rt"})
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "AUTH_EXPIRED" {
		t.Fatalf("expected AUTH_EXPIRED, got %v", err)
	}
}

func TestRefresh_Invalid(t *testing.T) {
	mint := newMintsAlways(t)
	mint.parseRefreshFn = func(string) (ids.UserID, error) { return "", jwtx.ErrInvalidToken }
	svc := newAuthSvc(t, &mockAppleVerifier{}, &mockAuthRepo{}, mint)

	_, err := svc.Refresh(context.Background(), RefreshInput{RefreshToken: "rt"})
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "AUTH_INVALID" {
		t.Fatalf("expected AUTH_INVALID, got %v", err)
	}
}

func TestRefresh_DeadAccount(t *testing.T) {
	mint := newMintsAlways(t)
	mint.parseRefreshFn = func(string) (ids.UserID, error) { return "u-dead", nil }
	repo := &mockAuthRepo{findFn: func(context.Context, ids.UserID) (*domain.User, error) {
		return nil, repository.ErrNotFound
	}}
	svc := newAuthSvc(t, &mockAppleVerifier{}, repo, mint)

	_, err := svc.Refresh(context.Background(), RefreshInput{RefreshToken: "rt"})
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED, got %v", err)
	}
}
