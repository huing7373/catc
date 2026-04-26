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

	var configPath string
	flag.StringVar(&configPath, "config", "", "path to config YAML (default: auto-detect server/configs/local.yaml or configs/local.yaml)")
	flag.Parse()

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
