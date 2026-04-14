package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/huing7373/catc/server/internal/ws"
	"github.com/huing7373/catc/server/pkg/jwtx"
)

// WSHandler upgrades HTTP requests to WebSocket after verifying a JWT
// access token provided via the ?token= query parameter.
type WSHandler struct {
	hub     *ws.Hub
	router  *ws.Router
	jwtMgr  *jwtx.Manager
	upgrade websocket.Upgrader
}

// NewWSHandler wires the handler. origins is the CORS-style allow-list
// used by the WebSocket upgrade check; an empty list accepts none.
func NewWSHandler(hub *ws.Hub, router *ws.Router, jwtMgr *jwtx.Manager, allowedOrigins []string) *WSHandler {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}
	return &WSHandler{
		hub:    hub,
		router: router,
		jwtMgr: jwtMgr,
		upgrade: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					// native apps (iOS/watchOS) omit Origin.
					return true
				}
				_, ok := originSet[origin]
				return ok
			},
		},
	}
}

// Serve handles GET /v1/ws. It verifies the access token, upgrades, and
// hands the client off to the hub.
func (h *WSHandler) Serve(c *gin.Context) {
	token := c.Query("token")
	uid, err := h.jwtMgr.ParseAccess(token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{"code": "UNAUTHORIZED", "message": "unauthorized"},
		})
		return
	}

	conn, err := h.upgrade.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := ws.NewClient(uid, conn)
	if err := h.hub.Register(client); err != nil {
		_ = conn.Close()
		return
	}

	ctx := c.Request.Context()
	go client.WritePump(ctx)
	go func() {
		defer h.hub.Unregister(client)
		client.ReadPump(ctx, func(ctx2 context.Context, env ws.Envelope) error {
			return h.router.Dispatch(ctx2, string(uid), env)
		})
	}()
}
