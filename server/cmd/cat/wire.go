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
	platform  *handler.PlatformHandler
	auth      *handler.AuthHandler
	jwtAuth   gin.HandlerFunc       // Story 1.3 — mounted on /v1/* group; nil in debug mode
	device    *handler.DeviceHandler // Story 1.4 — POST /v1/devices/apns-token
	user      *handler.UserHandler   // Story 1.6 — DELETE /v1/users/me
	// v1Routes lets test harnesses inject extra /v1/* routes (e.g. an
	// integration-test echo endpoint that reads UserIDFrom). Production
	// wiring leaves it nil — Story 1.4 onward adds real routes directly
	// inside buildRouter via the typed handler fields above.
	v1Routes func(*gin.RouterGroup)
}

func buildRouter(_ *config.Config, h *handlers) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.Logger())
	r.Use(middleware.Recover())
	r.Use(middleware.RequestID())
	r.GET("/healthz", h.health.Healthz)
	r.GET("/readyz", h.health.Readyz)
	// Bootstrap endpoint — intentionally OUTSIDE the /v1/* JWT group
	// (architecture line 814). Clients hit this pre-auth to verify protocol
	// compatibility (FR59 / Story 0.14 AC6). gin matches the explicit
	// top-level route before the /v1 group middleware, so the JWTAuth
	// middleware below does NOT intercept /v1/platform/ws-registry —
	// TestRouter_V1Group_DoesNotIntercept_PlatformRegistry locks this.
	r.GET("/v1/platform/ws-registry", h.platform.WSRegistry)
	// Bootstrap auth endpoints — also OUTSIDE /v1/* JWT group. Story 1.1
	// shipped /auth/apple; Story 1.2 adds /auth/refresh (rolling-rotation +
	// stolen-token reuse detection). The refresh token in the body IS the
	// credential — no JWT middleware.
	if h.auth != nil {
		r.POST("/auth/apple", h.auth.SignInWithApple)
		r.POST("/auth/refresh", h.auth.Refresh)
	}
	r.GET("/ws", h.wsUpgrade.Handle)

	// --- Story 1.3: /v1/* authenticated group ---
	// Every business endpoint added from Story 1.4 onward (devices /
	// users / state / profile / blindbox / friend / skin) lands inside
	// this group. The platform/ws-registry endpoint above is the one
	// and only /v1/* exception (pre-auth protocol probe). Debug mode
	// leaves jwtAuth nil — the group is still created so v1Routes can
	// hook in if a test wires it, but no JWT middleware runs.
	v1 := r.Group("/v1")
	if h.jwtAuth != nil {
		v1.Use(h.jwtAuth)
	}
	// Story 1.4 — first /v1/* business endpoint. Route registration is
	// unconditional (always present even when apns.enabled=false) so a
	// staging client that POSTs here gets 200 / 401 / 429 from the
	// real handler chain rather than a 404 that triggers useless retry
	// storms. In debug mode jwtAuth is nil, so the handler's own
	// defense-in-depth check (UserIDFrom/DeviceIDFrom empty → 401) is
	// what keeps unauthenticated debug traffic out. See AC9 for the
	// "why not conditional route" rationale.
	if h.device != nil {
		v1.POST("/devices/apns-token", h.device.RegisterApnsToken)
	}
	// Story 1.6 — DELETE /v1/users/me (request account deletion).
	// Same conditional-mount rationale as h.device: unit tests that
	// exercise routing pieces in isolation do NOT need to wire the full
	// service graph. Release production always passes a non-nil handler
	// from initialize.go.
	if h.user != nil {
		v1.DELETE("/users/me", h.user.RequestDeletion)
	}
	if h.v1Routes != nil {
		h.v1Routes(v1)
	}
	// --- /Story 1.3 ---

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
