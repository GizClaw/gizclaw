package giznet

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
)

// Conn is the transport-independent peer connection surface used by gizclaw.
type Conn interface {
	Dial(service uint64) (net.Conn, error)
	ListenService(service uint64) ServiceListener
	CloseService(service uint64) error

	Read([]byte) (protocol byte, n int, err error)
	Write(protocol byte, payload []byte) (int, error)

	PublicKey() PublicKey
	PeerInfo() *PeerInfo
	Close() error
}

// ContextDialer is the optional context-aware service-stream dialing surface.
// Transports implement it when a pending service open can be canceled without
// closing the parent connection.
type ContextDialer interface {
	DialContext(context.Context, uint64) (net.Conn, error)
}

// ServiceAcceptor is the optional aggregate service-stream accept surface used
// by transparent transports. It avoids one blocked goroutine per registered
// service while preserving Conn's per-service listener API.
type ServiceAcceptor interface {
	AcceptService() (service uint64, stream net.Conn, err error)
}

// ServiceAcceptEnabler selects aggregate service delivery before remote
// streams can race with the first AcceptService call.
type ServiceAcceptEnabler interface {
	EnableServiceAccept()
}

// PacketReader optionally retains media timing while reading direct packets.
// The returned OpusInfo belongs to the caller. Cancellation stops this read,
// without closing the connection or its reliable services.
type PacketReader interface {
	ReadPacket(context.Context, []byte) (protocol byte, n int, opus *packet.OpusInfo, err error)
}

// RTPAudioReader declares whether a connection carries timed Opus. Its value
// is fixed before acceptance, so consumers can select PCM boundaries before the
// first media packet. Legacy payload-only tunnel sessions return false.
type RTPAudioReader interface {
	PacketReader
	RTPAudio() bool
}

// PacketWriter optionally forwards encoded media with its original RTP timing.
// Payload and metadata are borrowed only until WritePacket returns.
type PacketWriter interface {
	WritePacket(protocol byte, payload []byte, opus *packet.OpusInfo) (int, error)
}

// ReadPacket uses the metadata-aware reader when available. Legacy connections
// keep their existing Read behavior and return no media timing.
func ReadPacket(ctx context.Context, conn Conn, buf []byte) (byte, int, *packet.OpusInfo, error) {
	if reader, ok := conn.(PacketReader); ok {
		return reader.ReadPacket(ctx, buf)
	}
	protocol, n, err := conn.Read(buf)
	return protocol, n, nil, err
}

// WritePacket uses the metadata-aware writer when available. Legacy transports
// keep their existing payload-only Write behavior.
func WritePacket(conn Conn, protocol byte, payload []byte, opus *packet.OpusInfo) (int, error) {
	if writer, ok := conn.(PacketWriter); ok {
		return writer.WritePacket(protocol, payload, opus)
	}
	return conn.Write(protocol, payload)
}
