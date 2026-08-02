package gizwebrtc

import "time"

const (
	SignalingPath = "/webrtc/v1/offer"

	serviceLabelPrefix = "giznet/v1/service/"
	packetLabel        = "giznet/v1/packet"

	// MediaStreamOpus mirrors gizclaw.MediaStreamOpus without importing
	// pkg/gizclaw from the transport package.
	MediaStreamOpus = "audio/opus"

	maxPacketMessageSize = 64 * 1024
	// Message interleaving can leave partial messages queued across every active
	// DataChannel. Match the association receive credit to the qualified burst's
	// transferring streams and per-channel send budget; otherwise several
	// interleaved messages per stream can exhaust the receiver window before
	// delivery. This is one aggregate association limit, not a per-stream budget.
	sctpBurstServiceStreams = 64
	// GatewaySCTPReceiveBufferSize is reserved for bounded Edge-to-Server
	// upstream associations. Public client associations retain Pion's default
	// receive window.
	GatewaySCTPReceiveBufferSize = sctpBurstServiceStreams * streamWriteHighWater
	// SCTP's one-second initial retransmission is visible in burst DataChannel
	// setup when an INIT or COOKIE flight is lost. Cap the retry interval while
	// retaining reliable delivery and the existing retransmission count.
	sctpRetransmissionTimeoutMax = 250 * time.Millisecond
	// A lost DTLS flight otherwise waits Pion's one-second default before
	// retrying. A shorter interval keeps burst establishment bounded while
	// retaining DTLS's retransmission and exponential-backoff behavior.
	dtlsRetransmissionInterval = 250 * time.Millisecond
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
	// Bound remote service DataChannel admission per connection. This matches
	// the gateway's supported active-session ceiling per upstream association;
	// the SCTP receive window independently bounds aggregate queued bytes.
	maxInboundServiceStreams = 2048
)
