package gizrun

import (
	"context"
	"log/slog"
	"os"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/log/sink"
)

func init() {
	ExitAt(0x20, func(ctx *RunContext) error {
		_ = sink.Stop(context.Background())
		if ctx.volcLogHandler != nil {
			_ = ctx.volcLogHandler.Close()
			ctx.volcLogHandler = nil
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		return nil
	})
}
