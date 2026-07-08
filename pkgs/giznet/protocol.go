package giznet

const (
	// ProtocolServiceStream identifies the giznet reliable service-stream lane.
	//
	// Service stream ids remain application-owned uint64 values carried by
	// Dial and ListenService. This protocol is not returned by Conn.Read.
	ProtocolServiceStream byte = 0x00

	// ProtocolStampedOpusPacket identifies stamped Opus direct packets.
	ProtocolStampedOpusPacket byte = 0x10
)

// Direct packet protocol byte registry:
//   - 0x00 through 0x3f are reserved for giznet well-known protocols.
//   - 0x40 through 0xff are available for application/custom protocols.
