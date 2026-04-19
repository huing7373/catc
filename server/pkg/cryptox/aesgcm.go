// Package cryptox wraps stdlib AEAD primitives for field-level at-rest
// encryption (NFR-SEC-7). Story 1.4 is the first consumer — APNs device
// tokens are sealed per-row before being persisted to the apns_tokens
// Mongo collection. Future stories that persist sensitive material (e.g.
// OAuth refresh tokens if Growth reintroduces third-party login, analytics
// PII blobs) reuse the same key / sealed-envelope shape.
//
// # Sealed-envelope layout
//
// Seal returns a single byte slice: [12-byte nonce | ciphertext | 16-byte
// GCM tag]. The caller stores the whole slice verbatim (binary BSON,
// base64 in TOML, etc.) — no separate nonce field, no length prefix.
// Open splits the slice, verifies the tag, and returns plaintext.
//
// # Nonce
//
// Every Seal call generates a fresh random 12-byte nonce via crypto/rand.
// Re-using a nonce under the same key against two different plaintexts
// leaks the XOR of those plaintexts and destroys authenticity; the stdlib
// AEAD interface documents this loudly. Randomization under GCM is safe
// up to ~2^32 messages per key (the nonce-reuse bound under IND-CPA) —
// well beyond any per-deployment APNs token volume.
//
// # Error surface
//
// All decrypt failures collapse to ErrCipherTampered. The underlying
// AEAD.Open may distinguish "bad tag" from "short ciphertext"; we do NOT
// surface that distinction to callers because (a) either implies the
// byte blob is no longer usable, and (b) a probing attacker should learn
// nothing about why Open rejected.
package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// ErrCipherTampered is the single error Open returns for any decrypt
// failure — too-short payload, wrong key, or a flipped bit in nonce /
// ciphertext / tag. Callers branch via errors.Is rather than string
// compare.
var ErrCipherTampered = errors.New("cryptox: ciphertext tampered or wrong key")

// AESGCMSealer holds an AES-256-GCM AEAD ready for repeated Seal / Open
// calls. Constructed once per process (the AEAD is cheap to reuse and
// contains no state that needs resetting between messages).
type AESGCMSealer struct {
	aead cipher.AEAD
}

// NewAESGCMSealer builds a sealer from a 32-byte AES-256 key. Fail-fast
// on wrong length (or nil) keeps misconfigured deployments from silently
// writing garbage — the caller's log.Fatal wrapper (e.g.
// config.validateAPNs / cmd/cat/initialize.go mustBuildApnsTokenRepo)
// makes startup loud.
func NewAESGCMSealer(key []byte) (*AESGCMSealer, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("cryptox: key must be 32 bytes (AES-256), got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		// aes.NewCipher only errors on bad key length, which we just
		// checked. Wrap defensively so a future stdlib change surfaces
		// as a constructor error instead of a panic.
		return nil, fmt.Errorf("cryptox: aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cryptox: gcm: %w", err)
	}
	return &AESGCMSealer{aead: aead}, nil
}

// Seal encrypts plaintext with a fresh random nonce. The returned slice
// is laid out as [nonce | ciphertext | tag] and is safe to persist
// verbatim. See package docs for nonce / re-use discussion.
func (s *AESGCMSealer) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("cryptox: rng: %w", err)
	}
	// cipher.AEAD.Seal appends the ciphertext + tag to `dst` and returns
	// the combined slice. Passing `nonce` as dst gives the [nonce|ct|tag]
	// envelope in a single allocation.
	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open reverses Seal. Returns ErrCipherTampered on short input, wrong
// key, or any GCM verification failure. Any non-tamper error (currently
// unreachable — AEAD.Open has no such path) is wrapped so callers can
// still inspect via errors.As.
func (s *AESGCMSealer) Open(sealed []byte) ([]byte, error) {
	ns := s.aead.NonceSize()
	if len(sealed) < ns+s.aead.Overhead() {
		return nil, ErrCipherTampered
	}
	nonce, ciphertext := sealed[:ns], sealed[ns:]
	pt, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrCipherTampered
	}
	return pt, nil
}
