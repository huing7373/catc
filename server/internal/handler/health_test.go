package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func runHealth(t *testing.T, mongoErr, redisErr error) (int, map[string]any) {
	t.Helper()
	h := NewHealthHandler(
		func(ctx context.Context) error { return mongoErr },
		func(ctx context.Context) error { return redisErr },
	)
	r := gin.New()
	r.GET("/health", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	return w.Code, body
}

func TestHealth_AllOk(t *testing.T) {
	code, body := runHealth(t, nil, nil)
	if code != 200 {
		t.Errorf("status: %d", code)
	}
	if body["mongo"] != "ok" || body["redis"] != "ok" {
		t.Errorf("body: %+v", body)
	}
}

func TestHealth_MongoDownStillReturns200(t *testing.T) {
	code, body := runHealth(t, errors.New("mongo down"), nil)
	if code != 200 {
		t.Errorf("status must always be 200, got %d", code)
	}
	if body["mongo"] != "down" {
		t.Errorf("mongo status: %v", body["mongo"])
	}
	if body["redis"] != "ok" {
		t.Errorf("redis status: %v", body["redis"])
	}
}

func TestHealth_RedisDownStillReturns200(t *testing.T) {
	code, body := runHealth(t, nil, errors.New("redis down"))
	if code != 200 {
		t.Errorf("status: %d", code)
	}
	if body["redis"] != "down" {
		t.Errorf("redis status: %v", body["redis"])
	}
}

func TestHealth_BothDownStillReturns200(t *testing.T) {
	code, body := runHealth(t, errors.New("m"), errors.New("r"))
	if code != 200 {
		t.Errorf("status: %d", code)
	}
	if body["mongo"] != "down" || body["redis"] != "down" {
		t.Errorf("body: %+v", body)
	}
	if _, ok := body["goroutine"]; !ok {
		t.Errorf("goroutine field missing: %+v", body)
	}
	if _, ok := body["uptime_sec"]; !ok {
		t.Errorf("uptime_sec field missing: %+v", body)
	}
}
