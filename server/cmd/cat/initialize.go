package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/internal/cron"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/logx"
	"github.com/huing/cat/server/pkg/mongox"
	"github.com/huing/cat/server/pkg/redisx"
)

var buildVersion = "dev"

func initialize(cfg *config.Config) *App {
	logx.Init(logx.Options{
		Level:        cfg.Log.Level,
		Format:       cfg.Log.Format,
		BuildVersion: buildVersion,
		ConfigHash:   cfg.Hash,
	})

	log.Info().Msg("server starting")

	mongoCli := mongox.MustConnect(mongox.ConnectOptions{
		URI:        cfg.Mongo.URI,
		DB:         cfg.Mongo.DB,
		TimeoutSec: cfg.Mongo.TimeoutSec,
	})
	redisCli := redisx.MustConnect(redisx.ConnectOptions{
		Addr: cfg.Redis.Addr,
		DB:   cfg.Redis.DB,
	})

	clk := clockx.NewRealClock()
	locker := redisx.NewLocker(redisCli.Cmdable())
	cronSch := cron.NewScheduler(locker, redisCli.Cmdable(), clk)

	wsHub := ws.NewHub(ws.HubConfig{
		PingInterval:   time.Duration(cfg.WS.PingIntervalSec) * time.Second,
		PongTimeout:    time.Duration(cfg.WS.PongTimeoutSec) * time.Second,
		SendBufSize:    cfg.WS.SendBufSize,
		MaxConnections: cfg.WS.MaxConnections,
	}, clk)

	dedupStore := redisx.NewDedupStore(redisCli.Cmdable(), time.Duration(cfg.WS.DedupTTLSec)*time.Second)
	dispatcher := ws.NewDispatcher(dedupStore, clk)

	resumeCache := redisx.NewResumeCache(
		redisCli.Cmdable(),
		clk,
		time.Duration(cfg.WS.ResumeCacheTTLSec)*time.Second,
	)
	sessionResumeHandler := ws.NewSessionResumeHandler(resumeCache, clk, ws.ResumeProviders{
		User:         ws.EmptyUserProvider{},
		Friends:      ws.EmptyFriendsProvider{},
		CatState:     ws.EmptyCatStateProvider{},
		Skins:        ws.EmptySkinsProvider{},
		Blindboxes:   ws.EmptyBlindboxesProvider{},
		RoomSnapshot: ws.EmptyRoomSnapshotProvider{},
	})

	var validator ws.TokenValidator
	if cfg.Server.Mode == "debug" {
		validator = ws.NewDebugValidator()
		echoFn := func(_ context.Context, _ *ws.Client, env ws.Envelope) (json.RawMessage, error) {
			return env.Payload, nil
		}
		dispatcher.Register("debug.echo", echoFn)
		dispatcher.RegisterDedup("debug.echo.dedup", echoFn)
		// session.resume is registered only in debug mode while every
		// provider is an Empty*Provider. In release mode handing a client
		// `user: null, friends: [], skins: [], ...` is indistinguishable
		// from legitimate "new account" state and would materially corrupt
		// the UI. Re-enable here when the first real Provider lands
		// (Story 1.1 UserProvider is the first; by Story 4.5 all six are
		// real and this guard can be removed along with the EmptyProviders
		// that feed it).
		dispatcher.Register("session.resume", sessionResumeHandler.Handle)
		log.Info().Msg("debug mode: debug.echo, debug.echo.dedup, and session.resume handlers registered")
	} else {
		validator = ws.NewStubValidator()
		// Cast the unused handler and cache to a blank assignment so linters
		// don't flag them — they still need to be *constructed* so Redis
		// config validation runs on every boot (fail-fast on ResumeCacheTTLSec
		// misconfiguration), even though no route is bound in release mode.
		_ = sessionResumeHandler
		log.Info().Msg("release mode: session.resume handler NOT registered (Empty providers would corrupt client state); see initialize.go comment")
	}

	blacklist := redisx.NewBlacklist(redisCli.Cmdable())
	connLimiter := redisx.NewConnectRateLimiter(
		redisCli.Cmdable(),
		clk,
		int64(cfg.WS.ConnectRatePerWindow),
		time.Duration(cfg.WS.ConnectRateWindowSec)*time.Second,
	)
	upgradeHandler := ws.NewUpgradeHandler(wsHub, dispatcher, validator, blacklist, connLimiter)

	h := &handlers{
		health:    handler.NewHealthHandler(mongoCli, redisCli, wsHub, redisCli.Cmdable(), cfg.WS.MaxConnections*2),
		wsUpgrade: upgradeHandler,
	}

	router := buildRouter(cfg, h)
	httpSrv := newHTTPServer(cfg, router)

	app := NewApp(mongoCli, redisCli, cronSch, wsHub, httpSrv)
	app.OnReady(h.health.SetReady)
	return app
}
