package ws

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/pkg/ids"
)

// profileUpdater is the service surface the handler consumes. Declared
// locally so internal/ws does not depend on internal/service at
// compile time; the production wiring binds *service.ProfileService
// (consumer-side P2 §6.2, same pattern as userByIDLookup in
// user_provider.go).
type profileUpdater interface {
	Update(ctx context.Context, userID ids.UserID, p ProfileUpdateInput) (*domain.User, error)
}

// ProfileUpdateInput mirrors service.ProfileUpdate as a
// handler-package-local type. A single alias would create an
// internal/ws → internal/service import (forbidden direction per
// §M8 / review-antipatterns §13.1), so we redeclare the three
// pointer fields. The handler→service wiring in initialize.go converts
// between the two at construction time via a small adapter.
type ProfileUpdateInput struct {
	DisplayName *string
	Timezone    *string
	QuietHours  *domain.QuietHours
}

// ProfileHandler dispatches the `profile.update` WS RPC. Wrapped in
// dedup middleware via Dispatcher.RegisterDedup in initialize.go so
// replay of the same envelope.id returns the cached result without
// re-hitting Mongo (NFR-SEC-9; Story 0.10 contract).
type ProfileHandler struct {
	svc profileUpdater
}

// NewProfileHandler wires the handler against the service. nil panics
// at startup (§P3 fail-fast), never at request time.
func NewProfileHandler(svc profileUpdater) *ProfileHandler {
	if svc == nil {
		panic("ws.NewProfileHandler: service must not be nil")
	}
	return &ProfileHandler{svc: svc}
}

// HandleUpdate satisfies HandlerFunc. Flow:
//
//  1. Decode envelope.Payload into dto.ProfileUpdateRequest.
//  2. Run dto.ValidateProfileUpdateRequest (WS path has no Gin binding
//     validator — the handler invokes the same validator the HTTP
//     path would, by hand).
//  3. Trim displayName (validator only checks "trim to non-empty");
//     the repo receives the trimmed value (Story 1.5 Semantic #8).
//  4. Call ProfileService.Update. Err mapping:
//     - ErrUserNotFound → INTERNAL_ERROR (do not leak existence).
//     - Any other error → INTERNAL_ERROR.
//  5. Marshal ProfileUpdateResponse from the returned *domain.User.
func (h *ProfileHandler) HandleUpdate(ctx context.Context, client *Client, env Envelope) (json.RawMessage, error) {
	var req dto.ProfileUpdateRequest
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, validationError("invalid profile.update payload")
		}
	}
	if err := dto.ValidateProfileUpdateRequest(&req); err != nil {
		return nil, err
	}

	input := toHandlerInput(&req)
	updated, err := h.svc.Update(ctx, ids.UserID(client.UserID()), input)
	if err != nil {
		// ErrUserNotFound leaking to the wire would let a client probe
		// which ids.UserID values exist in the system. Map it to the
		// generic internal error; logs still carry the truth via the
		// service's own error-level line.
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, dto.ErrInternalError.WithCause(err)
		}
		return nil, dto.ErrInternalError.WithCause(err)
	}

	resp := dto.ProfileUpdateResponse{User: dto.UserPublicProfileFromDomain(updated)}
	return json.Marshal(resp)
}

// toHandlerInput converts the wire DTO to the service-layer input type.
// displayName is trimmed *here* (after validation) so the repo sees
// the canonical value — a raw " Alice " passing the validator but
// landing in Mongo with spaces would surprise friend-list readers.
func toHandlerInput(req *dto.ProfileUpdateRequest) ProfileUpdateInput {
	out := ProfileUpdateInput{}
	if req.DisplayName != nil {
		trimmed := strings.TrimSpace(*req.DisplayName)
		out.DisplayName = &trimmed
	}
	if req.Timezone != nil {
		out.Timezone = req.Timezone
	}
	if req.QuietHours != nil {
		out.QuietHours = &domain.QuietHours{
			Start: req.QuietHours.Start,
			End:   req.QuietHours.End,
		}
	}
	return out
}

// Interface assertion keeps HandleUpdate's signature honest against the
// Dispatcher's HandlerFunc contract. If the shape drifts, the build
// fails here rather than at Dispatcher.RegisterDedup call time.
var _ HandlerFunc = (*ProfileHandler)(nil).HandleUpdate
