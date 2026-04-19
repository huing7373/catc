package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"empty header", "", ""},
		{"only scheme no token", "Bearer", ""},
		{"scheme + space + empty", "Bearer ", ""},
		{"happy path", "Bearer token123", "token123"},
		{"case-insensitive scheme lowercase", "bearer token123", "token123"},
		{"case-insensitive scheme mixed", "BeArEr token123", "token123"},
		{"basic scheme rejected", "Basic dXNlcjpwYXNz", ""},
		{"double space — token preserved minus leading whitespace",
			"Bearer  doubleSpaceToken", "doubleSpaceToken"},
		{"no space scheme run-on rejected", "Bearerfoo", ""},
		{"trailing whitespace trimmed", "Bearer token456  ", "token456"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractBearerToken(tc.header)
			assert.Equal(t, tc.want, got, "input: %q", tc.header)
		})
	}
}
