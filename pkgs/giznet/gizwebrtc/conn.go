package gizwebrtc

import (
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

type Conn struct {
	pk giznet.PublicKey

	pc     *webrtc.PeerConnection
	policy giznet.SecurityPolicy

	localAddr  net.Addr
	remoteAddr net.Addr

	packetMu  sync.RWMutex
	packetDC  *webrtc.DataChannel
	packetRaw datachannel.ReadWriteCloserDeadliner

	serviceMu sync.Mutex
	services  map[uint64]*ServiceListener
	streams   map[uint64]map[*dataChannelConn]struct{}
	closedSvc map[uint64]bool

	readCh  chan directPacket
	readyCh chan struct{}
	closeCh chan struct{}
	once    sync.Once
	closed  atomic.Bool

	audioTrack sampleWriter
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
	if _, err := pc.AddTrack(audioTrack); err != nil {
		return nil, err
	}
	c := &Conn{
		pk:         pk,
		pc:         pc,
		policy:     policy,
		localAddr:  addr("gizwebrtc:" + role + ":local"),
		remoteAddr: addr("gizwebrtc:" + role + ":remote"),
		services:   make(map[uint64]*ServiceListener),
		streams:    make(map[uint64]map[*dataChannelConn]struct{}),
		closedSvc:  make(map[uint64]bool),
		readCh:     make(chan directPacket, readPacketQueueSize),
		readyCh:    make(chan struct{}),
		closeCh:    make(chan struct{}),
		audioTrack: audioTrack,
	}
	pc.OnDataChannel(c.handleDataChannel)
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if strings.EqualFold(track.Codec().MimeType, MediaStreamOpus) {
			go c.readRemoteOpus(track)
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateDisconnected:
			_ = c.Close()
		}
	})
	return c, nil
}

func (c *Conn) Dial(service uint64) (net.Conn, error) {
	if err := c.validate(); err != nil {
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
	raw, err := detachWhenOpen(dc)
	if err != nil {
		_ = dc.Close()
		return nil, err
	}
	stream := newDataChannelConn(raw, dc, c.localAddr, c.remoteAddr)
	c.trackStream(service, stream)
	return stream, nil
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
	if l := c.services[service]; l != nil {
		_ = l.Close()
	}
	streams := make([]*dataChannelConn, 0, len(c.streams[service]))
	for s := range c.streams[service] {
		streams = append(streams, s)
	}
	delete(c.streams, service)
	c.serviceMu.Unlock()
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
		return pkt.protocol, len(pkt.payload), nil
	case <-c.closeCh:
		return 0, 0, giznet.ErrConnClosed
	}
}

func (c *Conn) Write(protocol byte, payload []byte) (int, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	if protocol == giznet.ProtocolOpusPacket {
		return c.writeOpus(payload)
	}
	c.packetMu.RLock()
	raw := c.packetRaw
	c.packetMu.RUnlock()
	return writePacket(raw, protocol, payload)
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
		LastSeen:  time.Now(),
	}
}

func (c *Conn) Close() error {
	if c == nil {
		return giznet.ErrNilConn
	}
	c.once.Do(func() {
		c.closed.Store(true)
		close(c.closeCh)
		c.serviceMu.Lock()
		for _, l := range c.services {
			_ = l.Close()
		}
		var streams []*dataChannelConn
		for _, serviceStreams := range c.streams {
			for s := range serviceStreams {
				streams = append(streams, s)
			}
		}
		c.streams = make(map[uint64]map[*dataChannelConn]struct{})
		c.serviceMu.Unlock()
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
		_ = c.pc.Close()
	})
	return nil
}

func (c *Conn) validate() error {
	if c == nil || c.pc == nil {
		return giznet.ErrNilConn
	}
	if c.closed.Load() {
		return giznet.ErrConnClosed
	}
	return nil
}

func (c *Conn) handleDataChannel(dc *webrtc.DataChannel) {
	label := dc.Label()
	if label == packetLabel {
		dc.OnClose(func() {
			_ = c.Close()
		})
	}
	dc.OnOpen(func() {
		raw, err := dc.DetachWithDeadline()
		if err != nil {
			_ = dc.Close()
			return
		}
		if label == packetLabel {
			c.setPacket(dc, raw)
			return
		}
		service, ok := parseServiceLabel(label)
		if !ok {
			_ = raw.Close()
			return
		}
		if c.policy != nil && !c.policy.AllowService(c.pk, service) {
			_ = raw.Close()
			return
		}
		c.serviceMu.Lock()
		if c.closedSvc[service] {
			c.serviceMu.Unlock()
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
		c.trackStream(service, stream)
		_ = l.enqueue(stream)
	})
}

func (c *Conn) setPacket(dc *webrtc.DataChannel, raw datachannel.ReadWriteCloserDeadliner) {
	c.packetMu.Lock()
	if c.packetRaw != nil {
		c.packetMu.Unlock()
		_ = raw.Close()
		return
	}
	c.packetDC = dc
	c.packetRaw = raw
	c.packetMu.Unlock()
	close(c.readyCh)
	go c.readPacketLoop(raw)
}

func (c *Conn) readPacketLoop(raw datachannel.ReadWriteCloserDeadliner) {
	for {
		pkt, err := readPacket(raw)
		if err != nil {
			_ = c.Close()
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

func (c *Conn) trackStream(service uint64, s *dataChannelConn) {
	c.serviceMu.Lock()
	defer c.serviceMu.Unlock()
	if c.streams[service] == nil {
		c.streams[service] = make(map[*dataChannelConn]struct{})
	}
	c.streams[service][s] = struct{}{}
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

func detachWhenOpen(dc *webrtc.DataChannel) (datachannel.ReadWriteCloserDeadliner, error) {
	ready := make(chan datachannel.ReadWriteCloserDeadliner, 1)
	errCh := make(chan error, 1)
	dc.OnOpen(func() {
		raw, err := dc.DetachWithDeadline()
		if err != nil {
			errCh <- err
			return
		}
		ready <- raw
	})
	select {
	case raw := <-ready:
		return raw, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("gizwebrtc: timeout waiting for data channel open")
	}
}
