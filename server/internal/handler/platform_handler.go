package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
)

// PlatformHandler serves the unauthenticated bootstrap platform endpoints.
// Today it exposes GET /v1/platform/ws-registry (FR59 / Story 0.14). Future
// platform-level probes (feature flags, build metadata) may land here.
type PlatformHandler struct {
	clock      clockx.Clock
	serverMode string
}

// NewPlatformHandler constructs a PlatformHandler. clock MUST be non-nil
// (M9 forbids direct time.Now; serverTime is sourced from clock.Now()).
// serverMode is the verbatim cfg.Server.Mode ("debug" or "release"); any
// non-"debug" value hides DebugOnly messages on the wire.
func NewPlatformHandler(clock clockx.Clock, serverMode string) *PlatformHandler {
	if clock == nil {
		panic("handler.NewPlatformHandler: clock is required")
	}
	return &PlatformHandler{clock: clock, serverMode: serverMode}
}

// WSRegistryResponse is the wire shape for GET /v1/platform/ws-registry.
// Additive changes are non-breaking; removing/renaming a field requires the
// Versioning Strategy bump described in Story 0.14 AC10.
type WSRegistryResponse struct {
	APIVersion string              `json:"apiVersion"`
	ServerTime string              `json:"serverTime"` // RFC3339 UTC
	Messages   []WSRegistryMessage `json:"messages"`
}

// WSRegistryMessage is one row of the registry response. DebugOnly is
// intentionally NOT surfaced — release clients never see it; debug clients
// do not need it. The release-mode filter applies before marshaling.
type WSRegistryMessage struct {
	Type          string `json:"type"`
	Version       string `json:"version"`
	Direction     string `json:"direction"`
	RequiresAuth  bool   `json:"requiresAuth"`
	RequiresDedup bool   `json:"requiresDedup"`
}

// WSRegistry serves GET /v1/platform/ws-registry. Intentionally mounted
// OUTSIDE any JWT group because clients call it pre-authentication to decide
// whether the server speaks their protocol dialect (FR59, architecture
// line 814). Future JWT middleware MUST preserve this placement.
func (h *PlatformHandler) WSRegistry(c *gin.Context) {
	msgs := make([]WSRegistryMessage, 0, len(dto.WSMessages))
	for _, meta := range dto.WSMessages {
		if meta.DebugOnly && h.serverMode != "debug" {
			continue
		}
		msgs = append(msgs, WSRegistryMessage{
			Type:          meta.Type,
			Version:       meta.Version,
			Direction:     string(meta.Direction),
			RequiresAuth:  meta.RequiresAuth,
			RequiresDedup: meta.RequiresDedup,
		})
	}

	c.JSON(http.StatusOK, WSRegistryResponse{
		APIVersion: "v1",
		ServerTime: h.clock.Now().UTC().Format(time.RFC3339),
		Messages:   msgs,
	})
}
