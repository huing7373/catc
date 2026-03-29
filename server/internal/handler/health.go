package handler

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	catredis "github.com/huing7373/catc/server/pkg/redis"
	"gorm.io/gorm"
)

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	db        *gorm.DB
	redis     *catredis.Client
	startTime time.Time
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *gorm.DB, redis *catredis.Client) *HealthHandler {
	return &HealthHandler{
		db:        db,
		redis:     redis,
		startTime: time.Now(),
	}
}

// HealthResponse is the response body for GET /health.
type HealthResponse struct {
	Status     string `json:"status"`
	Postgres   string `json:"postgres"`
	Redis      string `json:"redis"`
	Goroutines int    `json:"goroutines"`
	UptimeSec  int64  `json:"uptime_sec"`
}

// Health returns the server health status including DB/Redis connectivity.
func (h *HealthHandler) Health(c *gin.Context) {
	resp := HealthResponse{
		Status:     "ok",
		Postgres:   "up",
		Redis:      "up",
		Goroutines: runtime.NumGoroutine(),
		UptimeSec:  int64(time.Since(h.startTime).Seconds()),
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// Check PostgreSQL
	sqlDB, err := h.db.DB()
	if err != nil {
		resp.Postgres = "down"
		resp.Status = "degraded"
	} else if err := sqlDB.PingContext(ctx); err != nil {
		resp.Postgres = "down"
		resp.Status = "degraded"
	}

	// Check Redis
	if err := h.redis.Ping(ctx); err != nil {
		resp.Redis = "down"
		resp.Status = "degraded"
	}

	// AC #2: GET /health always returns 200 + JSON with component status
	c.JSON(http.StatusOK, resp)
}
