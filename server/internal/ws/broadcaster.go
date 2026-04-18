package ws

import (
	"context"

	"github.com/rs/zerolog/log"
)

type ConnID = string
type UserID = string
type RoomID = string

type Broadcaster interface {
	BroadcastToUser(ctx context.Context, userID UserID, msg []byte) error
	BroadcastToRoom(ctx context.Context, roomID RoomID, msg []byte) error
	PushOnConnect(ctx context.Context, connID ConnID, userID UserID) error
	BroadcastDiff(ctx context.Context, userID UserID, diff []byte) error
}

type InMemoryBroadcaster struct {
	hub *Hub
}

func NewInMemoryBroadcaster(hub *Hub) *InMemoryBroadcaster {
	return &InMemoryBroadcaster{hub: hub}
}

func (b *InMemoryBroadcaster) BroadcastToUser(_ context.Context, userID UserID, msg []byte) error {
	clients := b.hub.FindByUser(userID)
	for _, c := range clients {
		if !c.trySend(msg) {
			b.hub.unregisterClient(c)
		}
	}
	return nil
}

func (b *InMemoryBroadcaster) BroadcastToRoom(ctx context.Context, roomID RoomID, _ []byte) error {
	log.Ctx(ctx).Warn().Str("room_id", roomID).Msg("BroadcastToRoom not implemented (MVP)")
	return nil
}

func (b *InMemoryBroadcaster) PushOnConnect(ctx context.Context, connID ConnID, userID UserID) error {
	log.Ctx(ctx).Debug().Str("conn_id", connID).Str("user_id", userID).Msg("PushOnConnect no-op (D6 预留)")
	return nil
}

func (b *InMemoryBroadcaster) BroadcastDiff(ctx context.Context, userID UserID, _ []byte) error {
	log.Ctx(ctx).Debug().Str("user_id", userID).Msg("BroadcastDiff no-op (D6 预留)")
	return nil
}

var _ Broadcaster = (*InMemoryBroadcaster)(nil)
