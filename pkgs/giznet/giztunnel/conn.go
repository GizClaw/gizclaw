package giztunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

const (
	defaultMaxChannelsPerSession = 32
	defaultMaxChannels           = 8192
	defaultMaxPendingSessions    = 2048
	defaultServiceQueueSize      = 16
	defaultHandshakeTimeout      = 10 * time.Second
	serviceOpenTimeout           = 10 * time.Second
	packetReadBufferSize         = 64 * 1024
)

// Config bounds one physical Edge tunnel router.
type Config struct {
	AcceptSessions        bool
	MaxChannelsPerSession int
	MaxChannels           int
	MaxPendingSessions    int
	ServiceQueueSize      int
	HandshakeTimeout      time.Duration
	MaxBufferedBytes      int64
	AllowRemoteService    func(client giznet.PublicKey, service uint64) bool
	AggregateServices     bool
	// OnActiveChannels receives association-wide active tunnel channel counts.
	// The callback must be non-blocking and must not call back into this Router.
	OnActiveChannels func(active int)
}

func (c Config) withDefaults() Config {
	if c.MaxChannelsPerSession <= 0 {
		c.MaxChannelsPerSession = defaultMaxChannelsPerSession
	}
	if c.MaxChannels <= 0 {
		c.MaxChannels = defaultMaxChannels
	}
	if c.MaxPendingSessions <= 0 {
		c.MaxPendingSessions = defaultMaxPendingSessions
	}
	if c.ServiceQueueSize <= 0 {
		c.ServiceQueueSize = defaultServiceQueueSize
	}
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = defaultHandshakeTimeout
	}
	if c.MaxBufferedBytes <= 0 {
		c.MaxBufferedBytes = 1 << 20
	}
	return c
}

func (c Config) validate() error {
	if c.MaxChannelsPerSession < 3 || c.MaxChannelsPerSession > defaultMaxChannelsPerSession {
		return fmt.Errorf("giztunnel: max channels per session must be between 3 and %d", defaultMaxChannelsPerSession)
	}
	if c.MaxChannels < 3 || c.MaxChannels > defaultMaxChannels {
		return fmt.Errorf("giztunnel: max channels must be between 3 and %d", defaultMaxChannels)
	}
	if c.MaxPendingSessions <= 0 || c.MaxPendingSessions > defaultMaxPendingSessions {
		return fmt.Errorf("giztunnel: max pending sessions must be between 1 and %d", defaultMaxPendingSessions)
	}
	if c.MaxBufferedBytes < 64*1024 || c.MaxBufferedBytes > 16*1024*1024 {
		return fmt.Errorf("giztunnel: max buffered bytes must be between 65536 and 16777216")
	}
	return nil
}

// Router owns the v2 tunnel namespace on one physical WebRTC connection.
type Router struct {
	transport *gizwebrtc.Conn
	cfg       Config

	mu                sync.Mutex
	sessions          map[SessionID]*Conn
	pending           map[SessionID]*pendingSession
	retired           map[SessionID]*time.Timer
	activeChannels    int
	pendingAdmissions int
	closed            bool
	acceptCh          chan acceptedSession
	closeCh           chan struct{}
	closeOnce         sync.Once
	unregister        func()
	packetWriteMu     sync.Mutex
	writeBudget       *gizwebrtc.WriteBudget
}

type acceptedSession struct {
	conn        *Conn
	declaration SessionDeclaration
}

type pendingSession struct {
	declaration  SessionDeclaration
	control      *trackedChannel
	packet       *trackedChannel
	controlLease *channelLease
	packetLease  *channelLease
	initiated    bool
	timer        *time.Timer
	active       int
}

type channelLease struct {
	router  *Router
	session SessionID
	once    sync.Once
}

type channelCapacityError struct {
	scope  string
	active int
	limit  int
}

func (*channelCapacityError) Error() string { return ErrBufferLimit.Error() }
func (*channelCapacityError) Unwrap() error { return ErrBufferLimit }

func channelCapacityFromError(err error) (*channelCapacityError, bool) {
	var capacity *channelCapacityError
	if !errors.As(err, &capacity) || capacity == nil {
		return nil, false
	}
	return capacity, true
}

func (l *channelLease) release() {
	if l == nil || l.router == nil {
		return
	}
	l.once.Do(func() { l.router.releaseChannel(l.session) })
}

type trackedChannel struct {
	*gizwebrtc.NativeChannel
	lease *channelLease
	once  sync.Once
}

func (c *trackedChannel) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	c.once.Do(func() {
		if c.NativeChannel != nil {
			closeErr = c.NativeChannel.Close()
		}
		c.lease.release()
	})
	return closeErr
}

