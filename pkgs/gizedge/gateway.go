package gizedge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

var ErrGatewayOverCapacity = errors.New("edge: gateway over capacity")

const (
	gatewayPoolWarmUpstreams       = 4
	gatewayPoolWarmupTimeout       = 5 * time.Second
	gatewayPoolReplenishRetryDelay = time.Second
	gatewaySessionHandshakeTimeout = 10 * time.Second
	gatewaySessionEstablishTimeout = 30 * time.Second
	gatewaySessionMaxAttempts      = 2
	retiredEdgeTunnelServiceID     = 0x32
	// The first bounded set of admitted client associations gets enough receive
	// credit for a 1 MiB burst without exposing the 32 MiB upstream profile to
	// every public peer. This caps burst-profile receive credit at 256 MiB per
	// Edge.
	gatewayClientSCTPReceiveBufferSize = 4 * 1024 * 1024
	gatewayClientBurstSCTPLimit        = 64
)

type gatewayAdmissionContextKey struct{}

type Gateway struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	listener *gizwebrtc.Listener
	pool     *gatewayPool

	capacityMu sync.Mutex
	pending    int
	active     int
	burstSCTP  int

	admissionMu     sync.Mutex
	admissions      map[giznet.PublicKey][]*gatewayAdmission
	admissionNotify chan struct{}

	sessionMu  sync.Mutex
	sessions   map[*gatewaySession]struct{}
	sessionWG  sync.WaitGroup
	acceptDone chan struct{}
	closeOnce  sync.Once
}

type gatewayAdmission struct {
	gateway         *Gateway
	state           atomic.Int32
	timer           *time.Timer
	clientKey       giznet.PublicKey
	remoteAddr      string
	upstream        *gatewayUpstream
	releaseUpstream func()
	burstSCTP       bool
}

type gatewaySession struct {
	client  giznet.Conn
	logical *giztunnel.Conn
}

func newGateway(
	parent context.Context,
	cfg Config,
	upstreamURL *url.URL,
	relaySelector *upstreamRelaySelector,
) (*Gateway, error) {
	ctx, cancel := context.WithCancel(parent)
	gateway := &Gateway{
		ctx:             ctx,
		cancel:          cancel,
		cfg:             cfg,
		admissions:      make(map[giznet.PublicKey][]*gatewayAdmission),
		admissionNotify: make(chan struct{}),
		sessions:        make(map[*gatewaySession]struct{}),
		acceptDone:      make(chan struct{}),
	}
	listener, err := (&gizwebrtc.ListenConfig{
		ICEUDPAddr:                   cfg.Listen,
		PublicICEUDPAddr:             publicGatewayICEAddr(cfg.Endpoint),
		ICELite:                      true,
		SecurityPolicy:               gatewayClientSecurityPolicy{},
		AggregateServices:            true,
		GatewaySCTPPeer:              gateway.allowBurstSCTP,
		GatewaySCTPReceiveBufferSize: gatewayClientSCTPReceiveBufferSize,
	}).Listen(cfg.KeyPair)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("edge: start gateway listener: %w", err)
	}
	pool := newGatewayPool(ctx, cfg, upstreamURL, relaySelector)
	startupCtx, startupCancel := context.WithTimeout(ctx, upstreamDialTimeout)
	defer startupCancel()
	if err := retryGatewayStartupRelay(startupCtx, pool.ensureOne); err != nil {
		_ = listener.Close()
		cancel()
		return nil, err
	}
	if err := retryGatewayStartupRelay(startupCtx, pool.warm); err != nil {
		_ = pool.Close()
		_ = listener.Close()
		cancel()
		return nil, err
	}
	gateway.listener = listener
	gateway.pool = pool
	go gateway.acceptLoop()
	return gateway, nil
}

func publicGatewayICEAddr(endpoint string) string {
	host, _, err := netSplitNumericHostPort("endpoint", endpoint)
	if err != nil {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		return ""
	}
	return endpoint
}

func retryGatewayStartupRelay(ctx context.Context, operation func(context.Context) error) error {
	for {
		err := operation(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		var unavailable *upstreamRelaysUnavailableError
		if errors.Is(err, context.DeadlineExceeded) {
			continue
		}
		if !errors.As(err, &unavailable) || unavailable.retryAfter <= 0 {
			return err
		}
		timer := time.NewTimer(unavailable.retryAfter)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return errors.Join(err, ctx.Err())
		}
	}
}

func (g *Gateway) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != gizwebrtc.SignalingPath || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		g.serveSignaling(w, r)
	})
}

