package gizcli

const (
	// ServicePeerRPC is the reliable peer RPC service stream.
	ServicePeerRPC uint64 = 0x00
	// ServicePeerHTTP is the reliable peer HTTP service stream.
	ServicePeerHTTP uint64 = 0x01
	// ServicePeerOpenAI is the reliable peer OpenAI-compatible HTTP service stream.
	ServicePeerOpenAI uint64 = 0x02
	// ServiceAdminHTTP is the reliable admin HTTP service stream.
	ServiceAdminHTTP uint64 = 0x10

	// EventStreamAgent is the reliable agent event stream.
	EventStreamAgent uint64 = 0x20
	// EventStreamTelemetry is the unreliable telemetry event packet.
	EventStreamTelemetry byte = 0x11

	// MediaStreamOpus is the WebRTC Opus media stream codec.
	MediaStreamOpus = "audio/opus"
	// PacketStampedOpus is the internal stamped Opus packet bridge.
	PacketStampedOpus byte = 0x10
)
