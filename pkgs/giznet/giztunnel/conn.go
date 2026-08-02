package giztunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	defaultMaxBufferedBytes = 1 << 20
	defaultStreamQueueSize  = 32
	defaultServiceQueueSize = 16
	defaultHandshakeTimeout = 10 * time.Second
	// Leave room for the tunnel frame and stream ID so one chunk fits exactly
	// in a stable 32 KiB WebRTC DataChannel write.
	streamChunkSize = 32*1024 - frameHeaderSize - 8
)

var streamBufferPool = sync.Pool{New: func() any {
	buffer := make([]byte, 0, streamChunkSize)
	return &buffer
}}

type Config struct {
	MaxFrameSize       int
	MaxBufferedBytes   int64
	StreamQueueSize    int
	ServiceQueueSize   int
	HandshakeTimeout   time.Duration
	PeerPublicKey      giznet.PublicKey
	PeerInfo           *giznet.PeerInfo
	AllowRemoteService func(service uint64) bool
	AggregateServices  bool
}

func (c Config) withDefaults() Config {
	if c.MaxFrameSize <= 0 {
		c.MaxFrameSize = defaultMaxFrameSize
	}
	if c.MaxBufferedBytes <= 0 {
		c.MaxBufferedBytes = defaultMaxBufferedBytes
	}
	if c.StreamQueueSize <= 0 {
		c.StreamQueueSize = defaultStreamQueueSize
	}
	if c.ServiceQueueSize <= 0 {
		c.ServiceQueueSize = defaultServiceQueueSize
	}
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = defaultHandshakeTimeout
	}
	return c
}

// Conn is one logical Giznet connection multiplexed over a tunnel stream.
type Conn struct {
	stream  net.Conn
	packets *packetEndpoint
	cfg     Config
	id      SessionID

	writeMu sync.Mutex
	mu      sync.Mutex
	streams map[uint64]*virtualStream
	bySvc   map[uint64]map[uint64]*virtualStream
	svcs    map[uint64]*serviceListener
	closed  map[uint64]bool

	acceptAll     atomic.Bool
	serviceCh     chan acceptedService
	nextID        atomic.Uint64
	localIDParity uint64
	bufferMu      sync.Mutex
	bufferWake    chan struct{}
	buffered      atomic.Int64
	lastSeen      atomic.Int64
	closeOnce     sync.Once
	closeCh       chan struct{}
	closeErr      atomic.Pointer[error]
}

type acceptedService struct {
	service uint64
	stream  net.Conn
}

// Dial opens a logical tunnel session and waits for the remote acceptance.
func Dial(ctx context.Context, stream net.Conn, packets *PacketMux, open OpenRequest, cfg Config) (*Conn, error) {
	if stream == nil || packets == nil {
		return nil, ErrInvalidState
	}
	cfg = cfg.withDefaults()
	end, err := packets.register(open.SessionID)
	if err != nil {
		return nil, err
	}
	payload, err := encodeOpenRequest(open)
	if err != nil {
		end.close()
		return nil, err
	}
	reset, err := setHandshakeDeadline(ctx, stream, cfg.HandshakeTimeout)
	if err != nil {
		end.close()
		return nil, err
	}
	defer reset()
	if err := writeFrame(stream, frameSessionOpen, payload, cfg.MaxFrameSize); err != nil {
		end.close()
		return nil, err
	}
	typ, response, err := readFrame(stream, cfg.MaxFrameSize)
	if err != nil {
		end.close()
		return nil, err
	}
	switch typ {
	case frameSessionAccepted:
		if len(response) != 0 {
			end.close()
			return nil, ErrInvalidFrame
		}
	case frameSessionRejected:
		end.close()
		if len(response) > maxRejectReasonSize {
			return nil, ErrFrameTooLarge
		}
		return nil, fmt.Errorf("%w: %s", ErrSessionRejected, string(response))
	default:
		end.close()
		return nil, fmt.Errorf("%w: handshake frame %d", ErrInvalidState, typ)
	}
	conn := newConn(stream, end, open.SessionID, cfg, true)
	go conn.readLoop()
	return conn, nil
}

