package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/pkg/auth"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// UserIDKey 是 Auth 中间件把验证通过的 userID（uint64）写到 gin.Context 的 key。
//
// 下游 handler 取法：
//
//	uid, ok := c.Get(middleware.UserIDKey)
//	if !ok { /* 不可能：auth 通过必有 uid；防御性写 1009 */ }
//	userID := uid.(uint64)
//
// 选 uint64 而非 string 的理由：
//   - 数据库设计 §3.1 钦定主键 BIGINT UNSIGNED → Go 端 uint64
//   - Story 4.4 token util 已经返回 Claims.UserID (uint64)；本 key 直接传递，
//     避免下游 handler 反复做 strconv.ParseUint
//   - 与 V1 §2.5 "JSON 端 string / Go 内部 uint64" 划分一致
//
// 注意：本 key 仅在通过 Auth 中间件的请求中存在；handler 不能假设非 auth 路由
// （如 /auth/guest-login）c.Get(UserIDKey) 一定能拿到。
//
// 命名风格说明：camelCase 与 V1 §2.5 / Go 字段命名惯例对齐；**不**与
// RequestIDKey (`request_id`) / ResponseErrorCodeKey (`response_error_code`)
// 同样下划线，因为 RequestIDKey 来自 V1 §2.4 envelope 字段（要写进 JSON），
// 而 UserIDKey 是 server 内部 ctx key（不出 JSON）。两者命名风格不一致是**故意**
// （标识"哪些是契约值 / 哪些是内部值"）。
const UserIDKey = "userID"

// Auth 中间件：从 Authorization: Bearer <token> 解析 token →
// 调用 *auth.Signer.Verify → 成功后把 Claims.UserID 写入 c.Set(UserIDKey, uid)；
// 失败 c.Error + c.Abort（让 ErrorMappingMiddleware 写 1001 envelope）。
//
// # 挂载位置
//
// 业务路由组层级（**不**全局；运维端点 /ping / /version / /metrics 不能挂）：
//
//	api := r.Group("/api/v1")
//	authedGroup := api.Group("", middleware.Auth(deps.Signer))
//	authedGroup.GET("/me", meHandler)
//
// # 错误映射
//
// header 缺失 / scheme 不是 Bearer / token 为空 / token 无效 / token 过期 →
// 通过 c.Error 推 AppError(1001)；ErrorMappingMiddleware 在 c.Next() 之后扫到
// → 写 envelope（V1 §2.4 钦定的统一结构）。
//
// 本中间件**不**直接调 response.Error（避免绕过 ADR-0006 钦定的"单一 envelope
// 生产者"，详见 docs/lessons/2026-04-24-error-envelope-single-producer.md）。
//
// # 错误细分
//
// 本中间件只产出统一的 1001 envelope（V1 §3 钦定），但 cause 链上保留
// auth.ErrTokenExpired / auth.ErrTokenInvalid sentinel，让下游 logger 用
// errors.Is 区分日志级别（过期 INFO 是正常重新登录路径；篡改 / 格式错 WARN
// 是潜在攻击）。
//
// # ctx 传播
//
// 本中间件**不**做 `c.Request = c.Request.WithContext(ctxWithUserID)` 二次包装
// （ADR-0007 §6 反对"用 ctx.Value 传业务字段"）；c.Set(UserIDKey, ...) 是
// gin.Context Keys，下游 handler / service 通过显式参数传 userID。
//
// # 性能
//
// header 解析 + Verify 是同步纯 CPU < 1µs，不需要 select ctx.Done()；
// ctx cancel 时下游 handler 不会被本中间件阻塞。
func Auth(signer *auth.Signer) gin.HandlerFunc {
	const bearerPrefix = "Bearer "
	return func(c *gin.Context) {
		rawHeader := c.GetHeader("Authorization")
		if rawHeader == "" {
			_ = c.Error(apperror.New(apperror.ErrUnauthorized, apperror.DefaultMessages[apperror.ErrUnauthorized]))
			c.Abort()
			return
		}

		// 解析 "Bearer <token>" 形态。RFC 6750 §2.1 钦定 scheme 大小写不敏感
		// （建议大写 Bearer），token 部分原样保留。
		// 严格 scheme 校验：拒绝 "Basic" / 无前缀 / 长度不足 prefix 的请求。
		if len(rawHeader) <= len(bearerPrefix) || !strings.EqualFold(rawHeader[:len(bearerPrefix)], bearerPrefix) {
			_ = c.Error(apperror.Wrap(
				fmt.Errorf("auth: invalid Authorization scheme"),
				apperror.ErrUnauthorized,
				apperror.DefaultMessages[apperror.ErrUnauthorized],
			))
			c.Abort()
			return
		}
		tokenStr := rawHeader[len(bearerPrefix):]
		// 防御 leading whitespace（"Bearer  abc" 多空格）/ trailing whitespace
		// （client 自己拼错），整段 trim：
		tokenStr = strings.TrimSpace(tokenStr)
		if tokenStr == "" {
			_ = c.Error(apperror.New(apperror.ErrUnauthorized, apperror.DefaultMessages[apperror.ErrUnauthorized]))
			c.Abort()
			return
		}

		claims, err := signer.Verify(tokenStr)
		if err != nil {
			// err 链上保留 auth.ErrTokenInvalid / auth.ErrTokenExpired sentinel，
			// 让下游 logger / future ErrorMappingMiddleware 用 errors.Is 区分级别。
			_ = c.Error(apperror.Wrap(err, apperror.ErrUnauthorized, apperror.DefaultMessages[apperror.ErrUnauthorized]))
			c.Abort()
			return
		}

		c.Set(UserIDKey, claims.UserID)
		c.Next()
	}
}
