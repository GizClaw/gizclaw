package gizwebrtc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const serviceOpenTimeout = 10 * time.Second

// ErrServiceOpen reports a DataChannel that closed or failed before becoming
// an attached service stream. It does not imply that the parent connection is
// terminal.
var ErrServiceOpen = errors.New("gizwebrtc: service open")

type Conn struct {
	pk giznet.PublicKey

	pc     *webrtc.PeerConnection
	policy giznet.SecurityPolicy

	localAddr  net.Addr
	remoteAddr net.Addr

	packetMu  sync.RWMutex
	packetDC  *webrtc.DataChannel
	packetRaw datachannel.ReadWriteCloserDeadliner
	audioUp   atomic.Bool

	serviceMu sync.Mutex
	services  map[uint64]*ServiceListener
	streams   map[uint64]map[*dataChannelConn]struct{}
	inbound   map[*webrtc.DataChannel]struct{}
	closedSvc map[uint64]bool
	acceptAll atomic.Bool
	serviceCh chan acceptedService

	readCh   chan directPacket
	readyCh  chan struct{}
	closeCh  chan struct{}
	once     sync.Once
	closed   atomic.Bool
	closeMu  sync.RWMutex
	closeErr error
	rxBytes  atomic.Uint64
	txBytes  atomic.Uint64

	audioTrack sampleWriter
}

type acceptedService struct {
	service uint64
	stream  net.Conn
}

type sampleWriter interface {
	WriteSample(media.Sample) error
}

func newConn(pk giznet.PublicKey, pc *webrtc.PeerConnection, policy giznet.SecurityPolicy, role string) (*Conn, error) {
	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:  MediaStreamOpus,
		ClockRate: 48000,
		Channels:  2,
	}, "giznet-opus", "giznet")
	if err != nil {
		return nil, err
	}
	rtpSender, err := pc.AddTrack(audioTrack)
	if err != nil {
		return nil, err
	}
	go func() {
		_ = drainRTCP(func(buffer []byte) error {
			_, _, err := rtpSender.Read(buffer)
			return err
		})
	}()
	c := &Conn{
		pk:         pk,
		pc:         pc,
		policy:     policy,
		localAddr:  addr("gizwebrtc:" + role + ":local"),
		remoteAddr: addr("gizwebrtc:" + role + ":remote"),
		services:   make(map[uint64]*ServiceListener),
		streams:    make(map[uint64]map[*dataChannelConn]struct{}),
		closedSvc:  make(map[uint64]bool),
		serviceCh:  make(chan acceptedService, serviceQueueSize),
		readCh:     make(chan directPacket, readPacketQueueSize),
		readyCh:    make(chan struct{}),
		closeCh:    make(chan struct{}),
		audioTrack: audioTrack,
	}
	pc.OnDataChannel(c.handleDataChannel)
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if strings.EqualFold(track.Codec().MimeType, MediaStreamOpus) {
			if !c.audioUp.CompareAndSwap(false, true) {
				_ = receiver.Stop()
				return
			}
			go c.readRemoteOpus(track)
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if peerConnectionStateIsTerminal(state) {
			_ = c.closeWithError(peerConnectionCloseError(pc, state))
		}
	})
	return c, nil
}

func drainRTCP(read func([]byte) error) error {
	if read == nil {
		return errors.New("gizwebrtc: nil RTCP reader")
	}
	buffer := make([]byte, 1500)
	for {
		if err := read(buffer); err != nil {
			return err
		}
	}
}

func peerConnectionStateIsTerminal(state webrtc.PeerConnectionState) bool {
	return state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed
}

