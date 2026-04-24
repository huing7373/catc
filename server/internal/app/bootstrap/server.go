package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
)

const shutdownTimeout = 5 * time.Second

// Run 启动 HTTP server 并阻塞直到 ctx 取消或 serve 循环失败。
//
// bind 是**同步**完成的（net.Listen），失败直接作为 Run 的返回值抛出；
// 只有在 bind 确实成功后才打印 "server started on :<port>"。
// 这样避免了 "server started" 假阳性 banner —— 历史坑见
// docs/lessons/2026-04-24-config-path-and-bind-banner.md。
func Run(ctx context.Context, cfg *config.Config) error {
	router := NewRouter()
	addr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
	srv := &http.Server{
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	slog.Info("server started", slog.String("addr", addr))

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown error", slog.Any("error", err))
			return err
		}
		slog.Info("server stopped")
		return nil
	case err := <-errCh:
		return err
	}
}
