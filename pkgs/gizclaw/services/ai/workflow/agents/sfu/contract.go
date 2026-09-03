// Package sfu implements the provider-neutral SFU Workspace driver. The
// Workspace bridges one authenticated Peer's GenX audio stream to the SFU Room
// declared by its Social resource. LiveKit is the first connector.
package sfu

import (
	"context"
	"errors"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
)

// Type is the Workflow driver name registered with AgentHost.
const Type = "sfu"

var (
	// ErrNotMember reports that the Peer is not a current member of the
	// Workspace's Social resource. Callers must fail closed.
	ErrNotMember = errors.New("sfu: peer is not a member of the workspace")
	// ErrNotBound reports that the Workspace has no SFU binding.
	ErrNotBound = errors.New("sfu: workspace has no SFU binding")
	// ErrRevoked reports that the binding generation changed or the Social
	// resource entered retirement while the runtime was attached.
	ErrRevoked = errors.New("sfu: workspace access revoked")
	// ErrDuplicateIdentity reports that the SFU disconnected this participant
	// because the same Peer joined from another Server. The runtime must
	// terminate without reconnecting.
	ErrDuplicateIdentity = errors.New("sfu: participant replaced by a newer connection")
)

// Config is the Server-level SFU connector configuration. API credentials
// never leave the Server process.
type Config struct {
	URL       string
	APIKey    string
	APISecret string
	// RecheckInterval bounds how often an attached runtime re-validates its
	// binding against the authoritative Social KV.
	RecheckInterval time.Duration
	// ReconnectTimeout bounds reconnection attempts after a network error or
	// SFU restart before the runtime reports failure.
	ReconnectTimeout time.Duration
	// TalkHangover closes the Peer's open talk utterance once no voiced Opus
	// frame arrived from the device for this long. It is the only uplink
	// voice activity rule: an utterance opens on the first voiced frame and
	// closes on the device's EOS or on this hangover.
	TalkHangover time.Duration
	// FloorIdle releases the downlink floor once the holding participant
	// delivered no voiced packet for this long, so a stalled or silent holder
	// cannot keep every other participant muted.
	FloorIdle time.Duration
}

// BindingResolver resolves the authoritative SFU binding for one Workspace
// and verifies the Peer's current membership.
type BindingResolver interface {
	ResolveSFUWorkspaceBinding(ctx context.Context, workspaceID, peerPublicKey string) (socialutil.SFUWorkspaceBinding, error)
}
