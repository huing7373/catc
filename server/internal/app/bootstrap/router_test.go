package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRouter_Ping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var env struct {
		Code      int            `json:"code"`
		Message   string         `json:"message"`
		Data      map[string]any `json:"data"`
		RequestID string         `json:"requestId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON body: %v; body=%s", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("code = %d, want 0", env.Code)
	}
	if env.Message != "pong" {
		t.Errorf("message = %q, want %q", env.Message, "pong")
	}
	if env.Data == nil {
		t.Errorf("data should be non-nil empty object, got nil (JSON null)")
	}
	if len(env.Data) != 0 {
		t.Errorf("data = %v, want empty object", env.Data)
	}
	if env.RequestID == "" {
		t.Errorf("requestId is empty, want non-empty fallback")
	}
}
