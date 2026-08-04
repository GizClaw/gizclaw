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

// Serve starts the Edge HTTP ingress and optional client gateway, forwarding
// authoritative work to the configured Server over giznet.
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
	relaySelector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		return fmt.Errorf("edge: prepare upstream relay selector: %w", err)
	}
	turnRuntime, err := startTURN(cfg.TURN)
	if err != nil {
		return err
	}
	defer turnRuntime.Close()

	upstreamTransport, err := newUpstreamTransport(ctx, cfg, upstreamURL, relaySelector)
	if err != nil {
		return err
	}
	defer upstreamTransport.Close()

	var gateway *Gateway
	if cfg.Gateway.Enabled {
		gateway, err = newGateway(ctx, cfg, upstreamURL, relaySelector)
		if err != nil {
			return err
		}
		defer gateway.Close()
	}

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("edge: listen public http: %w", err)
	}
	defer listener.Close()

	var transport *serverInfoTransport
	if gateway != nil {
		transport = &serverInfoTransport{
			Mode:          "edge-gateway",
			Endpoint:      cfg.Endpoint,
			PublicKey:     cfg.KeyPair.Public.String(),
			SignalingPath: gizwebrtc.SignalingPath,
		}
	}
	proxy := newPeerHTTPProxy(cfg.Endpoint, upstreamTransport, transport)
	handler := http.Handler(proxy)
	if gateway != nil {
		handler = gateway.Handler(proxy)
	}
	server := &http.Server{Handler: handler}
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

func dialUpstream(
	ctx context.Context,
	cfg Config,
	upstreamURL *url.URL,
	relaySelector *upstreamRelaySelector,
) (giznet.Conn, giznet.Listener, *upstreamRelayAttempt, error) {
	if cfg.Upstream.PublicKey.IsZero() {
		return nil, nil, nil, fmt.Errorf("edge: missing upstream.public-key")
	}
	dialCtx, cancel := context.WithTimeout(ctx, upstreamDialTimeout)
	defer cancel()
	if relaySelector != nil {
		conn, listener, attempt, err := relaySelector.dialUpstream(dialCtx, cfg, upstreamURL)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("edge: dial upstream server: %w", err)
		}
		return conn, listener, attempt, nil
	}
	listener, conn, err := gizwebrtc.Dial(dialCtx, cfg.KeyPair, cfg.Upstream.PublicKey, gizwebrtc.DialConfig{
		SignalingURL:          upstreamSignalingURL(upstreamURL),
		SecurityPolicy:        edgeSecurityPolicy{},
		SCTPReceiveBufferSize: gizwebrtc.GatewaySCTPReceiveBufferSize,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("edge: dial upstream server: %w", err)
	}
	return conn, listener, nil, nil
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
	relay       *upstreamRelaySelector

	mu           sync.Mutex
	conn         giznet.Conn
	listener     giznet.Listener
	relayAttempt *upstreamRelayAttempt
	connEpoch    uint64
}

func newUpstreamTransport(
	ctx context.Context,
	cfg Config,
	upstreamURL *url.URL,
	relay *upstreamRelaySelector,
) (*upstreamTransport, error) {
	transport := &upstreamTransport{ctx: ctx, cfg: cfg, upstreamURL: upstreamURL, relay: relay}
	if _, _, err := transport.currentConn(); err != nil {
		return nil, err
	}
	return transport, nil
}

func (t *upstreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, conn, epoch, err := t.roundTrip(req)
	if err == nil {
		return resp, nil
	}
	connectionFailed := upstreamConnectionFailed(conn, err)
	discoveryTimedOut := upstreamDiscoveryTimedOut(req, err)
	if !connectionFailed && !discoveryTimedOut {
		return nil, err
	}
	reportRelayFailure := connectionFailed && req.Context().Err() == nil && t.ctx.Err() == nil
	t.resetConn(epoch, reportRelayFailure)
	if req.Context().Err() != nil {
		return nil, err
	}
	if !canRetryUpstreamRequest(req.Method) {
		return nil, err
	}
	resp, _, _, err = t.roundTrip(req)
	return resp, err
}

func upstreamDiscoveryTimedOut(req *http.Request, err error) bool {
	return req != nil && req.URL != nil && req.URL.Path == "/server-info" &&
		errors.Is(err, context.DeadlineExceeded)
}

func (t *upstreamTransport) roundTrip(req *http.Request) (*http.Response, giznet.Conn, uint64, error) {
	conn, epoch, err := t.currentConn()
	if err != nil {
		return nil, nil, 0, err
	}
	resp, err := gizhttp.NewRoundTripper(conn, gizclaw.ServiceEdgeHTTP).RoundTrip(req)
	return resp, conn, epoch, err
}

func (t *upstreamTransport) currentConn() (giznet.Conn, uint64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		return t.conn, t.connEpoch, nil
	}
	conn, listener, relayAttempt, err := dialUpstream(t.ctx, t.cfg, t.upstreamURL, t.relay)
	if err != nil {
		return nil, 0, err
	}
	t.conn = conn
	t.listener = listener
	t.relayAttempt = relayAttempt
	t.connEpoch++
	return conn, t.connEpoch, nil
}

func upstreamConnectionFailed(conn giznet.Conn, err error) bool {
	if gizhttp.IsClosed(err) {
		return true
	}
	if conn == nil {
		return false
	}
	info := conn.PeerInfo()
	return info != nil && info.State == giznet.PeerStateOffline
}

func (t *upstreamTransport) resetConn(epoch uint64, reportRelayFailure bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if epoch == 0 || epoch != t.connEpoch {
		return
	}
	if reportRelayFailure {
		t.relayAttempt.reportFailure()
	}
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
	t.relayAttempt = nil
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

type serverInfoTransport struct {
	Mode          string `json:"mode"`
	Endpoint      string `json:"endpoint"`
	PublicKey     string `json:"public_key"`
	SignalingPath string `json:"signaling_path"`
}

func newPeerHTTPProxy(edgeEndpoint string, transport http.RoundTripper, gatewayTransport ...*serverInfoTransport) http.Handler {
	var infoTransport *serverInfoTransport
	if len(gatewayTransport) > 0 {
		infoTransport = gatewayTransport[0]
	}
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
				return rewriteServerInfo(resp, edgeEndpoint, infoTransport)
			}
			return nil
		},
	}
	return edgeCORSHandler(proxy)
}

func rewriteServerInfoEndpoint(resp *http.Response, edgeEndpoint string) error {
	return rewriteServerInfo(resp, edgeEndpoint, nil)
}

func rewriteServerInfo(resp *http.Response, edgeEndpoint string, transport *serverInfoTransport) error {
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
	if transport != nil {
		body["transport"] = transport
		body["ice"] = map[string]bool{"udp": true, "tcp": false}
		delete(body, "ice_servers")
	}
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
	header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Public-Key,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp")
	header.Set("Access-Control-Expose-Headers", "Content-Length,Content-Type,X-GizClaw-Gateway-Upstream")
}

func isEdgePeerHTTPPath(path string) bool {
	if strings.HasPrefix(path, "/me/side-control/") || strings.HasPrefix(path, "/side-control/") {
		return true
	}
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
