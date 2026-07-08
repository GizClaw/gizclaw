package giznet

import "errors"

var (
	ErrNilListener = errors.New("giznet: nil listener")
	ErrNilConn     = errors.New("giznet: nil conn")
	ErrClosed      = errors.New("giznet: listener closed")
	ErrConnClosed  = errors.New("giznet: conn closed")

	ErrPacketTooLarge = errors.New("giznet: packet too large")
	ErrPacketBuffer   = errors.New("giznet: packet buffer too small")
	ErrPacketProtocol = errors.New("giznet: invalid packet protocol")

	ErrNoSession         = errors.New("giznet: no established session")
	ErrPeerNotFound      = errors.New("giznet: peer not found")
	ErrAcceptQueueClosed = errors.New("giznet: accept queue closed")
	ErrServiceMuxClosed  = errors.New("giznet: service mux closed")
)
