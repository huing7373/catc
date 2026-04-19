package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/internal/cron"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/push"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/cryptox"
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/logx"
	"github.com/huing/cat/server/pkg/mongox"
	"github.com/huing/cat/server/pkg/redisx"
	"go.mongodb.org/mongo-driver/v2/mongo"
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

	// --- Story 1.4: APNs device token repo (§21.2 Empty→Real) ---
	// Built BEFORE cron + push wiring so the same *MongoApnsTokenRepository
	// satisfies push.TokenProvider / TokenDeleter / TokenCleaner in all
	// three downstream constructors.
	apnsTokenRepo := mustBuildApnsTokenRepo(cfg, mongoCli.DB(), clk)
	if err := apnsTokenRepo.EnsureIndexes(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("apns_token repo EnsureIndexes failed")
	}
	// --- /Story 1.4 ---

	// TokenCleaner real impl — swapped Empty via Story 1.4 (§21.2).
	cronSch := cron.NewScheduler(
		locker, redisCli.Cmdable(), clk, apnsTokenRepo,
		time.Duration(cfg.APNs.TokenExpiryDays)*24*time.Hour,
	)

	// Push platform — APNs worker + Pusher façade. When apns.enabled=false
	// (default / debug), NoopPusher keeps downstream service code safe
	// without opening any APNs HTTP/2 connection. Real impl opts in via
	// release-deploy config (key_path + key_id + team_id + topics).
	var pusher push.Pusher = push.NoopPusher{}
	var apnsWorker *push.APNsWorker
	if cfg.APNs.Enabled {
		sender, err := push.NewApnsClient(
			cfg.APNs.KeyPath, cfg.APNs.KeyID, cfg.APNs.TeamID,
			cfg.Server.Mode == "release",
		)
		if err != nil {
			log.Fatal().Err(err).Msg("apns client init failed")
		}
		streamPusher := redisx.NewStreamPusher(redisCli.Cmdable(), cfg.APNs.StreamKey)
		pusher = push.NewRedisStreamsPusher(
			streamPusher, redisCli.Cmdable(), clk,
			time.Duration(cfg.APNs.IdemTTLSec)*time.Second,
		)
		// TokenProvider real impl — swapped Empty via Story 1.4 (§21.2).
		router := push.NewAPNsRouter(apnsTokenRepo, cfg.APNs.WatchTopic, cfg.APNs.IphoneTopic)
		// TokenDeleter real impl — swapped Empty via Story 1.4 (§21.2).
		// QuietHoursResolver real impl — Story 1.5 fills this.
		apnsWorker = push.NewAPNsWorker(push.APNsWorkerConfig{
			InstanceID:      locker.InstanceID(),
			StreamKey:       cfg.APNs.StreamKey,
			DLQKey:          cfg.APNs.DLQKey,
			RetryZSetKey:    cfg.APNs.RetryZSetKey,
			ConsumerGroup:   cfg.APNs.ConsumerGroup,
			WorkerCount:     cfg.APNs.WorkerCount,
			ReadBlock:       time.Duration(cfg.APNs.ReadBlockMs) * time.Millisecond,
			ReadCount:       int64(cfg.APNs.ReadCount),
			RetryBackoffsMs: cfg.APNs.RetryBackoffsMs,
			MaxAttempts:     cfg.APNs.MaxAttempts,
		}, redisCli.Cmdable(), sender, router, push.EmptyQuietHoursResolver{}, apnsTokenRepo, clk)
		log.Info().Msg("apns push platform enabled")
	} else {
		log.Info().Msg("apns disabled (cfg.apns.enabled=false); NoopPusher in use")
	}
	_ = pusher // Epic 0: no service consumes yet; Story 5.2/6.2/8.2/1.5 will.

	wsHub := ws.NewHub(ws.HubConfig{
		PingInterval:   time.Duration(cfg.WS.PingIntervalSec) * time.Second,
		PongTimeout:    time.Duration(cfg.WS.PongTimeoutSec) * time.Second,
		SendBufSize:    cfg.WS.SendBufSize,
		MaxConnections: cfg.WS.MaxConnections,
	}, clk)

	dedupStore := redisx.NewDedupStore(redisCli.Cmdable(), time.Duration(cfg.WS.DedupTTLSec)*time.Second)
	dispatcher := ws.NewDispatcher(dedupStore, clk)

	// --- Story 1.1 Apple SIWA wiring ---
	appleJWKFetcher := jwtx.NewAppleJWKFetcher(redisCli.Cmdable(), clk, jwtx.AppleJWKConfig{
		JWKSURL:      cfg.Apple.JWKSURL,
		CacheKey:     cfg.Apple.JWKSCacheKey,
		CacheTTL:     time.Duration(cfg.Apple.JWKSCacheTTLSec) * time.Second,
		FetchTimeout: time.Duration(cfg.Apple.JWKSFetchTimeoutSec) * time.Second,
	})
	jwtMgr := jwtx.NewManagerWithApple(jwtx.Options{
		PrivateKeyPath:    cfg.JWT.PrivateKeyPath,
		PrivateKeyPathOld: cfg.JWT.PrivateKeyPathOld,
		ActiveKID:         cfg.JWT.ActiveKID,
		OldKID:            cfg.JWT.OldKID,
		Issuer:            cfg.JWT.Issuer,
		AccessExpirySec:   cfg.JWT.AccessExpirySec,
		RefreshExpirySec:  cfg.JWT.RefreshExpirySec,
	}, jwtx.AppleVerifyDeps{
		Fetcher:  appleJWKFetcher,
		BundleID: cfg.Apple.BundleID,
		Clock:    clk,
	})
	userRepo := repository.NewMongoUserRepository(mongoCli.DB(), clk)
	if err := userRepo.EnsureIndexes(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("user repo EnsureIndexes failed")
	}
	// --- Story 1.2 refresh blacklist wiring ---
	refreshBlacklist := redisx.NewRefreshBlacklist(redisCli.Cmdable(), clk)
	// --- /Story 1.2 ---
	authSvc := service.NewAuthService(
		userRepo,
		jwtMgr,           // AppleVerifier
		jwtMgr,           // RefreshVerifier
		jwtMgr,           // JWTIssuer (Issue + RefreshExpiry)
		refreshBlacklist, // RefreshBlacklist
		clk,
		cfg.Server.Mode,
	)
	authHandler := handler.NewAuthHandler(authSvc)
	// --- /Story 1.1 ---

	resumeCache := redisx.NewResumeCache(
		redisCli.Cmdable(),
		clk,
		time.Duration(cfg.WS.ResumeCacheTTLSec)*time.Second,
	)
	// UserProvider 真实实现 — removed Empty via Story 1.1.
	realUserProvider := ws.NewRealUserProvider(userRepo, clk)
	sessionResumeHandler := ws.NewSessionResumeHandler(resumeCache, clk, ws.ResumeProviders{
		User:         realUserProvider,
		Friends:      ws.EmptyFriendsProvider{},
		CatState:     ws.EmptyCatStateProvider{},
		Skins:        ws.EmptySkinsProvider{},
		Blindboxes:   ws.EmptyBlindboxesProvider{},
		RoomSnapshot: ws.EmptyRoomSnapshotProvider{},
	})

	broadcaster := ws.NewInMemoryBroadcaster(wsHub)

	// Story 1.1 — session.resume now registers in BOTH modes. The
	// UserProvider is real; the remaining five Empty providers return
	// genuine empty state for a brand-new account (no friends, no
	// skins, no blindboxes, no cat state, no room snapshot). The
	// release guard around session.resume can be deleted entirely
	// once Story 4.5 lands the last real provider.
	dispatcher.Register("session.resume", sessionResumeHandler.Handle)

	var validator ws.TokenValidator
	if cfg.Server.Mode == "debug" {
		validator = ws.NewDebugValidator()
		echoFn := func(_ context.Context, _ *ws.Client, env ws.Envelope) (json.RawMessage, error) {
			return env.Payload, nil
		}
		dispatcher.Register("debug.echo", echoFn)
		dispatcher.RegisterDedup("debug.echo.dedup", echoFn)

		// Story 10.1 联调 MVP: room.join / action.update handlers + an
		// in-memory RoomManager. action.broadcast is Direction=down (no
		// handler). RoomManager observes Hub disconnects to clean up
		// rooms on client drop. The entire wiring below — and
		// room_mvp.go — disappears when Epic 4.1 ships.
		roomManager := ws.NewRoomManager(clk, broadcaster)
		wsHub.AddObserver(roomManager)
		dispatcher.Register("room.join", roomManager.HandleJoin)
		dispatcher.Register("action.update", roomManager.HandleActionUpdate)

		log.Info().Msg("debug mode: debug.echo, debug.echo.dedup, session.resume, room.join, action.update handlers registered")
	} else {
		validator = ws.NewJWTValidator(jwtMgr)
		log.Info().Msg("release mode: JWT validator wired against jwtx.Manager (Story 1.1 — accepts access tokens issued by /auth/apple)")
	}

	blacklist := redisx.NewBlacklist(redisCli.Cmdable())
	connLimiter := redisx.NewConnectRateLimiter(
		redisCli.Cmdable(),
		clk,
		int64(cfg.WS.ConnectRatePerWindow),
		time.Duration(cfg.WS.ConnectRateWindowSec)*time.Second,
	)
	upgradeHandler := ws.NewUpgradeHandler(wsHub, dispatcher, validator, blacklist, connLimiter)

	// Registry-drift fail-fast (Story 0.14 AC15): every dispatcher
	// registration MUST have a dto.WSMessages entry, and every non-DebugOnly
	// entry MUST be registered in release mode. Unit tests cover this at CI
	// time; the runtime check catches feature-flag drift (e.g. a conditional
	// Register gated on an env var). Failing fast beats serving a registry
	// response that lies about what the dispatcher accepts.
	if err := validateRegistryConsistency(dispatcher, cfg.Server.Mode); err != nil {
		log.Fatal().Err(err).Msg("ws message registry drift detected")
	}

	httpJWTAuth := buildHTTPJWTAuth(cfg.Server.Mode, jwtMgr)

	// --- Story 1.4: device handler wiring ---
	apnsRegisterLimiter := redisx.NewUserSlidingWindowLimiter(
		redisCli.Cmdable(), clk,
		"ratelimit:apns_token:",
		int64(cfg.APNs.RegisterRatePerWindow),
		time.Duration(cfg.APNs.RegisterRateWindowSec)*time.Second,
	)
	apnsTokenSvc := service.NewApnsTokenService(apnsTokenRepo, userRepo, apnsRegisterLimiter, clk)
	deviceHandler := handler.NewDeviceHandler(apnsTokenSvc)
	// --- /Story 1.4 ---

	h := &handlers{
		health:    handler.NewHealthHandler(mongoCli, redisCli, wsHub, redisCli.Cmdable(), cfg.WS.MaxConnections*2),
		wsUpgrade: upgradeHandler,
		platform:  handler.NewPlatformHandler(clk, cfg.Server.Mode),
		auth:      authHandler,
		jwtAuth:   httpJWTAuth,
		device:    deviceHandler,
	}

	router := buildRouter(cfg, h)
	httpSrv := newHTTPServer(cfg, router)

	// Runs order: mongo, redis, cron, wsHub, [apnsWorker?], http.
	// Reverse Final order matches architecture §Graceful Shutdown line 218
	// (HTTP → wsHub → cron → APNs worker → redis → mongo). Keeping the
	// worker AFTER cron in positive order puts its Final BEFORE cron's —
	// the worker drains in-flight APNs sends first, then cron stops.
	var app *App
	if apnsWorker != nil {
		app = NewApp(mongoCli, redisCli, cronSch, wsHub, apnsWorker, httpSrv)
	} else {
		app = NewApp(mongoCli, redisCli, cronSch, wsHub, httpSrv)
	}
	app.OnReady(h.health.SetReady)
	return app
}