// NewRouter claims the v2 tunnel namespace on transport.
func NewRouter(transport *gizwebrtc.Conn, cfg Config) (*Router, error) {
	if transport == nil {
		return nil, giznet.ErrNilConn
	}
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	router := &Router{
		transport:   transport,
		cfg:         cfg,
		sessions:    make(map[SessionID]*Conn),
		pending:     make(map[SessionID]*pendingSession),
		retired:     make(map[SessionID]*time.Timer),
		acceptCh:    make(chan acceptedSession, cfg.MaxPendingSessions),
		closeCh:     make(chan struct{}),
		writeBudget: gizwebrtc.NewWriteBudget(gizwebrtc.GatewaySCTPWriteBudgetSize),
	}
	unregister, err := transport.RegisterNativeChannelHandler(LabelPrefix, router.handleNativeChannel)
	if err != nil {
		return nil, err
	}
	router.unregister = unregister
	return router, nil
}

// Dial establishes one Edge-declared logical session and waits for the Server
// application result, not merely DCEP open.
func (r *Router) Dial(ctx context.Context, declaration SessionDeclaration) (*Conn, error) {
	if ctx == nil {
		return nil, errors.New("giztunnel: nil dial context")
	}
	controlName, err := controlLabel(declaration)
	if err != nil {
		return nil, err
	}
	packetName, err := packetLabel(declaration.SessionID)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	if err := r.reservePendingLocked(declaration, true); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	pending := r.pending[declaration.SessionID]
	pending.controlLease, err = r.reserveChannelLocked(declaration.SessionID)
	if err == nil {
		pending.packetLease, err = r.reserveChannelLocked(declaration.SessionID)
	}
	r.mu.Unlock()
	if err != nil {
		r.dropPending(declaration.SessionID, pending)
		return nil, err
	}

	control, err := r.transport.OpenNativeChannel(ctx, controlName, gizwebrtc.NativeChannelOptions{Ordered: true})
	if err != nil {
		r.dropPending(declaration.SessionID, pending)
		return nil, err
	}
	r.mu.Lock()
	if r.pending[declaration.SessionID] != pending {
		r.mu.Unlock()
		_ = control.Close()
		return nil, giznet.ErrConnClosed
	}
	pending.control = &trackedChannel{NativeChannel: control, lease: pending.controlLease}
	r.mu.Unlock()

	zero := uint16(0)
	packet, err := r.transport.OpenNativeChannel(ctx, packetName, gizwebrtc.NativeChannelOptions{
		Ordered:        false,
		MaxRetransmits: &zero,
	})
	if err != nil {
		r.dropPending(declaration.SessionID, pending)
		return nil, err
	}
	r.mu.Lock()
	if r.pending[declaration.SessionID] != pending {
		r.mu.Unlock()
		_ = packet.Close()
		return nil, giznet.ErrConnClosed
	}
	pending.packet = &trackedChannel{NativeChannel: packet, lease: pending.packetLease}
	r.mu.Unlock()

	reset, err := setChannelDeadline(ctx, pending.control, r.cfg.HandshakeTimeout)
	if err != nil {
		r.dropPending(declaration.SessionID, pending)
		return nil, err
	}
	result := make([]byte, sessionResultHeaderSize+maxRejectReasonSize)
	n, readErr := pending.control.Read(result)
	reset()
	if readErr != nil {
		r.dropPending(declaration.SessionID, pending)
		return nil, readErr
	}
	status, reason, err := decodeSessionResult(result[:n])
	if err != nil {
		r.dropPending(declaration.SessionID, pending)
		return nil, err
	}
	if status == sessionRejected {
		r.dropPending(declaration.SessionID, pending)
		return nil, rejectionError(reason)
	}

	r.mu.Lock()
	if r.closed || r.pending[declaration.SessionID] != pending {
		r.mu.Unlock()
		r.dropPending(declaration.SessionID, pending)
		return nil, giznet.ErrConnClosed
	}
	delete(r.pending, declaration.SessionID)
	stopPendingTimer(pending)
	r.pendingAdmissions--
	conn := newConn(r, declaration, pending.control, pending.packet, true, pending.active)
	r.sessions[declaration.SessionID] = conn
	r.mu.Unlock()
	conn.start()
	return conn, nil
}

// Accept waits for one Server-admitted logical session.
func (r *Router) Accept(ctx context.Context) (*Conn, SessionDeclaration, error) {
	if r == nil {
		return nil, SessionDeclaration{}, giznet.ErrNilConn
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case accepted := <-r.acceptCh:
			if !accepted.conn.acceptApplication() {
				continue
			}
			result, _ := encodeSessionResult(sessionAccepted, "")
			if _, err := accepted.conn.control.Write(result); err != nil {
				_ = accepted.conn.closeWithError(err)
				return nil, SessionDeclaration{}, err
			}
			accepted.conn.start()
			return accepted.conn, accepted.declaration, nil
		case <-ctx.Done():
			return nil, SessionDeclaration{}, ctx.Err()
		case <-r.closeCh:
			return nil, SessionDeclaration{}, giznet.ErrConnClosed
		}
	}
}

// ActiveChannels reports the currently reserved tunnel channels.
func (r *Router) ActiveChannels() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeChannels
}

