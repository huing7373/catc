// Package ids defines typed identifiers used across the cat backend.
// Typed IDs prevent accidental cross-wiring between different entity
// kinds (e.g., passing a SkinID where a UserID is expected).
package ids

// UserID identifies a user.
type UserID string

// SkinID identifies a skin asset.
type SkinID string

// FriendshipID identifies a friendship (friend-pair) record.
type FriendshipID string

// GiftID identifies a serialized gift sequence entry.
type GiftID string

// DeviceID identifies a physical device (watch or phone) claimed by a user.
type DeviceID string
