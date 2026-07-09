package edge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

// Serve starts an experimental edge-node HTTP ingress and forwards requests to
// the configured upstream server through a giznet service stream.
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
	upstreamConn, upstreamListener, err := dialUpstream(ctx, cfg, upstreamURL)
	if err != nil {
		return err
	}
	defer upstreamConn.Close()
	defer upstreamListener.Close()

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("edge: listen public http: %w", err)
	}
	defer listener.Close()

	proxy := newPeerHTTPProxy(upstreamConn)
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

func dialUpstream(ctx context.Context, cfg Config, upstreamURL *url.URL) (giznet.Conn, giznet.Listener, error) {
	if cfg.Upstream.PublicKey.IsZero() {
		return nil, nil, fmt.Errorf("edge: missing upstream.public-key")
	}
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	listener, conn, err := gizwebrtc.Dial(dialCtx, cfg.KeyPair, cfg.Upstream.PublicKey, gizwebrtc.DialConfig{
		SignalingURL:   upstreamSignalingURL(upstreamURL),
		SecurityPolicy: edgeSecurityPolicy{},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("edge: dial upstream server: %w", err)
	}
	return conn, listener, nil
}

func upstreamSignalingURL(upstreamURL *url.URL) string {
	next := *upstreamURL
	if next.Path == "" || next.Path == "/" {
		next.Path = gizwebrtc.SignalingPath
	}
	return next.String()
}

func newPeerHTTPProxy(conn giznet.Conn) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "gizclaw"
			req.Host = "gizclaw"
		},
		Transport: gizhttp.NewRoundTripper(conn, gizclaw.ServicePeerHTTP),
	}
}

type edgeSecurityPolicy struct{}

func (edgeSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (edgeSecurityPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}