func (r *Router) handleNativeChannel(channel *gizwebrtc.NativeChannel) {
	label, err := parseLabel(channel.Label())
	if err != nil {
		_ = channel.Close()
		return
	}
	if !channelOptionsMatch(label.kind, channel) {
		_ = channel.Close()
		return
	}
	switch label.kind {
	case labelControl:
		r.handleControlChannel(label, channel)
	case labelPacket:
		r.handlePacketChannel(label, channel)
	case labelService:
		r.handleServiceChannel(label, channel)
	default:
		_ = channel.Close()
	}
}

func channelOptionsMatch(kind labelKind, channel *gizwebrtc.NativeChannel) bool {
	if channel == nil {
		return false
	}
	switch kind {
	case labelPacket:
		maxRetransmits := channel.MaxRetransmits()
		return !channel.Ordered() && channel.MaxPacketLifeTime() == nil &&
			maxRetransmits != nil && *maxRetransmits == 0
	case labelControl, labelService:
		return channel.Ordered() && channel.MaxPacketLifeTime() == nil && channel.MaxRetransmits() == nil
	default:
		return false
	}
}

func (r *Router) handleControlChannel(label parsedLabel, native *gizwebrtc.NativeChannel) {
	declaration := SessionDeclaration{SessionID: label.session, ClientPublicKey: label.client, RemoteAddr: label.remote}
	r.mu.Lock()
	if !r.cfg.AcceptSessions {
		r.mu.Unlock()
		rejectNativeChannel(native, "remote session creation is disabled")
		return
	}
	if err := r.reservePendingLocked(declaration, false); err != nil {
		r.mu.Unlock()
		rejectNativeChannel(native, err.Error())
		return
	}
	pending := r.pending[label.session]
	if pending.control != nil {
		r.mu.Unlock()
		rejectNativeChannel(native, "duplicate control channel")
		return
	}
	lease, err := r.reserveChannelLocked(label.session)
	if err != nil {
		r.mu.Unlock()
		rejectNativeChannel(native, err.Error())
		r.dropPending(label.session, pending)
		return
	}
	pending.controlLease = lease
	pending.control = &trackedChannel{NativeChannel: native, lease: lease}
	ready := pending.packet != nil
	r.mu.Unlock()
	if ready {
		r.acceptPending(label.session, pending)
	}
}

func (r *Router) handlePacketChannel(label parsedLabel, native *gizwebrtc.NativeChannel) {
	r.mu.Lock()
	if !r.cfg.AcceptSessions {
		conn := r.sessions[label.session]
		r.mu.Unlock()
		if conn == nil {
			_ = native.Close()
			return
		}
		_ = native.Close()
		return
	}
	pending := r.pending[label.session]
	if pending == nil {
		if r.sessions[label.session] != nil {
			r.mu.Unlock()
			_ = native.Close()
			return
		}
		if _, retired := r.retired[label.session]; retired || r.pendingAdmissions >= r.cfg.MaxPendingSessions || r.closed {
			r.mu.Unlock()
			_ = native.Close()
			return
		}
		pending = &pendingSession{declaration: SessionDeclaration{SessionID: label.session}}
		pending.timer = time.AfterFunc(r.cfg.HandshakeTimeout, func() { r.expirePending(label.session, pending) })
		r.pending[label.session] = pending
		r.pendingAdmissions++
	}
	if pending.packet != nil {
		r.mu.Unlock()
		_ = native.Close()
		return
	}
	lease, err := r.reserveChannelLocked(label.session)
	if err != nil {
		control := pending.control
		r.mu.Unlock()
		if control != nil {
			rejectNativeChannel(control.NativeChannel, err.Error())
		}
		_ = native.Close()
		r.dropPending(label.session, pending)
		return
	}
	pending.packetLease = lease
	pending.packet = &trackedChannel{NativeChannel: native, lease: lease}
	ready := pending.control != nil
	r.mu.Unlock()
	if ready {
		r.acceptPending(label.session, pending)
	}
}

func (r *Router) handleServiceChannel(label parsedLabel, native *gizwebrtc.NativeChannel) {
	r.mu.Lock()
	conn := r.sessions[label.session]
	if conn == nil || r.closed || !conn.applicationAccepted() {
		r.mu.Unlock()
		_ = native.Close()
		return
	}
	lease, err := r.reserveChannelLocked(label.session)
	r.mu.Unlock()
	if err != nil {
		_ = native.Close()
		return
	}
	tracked := &trackedChannel{NativeChannel: native, lease: lease}
	if err := conn.acceptRemoteService(label.service, label.channelID, tracked); err != nil {
		_ = tracked.Close()
	}
}

