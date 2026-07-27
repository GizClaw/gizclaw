package gizedge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type Gateway struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	listener *gizwebrtc.Listener
	pool     *gatewayPool

	capacityMu sync.Mutex
	pending    int
	active     int

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
}

type gatewaySession struct {
	client  giznet.Conn
	logical *giztunnel.Conn
}

func newGateway(parent context.Context, cfg Config, upstreamURL *url.URL) (*Gateway, error) {
	ctx, cancel := context.WithCancel(parent)
	listener, err := (&gizwebrtc.ListenConfig{
		ICEUDPAddr:        cfg.Gateway.ICEUDPListen,
		PublicICEUDPAddr:  cfg.Gateway.PublicICEUDP,
		SecurityPolicy:    gatewayClientSecurityPolicy{},
		AggregateServices: true,
	}).Listen(cfg.KeyPair)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("edge: start gateway listener: %w", err)
	}
	pool := newGatewayPool(ctx, cfg, upstreamURL)
	if err := pool.ensureOne(ctx); err != nil {
		_ = listener.Close()
		cancel()
		return nil, err
	}
	gateway := &Gateway{
		ctx:             ctx,
		cancel:          cancel,
		cfg:             cfg,
		listener:        listener,
		pool:            pool,
		admissions:      make(map[giznet.PublicKey][]*gatewayAdmission),
		admissionNotify: make(chan struct{}),
		sessions:        make(map[*gatewaySession]struct{}),
		acceptDone:      make(chan struct{}),
	}
	go gateway.acceptLoop()
	return gateway, nil
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
		g.listener.SignalingHandler().ServeHTTP(w, r)
		return
	}
	admission, err := g.reserveAdmission()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "gateway_over_capacity"})
		return
	}
	admission.clientKey = clientKey
	admission.remoteAddr = r.RemoteAddr
	entry, release, err := g.pool.acquire(r.Context())
	if err != nil {
		admission.releasePending()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "gateway_over_capacity"})
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
	return &gatewayAdmission{gateway: g}, nil
}

func (a *gatewayAdmission) releasePending() {
	if a == nil || a.gateway == nil || !a.state.CompareAndSwap(0, 2) {
		return
	}
	a.gateway.capacityMu.Lock()
	if a.gateway.pending > 0 {
		a.gateway.pending--
	}
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
	a.gateway.capacityMu.Unlock()
	a.releasePool()
}

func (a *gatewayAdmission) releasePool() {
	if a == nil || a.releaseUpstream == nil {
		return
	}
	a.releaseUpstream()
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
	entry := admission.upstream
	if entry == nil {
		_ = client.Close()
		return
	}
	stream, err := entry.conn.Dial(gizclaw.ServiceEdgeTunnel)
	if err != nil {
		_ = client.Close()
		return
	}
	sessionID, err := giztunnel.NewSessionID()
	if err != nil {
		_ = stream.Close()
		_ = client.Close()
		return
	}
	now := time.Now()
	logical, err := giztunnel.Dial(
		g.ctx,
		stream,
		entry.packets,
		giztunnel.OpenRequest{
			SessionID:       sessionID,
			ClientPublicKey: client.PublicKey(),
			EdgePublicKey:   g.cfg.KeyPair.Public,
			ServerPublicKey: g.cfg.Upstream.PublicKey,
			IssuedAtUnix:    now.Unix(),
			ExpiresAtUnix:   now.Add(g.cfg.Gateway.DelegatedEnvelopeValidity).Unix(),
			RemoteAddr:      admission.remoteAddr,
		},
		giztunnel.Config{
			MaxBufferedBytes:  g.cfg.Gateway.SessionBufferBytes,
			PeerPublicKey:     g.cfg.Upstream.PublicKey,
			AggregateServices: true,
			AllowRemoteService: func(service uint64) bool {
				return service != gizclaw.ServiceEdgeTunnel
			},
		},
	)
	if err != nil {
		_ = stream.Close()
		_ = client.Close()
		return
	}
	session := &gatewaySession{client: client, logical: logical}
	g.addSession(session)
	defer g.removeSession(session)
	done := make(chan struct{})
	go g.enforceIdle(session, done)
	err = giztunnel.Bridge(client, logical)
	close(done)
	if err != nil && !errors.Is(err, context.Canceled) {
		return
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
	_ = session.client.Close()
	_ = session.logical.Close()
}

func (g *Gateway) closeSessions() {
	g.sessionMu.Lock()
	sessions := make([]*gatewaySession, 0, len(g.sessions))
	for session := range g.sessions {
		sessions = append(sessions, session)
	}
	g.sessionMu.Unlock()
	for _, session := range sessions {
		_ = session.client.Close()
		_ = session.logical.Close()
	}
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

	acquireMu sync.Mutex
	mu        sync.Mutex
	entries   []*gatewayUpstream
	nextID    uint64
	closed    bool
}

type gatewayUpstream struct {
	id       uint64
	pool     *gatewayPool
	conn     giznet.Conn
	listener giznet.Listener
	packets  *giztunnel.PacketMux

	active    int
	opened    int
	draining  bool
	failed    bool
	closeOnce sync.Once
}

func newGatewayPool(ctx context.Context, cfg Config, upstreamURL *url.URL) *gatewayPool {
	return &gatewayPool{ctx: ctx, cfg: cfg, upstreamURL: upstreamURL}
}

func (p *gatewayPool) ensureOne(ctx context.Context) error {
	entry, err := p.dial(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = entry.close()
		return giznet.ErrConnClosed
	}
	p.nextID++
	entry.id = p.nextID
	p.entries = append(p.entries, entry)
	p.mu.Unlock()
	return nil
}

func (p *gatewayPool) canAccept() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	for _, entry := range p.entries {
		if !entry.failed && !entry.draining && entry.active < p.cfg.Gateway.SessionsPerUpstream {
			return true
		}
	}
	return len(p.entries) < p.cfg.Gateway.MaxUpstreams
}

