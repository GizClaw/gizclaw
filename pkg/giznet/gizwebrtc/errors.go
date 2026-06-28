package gizwebrtc

import "errors"

var (
	ErrNilListener      = errors.New("gizwebrtc: nil listener")
	ErrNilConn          = errors.New("gizwebrtc: nil conn")
	ErrClosed           = errors.New("gizwebrtc: listener closed")
	ErrConnClosed       = errors.New("gizwebrtc: conn closed")
	ErrPacketTooLarge   = errors.New("gizwebrtc: packet too large")
	ErrPacketBuffer     = errors.New("gizwebrtc: packet buffer too small")
	ErrPacketChannel    = errors.New("gizwebrtc: packet channel not ready")
	ErrInvalidLabel     = errors.New("gizwebrtc: invalid data channel label")
	ErrServiceClosed    = errors.New("gizwebrtc: service closed")
	ErrSignalingReplay  = errors.New("gizwebrtc: replayed signaling nonce")
	ErrInvalidSDP       = errors.New("gizwebrtc: invalid sdp")
	ErrUnauthorized     = errors.New("gizwebrtc: unauthorized signaling request")
	ErrPeerForbidden    = errors.New("gizwebrtc: peer forbidden")
	ErrUnsupportedCodec = errors.New("gizwebrtc: missing opus audio")
)
