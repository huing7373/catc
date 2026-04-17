package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/internal/handler"
	"github.com/rs/zerolog/log"
)

type handlers struct {
	health *handler.HealthHandler
}

func buildRouter(_ *config.Config, h *handlers) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/healthz", h.health.Healthz)
	return r
}

type httpServer struct {
	srv *http.Server
}

func newHTTPServer(cfg *config.Config, router *gin.Engine) *httpServer {
	return &httpServer{
		srv: &http.Server{
			Addr:    net.JoinHostPort(cfg.Server.Host, fmt.Sprintf("%d", cfg.Server.Port)),
			Handler: router,
		},
	}
}

func (h *httpServer) Name() string { return "http_server" }

func (h *httpServer) Start(_ context.Context) error {
	log.Info().Str("addr", h.srv.Addr).Msg("http server listening")
	if err := h.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (h *httpServer) Final(ctx context.Context) error {
	log.Info().Msg("http server shutting down")
	return h.srv.Shutdown(ctx)
}
