package dto_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
)

func ptr(s string) *string { return &s }

// --- ValidateProfileUpdateRequest: rejection cases ---

func TestValidateProfileUpdateRequest_EmptyPayload(t *testing.T) {
	t.Parallel()
	err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{})
	require.Error(t, err)
	appErr := mustAppErr(t, err)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
	assert.Contains(t, appErr.Message, "at least one of displayName")
}

func TestValidateProfileUpdateRequest_NilPayload(t *testing.T) {
	t.Parallel()
	err := dto.ValidateProfileUpdateRequest(nil)
	require.Error(t, err)
	assert.Equal(t, "VALIDATION_ERROR", mustAppErr(t, err).Code)
}

func TestValidateProfileUpdateRequest_DisplayName_TrimTooShort(t *testing.T) {
	t.Parallel()
	cases := []string{"", " ", "\t", "    "}
	for _, s := range cases {
		t.Run("whitespace-only="+s, func(t *testing.T) {
			t.Parallel()
			err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{DisplayName: ptr(s)})
			require.Error(t, err)
			msg := mustAppErr(t, err).Message
			// Either "at least 1 character" (after trim) or "control
			// characters" for \t — both are valid rejections for the
			// semantic "whitespace-only name".
			assert.True(t, strings.Contains(msg, "at least") || strings.Contains(msg, "control"),
				"got: %s", msg)
		})
	}
}

func TestValidateProfileUpdateRequest_DisplayName_TooLong(t *testing.T) {
	t.Parallel()
	// 33 'a' characters — one over the 32-rune cap after trim.
	err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{
		DisplayName: ptr(strings.Repeat("a", 33)),
	})
	require.Error(t, err)
	assert.Contains(t, mustAppErr(t, err).Message, "at most")
}

func TestValidateProfileUpdateRequest_DisplayName_ControlCharRejected(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"null":         "Ali\x00ce",
		"bell":         "Bo\x07b",
		"del":          "Eve\x7f",
		"escape":       "Mal\x1blory",
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{DisplayName: ptr(s)})
			require.Error(t, err)
			assert.Contains(t, mustAppErr(t, err).Message, "control")
		})
	}
}

func TestValidateProfileUpdateRequest_DisplayName_InvalidUTF8(t *testing.T) {
	t.Parallel()
	// 0xff is never valid as a UTF-8 start byte.
	bad := string([]byte{0xff, 0xfe, 0xfd})
	err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{DisplayName: ptr(bad)})
	require.Error(t, err)
	assert.Contains(t, mustAppErr(t, err).Message, "UTF-8")
}

func TestValidateProfileUpdateRequest_Timezone_Invalid(t *testing.T) {
	t.Parallel()
	bad := []string{"Pacific/Nope", "Not/A/Zone", "asdf", "Asia/ShangHai"}
	for _, tz := range bad {
		t.Run(tz, func(t *testing.T) {
			t.Parallel()
			err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{Timezone: ptr(tz)})
			require.Error(t, err)
			assert.Contains(t, mustAppErr(t, err).Message, "IANA")
		})
	}
}

func TestValidateProfileUpdateRequest_Timezone_EmptyString(t *testing.T) {
	t.Parallel()
	err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{Timezone: ptr("")})
	require.Error(t, err)
	assert.Equal(t, "VALIDATION_ERROR", mustAppErr(t, err).Code)
}

func TestValidateProfileUpdateRequest_QuietHours_InvalidFormat(t *testing.T) {
	t.Parallel()
	// Each case is a (start, end) pair with at least one malformed side.
	cases := []struct{ start, end string }{
		{"24:00", "07:00"}, // hours >23
		{"25:90", "07:00"}, // fully bogus
		{"23:5", "07:00"},  // missing leading zero on minute
		{"ab:cd", "07:00"}, // non-numeric
		{"23:00", "07:60"}, // minutes >59
		{"23:00", "7:00"},  // missing leading zero on hour
		{"", "07:00"},      // empty start
		{"23:00", ""},      // empty end
	}
	for _, c := range cases {
		t.Run(c.start+"->"+c.end, func(t *testing.T) {
			t.Parallel()
			err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{
				QuietHours: &dto.QuietHoursDTO{Start: c.start, End: c.end},
			})
			require.Error(t, err, "expected rejection for %q/%q", c.start, c.end)
			assert.Contains(t, mustAppErr(t, err).Message, "HH:MM")
		})
	}
}

// --- ValidateProfileUpdateRequest: acceptance cases ---

func TestValidateProfileUpdateRequest_DisplayName_Valid(t *testing.T) {
	t.Parallel()
	valid := []string{
		"Alice",
		"王小明",       // CJK
		"John Doe",
		" Alice ",   // trimmable to valid
		"a",         // 1 rune (minimum)
		strings.Repeat("x", 32), // exactly 32 runes (max)
		"猫咪大师",
		"🐈",          // emoji
	}
	for _, s := range valid {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{DisplayName: ptr(s)})
			assert.NoError(t, err)
		})
	}
}

