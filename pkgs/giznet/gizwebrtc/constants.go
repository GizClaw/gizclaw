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
	// One Edge upstream carries several logical sessions. Allow four 1 MiB
	// transfers to remain in flight before applying bounded backpressure so one
	// session cannot make every sibling wait for the low-water callback.
	streamWriteHighWater = 4 * 1024 * 1024
	streamWriteLowWater  = 1 * 1024 * 1024
	readPacketQueueSize  = 256
	acceptQueueSize      = 64
	serviceQueueSize     = 64
)