func (r *Router) reservePendingLocked(declaration SessionDeclaration, initiated bool) error {
	if r.closed {
		return giznet.ErrConnClosed
	}
	if declaration.SessionID.IsZero() || declaration.ClientPublicKey.IsZero() {
		return ErrInvalidFrame
	}
	if _, retired := r.retired[declaration.SessionID]; retired {
		return ErrSessionExists
	}
	if r.sessions[declaration.SessionID] != nil {
		return ErrSessionExists
	}
	if pending := r.pending[declaration.SessionID]; pending != nil {
		if pending.declaration.ClientPublicKey.IsZero() && !declaration.ClientPublicKey.IsZero() {
			pending.declaration = declaration
			pending.initiated = initiated
			return nil
		}
		if pending.declaration != declaration || pending.initiated != initiated {
			return ErrSessionExists
		}
		return nil
	}
	if r.pendingAdmissions >= r.cfg.MaxPendingSessions {
		return ErrBufferLimit
	}
	pending := &pendingSession{declaration: declaration, initiated: initiated}
	pending.timer = time.AfterFunc(r.cfg.HandshakeTimeout, func() { r.expirePending(declaration.SessionID, pending) })
	r.pending[declaration.SessionID] = pending
	r.pendingAdmissions++
	return nil
}

func (r *Router) reserveChannelLocked(session SessionID) (*channelLease, error) {
	if r.closed {
		return nil, ErrBufferLimit
	}
	active := 0
	conn := r.sessions[session]
	if r.activeChannels >= r.cfg.MaxChannels {
		if conn != nil {
			return nil, &channelCapacityError{
				scope:  "association",
				active: r.activeChannels,
				limit:  r.cfg.MaxChannels,
			}
		}
		return nil, ErrBufferLimit
	}
	if conn != nil {
		active = conn.activeChannelCount()
	} else if pending := r.pending[session]; pending != nil {
		active = pending.active
	} else {
		return nil, ErrSessionNotFound
	}
	if active >= r.cfg.MaxChannelsPerSession {
		if conn != nil {
			return nil, &channelCapacityError{
				scope:  "session",
				active: active,
				limit:  r.cfg.MaxChannelsPerSession,
			}
		}
		return nil, ErrBufferLimit
	}
	r.activeChannels++
	if r.cfg.OnActiveChannels != nil {
		r.cfg.OnActiveChannels(r.activeChannels)
	}
	if conn := r.sessions[session]; conn != nil {
		conn.incrementActiveChannels()
	} else {
		r.pending[session].active++
	}
	return &channelLease{router: r, session: session}, nil
}

func (r *Router) releaseChannel(session SessionID) {
	r.mu.Lock()
	if r.activeChannels > 0 {
		r.activeChannels--
	}
	activeChannels := r.activeChannels
	if conn := r.sessions[session]; conn != nil {
		conn.decrementActiveChannels()
	} else if pending := r.pending[session]; pending != nil && pending.active > 0 {
		pending.active--
	}
	r.mu.Unlock()
	if r.cfg.OnActiveChannels != nil {
		r.cfg.OnActiveChannels(activeChannels)
	}
}

func (r *Router) acceptPending(id SessionID, pending *pendingSession) {
	r.mu.Lock()
	if r.closed || r.pending[id] != pending || pending.control == nil || pending.packet == nil ||
		pending.declaration.ClientPublicKey.IsZero() {
		r.mu.Unlock()
		return
	}
	delete(r.pending, id)
	stopPendingTimer(pending)
	conn := newConn(r, pending.declaration, pending.control, pending.packet, false, pending.active)
	conn.admissionSlot.Store(true)
	r.sessions[id] = conn
	conn.armAdmissionTimeout(r.cfg.HandshakeTimeout)
	r.mu.Unlock()

	select {
	case r.acceptCh <- acceptedSession{conn: conn, declaration: pending.declaration}:
	case <-r.closeCh:
		_ = conn.Close()
	}
}

func rejectNativeChannel(channel *gizwebrtc.NativeChannel, reason string) {
	if len(reason) > maxRejectReasonSize {
		reason = reason[:maxRejectReasonSize]
	}
	result, err := encodeSessionResult(sessionRejected, reason)
	if err == nil {
		_, _ = channel.Write(result)
	}
	_ = channel.Close()
}

func (r *Router) expirePending(id SessionID, pending *pendingSession) {
	r.dropPending(id, pending)
}

func (r *Router) dropPending(id SessionID, expected *pendingSession) {
	r.mu.Lock()
	pending := r.pending[id]
	if pending == nil || expected != nil && pending != expected {
		r.mu.Unlock()
		return
	}
	delete(r.pending, id)
	r.pendingAdmissions--
	r.retireSessionLocked(id)
	stopPendingTimer(pending)
	r.mu.Unlock()
	if pending.control != nil {
		_ = pending.control.Close()
	} else {
		pending.controlLease.release()
	}
	if pending.packet != nil {
		_ = pending.packet.Close()
	} else {
		pending.packetLease.release()
	}
}

func stopPendingTimer(pending *pendingSession) {
	if pending != nil && pending.timer != nil {
		pending.timer.Stop()
	}
}

func setChannelDeadline(ctx context.Context, channel net.Conn, timeout time.Duration) (func(), error) {
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := channel.SetDeadline(deadline); err != nil {
		return nil, err
	}
	return func() { _ = channel.SetDeadline(time.Time{}) }, nil
}

func (r *Router) removeSession(id SessionID, conn *Conn) {
	r.mu.Lock()
	if r.sessions[id] == conn {
		delete(r.sessions, id)
		r.retireSessionLocked(id)
	}
	r.mu.Unlock()
}

