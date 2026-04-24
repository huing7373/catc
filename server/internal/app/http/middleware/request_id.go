package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDKey 是 request_id 在 gin.Context / context.Value 里的 key（小写下划线，
// 与结构化日志字段名一致，便于 grep / 断言）。
const RequestIDKey = "request_id"

// RequestIDHeader 是请求 / 响应里承载 request_id 的 HTTP header。
const RequestIDHeader = "X-Request-Id"

// RequestID 中间件：为每个请求附加 request_id。
//
// 优先读取请求 header `X-Request-Id`（跨服务链路追踪时上游已带）；
// 若 header 为空，生成 UUID v4（google/uuid）。
//
// 存入两个位置：
//   1. `c.Set(RequestIDKey, rid)`  — 下游中间件 / handler 读
//   2. `c.Header(RequestIDHeader, rid)` — 响应回给 client，便于排障
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDHeader)
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(RequestIDKey, rid)
		c.Header(RequestIDHeader, rid)
		c.Next()
	}
}
