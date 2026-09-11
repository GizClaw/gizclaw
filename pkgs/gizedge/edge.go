package gizedge

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/felixge/httpsnoop"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/monitor"
	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

const edgeShutdownTimeout = 5 * time.Second

// Serve starts the Edge HTTP ingress and optional client gateway, forwarding
// authoritative work to the configured Server over giznet.
// BuildInfo is the binary identity an Edge reports in its monitor snapshot.
type BuildInfo struct {
	Version, Commit string
}

func Serve(root string, build BuildInfo) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := PrepareWorkspaceConfig(root)
	if err != nil {
		return err
	}
	// The standalone process owns its logger even without explicit sinks.
	// Embedded ServeContext callers may instead supply their host logger.
	cfg.systemLogConfigured = true
	cfg.build = build
	return servePreparedContext(ctx, cfg)
}

func ServeContext(ctx context.Context, root string) error {
	cfg, err := PrepareWorkspaceConfig(root)
	if err != nil {
		return err
	}
	return servePreparedContext(ctx, cfg)
}

func servePreparedContext(ctx context.Context, cfg Config) (serveErr error) {
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
	handler := monitor.Handler(cfg.Monitor, monitor.Node{
		Role:      "edge",
		PublicKey: cfg.KeyPair.Public.String(),
		Version:   cfg.build.Version,
		Commit:    cfg.build.Commit,
	}, edgeIngressHandler(proxy, gateway))
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

// maxConcurrentUpstreamRequests bounds the service streams the Edge opens
// concurrently on one upstream Server association: forwarded HTTP requests and
// route-resolution RPCs share it. Every such request opens a fresh service
// DataChannel, and the Server accepts inbound DataChannels through a single
// serial loop fed by pion-sctp's accept queue, which holds 16 streams and
// silently drops the DATA of any new stream beyond that. A dropped open only
// recovers through SCTP T3 retransmission with exponential backoff. The pinned
// pion-webrtc fork reads each stream's DCEP OPEN off that loop with a
// deadline, so a lost open no longer wedges it, but an overflowing queue still
// delays opens by whole retransmission timeouts. The bound is 15 so that,
// together with the single-flight liveness probe, which bypasses it so a
// saturated bound cannot fail the probe and evict a healthy association, at
// most 16 opens are in flight: the accept queue cannot overflow regardless of
// packet timing, and stays far below the receive window provisioned for
// GatewaySCTPReceiveBufferSize. Burst tests against the earlier blocking accept
// loop showed a bound of 64 failed as badly as no bound, because slots held by
// requests waiting on retransmission starved the queued requests. Excess
// requests wait for a slot or fail with their context rather than piling onto
// SCTP.
const maxConcurrentUpstreamRequests = 15

type upstreamTransport struct {
	ctx         context.Context
	cfg         Config
	upstreamURL *url.URL
	relay       *upstreamRelaySelector
	liveness    upstreamLivenessConfig

	semOnce sync.Once
	sem     chan struct{}

	mu           sync.Mutex
	conn         giznet.Conn
	listener     giznet.Listener
	relayAttempt *upstreamRelayAttempt
	connEpoch    uint64
	closed       bool

	// lastResponse records when a forwarded request last received response
	// headers; recent traffic already proves liveness, so periodic probes skip.
	lastResponse atomic.Int64
	probeKick    chan struct{}
	monitorStop  chan struct{}
	monitorDone  chan struct{}
}

// acquireSlot reserves one concurrent-request slot on this upstream. It returns
// a release function that must be called exactly once when the forwarded
// request (including its streamed response body) is done. A canceled transport
// lifetime rejects acquisition even when a slot is free. The request context
// only bounds the wait: a free slot is always taken, so a request that is
// already canceled still reaches the round trip's stale-connection handling.
func (t *upstreamTransport) acquireSlot(ctx context.Context) (func(), error) {
	t.semOnce.Do(func() { t.sem = make(chan struct{}, maxConcurrentUpstreamRequests) })
	if ctx == nil {
		ctx = context.Background()
	}
	if t.ctx != nil {
		if err := t.ctx.Err(); err != nil {
			return nil, err
		}
	}
	select {
	case t.sem <- struct{}{}:
	default:
		select {
		case t.sem <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.contextDone():
			return nil, t.ctx.Err()
		}
	}
	var once sync.Once
	return func() { once.Do(func() { <-t.sem }) }, nil
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
	transport.startLivenessMonitor()
	return transport, nil
}

// startLivenessMonitor must run before the transport is shared.
func (t *upstreamTransport) startLivenessMonitor() {
	if t.monitorStop != nil {
		return
	}
	t.liveness = t.liveness.withDefaults()
	t.probeKick = make(chan struct{}, 1)
	t.monitorStop = make(chan struct{})
	t.monitorDone = make(chan struct{})
	go t.monitorLiveness()
}

func (t *upstreamTransport) kickLivenessProbe() {
	select {
	case t.probeKick <- struct{}{}:
	default:
	}
}

func (t *upstreamTransport) monitorContext() context.Context {
	if t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}

func (t *upstreamTransport) contextDone() <-chan struct{} {
	if t.ctx == nil {
		return nil
	}
	return t.ctx.Done()
}

func (t *upstreamTransport) monitorLiveness() {
	defer close(t.monitorDone)
	cfg := t.liveness
	backoff := cfg.backoffInitial
	timer := time.NewTimer(cfg.interval)
	defer timer.Stop()
	for {
		kicked := false
		select {
		case <-t.contextDone():
			return
		case <-t.monitorStop:
			return
		case <-timer.C:
		case <-t.probeKick:
			kicked = true
			timer.Stop()
		}
		next := cfg.interval
		conn, epoch, redial, closed := t.livenessTarget()
		switch {
		case closed:
			return
		case redial:
			// A previously connected upstream was evicted or reset. Reconnect in
			// the background so the next request finds a ready association.
			slog.Info("edge: upstream redialing",
				"upstream_kind", "control",
				"upstream_id", "control",
				"previous_connection_epoch", epoch,
			)
			if _, _, err := t.currentConn(); err != nil {
				if t.ctx != nil && t.ctx.Err() != nil {
					return
				}
				slog.Warn("edge: upstream redial failed",
					"upstream_kind", "control",
					"upstream_id", "control",
					"previous_connection_epoch", epoch,
					"retry_in", backoff.String(),
					"error", err,
				)
				next = backoff
				backoff = nextUpstreamRedialBackoff(backoff, cfg)
			} else {
				backoff = cfg.backoffInitial
			}
		case conn == nil:
			// Never connected: ordered fallbacks stay lazy until a request needs them.
		case !kicked && time.Since(time.Unix(0, t.lastResponse.Load())) < cfg.interval:
		default:
			started := time.Now()
			err := cfg.check(t.monitorContext(), conn)
			if err == nil {
				break
			}
			if t.ctx != nil && t.ctx.Err() != nil {
				return
			}
			if current, _, _, closed := t.livenessTarget(); closed || current != conn {
				// Closed or replaced while probing; the failure says nothing new.
				break
			}
			if errors.Is(err, errUpstreamSlow) {
				slog.Info("edge: upstream slow",
					"upstream_kind", "control",
					"upstream_id", "control",
					"connection_epoch", epoch,
					"trigger", livenessTrigger(kicked),
					"probe_ms", time.Since(started).Milliseconds(),
					"error", err,
				)
				break
			}
			slog.Warn("edge: upstream stalled",
				"upstream_kind", "control",
				"upstream_id", "control",
				"connection_epoch", epoch,
				"trigger", livenessTrigger(kicked),
				"probe_ms", time.Since(started).Milliseconds(),
				"last_activity", upstreamLastActivity(conn),
				"error", err,
			)
			if t.evictConn(epoch) {
				slog.Warn("edge: upstream evicted",
					"upstream_kind", "control",
					"upstream_id", "control",
					"connection_epoch", epoch,
					"reason", "liveness_probe_failed",
				)
			}
			next = 0
		}
		timer.Reset(max(next, time.Nanosecond))
	}
}

func livenessTrigger(kicked bool) string {
	if kicked {
		return "slow_request"
	}
	return "periodic"
}

func upstreamLastActivity(conn giznet.Conn) string {
	info := conn.PeerInfo()
	if info == nil || info.LastSeen.IsZero() {
		return ""
	}
	return info.LastSeen.Format(time.RFC3339Nano)
}

func (t *upstreamTransport) livenessTarget() (giznet.Conn, uint64, bool, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, 0, false, true
	}
	if t.conn == nil {
		return nil, t.connEpoch, t.connEpoch > 0, false
	}
	return t.conn, t.connEpoch, false, false
}

