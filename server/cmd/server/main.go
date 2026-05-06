package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/huing/cat/server/internal/app/bootstrap"
	"github.com/huing/cat/server/internal/app/http/devtools"
	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/cli"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/infra/logger"
	redisinfra "github.com/huing/cat/server/internal/infra/redis"
	"github.com/huing/cat/server/internal/pkg/auth"
	"github.com/huing/cat/server/internal/repo/tx"
)

// dbOpenTimeout 是启动阶段调 db.Open 的最大等待。
//
// 为什么独立于 main 的 signal-ctx：`signal.NotifyContext` 创建的 ctx 只在收到
// SIGINT/SIGTERM 时 cancel，**没有 deadline**。如果 DSN 指向 blackholed host 或
// DNS 解析慢，PingContext 会被 driver 的 default dial timeout（典型 30s+）卡住，
// fail-fast 语义实际不快。这里包一层短 timeout，强制启动阶段 5s 内见结果。
//
// 选 5s 是平衡：
//   - 太短（< 2s）会误杀慢机 / VPN / 本地 docker mysql 启动延迟场景
//   - 太长（> 10s）违反 fail-fast 初衷（运维等不到 readiness 失败信号）
//
// 短 timeout 仅作用于 db.Open；后续 bootstrap.Run 仍用主 signal-ctx，正常 server
// lifecycle 不受影响。详见 docs/lessons/2026-04-26-startup-blocking-io-needs-deadline.md。
const dbOpenTimeout = 5 * time.Second

// redisOpenTimeout 是启动阶段调 redis.Open 的最大等待。
//
// 与 dbOpenTimeout 同模式（详见上文）：主 signal-ctx 没 deadline，碰到 blackhole
// host / 慢 DNS 时 PingContext 会被 driver 卡 30s+，fail-fast 实际不快。
// Story 10.2 引入。
const redisOpenTimeout = 5 * time.Second