func (g *Gateway) serveSignaling(w http.ResponseWriter, r *http.Request) {
	var clientKey giznet.PublicKey
	if err := clientKey.UnmarshalText([]byte(r.Header.Get("X-Giznet-Public-Key"))); err != nil ||
		clientKey.IsZero() {
		writeGatewaySignalingError(w, http.StatusBadRequest, "invalid_public_key", false)
		return
	}
	admission, err := g.reserveAdmission()
	if err != nil {
		writeGatewaySignalingError(w, http.StatusServiceUnavailable, "gateway_over_capacity", true)
		return
	}
	admission.clientKey = clientKey
	admission.remoteAddr = r.RemoteAddr
	r = r.WithContext(context.WithValue(r.Context(), gatewayAdmissionContextKey{}, admission))
	entry, release, err := g.pool.acquire(r.Context())
	if err != nil {
		admission.releasePending()
		writeGatewaySignalingError(w, http.StatusServiceUnavailable, "gateway_over_capacity", true)
		return
	}
	admission.upstream = entry
	admission.releaseUpstream = release
	w.Header().Set("X-GizClaw-Gateway-Upstream", fmt.Sprintf("%d", entry.id))
	recorder := &gatewayStatusWriter{ResponseWriter: w}
	g.listener.SignalingHandler().ServeHTTP(recorder, r)
	if recorder.status < http.StatusOK || recorder.status >= http.StatusMultipleChoices {
		admission.releasePending()
		return
	}
	admission.timer = time.AfterFunc(30*time.Second, admission.releasePending)
	if !g.enqueueAdmission(admission) {
		admission.releasePending()
	}
}

func writeGatewaySignalingError(w http.ResponseWriter, status int, name string, retry bool) {
	w.Header().Set("Content-Type", "application/json")
	if retry {
		w.Header().Set("Retry-After", "1")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": name})
}

func (g *Gateway) reserveAdmission() (*gatewayAdmission, error) {
	if g == nil {
		return nil, ErrGatewayOverCapacity
	}
	g.capacityMu.Lock()
	defer g.capacityMu.Unlock()
	if g.ctx.Err() != nil ||
		g.pending >= g.cfg.Gateway.MaxPendingHandshakes ||
		g.pending+g.active >= g.cfg.Gateway.MaxSessions ||
		!g.pool.canAccept() {
		return nil, ErrGatewayOverCapacity
	}
	g.pending++
	admission := &gatewayAdmission{gateway: g}
	if g.burstSCTP < gatewayClientBurstSCTPLimit {
		g.burstSCTP++
		admission.burstSCTP = true
	}
	return admission, nil
}

func (g *Gateway) allowBurstSCTP(ctx context.Context, publicKey giznet.PublicKey) bool {
	admission, _ := ctx.Value(gatewayAdmissionContextKey{}).(*gatewayAdmission)
	return admission != nil && admission.gateway == g && admission.burstSCTP &&
		admission.clientKey == publicKey
}

func (a *gatewayAdmission) releaseBurstSCTPLocked() {
	if !a.burstSCTP {
		return
	}
	a.burstSCTP = false
	if a.gateway.burstSCTP > 0 {
		a.gateway.burstSCTP--
	}
}

func (a *gatewayAdmission) releasePending() {
	if a == nil || a.gateway == nil || !a.state.CompareAndSwap(0, 2) {
		return
	}
	a.gateway.capacityMu.Lock()
	if a.gateway.pending > 0 {
		a.gateway.pending--
	}
	a.releaseBurstSCTPLocked()
	a.gateway.capacityMu.Unlock()
	a.releasePool()
	a.gateway.removeAdmission(a)
}

func (a *gatewayAdmission) promote() bool {
	if a == nil || a.gateway == nil || !a.state.CompareAndSwap(0, 1) {
		return false
	}
	if a.timer != nil {
		a.timer.Stop()
	}
	a.gateway.capacityMu.Lock()
	if a.gateway.pending > 0 {
		a.gateway.pending--
	}
	a.gateway.active++
	a.gateway.capacityMu.Unlock()
	return true
}

func (a *gatewayAdmission) releaseActive() {
	if a == nil || a.gateway == nil || !a.state.CompareAndSwap(1, 2) {
		return
	}
	a.gateway.capacityMu.Lock()
	if a.gateway.active > 0 {
		a.gateway.active--
	}
	a.releaseBurstSCTPLocked()
	a.gateway.capacityMu.Unlock()
	a.releasePool()
}

func (a *gatewayAdmission) releasePool() {
	if a == nil || a.releaseUpstream == nil {
		return
	}
	release := a.releaseUpstream
	a.releaseUpstream = nil
	release()
}

func (a *gatewayAdmission) setUpstream(entry *gatewayUpstream, release func()) {
	a.upstream = entry
	a.releaseUpstream = release
}

func (g *Gateway) enqueueAdmission(admission *gatewayAdmission) bool {
	if g == nil || admission == nil || admission.clientKey.IsZero() ||
		admission.state.Load() != 0 || g.ctx.Err() != nil {
		return false
	}
	g.admissionMu.Lock()
	defer g.admissionMu.Unlock()
	if admission.state.Load() != 0 || g.ctx.Err() != nil {
		return false
	}
	g.admissions[admission.clientKey] = append(g.admissions[admission.clientKey], admission)
	g.signalAdmissionLocked()
	return true
}

func (g *Gateway) removeAdmission(target *gatewayAdmission) {
	if g == nil || target == nil || target.clientKey.IsZero() {
		return
	}
	g.admissionMu.Lock()
	queue := g.admissions[target.clientKey]
	for index, admission := range queue {
		if admission == target {
			queue = append(queue[:index], queue[index+1:]...)
			break
		}
	}
	if len(queue) == 0 {
		delete(g.admissions, target.clientKey)
	} else {
		g.admissions[target.clientKey] = queue
	}
	g.signalAdmissionLocked()
	g.admissionMu.Unlock()
}

func (g *Gateway) signalAdmissionLocked() {
	close(g.admissionNotify)
	g.admissionNotify = make(chan struct{})
}

func (g *Gateway) claimAdmission(clientKey giznet.PublicKey) (*gatewayAdmission, error) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		g.admissionMu.Lock()
		queue := g.admissions[clientKey]
		var admission *gatewayAdmission
		if len(queue) > 0 {
			admission = queue[0]
			queue = queue[1:]
			if len(queue) == 0 {
				delete(g.admissions, clientKey)
			} else {
				g.admissions[clientKey] = queue
			}
		}
		notify := g.admissionNotify
		g.admissionMu.Unlock()
		if admission != nil {
			if admission.promote() {
				return admission, nil
			}
			continue
		}
		select {
		case <-notify:
		case <-g.ctx.Done():
			return nil, g.ctx.Err()
		case <-timer.C:
			return nil, context.DeadlineExceeded
		}
	}
}