// evictConn closes the association of epoch after a failed liveness probe.
// Closing it fails every in-flight request on it, which RoundTrip retries on
// a fresh association when the method allows.
func (t *upstreamTransport) evictConn(epoch uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil || epoch != t.connEpoch {
		return false
	}
	t.relayAttempt.reportFailure()
	_ = t.closeLocked()
	return true
}

func (t *upstreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	release, err := t.acquireSlot(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := t.roundTripWithRetry(req)
	if err != nil {
		release()
		return nil, err
	}
	// Hold the slot until the streamed response body is closed: the upstream
	// service DataChannel stays open for the whole response, so concurrency is
	// bounded by in-flight requests, not just by RoundTrip calls in progress.
	resp.Body = &releaseReadCloser{ReadCloser: resp.Body, release: release}
	return resp, nil
}

func (t *upstreamTransport) roundTripWithRetry(req *http.Request) (*http.Response, error) {
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

// releaseReadCloser releases the upstream concurrency slot once, after the
// wrapped response body is closed.
type releaseReadCloser struct {
	io.ReadCloser
	release func()
}

func (r *releaseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.release()
	return err
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
	var stallProbe *time.Timer
	if t.probeKick != nil {
		stallProbe = time.AfterFunc(t.liveness.stallDelay, t.kickLivenessProbe)
	}
	resp, err := gizhttp.NewRoundTripper(conn, gizclaw.ServiceEdgeHTTP).RoundTrip(req)
	if stallProbe != nil {
		stallProbe.Stop()
	}
	if err == nil {
		t.lastResponse.Store(time.Now().UnixNano())
	}
	return resp, conn, epoch, err
}

func (t *upstreamTransport) currentConn() (giznet.Conn, uint64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, 0, giznet.ErrConnClosed
	}
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
	alreadyClosed := t.closed
	t.closed = true
	err := t.closeLocked()
	t.mu.Unlock()
	if t.monitorStop != nil && !alreadyClosed {
		close(t.monitorStop)
		<-t.monitorDone
	}
	return err
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
		Transport:    transport,
		ErrorHandler: writeEdgeProxyError,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		metadata := gizlog.HTTPRequestMetadata(req, false)
		requestID, err := gizlog.NewID()
		if err != nil {
			http.Error(w, "request ID generation failed", http.StatusInternalServerError)
			return
		}
		req = req.Clone(gizlog.WithRequestID(req.Context(), requestID))
		req.Header.Set("X-Request-ID", requestID)
		req.Header.Set(gizlog.ClientIPHeader, metadata.ClientIP)
		req.Header.Del(gizlog.AuthenticatedPeerHeader)
		req.Header.Del(gizlog.APIKeyNameHeader)
		completionCtx := req.Context()
		w.Header().Set("X-Request-ID", requestID)
		started := time.Now()
		status := http.StatusOK
		w = httpsnoop.Wrap(w, httpsnoop.Hooks{WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) { status = code; w.Header().Set("X-Request-ID", requestID); next(code) }
		}})
		defer func() {
			slog.InfoContext(context.WithoutCancel(completionCtx), "gizedge: HTTP request completed", "request_path", metadata.RequestPath, "client_ip", metadata.ClientIP, "user_agent", metadata.UserAgent, "method", req.Method, "status", status, "started_at", started, "ended_at", time.Now(), "duration_ms", time.Since(started).Milliseconds())
		}()
		// Capture the ingress origin before the Director rewrites the request.
		requestTransport := infoTransport
		if infoTransport != nil && req.URL.Path == "/server-info" {
			if !validSignalingAuthority(req.Host) {
				http.Error(w, "invalid request Host", http.StatusBadRequest)
				return
			}
			info := *infoTransport
			endpoint := url.URL{Scheme: "http", Host: req.Host}
			if req.TLS != nil {
				endpoint.Scheme = "https"
			}
			if configured, err := url.Parse(info.Endpoint); err == nil && configured.IsAbs() && configured.Host != "" {
				endpoint.Path = configured.Path
				endpoint.RawPath = configured.RawPath
			}
			info.Endpoint = endpoint.String()
			requestTransport = &info
		}
		requestProxy := *proxy
		requestProxy.ModifyResponse = func(resp *http.Response) error {
			completionCtx = gizlog.WithPeerPublicKey(completionCtx, resp.Header.Get(gizlog.AuthenticatedPeerHeader))
			completionCtx = gizlog.WithAPIKeyName(completionCtx, resp.Header.Get(gizlog.APIKeyNameHeader))
			resp.Header.Del(gizlog.AuthenticatedPeerHeader)
			resp.Header.Del(gizlog.APIKeyNameHeader)
			clearEdgeUpstreamCORSHeaders(resp.Header)
			if resp.Request != nil && resp.Request.URL != nil && resp.Request.URL.Path == "/server-info" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return rewriteServerInfo(resp, edgeEndpoint, requestTransport)
			}
			return nil
		}
		requestProxy.ServeHTTP(w, req)
	})
}

