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
	// GatewaySCTPWriteBudgetSize bounds reliable bytes outstanding across all
	// tunnel service channels on one upstream SCTP association.
	GatewaySCTPWriteBudgetSize = GatewaySCTPReceiveBufferSize
	// A lost SCTP INIT or COOKIE flight otherwise waits Pion's one-second
	// default before retrying. Keep only the handshake timers short; established
	// DATA/T3 timers retain the RFC-compliant association defaults.
	sctpHandshakeRetransmissionTimeoutMax = 150 * time.Millisecond
	// A lost DTLS flight otherwise waits Pion's one-second default before
	// retrying. A shorter interval keeps burst establishment bounded while
	// retaining DTLS's retransmission and exponential-backoff behavior.
	dtlsRetransmissionInterval = 150 * time.Millisecond
	// The ICE agent checks candidate pairs every 200 ms. Pion's default of seven
	// requests can permanently discard an otherwise valid relay pair during a
	// 1,000-session synchronized burst. Keep retrying for about five seconds;
	// the caller's Dial context remains the authoritative overall bound.
	iceMaxBindingRequests = 25
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
	maxInboundServiceStreams  = 2048
	maxNativeChannelLabelSize = 512
	maxInboundNativeChannels  = 8192
)
