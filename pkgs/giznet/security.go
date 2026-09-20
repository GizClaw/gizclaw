package giznet

import "context"

// PeerAdmission contains the authenticated transport identity and an optional
// opaque credential. Credential is borrowed for the duration of AllowPeer;
// policies must not retain it. The transport assigns no meaning to its bytes.
type PeerAdmission struct {
	PublicKey  PublicKey
	Credential []byte
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
