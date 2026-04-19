package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

// userByIDLookup is the shape RealUserProvider needs from the user
// repository. Defined locally so this file does not import
// internal/service or internal/repository for an interface — the
// concrete *repository.MongoUserRepository satisfies it implicitly.
//
// This is the same pattern session_resume.go uses for ResumeCache /
// ResumeProviders: ws/* depends on a small consumer-side surface, the
// production wiring binds the real Mongo repo.
type userByIDLookup interface {
	FindByID(ctx context.Context, id ids.UserID) (*domain.User, error)
}

// RealUserProvider is the Story 1.1 replacement for EmptyUserProvider.
// It loads the authenticated user's domain.User by id and returns a
// JSON projection (UserPublic + preferences.quietHours) suitable for
// the session.resume payload's `user` field.
//
// Story 1.5 will extend the projection with the full preferences /
// timezone block once the profile endpoint lands. Until then, the
// session.resume client sees the same UserPublic the SignInWithApple
// response returns plus the seed quiet-hours window — enough for the
// Watch UI to render the home screen on first launch.
type RealUserProvider struct {
	repo  userByIDLookup
	clock clockx.Clock
}

// NewRealUserProvider wires the provider against the user repository.
// Both arguments are required — nil panics at startup, never at
// request time.
func NewRealUserProvider(repo userByIDLookup, clk clockx.Clock) *RealUserProvider {
	if repo == nil {
		panic("ws.NewRealUserProvider: repo must not be nil")
	}
	if clk == nil {
		panic("ws.NewRealUserProvider: clock must not be nil")
	}
	return &RealUserProvider{repo: repo, clock: clk}
}

// resumeUserPayload is the JSON shape the session.resume cache stores
// for the `user` field. Decoupled from dto.UserPublic so we can extend
// with quiet hours without expanding the SignInWithApple response.
type resumeUserPayload struct {
	dto.UserPublic
	Preferences resumeUserPrefs `json:"preferences"`
}

type resumeUserPrefs struct {
	QuietHours resumeQuietHours `json:"quietHours"`
}

type resumeQuietHours struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// GetUser implements ws.UserProvider. Returns the JSON-marshalled user
// payload for userID, or an error that the SessionResumeHandler will
// surface as ErrInternalError. A NotFound is treated as an error
// (not a silent null) — by the time session.resume runs, the JWT
// middleware has already validated the userId, so a missing user row
// is a data-corruption signal worth refusing the request over.
func (p *RealUserProvider) GetUser(ctx context.Context, userID string) (json.RawMessage, error) {
	if userID == "" {
		return nil, errors.New("ws.RealUserProvider: empty userID")
	}
	u, err := p.repo.FindByID(ctx, ids.UserID(userID))
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, fmt.Errorf("ws.RealUserProvider: user %s not found", userID)
		}
		return nil, fmt.Errorf("ws.RealUserProvider: load user: %w", err)
	}
	payload := resumeUserPayload{
		UserPublic: dto.UserPublicFromDomain(u),
		Preferences: resumeUserPrefs{
			QuietHours: resumeQuietHours{
				Start: u.Preferences.QuietHours.Start,
				End:   u.Preferences.QuietHours.End,
			},
		},
	}
	return json.Marshal(payload)
}
