package giztunnel

import (
	"fmt"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	packetHeaderSize       = 18
	defaultPacketQueueSize = 32
	maxPacketPayloadSize   = 64*1024 - packetHeaderSize - 1
)

type packetWriter interface {
	Write(protocol byte, payload []byte) (int, error)
}

type tunnelPacket struct {
	protocol byte
	payload  []byte
}

// PacketMux routes session-tagged tunnel packets over one physical Giznet
// packet channel.
type PacketMux struct {
	writer packetWriter

	writeMu sync.Mutex
	mu      sync.RWMutex
	closed  bool
	ends    map[SessionID]*packetEndpoint
}

func NewPacketMux(writer packetWriter) *PacketMux {
	return &PacketMux{writer: writer, ends: make(map[SessionID]*packetEndpoint)}
}

func (m *PacketMux) register(id SessionID) (*packetEndpoint, error) {
	if m == nil || m.writer == nil || id.IsZero() {
		return nil, ErrInvalidState
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, giznet.ErrConnClosed
	}
	if _, ok := m.ends[id]; ok {
		return nil, ErrSessionExists
	}
	end := &packetEndpoint{
		id:      id,
		mux:     m,
		readCh:  make(chan tunnelPacket, defaultPacketQueueSize),
		closeCh: make(chan struct{}),
	}
	m.ends[id] = end
	return end, nil
}

// HandlePacket delivers one physical ProtocolTunnelPacket payload.
func (m *PacketMux) HandlePacket(payload []byte) error {
	if len(payload) < packetHeaderSize {
		return ErrInvalidFrame
	}
	if payload[0] != Version {
		return fmt.Errorf("%w: packet version %d", ErrInvalidFrame, payload[0])
	}
	var id SessionID
	copy(id[:], payload[1:17])
	inner := payload[17]
	if inner == giznet.ProtocolTunnelPacket || (inner != giznet.ProtocolOpusPacket && inner < 0x40) {
		return giznet.ErrPacketProtocol
	}
	m.mu.RLock()
	end := m.ends[id]
	m.mu.RUnlock()
	if end == nil {
		return ErrSessionNotFound
	}
	packet := tunnelPacket{protocol: inner, payload: append([]byte(nil), payload[packetHeaderSize:]...)}
	select {
	case end.readCh <- packet:
		return nil
	case <-end.closeCh:
		return ErrSessionNotFound
	default:
		end.close()
		return ErrBufferLimit
	}
}

func (m *PacketMux) write(id SessionID, protocol byte, payload []byte) (int, error) {
	if protocol == giznet.ProtocolTunnelPacket || (protocol != giznet.ProtocolOpusPacket && protocol < 0x40) {
		return 0, giznet.ErrPacketProtocol
	}
	if len(payload) > maxPacketPayloadSize {
		return 0, giznet.ErrPacketTooLarge
	}
	frame := make([]byte, packetHeaderSize+len(payload))
	frame[0] = Version
	copy(frame[1:17], id[:])
	frame[17] = protocol
	copy(frame[packetHeaderSize:], payload)
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if _, err := m.writer.Write(giznet.ProtocolTunnelPacket, frame); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (m *PacketMux) unregister(id SessionID, end *packetEndpoint) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.ends[id] == end {
		delete(m.ends, id)
	}
	m.mu.Unlock()
}

func (m *PacketMux) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	ends := make([]*packetEndpoint, 0, len(m.ends))
	for _, end := range m.ends {
		ends = append(ends, end)
	}
	m.ends = make(map[SessionID]*packetEndpoint)
	m.mu.Unlock()
	for _, end := range ends {
		end.close()
	}
	return nil
}

type packetEndpoint struct {
	id  SessionID
	mux *PacketMux

	readCh    chan tunnelPacket
	closeCh   chan struct{}
	closeOnce sync.Once
}

func (e *packetEndpoint) read(buf []byte) (byte, int, error) {
	if e == nil {
		return 0, 0, giznet.ErrNilConn
	}
	select {
	case packet := <-e.readCh:
		if len(packet.payload) > len(buf) {
			return 0, 0, giznet.ErrPacketBuffer
		}
		copy(buf, packet.payload)
		return packet.protocol, len(packet.payload), nil
	case <-e.closeCh:
		return 0, 0, io.EOF
	}
}

func (e *packetEndpoint) write(protocol byte, payload []byte) (int, error) {
	if e == nil || e.mux == nil {
		return 0, giznet.ErrNilConn
	}
	select {
	case <-e.closeCh:
		return 0, giznet.ErrConnClosed
	default:
		return e.mux.write(e.id, protocol, payload)
	}
}

func (e *packetEndpoint) close() {
	if e == nil {
		return
	}
	e.closeOnce.Do(func() {
		if e.mux != nil {
			e.mux.unregister(e.id, e)
		}
		close(e.closeCh)
	})
}
