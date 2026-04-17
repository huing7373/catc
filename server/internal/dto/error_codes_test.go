package dto

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestAppError_Error(t *testing.T) {
	t.Parallel()
	ae := NewAppError("TEST_CODE", "test message", 400, CategoryClientError)
	assert.Equal(t, "TEST_CODE: test message", ae.Error())
}

func TestAppError_Unwrap(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("root cause")
	ae := NewAppError("TEST_CODE", "test message", 400, CategoryClientError).WithCause(cause)
	assert.Equal(t, cause, errors.Unwrap(ae))
}

func TestAppError_WithCause_ReturnsNewCopy(t *testing.T) {
	t.Parallel()
	original := NewAppError("TEST_CODE", "test message", 400, CategoryClientError)
	cause := fmt.Errorf("some cause")
	withCause := original.WithCause(cause)

	assert.Nil(t, original.Cause)
	assert.Equal(t, cause, withCause.Cause)
	assert.Equal(t, original.Code, withCause.Code)
}

func TestAppError_ErrorsIs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{
			name:   "same sentinel",
			err:    ErrInternalError,
			target: ErrInternalError,
			want:   true,
		},
		{
			name:   "with cause matches sentinel",
			err:    ErrInternalError.WithCause(fmt.Errorf("db timeout")),
			target: ErrInternalError,
			want:   true,
		},
		{
			name:   "different codes do not match",
			err:    ErrValidationError,
			target: ErrInternalError,
			want:   false,
		},
		{
			name:   "wrapped in fmt.Errorf matches via Unwrap chain",
			err:    fmt.Errorf("wrapper: %w", ErrFriendBlocked),
			target: ErrFriendBlocked,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, errors.Is(tt.err, tt.target))
		})
	}
}

func TestAppError_ErrorsAs(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("handler: %w", ErrAuthTokenExpired.WithCause(fmt.Errorf("jwt expired")))
	var ae *AppError
	require.True(t, errors.As(wrapped, &ae))
	assert.Equal(t, "AUTH_TOKEN_EXPIRED", ae.Code)
	assert.Equal(t, CategoryFatal, ae.Category)
}

func TestAllCodes_HaveCategory(t *testing.T) {
	t.Parallel()
	for _, code := range allCodes {
		assert.NotEmpty(t, code.Category, "error code %q has no category", code.Code)
	}
}

func TestRegistry_ContainsAllCodes(t *testing.T) {
	t.Parallel()
	reg := RegisteredCodes()
	for _, code := range allCodes {
		cat, ok := reg[code.Code]
		assert.True(t, ok, "error code %q not in registry", code.Code)
		assert.Equal(t, code.Category, cat, "category mismatch for %q", code.Code)
	}
}

func TestRegistry_NoDuplicates(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, code := range allCodes {
		assert.False(t, seen[code.Code], "duplicate error code %q", code.Code)
		seen[code.Code] = true
	}
}

func TestRegistry_AllSentinelsRegistered(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("error_codes.go")
	require.NoError(t, err)

	re := regexp.MustCompile(`(?m)^\s*(Err\w+)\s*=\s*NewAppError\("([A-Z_]+)"`)
	matches := re.FindAllStringSubmatch(string(src), -1)
	require.NotEmpty(t, matches, "failed to parse any sentinel from error_codes.go")

	registeredCodes := map[string]bool{}
	for _, code := range allCodes {
		registeredCodes[code.Code] = true
	}

	for _, m := range matches {
		varName, code := m[1], m[2]
		assert.True(t, registeredCodes[code],
			"sentinel %s (code %q) defined in error_codes.go but missing from allCodes slice", varName, code)
	}

	assert.Equal(t, len(matches), len(allCodes),
		"allCodes length (%d) != sentinel count in source (%d)", len(allCodes), len(matches))
}

