package gizwebrtc

const (
	SignalingPath = "/webrtc/v1/offer"

	serviceLabelPrefix = "giznet/v1/service/"
	packetLabel        = "giznet/v1/packet"

	// MediaStreamOpus mirrors gizclaw.MediaStreamOpus without importing
	// pkg/gizclaw from the transport package.
	MediaStreamOpus = "audio/opus"

	maxPacketMessageSize = 64 * 1024
	// Keep stream writes aligned with the 16 KiB GizTunnel frame size. The
	// previous 1400-byte application-level split multiplied every tunnel frame
	// into twelve SCTP messages and throttled sustained Edge throughput.
	streamChunkSize      = 16 * 1024
	streamWriteHighWater = 1 * 1024 * 1024
	streamWriteLowWater  = 256 * 1024
	readPacketQueueSize  = 256
	acceptQueueSize      = 64
	serviceQueueSize     = 64
)
