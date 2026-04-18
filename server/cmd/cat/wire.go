package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/ws"
	"github.com/rs/zerolog/log"
)

type handlers struct {
	health    *handler.HealthHandler
	wsUpgrade *ws.UpgradeHandler
}

func buildRouter(_ *config.Config, h *handlers) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.Logger())
	r.Use(middleware.Recover())
	r.Use(middleware.RequestID())
	r.GET("/healthz", h.health.Healthz)
	r.GET("/readyz", h.health.Readyz)
	r.GET("/ws", h.wsUpgrade.Handle)
	return r
}

type httpServer struct {
	srv   *http.Server
	ready chan struct{}
}

func newHTTPServer(cfg *config.Config, router *gin.Engine) *httpServer {
	return &httpServer{
		srv: &http.Server{
			Addr:    net.JoinHostPort(cfg.Server.Host, fmt.Sprintf("%d", cfg.Server.Port)),
			Handler: router,
		},
		ready: make(chan struct{}),
	}
}

func (h *httpServer) Name() string { return "http_server" }

func (h *httpServer) Ready() <-chan struct{} { return h.ready }

func (h *httpServer) Start(_ context.Context) error {
	ln, err := net.Listen("tcp", h.srv.Addr)
	if err != nil {
		return err
	}
	log.Info().Str("addr", ln.Addr().String()).Msg("http server listening")
	close(h.ready)
	if err := h.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (h *httpServer) Final(ctx context.Context) error {
	log.Info().Msg("http server shutting down")
	return h.srv.Shutdown(ctx)
}
