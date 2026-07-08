package gizclaw

import "github.com/GizClaw/gizclaw-go/pkgs/giznet"

const (
	// PacketStampedOpus is kept for peer code compatibility. New transport-level
	// code should use giznet.ProtocolStampedOpusPacket.
	PacketStampedOpus byte = giznet.ProtocolStampedOpusPacket
)
