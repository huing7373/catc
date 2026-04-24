package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestRequestID_GeneratesUUIDv4WhenHeaderAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	var captured string
	r.GET("/probe", func(c *gin.Context) {
		v, _ := c.Get(RequestIDKey)
		captured, _ = v.(string)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !uuidV4Pattern.MatchString(captured) {
		t.Errorf("c.Get(request_id) = %q, want UUIDv4", captured)
	}
	if got := w.Header().Get(RequestIDHeader); got != captured {
		t.Errorf("response header %s = %q, want %q", RequestIDHeader, got, captured)
	}
}

func TestRequestID_PropagatesExistingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	var captured string
	r.GET("/probe", func(c *gin.Context) {
		v, _ := c.Get(RequestIDKey)
		captured, _ = v.(string)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set(RequestIDHeader, "my-custom-rid-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if captured != "my-custom-rid-123" {
		t.Errorf("c.Get(request_id) = %q, want %q", captured, "my-custom-rid-123")
	}
	if got := w.Header().Get(RequestIDHeader); got != "my-custom-rid-123" {
		t.Errorf("response header = %q, should preserve supplied value", got)
	}
}
