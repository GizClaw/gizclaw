package gizcli

const (
	ServiceRPC          uint64 = 0x00
	ServiceServerPublic uint64 = 0x01
	ServiceOpenAI       uint64 = 0x02
	ServiceAdmin        uint64 = 0x10
	ServiceAgentStream  uint64 = 0x20
	ServiceEvent               = ServiceAgentStream

	ProtocolEvent       byte = 0x03
	ProtocolStampedOpus byte = 0x10
	ProtocolTelemetry   byte = 0x11
)
