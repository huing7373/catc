package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRequestLogger_EmitsStructuredFields(t *testing.T) {
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)

	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/x", func(c *gin.Context) {
		c.Set(CtxKeyUserID, "user-42")
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	out := buf.String()
	for _, want := range []string{
		`"request_id":"`,
		`"endpoint":"GET /x"`,
		`"status_code":200`,
		`"duration_ms":`,
		`"user_id":"user-42"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q; got: %s", want, out)
		}
	}

	if got := w.Header().Get("X-Request-ID"); got == "" {
		t.Error("X-Request-ID header missing in response")
	}
}

func TestRequestLogger_NoUserIDWhenAnonymous(t *testing.T) {
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)

	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/a", func(c *gin.Context) { c.Status(204) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/a", nil))

	if strings.Contains(buf.String(), `"user_id":`) {
		t.Errorf("anonymous request must not log user_id; got: %s", buf.String())
	}
}