func peerConnectionCloseError(pc *webrtc.PeerConnection, state webrtc.PeerConnectionState) error {
	if pc == nil || state != webrtc.PeerConnectionStateFailed {
		return fmt.Errorf("gizwebrtc: peer connection state %s", state)
	}
	report := pc.GetStats()
	pair, ok := selectedICECandidatePair(report)
	if !ok {
		return fmt.Errorf("gizwebrtc: peer connection state %s", state)
	}
	local, _ := report[pair.LocalCandidateID].(webrtc.ICECandidateStats)
	remote, _ := report[pair.RemoteCandidateID].(webrtc.ICECandidateStats)
	return fmt.Errorf(
		"gizwebrtc: peer connection state %s: ice_pair_state=%s nominated=%t "+
			"local=%s/%s remote=%s/%s packets_sent=%d packets_received=%d "+
			"requests_sent=%d responses_received=%d requests_received=%d responses_sent=%d "+
			"packets_discarded_on_send=%d",
		state,
		pair.State,
		pair.Nominated,
		local.CandidateType,
		local.Protocol,
		remote.CandidateType,
		remote.Protocol,
		pair.PacketsSent,
		pair.PacketsReceived,
		pair.RequestsSent,
		pair.ResponsesReceived,
		pair.RequestsReceived,
		pair.ResponsesSent,
		pair.PacketsDiscardedOnSend,
	)
}

func selectedICECandidatePair(report webrtc.StatsReport) (webrtc.ICECandidatePairStats, bool) {
	var selected webrtc.ICECandidatePairStats
	found := false
	for _, stat := range report {
		pair, ok := stat.(webrtc.ICECandidatePairStats)
		if !ok {
			continue
		}
		if !found || pair.Nominated && !selected.Nominated ||
			pair.Nominated == selected.Nominated &&
				pair.BytesSent+pair.BytesReceived > selected.BytesSent+selected.BytesReceived {
			selected = pair
			found = true
		}
	}
	return selected, found
}

// AcceptService accepts the next remotely opened service stream together with
// its service identifier. Callers must not mix this aggregate surface with
// ListenService on the same connection.
func (c *Conn) AcceptService() (uint64, net.Conn, error) {
	if err := c.validate(); err != nil {
		return 0, nil, err
	}
	c.acceptAll.Store(true)
	select {
	case accepted := <-c.serviceCh:
		return accepted.service, accepted.stream, nil
	case <-c.closeCh:
		return 0, nil, giznet.ErrConnClosed
	}
}

// EnableServiceAccept selects aggregate delivery for remotely opened streams.
func (c *Conn) EnableServiceAccept() {
	if c != nil {
		c.acceptAll.Store(true)
	}
}

func (c *Conn) Dial(service uint64) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceOpenTimeout)
	defer cancel()
	return c.DialContext(ctx, service)
}