// Accept validates and accepts one logical session opened on stream.
func Accept(ctx context.Context, stream net.Conn, packets *PacketMux, validate func(OpenRequest) error, cfg Config) (*Conn, OpenRequest, error) {
	if stream == nil || packets == nil {
		return nil, OpenRequest{}, ErrInvalidState
	}
	cfg = cfg.withDefaults()
	reset, err := setHandshakeDeadline(ctx, stream, cfg.HandshakeTimeout)
	if err != nil {
		return nil, OpenRequest{}, err
	}
	defer reset()
	typ, payload, err := readFrame(stream, cfg.MaxFrameSize)
	if err != nil {
		return nil, OpenRequest{}, err
	}
	if typ != frameSessionOpen {
		return nil, OpenRequest{}, fmt.Errorf("%w: expected session open", ErrInvalidState)
	}
	open, err := decodeOpenRequest(payload)
	if err == nil && validate != nil {
		err = validate(open)
	}
	if err != nil {
		reason := err.Error()
		if len(reason) > maxRejectReasonSize {
			reason = reason[:maxRejectReasonSize]
		}
		_ = writeFrame(stream, frameSessionRejected, []byte(reason), cfg.MaxFrameSize)
		return nil, OpenRequest{}, err
	}
	end, err := packets.register(open.SessionID)
	if err != nil {
		_ = writeFrame(stream, frameSessionRejected, []byte(err.Error()), cfg.MaxFrameSize)
		return nil, OpenRequest{}, err
	}
	if err := writeFrame(stream, frameSessionAccepted, nil, cfg.MaxFrameSize); err != nil {
		end.close()
		return nil, OpenRequest{}, err
	}
	if cfg.PeerPublicKey.IsZero() {
		cfg.PeerPublicKey = open.ClientPublicKey
	}
	conn := newConn(stream, end, open.SessionID, cfg, false)
	go conn.readLoop()
	return conn, open, nil
}