func (r *Router) retireSessionLocked(id SessionID) {
	if r.closed {
		return
	}
	if _, exists := r.retired[id]; exists {
		return
	}
	timer := time.AfterFunc(r.cfg.HandshakeTimeout, func() {
		r.mu.Lock()
		delete(r.retired, id)
		r.mu.Unlock()
	})
	r.retired[id] = timer
}

// Close releases every pending and established logical session.
func (r *Router) Close() error {
	if r == nil {
		return nil
	}
	var closeErr error
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.closeCh)
		for id, timer := range r.retired {
			timer.Stop()
			delete(r.retired, id)
		}
		sessions := make([]*Conn, 0, len(r.sessions))
		for _, conn := range r.sessions {
			sessions = append(sessions, conn)
		}
		pending := make(map[SessionID]*pendingSession, len(r.pending))
		maps.Copy(pending, r.pending)
		r.mu.Unlock()
		for _, conn := range sessions {
			closeErr = errors.Join(closeErr, conn.Close())
		}
		for id, session := range pending {
			r.dropPending(id, session)
		}
		if r.unregister != nil {
			r.unregister()
		}
	})
	return closeErr
}

// Conn is one logical Giznet connection aggregated from native channels.
type Conn struct {
	router      *Router
	declaration SessionDeclaration
	control     *trackedChannel
	packet      *trackedChannel
	initiator   bool

	mu             sync.Mutex
	streams        map[uint64]*serviceStream
	seenStreams    map[uint64]struct{}
	byService      map[uint64]map[uint64]*serviceStream
	listeners      map[uint64]*serviceListener
	closedSvc      map[uint64]bool
	nextID         uint64
	localParity    uint64
	activeChannels atomic.Int64
	writeBudget    *gizwebrtc.WriteBudget
	admission      atomic.Uint32
	admissionSlot  atomic.Bool
	admissionMu    sync.Mutex
	admissionTimer *time.Timer

	acceptAll atomic.Bool
	serviceCh chan acceptedService
	readCh    chan directPacket
	lastSeen  atomic.Int64
	closeCh   chan struct{}
	closeOnce sync.Once
	closeErr  atomic.Pointer[error]
}

type acceptedService struct {
	service uint64
	stream  net.Conn
}

type directPacket struct {
	protocol byte
	payload  []byte
}

const (
	admissionPending uint32 = iota
	admissionAccepted
	admissionExpired
)

func newConn(
	router *Router,
	declaration SessionDeclaration,
	control, packet *trackedChannel,
	initiator bool,
	active int,
) *Conn {
	conn := &Conn{
		router:      router,
		declaration: declaration,
		control:     control,
		packet:      packet,
		initiator:   initiator,
		streams:     make(map[uint64]*serviceStream),
		seenStreams: make(map[uint64]struct{}),
		byService:   make(map[uint64]map[uint64]*serviceStream),
		listeners:   make(map[uint64]*serviceListener),
		closedSvc:   make(map[uint64]bool),
		serviceCh:   make(chan acceptedService, router.cfg.ServiceQueueSize),
		readCh:      make(chan directPacket, 256),
		closeCh:     make(chan struct{}),
		writeBudget: gizwebrtc.NewWriteBudget(uint64(router.cfg.MaxBufferedBytes)),
	}
	if initiator {
		conn.admission.Store(admissionAccepted)
		conn.nextID = 1
		conn.localParity = 1
	} else {
		conn.nextID = 2
	}
	conn.activeChannels.Store(int64(active))
	if router.cfg.AggregateServices {
		conn.EnableServiceAccept()
	}
	conn.touch()
	return conn
}

func (c *Conn) armAdmissionTimeout(timeout time.Duration) {
	c.admissionMu.Lock()
	c.admissionTimer = time.AfterFunc(timeout, func() {
		if c.admission.CompareAndSwap(admissionPending, admissionExpired) {
			_ = c.closeWithError(context.DeadlineExceeded)
		}
	})
	c.admissionMu.Unlock()
}

func (c *Conn) acceptApplication() bool {
	if c == nil || c.router == nil {
		return false
	}
	c.router.mu.Lock()
	if c.router.closed || c.router.sessions[c.declaration.SessionID] != c {
		c.router.mu.Unlock()
		return false
	}
	if !c.admission.CompareAndSwap(admissionPending, admissionAccepted) {
		accepted := c.admission.Load() == admissionAccepted
		c.router.mu.Unlock()
		return accepted
	}
	c.router.mu.Unlock()
	c.stopAdmissionTimer()
	c.releaseAdmissionSlot()
	return true
}

func (c *Conn) applicationAccepted() bool {
	return c != nil && c.admission.Load() == admissionAccepted
}

func (c *Conn) stopAdmissionTimer() {
	c.admissionMu.Lock()
	if c.admissionTimer != nil {
		c.admissionTimer.Stop()
		c.admissionTimer = nil
	}
	c.admissionMu.Unlock()
}