// DialContext opens a service DataChannel and cancels the pending open when
// ctx completes without closing the parent PeerConnection.
func (c *Conn) DialContext(ctx context.Context, service uint64) (net.Conn, error) {
	if ctx == nil {
		return nil, errors.New("gizwebrtc: nil service-open context")
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.serviceMu.Lock()
	if c.closedSvc[service] {
		c.serviceMu.Unlock()
		return nil, giznet.ErrServiceMuxClosed
	}
	c.serviceMu.Unlock()
	dc, err := c.pc.CreateDataChannel(serviceLabel(service), &webrtc.DataChannelInit{})
	if err != nil {
		return nil, err
	}
	raw, err := detachWhenOpen(ctx, dc, c.closeCh, c.parentCloseError)
	if err != nil {
		_ = dc.Close()
		return nil, err
	}
	if err := c.validate(); err != nil {
		_ = raw.Close()
		_ = dc.Close()
		return nil, err
	}
	stream := newDataChannelConn(raw, dc, c.localAddr, c.remoteAddr)
	stream.rx = &c.rxBytes
	stream.tx = &c.txBytes
	if err := c.trackStream(service, stream, nil); err != nil {
		_ = stream.Close()
		_ = dc.Close()
		return nil, err
	}
	return stream, nil
}

func (c *Conn) parentCloseError() error {
	if err := c.closeError(); err != nil {
		return err
	}
	return giznet.ErrConnClosed
}

func (c *Conn) ListenService(service uint64) giznet.ServiceListener {
	if c == nil {
		return nil
	}
	c.serviceMu.Lock()
	defer c.serviceMu.Unlock()
	if l, ok := c.services[service]; ok {
		return l
	}
	l := newServiceListener(c, service)
	c.services[service] = l
	return l
}

func (c *Conn) CloseService(service uint64) error {
	if c == nil {
		return giznet.ErrNilConn
	}
	c.serviceMu.Lock()
	c.closedSvc[service] = true
	listener := c.services[service]
	streams := make([]*dataChannelConn, 0, len(c.streams[service]))
	for s := range c.streams[service] {
		streams = append(streams, s)
	}
	delete(c.streams, service)
	c.serviceMu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	for _, s := range streams {
		_ = s.Close()
	}
	return nil
}

func (c *Conn) Read(buf []byte) (byte, int, error) {
	if err := c.validate(); err != nil {
		return 0, 0, err
	}
	select {
	case pkt := <-c.readCh:
		if len(pkt.payload) > len(buf) {
			return 0, 0, giznet.ErrPacketBuffer
		}
		copy(buf, pkt.payload)
		c.rxBytes.Add(uint64(len(pkt.payload)))
		return pkt.protocol, len(pkt.payload), nil
	case <-c.closeCh:
		if err := c.closeError(); err != nil {
			return 0, 0, err
		}
		return 0, 0, giznet.ErrConnClosed
	}
}

func (c *Conn) Write(protocol byte, payload []byte) (int, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	if protocol == giznet.ProtocolOpusPacket {
		n, err := c.writeOpus(payload)
		if n > 0 {
			c.txBytes.Add(uint64(n))
		}
		return n, err
	}
	c.packetMu.RLock()
	raw := c.packetRaw
	c.packetMu.RUnlock()
	n, err := writePacket(raw, protocol, payload)
	if n > 0 {
		c.txBytes.Add(uint64(n))
	}
	return n, err
}

func (c *Conn) PublicKey() giznet.PublicKey {
	if c == nil {
		return giznet.PublicKey{}
	}
	return c.pk
}

func (c *Conn) PeerInfo() *giznet.PeerInfo {
	if c == nil {
		return nil
	}
	state := giznet.PeerStateEstablished
	if c.closed.Load() {
		state = giznet.PeerStateOffline
	}
	return &giznet.PeerInfo{
		PublicKey: c.pk,
		Endpoint:  c.remoteAddr,
		State:     state,
		RxBytes:   c.rxBytes.Load(),
		TxBytes:   c.txBytes.Load(),
		LastSeen:  time.Now(),
	}
}

func (c *Conn) Close() error {
	return c.close(nil, true)
}

func (c *Conn) closeWithError(cause error) error {
	return c.close(cause, false)
}

func (c *Conn) close(cause error, graceful bool) error {
	if c == nil {
		return giznet.ErrNilConn
	}
	var closeErr error
	c.once.Do(func() {
		c.closeMu.Lock()
		if cause != nil {
			c.closeErr = errors.Join(giznet.ErrConnClosed, cause)
		}
		c.closeMu.Unlock()
		c.closed.Store(true)
		close(c.closeCh)
		c.serviceMu.Lock()
		listeners := make([]*ServiceListener, 0, len(c.services))
		for _, listener := range c.services {
			listeners = append(listeners, listener)
		}
		var streams []*dataChannelConn
		for _, serviceStreams := range c.streams {
			for s := range serviceStreams {
				streams = append(streams, s)
			}
		}
		c.streams = make(map[uint64]map[*dataChannelConn]struct{})
		c.serviceMu.Unlock()
		for _, listener := range listeners {
			_ = listener.Close()
		}
		for _, s := range streams {
			_ = s.Close()
		}
		c.packetMu.Lock()
		if c.packetDC != nil {
			_ = c.packetDC.Close()
		}
		if c.packetRaw != nil {
			_ = c.packetRaw.Close()
		}
		c.packetMu.Unlock()
		if graceful {
			closeErr = c.pc.GracefulClose()
		} else {
			closeErr = c.pc.Close()
		}
	})
	return closeErr
}

func (c *Conn) closeError() error {
	if c == nil {
		return nil
	}
	c.closeMu.RLock()
	defer c.closeMu.RUnlock()
	return c.closeErr
}

func (c *Conn) validate() error {
	if c == nil || c.pc == nil {
		return giznet.ErrNilConn
	}
	if c.closed.Load() {
		if err := c.closeError(); err != nil {
			return err
		}
		return giznet.ErrConnClosed
	}
	return nil
}

func (c *Conn) handleDataChannel(dc *webrtc.DataChannel) {
	label := dc.Label()
	if label == packetLabel && !c.reservePacketDataChannel(dc) {
		_ = dc.Close()
		return
	}
	if label == packetLabel {
		dc.OnOpen(func() {
			raw, err := dc.DetachWithDeadline()
			if err != nil {
				_ = dc.Close()
				return
			}
			c.setPacket(dc, raw)
		})
		return
	}
	service, ok := parseServiceLabel(label)
	if !ok || c.policy != nil && !c.policy.AllowService(c.pk, service) {
		_ = dc.Close()
		return
	}
	release, ok := c.reserveInboundServiceStream(dc)
	if !ok {
		_ = dc.Close()
		return
	}
	dc.OnClose(release)
	dc.OnOpen(func() {
		raw, err := dc.DetachWithDeadline()
		if err != nil {
			release()
			_ = dc.Close()
			return
		}
		c.serviceMu.Lock()
		if c.closedSvc[service] {
			c.serviceMu.Unlock()
			release()
			_ = raw.Close()
			return
		}
		l := c.services[service]
		if l == nil {
			l = newServiceListener(c, service)
			c.services[service] = l
		}
		c.serviceMu.Unlock()
		stream := newDataChannelConn(raw, dc, c.localAddr, c.remoteAddr)
		stream.rx = &c.rxBytes
		stream.tx = &c.txBytes
		// A detached channel closes through raw.Close, which does not invoke
		// the Pion wrapper's OnClose callback. Bind the admission release to
		// the service stream itself so every unary RPC drops its DataChannel.
		if err := c.trackStream(service, stream, release); err != nil {
			release()
			_ = stream.Close()
			return
		}
		if c.acceptAll.Load() {
			select {
			case c.serviceCh <- acceptedService{service: service, stream: stream}:
			case <-c.closeCh:
				release()
				_ = stream.Close()
			}
			return
		}
		if err := l.enqueue(stream); err != nil {
			release()
		}
	})
}

func (c *Conn) reserveInboundServiceStream(dc *webrtc.DataChannel) (func(), bool) {
	c.serviceMu.Lock()
	if c.inbound == nil {
		c.inbound = make(map[*webrtc.DataChannel]struct{})
	}
	if len(c.inbound) >= maxInboundServiceStreams {
		c.serviceMu.Unlock()
		return nil, false
	}
	if _, exists := c.inbound[dc]; exists {
		c.serviceMu.Unlock()
		return nil, false
	}
	c.inbound[dc] = struct{}{}
	c.serviceMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.serviceMu.Lock()
			delete(c.inbound, dc)
			c.serviceMu.Unlock()
		})
	}, true
}