func setHandshakeDeadline(ctx context.Context, stream net.Conn, timeout time.Duration) (func(), error) {
	deadline := time.Now().Add(timeout)
	if ctx != nil {
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if err := stream.SetDeadline(deadline); err != nil {
		return nil, err
	}
	return func() { _ = stream.SetDeadline(time.Time{}) }, nil
}

func newConn(stream net.Conn, packets *packetEndpoint, id SessionID, cfg Config, initiator bool) *Conn {
	conn := &Conn{
		stream:    stream,
		packets:   packets,
		cfg:       cfg,
		id:        id,
		streams:   make(map[uint64]*virtualStream),
		bySvc:     make(map[uint64]map[uint64]*virtualStream),
		svcs:      make(map[uint64]*serviceListener),
		closed:    make(map[uint64]bool),
		serviceCh: make(chan acceptedService, cfg.ServiceQueueSize),
		closeCh:   make(chan struct{}),
	}
	if initiator {
		conn.nextID.Store(1)
		conn.localIDParity = 1
	} else {
		conn.nextID.Store(2)
	}
	if cfg.AggregateServices {
		conn.EnableServiceAccept()
	}
	conn.touch()
	return conn
}

// LastActivity reports the last successful tunnel frame or packet activity.
func (c *Conn) LastActivity() time.Time {
	if c == nil {
		return time.Time{}
	}
	return time.Unix(0, c.lastSeen.Load())
}

func (c *Conn) Dial(service uint64) (net.Conn, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed[service] {
		c.mu.Unlock()
		return nil, giznet.ErrServiceMuxClosed
	}
	id := c.nextID.Load()
	if id == 0 || id > ^uint64(0)-2 {
		c.mu.Unlock()
		return nil, ErrInvalidState
	}
	if c.streams[id] != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: stream ID collision %d", ErrInvalidState, id)
	}
	c.nextID.Store(id + 2)
	stream := newVirtualStream(c, id, service)
	c.addStreamLocked(stream)
	c.mu.Unlock()
	if err := c.send(frameStreamOpen, encodeStreamOpen(id, service)); err != nil {
		c.removeStream(id)
		stream.abort()
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
	c.closed[service] = true
	listener := c.svcs[service]
	streams := make([]*virtualStream, 0, len(c.bySvc[service]))
	for _, stream := range c.bySvc[service] {
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
	protocol, n, err := c.packets.read(buf)
	if err == nil {
		c.touch()
	}
	return protocol, n, err
}

func (c *Conn) Write(protocol byte, payload []byte) (int, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	n, err := c.packets.write(protocol, payload)
	if err == nil {
		c.touch()
	}
	return n, err
}

func (c *Conn) PublicKey() giznet.PublicKey {
	if c == nil {
		return giznet.PublicKey{}
	}
	return c.cfg.PeerPublicKey
}

func (c *Conn) PeerInfo() *giznet.PeerInfo {
	if c == nil {
		return nil
	}
	if c.cfg.PeerInfo == nil {
		return &giznet.PeerInfo{PublicKey: c.cfg.PeerPublicKey, State: giznet.PeerStateEstablished, LastSeen: time.Now()}
	}
	info := *c.cfg.PeerInfo
	info.PublicKey = c.cfg.PeerPublicKey
	info.LastSeen = c.LastActivity()
	select {
	case <-c.closeCh:
		info.State = giznet.PeerStateOffline
	default:
		info.State = giznet.PeerStateEstablished
	}
	return &info
}

func (c *Conn) Close() error {
	return c.closeWithError(giznet.ErrConnClosed, true)
}

func (c *Conn) validate() error {
	if c == nil || c.stream == nil || c.packets == nil {
		return giznet.ErrNilConn
	}
	select {
	case <-c.closeCh:
		return c.err()
	default:
		return nil
	}
}

func (c *Conn) readLoop() {
	for {
		typ, payload, err := readFrame(c.stream, c.cfg.MaxFrameSize)
		if err != nil {
			_ = c.closeWithError(err, false)
			return
		}
		if typ == frameSessionClose {
			if len(payload) != 0 {
				_ = c.closeWithError(ErrInvalidFrame, true)
			} else {
				_ = c.closeWithError(io.EOF, false)
			}
			return
		}
		if err := c.handleFrame(typ, payload); err != nil {
			_ = c.closeWithError(err, true)
			return
		}
		c.touch()
	}
}

func (c *Conn) handleFrame(typ frameType, payload []byte) error {
	switch typ {
	case frameStreamOpen:
		id, service, err := decodeStreamOpen(payload)
		if err != nil {
			return err
		}
		return c.acceptRemoteStream(id, service)
	case frameStreamData:
		id, data, err := decodeStreamData(payload)
		if err != nil {
			return err
		}
		c.mu.Lock()
		stream := c.streams[id]
		c.mu.Unlock()
		if stream == nil {
			return fmt.Errorf("%w: stream %d", ErrInvalidState, id)
		}
		return stream.deliver(data)
	case frameStreamClose:
		id, err := decodeStreamID(payload)
		if err != nil {
			return err
		}
		c.mu.Lock()
		stream := c.streams[id]
		c.mu.Unlock()
		if stream != nil {
			stream.finishRemote()
		}
		return nil
	default:
		return fmt.Errorf("%w: frame type %d", ErrInvalidFrame, typ)
	}
}

func (c *Conn) acceptRemoteStream(id, service uint64) error {
	if id == 0 || id&1 == c.localIDParity {
		return ErrInvalidFrame
	}
	if c.cfg.AllowRemoteService != nil && !c.cfg.AllowRemoteService(service) {
		return fmt.Errorf("%w: %d", ErrServiceForbidden, service)
	}
	c.mu.Lock()
	if c.closed[service] {
		c.mu.Unlock()
		return giznet.ErrServiceMuxClosed
	}
	if c.streams[id] != nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: duplicate stream %d", ErrInvalidState, id)
	}
	stream := newVirtualStream(c, id, service)
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

func (c *Conn) listenerLocked(service uint64) *serviceListener {
	if listener := c.svcs[service]; listener != nil {
		return listener
	}
	listener := &serviceListener{
		conn:    c,
		service: service,
		accept:  make(chan net.Conn, c.cfg.ServiceQueueSize),
		closeCh: make(chan struct{}),
	}
	c.svcs[service] = listener
	return listener
}

func (c *Conn) addStreamLocked(stream *virtualStream) {
	c.streams[stream.id] = stream
	if c.bySvc[stream.service] == nil {
		c.bySvc[stream.service] = make(map[uint64]*virtualStream)
	}
	c.bySvc[stream.service][stream.id] = stream
}

func (c *Conn) removeStream(id uint64) {
	c.mu.Lock()
	stream := c.streams[id]
	if stream != nil {
		delete(c.streams, id)
		delete(c.bySvc[stream.service], id)
	}
	c.mu.Unlock()
}

func (c *Conn) send(typ frameType, payload []byte) error {
	if err := c.validate(); err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := writeFrame(c.stream, typ, payload, c.cfg.MaxFrameSize); err != nil {
		return err
	}
	c.touch()
	return nil
}

func (c *Conn) touch() {
	if c != nil {
		c.lastSeen.Store(time.Now().UnixNano())
	}
}

func (c *Conn) reserve(size int, stop <-chan struct{}) error {
	if size < 0 || int64(size) > c.cfg.MaxBufferedBytes {
		return ErrBufferLimit
	}
	for {
		c.bufferMu.Lock()
		current := c.buffered.Load()
		if int64(size) <= c.cfg.MaxBufferedBytes-current {
			c.buffered.Add(int64(size))
			c.bufferMu.Unlock()
			return nil
		}
		if c.bufferWake == nil {
			c.bufferWake = make(chan struct{})
		}
		wake := c.bufferWake
		c.bufferMu.Unlock()
		select {
		case <-wake:
		case <-stop:
			return io.ErrClosedPipe
		case <-c.closeCh:
			return c.err()
		}
	}
}

func (c *Conn) release(size int) {
	if size > 0 {
		c.bufferMu.Lock()
		c.buffered.Add(-int64(size))
		if c.bufferWake != nil {
			close(c.bufferWake)
			c.bufferWake = nil
		}
		c.bufferMu.Unlock()
	}
}

func (c *Conn) closeWithError(err error, notify bool) error {
	if c == nil {
		return nil
	}
	var closeErr error
	c.closeOnce.Do(func() {
		if err == nil {
			err = giznet.ErrConnClosed
		}
		c.closeErr.Store(&err)
		if notify {
			c.writeMu.Lock()
			_ = writeFrame(c.stream, frameSessionClose, nil, c.cfg.MaxFrameSize)
			c.writeMu.Unlock()
		}
		close(c.closeCh)
		c.mu.Lock()
		streams := make([]*virtualStream, 0, len(c.streams))
		for _, stream := range c.streams {
			streams = append(streams, stream)
		}
		listeners := make([]*serviceListener, 0, len(c.svcs))
		for _, listener := range c.svcs {
			listeners = append(listeners, listener)
		}
		c.streams = make(map[uint64]*virtualStream)
		c.bySvc = make(map[uint64]map[uint64]*virtualStream)
		c.mu.Unlock()
		for _, stream := range streams {
			stream.abort()
		}
		for _, listener := range listeners {
			_ = listener.Close()
		}
		c.packets.close()
		closeErr = c.stream.Close()
	})
	return closeErr
}

func (c *Conn) err() error {
	if c == nil {
		return giznet.ErrNilConn
	}
	if ptr := c.closeErr.Load(); ptr != nil && *ptr != nil {
		return *ptr
	}
	return giznet.ErrConnClosed
}

type virtualStream struct {
	conn    *Conn
	id      uint64
	service uint64

	readMu    sync.Mutex
	readBuf   []byte
	readCh    chan []byte
	remoteCh  chan struct{}
	remoteOne sync.Once
	abortOne  sync.Once
	localOne  sync.Once
	deliverMu sync.Mutex
	writeMu   sync.Mutex

	deadlineMu    sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time
	readWake      chan struct{}
}

func newVirtualStream(conn *Conn, id, service uint64) *virtualStream {
	return &virtualStream{
		conn:     conn,
		id:       id,
		service:  service,
		readCh:   make(chan []byte, conn.cfg.StreamQueueSize),
		remoteCh: make(chan struct{}),
		readWake: make(chan struct{}),
	}
}

func (s *virtualStream) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	for len(s.readBuf) == 0 {
		deadline, wake := s.readDeadlineSnapshot()
		var timer *time.Timer
		var timerCh <-chan time.Time
		if !deadline.IsZero() {
			delay := time.Until(deadline)
			if delay <= 0 {
				return 0, os.ErrDeadlineExceeded
			}
			timer = time.NewTimer(delay)
			timerCh = timer.C
		}
		select {
		case data := <-s.readCh:
			s.readBuf = data
		case <-s.remoteCh:
			// Stream data and the following close frame are handled in
			// order, but both channels can be ready before this goroutine
			// is scheduled. Drain the final queued data before reporting
			// the orderly remote close.
			select {
			case data := <-s.readCh:
				s.readBuf = data
			default:
				stopTimer(timer)
				return 0, io.EOF
			}
		case <-s.conn.closeCh:
			stopTimer(timer)
			return 0, s.conn.err()
		case <-wake:
			stopTimer(timer)
			continue
		case <-timerCh:
			return 0, os.ErrDeadlineExceeded
		}
		stopTimer(timer)
	}
	n := copy(buf, s.readBuf)
	s.readBuf = s.readBuf[n:]
	s.conn.release(n)
	return n, nil
}

func (s *virtualStream) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	written := 0
	chunkSize := min(streamChunkSize, s.conn.cfg.MaxFrameSize-8)
	if chunkSize <= 0 {
		return 0, ErrFrameTooLarge
	}
	for written < len(data) {
		if s.writeDeadlineExceeded() {
			return written, os.ErrDeadlineExceeded
		}
		end := min(written+chunkSize, len(data))
		if err := s.conn.send(frameStreamData, encodeStreamData(s.id, data[written:end])); err != nil {
			return written, err
		}
		written = end
	}
	return written, nil
}

