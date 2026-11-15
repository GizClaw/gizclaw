package gizedge

import (
	"context"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestAdmissionRedirectUsesConfiguredOwnerWithoutAnotherLookup(t *testing.T) {
	home, foreign := testKeyPair(t, 0xb1).Public, testKeyPair(t, 0xb2).Public
	poolA, entryA := routeTestPool(t, &failingGiznetConn{state: giznet.PeerStateEstablished})
	readyConn := &routeRPCGiznetConn{failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished}}
	poolB, entryB := routeTestPool(t, readyConn)
	gateway := &Gateway{
		pools: map[giznet.PublicKey]*gatewayPool{foreign: poolA, home: poolB},
		resolvePeerRoute: func(context.Context, giznet.PublicKey) (*rpcpb.PeerAssignment, error) {
			t.Fatal("redirect repeated stale route lookup")
			return nil, nil
		},
	}
	entry, release, err := gateway.acquireRedirectUpstream(t.Context(), entryA, home)
	if err != nil || entry != entryB {
		t.Fatalf("redirect=%p %v, want configured owner %p", entry, err, entryB)
	}
	release()
	if readyConn.httpCalls.Load() != 1 || readyConn.calls.Load() != 0 {
		t.Fatal("redirect did not confirm control readiness independently of KV route lookup")
	}
	if entryB.active != 0 {
		t.Fatal("redirect leaked owner pool reservation")
	}
	for _, target := range []giznet.PublicKey{{}, foreign, testKeyPair(t, 0xb3).Public} {
		if _, release, err := gateway.acquireRedirectUpstream(t.Context(), entryA, target); err == nil || release != nil {
			t.Fatalf("redirect accepted zero/self/unconfigured target %s: %v", target, err)
		}
	}
	if entryA.active != 0 || entryB.active != 0 {
		t.Fatal("denied redirect changed pool reservations")
	}
}
