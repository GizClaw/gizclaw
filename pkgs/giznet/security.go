package giznet

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"
)

// PeerAdmission contains the authenticated transport identity and an optional
// structured credential. Credential is nil when absent and borrowed read-only
// for AllowPeer; policies must not retain it. Only policies interpret its fields.
type PeerAdmission struct {
	PublicKey  PublicKey
	Credential *giznetpb.AdmissionCredential
}

// SecurityPolicy controls connection admission and service authorization.
type SecurityPolicy interface {
	AllowPeer(context.Context, PeerAdmission) bool
	AllowService(PublicKey, uint64) bool
}

type PeerEventHandler interface {
	HandlePeerEvent(PeerEvent)
}

type PeerEventHandleFunc func(PeerEvent)

func (f PeerEventHandleFunc) HandlePeerEvent(ev PeerEvent) {
	f(ev)
}
