package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/pkg/response"
)

func PingHandler(c *gin.Context) {
	response.Success(c, map[string]any{}, "pong")
}
