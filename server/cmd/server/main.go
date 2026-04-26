package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/huing/cat/server/internal/app/bootstrap"
	"github.com/huing/cat/server/internal/app/http/devtools"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/infra/logger"
	"github.com/huing/cat/server/internal/repo/tx"
)

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
	gormDB, err := db.Open(ctx, cfg.MySQL)
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
