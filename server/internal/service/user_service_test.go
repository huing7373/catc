package service

import (
	"context"
	"errors"
	"testing"

	"github.com/huing7373/catc/server/internal/domain"
	"github.com/huing7373/catc/server/internal/dto"
	"github.com/huing7373/catc/server/internal/repository"
	"github.com/huing7373/catc/server/pkg/ids"
)

// mockUserRepo is a hand-written implementation of the userRepo
// interface declared in user_service.go.
type mockUserRepo struct {
	findByIDFn          func(ctx context.Context, id ids.UserID) (*domain.User, error)
	updateDisplayNameFn func(ctx context.Context, id ids.UserID, name string) error
}

func (m *mockUserRepo) FindByID(ctx context.Context, id ids.UserID) (*domain.User, error) {
	return m.findByIDFn(ctx, id)
}

func (m *mockUserRepo) UpdateDisplayName(ctx context.Context, id ids.UserID, name string) error {
	return m.updateDisplayNameFn(ctx, id, name)
}

func TestUserService_GetProfile_Happy(t *testing.T) {
	want := &domain.User{ID: "u1", DisplayName: "alice"}
	svc := NewUserService(&mockUserRepo{
		findByIDFn: func(context.Context, ids.UserID) (*domain.User, error) { return want, nil },
	})
	got, err := svc.GetProfile(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if got != want {
		t.Fatalf("user pointer mismatch")
	}
}

func TestUserService_GetProfile_NotFound(t *testing.T) {
	svc := NewUserService(&mockUserRepo{
		findByIDFn: func(context.Context, ids.UserID) (*domain.User, error) {
			return nil, repository.ErrNotFound
		},
	})
	_, err := svc.GetProfile(context.Background(), "u1")
	var ae *dto.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected AppError, got %v", err)
	}
	if ae.Code != "USER_NOT_FOUND" || ae.HTTPStatus != 404 {
		t.Errorf("unexpected AppError: %+v", ae)
	}
}

func TestUserService_GetProfile_WrappedOnUnknownError(t *testing.T) {
	svc := NewUserService(&mockUserRepo{
		findByIDFn: func(context.Context, ids.UserID) (*domain.User, error) {
			return nil, errors.New("db unavailable")
		},
	})
	_, err := svc.GetProfile(context.Background(), "u1")
	var ae *dto.AppError
	if errors.As(err, &ae) {
		t.Fatalf("generic errors must NOT be AppError; got %+v", ae)
	}
}

func TestUserService_UpdateDisplayName_Happy(t *testing.T) {
	called := false
	svc := NewUserService(&mockUserRepo{
		findByIDFn: func(context.Context, ids.UserID) (*domain.User, error) {
			return &domain.User{ID: "u1", DisplayName: "old"}, nil
		},
		updateDisplayNameFn: func(ctx context.Context, id ids.UserID, name string) error {
			called = true
			if name != "new" || id != "u1" {
				t.Errorf("mock received unexpected args: %q / %q", id, name)
			}
			return nil
		},
	})
	if err := svc.UpdateDisplayName(context.Background(), "u1", "new"); err != nil {
		t.Fatalf("UpdateDisplayName: %v", err)
	}
	if !called {
		t.Error("repo UpdateDisplayName not called")
	}
}

func TestUserService_UpdateDisplayName_SameName(t *testing.T) {
	svc := NewUserService(&mockUserRepo{
		findByIDFn: func(context.Context, ids.UserID) (*domain.User, error) {
			return &domain.User{ID: "u1", DisplayName: "same"}, nil
		},
		updateDisplayNameFn: func(ctx context.Context, id ids.UserID, name string) error {
			t.Fatalf("repo must NOT be called for same-name")
			return nil
		},
	})
	err := svc.UpdateDisplayName(context.Background(), "u1", "same")
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "NICKNAME_SAME" {
		t.Fatalf("expected NICKNAME_SAME, got %v", err)
	}
}

func TestUserService_UpdateDisplayName_EmptyName(t *testing.T) {
	svc := NewUserService(&mockUserRepo{
		findByIDFn: func(context.Context, ids.UserID) (*domain.User, error) {
			return &domain.User{ID: "u1", DisplayName: "x"}, nil
		},
		updateDisplayNameFn: func(ctx context.Context, id ids.UserID, name string) error {
			t.Fatalf("repo must NOT be called for invalid name")
			return nil
		},
	})
	err := svc.UpdateDisplayName(context.Background(), "u1", "")
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "NICKNAME_INVALID" {
		t.Fatalf("expected NICKNAME_INVALID, got %v", err)
	}
}

func TestUserService_UpdateDisplayName_UserGone(t *testing.T) {
	svc := NewUserService(&mockUserRepo{
		findByIDFn: func(context.Context, ids.UserID) (*domain.User, error) {
			return nil, repository.ErrNotFound
		},
	})
	err := svc.UpdateDisplayName(context.Background(), "u1", "new")
	var ae *dto.AppError
	if !errors.As(err, &ae) || ae.Code != "USER_NOT_FOUND" {
		t.Fatalf("expected USER_NOT_FOUND, got %v", err)
	}
}
