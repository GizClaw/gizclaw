package gizrun

import (
	"log/slog"
	"os"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/log"
	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/log/sink"
	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/log/volclog"
)

func init() {
	PostInitAt(0x11, func(ctx *RunContext) error {
		ctx.logHandlers = append(ctx.logHandlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: &ctx.logLevel}))

		var volcConfig volcLogConfig
		if section, ok := ctx.config("volc_log"); ok {
			value, ok := section.(volcLogConfig)
			if ok && value.Enabled {
				volcConfig = value.withLevel(&ctx.logLevel).expandEnv()
			}
		} else if ctx.volcLog.Enabled {
			volcConfig = ctx.volcLog.withLevel(&ctx.logLevel).expandEnv()
		}
		if volcConfig.Enabled {
			handler, err := volclog.NewHandler(volclog.Config{
				Endpoint:         volcConfig.Endpoint,
				Region:           volcConfig.Region,
				AccessKeyID:      volcConfig.AccessKeyID,
				AccessKeySecret:  volcConfig.AccessKeySecret,
				SecurityToken:    volcConfig.SecurityToken,
				APIKey:           volcConfig.APIKey,
				TopicID:          volcConfig.TopicID,
				Source:           volcConfig.Source,
				FileName:         volcConfig.FileName,
				ShardHash:        volcConfig.ShardHash,
				Level:            volcConfig.Level,
				AddSource:        volcConfig.AddSource,
				EnableNanosecond: volcConfig.EnableNanosecond,
			})
			if err != nil {
				return err
			}
			ctx.volcLogHandler = handler
			ctx.logHandlers = append(ctx.logHandlers, handler)
		}

		var handler slog.Handler
		if len(ctx.logHandlers) == 1 {
			handler = ctx.logHandlers[0]
		} else {
			handler = log.NewFanoutHandler(ctx.logHandlers...)
		}
		return sink.Start(sink.NewHandlerSink(handler), nil)
	})
}
