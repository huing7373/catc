package bootstrap

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/app/http/devtools"
	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/metrics"
	"github.com/huing/cat/server/internal/pkg/auth"
	"github.com/huing/cat/server/internal/repo/tx"
)

// Deps 是 bootstrap 期收集的依赖集合，由 main.go 构造后透传给 Run / NewRouter。
//
// 引入 Deps 是 Story 4.5 完成的"签名收敛"动作：4.4 已经把 Run 从 4 参数扩到 5
// 参数（加 signer），本 story 第二次扩（加 RateLimitCfg）时收敛为 struct，
// 避免后续 4.6 / 4.8 / Epic 5+ 每加一个依赖就改 Run / NewRouter 签名。
//
// 字段 nil-tolerant：单元测试可传 Deps{} 零值（仅依赖 router 基础四件套 +
// /ping / /version / /metrics 不消费 deps）。生产路径 main.go 必填。
type Deps struct {
	GormDB       *gorm.DB           // Story 4.2 加：MySQL gorm 句柄
	TxMgr        tx.Manager         // Story 4.2 加：tx manager
	Signer       *auth.Signer       // Story 4.4 加：JWT signer 单例
	RateLimitCfg config.RateLimitConfig // Story 4.5 加：限频策略参数
}

// NewRouter 构造挂好四件套中间件的 Gin engine。
//
// 中间件顺序严格：
//
//	RequestID → Logging → ErrorMappingMiddleware → Recovery → handler
//
// 顺序约束：
//   - RequestID 最外：所有后续中间件 / handler 都需要 request_id 进 ctx
//   - Logging 次外：在 c.Next() 之前打开 logger 进 ctx，之后扫 c.Errors 写
//     http_request 日志（含 error_code）
//   - ErrorMappingMiddleware 在 Recovery **外层**：panic 流程是 "handler panic →
//     Recovery defer recover → c.Error(AppError) + c.Abort → Recovery 返回 →
//     ErrorMappingMiddleware 的 after-c.Next() 写 envelope"。把 ErrorMappingMiddleware
//     放 Recovery 内层会让 envelope 写不出来（panic 已 unwind 越过它）。
//   - Recovery 最内：直接包裹 handler 抓 panic
//
// 详情见 middleware/error_mapping.go + middleware/recover.go 顶部注释。
//
// 运维端点（不走 /api/v1 前缀，**不**挂 auth / rate_limit）：
//   - GET /ping      — liveness 探活
//   - GET /version   — 构建信息（commit / builtAt，编译期 ldflags 注入）
//   - GET /metrics   — Prometheus scrape 端点
//   - /dev/*         — 开发工具路由组，仅在 BUILD_DEV=true 或 -tags devtools 时注册
//     （示例端点 /dev/ping-dev；业务 dev 端点由 Epic 7 / Epic 20 扩展）
//
// **关键反模式**：health check / metrics scrape 自爆。给 /ping / /version /
// /metrics 挂 rate_limit / auth 在告警风暴 / 运维巡检时会误伤；这两条路由
// **永远**保持公开 + 高频可用。
//
// # 安全：SetTrustedProxies(nil)
//
// 节点 2 阶段 server 直接对外（无反代），调 r.SetTrustedProxies(nil) 让
// c.ClientIP() **不**信任任何 X-Forwarded-For / X-Real-IP header —— 直接用
// Request.RemoteAddr 当客户端 IP。
//
// 不调这一行的危险：Gin 默认 trustedProxies 是 "0.0.0.0/0"（信任**所有**反代
// IP），任何客户端发 `X-Forwarded-For: 1.2.3.4` 都会被采信 → 限频 / 审计日志
// 全部基于伪造 IP。攻击者循环伪造 XFF 可绕过 60/min 限制（rate_limit.go
// RateLimitByIP 已切到 c.RemoteIP() 兜底，但 logging.go / devtools.go 的
// audit 字段仍依赖 ClientIP，需要 SetTrustedProxies 在 engine 层一刀切）。
//
// 节点 36 部署 epic 上反代时改成显式白名单（CDN / nginx 出口 IP 段），不再是
// nil。本调用是 **安全默认值**，不是 future TODO。
//
// # 业务路由组（Story 4.5 引入；4.6 / 4.8 落地具体 handler）
//
// 4.6 / 4.8 落地后预期形态（本 story 只产出中间件 + Deps 透传，不挂业务 handler）：
//
//	api := r.Group("/api/v1")
//
//	// /auth 子组：rate_limit by IP（V1 §4.1 钦定），**不**挂 auth
//	authGroup := api.Group("/auth", middleware.RateLimit(deps.RateLimitCfg, middleware.RateLimitByIP))
//	// authGroup.POST("/guest-login", authHandler.GuestLogin)  // 4.6 加
//
//	// 已认证子组：先 auth 再 rate_limit by userID
//	authedGroup := api.Group("",
//	    middleware.Auth(deps.Signer),
//	    middleware.RateLimit(deps.RateLimitCfg, middleware.RateLimitByUserID),
//	)
//	// authedGroup.GET("/home", homeHandler)     // 4.8 加
//	// authedGroup.GET("/me", meHandler)         // 4.6 加
//
// TODO(Epic-36): /metrics 上线前需要加 auth / 独立端口。
func NewRouter(deps Deps) *gin.Engine {
	r := gin.New()
	// 安全默认：不信任任何代理 → c.ClientIP() 不解析 X-Forwarded-For。
	// 详见函数顶部 "# 安全：SetTrustedProxies(nil)" 一节。
	// 错误返回（仅在传入 IP CIDR 时可能 parse 失败）传 nil 时不会失败，故
	// 此处忽略；future 改成显式 CIDR 白名单时需要 wrap fail-fast。
	_ = r.SetTrustedProxies(nil)
	r.Use(middleware.RequestID())
	r.Use(middleware.Logging())
	r.Use(middleware.ErrorMappingMiddleware())
	r.Use(middleware.Recovery())

	r.GET("/ping", handler.PingHandler)
	r.GET("/version", handler.VersionHandler)
	r.GET("/metrics", gin.WrapH(metrics.Handler()))

	// dev 模式下挂 /dev/* 路由组；未启用时本调用完全透明。
	devtools.Register(r)

	// Story 4.5：deps 透传到 router 工厂，但本 story **不**挂任何 /api/v1
	// 业务 handler；4.6 / 4.8 落地时往该 group 填 handler。
	//
	// 参数现在通过 Deps 显式承接，避免未读触发 vet "declared and not used"。
	_ = deps

	return r
}
