package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type InfraChecker interface {
	HealthCheck(ctx context.Context) error
}

type WSHubChecker interface {
	GoroutineCount() int
}

const healthCheckTimeout = 3 * time.Second

type HealthHandler struct {
	mongo      InfraChecker
	redis      InfraChecker
	wsHub      WSHubChecker
	redisCmd   redis.Cmdable
	maxConn    int
	ready      atomic.Bool
}

func NewHealthHandler(mongo, rds InfraChecker, wsHub WSHubChecker, redisCmd redis.Cmdable, maxConn int) *HealthHandler {
	return &HealthHandler{
		mongo:    mongo,
		redis:    rds,
		wsHub:    wsHub,
		redisCmd: redisCmd,
		maxConn:  maxConn,
	}
}

func (h *HealthHandler) SetReady() {
	h.ready.Store(true)
}

func (h *HealthHandler) Healthz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), healthCheckTimeout)
	defer cancel()
	status := http.StatusOK

	var mongoErr, redisErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		mongoErr = h.mongo.HealthCheck(ctx)
	}()
	go func() {
		defer wg.Done()
		redisErr = h.redis.HealthCheck(ctx)
	}()
	wg.Wait()

	mongoStatus := "ok"
	if mongoErr != nil {
		mongoStatus = fmt.Sprintf("error: %s", mongoErr.Error())
		status = http.StatusServiceUnavailable
	}

	redisStatus := "ok"
	if redisErr != nil {
		redisStatus = fmt.Sprintf("error: %s", redisErr.Error())
		status = http.StatusServiceUnavailable
	}

	wsHubStatus := "ok"
	if count := h.wsHub.GoroutineCount(); count > h.maxConn {
		wsHubStatus = fmt.Sprintf("error: goroutine count %d exceeds max %d", count, h.maxConn)
		status = http.StatusServiceUnavailable
	}

	lastCronTick := ""
	val, err := h.redisCmd.Get(ctx, "cron:last_tick").Result()
	if err == nil {
		lastCronTick = val
	}

	overall := "ok"
	if status != http.StatusOK {
		overall = "error"
	}

	c.JSON(status, gin.H{
		"status":       overall,
		"mongo":        mongoStatus,
		"redis":        redisStatus,
		"wsHub":        wsHubStatus,
		"lastCronTick": lastCronTick,
	})
}

func (h *HealthHandler) Readyz(c *gin.Context) {
	if h.ready.Load() {
		c.JSON(http.StatusOK, gin.H{"ready": true})
		return
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"ready": false})
}
