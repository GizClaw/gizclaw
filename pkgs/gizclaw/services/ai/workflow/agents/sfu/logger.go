package sfu

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/livekit/protocol/livekit"
	protoLogger "github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/logger/zaputil"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

var livekitLoggerOnce sync.Once

// The SDK's internal Pion factory uses its package logger. Configure it once
// before constructing any Room, with a bridge that follows slog.Default.
func installLiveKitLogger() {
	livekitLoggerOnce.Do(func() { lksdk.SetLogger(newLiveKitLogger(context.Background(), nil)) })
}

type livekitLogger struct {
	ctx    context.Context
	logger *slog.Logger
	name   string
	depth  int
	attrs  []slog.Attr
}

var _ protoLogger.Logger = livekitLogger{}

func newLiveKitLogger(ctx context.Context, logger *slog.Logger) protoLogger.Logger {
	return livekitLogger{ctx: context.WithoutCancel(ctx), logger: logger}
}

func (l livekitLogger) Debugw(msg string, values ...any) {
	l.write(slog.LevelDebug, msg, nil, values)
}

func (l livekitLogger) Infow(msg string, values ...any) {
	l.write(slog.LevelInfo, msg, nil, values)
}

func (l livekitLogger) Warnw(msg string, err error, values ...any) {
	l.write(slog.LevelWarn, msg, err, values)
}

func (l livekitLogger) Errorw(msg string, err error, values ...any) {
	l.write(slog.LevelError, msg, err, values)
}

func (l livekitLogger) write(level slog.Level, msg string, err error, values []any) {
	logger := l.logger
	if logger == nil {
		logger = slog.Default()
	}
	if !logger.Enabled(l.ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3+l.depth, pcs[:])
	record := slog.NewRecord(time.Now(), level, msg, pcs[0])
	record.AddAttrs(slog.String("sdk", "livekit"))
	if l.name != "" {
		record.AddAttrs(slog.String("component", l.name))
	}
	record.AddAttrs(l.attrs...)
	record.AddAttrs(livekitLogAttrs(values)...)
	if err != nil {
		// Provider error bodies can contain URLs or credentials.
		record.AddAttrs(slog.Bool("error_present", true))
	}
	_ = logger.Handler().Handle(l.ctx, record)
}

// Only transport/state enums are copied from SDK fields. Raw SDP, ICE servers,
// candidates, HTTP responses and provider errors never become log attributes.
func livekitLogAttrs(values []any) []slog.Attr {
	var attrs []slog.Attr
	for i := 0; i+1 < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			continue
		}
		switch key {
		case "transport":
			if value, ok := values[i+1].(livekit.SignalTarget); ok {
				switch value {
				case livekit.SignalTarget_PUBLISHER, livekit.SignalTarget_SUBSCRIBER:
					attrs = append(attrs, slog.String(key, value.String()))
				}
			}
		case "state", "from", "to":
			if value, ok := values[i+1].(string); ok {
				switch strings.ToLower(value) {
				case "initial", "connecting", "connected", "disconnected", "resuming", "reconnecting", "closed", "new", "checking", "completed", "failed":
					attrs = append(attrs, slog.String(key, value))
				}
			}
		}
	}
	return attrs
}

func (l livekitLogger) WithValues(values ...any) protoLogger.Logger {
	l.attrs = append(append([]slog.Attr(nil), l.attrs...), livekitLogAttrs(values)...)
	return l
}

func (l livekitLogger) WithUnlikelyValues(values ...any) protoLogger.UnlikelyLogger {
	return protoLogger.NewUnlikelyLogger(l, values)
}

func (l livekitLogger) WithName(name string) protoLogger.Logger {
	if l.name == "" {
		l.name = name
	} else if name != "" {
		l.name += "." + name
	}
	return l
}

func (l livekitLogger) WithComponent(component string) protoLogger.Logger {
	return l.WithName(component)
}

func (l livekitLogger) WithCallDepth(depth int) protoLogger.Logger {
	l.depth += depth
	return l
}

func (l livekitLogger) WithItemSampler() protoLogger.Logger { return l }

func (l livekitLogger) WithoutSampler() protoLogger.Logger { return l }

func (l livekitLogger) WithDeferredValues() (protoLogger.Logger, protoLogger.DeferredFieldResolver) {
	return l, zaputil.NoOpDeferrer{}
}
