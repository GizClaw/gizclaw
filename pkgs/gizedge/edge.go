package gizedge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

const edgeShutdownTimeout = 5 * time.Second

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
	turnRuntime, err := startTURN(cfg.TURN)
	if err != nil {
		return err
	}
	defer turnRuntime.Close()

	upstreamTransport, err := newUpstreamTransport(ctx, cfg, upstreamURL)
	if err != nil {
		return err
	}
	defer upstreamTransport.Close()

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("edge: listen public http: %w", err)
	}
	defer listener.Close()

	proxy := newPeerHTTPProxy(cfg.Endpoint, upstreamTransport)
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
		return shutdownHTTPServer(server, errCh, edgeShutdownTimeout)
	}
}

func shutdownHTTPServer(server *http.Server, errCh <-chan error, timeout time.Duration) error {
	if server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	cancel()
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	serveErr := <-errCh
	return errors.Join(shutdownErr, serveErr)
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

type upstreamTransport struct {
	ctx         context.Context
	cfg         Config
	upstreamURL *url.URL

	mu       sync.Mutex
	conn     giznet.Conn
	listener giznet.Listener
}

func newUpstreamTransport(ctx context.Context, cfg Config, upstreamURL *url.URL) (*upstreamTransport, error) {
	transport := &upstreamTransport{ctx: ctx, cfg: cfg, upstreamURL: upstreamURL}
	if _, err := transport.currentConn(); err != nil {
		return nil, err
	}
	return transport, nil
}

func (t *upstreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.roundTrip(req)
	if err == nil {
		return resp, nil
	}
	t.resetConn()
	if req.Context().Err() != nil {
		return nil, err
	}
	if !canRetryUpstreamRequest(req.Method) {
		return nil, err
	}
	return t.roundTrip(req)
}

func (t *upstreamTransport) roundTrip(req *http.Request) (*http.Response, error) {
	conn, err := t.currentConn()
	if err != nil {
		return nil, err
	}
	return gizhttp.NewRoundTripper(conn, gizclaw.ServiceEdgeHTTP).RoundTrip(req)
}

func (t *upstreamTransport) currentConn() (giznet.Conn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		return t.conn, nil
	}
	conn, listener, err := dialUpstream(t.ctx, t.cfg, t.upstreamURL)
	if err != nil {
		return nil, err
	}
	t.conn = conn
	t.listener = listener
	return conn, nil
}

func (t *upstreamTransport) resetConn() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeLocked()
}

func (t *upstreamTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closeLocked()
}

func (t *upstreamTransport) closeLocked() error {
	var errs []error
	if t.conn != nil {
		errs = append(errs, t.conn.Close())
		t.conn = nil
	}
	if t.listener != nil {
		errs = append(errs, t.listener.Close())
		t.listener = nil
	}
	return errors.Join(errs...)
}

func canRetryUpstreamRequest(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func newPeerHTTPProxy(edgeEndpoint string, transport http.RoundTripper) http.Handler {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "gizclaw"
			req.Host = "gizclaw"
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			setEdgeCORSHeaders(resp.Header)
			if resp.Request != nil && resp.Request.URL != nil && resp.Request.URL.Path == "/server-info" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return rewriteServerInfoEndpoint(resp, edgeEndpoint)
			}
			return nil
		},
	}
	return edgeCORSHandler(proxy)
}

func rewriteServerInfoEndpoint(resp *http.Response, edgeEndpoint string) error {
	if resp == nil || resp.Body == nil || edgeEndpoint == "" {
		return nil
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return err
	}
	body["endpoint"] = edgeEndpoint
	body["signaling_path"] = gizwebrtc.SignalingPath
	rewritten, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp.Body = io.NopCloser(bytes.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(rewritten)))
	resp.Header.Set("Content-Type", "application/json")
	return nil
}

func edgeCORSHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodOptions && isEdgePeerHTTPPath(req.URL.Path) {
			setEdgeCORSHeaders(w.Header())
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func setEdgeCORSHeaders(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Public-Key,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp")
	header.Set("Access-Control-Expose-Headers", "Content-Length,Content-Type")
}

func isEdgePeerHTTPPath(path string) bool {
	switch path {
	case "/login", "/server-info", "/webrtc/v1/offer", "/me", "/me/status", "/me/runtime":
		return true
	default:
		return strings.HasPrefix(path, "/openai/v1/")
	}
}

type edgeSecurityPolicy struct{}

func (edgeSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (edgeSecurityPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}
