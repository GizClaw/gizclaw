package gizrun

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/metrics"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
)

func init() {
	PostInitAt(0x30, func(ctx *RunContext) error {
		ctx.debugApp.Get("/metrics", adaptor.HTTPHandler(metrics.Handler()))
		if ctx.enablePprof {
			mux := http.NewServeMux()
			mux.HandleFunc("/debug/pprof/", pprof.Index)
			mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
			ctx.debugApp.Use("/debug/pprof", adaptor.HTTPHandler(mux))
		}
		addr, err := normalizeDebugPort(ctx.debugPort)
		if err != nil {
			return err
		}
		if addr == "" {
			return nil
		}
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("gizrun: listen debug Fiber app %s: %w", addr, err)
		}
		addr = listener.Addr().String()
		ctx.debugListening = true
		slog.Info("gizrun debug Fiber app listening", "addr", addr)
		go func() {
			if err := ctx.debugApp.Listener(listener); err != nil && !errors.Is(err, net.ErrClosed) {
				slog.Error("gizrun debug Fiber app failed", "error", err)
			}
		}()
		return nil
	})
}
