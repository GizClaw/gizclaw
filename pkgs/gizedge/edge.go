package gizedge

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
	httpListener := listener
	var publicMux *publicTCPMux
	relayCtx, stopRelays := context.WithCancel(ctx)
	defer stopRelays()
	if cfg.Upstream.ICE != "" {
		publicMux = newPublicTCPMux(listener)
		defer publicMux.Close()
		httpListener = publicMux.HTTPListener()
	} else {
		defer listener.Close()
	}
	var tcpRelay *tcpICERelay
	var udpRelay *udpICERelay
	if publicMux != nil {
		tcpRelay = newTCPICERelay(publicMux.ICETCPListener(), cfg.Upstream.ICE)
		udpRelay, err = newUDPICERelay(cfg.Listen, cfg.Upstream.ICE)
		if err != nil {
			return err
		}
	}

	proxy := newPeerHTTPProxy(upstreamConn)
	server := &http.Server{Handler: proxy}
	errCh := make(chan error, 3)
	go func() {
		err := server.Serve(httpListener)
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			err = nil
		}
		errCh <- err
	}()
	if tcpRelay != nil {
		go func() {
			errCh <- tcpRelay.Serve(relayCtx)
		}()
	}
	if udpRelay != nil {
		go func() {
			errCh <- udpRelay.Serve(relayCtx)
		}()
	}

	select {
	case err := <-errCh:
		stopRelays()
		_ = server.Shutdown(context.Background())
		return err
	case <-ctx.Done():
		stopRelays()
		shutdownErr := server.Shutdown(context.Background())
		var serveErr error
		for i := 0; i < cap(errCh); i++ {
			select {
			case err := <-errCh:
				serveErr = errors.Join(serveErr, err)
			default:
			}
		}
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