func (g *Gateway) acceptLoop() {
	defer close(g.acceptDone)
	for {
		client, err := g.listener.Accept()
		if err != nil {
			return
		}
		admission, err := g.claimAdmission(client.PublicKey())
		if err != nil {
			_ = client.Close()
			if g.ctx.Err() != nil {
				return
			}
			continue
		}
		g.sessionWG.Go(func() {
			defer admission.releaseActive()
			g.handleClient(client, admission)
		})
	}
}

func (g *Gateway) handleClient(client giznet.Conn, admission *gatewayAdmission) {
	if admission == nil {
		_ = client.Close()
		return
	}
	establishCtx, cancel := context.WithTimeout(g.ctx, gatewaySessionEstablishTimeout)
	defer cancel()
	for attempt := range gatewaySessionMaxAttempts {
		entry := admission.upstream
		if entry == nil {
			break
		}
		logical, sessionID, retry := g.openLogicalSession(establishCtx, client, admission, entry)
		if logical != nil {
			if attempt > 0 {
				slog.InfoContext(g.ctx, "gateway logical session alternate succeeded",
					"entry_id", entry.id,
					"attempt", attempt+1,
				)
			}
			g.bridgeLogicalSession(client, logical, entry.id, sessionID)
			return
		}
		if !retry || attempt+1 >= gatewaySessionMaxAttempts || establishCtx.Err() != nil {
			break
		}
		admission.releasePool()
		alternate, release, err := g.pool.acquire(establishCtx)
		if err != nil {
			break
		}
		admission.setUpstream(alternate, release)
		slog.InfoContext(g.ctx, "gateway logical session retry",
			"from_entry_id", entry.id,
			"to_entry_id", alternate.id,
			"attempt", attempt+2,
		)
	}
	_ = client.Close()
}

func (g *Gateway) openLogicalSession(
	ctx context.Context,
	client giznet.Conn,
	admission *gatewayAdmission,
	entry *gatewayUpstream,
) (*giztunnel.Conn, string, bool) {
	if entry == nil || entry.router == nil {
		return nil, "", false
	}
	sessionID, err := giztunnel.NewSessionID()
	if err != nil {
		return nil, "", false
	}
	handshakeCtx, cancelHandshake, completeHandshakeBudget := boundedAttemptContext(
		ctx,
		gatewaySessionHandshakeTimeout,
	)
	handshakeStarted := time.Now()
	slog.InfoContext(g.ctx, peerStreamLifecycleMessage,
		"component", "edge_gateway",
		"stage", "session_establishing",
		"result", "success",
		"tunnel_session_id", sessionID.String(),
		"peer_public_key", client.PublicKey().String(),
		"entry_id", entry.id,
	)
	logical, err := entry.router.Dial(
		handshakeCtx,
		giztunnel.SessionDeclaration{
			SessionID:       sessionID,
			ClientPublicKey: client.PublicKey(),
			RemoteAddr:      admission.remoteAddr,
		},
	)
	cancelHandshake()
	if err != nil {
		reason := preSessionHandshakeFailureCategory(err, completeHandshakeBudget)
		slog.InfoContext(g.ctx, "gateway logical session establishment failed",
			"entry_id", entry.id,
			"reason", reason,
		)
		slog.InfoContext(g.ctx, peerStreamLifecycleMessage,
			"component", "edge_gateway",
			"stage", "terminal",
			"result", gatewaySessionEstablishmentResult(err, completeHandshakeBudget),
			"reason", reason,
			"last_stage", "session_establishing",
			"duration_ms", time.Since(handshakeStarted).Milliseconds(),
			"tunnel_session_id", sessionID.String(),
			"peer_public_key", client.PublicKey().String(),
			"entry_id", entry.id,
		)
		return nil, sessionID.String(), g.classifySessionHandshakeFailure(ctx, entry, err, completeHandshakeBudget)
	}
	slog.InfoContext(g.ctx, peerStreamLifecycleMessage,
		"component", "edge_gateway",
		"stage", "session_accepted",
		"result", "success",
		"tunnel_session_id", sessionID.String(),
		"peer_public_key", client.PublicKey().String(),
		"entry_id", entry.id,
		"duration_ms", time.Since(handshakeStarted).Milliseconds(),
	)
	return logical, sessionID.String(), false
}

