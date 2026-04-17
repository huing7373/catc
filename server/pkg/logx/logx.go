package logx

import (
	"context"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Options struct {
	Level        string
	Format       string
	BuildVersion string
	ConfigHash   string
}

func Init(opts Options) {
	level := parseLevel(opts.Level)
	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	var logger zerolog.Logger
	if strings.EqualFold(opts.Format, "console") {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	log.Logger = logger.With().
		Str("buildVersion", opts.BuildVersion).
		Str("configHash", opts.ConfigHash).
		Logger()
}

func parseLevel(s string) zerolog.Level {
	if s == "" {
		return zerolog.InfoLevel
	}
	lvl, err := zerolog.ParseLevel(strings.ToLower(s))
	if err != nil {
		return zerolog.InfoLevel
	}
	return lvl
}

func Ctx(ctx context.Context) *zerolog.Logger {
	l := zerolog.Ctx(ctx)
	if l.GetLevel() == zerolog.Disabled {
		return &log.Logger
	}
	return l
}

func ctxLogger(ctx context.Context) zerolog.Logger {
	l := zerolog.Ctx(ctx)
	if l.GetLevel() == zerolog.Disabled {
		return log.Logger
	}
	return *l
}

func WithRequestID(ctx context.Context, id string) context.Context {
	logger := ctxLogger(ctx).With().Str("requestId", id).Logger()
	return logger.WithContext(ctx)
}

func WithUserID(ctx context.Context, id string) context.Context {
	logger := ctxLogger(ctx).With().Str("userId", id).Logger()
	return logger.WithContext(ctx)
}

func WithConnID(ctx context.Context, id string) context.Context {
	logger := ctxLogger(ctx).With().Str("connId", id).Logger()
	return logger.WithContext(ctx)
}