// validateRegistryConsistency fails fast if the dispatcher's registered
// types drift from the dto.WSMessages source of truth. Invariants:
//
//  1. Every dispatcher registration has a matching dto.WSMessages entry
//     (regardless of mode).
//  2. In debug mode: every dto.WSMessages entry with Direction up/bi is
//     registered. A missing registration is drift — the registry endpoint
//     still advertises the type, but the dispatcher would return
//     UNKNOWN_MESSAGE_TYPE when a client sends it. Direction=down entries
//     are server→client pushes that never flow through Dispatch, so they
//     are exempt from the "must be registered" requirement (Story 10.1).
//  3. In release mode: every non-DebugOnly dto.WSMessages entry with
//     Direction up/bi is registered; no DebugOnly entry is registered
//     (DebugOnly entries are deliberately absent in release — that is
//     their purpose). Direction=down entries are exempt as in debug.
//
// Kept unexported and in cmd/cat so it can read cfg.Server.Mode directly;
// unit tests in initialize_test.go mirror each invariant per mode.
func validateRegistryConsistency(d *ws.Dispatcher, mode string) error {
	registered := make(map[string]bool, len(d.RegisteredTypes()))
	for _, t := range d.RegisteredTypes() {
		registered[t] = true
	}

	known := dto.WSMessagesByType
	var unknownRegistered, missingInDebug, missingInRelease, forbiddenInRelease []string

	for t := range registered {
		if _, ok := known[t]; !ok {
			unknownRegistered = append(unknownRegistered, t)
		}
	}

	if mode == "debug" {
		for _, meta := range dto.WSMessages {
			if meta.Direction == dto.WSDirectionDown {
				// downstream-only pushes never go through Dispatch; they
				// live in WSMessages purely for the registry endpoint.
				continue
			}
			if !registered[meta.Type] {
				missingInDebug = append(missingInDebug, meta.Type)
			}
		}
	} else {
		for _, meta := range dto.WSMessages {
			if meta.DebugOnly {
				if registered[meta.Type] {
					forbiddenInRelease = append(forbiddenInRelease, meta.Type)
				}
				continue
			}
			if meta.Direction == dto.WSDirectionDown {
				continue
			}
			if !registered[meta.Type] {
				missingInRelease = append(missingInRelease, meta.Type)
			}
		}
	}

	sort.Strings(unknownRegistered)
	sort.Strings(missingInDebug)
	sort.Strings(missingInRelease)
	sort.Strings(forbiddenInRelease)

	if len(unknownRegistered) == 0 && len(missingInDebug) == 0 && len(missingInRelease) == 0 && len(forbiddenInRelease) == 0 {
		return nil
	}
	return fmt.Errorf(
		"ws registry drift: unknownRegistered=%v missingInDebug=%v missingInRelease=%v debugOnlyInRelease=%v",
		unknownRegistered, missingInDebug, missingInRelease, forbiddenInRelease,
	)
}

