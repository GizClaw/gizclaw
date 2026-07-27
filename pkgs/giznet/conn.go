package giznet

import (
	"net"
)

// Conn is the transport-independent peer connection surface used by gizclaw.
type Conn interface {
	Dial(service uint64) (net.Conn, error)
	ListenService(service uint64) ServiceListener
	CloseService(service uint64) error

	Read([]byte) (protocol byte, n int, err error)
	Write(protocol byte, payload []byte) (int, error)

	PublicKey() PublicKey
	PeerInfo() *PeerInfo
	Close() error
}

// ServiceAcceptor is the optional aggregate service-stream accept surface used
// by transparent transports. It avoids one blocked goroutine per registered
// service while preserving Conn's per-service listener API.
type ServiceAcceptor interface {
	AcceptService() (service uint64, stream net.Conn, err error)
}

// ServiceAcceptEnabler selects aggregate service delivery before remote
// streams can race with the first AcceptService call.
type ServiceAcceptEnabler interface {
	EnableServiceAccept()
}