func (s *virtualStream) WriteBuffers(buffers net.Buffers) (int64, error) {
	if len(buffers) == 0 {
		return 0, nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	chunkSize := min(streamChunkSize, s.conn.cfg.MaxFrameSize-8)
	if chunkSize <= 0 {
		return 0, ErrFrameTooLarge
	}
	pooled := streamBufferPool.Get().(*[]byte)
	chunk := (*pooled)[:0]
	defer func() {
		*pooled = chunk[:0]
		streamBufferPool.Put(pooled)
	}()
	var written int64
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		if s.writeDeadlineExceeded() {
			return os.ErrDeadlineExceeded
		}
		if err := s.conn.send(frameStreamData, encodeStreamData(s.id, chunk)); err != nil {
			return err
		}
		written += int64(len(chunk))
		chunk = chunk[:0]
		return nil
	}
	for _, buffer := range buffers {
		for len(buffer) > 0 {
			count := min(len(buffer), chunkSize-len(chunk))
			chunk = append(chunk, buffer[:count]...)
			buffer = buffer[count:]
			if len(chunk) == chunkSize {
				if err := flush(); err != nil {
					return written, err
				}
			}
		}
	}
	if err := flush(); err != nil {
		return written, err
	}
	return written, nil
}

func (s *virtualStream) Close() error {
	var err error
	s.localOne.Do(func() {
		err = s.conn.send(frameStreamClose, encodeStreamID(s.id))
		s.conn.removeStream(s.id)
		s.abort()
	})
	return err
}

