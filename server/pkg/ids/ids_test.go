package ids

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// uuidV4Pattern matches the canonical UUID v4 textual form: 8-4-4-4-12 hex
// digits with the version nibble pinned to 4 and the variant nibble in the
// 8/9/a/b set.
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUserID_UUID4Format(t *testing.T) {
	t.Parallel()

	id := NewUserID()
	assert.Len(t, string(id), 36, "UUID v4 textual form is exactly 36 chars")
	assert.True(t, uuidV4Pattern.MatchString(string(id)),
		"NewUserID must emit canonical UUID v4 (got %q)", string(id))
}

// TestNewUserID_Uniqueness rolls 1000 ids and asserts they are all
// distinct. A naive bug — e.g. seeding an RNG once and forgetting to
// advance it — would surface here long before it caused a Mongo
// duplicate-key collision in production.
func TestNewUserID_Uniqueness(t *testing.T) {
	t.Parallel()

	const n = 1000
	seen := make(map[UserID]struct{}, n)
	for i := range n {
		id := NewUserID()
		_, dup := seen[id]
		assert.False(t, dup, "NewUserID returned duplicate %q after %d iterations", id, i)
		seen[id] = struct{}{}
	}
	assert.Len(t, seen, n)
}