func (c *Conn) reservePacketDataChannel(dc *webrtc.DataChannel) bool {
	c.packetMu.Lock()
	defer c.packetMu.Unlock()
	if c.packetDC != nil {
		return false
	}
	c.packetDC = dc
	dc.OnClose(func() {
		_ = c.closeWithError(errors.New("gizwebrtc: packet data channel closed"))
	})
	dc.OnError(func(err error) {
		_ = c.closeWithError(fmt.Errorf("gizwebrtc: packet data channel: %w", err))
	})
	return true
}

func (c *Conn) setPacket(dc *webrtc.DataChannel, raw datachannel.ReadWriteCloserDeadliner) {
	c.packetMu.Lock()
	if c.packetDC != dc || c.packetRaw != nil {
		c.packetMu.Unlock()
		_ = raw.Close()
		return
	}
	c.packetRaw = raw
	c.packetMu.Unlock()
	close(c.readyCh)
	go c.readPacketLoop(raw)
}

func (c *Conn) readPacketLoop(raw datachannel.ReadWriteCloserDeadliner) {
	for {
		pkt, err := readPacket(raw)
		if err != nil {
			if errors.Is(err, errPacketProtocolIgnored) {
				continue
			}
			_ = c.closeWithError(fmt.Errorf("gizwebrtc: read packet data channel: %w", err))
			return
		}
		c.enqueuePacket(pkt)
	}
}

func (c *Conn) enqueuePacket(pkt directPacket) {
	select {
	case c.readCh <- pkt:
	case <-c.closeCh:
	}
}