func (s *virtualStream) LocalAddr() net.Addr {
	return tunnelAddr("giztunnel:local")
}

func (s *virtualStream) RemoteAddr() net.Addr {
	return tunnelAddr("giztunnel:remote")
}

func (s *virtualStream) SetDeadline(deadline time.Time) error {
	if err := s.SetReadDeadline(deadline); err != nil {
		return err
	}
	return s.SetWriteDeadline(deadline)
}

func (s *virtualStream) SetReadDeadline(deadline time.Time) error {
	s.deadlineMu.Lock()
	s.readDeadline = deadline
	close(s.readWake)
	s.readWake = make(chan struct{})
	s.deadlineMu.Unlock()
	return nil
}

func (s *virtualStream) SetWriteDeadline(deadline time.Time) error {
	s.deadlineMu.Lock()
	s.writeDeadline = deadline
	s.deadlineMu.Unlock()
	return nil
}

func (s *virtualStream) readDeadlineSnapshot() (time.Time, <-chan struct{}) {
	s.deadlineMu.Lock()
	defer s.deadlineMu.Unlock()
	return s.readDeadline, s.readWake
}

func (s *virtualStream) writeDeadlineExceeded() bool {
	s.deadlineMu.Lock()
	defer s.deadlineMu.Unlock()
	return !s.writeDeadline.IsZero() && !time.Now().Before(s.writeDeadline)
}

