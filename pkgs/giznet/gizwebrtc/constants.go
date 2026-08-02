package gizwebrtc

const (
	SignalingPath = "/webrtc/v1/offer"

	serviceLabelPrefix = "giznet/v1/service/"
	packetLabel        = "giznet/v1/packet"

	// MediaStreamOpus mirrors gizclaw.MediaStreamOpus without importing
	// pkg/gizclaw from the transport package.
	MediaStreamOpus = "audio/opus"

	maxPacketMessageSize = 64 * 1024
	// Keep stream writes large enough to carry a 32 KiB RPC payload with few
	// SCTP messages while staying below the unstable maximum message boundary.
	// The previous 1400-byte split multiplied every tunnel frame into dozens of
	// SCTP messages and throttled sustained Edge throughput.
	streamChunkSize = 32 * 1024
	// BufferedAmount is scoped to one DataChannel, while several service
	// DataChannels share one SCTP association. Keep each channel's queue small
	// enough that a burst cannot hide tens of MiB behind one association.
	streamWriteHighWater = 512 * 1024
	streamWriteLowWater  = 128 * 1024
	readPacketQueueSize  = 256
	acceptQueueSize      = 64
	serviceQueueSize     = 64
)
