package bootstrap

import (
	stderrors "errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/app/http/devtools"
	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/metrics"
	redisinfra "github.com/huing/cat/server/internal/infra/redis"
	"github.com/huing/cat/server/internal/pkg/auth"
	repomysql "github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// MySQL server 错误号常量。go-sql-driver/mysql 把 server 端错误号原样填到
// *mysql.MySQLError.Number；与 repo/mysql/auth_binding_repo.go 用 1062 / user_repo.go
// 用 1062 同模式，只比 Number 不解析 Message 字符串（locale / 版本不稳）。
//
// 这三个号在 wsTablesReady 中被视为 **misconfig fail-fast 信号**（不是 transient）：
//   - 1146 ER_NO_SUCH_TABLE：表本身不存在 → schema 漂移
//   - 1142 ER_TABLEACCESS_DENIED_ERROR：DB role 连得上但没有该表的 SELECT 权限
//   - 1044 ER_DBACCESS_DENIED_ERROR：DB role 没有该 schema 的访问权限
//
// **r8 [P1] 修订**（与 r6 后语义相比的收窄）：
// r6 把"非 1146 错误"全部归为"transient / 非致命"warn + continue。但 r8 反指：当 probe
// 改为直接打 app table 后，1142 / 1044 已经不是 information_schema sniff 的"假阳性
// 权限拒绝"，而是**真正的 misconfig** —— DB role 连得上但 SELECT rooms/room_members
// 都被拒，每次 WS 握手都会以 close 1011 失败，feature 完全不可用，但 healthcheck
// 看着健康（静默灾难）。这种场景应该启动期立即 fail-fast，让 systemd / k8s
// CrashLoopBackOff 告警，而不是 warn-and-continue。
//
// 1040 (ER_CON_COUNT_ERROR, too many connections) / 网络抖动 / context 取消等才是
// 真正的 transient infrastructure 问题，那些保持 warn + continue。
const (
	mysqlErrCodeNoSuchTable       = 1146
	mysqlErrCodeTableAccessDenied = 1142
	mysqlErrCodeDBAccessDenied    = 1044
)

// wsTablesReady 探测 rooms / room_members 两张表是否在当前 schema 下可用。
//
// **历史**（Story 10.3 多轮 review）：
//   - r3 [P1] 引入：当时 rooms / room_members migration 还没落地（钦定 Epic 11.2
//     接管），prod 启动会在 RoomMemberRepo 阶段 SQL 报 "table doesn't exist" →
//     Gateway close 1011；初版用 warn-and-skip（表缺 → 跳过路由挂载）兜底
//   - r5 [P1] 改写：warn-and-skip 让 prod 永远拿到 HTTP 404 而非 documented
//     WS close codes，Story 10.3 在正常 server startup 完全不可用 →
//     **真正的修法**：把 rooms / room_members 的 CREATE TABLE 拆出 Epic 11.2
//     提前到 Story 10.3（migrations 0007 / 0008 已 ship），让 Story 10.3
//     self-contained 可部署；JOIN / LEAVE 等业务事务仍由 Epic 11.4 / 11.5 接管。
//     r5 实装用 `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema =
//     DATABASE() AND table_name IN ('rooms','room_members')` 单 query sniff，
//     缺表 → panic（fail-fast）。
//   - r6 [P2] 改写：r5 的 information_schema query 在 prod hardened DB user（最小
//     权限原则：只授权 app schema，**不**授权 information_schema）下会失败 →
//     wsTablesReady panic → 整个 HTTP server 启动失败（不只是 /ws，所有 HTTP
//     端点 /ping / /version / 业务 API 也挂）。改用**直接 probe 表**的方案：
//     `SELECT 1 FROM rooms LIMIT 1` / `SELECT 1 FROM room_members LIMIT 1`
//     走和 RoomExists 同一条 app schema 权限路径，不引入额外 privilege 要求。
//
// **r8 后语义**（与 r6 行为相比的收窄）：
//
// 错误分流（基于 MySQL error number，而非字符串匹配）：
//   - err == nil（query 成功，包括空表 / 有数据两种结果）→ 表存在 → continue
//   - err 是 ER_NO_SUCH_TABLE (1146) → **schema 漂移** → 返 false →
//     调用方 panic（fail-fast）
//   - err 是 ER_TABLEACCESS_DENIED_ERROR (1142) → **app-table SELECT 权限缺失**
//     misconfig → 返 false → 调用方 panic（fail-fast）。r8 [P1] 新增。
//   - err 是 ER_DBACCESS_DENIED_ERROR (1044) → **schema-level access 缺失**
//     misconfig → 返 false → 调用方 panic（fail-fast）。r8 [P1] 新增。
//   - 其他 err（连接断 / too-many-connections 1040 / context 取消 / 非 mysql
//     driver-level 错误 / 等）→ log warn → continue（视为表存在但当前访问失败）。
//     这些是 transient infrastructure 问题，不是 misconfig；让后续 RoomExists
//     调用时再失败 → client 拿到 documented WS close 1011 → fail-fast 仍成立，
//     只是从启动期推迟到 first request
//
// 为什么 r8 把 1142 / 1044 收窄成 fail-fast（推翻 r6 的"非 1146 一律 warn"）：
// r6 的原理由是"information_schema sniff 在 hardened DB user 下假阳性 panic"。但 r6
// 已经把 probe 改成直接打 app table（rooms / room_members），不再走 information_schema。
// 这种 probe 路径下 1142 / 1044 不是"information_schema 权限副作用"，而是**真正的**
// app-table 权限缺失 —— 部署到这种环境，每次 WS 握手都会在 RoomExists / IsUserInRoom
// 处以 close 1011 失败，feature 完全不可用 + healthcheck 看着健康 = 静默灾难。
// 这种场景应该启动期立即 fail-fast。
//
// 为什么仍保留本 helper（r6 后没有"unused code"）：
//   - 防御部署事故：如果某环境因为 migration 工具配置错 / 手动运维误操作让
//     0007 / 0008 没跑（schema 与代码版本不一致），fail-fast 让运维**立即**看到
//     启动失败，而不是 server 起来后客户端在 WS 握手时拿到 close 1011 / 404
//     才间接发现 schema 漂移
//   - 与 prod / dev / staging 一致：所有环境都期望 rooms / room_members 存在
//     （migrations 0007 / 0008 已是默认部署集合的一部分）；任何环境表缺都是
//     部署 bug，应该 fail-fast 而非降级
//   - 集成测试 fixture（startMySQLWithRoomMemberFixture）在容器内 CREATE TABLE
//     仍然让 wsTablesReady 通过，集成路径不受影响
//
// **不**用 `SHOW TABLES LIKE`：GORM .Count 翻译为子查询，元数据 query 类型不兼容；
// 而且 SHOW TABLES 在某些 hardened 环境下也可能受限。直 probe 表是最稳的方案。
//
// **db nil 安全**：调用方在 `deps.GormDB != nil` 分支内才调本 helper；
// 单元测试 Deps{} 零值场景由外层 if-guard 拦截，本函数不重复 nil-check。
//
// **返回值契约**：true = 表可用（或访问失败但属于 transient 类，留给后续 request 处理）；
// false = misconfig（缺表 1146 / 表权限 1142 / schema 权限 1044）→ 调用方应 panic 终止启动。
func wsTablesReady(db *gorm.DB) bool {
	for _, table := range []string{"rooms", "room_members"} {
		var dummy int
		// SELECT 1 FROM <table> LIMIT 1：query 走和 RoomExists / 业务事务相同的
		// app schema 权限路径，不需要 information_schema 权限。空表也返 nil error
		// （只是 row 集合为空），故 err == nil 即视为表可访问。
		err := db.Raw(fmt.Sprintf("SELECT 1 FROM %s LIMIT 1", table)).Scan(&dummy).Error
		if err == nil {
			continue
		}
		// MySQL server 错误分流：misconfig 类（缺表 / 权限缺失）→ fail-fast。
		var mysqlErr *mysql.MySQLError
		if stderrors.As(err, &mysqlErr) {
			switch mysqlErr.Number {
			case mysqlErrCodeNoSuchTable:
				slog.Error(
					"ws backing table missing: schema drift detected (MySQL 1146)",
					slog.String("table", table),
					slog.Any("error", err),
				)
				return false
			case mysqlErrCodeTableAccessDenied:
				slog.Error(
					"ws backing table SELECT denied: app role lacks privilege (MySQL 1142) — startup fail-fast to surface misconfig",
					slog.String("table", table),
					slog.Any("error", err),
				)
				return false
			case mysqlErrCodeDBAccessDenied:
				slog.Error(
					"ws backing table DB-level access denied: app role lacks schema privilege (MySQL 1044) — startup fail-fast to surface misconfig",
					slog.String("table", table),
					slog.Any("error", err),
				)
				return false
			}
		}
		// 其他错误（连接失败 / 1040 too-many-connections / context 取消 / 非 mysql
		// driver-level error / 等）→ 不阻断启动。视为"表存在但当前 probe 访问失败
		// 的 transient 状况"，让后续 request 阶段 RoomExists 自然 fail 走 documented
		// close 1011。这些不是 misconfig，启动期 fail-fast 反而会把 transient
		// infrastructure flap 升级成 CrashLoopBackOff。
		slog.Warn(
			"ws backing table probe non-fatal error (treating as table exists; will defer to request-time RoomExists)",
			slog.String("table", table),
			slog.Any("error", err),
		)
	}
	return true
}

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
		// 8 个 mysql repo（Story 7.3 加 stepSyncLogRepo；Story 11.3 加 roomRepo +
		// 把 roomMemberRepo 实例上移让 ws / room 路由块复用同一实例）
		userRepo := repomysql.NewUserRepo(deps.GormDB)
		authBindingRepo := repomysql.NewAuthBindingRepo(deps.GormDB)
		petRepo := repomysql.NewPetRepo(deps.GormDB)
		stepAccountRepo := repomysql.NewStepAccountRepo(deps.GormDB)
		chestRepo := repomysql.NewChestRepo(deps.GormDB)
		stepSyncLogRepo := repomysql.NewStepSyncLogRepo(deps.GormDB)
		// Story 11.3 加：roomRepo + 把 roomMemberRepo 实例上移让 ws / room 路由块共享同一
		// 实例（review r4 同源风险 —— 避免双实例引入隐性 race / 测试 mock 不一致）。
		// 注意：ws 路由挂载段（下面 if deps.SessionMgr != nil 块）原先在内部再调
		// `repomysql.NewRoomMemberRepo(deps.GormDB)` 构造一份新的；本 story 移除该重复
		// 构造，gateway / SnapshotBuilder 复用本处实例。
		roomRepo := repomysql.NewRoomRepo(deps.GormDB)
		roomMemberRepo := repomysql.NewRoomMemberRepo(deps.GormDB)

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

		// Story 11.3 加：room service + handler（4 步事务：FindByID 预检 → INSERT rooms →
		// INSERT room_members → UPDATE users.current_room_id）。复用上面构造的 roomRepo /
		// roomMemberRepo / userRepo / txMgr。
		//
		// 防御性 schema sniff：rooms / room_members 必须存在；MySQL 1146（真的缺表） →
		// 返 false → panic 终止启动；其他错误（权限 / 连接）由 wsTablesReady 内部
		// log warn 后视为表存在，让后续 request 阶段自然处理。
		//
		// **Story 11.3 review r1 [P2] 修**：本 probe 原先只在下方 `deps.SessionMgr != nil`
		// 块（WS 路由挂载段）内执行；但 11.3 把 `POST /api/v1/rooms` HTTP handler
		// 挂载提前到了不依赖 SessionMgr 的位置（GormDB / TxMgr / Signer 都有就挂），
		// 导致 HTTP-only wiring（或 test fixture）下 SessionMgr=nil 时缺 0007/0008
		// migration 在启动时不被捕获，第一个 POST /api/v1/rooms 请求才会拿 generic 1009
		// 而非启动期清晰报错。把 probe 提到 GormDB 块顶部（与 handler 挂载条件对齐）：
		// 任何持有 GormDB 的部署形态（HTTP-only / HTTP+WS）都跑同一份 fail-fast 检查。
		if !wsTablesReady(deps.GormDB) {
			panic("ws backing tables missing: rooms / room_members must exist (run migrations 0007 / 0008)")
		}
		roomSvc := service.NewRoomService(deps.TxMgr, userRepo, roomRepo, roomMemberRepo)
		roomHandler := handler.NewRoomHandler(roomSvc)

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
		authedGroup.POST("/rooms", roomHandler.CreateRoom)         // Story 11.3 加

		// Story 10.3 加：WS 网关路由
		// **不**挂在 /api/v1 前缀下（V1 §12.1 钦定 path 是 /ws/rooms/{roomId}）；
		// **不**挂 RateLimit / Auth 中间件（V1 §12.1 行 1275 钦定 WS 不走 HTTP rate_limit；
		// 鉴权在 Gateway.Handle 内部按 §12.1 校验顺序实装，校验失败发 close frame
		// 而非 HTTP 401）。
		// 仅当 deps.SessionMgr 非空时挂载（与 GormDB / Signer if-guard 同模式）；
		// 单元测试 Deps{} 零值场景跳过本路由，与 router_test.go 既有约定一致。
		//
		// **review r5 [P1]** 后的形态：
		//   - migrations 0007 / 0008 已 ship rooms / room_members CREATE TABLE
		//     （拆自原 Epic 11.2，让 Story 10.3 self-contained 可部署）
		//   - 路由**总是**挂载（移除 r3 的 warn-and-skip gate；prod 不再静默 404）
		//   - wsTablesReady() 保留为**防御性早期检测**：表缺 → fail-fast panic
		//     （让 systemd / k8s CrashLoopBackOff 触发运维告警，而不是 server 起
		//     来后客户端拿 WS close 1011 才间接发现 schema 漂移）
		//   - **Story 11.3 review r1 [P2]** 后：probe 已上移到本 GormDB 块顶部（room
		//     handler 挂载之前），HTTP-only / HTTP+WS 两种 wiring 共用同一份 fail-fast
		//     检查；本块内不再重复 probe（redundant 且会让 sqlmock 测试期望对不上）。
		if deps.SessionMgr != nil {
			// Story 11.3 修：roomMemberRepo 实例上移到 if deps.GormDB ... 块的开头
			// （与其他 repo 平级），ws / room 两路由块共享同一个实例；避免双实例
			// 引入隐性 race / 测试 mock 不一致（review r4 同源风险）。
			// Story 10.7 加：构造 SnapshotBuilder（节点 4 阶段 placeholder 实装；
			// Story 11.7 真实实装时把本行替换为 NewRealSnapshotBuilder，gateway
			// 层不感知）。
			snapshotBuilder := wsapp.NewPlaceholderSnapshotBuilder(roomMemberRepo)
			gateway := wsapp.NewGateway(
				deps.Signer,
				deps.SessionMgr,
				roomMemberRepo,
				deps.WSCfg,
				deps.EnvName,    // review r2 P2 加：prod contract override 强制
				snapshotBuilder, // Story 10.7 加：snapshot 构造路径
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