func (c *Conn) trackStream(service uint64, s *dataChannelConn, releaseInbound func()) error {
	c.serviceMu.Lock()
	defer c.serviceMu.Unlock()
	if c.closed.Load() {
		return c.parentCloseError()
	}
	if c.closedSvc[service] {
		return giznet.ErrServiceMuxClosed
	}
	if c.streams[service] == nil {
		c.streams[service] = make(map[*dataChannelConn]struct{})
	}
	s.onClose = func() {
		if releaseInbound != nil {
			releaseInbound()
		}
		c.untrackStream(service, s)
	}
	c.streams[service][s] = struct{}{}
	return nil
}

func (c *Conn) untrackStream(service uint64, s *dataChannelConn) {
	c.serviceMu.Lock()
	defer c.serviceMu.Unlock()
	delete(c.streams[service], s)
	if len(c.streams[service]) == 0 {
		delete(c.streams, service)
	}
}

func (c *Conn) writeOpus(payload []byte) (int, error) {
	if len(payload) == 0 {
		return 0, fmt.Errorf("gizwebrtc: empty opus packet")
	}
	ticks := codecconv.OpusPacketRTPTicks(payload)
	duration := time.Duration(ticks) * time.Second / 48000
	if err := c.audioTrack.WriteSample(media.Sample{Data: payload, Duration: duration}); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (c *Conn) readRemoteOpus(track *webrtc.TrackRemote) {
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			_ = c.closeWithError(fmt.Errorf("gizwebrtc: read remote opus: %w", err))
			return
		}
		c.enqueueRemoteOpusFrame(pkt.Payload)
	}
}

func (c *Conn) enqueueRemoteOpusFrame(frame []byte) {
	c.enqueuePacket(directPacket{protocol: giznet.ProtocolOpusPacket, payload: append([]byte(nil), frame...)})
}

func serviceLabel(service uint64) string {
	return serviceLabelPrefix + strconv.FormatUint(service, 10)
}

func parseServiceLabel(label string) (uint64, bool) {
	if !strings.HasPrefix(label, serviceLabelPrefix) {
		return 0, false
	}
	service, err := strconv.ParseUint(strings.TrimPrefix(label, serviceLabelPrefix), 10, 64)
	return service, err == nil
}

type serviceDataChannel interface {
	OnOpen(func())
	OnClose(func())
	OnError(func(error))
	DetachWithDeadline() (datachannel.ReadWriteCloserDeadliner, error)
	Close() error
}

type serviceOpenResult struct {
	raw datachannel.ReadWriteCloserDeadliner
	err error
}

type serviceOpenError struct {
	event string
	cause error
}

func (e *serviceOpenError) Error() string {
	return ErrServiceOpen.Error() + ": " + e.event
}

func (e *serviceOpenError) Unwrap() []error {
	return []error{ErrServiceOpen, e.cause}
}

func newServiceOpenError(event string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: %s", ErrServiceOpen, event)
	}
	return &serviceOpenError{event: event, cause: cause}
}

func detachWhenOpen(
	ctx context.Context,
	dc serviceDataChannel,
	parentClose <-chan struct{},
	parentCloseError func() error,
) (datachannel.ReadWriteCloserDeadliner, error) {
	resultCh := make(chan serviceOpenResult, 1)
	var resultOnce sync.Once
	complete := func(result serviceOpenResult) bool {
		won := false
		resultOnce.Do(func() {
			won = true
			resultCh <- result
		})
		return won
	}
	dc.OnOpen(func() {
		raw, err := dc.DetachWithDeadline()
		if err != nil {
			complete(serviceOpenResult{err: newServiceOpenError("data channel detach failed", err)})
			return
		}
		if !complete(serviceOpenResult{raw: raw}) {
			_ = raw.Close()
		}
	})
	dc.OnClose(func() {
		complete(serviceOpenResult{err: fmt.Errorf("%w: data channel closed before open", ErrServiceOpen)})
	})
	dc.OnError(func(err error) {
		complete(serviceOpenResult{err: newServiceOpenError("data channel error", err)})
	})

	select {
	case result := <-resultCh:
		return result.raw, result.err
	case <-ctx.Done():
		complete(serviceOpenResult{err: ctx.Err()})
	case <-parentClose:
		err := giznet.ErrConnClosed
		if parentCloseError != nil {
			err = parentCloseError()
		}
		complete(serviceOpenResult{err: err})
	}
	result := <-resultCh
	return result.raw, result.err
}

var _ giznet.ContextDialer = (*Conn)(nil)