func preSessionHandshakeFailureCategory(err error, completeBudget bool) string {
	switch {
	case errors.Is(err, giztunnel.ErrSessionRejected):
		return "session_rejected"
	case errors.Is(err, giznet.ErrConnClosed):
		return "terminal_connection_failure"
	case isPreAcceptStreamClose(err):
		return "session_stream_closed"
	case completeBudget && isPreAcceptHandshakeTimeout(err):
		return "session_handshake_timeout"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "establishment_context_done"
	default:
		return "session_handshake_error"
	}
}

func boundedAttemptContext(
	ctx context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc, bool) {
	deadline := time.Now().Add(timeout)
	completeBudget := true
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
		completeBudget = false
	}
	attemptCtx, cancel := context.WithDeadline(ctx, deadline)
	return attemptCtx, cancel, completeBudget
}

func (g *Gateway) classifySessionHandshakeFailure(
	ctx context.Context,
	entry *gatewayUpstream,
	err error,
	completeBudget bool,
) bool {
	if ctx.Err() != nil || g.ctx.Err() != nil {
		return false
	}
	info := entry.conn.PeerInfo()
	if errors.Is(err, giznet.ErrConnClosed) || info != nil && info.State == giznet.PeerStateOffline {
		entry.pool.markFailed(entry, "terminal_connection_failure", true)
		return true
	}
	if isPreAcceptStreamClose(err) {
		entry.pool.markDraining(entry, "session_stream_closed")
		return true
	}
	if errors.Is(err, gizwebrtc.ErrServiceOpen) {
		entry.pool.markDraining(entry, "native_channel_open_error")
		return true
	}
	if completeBudget && isPreAcceptHandshakeTimeout(err) {
		entry.pool.markDraining(entry, "session_handshake_timeout")
		return true
	}
	return false
}

func isPreAcceptStreamClose(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.ErrClosedPipe)
}

func isPreAcceptHandshakeTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (g *Gateway) bridgeLogicalSession(client giznet.Conn, logical *giztunnel.Conn, entryID uint64, sessionID string) {
	started := time.Now()
	slog.InfoContext(g.ctx, peerStreamLifecycleMessage,
		"component", "edge_gateway",
		"stage", "bridge_started",
		"result", "success",
		"tunnel_session_id", sessionID,
		"peer_public_key", client.PublicKey().String(),
		"entry_id", entryID,
	)
	session := &gatewaySession{client: client, logical: logical}
	g.addSession(session)
	defer g.removeSession(session)
	done := make(chan struct{})
	go g.enforceIdle(session, done)
	err := giztunnel.Bridge(client, logical)
	close(done)
	result, reason := gatewayBridgeLifecycleResult(err)
	slog.InfoContext(g.ctx, peerStreamLifecycleMessage,
		"component", "edge_gateway",
		"stage", "terminal",
		"result", result,
		"reason", reason,
		"last_stage", "bridge_started",
		"duration_ms", time.Since(started).Milliseconds(),
		"tunnel_session_id", sessionID,
		"peer_public_key", client.PublicKey().String(),
		"entry_id", entryID,
	)
}

const peerStreamLifecycleMessage = "gizclaw: peer stream lifecycle"

func gatewayBridgeLifecycleResult(err error) (string, string) {
	switch {
	case err == nil:
		return "success", "completed"
	case errors.Is(err, context.Canceled):
		return "canceled", "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout", "deadline_exceeded"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.ErrClosedPipe), errors.Is(err, giznet.ErrConnClosed):
		return "closed", "transport_closed"
	default:
		return "transport_error", "bridge_error"
	}
}

func gatewaySessionEstablishmentResult(err error, completeBudget bool) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), completeBudget && isPreAcceptHandshakeTimeout(err):
		return "timeout"
	case isPreAcceptStreamClose(err), errors.Is(err, giznet.ErrConnClosed):
		return "closed"
	default:
		return "transport_error"
	}
}

func (g *Gateway) enforceIdle(session *gatewaySession, done <-chan struct{}) {
	interval := min(g.cfg.Gateway.IdleTimeout/2, 30*time.Second)
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if time.Since(session.logical.LastActivity()) >= g.cfg.Gateway.IdleTimeout {
				_ = session.client.Close()
				_ = session.logical.Close()
				return
			}
		case <-done:
			return
		case <-g.ctx.Done():
			return
		}
	}
}

func (g *Gateway) addSession(session *gatewaySession) {
	g.sessionMu.Lock()
	g.sessions[session] = struct{}{}
	g.sessionMu.Unlock()
}

func (g *Gateway) removeSession(session *gatewaySession) {
	g.sessionMu.Lock()
	delete(g.sessions, session)
	g.sessionMu.Unlock()
	_ = session.close()
}