func stopTimer(timer *time.Timer) {
	if timer != nil {
		timer.Stop()
	}
}

func (s *virtualStream) deliver(data []byte) error {
	s.deliverMu.Lock()
	defer s.deliverMu.Unlock()

	copyData := append([]byte(nil), data...)
	if err := s.conn.reserve(len(copyData), s.remoteCh); err != nil {
		return err
	}
	select {
	case s.readCh <- copyData:
		return nil
	case <-s.remoteCh:
		s.conn.release(len(copyData))
		return io.ErrClosedPipe
	case <-s.conn.closeCh:
		s.conn.release(len(copyData))
		return s.conn.err()
	}
}

func (s *virtualStream) finishRemote() {
	s.remoteOne.Do(func() {
		close(s.remoteCh)
	})
}

func (s *virtualStream) abort() {
	s.finishRemote()
	s.abortOne.Do(func() {
		s.deliverMu.Lock()
		defer s.deliverMu.Unlock()

		s.readMu.Lock()
		s.conn.release(len(s.readBuf))
		s.readBuf = nil
		for {
			select {
			case data := <-s.readCh:
				s.conn.release(len(data))
			default:
				s.readMu.Unlock()
				return
			}
		}
	})
}

type serviceListener struct {
	conn     *Conn
	service  uint64
	accept   chan net.Conn
	closeCh  chan struct{}
	closeOne sync.Once
}

func (l *serviceListener) Accept() (net.Conn, error) {
	if l == nil || l.conn == nil {
		return nil, giznet.ErrNilConn
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
	if l == nil {
		return nil
	}
	l.closeOne.Do(func() { close(l.closeCh) })
	return nil
}

func (l *serviceListener) Addr() net.Addr {
	return tunnelAddr("giztunnel:listener")
}

type tunnelAddr string

func (a tunnelAddr) Network() string { return "giztunnel" }
func (a tunnelAddr) String() string  { return string(a) }

var (
	_ giznet.Conn                 = (*Conn)(nil)
	_ giznet.ServiceAcceptor      = (*Conn)(nil)
	_ giznet.ServiceAcceptEnabler = (*Conn)(nil)
	_ net.Conn                    = (*virtualStream)(nil)
)
