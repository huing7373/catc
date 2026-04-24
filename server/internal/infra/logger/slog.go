package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{}

// Init 构造 slog.Logger（JSON handler，写 stdout），并设为 slog.Default()。
// level 支持 debug / info / warn / error（大小写无关）；非法值回落 info 并通过 WARN 日志提示一次。
//
// 设计意图：本函数**不依赖 config 包**，使 cmd/server/main.go 可以在
// config 加载**之前**就初始化 JSON logger（以 "info" 为 bootstrap level），
// 这样 config.LocateDefault / config.Load 的启动失败错误也是 JSON 格式。
// config 加载成功后再用 cfg.Log.Level 重复调用一次，应用用户配置的级别。
// 历史背景：docs/lessons/2026-04-25-slog-init-before-startup-errors.md。
func Init(level string) *slog.Logger {
	lvl, ok := parseLevel(level)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	l := slog.New(handler)
	slog.SetDefault(l)
	if !ok {
		l.Warn("unknown log level, fallback to info", slog.String("given", level))
	}
	return l
}

// NewContext 把 *slog.Logger 塞进 ctx，下游用 FromContext 取回。
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext 从 ctx 取回 *slog.Logger；没有则返回 slog.Default()。
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// parseLevel 将字符串 level 映射为 slog.Level；不识别时 (info, false)。
func parseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "", "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}