func (g *Gateway) closeSessions() {
	g.sessionMu.Lock()
	sessions := make([]*gatewaySession, 0, len(g.sessions))
	for session := range g.sessions {
		sessions = append(sessions, session)
	}
	g.sessionMu.Unlock()
	var closeWG sync.WaitGroup
	for _, session := range sessions {
		closeWG.Go(func() { _ = session.close() })
	}
	closeWG.Wait()
}

func (s *gatewaySession) close() error {
	if s == nil {
		return nil
	}
	var err error
	if s.client != nil {
		err = errors.Join(err, s.client.Close())
	}
	if s.logical != nil {
		err = errors.Join(err, s.logical.Close())
	}
	return err
}

func (g *Gateway) Close() error {
	if g == nil {
		return nil
	}
	var closeErr error
	g.closeOnce.Do(func() {
		g.cancel()
		closeErr = errors.Join(closeErr, g.listener.Close())
		<-g.acceptDone
		g.releaseAdmissions()
		drained := make(chan struct{})
		go func() {
			g.sessionWG.Wait()
			close(drained)
		}()
		timer := time.NewTimer(g.cfg.Gateway.DrainTimeout)
		select {
		case <-drained:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			g.closeSessions()
			<-drained
		}
		closeErr = errors.Join(closeErr, g.pool.Close())
	})
	return closeErr
}

func (g *Gateway) releaseAdmissions() {
	g.admissionMu.Lock()
	var admissions []*gatewayAdmission
	for _, queue := range g.admissions {
		admissions = append(admissions, queue...)
	}
	g.admissions = make(map[giznet.PublicKey][]*gatewayAdmission)
	g.signalAdmissionLocked()
	g.admissionMu.Unlock()
	for _, admission := range admissions {
		admission.releasePending()
	}
}

type gatewayStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *gatewayStatusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gatewayStatusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

type gatewayClientSecurityPolicy struct{}

