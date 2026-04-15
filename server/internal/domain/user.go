// Package domain contains entities and rules that are independent of
// HTTP, MongoDB, Redis, and any other infrastructure concern.
package domain

import (
	"time"

	"github.com/huing7373/catc/server/pkg/ids"
)

// NicknameMaxLen is the upper bound on display-name length enforced at
// the domain level.
const NicknameMaxLen = 40

// User is the domain representation of a registered user. It has no
// BSON/JSON tags: the repository layer is responsible for schema
// serialisation.
type User struct {
	ID                  ids.UserID
	AppleID             string
	DisplayName         string
	DeviceID            ids.DeviceID
	DnDStart            *time.Time // do-not-disturb window start; nil = no DnD
	DnDEnd              *time.Time
	IsDeleted           bool
	DeletionScheduledAt *time.Time
	CreatedAt           time.Time
	LastActiveAt        time.Time
}

// ValidateNickname returns ErrNicknameEmpty or ErrNicknameTooLong for
// inputs that would violate the domain invariant.
func ValidateNickname(name string) error {
	if name == "" {
		return ErrNicknameEmpty
	}
	// Count runes, not bytes — multi-byte characters each count as 1.
	n := 0
	for range name {
		n++
	}
	if n > NicknameMaxLen {
		return ErrNicknameTooLong
	}
	return nil
}

// CanChangeNameTo returns nil if newName is a valid, different display
// name. Callers layer additional cooldown / rate-limit checks on top via
// the cdCheck callback.
func (u *User) CanChangeNameTo(newName string, cdCheck func() error) error {
	if u.DisplayName == newName {
		return ErrSameName
	}
	if err := ValidateNickname(newName); err != nil {
		return err
	}
	if cdCheck != nil {
		if err := cdCheck(); err != nil {
			return err
		}
	}
	return nil
}