func main() {
	// Bootstrap logger 先拿 "info" level 的 JSON handler（filePath 留空 → 只写 stdout），
	// 确保 config 加载失败这类**早期启动错误**也走结构化 JSON 输出。config 加载成功后会用
	// 实际配置的 level + file 再 Init 一次。
	// 见 docs/lessons/2026-04-25-slog-init-before-startup-errors.md。
	logger.Init("info", "")

	// Story 4.3 review fix：子命令路径必须**先**拆 args 再 flag.Parse。
	//
	// `flag.Parse()` 在第一个非 flag 参数后停止解析。文档化的调用形态
	// `catserver migrate up -config configs/dev.yaml` 中 args[0]="migrate" 是
	// 第一个非 flag → 默认 flag.Parse 直接停在这里，**不**会解析后面的 -config，
	// migrate 子命令会用 LocateDefault 找到的 local.yaml（错误的 DB）。
	//
	// 修法：parseTopLevelArgs 把 os.Args[1:] 拆成 (preMigrate, postMigrate)，
	// preMigrate 走 flag.NewFlagSet 解析（覆盖 `catserver -config X migrate up` 形态），
	// postMigrate 通过 cli.RunMigrate → cli.ParseMigrateArgs 用独立 NewFlagSet 解析
	// （覆盖 `catserver migrate up -config X` 形态）。两条路径都能拿到正确的 -config。
	configPath, migrateArgs, isMigrate := parseTopLevelArgs(os.Args[1:])

	// Story 4.3 review round 2 fix：migrate 子命令必须**绕过** main 的 default config load。
	//
	// 否则 `catserver migrate up -config dev.yaml`（CI/container 只 ship dev.yaml，
	// 没有 local.yaml）会在下面 LocateDefault → Load 阶段直接 os.Exit(1)，根本走不到
	// migrate 分支让 RunMigrate 消费自己的 -config。
	//
	// 修法：检测到 migrate 路径后立刻进入分支；让 RunMigrate 自己负责 config 加载
	// （cfg=nil 传入，args 里有 -config 用 -config，没有走 LocateDefault）。
	// 详见 docs/lessons/2026-04-26-cli-cancellation-and-gomigrate-gracefulstop.md。
	if isMigrate {
		// migrate 分支用**自己的** signal-ctx —— migrate up 跑多个 SQL 文件可能耗时
		// 几十秒，且现在 ctx-aware（gomigrate.GracefulStop chan）能让 SIGINT 在
		// statement 边界提前停下。详见 internal/infra/migrate/migrate.go。
		migrateCtx, migrateStop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer migrateStop()
		// preMigrate 路径上若也有 -config（`catserver -config X migrate up` 形态），
		// 仍优先用它 —— 通过 Load 拿到 cfg 传给 RunMigrate；RunMigrate 内部 -config
		// override 优先级仍比传入 cfg 高（`catserver -config A migrate up -config B`
		// 用 B；这种调用形态边缘但语义一致）。
		var preCfg *config.Config
		if configPath != "" {
			c, err := config.Load(configPath)
			if err != nil {
				slog.Error("config load failed", slog.Any("error", err), slog.String("path", configPath))
				os.Exit(1)
			}
			preCfg = c
			logger.Init(c.Log.Level, c.Log.File)
		}
		if err := cli.RunMigrate(migrateCtx, preCfg, migrateArgs); err != nil {
			slog.Error("migrate failed", slog.Any("error", err))
			os.Exit(1)
		}
		os.Exit(0)
	}

	// 非 migrate 路径：走原有 default config load 流程。
	if configPath == "" {
		p, err := config.LocateDefault()
		if err != nil {
			slog.Error("config locate failed", slog.Any("error", err))
			os.Exit(1)
		}
		configPath = p
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("config load failed", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Init(cfg.Log.Level, cfg.Log.File)
	slog.Info("config loaded",
		slog.String("path", configPath),
		slog.Int("http_port", cfg.Server.HTTPPort),
		slog.String("log_level", cfg.Log.Level),
		slog.String("log_file", cfg.Log.File),
	)

	// Dev 模式（BUILD_DEV=true 或 build tag `devtools`）启用时在启动阶段打一条
	// 醒目 WARN。放在 logger.Init(cfg.Log.Level) 之后 → 用户配置的 log level
	// 已生效；devtools.Register 内部会在路由实际注册完成后再打一条同内容 WARN，
	// 两条共同构成完整的"触发源 → 注册完成"链路，便于 ops 排障。
	if devtools.IsEnabled() {
		slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Story 4.2：MySQL 接入 + tx manager 构造。失败必须 fail-fast（os.Exit(1)）：
	//   - DSN 空（local.yaml 漏配）
	//   - DSN 解析错（驱动 parse 失败）
	//   - PingContext 失败（network unreachable / auth / MySQL 未起来）
	//
	// 不容忍降级：用空连接池继续启动会让用户在第一个业务请求时才发现问题，
	// 违反 NFR3 / 总体架构设计 §"状态以 server 为准" + MEMORY.md "No Backup Fallback"。
	//
	// **必须用本地短 timeout ctx**：主 signal-ctx 没 deadline，碰到 blackhole host /
	// 慢 DNS 时 PingContext 会被 driver 卡 30s+，fail-fast 实际不快。dbOpenTimeout 强制
	// 启动阶段 5s 内见结果；timeout 触发后 cancel 仅影响 db.Open 这一阻塞 IO，主 ctx
	// 不受影响。Story 4.2 review round 2 补漏。
	dbOpenCtx, dbOpenCancel := context.WithTimeout(ctx, dbOpenTimeout)
	gormDB, err := db.Open(dbOpenCtx, cfg.MySQL)
	dbOpenCancel()
	if err != nil {
		slog.Error("mysql open failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		sqlDB, sqlErr := gormDB.DB()
		if sqlErr != nil {
			// 几乎不可能（gormDB 已经 Open 成功，DB() 这时不应该失败）；保险起见 log
			slog.Error("mysql close: get *sql.DB failed", slog.Any("error", sqlErr))
			return
		}
		if cerr := sqlDB.Close(); cerr != nil {
			slog.Error("mysql close failed", slog.Any("error", cerr))
		}
	}()

	// Tx manager 构造。本 story 阶段 router / handler 暂不消费（Epic 4 Story 4.6
	// 才挂业务 handler）—— 这里只确保依赖能 wire 起来，不让 gormDB / txMgr 变成
	// 一个全局变量（避免 ADR-0001 §3.4 接口边界 mock 的反模式）。
	txMgr := tx.NewManager(gormDB)
	slog.Info("mysql connected", slog.Int("max_open_conns", cfg.MySQL.MaxOpenConns))

	// Story 10.2：Redis 接入。失败必须 fail-fast：
	//   - addr 空（local.yaml 漏配）→ "redis.addr is empty"
	//   - 连接失败 / authentication failed / wrong db → fmt.Errorf("redis ping: %w", err)
	//
	// 与 db.Open 同模式：本地短 timeout ctx 强制 5s 内见结果，避免 blackhole host /
	// 慢 DNS 卡 30s+。本 story 阶段 router / handler 暂不消费 RedisClient（Story 10.6
	// 才挂 presence repo）—— 这里只确保依赖能 wire 起来。
	redisOpenCtx, redisOpenCancel := context.WithTimeout(ctx, redisOpenTimeout)
	redisClient, err := redisinfra.Open(redisOpenCtx, cfg.Redis)
	redisOpenCancel()
	if err != nil {
		slog.Error("redis open failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if cerr := redisClient.Close(); cerr != nil {
			slog.Error("redis close failed", slog.Any("error", cerr))
		}
	}()
	slog.Info("redis connected",
		slog.String("addr", cfg.Redis.Addr),
		slog.Int("pool_size", cfg.Redis.PoolSize),
		slog.Int("db", cfg.Redis.DB),
	)

	// Story 10.3：WS SessionManager 构造。
	// 本 story 阶段不挂任何 lifecycle 钩子（10.6 才挂 Redis presence 钩子）。
	// SessionManager 是纯内存对象（map + sync.RWMutex），构造无 IO；不需要 timeout ctx。
	sessionMgr := wsapp.NewSessionManager()
	// **defer LIFO 顺序约束**（Story 10.4 引入；详见 heartbeat scanner 段注释）：
	// sessionMgr.Close 必须**先**注册（后执行）；scanner 的 cancel 必须**后**注册（先执行）。
	defer func() {
		if cerr := sessionMgr.Close(); cerr != nil {
			slog.Error("session manager close failed", slog.Any("error", cerr))
		}
	}()
	slog.Info("ws session manager ready")

	// Story 10.4：HeartbeatScanner 构造 + 启动后台 goroutine。
	// scanner 周期性扫描 SessionManager 内所有 Session，超时（cfg.WS.HeartbeatTimeoutSec）
	// 触发 close 4005 frame + 释放本地 Session 资源（V1 §12.1 钦定 4005 = transient
	// network failure，触发 iOS Story 12.5 自动重连指数退避路径）。
	//
	// 用 ctx + cancel 让 scanner 在 main 退出（graceful shutdown）时优雅退出；
	// 不需要 .Stop() 方法（与 Story 8.5 步数同步触发器同模式）。
	//
	// **defer LIFO 顺序**：cancelHeartbeat 在 sessionMgr.Close 之后注册 → main 退出时
	// 先执行（LIFO）→ scanner 先停 ticker / 不再 fanout 新 close → 再 sessionMgr.Close
	// 批量清理 Session。反序会让 sessionMgr.Close 跑批量 s.Close() 同时 scanner 还在
	// fanout CloseWithCode → 双方对同 Session 并发 close（虽然 Close 幂等不 panic，但
	// 日志噪声 + onUnregister 钩子可能多调）。
	heartbeatCtx, cancelHeartbeat := context.WithCancel(context.Background())
	heartbeatScanner := wsapp.NewHeartbeatScanner(sessionMgr, cfg.WS.HeartbeatTimeoutSec, slog.Default())
	go heartbeatScanner.Run(heartbeatCtx)
	defer func() {
		cancelHeartbeat()
		slog.Info("ws heartbeat scanner cancel signaled")
	}()
	slog.Info("ws heartbeat scanner ready",
		slog.Int("heartbeatTimeoutSec", cfg.WS.HeartbeatTimeoutSec),
	)

	// Story 4.4：JWT signer 构造 fail-fast。
	//
	// auth.New 校验：secret 空 / < 16 字节 / expireSec ≤ 0 / expireSec > 30 天 → 返 error。
	// 任一失败必须 os.Exit(1)：
	//   - 空 secret 让所有 token 可被任意伪造（严重安全漏洞）
	//   - secret 过短易被暴力破解
	//   - 异常 expireSec 违反 V1 §4.1 "默认过期 7 天" 契约
	//
	// 与 db.Open 同 fail-fast 模式（MEMORY.md "No Backup Fallback" 钦定反对 fallback
	// 掩盖核心架构风险；启动时尽早暴露问题，避免业务请求才发现 secret 缺失）。
	//
	// 不需要 timeout ctx：auth.New 是纯 CPU < 1µs，不阻塞 IO（与 db.Open 不同）。
	signer, err := auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
	if err != nil {
		slog.Error("auth token signer init failed", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("auth token signer ready", slog.Int64("default_expire_sec", cfg.Auth.TokenExpireSec))

	// Story 4.5：bootstrap.Run 签名收敛为 Deps struct（替代之前 5 个平铺参数）。
	// 后续 Story 4.6 / 4.8 / Epic 5+ 加共享依赖时只改 Deps 字段，不再改 Run 签名。
	//
	// **EnvName**（Story 7.3 review r6 [P2]）：从 `CAT_ENV` env 读取，默认 "prod"。
	// 用于 service.NewStepService 强制 prod 必须用默认 cap（5000/50000，跨端契约一部分）。
	// 部署侧只需在 dev / staging / test 显式 `export CAT_ENV=dev|staging|test` 即可允许 YAML 覆盖；
	// 漏配 / prod 部署直接走严格分支（safe-by-default，反向纠偏比要求"运维记得配"更可靠）。
	envName := os.Getenv("CAT_ENV")
	if envName == "" {
		envName = "prod"
	}
	deps := bootstrap.Deps{
		GormDB:       gormDB,
		TxMgr:        txMgr,
		Signer:       signer,
		RateLimitCfg: cfg.RateLimit,
		StepsCfg:     cfg.Steps,   // Story 7.3 加：步数同步防作弊阈值
		EnvName:      envName,     // Story 7.3 review r6 [P2] 加：prod cap override 强制
		RedisClient:  redisClient, // Story 10.2 加：Redis 单例 client
		SessionMgr:   sessionMgr,  // Story 10.3 加：WS Session 注册中心
		WSCfg:        cfg.WS,      // Story 10.3 加：WS 配置（心跳超时 / max message size / write timeout）
	}
	if err := bootstrap.Run(ctx, cfg, deps); err != nil {
		slog.Error("server run failed", slog.Any("error", err))
		os.Exit(1)
	}
}

// parseTopLevelArgs 把 os.Args[1:] 拆分成 (configPath, migrateArgs, isMigrate)。
//
// 支持以下两种文档化调用形态：
//
//  1. `catserver -config X` / `catserver` —— 普通 server 启动；isMigrate=false
//  2. `catserver -config X migrate <action> [-config Y]` ——
//     migrate 子命令；isMigrate=true，migrateArgs 是 "migrate" 之后的全部参数
//     （含可能再次出现的 -config，由 cli.RunMigrate 内部 NewFlagSet 解析）
//
// 实装策略：手动扫描 args 找到 "migrate" 的位置（不走 flag.Parse —— 它在第一个
// 非 flag 参数停止）。"migrate" 之前的参数走主 flag.Parse 解析 -config；之后的
// 参数原样转发给 cli.RunMigrate。
//
// 如果 "migrate" 之前 + 之后都给了 -config，**子命令的优先**（更靠近 action 的语义胜出）；
// 这与"被显式指定的更晚的值覆盖默认"的常规 CLI 直觉一致。
func parseTopLevelArgs(rawArgs []string) (configPath string, migrateArgs []string, isMigrate bool) {
	// 找 "migrate" 在 rawArgs 里的位置。它必须是一个独立的 token（不是 -migrate 之类）。
	migrateIdx := -1
	for i, a := range rawArgs {
		if a == "migrate" {
			migrateIdx = i
			break
		}
	}

	var preMigrate []string
	if migrateIdx == -1 {
		preMigrate = rawArgs
	} else {
		preMigrate = rawArgs[:migrateIdx]
		// migrateArgs 是 "migrate" **之后**的全部参数，cli.RunMigrate 自己拆 action + flags
		migrateArgs = rawArgs[migrateIdx+1:]
		isMigrate = true
	}

	// 用 ContinueOnError 避免 preMigrate 解析失败直接 os.Exit —— 主流程仍然 fail-fast，
	// 但出错路径走 main 的 slog.Error + os.Exit（统一退出）。
	fs := flag.NewFlagSet("catserver", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "path to config YAML (default: auto-detect server/configs/local.yaml or configs/local.yaml)")
	if err := fs.Parse(preMigrate); err != nil {
		slog.Error("flag parse failed", slog.Any("error", err))
		os.Exit(2)
	}
	return configPath, migrateArgs, isMigrate
}
