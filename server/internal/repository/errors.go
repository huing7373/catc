package repository

import "errors"

// Sentinel errors produced by repositories. Services use errors.Is to
// branch on them.
var (
	// ErrNotFound indicates that the queried document does not exist or
	// is soft-deleted.
	ErrNotFound = errors.New("repository: not found")

	// ErrConflict indicates a uniqueness or optimistic-concurrency clash.
	ErrConflict = errors.New("repository: conflict")
)
