package giztunnel

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	opusHeaderSize     = 1 + len(SessionID{})
	maxOpusPayloadSize = 64*1024 - opusHeaderSize
)

type packetWriter interface {
	Write(protocol byte, payload []byte) (int, error)
}

func encodeOpusPacket(id SessionID, payload []byte) ([]byte, error) {
	if id.IsZero() || len(payload) == 0 {
		return nil, ErrInvalidFrame
	}
	if len(payload) > maxOpusPayloadSize {
		return nil, giznet.ErrPacketTooLarge
	}
	frame := make([]byte, opusHeaderSize+len(payload))
	frame[0] = Version
	copy(frame[1:opusHeaderSize], id[:])
	copy(frame[opusHeaderSize:], payload)
	return frame, nil
}

func decodeOpusPacket(frame []byte) (SessionID, []byte, error) {
	if len(frame) <= opusHeaderSize || frame[0] != Version {
		return SessionID{}, nil, ErrInvalidFrame
	}
	var id SessionID
	copy(id[:], frame[1:opusHeaderSize])
	if id.IsZero() {
		return SessionID{}, nil, ErrInvalidFrame
	}
	return id, frame[opusHeaderSize:], nil
}

func (r *Router) HandlePacket(payload []byte) error {
	if r == nil {
		return ErrInvalidState
	}
	id, opus, err := decodeOpusPacket(payload)
	if err != nil {
		return err
	}
	r.mu.Lock()
	conn := r.sessions[id]
	r.mu.Unlock()
	if conn == nil || !conn.applicationAccepted() {
		return ErrSessionNotFound
	}
	return conn.enqueuePacket(giznet.ProtocolOpusPacket, opus)
}

func (r *Router) writeOpus(id SessionID, payload []byte) (int, error) {
	frame, err := encodeOpusPacket(id, payload)
	if err != nil {
		return 0, err
	}
	r.packetWriteMu.Lock()
	defer r.packetWriteMu.Unlock()
	if _, err := r.transport.Write(giznet.ProtocolTunnelPacket, frame); err != nil {
		return 0, fmt.Errorf("giztunnel: write opus packet: %w", err)
	}
	return len(payload), nil
}
