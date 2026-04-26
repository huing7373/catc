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
//
// 参数（Story 4.5 收敛后）：
//
//   - cfg 是已加载并默认值兜底完成的全局 Config。
//   - deps 是 bootstrap 期收集的依赖集合（GormDB / TxMgr / Signer / RateLimitCfg）；
//     字段 nil-tolerant，测试路径可传 Deps{} 零值（仅依赖 router 四件套 + 运维端点）。
//
// 历史背景（4.4 → 4.5 演进）：
//   - Story 4.2 把 Run 扩成 4 参数（加 gormDB / txMgr）
//   - Story 4.4 又扩成 5 参数（加 signer）
//   - Story 4.5 第二次扩（加 rateLimitCfg）时收敛为 Deps struct，避免每加一个依赖
//     就改 Run 签名 + 全部测试。后续 4.6 / 4.8 / Epic 5+ 加共享依赖时只改 Deps 字段。
func Run(ctx context.Context, cfg *config.Config, deps Deps) error {
	router := NewRouter(deps)
	// BindHost 空串保持原行为（0.0.0.0，所有接口）= 生产默认；
	// 测试注入 "127.0.0.1" → loopback-only，避开 Windows Firewall 弹窗。
	addr := fmt.Sprintf("%s:%d", cfg.Server.BindHost, cfg.Server.HTTPPort)
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
