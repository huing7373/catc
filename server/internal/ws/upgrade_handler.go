package ws

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/dto"
)

type TokenValidator interface {
	ValidateToken(token string) (userID string, err error)
}

type debugValidator struct{}

func (debugValidator) ValidateToken(token string) (string, error) {
	if token == "" {
		return "", errors.New("empty token")
	}
	return token, nil
}

type stubValidator struct{}

func (stubValidator) ValidateToken(_ string) (string, error) {
	return "", dto.ErrAuthInvalidIdentityToken
}

func NewDebugValidator() TokenValidator  { return debugValidator{} }
func NewStubValidator() TokenValidator   { return stubValidator{} }

type UpgradeHandler struct {
	hub        *Hub
	dispatcher *Dispatcher
	validator  TokenValidator
	upgrader   websocket.Upgrader
}

func NewUpgradeHandler(hub *Hub, dispatcher *Dispatcher, validator TokenValidator) *UpgradeHandler {
	return &UpgradeHandler{
		hub:        hub,
		dispatcher: dispatcher,
		validator:  validator,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *UpgradeHandler) Handle(c *gin.Context) {
	token := extractBearerToken(c.GetHeader("Authorization"))
	if token == "" {
		dto.RespondAppError(c, dto.ErrAuthInvalidIdentityToken)
		return
	}

	userID, err := h.validator.ValidateToken(token)
	if err != nil {
		dto.RespondAppError(c, err)
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("ws upgrade failed")
		return
	}

	client := &Client{
		connID:     uuid.New().String(),
		userID:     userID,
		conn:       conn,
		send:       make(chan []byte, h.hub.cfg.SendBufSize),
		done:       make(chan struct{}),
		hub:        h.hub,
		dispatcher: h.dispatcher,
	}

	h.hub.Register(client)
	go h.hub.writePump(client)
	go h.hub.readPump(client)
}

func extractBearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}
