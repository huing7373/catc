package dto

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAppError_ErrorString(t *testing.T) {
	e := &AppError{Code: "X", Message: "y"}
	if got := e.Error(); got != "X: y" {
		t.Errorf("Error(): %q", got)
	}
	var nilErr *AppError
	if got := nilErr.Error(); got != "" {
		t.Errorf("nil Error(): %q", got)
	}
}

func TestAppError_UnwrapChain(t *testing.T) {
	cause := errors.New("root cause")
	e := (&AppError{Code: "X", Message: "m"}).WithCause(cause)
	if !errors.Is(e, cause) {
		t.Error("errors.Is should find root cause through Unwrap")
	}
	var ae *AppError
	if !errors.As(e, &ae) {
		t.Error("errors.As should extract *AppError")
	}
}

func TestWithCause_DoesNotMutateOriginal(t *testing.T) {
	base := &AppError{Code: "X", Message: "m"}
	cause := errors.New("c")
	withCause := base.WithCause(cause)
	if base.Wrapped != nil {
		t.Error("base should remain untouched")
	}
	if withCause.Wrapped != cause {
		t.Error("withCause should carry cause")
	}
}

func TestRespondAppError_AppError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	RespondAppError(c, &AppError{HTTPStatus: 404, Code: "NOT_FOUND", Message: "x"})

	if w.Code != 404 {
		t.Errorf("status: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":"NOT_FOUND"`) {
		t.Errorf("body missing code: %s", body)
	}
}

func TestRespondAppError_GenericError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	RespondAppError(c, errors.New("boom"))

	if w.Code != 500 {
		t.Errorf("status: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Errorf("body missing INTERNAL_ERROR: %s", w.Body.String())
	}
}

func TestRespondSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	RespondSuccess(c, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"hello":"world"`) {
		t.Errorf("body: %s", w.Body.String())
	}
}
