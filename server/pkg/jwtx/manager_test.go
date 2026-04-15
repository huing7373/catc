package jwtx

import (
	"errors"
	"testing"
	"time"

	"github.com/huing7373/catc/server/pkg/ids"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := New(Config{
		AccessSecret:  "access-secret",
		RefreshSecret: "refresh-secret",
		AccessTTL:     time.Minute,
		RefreshTTL:    time.Hour,
		Issuer:        "cat-test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

func TestNew_EmptySecret(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("expected ErrEmptySecret, got %v", err)
	}
	if _, err := New(Config{AccessSecret: "x"}); !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("expected ErrEmptySecret when refresh missing, got %v", err)
	}
}

func TestSignParseAccess_RoundTrip(t *testing.T) {
	m := newTestManager(t)
	tok, err := m.SignAccess(ids.UserID("user-1"))
	if err != nil {
		t.Fatalf("SignAccess: %v", err)
	}
	uid, err := m.ParseAccess(tok)
	if err != nil {
		t.Fatalf("ParseAccess: %v", err)
	}
	if uid != ids.UserID("user-1") {
		t.Fatalf("uid mismatch: %q", uid)
	}
}

func TestSignParseRefresh_RoundTrip(t *testing.T) {
	m := newTestManager(t)
	tok, err := m.SignRefresh(ids.UserID("user-2"))
	if err != nil {
		t.Fatalf("SignRefresh: %v", err)
	}
	uid, err := m.ParseRefresh(tok)
	if err != nil {
		t.Fatalf("ParseRefresh: %v", err)
	}
	if uid != "user-2" {
		t.Fatalf("uid: %q", uid)
	}
}

func TestParseAccess_WrongKindRejected(t *testing.T) {
	m := newTestManager(t)
	refresh, err := m.SignRefresh("u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ParseAccess(refresh); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for wrong kind, got %v", err)
	}
}

func TestParseAccess_ExpiredReturnsExpiredSentinel(t *testing.T) {
	m, err := New(Config{
		AccessSecret:  "a",
		RefreshSecret: "b",
		AccessTTL:     -time.Second, // already expired
		RefreshTTL:    time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := m.SignAccess("u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ParseAccess(tok); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestParseAccess_BadSignatureRejected(t *testing.T) {
	m := newTestManager(t)
	tok, _ := m.SignAccess("u")
	// Replace the last char with one that won't collide. JWT signatures
	// are url-safe base64 so flipping by 'A' avoids coincidental matches
	// when the original char was already 'x'.
	last := tok[len(tok)-1]
	repl := byte('A')
	if last == repl {
		repl = 'B'
	}
	bad := tok[:len(tok)-1] + string(repl)
	if _, err := m.ParseAccess(bad); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for bad signature, got %v", err)
	}
}

// --- dual-secret rotation (AC #2 / AC #12) ---

func newDualKeyManager(t *testing.T, accessTTL time.Duration) (current, previous *Manager, dual *Manager) {
	t.Helper()
	mk := func(prevA, prevR string) *Manager {
		m, err := New(Config{
			AccessSecret:          "access-current",
			RefreshSecret:         "refresh-current",
			AccessSecretPrevious:  prevA,
			RefreshSecretPrevious: prevR,
			AccessTTL:             accessTTL,
			RefreshTTL:            time.Hour,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return m
	}
	// previous-only manager mints tokens with the OLD secret.
	previous, err := New(Config{
		AccessSecret:  "access-previous",
		RefreshSecret: "refresh-previous",
		AccessTTL:     accessTTL,
		RefreshTTL:    time.Hour,
	})
	if err != nil {
		t.Fatalf("New previous: %v", err)
	}
	current = mk("", "")                                  // single-key manager
	dual = mk("access-previous", "refresh-previous")      // dual-key manager
	return current, previous, dual
}

func TestParse_DualKey_CurrentSignsCurrentParses(t *testing.T) {
	_, _, dual := newDualKeyManager(t, time.Minute)
	tok, err := dual.SignAccess("u-cur")
	if err != nil {
		t.Fatal(err)
	}
	uid, err := dual.ParseAccess(tok)
	if err != nil {
		t.Fatalf("ParseAccess: %v", err)
	}
	if uid != "u-cur" {
		t.Fatalf("uid: %q", uid)
	}
}

func TestParse_DualKey_PreviousSignsParsesViaFallback(t *testing.T) {
	_, prev, dual := newDualKeyManager(t, time.Minute)
	tok, err := prev.SignAccess("u-prev")
	if err != nil {
		t.Fatal(err)
	}
	uid, err := dual.ParseAccess(tok)
	if err != nil {
		t.Fatalf("ParseAccess (prev): %v", err)
	}
	if uid != "u-prev" {
		t.Fatalf("uid: %q", uid)
	}

	// And refresh side mirrors.
	rt, err := prev.SignRefresh("u-prev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dual.ParseRefresh(rt); err != nil {
		t.Fatalf("ParseRefresh (prev): %v", err)
	}
}

func TestParse_NoPrevious_StillCurrentOnly(t *testing.T) {
	cur, prev, _ := newDualKeyManager(t, time.Minute)
	// previous-signed token must FAIL on a single-key manager.
	tok, err := prev.SignAccess("u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cur.ParseAccess(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("single-key manager should reject prev-signed, got %v", err)
	}

	// And current-signed still succeeds.
	tok2, _ := cur.SignAccess("u")
	if _, err := cur.ParseAccess(tok2); err != nil {
		t.Fatalf("current rt: %v", err)
	}
}

func TestParse_DualKey_NeitherSecretWorks(t *testing.T) {
	_, _, dual := newDualKeyManager(t, time.Minute)
	stranger, err := New(Config{
		AccessSecret:  "stranger",
		RefreshSecret: "stranger-r",
		AccessTTL:     time.Minute,
		RefreshTTL:    time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := stranger.SignAccess("u")
	if _, err := dual.ParseAccess(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for unknown secret, got %v", err)
	}
}

func TestParse_DualKey_PreviousSignedExpired_ExpiryWins(t *testing.T) {
	_, prev, dual := newDualKeyManager(t, -time.Second) // both managers share TTL
	// prev signs a token that is already past exp.
	tok, err := prev.SignAccess("u")
	if err != nil {
		t.Fatal(err)
	}
	// Dual must surface ErrExpiredToken, NOT silently retry on expired.
	if _, err := dual.ParseAccess(tok); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken (expiry-before-rotation), got %v", err)
	}
}
