package main

import (
	"context"
	"errors"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/ids"
)

// quietHoursUserLookupAdapter bridges
// *repository.MongoUserRepository.FindByID (returning
// (*domain.User, error) with repository.ErrUserNotFound) to the
// push.RealQuietHoursResolver's (user, found, err) interface.
//
// Lives in cmd/cat because only the wiring layer knows both types;
// internal/push cannot import internal/repository (cycle:
// repository → push for TokenInfo), so the adapter MUST be on the
// initialize.go side.
type quietHoursUserLookupAdapter struct {
	repo *repository.MongoUserRepository
}

// FindByID satisfies push.quietHoursUserLookup by mapping
// repository.ErrUserNotFound → found=false, err=nil (fail-open path),
// and passing other errors through unchanged so the APNs worker's
// warn-log path sees them.
func (a *quietHoursUserLookupAdapter) FindByID(ctx context.Context, id ids.UserID) (*domain.User, bool, error) {
	u, err := a.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return u, true, nil
}

// profileServiceHandlerAdapter bridges the service-package
// ProfileService (which consumes service.ProfileUpdate) and the
// ws-package ProfileHandler (which consumes ws.ProfileUpdateInput).
// A single alias would form an internal/ws → internal/service import
// (forbidden direction per review-antipatterns §13.1); the adapter
// does the one-shot conversion at the wiring boundary.
type profileServiceHandlerAdapter struct {
	svc *service.ProfileService
}

// Update satisfies ws.profileUpdater.
func (a *profileServiceHandlerAdapter) Update(ctx context.Context, userID ids.UserID, p ws.ProfileUpdateInput) (*domain.User, error) {
	return a.svc.Update(ctx, userID, service.ProfileUpdate{
		DisplayName: p.DisplayName,
		Timezone:    p.Timezone,
		QuietHours:  p.QuietHours,
	})
}
