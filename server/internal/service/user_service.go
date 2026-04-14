package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/huing7373/catc/server/internal/domain"
	"github.com/huing7373/catc/server/internal/repository"
	"github.com/huing7373/catc/server/pkg/ids"
)

// UserSvc is the contract consumed by the HTTP handler. Handler code
// holds this interface, never *UserService.
type UserSvc interface {
	GetProfile(ctx context.Context, id ids.UserID) (*domain.User, error)
	UpdateDisplayName(ctx context.Context, id ids.UserID, name string) error
}

// userRepo is the repository surface the user service needs. Declared
// here (the consumer side) per the layering rule: interfaces belong to
// the package that calls them.
type userRepo interface {
	FindByID(ctx context.Context, id ids.UserID) (*domain.User, error)
	UpdateDisplayName(ctx context.Context, id ids.UserID, name string) error
}

// UserService implements UserSvc.
type UserService struct {
	repo userRepo
}

// NewUserService builds a *UserService. The constructor does no I/O.
func NewUserService(repo userRepo) *UserService {
	return &UserService{repo: repo}
}

// GetProfile returns the user's profile. repository.ErrNotFound is
// translated into the stable AppError ErrUserNotFound; anything else is
// wrapped and surfaces as 500.
func (s *UserService) GetProfile(ctx context.Context, id ids.UserID) (*domain.User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrUserNotFound.WithCause(err)
		}
		return nil, fmt.Errorf("user service: find by id: %w", err)
	}
	return u, nil
}

// UpdateDisplayName validates + persists a new display name. Same-value
// and invalid-nickname cases return dedicated AppErrors so handlers can
// map them to HTTP 400 without string-matching.
func (s *UserService) UpdateDisplayName(ctx context.Context, id ids.UserID, name string) error {
	current, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrUserNotFound.WithCause(err)
		}
		return fmt.Errorf("user service: load for update: %w", err)
	}
	if err := current.CanChangeNameTo(name, nil); err != nil {
		switch {
		case errors.Is(err, domain.ErrSameName):
			return ErrNicknameSame.WithCause(err)
		case errors.Is(err, domain.ErrNicknameEmpty), errors.Is(err, domain.ErrNicknameTooLong):
			return ErrNicknameInvalid.WithCause(err)
		default:
			return fmt.Errorf("user service: can change name: %w", err)
		}
	}
	if err := s.repo.UpdateDisplayName(ctx, id, name); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrUserNotFound.WithCause(err)
		}
		return fmt.Errorf("user service: persist display name: %w", err)
	}
	return nil
}
