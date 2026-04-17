package logx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskPII(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"display name", "张三", "[REDACTED]"},
		{"email", "user@example.com", "[REDACTED]"},
		{"single char", "a", "[REDACTED]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, MaskPII(tt.input))
		})
	}
}

func TestMaskAPNsToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"short token (8 chars)", "abcd1234", "abcd1234"},
		{"short token (5 chars)", "abcde", "abcde"},
		{"full token 64 chars", "abcd1234efgh5678ijkl9012mnop3456qrst7890uvwx1234yzab5678cdef9012", "abcd1234..."},
		{"9 chars", "abcd12345", "abcd1234..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, MaskAPNsToken(tt.input))
		})
	}
}
