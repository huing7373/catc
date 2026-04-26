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
	"github.com/huing/cat/server/internal/cli"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/infra/logger"
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

func main() {
	// Bootstrap logger 先拿 "info" level 的 JSON handler，确保 config 加载失败这类
	// **早期启动错误**也走结构化 JSON 输出。config 加载成功后会用实际配置的 level 再 Init 一次。
	// 见 docs/lessons/2026-04-25-slog-init-before-startup-errors.md。
	logger.Init("info")

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

	logger.Init(cfg.Log.Level)
	slog.Info("config loaded",
		slog.String("path", configPath),
		slog.Int("http_port", cfg.Server.HTTPPort),
		slog.String("log_level", cfg.Log.Level),
	)

	// Story 4.3：migrate 子命令分支。
	// `catserver migrate {up|down|status}` 走这条路径，之后 os.Exit 立刻退出，
	// **不**进入 server 启动路径（不调 db.Open / bootstrap.Run）。
	//
	// 为什么必须在 db.Open **之前**：schema 不存在时 db.Open 的 PingContext 会失败
	// （表不存在不是 ping 失败，但首次部署 / 全新 DB 场景仍可能遇到 schema 校验路径
	// 失败）；更重要的是 migrate 工具自己用独立 driver 上 advisory lock，与 4.2 的
	// gorm.DB 解耦，没有理由强制先 Open。
	//
	// 子命令分支用**自己的** signal-ctx，不复用 db.Open 的 5s timeout —— migrate up
	// 跑多个 SQL 文件可能耗时几十秒。详见 Story 4.3 Dev Notes。
	if isMigrate {
		migrateCtx, migrateStop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer migrateStop()
		if err := cli.RunMigrate(migrateCtx, cfg, migrateArgs); err != nil {
			slog.Error("migrate failed", slog.Any("error", err))
			os.Exit(1)
		}
		os.Exit(0)
	}

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

	if err := bootstrap.Run(ctx, cfg, gormDB, txMgr); err != nil {
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