func TestValidateProfileUpdateRequest_Timezone_Valid(t *testing.T) {
	t.Parallel()
	valid := []string{"Asia/Shanghai", "America/New_York", "UTC", "Europe/London", "Pacific/Auckland"}
	for _, tz := range valid {
		t.Run(tz, func(t *testing.T) {
			t.Parallel()
			err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{Timezone: ptr(tz)})
			assert.NoError(t, err)
		})
	}
}

func TestValidateProfileUpdateRequest_QuietHours_Valid(t *testing.T) {
	t.Parallel()
	valid := []dto.QuietHoursDTO{
		{Start: "00:00", End: "23:59"},
		{Start: "23:00", End: "07:00"}, // overnight
		{Start: "10:00", End: "15:00"}, // same-day window
		{Start: "22:00", End: "22:00"}, // equal — 24h silent, allowed
		{Start: "23:59", End: "00:00"}, // 1-minute before midnight
	}
	for _, q := range valid {
		t.Run(q.Start+"->"+q.End, func(t *testing.T) {
			t.Parallel()
			err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{
				QuietHours: &dto.QuietHoursDTO{Start: q.Start, End: q.End},
			})
			assert.NoError(t, err)
		})
	}
}

func TestValidateProfileUpdateRequest_AllThreeFields(t *testing.T) {
	t.Parallel()
	err := dto.ValidateProfileUpdateRequest(&dto.ProfileUpdateRequest{
		DisplayName: ptr("Alice"),
		Timezone:    ptr("Asia/Shanghai"),
		QuietHours:  &dto.QuietHoursDTO{Start: "23:00", End: "07:00"},
	})
	assert.NoError(t, err)
}

func TestValidateProfileUpdateRequest_SingleFieldOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  dto.ProfileUpdateRequest
	}{
		{name: "displayName-only", req: dto.ProfileUpdateRequest{DisplayName: ptr("Alice")}},
		{name: "timezone-only", req: dto.ProfileUpdateRequest{Timezone: ptr("UTC")}},
		{name: "quietHours-only", req: dto.ProfileUpdateRequest{QuietHours: &dto.QuietHoursDTO{Start: "22:00", End: "06:00"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := dto.ValidateProfileUpdateRequest(&c.req)
			assert.NoError(t, err)
		})
	}
}

// --- ProfileUpdateRequest JSON decoding ---

func TestProfileUpdateRequest_JSONRoundtrip(t *testing.T) {
	t.Parallel()
	raw := `{"displayName":"Alice","timezone":"Asia/Shanghai","quietHours":{"start":"23:00","end":"07:00"}}`
	var req dto.ProfileUpdateRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &req))
	require.NotNil(t, req.DisplayName)
	require.NotNil(t, req.Timezone)
	require.NotNil(t, req.QuietHours)
	assert.Equal(t, "Alice", *req.DisplayName)
	assert.Equal(t, "Asia/Shanghai", *req.Timezone)
	assert.Equal(t, "23:00", req.QuietHours.Start)
	assert.Equal(t, "07:00", req.QuietHours.End)
}

func TestProfileUpdateRequest_JSONPartial(t *testing.T) {
	t.Parallel()
	// Only displayName set; other fields must decode as nil pointers so
	// the validator sees them as "not changed".
	raw := `{"displayName":"Bob"}`
	var req dto.ProfileUpdateRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &req))
	assert.NotNil(t, req.DisplayName)
	assert.Nil(t, req.Timezone)
	assert.Nil(t, req.QuietHours)
}

// --- UserPublicProfileFromDomain projection ---

func TestUserPublicProfileFromDomain(t *testing.T) {
	t.Parallel()
	dn := "Alice"
	tz := "Asia/Shanghai"
	u := &domain.User{
		ID:          "u1",
		DisplayName: &dn,
		Timezone:    &tz,
		Preferences: domain.UserPreferences{QuietHours: domain.QuietHours{Start: "23:00", End: "07:00"}},
	}
	got := dto.UserPublicProfileFromDomain(u)
	assert.Equal(t, "u1", got.ID)
	require.NotNil(t, got.DisplayName)
	assert.Equal(t, "Alice", *got.DisplayName)
	require.NotNil(t, got.Timezone)
	assert.Equal(t, "Asia/Shanghai", *got.Timezone)
	assert.Equal(t, "23:00", got.Preferences.QuietHours.Start)
	assert.Equal(t, "07:00", got.Preferences.QuietHours.End)

	// Response envelope shape round-trips through JSON with the nested
	// preferences.quietHours intact.
	resp := dto.ProfileUpdateResponse{User: got}
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"quietHours":{"start":"23:00","end":"07:00"}`)
}

// --- helper ---

func mustAppErr(t *testing.T, err error) *dto.AppError {
	t.Helper()
	ae, ok := err.(*dto.AppError)
	require.True(t, ok, "expected *dto.AppError, got %T: %v", err, err)
	return ae
}
