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
	"github.com/huing/cat/server/internal/infra/logger"
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

	if err := bootstrap.Run(ctx, cfg); err != nil {
		slog.Error("server run failed", slog.Any("error", err))
		os.Exit(1)
	}
}
