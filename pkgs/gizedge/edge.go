package gizedge

import (
	"bytes"
	"context"
	"crypto/tls"
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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

const edgeShutdownTimeout = 5 * time.Second

// Serve starts the Edge HTTP ingress and optional client gateway, forwarding
// authoritative work to the configured Server over giznet.
func Serve(root string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ServeContext(ctx, root)
}

func ServeContext(ctx context.Context, root string) (serveErr error) {
	cfg, err := PrepareWorkspaceConfig(root)
	if err != nil {
		return err
	}
	closeLogging, err := installConfiguredEdgeLogging(cfg)
	if err != nil {
		return fmt.Errorf("edge: configure system log: %w", err)
	}
	defer func() {
		serveErr = errors.Join(serveErr, closeLogging())
	}()
	shutdownMetrics, metricsStore, err := installEdgeMetrics(cfg.Metrics)
	if err != nil {
		return fmt.Errorf("edge: configure metrics: %w", err)
	}
	if shutdownMetrics != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), edgeShutdownTimeout)
			defer cancel()
			serveErr = errors.Join(serveErr, shutdownMetrics(shutdownCtx), metricsStore.Close())
		}()
	}
	_, err = cfg.configuredUpstreams()
	if err != nil {
		return err
	}
	turnRuntime, err := startTURN(cfg.TURN)
	if err != nil {
		return err
	}
	defer turnRuntime.Close()

	upstreamTransport, err := newOrderedUpstreamTransport(ctx, cfg)
	if err != nil {
		return err
	}
	defer upstreamTransport.Close()

	var gateway *Gateway
	if cfg.Gateway.Enabled {
		gateway, err = newGateway(ctx, cfg)
		if err != nil {
			return err
		}
		gateway.resolvePeerRoute = upstreamTransport.resolvePeerAssignment
		defer gateway.Close()
	}

	var transport *serverInfoTransport
	if gateway != nil {
		transport = &serverInfoTransport{
			Mode:          "edge-gateway",
			Endpoint:      cfg.publicHTTPEndpoint(),
			PublicKey:     cfg.KeyPair.Public.String(),
			SignalingPath: gizwebrtc.SignalingPath,
		}
	}
	proxy := newPeerHTTPProxy(cfg.WebRTC.Endpoint, upstreamTransport, transport)
	handler := edgeIngressHandler(proxy, gateway)
	httpRuntime, err := startEdgeHTTP(cfg.HTTP.Listeners, handler)
	if err != nil {
		return err
	}

	select {
	case err := <-httpRuntime.errCh:
		httpRuntime.errCh <- err
		return httpRuntime.shutdown(edgeShutdownTimeout)
	case <-ctx.Done():
		return httpRuntime.shutdown(edgeShutdownTimeout)
	}
}

func installConfiguredEdgeLogging(cfg Config) (func() error, error) {
	if !cfg.systemLogConfigured {
		return func() error { return nil }, nil
	}
	return installEdgeLogging(cfg)
}

func installEdgeLogging(cfg Config) (func() error, error) {
	physical, err := storage.New(cfg.Storage)
	if err != nil {
		return nil, err
	}
	logical, err := store.New(cfg.Stores, physical)
	if err != nil {
		return nil, errors.Join(err, physical.Close())
	}
	closeLogger, err := gizlog.InstallDefault(cfg.SystemLog, logical)
	if err != nil {
		return nil, errors.Join(err, logical.Close(), physical.Close())
	}
	return func() error {
		return errors.Join(closeLogger(), logical.Close(), physical.Close())
	}, nil
}

type edgeHTTPRuntime struct {
	servers   []*http.Server
	listeners []net.Listener
	errCh     chan error
}

func startEdgeHTTP(configs []HTTPListenerConfig, handler http.Handler) (*edgeHTTPRuntime, error) {
	type preparedListener struct {
		server   *http.Server
		listener net.Listener
	}
	prepared := make([]preparedListener, 0, len(configs))
	for index, cfg := range configs {
		listener, err := net.Listen("tcp", cfg.Listen)
		if err != nil {
			for _, item := range prepared {
				_ = item.listener.Close()
			}
			return nil, fmt.Errorf("edge: listen http.listeners[%d]: %w", index, err)
		}
		tlsConfig, err := cfg.TLS.tlsConfig(fmt.Sprintf("http.listeners[%d].tls", index))
		if err != nil {
			_ = listener.Close()
			for _, item := range prepared {
				_ = item.listener.Close()
			}
			return nil, err
		}
		if tlsConfig != nil {
			listener = tls.NewListener(listener, tlsConfig)
		}
		prepared = append(prepared, preparedListener{server: &http.Server{Handler: handler}, listener: listener})
	}
	runtime := &edgeHTTPRuntime{
		servers: make([]*http.Server, 0, len(prepared)), listeners: make([]net.Listener, 0, len(prepared)),
		errCh: make(chan error, len(prepared)),
	}
	for _, item := range prepared {
		runtime.servers = append(runtime.servers, item.server)
		runtime.listeners = append(runtime.listeners, item.listener)
		go func(server *http.Server, listener net.Listener) {
			err := server.Serve(listener)
			if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
				err = nil
			}
			runtime.errCh <- err
		}(item.server, item.listener)
	}
	return runtime, nil
}

