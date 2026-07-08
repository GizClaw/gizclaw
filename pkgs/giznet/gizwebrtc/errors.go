package gizwebrtc

import (
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var (
	ErrNilListener      = giznet.ErrNilListener
	ErrNilConn          = giznet.ErrNilConn
	ErrClosed           = giznet.ErrClosed
	ErrConnClosed       = giznet.ErrConnClosed
	ErrPacketTooLarge   = giznet.ErrPacketTooLarge
	ErrPacketBuffer     = giznet.ErrPacketBuffer
	ErrPacketChannel    = errors.New("gizwebrtc: packet channel not ready")
	ErrInvalidLabel     = errors.New("gizwebrtc: invalid data channel label")
	ErrServiceClosed    = giznet.ErrServiceMuxClosed
	ErrSignalingReplay  = errors.New("gizwebrtc: replayed signaling nonce")
	ErrInvalidSDP       = errors.New("gizwebrtc: invalid sdp")
	ErrUnauthorized     = errors.New("gizwebrtc: unauthorized signaling request")
	ErrPeerForbidden    = errors.New("gizwebrtc: peer forbidden")
	ErrUnsupportedCodec = errors.New("gizwebrtc: missing opus audio")
)
