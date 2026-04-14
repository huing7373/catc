// Package logx wraps zerolog with project-wide conventions:
//   - One call to Init at startup configures the global logger.
//   - request_id is carried through context; log.Ctx(ctx) inherits it.
//   - user_id is added after authentication via WithUserID.
//
// Business code obtains a logger via zerolog's log.Ctx(ctx) and emits
// structured fields (never formatted strings).
package logx

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Config is the subset of the overall config that logx needs. Mirroring
// the schema in internal/config without importing internal keeps pkg/
// independent.
type Config struct {
	Level  string // "debug" | "info" | "warn" | "error"
	Format string // "json" | "console"
}

// Init configures zerolog's global logger. It must be called once, early
// in initialize, before any other logging occurs.
func Init(cfg Config) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "timestamp"

	lvl := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(lvl)

	var out io.Writer = os.Stdout
	if strings.EqualFold(cfg.Format, "console") {
		out = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	}
	log.Logger = zerolog.New(out).With().Timestamp().Logger()
}

func parseLevel(name string) zerolog.Level {
	switch strings.ToLower(name) {
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

// ctxKey is an unexported type to avoid collision with other packages'
// context keys.
type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota + 1
	ctxKeyUserID
)

// ContextWithRequestID attaches a request_id to ctx and returns the new
// context. It also returns a derived context carrying a zerolog logger
// with the request_id field pre-populated, so log.Ctx(ctx) inherits it.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyRequestID, id)
	l := log.With().Str("request_id", id).Logger()
	return l.WithContext(ctx)
}

// RequestIDFromContext reads the request_id from ctx. Returns "" if none.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// WithUserID adds user_id to the zerolog logger bound to ctx. Callers
// typically invoke this after auth succeeds so subsequent log.Ctx(ctx)
// includes the user field.
func WithUserID(ctx context.Context, userID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyUserID, userID)
	l := log.Ctx(ctx).With().Str("user_id", userID).Logger()
	return l.WithContext(ctx)
}

// UserIDFromContext reads the user_id previously attached by WithUserID.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyUserID).(string); ok {
		return v
	}
	return ""
}
