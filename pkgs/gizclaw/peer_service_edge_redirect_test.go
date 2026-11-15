package gizclaw

import (
	"bytes"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/peerruntest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
)

func TestEdgeTunnelAdmissionRedirectsMissingReplicaToCommittedOwner(t *testing.T) {
	ctx := t.Context()
	key, home, foreign := giznet.PublicKey{71}, giznet.PublicKey{72}, giznet.PublicKey{73}
	peerStore, authority, replica := kv.NewMemory(nil), kv.NewMemory(nil), kv.NewMemory(nil)
	peers := &peer.Server{Store: peerStore, LocalRuns: peerruntest.New(t)}
	if _, err := peers.EnsureConnectedPeerGuarded(ctx, key, nil); err != nil {
		t.Fatal(err)
	}
	owner := &peerroute.Server{Store: authority, Peers: peers, ServerPublicKey: home, ServerEndpoint: "home:9820"}
	if _, err := owner.Assign(ctx, key, nil); err != nil {
		t.Fatal(err)
	}
	before, err := authority.Get(ctx, kv.Key{"by-peer", key.String()})
	if err != nil {
		t.Fatal(err)
	}
	view := bootstrapReadView{Store: kvfixture.ReadView(authority, replica, nil, nil), authority: authority}
	manager := NewManager(peers)
	manager.PeerRoutes = &peerroute.Server{Store: view, Peers: peers, ServerPublicKey: foreign, ServerEndpoint: "foreign:9820"}
	if _, err := manager.PeerRoutes.Lookup(ctx, key); !errors.Is(err, peerroute.ErrAssignmentNotFound) {
		t.Fatalf("replica unexpectedly sees assignment: %v", err)
	}
	host := &PeerConn{Service: &PeerService{manager: manager}}
	err = host.admitEdgeTunnelSession(ctx, giztunnel.SessionDeclaration{ClientPublicKey: key})
	redirect, ok := errors.AsType[*giztunnel.SessionRedirectError](err)
	if !ok || redirect.ServerPublicKey != home {
		t.Fatalf("admission=%v, want redirect to %s", err, home)
	}
	after, err := authority.Get(ctx, kv.Key{"by-peer", key.String()})
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("redirect changed fixed assignment: %v", err)
	}
	if _, err := manager.PeerRoutes.Lookup(ctx, key); !errors.Is(err, peerroute.ErrAssignmentNotFound) {
		t.Fatalf("redirect changed ordinary read policy: %v", err)
	}
	if _, online := manager.Peer(key); online {
		t.Fatal("redirect published foreign connection")
	}
	var count int
	if err := peers.LocalRuns.DB.QueryRowContext(ctx, "SELECT count(*) FROM peer_runs").Scan(&count); err != nil || count != 0 {
		t.Fatalf("redirect wrote local PeerRun: rows=%d err=%v", count, err)
	}
}

func TestEdgeTunnelAdmissionClaimsBeforeActivationWithoutPublishing(t *testing.T) {
	ctx := t.Context()
	key, home := giznet.PublicKey{74}, giznet.PublicKey{75}
	peers := &peer.Server{Store: kv.NewMemory(nil), LocalRuns: peerruntest.New(t)}
	manager := NewManager(peers)
	manager.PeerRoutes = &peerroute.Server{Store: kv.NewMemory(nil), Peers: peers, ServerPublicKey: home, ServerEndpoint: "home:9820"}
	host := &PeerConn{Service: &PeerService{manager: manager}}
	declaration := giztunnel.SessionDeclaration{ClientPublicKey: key}
	if err := host.admitEdgeTunnelSession(ctx, declaration); err != nil {
		t.Fatal(err)
	}
	assignment, err := manager.PeerRoutes.Lookup(ctx, key)
	if err != nil || assignment.ServerPublicKey != home.String() || assignment.Version != 1 {
		t.Fatalf("claim=%+v err=%v", assignment, err)
	}
	if _, online := manager.Peer(key); online {
		t.Fatal("pre-admission published connection before Event Stream")
	}
	keys, _, err := peers.LocalRuns.ListPeerPublicKeys(ctx, "", 10)
	if err != nil || len(keys) != 0 {
		t.Fatalf("pre-admission published local directory: %v %v", keys, err)
	}
	conn := &testGiznetConn{publicKey: key}
	if _, err := manager.activatePeer(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if current, online := manager.Peer(key); !online || current != conn {
		t.Fatal("normal activation did not publish admitted connection")
	}
}

func TestEdgeTunnelAdmissionRejectsBlockedAndDeletedPeers(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(map[bool]string{false: "blocked", true: "deleted"}[deleted], func(t *testing.T) {
			key := giznet.PublicKey{76}
			peers := &peer.Server{Store: kv.NewMemory(nil)}
			status := apitypes.PeerRegistrationStatusBlocked
			if deleted {
				status = apitypes.PeerRegistrationStatusActive
			}
			if _, err := peers.SavePeer(t.Context(), apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: status}); err != nil {
				t.Fatal(err)
			}
			if deleted {
				if err := peers.DeleteSelf(t.Context(), key); err != nil {
					t.Fatal(err)
				}
			}
			manager := NewManager(peers)
			manager.PeerRoutes = &peerroute.Server{Store: kv.NewMemory(nil), Peers: peers, ServerPublicKey: giznet.PublicKey{77}, ServerEndpoint: "home:9820"}
			host := &PeerConn{Service: &PeerService{manager: manager}}
			err := host.admitEdgeTunnelSession(t.Context(), giztunnel.SessionDeclaration{ClientPublicKey: key})
			want := peer.ErrPeerBlocked
			if deleted {
				want = peer.ErrPeerDeleted
			}
			if !errors.Is(err, want) {
				t.Fatalf("admission=%v, want %v", err, want)
			}
			if _, err := manager.PeerRoutes.Lookup(t.Context(), key); !errors.Is(err, peerroute.ErrAssignmentNotFound) {
				t.Fatalf("denied admission created route: %v", err)
			}
		})
	}
}
