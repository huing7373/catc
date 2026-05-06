package bootstrap

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/app/http/devtools"
	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/metrics"
	redisinfra "github.com/huing/cat/server/internal/infra/redis"
	"github.com/huing/cat/server/internal/pkg/auth"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
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
	GormDB       *gorm.DB               // Story 4.2 加：MySQL gorm 句柄
	TxMgr        tx.Manager             // Story 4.2 加：tx manager
	Signer       *auth.Signer           // Story 4.4 加：JWT signer 单例
	RateLimitCfg config.RateLimitConfig // Story 4.5 加：限频策略参数
	StepsCfg     config.StepsConfig     // Story 7.3 加：步数同步防作弊阈值（dev/test 覆盖；prod 默认）
	// EnvName 是部署环境名（main.go 从 CAT_ENV env var 读取，默认 "prod"）。
	// Story 7.3 review r6 [P2] 加：传给 service.NewStepService 强制 prod 必须用默认 cap。
	// 未注入 / 不识别值时 service 按 prod 严格策略（safe-by-default）。
	EnvName string
	// RedisClient 是 Story 10.2 引入的 Redis 单例 client。
	//
	// 本 story 阶段**不**有业务 handler 消费（10.6 / Epic 20 / 32 才挂 presence /
	// idempotency repo）；这里只让依赖在 main.go 透传到 Deps，让 future story 加业务
	// 路由时直接用 deps.RedisClient 而不是再扩 Deps 字段。
	//
	// 单元测试 Deps{} 零值场景下 RedisClient 是 nil；当前没有业务 handler 消费 →
	// 不需要在 NewRouter 内部加 if-guard 兜底（与 GormDB / Signer 的 if-guard 区分：
	// 那些字段已有 handler 消费，必须 nil-tolerant）。Future Story 10.6 引入 presence
	// handler 时再加 `if deps.RedisClient != nil` 前置 if-guard。
	RedisClient redisinfra.RedisClient

	// SessionMgr 是 Story 10.3 引入的 WS Session 注册中心。
	//
	// 单元测试 Deps{} 零值场景下 SessionMgr 是 nil；NewRouter 内 WS 路由挂载段
	// 用 `&& deps.SessionMgr != nil` 前置 if-guard 兜底（与 GormDB / Signer 的
	// if-guard 同模式）。生产 main.go 已 fail-fast wire（mgr 是纯内存构造，理论
	// 不会失败）。
	SessionMgr wsapp.SessionManager

	// WSCfg 是 Story 10.3 引入的 WS 配置（心跳超时 / max message size / write
	// timeout）。Deps{} 零值场景下 WSCfg 是 zero-value WSConfig；NewGateway 内部
	// 对零值字段做了兜底（writeTimeout <= 0 → 5s）；其他字段（HeartbeatTimeoutSec /
	// MaxMessageSizeBytes）仅 10.4 / Session.SetReadLimit 消费，零值即 0 = "不
	// 设置上限"，单元测试场景安全。
	WSCfg config.WSConfig
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

	// Story 7.5 加：dev 业务 handler；提前声明 nil，deps 完整时在 if 块内填充；
	// devtools.Register 留 if 块外（IsEnabled() 不依赖 deps，单测 Deps{} 场景仍要挂 ping-dev）。
	var devStepsHandler *handler.DevStepsHandler

	// Story 4.6 wire：仅当 deps 完整时挂业务路由。
	//
	// **deps 完整性 if-guard**（fail-fast 与 Deps{} 零值兼容）：
	//   - 单元测试 Deps{} 零值 → 业务路由不挂 → 测试不需要构造 mock GormDB / Signer
	//   - 生产 main.go 已 fail-fast（4.2 db.Open / 4.4 auth.New），所以 "deps 不完整"
	//     在生产是不可达分支；测试场景需要保留 zero-deps 路径
	//
	// /auth 子组：rate_limit by IP（V1 §4.1 行 218 钦定 IP 维度），**不**挂 auth 中间件
	// （登录前路径无 userID）。RateLimit 工厂在 NewRouter 调用期校验 cfg（PerKeyPerMin
	// <= 0 → panic）；与 4.5 fail-fast 模式一致。
	if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil {
		// 6 个 mysql repo（Story 7.3 加 stepSyncLogRepo）
		userRepo := mysql.NewUserRepo(deps.GormDB)
		authBindingRepo := mysql.NewAuthBindingRepo(deps.GormDB)
		petRepo := mysql.NewPetRepo(deps.GormDB)
		stepAccountRepo := mysql.NewStepAccountRepo(deps.GormDB)
		chestRepo := mysql.NewChestRepo(deps.GormDB)
		stepSyncLogRepo := mysql.NewStepSyncLogRepo(deps.GormDB)

		// auth service：5 repo + txMgr + signer
		authSvc := service.NewAuthService(
			deps.TxMgr,
			deps.Signer,
			userRepo,
			authBindingRepo,
			petRepo,
			stepAccountRepo,
			chestRepo,
		)

		// auth handler
		authHandler := handler.NewAuthHandler(authSvc)

		// Story 4.8 加：home service + handler（4 repo 串行 + chest 动态判定；
		// **不**依赖 authBindingRepo / txMgr / signer —— GET /home 全只读）。
		homeSvc := service.NewHomeService(userRepo, petRepo, stepAccountRepo, chestRepo)
		homeHandler := handler.NewHomeHandler(homeSvc)

		// Story 7.3 加：step service + handler（事务内差值入账 + 防作弊；
		// 复用 stepAccountRepo + 新 stepSyncLogRepo + txMgr）。
		stepSvc := service.NewStepService(deps.TxMgr, stepAccountRepo, stepSyncLogRepo, deps.StepsCfg, deps.EnvName)
		stepsHandler := handler.NewStepsHandler(stepSvc)

		// Story 7.5 加：dev step service + handler。即便 BUILD_DEV=false 也构造（开销可忽略），
		// 由 devtools.Register 内部 IsEnabled() 决定是否真注册路由 —— 决策集中在 devtools 包。
		// 复用 7.3 / 4.6 已 wire 的 userRepo / stepAccountRepo / stepSyncLogRepo / txMgr。
		devStepSvc := service.NewDevStepService(deps.TxMgr, userRepo, stepAccountRepo, stepSyncLogRepo)
		devStepsHandler = handler.NewDevStepsHandler(devStepSvc)

		api := r.Group("/api/v1")

		// /auth 子组：RateLimitByIP（V1 §4.1 行 218）
		authGroup := api.Group("/auth", middleware.RateLimit(deps.RateLimitCfg, middleware.RateLimitByIP))
		authGroup.POST("/guest-login", authHandler.GuestLogin)

		// 已认证子组：先 Auth 校验 Bearer token + 注 userID 到 c.Keys；
		// 再 RateLimit by userID（同 IP 多 user 互不影响 NAT 共享）。
		// future GET /me / GET /pets 等同模块路由共享本子组配置。
		authedGroup := api.Group("",
			middleware.Auth(deps.Signer),
			middleware.RateLimit(deps.RateLimitCfg, middleware.RateLimitByUserID),
		)
		authedGroup.GET("/home", homeHandler.LoadHome)
		authedGroup.POST("/steps/sync", stepsHandler.PostSync)     // Story 7.3 加
		authedGroup.GET("/steps/account", stepsHandler.GetAccount) // Story 7.4 加

		// Story 10.3 加：WS 网关路由
		// **不**挂在 /api/v1 前缀下（V1 §12.1 钦定 path 是 /ws/rooms/{roomId}）；
		// **不**挂 RateLimit / Auth 中间件（V1 §12.1 行 1275 钦定 WS 不走 HTTP rate_limit；
		// 鉴权在 Gateway.Handle 内部按 §12.1 校验顺序实装，校验失败发 close frame
		// 而非 HTTP 401）。
		// 仅当 deps.SessionMgr 非空时挂载（与 GormDB / Signer if-guard 同模式）；
		// 单元测试 Deps{} 零值场景跳过本路由，与 router_test.go 既有约定一致。
		if deps.SessionMgr != nil {
			roomMemberRepo := mysql.NewRoomMemberRepo(deps.GormDB)
			gateway := wsapp.NewGateway(
				deps.Signer,
				deps.SessionMgr,
				roomMemberRepo,
				deps.WSCfg,
				deps.EnvName, // review r2 P2 加：prod contract override 强制
			)
			r.GET("/ws/rooms/:roomId", gateway.Handle)
		}
	}

	// dev 模式下挂 /dev/* 路由组；未启用时本调用完全透明。
	// devStepsHandler 在 deps 完整时由 if 块内填充，否则保持 nil（Register 内部跳过 grant-steps 路由）。
	//
	// **关键 Go 接口 nil 陷阱**：直接传 `*handler.DevStepsHandler(nil)` 给 `devtools.DevStepsHandler`
	// interface，interface header 是 (type=*handler.DevStepsHandler, value=nil) → **非 nil interface**，
	// Register 内 `if devStepsHandler != nil` 判定会通过 → 调 PostGrantSteps 时 nil receiver panic。
	// 显式分支确保 nil 指针 → nil interface。
	if devStepsHandler == nil {
		devtools.Register(r, nil)
	} else {
		devtools.Register(r, devStepsHandler) // Story 7.5 修改：透传 dev handler
	}

	return r
}
