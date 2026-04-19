package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDefaultPreferences_ReturnsFreshCopy guards M6: callers must not be
// able to mutate a shared singleton. If DefaultPreferences ever cached a
// package-level value, mutating one returned QuietHours would silently
// corrupt every subsequent SignInWithApple seed.
func TestDefaultPreferences_ReturnsFreshCopy(t *testing.T) {
	t.Parallel()

	a := DefaultPreferences()
	a.QuietHours.Start = "00:00"
	a.QuietHours.End = "00:00"

	b := DefaultPreferences()
	assert.Equal(t, "23:00", b.QuietHours.Start, "second call must not see the mutation made to the first")
	assert.Equal(t, "07:00", b.QuietHours.End, "second call must not see the mutation made to the first")
}

func TestDefaultPreferences_DefaultQuietHoursWindow(t *testing.T) {
	t.Parallel()

	pref := DefaultPreferences()
	assert.Equal(t, "23:00", pref.QuietHours.Start)
	assert.Equal(t, "07:00", pref.QuietHours.End)
}