func (p *gatewayPool) acquire(ctx context.Context) (*gatewayUpstream, func(), error) {
	p.acquireMu.Lock()
	defer p.acquireMu.Unlock()
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, nil, giznet.ErrConnClosed
		}
		var selected *gatewayUpstream
		for _, entry := range p.entries {
			if entry.failed || entry.draining ||
				entry.active >= p.cfg.Gateway.SessionsPerUpstream {
				continue
			}
			if selected == nil || entry.active < selected.active {
				selected = entry
			}
		}
		if selected != nil {
			selected.active++
			selected.opened++
			if selected.opened >= p.cfg.Gateway.StreamsPerUpstream {
				selected.draining = true
			}
			p.mu.Unlock()
			var once sync.Once
			return selected, func() {
				once.Do(func() { p.release(selected) })
			}, nil
		}
		if len(p.entries) >= p.cfg.Gateway.MaxUpstreams {
			p.mu.Unlock()
			return nil, nil, ErrGatewayOverCapacity
		}
		p.mu.Unlock()
		entry, err := p.dial(ctx)
		if err != nil {
			return nil, nil, err
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = entry.close()
			return nil, nil, giznet.ErrConnClosed
		}
		p.nextID++
		entry.id = p.nextID
		p.entries = append(p.entries, entry)
		p.mu.Unlock()
	}
}

func (p *gatewayPool) dial(ctx context.Context) (*gatewayUpstream, error) {
	conn, listener, err := dialUpstream(ctx, p.cfg, p.upstreamURL)
	if err != nil {
		return nil, err
	}
	entry := &gatewayUpstream{
		pool:     p,
		conn:     conn,
		listener: listener,
		packets:  giztunnel.NewPacketMux(conn),
	}
	go entry.readPackets()
	return entry, nil
}

func (p *gatewayPool) release(entry *gatewayUpstream) {
	p.mu.Lock()
	if entry.active > 0 {
		entry.active--
	}
	closeEntry := (entry.draining || entry.failed) && entry.active == 0
	if closeEntry {
		p.removeLocked(entry)
	}
	p.mu.Unlock()
	if closeEntry {
		_ = entry.close()
	}
}

func (p *gatewayPool) markFailed(entry *gatewayUpstream) {
	p.mu.Lock()
	entry.failed = true
	p.removeLocked(entry)
	p.mu.Unlock()
	_ = entry.close()
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
	p.mu.Unlock()
	var err error
	for _, entry := range entries {
		err = errors.Join(err, entry.close())
	}
	return err
}

func (e *gatewayUpstream) readPackets() {
	buf := make([]byte, 64*1024)
	for {
		protocol, n, err := e.conn.Read(buf)
		if err != nil {
			e.pool.markFailed(e)
			return
		}
		if protocol != giznet.ProtocolTunnelPacket {
			continue
		}
		if err := e.packets.HandlePacket(buf[:n]); err != nil &&
			!errors.Is(err, giztunnel.ErrSessionNotFound) {
			continue
		}
	}
}

func (e *gatewayUpstream) close() error {
	if e == nil {
		return nil
	}
	var err error
	e.closeOnce.Do(func() {
		if e.packets != nil {
			err = errors.Join(err, e.packets.Close())
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