// buildHTTPJWTAuth picks the gin middleware mounted on /v1/*.
//
// Release mode (anything except "debug") returns
// middleware.JWTAuth(verifier). Debug mode deliberately returns nil —
// MVP debug has no /v1/* business endpoint to protect (the only /v1/*
// route is the pre-auth /v1/platform/ws-registry probe, which is
// hoisted out of the group at the top-level r.GET in wire.go) and
// unit tests that do need auth wire middleware.JWTAuth(fakeVerifier)
// directly. Mirrors the debug/release split of the WS validator
// (DebugValidator vs JWTValidator) so the two auth surfaces stay in
// lockstep. Extracted from initialize() so the inverse-gate
// regression is unit-testable without booting a full Mongo+Redis
// stack — review-antipatterns §7.1 ("gate written backwards"). The
// conditional is `!= "debug"` so a release deployment ALWAYS mounts
// the middleware; the initialize_test.go pair locks both branches.
func buildHTTPJWTAuth(mode string, verifier middleware.JWTVerifier) gin.HandlerFunc {
	if mode != "debug" {
		log.Info().Str("mode", mode).Msg("release mode: HTTP JWTAuth middleware mounted on /v1/* group")
		return middleware.JWTAuth(verifier)
	}
	log.Info().Msg("debug mode: HTTP JWTAuth NOT mounted (no /v1/* business endpoint yet; debug handlers wire JWTAuth(fakeVerifier) directly)")
	return nil
}

