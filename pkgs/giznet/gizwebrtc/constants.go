package gizwebrtc

const (
	SignalingPath = "/webrtc/v1/offer"

	serviceLabelPrefix = "giznet/v1/service/"
	packetLabel        = "giznet/v1/packet"

	// ProtocolStampedOpus mirrors gizclaw.ProtocolStampedOpus without importing
	// pkg/gizclaw from the transport package.
	ProtocolStampedOpus byte = 0x10
	// ProtocolTelemetry mirrors gizclaw.ProtocolTelemetry without importing
	// pkg/gizclaw from the transport package.
	ProtocolTelemetry byte = 0x11

	maxPacketMessageSize = 64 * 1024
	streamChunkSize      = 1400
	streamWriteHighWater = 1 * 1024 * 1024
	streamWriteLowWater  = 256 * 1024
	readPacketQueueSize  = 256
	acceptQueueSize      = 64
	serviceQueueSize     = 64
)
