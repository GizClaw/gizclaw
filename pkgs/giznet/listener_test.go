package giznet

import (
	"context"
	"testing"
)

type allowAllSecurityPolicy struct{}

func (allowAllSecurityPolicy) AllowPeer(context.Context, PeerAdmission) bool {
	return true
}

func (allowAllSecurityPolicy) AllowService(_ PublicKey, service uint64) bool {
	return service == 0
}

func TestPeerEventHandleFuncReceivesEvents(t *testing.T) {
	key, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	events := make(chan PeerEvent, 1)
	handler := PeerEventHandleFunc(func(ev PeerEvent) {
		events <- ev
	})

	ev := PeerEvent{PublicKey: key.Public, State: PeerStateOffline}
	handler.HandlePeerEvent(ev)

	if got := <-events; got != ev {
		t.Fatalf("event=%+v, want %+v", got, ev)
	}
}
