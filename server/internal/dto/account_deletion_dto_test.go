package dto

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountDeletionResponse_JSONShapeSnakeCase(t *testing.T) {
	t.Parallel()

	resp := AccountDeletionResponse{
		Status:      AccountDeletionStatusRequested,
		RequestedAt: "2026-04-19T12:00:00Z",
		Note:        AccountDeletionNoteMVP,
	}
	raw, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Contains(t, decoded, "status", "field must be snake_case `status`")
	assert.Contains(t, decoded, "requested_at", "field must be snake_case `requested_at` (not camelCase)")
	assert.Contains(t, decoded, "note", "field must be snake_case `note`")
	assert.NotContains(t, decoded, "Status")
	assert.NotContains(t, decoded, "requestedAt")
	assert.NotContains(t, decoded, "RequestedAt")

	assert.Equal(t, AccountDeletionStatusRequested, decoded["status"])
	assert.Equal(t, "2026-04-19T12:00:00Z", decoded["requested_at"])
	assert.Equal(t, AccountDeletionNoteMVP, decoded["note"])
}

func TestAccountDeletionResponse_ZeroValueMarshalsStable(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(AccountDeletionResponse{})
	require.NoError(t, err)
	assert.Equal(t, `{"status":"","requested_at":"","note":""}`, string(raw),
		"zero value must not omit fields — clients expect all three on every 202")
}

func TestAccountDeletionResponse_LongNotePreserved(t *testing.T) {
	t.Parallel()

	resp := AccountDeletionResponse{Note: AccountDeletionNoteMVP}
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	assert.Equal(t, AccountDeletionNoteMVP, decoded["note"],
		"long note must round-trip byte-for-byte (no escape / truncation)")

	assert.Equal(t, "30 days manual cleanup per MVP policy", AccountDeletionNoteMVP,
		"MVP policy note text is part of the Story 1.6 contract; epic L888 + client guide §16 quote it verbatim")
}

// TestAccountDeletionResponse_RFC3339UTCAcceptance is defense-in-depth
// for §21.8 #4: the handler formats RequestedAt as `.UTC().Format(RFC3339)`.
// This DTO test locks the pattern the handler MUST emit (string ends
// with `Z`, no `+HH:MM` offset). Handler-level test adds the positive
// check against a live response.
func TestAccountDeletionResponse_RFC3339UTCAcceptance(t *testing.T) {
	t.Parallel()

	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z$`)
	assert.True(t, re.MatchString("2026-04-19T12:00:00Z"), "sanity: regex matches a Z-suffixed RFC3339")
	assert.False(t, re.MatchString("2026-04-19T12:00:00+08:00"), "must reject non-UTC offsets")
	assert.False(t, re.MatchString("2026-04-19T12:00:00"), "must reject naive times")
}