func (c *Conn) releaseAdmissionSlot() {
	if c == nil || c.router == nil || !c.admissionSlot.CompareAndSwap(true, false) {
		return
	}
	c.router.mu.Lock()
	c.router.pendingAdmissions--
	c.router.mu.Unlock()
}

func (c *Conn) start() {
	go c.monitorControl()
	go c.readPacketLoop()
}

func (c *Conn) monitorControl() {
	buf := make([]byte, 1)
	_, err := c.control.Read(buf)
	if err == nil {
		err = ErrInvalidFrame
	}
	_ = c.closeWithError(err)
}

func (c *Conn) readPacketLoop() {
	buf := make([]byte, packetReadBufferSize)
	for {
		n, err := c.packet.ReadMessage(buf)
		if err != nil {
			_ = c.closeWithError(err)
			return
		}
		if n < 1 {
			_ = c.closeWithError(ErrInvalidFrame)
			return
		}
		protocol := buf[0]
		if protocol < 0x40 || protocol == giznet.ProtocolOpusPacket || protocol == giznet.ProtocolTunnelPacket {
			_ = c.closeWithError(giznet.ErrPacketProtocol)
			return
		}
		if err := c.enqueuePacket(protocol, buf[1:n]); err != nil {
			return
		}
	}
}

func (c *Conn) enqueuePacket(protocol byte, payload []byte) error {
	packet := directPacket{protocol: protocol, payload: append([]byte(nil), payload...)}
	select {
	case c.readCh <- packet:
		c.touch()
		return nil
	case <-c.closeCh:
		return c.err()
	default:
		_ = c.closeWithError(ErrBufferLimit)
		return ErrBufferLimit
	}
}

// LastActivity reports the last native channel or packet activity.
func (c *Conn) LastActivity() time.Time {
	if c == nil {
		return time.Time{}
	}
	return time.Unix(0, c.lastSeen.Load())
}

func (c *Conn) Dial(service uint64) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceOpenTimeout)
	defer cancel()
	return c.dialContext(ctx, service)
}

func (c *Conn) dialContext(ctx context.Context, service uint64) (net.Conn, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closedSvc[service] {
		c.mu.Unlock()
		return nil, giznet.ErrServiceMuxClosed
	}
	id := c.nextID
	if id == 0 || id > ^uint64(0)-2 {
		c.mu.Unlock()
		return nil, ErrInvalidState
	}
	c.nextID += 2
	c.mu.Unlock()

	label, err := serviceLabel(c.declaration.SessionID, service, id)
	if err != nil {
		return nil, err
	}
	c.router.mu.Lock()
	lease, err := c.router.reserveChannelLocked(c.declaration.SessionID)
	c.router.mu.Unlock()
	if err != nil {
		return nil, err
	}
	native, err := c.router.transport.OpenNativeChannel(ctx, label, gizwebrtc.NativeChannelOptions{Ordered: true})
	if err != nil {
		lease.release()
		return nil, err
	}
	if err := c.configureServiceWriteBudgets(native); err != nil {
		_ = native.Close()
		lease.release()
		return nil, err
	}
	stream := newServiceStream(c, id, service, &trackedChannel{NativeChannel: native, lease: lease})
	if err := c.addLocalStream(stream); err != nil {
		_ = stream.Close()
		return nil, err
	}
	return stream, nil
}

func (c *Conn) ListenService(service uint64) giznet.ServiceListener {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listenerLocked(service)
}

func (c *Conn) AcceptService() (uint64, net.Conn, error) {
	if err := c.validate(); err != nil {
		return 0, nil, err
	}
	c.acceptAll.Store(true)
	select {
	case accepted := <-c.serviceCh:
		return accepted.service, accepted.stream, nil
	case <-c.closeCh:
		return 0, nil, c.err()
	}
}

func (c *Conn) EnableServiceAccept() {
	if c != nil {
		c.acceptAll.Store(true)
	}
}

func (c *Conn) CloseService(service uint64) error {
	if c == nil {
		return giznet.ErrNilConn
	}
	c.mu.Lock()
	c.closedSvc[service] = true
	listener := c.listeners[service]
	streams := make([]*serviceStream, 0, len(c.byService[service]))
	for _, stream := range c.byService[service] {
		streams = append(streams, stream)
	}
	c.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	for _, stream := range streams {
		_ = stream.Close()
	}
	return nil
}

func (c *Conn) Read(buf []byte) (byte, int, error) {
	if err := c.validate(); err != nil {
		return 0, 0, err
	}
	select {
	case packet := <-c.readCh:
		if len(packet.payload) > len(buf) {
			return 0, 0, giznet.ErrPacketBuffer
		}
		copy(buf, packet.payload)
		return packet.protocol, len(packet.payload), nil
	case <-c.closeCh:
		return 0, 0, c.err()
	}
}

