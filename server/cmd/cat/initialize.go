package main

import (
	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/internal/handler"
	"github.com/rs/zerolog/log"
)

var buildVersion = "dev"

func initialize(cfg *config.Config) *App {
	log.Info().
		Str("build_version", buildVersion).
		Str("config_hash", cfg.Hash).
		Msg("server starting")

	h := &handlers{
		health: handler.NewHealthHandler(),
	}

	router := buildRouter(cfg, h)
	httpSrv := newHTTPServer(cfg, router)

	return NewApp(httpSrv)
}
