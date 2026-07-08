package gizwebrtc

import "github.com/GizClaw/gizclaw-go/pkgs/giznet"

const (
	SignalingPath = "/webrtc/v1/offer"

	serviceLabelPrefix = "giznet/v1/service/"
	packetLabel        = "giznet/v1/packet"

	// PacketStampedOpus is kept for compatibility. New code should use
	// giznet.ProtocolStampedOpusPacket.
	PacketStampedOpus byte = giznet.ProtocolStampedOpusPacket
	// EventStreamTelemetry is kept for compatibility with callers that used the
	// WebRTC transport package constant directly. gizwebrtc treats this as an
	// opaque application packet.
	//
	// Deprecated: use gizclaw.EventStreamTelemetry.
	EventStreamTelemetry byte = 0x40
	// MediaStreamOpus mirrors gizclaw.MediaStreamOpus without importing
	// pkg/gizclaw from the transport package.
	MediaStreamOpus = "audio/opus"

	maxPacketMessageSize = 64 * 1024
	streamChunkSize      = 1400
	streamWriteHighWater = 1 * 1024 * 1024
	streamWriteLowWater  = 256 * 1024
	readPacketQueueSize  = 256
	acceptQueueSize      = 64
	serviceQueueSize     = 64
)