func (c *Conn) Write(protocol byte, payload []byte) (int, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	if protocol == giznet.ProtocolOpusPacket {
		n, err := c.router.writeOpus(c.declaration.SessionID, payload)
		if err == nil {
			c.touch()
		}
		return n, err
	}
	if protocol < 0x40 || protocol == giznet.ProtocolTunnelPacket {
		return 0, giznet.ErrPacketProtocol
	}
	if len(payload) > packetReadBufferSize-1 {
		return 0, giznet.ErrPacketTooLarge
	}
	message := make([]byte, 1+len(payload))
	message[0] = protocol
	copy(message[1:], payload)
	n, err := c.packet.WriteMessage(message)
	if err != nil {
		return 0, err
	}
	if n != len(message) {
		return max(0, n-1), io.ErrShortWrite
	}
	c.touch()
	return len(payload), nil
}

func (c *Conn) PublicKey() giznet.PublicKey {
	if c == nil {
		return giznet.PublicKey{}
	}
	return c.declaration.ClientPublicKey
}

func (c *Conn) PeerInfo() *giznet.PeerInfo {
	if c == nil {
		return nil
	}
	state := giznet.PeerStateEstablished
	select {
	case <-c.closeCh:
		state = giznet.PeerStateOffline
	default:
	}
	return &giznet.PeerInfo{
		PublicKey: c.declaration.ClientPublicKey,
		Endpoint:  tunnelAddr(c.declaration.RemoteAddr),
		State:     state,
		LastSeen:  c.LastActivity(),
	}
}

func (c *Conn) Close() error { return c.closeWithError(giznet.ErrConnClosed) }

func (c *Conn) closeWithError(cause error) error {
	if c == nil {
		return nil
	}
	var closeErr error
	c.closeOnce.Do(func() {
		if cause == nil {
			cause = giznet.ErrConnClosed
		}
		c.closeErr.Store(&cause)
		c.admission.CompareAndSwap(admissionPending, admissionExpired)
		c.stopAdmissionTimer()
		c.releaseAdmissionSlot()
		close(c.closeCh)
		c.mu.Lock()
		streams := make([]*serviceStream, 0, len(c.streams))
		for _, stream := range c.streams {
			streams = append(streams, stream)
		}
		listeners := make([]*serviceListener, 0, len(c.listeners))
		for _, listener := range c.listeners {
			listeners = append(listeners, listener)
		}
		c.mu.Unlock()
		for _, listener := range listeners {
			_ = listener.Close()
		}
		for _, stream := range streams {
			closeErr = errors.Join(closeErr, stream.Close())
		}
		closeErr = errors.Join(closeErr, c.packet.Close(), c.control.Close())
		c.router.removeSession(c.declaration.SessionID, c)
	})
	return closeErr
}

func (c *Conn) validate() error {
	if c == nil || c.router == nil || c.control == nil || c.packet == nil {
		return giznet.ErrNilConn
	}
	select {
	case <-c.closeCh:
		return c.err()
	default:
		return nil
	}
}

func (c *Conn) err() error {
	if c == nil {
		return giznet.ErrNilConn
	}
	if value := c.closeErr.Load(); value != nil && *value != nil {
		return *value
	}
	return giznet.ErrConnClosed
}

func (c *Conn) acceptRemoteService(service, id uint64, channel *trackedChannel) error {
	if id == 0 || id&1 == c.localParity {
		return ErrInvalidFrame
	}
	if c.router.cfg.AllowRemoteService != nil && !c.router.cfg.AllowRemoteService(c.PublicKey(), service) {
		return fmt.Errorf("%w: %d", ErrServiceForbidden, service)
	}
	if err := c.configureServiceWriteBudgets(channel.NativeChannel); err != nil {
		return err
	}
	c.mu.Lock()
	if err := c.validate(); err != nil {
		c.mu.Unlock()
		return err
	}
	if c.closedSvc[service] {
		c.mu.Unlock()
		return giznet.ErrServiceMuxClosed
	}
	if _, seen := c.seenStreams[id]; seen {
		c.mu.Unlock()
		return fmt.Errorf("%w: reused channel %d", ErrInvalidState, id)
	}
	stream := newServiceStream(c, id, service, channel)
	c.seenStreams[id] = struct{}{}
	c.addStreamLocked(stream)
	listener := c.listenerLocked(service)
	c.mu.Unlock()
	if c.acceptAll.Load() {
		select {
		case c.serviceCh <- acceptedService{service: service, stream: stream}:
			return nil
		default:
			_ = stream.Close()
			return ErrBufferLimit
		}
	}
	return listener.enqueue(stream)
}

func (c *Conn) configureServiceWriteBudgets(channel *gizwebrtc.NativeChannel) error {
	if c == nil || channel == nil {
		return giznet.ErrNilConn
	}
	perChannel := uint64(c.router.cfg.MaxBufferedBytes / 2)
	if perChannel == 0 {
		return ErrBufferLimit
	}
	return channel.SetWriteBudgets(
		gizwebrtc.NewWriteBudget(perChannel),
		c.writeBudget,
		c.router.writeBudget,
	)
}

func (c *Conn) addLocalStream(stream *serviceStream) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.validate(); err != nil {
		return err
	}
	if c.closedSvc[stream.service] || c.streams[stream.id] != nil {
		return giznet.ErrServiceMuxClosed
	}
	c.addStreamLocked(stream)
	return nil
}

