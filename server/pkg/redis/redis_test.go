package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_InvalidAddr(t *testing.T) {
	// Connecting to an invalid address should fail
	_, err := New("invalid:0", "")
	assert.Error(t, err)
}
