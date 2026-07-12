package gizedge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
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

	proxy := newPeerHTTPProxy(upstreamTransport)
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
		HTTPClient:     newPrivateIngressHTTPClient(cfg, upstreamURL),
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

func upstreamLoginURL(upstreamURL *url.URL) string {
	next := *upstreamURL
	next.Path = "/login"
	next.RawQuery = ""
	next.Fragment = ""
	return next.String()
}

func newPrivateIngressHTTPClient(cfg Config, upstreamURL *url.URL) *http.Client {
	return &http.Client{
		Transport: &privateIngressRoundTripper{
			base:     http.DefaultTransport,
			cfg:      cfg,
			loginURL: upstreamLoginURL(upstreamURL),
		},
	}
}

type privateIngressRoundTripper struct {
	base     http.RoundTripper
	cfg      Config
	loginURL string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func (t *privateIngressRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.sessionToken(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := t.roundTripWithToken(req, token)
	if err != nil || !isPrivateIngressUnauthorized(resp.StatusCode) {
		return resp, err
	}
	t.clearSessionToken(token)
	if !canReplayPrivateIngressRequest(req) {
		return resp, nil
	}
	_ = resp.Body.Close()
	token, err = t.sessionToken(req.Context())
	if err != nil {
		return nil, err
	}
	return t.roundTripWithToken(req, token)
}

func (t *privateIngressRoundTripper) roundTripWithToken(req *http.Request, token string) (*http.Response, error) {
	next, err := clonePrivateIngressRequest(req)
	if err != nil {
		return nil, err
	}
	next.Header.Set("Authorization", "Bearer "+token)
	next.Header.Set(publiclogin.PublicKeyHeader, t.cfg.KeyPair.Public.String())
	return t.transport().RoundTrip(next)
}

func clonePrivateIngressRequest(req *http.Request) (*http.Request, error) {
	next := req.Clone(req.Context())
	next.Header = req.Header.Clone()
	if req.Body != nil && req.Body != http.NoBody && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		next.Body = body
	}
	return next, nil
}

func canReplayPrivateIngressRequest(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

func isPrivateIngressUnauthorized(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

func (t *privateIngressRoundTripper) clearSessionToken(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token == token {
		t.token = ""
		t.expiresAt = time.Time{}
	}
}

func (t *privateIngressRoundTripper) sessionToken(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && time.Until(t.expiresAt) > time.Minute {
		return t.token, nil
	}
	assertion, err := publiclogin.NewLoginAssertion(t.cfg.KeyPair, t.cfg.Upstream.PublicKey, time.Minute)
	if err != nil {
		return "", fmt.Errorf("edge: create upstream login assertion: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.loginURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+assertion)
	req.Header.Set(publiclogin.PublicKeyHeader, t.cfg.KeyPair.Public.String())
	resp, err := t.transport().RoundTrip(req)
	if err != nil {
		return "", fmt.Errorf("edge: login upstream private ingress: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("edge: login upstream private ingress: %s", resp.Status)
	}
	var result publiclogin.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("edge: decode upstream login response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("edge: upstream login response missing access token")
	}
	t.token = result.AccessToken
	t.expiresAt = time.UnixMilli(result.ExpiresAt)
	return t.token, nil
}

func (t *privateIngressRoundTripper) transport() http.RoundTripper {
	if t.base != nil {
		return t.base
	}
	return http.DefaultTransport
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
	if req.Context().Err() != nil {
		return nil, err
	}
	t.resetConn()
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

func newPeerHTTPProxy(transport http.RoundTripper) http.Handler {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "gizclaw"
			req.Host = "gizclaw"
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			setEdgeCORSHeaders(resp.Header)
			return nil
		},
	}
	return edgeCORSHandler(proxy)
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