func TestCategoryHTTPStatus_Mapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		sentinel   *AppError
		wantCat    ErrCategory
		wantStatus int
	}{
		{"INTERNAL_ERROR is retryable 500", ErrInternalError, CategoryRetryable, 500},
		{"VALIDATION_ERROR is client_error 400", ErrValidationError, CategoryClientError, 400},
		{"FRIEND_ALREADY_EXISTS is client_error 409", ErrFriendAlreadyExists, CategoryClientError, 409},
		{"RATE_LIMIT_EXCEEDED is retry_after 429", ErrRateLimitExceeded, CategoryRetryAfter, 429},
		{"AUTH_TOKEN_EXPIRED is fatal 401", ErrAuthTokenExpired, CategoryFatal, 401},
		{"DEVICE_BLACKLISTED is fatal 403", ErrDeviceBlacklisted, CategoryFatal, 403},
		{"FRIEND_INVITE_EXPIRED is client_error 410", ErrFriendInviteExpired, CategoryClientError, 410},
		{"BLINDBOX_INSUFFICIENT_STEPS is client_error 422", ErrBlindboxInsufficientSteps, CategoryClientError, 422},
		{"ROOM_FULL is client_error 409", ErrRoomFull, CategoryClientError, 409},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantCat, tt.sentinel.Category)
			assert.Equal(t, tt.wantStatus, tt.sentinel.HTTPStatus)
		})
	}
}

func TestRespondAppError_WithAppError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	cause := fmt.Errorf("database timeout")
	err := ErrInternalError.WithCause(cause)
	RespondAppError(c, err)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INTERNAL_ERROR", body["error"]["code"])
	assert.Equal(t, "internal server error", body["error"]["message"])
	assert.NotContains(t, w.Body.String(), "database timeout")
}

func TestRespondAppError_WithNonAppError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	RespondAppError(c, fmt.Errorf("some random error"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INTERNAL_ERROR", body["error"]["code"])
	assert.Equal(t, "internal server error", body["error"]["message"])
	assert.NotContains(t, w.Body.String(), "some random error")
}

func TestRespondAppError_CauseNotLeaked(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	secretCause := fmt.Errorf("secret: password=hunter2, db=prod")
	err := ErrFriendNotFound.WithCause(secretCause)
	RespondAppError(c, err)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "secret")
	assert.NotContains(t, w.Body.String(), "hunter2")
	assert.NotContains(t, w.Body.String(), "prod")
}

func TestRespondAppError_RetryAfterHeader(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	err := ErrRateLimitExceeded.WithRetryAfter(60)
	RespondAppError(c, err)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "60", w.Header().Get("Retry-After"))

	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", body["error"]["code"])
}

func TestRespondAppError_NoRetryAfterWhenZero(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	RespondAppError(c, ErrInternalError)

	assert.Empty(t, w.Header().Get("Retry-After"))
}

func TestRespondAppError_ClientError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)

	RespondAppError(c, ErrFriendAlreadyExists)

	assert.Equal(t, http.StatusConflict, w.Code)

	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "FRIEND_ALREADY_EXISTS", body["error"]["code"])
}

func TestErrorCodesMd_ConsistentWithRegistry(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../docs/error-codes.md")
	require.NoError(t, err, "docs/error-codes.md must exist")

	content := string(data)

	// Parse each row: | `CODE` | category | status | message |
	re := regexp.MustCompile("\\|\\s*`([A-Z_]+)`\\s*\\|\\s*(\\w+)\\s*\\|\\s*(\\d+)\\s*\\|\\s*([^|]+?)\\s*\\|")
	matches := re.FindAllStringSubmatch(content, -1)
	require.NotEmpty(t, matches, "failed to parse any rows from docs/error-codes.md")

	type docEntry struct {
		category string
		status   int
		message  string
	}
	docCodes := map[string]docEntry{}
	for _, m := range matches {
		status, err := strconv.Atoi(m[3])
		require.NoError(t, err)
		docCodes[m[1]] = docEntry{
			category: m[2],
			status:   status,
			message:  strings.TrimSpace(m[4]),
		}
	}

	for _, ae := range allCodes {
		entry, ok := docCodes[ae.Code]
		if !assert.True(t, ok, "error code %q in registry but missing from docs/error-codes.md", ae.Code) {
			continue
		}
		assert.Equal(t, string(ae.Category), entry.category,
			"category mismatch for %q in docs/error-codes.md", ae.Code)
		assert.Equal(t, ae.HTTPStatus, entry.status,
			"HTTP status mismatch for %q in docs/error-codes.md", ae.Code)
		assert.Equal(t, ae.Message, entry.message,
			"message mismatch for %q in docs/error-codes.md", ae.Code)
	}

	assert.Equal(t, len(allCodes), len(docCodes),
		"docs/error-codes.md has %d codes but registry has %d", len(docCodes), len(allCodes))
}