// validSignalingAuthority checks the authority without changing its spelling or port.
func validSignalingAuthority(authority string) bool {
	parsed, err := url.Parse("http://" + authority)
	if err != nil || parsed.Host != authority || parsed.User != nil || parsed.Path != "" || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(authority, "#%") {
		return false
	}
	if _, err := normalizeHTTPPublicEndpoint("http://" + authority); err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.HasPrefix(authority, "[") {
		return strings.Contains(host, ":") && net.ParseIP(host) != nil
	}
	if len(host) > 253 {
		return false
	}
	for label := range strings.SplitSeq(strings.TrimSuffix(host, "."), ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func writeEdgeProxyError(w http.ResponseWriter, req *http.Request, err error) {
	if _, debug, _ := peerhttp.DebugPublicKey(req.Header.Get("Authorization")); debug && peerhttp.IsDebugDataPath(req.URL.Path) {
		w.Header().Set("Cache-Control", "no-store")
	}
	status := http.StatusBadGateway
	code := "UPSTREAM_ERROR"
	switch {
	case errors.Is(err, errInvalidDebugPublicKey):
		status = http.StatusBadRequest
		code = "INVALID_REQUEST"
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
	level := slog.LevelWarn
	if req.Context().Err() != nil {
		// The client went away first; the upstream error is only a consequence.
		level = slog.LevelInfo
	}
	slog.Log(context.WithoutCancel(req.Context()), level, "gizedge: upstream proxy error",
		"request_path", req.URL.Path,
		"method", req.Method,
		"status", status,
		"client_canceled", req.Context().Err() != nil,
		"error", err,
	)
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
