package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type ctxKey struct{}

// 持有上一次 Init 打开的 log 文件，下次 Init 时关掉旧的避免 fd 泄漏。
// main.go 在 bootstrap + config 加载后会调 Init 两次（filePath 可能不同）。
var (
	mu          sync.Mutex
	currentFile *os.File
)

// Init 构造 slog.Logger（JSON handler）并设为 slog.Default()。
// level 支持 debug / info / warn / error（大小写无关）；非法值回落 info 并通过 WARN 日志提示一次。
// filePath 非空时，logger 同时写入 stdout 和该文件（追加模式）；空串 = 只写 stdout。
// 文件打开失败不阻断启动，退化为只写 stdout 并通过 WARN 日志提示。
//
// 设计意图：本函数**不依赖 config 包**，使 cmd/server/main.go 可以在
// config 加载**之前**就初始化 JSON logger（以 "info" 为 bootstrap level + 空 filePath），
// 这样 config.LocateDefault / config.Load 的启动失败错误也是 JSON 格式。
// config 加载成功后再用 cfg.Log.Level + cfg.Log.File 重复调用一次，应用用户配置。
// 历史背景：docs/lessons/2026-04-25-slog-init-before-startup-errors.md。
func Init(level, filePath string) *slog.Logger {
	lvl, ok := parseLevel(level)

	mu.Lock()
	defer mu.Unlock()

	// 关掉上次 Init 打开的文件（如果有），避免重复调 Init 时 fd 泄漏。
	if currentFile != nil {
		_ = currentFile.Close()
		currentFile = nil
	}

	var writer io.Writer = os.Stdout
	var fileOpenErr error
	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			// 退化只写 stdout；不阻断启动。fail-soft 而非 fail-fast 是因为日志落盘
			// 是辅助能力，比 stdout 输出更次要 —— 文件路径错配不应该让 server 起不来。
			fileOpenErr = err
		} else {
			writer = io.MultiWriter(os.Stdout, f)
			currentFile = f
		}
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: lvl})
	l := slog.New(handler)
	slog.SetDefault(l)
	if !ok {
		l.Warn("unknown log level, fallback to info", slog.String("given", level))
	}
	if fileOpenErr != nil {
		l.Warn("log file open failed, using stdout only",
			slog.String("path", filePath),
			slog.Any("error", fileOpenErr),
		)
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
