package main

import (
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/pkg/mongox"
	"github.com/huing/cat/server/pkg/redisx"
)

var buildVersion = "dev"

func initialize(cfg *config.Config) *App {
	log.Info().
		Str("build_version", buildVersion).
		Str("config_hash", cfg.Hash).
		Msg("server starting")

	mongoCli := mongox.MustConnect(mongox.ConnectOptions{
		URI:        cfg.Mongo.URI,
		DB:         cfg.Mongo.DB,
		TimeoutSec: cfg.Mongo.TimeoutSec,
	})
	redisCli := redisx.MustConnect(redisx.ConnectOptions{
		Addr: cfg.Redis.Addr,
		DB:   cfg.Redis.DB,
	})

	h := &handlers{
		health: handler.NewHealthHandler(),
	}

	router := buildRouter(cfg, h)
	httpSrv := newHTTPServer(cfg, router)

	return NewApp(mongoCli, redisCli, httpSrv)
}