func (r *edgeHTTPRuntime) shutdown(timeout time.Duration) error {
	if r == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var errs []error
	for _, server := range r.servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err, server.Close())
		}
	}
	for range r.servers {
		errs = append(errs, <-r.errCh)
	}
	return errors.Join(errs...)
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
) (giznet.Conn, giznet.Listener, *upstreamRelayAttempt, *gizwebrtc.ICECandidatePairObservation, error) {
	if cfg.selectedUpstream.PublicKey.IsZero() {
		return nil, nil, nil, nil, fmt.Errorf("edge: missing upstream.public-key")
	}
	dialCtx, cancel := context.WithTimeout(ctx, upstreamDialTimeout)
	defer cancel()
	if relaySelector != nil {
		conn, listener, attempt, observation, err := relaySelector.dialUpstream(dialCtx, cfg, upstreamURL)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("edge: dial upstream server: %w", err)
		}
		return conn, listener, attempt, observation, nil
	}
	var timing gizwebrtc.DialTiming
	listener, conn, err := gizwebrtc.Dial(dialCtx, cfg.KeyPair, cfg.selectedUpstream.PublicKey, gizwebrtc.DialConfig{
		MetricsNodeRole:       "edge",
		SignalingURL:          upstreamSignalingURL(upstreamURL),
		SecurityPolicy:        edgeSecurityPolicy{},
		SCTPReceiveBufferSize: gizwebrtc.GatewaySCTPReceiveBufferSize,
		OnTiming: func(observation gizwebrtc.DialTiming) {
			timing = observation
		},
	})
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("edge: dial upstream server: %w", err)
	}
	return conn, listener, nil, timing.SelectedCandidatePair, nil
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
	conn, listener, relayAttempt, observation, err := dialUpstream(t.ctx, t.cfg, t.upstreamURL, t.relay)
	if err != nil {
		return nil, 0, err
	}
	t.conn = conn
	t.listener = listener
	t.relayAttempt = relayAttempt
	t.connEpoch++
	logUpstreamICE("control", "control", t.connEpoch, relayAttempt, observation)
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
			clearEdgeUpstreamCORSHeaders(resp.Header)
			if resp.Request != nil && resp.Request.URL != nil && resp.Request.URL.Path == "/server-info" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return rewriteServerInfo(resp, edgeEndpoint, infoTransport)
			}
			return nil
		},
		ErrorHandler: writeEdgeProxyError,
	}
	return proxy
}

func writeEdgeProxyError(w http.ResponseWriter, _ *http.Request, err error) {
	status := http.StatusBadGateway
	code := "UPSTREAM_ERROR"
	switch {
	case errors.Is(err, errAPIKeyUnauthorized):
		status = http.StatusUnauthorized
		code = "INVALID_API_KEY"
		w.Header().Set("WWW-Authenticate", "Bearer")
	case errors.Is(err, errAPIKeyOwnerUnavailable):
		status = http.StatusForbidden
		code = "API_KEY_OWNER_UNAVAILABLE"
	case errors.Is(err, errAPIKeyTargetUnconfigured), errors.Is(err, errAPIKeyTargetUnavailable):
		status = http.StatusServiceUnavailable
		code = "API_KEY_SERVER_UNAVAILABLE"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apitypes.NewErrorResponse(code, http.StatusText(status)))
}

func edgeIngressHandler(next http.Handler, gateway *Gateway) http.Handler {
	if gateway != nil {
		next = gateway.Handler(next)
	}
	return edgeCORSHandler(next)
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
		setEdgeCORSHeaders(w.Header(), req.Header.Get("Origin"))
		if req.Method == http.MethodOptions && isEdgePeerHTTPPath(req.URL.Path) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func clearEdgeUpstreamCORSHeaders(header http.Header) {
	for name := range header {
		if strings.HasPrefix(strings.ToLower(name), "access-control-") {
			delete(header, name)
		}
	}
}

func setEdgeCORSHeaders(header http.Header, origin string) {
	setEdgeCORSOrigin(header, origin)
	header.Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp,X-Request-ID")
	header.Set("Access-Control-Expose-Headers", "Content-Length,Content-Type,X-GizClaw-Gateway-Upstream,X-Request-ID")
}

func setEdgeCORSOrigin(header http.Header, origin string) {
	if origin == "" {
		header.Set("Access-Control-Allow-Origin", "*")
		return
	}
	header.Set("Access-Control-Allow-Origin", origin)
	for _, value := range header.Values("Vary") {
		for token := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "Origin") || strings.TrimSpace(token) == "*" {
				return
			}
		}
	}
	header.Add("Vary", "Origin")
}

func isEdgePeerHTTPPath(path string) bool {
	switch path {
	case "/server-info", "/webrtc/v1/offer":
		return true
	default:
		return strings.HasPrefix(path, "/gizclaw/v1/") || strings.HasPrefix(path, "/openai/v1/")
	}
}

type edgeSecurityPolicy struct{}

func (edgeSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (edgeSecurityPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}