func (gatewayClientSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (gatewayClientSecurityPolicy) AllowService(_ giznet.PublicKey, service uint64) bool {
	switch service {
	case gizclaw.ServicePeerRPC,
		gizclaw.ServicePeerHTTP,
		gizclaw.ServicePeerOpenAI,
		gizclaw.ServiceAdminHTTP,
		gizclaw.EventStreamAgent:
		return true
	default:
		return false
	}
}

type gatewayPool struct {
	ctx         context.Context
	cfg         Config
	upstreamURL *url.URL
	relay       *upstreamRelaySelector
	newUpstream func(context.Context) (*gatewayUpstream, error)

	mu         sync.Mutex
	entries    []*gatewayUpstream
	nextID     uint64
	growthDone chan struct{}
	closed     bool
}

type gatewayUpstream struct {
	id           uint64
	pool         *gatewayPool
	conn         giznet.Conn
	listener     giznet.Listener
	router       *giztunnel.Router
	relayAttempt *upstreamRelayAttempt
	icePair      *gizwebrtc.ICECandidatePairObservation

	active       int
	state        gatewayUpstreamState
	peakChannels atomic.Int64

	closing   atomic.Bool
	closeOnce sync.Once
}

type gatewayUpstreamState uint8

const (
	gatewayUpstreamSelectable gatewayUpstreamState = iota
	gatewayUpstreamDraining
	gatewayUpstreamFailed
)

func newGatewayPool(
	ctx context.Context,
	cfg Config,
	upstreamURL *url.URL,
	relay *upstreamRelaySelector,
) *gatewayPool {
	return &gatewayPool{ctx: ctx, cfg: cfg, upstreamURL: upstreamURL, relay: relay}
}

func (p *gatewayPool) ensureOne(ctx context.Context) error {
	entry, err := p.dial(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed || p.contextErr() != nil {
		p.mu.Unlock()
		_ = entry.close()
		return giznet.ErrConnClosed
	}
	p.nextID++
	entry.id = p.nextID
	p.entries = append(p.entries, entry)
	p.mu.Unlock()
	logGatewayUpstreamICE(entry)
	return nil
}

func (p *gatewayPool) warm(ctx context.Context) error {
	p.mu.Lock()
	target := p.warmTarget()
	remaining := min(target-p.selectableCountLocked(), p.cfg.Gateway.MaxUpstreams-len(p.entries))
	p.mu.Unlock()
	if remaining <= 0 {
		return nil
	}

	warmCtx, cancel := context.WithTimeout(ctx, gatewayPoolWarmupTimeout)
	defer cancel()
	errs := make(chan error, remaining)
	var wg sync.WaitGroup
	for range remaining {
		wg.Go(func() {
			entry, err := p.dial(warmCtx)
			if err != nil {
				errs <- fmt.Errorf("edge: warm gateway upstream: %w", err)
				return
			}
			p.mu.Lock()
			if p.closed || p.contextErr() != nil ||
				p.selectableCountLocked() >= target || len(p.entries) >= p.cfg.Gateway.MaxUpstreams {
				p.mu.Unlock()
				_ = entry.close()
				return
			}
			p.nextID++
			entry.id = p.nextID
			p.entries = append(p.entries, entry)
			p.mu.Unlock()
			logGatewayUpstreamICE(entry)
		})
	}
	wg.Wait()
	close(errs)
	var err error
	for warmErr := range errs {
		err = errors.Join(err, warmErr)
	}
	return err
}

func (p *gatewayPool) warmTarget() int {
	return min(p.cfg.Gateway.MaxUpstreams, gatewayPoolWarmUpstreams)
}

func (p *gatewayPool) reserveWarmGrowthLocked() chan struct{} {
	if p.ctx == nil || p.closed || p.contextErr() != nil ||
		p.selectableCountLocked() >= p.warmTarget() ||
		len(p.entries) >= p.cfg.Gateway.MaxUpstreams || p.growthDone != nil {
		return nil
	}
	done := make(chan struct{})
	p.growthDone = done
	return done
}

func (p *gatewayPool) replenishWarm(done chan struct{}) {
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		p.mu.Lock()
		if p.growthDone != done || p.closed || p.contextErr() != nil {
			if p.growthDone == done {
				close(done)
				p.growthDone = nil
			}
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()

		growCtx, cancel := context.WithTimeout(ctx, gatewayPoolWarmupTimeout)
		entry, err := p.dial(growCtx)
		cancel()

		p.mu.Lock()
		if p.growthDone != done {
			p.mu.Unlock()
			if entry != nil {
				_ = entry.close()
			}
			return
		}
		if p.closed || p.contextErr() != nil {
			close(done)
			p.growthDone = nil
			p.mu.Unlock()
			if entry != nil {
				_ = entry.close()
			}
			return
		}
		if err == nil {
			if p.selectableCountLocked() >= p.warmTarget() ||
				len(p.entries) >= p.cfg.Gateway.MaxUpstreams {
				close(done)
				p.growthDone = nil
				p.mu.Unlock()
				_ = entry.close()
				return
			}
			p.nextID++
			entry.id = p.nextID
			p.entries = append(p.entries, entry)
			needsMore := p.selectableCountLocked() < p.warmTarget() && len(p.entries) < p.cfg.Gateway.MaxUpstreams
			close(done)
			if needsMore {
				done = make(chan struct{})
				p.growthDone = done
			} else {
				p.growthDone = nil
			}
			p.mu.Unlock()
			logGatewayUpstreamICE(entry)
			if needsMore {
				continue
			}
			return
		}
		p.mu.Unlock()

		timer := time.NewTimer(gatewayPoolReplenishRetryDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}
}

func (p *gatewayPool) canAccept() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.contextErr() != nil {
		return false
	}
	for _, entry := range p.entries {
		if entry.state == gatewayUpstreamSelectable &&
			entry.active < p.cfg.Gateway.SessionsPerUpstream &&
			entry.hasChannelCapacity(p.cfg.Gateway.ChannelsPerUpstream) {
			return true
		}
	}
	return len(p.entries) < p.cfg.Gateway.MaxUpstreams
}

func (p *gatewayPool) acquire(ctx context.Context) (*gatewayUpstream, func(), error) {
	for {
		p.mu.Lock()
		if p.closed || p.contextErr() != nil {
			p.mu.Unlock()
			return nil, nil, giznet.ErrConnClosed
		}
		var selected *gatewayUpstream
		for _, entry := range p.entries {
			if entry.state != gatewayUpstreamSelectable ||
				entry.active >= p.cfg.Gateway.SessionsPerUpstream ||
				!entry.hasChannelCapacity(p.cfg.Gateway.ChannelsPerUpstream) {
				continue
			}
			if selected == nil || entry.active < selected.active {
				selected = entry
			}
		}
		// MaxUpstreams is a capacity ceiling, not a warm-pool target. Reuse a
		// healthy association until its configured session capacity is reached;
		// eagerly opening associations gives every cold SCTP path its own small
		// congestion window and makes modest bursts slower and more expensive.
		if selected != nil {
			selected.active++
			growthDone := p.reserveWarmGrowthLocked()
			p.mu.Unlock()
			if growthDone != nil {
				go p.replenishWarm(growthDone)
			}
			var once sync.Once
			return selected, func() {
				once.Do(func() { p.release(selected) })
			}, nil
		}
		if len(p.entries) >= p.cfg.Gateway.MaxUpstreams {
			p.mu.Unlock()
			return nil, nil, ErrGatewayOverCapacity
		}
		if p.growthDone != nil {
			growthDone := p.growthDone
			p.mu.Unlock()
			select {
			case <-growthDone:
				continue
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-p.contextDone():
				return nil, nil, giznet.ErrConnClosed
			}
		}
		growthDone := make(chan struct{})
		p.growthDone = growthDone
		p.mu.Unlock()
		dialCtx, cancelDial := p.dialContext(ctx)
		entry, err := p.dial(dialCtx)
		cancelDial()
		p.mu.Lock()
		if p.growthDone == growthDone {
			close(growthDone)
			p.growthDone = nil
		}
		if err != nil {
			p.mu.Unlock()
			return nil, nil, err
		}
		if p.closed || p.contextErr() != nil {
			p.mu.Unlock()
			_ = entry.close()
			return nil, nil, giznet.ErrConnClosed
		}
		p.nextID++
		entry.id = p.nextID
		p.entries = append(p.entries, entry)
		p.mu.Unlock()
		logGatewayUpstreamICE(entry)
	}
}

func logGatewayUpstreamICE(entry *gatewayUpstream) {
	if entry == nil {
		return
	}
	logUpstreamICE("gateway", fmt.Sprintf("%d", entry.id), entry.id, entry.relayAttempt, entry.icePair)
}

func logUpstreamICE(
	kind string,
	id string,
	epoch uint64,
	relayAttempt *upstreamRelayAttempt,
	pair *gizwebrtc.ICECandidatePairObservation,
) {
	if pair == nil {
		slog.Warn("edge: upstream ICE observation unavailable",
			"upstream_kind", kind,
			"upstream_id", id,
			"connection_epoch", epoch,
		)
		return
	}
	attrs := []any{
		"upstream_kind", kind,
		"upstream_id", id,
		"connection_epoch", epoch,
		"local_candidate_type", pair.Local.Type,
		"local_protocol", pair.Local.Protocol,
		"local_address_family", pair.Local.AddressFamily,
		"local_component", pair.Local.Component,
		"remote_candidate_type", pair.Remote.Type,
		"remote_protocol", pair.Remote.Protocol,
		"remote_address_family", pair.Remote.AddressFamily,
		"remote_component", pair.Remote.Component,
		"pair_state", pair.State,
		"nominated", pair.Nominated,
		"counters_supported", pair.CountersSupported,
		"packets_sent", pair.PacketsSent,
		"packets_received", pair.PacketsReceived,
		"bytes_sent", pair.BytesSent,
		"bytes_received", pair.BytesReceived,
		"current_rtt_seconds", pair.CurrentRoundTripTime,
		"requests_sent", pair.RequestsSent,
		"responses_received", pair.ResponsesReceived,
		"retransmissions_sent", pair.RetransmissionsSent,
		"retransmissions_received", pair.RetransmissionsReceived,
		"packets_discarded_on_send", pair.PacketsDiscardedOnSend,
		"bytes_discarded_on_send", pair.BytesDiscardedOnSend,
	}
	if relayAttempt != nil {
		attrs = append(attrs, "relay_member", relayAttempt.member)
	}
	slog.Info("edge: upstream ICE selected", attrs...)
}

func (p *gatewayPool) dialContext(ctx context.Context) (context.Context, func()) {
	if p == nil || p.ctx == nil {
		return ctx, func() {}
	}
	dialCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(p.ctx, cancel)
	return dialCtx, func() {
		stop()
		cancel()
	}
}

func (p *gatewayPool) contextDone() <-chan struct{} {
	if p == nil || p.ctx == nil {
		return nil
	}
	return p.ctx.Done()
}

func (p *gatewayPool) contextErr() error {
	if p == nil || p.ctx == nil {
		return nil
	}
	return p.ctx.Err()
}

func (p *gatewayPool) dial(ctx context.Context) (*gatewayUpstream, error) {
	if p.newUpstream != nil {
		entry, err := p.newUpstream(ctx)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, errors.New("edge: nil gateway upstream")
		}
		entry.pool = p
		return entry, nil
	}
	conn, listener, relayAttempt, icePair, err := dialUpstream(ctx, p.cfg, p.upstreamURL, p.relay)
	if err != nil {
		return nil, err
	}
	webrtcConn, ok := conn.(*gizwebrtc.Conn)
	if !ok {
		_ = conn.Close()
		_ = listener.Close()
		return nil, errors.New("edge: gateway upstream is not WebRTC")
	}
	entry := &gatewayUpstream{
		pool:         p,
		conn:         conn,
		listener:     listener,
		relayAttempt: relayAttempt,
		icePair:      icePair,
	}
	router, err := giztunnel.NewRouter(webrtcConn, giztunnel.Config{
		MaxChannelsPerSession: p.cfg.Gateway.ChannelsPerSession,
		MaxChannels:           p.cfg.Gateway.ChannelsPerUpstream,
		MaxPendingSessions:    p.cfg.Gateway.MaxPendingHandshakes,
		MaxBufferedBytes:      p.cfg.Gateway.SessionBufferBytes,
		HandshakeTimeout:      gatewaySessionHandshakeTimeout,
		AggregateServices:     true,
		AllowRemoteService: func(_ giznet.PublicKey, service uint64) bool {
			return service != retiredEdgeTunnelServiceID
		},
		OnActiveChannels: entry.observeActiveChannels,
	})
	if err != nil {
		_ = conn.Close()
		_ = listener.Close()
		return nil, fmt.Errorf("edge: create gateway tunnel router: %w", err)
	}
	entry.router = router
	go entry.readPackets()
	return entry, nil
}

func (p *gatewayPool) release(entry *gatewayUpstream) {
	p.mu.Lock()
	if entry.active > 0 {
		entry.active--
	}
	closeEntry := entry.state != gatewayUpstreamSelectable && entry.active == 0
	if closeEntry {
		p.removeLocked(entry)
	}
	growthDone := p.reserveWarmGrowthLocked()
	p.mu.Unlock()
	if closeEntry {
		_ = entry.close()
	}
	if growthDone != nil {
		go p.replenishWarm(growthDone)
	}
}

func (p *gatewayPool) markDraining(entry *gatewayUpstream, reason string) bool {
	p.mu.Lock()
	if entry.state != gatewayUpstreamSelectable {
		p.mu.Unlock()
		return false
	}
	entry.state = gatewayUpstreamDraining
	closeEntry := entry.active == 0
	if closeEntry {
		p.removeLocked(entry)
	}
	growthDone := p.reserveWarmGrowthLocked()
	p.mu.Unlock()
	p.logTransition(entry, "draining", reason)
	if closeEntry {
		_ = entry.close()
	}
	if growthDone != nil {
		go p.replenishWarm(growthDone)
	}
	return true
}

func (p *gatewayPool) markFailed(entry *gatewayUpstream, reason string, relayFailure bool) bool {
	p.mu.Lock()
	if entry.state == gatewayUpstreamFailed {
		p.mu.Unlock()
		return false
	}
	entry.state = gatewayUpstreamFailed
	p.removeLocked(entry)
	growthDone := p.reserveWarmGrowthLocked()
	p.mu.Unlock()
	if relayFailure && !entry.closing.Load() && p.contextErr() == nil {
		entry.relayAttempt.reportFailure()
	}
	p.logTransition(entry, "failed", reason)
	_ = entry.close()
	if growthDone != nil {
		go p.replenishWarm(growthDone)
	}
	return true
}

func (p *gatewayPool) selectableCountLocked() int {
	count := 0
	for _, entry := range p.entries {
		if entry.state == gatewayUpstreamSelectable {
			count++
		}
	}
	return count
}

func (p *gatewayPool) logTransition(entry *gatewayUpstream, state, reason string) {
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	slog.InfoContext(ctx, "gateway upstream transition",
		"entry_id", entry.id,
		"state", state,
		"reason", reason,
	)
}

func (p *gatewayPool) removeLocked(target *gatewayUpstream) {
	for i, entry := range p.entries {
		if entry == target {
			p.entries = append(p.entries[:i], p.entries[i+1:]...)
			return
		}
	}
}

func (p *gatewayPool) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	entries := append([]*gatewayUpstream(nil), p.entries...)
	p.entries = nil
	if p.growthDone != nil {
		close(p.growthDone)
		p.growthDone = nil
	}
	p.mu.Unlock()
	errs := make([]error, len(entries))
	var closeWG sync.WaitGroup
	for i, entry := range entries {
		closeWG.Go(func() { errs[i] = entry.close() })
	}
	closeWG.Wait()
	return errors.Join(errs...)
}