// mustBuildApnsTokenRepo constructs the Story 1.4 APNs token repo with
// an AES-GCM sealer. Release mode requires cfg.APNs.TokenEncryptionKeyHex
// (enforced earlier by config.validateAPNs — the release-mode check
// runs regardless of APNs.Enabled so staging deployments cannot
// accidentally persist tokens under a dev-only key). Debug / test mode
// falls back to an all-zero 32-byte key so tests that exercise the repo
// do not need secret plumbing; that branch is NEVER reachable in
// release (§7.1 debug/release gate).
func mustBuildApnsTokenRepo(cfg *config.Config, db *mongo.Database, clk clockx.Clock) *repository.MongoApnsTokenRepository {
	var key []byte
	if cfg.APNs.TokenEncryptionKeyHex == "" {
		// Debug / test fallback. validateAPNs would have log.Fatal'd
		// already in release mode.
		log.Info().Msg("apns token repo: using dev all-zero encryption key (non-release)")
		key = make([]byte, 32)
	} else {
		decoded, err := hex.DecodeString(cfg.APNs.TokenEncryptionKeyHex)
		if err != nil {
			log.Fatal().Err(err).Msg("apns token_encryption_key_hex decode failed")
		}
		key = decoded
	}
	sealer, err := cryptox.NewAESGCMSealer(key)
	if err != nil {
		log.Fatal().Err(err).Msg("apns token sealer init failed")
	}
	return repository.NewMongoApnsTokenRepository(db, clk, sealer)
}
