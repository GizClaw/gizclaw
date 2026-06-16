package gizcli

const (
	ServiceRPC          uint64 = 0x00
	ServiceServerPublic uint64 = 0x01
	ServiceOpenAI       uint64 = 0x02
	ServiceAdmin        uint64 = 0x10
	ServiceEvent        uint64 = 0x20

	ProtocolEvent       byte = 0x03
	ProtocolStampedOpus byte = 0x10
)
