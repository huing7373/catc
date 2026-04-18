// Package push — APNs topic router (AC5 / FR58).
//
// Each registered device token belongs to a platform ("watch" | "iphone");
// APNs requires the Notification.Topic header to match the platform's
// bundle ID. APNsRouter translates a userID into one RoutedToken per
// device by consulting the TokenProvider and mapping platform → topic.
//
// One notification per (user, token) — never multicast a single
// apns2.Notification across tokens (APNs protocol binds each Notification
// to exactly one DeviceToken).
package push

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/pkg/ids"
)

// RoutedToken packages a single target device with its resolved topic.
type RoutedToken struct {
	DeviceToken string
	Topic       string
	Platform    string
}

// APNsRouter resolves a userID into per-device routing decisions.
type APNsRouter struct {
	tokens      TokenProvider
	watchTopic  string
	iphoneTopic string
}

// NewAPNsRouter panics on nil TokenProvider — the provider must always
// exist (EmptyTokenProvider is the valid "disabled" placeholder). Topic
// strings MAY be empty at construction time for Epic 0 debug configs;
// AC15 startup validation is responsible for rejecting empty topics when
// apns.enabled = true.
func NewAPNsRouter(tokens TokenProvider, watchTopic, iphoneTopic string) *APNsRouter {
	if tokens == nil {
		panic("push.NewAPNsRouter: tokens must not be nil")
	}
	return &APNsRouter{
		tokens:      tokens,
		watchTopic:  watchTopic,
		iphoneTopic: iphoneTopic,
	}
}

// RouteTokens fans out a userID into RoutedToken entries — one per
// registered device. An unknown platform value on a TokenInfo is skipped
// with a warn (rather than routed to either bundle); this prevents an
// accidentally-corrupted or newer-schema row from pushing to the wrong
// device class while still allowing the remaining tokens to deliver.
func (r *APNsRouter) RouteTokens(ctx context.Context, userID ids.UserID) ([]RoutedToken, error) {
	infos, err := r.tokens.ListTokens(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, nil
	}

	out := make([]RoutedToken, 0, len(infos))
	for _, info := range infos {
		switch info.Platform {
		case string(ids.PlatformWatch):
			out = append(out, RoutedToken{
				DeviceToken: info.DeviceToken,
				Topic:       r.watchTopic,
				Platform:    info.Platform,
			})
		case string(ids.PlatformIphone):
			out = append(out, RoutedToken{
				DeviceToken: info.DeviceToken,
				Topic:       r.iphoneTopic,
				Platform:    info.Platform,
			})
		default:
			log.Warn().
				Str("action", "apns_route_unknown_platform").
				Str("userId", string(userID)).
				Str("platform", info.Platform).
				Msg("apns router skipped token with unknown platform")
		}
	}
	return out, nil
}
