package cryptox

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newKey(b byte) []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = b
	}
	return k
}

func TestNewAESGCMSealer_WrongKeyLength(t *testing.T) {
	t.Parallel()
	cases := []int{0, 1, 15, 16, 24, 31, 33, 64, 128}
	for _, n := range cases {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			_, err := NewAESGCMSealer(make([]byte, n))
			require.Error(t, err, "len=%d must error", n)
			assert.Contains(t, err.Error(), "32 bytes")
		})
	}
}

func TestNewAESGCMSealer_NilKey(t *testing.T) {
	t.Parallel()
	_, err := NewAESGCMSealer(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestSealOpen_RoundTrip(t *testing.T) {
	t.Parallel()
	s, err := NewAESGCMSealer(newKey(0xAB))
	require.NoError(t, err)

	pt := []byte("hello world — 你好世界")
	sealed, err := s.Seal(pt)
	require.NoError(t, err)

	got, err := s.Open(sealed)
	require.NoError(t, err)
	assert.Equal(t, pt, got)
}

// TestSeal_DifferentNonceEachCall locks the IND-CPA property: sealing the
// same plaintext twice MUST produce different byte slices. A regression
// that hardcodes the nonce (tempting for "deterministic" tests) would
// fail here — and a hardcoded nonce destroys GCM's security proof.
func TestSeal_DifferentNonceEachCall(t *testing.T) {
	t.Parallel()
	s, err := NewAESGCMSealer(newKey(0x55))
	require.NoError(t, err)

	pt := []byte("identical plaintext")
	a, err := s.Seal(pt)
	require.NoError(t, err)
	b, err := s.Seal(pt)
	require.NoError(t, err)
	assert.False(t, bytes.Equal(a, b),
		"two Seals of the same plaintext must differ (random nonce)")

	// Sanity: both still Open to the same plaintext.
	gotA, err := s.Open(a)
	require.NoError(t, err)
	gotB, err := s.Open(b)
	require.NoError(t, err)
	assert.Equal(t, pt, gotA)
	assert.Equal(t, pt, gotB)
}

func TestOpen_EmptyOrShortPayload(t *testing.T) {
	t.Parallel()
	s, err := NewAESGCMSealer(newKey(0x01))
	require.NoError(t, err)

	cases := [][]byte{nil, {}, make([]byte, 1), make([]byte, 11), make([]byte, 27)}
	for _, in := range cases {
		_, err := s.Open(in)
		assert.ErrorIs(t, err, ErrCipherTampered, "len=%d must be rejected", len(in))
	}
}

func TestOpen_TagTampered(t *testing.T) {
	t.Parallel()
	s, err := NewAESGCMSealer(newKey(0x77))
	require.NoError(t, err)

	pt := []byte("tampered-tag test")
	sealed, err := s.Seal(pt)
	require.NoError(t, err)

	sealed[len(sealed)-1] ^= 0x01
	_, err = s.Open(sealed)
	assert.ErrorIs(t, err, ErrCipherTampered)
}

func TestOpen_NonceTampered(t *testing.T) {
	t.Parallel()
	s, err := NewAESGCMSealer(newKey(0x22))
	require.NoError(t, err)

	pt := []byte("tampered-nonce test")
	sealed, err := s.Seal(pt)
	require.NoError(t, err)

	sealed[0] ^= 0x01
	_, err = s.Open(sealed)
	assert.ErrorIs(t, err, ErrCipherTampered)
}

func TestOpen_WrongKey(t *testing.T) {
	t.Parallel()
	sa, err := NewAESGCMSealer(newKey(0xAA))
	require.NoError(t, err)
	sb, err := NewAESGCMSealer(newKey(0xBB))
	require.NoError(t, err)

	sealed, err := sa.Seal([]byte("message"))
	require.NoError(t, err)
	_, err = sb.Open(sealed)
	assert.ErrorIs(t, err, ErrCipherTampered)
}

func TestSealOpen_LargePayload(t *testing.T) {
	t.Parallel()
	s, err := NewAESGCMSealer(newKey(0x33))
	require.NoError(t, err)

	pt := bytes.Repeat([]byte("x"), 100*1024) // 100 KiB
	sealed, err := s.Seal(pt)
	require.NoError(t, err)
	got, err := s.Open(sealed)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(pt, got))
}

// TestErrCipherTampered_IsSentinel locks the errors.Is contract so
// callers who wrap ErrCipherTampered downstream (e.g. repository returns
// ErrApnsTokenCipherTampered that wraps us) keep a stable identity.
func TestErrCipherTampered_IsSentinel(t *testing.T) {
	t.Parallel()
	assert.True(t, errors.Is(ErrCipherTampered, ErrCipherTampered))
}
