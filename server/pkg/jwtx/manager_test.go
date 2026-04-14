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
	// flip last character
	bad := tok[:len(tok)-1] + "x"
	if _, err := m.ParseAccess(bad); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for bad signature, got %v", err)
	}
}
