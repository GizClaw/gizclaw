package agenthost

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type requestCallerMux struct{ seen chan giznet.PublicKey }

func (m requestCallerMux) Transform(ctx context.Context, _ string, _ genx.Stream) (genx.Stream, error) {
	// A Workspace owner switch may occur downstream of the Peer runtime.
	ctx = ownership.WithOwner(ctx, giznet.PublicKey{3}.String())
	m.seen <- ownership.Caller(ctx)
	return newBlockingStream(), nil
}

func TestServiceReloadUsesConnectedPeerAsRequestCaller(t *testing.T) {
	peer, controller := giznet.PublicKey{1}, giznet.PublicKey{2}
	store := peerruntest.New(t)
	if _, err := store.SetRunAgent(t.Context(), peer, apitypes.AgentSelection{WorkspaceName: "chat"}); err != nil {
		t.Fatal(err)
	}
	seen := make(chan giznet.PublicKey, 2)
	svc := &Service{
		Host: requestCallerMux{seen: seen}, PeerRun: store, PublicKey: peer,
		Source:   StreamSourceFunc(func(context.Context) (genx.Stream, error) { return NewInputStream(1), nil }),
		Consumer: StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error { <-ctx.Done(); return ctx.Err() }),
	}
	defer svc.Stop(t.Context())
	for range 2 {
		if _, err := svc.Reload(ownership.WithCaller(t.Context(), controller)); err != nil {
			t.Fatal(err)
		}
		if got := <-seen; got != peer {
			t.Fatalf("provider caller = %v, want connected Peer %v", got, peer)
		}
	}
}
