package gizrun

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

func init() {
	PostInitAt(0x31, func(ctx *RunContext) error {
		addr := strings.TrimSpace(ctx.addr)
		if addr == "" {
			return nil
		}
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("gizrun: listen Fiber app %s: %w", addr, err)
		}
		ctx.addr = listener.Addr().String()
		ctx.publicListening = true
		slog.Info("gizrun public Fiber app listening", "addr", ctx.addr)
		go func() {
			if err := ctx.publicApp.Listener(listener); err != nil && !errors.Is(err, net.ErrClosed) {
				slog.Error("gizrun public Fiber app failed", "error", err)
			}
		}()
		return nil
	})
}
