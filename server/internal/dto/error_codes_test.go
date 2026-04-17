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

func TestAppError_WithRetryAfter_ReturnsNewCopy(t *testing.T) {
	t.Parallel()
	original := NewAppError("TEST_CODE", "test", 429, CategoryRetryAfter)
	withRetry := original.WithRetryAfter(30)

	assert.Equal(t, 0, original.RetryAfter)
	assert.Equal(t, 30, withRetry.RetryAfter)
	assert.Equal(t, original.Code, withRetry.Code)
}

func TestNewAppError_PanicsOnInvalidCategory(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { NewAppError("BAD", "bad", 400, "") })
	assert.Panics(t, func() { NewAppError("BAD", "bad", 400, "nonexistent") })
	assert.NotPanics(t, func() { NewAppError("OK", "ok", 400, CategoryClientError) })
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

func TestRegistry_AllCodesHaveCategory(t *testing.T) {
	t.Parallel()
	for code, ae := range RegisteredCodes() {
		assert.NotEmpty(t, ae.Category, "error code %q has no category", code)
	}
}

func TestRegistry_NoDuplicates(t *testing.T) {
	t.Parallel()
	reg := RegisteredCodes()
	assert.NotEmpty(t, reg, "registry is empty")
}

func TestRegistry_AllSentinelsRegistered(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("error_codes.go")
	require.NoError(t, err)

	re := regexp.MustCompile(`(?m)^\s*(Err\w+)\s*=\s*register\("([A-Z_]+)"`)
	matches := re.FindAllStringSubmatch(string(src), -1)
	require.NotEmpty(t, matches, "failed to parse any sentinel from error_codes.go")

	reg := RegisteredCodes()
	for _, m := range matches {
		varName, code := m[1], m[2]
		_, ok := reg[code]
		assert.True(t, ok,
			"sentinel %s (code %q) defined in error_codes.go but missing from registry", varName, code)
	}

	assert.Equal(t, len(matches), len(reg),
		"registry size (%d) != sentinel count in source (%d)", len(reg), len(matches))
}

func TestCategoryHTTPStatus_AllCodes(t *testing.T) {
	t.Parallel()
	reg := RegisteredCodes()
	require.NotEmpty(t, reg)

	for code, ae := range reg {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			assert.NotEmpty(t, ae.Category, "missing category")
			assert.Greater(t, ae.HTTPStatus, 0, "invalid HTTP status")

			switch ae.Category {
			case CategoryRetryable:
				assert.GreaterOrEqual(t, ae.HTTPStatus, 500, "retryable should be 5xx")
			case CategoryClientError:
				assert.GreaterOrEqual(t, ae.HTTPStatus, 400, "client_error should be 4xx")
				assert.Less(t, ae.HTTPStatus, 500, "client_error should be 4xx")
			case CategoryRetryAfter:
				assert.Equal(t, 429, ae.HTTPStatus, "retry_after should be 429")
			case CategoryFatal:
				assert.True(t, ae.HTTPStatus == 401 || ae.HTTPStatus == 403,
					"fatal should be 401 or 403, got %d", ae.HTTPStatus)
			case CategorySilentDrop:
				assert.Equal(t, 200, ae.HTTPStatus, "silent_drop should be 200")
			default:
				t.Errorf("unknown category %q", ae.Category)
			}
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

	err := ErrRateLimitExceeded.WithRetryAfter(120)
	RespondAppError(c, err)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "120", w.Header().Get("Retry-After"))
}

func TestRespondAppError_RetryAfterDefaultForCategory(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	RespondAppError(c, ErrRateLimitExceeded)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "60", w.Header().Get("Retry-After"))
}

func TestRespondAppError_NoRetryAfterForOtherCategories(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	RespondAppError(c, ErrInternalError)

	assert.Empty(t, w.Header().Get("Retry-After"))
}

func TestRespondAppError_TypedNilAppError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	var nilAE *AppError
	// typed-nil interface: error interface holds (*AppError)(nil)
	var err error = nilAE
	RespondAppError(c, err)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INTERNAL_ERROR", body["error"]["code"])
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

	reg := RegisteredCodes()
	for code, ae := range reg {
		entry, ok := docCodes[code]
		if !assert.True(t, ok, "error code %q in registry but missing from docs/error-codes.md", code) {
			continue
		}
		assert.Equal(t, string(ae.Category), entry.category,
			"category mismatch for %q in docs/error-codes.md", code)
		assert.Equal(t, ae.HTTPStatus, entry.status,
			"HTTP status mismatch for %q in docs/error-codes.md", code)
		assert.Equal(t, ae.Message, entry.message,
			"message mismatch for %q in docs/error-codes.md", code)
	}

	assert.Equal(t, len(reg), len(docCodes),
		"docs/error-codes.md has %d codes but registry has %d", len(docCodes), len(reg))
}
