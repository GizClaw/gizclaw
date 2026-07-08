package gizwebrtc

const (
	SignalingPath = "/webrtc/v1/offer"

	serviceLabelPrefix = "giznet/v1/service/"
	packetLabel        = "giznet/v1/packet"

	// PacketStampedOpus mirrors gizclaw.PacketStampedOpus without importing
	// pkg/gizclaw from the transport package.
	PacketStampedOpus byte = 0x10
	// EventStreamTelemetry mirrors gizclaw.EventStreamTelemetry without importing
	// pkg/gizclaw from the transport package.
	EventStreamTelemetry byte = 0x11
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
