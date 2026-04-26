package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/repo/tx"
)

const shutdownTimeout = 5 * time.Second

// Run 启动 HTTP server 并阻塞直到 ctx 取消或 serve 循环失败。
//
// bind 是**同步**完成的（net.Listen），失败直接作为 Run 的返回值抛出；
// 只有在 bind 确实成功后才打印 "server started on :<port>"。
// 这样避免了 "server started" 假阳性 banner —— 历史坑见
// docs/lessons/2026-04-24-config-path-and-bind-banner.md。
//
// 参数（Story 4.2 扩展）：
//
//   - gormDB / txMgr 是 Story 4.2 接入 MySQL 后的依赖。本 story 阶段 router / handler
//     **暂不消费**（Story 4.6 挂业务 handler 时才用）；签名先扩展是为了：
//     (1) Story 4.6 落地时不再改 main.go → bootstrap.Run 调用链
//     (2) 测试可注入：Story 4.7 Layer 2 集成测试 / 现有 server_test 通过显式参数
//         传入 mock 或 dockertest 真实 db handle
//   - 允许 gormDB / txMgr 为 nil：仅当现有不需要 db 的测试路径（如本包 server_test
//     验证 bind 失败 / 优雅关停）。生产路径 main.go 必传非 nil。
func Run(ctx context.Context, cfg *config.Config, gormDB *gorm.DB, txMgr tx.Manager) error {
	// 防御式：避免参数变量未读触发 vet "declared and not used"。本 story 阶段
	// router 暂不消费，但保留参数让未来 Story 4.6 落地时无需再改签名。
	_ = gormDB
	_ = txMgr

	router := NewRouter()
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