func (e *gatewayUpstream) readPackets() {
	buf := make([]byte, 64*1024)
	for {
		protocol, n, err := e.conn.Read(buf)
		if err != nil {
			if e.closing.Load() || e.pool.contextErr() != nil {
				return
			}
			e.pool.markFailed(e, "terminal_packet_read", true)
			return
		}
		if protocol != giznet.ProtocolTunnelPacket {
			continue
		}
		if err := e.router.HandlePacket(buf[:n]); err != nil &&
			!errors.Is(err, giztunnel.ErrSessionNotFound) {
			continue
		}
	}
}

func (e *gatewayUpstream) hasChannelCapacity(limit int) bool {
	if e == nil {
		return false
	}
	if limit < 3 {
		return true
	}
	// active includes acquisitions that have not reached Router.Dial yet. Keep
	// three persistent slots for every such session so concurrent admissions
	// cannot all observe the same stale Router.ActiveChannels value.
	claimed := e.active * 3
	if e.router != nil {
		claimed = max(claimed, e.router.ActiveChannels())
	}
	return claimed <= limit-3
}

func (e *gatewayUpstream) observeActiveChannels(active int) {
	if e == nil {
		return
	}
	peak := e.peakChannels.Load()
	for int64(active) > peak && !e.peakChannels.CompareAndSwap(peak, int64(active)) {
		peak = e.peakChannels.Load()
	}
	newPeak := int64(active) > peak
	limit := 0
	if e.pool != nil {
		limit = e.pool.cfg.Gateway.ChannelsPerUpstream
	}
	if active == 0 || newPeak && (active == limit || active%1024 == 0) {
		slog.Info("edge: gateway tunnel channel usage",
			"entry_id", e.id,
			"active_channels", active,
			"peak_active_channels", e.peakChannels.Load(),
		)
	}
}

func (e *gatewayUpstream) close() error {
	if e == nil {
		return nil
	}
	var err error
	e.closeOnce.Do(func() {
		e.closing.Store(true)
		if e.router != nil {
			err = errors.Join(err, e.router.Close())
		}
		if e.conn != nil {
			err = errors.Join(err, e.conn.Close())
		}
		if e.listener != nil {
			err = errors.Join(err, e.listener.Close())
		}
	})
	return err
}

var _ io.Writer = (*gatewayStatusWriter)(nil)
