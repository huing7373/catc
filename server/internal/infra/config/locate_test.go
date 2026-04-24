package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocateIn_PicksFirstExistingCWDCandidate(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "missing.yaml")
	present := filepath.Join(tmp, "present.yaml")
	if err := os.WriteFile(present, []byte("server:\n  http_port: 1\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := locateIn([]string{missing, present}, nil)
	if err != nil {
		t.Fatalf("locateIn returned unexpected error: %v", err)
	}
	if got != present {
		t.Errorf("locateIn returned %q, want %q", got, present)
	}
}

func TestLocateIn_AllMissingFallsBackToExe(t *testing.T) {
	tmp := t.TempDir()
	exeFallback := filepath.Join(tmp, "exe-relative.yaml")
	if err := os.WriteFile(exeFallback, []byte("server:\n  http_port: 1\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := locateIn(
		[]string{filepath.Join(tmp, "never.yaml")},
		func() []string { return []string{exeFallback} },
	)
	if err != nil {
		t.Fatalf("locateIn returned unexpected error: %v", err)
	}
	if got != exeFallback {
		t.Errorf("locateIn returned %q, want %q", got, exeFallback)
	}
}

func TestLocateIn_AllMissingReturnsError(t *testing.T) {
	tmp := t.TempDir()
	_, err := locateIn(
		[]string{filepath.Join(tmp, "a.yaml"), filepath.Join(tmp, "b.yaml")},
		func() []string { return []string{filepath.Join(tmp, "c.yaml")} },
	)
	if err == nil {
		t.Fatalf("locateIn returned nil error, want not-found error")
	}
	if !strings.Contains(err.Error(), "no config file found") {
		t.Errorf("error message = %q, want substring %q", err.Error(), "no config file found")
	}
	if !strings.Contains(err.Error(), "-config") {
		t.Errorf("error message = %q, should mention -config flag hint", err.Error())
	}
}

func TestLocateIn_IgnoresDirectoryMatches(t *testing.T) {
	tmp := t.TempDir()
	dirCandidate := filepath.Join(tmp, "looks-like-file")
	if err := os.Mkdir(dirCandidate, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	filePath := filepath.Join(tmp, "real.yaml")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := locateIn([]string{dirCandidate, filePath}, nil)
	if err != nil {
		t.Fatalf("locateIn returned unexpected error: %v", err)
	}
	if got != filePath {
		t.Errorf("locateIn returned %q, want %q (directories must be skipped)", got, filePath)
	}
}