func (c *Conn) addStreamLocked(stream *serviceStream) {
	c.streams[stream.id] = stream
	if c.byService[stream.service] == nil {
		c.byService[stream.service] = make(map[uint64]*serviceStream)
	}
	c.byService[stream.service][stream.id] = stream
}

func (c *Conn) removeStream(stream *serviceStream) {
	c.mu.Lock()
	if c.streams[stream.id] == stream {
		delete(c.streams, stream.id)
		delete(c.byService[stream.service], stream.id)
		if len(c.byService[stream.service]) == 0 {
			delete(c.byService, stream.service)
		}
	}
	c.mu.Unlock()
}

func (c *Conn) listenerLocked(service uint64) *serviceListener {
	if listener := c.listeners[service]; listener != nil {
		return listener
	}
	listener := &serviceListener{
		conn:    c,
		service: service,
		accept:  make(chan net.Conn, c.router.cfg.ServiceQueueSize),
		closeCh: make(chan struct{}),
	}
	c.listeners[service] = listener
	return listener
}

func (c *Conn) touch() { c.lastSeen.Store(time.Now().UnixNano()) }

func (c *Conn) activeChannelCount() int  { return int(c.activeChannels.Load()) }
func (c *Conn) incrementActiveChannels() { c.activeChannels.Add(1) }
func (c *Conn) decrementActiveChannels() { c.activeChannels.Add(-1) }

type serviceStream struct {
	conn    *Conn
	id      uint64
	service uint64
	channel *trackedChannel
	once    sync.Once
}

func newServiceStream(conn *Conn, id, service uint64, channel *trackedChannel) *serviceStream {
	return &serviceStream{conn: conn, id: id, service: service, channel: channel}
}

func (s *serviceStream) Read(buf []byte) (int, error) {
	n, err := s.channel.Read(buf)
	if n > 0 {
		s.conn.touch()
	}
	return n, err
}

func (s *serviceStream) Write(buf []byte) (int, error) {
	n, err := s.channel.Write(buf)
	if n > 0 {
		s.conn.touch()
	}
	return n, err
}

func (s *serviceStream) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	s.once.Do(func() {
		s.conn.removeStream(s)
		closeErr = s.channel.Close()
	})
	return closeErr
}

func (s *serviceStream) LocalAddr() net.Addr                { return s.channel.LocalAddr() }
func (s *serviceStream) RemoteAddr() net.Addr               { return s.channel.RemoteAddr() }
func (s *serviceStream) SetDeadline(t time.Time) error      { return s.channel.SetDeadline(t) }
func (s *serviceStream) SetReadDeadline(t time.Time) error  { return s.channel.SetReadDeadline(t) }
func (s *serviceStream) SetWriteDeadline(t time.Time) error { return s.channel.SetWriteDeadline(t) }

type serviceListener struct {
	conn     *Conn
	service  uint64
	accept   chan net.Conn
	closeCh  chan struct{}
	closeOne sync.Once

	enqueueMu sync.Mutex
	closed    bool
	enqueues  sync.WaitGroup
}

func (l *serviceListener) Accept() (net.Conn, error) {
	if l == nil || l.conn == nil {
		return nil, giznet.ErrNilConn
	}
	select {
	case <-l.closeCh:
		return nil, giznet.ErrServiceMuxClosed
	default:
	}
	select {
	case stream := <-l.accept:
		return stream, nil
	case <-l.closeCh:
		return nil, giznet.ErrServiceMuxClosed
	case <-l.conn.closeCh:
		return nil, l.conn.err()
	}
}

func (l *serviceListener) enqueue(stream net.Conn) error {
	l.enqueueMu.Lock()
	if l.closed {
		l.enqueueMu.Unlock()
		_ = stream.Close()
		return giznet.ErrServiceMuxClosed
	}
	l.enqueues.Add(1)
	l.enqueueMu.Unlock()
	defer l.enqueues.Done()

	select {
	case l.accept <- stream:
		return nil
	case <-l.closeCh:
		_ = stream.Close()
		return giznet.ErrServiceMuxClosed
	default:
		_ = stream.Close()
		return ErrBufferLimit
	}
}

func (l *serviceListener) Close() error {
	if l != nil {
		l.closeOne.Do(func() {
			l.enqueueMu.Lock()
			l.closed = true
			close(l.closeCh)
			l.enqueueMu.Unlock()
			l.enqueues.Wait()
			for {
				select {
				case stream := <-l.accept:
					_ = stream.Close()
				default:
					return
				}
			}
		})
	}
	return nil
}

func (l *serviceListener) Addr() net.Addr { return tunnelAddr("giztunnel:listener") }

type tunnelAddr string

func (a tunnelAddr) Network() string { return "giztunnel" }
func (a tunnelAddr) String() string  { return string(a) }

var (
	_ giznet.Conn                 = (*Conn)(nil)
	_ giznet.ServiceAcceptor      = (*Conn)(nil)
	_ giznet.ServiceAcceptEnabler = (*Conn)(nil)
	_ net.Conn                    = (*serviceStream)(nil)
)
