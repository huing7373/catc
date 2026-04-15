package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing7373/catc/server/pkg/jwtx"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newJWTMgr(t *testing.T) *jwtx.Manager {
	t.Helper()
	mgr, err := jwtx.New(jwtx.Config{
		AccessSecret:  "a",
		RefreshSecret: "b",
		AccessTTL:     time.Minute,
		RefreshTTL:    time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestAuth_MissingHeader401(t *testing.T) {
	r := gin.New()
	r.Use(AuthRequired(newJWTMgr(t)))
	r.GET("/p", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/p", nil))
	if w.Code != 401 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestAuth_MalformedHeader401(t *testing.T) {
	r := gin.New()
	r.Use(AuthRequired(newJWTMgr(t)))
	r.GET("/p", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Basic abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestAuth_ValidToken_SetsUserID(t *testing.T) {
	mgr := newJWTMgr(t)
	tok, err := mgr.SignAccess("u-7")
	if err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.Use(AuthRequired(mgr))
	r.GET("/p", func(c *gin.Context) {
		if uid := UserIDFrom(c); uid != "u-7" {
			t.Errorf("UserIDFrom: %q", uid)
		}
		c.Status(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestAuth_ExpiredToken401(t *testing.T) {
	mgr, err := jwtx.New(jwtx.Config{
		AccessSecret:  "a",
		RefreshSecret: "b",
		AccessTTL:     -time.Second,
		RefreshTTL:    time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := mgr.SignAccess("u")

	r := gin.New()
	r.Use(AuthRequired(mgr))
	r.GET("/p", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("status: %d", w.Code)
	}
}
