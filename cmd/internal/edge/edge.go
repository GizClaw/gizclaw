package edge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"syscall"
)

// Serve starts an experimental edge-node HTTP ingress and forwards requests to
// the configured upstream server HTTP endpoint.
func Serve(root string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ServeContext(ctx, root)
}

func ServeContext(ctx context.Context, root string) error {
	cfg, err := PrepareWorkspaceConfig(root)
	if err != nil {
		return err
	}
	upstreamURL, err := cfg.UpstreamURL()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("edge: listen public http: %w", err)
	}
	defer listener.Close()

	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	server := &http.Server{Handler: proxy}
	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownErr := server.Shutdown(context.Background())
		serveErr := <-errCh
		return errors.Join(shutdownErr, serveErr)
	}
}
