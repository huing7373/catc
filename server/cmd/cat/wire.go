package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/huing7373/catc/server/internal/config"
	"github.com/huing7373/catc/server/internal/handler"
	"github.com/huing7373/catc/server/internal/middleware"
	"github.com/huing7373/catc/server/pkg/jwtx"
	"github.com/huing7373/catc/server/pkg/mongox"
	"github.com/huing7373/catc/server/pkg/redisx"
)

// handlers is the aggregate struct passed to buildRouter. Adding a new
// handler == add a field here and wire in initialize.
type handlers struct {
	health *handler.HealthHandler
	auth   *handler.AuthHandler
	ws     *handler.WSHandler
}

// buildRouter constructs the Gin engine with middleware in the required
// order and mounts every handler.
//
// Middleware order (per architecture guide §13):
//  1. Recovery      — outermost panic safety net
//  2. RequestLogger — request_id injection + access log
//  3. CORS          — production whitelist
//  4. (auth/ratelimit are mounted inside /v1/ groups, not globally)
func buildRouter(cfg *config.Config, h handlers, jwtMgr *jwtx.Manager, authLimiter middleware.Limiter) *gin.Engine {
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())
	r.Use(middleware.CORS(cfg.Server.CORSAllowedOrigins))

	r.GET("/health", h.health.Get)

	v1 := r.Group("/v1")
	{
		// /v1/auth — public (no AuthRequired); guarded by per-IP
		// sliding-window rate-limit (10 / minute, burst 10).
		auth := v1.Group("/auth")
		auth.Use(middleware.RateLimit(authLimiter, middleware.IPKey, 10, 10))
		{
			auth.POST("/login", h.auth.Login)
			auth.POST("/refresh", h.auth.Refresh)
		}

		// WebSocket. Auth happens inside Serve via JWT query param.
		v1.GET("/ws", h.ws.Serve)

		// Authenticated business endpoints land here in later stories.
		authed := v1.Group("")
		authed.Use(middleware.AuthRequired(jwtMgr))
		_ = authed
	}

	return r
}

// httpServer is a Runnable wrapper around *http.Server.
type httpServer struct {
	srv *http.Server
}

func newHTTPServer(addr string, handler http.Handler) *httpServer {
	return &httpServer{
		srv: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

func (s *httpServer) Name() string { return "http" }

func (s *httpServer) Start(ctx context.Context) error {
	err := s.srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *httpServer) Final(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// healthProbes returns HealthChecker callbacks wired to the live
// client instances. Kept out of initialize to keep it short.
func healthProbes(mongoCli *mongo.Client, rdb *redis.Client) (handler.HealthChecker, handler.HealthChecker) {
	mcheck := func(ctx context.Context) error { return mongox.HealthCheck(ctx, mongoCli) }
	rcheck := func(ctx context.Context) error { return redisx.HealthCheck(ctx, rdb) }
	return mcheck, rcheck
}

// mustNewJWT centralises panic-to-fatal translation for the JWT
// manager constructor so initialize stays a clean linear script.
//
// The previous-secret pair is forwarded so a 24h key rotation actually
// takes effect at runtime (otherwise the manager runs single-key and
// any token signed with the old secret 401s the moment the new secret
// is deployed).
func mustNewJWT(cfg config.JWTCfg, accessTTL, refreshTTL time.Duration) *jwtx.Manager {
	mgr, err := jwtx.New(jwtx.Config{
		AccessSecret:          cfg.AccessSecret,
		RefreshSecret:         cfg.RefreshSecret,
		AccessSecretPrevious:  cfg.AccessSecretPrevious,
		RefreshSecretPrevious: cfg.RefreshSecretPrevious,
		AccessTTL:             accessTTL,
		RefreshTTL:            refreshTTL,
		Issuer:                cfg.Issuer,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("jwt manager build failed")
	}
	return mgr
}

// addrOf formats the listen address from a port.
func addrOf(port int) string { return fmt.Sprintf(":%d", port) }
