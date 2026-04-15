package main

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/huing7373/catc/server/internal/config"
	"github.com/huing7373/catc/server/internal/cron"
	"github.com/huing7373/catc/server/internal/handler"
	"github.com/huing7373/catc/server/internal/middleware"
	"github.com/huing7373/catc/server/internal/push"
	"github.com/huing7373/catc/server/internal/repository"
	"github.com/huing7373/catc/server/internal/service"
	"github.com/huing7373/catc/server/internal/ws"
	"github.com/huing7373/catc/server/pkg/applex"
	"github.com/huing7373/catc/server/pkg/logx"
	"github.com/huing7373/catc/server/pkg/mongox"
	"github.com/huing7373/catc/server/pkg/redisx"
)

// initialize performs the one and only explicit dependency-injection
// pass. Construction order:
//
//  1. Infrastructure: logx → mongo → redis → jwt → apns
//  2. Repositories (receive infrastructure)
//  3. EnsureIndexes (startup-only I/O on the indexes)
//  4. Services (receive repositories)
//  5. WebSocket hub (receives services)
//  6. Handlers (receive services/hub/JWT)
//  7. HTTP router (receives handlers)
//  8. Cron scheduler (receives services/repos)
//  9. App (receives every Runnable in start order)
func initialize(cfg *config.Config) *App {
	// 1. Infrastructure.
	logx.Init(logx.Config{Level: cfg.Log.Level, Format: cfg.Log.Format})

	mongoCli := mongox.MustConnect(mongox.Config{
		URI:        cfg.Mongo.URI,
		Database:   cfg.Mongo.Database,
		TimeoutSec: cfg.Mongo.TimeoutSec,
	})
	rdb := redisx.MustConnect(redisx.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	jwtMgr := mustNewJWT(cfg.JWT, cfg.AccessTTL(), cfg.RefreshTTL())

	appleVerifier := applex.New(applex.Config{
		JWKSURL:          cfg.Apple.JWKSURL,
		JWKSCacheTTL:     cfg.AppleJWKSCacheTTL(),
		AllowedAudiences: cfg.Apple.AllowedAudiences,
	})

	authLimiter := middleware.NewRedisLimiter(rdb, "auth-login", 60, repository.RateLimitKey)

	var pusher push.Pusher = push.NewAPNsPusher(push.Config{
		KeyID:    cfg.APNs.KeyID,
		TeamID:   cfg.APNs.TeamID,
		BundleID: cfg.APNs.BundleID,
		KeyPath:  cfg.APNs.KeyPath,
	})
	if cfg.APNs.KeyPath == "" {
		// No credentials configured yet → silent pusher keeps main
		// flows unblocked in dev.
		pusher = push.NullPusher{}
	}
	_ = pusher

	// 2. Repositories.
	userRepo := repository.NewUserRepo(mongoCli, cfg.Mongo.Database, rdb)

	// 3. EnsureIndexes — startup-only I/O on the schema.
	ensureCtx, cancel := context.WithTimeout(context.Background(), cfg.MongoTimeout())
	defer cancel()
	if err := userRepo.EnsureIndexes(ensureCtx); err != nil {
		log.Fatal().Err(err).Msg("ensure indexes failed")
	}

	// 4. Services.
	userSvc := service.NewUserService(userRepo)
	_ = userSvc
	authSvc := service.NewAuthService(appleVerifier, userRepo, jwtMgr, cfg.AccessTTL(), cfg.RefreshTTL())

	// 5. WebSocket hub + router.
	hub := ws.NewHub()
	wsRouter := ws.NewRouter()

	// 6. Handlers.
	mcheck, rcheck := healthProbes(mongoCli, rdb)
	h := handlers{
		health: handler.NewHealthHandler(mcheck, rcheck),
		auth:   handler.NewAuthHandler(authSvc),
		ws:     handler.NewWSHandler(hub, wsRouter, jwtMgr, cfg.Server.CORSAllowedOrigins),
	}

	// 7. HTTP router.
	engine := buildRouter(cfg, h, jwtMgr, authLimiter)
	http := newHTTPServer(addrOf(cfg.Server.Port), engine)

	// 8. Cron scheduler.
	sched := cron.NewScheduler()
	if err := cron.RegisterJobs(sched, nil); err != nil {
		log.Fatal().Err(err).Msg("cron register failed")
	}

	// 9. App assembly. Order determines startup order, and reverse
	// order determines shutdown order — HTTP and hub shut before the
	// underlying Mongo/Redis clients are torn down.
	return NewApp(cfg,
		mongox.NewRunnable(mongoCli),
		redisx.NewRunnable(rdb),
		hub,
		sched,
		http,
	)
}
