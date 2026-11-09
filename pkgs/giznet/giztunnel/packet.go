package giztunnel

import (
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/rtp"
)

const (
	opusHeaderSize     = 1 + len(SessionID{})
	maxOpusPayloadSize = 64*1024 - opusHeaderSize
	rtpPacketVersion   = 3
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

func encodeRTPPacket(id SessionID, payload []byte, info *packet.OpusInfo) ([]byte, error) {
	if id.IsZero() || info == nil || info.Missing || len(payload) == 0 || info.Header.Version != 2 || info.Header.PayloadType > 127 || len(info.Header.CSRC) > 15 {
		return nil, ErrInvalidFrame
	}
	duration := packet.OpusDuration48(payload)
	if duration == 0 || duration > 5760 {
		return nil, ErrInvalidFrame
	}
	// Only scalar fields change. MarshalTo borrows the header slices and payload
	// synchronously and copies them directly into the owned tunnel envelope.
	header := info.Header
	// ReadRTP already removed padding bytes; they are not part of the payload.
	header.Padding = false
	header.PaddingSize = 0
	if header.Extension && header.ExtensionProfile != rtp.ExtensionProfileOneByte && header.ExtensionProfile != rtp.ExtensionProfileTwoByte && len(header.Extensions) > 0 {
		// Pion serializes only the first opaque extension and rejects lengths
		// that are not whole words. Preserve that error before the size limit.
		first := header
		first.Extensions = first.Extensions[:1]
		if len(first.GetExtension(first.GetExtensionIDs()[0]))%4 != 0 {
			return nil, ErrInvalidFrame
		}
	}
	headerSize := header.MarshalSize()
	if headerSize > 64*1024-1-opusHeaderSize || len(payload) > 64*1024-1-opusHeaderSize-headerSize {
		return nil, giznet.ErrPacketTooLarge
	}
	frame := make([]byte, opusHeaderSize+headerSize+len(payload))
	frame[0] = rtpPacketVersion
	copy(frame[1:opusHeaderSize], id[:])
	if _, err := (&rtp.Packet{Header: header, Payload: payload}).MarshalTo(frame[opusHeaderSize:]); err != nil {
		return nil, ErrInvalidFrame
	}
	return frame, nil
}

func decodeRTPPacket(frame []byte) (SessionID, []byte, *packet.OpusInfo, error) {
	if len(frame) <= opusHeaderSize || len(frame) > 64*1024-1 || frame[0] != rtpPacketVersion {
		return SessionID{}, nil, nil, ErrInvalidFrame
	}
	var id SessionID
	copy(id[:], frame[1:opusHeaderSize])
	if id.IsZero() {
		return SessionID{}, nil, nil, ErrInvalidFrame
	}
	var media rtp.Packet
	if err := media.Unmarshal(frame[opusHeaderSize:]); err != nil || media.Version != 2 || len(media.Payload) == 0 {
		return SessionID{}, nil, nil, ErrInvalidFrame
	}
	duration := packet.OpusDuration48(media.Payload)
	if duration == 0 || duration > 5760 {
		return SessionID{}, nil, nil, ErrInvalidFrame
	}
	return id, media.Payload, &packet.OpusInfo{
		SourceID: fmt.Sprintf("%s/%08x", id.String(), media.SSRC),
		Header:   media.Header.Clone(), Duration48: duration,
	}, nil
}

func (r *Router) HandlePacket(payload []byte) error {
	if r == nil {
		return ErrInvalidState
	}
	var id SessionID
	var opus []byte
	var info *packet.OpusInfo
	var err error
	if len(payload) > 0 && payload[0] == rtpPacketVersion {
		id, opus, info, err = decodeRTPPacket(payload)
	} else {
		id, opus, err = decodeOpusPacket(payload)
	}
	if err != nil {
		return err
	}
	r.mu.Lock()
	conn := r.sessions[id]
	r.mu.Unlock()
	if conn == nil || !conn.applicationAccepted() {
		return ErrSessionNotFound
	}
	if conn.declaration.RTPAudio != (info != nil) {
		return ErrInvalidFrame
	}
	return conn.enqueueOpusPacket(opus, info)
}

func (r *Router) writeOpus(id SessionID, payload []byte) (int, error) {
	frame, err := encodeOpusPacket(id, payload)
	if err != nil {
		return 0, err
	}
	return r.writeMediaFrame(frame, len(payload))
}

func (r *Router) writeRTP(id SessionID, payload []byte, info *packet.OpusInfo) (int, error) {
	frame, err := encodeRTPPacket(id, payload, info)
	if err != nil {
		return 0, err
	}
	return r.writeMediaFrame(frame, len(payload))
}

func (r *Router) writeMediaFrame(frame []byte, payloadSize int) (int, error) {
	r.packetWriteMu.Lock()
	defer r.packetWriteMu.Unlock()
	if _, err := r.transport.Write(giznet.ProtocolTunnelPacket, frame); err != nil {
		return 0, fmt.Errorf("giztunnel: write opus packet: %w", err)
	}
	return payloadSize, nil
}
