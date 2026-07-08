package gizwebrtc

import (
	"errors"
)

var (
	ErrPacketChannel    = errors.New("gizwebrtc: packet channel not ready")
	ErrInvalidLabel     = errors.New("gizwebrtc: invalid data channel label")
	ErrSignalingReplay  = errors.New("gizwebrtc: replayed signaling nonce")
	ErrInvalidSDP       = errors.New("gizwebrtc: invalid sdp")
	ErrUnauthorized     = errors.New("gizwebrtc: unauthorized signaling request")
	ErrPeerForbidden    = errors.New("gizwebrtc: peer forbidden")
	ErrUnsupportedCodec = errors.New("gizwebrtc: missing opus audio")
)
