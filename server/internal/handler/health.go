// Package handler contains Gin HTTP handlers. Handlers translate HTTP
// to/from service calls; they never hold database clients directly,
// except for the health handler which is intentionally wired to the
// connection primitives.
package handler

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthChecker is a minimal probe: returns nil if the dependency is
// healthy, non-nil otherwise. Tests and wiring both use this interface.
type HealthChecker func(ctx context.Context) error

// HealthHandler exposes GET /health.
type HealthHandler struct {
	checkMongo HealthChecker
	checkRedis HealthChecker
	startedAt  time.Time
}

// NewHealthHandler wires the handler to dependency probes. Both probes
// must be non-nil.
func NewHealthHandler(checkMongo, checkRedis HealthChecker) *HealthHandler {
	return &HealthHandler{
		checkMongo: checkMongo,
		checkRedis: checkRedis,
		startedAt:  time.Now().UTC(),
	}
}

// healthBody is the response payload. Fields are snake_case per the API
// convention.
type healthBody struct {
	Mongo     string `json:"mongo"`
	Redis     string `json:"redis"`
	Goroutine int    `json:"goroutine"`
	UptimeSec int64  `json:"uptime_sec"`
}

// Get always returns HTTP 200 — dependency status is expressed in the
// JSON body, never in the status code, so monitoring tools can scrape
// details without treating every Mongo hiccup as a server error.
func (h *HealthHandler) Get(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	body := healthBody{
		Mongo:     statusOf(h.checkMongo(ctx)),
		Redis:     statusOf(h.checkRedis(ctx)),
		Goroutine: runtime.NumGoroutine(),
		UptimeSec: int64(time.Since(h.startedAt).Seconds()),
	}
	c.JSON(http.StatusOK, body)
}

func statusOf(err error) string {
	if err == nil {
		return "ok"
	}
	return "down"
}
