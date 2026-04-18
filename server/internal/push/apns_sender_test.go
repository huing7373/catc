package push

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApnsClient_MissingKeyPath_Error(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		keyPath string
		keyID   string
		teamID  string
	}{
		{"empty keyPath", "", "K", "T"},
		{"empty keyID", "path/to/key.p8", "", "T"},
		{"empty teamID", "path/to/key.p8", "K", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := NewApnsClient(tc.keyPath, tc.keyID, tc.teamID, false)
			assert.Error(t, err)
			assert.Nil(t, c)
		})
	}
}

func TestNewApnsClient_InvalidPEM_Error(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bogus := filepath.Join(dir, "bogus.p8")
	require.NoError(t, os.WriteFile(bogus, []byte("not a pem file"), 0o600))

	c, err := NewApnsClient(bogus, "KEYID123", "TEAMID12", false)
	assert.Error(t, err)
	assert.Nil(t, c)
}

func TestNewApnsClient_ValidKey_Success(t *testing.T) {
	t.Parallel()

	keyPath := filepath.Join("testdata", "test_key.p8")
	// Sanity check the fixture exists — the rest of the suite relies on it.
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("test fixture missing at %s: %v", keyPath, err)
	}

	c, err := NewApnsClient(keyPath, "KEYID123", "TEAMID12", false)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.NotNil(t, c.inner, "apns2.Client must be constructed")
}
